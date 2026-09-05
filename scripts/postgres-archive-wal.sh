#!/usr/bin/env bash
set -euo pipefail

ENV_SOURCE="${1:-/etc/wangzhe/pitr.env}"
WAL_SOURCE="${2:-}"
WAL_NAME="${3:-}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/strict-env.sh
source "$SCRIPT_DIR/lib/strict-env.sh"
# shellcheck source=lib/encrypted-backup.sh
source "$SCRIPT_DIR/lib/encrypted-backup.sh"
if [[ "$ENV_SOURCE" != "--current-env" ]]; then
  load_strict_env "$ENV_SOURCE" '^PITR_[A-Z0-9_]+$'
fi

for command_name in age awk basename chmod date dirname find flock getent grep mkdir mv openssl rm sha256sum stat tr; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done
: "${PITR_AGE_RECIPIENT:?缺少 PITR_AGE_RECIPIENT}"
: "${PITR_CLUSTER_ID:?缺少 PITR_CLUSTER_ID}"
: "${PITR_CLUSTER_ID_FILE:=/etc/wangzhe/pitr-cluster-id}"
: "${PITR_REQUIRE_OFFSITE:?缺少 PITR_REQUIRE_OFFSITE}"
: "${PITR_WAL_ARCHIVE_DIR:?缺少 PITR_WAL_ARCHIVE_DIR}"
[[ "$PITR_CLUSTER_ID" =~ ^[0-9]{10,30}$ ]] || { echo "PITR_CLUSTER_ID 必须是 PostgreSQL system identifier" >&2; exit 1; }
[[ -f "$PITR_CLUSTER_ID_FILE" && ! -L "$PITR_CLUSTER_ID_FILE" && "$(stat -c '%u' "$PITR_CLUSTER_ID_FILE")" == 0 && -z "$(find "$PITR_CLUSTER_ID_FILE" -perm /022 -print -quit)" ]] || {
  echo "PITR 集群标识信任文件必须由 root 保护" >&2
  exit 1
}
validate_no_symlink_path_components "$PITR_CLUSTER_ID_FILE"
read -r trusted_cluster_id trusted_cluster_extra <"$PITR_CLUSTER_ID_FILE" || { echo "PITR 集群标识信任文件不可读" >&2; exit 1; }
[[ "$trusted_cluster_id" == "$PITR_CLUSTER_ID" && -z "${trusted_cluster_extra:-}" ]] || { echo "PITR_CLUSTER_ID 与 root 信任文件不一致" >&2; exit 1; }
[[ "$(basename "$PITR_WAL_ARCHIVE_DIR")" == "$PITR_CLUSTER_ID" ]] || { echo "WAL 归档目录末级必须等于 PITR_CLUSTER_ID" >&2; exit 1; }
[[ "$WAL_NAME" =~ ^([0-9A-F]{24}(\.[0-9A-F]{8}\.backup)?|[0-9A-F]{8}\.history)$ ]] || { echo "WAL 文件名无效" >&2; exit 1; }
[[ -f "$WAL_SOURCE" && ! -L "$WAL_SOURCE" && "$(basename "$WAL_SOURCE")" == "$WAL_NAME" ]] || {
  echo "WAL 源文件无效或名称不匹配" >&2
  exit 1
}
validate_age_recipient "$PITR_AGE_RECIPIENT"
[[ "$PITR_REQUIRE_OFFSITE" == "0" || "$PITR_REQUIRE_OFFSITE" == "1" ]] || { echo "PITR_REQUIRE_OFFSITE 只能是 0 或 1" >&2; exit 1; }
if [[ -n "${PITR_REMOTE_DESTINATION:-}" ]]; then
  command -v rclone >/dev/null 2>&1 || { echo "配置 WAL 异机归档时必须安装 rclone" >&2; exit 1; }
  validate_remote_destination "$PITR_REMOTE_DESTINATION"
  [[ "$(basename "${PITR_REMOTE_DESTINATION#*:}")" == "$PITR_CLUSTER_ID" ]] || { echo "WAL 异机目录末级必须等于 PITR_CLUSTER_ID" >&2; exit 1; }
  : "${PITR_RCLONE_CONFIG:?配置 WAL 异机归档时必须设置 PITR_RCLONE_CONFIG}"
  : "${PITR_PROVENANCE_SIGNING_KEY_FILE:?配置 WAL 异机归档时必须设置 PITR_PROVENANCE_SIGNING_KEY_FILE}"
  [[ "$PITR_PROVENANCE_SIGNING_KEY_FILE" == /etc/wangzhe/pitr-provenance-ed25519-private.pem ]] || { echo "PITR 备份来源签名私钥必须使用固定独立路径" >&2; exit 1; }
  validate_rclone_config "$PITR_RCLONE_CONFIG"
  validate_ed25519_private_key "$PITR_PROVENANCE_SIGNING_KEY_FILE" "PITR 备份来源 Ed25519 私钥"
elif [[ "$PITR_REQUIRE_OFFSITE" == "1" ]]; then
  echo "WAL 归档要求异机副本，但未配置 PITR_REMOTE_DESTINATION" >&2
  exit 1
fi
LOCK_WAIT_SECONDS="${PITR_LOCK_WAIT_SECONDS:-60}"
[[ "$LOCK_WAIT_SECONDS" =~ ^[0-9]+$ ]] && (( LOCK_WAIT_SECONDS >= 0 && LOCK_WAIT_SECONDS <= 600 )) || { echo "PITR_LOCK_WAIT_SECONDS 无效" >&2; exit 1; }

umask 077
validate_backup_directory "$PITR_WAL_ARCHIVE_DIR" "WAL 归档目录"
validate_backup_monitor_directory "$PITR_WAL_ARCHIVE_DIR"
exec 9>"$PITR_WAL_ARCHIVE_DIR/.archive.lock"
flock -w "$LOCK_WAIT_SECONDS" 9 || { echo "另一个 WAL 归档仍在运行" >&2; exit 1; }
target="$PITR_WAL_ARCHIVE_DIR/$WAL_NAME.age"
encrypted_partial="$target.partial"
checksum_partial="$target.sha256.partial"
source_manifest="$target.source.sha256"
source_manifest_partial="$source_manifest.partial"
offsite_partial="$target.offsite-ok.partial"
provenance_partial="$target.provenance.partial"
signature_partial="$target.provenance.sig.partial"
source_checksum="$(sha256sum "$WAL_SOURCE" | awk '{print $1}')"
cleanup_partial() {
  [[ -z "${encrypted_partial:-}" ]] || rm -f -- "$encrypted_partial"
  [[ -z "${checksum_partial:-}" ]] || rm -f -- "$checksum_partial"
  [[ -z "${source_manifest_partial:-}" ]] || rm -f -- "$source_manifest_partial"
  [[ -z "${offsite_partial:-}" ]] || rm -f -- "$offsite_partial"
  [[ -z "${provenance_partial:-}" ]] || rm -f -- "$provenance_partial"
  [[ -z "${signature_partial:-}" ]] || rm -f -- "$signature_partial"
  return 0
}
trap cleanup_partial EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if [[ -e "$target" || -L "$target" ]]; then
  validate_encrypted_backup_and_manifest "$target" || { echo "已有 WAL 归档损坏：$target" >&2; exit 1; }
  [[ -f "$source_manifest" && ! -L "$source_manifest" ]] || { echo "已有 WAL 缺少源文件凭证：$source_manifest" >&2; exit 1; }
  read -r recorded_source_checksum recorded_wal_name recorded_cluster_id recorded_extra <"$source_manifest" || { echo "已有 WAL 源文件凭证不可读" >&2; exit 1; }
  [[ "$recorded_source_checksum" == "$source_checksum" && "$recorded_wal_name" == "$WAL_NAME" && "$recorded_cluster_id" == "$PITR_CLUSTER_ID" && -z "${recorded_extra:-}" ]] || {
    echo "已有 WAL 不属于当前源文件或 PostgreSQL 集群，拒绝复用" >&2
    exit 1
  }
  if [[ -n "${PITR_REMOTE_DESTINATION:-}" ]]; then
    verify_backup_provenance "$target" pitr-wal "$PITR_CLUSTER_ID" \
      "${PITR_REMOTE_DESTINATION%/}/$(basename "$target")" "$PITR_PROVENANCE_SIGNING_KEY_FILE" private || { echo "已有 WAL 来源签名凭证无效" >&2; exit 1; }
    make_backup_artifacts_monitor_readable "$target" "$target.sha256" "$source_manifest" "$target.provenance" "$target.provenance.sig"
  else
    make_backup_artifacts_monitor_readable "$target" "$target.sha256" "$source_manifest"
  fi
else
  for candidate in "$target.sha256" "$target.offsite-ok" "$target.provenance" "$target.provenance.sig" "$source_manifest" "$encrypted_partial" "$checksum_partial" "$source_manifest_partial" "$offsite_partial" "$provenance_partial" "$signature_partial"; do
    [[ ! -e "$candidate" && ! -L "$candidate" ]] || { echo "WAL 归档残留同名文件，拒绝覆盖：$candidate" >&2; exit 1; }
  done
  encrypt_backup_file "$WAL_SOURCE" "$encrypted_partial" "$PITR_AGE_RECIPIENT"
  mv "$encrypted_partial" "$target"
  encrypted_partial=""
  write_backup_checksum "$target" "$checksum_partial"
  mv "$checksum_partial" "$target.sha256"
  checksum_partial=""
  printf '%s  %s  %s\n' "$source_checksum" "$WAL_NAME" "$PITR_CLUSTER_ID" >"$source_manifest_partial"
  mv "$source_manifest_partial" "$source_manifest"
  source_manifest_partial=""
  if [[ -n "${PITR_REMOTE_DESTINATION:-}" ]]; then
    provenance_created_epoch="$(date +%s)"
    write_backup_provenance "$target" "$PITR_REMOTE_DESTINATION" pitr-wal "$PITR_CLUSTER_ID" "$provenance_created_epoch" \
      "$PITR_PROVENANCE_SIGNING_KEY_FILE" "$provenance_partial" "$signature_partial"
    mv "$provenance_partial" "$target.provenance"
    provenance_partial=""
    mv "$signature_partial" "$target.provenance.sig"
    signature_partial=""
    verify_backup_provenance "$target" pitr-wal "$PITR_CLUSTER_ID" \
      "${PITR_REMOTE_DESTINATION%/}/$(basename "$target")" "$PITR_PROVENANCE_SIGNING_KEY_FILE" private || { echo "WAL 来源签名凭证自检失败" >&2; exit 1; }
    make_backup_artifacts_monitor_readable "$target" "$target.sha256" "$source_manifest" "$target.provenance" "$target.provenance.sig"
  else
    make_backup_artifacts_monitor_readable "$target" "$target.sha256" "$source_manifest"
  fi
fi

if [[ -n "${PITR_REMOTE_DESTINATION:-}" ]]; then
  if validate_offsite_marker "$target" "$PITR_REMOTE_DESTINATION"; then
    :
  elif [[ -e "$target.offsite-ok" || -L "$target.offsite-ok" ]]; then
    echo "已有 WAL 异机回读凭证无效，拒绝确认归档：$target.offsite-ok" >&2
    exit 1
  else
    sync_backup_offsite "$target" "$target.sha256" "$PITR_REMOTE_DESTINATION" "$offsite_partial" "$PITR_RCLONE_CONFIG" \
      "$target.provenance" "$target.provenance.sig"
    remote_source_manifest="${PITR_REMOTE_DESTINATION%/}/$(basename "$source_manifest")"
    rclone --config "$PITR_RCLONE_CONFIG" --contimeout 10s --timeout 2m --retries 2 --low-level-retries 2 copyto "$source_manifest" "$remote_source_manifest" --checksum --no-traverse
    local_source_manifest_checksum="$(sha256sum "$source_manifest" | awk '{print $1}')"
    remote_source_manifest_checksum="$(rclone --config "$PITR_RCLONE_CONFIG" --contimeout 10s --timeout 2m --retries 2 --low-level-retries 2 hashsum sha256 "$remote_source_manifest" --download | awk 'NR == 1 {print $1}')"
    [[ "$remote_source_manifest_checksum" == "$local_source_manifest_checksum" ]] || { echo "WAL 源文件凭证异机回读失败" >&2; exit 1; }
    mv "$offsite_partial" "$target.offsite-ok"
    offsite_partial=""
    validate_offsite_marker "$target" "$PITR_REMOTE_DESTINATION" || { echo "WAL 异机回读凭证无效" >&2; exit 1; }
    make_backup_artifacts_monitor_readable "$target.offsite-ok"
  fi
fi
if [[ -n "${PITR_REMOTE_DESTINATION:-}" ]]; then
  remote_source_manifest="${PITR_REMOTE_DESTINATION%/}/$(basename "$source_manifest")"
  local_source_manifest_checksum="$(sha256sum "$source_manifest" | awk '{print $1}')"
  remote_source_manifest_checksum="$(rclone --config "$PITR_RCLONE_CONFIG" --contimeout 10s --timeout 2m --retries 2 --low-level-retries 2 hashsum sha256 "$remote_source_manifest" --download 2>/dev/null | awk 'NR == 1 {print $1}' || true)"
  if [[ "$remote_source_manifest_checksum" != "$local_source_manifest_checksum" ]]; then
    rclone --config "$PITR_RCLONE_CONFIG" --contimeout 10s --timeout 2m --retries 2 --low-level-retries 2 copyto "$source_manifest" "$remote_source_manifest" --checksum --no-traverse
    remote_source_manifest_checksum="$(rclone --config "$PITR_RCLONE_CONFIG" --contimeout 10s --timeout 2m --retries 2 --low-level-retries 2 hashsum sha256 "$remote_source_manifest" --download | awk 'NR == 1 {print $1}')"
    [[ "$remote_source_manifest_checksum" == "$local_source_manifest_checksum" ]] || { echo "WAL 源文件凭证异机回读失败" >&2; exit 1; }
  fi
fi
if [[ "$PITR_REQUIRE_OFFSITE" == "1" ]] && ! validate_offsite_marker "$target" "$PITR_REMOTE_DESTINATION"; then
  echo "WAL 异机归档尚未验证" >&2
  exit 1
fi
echo "WAL 加密归档完成：$WAL_NAME"
