#!/usr/bin/env bash
set -euo pipefail

ENV_SOURCE="${1:-/etc/wangzhe/pitr-restore.env}"
WAL_NAME="${2:-}"
RESTORE_TARGET="${3:-}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/strict-env.sh
source "$SCRIPT_DIR/lib/strict-env.sh"
# shellcheck source=lib/encrypted-backup.sh
source "$SCRIPT_DIR/lib/encrypted-backup.sh"
if [[ "$ENV_SOURCE" != "--current-env" ]]; then
  load_strict_env "$ENV_SOURCE" '^PITR_RESTORE_[A-Z0-9_]+$'
fi
for command_name in age awk basename date dirname grep mktemp mv openssl rm rmdir sha256sum stat; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done
: "${PITR_RESTORE_AGE_IDENTITY_FILE:?缺少 PITR_RESTORE_AGE_IDENTITY_FILE}"
: "${PITR_RESTORE_LOCAL_ARCHIVE_DIR:?缺少 PITR_RESTORE_LOCAL_ARCHIVE_DIR}"
: "${PITR_RESTORE_CLUSTER_ID:?缺少 PITR_RESTORE_CLUSTER_ID}"
: "${PITR_RESTORE_PROVENANCE_VERIFY_KEY_FILE:?缺少 PITR_RESTORE_PROVENANCE_VERIFY_KEY_FILE}"
[[ "$PITR_RESTORE_CLUSTER_ID" =~ ^[0-9]{10,30}$ ]] || { echo "PITR_RESTORE_CLUSTER_ID 必须是 PostgreSQL system identifier" >&2; exit 1; }
[[ "$(basename "$PITR_RESTORE_LOCAL_ARCHIVE_DIR")" == "$PITR_RESTORE_CLUSTER_ID" ]] || { echo "本地 WAL 目录末级必须等于 PITR_RESTORE_CLUSTER_ID" >&2; exit 1; }
[[ "$WAL_NAME" =~ ^([0-9A-F]{24}(\.[0-9A-F]{8}\.backup)?|[0-9A-F]{8}\.history)$ ]] || { echo "WAL 文件名无效" >&2; exit 1; }
[[ -n "$RESTORE_TARGET" && "$(basename "$RESTORE_TARGET")" == "$WAL_NAME" && ! -e "$RESTORE_TARGET" && ! -L "$RESTORE_TARGET" ]] || {
  echo "WAL 恢复目标必须是尚不存在且名称匹配的精确路径" >&2
  exit 1
}
restore_parent="$(dirname "$RESTORE_TARGET")"
[[ -d "$restore_parent" && ! -L "$restore_parent" ]] || { echo "WAL 恢复目标父目录无效" >&2; exit 1; }
restore_parent="$(cd "$restore_parent" && pwd -P)"
validate_no_symlink_path_components "$restore_parent"
RESTORE_TARGET="$restore_parent/$WAL_NAME"
[[ ! -e "$RESTORE_TARGET" && ! -L "$RESTORE_TARGET" ]] || { echo "规范化后的 WAL 恢复目标已存在" >&2; exit 1; }
validate_private_file "$PITR_RESTORE_AGE_IDENTITY_FILE" "AGE 恢复身份文件"
[[ "$PITR_RESTORE_PROVENANCE_VERIFY_KEY_FILE" == /etc/wangzhe/pitr-provenance-ed25519-public.pem ]] || { echo "PITR 来源验签公钥必须使用固定路径" >&2; exit 1; }
validate_ed25519_public_key "$PITR_RESTORE_PROVENANCE_VERIFY_KEY_FILE" "PITR 备份来源 Ed25519 公钥"
[[ -d "$PITR_RESTORE_LOCAL_ARCHIVE_DIR" && ! -L "$PITR_RESTORE_LOCAL_ARCHIVE_DIR" ]] || { echo "本地 WAL 归档目录无效" >&2; exit 1; }
validate_no_symlink_path_components "$PITR_RESTORE_LOCAL_ARCHIVE_DIR"
umask 077
encrypted="$PITR_RESTORE_LOCAL_ARCHIVE_DIR/$WAL_NAME.age"
temporary_remote=""
temporary_checksum=""
temporary_source_manifest=""
temporary_provenance=""
temporary_signature=""
temporary_directory=""
partial=""
cleanup_restore() {
  [[ -z "${partial:-}" ]] || rm -f -- "$partial"
  [[ -z "${temporary_remote:-}" ]] || rm -f -- "$temporary_remote"
  [[ -z "${temporary_checksum:-}" ]] || rm -f -- "$temporary_checksum"
  [[ -z "${temporary_source_manifest:-}" ]] || rm -f -- "$temporary_source_manifest"
  [[ -z "${temporary_provenance:-}" ]] || rm -f -- "$temporary_provenance"
  [[ -z "${temporary_signature:-}" ]] || rm -f -- "$temporary_signature"
  [[ -z "${temporary_directory:-}" ]] || rmdir -- "$temporary_directory" 2>/dev/null || true
}
trap cleanup_restore EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
provenance_expected_remote=""
source_manifest="$encrypted.source.sha256"
if [[ -f "$encrypted" && ! -L "$encrypted" ]]; then
  validate_encrypted_backup_and_manifest "$encrypted" || { echo "本地 WAL 加密制品或校验清单无效" >&2; exit 1; }
  [[ -f "$source_manifest" && ! -L "$source_manifest" ]] || { echo "本地 WAL 缺少源文件凭证" >&2; exit 1; }
  validate_offsite_marker "$encrypted" "${PITR_RESTORE_REMOTE_DESTINATION:-}" || { echo "本地 WAL 缺少有效异机来源凭证" >&2; exit 1; }
  read -r _ provenance_expected_remote _ <"$encrypted.offsite-ok" || { echo "本地 WAL 异机来源凭证不可读" >&2; exit 1; }
elif [[ -e "$encrypted" || -L "$encrypted" ]]; then
  echo "本地 WAL 加密制品不是普通文件" >&2
  exit 1
else
  : "${PITR_RESTORE_REMOTE_DESTINATION:?本地缺失 WAL 且未配置远端归档}"
  : "${PITR_RESTORE_RCLONE_CONFIG:?远端恢复 WAL 必须配置 PITR_RESTORE_RCLONE_CONFIG}"
  [[ "$PITR_RESTORE_RCLONE_CONFIG" == /etc/wangzhe/pitr-wal-read-rclone.conf ]] || { echo "PITR 远端 WAL 恢复必须使用固定的只读 rclone 配置" >&2; exit 1; }
  command -v rclone >/dev/null 2>&1 || { echo "远端恢复 WAL 必须安装 rclone" >&2; exit 1; }
  validate_remote_destination "$PITR_RESTORE_REMOTE_DESTINATION"
  [[ "$(basename "${PITR_RESTORE_REMOTE_DESTINATION#*:}")" == "$PITR_RESTORE_CLUSTER_ID" ]] || { echo "远端 WAL 目录末级必须等于 PITR_RESTORE_CLUSTER_ID" >&2; exit 1; }
  validate_rclone_config "$PITR_RESTORE_RCLONE_CONFIG"
  # Preserve the signed artifact basename inside a random private directory.
  # Provenance deliberately binds artifact_name to "$WAL_NAME.age"; naming the
  # downloaded file itself .restore-*.XXXXXX would make every remote-only
  # recovery fail closed even when all five remote objects are authentic.
  temporary_directory="$(mktemp -d "$PITR_RESTORE_LOCAL_ARCHIVE_DIR/.restore-$WAL_NAME.XXXXXX")"
  temporary_remote="$temporary_directory/$WAL_NAME.age"
  temporary_checksum="$temporary_remote.sha256"
  temporary_source_manifest="$temporary_remote.source.sha256"
  temporary_provenance="$temporary_remote.provenance"
  temporary_signature="$temporary_remote.provenance.sig"
  rclone --config "$PITR_RESTORE_RCLONE_CONFIG" --contimeout 10s --timeout 2m --retries 2 --low-level-retries 2 copyto "${PITR_RESTORE_REMOTE_DESTINATION%/}/$WAL_NAME.age" "$temporary_remote" --no-traverse
  rclone --config "$PITR_RESTORE_RCLONE_CONFIG" --contimeout 10s --timeout 2m --retries 2 --low-level-retries 2 copyto "${PITR_RESTORE_REMOTE_DESTINATION%/}/$WAL_NAME.age.sha256" "$temporary_checksum" --no-traverse
  rclone --config "$PITR_RESTORE_RCLONE_CONFIG" --contimeout 10s --timeout 2m --retries 2 --low-level-retries 2 copyto "${PITR_RESTORE_REMOTE_DESTINATION%/}/$WAL_NAME.age.source.sha256" "$temporary_source_manifest" --no-traverse
  rclone --config "$PITR_RESTORE_RCLONE_CONFIG" --contimeout 10s --timeout 2m --retries 2 --low-level-retries 2 copyto "${PITR_RESTORE_REMOTE_DESTINATION%/}/$WAL_NAME.age.provenance" "$temporary_provenance" --no-traverse
  rclone --config "$PITR_RESTORE_RCLONE_CONFIG" --contimeout 10s --timeout 2m --retries 2 --low-level-retries 2 copyto "${PITR_RESTORE_REMOTE_DESTINATION%/}/$WAL_NAME.age.provenance.sig" "$temporary_signature" --no-traverse
  read -r downloaded_checksum downloaded_name downloaded_extra <"$temporary_checksum" || { echo "远端 WAL 校验清单不可读" >&2; exit 1; }
  actual_downloaded_checksum="$(sha256sum "$temporary_remote" | awk '{print $1}')"
  [[ "$downloaded_checksum" =~ ^[0-9a-f]{64}$ && "$downloaded_checksum" == "$actual_downloaded_checksum" && "$downloaded_name" == "$WAL_NAME.age" && -z "${downloaded_extra:-}" ]] || {
    echo "远端 WAL 加密制品校验失败" >&2
    exit 1
  }
  encrypted="$temporary_remote"
  source_manifest="$temporary_source_manifest"
  provenance_expected_remote="${PITR_RESTORE_REMOTE_DESTINATION%/}/$WAL_NAME.age"
fi
verify_backup_provenance "$encrypted" pitr-wal "$PITR_RESTORE_CLUSTER_ID" "$provenance_expected_remote" \
  "$PITR_RESTORE_PROVENANCE_VERIFY_KEY_FILE" || { echo "WAL 来源签名或绑定字段无效" >&2; exit 1; }
[[ -f "$source_manifest" && ! -L "$source_manifest" ]] || { echo "WAL 源文件凭证无效" >&2; exit 1; }
read -r expected_plaintext_checksum recorded_wal_name recorded_cluster_id recorded_extra <"$source_manifest" || { echo "WAL 源文件凭证不可读" >&2; exit 1; }
[[ "$expected_plaintext_checksum" =~ ^[0-9a-f]{64}$ && "$recorded_wal_name" == "$WAL_NAME" && "$recorded_cluster_id" == "$PITR_RESTORE_CLUSTER_ID" && -z "${recorded_extra:-}" ]] || {
  echo "WAL 源文件凭证与目标集群不匹配" >&2
  exit 1
}
partial="$RESTORE_TARGET.partial.$$"
[[ ! -e "$partial" && ! -L "$partial" ]] || { echo "WAL 恢复临时文件已存在" >&2; exit 1; }
age --decrypt --identity "$PITR_RESTORE_AGE_IDENTITY_FILE" --output "$partial" "$encrypted"
[[ -s "$partial" ]] || { echo "解密后的 WAL 为空" >&2; exit 1; }
actual_plaintext_checksum="$(sha256sum "$partial" | awk '{print $1}')"
[[ "$actual_plaintext_checksum" == "$expected_plaintext_checksum" ]] || { echo "解密后的 WAL 与源文件凭证不一致" >&2; exit 1; }
mv "$partial" "$RESTORE_TARGET"
partial=""
echo "WAL 恢复完成：$WAL_NAME"
