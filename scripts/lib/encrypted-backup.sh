#!/usr/bin/env bash

# Backup and credential paths are controlled by privileged deployment config.
# Reject a symlink at any component so a writable parent cannot redirect an
# otherwise safe-looking absolute path after review.
validate_no_symlink_path_components() {
  local candidate="$1"
  while [[ "$candidate" != / ]]; do
    [[ ! -L "$candidate" ]] || { echo "路径包含符号链接：$candidate" >&2; return 1; }
    candidate="$(dirname "$candidate")"
  done
}

validate_backup_directory() {
  local directory="$1" label="${2:-备份目录}"
  [[ "$directory" == /* ]] || { echo "$label 必须是绝对路径" >&2; return 1; }
  case "$directory" in
    /|/bin|/etc|/home|/opt|/root|/tmp|/usr|/var|/var/backups|/var/lib|/var/tmp|/Users)
      echo "拒绝使用过宽的${label}：$directory" >&2
      return 1
      ;;
  esac
  validate_no_symlink_path_components "$directory"
  mkdir -p -- "$directory"
  [[ -d "$directory" && ! -L "$directory" ]] || { echo "$label 不能是符号链接：$directory" >&2; return 1; }
}

# Recovery jobs deliberately write plaintext while validating or replaying a
# backup.  A path prefix alone is not evidence that the intended encrypted
# filesystem is mounted: after an unlock/mount failure the same path would be a
# normal directory on the root filesystem.  These wrappers are kept separate
# so cross-platform tests can replace the Linux mount/sysfs probes without
# weakening the production checks.
direct_luks_findmnt_record() {
  local mode="$1" path="$2"
  case "$mode" in
    mountpoint)
      findmnt --mountpoint "$path" --noheadings --raw --output SOURCE,FSTYPE,MAJ:MIN,OPTIONS
      ;;
    target)
      findmnt --target "$path" --noheadings --raw --output TARGET
      ;;
    *) return 1 ;;
  esac
}

direct_luks_dm_uuid() {
  local device_number="$1"
  local dm_uuid_file="/sys/dev/block/$device_number/dm/uuid"
  [[ -r "$dm_uuid_file" ]] || return 1
  head -n 1 -- "$dm_uuid_file"
}

direct_luks_stat() {
  local field="$1" path="$2"
  case "$field" in
    owner)
      stat -c '%u' "$path" 2>/dev/null || stat -f '%u' "$path"
      ;;
    mode)
      stat -c '%a' "$path" 2>/dev/null || stat -f '%Lp' "$path"
      ;;
    device)
      stat -c '%d' "$path" 2>/dev/null || stat -f '%d' "$path"
      ;;
    *) return 1 ;;
  esac
}

validate_direct_luks_mount() {
  local mount_root="$1" label="${2:-加密恢复挂载点}"
  local canonical_root mount_record mount_source mount_fstype mount_device_number mount_options extra
  local dm_uuid root_owner root_mode root_mode_value required_option

  [[ "$mount_root" == /* && -d "$mount_root" && ! -L "$mount_root" ]] || {
    echo "$label 必须是预先挂载的绝对目录：$mount_root" >&2
    return 1
  }
  validate_no_symlink_path_components "$mount_root" || return 1
  canonical_root="$(cd -P -- "$mount_root" && pwd -P)" || return 1
  [[ "$canonical_root" == "$mount_root" ]] || {
    echo "$label 必须使用无符号链接、.、.. 或重复斜线的规范路径：$mount_root" >&2
    return 1
  }

  mount_record="$(direct_luks_findmnt_record mountpoint "$mount_root" 2>/dev/null)" || {
    echo "$label 不是独立挂载点（禁止回退到系统盘）：$mount_root" >&2
    return 1
  }
  [[ "$mount_record" != *$'\n'* ]] || { echo "$label 挂载记录不唯一" >&2; return 1; }
  read -r mount_source mount_fstype mount_device_number mount_options extra <<<"$mount_record"
  [[ -n "$mount_source" && -n "$mount_fstype" && -n "$mount_device_number" && -n "$mount_options" && -z "${extra:-}" ]] || {
    echo "$label 挂载记录不完整" >&2
    return 1
  }
  [[ "$mount_source" =~ ^/dev/(mapper/[^/[:space:]]+|dm-[0-9]+)$ ]] || {
    echo "$label 必须直接挂载 dm-crypt 映射，当前来源：$mount_source" >&2
    return 1
  }
  case "$mount_fstype" in
    ext4|xfs) ;;
    *) echo "$label 只允许直接位于 dm-crypt 上的 ext4/xfs，当前类型：$mount_fstype" >&2; return 1 ;;
  esac
  [[ "$mount_device_number" =~ ^[0-9]+:[0-9]+$ ]] || { echo "$label 块设备编号无效" >&2; return 1; }
  dm_uuid="$(direct_luks_dm_uuid "$mount_device_number" 2>/dev/null)" || {
    echo "$label 无法从 /sys/dev/block/$mount_device_number/dm/uuid 确认 dm-crypt 身份" >&2
    return 1
  }
  [[ "$dm_uuid" == CRYPT-LUKS1-* || "$dm_uuid" == CRYPT-LUKS2-* ]] || {
    echo "$label 不是 LUKS1/LUKS2 映射" >&2
    return 1
  }
  for required_option in nodev nosuid noexec; do
    [[ ",$mount_options," == *",$required_option,"* ]] || {
      echo "$label 缺少挂载选项 $required_option" >&2
      return 1
    }
  done

  root_owner="$(direct_luks_stat owner "$mount_root")" || return 1
  root_mode="$(direct_luks_stat mode "$mount_root")" || return 1
  [[ "$root_owner" == 0 && "$root_mode" =~ ^[0-7]{3,4}$ ]] || {
    echo "$label 根目录必须由 root 所有" >&2
    return 1
  }
  root_mode_value=$((8#$root_mode))
  (( (root_mode_value & 022) == 0 )) || {
    echo "$label 根目录不能被组或其他用户修改" >&2
    return 1
  }
}

direct_luks_mount_for_directory() {
  local directory="$1" label="${2:-恢复目录}" mount_root directory_device mount_device
  [[ "$directory" == /* && -d "$directory" && ! -L "$directory" ]] || {
    echo "$label 必须是已存在的绝对目录：$directory" >&2
    return 1
  }
  validate_no_symlink_path_components "$directory" || return 1
  case "/${directory#/}/" in
    *//*|*/./*|*/../*)
      echo "$label 必须使用规范路径：$directory" >&2
      return 1
      ;;
  esac
  [[ "$directory" == / || "$directory" != */ ]] || {
    echo "$label 必须使用规范路径：$directory" >&2
    return 1
  }
  mount_root="$(direct_luks_findmnt_record target "$directory" 2>/dev/null)" || {
    echo "$label 找不到承载文件系统" >&2
    return 1
  }
  [[ "$mount_root" == /* && "$mount_root" != *$'\n'* ]] || { echo "$label 挂载点无效" >&2; return 1; }
  case "$directory" in
    "$mount_root"|"$mount_root"/*) ;;
    *) echo "$label 不在 findmnt 返回的挂载点内" >&2; return 1 ;;
  esac
  validate_direct_luks_mount "$mount_root" "$label 所在 LUKS 文件系统" || return 1
  directory_device="$(direct_luks_stat device "$directory")" || return 1
  mount_device="$(direct_luks_stat device "$mount_root")" || return 1
  [[ "$directory_device" == "$mount_device" ]] || {
    echo "$label 与已验证的 LUKS 挂载点不在同一文件系统" >&2
    return 1
  }
  printf '%s\n' "$mount_root"
}

validate_luks_service_directory() {
  local directory="$1" expected_mount="$2" label="${3:-恢复工作目录}"
  local actual_mount owner mode mode_value expected_owner
  actual_mount="$(direct_luks_mount_for_directory "$directory" "$label")" || return 1
  [[ "$actual_mount" == "$expected_mount" && "$directory" == "$expected_mount"/* ]] || {
    echo "$label 必须位于固定 LUKS 挂载点 $expected_mount 的私有子目录中" >&2
    return 1
  }
  owner="$(direct_luks_stat owner "$directory")" || return 1
  mode="$(direct_luks_stat mode "$directory")" || return 1
  expected_owner="${EUID:-$(id -u)}"
  [[ "$owner" == "$expected_owner" && "$mode" =~ ^[0-7]{3,4}$ ]] || {
    echo "$label 必须属于当前服务用户" >&2
    return 1
  }
  mode_value=$((8#$mode))
  (( mode_value == 0700 )) || {
    echo "$label 权限必须精确为 0700" >&2
    return 1
  }
}

# Plaintext backup material must never be staged on the persistent backup
# filesystem.  Each job gets an empty, service-owned directory on one fixed
# LUKS/dm-crypt mount; a missing mount must fail closed instead of silently
# writing into the underlying root filesystem.
validate_encrypted_work_directory() {
  local directory="$1" expected_directory="$2" label="${3:-备份明文工作目录}"
  local mount_root=/var/lib/wangzhe-backup-work
  local mode owner mode_value expected_owner directory_device mount_device root_mode root_owner root_mode_value
  local mount_source mount_fstype mount_device_number mount_options dm_uuid_file dm_uuid residual

  [[ "$directory" == "$expected_directory" && "$directory" == "$mount_root"/* ]] || {
    echo "$label 必须精确设置为 $expected_directory" >&2
    return 1
  }
  validate_no_symlink_path_components "$mount_root"
  validate_no_symlink_path_components "$directory"
  [[ -d "$mount_root" && ! -L "$mount_root" && -d "$directory" && ! -L "$directory" ]] || {
    echo "$label 或加密挂载点不存在（禁止自动创建）：$directory" >&2
    return 1
  }

  mount_source="$(findmnt --mountpoint "$mount_root" --noheadings --raw --output SOURCE 2>/dev/null | awk 'NR == 1 {print; exit}')"
  mount_fstype="$(findmnt --mountpoint "$mount_root" --noheadings --raw --output FSTYPE 2>/dev/null | awk 'NR == 1 {print; exit}')"
  mount_device_number="$(findmnt --mountpoint "$mount_root" --noheadings --raw --output MAJ:MIN 2>/dev/null | awk 'NR == 1 {print; exit}')"
  mount_options="$(findmnt --mountpoint "$mount_root" --noheadings --raw --output OPTIONS 2>/dev/null | awk 'NR == 1 {print; exit}')"
  [[ "$mount_source" =~ ^/dev/(mapper/[^/[:space:]]+|dm-[0-9]+)$ ]] || {
    echo "$mount_root 必须是独立的 dm-crypt 映射挂载，当前来源：${mount_source:-未挂载}" >&2
    return 1
  }
  case "$mount_fstype" in
    ext4|xfs) ;;
    *) echo "$mount_root 只允许使用直接位于 dm-crypt 上的 ext4/xfs，当前类型：${mount_fstype:-未知}" >&2; return 1 ;;
  esac
  [[ "$mount_device_number" =~ ^[0-9]+:[0-9]+$ ]] || { echo "无法确认 $mount_root 的块设备" >&2; return 1; }
  dm_uuid_file="/sys/dev/block/$mount_device_number/dm/uuid"
  [[ -r "$dm_uuid_file" ]] || { echo "无法读取 dm-crypt 设备身份：$dm_uuid_file" >&2; return 1; }
  read -r dm_uuid <"$dm_uuid_file" || { echo "无法读取 dm-crypt UUID" >&2; return 1; }
  [[ "$dm_uuid" == CRYPT-LUKS1-* || "$dm_uuid" == CRYPT-LUKS2-* ]] || {
    echo "$mount_root 不是 LUKS1/LUKS2 映射" >&2
    return 1
  }
  for required_option in nodev nosuid noexec; do
    [[ ",$mount_options," == *",$required_option,"* ]] || {
      echo "$mount_root 缺少挂载选项 $required_option" >&2
      return 1
    }
  done

  if stat -c '%a' "$directory" >/dev/null 2>&1; then
    mode="$(stat -c '%a' "$directory")"
    owner="$(stat -c '%u' "$directory")"
    directory_device="$(stat -c '%d' "$directory")"
    mount_device="$(stat -c '%d' "$mount_root")"
    root_mode="$(stat -c '%a' "$mount_root")"
    root_owner="$(stat -c '%u' "$mount_root")"
  else
    mode="$(stat -f '%Lp' "$directory")"
    owner="$(stat -f '%u' "$directory")"
    directory_device="$(stat -f '%d' "$directory")"
    mount_device="$(stat -f '%d' "$mount_root")"
    root_mode="$(stat -f '%Lp' "$mount_root")"
    root_owner="$(stat -f '%u' "$mount_root")"
  fi
  [[ "$root_owner" == 0 && "$root_mode" =~ ^[0-7]{3,4}$ ]] || { echo "加密工作盘根目录必须由 root 保护" >&2; return 1; }
  root_mode_value=$((8#$root_mode))
  (( (root_mode_value & 022) == 0 )) || { echo "加密工作盘根目录不能被组或其他用户修改" >&2; return 1; }
  expected_owner="${EUID:-$(id -u)}"
  [[ "$owner" == "$expected_owner" ]] || { echo "$label 必须属于当前服务用户" >&2; return 1; }
  [[ "$mode" =~ ^[0-7]{3,4}$ ]] || { echo "无法确认 $label 权限" >&2; return 1; }
  mode_value=$((8#$mode))
  (( (mode_value & 077) == 0 )) || { echo "$label 必须禁止组和其他用户访问（0700）" >&2; return 1; }
  [[ "$directory_device" == "$mount_device" ]] || { echo "$label 不在受信任的加密挂载上" >&2; return 1; }
  residual="$(find "$directory" -mindepth 1 -maxdepth 1 -print -quit)"
  [[ -z "$residual" ]] || {
    echo "$label 存在上次任务遗留，拒绝覆盖或自动删除：$residual" >&2
    return 1
  }
}

validate_age_recipient() {
  local recipient="$1" recipient_lower
  [[ "$recipient" == age1* && "$recipient" != *[[:space:]]* ]] || {
    echo "BACKUP_AGE_RECIPIENT 必须是无空白的 age X25519 公钥（age1...）" >&2
    return 1
  }
  recipient_lower="$(printf '%s' "$recipient" | tr '[:upper:]' '[:lower:]')"
  case "$recipient_lower" in
    *change*|*example*|*replace*) echo "BACKUP_AGE_RECIPIENT 仍是示例值" >&2; return 1 ;;
  esac
}

validate_remote_destination() {
  local destination="$1"
  [[ "$destination" =~ ^[A-Za-z0-9_.-]+:.+[^/]$ ]] || {
    echo "BACKUP_REMOTE_DESTINATION 必须是精确的 rclone remote:path，且不能只指向 remote 根" >&2
    return 1
  }
  [[ "$destination" != *[[:space:]]* && "$destination" != *'..'* ]] || {
    echo "BACKUP_REMOTE_DESTINATION 包含不安全路径" >&2
    return 1
  }
}

validate_rclone_config() {
  local config_file="$1"
  validate_private_file "$config_file" "rclone 配置"
}

validate_private_file() {
  local file="$1" label="${2:-敏感文件}" mode owner mode_value expected_owner
  [[ "$file" == /* && -f "$file" && ! -L "$file" ]] || {
    echo "$label 必须是非符号链接的绝对普通文件" >&2
    return 1
  }
  validate_no_symlink_path_components "$file" || return 1
  if stat -c '%a' "$file" >/dev/null 2>&1; then
    mode="$(stat -c '%a' "$file")"
    owner="$(stat -c '%u' "$file")"
  else
    mode="$(stat -f '%Lp' "$file")"
    owner="$(stat -f '%u' "$file")"
  fi
  [[ "$mode" =~ ^[0-7]{3,4}$ ]] || { echo "无法确认${label}权限" >&2; return 1; }
  mode_value=$((8#$mode))
  expected_owner="${EUID:-$(id -u)}"
  [[ "$owner" == "$expected_owner" ]] || { echo "${label}必须属于当前服务用户" >&2; return 1; }
  (( (mode_value & 077) == 0 )) || { echo "${label}不能被组或其他用户读取/修改" >&2; return 1; }
}

validate_ed25519_private_key() {
  local key_file="$1" label="${2:-Ed25519 签名私钥}"
  validate_private_file "$key_file" "$label" || return 1
  openssl pkey -in "$key_file" -noout -text 2>/dev/null | grep -q '^ED25519 Private-Key:' || {
    echo "$label 不是有效的 Ed25519 私钥" >&2
    return 1
  }
}

validate_ed25519_public_key() {
  local key_file="$1" label="${2:-Ed25519 验签公钥}" owner mode mode_value
  [[ "$key_file" == /* && -f "$key_file" && ! -L "$key_file" && -r "$key_file" ]] || {
    echo "$label 必须是可读的绝对普通文件" >&2
    return 1
  }
  validate_no_symlink_path_components "$key_file" || return 1
  if stat -c '%a' "$key_file" >/dev/null 2>&1; then
    mode="$(stat -c '%a' "$key_file")"
    owner="$(stat -c '%u' "$key_file")"
  else
    mode="$(stat -f '%Lp' "$key_file")"
    owner="$(stat -f '%u' "$key_file")"
  fi
  [[ "$owner" == 0 && "$mode" =~ ^[0-7]{3,4}$ ]] || { echo "$label 必须由 root 所有" >&2; return 1; }
  mode_value=$((8#$mode))
  (( (mode_value & 022) == 0 )) || { echo "$label 不能被非 root 修改" >&2; return 1; }
  openssl pkey -pubin -in "$key_file" -noout -text 2>/dev/null | grep -q '^ED25519 Public-Key:' || {
    echo "$label 不是有效的 Ed25519 公钥" >&2
    return 1
  }
}

validate_provenance_value() {
  local value="$1" label="$2"
  [[ -n "$value" && "$value" != *$'\n'* && "$value" != *$'\r'* && "$value" != *'='* ]] || {
    echo "$label 不能安全写入备份来源凭证" >&2
    return 1
  }
}

backup_provenance_field() {
  local file="$1" key="$2"
  awk -F= -v key="$key" '$1 == key { count++; value=substr($0, length(key) + 2) } END { if (count == 1) print value; else exit 1 }' "$file"
}

write_backup_provenance() {
  local target="$1" destination="$2" artifact_class="$3" source_id="$4" created_epoch="$5"
  local signing_key="$6" provenance_partial="$7" signature_partial="$8"
  local cipher_sha remote_object provenance_bytes signature_bytes
  validate_remote_destination "$destination" || return 1
  validate_provenance_value "$artifact_class" "制品类别" || return 1
  validate_provenance_value "$source_id" "制品来源标识" || return 1
  [[ "$artifact_class" =~ ^(database|uploads|pitr-basebackup|pitr-wal)$ ]] || { echo "制品类别无效" >&2; return 1; }
  [[ "$created_epoch" =~ ^[1-9][0-9]{0,11}$ ]] || { echo "制品创建时间无效" >&2; return 1; }
  validate_ed25519_private_key "$signing_key" "备份来源 Ed25519 签名私钥" || return 1
  [[ -f "$target" && ! -L "$target" && -s "$target" ]] || { echo "来源凭证目标制品无效" >&2; return 1; }
  [[ ! -e "$provenance_partial" && ! -L "$provenance_partial" && ! -e "$signature_partial" && ! -L "$signature_partial" ]] || {
    echo "备份来源凭证临时文件已存在" >&2
    return 1
  }
  cipher_sha="$(sha256sum "$target" | awk '{print $1}')"
  remote_object="${destination%/}/$(basename "$target")"
  validate_remote_destination "$remote_object" || return 1
  printf 'schema=wangzhe.backup-provenance.v1\nartifact_class=%s\nartifact_name=%s\nremote_object=%s\ncipher_sha256=%s\nsource_id=%s\ncreated_at_epoch=%s\n' \
    "$artifact_class" "$(basename "$target")" "$remote_object" "$cipher_sha" "$source_id" "$created_epoch" >"$provenance_partial"
  provenance_bytes="$(stat -c '%s' "$provenance_partial" 2>/dev/null || stat -f '%z' "$provenance_partial")"
  [[ "$provenance_bytes" =~ ^[0-9]+$ ]] && (( provenance_bytes >= 1 && provenance_bytes <= 4096 )) || {
    echo "备份来源凭证大小无效" >&2
    return 1
  }
  openssl pkeyutl -sign -rawin -inkey "$signing_key" -in "$provenance_partial" -out "$signature_partial"
  signature_bytes="$(stat -c '%s' "$signature_partial" 2>/dev/null || stat -f '%z' "$signature_partial")"
  [[ "$signature_bytes" == 64 ]] || { echo "备份来源 Ed25519 签名长度无效" >&2; return 1; }
  openssl pkeyutl -verify -rawin -inkey "$signing_key" -in "$provenance_partial" -sigfile "$signature_partial" >/dev/null || {
    echo "备份来源 Ed25519 签名自检失败" >&2
    return 1
  }
}

verify_backup_provenance() {
  local target="$1" expected_class="$2" expected_source_id="$3" expected_remote="$4" verify_key="$5" key_mode="${6:-public}"
  local provenance="$target.provenance" signature="$target.provenance.sig"
  local schema artifact_class artifact_name remote_object cipher_sha source_id created_epoch
  local actual_sha provenance_bytes signature_bytes now_epoch
  [[ -f "$provenance" && ! -L "$provenance" && -f "$signature" && ! -L "$signature" ]] || return 1
  provenance_bytes="$(stat -c '%s' "$provenance" 2>/dev/null || stat -f '%z' "$provenance")"
  signature_bytes="$(stat -c '%s' "$signature" 2>/dev/null || stat -f '%z' "$signature")"
  [[ "$provenance_bytes" =~ ^[0-9]+$ ]] && (( provenance_bytes >= 1 && provenance_bytes <= 4096 )) || return 1
  [[ "$signature_bytes" == 64 ]] || return 1
  case "$key_mode" in
    public)
      validate_ed25519_public_key "$verify_key" "备份来源 Ed25519 验签公钥" || return 1
      openssl pkeyutl -verify -pubin -rawin -inkey "$verify_key" -in "$provenance" -sigfile "$signature" >/dev/null || return 1
      ;;
    private)
      validate_ed25519_private_key "$verify_key" "备份来源 Ed25519 签名私钥" || return 1
      openssl pkeyutl -verify -rawin -inkey "$verify_key" -in "$provenance" -sigfile "$signature" >/dev/null || return 1
      ;;
    *) return 1 ;;
  esac
  schema="$(backup_provenance_field "$provenance" schema)" || return 1
  artifact_class="$(backup_provenance_field "$provenance" artifact_class)" || return 1
  artifact_name="$(backup_provenance_field "$provenance" artifact_name)" || return 1
  remote_object="$(backup_provenance_field "$provenance" remote_object)" || return 1
  cipher_sha="$(backup_provenance_field "$provenance" cipher_sha256)" || return 1
  source_id="$(backup_provenance_field "$provenance" source_id)" || return 1
  created_epoch="$(backup_provenance_field "$provenance" created_at_epoch)" || return 1
  actual_sha="$(sha256sum "$target" | awk '{print $1}')"
  now_epoch="$(date +%s)"
  [[ "$schema" == wangzhe.backup-provenance.v1 && "$artifact_class" == "$expected_class" ]] || return 1
  [[ "$artifact_name" == "$(basename "$target")" && "$remote_object" == "$expected_remote" ]] || return 1
  [[ "$cipher_sha" =~ ^[0-9a-f]{64}$ && "$cipher_sha" == "$actual_sha" && "$source_id" == "$expected_source_id" ]] || return 1
  [[ "$created_epoch" =~ ^[1-9][0-9]{0,11}$ ]] && (( created_epoch <= now_epoch + 300 ))
}

encrypt_backup_file() {
  local plaintext="$1" encrypted_partial="$2" recipient="$3"
  [[ -f "$plaintext" && ! -L "$plaintext" && -s "$plaintext" ]] || { echo "待加密备份无效" >&2; return 1; }
  [[ ! -e "$encrypted_partial" && ! -L "$encrypted_partial" ]] || { echo "拒绝覆盖加密临时文件" >&2; return 1; }
  age --encrypt --recipient "$recipient" --output "$encrypted_partial" "$plaintext"
  [[ -s "$encrypted_partial" ]] || { echo "加密备份为空" >&2; return 1; }
}

write_backup_checksum() {
  local target="$1" manifest_partial="$2"
  local digest
  digest="$(sha256sum "$target" | awk '{print $1}')"
  printf '%s  %s\n' "$digest" "$(basename "$target")" >"$manifest_partial"
}

resolve_backup_monitor_gid() {
  local monitor_group=wangzhe-monitor
  local group_record resolved_group _ resolved_gid _members
  [[ "$monitor_group" =~ ^[a-z_][a-z0-9_-]*[$]?$ ]] || {
    echo "备份监控组名无效" >&2
    return 1
  }
  group_record="$(getent group "$monitor_group")" || {
    echo "备份监控组不存在：$monitor_group" >&2
    return 1
  }
  [[ "$group_record" != *$'\n'* ]] || { echo "备份监控组解析结果不唯一" >&2; return 1; }
  IFS=: read -r resolved_group _ resolved_gid _members <<<"$group_record"
  [[ "$resolved_group" == "$monitor_group" && "$resolved_gid" =~ ^[0-9]+$ ]] || {
    echo "无法解析备份监控组：$monitor_group" >&2
    return 1
  }
  printf '%s\n' "$resolved_gid"
}

validate_backup_monitor_directory() {
  local directory="$1" resolved_gid parent_group parent_mode parent_mode_value parent_owner expected_owner
  resolved_gid="$(resolve_backup_monitor_gid)" || return 1
  validate_no_symlink_path_components "$directory"
  [[ -d "$directory" && ! -L "$directory" ]] || { echo "备份目录无效：$directory" >&2; return 1; }
  if stat -c '%a' "$directory" >/dev/null 2>&1; then
    parent_group="$(stat -c '%g' "$directory")"
    parent_mode="$(stat -c '%a' "$directory")"
    parent_owner="$(stat -c '%u' "$directory")"
  else
    parent_group="$(stat -f '%g' "$directory")"
    parent_mode="$(stat -f '%Lp' "$directory")"
    parent_owner="$(stat -f '%u' "$directory")"
  fi
  expected_owner="${EUID:-$(id -u)}"
  [[ "$parent_owner" == "$expected_owner" && "$parent_group" == "$resolved_gid" ]] || {
    echo "备份目录 owner 或监控组漂移：$directory" >&2
    return 1
  }
  [[ "$parent_mode" =~ ^[0-7]{3,4}$ ]] || { echo "无法确认备份目录权限" >&2; return 1; }
  parent_mode_value=$((8#$parent_mode))
  (( (parent_mode_value & 02000) != 0 && (parent_mode_value & 0022) == 0 )) || {
    echo "备份目录必须 setgid 且监控组/其他用户不可写：$directory" >&2
    return 1
  }
}

make_backup_artifacts_monitor_readable() {
  local resolved_gid artifact parent artifact_group artifact_owner expected_owner artifact_mode
  resolved_gid="$(resolve_backup_monitor_gid)" || return 1
  expected_owner="${EUID:-$(id -u)}"
  for artifact in "$@"; do
    [[ -f "$artifact" && ! -L "$artifact" ]] || {
      echo "备份监控制品无效：$artifact" >&2
      return 1
    }
    parent="$(dirname "$artifact")"
    validate_backup_monitor_directory "$parent" || return 1
    if stat -c '%a' "$parent" >/dev/null 2>&1; then
      artifact_group="$(stat -c '%g' "$artifact")"
      artifact_owner="$(stat -c '%u' "$artifact")"
    else
      artifact_group="$(stat -f '%g' "$artifact")"
      artifact_owner="$(stat -f '%u' "$artifact")"
    fi
    [[ "$artifact_group" == "$resolved_gid" ]] || {
      echo "备份制品监控组漂移：$artifact" >&2
      return 1
    }
    [[ "$artifact_owner" == "$expected_owner" ]] || { echo "备份制品 owner 漂移：$artifact" >&2; return 1; }
    # Backup directories are setgid to the dedicated monitor group.  Plaintext
    # partials retain umask 077; only encrypted artifacts and their evidence
    # are widened to group-read after successful validation.
    chmod 0640 -- "$artifact"
    if stat -c '%a' "$artifact" >/dev/null 2>&1; then
      artifact_mode="$(stat -c '%a' "$artifact")"
      artifact_group="$(stat -c '%g' "$artifact")"
    else
      artifact_mode="$(stat -f '%Lp' "$artifact")"
      artifact_group="$(stat -f '%g' "$artifact")"
    fi
    [[ "$artifact_mode" == 640 && "$artifact_group" == "$resolved_gid" ]] || {
      echo "备份制品监控权限设置失败：$artifact" >&2
      return 1
    }
  done
}

validate_offsite_marker() {
  local target="$1" expected_destination="${2:-}" marker checksum remote extra actual_checksum
  marker="$target.offsite-ok"
  [[ -f "$marker" && ! -L "$marker" ]] || return 1
  read -r checksum remote extra <"$marker" || return 1
  actual_checksum="$(sha256sum "$target" | awk '{print $1}')"
  [[ "$checksum" == "$actual_checksum" && -z "${extra:-}" ]] || return 1
  validate_remote_destination "$remote" >/dev/null 2>&1 || return 1
  [[ -z "$expected_destination" || "$remote" == "${expected_destination%/}/$(basename "$target")" ]]
}

sync_backup_offsite() {
  local target="$1" checksum_file="$2" destination="$3" marker_partial="$4" rclone_config="$5"
  local provenance_file="$6" signature_file="$7"
  local remote_target remote_checksum remote_provenance remote_signature actual_checksum observed_checksum
  local provenance_checksum remote_provenance_checksum signature_checksum remote_signature_checksum
  validate_remote_destination "$destination"
  validate_rclone_config "$rclone_config"
  remote_target="${destination%/}/$(basename "$target")"
  remote_checksum="$remote_target.sha256"
  remote_provenance="$remote_target.provenance"
  remote_signature="$remote_target.provenance.sig"
  rclone --config "$rclone_config" --contimeout 10s --timeout 2m --retries 2 --low-level-retries 2 copyto "$target" "$remote_target" --checksum --no-traverse
  rclone --config "$rclone_config" --contimeout 10s --timeout 2m --retries 2 --low-level-retries 2 copyto "$checksum_file" "$remote_checksum" --checksum --no-traverse
  rclone --config "$rclone_config" --contimeout 10s --timeout 2m --retries 2 --low-level-retries 2 copyto "$provenance_file" "$remote_provenance" --checksum --no-traverse
  rclone --config "$rclone_config" --contimeout 10s --timeout 2m --retries 2 --low-level-retries 2 copyto "$signature_file" "$remote_signature" --checksum --no-traverse

  actual_checksum="$(sha256sum "$target" | awk '{print $1}')"
  observed_checksum="$(rclone --config "$rclone_config" --contimeout 10s --timeout 2m --retries 2 --low-level-retries 2 hashsum sha256 "$remote_target" --download | awk 'NR == 1 {print $1}')"
  [[ "$observed_checksum" == "$actual_checksum" ]] || {
    echo "异机备份回读 SHA-256 不一致" >&2
    return 1
  }
  provenance_checksum="$(sha256sum "$provenance_file" | awk '{print $1}')"
  remote_provenance_checksum="$(rclone --config "$rclone_config" --contimeout 10s --timeout 2m --retries 2 --low-level-retries 2 hashsum sha256 "$remote_provenance" --download | awk 'NR == 1 {print $1}')"
  signature_checksum="$(sha256sum "$signature_file" | awk '{print $1}')"
  remote_signature_checksum="$(rclone --config "$rclone_config" --contimeout 10s --timeout 2m --retries 2 --low-level-retries 2 hashsum sha256 "$remote_signature" --download | awk 'NR == 1 {print $1}')"
  [[ "$remote_provenance_checksum" == "$provenance_checksum" && "$remote_signature_checksum" == "$signature_checksum" ]] || {
    echo "异机备份来源凭证或签名回读 SHA-256 不一致" >&2
    return 1
  }
  printf '%s  %s\n' "$actual_checksum" "$remote_target" >"$marker_partial"
}

validate_encrypted_backup_and_manifest() {
  local target="$1"
  [[ -f "$target" && ! -L "$target" && -s "$target" && -f "$target.sha256" && ! -L "$target.sha256" ]] || return 1
  local recorded_checksum recorded_name extra actual_checksum
  read -r recorded_checksum recorded_name extra <"$target.sha256" || return 1
  [[ "$recorded_checksum" =~ ^[0-9a-f]{64}$ && "$recorded_name" == "$(basename "$target")" && -z "${extra:-}" ]] || return 1
  actual_checksum="$(sha256sum "$target" | awk '{print $1}')"
  [[ "$actual_checksum" == "$recorded_checksum" ]]
}
