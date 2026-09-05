#!/usr/bin/env bash
set -euo pipefail

ENV_SOURCE="${1:-/etc/wangzhe/pitr.env}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/strict-env.sh
source "$SCRIPT_DIR/lib/strict-env.sh"
# shellcheck source=lib/encrypted-backup.sh
source "$SCRIPT_DIR/lib/encrypted-backup.sh"
if [[ "$ENV_SOURCE" != "--current-env" ]]; then
  load_strict_env "$ENV_SOURCE" '^PITR_[A-Z0-9_]+$'
fi
for command_name in age awk basename chmod date dirname find findmnt flock getent grep mkdir mktemp mv openssl pg_basebackup pg_controldata pg_verifybackup rm sha256sum stat tar tr; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done
for key in PITR_DATABASE_HOST PITR_DATABASE_PORT PITR_DATABASE_USER PITR_DATABASE_PASSWORD PITR_DATABASE_SSLMODE PITR_AGE_RECIPIENT PITR_REQUIRE_OFFSITE PITR_BASEBACKUP_DIR; do
  [[ -n "${!key:-}" ]] || { echo "缺少 $key" >&2; exit 1; }
done
: "${PITR_CLUSTER_ID:?缺少 PITR_CLUSTER_ID}"
: "${PITR_CLUSTER_ID_FILE:=/etc/wangzhe/pitr-cluster-id}"
: "${PITR_PLAINTEXT_WORK_DIR:?缺少 PITR_PLAINTEXT_WORK_DIR}"
[[ "$PITR_CLUSTER_ID" =~ ^[0-9]{10,30}$ ]] || { echo "PITR_CLUSTER_ID 必须是 PostgreSQL system identifier" >&2; exit 1; }
[[ -f "$PITR_CLUSTER_ID_FILE" && ! -L "$PITR_CLUSTER_ID_FILE" && "$(stat -c '%u' "$PITR_CLUSTER_ID_FILE")" == 0 && -z "$(find "$PITR_CLUSTER_ID_FILE" -perm /022 -print -quit)" ]] || {
  echo "PITR 集群标识信任文件必须由 root 保护" >&2
  exit 1
}
validate_no_symlink_path_components "$PITR_CLUSTER_ID_FILE"
read -r trusted_cluster_id trusted_cluster_extra <"$PITR_CLUSTER_ID_FILE" || { echo "PITR 集群标识信任文件不可读" >&2; exit 1; }
[[ "$trusted_cluster_id" == "$PITR_CLUSTER_ID" && -z "${trusted_cluster_extra:-}" ]] || { echo "PITR_CLUSTER_ID 与 root 信任文件不一致" >&2; exit 1; }
[[ "$(basename "$PITR_BASEBACKUP_DIR")" == "$PITR_CLUSTER_ID" ]] || { echo "基础备份目录末级必须等于 PITR_CLUSTER_ID" >&2; exit 1; }
[[ "$PITR_DATABASE_PORT" =~ ^[0-9]+$ ]] && (( PITR_DATABASE_PORT >= 1 && PITR_DATABASE_PORT <= 65535 )) || { echo "PITR_DATABASE_PORT 无效" >&2; exit 1; }
case "$PITR_DATABASE_SSLMODE" in disable|verify-ca|verify-full) ;; *) echo "PITR_DATABASE_SSLMODE 无效" >&2; exit 1;; esac
case "$PITR_DATABASE_HOST" in localhost|127.0.0.1|::1) ;; *) [[ "$PITR_DATABASE_SSLMODE" == verify-ca || "$PITR_DATABASE_SSLMODE" == verify-full ]] || { echo "远程 PostgreSQL 基础备份必须校验证书" >&2; exit 1; };; esac
validate_age_recipient "$PITR_AGE_RECIPIENT"
[[ "$PITR_REQUIRE_OFFSITE" == 0 || "$PITR_REQUIRE_OFFSITE" == 1 ]] || { echo "PITR_REQUIRE_OFFSITE 只能是 0 或 1" >&2; exit 1; }
if [[ -n "${PITR_REMOTE_DESTINATION:-}" ]]; then
  command -v rclone >/dev/null 2>&1 || { echo "配置基础备份异机同步时必须安装 rclone" >&2; exit 1; }
  validate_remote_destination "$PITR_REMOTE_DESTINATION"
  [[ "$(basename "${PITR_REMOTE_DESTINATION#*:}")" == "$PITR_CLUSTER_ID" ]] || { echo "基础备份异机目录末级必须等于 PITR_CLUSTER_ID" >&2; exit 1; }
  : "${PITR_RCLONE_CONFIG:?配置基础备份异机同步时必须设置 PITR_RCLONE_CONFIG}"
  : "${PITR_PROVENANCE_SIGNING_KEY_FILE:?配置基础备份异机同步时必须设置 PITR_PROVENANCE_SIGNING_KEY_FILE}"
  [[ "$PITR_PROVENANCE_SIGNING_KEY_FILE" == /etc/wangzhe/pitr-provenance-ed25519-private.pem ]] || { echo "PITR 备份来源签名私钥必须使用固定独立路径" >&2; exit 1; }
  validate_rclone_config "$PITR_RCLONE_CONFIG"
  validate_ed25519_private_key "$PITR_PROVENANCE_SIGNING_KEY_FILE" "PITR 备份来源 Ed25519 私钥"
elif [[ "$PITR_REQUIRE_OFFSITE" == 1 ]]; then
  echo "基础备份要求异机副本，但未配置 PITR_REMOTE_DESTINATION" >&2
  exit 1
fi
RETENTION_DAYS="${PITR_BASEBACKUP_RETENTION_DAYS:-35}"
LOCK_WAIT_SECONDS="${PITR_LOCK_WAIT_SECONDS:-60}"
[[ "$RETENTION_DAYS" =~ ^[0-9]+$ ]] && (( RETENTION_DAYS >= 7 && RETENTION_DAYS <= 3650 )) || { echo "PITR_BASEBACKUP_RETENTION_DAYS 必须是 7-3650" >&2; exit 1; }
[[ "$LOCK_WAIT_SECONDS" =~ ^[0-9]+$ ]] && (( LOCK_WAIT_SECONDS >= 0 && LOCK_WAIT_SECONDS <= 600 )) || { echo "PITR_LOCK_WAIT_SECONDS 无效" >&2; exit 1; }

umask 077
validate_backup_directory "$PITR_BASEBACKUP_DIR" "基础备份目录"
validate_backup_monitor_directory "$PITR_BASEBACKUP_DIR"
exec 9>"$PITR_BASEBACKUP_DIR/.basebackup.lock"
flock -w "$LOCK_WAIT_SECONDS" 9 || { echo "另一个基础备份仍在运行" >&2; exit 1; }
validate_encrypted_work_directory "$PITR_PLAINTEXT_WORK_DIR" "/var/lib/wangzhe-backup-work/pitr" "PITR 基础备份明文工作目录"
timestamp="$(date +%Y%m%d-%H%M%S)-$$"
work_dir=""
plaintext_partial="$PITR_PLAINTEXT_WORK_DIR/.basebackup-${timestamp}.tar.partial"
target="$PITR_BASEBACKUP_DIR/basebackup-${timestamp}.tar.age"
encrypted_partial="$target.partial"
checksum_partial="$target.sha256.partial"
offsite_partial="$target.offsite-ok.partial"
provenance_partial="$target.provenance.partial"
signature_partial="$target.provenance.sig.partial"
for candidate in "$plaintext_partial" "$target" "$target.sha256" "$target.offsite-ok" "$target.provenance" "$target.provenance.sig" "$encrypted_partial" "$checksum_partial" "$offsite_partial" "$provenance_partial" "$signature_partial"; do
  [[ ! -e "$candidate" && ! -L "$candidate" ]] || { echo "同名基础备份文件已存在，拒绝覆盖：$candidate" >&2; exit 1; }
done
cleanup_partial() {
  case "${work_dir:-}" in "$PITR_PLAINTEXT_WORK_DIR"/.basebackup-*) rm -rf -- "$work_dir" ;; esac
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
work_dir="$(mktemp -d "$PITR_PLAINTEXT_WORK_DIR/.basebackup-${timestamp}.XXXXXX")"

export PGPASSWORD="$PITR_DATABASE_PASSWORD"
export PGSSLMODE="$PITR_DATABASE_SSLMODE"
pg_basebackup --host "$PITR_DATABASE_HOST" --port "$PITR_DATABASE_PORT" --username "$PITR_DATABASE_USER" \
  --pgdata "$work_dir/data" --format=plain --wal-method=stream --checkpoint=fast --no-password \
  --manifest-checksums=SHA256
unset PGPASSWORD PGSSLMODE
pg_verifybackup "$work_dir/data"
backup_cluster_id="$(pg_controldata "$work_dir/data" | awk -F: '/Database system identifier/ {gsub(/[[:space:]]/, "", $2); print $2}')"
[[ "$backup_cluster_id" == "$PITR_CLUSTER_ID" ]] || { echo "基础备份实际 system_identifier 与配置不一致" >&2; exit 1; }
tar --create --file "$plaintext_partial" --numeric-owner --owner=0 --group=0 --directory "$work_dir" data
encrypt_backup_file "$plaintext_partial" "$encrypted_partial" "$PITR_AGE_RECIPIENT"
rm -rf -- "$work_dir"
work_dir=""
rm -f -- "$plaintext_partial"
plaintext_partial=""
mv "$encrypted_partial" "$target"
encrypted_partial=""
write_backup_checksum "$target" "$checksum_partial"
mv "$checksum_partial" "$target.sha256"
checksum_partial=""
if [[ -n "${PITR_REMOTE_DESTINATION:-}" ]]; then
  provenance_created_epoch="$(date +%s)"
  write_backup_provenance "$target" "$PITR_REMOTE_DESTINATION" pitr-basebackup "$PITR_CLUSTER_ID" "$provenance_created_epoch" \
    "$PITR_PROVENANCE_SIGNING_KEY_FILE" "$provenance_partial" "$signature_partial"
  mv "$provenance_partial" "$target.provenance"
  provenance_partial=""
  mv "$signature_partial" "$target.provenance.sig"
  signature_partial=""
  verify_backup_provenance "$target" pitr-basebackup "$PITR_CLUSTER_ID" \
    "${PITR_REMOTE_DESTINATION%/}/$(basename "$target")" "$PITR_PROVENANCE_SIGNING_KEY_FILE" private || { echo "PITR 基础备份来源凭证自检失败" >&2; exit 1; }
  make_backup_artifacts_monitor_readable "$target" "$target.sha256" "$target.provenance" "$target.provenance.sig"
  sync_backup_offsite "$target" "$target.sha256" "$PITR_REMOTE_DESTINATION" "$offsite_partial" "$PITR_RCLONE_CONFIG" \
    "$target.provenance" "$target.provenance.sig"
  mv "$offsite_partial" "$target.offsite-ok"
  offsite_partial=""
  validate_offsite_marker "$target" "$PITR_REMOTE_DESTINATION" || { echo "基础备份异机回读凭证无效" >&2; exit 1; }
  make_backup_artifacts_monitor_readable "$target.offsite-ok"
else
  make_backup_artifacts_monitor_readable "$target" "$target.sha256"
fi
find "$PITR_BASEBACKUP_DIR" -maxdepth 1 -type f \
  \( -name 'basebackup-*.tar.age' -o -name 'basebackup-*.tar.age.sha256' -o -name 'basebackup-*.tar.age.offsite-ok' -o -name 'basebackup-*.tar.age.provenance' -o -name 'basebackup-*.tar.age.provenance.sig' \) \
  -mtime "+$RETENTION_DAYS" -print -delete
echo "PostgreSQL 加密基础备份完成：$target"
