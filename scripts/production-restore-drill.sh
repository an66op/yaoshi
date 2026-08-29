#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

ENV_SOURCE="${1:-/etc/wangzhe/restore-drill.env}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/strict-env.sh
source "$SCRIPT_DIR/lib/strict-env.sh"
# shellcheck source=lib/encrypted-backup.sh
source "$SCRIPT_DIR/lib/encrypted-backup.sh"
for command_name in age awk basename createdb date dirname dropdb find findmnt flock grep head id mkdir mktemp mv openssl pg_restore psql pwd rclone rm sha256sum sort stat tail tar uniq wc; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done
pkeyutl_help="$(openssl pkeyutl -help 2>&1 || true)"
grep -q -- '-rawin' <<<"$pkeyutl_help" || { echo "恢复状态 Ed25519 签名要求 OpenSSL 3.0+（缺少 pkeyutl -rawin）" >&2; exit 1; }
unset pkeyutl_help

require_canonical_existing_directory() {
  local directory="$1" label="$2" physical_directory
  [[ "$directory" == /* ]] || { echo "$label 必须是绝对路径" >&2; return 1; }
  validate_no_symlink_path_components "$directory" || return 1
  [[ -d "$directory" && ! -L "$directory" ]] || { echo "$label 无效：$directory" >&2; return 1; }
  physical_directory="$(cd -P -- "$directory" && pwd -P)" || return 1
  [[ "$directory" == "$physical_directory" ]] || {
    echo "$label 必须使用无 .、..、重复斜线或符号链接的规范路径：$directory" >&2
    return 1
  }
}

require_canonical_existing_file() {
  local file="$1" label="$2" parent physical_parent canonical_file
  [[ "$file" == /* ]] || { echo "$label 必须是绝对路径" >&2; return 1; }
  validate_no_symlink_path_components "$file" || return 1
  [[ -f "$file" && ! -L "$file" ]] || { echo "$label 无效：$file" >&2; return 1; }
  parent="$(dirname -- "$file")"
  physical_parent="$(cd -P -- "$parent" && pwd -P)" || return 1
  canonical_file="$physical_parent/$(basename -- "$file")"
  [[ "$file" == "$canonical_file" ]] || {
    echo "$label 必须使用无 .、..、重复斜线或符号链接的规范路径：$file" >&2
    return 1
  }
}

validate_status_value() {
  local value="$1" label="$2"
  [[ -n "$value" && "$value" != *$'\n'* && "$value" != *$'\r'* && "$value" != *'='* ]] || {
    echo "$label 不能安全写入恢复状态" >&2
    return 1
  }
}

select_latest_remote_backup() {
  local remote_source="$1" include_pattern="$2" name_regex="$3" label="$4" listing_file="$5"
  local candidate latest=""
  run_source_rclone lsf "$remote_source" --files-only --max-depth 1 --format p --include "$include_pattern" >"$listing_file"
  while IFS= read -r candidate || [[ -n "$candidate" ]]; do
    [[ -n "$candidate" && "$candidate" != */* && "$candidate" != *'..'* && "$candidate" =~ $name_regex ]] || {
      echo "$label 远端目录返回不安全的对象名：$candidate" >&2
      return 1
    }
    if [[ -z "$latest" || "$candidate" > "$latest" ]]; then
      latest="$candidate"
    fi
  done <"$listing_file"
  [[ -n "$latest" ]] || {
    echo "$label 远端目录没有符合严格命名规则的加密备份" >&2
    return 1
  }
  printf '%s\n' "$latest"
}
if [[ "$ENV_SOURCE" != "--current-env" ]]; then
  require_canonical_existing_file "$ENV_SOURCE" "恢复演练环境文件"
  load_strict_env "$ENV_SOURCE" '^RESTORE_DRILL_[A-Z0-9_]+$'
fi
required=(
  RESTORE_DRILL_ISOLATION_MARKER RESTORE_DRILL_DATABASE_HOST RESTORE_DRILL_DATABASE_PORT
  RESTORE_DRILL_DATABASE_USER RESTORE_DRILL_DATABASE_PASSWORD RESTORE_DRILL_DATABASE_SSLMODE
  RESTORE_DRILL_MAINTENANCE_DATABASE RESTORE_DRILL_TARGET_DATABASE RESTORE_DRILL_AGE_IDENTITY_FILE
  RESTORE_DRILL_BACKUP_PROVENANCE_VERIFY_KEY_FILE
  RESTORE_DRILL_SOURCE_DATABASE_NAME RESTORE_DRILL_DATABASE_REMOTE_SOURCE RESTORE_DRILL_UPLOAD_REMOTE_SOURCE
  RESTORE_DRILL_SOURCE_RCLONE_CONFIG RESTORE_DRILL_STATUS_RCLONE_CONFIG
  RESTORE_DRILL_UPLOAD_TARGET_DIR RESTORE_DRILL_STATUS_FILE
  RESTORE_DRILL_STATUS_REMOTE_DESTINATION RESTORE_DRILL_STATUS_SIGNING_KEY_FILE
)
for key in "${required[@]}"; do
  [[ -n "${!key:-}" ]] || { echo "缺少 $key" >&2; exit 1; }
done

require_canonical_existing_file "$RESTORE_DRILL_ISOLATION_MARKER" "隔离恢复主机确认标记" || {
  echo "缺少仅在隔离恢复主机创建的有效确认标记" >&2
  exit 1
}
marker_mode="$(strict_env_stat '%a' '%Lp' "$RESTORE_DRILL_ISOLATION_MARKER")"
marker_owner="$(strict_env_stat '%u' '%u' "$RESTORE_DRILL_ISOLATION_MARKER")"
[[ "$marker_owner" == 0 && "$marker_mode" =~ ^[0-7]{3,4}$ ]] || { echo "隔离主机标记必须由 root 所有" >&2; exit 1; }
(( (8#$marker_mode & 022) == 0 )) || { echo "隔离主机标记不能被非 root 修改" >&2; exit 1; }
grep -qx 'WANGZHE_ISOLATED_RECOVERY_HOST' "$RESTORE_DRILL_ISOLATION_MARKER" || { echo "隔离主机标记内容无效" >&2; exit 1; }
[[ "$RESTORE_DRILL_DATABASE_PORT" =~ ^[0-9]+$ ]] && (( RESTORE_DRILL_DATABASE_PORT >= 1 && RESTORE_DRILL_DATABASE_PORT <= 65535 )) || { echo "恢复数据库端口无效" >&2; exit 1; }
[[ "$RESTORE_DRILL_TARGET_DATABASE" =~ ^wangzhe_restore_[A-Za-z0-9_]+$ ]] || { echo "恢复目标库必须使用 wangzhe_restore_ 前缀" >&2; exit 1; }
[[ "$RESTORE_DRILL_MAINTENANCE_DATABASE" =~ ^[A-Za-z0-9_]+$ ]] || { echo "维护数据库名无效" >&2; exit 1; }
[[ "$RESTORE_DRILL_MAINTENANCE_DATABASE" != "$RESTORE_DRILL_TARGET_DATABASE" ]] || { echo "维护数据库不能是恢复目标库" >&2; exit 1; }
case "$RESTORE_DRILL_DATABASE_SSLMODE" in disable|verify-ca|verify-full) ;; *) echo "恢复数据库 SSLMODE 无效" >&2; exit 1;; esac
case "$RESTORE_DRILL_DATABASE_HOST" in
  127.0.0.1|::1) ;;
  *) echo "恢复数据库必须使用隔离主机的数字回环地址（127.0.0.1 或 ::1）" >&2; exit 1 ;;
esac
[[ "$RESTORE_DRILL_SOURCE_DATABASE_NAME" =~ ^[A-Za-z][A-Za-z0-9_]{0,62}$ ]] || {
  echo "恢复源数据库名只能包含安全的字母、数字和下划线，且必须以字母开头" >&2
  exit 1
}
validate_private_file "$RESTORE_DRILL_AGE_IDENTITY_FILE" "AGE 恢复身份文件"
require_canonical_existing_file "$RESTORE_DRILL_AGE_IDENTITY_FILE" "AGE 恢复身份文件"
[[ "$RESTORE_DRILL_BACKUP_PROVENANCE_VERIFY_KEY_FILE" == /etc/wangzhe/backup-provenance-ed25519-public.pem ]] || {
  echo "逻辑恢复的备份来源验签公钥必须使用固定路径" >&2
  exit 1
}
validate_ed25519_public_key "$RESTORE_DRILL_BACKUP_PROVENANCE_VERIFY_KEY_FILE" "数据库/上传备份来源 Ed25519 公钥"
[[ "$RESTORE_DRILL_STATUS_SIGNING_KEY_FILE" == /etc/wangzhe/logical-restore-status-ed25519-private.pem ]] || {
  echo "逻辑恢复状态签名私钥必须使用固定独立路径" >&2
  exit 1
}
validate_private_file "$RESTORE_DRILL_STATUS_SIGNING_KEY_FILE" "逻辑恢复状态 Ed25519 私钥"
require_canonical_existing_file "$RESTORE_DRILL_STATUS_SIGNING_KEY_FILE" "逻辑恢复状态 Ed25519 私钥"
openssl pkey -in "$RESTORE_DRILL_STATUS_SIGNING_KEY_FILE" -noout -text 2>/dev/null | grep -q '^ED25519 Private-Key:' || {
  echo "逻辑恢复状态签名私钥不是有效 Ed25519 私钥" >&2
  exit 1
}
[[ "$RESTORE_DRILL_UPLOAD_TARGET_DIR" == /var/lib/wangzhe-restore/work/uploads ]] || {
  echo "上传恢复目标必须精确为 /var/lib/wangzhe-restore/work/uploads" >&2
  exit 1
}
[[ "$RESTORE_DRILL_STATUS_FILE" == /var/lib/wangzhe-restore/work/last-success.status ]] || {
  echo "恢复状态文件必须精确为 /var/lib/wangzhe-restore/work/last-success.status" >&2
  exit 1
}
validate_remote_destination "$RESTORE_DRILL_DATABASE_REMOTE_SOURCE"
validate_remote_destination "$RESTORE_DRILL_UPLOAD_REMOTE_SOURCE"
validate_remote_destination "$RESTORE_DRILL_STATUS_REMOTE_DESTINATION"
[[ "${RESTORE_DRILL_STATUS_REMOTE_DESTINATION##*/}" == last-success.status ]] || {
  echo "恢复状态远端目标必须精确指向 last-success.status" >&2
  exit 1
}
[[ "$RESTORE_DRILL_SOURCE_RCLONE_CONFIG" == /etc/wangzhe/logical-restore-source-read-rclone.conf ]] || {
  echo "逻辑恢复异机源必须使用固定的只读 rclone 配置" >&2
  exit 1
}
[[ "$RESTORE_DRILL_STATUS_RCLONE_CONFIG" == /etc/wangzhe/logical-restore-status-write-rclone.conf ]] || {
  echo "逻辑恢复状态发布必须使用固定的独立写入 rclone 配置" >&2
  exit 1
}
[[ "$RESTORE_DRILL_SOURCE_RCLONE_CONFIG" != "$RESTORE_DRILL_STATUS_RCLONE_CONFIG" ]] || {
  echo "恢复源只读凭据与状态写入凭据不能使用同一路径" >&2
  exit 1
}
validate_rclone_config "$RESTORE_DRILL_SOURCE_RCLONE_CONFIG"
validate_rclone_config "$RESTORE_DRILL_STATUS_RCLONE_CONFIG"
require_canonical_existing_file "$RESTORE_DRILL_SOURCE_RCLONE_CONFIG" "恢复源只读 rclone 配置"
require_canonical_existing_file "$RESTORE_DRILL_STATUS_RCLONE_CONFIG" "恢复状态写入 rclone 配置"
[[ ! "$RESTORE_DRILL_SOURCE_RCLONE_CONFIG" -ef "$RESTORE_DRILL_STATUS_RCLONE_CONFIG" ]] || {
  echo "恢复源只读凭据与状态写入凭据不能是同一文件或硬链接" >&2
  exit 1
}
source_config_sha256="$(sha256sum "$RESTORE_DRILL_SOURCE_RCLONE_CONFIG" | awk '{print $1}')"
status_config_sha256="$(sha256sum "$RESTORE_DRILL_STATUS_RCLONE_CONFIG" | awk '{print $1}')"
[[ "$source_config_sha256" =~ ^[0-9a-f]{64}$ && "$status_config_sha256" =~ ^[0-9a-f]{64}$ && "$source_config_sha256" != "$status_config_sha256" ]] || {
  echo "恢复源只读凭据与状态写入凭据必须是不同配置" >&2
  exit 1
}
unset source_config_sha256 status_config_sha256
source_rclone_args=(--config "$RESTORE_DRILL_SOURCE_RCLONE_CONFIG" --contimeout 10s --timeout 2m --retries 2 --low-level-retries 2)
status_rclone_args=(--config "$RESTORE_DRILL_STATUS_RCLONE_CONFIG" --contimeout 10s --timeout 2m --retries 2 --low-level-retries 2)
run_source_rclone() { rclone "${source_rclone_args[@]}" "$@"; }
run_status_rclone() { rclone "${status_rclone_args[@]}" "$@"; }

umask 077
restore_mount_root=/var/lib/wangzhe-restore
restore_root="$(dirname "$RESTORE_DRILL_UPLOAD_TARGET_DIR")"
[[ "$restore_root" == /var/lib/wangzhe-restore/work ]] || { echo "恢复根目录计算结果异常" >&2; exit 1; }
validate_no_symlink_path_components "$restore_root"
require_canonical_existing_directory "$restore_root" "恢复工作目录" || exit 1
validate_direct_luks_mount "$restore_mount_root" "逻辑恢复 LUKS 挂载点" || exit 1
validate_luks_service_directory "$restore_root" "$restore_mount_root" "逻辑恢复私有工作目录" || exit 1
lock_file="$restore_root/.restore-drill.lock"
if [[ -e "$lock_file" || -L "$lock_file" ]]; then
  [[ -f "$lock_file" && ! -L "$lock_file" ]] || { echo "恢复演练锁文件不是普通文件" >&2; exit 1; }
fi
validate_no_symlink_path_components "$lock_file"
exec 9>>"$lock_file"
flock -w 1 9 || { echo "另一个恢复演练仍在运行" >&2; exit 1; }
unexpected_restore_entry="$(find "$restore_root" -xdev -mindepth 1 -maxdepth 1 \
  ! -name .restore-drill.lock ! -name uploads \
  ! -name last-success.status ! -name last-success.status.sha256 ! -name last-success.status.sig \
  -print -quit)"
[[ -z "$unexpected_restore_entry" ]] || {
  echo "逻辑恢复 LUKS 工作目录存在未完成任务遗留，拒绝覆盖或自动删除：$unexpected_restore_entry" >&2
  exit 1
}
work_dir="$(mktemp -d "$restore_root/.drill.XXXXXX")"
require_canonical_existing_directory "$work_dir" "恢复临时目录" || exit 1
previous_target=""
cleanup_drill() {
  if [[ -n "${previous_target:-}" && -d "$previous_target" && ! -L "$previous_target" && ! -e "$RESTORE_DRILL_UPLOAD_TARGET_DIR" && ! -L "$RESTORE_DRILL_UPLOAD_TARGET_DIR" ]]; then
    mv -- "$previous_target" "$RESTORE_DRILL_UPLOAD_TARGET_DIR" || true
  fi
  case "${work_dir:-}" in "$restore_root"/.drill.*) rm -rf -- "$work_dir" ;; esac
  return 0
}
trap cleanup_drill EXIT INT TERM
database_listing="$work_dir/database.remote.lsf"
upload_listing="$work_dir/uploads.remote.lsf"
database_name_regex="^${RESTORE_DRILL_SOURCE_DATABASE_NAME}-[0-9]{8}-[0-9]{6}-[0-9]+\\.dump\\.age$"
upload_name_regex='^uploads-[0-9]{8}-[0-9]{6}-[0-9]+\.tar\.age$'
database_backup_name="$(select_latest_remote_backup "$RESTORE_DRILL_DATABASE_REMOTE_SOURCE" "${RESTORE_DRILL_SOURCE_DATABASE_NAME}-*.dump.age" "$database_name_regex" "数据库备份" "$database_listing")"
upload_backup_name="$(select_latest_remote_backup "$RESTORE_DRILL_UPLOAD_REMOTE_SOURCE" 'uploads-*.tar.age' "$upload_name_regex" "上传备份" "$upload_listing")"
database_offsite_source="${RESTORE_DRILL_DATABASE_REMOTE_SOURCE%/}/$database_backup_name"
upload_offsite_source="${RESTORE_DRILL_UPLOAD_REMOTE_SOURCE%/}/$upload_backup_name"
validate_remote_destination "$database_offsite_source"
validate_remote_destination "$upload_offsite_source"
database_backup="$work_dir/$database_backup_name"
upload_backup="$work_dir/$upload_backup_name"
for download_target in "$database_backup" "$database_backup.sha256" "$database_backup.provenance" "$database_backup.provenance.sig" \
  "$upload_backup" "$upload_backup.sha256" "$upload_backup.provenance" "$upload_backup.provenance.sig"; do
  [[ ! -e "$download_target" && ! -L "$download_target" ]] || { echo "恢复下载目标已存在：$download_target" >&2; exit 1; }
done
run_source_rclone copyto "$database_offsite_source" "$database_backup" --no-traverse
run_source_rclone copyto "$database_offsite_source.sha256" "$database_backup.sha256" --no-traverse
run_source_rclone copyto "$database_offsite_source.provenance" "$database_backup.provenance" --no-traverse
run_source_rclone copyto "$database_offsite_source.provenance.sig" "$database_backup.provenance.sig" --no-traverse
run_source_rclone copyto "$upload_offsite_source" "$upload_backup" --no-traverse
run_source_rclone copyto "$upload_offsite_source.sha256" "$upload_backup.sha256" --no-traverse
run_source_rclone copyto "$upload_offsite_source.provenance" "$upload_backup.provenance" --no-traverse
run_source_rclone copyto "$upload_offsite_source.provenance.sig" "$upload_backup.provenance.sig" --no-traverse
validate_encrypted_backup_and_manifest "$database_backup" || { echo "异机数据库备份完整 SHA-256 校验失败" >&2; exit 1; }
validate_encrypted_backup_and_manifest "$upload_backup" || { echo "异机上传备份完整 SHA-256 校验失败" >&2; exit 1; }
verify_backup_provenance "$database_backup" database "$RESTORE_DRILL_SOURCE_DATABASE_NAME" "$database_offsite_source" \
  "$RESTORE_DRILL_BACKUP_PROVENANCE_VERIFY_KEY_FILE" || { echo "异机数据库备份来源签名验证失败" >&2; exit 1; }
verify_backup_provenance "$upload_backup" uploads /var/lib/wangzhe/uploads "$upload_offsite_source" \
  "$RESTORE_DRILL_BACKUP_PROVENANCE_VERIFY_KEY_FILE" || { echo "异机上传备份来源签名验证失败" >&2; exit 1; }
database_plain="$work_dir/database.dump"
upload_plain="$work_dir/uploads.tar"
extract_dir="$work_dir/extracted"

export PGPASSWORD="$RESTORE_DRILL_DATABASE_PASSWORD"
export PGSSLMODE="$RESTORE_DRILL_DATABASE_SSLMODE"
pg_args=(--host "$RESTORE_DRILL_DATABASE_HOST" --port "$RESTORE_DRILL_DATABASE_PORT" --username "$RESTORE_DRILL_DATABASE_USER")
postgres_data_directory="$(psql "${pg_args[@]}" --dbname "$RESTORE_DRILL_MAINTENANCE_DATABASE" --no-psqlrc --tuples-only --no-align --set ON_ERROR_STOP=1 --command 'SHOW data_directory;')"
[[ "$postgres_data_directory" == /* && "$postgres_data_directory" != *$'\n'* && "$postgres_data_directory" != *$'\r'* ]] || {
  echo "恢复 PostgreSQL data_directory 不是安全的绝对路径" >&2
  exit 1
}
postgres_data_mount="$(direct_luks_mount_for_directory "$postgres_data_directory" "恢复 PostgreSQL data_directory")" || {
  echo "恢复 PostgreSQL data_directory 必须位于直接 LUKS dm-crypt 文件系统" >&2
  exit 1
}

age --decrypt --identity "$RESTORE_DRILL_AGE_IDENTITY_FILE" --output "$database_plain" "$database_backup"
pg_restore --list "$database_plain" >/dev/null
dropdb "${pg_args[@]}" --maintenance-db "$RESTORE_DRILL_MAINTENANCE_DATABASE" --if-exists --force "$RESTORE_DRILL_TARGET_DATABASE"
createdb "${pg_args[@]}" --maintenance-db "$RESTORE_DRILL_MAINTENANCE_DATABASE" "$RESTORE_DRILL_TARGET_DATABASE"
pg_restore "${pg_args[@]}" --dbname "$RESTORE_DRILL_TARGET_DATABASE" --exit-on-error --no-owner "$database_plain"

schema_count="$(psql "${pg_args[@]}" --dbname "$RESTORE_DRILL_TARGET_DATABASE" --no-psqlrc --tuples-only --no-align --command 'SELECT count(*) FROM schema_migrations;')"
negative_balances="$(psql "${pg_args[@]}" --dbname "$RESTORE_DRILL_TARGET_DATABASE" --no-psqlrc --tuples-only --no-align --command 'SELECT count(*) FROM "user" WHERE balance_cents < 0 AND deleted_at IS NULL;')"
orphan_bets="$(psql "${pg_args[@]}" --dbname "$RESTORE_DRILL_TARGET_DATABASE" --no-psqlrc --tuples-only --no-align --command 'SELECT count(*) FROM lottery_bets WHERE workspace_id = 0;')"
unset PGPASSWORD PGSSLMODE
[[ "$schema_count" =~ ^[0-9]+$ && "$schema_count" -gt 0 && "$negative_balances" == 0 && "$orphan_bets" == 0 ]] || {
  echo "恢复数据库业务一致性检查失败：migrations=$schema_count negative=$negative_balances orphan_bets=$orphan_bets" >&2
  exit 1
}

age --decrypt --identity "$RESTORE_DRILL_AGE_IDENTITY_FILE" --output "$upload_plain" "$upload_backup"
mkdir "$extract_dir"
manifest_name=""
archive_paths_file="$work_dir/upload-archive-paths"
: >"$archive_paths_file"
archive_entry_count=0
while IFS= read -r entry; do
  archive_path="${entry%/}"
  [[ -n "$archive_path" && "$archive_path" != /* && "$archive_path" != *$'\r'* ]] || {
    echo "上传备份包含无效路径" >&2
    exit 1
  }
  IFS='/' read -r -a archive_components <<<"$archive_path"
  for archive_component in "${archive_components[@]}"; do
    [[ -n "$archive_component" && "$archive_component" != . && "$archive_component" != .. ]] || {
      echo "上传备份路径不是规范相对路径：$entry" >&2
      exit 1
    }
  done
  case "$entry" in
    uploads|uploads/|uploads/*) ;;
    .uploads-*.manifest.partial)
      [[ "$entry" =~ ^\.uploads-[0-9]{8}-[0-9]{6}-[0-9]+\.manifest\.partial$ ]] || { echo "上传备份清单名无效" >&2; exit 1; }
      [[ -z "$manifest_name" ]] || { echo "上传备份包含多个清单" >&2; exit 1; }
      manifest_name="$entry"
      ;;
    *) echo "上传备份包含越界路径：$entry" >&2; exit 1 ;;
  esac
  printf '%s\n' "$archive_path" >>"$archive_paths_file"
  archive_entry_count=$((archive_entry_count + 1))
done < <(tar --list --file "$upload_plain")
[[ -n "$manifest_name" ]] || { echo "上传备份缺少文件清单" >&2; exit 1; }
duplicate_archive_path="$(LC_ALL=C sort "$archive_paths_file" | uniq -d | head -n 1)"
[[ -z "$duplicate_archive_path" ]] || { echo "上传备份包含重复规范路径：$duplicate_archive_path" >&2; exit 1; }
archive_type_count=0
while IFS= read -r verbose_entry; do
  [[ -n "$verbose_entry" ]] || continue
  case "${verbose_entry:0:1}" in
    -|d) ;;
    *) echo "上传备份包含符号链接、硬链接或特殊文件" >&2; exit 1 ;;
  esac
  archive_type_count=$((archive_type_count + 1))
done < <(tar --verbose --list --file "$upload_plain")
[[ "$archive_type_count" == "$archive_entry_count" ]] || { echo "上传备份条目与类型清单数量不一致" >&2; exit 1; }
tar --extract --file "$upload_plain" --directory "$extract_dir" --no-same-owner --no-same-permissions
manifest_path="$extract_dir/$manifest_name"
[[ -f "$manifest_path" && ! -L "$manifest_path" ]] || { echo "上传备份文件清单不是普通文件" >&2; exit 1; }
[[ -d "$extract_dir/uploads" && ! -L "$extract_dir/uploads" ]] || { echo "上传恢复目录无效" >&2; exit 1; }
unsafe_entry="$(find "$extract_dir/uploads" -xdev \( -type l -o \( ! -type f -a ! -type d \) \) -print -quit)"
[[ -z "$unsafe_entry" ]] || { echo "恢复后的上传目录包含符号链接或特殊文件" >&2; exit 1; }
while IFS= read -r -d '' restored_entry; do
  [[ "$restored_entry" != *$'\n'* && "$restored_entry" != *$'\r'* ]] || {
    echo "恢复后的上传目录包含换行文件名" >&2
    exit 1
  }
done < <(find "$extract_dir/uploads" -xdev -print0)
manifest_file_count=0
while IFS= read -r manifest_line || [[ -n "$manifest_line" ]]; do
  manifest_record="$manifest_line"
  if [[ "${manifest_record:0:1}" == '\' ]]; then
    manifest_record="${manifest_record:1}"
  fi
  manifest_hash="${manifest_record:0:64}"
  manifest_separator="${manifest_record:64:2}"
  manifest_entry="${manifest_record:66}"
  [[ "$manifest_hash" =~ ^[0-9a-f]{64}$ && ( "$manifest_separator" == "  " || "$manifest_separator" == " *" ) ]] || {
    echo "上传备份清单包含格式无效的摘要记录" >&2
    exit 1
  }
  [[ "$manifest_entry" == uploads/* && "$manifest_entry" != *$'\r'* ]] || {
    echo "上传备份清单包含越界文件路径" >&2
    exit 1
  }
  IFS='/' read -r -a manifest_components <<<"$manifest_entry"
  for manifest_component in "${manifest_components[@]}"; do
    [[ -n "$manifest_component" && "$manifest_component" != . && "$manifest_component" != .. ]] || {
      echo "上传备份清单包含非规范文件路径" >&2
      exit 1
    }
  done
  manifest_file_count=$((manifest_file_count + 1))
done <"$manifest_path"
restored_manifest="$work_dir/restored-uploads.manifest"
(
  cd "$extract_dir"
  while IFS= read -r -d '' restored_file; do
    sha256sum "$restored_file"
  done < <(find uploads -xdev -type f -print0 | LC_ALL=C sort -z)
) >"$restored_manifest"
[[ "$(sha256sum "$manifest_path" | awk '{print $1}')" == "$(sha256sum "$restored_manifest" | awk '{print $1}')" ]] || {
  echo "上传备份清单与恢复后的完整文件集合不一致" >&2
  exit 1
}
if [[ -s "$manifest_path" ]]; then
  (
    cd "$extract_dir"
    sha256sum --check --strict "$manifest_name"
  )
elif [[ -n "$(find "$extract_dir/uploads" -xdev -type f -print -quit)" ]]; then
  echo "上传备份清单为空，但归档中存在文件" >&2
  exit 1
fi
upload_file_count="$(find "$extract_dir/uploads" -xdev -type f -printf '.' | wc -c | awk '{print $1}')"
upload_total_bytes="$(find "$extract_dir/uploads" -xdev -type f -printf '%s\n' | awk '{total += $1} END {printf "%.0f", total + 0}')"
[[ "$manifest_file_count" =~ ^[0-9]+$ && "$upload_file_count" =~ ^[0-9]+$ && "$upload_total_bytes" =~ ^[0-9]+$ ]] || {
  echo "无法生成上传恢复统计" >&2
  exit 1
}
[[ "$manifest_file_count" == "$upload_file_count" ]] || {
  echo "上传备份清单条目数与恢复文件数不一致" >&2
  exit 1
}

previous_target="$restore_root/.uploads.previous.$$"
[[ ! -e "$previous_target" && ! -L "$previous_target" ]] || { echo "恢复临时目标已存在" >&2; exit 1; }
validate_no_symlink_path_components "$RESTORE_DRILL_UPLOAD_TARGET_DIR"
if [[ -e "$RESTORE_DRILL_UPLOAD_TARGET_DIR" || -L "$RESTORE_DRILL_UPLOAD_TARGET_DIR" ]]; then
  [[ -d "$RESTORE_DRILL_UPLOAD_TARGET_DIR" && ! -L "$RESTORE_DRILL_UPLOAD_TARGET_DIR" ]] || { echo "既有上传恢复目标不是普通目录" >&2; exit 1; }
  mv -- "$RESTORE_DRILL_UPLOAD_TARGET_DIR" "$previous_target"
fi
mv -- "$extract_dir/uploads" "$RESTORE_DRILL_UPLOAD_TARGET_DIR"
require_canonical_existing_directory "$RESTORE_DRILL_UPLOAD_TARGET_DIR" "上传恢复目标" || exit 1
upload_target_mount="$(direct_luks_mount_for_directory "$RESTORE_DRILL_UPLOAD_TARGET_DIR" "上传恢复目标")" || exit 1
[[ "$upload_target_mount" == "$restore_mount_root" ]] || { echo "上传恢复目标不在固定逻辑恢复 LUKS 挂载点" >&2; exit 1; }
validate_luks_service_directory "$RESTORE_DRILL_UPLOAD_TARGET_DIR" "$restore_mount_root" "上传恢复目标" || exit 1
if [[ -d "$previous_target" && ! -L "$previous_target" ]]; then
  rm -rf -- "$previous_target"
fi
previous_target=""

database_checksum="$(sha256sum "$database_backup" | awk '{print $1}')"
upload_checksum="$(sha256sum "$upload_backup" | awk '{print $1}')"
database_provenance_checksum="$(sha256sum "$database_backup.provenance" | awk '{print $1}')"
upload_provenance_checksum="$(sha256sum "$upload_backup.provenance" | awk '{print $1}')"
[[ "$database_checksum" =~ ^[0-9a-f]{64}$ && "$upload_checksum" =~ ^[0-9a-f]{64}$ && "$database_provenance_checksum" =~ ^[0-9a-f]{64}$ && "$upload_provenance_checksum" =~ ^[0-9a-f]{64}$ ]] || { echo "恢复制品摘要格式无效" >&2; exit 1; }
completed_at_epoch="$(date +%s)"
completed_at_utc="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
drill_id="$completed_at_epoch-$$"
database_artifact_bytes="$(strict_env_stat '%s' '%z' "$database_backup")"
upload_artifact_bytes="$(strict_env_stat '%s' '%z' "$upload_backup")"
isolation_marker_sha256="$(sha256sum "$RESTORE_DRILL_ISOLATION_MARKER" | awk '{print $1}')"
validate_status_value "$completed_at_epoch" "完成时间"
validate_status_value "$completed_at_utc" "UTC 完成时间"
validate_status_value "$drill_id" "演练编号"
validate_status_value "$RESTORE_DRILL_SOURCE_DATABASE_NAME" "恢复源数据库名"
validate_status_value "$database_backup_name" "数据库备份名"
validate_status_value "$upload_backup_name" "上传备份名"
validate_status_value "$RESTORE_DRILL_TARGET_DATABASE" "恢复目标库"
validate_status_value "$RESTORE_DRILL_DATABASE_PORT" "恢复数据库端口"
validate_status_value "$database_artifact_bytes" "数据库备份大小"
validate_status_value "$upload_artifact_bytes" "上传备份大小"
validate_status_value "$schema_count" "迁移数"
validate_status_value "$negative_balances" "负余额数"
validate_status_value "$orphan_bets" "异常注单数"
validate_status_value "$manifest_file_count" "上传清单条目数"
validate_status_value "$upload_file_count" "上传文件数"
validate_status_value "$upload_total_bytes" "上传文件字节数"
validate_status_value "$database_offsite_source" "数据库异机来源"
validate_status_value "$upload_offsite_source" "上传异机来源"
validate_status_value "$restore_mount_root" "逻辑恢复 LUKS 挂载点"
validate_status_value "$postgres_data_mount" "恢复数据库 LUKS 挂载点"
validate_status_value "$upload_target_mount" "上传恢复 LUKS 挂载点"
[[ "$completed_at_epoch" =~ ^[0-9]+$ && "$drill_id" =~ ^[0-9]+-[0-9]+$ ]] || { echo "恢复演练时间字段无效" >&2; exit 1; }
[[ "$completed_at_utc" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]] || { echo "恢复演练 UTC 时间字段无效" >&2; exit 1; }
[[ "$database_artifact_bytes" =~ ^[0-9]+$ && "$upload_artifact_bytes" =~ ^[0-9]+$ && "$isolation_marker_sha256" =~ ^[0-9a-f]{64}$ ]] || {
  echo "恢复演练证据字段无效" >&2
  exit 1
}

status_partial="$work_dir/last-success.status.partial"
status_checksum_partial="$work_dir/last-success.status.sha256.partial"
status_signature_partial="$work_dir/last-success.status.sig.partial"
remote_status_checksum_file="$work_dir/remote-last-success.status.sha256"
remote_status_signature_file="$work_dir/remote-last-success.status.sig"
for existing_status in "$RESTORE_DRILL_STATUS_FILE" "$RESTORE_DRILL_STATUS_FILE.sha256" "$RESTORE_DRILL_STATUS_FILE.sig"; do
  if [[ -e "$existing_status" || -L "$existing_status" ]]; then
    [[ -f "$existing_status" && ! -L "$existing_status" ]] || { echo "既有恢复状态不是普通文件：$existing_status" >&2; exit 1; }
  fi
done
validate_no_symlink_path_components "$RESTORE_DRILL_STATUS_FILE"
{
  printf 'status_schema=wangzhe.restore-drill.v2\n'
  printf 'outcome=success\n'
  printf 'scope=logical_database_and_uploads\n'
  printf 'drill_id=%s\n' "$drill_id"
  printf 'completed_at_epoch=%s\n' "$completed_at_epoch"
  printf 'completed_at_utc=%s\n' "$completed_at_utc"
  printf 'isolation=offsite_download_loopback_database_and_fixed_targets\n'
  printf 'isolation_marker_sha256=%s\n' "$isolation_marker_sha256"
  printf 'database_host=loopback\n'
  printf 'database_port=%s\n' "$RESTORE_DRILL_DATABASE_PORT"
  printf 'database_target=%s\n' "$RESTORE_DRILL_TARGET_DATABASE"
  printf 'database_data_luks_mount=%s\n' "$postgres_data_mount"
  printf 'database_source_name=%s\n' "$RESTORE_DRILL_SOURCE_DATABASE_NAME"
  printf 'database_backup_name=%s\n' "$database_backup_name"
  printf 'database_artifact_bytes=%s\n' "$database_artifact_bytes"
  printf 'database_sha256=%s\n' "$database_checksum"
  printf 'database_provenance_sha256=%s\n' "$database_provenance_checksum"
  printf 'database_offsite_source=%s\n' "$database_offsite_source"
  printf 'database_restore=verified\n'
  printf 'schema_migrations=%s\n' "$schema_count"
  printf 'negative_balances=%s\n' "$negative_balances"
  printf 'orphan_bets=%s\n' "$orphan_bets"
  printf 'upload_backup_name=%s\n' "$upload_backup_name"
  printf 'upload_artifact_bytes=%s\n' "$upload_artifact_bytes"
  printf 'upload_sha256=%s\n' "$upload_checksum"
  printf 'upload_provenance_sha256=%s\n' "$upload_provenance_checksum"
  printf 'upload_offsite_source=%s\n' "$upload_offsite_source"
  printf 'upload_target_luks_mount=%s\n' "$upload_target_mount"
  printf 'restore_work_luks_mount=%s\n' "$restore_mount_root"
  printf 'upload_restore=verified\n'
  printf 'upload_manifest_entries=%s\n' "$manifest_file_count"
  printf 'upload_restored_files=%s\n' "$upload_file_count"
  printf 'upload_restored_bytes=%s\n' "$upload_total_bytes"
  printf 'pitr_restore=not_in_scope\n'
} >"$status_partial"
status_bytes="$(strict_env_stat '%s' '%z' "$status_partial")"
[[ "$status_bytes" =~ ^[0-9]+$ && "$status_bytes" -ge 1 && "$status_bytes" -le 16384 ]] || { echo "逻辑恢复状态大小无效" >&2; exit 1; }
status_checksum="$(sha256sum "$status_partial" | awk '{print $1}')"
printf '%s  %s\n' "$status_checksum" "$(basename "$RESTORE_DRILL_STATUS_FILE")" >"$status_checksum_partial"
openssl pkeyutl -sign -rawin -inkey "$RESTORE_DRILL_STATUS_SIGNING_KEY_FILE" -in "$status_partial" -out "$status_signature_partial"
[[ "$(strict_env_stat '%s' '%z' "$status_signature_partial")" == 64 ]] || { echo "逻辑恢复状态 Ed25519 签名长度无效" >&2; exit 1; }
openssl pkeyutl -verify -rawin -inkey "$RESTORE_DRILL_STATUS_SIGNING_KEY_FILE" -in "$status_partial" -sigfile "$status_signature_partial" >/dev/null || {
  echo "逻辑恢复状态本地签名自检失败" >&2
  exit 1
}
run_status_rclone copyto "$status_partial" "$RESTORE_DRILL_STATUS_REMOTE_DESTINATION" --checksum --no-traverse
run_status_rclone copyto "$status_checksum_partial" "$RESTORE_DRILL_STATUS_REMOTE_DESTINATION.sha256" --checksum --no-traverse
run_status_rclone copyto "$status_signature_partial" "$RESTORE_DRILL_STATUS_REMOTE_DESTINATION.sig" --checksum --no-traverse
remote_status_checksum="$(run_status_rclone hashsum sha256 "$RESTORE_DRILL_STATUS_REMOTE_DESTINATION" --download | awk 'NR == 1 {print $1}')"
[[ "$remote_status_checksum" == "$status_checksum" ]] || { echo "恢复演练状态异机回读校验失败" >&2; exit 1; }
run_status_rclone copyto "$RESTORE_DRILL_STATUS_REMOTE_DESTINATION.sha256" "$remote_status_checksum_file" --no-traverse
read -r remote_manifest_checksum remote_manifest_name remote_manifest_extra <"$remote_status_checksum_file" || { echo "恢复演练远端摘要不可读" >&2; exit 1; }
[[ "$remote_manifest_checksum" == "$status_checksum" && "$remote_manifest_name" == "$(basename "$RESTORE_DRILL_STATUS_FILE")" && -z "${remote_manifest_extra:-}" ]] || {
  echo "恢复演练远端摘要凭证无效" >&2
  exit 1
}
run_status_rclone copyto "$RESTORE_DRILL_STATUS_REMOTE_DESTINATION.sig" "$remote_status_signature_file" --no-traverse
[[ "$(strict_env_stat '%s' '%z' "$remote_status_signature_file")" == 64 ]] || { echo "恢复演练远端 Ed25519 签名长度无效" >&2; exit 1; }
openssl pkeyutl -verify -rawin -inkey "$RESTORE_DRILL_STATUS_SIGNING_KEY_FILE" -in "$status_partial" -sigfile "$remote_status_signature_file" >/dev/null || {
  echo "恢复演练远端 Ed25519 签名回读验证失败" >&2
  exit 1
}
validate_no_symlink_path_components "$RESTORE_DRILL_STATUS_FILE"
validate_no_symlink_path_components "$RESTORE_DRILL_STATUS_FILE.sha256"
validate_no_symlink_path_components "$RESTORE_DRILL_STATUS_FILE.sig"
mv -- "$status_signature_partial" "$RESTORE_DRILL_STATUS_FILE.sig"
mv -- "$status_checksum_partial" "$RESTORE_DRILL_STATUS_FILE.sha256"
mv -- "$status_partial" "$RESTORE_DRILL_STATUS_FILE"
echo "隔离恢复演练通过：数据库和上传文件均已校验"
