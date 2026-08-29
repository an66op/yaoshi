#!/usr/bin/env bash
set -euo pipefail
export PATH=/usr/sbin:/usr/bin:/sbin:/bin

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  echo "Usage: sudo CONFIRM_SCHEMA_COMPATIBLE=YES READY_TIMEOUT_SECONDS=90 /usr/local/sbin/wangzhe-production-rollback"
  echo "Switches /opt/wangzhe/current to /opt/wangzhe/previous; it never rolls back the database."
  exit 0
fi
[[ $# -eq 0 ]] || { echo "回滚脚本不接受版本路径；只允许回到受控 previous 版本" >&2; exit 2; }
(( EUID == 0 )) || { echo "必须以 root 运行生产回滚脚本" >&2; exit 1; }
[[ "${CONFIRM_SCHEMA_COMPATIBLE:-}" == "YES" ]] || {
  echo "必须先核对新增迁移与旧版兼容，再设置 CONFIRM_SCHEMA_COMPATIBLE=YES" >&2
  exit 1
}

for command_name in awk chmod curl dirname find flock grep install ln mktemp mv readlink rm sleep stat systemctl tr uname; do
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
for trusted_file in production-rollback.sh production-config-check.sh release-integrity.sh lib/backend-env.sh lib/maintenance-edge.sh; do
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
ADMIN_URL="${ADMIN_URL:-https://admin.wz6688.app}"
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
  read -r candidate_target <"$candidate/TARGET"
  [[ "$candidate_target" == "$host_target" ]]
}
for target in "$current_target" "$previous_target"; do
  validate_release "$target" || { echo "发布版本被修改、权限不安全或完整性校验失败：$target" >&2; exit 1; }
done

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

if ! ensure_maintenance_marker "$MAINTENANCE_FLAG"; then
  echo "无法安全进入维护模式，未切换任何版本" >&2
  exit 1
fi
if ! verify_maintenance_edge "$PUBLIC_URL" "$ADMIN_URL"; then
  echo "当前 Nginx 未可靠进入维护模式；保留维护标记且未切换任何版本" >&2
  exit 1
fi
link_tmp="/opt/wangzhe/.rollback-$$"
cleanup_link() { rm -f -- "$link_tmp"; }
trap cleanup_link EXIT INT TERM
ln -s "$previous_target" "$link_tmp"
mv -Tf "$link_tmp" "$CURRENT_LINK"

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

previous_tmp="/opt/wangzhe/.previous-rollback-$$"
ln -s "$current_target" "$previous_tmp"
mv -Tf "$previous_tmp" "$PREVIOUS_LINK"
systemctl reload nginx.service
echo "代码回滚成功：$previous_target"
if (( maintenance_was_active == 1 )); then
  echo "回滚前已有维护标记，已按原样保留。"
else
  echo "本次回滚创建的维护标记已保留。"
fi
echo "维护模式仍然开启；请先运行完整 production-readiness.sh 门禁，通过后再人工删除 $MAINTENANCE_FLAG。"
echo "数据库未回退；如需恢复数据库，必须进入维护窗口并人工执行已验证备份。"
