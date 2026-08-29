#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

readonly RECOVERY_EVIDENCE_ENV_FILE=/etc/wangzhe/recovery-evidence.env
readonly RECOVERY_EVIDENCE_RCLONE_CONFIG_FILE=/etc/wangzhe/recovery-evidence-read-rclone.conf
readonly LOGICAL_STATUS_VERIFY_KEY_FILE=/etc/wangzhe/logical-restore-status-ed25519-public.pem
readonly PITR_STATUS_VERIFY_KEY_FILE=/etc/wangzhe/pitr-restore-status-ed25519-public.pem
readonly LOGICAL_RESTORE_LUKS_MOUNT=/var/lib/wangzhe-restore
readonly LOGICAL_DATABASE_LUKS_MOUNT=/var/lib/wangzhe-recovery-postgresql
readonly PITR_DRILL_LUKS_MOUNT=/var/lib/wangzhe-pitr-drill

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=lib/strict-env.sh
source "$SCRIPT_DIR/lib/strict-env.sh"
# shellcheck source=lib/encrypted-backup.sh
source "$SCRIPT_DIR/lib/encrypted-backup.sh"

recovery_evidence_fail() { echo "$*" >&2; return 1; }

recovery_evidence_field() {
  local file="$1" key="$2"
  awk -F= -v key="$key" '$1 == key { count++; value=substr($0, length(key) + 2) } END { if (count == 1) print value; else exit 1 }' "$file"
}

verify_status_bundle() {
  local status_file="$1" checksum_file="$2" signature_file="$3" expected_name="$4" verify_key="$5" label="$6"
  local status_bytes checksum_bytes signature_bytes expected_sha manifest_name manifest_extra actual_sha
  [[ -f "$status_file" && ! -L "$status_file" && -f "$checksum_file" && ! -L "$checksum_file" && -f "$signature_file" && ! -L "$signature_file" ]] || recovery_evidence_fail "$label 三件套不完整或不安全" || return 1
  status_bytes="$(strict_env_stat '%s' '%z' "$status_file")"
  checksum_bytes="$(strict_env_stat '%s' '%z' "$checksum_file")"
  signature_bytes="$(strict_env_stat '%s' '%z' "$signature_file")"
  [[ "$status_bytes" =~ ^[0-9]+$ && "$checksum_bytes" =~ ^[0-9]+$ && "$signature_bytes" == 64 ]] || recovery_evidence_fail "$label 文件大小无效" || return 1
  (( status_bytes >= 1 && status_bytes <= 16384 && checksum_bytes >= 1 && checksum_bytes <= 4096 )) || recovery_evidence_fail "$label 状态或摘要大小越界" || return 1
  read -r expected_sha manifest_name manifest_extra <"$checksum_file" || recovery_evidence_fail "$label 摘要不可读" || return 1
  actual_sha="$(sha256sum "$status_file" | awk '{print $1}')"
  [[ "$expected_sha" =~ ^[0-9a-f]{64}$ && "$expected_sha" == "$actual_sha" && "$manifest_name" == "$expected_name" && -z "${manifest_extra:-}" ]] || recovery_evidence_fail "$label 摘要或对象名绑定无效" || return 1
  openssl pkeyutl -verify -pubin -rawin -inkey "$verify_key" -in "$status_file" -sigfile "$signature_file" >/dev/null 2>&1 || recovery_evidence_fail "$label Ed25519 签名无效" || return 1
}

validate_logical_recovery_evidence() {
  local file="$1" now_epoch="$2" max_age="$3"
  local schema outcome scope isolation host database_restore upload_restore pitr_restore epoch utc
  local database_source database_name upload_name database_remote upload_remote database_sha upload_sha
  local database_provenance upload_provenance work_mount database_mount upload_mount migrations negative orphan
  local manifest_entries restored_files database_bytes upload_bytes
  schema="$(recovery_evidence_field "$file" status_schema)" || return 1
  outcome="$(recovery_evidence_field "$file" outcome)" || return 1
  scope="$(recovery_evidence_field "$file" scope)" || return 1
  isolation="$(recovery_evidence_field "$file" isolation)" || return 1
  host="$(recovery_evidence_field "$file" database_host)" || return 1
  database_restore="$(recovery_evidence_field "$file" database_restore)" || return 1
  upload_restore="$(recovery_evidence_field "$file" upload_restore)" || return 1
  pitr_restore="$(recovery_evidence_field "$file" pitr_restore)" || return 1
  epoch="$(recovery_evidence_field "$file" completed_at_epoch)" || return 1
  utc="$(recovery_evidence_field "$file" completed_at_utc)" || return 1
  database_source="$(recovery_evidence_field "$file" database_source_name)" || return 1
  database_name="$(recovery_evidence_field "$file" database_backup_name)" || return 1
  upload_name="$(recovery_evidence_field "$file" upload_backup_name)" || return 1
  database_remote="$(recovery_evidence_field "$file" database_offsite_source)" || return 1
  upload_remote="$(recovery_evidence_field "$file" upload_offsite_source)" || return 1
  database_sha="$(recovery_evidence_field "$file" database_sha256)" || return 1
  upload_sha="$(recovery_evidence_field "$file" upload_sha256)" || return 1
  database_provenance="$(recovery_evidence_field "$file" database_provenance_sha256)" || return 1
  upload_provenance="$(recovery_evidence_field "$file" upload_provenance_sha256)" || return 1
  work_mount="$(recovery_evidence_field "$file" restore_work_luks_mount)" || return 1
  database_mount="$(recovery_evidence_field "$file" database_data_luks_mount)" || return 1
  upload_mount="$(recovery_evidence_field "$file" upload_target_luks_mount)" || return 1
  migrations="$(recovery_evidence_field "$file" schema_migrations)" || return 1
  negative="$(recovery_evidence_field "$file" negative_balances)" || return 1
  orphan="$(recovery_evidence_field "$file" orphan_bets)" || return 1
  manifest_entries="$(recovery_evidence_field "$file" upload_manifest_entries)" || return 1
  restored_files="$(recovery_evidence_field "$file" upload_restored_files)" || return 1
  database_bytes="$(recovery_evidence_field "$file" database_artifact_bytes)" || return 1
  upload_bytes="$(recovery_evidence_field "$file" upload_artifact_bytes)" || return 1

  [[ "$schema" == wangzhe.restore-drill.v2 && "$outcome" == success && "$scope" == logical_database_and_uploads ]] || return 1
  [[ "$isolation" == offsite_download_loopback_database_and_fixed_targets && "$host" == loopback ]] || return 1
  [[ "$database_restore" == verified && "$upload_restore" == verified && "$pitr_restore" == not_in_scope ]] || return 1
  [[ "$epoch" =~ ^[1-9][0-9]{0,11}$ && "$utc" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]] || return 1
  (( epoch <= now_epoch + 300 && now_epoch - epoch <= max_age )) || return 1
  [[ "$database_source" == "$RECOVERY_EVIDENCE_EXPECTED_DATABASE_NAME" ]] || return 1
  [[ "$database_name" =~ ^${database_source}-[0-9]{8}-[0-9]{6}-[0-9]+\.dump\.age$ ]] || return 1
  [[ "$upload_name" =~ ^uploads-[0-9]{8}-[0-9]{6}-[0-9]+\.tar\.age$ ]] || return 1
  [[ "$database_remote" == "${RECOVERY_EVIDENCE_EXPECTED_DATABASE_REMOTE_SOURCE%/}/$database_name" ]] || return 1
  [[ "$upload_remote" == "${RECOVERY_EVIDENCE_EXPECTED_UPLOAD_REMOTE_SOURCE%/}/$upload_name" ]] || return 1
  validate_remote_destination "$database_remote" >/dev/null 2>&1 || return 1
  validate_remote_destination "$upload_remote" >/dev/null 2>&1 || return 1
  [[ "$database_sha" =~ ^[0-9a-f]{64}$ && "$upload_sha" =~ ^[0-9a-f]{64}$ && "$database_provenance" =~ ^[0-9a-f]{64}$ && "$upload_provenance" =~ ^[0-9a-f]{64}$ ]] || return 1
  [[ "$work_mount" == "$LOGICAL_RESTORE_LUKS_MOUNT" && "$upload_mount" == "$LOGICAL_RESTORE_LUKS_MOUNT" && "$database_mount" == "$LOGICAL_DATABASE_LUKS_MOUNT" ]] || return 1
  [[ "$migrations" =~ ^[1-9][0-9]*$ && "$negative" == 0 && "$orphan" == 0 ]] || return 1
  [[ "$manifest_entries" =~ ^[0-9]+$ && "$restored_files" == "$manifest_entries" ]] || return 1
  [[ "$database_bytes" =~ ^[1-9][0-9]*$ && "$upload_bytes" =~ ^[1-9][0-9]*$ ]] || return 1
}

validate_pitr_recovery_evidence() {
  local file="$1" now_epoch="$2" max_age="$3" max_source_age="$4"
  local version completed reached epoch target_epoch target_utc mount generation source_remote snapshot source_epoch
  local base_count source_wal_count source_segment_count base_name base_sha major system_id timeline replay_lsn replay_timestamp
  local wal_count segment_count first_wal last_wal wal_sha migrations negative orphan
  version="$(recovery_evidence_field "$file" format_version)" || return 1
  completed="$(recovery_evidence_field "$file" pitr_completed)" || return 1
  reached="$(recovery_evidence_field "$file" target_reached)" || return 1
  epoch="$(recovery_evidence_field "$file" completed_at_epoch)" || return 1
  target_epoch="$(recovery_evidence_field "$file" target_at_epoch)" || return 1
  target_utc="$(recovery_evidence_field "$file" target_at_utc)" || return 1
  mount="$(recovery_evidence_field "$file" drill_luks_mount)" || return 1
  generation="$(recovery_evidence_field "$file" source_generation)" || return 1
  source_remote="$(recovery_evidence_field "$file" source_remote_destination)" || return 1
  snapshot="$(recovery_evidence_field "$file" source_snapshot_sha256)" || return 1
  source_epoch="$(recovery_evidence_field "$file" source_synced_at_epoch)" || return 1
  base_count="$(recovery_evidence_field "$file" source_basebackup_count)" || return 1
  source_wal_count="$(recovery_evidence_field "$file" source_wal_count)" || return 1
  source_segment_count="$(recovery_evidence_field "$file" source_wal_segment_count)" || return 1
  base_name="$(recovery_evidence_field "$file" basebackup_file)" || return 1
  base_sha="$(recovery_evidence_field "$file" basebackup_sha256)" || return 1
  major="$(recovery_evidence_field "$file" postgres_major)" || return 1
  system_id="$(recovery_evidence_field "$file" system_identifier)" || return 1
  timeline="$(recovery_evidence_field "$file" timeline_id)" || return 1
  replay_lsn="$(recovery_evidence_field "$file" replay_lsn)" || return 1
  replay_timestamp="$(recovery_evidence_field "$file" replay_timestamp)" || return 1
  wal_count="$(recovery_evidence_field "$file" restored_wal_count)" || return 1
  segment_count="$(recovery_evidence_field "$file" restored_wal_segment_count)" || return 1
  first_wal="$(recovery_evidence_field "$file" first_restored_wal)" || return 1
  last_wal="$(recovery_evidence_field "$file" last_restored_wal)" || return 1
  wal_sha="$(recovery_evidence_field "$file" wal_audit_sha256)" || return 1
  migrations="$(recovery_evidence_field "$file" schema_migrations)" || return 1
  negative="$(recovery_evidence_field "$file" negative_balances)" || return 1
  orphan="$(recovery_evidence_field "$file" orphan_bets)" || return 1

  [[ "$version" == 2 && "$completed" == 1 && "$reached" == 1 && "$mount" == "$PITR_DRILL_LUKS_MOUNT" ]] || return 1
  [[ "$epoch" =~ ^[1-9][0-9]{0,11}$ && "$target_epoch" =~ ^[1-9][0-9]{0,11}$ ]] || return 1
  (( target_epoch < epoch && epoch <= now_epoch + 300 && now_epoch - epoch <= max_age )) || return 1
  (( epoch - target_epoch >= 300 && epoch - target_epoch <= 86400 )) || return 1
  [[ "$target_utc" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}[[:space:]][0-9]{2}:[0-9]{2}:[0-9]{2}\+00$ ]] || return 1
  [[ "$generation" =~ ^[0-9]{8}-[0-9]{6}-[0-9]+$ && "$source_remote" == "$RECOVERY_EVIDENCE_EXPECTED_PITR_REMOTE_SOURCE" ]] || return 1
  validate_remote_destination "$source_remote" >/dev/null 2>&1 || return 1
  [[ "$(basename "${source_remote#*:}")" == "$RECOVERY_EVIDENCE_PITR_CLUSTER_ID" ]] || return 1
  [[ "$snapshot" =~ ^[0-9a-f]{64}$ && "$source_epoch" =~ ^[1-9][0-9]{0,11}$ ]] || return 1
  (( source_epoch <= epoch && epoch - source_epoch <= max_source_age )) || return 1
  [[ "$base_count" =~ ^[1-9][0-9]*$ && "$source_wal_count" =~ ^[1-9][0-9]*$ && "$source_segment_count" =~ ^[1-9][0-9]*$ ]] || return 1
  (( 10#$source_segment_count <= 10#$source_wal_count )) || return 1
  [[ "$base_name" =~ ^basebackup-[0-9]{8}-[0-9]{6}-[0-9]+\.tar\.age$ && "$base_sha" =~ ^[0-9a-f]{64}$ ]] || return 1
  [[ "$major" =~ ^[0-9]+$ && "$system_id" == "$RECOVERY_EVIDENCE_PITR_CLUSTER_ID" ]] || return 1
  [[ "$timeline" =~ ^[1-9][0-9]*$ && "$replay_lsn" =~ ^[0-9A-F]+/[0-9A-F]+$ && -n "$replay_timestamp" ]] || return 1
  [[ "$wal_count" =~ ^[1-9][0-9]*$ && "$segment_count" =~ ^[1-9][0-9]*$ ]] || return 1
  (( 10#$segment_count <= 10#$wal_count )) || return 1
  [[ "$first_wal" =~ ^([0-9A-F]{24}(\.[0-9A-F]{8}\.backup)?|[0-9A-F]{8}\.history)$ ]] || return 1
  [[ "$last_wal" =~ ^([0-9A-F]{24}(\.[0-9A-F]{8}\.backup)?|[0-9A-F]{8}\.history)$ && "$wal_sha" =~ ^[0-9a-f]{64}$ ]] || return 1
  [[ "$migrations" =~ ^[1-9][0-9]*$ && "$negative" == 0 && "$orphan" == 0 ]] || return 1
}

require_distinct_status_key_domains() {
  local logical_key="$1" pitr_key="$2" logical_fingerprint pitr_fingerprint
  logical_fingerprint="$(openssl pkey -pubin -in "$logical_key" -pubout -outform DER 2>/dev/null | sha256sum | awk '{print $1}')" || return 1
  pitr_fingerprint="$(openssl pkey -pubin -in "$pitr_key" -pubout -outform DER 2>/dev/null | sha256sum | awk '{print $1}')" || return 1
  [[ "$logical_fingerprint" =~ ^[0-9a-f]{64}$ && "$pitr_fingerprint" =~ ^[0-9a-f]{64}$ && "$logical_fingerprint" != "$pitr_fingerprint" ]] || recovery_evidence_fail "逻辑恢复和 PITR 演练必须使用不同的 Ed25519 签名域"
}

download_status_bundle() {
  local remote="$1" destination="$2"
  timeout 45 rclone --config "$RECOVERY_EVIDENCE_RCLONE_CONFIG_FILE" --contimeout 5s --timeout 30s --retries 1 --low-level-retries 1 copyto "$remote" "$destination" --no-traverse
  timeout 45 rclone --config "$RECOVERY_EVIDENCE_RCLONE_CONFIG_FILE" --contimeout 5s --timeout 30s --retries 1 --low-level-retries 1 copyto "$remote.sha256" "$destination.sha256" --no-traverse
  timeout 45 rclone --config "$RECOVERY_EVIDENCE_RCLONE_CONFIG_FILE" --contimeout 5s --timeout 30s --retries 1 --low-level-retries 1 copyto "$remote.sig" "$destination.sig" --no-traverse
}

production_recovery_evidence_main() {
  [[ $# == 0 ]] || { echo "此门禁不接受命令行覆盖" >&2; exit 1; }
  for command_name in awk basename date dirname grep mktemp openssl rclone rm sha256sum stat timeout; do
    command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
  done
  grep -q -- '-rawin' < <(openssl pkeyutl -help 2>&1 || true) || { echo "恢复证据验签要求 OpenSSL 3.0+" >&2; exit 1; }
  load_strict_env "$RECOVERY_EVIDENCE_ENV_FILE" '^RECOVERY_EVIDENCE_[A-Z0-9_]+$'
  local required_key
  for required_key in RECOVERY_EVIDENCE_LOGICAL_STATUS_REMOTE_SOURCE RECOVERY_EVIDENCE_PITR_STATUS_REMOTE_SOURCE \
    RECOVERY_EVIDENCE_EXPECTED_DATABASE_NAME RECOVERY_EVIDENCE_EXPECTED_DATABASE_REMOTE_SOURCE \
    RECOVERY_EVIDENCE_EXPECTED_UPLOAD_REMOTE_SOURCE RECOVERY_EVIDENCE_EXPECTED_PITR_REMOTE_SOURCE \
    RECOVERY_EVIDENCE_PITR_CLUSTER_ID RECOVERY_EVIDENCE_LOGICAL_MAX_AGE_SECONDS \
    RECOVERY_EVIDENCE_PITR_MAX_AGE_SECONDS RECOVERY_EVIDENCE_PITR_MAX_SOURCE_AGE_SECONDS; do
    [[ -n "${!required_key:-}" ]] || { echo "恢复证据配置缺少 $required_key" >&2; exit 1; }
  done
  [[ "$RECOVERY_EVIDENCE_EXPECTED_DATABASE_NAME" =~ ^[A-Za-z][A-Za-z0-9_]{0,62}$ ]] || { echo "生产数据库来源名无效" >&2; exit 1; }
  [[ "$RECOVERY_EVIDENCE_PITR_CLUSTER_ID" =~ ^[0-9]{10,30}$ ]] || { echo "PITR 集群标识无效" >&2; exit 1; }
  validate_remote_destination "$RECOVERY_EVIDENCE_LOGICAL_STATUS_REMOTE_SOURCE"
  validate_remote_destination "$RECOVERY_EVIDENCE_PITR_STATUS_REMOTE_SOURCE"
  validate_remote_destination "$RECOVERY_EVIDENCE_EXPECTED_DATABASE_REMOTE_SOURCE"
  validate_remote_destination "$RECOVERY_EVIDENCE_EXPECTED_UPLOAD_REMOTE_SOURCE"
  validate_remote_destination "$RECOVERY_EVIDENCE_EXPECTED_PITR_REMOTE_SOURCE"
  [[ "${RECOVERY_EVIDENCE_LOGICAL_STATUS_REMOTE_SOURCE##*/}" == last-success.status ]] || { echo "逻辑恢复状态对象名必须是 last-success.status" >&2; exit 1; }
  [[ "${RECOVERY_EVIDENCE_PITR_STATUS_REMOTE_SOURCE##*/}" == last-pitr-success.status ]] || { echo "PITR 状态对象名必须是 last-pitr-success.status" >&2; exit 1; }
  [[ "${RECOVERY_EVIDENCE_LOGICAL_STATUS_REMOTE_SOURCE%/*}" == "${RECOVERY_EVIDENCE_PITR_STATUS_REMOTE_SOURCE%/*}" ]] || { echo "两类恢复状态必须绑定到同一生产状态前缀" >&2; exit 1; }
  [[ "$(basename "${RECOVERY_EVIDENCE_EXPECTED_PITR_REMOTE_SOURCE#*:}")" == "$RECOVERY_EVIDENCE_PITR_CLUSTER_ID" ]] || { echo "PITR 生产来源末级与集群标识不一致" >&2; exit 1; }
  [[ "$RECOVERY_EVIDENCE_LOGICAL_MAX_AGE_SECONDS" =~ ^[0-9]+$ && "$RECOVERY_EVIDENCE_PITR_MAX_AGE_SECONDS" =~ ^[0-9]+$ && "$RECOVERY_EVIDENCE_PITR_MAX_SOURCE_AGE_SECONDS" =~ ^[0-9]+$ ]] || { echo "恢复证据时间阈值无效" >&2; exit 1; }
  (( RECOVERY_EVIDENCE_LOGICAL_MAX_AGE_SECONDS >= 3600 && RECOVERY_EVIDENCE_LOGICAL_MAX_AGE_SECONDS <= 3024000 )) || { echo "逻辑恢复证据最长只能放宽到 35 天" >&2; exit 1; }
  (( RECOVERY_EVIDENCE_PITR_MAX_AGE_SECONDS >= 3600 && RECOVERY_EVIDENCE_PITR_MAX_AGE_SECONDS <= 3024000 )) || { echo "PITR 恢复证据最长只能放宽到 35 天" >&2; exit 1; }
  (( RECOVERY_EVIDENCE_PITR_MAX_SOURCE_AGE_SECONDS >= 3600 && RECOVERY_EVIDENCE_PITR_MAX_SOURCE_AGE_SECONDS <= 172800 )) || { echo "PITR 异机源最多允许 48 小时陈旧" >&2; exit 1; }
  validate_rclone_config "$RECOVERY_EVIDENCE_RCLONE_CONFIG_FILE"
  validate_ed25519_public_key "$LOGICAL_STATUS_VERIFY_KEY_FILE" "逻辑恢复状态 Ed25519 公钥"
  validate_ed25519_public_key "$PITR_STATUS_VERIFY_KEY_FILE" "PITR 恢复状态 Ed25519 公钥"
  require_distinct_status_key_domains "$LOGICAL_STATUS_VERIFY_KEY_FILE" "$PITR_STATUS_VERIFY_KEY_FILE"

  umask 077
  local work_dir logical_file pitr_file now_epoch
  work_dir="$(mktemp -d "${TMPDIR:-/tmp}/wangzhe-recovery-evidence.XXXXXX")"
  cleanup_recovery_evidence() { rm -rf -- "$work_dir"; }
  trap cleanup_recovery_evidence EXIT INT TERM
  logical_file="$work_dir/last-success.status"
  pitr_file="$work_dir/last-pitr-success.status"
  download_status_bundle "$RECOVERY_EVIDENCE_LOGICAL_STATUS_REMOTE_SOURCE" "$logical_file"
  download_status_bundle "$RECOVERY_EVIDENCE_PITR_STATUS_REMOTE_SOURCE" "$pitr_file"
  verify_status_bundle "$logical_file" "$logical_file.sha256" "$logical_file.sig" last-success.status "$LOGICAL_STATUS_VERIFY_KEY_FILE" "逻辑恢复证据"
  verify_status_bundle "$pitr_file" "$pitr_file.sha256" "$pitr_file.sig" last-pitr-success.status "$PITR_STATUS_VERIFY_KEY_FILE" "PITR 恢复证据"
  now_epoch="$(date +%s)"
  validate_logical_recovery_evidence "$logical_file" "$now_epoch" "$RECOVERY_EVIDENCE_LOGICAL_MAX_AGE_SECONDS" || { echo "逻辑数据库/上传恢复证据字段、生产来源绑定或新鲜度无效" >&2; exit 1; }
  validate_pitr_recovery_evidence "$pitr_file" "$now_epoch" "$RECOVERY_EVIDENCE_PITR_MAX_AGE_SECONDS" "$RECOVERY_EVIDENCE_PITR_MAX_SOURCE_AGE_SECONDS" || { echo "PITR 恢复证据字段、生产来源绑定或新鲜度无效" >&2; exit 1; }
  echo "生产恢复证据门禁通过：逻辑数据库/上传恢复与 PITR 均有近期、独立签名的生产演练证据"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  production_recovery_evidence_main "$@"
fi
