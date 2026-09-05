#!/usr/bin/env bash
set -euo pipefail
export PATH=/usr/sbin:/usr/bin:/sbin:/bin

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  echo "Usage: sudo CONFIRM_SCHEMA_COMPATIBLE=YES READY_TIMEOUT_SECONDS=90 /usr/local/sbin/wangzhe-production-rollback"
  echo "Checks schema and encrypted-field compatibility, then switches current to previous; it never rolls back the database."
  exit 0
fi
[[ $# -eq 0 ]] || { echo "回滚脚本不接受版本路径；只允许回到受控 previous 版本" >&2; exit 2; }
(( EUID == 0 )) || { echo "必须以 root 运行生产回滚脚本" >&2; exit 1; }
[[ "${CONFIRM_SCHEMA_COMPATIBLE:-}" == "YES" ]] || {
  echo "必须先核对新增迁移与旧版兼容，再设置 CONFIRM_SCHEMA_COMPATIBLE=YES" >&2
  exit 1
}

for command_name in awk chmod curl dirname find flock grep install ln mktemp mv readlink rm sleep stat systemctl systemd-run tr uname; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done

SCRIPT_PATH="$(readlink -f "${BASH_SOURCE[0]}")"
SCRIPT_DIR="$(cd "$(dirname "$SCRIPT_PATH")" && pwd -P)"
[[ "$SCRIPT_DIR" == "/usr/local/libexec/wangzhe" ]] || {
  echo "请使用服务器预装的 /usr/local/sbin/wangzhe-production-rollback，禁止直接执行发布包内脚本" >&2
  exit 1
}
for trusted_dir in /usr/local /usr/local/libexec "$SCRIPT_DIR" "$SCRIPT_DIR/lib"; do
  [[ -d "$trusted_dir" && ! -L "$trusted_dir" && "$(stat -c '%u' "$trusted_dir")" == "0" && -z "$(find "$trusted_dir" -maxdepth 0 -perm /022 -print -quit)" ]] || {
    echo "可信回滚工具路径必须属于 root 且不能允许非 root 写入：$trusted_dir" >&2
    exit 1
  }
done
for trusted_file in production-rollback.sh production-config-check.sh release-integrity.sh lib/backend-env.sh lib/maintenance-edge.sh lib/encryption-capabilities.sh; do
  trusted_path="$SCRIPT_DIR/$trusted_file"
  [[ -f "$trusted_path" && ! -L "$trusted_path" && "$(stat -c '%u' "$trusted_path")" == "0" && -z "$(find "$trusted_path" -perm /022 -print -quit)" ]] || {
    echo "可信回滚工具无效、所有者或权限不安全：$trusted_path" >&2
    exit 1
  }
done

READY_TIMEOUT_SECONDS="${READY_TIMEOUT_SECONDS:-90}"
RELEASE_ROOT=/opt/wangzhe/releases
CURRENT_LINK=/opt/wangzhe/current
PREVIOUS_LINK=/opt/wangzhe/previous
APP_ENV="${APP_ENV:-/etc/wangzhe/backend.env}"
PUBLIC_URL="${PUBLIC_URL:-https://wz6688.app}"
PUBLIC_WWW_URL="${PUBLIC_WWW_URL:-https://www.wz6688.app}"
ADMIN_URL="${ADMIN_URL:-https://admin.wz888.site}"
MAINTENANCE_FLAG=/etc/wangzhe/maintenance
[[ "$READY_TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] && (( READY_TIMEOUT_SECONDS >= 10 && READY_TIMEOUT_SECONDS <= 600 )) || {
  echo "READY_TIMEOUT_SECONDS 必须在 10-600 之间" >&2
  exit 1
}
[[ -L "$CURRENT_LINK" && -L "$PREVIOUS_LINK" ]] || { echo "缺少 current 或 previous 发布链接" >&2; exit 1; }

exec 9>/run/lock/wangzhe-deploy.lock
flock -n 9 || { echo "另一个发布或回滚仍在进行" >&2; exit 1; }

# Record whether maintenance was already active before this rollback. Rollback
# only probes /ready, so every success path retains the marker until an operator
# completes the full production-readiness.sh gate and removes it manually.
# shellcheck source=lib/maintenance-edge.sh
source "$SCRIPT_DIR/lib/maintenance-edge.sh"
capture_maintenance_marker_state "$MAINTENANCE_FLAG"

current_target="$(readlink -f "$CURRENT_LINK")"
previous_target="$(readlink -f "$PREVIOUS_LINK")"
[[ "$current_target" != "$previous_target" ]] || { echo "current 与 previous 指向同一版本" >&2; exit 1; }

"$SCRIPT_DIR/production-config-check.sh" "$APP_ENV"
# shellcheck source=lib/backend-env.sh
source "$SCRIPT_DIR/lib/backend-env.sh"
load_backend_env "$APP_ENV"
# shellcheck source=lib/encryption-capabilities.sh
source "$SCRIPT_DIR/lib/encryption-capabilities.sh"

case "$(uname -m)" in
  x86_64) host_target=linux/amd64 ;;
  aarch64|arm64) host_target=linux/arm64 ;;
  *) echo "不支持的服务器架构：$(uname -m)" >&2; exit 1 ;;
esac
validate_release() {
  local candidate="$1" bad_owner bad_mode candidate_target
  [[ "$candidate" == "$RELEASE_ROOT/"* && -d "$candidate" ]] || return 1
  [[ "$(stat -c '%u' "$candidate")" == "0" ]] || return 1
  bad_owner="$(find "$candidate" -mindepth 1 ! -user root -print -quit)"
  [[ -z "$bad_owner" ]] || return 1
  bad_mode="$(find "$candidate" -mindepth 1 -perm /022 -print -quit)"
  [[ -z "$bad_mode" ]] || return 1
  "$SCRIPT_DIR/release-integrity.sh" verify "$candidate" >/dev/null || return 1
  [[ -x "$candidate/bin/wangzhe-field-encryption-audit" ]] || return 1
  [[ -f "$candidate/FIELD_ENCRYPTION_CAPABILITIES" && ! -L "$candidate/FIELD_ENCRYPTION_CAPABILITIES" ]] || return 1
  read -r candidate_target <"$candidate/TARGET"
  [[ "$candidate_target" == "$host_target" ]]
}
for target in "$current_target" "$previous_target"; do
  validate_release "$target" || { echo "发布版本被修改、权限不安全或完整性校验失败：$target" >&2; exit 1; }
done

load_release_encryption_capabilities "$current_target" || exit 1
# Initialized by load_release_encryption_capabilities from the trusted parser.
# shellcheck disable=SC2154
current_read_versions="$encryption_cap_read_versions"
# shellcheck disable=SC2154
current_write_version="$encryption_cap_write_version"
load_release_encryption_capabilities "$previous_target" || exit 1
# shellcheck disable=SC2154
previous_read_versions="$encryption_cap_read_versions"
# shellcheck disable=SC2154
previous_write_version="$encryption_cap_write_version"
# shellcheck disable=SC2154
previous_key_fallback="$encryption_cap_previous_key_fallback"

# The target must understand envelopes the current release can write even when
# the encrypted tables happen to be empty. The current release must likewise
# understand target writes so a failed rollback can be recovered safely.
encryption_version_supported "$previous_read_versions" "$current_write_version" || {
  echo "上一版本无法读取当前版本的加密信封写入格式，拒绝回滚" >&2
  exit 1
}
encryption_version_supported "$current_read_versions" "$previous_write_version" || {
  echo "当前版本无法读取上一版本的加密信封写入格式，拒绝回滚" >&2
  exit 1
}

wait_for_backend() {
  local ready_url="http://${BACKEND_SERVER_BIND}:${BACKEND_SERVER_PORT}/ready" second
  for ((second = 1; second <= READY_TIMEOUT_SECONDS; second++)); do
    if curl --silent --show-error --fail --max-time 2 "$ready_url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

assert_backend_writes_frozen() {
  local active_state main_pid
  active_state="$(systemctl show wangzhe-backend.service --property=ActiveState --value)" || return 1
  main_pid="$(systemctl show wangzhe-backend.service --property=MainPID --value)" || return 1
  [[ "$active_state" == "inactive" && "$main_pid" == "0" ]]
}

audit_encryption_for_rollback() {
  local audit_unit="wangzhe-field-encryption-rollback-$$"
  systemd-run --quiet --wait --pipe --collect --service-type=exec \
    --unit="$audit_unit" --uid=wangzhe --gid=wangzhe \
    --property="EnvironmentFile=$APP_ENV" \
    --property=WorkingDirectory=/var/lib/wangzhe \
    --property=TimeoutStartSec=300 \
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
    --target-read-versions="$previous_read_versions" \
    --target-supports-previous-keys="$previous_key_fallback"
}

link_tmp="/opt/wangzhe/.rollback-$$"
previous_tmp="/opt/wangzhe/.previous-rollback-$$"
restart_current_on_exit=0
cleanup_rollback() {
  local active_target=""
  rm -f -- "$link_tmp" "$previous_tmp"
  if (( restart_current_on_exit == 1 )); then
    active_target="$(readlink -f "$CURRENT_LINK" 2>/dev/null || true)"
    if [[ "$active_target" == "$current_target" ]]; then
      systemctl restart wangzhe-backend.service >/dev/null 2>&1 || \
        echo "退出回滚时无法恢复当前后端；维护模式仍开启" >&2
    fi
  fi
}
trap cleanup_rollback EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if ! ensure_maintenance_marker "$MAINTENANCE_FLAG"; then
  echo "无法安全进入维护模式，未切换任何版本" >&2
  exit 1
fi
if ! verify_maintenance_edge "$PUBLIC_URL" "$PUBLIC_WWW_URL" "$ADMIN_URL"; then
  echo "当前 Nginx 未可靠进入维护模式；保留维护标记且未切换任何版本" >&2
  exit 1
fi
restart_current_on_exit=1
if ! systemctl stop wangzhe-backend.service || ! assert_backend_writes_frozen; then
  echo "无法停止当前后端并冻结敏感字段写入，未切换任何版本" >&2
  exit 1
fi
if ! audit_encryption_for_rollback; then
  if systemctl restart wangzhe-backend.service && wait_for_backend; then
    restart_current_on_exit=0
    echo "上一版本与数据库加密信封或当前密钥配置不兼容；已恢复当前后端，保持维护模式" >&2
  else
    echo "加密信封回滚门禁失败，当前后端也未能恢复；保持维护模式，请立即检查服务" >&2
  fi
  exit 1
fi
ln -s "$previous_target" "$link_tmp"
mv -Tf "$link_tmp" "$CURRENT_LINK"
restart_current_on_exit=0

if ! systemctl restart wangzhe-backend.service || ! wait_for_backend; then
  ln -s "$current_target" "$link_tmp"
  mv -Tf "$link_tmp" "$CURRENT_LINK"
  if systemctl restart wangzhe-backend.service && wait_for_backend; then
    echo "上一版无法就绪，已恢复并验证回滚前代码" >&2
  else
    echo "上一版无法就绪，回滚前版本恢复后也未就绪，请立即检查服务" >&2
  fi
  exit 1
fi

ln -s "$current_target" "$previous_tmp"
mv -Tf "$previous_tmp" "$PREVIOUS_LINK"
systemctl reload nginx.service
echo "代码回滚成功：$previous_target"
# Initialized by sourced maintenance-edge.sh.
# shellcheck disable=SC2154
if (( maintenance_was_active == 1 )); then
  echo "回滚前已有维护标记，已按原样保留。"
else
  echo "本次回滚创建的维护标记已保留。"
fi
echo "维护模式仍然开启；请先运行完整 production-readiness.sh 门禁，通过后再人工删除 ${MAINTENANCE_FLAG}。"
echo "数据库未回退；如需恢复数据库，必须进入维护窗口并人工执行已验证备份。"
