#!/usr/bin/env bash
set -euo pipefail

ENV_SOURCE="${1:-/etc/wangzhe/backend.env}"
CRYPTO_ENV_SOURCE="${2:-/etc/wangzhe/backup-crypto.env}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/backend-env.sh
source "$SCRIPT_DIR/lib/backend-env.sh"
# shellcheck source=lib/strict-env.sh
source "$SCRIPT_DIR/lib/strict-env.sh"
# shellcheck source=lib/encrypted-backup.sh
source "$SCRIPT_DIR/lib/encrypted-backup.sh"
if [[ "$ENV_SOURCE" == "--current-env" ]]; then
  : # The caller deliberately supplied every BACKEND_* value in this process.
else
  load_backend_env "$ENV_SOURCE"
  load_strict_env "$CRYPTO_ENV_SOURCE" '^BACKUP_[A-Z0-9_]+$'
fi
BACKUP_DIR="${BACKUP_DIR:-/var/backups/wangzhe}"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-14}"
LOCK_WAIT_SECONDS="${BACKUP_LOCK_WAIT_SECONDS:-60}"

for command_name in age awk basename chmod date dirname find findmnt flock getent grep mkdir mv openssl pg_dump pg_restore rm sha256sum stat tr; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done

: "${BACKEND_DATABASE_HOST:?缺少 BACKEND_DATABASE_HOST}"
: "${BACKEND_DATABASE_PORT:?缺少 BACKEND_DATABASE_PORT}"
: "${BACKEND_DATABASE_USER:?缺少 BACKEND_DATABASE_USER}"
: "${BACKEND_DATABASE_DBNAME:?缺少 BACKEND_DATABASE_DBNAME}"
: "${BACKEND_DATABASE_PASSWORD:?缺少 BACKEND_DATABASE_PASSWORD}"
: "${BACKEND_DATABASE_SSLMODE:?缺少 BACKEND_DATABASE_SSLMODE}"
case "$BACKEND_DATABASE_SSLMODE" in
  disable|verify-ca|verify-full) ;;
  *) echo "BACKEND_DATABASE_SSLMODE 不正确" >&2; exit 1 ;;
esac
case "$BACKEND_DATABASE_HOST" in
  localhost|127.0.0.1|::1) ;;
  *) [[ "$BACKEND_DATABASE_SSLMODE" == "verify-ca" || "$BACKEND_DATABASE_SSLMODE" == "verify-full" ]] || {
    echo "远程 PostgreSQL 备份必须校验证书（verify-ca/verify-full）" >&2
    exit 1
  } ;;
esac
[[ -z "${APPLICATION_DATABASE_USER:-}" || "$BACKEND_DATABASE_USER" != "$APPLICATION_DATABASE_USER" ]] || {
  echo "备份必须使用独立于应用的数据库账号" >&2
  exit 1
}
: "${BACKUP_AGE_RECIPIENT:?缺少 BACKUP_AGE_RECIPIENT}"
: "${BACKUP_REQUIRE_OFFSITE:?缺少 BACKUP_REQUIRE_OFFSITE}"
: "${BACKUP_PLAINTEXT_WORK_DIR:?缺少 BACKUP_PLAINTEXT_WORK_DIR}"
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
  [[ "$BACKUP_PROVENANCE_SIGNING_KEY_FILE" == /etc/wangzhe/backup-provenance-ed25519-private.pem ]] || { echo "数据库备份来源签名私钥必须使用固定独立路径" >&2; exit 1; }
  validate_rclone_config "$BACKUP_RCLONE_CONFIG"
  validate_ed25519_private_key "$BACKUP_PROVENANCE_SIGNING_KEY_FILE" "数据库/上传备份来源 Ed25519 私钥"
elif [[ "$BACKUP_REQUIRE_OFFSITE" == "1" ]]; then
  echo "生产备份要求异机副本，但未配置 BACKUP_REMOTE_DESTINATION" >&2
  exit 1
fi

umask 077
validate_backup_directory "$BACKUP_DIR"
validate_backup_monitor_directory "$BACKUP_DIR"
[[ "$BACKEND_DATABASE_DBNAME" =~ ^[A-Za-z0-9_.-]+$ ]] || { echo "数据库名不能用于安全的备份文件名" >&2; exit 1; }
[[ "$RETENTION_DAYS" =~ ^[0-9]+$ ]] && (( RETENTION_DAYS >= 1 && RETENTION_DAYS <= 3650 )) || {
  echo "BACKUP_RETENTION_DAYS 必须是 1-3650 的整数" >&2
  exit 1
}
[[ "$LOCK_WAIT_SECONDS" =~ ^[0-9]+$ ]] && (( LOCK_WAIT_SECONDS >= 0 && LOCK_WAIT_SECONDS <= 600 )) || {
  echo "BACKUP_LOCK_WAIT_SECONDS 必须是 0-600 的整数" >&2
  exit 1
}

exec 9>"$BACKUP_DIR/.backup.lock"
flock -w "$LOCK_WAIT_SECONDS" 9 || { echo "另一个数据库备份仍在运行" >&2; exit 1; }
validate_encrypted_work_directory "$BACKUP_PLAINTEXT_WORK_DIR" "/var/lib/wangzhe-backup-work/database" "数据库备份明文工作目录"
timestamp="$(date +%Y%m%d-%H%M%S)-$$"
target="$BACKUP_DIR/${BACKEND_DATABASE_DBNAME}-${timestamp}.dump.age"
plaintext_partial="$BACKUP_PLAINTEXT_WORK_DIR/.${BACKEND_DATABASE_DBNAME}-${timestamp}.dump.partial"
encrypted_partial="$target.partial"
checksum_partial="$target.sha256.partial"
offsite_partial="$target.offsite-ok.partial"
provenance_partial="$target.provenance.partial"
signature_partial="$target.provenance.sig.partial"
for candidate in "$target" "$target.sha256" "$target.offsite-ok" "$target.provenance" "$target.provenance.sig" "$plaintext_partial" "$encrypted_partial" "$checksum_partial" "$offsite_partial" "$provenance_partial" "$signature_partial"; do
  [[ ! -e "$candidate" && ! -L "$candidate" ]] || { echo "同名备份文件已存在，拒绝覆盖：$candidate" >&2; exit 1; }
done
cleanup_partial() {
  [[ -z "${plaintext_partial:-}" ]] || rm -f -- "$plaintext_partial"
  [[ -z "${encrypted_partial:-}" ]] || rm -f -- "$encrypted_partial"
  [[ -z "${checksum_partial:-}" ]] || rm -f -- "$checksum_partial"
  [[ -z "${offsite_partial:-}" ]] || rm -f -- "$offsite_partial"
  [[ -z "${provenance_partial:-}" ]] || rm -f -- "$provenance_partial"
  [[ -z "${signature_partial:-}" ]] || rm -f -- "$signature_partial"
  return 0
}
trap cleanup_partial EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

export PGPASSWORD="$BACKEND_DATABASE_PASSWORD"
export PGSSLMODE="$BACKEND_DATABASE_SSLMODE"
pg_dump \
  --host "$BACKEND_DATABASE_HOST" \
  --port "$BACKEND_DATABASE_PORT" \
  --username "$BACKEND_DATABASE_USER" \
  --dbname "$BACKEND_DATABASE_DBNAME" \
  --format custom \
  --no-owner \
  --file "$plaintext_partial"
pg_restore --list "$plaintext_partial" >/dev/null
unset PGPASSWORD PGSSLMODE
[[ -s "$plaintext_partial" ]] || { echo "备份文件为空" >&2; exit 1; }
encrypt_backup_file "$plaintext_partial" "$encrypted_partial" "$BACKUP_AGE_RECIPIENT"
rm -f -- "$plaintext_partial"
plaintext_partial=""
mv "$encrypted_partial" "$target"
encrypted_partial=""
write_backup_checksum "$target" "$checksum_partial"
mv "$checksum_partial" "$target.sha256"
checksum_partial=""
if [[ -n "${BACKUP_REMOTE_DESTINATION:-}" ]]; then
  provenance_created_epoch="$(date +%s)"
  write_backup_provenance "$target" "$BACKUP_REMOTE_DESTINATION" database "$BACKEND_DATABASE_DBNAME" "$provenance_created_epoch" \
    "$BACKUP_PROVENANCE_SIGNING_KEY_FILE" "$provenance_partial" "$signature_partial"
  mv "$provenance_partial" "$target.provenance"
  provenance_partial=""
  mv "$signature_partial" "$target.provenance.sig"
  signature_partial=""
  verify_backup_provenance "$target" database "$BACKEND_DATABASE_DBNAME" \
    "${BACKUP_REMOTE_DESTINATION%/}/$(basename "$target")" "$BACKUP_PROVENANCE_SIGNING_KEY_FILE" private || { echo "数据库备份来源凭证自检失败" >&2; exit 1; }
  make_backup_artifacts_monitor_readable "$target" "$target.sha256" "$target.provenance" "$target.provenance.sig"
  sync_backup_offsite "$target" "$target.sha256" "$BACKUP_REMOTE_DESTINATION" "$offsite_partial" "$BACKUP_RCLONE_CONFIG" \
    "$target.provenance" "$target.provenance.sig"
  mv "$offsite_partial" "$target.offsite-ok"
  offsite_partial=""
  validate_offsite_marker "$target" "$BACKUP_REMOTE_DESTINATION" || { echo "数据库异机回读凭证无效" >&2; exit 1; }
  make_backup_artifacts_monitor_readable "$target.offsite-ok"
else
  make_backup_artifacts_monitor_readable "$target" "$target.sha256"
fi

find "$BACKUP_DIR" -maxdepth 1 -type f \
  \( -name "${BACKEND_DATABASE_DBNAME}-*.dump.age" -o -name "${BACKEND_DATABASE_DBNAME}-*.dump.age.sha256" -o -name "${BACKEND_DATABASE_DBNAME}-*.dump.age.offsite-ok" -o -name "${BACKEND_DATABASE_DBNAME}-*.dump.age.provenance" -o -name "${BACKEND_DATABASE_DBNAME}-*.dump.age.provenance.sig" \) \
  -mtime "+$RETENTION_DAYS" -print -delete

echo "数据库加密备份完成：$target"
