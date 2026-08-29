#!/usr/bin/env bash
set -euo pipefail

CRYPTO_ENV_SOURCE="${1:-/etc/wangzhe/backup-crypto.env}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/strict-env.sh
source "$SCRIPT_DIR/lib/strict-env.sh"
# shellcheck source=lib/encrypted-backup.sh
source "$SCRIPT_DIR/lib/encrypted-backup.sh"
if [[ "$CRYPTO_ENV_SOURCE" != "--current-env" ]]; then
  load_strict_env "$CRYPTO_ENV_SOURCE" '^BACKUP_[A-Z0-9_]+$'
fi

for command_name in age awk basename chmod cmp date dirname find findmnt flock getent grep mkdir mktemp mv openssl rm sha256sum sort stat tar tr; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done

: "${BACKUP_UPLOAD_SOURCE_DIR:?缺少 BACKUP_UPLOAD_SOURCE_DIR}"
: "${BACKUP_AGE_RECIPIENT:?缺少 BACKUP_AGE_RECIPIENT}"
: "${BACKUP_REQUIRE_OFFSITE:?缺少 BACKUP_REQUIRE_OFFSITE}"
: "${BACKUP_UPLOAD_PLAINTEXT_WORK_DIR:?缺少 BACKUP_UPLOAD_PLAINTEXT_WORK_DIR}"
UPLOAD_BACKUP_DIR="${BACKUP_UPLOAD_DIR:-/var/backups/wangzhe/uploads}"
RETENTION_DAYS="${BACKUP_UPLOAD_RETENTION_DAYS:-14}"
LOCK_WAIT_SECONDS="${BACKUP_LOCK_WAIT_SECONDS:-60}"

[[ "$BACKUP_UPLOAD_SOURCE_DIR" == /var/lib/wangzhe/uploads ]] || {
  echo "上传源目录必须精确设置为 /var/lib/wangzhe/uploads" >&2
  exit 1
}
validate_no_symlink_path_components "$BACKUP_UPLOAD_SOURCE_DIR"
[[ -d "$BACKUP_UPLOAD_SOURCE_DIR" && ! -L "$BACKUP_UPLOAD_SOURCE_DIR" ]] || {
  echo "上传源目录不存在或是符号链接：$BACKUP_UPLOAD_SOURCE_DIR" >&2
  exit 1
}
source_name="$(basename "$BACKUP_UPLOAD_SOURCE_DIR")"
source_parent="$(dirname "$BACKUP_UPLOAD_SOURCE_DIR")"
[[ "$source_name" =~ ^[A-Za-z0-9_.-]+$ && "$source_name" != -* ]] || { echo "上传目录名不安全" >&2; exit 1; }
unsafe_entry="$(find "$BACKUP_UPLOAD_SOURCE_DIR" -xdev \( -type l -o \( ! -type f -a ! -type d \) \) -print -quit)"
[[ -z "$unsafe_entry" ]] || { echo "上传目录包含符号链接或特殊文件，拒绝备份：$unsafe_entry" >&2; exit 1; }
while IFS= read -r -d '' source_entry; do
  [[ "$source_entry" != *$'\n'* && "$source_entry" != *$'\r'* ]] || { echo "上传目录包含换行文件名，拒绝备份" >&2; exit 1; }
done < <(find "$BACKUP_UPLOAD_SOURCE_DIR" -xdev -print0)
validate_age_recipient "$BACKUP_AGE_RECIPIENT"
[[ "$BACKUP_REQUIRE_OFFSITE" == "0" || "$BACKUP_REQUIRE_OFFSITE" == "1" ]] || {
  echo "BACKUP_REQUIRE_OFFSITE 只能是 0 或 1" >&2
  exit 1
}
if [[ -n "${BACKUP_REMOTE_DESTINATION:-}" ]]; then
  command -v rclone >/dev/null 2>&1 || { echo "配置异机备份时必须安装 rclone" >&2; exit 1; }
  validate_remote_destination "$BACKUP_REMOTE_DESTINATION"
  : "${BACKUP_RCLONE_CONFIG:?配置异机备份时必须设置 BACKUP_RCLONE_CONFIG}"
  : "${BACKUP_PROVENANCE_SIGNING_KEY_FILE:?配置异机备份时必须设置 BACKUP_PROVENANCE_SIGNING_KEY_FILE}"
  [[ "$BACKUP_PROVENANCE_SIGNING_KEY_FILE" == /etc/wangzhe/backup-provenance-ed25519-private.pem ]] || { echo "上传备份来源签名私钥必须使用固定独立路径" >&2; exit 1; }
  validate_rclone_config "$BACKUP_RCLONE_CONFIG"
  validate_ed25519_private_key "$BACKUP_PROVENANCE_SIGNING_KEY_FILE" "数据库/上传备份来源 Ed25519 私钥"
elif [[ "$BACKUP_REQUIRE_OFFSITE" == "1" ]]; then
  echo "生产备份要求异机副本，但未配置 BACKUP_REMOTE_DESTINATION" >&2
  exit 1
fi
[[ "$RETENTION_DAYS" =~ ^[0-9]+$ ]] && (( RETENTION_DAYS >= 1 && RETENTION_DAYS <= 3650 )) || {
  echo "BACKUP_UPLOAD_RETENTION_DAYS 必须是 1-3650 的整数" >&2
  exit 1
}
[[ "$LOCK_WAIT_SECONDS" =~ ^[0-9]+$ ]] && (( LOCK_WAIT_SECONDS >= 0 && LOCK_WAIT_SECONDS <= 600 )) || {
  echo "BACKUP_LOCK_WAIT_SECONDS 必须是 0-600 的整数" >&2
  exit 1
}

umask 077
validate_backup_directory "$UPLOAD_BACKUP_DIR" "上传备份目录"
validate_backup_monitor_directory "$UPLOAD_BACKUP_DIR"
exec 9>"$UPLOAD_BACKUP_DIR/.backup.lock"
flock -w "$LOCK_WAIT_SECONDS" 9 || { echo "另一个上传备份仍在运行" >&2; exit 1; }
validate_encrypted_work_directory "$BACKUP_UPLOAD_PLAINTEXT_WORK_DIR" "/var/lib/wangzhe-backup-work/uploads" "上传备份明文工作目录"

timestamp="$(date +%Y%m%d-%H%M%S)-$$"
target="$UPLOAD_BACKUP_DIR/uploads-${timestamp}.tar.age"
manifest_partial="$BACKUP_UPLOAD_PLAINTEXT_WORK_DIR/.uploads-${timestamp}.manifest.partial"
plaintext_partial="$BACKUP_UPLOAD_PLAINTEXT_WORK_DIR/.uploads-${timestamp}.tar.partial"
encrypted_partial="$target.partial"
checksum_partial="$target.sha256.partial"
offsite_partial="$target.offsite-ok.partial"
provenance_partial="$target.provenance.partial"
signature_partial="$target.provenance.sig.partial"
verify_dir=""
for candidate in "$target" "$target.sha256" "$target.offsite-ok" "$target.provenance" "$target.provenance.sig" "$manifest_partial" "$plaintext_partial" "$encrypted_partial" "$checksum_partial" "$offsite_partial" "$provenance_partial" "$signature_partial"; do
  [[ ! -e "$candidate" && ! -L "$candidate" ]] || { echo "同名上传备份文件已存在，拒绝覆盖" >&2; exit 1; }
done
cleanup_partial() {
  [[ -z "${manifest_partial:-}" ]] || rm -f -- "$manifest_partial"
  [[ -z "${plaintext_partial:-}" ]] || rm -f -- "$plaintext_partial"
  [[ -z "${encrypted_partial:-}" ]] || rm -f -- "$encrypted_partial"
  [[ -z "${checksum_partial:-}" ]] || rm -f -- "$checksum_partial"
  [[ -z "${offsite_partial:-}" ]] || rm -f -- "$offsite_partial"
  [[ -z "${provenance_partial:-}" ]] || rm -f -- "$provenance_partial"
  [[ -z "${signature_partial:-}" ]] || rm -f -- "$signature_partial"
  case "${verify_dir:-}" in "$BACKUP_UPLOAD_PLAINTEXT_WORK_DIR"/.verify-*) rm -rf -- "$verify_dir" ;; esac
  return 0
}
trap cleanup_partial EXIT INT TERM

(
  cd "$source_parent"
  while IFS= read -r -d '' path; do
    sha256sum "$path"
  done < <(find "$source_name" -xdev -type f -print0 | LC_ALL=C sort -z)
) >"$manifest_partial"

tar --create --file "$plaintext_partial" --numeric-owner --owner=0 --group=0 \
  --directory "$source_parent" "$source_name" \
  --directory "$BACKUP_UPLOAD_PLAINTEXT_WORK_DIR" "$(basename "$manifest_partial")"
tar --list --file "$plaintext_partial" >/dev/null
validate_no_symlink_path_components "$BACKUP_UPLOAD_SOURCE_DIR"
unsafe_entry="$(find "$BACKUP_UPLOAD_SOURCE_DIR" -xdev \( -type l -o \( ! -type f -a ! -type d \) \) -print -quit)"
[[ -z "$unsafe_entry" ]] || { echo "上传目录在备份期间出现符号链接或特殊文件，拒绝发布" >&2; exit 1; }
verify_dir="$(mktemp -d "$BACKUP_UPLOAD_PLAINTEXT_WORK_DIR/.verify-${timestamp}.XXXXXX")"
while IFS= read -r archive_entry; do
  case "$archive_entry" in
    "$source_name"|"$source_name"/*|"$(basename "$manifest_partial")") ;;
    *) echo "上传备份包含越界路径：$archive_entry" >&2; exit 1 ;;
  esac
done < <(tar --list --file "$plaintext_partial")
tar --extract --file "$plaintext_partial" --directory "$verify_dir" --no-same-owner --no-same-permissions
[[ -d "$verify_dir/$source_name" && ! -L "$verify_dir/$source_name" ]] || {
  echo "上传备份验证副本缺少安全的上传目录" >&2
  exit 1
}
unsafe_entry="$(find "$verify_dir/$source_name" -xdev \( -type l -o \( ! -type f -a ! -type d \) \) -print -quit)"
[[ -z "$unsafe_entry" ]] || { echo "上传备份验证副本包含符号链接或特殊文件" >&2; exit 1; }
while IFS= read -r -d '' archived_entry; do
  [[ "$archived_entry" != *$'\n'* && "$archived_entry" != *$'\r'* ]] || {
    echo "上传备份验证副本包含换行文件名" >&2
    exit 1
  }
done < <(find "$verify_dir/$source_name" -xdev -print0)
archive_manifest="$verify_dir/.archive-files-${timestamp}.manifest"
(
  cd "$verify_dir"
  while IFS= read -r -d '' archived_file; do
    sha256sum "$archived_file"
  done < <(find "$source_name" -xdev -type f -print0 | LC_ALL=C sort -z)
) >"$archive_manifest"
cmp -s "$verify_dir/$(basename "$manifest_partial")" "$archive_manifest" || {
  echo "上传源目录在清单与归档期间发生变化，拒绝发布不一致备份" >&2
  exit 1
}
if [[ -s "$verify_dir/$(basename "$manifest_partial")" ]]; then
  (
    cd "$verify_dir"
    sha256sum --check --strict "$(basename "$manifest_partial")" >/dev/null
  )
fi
rm -rf -- "$verify_dir"
verify_dir=""
encrypt_backup_file "$plaintext_partial" "$encrypted_partial" "$BACKUP_AGE_RECIPIENT"
rm -f -- "$plaintext_partial" "$manifest_partial"
plaintext_partial=""
manifest_partial=""
mv "$encrypted_partial" "$target"
encrypted_partial=""
write_backup_checksum "$target" "$checksum_partial"
mv "$checksum_partial" "$target.sha256"
checksum_partial=""
if [[ -n "${BACKUP_REMOTE_DESTINATION:-}" ]]; then
  provenance_created_epoch="$(date +%s)"
  write_backup_provenance "$target" "$BACKUP_REMOTE_DESTINATION" uploads "$BACKUP_UPLOAD_SOURCE_DIR" "$provenance_created_epoch" \
    "$BACKUP_PROVENANCE_SIGNING_KEY_FILE" "$provenance_partial" "$signature_partial"
  mv "$provenance_partial" "$target.provenance"
  provenance_partial=""
  mv "$signature_partial" "$target.provenance.sig"
  signature_partial=""
  verify_backup_provenance "$target" uploads "$BACKUP_UPLOAD_SOURCE_DIR" \
    "${BACKUP_REMOTE_DESTINATION%/}/$(basename "$target")" "$BACKUP_PROVENANCE_SIGNING_KEY_FILE" private || { echo "上传备份来源凭证自检失败" >&2; exit 1; }
  make_backup_artifacts_monitor_readable "$target" "$target.sha256" "$target.provenance" "$target.provenance.sig"
  sync_backup_offsite "$target" "$target.sha256" "$BACKUP_REMOTE_DESTINATION" "$offsite_partial" "$BACKUP_RCLONE_CONFIG" \
    "$target.provenance" "$target.provenance.sig"
  mv "$offsite_partial" "$target.offsite-ok"
  offsite_partial=""
  validate_offsite_marker "$target" "$BACKUP_REMOTE_DESTINATION" || { echo "上传目录异机回读凭证无效" >&2; exit 1; }
  make_backup_artifacts_monitor_readable "$target.offsite-ok"
else
  make_backup_artifacts_monitor_readable "$target" "$target.sha256"
fi

find "$UPLOAD_BACKUP_DIR" -maxdepth 1 -type f \
  \( -name 'uploads-*.tar.age' -o -name 'uploads-*.tar.age.sha256' -o -name 'uploads-*.tar.age.offsite-ok' -o -name 'uploads-*.tar.age.provenance' -o -name 'uploads-*.tar.age.provenance.sig' \) \
  -mtime "+$RETENTION_DAYS" -print -delete

echo "上传目录加密备份完成：$target"
