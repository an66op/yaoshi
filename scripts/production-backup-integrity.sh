#!/usr/bin/env bash
set -Eeuo pipefail

ENV_SOURCE="${1:-/etc/wangzhe/monitor.env}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/strict-env.sh
source "$SCRIPT_DIR/lib/strict-env.sh"
# shellcheck source=lib/encrypted-backup.sh
source "$SCRIPT_DIR/lib/encrypted-backup.sh"

if [[ "$ENV_SOURCE" != "--current-env" ]]; then
  load_strict_env "$ENV_SOURCE" '^MONITOR_[A-Z0-9_]+$'
fi

for command_name in awk basename cmp date dirname find grep head id jq mktemp mv openssl rclone rm sha256sum sort stat tail tr uniq wc; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "低频备份完整性检查缺少命令：$command_name" >&2; exit 1; }
done

required=(
  MONITOR_DATABASE_DBNAME MONITOR_DATABASE_BACKUP_DIR MONITOR_UPLOAD_BACKUP_DIR
  MONITOR_PITR_WAL_DIR MONITOR_PITR_BASEBACKUP_DIR MONITOR_RCLONE_CONFIG
  MONITOR_STATE_DIR MONITOR_BACKUP_INTEGRITY_STATUS_FILE
  MONITOR_BACKUP_PROVENANCE_VERIFY_KEY_FILE MONITOR_PITR_PROVENANCE_VERIFY_KEY_FILE
)
for key in "${required[@]}"; do
  [[ -n "${!key:-}" ]] || { echo "低频备份完整性配置缺少 $key" >&2; exit 1; }
done

[[ "$MONITOR_DATABASE_DBNAME" =~ ^[A-Za-z0-9_.-]+$ && "$MONITOR_DATABASE_DBNAME" != "." && "$MONITOR_DATABASE_DBNAME" != ".." ]] || {
  echo "MONITOR_DATABASE_DBNAME 不能用于安全的备份文件名" >&2
  exit 1
}
[[ "$MONITOR_DATABASE_BACKUP_DIR" == /var/backups/wangzhe/database ]] || {
  echo "数据库完整性检查目录必须固定为 /var/backups/wangzhe/database" >&2
  exit 1
}
[[ "$MONITOR_UPLOAD_BACKUP_DIR" == /var/backups/wangzhe/uploads ]] || {
  echo "uploads 完整性检查目录必须固定为 /var/backups/wangzhe/uploads" >&2
  exit 1
}
[[ "$MONITOR_PITR_WAL_DIR" =~ ^/var/backups/wangzhe/wal/([0-9]{10,30})$ ]] || {
  echo "WAL 完整性检查目录必须包含合法 PostgreSQL system_identifier" >&2
  exit 1
}
readonly pitr_cluster_id="${BASH_REMATCH[1]}"
[[ "$MONITOR_PITR_BASEBACKUP_DIR" == "/var/backups/wangzhe/base/$pitr_cluster_id" ]] || {
  echo "WAL 与基础备份目录必须绑定同一个 PostgreSQL system_identifier" >&2
  exit 1
}
[[ "$MONITOR_STATE_DIR" == /var/lib/wangzhe-monitor && "$MONITOR_BACKUP_INTEGRITY_STATUS_FILE" == "$MONITOR_STATE_DIR/last-backup-integrity.status" ]] || {
  echo "备份完整性成功状态必须固定在 /var/lib/wangzhe-monitor" >&2
  exit 1
}
validate_no_symlink_path_components "$MONITOR_STATE_DIR"
[[ -d "$MONITOR_STATE_DIR" && ! -L "$MONITOR_STATE_DIR" && -w "$MONITOR_STATE_DIR" ]] || {
  echo "备份完整性状态目录不存在、不可写或是符号链接" >&2
  exit 1
}

validate_rclone_config "$MONITOR_RCLONE_CONFIG"
[[ "$MONITOR_BACKUP_PROVENANCE_VERIFY_KEY_FILE" == /etc/wangzhe/backup-provenance-ed25519-public.pem ]] || {
  echo "数据库/上传来源验签公钥必须使用固定路径" >&2
  exit 1
}
[[ "$MONITOR_PITR_PROVENANCE_VERIFY_KEY_FILE" == /etc/wangzhe/pitr-provenance-ed25519-public.pem ]] || {
  echo "PITR 来源验签公钥必须使用固定路径" >&2
  exit 1
}
validate_ed25519_public_key "$MONITOR_BACKUP_PROVENANCE_VERIFY_KEY_FILE" "数据库/上传备份来源 Ed25519 公钥"
validate_ed25519_public_key "$MONITOR_PITR_PROVENANCE_VERIFY_KEY_FILE" "PITR 备份来源 Ed25519 公钥"
backup_key_fingerprint="$(openssl pkey -pubin -in "$MONITOR_BACKUP_PROVENANCE_VERIFY_KEY_FILE" -outform DER | sha256sum | awk '{print $1}')"
pitr_key_fingerprint="$(openssl pkey -pubin -in "$MONITOR_PITR_PROVENANCE_VERIFY_KEY_FILE" -outform DER | sha256sum | awk '{print $1}')"
[[ "$backup_key_fingerprint" =~ ^[0-9a-f]{64}$ && "$pitr_key_fingerprint" =~ ^[0-9a-f]{64}$ && "$backup_key_fingerprint" != "$pitr_key_fingerprint" ]] || {
  echo "数据库/上传与 PITR 必须使用两把不同的来源签名密钥" >&2
  exit 1
}
readonly monitor_uid="${EUID:-$(id -u)}"
monitor_gid="$(id -g)"
readonly monitor_gid
umask 077

stat_value() {
  strict_env_stat "$1" "$2" "$3"
}

validate_readonly_backup_directory() {
  local directory="$1" label="$2" mode mode_value owner group
  [[ "$directory" == /* && "$directory" != / ]] || { echo "$label 必须是非根绝对路径" >&2; return 1; }
  validate_no_symlink_path_components "$directory"
  [[ -d "$directory" && ! -L "$directory" && -r "$directory" && -x "$directory" ]] || {
    echo "$label 不是监控用户可读的真实目录：$directory" >&2
    return 1
  }
  mode="$(stat_value '%a' '%Lp' "$directory")"
  owner="$(stat_value '%u' '%u' "$directory")"
  group="$(stat_value '%g' '%g' "$directory")"
  [[ "$mode" =~ ^[0-7]{3,4}$ && "$owner" =~ ^[0-9]+$ && "$group" =~ ^[0-9]+$ ]] || {
    echo "$label 权限元数据无效" >&2
    return 1
  }
  mode_value=$((8#$mode))
  [[ "$owner" != "$monitor_uid" && "$group" == "$monitor_gid" ]] || {
    echo "$label 必须由备份服务拥有，并只授予监控组读取权限" >&2
    return 1
  }
  (( (mode_value & 02000) != 0 && (mode_value & 0050) == 0050 && (mode_value & 0027) == 0 )) || {
    echo "$label 必须保持 setgid、监控组只读且其他用户不可访问" >&2
    return 1
  }
}

validate_artifact_file() {
  local file="$1" label="$2" max_bytes="${3:-0}" mode owner group bytes
  [[ -f "$file" && ! -L "$file" && -r "$file" && -s "$file" ]] || {
    echo "$label 缺失、为空、不可读或是符号链接：$file" >&2
    return 1
  }
  validate_no_symlink_path_components "$file"
  mode="$(stat_value '%a' '%Lp' "$file")"
  owner="$(stat_value '%u' '%u' "$file")"
  group="$(stat_value '%g' '%g' "$file")"
  bytes="$(stat_value '%s' '%z' "$file")"
  [[ "$mode" == 640 && "$owner" =~ ^[0-9]+$ && "$owner" != "$monitor_uid" && "$group" == "$monitor_gid" ]] || {
    echo "$label 必须由备份服务拥有并保持 0640、监控组只读：$file" >&2
    return 1
  }
  [[ "$bytes" =~ ^[0-9]+$ ]] || { echo "$label 大小无效：$file" >&2; return 1; }
  if (( max_bytes > 0 )); then
    (( bytes >= 1 && bytes <= max_bytes )) || { echo "$label 超过安全大小上限：$file" >&2; return 1; }
  fi
}

validate_marker_name() {
  local kind="$1" marker_name="$2" suffix
  case "$kind" in
    database)
      [[ "$marker_name" == "$MONITOR_DATABASE_DBNAME"-* ]] || return 1
      suffix="${marker_name#"$MONITOR_DATABASE_DBNAME"-}"
      [[ "$suffix" =~ ^[0-9]{8}-[0-9]{6}-[0-9]+\.dump\.age\.offsite-ok$ ]]
      ;;
    uploads)
      [[ "$marker_name" =~ ^uploads-[0-9]{8}-[0-9]{6}-[0-9]+\.tar\.age\.offsite-ok$ ]]
      ;;
    basebackup)
      [[ "$marker_name" =~ ^basebackup-[0-9]{8}-[0-9]{6}-[0-9]+\.tar\.age\.offsite-ok$ ]]
      ;;
    wal)
      [[ "$marker_name" =~ ^([0-9A-F]{24}(\.[0-9A-F]{8}\.backup)?|[0-9A-F]{8}\.history)\.age\.offsite-ok$ ]]
      ;;
    *) return 1 ;;
  esac
}

latest_target=""
select_latest_published_artifact() {
  local directory="$1" marker_pattern="$2" kind="$3" label="$4"
  local marker marker_name marker_mtime target latest_mtime=-1 latest_name="" marker_output
  latest_target=""
  marker_output="$(find "$directory" -mindepth 1 -maxdepth 1 -name "$marker_pattern" -print)" || {
    echo "$label 无法完整枚举异机凭证" >&2
    return 1
  }
  [[ -n "$marker_output" ]] || { echo "$label 没有已完成异机回读的制品" >&2; return 1; }
  while IFS= read -r marker; do
    [[ "$(dirname "$marker")" == "$directory" ]] || { echo "$label 凭证越出受控目录" >&2; return 1; }
    marker_name="$(basename "$marker")"
    validate_marker_name "$kind" "$marker_name" || { echo "$label 凭证文件名异常：$marker_name" >&2; return 1; }
    validate_artifact_file "$marker" "$label 异机凭证" 4096 || return 1
    marker_mtime="$(stat_value '%Y' '%m' "$marker")"
    [[ "$marker_mtime" =~ ^[0-9]+$ ]] || { echo "$label 凭证时间无效" >&2; return 1; }
    target="${marker%.offsite-ok}"
    if (( marker_mtime > latest_mtime )) || { (( marker_mtime == latest_mtime )) && [[ "$marker_name" > "$latest_name" ]]; }; then
      latest_target="$target"
      latest_mtime="$marker_mtime"
      latest_name="$marker_name"
    fi
  done <<<"$marker_output"
  [[ -n "$latest_target" ]] || { echo "$label 无法选择最新制品" >&2; return 1; }
}

rclone_args=(
  --config "$MONITOR_RCLONE_CONFIG"
  --contimeout 10s --timeout 5m --retries 2 --low-level-retries 2
)
run_rclone() {
  rclone "${rclone_args[@]}" "$@"
}

verify_remote_evidence() {
  local remote="$1" local_file="$2" label="$3"
  local size_json remote_count remote_bytes local_bytes remote_content local_content
  validate_remote_destination "$remote"
  size_json="$(run_rclone size --json "$remote")" || { echo "$label 远端对象大小不可读" >&2; return 1; }
  remote_count="$(printf '%s' "$size_json" | jq -er '.count | numbers' 2>/dev/null)" || return 1
  remote_bytes="$(printf '%s' "$size_json" | jq -er '.bytes | numbers' 2>/dev/null)" || return 1
  local_bytes="$(stat_value '%s' '%z' "$local_file")"
  [[ "$remote_count" == 1 && "$remote_bytes" =~ ^[0-9]+$ && "$local_bytes" =~ ^[0-9]+$ ]] || {
    echo "$label 远端对象数量或大小格式无效" >&2
    return 1
  }
  (( local_bytes >= 1 && local_bytes <= 4096 && remote_bytes == local_bytes )) || {
    echo "$label 远端对象大小与本地证据不一致" >&2
    return 1
  }
  remote_content="$(run_rclone cat "$remote")" || { echo "$label 远端对象不可读" >&2; return 1; }
  local_content="$(<"$local_file")"
  [[ "$remote_content" == "$local_content" ]] || { echo "$label 远端内容与本地证据不一致" >&2; return 1; }
}

verify_remote_file_digest() {
  local remote="$1" local_file="$2" label="$3" max_bytes="${4:-4096}"
  local size_json remote_count remote_bytes local_bytes local_digest hash_output remote_digest remote_name remote_extra
  validate_remote_destination "$remote"
  size_json="$(run_rclone size --json "$remote")" || { echo "$label 远端对象大小不可读" >&2; return 1; }
  remote_count="$(printf '%s' "$size_json" | jq -er '.count | numbers' 2>/dev/null)" || return 1
  remote_bytes="$(printf '%s' "$size_json" | jq -er '.bytes | numbers' 2>/dev/null)" || return 1
  local_bytes="$(stat_value '%s' '%z' "$local_file")"
  [[ "$remote_count" == 1 && "$remote_bytes" =~ ^[0-9]+$ && "$local_bytes" =~ ^[0-9]+$ ]] || return 1
  (( local_bytes >= 1 && local_bytes <= max_bytes && remote_bytes == local_bytes )) || {
    echo "$label 远端对象大小与本地不一致" >&2
    return 1
  }
  local_digest="$(sha256sum "$local_file" | awk '{print $1}')"
  hash_output="$(run_rclone hashsum sha256 "$remote" --download)" || { echo "$label 远端对象无法完整回读" >&2; return 1; }
  [[ -n "$hash_output" && "$hash_output" != *$'\n'* ]] || { echo "$label 远端摘要输出不唯一" >&2; return 1; }
  read -r remote_digest remote_name remote_extra <<<"$hash_output"
  [[ "$remote_digest" == "$local_digest" && -n "$remote_name" && -z "${remote_extra:-}" ]] || {
    echo "$label 远端内容摘要与本地不一致" >&2
    return 1
  }
}

verified_remote_target=""
verify_ciphertext_and_evidence() {
  local target="$1" label="$2" artifact_class="$3" source_id="$4" verify_key="$5"
  local checksum_file="$target.sha256" marker_file="$target.offsite-ok"
  local provenance_file="$target.provenance" signature_file="$target.provenance.sig"
  local target_name local_digest recorded_digest recorded_name manifest_extra manifest_lines
  local marker_digest remote_target marker_extra marker_lines remote_path
  local remote_hash_output remote_digest remote_reported_name remote_extra
  validate_artifact_file "$target" "$label 密文" || return 1
  validate_artifact_file "$checksum_file" "$label SHA-256 清单" 4096 || return 1
  validate_artifact_file "$marker_file" "$label 异机凭证" 4096 || return 1
  validate_artifact_file "$provenance_file" "$label 来源凭证" 4096 || return 1
  validate_artifact_file "$signature_file" "$label 来源签名" 64 || return 1
  target_name="$(basename "$target")"

  manifest_lines="$(awk 'END {print NR}' "$checksum_file")"
  [[ "$manifest_lines" == 1 ]] || { echo "$label 本地 SHA-256 清单必须恰好一行" >&2; return 1; }
  read -r recorded_digest recorded_name manifest_extra <"$checksum_file" || return 1
  [[ "$recorded_digest" =~ ^[0-9a-f]{64}$ && "$recorded_name" == "$target_name" && -z "${manifest_extra:-}" ]] || {
    echo "$label 本地 SHA-256 清单格式无效" >&2
    return 1
  }
  local_digest="$(sha256sum "$target" | awk '{print $1}')"
  [[ "$local_digest" == "$recorded_digest" ]] || { echo "$label 本地完整 SHA-256 不一致" >&2; return 1; }

  marker_lines="$(awk 'END {print NR}' "$marker_file")"
  [[ "$marker_lines" == 1 ]] || { echo "$label 异机凭证必须恰好一行" >&2; return 1; }
  read -r marker_digest remote_target marker_extra <"$marker_file" || return 1
  [[ "$marker_digest" == "$local_digest" && -z "${marker_extra:-}" ]] || { echo "$label 异机凭证摘要无效" >&2; return 1; }
  validate_remote_destination "$remote_target"
  remote_path="${remote_target#*:}"
  [[ "$(basename "$remote_path")" == "$target_name" ]] || { echo "$label 异机目标文件名不匹配" >&2; return 1; }

  remote_hash_output="$(run_rclone hashsum sha256 "$remote_target" --download)" || {
    echo "$label 远端密文无法完成全量 SHA-256 回读" >&2
    return 1
  }
  [[ -n "$remote_hash_output" && "$remote_hash_output" != *$'\n'* ]] || { echo "$label 远端 SHA-256 输出不唯一" >&2; return 1; }
  read -r remote_digest remote_reported_name remote_extra <<<"$remote_hash_output"
  [[ "$remote_digest" == "$local_digest" && -n "$remote_reported_name" && -z "${remote_extra:-}" ]] || {
    echo "$label 远端完整 SHA-256 与本地不一致" >&2
    return 1
  }
  [[ "$(basename "$remote_reported_name")" == "$target_name" ]] || { echo "$label 远端 SHA-256 对象名不匹配" >&2; return 1; }
  verify_remote_evidence "$remote_target.sha256" "$checksum_file" "$label SHA-256 清单" || return 1
  verify_backup_provenance "$target" "$artifact_class" "$source_id" "$remote_target" "$verify_key" || {
    echo "$label 来源签名或绑定字段无效" >&2
    return 1
  }
  verify_remote_file_digest "$remote_target.provenance" "$provenance_file" "$label 来源凭证" 4096 || return 1
  verify_remote_file_digest "$remote_target.provenance.sig" "$signature_file" "$label 来源签名" 64 || return 1
  verified_remote_target="$remote_target"
}

verify_wal_source_manifest() {
  local target="$1" remote_target="$2" label="$3"
  local source_manifest="$target.source.sha256" source_lines source_digest source_wal_name source_cluster source_extra
  local expected_wal_name
  validate_artifact_file "$source_manifest" "$label 源 WAL 凭证" 4096 || return 1
  source_lines="$(awk 'END {print NR}' "$source_manifest")"
  [[ "$source_lines" == 1 ]] || { echo "$label 源 WAL 凭证必须恰好一行" >&2; return 1; }
  read -r source_digest source_wal_name source_cluster source_extra <"$source_manifest" || return 1
  expected_wal_name="$(basename "${target%.age}")"
  [[ "$source_digest" =~ ^[0-9a-f]{64}$ && "$source_wal_name" == "$expected_wal_name" && "$source_cluster" == "$pitr_cluster_id" && -z "${source_extra:-}" ]] || {
    echo "$label 源 WAL 摘要、文件名或集群绑定无效" >&2
    return 1
  }
  verify_remote_evidence "$remote_target.source.sha256" "$source_manifest" "$label 源 WAL 凭证"
}

validate_readonly_backup_directory "$MONITOR_DATABASE_BACKUP_DIR" "数据库备份目录"
validate_readonly_backup_directory "$MONITOR_UPLOAD_BACKUP_DIR" "uploads 备份目录"
validate_readonly_backup_directory "$MONITOR_PITR_BASEBACKUP_DIR" "基础备份目录"
validate_readonly_backup_directory "$MONITOR_PITR_WAL_DIR" "WAL 归档目录"

select_latest_published_artifact "$MONITOR_DATABASE_BACKUP_DIR" "${MONITOR_DATABASE_DBNAME}-*.dump.age.offsite-ok" database "数据库备份"
database_target="$latest_target"
verify_ciphertext_and_evidence "$database_target" "数据库备份" database "$MONITOR_DATABASE_DBNAME" "$MONITOR_BACKUP_PROVENANCE_VERIFY_KEY_FILE"

select_latest_published_artifact "$MONITOR_UPLOAD_BACKUP_DIR" 'uploads-*.tar.age.offsite-ok' uploads "uploads 备份"
upload_target="$latest_target"
verify_ciphertext_and_evidence "$upload_target" "uploads 备份" uploads /var/lib/wangzhe/uploads "$MONITOR_BACKUP_PROVENANCE_VERIFY_KEY_FILE"

select_latest_published_artifact "$MONITOR_PITR_BASEBACKUP_DIR" 'basebackup-*.tar.age.offsite-ok' basebackup "PITR 基础备份"
basebackup_target="$latest_target"
verify_ciphertext_and_evidence "$basebackup_target" "PITR 基础备份" pitr-basebackup "$pitr_cluster_id" "$MONITOR_PITR_PROVENANCE_VERIFY_KEY_FILE"

integrity_work="$(mktemp -d "$MONITOR_STATE_DIR/.wal-integrity.XXXXXX")"
integrity_status_partial=""
cleanup_integrity_work() {
  [[ -z "${integrity_status_partial:-}" ]] || rm -f -- "$integrity_status_partial"
  if [[ -n "${integrity_work:-}" && -d "$integrity_work" && ! -L "$integrity_work" && "$integrity_work" == "$MONITOR_STATE_DIR"/.wal-integrity.* ]]; then
    rm -rf -- "$integrity_work"
  fi
}
trap cleanup_integrity_work EXIT INT TERM
wal_targets_file="$integrity_work/wal-targets"
wal_inventory_file="$integrity_work/wal-inventory"
expected_remote_files="$integrity_work/expected-remote-files"
actual_remote_files="$integrity_work/actual-remote-files"
: >"$wal_targets_file"
: >"$wal_inventory_file"
: >"$expected_remote_files"
: >"$actual_remote_files"

while IFS= read -r wal_entry; do
  [[ -n "$wal_entry" ]] || continue
  if [[ "$wal_entry" == .archive.lock ]]; then
    continue
  fi
  [[ "$wal_entry" =~ ^([0-9A-F]{24}(\.[0-9A-F]{8}\.backup)?|[0-9A-F]{8}\.history)\.age(\.sha256|\.source\.sha256|\.offsite-ok|\.provenance(\.sig)?)?$ ]] || {
    echo "WAL 归档目录包含意外或未完成文件：$wal_entry" >&2
    exit 1
  }
done < <(find "$MONITOR_PITR_WAL_DIR" -xdev -mindepth 1 -maxdepth 1 -printf '%f\n' | LC_ALL=C sort)
unexpected_wal_type="$(find "$MONITOR_PITR_WAL_DIR" -xdev -mindepth 1 -maxdepth 1 ! -type f -print -quit)"
[[ -z "$unexpected_wal_type" ]] || { echo "WAL 归档目录包含目录、符号链接或特殊文件：$unexpected_wal_type" >&2; exit 1; }
find "$MONITOR_PITR_WAL_DIR" -xdev -maxdepth 1 -type f -name '*.age' -printf '%p\n' | LC_ALL=C sort >"$wal_targets_file"
wal_inventory_count="$(wc -l <"$wal_targets_file" | tr -d '[:space:]')"
[[ "$wal_inventory_count" =~ ^[1-9][0-9]*$ ]] || { echo "WAL 归档目录没有可验证的保留制品" >&2; exit 1; }

wal_remote_prefix=""
while IFS= read -r wal_target; do
  wal_name="$(basename "$wal_target")"
  [[ "$wal_name" =~ ^([0-9A-F]{24}(\.[0-9A-F]{8}\.backup)?|[0-9A-F]{8}\.history)\.age$ ]] || { echo "WAL 制品名无效：$wal_name" >&2; exit 1; }
  verify_ciphertext_and_evidence "$wal_target" "PITR WAL $wal_name" pitr-wal "$pitr_cluster_id" "$MONITOR_PITR_PROVENANCE_VERIFY_KEY_FILE"
  wal_remote_target="$verified_remote_target"
  verify_wal_source_manifest "$wal_target" "$wal_remote_target" "PITR WAL $wal_name"
  current_remote_prefix="${wal_remote_target%/*}"
  [[ "$current_remote_prefix" != "$wal_remote_target" ]] || { echo "WAL 远端目标缺少受控目录前缀" >&2; exit 1; }
  if [[ -z "$wal_remote_prefix" ]]; then
    wal_remote_prefix="$current_remote_prefix"
  else
    [[ "$wal_remote_prefix" == "$current_remote_prefix" ]] || { echo "保留 WAL 指向多个远端来源" >&2; exit 1; }
  fi
  wal_cipher_sha="$(sha256sum "$wal_target" | awk '{print $1}')"
  printf '%s %s %s\n' "$wal_name" "$wal_cipher_sha" "$wal_remote_target" >>"$wal_inventory_file"
  printf '%s\n' "$wal_name" "$wal_name.sha256" "$wal_name.source.sha256" "$wal_name.provenance" "$wal_name.provenance.sig" >>"$expected_remote_files"
done <"$wal_targets_file"

LC_ALL=C sort -u "$expected_remote_files" -o "$expected_remote_files"
[[ "$(wc -l <"$expected_remote_files" | tr -d '[:space:]')" == "$((wal_inventory_count * 5))" ]] || { echo "WAL 预期远端集合存在重复" >&2; exit 1; }
while IFS= read -r remote_name; do
  [[ -n "$remote_name" && "$remote_name" != */* ]] || { echo "PITR 远端枚举返回不安全对象名：$remote_name" >&2; exit 1; }
  if [[ "$remote_name" =~ ^([0-9A-F]{24}(\.[0-9A-F]{8}\.backup)?|[0-9A-F]{8}\.history)\.age(\.sha256|\.source\.sha256|\.provenance(\.sig)?)?$ ]]; then
    printf '%s\n' "$remote_name" >>"$actual_remote_files"
  elif [[ "$remote_name" =~ ^basebackup-[0-9]{8}-[0-9]{6}-[0-9]+\.tar\.age(\.sha256|\.provenance(\.sig)?)?$ ]]; then
    :
  else
    echo "PITR 远端目录包含不属于完整 base/WAL 制品集合的对象：$remote_name" >&2
    exit 1
  fi
done < <(run_rclone lsf "$wal_remote_prefix" --files-only --max-depth 1 --format p | LC_ALL=C sort)
LC_ALL=C sort -u "$actual_remote_files" -o "$actual_remote_files"
cmp -s "$expected_remote_files" "$actual_remote_files" || {
  echo "保留 WAL 的本地五件套与远端精确对象集合不一致" >&2
  exit 1
}
wal_inventory_first="$(basename "$(head -n 1 "$wal_targets_file")")"
wal_inventory_last="$(basename "$(tail -n 1 "$wal_targets_file")")"
wal_inventory_sha256="$(LC_ALL=C sort "$wal_inventory_file" | sha256sum | awk '{print $1}')"
[[ "$wal_inventory_first" =~ \.age$ && "$wal_inventory_last" =~ \.age$ && "$wal_inventory_sha256" =~ ^[0-9a-f]{64}$ ]] || { echo "WAL inventory 证据无效" >&2; exit 1; }

integrity_completed_epoch="$(date +%s)"
[[ "$integrity_completed_epoch" =~ ^[1-9][0-9]{0,11}$ ]] || { echo "无法生成备份完整性状态时间" >&2; exit 1; }
integrity_status_partial="$(mktemp "$MONITOR_STATE_DIR/.backup-integrity-status.XXXXXX")"
printf 'v2 %s %s %s %s %s %s %s %s\n' \
  "$integrity_completed_epoch" "$(basename "$database_target")" "$(basename "$upload_target")" \
  "$(basename "$basebackup_target")" "$wal_inventory_count" "$wal_inventory_first" "$wal_inventory_last" "$wal_inventory_sha256" >"$integrity_status_partial"
validate_no_symlink_path_components "$MONITOR_BACKUP_INTEGRITY_STATUS_FILE"
if [[ -e "$MONITOR_BACKUP_INTEGRITY_STATUS_FILE" || -L "$MONITOR_BACKUP_INTEGRITY_STATUS_FILE" ]]; then
  [[ -f "$MONITOR_BACKUP_INTEGRITY_STATUS_FILE" && ! -L "$MONITOR_BACKUP_INTEGRITY_STATUS_FILE" ]] || {
    echo "备份完整性成功状态目标不是普通文件" >&2
    exit 1
  }
fi
mv -- "$integrity_status_partial" "$MONITOR_BACKUP_INTEGRITY_STATUS_FILE"
integrity_status_partial=""
cleanup_integrity_work
integrity_work=""
trap - EXIT INT TERM

printf '低频备份完整性检查通过：database=%s uploads=%s basebackup=%s wal_count=%s inventory=%s\n' \
  "$(basename "$database_target")" "$(basename "$upload_target")" \
  "$(basename "$basebackup_target")" "$wal_inventory_count" "$wal_inventory_sha256"
