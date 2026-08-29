#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

readonly EXPECTED_ENV_FILE=/etc/wangzhe/pitr-source-sync.env
readonly EXPECTED_SOURCE_ROOT=/var/lib/wangzhe-pitr-source
readonly EXPECTED_ISOLATION_MARKER=/etc/wangzhe/recovery-host
readonly LOCK_FILE=/run/wangzhe-pitr-source/operation.lock

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=lib/strict-env.sh
source "$SCRIPT_DIR/lib/strict-env.sh"
# shellcheck source=lib/encrypted-backup.sh
source "$SCRIPT_DIR/lib/encrypted-backup.sh"

fail() {
  echo "$*" >&2
  exit 1
}

validate_recovery_host() {
  local owner mode mode_value marker_value marker_extra
  [[ -f "$EXPECTED_ISOLATION_MARKER" && ! -L "$EXPECTED_ISOLATION_MARKER" ]] || fail "缺少隔离恢复主机确认标记"
  validate_no_symlink_path_components "$EXPECTED_ISOLATION_MARKER"
  owner="$(strict_env_stat '%u' '%u' "$EXPECTED_ISOLATION_MARKER")"
  mode="$(strict_env_stat '%a' '%Lp' "$EXPECTED_ISOLATION_MARKER")"
  [[ "$owner" == 0 && "$mode" =~ ^[0-7]{3,4}$ ]] || fail "隔离恢复主机标记必须由 root 所有"
  mode_value=$((8#$mode))
  (( (mode_value & 022) == 0 )) || fail "隔离恢复主机标记不能被非 root 修改"
  read -r marker_value marker_extra <"$EXPECTED_ISOLATION_MARKER" || fail "隔离恢复主机标记不可读"
  [[ "$marker_value" == WANGZHE_ISOLATED_RECOVERY_HOST && -z "${marker_extra:-}" ]] || fail "隔离恢复主机标记内容无效"
}

safe_remove_tree() {
  local target="$1"
  case "$target" in
    "$EXPECTED_SOURCE_ROOT"/generations/.staging-*|"$EXPECTED_SOURCE_ROOT"/generations/[0-9]*) ;;
    *) fail "拒绝清理 PITR 异机同步目录之外的路径：$target" ;;
  esac
  [[ -d "$target" && ! -L "$target" ]] || fail "拒绝清理非普通目录：$target"
  mountpoint --quiet "$target" && fail "拒绝清理挂载点：$target"
  find "$target" -xdev -mindepth 1 -depth -delete
  rmdir -- "$target"
}

validate_source_root() {
  local owner mode mode_value unexpected existing_generation active_generation active_extra
  [[ "$PITR_SOURCE_SYNC_ROOT" == "$EXPECTED_SOURCE_ROOT" ]] || fail "PITR_SOURCE_SYNC_ROOT 必须精确为 $EXPECTED_SOURCE_ROOT"
  [[ -d "$PITR_SOURCE_SYNC_ROOT" && ! -L "$PITR_SOURCE_SYNC_ROOT" ]] || fail "systemd 必须预先创建 $PITR_SOURCE_SYNC_ROOT"
  validate_no_symlink_path_components "$PITR_SOURCE_SYNC_ROOT"
  owner="$(strict_env_stat '%u' '%u' "$PITR_SOURCE_SYNC_ROOT")"
  mode="$(strict_env_stat '%a' '%Lp' "$PITR_SOURCE_SYNC_ROOT")"
  [[ "$owner" == "${EUID:-$(id -u)}" && "$mode" =~ ^[0-7]{3,4}$ ]] || fail "PITR 异机同步根目录 owner/mode 无效"
  mode_value=$((8#$mode))
  (( (mode_value & 077) == 0 )) || fail "PITR 异机同步根目录必须为服务用户私有"

  unexpected="$(find "$PITR_SOURCE_SYNC_ROOT" -xdev -mindepth 1 -maxdepth 1 \
    ! -name generations ! -name active-generation -print -quit)"
  [[ -z "$unexpected" ]] || fail "PITR 异机同步根目录包含意外条目：$unexpected"
  if [[ -e "$PITR_SOURCE_SYNC_ROOT/active-generation" || -L "$PITR_SOURCE_SYNC_ROOT/active-generation" ]]; then
    [[ -f "$PITR_SOURCE_SYNC_ROOT/active-generation" && ! -L "$PITR_SOURCE_SYNC_ROOT/active-generation" ]] || fail "active-generation 不是普通文件"
  fi
  mkdir -p -- "$PITR_SOURCE_SYNC_ROOT/generations"
  chmod 0700 -- "$PITR_SOURCE_SYNC_ROOT/generations"
  [[ -d "$PITR_SOURCE_SYNC_ROOT/generations" && ! -L "$PITR_SOURCE_SYNC_ROOT/generations" ]] || fail "PITR generations 目录无效"
  validate_no_symlink_path_components "$PITR_SOURCE_SYNC_ROOT/generations"
  unexpected="$(find "$PITR_SOURCE_SYNC_ROOT/generations" -xdev -mindepth 1 -maxdepth 1 \
    ! -type d -print -quit)"
  [[ -z "$unexpected" ]] || fail "PITR generations 包含非目录条目：$unexpected"
  while IFS= read -r existing_generation; do
    [[ "$(basename "$existing_generation")" =~ ^[0-9]{8}-[0-9]{6}-[0-9]+$ ]] || fail "发现未完成或命名异常的 PITR generation，拒绝自动覆盖：$existing_generation"
  done < <(find "$PITR_SOURCE_SYNC_ROOT/generations" -xdev -mindepth 1 -maxdepth 1 -type d -print)
  unexpected="$(find "$PITR_SOURCE_SYNC_ROOT/generations" -xdev -mindepth 2 \
    \( -type l -o \( ! -type f -a ! -type d \) \) -print -quit)"
  [[ -z "$unexpected" ]] || fail "已有 PITR generation 包含符号链接或特殊文件：$unexpected"
  if [[ -f "$PITR_SOURCE_SYNC_ROOT/active-generation" ]]; then
    read -r active_generation active_extra <"$PITR_SOURCE_SYNC_ROOT/active-generation" || fail "active-generation 不可读"
    [[ "$active_generation" =~ ^[0-9]{8}-[0-9]{6}-[0-9]+$ && -z "${active_extra:-}" ]] || fail "active-generation 内容无效"
    [[ -d "$PITR_SOURCE_SYNC_ROOT/generations/$active_generation" && ! -L "$PITR_SOURCE_SYNC_ROOT/generations/$active_generation" ]] || fail "active-generation 指向不存在的 generation"
  fi
}

remote_snapshot() {
  local output="$1"
  run_rclone lsjson "$PITR_SOURCE_SYNC_REMOTE_DESTINATION" --recursive --files-only >"$output"
  jq -e '
    type == "array" and length > 0 and
    all(.[];
      (.IsDir == false) and (.Path | type == "string") and
      (.Path | test("^[A-Za-z0-9_.-]+$")) and
      (.Size | type == "number") and (.Size > 0)
    ) and
    ([.[].Path] | length == (unique | length))
  ' "$output" >/dev/null || fail "PITR 远端目录为空，或包含目录、重复/异常名称、空文件"

  while IFS= read -r remote_name; do
    if [[ "$remote_name" =~ ^basebackup-[0-9]{8}-[0-9]{6}-[0-9]+\.tar\.age(\.sha256|\.provenance(\.sig)?)?$ ]]; then
      continue
    fi
    if [[ "$remote_name" =~ ^([0-9A-F]{24}(\.[0-9A-F]{8}\.backup)?|[0-9A-F]{8}\.history)\.age(\.sha256|\.source\.sha256|\.provenance(\.sig)?)?$ ]]; then
      continue
    fi
    fail "PITR 远端目录包含意外文件：$remote_name"
  done < <(jq -r '.[].Path' "$output")
}

env_source="${1:-$EXPECTED_ENV_FILE}"
[[ "$env_source" == "$EXPECTED_ENV_FILE" ]] || fail "PITR 异机同步只接受 $EXPECTED_ENV_FILE"
validate_no_symlink_path_components "$env_source"
load_strict_env "$env_source" '^PITR_SOURCE_SYNC_[A-Z0-9_]+$'

for command_name in awk basename chmod cmp date dirname find flock grep id jq mkdir mountpoint mv openssl rclone rm rmdir sha256sum sort stat timeout; do
  command -v "$command_name" >/dev/null 2>&1 || fail "缺少命令：$command_name"
done
for key in PITR_SOURCE_SYNC_ROOT PITR_SOURCE_SYNC_CLUSTER_ID PITR_SOURCE_SYNC_REMOTE_DESTINATION PITR_SOURCE_SYNC_RCLONE_CONFIG PITR_SOURCE_SYNC_PROVENANCE_VERIFY_KEY_FILE; do
  [[ -n "${!key:-}" ]] || fail "缺少 $key"
done
[[ "$(id -un)" == postgres ]] || fail "PITR 异机同步必须以 postgres 服务用户运行"
[[ "$PITR_SOURCE_SYNC_CLUSTER_ID" =~ ^[0-9]{10,30}$ ]] || fail "PITR_SOURCE_SYNC_CLUSTER_ID 必须是 PostgreSQL system identifier"
validate_remote_destination "$PITR_SOURCE_SYNC_REMOTE_DESTINATION"
[[ "$(basename "${PITR_SOURCE_SYNC_REMOTE_DESTINATION#*:}")" == "$PITR_SOURCE_SYNC_CLUSTER_ID" ]] || fail "PITR 远端目录末级必须精确等于集群标识"
[[ "$PITR_SOURCE_SYNC_RCLONE_CONFIG" == /etc/wangzhe/pitr-wal-read-rclone.conf ]] || fail "PITR 异机同步必须使用固定的 PITR 前缀只读 rclone 配置"
[[ "$PITR_SOURCE_SYNC_PROVENANCE_VERIFY_KEY_FILE" == /etc/wangzhe/pitr-provenance-ed25519-public.pem ]] || fail "PITR 异机同步验签公钥必须使用固定路径"
validate_rclone_config "$PITR_SOURCE_SYNC_RCLONE_CONFIG"
validate_ed25519_public_key "$PITR_SOURCE_SYNC_PROVENANCE_VERIFY_KEY_FILE" "PITR 备份来源 Ed25519 公钥"
validate_recovery_host
validate_source_root

[[ -d "$(dirname "$LOCK_FILE")" && ! -L "$(dirname "$LOCK_FILE")" ]] || fail "systemd 未创建 PITR 同步锁目录"
validate_no_symlink_path_components "$(dirname "$LOCK_FILE")"
if [[ -e "$LOCK_FILE" || -L "$LOCK_FILE" ]]; then
  [[ -f "$LOCK_FILE" && ! -L "$LOCK_FILE" && "$(strict_env_stat '%h' '%l' "$LOCK_FILE")" == 1 ]] || fail "PITR 同步锁文件无效"
fi
exec 9>>"$LOCK_FILE"
flock -w 30 9 || fail "PITR 恢复演练或另一个同步任务仍在使用异机源"

rclone_args=(--config "$PITR_SOURCE_SYNC_RCLONE_CONFIG" --contimeout 10s --timeout 10m --retries 3 --low-level-retries 3)
run_rclone() { timeout 12h rclone "${rclone_args[@]}" "$@"; }

umask 077
generation="$(date -u +%Y%m%d-%H%M%S)-$$"
[[ "$generation" =~ ^[0-9]{8}-[0-9]{6}-[0-9]+$ ]] || fail "无法生成安全的 PITR generation 名"
staging="$PITR_SOURCE_SYNC_ROOT/generations/.staging-$generation"
final_generation="$PITR_SOURCE_SYNC_ROOT/generations/$generation"
pointer_partial="$PITR_SOURCE_SYNC_ROOT/.active-generation.partial.$$"
[[ ! -e "$staging" && ! -L "$staging" && ! -e "$final_generation" && ! -L "$final_generation" ]] || fail "PITR generation 已存在"
[[ ! -e "$pointer_partial" && ! -L "$pointer_partial" ]] || fail "PITR generation 指针临时文件已存在"
mkdir -m 0700 -- "$staging" "$staging/remote" "$staging/base" "$staging/wal"

cleanup_sync() {
  local rc=$?
  trap - EXIT INT TERM
  if [[ -n "${staging:-}" && -d "$staging" && ! -L "$staging" ]]; then
    safe_remove_tree "$staging"
  fi
  if [[ -n "${pointer_partial:-}" && -f "$pointer_partial" && ! -L "$pointer_partial" ]]; then
    rm -f -- "$pointer_partial"
  fi
  exit "$rc"
}
trap cleanup_sync EXIT INT TERM

remote_snapshot "$staging/remote-before.json"
run_rclone copy "$PITR_SOURCE_SYNC_REMOTE_DESTINATION" "$staging/remote" --no-traverse
remote_snapshot "$staging/remote-after.json"
jq -S 'sort_by(.Path) | map({Path,Size,ModTime})' "$staging/remote-before.json" >"$staging/before.canonical"
jq -S 'sort_by(.Path) | map({Path,Size,ModTime})' "$staging/remote-after.json" >"$staging/after.canonical"
cmp -s "$staging/before.canonical" "$staging/after.canonical" || fail "PITR 远端目录在下载期间发生变化，拒绝发布混合快照"
jq -r '.[].Path' "$staging/remote-before.json" | sort >"$staging/remote-files"
find "$staging/remote" -xdev -type f -printf '%P\n' | sort >"$staging/local-files"
cmp -s "$staging/remote-files" "$staging/local-files" || fail "PITR 远端与本地下载文件集合不一致"
unsafe_entry="$(find "$staging/remote" -xdev \( -type l -o \( ! -type f -a ! -type d \) \) -print -quit)"
[[ -z "$unsafe_entry" ]] || fail "PITR 下载包含符号链接或特殊文件：$unsafe_entry"

mkdir -m 0700 -- "$staging/base/$PITR_SOURCE_SYNC_CLUSTER_ID" "$staging/wal/$PITR_SOURCE_SYNC_CLUSTER_ID"
base_count=0
wal_count=0
wal_segment_count=0
while IFS= read -r artifact; do
  artifact_name="$(basename "$artifact")"
  checksum_file="$artifact.sha256"
  provenance_file="$artifact.provenance"
  signature_file="$artifact.provenance.sig"
  [[ -f "$checksum_file" && ! -L "$checksum_file" ]] || fail "PITR 制品缺少 SHA-256 清单：$artifact_name"
  read -r expected_checksum expected_name checksum_extra <"$checksum_file" || fail "PITR SHA-256 清单不可读：$artifact_name"
  actual_checksum="$(sha256sum "$artifact" | awk '{print $1}')"
  [[ "$expected_checksum" =~ ^[0-9a-f]{64}$ && "$expected_checksum" == "$actual_checksum" && "$expected_name" == "$artifact_name" && -z "${checksum_extra:-}" ]] || fail "PITR 制品完整 SHA-256 校验失败：$artifact_name"

  if [[ "$artifact_name" =~ ^basebackup-[0-9]{8}-[0-9]{6}-[0-9]+\.tar\.age$ ]]; then
    destination_dir="$staging/base/$PITR_SOURCE_SYNC_CLUSTER_ID"
    artifact_class=pitr-basebackup
    base_count=$((base_count + 1))
  else
    wal_name="${artifact_name%.age}"
    [[ "$wal_name" =~ ^([0-9A-F]{24}(\.[0-9A-F]{8}\.backup)?|[0-9A-F]{8}\.history)$ ]] || fail "PITR WAL 制品名无效：$artifact_name"
    source_manifest="$artifact.source.sha256"
    [[ -f "$source_manifest" && ! -L "$source_manifest" ]] || fail "PITR WAL 缺少源文件凭证：$artifact_name"
    read -r plaintext_checksum recorded_wal recorded_cluster source_extra <"$source_manifest" || fail "PITR WAL 源文件凭证不可读：$artifact_name"
    [[ "$plaintext_checksum" =~ ^[0-9a-f]{64}$ && "$recorded_wal" == "$wal_name" && "$recorded_cluster" == "$PITR_SOURCE_SYNC_CLUSTER_ID" && -z "${source_extra:-}" ]] || fail "PITR WAL 源文件凭证与集群不匹配：$artifact_name"
    destination_dir="$staging/wal/$PITR_SOURCE_SYNC_CLUSTER_ID"
    artifact_class=pitr-wal
    wal_count=$((wal_count + 1))
    [[ "$wal_name" =~ ^[0-9A-F]{24}$ ]] && wal_segment_count=$((wal_segment_count + 1))
    mv -- "$source_manifest" "$destination_dir/$(basename "$source_manifest")"
  fi

  verify_backup_provenance "$artifact" "$artifact_class" "$PITR_SOURCE_SYNC_CLUSTER_ID" \
    "${PITR_SOURCE_SYNC_REMOTE_DESTINATION%/}/$artifact_name" "$PITR_SOURCE_SYNC_PROVENANCE_VERIFY_KEY_FILE" || \
    fail "PITR 制品来源签名或绑定字段无效：$artifact_name"

  mv -- "$artifact" "$destination_dir/$artifact_name"
  mv -- "$checksum_file" "$destination_dir/$(basename "$checksum_file")"
  mv -- "$provenance_file" "$destination_dir/$(basename "$provenance_file")"
  mv -- "$signature_file" "$destination_dir/$(basename "$signature_file")"
  printf '%s  %s/%s\n' "$actual_checksum" "${PITR_SOURCE_SYNC_REMOTE_DESTINATION%/}" "$artifact_name" >"$destination_dir/$artifact_name.offsite-ok"
  validate_offsite_marker "$destination_dir/$artifact_name" "$PITR_SOURCE_SYNC_REMOTE_DESTINATION" || fail "PITR 异机来源凭证生成失败：$artifact_name"
done < <(find "$staging/remote" -xdev -maxdepth 1 -type f -name '*.age' -print | sort)

[[ "$base_count" -gt 0 ]] || fail "PITR 异机目录没有基础备份"
[[ "$wal_count" -gt 0 && "$wal_segment_count" -gt 0 ]] || fail "PITR 异机目录没有可重放 WAL 段"
leftover="$(find "$staging/remote" -xdev -mindepth 1 -print -quit)"
[[ -z "$leftover" ]] || fail "PITR 异机目录存在孤立或未消费文件：$leftover"
rmdir -- "$staging/remote"
snapshot_sha256="$(sha256sum "$staging/before.canonical" | awk '{print $1}')"
printf 'format_version=1\ncluster_id=%s\nremote_destination=%s\nremote_snapshot_sha256=%s\nbasebackup_count=%s\nwal_count=%s\nwal_segment_count=%s\nsynced_at_epoch=%s\n' \
  "$PITR_SOURCE_SYNC_CLUSTER_ID" "$PITR_SOURCE_SYNC_REMOTE_DESTINATION" "$snapshot_sha256" \
  "$base_count" "$wal_count" "$wal_segment_count" "$(date +%s)" >"$staging/source.status"
rm -f -- "$staging/remote-before.json" "$staging/remote-after.json" "$staging/before.canonical" "$staging/after.canonical" "$staging/remote-files" "$staging/local-files"
find "$staging" -xdev -type d -exec chmod 0700 {} +
find "$staging" -xdev -type f -exec chmod 0600 {} +

mv -- "$staging" "$final_generation"
staging=""
printf '%s\n' "$generation" >"$pointer_partial"
chmod 0600 "$pointer_partial"
mv -- "$pointer_partial" "$PITR_SOURCE_SYNC_ROOT/active-generation"
pointer_partial=""

kept=0
while IFS= read -r old_generation; do
  kept=$((kept + 1))
  (( kept <= 2 )) && continue
  [[ "$(basename "$old_generation")" =~ ^[0-9]{8}-[0-9]{6}-[0-9]+$ ]] || fail "拒绝清理命名异常的 PITR generation"
  safe_remove_tree "$old_generation"
done < <(find "$PITR_SOURCE_SYNC_ROOT/generations" -xdev -mindepth 1 -maxdepth 1 -type d -print | sort -r)

trap - EXIT INT TERM
echo "PITR 异机恢复源同步完成：generation=$generation base=$base_count wal=$wal_count"
