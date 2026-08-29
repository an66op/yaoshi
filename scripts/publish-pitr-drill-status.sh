#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

readonly EXPECTED_ENV_FILE=/etc/wangzhe/pitr-status.env
readonly EXPECTED_STATUS_FILE=/var/lib/wangzhe-pitr-drill/work/last-success.status
readonly EXPECTED_DRILL_LUKS_MOUNT=/var/lib/wangzhe-pitr-drill

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=lib/strict-env.sh
source "$SCRIPT_DIR/lib/strict-env.sh"
# shellcheck source=lib/encrypted-backup.sh
source "$SCRIPT_DIR/lib/encrypted-backup.sh"

fail() { echo "$*" >&2; exit 1; }
status_field() {
  local file="$1" key="$2"
  awk -F= -v key="$key" '$1 == key { count++; value=substr($0, length(key) + 2) } END { if (count == 1) print value; else exit 1 }' "$file"
}

env_source="${1:-$EXPECTED_ENV_FILE}"
[[ "$env_source" == "$EXPECTED_ENV_FILE" ]] || fail "PITR 状态发布只接受 $EXPECTED_ENV_FILE"
validate_no_symlink_path_components "$env_source"
load_strict_env "$env_source" '^PITR_STATUS_[A-Z0-9_]+$'
for command_name in awk basename date mktemp mv rclone rm sha256sum stat timeout; do
  command -v "$command_name" >/dev/null 2>&1 || fail "缺少命令：$command_name"
done
for key in PITR_STATUS_SOURCE_FILE PITR_STATUS_SIGNATURE_FILE PITR_STATUS_CLUSTER_ID PITR_STATUS_REMOTE_DESTINATION PITR_STATUS_RCLONE_CONFIG PITR_STATUS_MAX_AGE_SECONDS PITR_STATUS_EXPECTED_BACKUP_REMOTE_SOURCE; do
  [[ -n "${!key:-}" ]] || fail "缺少 $key"
done
[[ "$PITR_STATUS_SOURCE_FILE" == "$EXPECTED_STATUS_FILE" ]] || fail "PITR 状态源文件必须固定为 $EXPECTED_STATUS_FILE"
[[ "$PITR_STATUS_SIGNATURE_FILE" == "$EXPECTED_STATUS_FILE.sig" ]] || fail "PITR 状态签名必须固定为 $EXPECTED_STATUS_FILE.sig"
[[ "$PITR_STATUS_CLUSTER_ID" =~ ^[0-9]{10,30}$ ]] || fail "PITR_STATUS_CLUSTER_ID 必须是 PostgreSQL system identifier"
[[ "$PITR_STATUS_MAX_AGE_SECONDS" =~ ^[1-9][0-9]*$ ]] && (( PITR_STATUS_MAX_AGE_SECONDS >= 60 && PITR_STATUS_MAX_AGE_SECONDS <= 86400 )) || fail "PITR_STATUS_MAX_AGE_SECONDS 必须是 60-86400"
[[ -f "$PITR_STATUS_SOURCE_FILE" && ! -L "$PITR_STATUS_SOURCE_FILE" ]] || fail "PITR 成功状态不存在或不安全"
[[ -f "$PITR_STATUS_SOURCE_FILE.sha256" && ! -L "$PITR_STATUS_SOURCE_FILE.sha256" ]] || fail "PITR 成功状态摘要不存在或不安全"
[[ -f "$PITR_STATUS_SIGNATURE_FILE" && ! -L "$PITR_STATUS_SIGNATURE_FILE" ]] || fail "PITR 成功状态签名不存在或不安全"
validate_no_symlink_path_components "$PITR_STATUS_SOURCE_FILE"
validate_no_symlink_path_components "$PITR_STATUS_SIGNATURE_FILE"
[[ "$(strict_env_stat '%s' '%z' "$PITR_STATUS_SIGNATURE_FILE")" == 64 ]] || fail "PITR Ed25519 状态签名长度无效"
validate_remote_destination "$PITR_STATUS_REMOTE_DESTINATION"
[[ "${PITR_STATUS_REMOTE_DESTINATION##*/}" == last-pitr-success.status ]] || fail "PITR 远端状态必须精确指向 last-pitr-success.status"
validate_remote_destination "$PITR_STATUS_EXPECTED_BACKUP_REMOTE_SOURCE"
[[ "$(basename "${PITR_STATUS_EXPECTED_BACKUP_REMOTE_SOURCE#*:}")" == "$PITR_STATUS_CLUSTER_ID" ]] || fail "PITR 预期备份远端末级必须等于集群标识"
[[ "$PITR_STATUS_RCLONE_CONFIG" == /etc/wangzhe/pitr-status-write-rclone.conf ]] || fail "PITR 状态发布必须使用固定的 write-only rclone 配置"
validate_rclone_config "$PITR_STATUS_RCLONE_CONFIG"

read -r local_digest local_name local_extra <"$PITR_STATUS_SOURCE_FILE.sha256" || fail "PITR 本地状态摘要不可读"
actual_digest="$(sha256sum "$PITR_STATUS_SOURCE_FILE" | awk '{print $1}')"
[[ "$local_digest" == "$actual_digest" && "$local_name" == last-success.status && -z "${local_extra:-}" ]] || fail "PITR 本地状态摘要不匹配"

format_version="$(status_field "$PITR_STATUS_SOURCE_FILE" format_version)" || fail "PITR 状态缺少 format_version"
pitr_completed="$(status_field "$PITR_STATUS_SOURCE_FILE" pitr_completed)" || fail "PITR 状态缺少完成标记"
target_reached="$(status_field "$PITR_STATUS_SOURCE_FILE" target_reached)" || fail "PITR 状态缺少目标到达标记"
completed_epoch="$(status_field "$PITR_STATUS_SOURCE_FILE" completed_at_epoch)" || fail "PITR 状态缺少完成时间"
target_epoch="$(status_field "$PITR_STATUS_SOURCE_FILE" target_at_epoch)" || fail "PITR 状态缺少目标时间"
drill_luks_mount="$(status_field "$PITR_STATUS_SOURCE_FILE" drill_luks_mount)" || fail "PITR 状态缺少 LUKS 演练盘证据"
source_generation="$(status_field "$PITR_STATUS_SOURCE_FILE" source_generation)" || fail "PITR 状态缺少来源 generation"
source_remote="$(status_field "$PITR_STATUS_SOURCE_FILE" source_remote_destination)" || fail "PITR 状态缺少精确远端来源"
source_snapshot_sha="$(status_field "$PITR_STATUS_SOURCE_FILE" source_snapshot_sha256)" || fail "PITR 状态缺少来源快照摘要"
source_synced_epoch="$(status_field "$PITR_STATUS_SOURCE_FILE" source_synced_at_epoch)" || fail "PITR 状态缺少来源同步时间"
source_base_count="$(status_field "$PITR_STATUS_SOURCE_FILE" source_basebackup_count)" || fail "PITR 状态缺少来源基础备份数"
source_wal_count="$(status_field "$PITR_STATUS_SOURCE_FILE" source_wal_count)" || fail "PITR 状态缺少来源 WAL 数"
source_segment_count="$(status_field "$PITR_STATUS_SOURCE_FILE" source_wal_segment_count)" || fail "PITR 状态缺少来源 WAL 段数"
system_identifier="$(status_field "$PITR_STATUS_SOURCE_FILE" system_identifier)" || fail "PITR 状态缺少集群标识"
base_sha="$(status_field "$PITR_STATUS_SOURCE_FILE" basebackup_sha256)" || fail "PITR 状态缺少基础备份摘要"
replay_lsn="$(status_field "$PITR_STATUS_SOURCE_FILE" replay_lsn)" || fail "PITR 状态缺少重放 LSN"
timeline_id="$(status_field "$PITR_STATUS_SOURCE_FILE" timeline_id)" || fail "PITR 状态缺少时间线"
restored_wal_count="$(status_field "$PITR_STATUS_SOURCE_FILE" restored_wal_count)" || fail "PITR 状态缺少 WAL 恢复数"
restored_segment_count="$(status_field "$PITR_STATUS_SOURCE_FILE" restored_wal_segment_count)" || fail "PITR 状态缺少 WAL 段恢复数"
first_restored_wal="$(status_field "$PITR_STATUS_SOURCE_FILE" first_restored_wal)" || fail "PITR 状态缺少首个 WAL"
last_restored_wal="$(status_field "$PITR_STATUS_SOURCE_FILE" last_restored_wal)" || fail "PITR 状态缺少末个 WAL"
wal_audit_sha="$(status_field "$PITR_STATUS_SOURCE_FILE" wal_audit_sha256)" || fail "PITR 状态缺少 WAL 审计摘要"
schema_count="$(status_field "$PITR_STATUS_SOURCE_FILE" schema_migrations)" || fail "PITR 状态缺少迁移数"
negative_balances="$(status_field "$PITR_STATUS_SOURCE_FILE" negative_balances)" || fail "PITR 状态缺少负余额数"
orphan_bets="$(status_field "$PITR_STATUS_SOURCE_FILE" orphan_bets)" || fail "PITR 状态缺少孤儿注单数"
[[ "$format_version" == 2 && "$pitr_completed" == 1 && "$target_reached" == 1 ]] || fail "PITR 状态不是 v2 成功状态"
[[ "$drill_luks_mount" == "$EXPECTED_DRILL_LUKS_MOUNT" ]] || fail "PITR 状态的 LUKS 演练盘证据无效"
[[ "$completed_epoch" =~ ^[0-9]+$ && "$target_epoch" =~ ^[0-9]+$ && "$target_epoch" -lt "$completed_epoch" ]] || fail "PITR 时间证据无效"
now_epoch="$(date +%s)"
(( completed_epoch <= now_epoch + 300 && now_epoch - completed_epoch <= PITR_STATUS_MAX_AGE_SECONDS )) || fail "PITR 状态太旧或位于未来"
[[ "$source_generation" =~ ^[0-9]{8}-[0-9]{6}-[0-9]+$ && "$source_remote" == "$PITR_STATUS_EXPECTED_BACKUP_REMOTE_SOURCE" ]] || fail "PITR 来源 generation 或远端绑定无效"
[[ "$source_snapshot_sha" =~ ^[0-9a-f]{64}$ && "$source_synced_epoch" =~ ^[0-9]+$ ]] || fail "PITR 来源快照或同步时间无效"
(( source_synced_epoch <= completed_epoch && source_synced_epoch <= now_epoch + 300 )) || fail "PITR 来源同步时间晚于演练或位于未来"
[[ "$source_base_count" =~ ^[1-9][0-9]*$ && "$source_wal_count" =~ ^[1-9][0-9]*$ && "$source_segment_count" =~ ^[1-9][0-9]*$ ]] || fail "PITR 来源制品计数无效"
(( 10#$source_segment_count <= 10#$source_wal_count )) || fail "PITR 来源 WAL 段计数大于 WAL 制品数"
[[ "$system_identifier" == "$PITR_STATUS_CLUSTER_ID" && "$base_sha" =~ ^[0-9a-f]{64}$ ]] || fail "PITR 集群或基础备份证据无效"
[[ "$replay_lsn" =~ ^[0-9A-F]+/[0-9A-F]+$ && "$timeline_id" =~ ^[0-9]+$ ]] || fail "PITR WAL 重放证据无效"
[[ "$restored_wal_count" =~ ^[1-9][0-9]*$ && "$restored_segment_count" =~ ^[1-9][0-9]*$ ]] || fail "PITR 未证明实际读取归档 WAL"
[[ "$first_restored_wal" =~ ^([0-9A-F]{24}(\.[0-9A-F]{8}\.backup)?|[0-9A-F]{8}\.history)$ ]] || fail "PITR 首个 WAL 证据无效"
[[ "$last_restored_wal" =~ ^([0-9A-F]{24}(\.[0-9A-F]{8}\.backup)?|[0-9A-F]{8}\.history)$ && "$wal_audit_sha" =~ ^[0-9a-f]{64}$ ]] || fail "PITR 末个 WAL 或审计摘要无效"
[[ "$schema_count" =~ ^[0-9]+$ && "$negative_balances" == 0 && "$orphan_bets" == 0 ]] || fail "PITR 业务一致性证据无效"
(( 10#$schema_count > 0 )) || fail "PITR 恢复库没有迁移记录"

work_dir="$(mktemp -d "${TMPDIR:-/tmp}/wangzhe-pitr-status.XXXXXX")"
cleanup_publish() { rm -rf -- "$work_dir"; }
trap cleanup_publish EXIT INT TERM
remote_manifest="$work_dir/last-pitr-success.status.sha256"
downloaded_manifest="$work_dir/remote.sha256"
downloaded_signature="$work_dir/remote.sig"
printf '%s  %s\n' "$actual_digest" last-pitr-success.status >"$remote_manifest"
chmod 0600 "$remote_manifest"
rclone_args=(--config "$PITR_STATUS_RCLONE_CONFIG" --contimeout 5s --timeout 30s --retries 1 --low-level-retries 1)
run_rclone() { timeout 45 rclone "${rclone_args[@]}" "$@"; }
run_rclone copyto "$PITR_STATUS_SOURCE_FILE" "$PITR_STATUS_REMOTE_DESTINATION" --checksum --no-traverse
run_rclone copyto "$remote_manifest" "$PITR_STATUS_REMOTE_DESTINATION.sha256" --checksum --no-traverse
run_rclone copyto "$PITR_STATUS_SIGNATURE_FILE" "$PITR_STATUS_REMOTE_DESTINATION.sig" --checksum --no-traverse
remote_digest="$(run_rclone hashsum sha256 "$PITR_STATUS_REMOTE_DESTINATION" --download | awk 'NR == 1 {print $1}')"
[[ "$remote_digest" == "$actual_digest" ]] || fail "PITR 状态异机回读摘要不一致"
run_rclone copyto "$PITR_STATUS_REMOTE_DESTINATION.sha256" "$downloaded_manifest" --no-traverse
read -r manifest_digest manifest_name manifest_extra <"$downloaded_manifest" || fail "PITR 远端状态摘要不可读"
[[ "$manifest_digest" == "$actual_digest" && "$manifest_name" == last-pitr-success.status && -z "${manifest_extra:-}" ]] || fail "PITR 远端状态摘要凭证无效"
run_rclone copyto "$PITR_STATUS_REMOTE_DESTINATION.sig" "$downloaded_signature" --no-traverse
[[ "$(strict_env_stat '%s' '%z' "$downloaded_signature")" == 64 ]] || fail "PITR 远端 Ed25519 状态签名长度无效"
[[ "$(sha256sum "$downloaded_signature" | awk '{print $1}')" == "$(sha256sum "$PITR_STATUS_SIGNATURE_FILE" | awk '{print $1}')" ]] || fail "PITR 远端状态签名回读不一致"
echo "PITR 演练成功状态已发布并完成异机回读校验"
