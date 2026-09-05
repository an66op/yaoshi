#!/usr/bin/env bash
set -euo pipefail
export PATH=/usr/sbin:/usr/bin:/sbin:/bin

usage() {
  echo "Usage: sudo /usr/local/sbin/wangzhe-production-encryption-rewrap --dry-run|--execute PREVIOUS_KEY_SLOT [BATCH_SIZE]"
}
[[ $# -ge 2 && $# -le 3 ]] || { usage >&2; exit 2; }
mode="$1"
previous_key_slot="$2"
batch_size="${3:-100}"
[[ "$mode" == "--dry-run" || "$mode" == "--execute" ]] || { usage >&2; exit 2; }
[[ "$previous_key_slot" =~ ^[1-9][0-9]*$ ]] || { echo "历史密钥位置必须为正整数" >&2; exit 2; }
[[ "$batch_size" =~ ^[1-9][0-9]*$ ]] && (( 10#$batch_size <= 1000 )) || { echo "批量必须在 1-1000 之间" >&2; exit 2; }
(( EUID == 0 )) || { echo "必须以 root 运行生产敏感字段重加密脚本" >&2; exit 1; }

for command_name in awk chmod curl dirname find flock install readlink rm sleep stat systemctl systemd-run uname; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done

SCRIPT_PATH="$(readlink -f "${BASH_SOURCE[0]}")"
SCRIPT_DIR="$(cd "$(dirname "$SCRIPT_PATH")" && pwd -P)"
[[ "$SCRIPT_DIR" == "/usr/local/libexec/wangzhe" ]] || {
  echo "请使用服务器预装的受信任敏感字段重加密工具" >&2
  exit 1
}
for trusted_dir in /usr/local /usr/local/libexec "$SCRIPT_DIR" "$SCRIPT_DIR/lib"; do
  [[ -d "$trusted_dir" && ! -L "$trusted_dir" && "$(stat -c '%u' "$trusted_dir")" == "0" && -z "$(find "$trusted_dir" -maxdepth 0 -perm /022 -print -quit)" ]] || {
    echo "可信重加密工具路径权限不安全：$trusted_dir" >&2
    exit 1
  }
done
for trusted_file in production-encryption-rewrap.sh production-config-check.sh release-integrity.sh lib/backend-env.sh lib/maintenance-edge.sh lib/encryption-capabilities.sh; do
  trusted_path="$SCRIPT_DIR/$trusted_file"
  [[ -f "$trusted_path" && ! -L "$trusted_path" && "$(stat -c '%u' "$trusted_path")" == "0" && -z "$(find "$trusted_path" -perm /022 -print -quit)" ]] || {
    echo "可信重加密工具无效或权限不安全：$trusted_path" >&2
    exit 1
  }
done

APP_ENV="${APP_ENV:-/etc/wangzhe/backend.env}"
CURRENT_LINK=/opt/wangzhe/current
RELEASE_ROOT=/opt/wangzhe/releases
MAINTENANCE_FLAG=/etc/wangzhe/maintenance
PUBLIC_URL="${PUBLIC_URL:-https://wz6688.app}"
PUBLIC_WWW_URL="${PUBLIC_WWW_URL:-https://www.wz6688.app}"
ADMIN_URL="${ADMIN_URL:-https://admin.wz888.site}"
READY_TIMEOUT_SECONDS="${READY_TIMEOUT_SECONDS:-90}"
[[ "$READY_TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] && (( READY_TIMEOUT_SECONDS >= 10 && READY_TIMEOUT_SECONDS <= 600 )) || {
  echo "READY_TIMEOUT_SECONDS 必须在 10-600 之间" >&2
  exit 1
}
[[ -L "$CURRENT_LINK" ]] || { echo "缺少 current 发布链接" >&2; exit 1; }

exec 9>/run/lock/wangzhe-deploy.lock
flock -n 9 || { echo "另一个发布、回滚或重加密任务仍在进行" >&2; exit 1; }

current_target="$(readlink -f "$CURRENT_LINK")"
[[ "$current_target" == "$RELEASE_ROOT/"* && -d "$current_target" ]] || { echo "current 发布路径无效" >&2; exit 1; }
[[ "$(stat -c '%u' "$current_target")" == "0" && -z "$(find "$current_target" -mindepth 1 ! -user root -print -quit)" && -z "$(find "$current_target" -mindepth 1 -perm /022 -print -quit)" ]] || {
  echo "current 发布版本所有者或权限不安全" >&2
  exit 1
}
"$SCRIPT_DIR/release-integrity.sh" verify "$current_target" >/dev/null
[[ -x "$current_target/bin/wangzhe-field-encryption-audit" ]] || { echo "current 缺少加密审计工具" >&2; exit 1; }

case "$(uname -m)" in
  x86_64) host_target=linux/amd64 ;;
  aarch64|arm64) host_target=linux/arm64 ;;
  *) echo "不支持的服务器架构" >&2; exit 1 ;;
esac
read -r release_target <"$current_target/TARGET"
[[ "$release_target" == "$host_target" ]] || { echo "current 发布架构不匹配" >&2; exit 1; }

# shellcheck source=lib/encryption-capabilities.sh
source "$SCRIPT_DIR/lib/encryption-capabilities.sh"
load_release_encryption_capabilities "$current_target"
# Initialized by load_release_encryption_capabilities from the trusted parser.
# shellcheck disable=SC2154
[[ "$encryption_cap_write_version" == "2" && "$encryption_cap_previous_key_fallback" == "true" ]] || {
  echo "current 发布版本不具备受控历史密钥重加密能力" >&2
  exit 1
}
# Initialized by load_release_encryption_capabilities from the trusted parser.
# shellcheck disable=SC2154
encryption_version_supported "$encryption_cap_read_versions" 1 && encryption_version_supported "$encryption_cap_read_versions" 2 || {
  echo "current 发布版本不能同时读取 v1/v2 信封" >&2
  exit 1
}

"$SCRIPT_DIR/production-config-check.sh" "$APP_ENV"
rewrap_unit=""
rewrap_unit_cleanup_armed=0

cleanup_rewrap_unit() {
  (( rewrap_unit_cleanup_armed == 1 )) || return 0
  local unit_name active_state load_state state_output stop_attempt
  unit_name="${rewrap_unit}.service"
  if [[ ! "$rewrap_unit" =~ ^wangzhe-field-encryption-rewrap-[0-9]+$ ]]; then
    echo "WANGZHE_ENCRYPTION_REWRAP_UNIT_CLEANUP_FAILED unit=invalid load_state=unknown active_state=unknown" >&2
    return 1
  fi

  systemctl stop "$unit_name" >/dev/null 2>&1 || true
  for ((stop_attempt = 0; stop_attempt < 30; stop_attempt++)); do
    state_output="$(systemctl show "$unit_name" --property=LoadState --property=ActiveState 2>/dev/null || true)"
    load_state="$(awk -F= '$1 == "LoadState" { print $2 }' <<<"$state_output")"
    active_state="$(awk -F= '$1 == "ActiveState" { print $2 }' <<<"$state_output")"
    if [[ "$load_state" == not-found || "$active_state" == inactive || "$active_state" == failed ]]; then
      rewrap_unit_cleanup_armed=0
      return 0
    fi
    sleep 1
  done

  systemctl kill --kill-whom=all --signal=KILL "$unit_name" >/dev/null 2>&1 || true
  systemctl stop "$unit_name" >/dev/null 2>&1 || true
  for ((stop_attempt = 0; stop_attempt < 30; stop_attempt++)); do
    state_output="$(systemctl show "$unit_name" --property=LoadState --property=ActiveState 2>/dev/null || true)"
    load_state="$(awk -F= '$1 == "LoadState" { print $2 }' <<<"$state_output")"
    active_state="$(awk -F= '$1 == "ActiveState" { print $2 }' <<<"$state_output")"
    if [[ "$load_state" == not-found || "$active_state" == inactive || "$active_state" == failed ]]; then
      rewrap_unit_cleanup_armed=0
      return 0
    fi
    sleep 1
  done

  echo "WANGZHE_ENCRYPTION_REWRAP_UNIT_CLEANUP_FAILED unit=$unit_name load_state=${load_state:-unknown} active_state=${active_state:-unknown}" >&2
  return 1
}

run_rewrap_tool() {
  local execution_mode="$1" proof_source="" run_status
  local -a proof_property=()
  local -a execute_argument=()
  rewrap_unit="wangzhe-field-encryption-rewrap-$$"
  if [[ "$execution_mode" == "--execute" ]]; then
    install -d -o root -g root -m 0700 /run/wangzhe-encryption-rewrap
    proof_source="/run/wangzhe-encryption-rewrap/freeze-proof-$$"
    (umask 077; printf '%s\n' 'backend-writes-frozen-v1' >"$proof_source")
    chmod 0400 "$proof_source"
    proof_property=(--property="LoadCredential=freeze-proof:$proof_source")
    execute_argument=(--execute-rewrap)
  fi
  rewrap_unit_cleanup_armed=1
  if systemd-run --quiet --wait --pipe --collect --service-type=exec \
    --unit="$rewrap_unit" --uid=wangzhe --gid=wangzhe \
    --property="EnvironmentFile=$APP_ENV" \
    "${proof_property[@]}" \
    --property=WorkingDirectory=/var/lib/wangzhe \
    --property=TimeoutStartSec=1800 \
    --property=UMask=0077 \
    --property=NoNewPrivileges=true \
    --property=PrivateTmp=true \
    --property=PrivateDevices=true \
    --property=ProtectHome=true \
    --property=ProtectSystem=strict \
    --property=ProtectHostname=true \
    --property=ProtectProc=invisible \
    --property=ProcSubset=pid \
    --property=ProtectKernelTunables=true \
    --property=ProtectKernelModules=true \
    --property=ProtectKernelLogs=true \
    --property=ProtectControlGroups=true \
    --property=ProtectClock=true \
    --property=RestrictNamespaces=true \
    --property=RestrictRealtime=true \
    --property=RestrictSUIDSGID=true \
    --property=RemoveIPC=true \
    --property='RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6' \
    --property=CapabilityBoundingSet= \
    --property=LockPersonality=true \
    --property=MemoryDenyWriteExecute=true \
    --property=SystemCallArchitectures=native \
    "$current_target/bin/wangzhe-field-encryption-audit" \
    --rewrap-previous-key="$previous_key_slot" \
    --rewrap-batch-size="$batch_size" \
    "${execute_argument[@]}"; then
    run_status=0
  else
    run_status=$?
  fi
  if ! cleanup_rewrap_unit; then
    return 1
  fi
  return "$run_status"
}

assert_backend_writes_frozen() {
  local active_state main_pid
  active_state="$(systemctl show wangzhe-backend.service --property=ActiveState --value)" || return 1
  main_pid="$(systemctl show wangzhe-backend.service --property=MainPID --value)" || return 1
  [[ "$active_state" == "inactive" && "$main_pid" == "0" ]]
}

proof_source="/run/wangzhe-encryption-rewrap/freeze-proof-$$"
restart_backend_on_exit=0
cleanup_rewrap() {
  local original_status=$? cleanup_failed=0
  trap - EXIT
  trap '' INT TERM
  cleanup_rewrap_unit || cleanup_failed=1
  if (( cleanup_failed == 0 )); then
    rm -f -- "$proof_source" || cleanup_failed=1
  fi
  if (( cleanup_failed == 0 && restart_backend_on_exit == 1 )); then
    if ! systemctl restart wangzhe-backend.service >/dev/null 2>&1; then
      echo "重加密退出时无法恢复后端；维护模式仍开启" >&2
      cleanup_failed=1
    fi
  fi
  if (( cleanup_failed == 1 )); then
    echo "WANGZHE_ENCRYPTION_REWRAP_CLEANUP_FAILED original_status=$original_status; 后端未自动启动，维护模式必须保持" >&2
    (( original_status != 0 )) || original_status=1
  fi
  exit "$original_status"
}
trap cleanup_rewrap EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if [[ "$mode" == "--dry-run" ]]; then
  run_rewrap_tool "$mode"
  echo "敏感字段重加密 dry-run 完成；没有修改数据库。"
  exit 0
fi

# shellcheck source=lib/maintenance-edge.sh
source "$SCRIPT_DIR/lib/maintenance-edge.sh"
capture_maintenance_marker_state "$MAINTENANCE_FLAG"
ensure_maintenance_marker "$MAINTENANCE_FLAG" || { echo "无法安全进入维护模式" >&2; exit 1; }
verify_maintenance_edge "$PUBLIC_URL" "$PUBLIC_WWW_URL" "$ADMIN_URL" || {
  echo "公网入口未可靠进入维护模式，未执行重加密" >&2
  exit 1
}

restart_backend_on_exit=1
systemctl stop wangzhe-backend.service
if ! assert_backend_writes_frozen; then
  echo "后端敏感字段写入尚未完全停止，拒绝执行重加密" >&2
  exit 1
fi

if ! run_rewrap_tool "$mode"; then
  echo "敏感字段重加密或最终归零盘点失败；历史密钥不得移除" >&2
  exit 1
fi
if ! assert_backend_writes_frozen || ! verify_maintenance_edge "$PUBLIC_URL" "$PUBLIC_WWW_URL" "$ADMIN_URL"; then
  echo "最终盘点后停写状态或维护入口失效；历史密钥不得移除" >&2
  exit 1
fi
restart_backend_on_exit=0
rm -f -- "$proof_source"
echo "敏感字段重加密与最终归零盘点通过。后端仍已停止、维护模式仍开启。"
echo "现在才可从配置移除指定历史密钥；随后启动后端并运行完整 production-readiness，期间不得恢复流量。"
