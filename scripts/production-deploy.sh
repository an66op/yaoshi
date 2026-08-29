#!/usr/bin/env bash
set -euo pipefail
export PATH=/usr/sbin:/usr/bin:/sbin:/bin

usage() {
  cat <<'EOF'
Usage: sudo EXPECTED_MANIFEST_SHA256=<sha256> RELEASE_ID=20260829-1 \
  /usr/local/sbin/wangzhe-production-deploy RELEASE_DIR

Optional environment:
  EXPECTED_MANIFEST_SHA256=<digest copied through a trusted channel>
  APP_ENV=/etc/wangzhe/backend.env
  BACKUP_ENV=/etc/wangzhe/backup.env
  BACKUP_DIR=/var/backups/wangzhe
  READY_TIMEOUT_SECONDS=90

The script always validates configuration and Nginx, creates and verifies a
database backup, stages an immutable release, switches the current symlink,
waits for migrations/readiness, and runs the full production gate. It never
automatically reverses database migrations.
EOF
}

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  usage
  exit 0
fi
[[ $# -eq 1 ]] || { usage >&2; exit 2; }
(( EUID == 0 )) || { echo "必须以 root 运行生产发布脚本" >&2; exit 1; }

for command_name in awk cat chmod chown cp curl date dirname env find flock grep id install ln mktemp mv nginx readlink rm runuser sha256sum sleep stat systemctl tr uname; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done

# This entrypoint is a server-side trust anchor. Never execute the copy carried
# inside an uploaded release archive as root: an attacker who replaced the
# archive could replace its verifier as well.
TRUSTED_SCRIPT_PATH="$(readlink -f "${BASH_SOURCE[0]}")"
TRUSTED_SCRIPT_DIR="$(cd "$(dirname "$TRUSTED_SCRIPT_PATH")" && pwd -P)"
[[ "$TRUSTED_SCRIPT_DIR" == "/usr/local/libexec/wangzhe" ]] || {
  echo "请使用服务器预装的 /usr/local/sbin/wangzhe-production-deploy，禁止直接执行发布包内脚本" >&2
  exit 1
}
for trusted_dir in /usr/local /usr/local/libexec "$TRUSTED_SCRIPT_DIR" "$TRUSTED_SCRIPT_DIR/lib"; do
  [[ -d "$trusted_dir" && ! -L "$trusted_dir" && "$(stat -c '%u' "$trusted_dir")" == "0" && -z "$(find "$trusted_dir" -maxdepth 0 -perm /022 -print -quit)" ]] || {
    echo "可信部署工具路径必须属于 root 且不能允许非 root 写入：$trusted_dir" >&2
    exit 1
  }
done
for trusted_file in \
  production-deploy.sh production-rollback.sh release-integrity.sh production-config-check.sh production-readiness.sh postgres-backup.sh \
  lib/backend-env.sh lib/safe-integer.sh lib/maintenance-edge.sh; do
  trusted_path="$TRUSTED_SCRIPT_DIR/$trusted_file"
  [[ -f "$trusted_path" && ! -L "$trusted_path" && "$(stat -c '%u' "$trusted_path")" == "0" ]] || {
    echo "可信部署工具无效或不属于 root：$trusted_path" >&2
    exit 1
  }
  [[ -z "$(find "$trusted_path" -perm /022 -print -quit)" ]] || { echo "可信部署工具允许非 root 写入：$trusted_path" >&2; exit 1; }
done

SOURCE_DIR="$(cd "$1" && pwd -P)"
EXPECTED_MANIFEST_SHA256="${EXPECTED_MANIFEST_SHA256:-}"
APP_ENV="${APP_ENV:-/etc/wangzhe/backend.env}"
BACKUP_ENV="${BACKUP_ENV:-/etc/wangzhe/backup.env}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/wangzhe}"
READY_TIMEOUT_SECONDS="${READY_TIMEOUT_SECONDS:-90}"
PUBLIC_URL="${PUBLIC_URL:-https://wz6688.app}"
ADMIN_URL="${ADMIN_URL:-https://admin.wz6688.app}"
RELEASE_ID="${RELEASE_ID:-$(date -u +%Y%m%d-%H%M%S)}"
RELEASE_ROOT=/opt/wangzhe/releases
CURRENT_LINK=/opt/wangzhe/current
PREVIOUS_LINK=/opt/wangzhe/previous
MAINTENANCE_FLAG=/etc/wangzhe/maintenance

[[ "$EXPECTED_MANIFEST_SHA256" =~ ^[0-9a-f]{64}$ ]] || { echo "必须通过可信通道提供小写 EXPECTED_MANIFEST_SHA256" >&2; exit 1; }
[[ "$RELEASE_ID" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$ ]] || { echo "RELEASE_ID 格式不安全" >&2; exit 1; }
[[ "$READY_TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] && (( READY_TIMEOUT_SECONDS >= 10 && READY_TIMEOUT_SECONDS <= 600 )) || {
  echo "READY_TIMEOUT_SECONDS 必须在 10-600 之间" >&2
  exit 1
}
case "$BACKUP_DIR" in
  /|/opt|/var|/var/backups) echo "备份目录范围过宽：$BACKUP_DIR" >&2; exit 1 ;;
esac
[[ "$BACKUP_DIR" == /* ]] || { echo "BACKUP_DIR 必须是绝对路径" >&2; exit 1; }
validate_secure_source_path() {
  local candidate="$1" mode mode_value
  while :; do
    [[ -d "$candidate" && ! -L "$candidate" && "$(stat -c '%u' "$candidate")" == "0" ]] || {
      echo "发布目录及父路径必须是 root 所有的真实目录：$candidate" >&2
      return 1
    }
    mode="$(stat -c '%a' "$candidate")"
    [[ "$mode" =~ ^[0-7]{3,4}$ ]] || { echo "无法确认发布路径权限：$candidate" >&2; return 1; }
    mode_value=$((8#$mode))
    # Root-owned sticky directories such as /tmp safely prevent another user
    # from replacing a root-owned child entry; other writable parents do not.
    (( (mode_value & 022) == 0 || (mode_value & 01000) != 0 )) || {
      echo "发布路径父目录允许非 root 替换目录项：$candidate" >&2
      return 1
    }
    [[ "$candidate" == "/" ]] && break
    candidate="$(dirname "$candidate")"
  done
}
validate_secure_source_path "$SOURCE_DIR"
[[ -z "$(find "$SOURCE_DIR" -maxdepth 0 -perm /022 -print -quit)" ]] || {
  echo "发布目录本身不能允许非 root 写入，即使父目录带 sticky bit" >&2
  exit 1
}
non_root_owned="$(find "$SOURCE_DIR" -mindepth 1 ! -user root -print -quit)"
[[ -z "$non_root_owned" ]] || { echo "发布包内所有条目都必须属于 root：$non_root_owned" >&2; exit 1; }
unsafe_writable="$(find "$SOURCE_DIR" -mindepth 1 -perm /022 -print -quit)"
[[ -z "$unsafe_writable" ]] || { echo "发布包不能允许组或其他用户写入：$unsafe_writable" >&2; exit 1; }
[[ -f "$SOURCE_DIR/SHA256SUMS" && ! -L "$SOURCE_DIR/SHA256SUMS" ]] || { echo "发布包缺少 SHA256SUMS" >&2; exit 1; }
manifest_digest="$(sha256sum "$SOURCE_DIR/SHA256SUMS" | awk '{print $1}')"
[[ "$manifest_digest" == "$EXPECTED_MANIFEST_SHA256" ]] || {
  echo "发布清单与可信摘要不一致，拒绝以 root 读取或执行发布包内容" >&2
  exit 1
}

for required_file in \
  TARGET \
  bin/wangzhe-backend \
  bin/wangzhe-bootstrap-admin \
  member/index.html \
  admin/index.html \
  scripts/production-config-check.sh \
  scripts/release-integrity.sh \
  scripts/production-readiness.sh \
  scripts/production-rollback.sh \
  scripts/postgres-backup.sh \
  scripts/lib/backend-env.sh \
  scripts/lib/safe-integer.sh \
  scripts/lib/maintenance-edge.sh; do
  [[ -f "$SOURCE_DIR/$required_file" ]] || { echo "发布包缺少 $required_file" >&2; exit 1; }
done
for executable in \
  bin/wangzhe-backend \
  bin/wangzhe-bootstrap-admin \
  scripts/production-config-check.sh \
  scripts/release-integrity.sh \
  scripts/production-readiness.sh \
  scripts/production-rollback.sh \
  scripts/postgres-backup.sh; do
  [[ -x "$SOURCE_DIR/$executable" ]] || { echo "发布文件不可执行：$executable" >&2; exit 1; }
done
"$TRUSTED_SCRIPT_DIR/release-integrity.sh" verify "$SOURCE_DIR"
read -r release_target <"$SOURCE_DIR/TARGET"
case "$(uname -m)" in
  x86_64) host_target=linux/amd64 ;;
  aarch64|arm64) host_target=linux/arm64 ;;
  *) echo "不支持的服务器架构：$(uname -m)" >&2; exit 1 ;;
esac
[[ "$release_target" == "$host_target" ]] || {
  echo "发布包目标为 $release_target，当前服务器需要 $host_target" >&2
  exit 1
}
[[ -f "$APP_ENV" && ! -L "$APP_ENV" ]] || { echo "应用环境文件无效：$APP_ENV" >&2; exit 1; }
[[ -f "$BACKUP_ENV" && ! -L "$BACKUP_ENV" ]] || { echo "备份环境文件无效：$BACKUP_ENV" >&2; exit 1; }
id wangzhe >/dev/null 2>&1 || { echo "缺少系统用户 wangzhe" >&2; exit 1; }
id wangzhe-backup >/dev/null 2>&1 || { echo "缺少系统用户 wangzhe-backup" >&2; exit 1; }

exec 9>/run/lock/wangzhe-deploy.lock
flock -n 9 || { echo "另一个发布或回滚仍在进行" >&2; exit 1; }

# Capture ownership before any deployment operation. A marker that already
# exists belongs to an operator or an earlier failed run and must survive this
# command even when the new release succeeds.
# shellcheck source=lib/maintenance-edge.sh
source "$TRUSTED_SCRIPT_DIR/lib/maintenance-edge.sh"
capture_maintenance_marker_state "$MAINTENANCE_FLAG"

"$TRUSTED_SCRIPT_DIR/production-config-check.sh" "$APP_ENV"
# shellcheck source=lib/backend-env.sh
source "$TRUSTED_SCRIPT_DIR/lib/backend-env.sh"
load_backend_env "$APP_ENV"
nginx -t
systemctl cat wangzhe-backend.service >/dev/null

target="$RELEASE_ROOT/$RELEASE_ID"
[[ ! -e "$target" ]] || { echo "发布版本已存在，拒绝覆盖：$target" >&2; exit 1; }
if [[ -e "$CURRENT_LINK" && ! -L "$CURRENT_LINK" ]]; then
  echo "$CURRENT_LINK 必须是符号链接，拒绝覆盖真实目录" >&2
  exit 1
fi
if [[ -e "$PREVIOUS_LINK" && ! -L "$PREVIOUS_LINK" ]]; then
  echo "$PREVIOUS_LINK 必须是符号链接，拒绝覆盖真实目录" >&2
  exit 1
fi

validate_installed_release() {
  local candidate="$1" bad_owner bad_mode candidate_target
  [[ "$candidate" == "$RELEASE_ROOT/"* && -d "$candidate" ]] || return 1
  [[ "$(stat -c '%u' "$candidate")" == "0" ]] || return 1
  bad_owner="$(find "$candidate" -mindepth 1 ! -user root -print -quit)"
  [[ -z "$bad_owner" ]] || return 1
  bad_mode="$(find "$candidate" -mindepth 1 -perm /022 -print -quit)"
  [[ -z "$bad_mode" ]] || return 1
  "$TRUSTED_SCRIPT_DIR/release-integrity.sh" verify "$candidate" >/dev/null || return 1
  read -r candidate_target <"$candidate/TARGET"
  [[ "$candidate_target" == "$host_target" ]]
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

previous_target=""
if [[ -L "$CURRENT_LINK" ]]; then
  previous_target="$(readlink -f "$CURRENT_LINK")"
  validate_installed_release "$previous_target" || {
    echo "当前版本不在受控目录内、被修改或完整性校验失败：$previous_target" >&2
    exit 1
  }
fi

echo "发布前创建并验证数据库备份"
runuser --user wangzhe-backup -- \
  env -i PATH=/usr/bin:/bin HOME=/var/backups/wangzhe \
  BACKUP_DIR="$BACKUP_DIR" BACKUP_RETENTION_DAYS=14 \
  APPLICATION_DATABASE_USER="$BACKEND_DATABASE_USER" \
  "$TRUSTED_SCRIPT_DIR/postgres-backup.sh" "$BACKUP_ENV"

install -d -o root -g root -m 0755 /opt/wangzhe "$RELEASE_ROOT"
staging="$RELEASE_ROOT/.staging-$RELEASE_ID-$$"
[[ ! -e "$staging" ]] || { echo "临时发布目录已存在：$staging" >&2; exit 1; }
cleanup_staging() {
  if [[ -n "${staging:-}" && "$staging" == "$RELEASE_ROOT/.staging-"* ]]; then
    rm -rf -- "$staging"
  fi
}
trap cleanup_staging EXIT INT TERM
install -d -o root -g root -m 0755 "$staging"
cp -a "$SOURCE_DIR/." "$staging/"
chown -R root:root "$staging"
chmod 0755 "$staging/bin/wangzhe-backend" "$staging/bin/wangzhe-bootstrap-admin"
find "$staging/member" "$staging/admin" -type d -exec chmod 0755 {} +
find "$staging/member" "$staging/admin" -type f -exec chmod 0644 {} +
find "$staging/scripts" -type f -name '*.sh' -exec chmod 0755 {} +
staging_manifest_digest="$(sha256sum "$staging/SHA256SUMS" | awk '{print $1}')"
[[ "$staging_manifest_digest" == "$EXPECTED_MANIFEST_SHA256" ]] || {
  echo "复制后的发布清单与外部可信摘要不一致" >&2
  exit 1
}
"$TRUSTED_SCRIPT_DIR/release-integrity.sh" verify "$staging"
mv -T "$staging" "$target"
staging=""
trap - EXIT INT TERM
if ! validate_installed_release "$target"; then
  rm -rf -- "$target"
  echo "安装后的发布版本校验失败，已移除未启用版本：$target" >&2
  exit 1
fi

if [[ -n "$previous_target" ]]; then
  previous_tmp="/opt/wangzhe/.previous-$RELEASE_ID-$$"
  ln -s "$previous_target" "$previous_tmp"
  mv -Tf "$previous_tmp" "$PREVIOUS_LINK"
fi

# The two HTTPS vhosts return 503 while this root-owned marker exists. It is
# created before the symlink switch and removed only after the full production
# gate succeeds, so a failed candidate never becomes externally reachable.
if ! ensure_maintenance_marker "$MAINTENANCE_FLAG"; then
  echo "无法安全进入维护模式，未切换任何版本" >&2
  exit 1
fi
if ! verify_maintenance_edge "$PUBLIC_URL" "$ADMIN_URL"; then
  echo "当前 Nginx 未可靠进入维护模式；保留维护标记且未切换任何版本" >&2
  exit 1
fi
link_tmp="/opt/wangzhe/.current-$RELEASE_ID-$$"
cleanup_link() { rm -f -- "$link_tmp"; }
trap cleanup_link EXIT INT TERM
ln -s "$target" "$link_tmp"
mv -Tf "$link_tmp" "$CURRENT_LINK"

systemctl reset-failed wangzhe-backend.service || true
if ! systemctl restart wangzhe-backend.service; then
  systemctl stop wangzhe-backend.service || true
  echo "新版本启动失败。它可能已经提交迁移，因此保持维护模式、新版本链接并停止服务，不自动启动旧代码。" >&2
  [[ -n "$previous_target" ]] && echo "上一版仍保存在 $PREVIOUS_LINK；核对迁移兼容性后再人工回滚。" >&2
  exit 1
fi

if ! wait_for_backend; then
  systemctl stop wangzhe-backend.service || true
  echo "新版本在 ${READY_TIMEOUT_SECONDS}s 内未就绪，请检查 journalctl -u wangzhe-backend" >&2
  echo "服务可能已经提交迁移；保持维护模式、新版本链接且已停服，不自动启动旧代码。" >&2
  exit 1
fi

if ! env -i PATH=/usr/bin:/bin HOME=/root \
  BACKEND_URL="http://${BACKEND_SERVER_BIND}:${BACKEND_SERVER_PORT}" BACKUP_DIR="$BACKUP_DIR" ALLOW_MAINTENANCE_503=1 \
  "$TRUSTED_SCRIPT_DIR/production-readiness.sh" "$APP_ENV"; then
  systemctl stop wangzhe-backend.service || true
  echo "完整生产门禁失败；保持维护模式并停止新版本，未盲目切换旧代码" >&2
  [[ -n "$previous_target" ]] && echo "修复外部条件后重跑门禁，或人工执行受验证的 production-rollback.sh" >&2
  exit 1
fi

systemctl reload nginx.service
if ! finish_maintenance_marker "$MAINTENANCE_FLAG"; then
  echo "发布门禁已通过，但维护标记所有权发生变化；为安全起见继续保持维护模式" >&2
  exit 1
fi
echo "发布成功：$RELEASE_ID"
echo "当前版本：$target"
[[ -n "$previous_target" ]] && echo "可回滚版本：$previous_target"
if (( maintenance_was_active == 1 )); then
  echo "发布前已有维护标记，已按原样保留。"
fi
