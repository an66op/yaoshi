#!/usr/bin/env bash
set -euo pipefail
export PATH=/usr/sbin:/usr/bin:/sbin:/bin

usage() {
  cat <<'EOF'
Usage: sudo EXPECTED_MANIFEST_SHA256=<sha256> RELEASE_ID=20260829-1 \
  /usr/local/sbin/wangzhe-production-deploy RELEASE_DIR

First install into a production database with no users:
  sudo EXPECTED_MANIFEST_SHA256=<sha256> RELEASE_ID=20260829-1 \
    /usr/local/sbin/wangzhe-production-deploy --first-install \
    --first-admin-username platform-owner \
    --first-admin-password-file /run/wangzhe-bootstrap-admin/password \
    RELEASE_DIR

Optional environment:
  EXPECTED_MANIFEST_SHA256=<digest copied through a trusted channel>
  APP_ENV=/etc/wangzhe/backend.env
  BACKUP_ENV=/etc/wangzhe/backup.env
  BACKUP_CRYPTO_ENV=/etc/wangzhe/backup-crypto.env
  PITR_ENV=/etc/wangzhe/pitr.env
  BACKUP_DIR=/var/backups/wangzhe/database
  PITR_CLUSTER_ID_FILE=/etc/wangzhe/pitr-cluster-id
  READY_TIMEOUT_SECONDS=90

The script validates configuration and Nginx, creates and verifies backups,
stages an immutable release, switches the current symlink, waits for
migrations/readiness, and runs the full production gate. A truly empty
database uses a two-phase first install: phase 1 bootstraps under maintenance,
creates offsite-verified recovery inputs, and stops without switching current;
after both isolated recovery drills publish real evidence, a normal deployment
of the exact same authenticated package (the same EXPECTED_MANIFEST_SHA256)
with only a new RELEASE_ID performs phase 2 and may expose the service. The
script never automatically reverses database migrations.

The first-install flags are accepted only when no current release exists and
the database contains no users. The password itself is never accepted in an
argument or environment variable; systemd copies the protected one-line file
into a private service credential for the one-shot administrator bootstrap.
EOF
}

FIRST_INSTALL=0
FIRST_ADMIN_USERNAME=""
FIRST_ADMIN_PASSWORD_FILE=""
source_argument=""
while (( $# > 0 )); do
  case "$1" in
    --help|-h)
      usage
      exit 0
      ;;
    --first-install)
      (( FIRST_INSTALL == 0 )) || { echo "--first-install 不能重复" >&2; exit 2; }
      FIRST_INSTALL=1
      shift
      ;;
    --first-admin-username)
      [[ $# -ge 2 && -z "$FIRST_ADMIN_USERNAME" ]] || { echo "--first-admin-username 缺少值或重复" >&2; exit 2; }
      FIRST_ADMIN_USERNAME="$2"
      shift 2
      ;;
    --first-admin-password-file)
      [[ $# -ge 2 && -z "$FIRST_ADMIN_PASSWORD_FILE" ]] || { echo "--first-admin-password-file 缺少值或重复" >&2; exit 2; }
      FIRST_ADMIN_PASSWORD_FILE="$2"
      shift 2
      ;;
    --*)
      echo "未知选项：$1" >&2
      usage >&2
      exit 2
      ;;
    *)
      [[ -z "$source_argument" ]] || { echo "只能提供一个 RELEASE_DIR" >&2; exit 2; }
      source_argument="$1"
      shift
      ;;
  esac
done
[[ -n "$source_argument" ]] || { usage >&2; exit 2; }
if (( FIRST_INSTALL == 1 )); then
  [[ -n "$FIRST_ADMIN_USERNAME" && -n "$FIRST_ADMIN_PASSWORD_FILE" ]] || {
    echo "--first-install 必须同时提供首位管理员账号和受保护的密码文件" >&2
    exit 2
  }
else
  [[ -z "$FIRST_ADMIN_USERNAME" && -z "$FIRST_ADMIN_PASSWORD_FILE" ]] || {
    echo "管理员引导参数只能与 --first-install 一起使用" >&2
    exit 2
  }
fi
(( EUID == 0 )) || { echo "必须以 root 运行生产发布脚本" >&2; exit 1; }

for command_name in awk cat chmod chown cp curl date dirname env find flock grep id install ln mktemp mv nginx psql readlink rm runuser sha256sum sleep stat systemctl systemd-run tail timeout tr uname; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done

first_admin_password_cleanup_armed=0
first_admin_password_identity=""
bootstrap_unit=""
bootstrap_unit_cleanup_armed=0
cleanup_first_admin_password() {
  (( first_admin_password_cleanup_armed == 1 )) || return 0
  local current_identity=""
  if [[ -f "$FIRST_ADMIN_PASSWORD_FILE" && ! -L "$FIRST_ADMIN_PASSWORD_FILE" ]]; then
    current_identity="$(stat -c '%d:%i' "$FIRST_ADMIN_PASSWORD_FILE" 2>/dev/null || true)"
    if [[ "$current_identity" == "$first_admin_password_identity" ]]; then
      rm -f -- "$FIRST_ADMIN_PASSWORD_FILE" || true
    else
      echo "警告：首位管理员密码文件身份已变化，未删除替代文件" >&2
    fi
  fi
  first_admin_password_cleanup_armed=0
}

cleanup_bootstrap_unit() {
  (( bootstrap_unit_cleanup_armed == 1 )) || return 0
  # Once EXIT cleanup starts, a repeated terminal signal must not interrupt the
  # stop/wait sequence and leave the credential-bearing transient service alive.
  trap '' INT TERM
  local unit_name active_state load_state state_output stop_attempt
  unit_name="${bootstrap_unit}.service"
  if [[ ! "$bootstrap_unit" =~ ^wangzhe-bootstrap-admin-[0-9]+$ ]]; then
    echo "WANGZHE_BOOTSTRAP_UNIT_CLEANUP_FAILED unit=invalid load_state=unknown active_state=unknown" >&2
    # An armed but unrecognised name is a lifecycle invariant violation. Do
    # not claim cleanup completed or risk targeting an unrelated unit.
    return 1
  fi

  systemctl stop "$unit_name" >/dev/null 2>&1 || true
  # systemctl stop normally waits for the stop job. Keep an explicit state wait
  # as a second barrier before the deploy process (and its credential source)
  # is allowed to disappear.
  for ((stop_attempt = 0; stop_attempt < 30; stop_attempt++)); do
    state_output="$(systemctl show "$unit_name" --property=LoadState --property=ActiveState 2>/dev/null || true)"
    load_state="$(awk -F= '$1 == "LoadState" { print $2 }' <<<"$state_output")"
    active_state="$(awk -F= '$1 == "ActiveState" { print $2 }' <<<"$state_output")"
    if [[ "$load_state" == not-found || "$active_state" == inactive || "$active_state" == failed ]]; then
        bootstrap_unit_cleanup_armed=0
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
      bootstrap_unit_cleanup_armed=0
      return 0
    fi
    sleep 1
  done

  echo "WANGZHE_BOOTSTRAP_UNIT_CLEANUP_FAILED unit=$unit_name load_state=${load_state:-unknown} active_state=${active_state:-unknown}" >&2
  # Keep the cleanup armed so callers and logs can distinguish an unconfirmed
  # credential lifecycle from a completed cleanup.
  return 1
}

cleanup_deploy_exit() {
  local original_status=$? cleanup_failed=0
  trap - EXIT
  trap '' INT TERM
  cleanup_bootstrap_unit || cleanup_failed=1
  if declare -F cleanup_link >/dev/null 2>&1; then
    cleanup_link || cleanup_failed=1
  fi
  if declare -F cleanup_staging >/dev/null 2>&1; then
    cleanup_staging || cleanup_failed=1
  fi
  if declare -F cleanup_phase_marker_tmp >/dev/null 2>&1; then
    cleanup_phase_marker_tmp || cleanup_failed=1
  fi
  cleanup_first_admin_password || cleanup_failed=1
  if (( cleanup_failed == 1 )); then
    echo "WANGZHE_DEPLOY_CLEANUP_FAILED original_status=$original_status" >&2
    (( original_status != 0 )) || original_status=1
  fi
  exit "$original_status"
}
# Signal handlers must terminate the deployment. Resource cleanup remains in
# the EXIT trap; a handler which merely cleans and returns could let the
# release continue if a signal arrives between external commands.
trap 'exit 130' INT
trap 'exit 143' TERM
trap cleanup_deploy_exit EXIT

if (( FIRST_INSTALL == 1 )); then
  [[ "$FIRST_ADMIN_USERNAME" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{2,49}$ ]] || {
    echo "首位管理员账号必须是 3-50 位字母、数字、下划线、中划线或点" >&2
    exit 1
  }
  [[ "$FIRST_ADMIN_PASSWORD_FILE" == /run/wangzhe-bootstrap-admin/password ]] || {
    echo "首位管理员密码文件必须固定为 /run/wangzhe-bootstrap-admin/password" >&2
    exit 1
  }
  for protected_dir in /run /run/wangzhe-bootstrap-admin; do
    [[ -d "$protected_dir" && ! -L "$protected_dir" && "$(stat -c '%u' "$protected_dir")" == 0 && -z "$(find "$protected_dir" -maxdepth 0 -perm /022 -print -quit)" ]] || {
      echo "首位管理员密码目录必须属于 root 且不可被非 root 写入：$protected_dir" >&2
      exit 1
    }
  done
  [[ -f "$FIRST_ADMIN_PASSWORD_FILE" && ! -L "$FIRST_ADMIN_PASSWORD_FILE" && "$(stat -c '%u' "$FIRST_ADMIN_PASSWORD_FILE")" == 0 && "$(stat -c '%h' "$FIRST_ADMIN_PASSWORD_FILE")" == 1 && -z "$(find "$FIRST_ADMIN_PASSWORD_FILE" -perm /077 -print -quit)" ]] || {
    echo "首位管理员密码必须是 root 所有、owner-only、无硬链接的普通文件" >&2
    exit 1
  }
  first_admin_password_identity="$(stat -c '%d:%i' "$FIRST_ADMIN_PASSWORD_FILE")"
  first_admin_password_cleanup_armed=1
fi

systemd_version="$(systemctl --version | awk 'NR == 1 && $1 == "systemd" && $2 ~ /^[0-9]+$/ { print $2 }')"
[[ "$systemd_version" =~ ^[0-9]+$ ]] || { echo "无法确认 systemd 版本，拒绝生产发布" >&2; exit 1; }
systemd_version_decimal=$((10#$systemd_version))
(( systemd_version_decimal >= 249 )) || {
  echo "systemd ${systemd_version} 低于生产所需的 249（恢复演练状态发布依赖 OnSuccess=）" >&2
  exit 1
}

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
  production-deploy.sh production-rollback.sh production-encryption-rewrap.sh release-integrity.sh production-config-check.sh production-readiness.sh postgres-backup.sh upload-backup.sh redis-production-check.sh \
  postgres-archive-wal.sh postgres-base-backup.sh production-monitor.sh production-backup-integrity.sh production-recovery-evidence-check.sh \
  lib/backend-env.sh lib/safe-integer.sh lib/maintenance-edge.sh lib/strict-env.sh lib/encrypted-backup.sh lib/encryption-capabilities.sh; do
  trusted_path="$TRUSTED_SCRIPT_DIR/$trusted_file"
  [[ -f "$trusted_path" && ! -L "$trusted_path" && "$(stat -c '%u' "$trusted_path")" == "0" ]] || {
    echo "可信部署工具无效或不属于 root：$trusted_path" >&2
    exit 1
  }
  [[ -z "$(find "$trusted_path" -perm /022 -print -quit)" ]] || { echo "可信部署工具允许非 root 写入：$trusted_path" >&2; exit 1; }
done
# shellcheck source=lib/encryption-capabilities.sh
source "$TRUSTED_SCRIPT_DIR/lib/encryption-capabilities.sh"

SOURCE_DIR="$(cd "$source_argument" && pwd -P)"
EXPECTED_MANIFEST_SHA256="${EXPECTED_MANIFEST_SHA256:-}"
APP_ENV="${APP_ENV:-/etc/wangzhe/backend.env}"
BACKUP_ENV="${BACKUP_ENV:-/etc/wangzhe/backup.env}"
BACKUP_CRYPTO_ENV="${BACKUP_CRYPTO_ENV:-/etc/wangzhe/backup-crypto.env}"
PITR_ENV="${PITR_ENV:-/etc/wangzhe/pitr.env}"
BACKUP_INTEGRITY_ENV=/etc/wangzhe/monitor.env
BACKUP_DIR="${BACKUP_DIR:-/var/backups/wangzhe/database}"
PITR_CLUSTER_ID_FILE="${PITR_CLUSTER_ID_FILE:-/etc/wangzhe/pitr-cluster-id}"
READY_TIMEOUT_SECONDS="${READY_TIMEOUT_SECONDS:-90}"
PUBLIC_URL="${PUBLIC_URL:-https://wz6688.app}"
PUBLIC_WWW_URL="${PUBLIC_WWW_URL:-https://www.wz6688.app}"
ADMIN_URL="${ADMIN_URL:-https://admin.wz888.site}"
RELEASE_ID="${RELEASE_ID:-$(date -u +%Y%m%d-%H%M%S)}"
RELEASE_ROOT=/opt/wangzhe/releases
CURRENT_LINK=/opt/wangzhe/current
PREVIOUS_LINK=/opt/wangzhe/previous
MAINTENANCE_FLAG=/etc/wangzhe/maintenance
FIRST_INSTALL_PHASE_MARKER_PREFIX=wangzhe-first-install-phase1:v2
BACKUP_INTEGRITY_STATUS_FILE=/var/lib/wangzhe-monitor/last-backup-integrity.status

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
[[ -f "$PITR_CLUSTER_ID_FILE" && ! -L "$PITR_CLUSTER_ID_FILE" && "$(stat -c '%u' "$PITR_CLUSTER_ID_FILE")" == 0 && -z "$(find "$PITR_CLUSTER_ID_FILE" -perm /022 -print -quit)" ]] || {
  echo "PITR 集群标识文件必须是 root 所有且不可被非 root 修改的普通文件" >&2
  exit 1
}
read -r pitr_cluster_id pitr_cluster_extra <"$PITR_CLUSTER_ID_FILE" || { echo "无法读取 PITR 集群标识" >&2; exit 1; }
[[ "$pitr_cluster_id" =~ ^[0-9]{10,30}$ && -z "${pitr_cluster_extra:-}" ]] || { echo "PITR 集群标识必须是 PostgreSQL system identifier" >&2; exit 1; }
PITR_WAL_DIR="/var/backups/wangzhe/wal/$pitr_cluster_id"
PITR_BASEBACKUP_DIR="/var/backups/wangzhe/base/$pitr_cluster_id"
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
  FIELD_ENCRYPTION_CAPABILITIES \
  bin/wangzhe-backend \
  bin/wangzhe-bootstrap-admin \
  bin/wangzhe-field-encryption-audit \
  member/index.html \
  admin/index.html \
  scripts/production-config-check.sh \
  scripts/release-integrity.sh \
  scripts/production-readiness.sh \
  scripts/production-rollback.sh \
  scripts/production-encryption-rewrap.sh \
  scripts/postgres-backup.sh \
  scripts/upload-backup.sh \
  scripts/postgres-archive-wal.sh \
  scripts/postgres-base-backup.sh \
  scripts/postgres-restore-wal.sh \
  scripts/pitr-recovery-source-sync.sh \
  scripts/production-restore-drill.sh \
  scripts/production-pitr-restore-drill.sh \
  scripts/publish-pitr-drill-status.sh \
  scripts/production-unit-failure-alert.sh \
  scripts/production-monitor.sh \
  scripts/production-backup-integrity.sh \
  scripts/production-recovery-evidence-check.sh \
  scripts/redis-production-check.sh \
  scripts/lib/backend-env.sh \
  scripts/lib/safe-integer.sh \
  scripts/lib/maintenance-edge.sh \
  scripts/lib/strict-env.sh \
  scripts/lib/encrypted-backup.sh \
  scripts/lib/encryption-capabilities.sh; do
  [[ -f "$SOURCE_DIR/$required_file" ]] || { echo "发布包缺少 $required_file" >&2; exit 1; }
done
for executable in \
  bin/wangzhe-backend \
  bin/wangzhe-bootstrap-admin \
  bin/wangzhe-field-encryption-audit \
  scripts/production-config-check.sh \
  scripts/release-integrity.sh \
  scripts/production-readiness.sh \
  scripts/production-rollback.sh \
  scripts/production-encryption-rewrap.sh \
  scripts/postgres-backup.sh \
  scripts/upload-backup.sh \
  scripts/postgres-archive-wal.sh \
  scripts/postgres-base-backup.sh \
  scripts/postgres-restore-wal.sh \
  scripts/pitr-recovery-source-sync.sh \
  scripts/production-restore-drill.sh \
  scripts/production-pitr-restore-drill.sh \
  scripts/publish-pitr-drill-status.sh \
  scripts/production-unit-failure-alert.sh \
  scripts/production-monitor.sh \
  scripts/production-backup-integrity.sh \
  scripts/production-recovery-evidence-check.sh \
  scripts/redis-production-check.sh; do
  [[ -x "$SOURCE_DIR/$executable" ]] || { echo "发布文件不可执行：$executable" >&2; exit 1; }
done
"$TRUSTED_SCRIPT_DIR/release-integrity.sh" verify "$SOURCE_DIR"
load_release_encryption_capabilities "$SOURCE_DIR"

# A cryptographically intact package can still be an obsolete build.  The
# Bingo Mark Six rollout spans both the backend rule contract and the member
# board, so refuse a package that contains only one side (or neither side).
validate_member_betting_contract() {
  local candidate="$1"
  grep -aFq 'mark6-v2' "$candidate/bin/wangzhe-backend" || {
    echo "发布包后端缺少宾果六合彩当前 mark6-v2 规则" >&2
    return 1
  }
  grep -aFRq 'mark-six-bet-board' "$candidate/member/assets" || {
    echo "发布包会员端缺少宾果六合彩网投面板" >&2
    return 1
  }
  grep -aFRq 'web-bets' "$candidate/member/assets" || {
    echo "发布包会员端缺少批量网投接口" >&2
    return 1
  }
}
validate_member_betting_contract "$SOURCE_DIR"
read -r release_target <"$SOURCE_DIR/TARGET"
case "$(uname -m)" in
  x86_64) host_target=linux/amd64 ;;
  aarch64|arm64) host_target=linux/arm64 ;;
  *) echo "不支持的服务器架构：$(uname -m)" >&2; exit 1 ;;
esac
[[ "$release_target" == "$host_target" ]] || {
  echo "发布包目标为 ${release_target}，当前服务器需要 $host_target" >&2
  exit 1
}
[[ -f "$APP_ENV" && ! -L "$APP_ENV" ]] || { echo "应用环境文件无效：$APP_ENV" >&2; exit 1; }
[[ -f "$BACKUP_ENV" && ! -L "$BACKUP_ENV" ]] || { echo "备份环境文件无效：$BACKUP_ENV" >&2; exit 1; }
[[ -f "$BACKUP_CRYPTO_ENV" && ! -L "$BACKUP_CRYPTO_ENV" ]] || { echo "备份加密环境文件无效：$BACKUP_CRYPTO_ENV" >&2; exit 1; }
[[ -f "$BACKUP_INTEGRITY_ENV" && ! -L "$BACKUP_INTEGRITY_ENV" ]] || { echo "备份完整性环境文件无效：$BACKUP_INTEGRITY_ENV" >&2; exit 1; }
[[ -f "$PITR_ENV" && ! -L "$PITR_ENV" ]] || { echo "PITR 环境文件无效：$PITR_ENV" >&2; exit 1; }
id wangzhe >/dev/null 2>&1 || { echo "缺少系统用户 wangzhe" >&2; exit 1; }
id wangzhe-backup >/dev/null 2>&1 || { echo "缺少系统用户 wangzhe-backup" >&2; exit 1; }
id wangzhe-monitor >/dev/null 2>&1 || { echo "缺少系统用户 wangzhe-monitor" >&2; exit 1; }

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
  [[ -x "$candidate/bin/wangzhe-field-encryption-audit" && \
     -x "$candidate/scripts/production-encryption-rewrap.sh" && \
     -f "$candidate/scripts/lib/encryption-capabilities.sh" && \
     ! -L "$candidate/scripts/lib/encryption-capabilities.sh" ]] || return 1
  load_release_encryption_capabilities "$candidate" >/dev/null || return 1
  read -r candidate_target <"$candidate/TARGET"
  [[ "$candidate_target" == "$host_target" ]]
}

phase_one_maintenance_adopted=0
phase_marker_tmp=""
phase_payload_tmp=""
cleanup_phase_marker_tmp() {
  local candidate cleanup_failed=0
  for candidate in "${phase_marker_tmp:-}" "${phase_payload_tmp:-}"; do
    case "$candidate" in
      "") ;;
      "$MAINTENANCE_FLAG".phase1.*) rm -f -- "$candidate" || cleanup_failed=1 ;;
      *)
        echo "拒绝清理无法识别的首次安装阶段临时标记：$candidate" >&2
        cleanup_failed=1
        ;;
    esac
  done
  phase_marker_tmp=""
  phase_payload_tmp=""
  return "$cleanup_failed"
}

phase_marker_field() {
  local file="$1" key="$2"
  awk -F= -v key="$key" '$1 == key { count++; value=substr($0, length(key) + 2) } END { if (count == 1) print value; else exit 1 }' "$file"
}

replace_first_install_phase_marker() {
  local payload_file="$1" payload_sha phase_token
  payload_sha="$(sha256sum "$payload_file" | awk '{print $1}')"
  [[ "$payload_sha" =~ ^[0-9a-f]{64}$ ]] || return 1
  phase_token="${FIRST_INSTALL_PHASE_MARKER_PREFIX}:${payload_sha}"
  phase_marker_tmp="$(mktemp "${MAINTENANCE_FLAG}.phase1.marker.XXXXXX")"
  if ! { printf '%s\n' "$phase_token"; cat "$payload_file"; } >"$phase_marker_tmp" || ! chmod 0644 "$phase_marker_tmp"; then
    cleanup_phase_marker_tmp || true
    return 1
  fi
  # The file is replaced atomically, but only while the exact marker created
  # or previously upgraded by this phase-1 process is still present.
  if ! maintenance_marker_owned_by "$MAINTENANCE_FLAG" "$maintenance_marker_token"; then
    cleanup_phase_marker_tmp || true
    echo "首次安装阶段 1 维护标记已被外部替换；拒绝写入阶段状态" >&2
    return 1
  fi
  if ! mv -f -- "$phase_marker_tmp" "$MAINTENANCE_FLAG"; then
    cleanup_phase_marker_tmp || true
    return 1
  fi
  phase_marker_tmp=""
  rm -f -- "$payload_file"
  phase_payload_tmp=""
  maintenance_marker_token="$phase_token"
  maintenance_marker_created=1
  maintenance_marker_owned_by "$MAINTENANCE_FLAG" "$maintenance_marker_token" || {
    echo "首次安装阶段 1 状态写入后无法验证" >&2
    return 1
  }
}

persist_first_install_pending_marker() {
  local prepared_epoch
  (( FIRST_INSTALL == 1 )) || return 1
  # Initialized by sourced maintenance-edge.sh.
  # shellcheck disable=SC2154
  if (( maintenance_was_active == 1 )); then
    echo "首次安装不接管既有人工维护标记；请由原操作员解除后重新执行阶段 1" >&2
    return 1
  fi
  (( maintenance_marker_created == 1 )) && maintenance_marker_owned_by "$MAINTENANCE_FLAG" "$maintenance_marker_token" || {
    echo "无法确认首次安装阶段 1 对维护标记的所有权；拒绝启动管理员引导" >&2
    return 1
  }
  validate_installed_release "$target" || {
    echo "首次安装阶段 1 候选版本失去完整性；拒绝启动管理员引导" >&2
    return 1
  }
  prepared_epoch="$(date +%s)"
  [[ "$prepared_epoch" =~ ^[1-9][0-9]{0,11}$ ]] || return 1
  phase_payload_tmp="$(mktemp "${MAINTENANCE_FLAG}.phase1.payload.XXXXXX")"
  printf 'schema=wangzhe.first-install-phase1.v2\nstatus=bootstrap-pending\nmanifest_sha256=%s\nrelease_id=%s\nprepared_at_epoch=%s\n' \
    "$EXPECTED_MANIFEST_SHA256" "$RELEASE_ID" "$prepared_epoch" >"$phase_payload_tmp"
  replace_first_install_phase_marker "$phase_payload_tmp"
}

persist_first_install_phase_marker() {
  (( FIRST_INSTALL == 1 )) || return 1
  validate_installed_release "$target" || {
    echo "首次安装阶段 1 候选版本失去完整性；拒绝写入待演练状态" >&2
    return 1
  }
  [[ "${first_install_backup_completed_epoch:-}" =~ ^[1-9][0-9]{0,11}$ && \
     "${first_install_database_artifact_name:-}" =~ ^[A-Za-z][A-Za-z0-9_]{0,62}-[0-9]{8}-[0-9]{6}-[0-9]+\.dump\.age$ && \
     "${first_install_upload_artifact_name:-}" =~ ^uploads-[0-9]{8}-[0-9]{6}-[0-9]+\.tar\.age$ && \
     "${first_install_basebackup_artifact_name:-}" =~ ^basebackup-[0-9]{8}-[0-9]{6}-[0-9]+\.tar\.age$ && \
     "${first_install_database_cipher_sha256:-}" =~ ^[0-9a-f]{64}$ && \
     "${first_install_upload_cipher_sha256:-}" =~ ^[0-9a-f]{64}$ && \
     "${first_install_basebackup_cipher_sha256:-}" =~ ^[0-9a-f]{64}$ && \
     "${first_install_wal_inventory_sha256:-}" =~ ^[0-9a-f]{64}$ ]] || {
    echo "首次安装阶段 1 缺少本次备份制品的严格绑定" >&2
    return 1
  }
  phase_payload_tmp="$(mktemp "${MAINTENANCE_FLAG}.phase1.payload.XXXXXX")"
  printf 'schema=wangzhe.first-install-phase1.v2\nstatus=awaiting-recovery\nmanifest_sha256=%s\nrelease_id=%s\nbackup_completed_at_epoch=%s\ndatabase_artifact_name=%s\ndatabase_cipher_sha256=%s\nupload_artifact_name=%s\nupload_cipher_sha256=%s\nbasebackup_artifact_name=%s\nbasebackup_cipher_sha256=%s\nwal_inventory_sha256=%s\n' \
    "$EXPECTED_MANIFEST_SHA256" "$RELEASE_ID" "$first_install_backup_completed_epoch" \
    "$first_install_database_artifact_name" "$first_install_database_cipher_sha256" \
    "$first_install_upload_artifact_name" "$first_install_upload_cipher_sha256" \
    "$first_install_basebackup_artifact_name" "$first_install_basebackup_cipher_sha256" \
    "$first_install_wal_inventory_sha256" >"$phase_payload_tmp"
  replace_first_install_phase_marker "$phase_payload_tmp"
}

adopt_first_install_phase_marker() {
  local phase_token marker_owner marker_mode marker_links marker_size marker_lines marker_payload_sha actual_payload_sha
  local phase_schema phase_status phase_manifest phase_release_id phase_release actual_manifest
  local phase_backup_epoch phase_database_name phase_database_sha phase_upload_name phase_upload_sha
  local phase_base_name phase_base_sha phase_wal_sha
  (( FIRST_INSTALL == 0 && maintenance_was_active == 1 )) || return 2
  [[ -f "$MAINTENANCE_FLAG" && ! -L "$MAINTENANCE_FLAG" ]] || return 2
  IFS= read -r phase_token <"$MAINTENANCE_FLAG" || return 2
  [[ "$phase_token" == wangzhe-first-install-phase1:* ]] || return 2

  marker_owner="$(stat -c '%u' "$MAINTENANCE_FLAG")"
  marker_mode="$(stat -c '%a' "$MAINTENANCE_FLAG")"
  marker_links="$(stat -c '%h' "$MAINTENANCE_FLAG")"
  marker_size="$(stat -c '%s' "$MAINTENANCE_FLAG")"
  marker_lines="$(awk 'END { print NR }' "$MAINTENANCE_FLAG")"
  [[ "$marker_owner" == 0 && "$marker_mode" == 644 && "$marker_links" == 1 && \
     "$marker_size" =~ ^[0-9]+$ && "$marker_size" -le 2048 && "$marker_lines" -ge 6 && "$marker_lines" -le 13 ]] || {
    echo "首次安装阶段衔接标记的所有者、权限、硬链接或格式无效" >&2
    return 1
  }
  if [[ "$phase_token" =~ ^wangzhe-first-install-phase1:v2:([0-9a-f]{64})$ ]]; then
    marker_payload_sha="${BASH_REMATCH[1]}"
  else
    echo "首次安装阶段衔接标记内容无效" >&2
    return 1
  fi
  actual_payload_sha="$(tail -n +2 "$MAINTENANCE_FLAG" | sha256sum | awk '{print $1}')"
  [[ "$actual_payload_sha" == "$marker_payload_sha" ]] || { echo "首次安装阶段衔接标记摘要无效" >&2; return 1; }
  phase_schema="$(phase_marker_field "$MAINTENANCE_FLAG" schema)" || return 1
  phase_status="$(phase_marker_field "$MAINTENANCE_FLAG" status)" || return 1
  phase_manifest="$(phase_marker_field "$MAINTENANCE_FLAG" manifest_sha256)" || return 1
  phase_release_id="$(phase_marker_field "$MAINTENANCE_FLAG" release_id)" || return 1
  [[ "$phase_schema" == wangzhe.first-install-phase1.v2 && "$phase_manifest" =~ ^[0-9a-f]{64}$ && \
     "$phase_release_id" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$ ]] || { echo "首次安装阶段衔接字段无效" >&2; return 1; }
  if [[ "$phase_status" == bootstrap-pending ]]; then
    echo "首次安装阶段 1 在管理员引导或备份完成前中断；保持维护并拒绝按普通 active-admin 发布" >&2
    return 1
  fi
  [[ "$phase_status" == awaiting-recovery && "$marker_lines" == 13 ]] || {
    echo "首次安装阶段 1 尚未进入可验证的待恢复演练状态" >&2
    return 1
  }
  phase_backup_epoch="$(phase_marker_field "$MAINTENANCE_FLAG" backup_completed_at_epoch)" || return 1
  phase_database_name="$(phase_marker_field "$MAINTENANCE_FLAG" database_artifact_name)" || return 1
  phase_database_sha="$(phase_marker_field "$MAINTENANCE_FLAG" database_cipher_sha256)" || return 1
  phase_upload_name="$(phase_marker_field "$MAINTENANCE_FLAG" upload_artifact_name)" || return 1
  phase_upload_sha="$(phase_marker_field "$MAINTENANCE_FLAG" upload_cipher_sha256)" || return 1
  phase_base_name="$(phase_marker_field "$MAINTENANCE_FLAG" basebackup_artifact_name)" || return 1
  phase_base_sha="$(phase_marker_field "$MAINTENANCE_FLAG" basebackup_cipher_sha256)" || return 1
  phase_wal_sha="$(phase_marker_field "$MAINTENANCE_FLAG" wal_inventory_sha256)" || return 1
  [[ "$phase_backup_epoch" =~ ^[1-9][0-9]{0,11}$ && \
     "$phase_database_name" =~ ^[A-Za-z][A-Za-z0-9_]{0,62}-[0-9]{8}-[0-9]{6}-[0-9]+\.dump\.age$ && \
     "$phase_upload_name" =~ ^uploads-[0-9]{8}-[0-9]{6}-[0-9]+\.tar\.age$ && \
     "$phase_base_name" =~ ^basebackup-[0-9]{8}-[0-9]{6}-[0-9]+\.tar\.age$ && \
     "$phase_database_sha" =~ ^[0-9a-f]{64}$ && "$phase_upload_sha" =~ ^[0-9a-f]{64}$ && \
     "$phase_base_sha" =~ ^[0-9a-f]{64}$ && "$phase_wal_sha" =~ ^[0-9a-f]{64}$ ]] || {
    echo "首次安装阶段 1 备份绑定字段无效" >&2
    return 1
  }
  [[ "$phase_manifest" == "$EXPECTED_MANIFEST_SHA256" ]] || {
    echo "阶段 2 发布包与首次安装阶段 1 的可信摘要不一致" >&2
    return 1
  }
  [[ -z "$previous_target" && ! -e "$PREVIOUS_LINK" && ! -L "$PREVIOUS_LINK" && "${initial_database_state:-}" == active-admin ]] || {
    echo "首次安装阶段衔接标记与 current/previous 或管理员状态不一致" >&2
    return 1
  }
  phase_release="$RELEASE_ROOT/$phase_release_id"
  validate_installed_release "$phase_release" || {
    echo "首次安装阶段 1 的候选版本缺失或完整性无效：$phase_release" >&2
    return 1
  }
  actual_manifest="$(sha256sum "$phase_release/SHA256SUMS" | awk '{print $1}')"
  [[ "$actual_manifest" == "$phase_manifest" ]] || {
    echo "首次安装阶段 1 候选清单与阶段衔接标记不一致" >&2
    return 1
  }
  maintenance_marker_owned_by "$MAINTENANCE_FLAG" "$phase_token" || {
    echo "首次安装阶段衔接标记在验证期间发生变化" >&2
    return 1
  }

  # Treat the strictly verified phase-1 marker as operation-owned without
  # pretending it was absent. ensure_maintenance_marker will now fail if it
  # disappears, while finish_maintenance_marker may remove exactly this token
  # only after every phase-2 gate succeeds.
  maintenance_marker_created=1
  maintenance_marker_token="$phase_token"
  phase_one_maintenance_adopted=1
  echo "已验证并接管首次安装阶段 1 的维护标记；仅阶段 2 完整门禁通过后解除维护。"
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

if (( FIRST_INSTALL == 1 )); then
  [[ -z "$previous_target" && ! -e "$PREVIOUS_LINK" && ! -L "$PREVIOUS_LINK" ]] || {
    echo "--first-install 只允许没有 current/previous 发布链接的新主机" >&2
    exit 1
  }
fi

# A new host may legitimately adopt a restored database with an active
# administrator. A truly empty database, however, must use the explicit
# first-install path so the one-shot administrator exists before normal backend
# bootstrap. A migration-only retry is accepted only while the user table is
# still empty.
if [[ -z "$previous_target" ]]; then
  psql_first_install=(
    psql --no-psqlrc --tuples-only --no-align --set ON_ERROR_STOP=1
    --host "$BACKEND_DATABASE_HOST"
    --port "$BACKEND_DATABASE_PORT"
    --username "$BACKEND_DATABASE_USER"
    --dbname "$BACKEND_DATABASE_DBNAME"
  )
  user_table_exists="$(
    PGPASSWORD="$BACKEND_DATABASE_PASSWORD" PGSSLMODE="$BACKEND_DATABASE_SSLMODE" PGCONNECT_TIMEOUT=5 \
      "${psql_first_install[@]}" --command "SELECT to_regclass('public.\"user\"') IS NOT NULL;"
  )"
  case "$user_table_exists" in
    f)
      public_relation_count="$(
        PGPASSWORD="$BACKEND_DATABASE_PASSWORD" PGSSLMODE="$BACKEND_DATABASE_SSLMODE" PGCONNECT_TIMEOUT=5 \
          "${psql_first_install[@]}" --command \
          "SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = 'public' AND c.relkind IN ('r','p','v','m','S','f');"
      )"
      [[ "$public_relation_count" =~ ^[0-9]+$ ]] || { echo "无法确认生产 public schema 状态" >&2; exit 1; }
      if (( 10#$public_relation_count == 0 )); then
        initial_database_state=empty-schema
      else
        initial_database_state=unknown-schema
      fi
      ;;
    t)
      user_counts="$(
        PGPASSWORD="$BACKEND_DATABASE_PASSWORD" PGSSLMODE="$BACKEND_DATABASE_SSLMODE" PGCONNECT_TIMEOUT=5 \
          "${psql_first_install[@]}" --command \
          "SELECT count(*) || ':' || count(*) FILTER (WHERE role = 'admin') || ':' || count(*) FILTER (WHERE role = 'admin' AND status = 1 AND deleted_at IS NULL) FROM public.\"user\";"
      )"
      [[ "$user_counts" =~ ^([0-9]+):([0-9]+):([0-9]+)$ ]] || { echo "无法确认生产管理员状态" >&2; exit 1; }
      total_users="${BASH_REMATCH[1]}"
      total_admins="${BASH_REMATCH[2]}"
      active_admins="${BASH_REMATCH[3]}"
      if (( 10#$active_admins > 0 )); then
        initial_database_state=active-admin
      elif (( 10#$total_admins > 0 )); then
        initial_database_state=inactive-admin
      elif (( 10#$total_users > 0 )); then
        initial_database_state=users-without-admin
      else
        initial_database_state=empty-users
      fi
      ;;
    *) echo "无法确认生产 user 表状态" >&2; exit 1 ;;
  esac
  case "$initial_database_state:$FIRST_INSTALL" in
    active-admin:0) ;;
    active-admin:1)
      echo "数据库已有可用管理员，拒绝 first-install；请使用普通发布" >&2
      exit 1
      ;;
    empty-schema:1|empty-users:1) ;;
    empty-schema:0|empty-users:0)
      echo "空生产库必须显式使用 --first-install，并通过受保护文件提供首位管理员密码" >&2
      exit 1
      ;;
    inactive-admin:*|users-without-admin:*)
      echo "数据库已有账号但没有可用平台管理员；拒绝将其当作首次安装，请先人工审计修复" >&2
      exit 1
      ;;
    unknown-schema:*)
      echo "public schema 已有非本应用关系且缺少 user 表；拒绝将其当作空生产库" >&2
      exit 1
      ;;
    *) echo "无法判定首次部署状态" >&2; exit 1 ;;
  esac
fi

if (( maintenance_was_active == 1 )); then
  phase_marker_status=0
  adopt_first_install_phase_marker || phase_marker_status=$?
  if (( phase_marker_status == 1 )); then
    echo "首次安装阶段衔接标记验证失败；保持维护且拒绝发布" >&2
    exit 1
  fi
fi

# A phase-1 first install mutates an empty database before there can be honest
# recovery evidence. Put every public/admin edge into verified maintenance
# before staging or running that bootstrap, and deliberately leave the marker
# in place when phase 1 stops. The later check is repeated immediately before
# bootstrap so a drifting/reloaded Nginx configuration still fails closed.
if (( FIRST_INSTALL == 1 )); then
  # A two-phase first install must own the single durable state marker from the
  # start. Never relabel or later remove an operator-created maintenance file.
  if (( maintenance_was_active == 1 )); then
    echo "首次安装不接管既有人工维护标记；请由原操作员解除后重新执行阶段 1" >&2
    exit 1
  fi
  if ! ensure_maintenance_marker "$MAINTENANCE_FLAG"; then
    echo "首次安装阶段 1 无法安全进入维护模式；尚未准备候选版本或修改数据库" >&2
    exit 1
  fi
  if ! verify_maintenance_edge "$PUBLIC_URL" "$PUBLIC_WWW_URL" "$ADMIN_URL"; then
    echo "首次安装阶段 1 未能确认所有边缘均处于维护模式；拒绝准备候选版本或修改数据库" >&2
    exit 1
  fi
fi

create_and_verify_backup_set() {
  local include_base_backup="$1"
  backup_set_started_epoch="$(date +%s)"
  [[ "$backup_set_started_epoch" =~ ^[1-9][0-9]{0,11}$ ]] || { echo "无法记录备份集合开始时间" >&2; return 1; }
  echo "创建并验证数据库备份"
  runuser --user wangzhe-backup -- \
    env -i PATH=/usr/bin:/bin HOME=/var/backups/wangzhe \
    APPLICATION_DATABASE_USER="$BACKEND_DATABASE_USER" \
    "$TRUSTED_SCRIPT_DIR/postgres-backup.sh" "$BACKUP_ENV" "$BACKUP_CRYPTO_ENV"

  echo "创建并验证上传目录备份"
  runuser --user wangzhe-backup -- \
    env -i PATH=/usr/bin:/bin HOME=/var/backups/wangzhe \
    "$TRUSTED_SCRIPT_DIR/upload-backup.sh" "$BACKUP_CRYPTO_ENV"

  if (( include_base_backup == 1 )); then
    echo "无 current 主机创建并验证首份 PITR 基础备份"
    runuser --user postgres -- \
      env -i PATH=/usr/bin:/bin HOME=/var/lib/postgresql \
      "$TRUSTED_SCRIPT_DIR/postgres-base-backup.sh" "$PITR_ENV"
  fi

  # A local .offsite-ok only proves that one historical upload readback once
  # succeeded. Use the dedicated monitor identity and its read/list-only
  # credential to read every remote ciphertext and evidence object again.
  echo "实时回读并验签远端四类备份制品"
  timeout 12h runuser --user wangzhe-monitor -- \
    env -i PATH=/usr/bin:/bin HOME=/var/lib/wangzhe-monitor \
    "$TRUSTED_SCRIPT_DIR/production-backup-integrity.sh" "$BACKUP_INTEGRITY_ENV"
}

read_first_install_artifact_sha() {
  local target_path="$1" expected_name="$2" started_epoch="$3" completed_epoch="$4"
  local recorded_sha recorded_name checksum_extra actual_sha provenance_name provenance_sha provenance_epoch
  [[ -f "$target_path" && ! -L "$target_path" && -f "$target_path.sha256" && ! -L "$target_path.sha256" && \
     -f "$target_path.offsite-ok" && ! -L "$target_path.offsite-ok" && \
     -f "$target_path.provenance" && ! -L "$target_path.provenance" ]] || return 1
  read -r recorded_sha recorded_name checksum_extra <"$target_path.sha256" || return 1
  actual_sha="$(sha256sum "$target_path" | awk '{print $1}')"
  provenance_name="$(phase_marker_field "$target_path.provenance" artifact_name)" || return 1
  provenance_sha="$(phase_marker_field "$target_path.provenance" cipher_sha256)" || return 1
  provenance_epoch="$(phase_marker_field "$target_path.provenance" created_at_epoch)" || return 1
  [[ "$recorded_sha" =~ ^[0-9a-f]{64}$ && "$recorded_sha" == "$actual_sha" && "$recorded_name" == "$expected_name" && \
     -z "${checksum_extra:-}" && "$provenance_name" == "$expected_name" && "$provenance_sha" == "$recorded_sha" && \
     "$provenance_epoch" =~ ^[1-9][0-9]{0,11}$ ]] || return 1
  (( provenance_epoch >= started_epoch && provenance_epoch <= completed_epoch )) || return 1
  printf '%s\n' "$recorded_sha"
}

capture_first_install_backup_binding() {
  local status_version status_epoch database_name upload_name base_name wal_count wal_first wal_last wal_sha status_extra now_epoch database_suffix
  [[ "${backup_set_started_epoch:-}" =~ ^[1-9][0-9]{0,11}$ ]] || { echo "首次安装备份集合缺少开始时间" >&2; return 1; }
  [[ -f "$BACKUP_INTEGRITY_STATUS_FILE" && ! -L "$BACKUP_INTEGRITY_STATUS_FILE" ]] || {
    echo "首次安装缺少本次备份完整性状态" >&2
    return 1
  }
  read -r status_version status_epoch database_name upload_name base_name wal_count wal_first wal_last wal_sha status_extra <"$BACKUP_INTEGRITY_STATUS_FILE" || return 1
  now_epoch="$(date +%s)"
  [[ "$status_version" == v2 && "$status_epoch" =~ ^[1-9][0-9]{0,11}$ && -z "${status_extra:-}" && \
     "$database_name" == "$BACKEND_DATABASE_DBNAME"-* && \
     "$upload_name" =~ ^uploads-[0-9]{8}-[0-9]{6}-[0-9]+\.tar\.age$ && \
     "$base_name" =~ ^basebackup-[0-9]{8}-[0-9]{6}-[0-9]+\.tar\.age$ && \
     "$wal_count" =~ ^[1-9][0-9]*$ && "$wal_first" =~ \.age$ && "$wal_last" =~ \.age$ && "$wal_sha" =~ ^[0-9a-f]{64}$ ]] || {
    echo "首次安装备份完整性状态字段无效" >&2
    return 1
  }
  database_suffix="${database_name#"$BACKEND_DATABASE_DBNAME"-}"
  [[ "$database_suffix" =~ ^[0-9]{8}-[0-9]{6}-[0-9]+\.dump\.age$ ]] || { echo "首次安装数据库备份名无效" >&2; return 1; }
  (( status_epoch >= backup_set_started_epoch && status_epoch <= now_epoch + 300 )) || {
    echo "备份完整性状态并非本次首次安装生成" >&2
    return 1
  }

  first_install_database_cipher_sha256="$(read_first_install_artifact_sha "$BACKUP_DIR/$database_name" "$database_name" "$backup_set_started_epoch" "$status_epoch")" || {
    echo "无法绑定本次首次安装数据库备份" >&2
    return 1
  }
  first_install_upload_cipher_sha256="$(read_first_install_artifact_sha "/var/backups/wangzhe/uploads/$upload_name" "$upload_name" "$backup_set_started_epoch" "$status_epoch")" || {
    echo "无法绑定本次首次安装 uploads 备份" >&2
    return 1
  }
  first_install_basebackup_cipher_sha256="$(read_first_install_artifact_sha "$PITR_BASEBACKUP_DIR/$base_name" "$base_name" "$backup_set_started_epoch" "$status_epoch")" || {
    echo "无法绑定本次首次安装 PITR 基础备份" >&2
    return 1
  }
  first_install_backup_completed_epoch="$status_epoch"
  first_install_database_artifact_name="$database_name"
  first_install_upload_artifact_name="$upload_name"
  first_install_basebackup_artifact_name="$base_name"
  first_install_wal_inventory_sha256="$wal_sha"
}

run_pre_release_recovery_gates() {
  local include_base_backup=0
  if (( FIRST_INSTALL == 1 )); then
    echo "首次安装阶段 1：空库尚无真实恢复证据，将在受限引导和异机备份后停机等待隔离演练"
    return 0
  fi

  [[ -n "$previous_target" ]] || include_base_backup=1
  create_and_verify_backup_set "$include_base_backup"

  # A successful upload is not proof that either recovery path still works.
  # This gate is never waived in the phase-2/normal path and still runs before
  # any candidate directory or release link is mutated.
  echo "发布前验证近期逻辑恢复与 PITR 演练证据"
  timeout 10m "$TRUSTED_SCRIPT_DIR/production-recovery-evidence-check.sh"
}

complete_first_install_phase_one() {
  create_and_verify_backup_set 1
  capture_first_install_backup_binding
  persist_first_install_phase_marker
  echo "首次安装阶段 1 完成：current 尚未切换，后端保持停止，维护模式保持开启。"
  echo "请在隔离恢复机对本次异机制品完成逻辑恢复与 PITR 演练；证据发布后使用新的 RELEASE_ID 执行普通发布完成阶段 2。"
}

run_pre_release_recovery_gates

install -d -o root -g root -m 0755 /opt/wangzhe "$RELEASE_ROOT"
staging="$RELEASE_ROOT/.staging-$RELEASE_ID-$$"
[[ ! -e "$staging" ]] || { echo "临时发布目录已存在：$staging" >&2; exit 1; }
cleanup_staging() {
  if [[ -n "${staging:-}" && "$staging" == "$RELEASE_ROOT/.staging-"* ]]; then
    rm -rf -- "$staging"
  fi
}
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
validate_member_betting_contract "$staging"
mv -T "$staging" "$target"
staging=""
if ! validate_installed_release "$target"; then
  rm -rf -- "$target"
  echo "安装后的发布版本校验失败，已移除未启用版本：$target" >&2
  exit 1
fi

# The two HTTPS vhosts return 503 while this root-owned marker exists. It is
# created before the symlink switch and removed only after the full production
# gate succeeds, so a failed candidate never becomes externally reachable.
if ! ensure_maintenance_marker "$MAINTENANCE_FLAG"; then
  echo "无法安全进入维护模式，未切换任何版本" >&2
  exit 1
fi
if ! verify_maintenance_edge "$PUBLIC_URL" "$PUBLIC_WWW_URL" "$ADMIN_URL"; then
  echo "当前 Nginx 未可靠进入维护模式；保留维护标记且未切换任何版本" >&2
  exit 1
fi

if (( FIRST_INSTALL == 1 )); then
  # Persist an atomically hashed pending state before the credential-bearing
  # unit can mutate the empty database. A signal after this point can never be
  # mistaken by a later process for an ordinary restored active-admin host.
  persist_first_install_pending_marker
  echo "空生产库首次安装：在后端启动前执行迁移并创建首位管理员"
  systemctl stop wangzhe-backend.service || true
  bootstrap_unit="wangzhe-bootstrap-admin-$$"
  bootstrap_credential="/run/credentials/${bootstrap_unit}.service/admin-password"
  bootstrap_unit_cleanup_armed=1
  if ! systemd-run --quiet --wait --pipe --collect --service-type=exec \
    --unit="$bootstrap_unit" --uid=wangzhe --gid=wangzhe \
    --property="EnvironmentFile=$APP_ENV" \
    --property="LoadCredential=admin-password:$FIRST_ADMIN_PASSWORD_FILE" \
    --property=WorkingDirectory=/var/lib/wangzhe \
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
    "$target/bin/wangzhe-bootstrap-admin" \
    --username "$FIRST_ADMIN_USERNAME" --password-file "$bootstrap_credential"; then
    echo "首位管理员创建失败；后端尚未启动，维护模式和已安装候选版本保持不变" >&2
    exit 1
  fi
  bootstrap_unit_cleanup_armed=0
  current_password_identity="$(stat -c '%d:%i' "$FIRST_ADMIN_PASSWORD_FILE" 2>/dev/null || true)"
  [[ "$current_password_identity" == "$first_admin_password_identity" ]] || {
    echo "首位管理员创建后密码源文件身份发生变化；拒绝继续启动" >&2
    exit 1
  }
  rm -f -- "$FIRST_ADMIN_PASSWORD_FILE"
  first_admin_password_cleanup_armed=0

  # The empty database could not have produced honest recovery evidence before
  # migration. Build and fully read back the real post-bootstrap backup set,
  # then stop with maintenance active and no current link. An isolated recovery
  # host must publish both signed drill statuses before a normal phase-2 deploy.
  complete_first_install_phase_one
  exit 0
fi

current_link_original_present=0
current_link_original_target=""
previous_link_original_present=0
previous_link_original_target=""
if [[ -L "$CURRENT_LINK" ]]; then
  current_link_original_present=1
  current_link_original_target="$(readlink "$CURRENT_LINK")"
fi
if [[ -L "$PREVIOUS_LINK" ]]; then
  previous_link_original_present=1
  previous_link_original_target="$(readlink "$PREVIOUS_LINK")"
fi

link_tmp="/opt/wangzhe/.current-$RELEASE_ID-$$"
previous_tmp="/opt/wangzhe/.previous-$RELEASE_ID-$$"
current_restore_tmp="/opt/wangzhe/.current-restore-$RELEASE_ID-$$"
previous_restore_tmp="/opt/wangzhe/.previous-restore-$RELEASE_ID-$$"
link_transaction_armed=0
link_transaction_committed=0
restore_release_link_state() {
  local link_path="$1" original_present="$2" original_target="$3" restore_tmp="$4"
  rm -f -- "$restore_tmp" || true
  if (( original_present == 1 )); then
    if ! ln -s "$original_target" "$restore_tmp"; then
      echo "警告：无法为发布链接创建恢复临时项：$link_path" >&2
      return 1
    fi
    if ! mv -Tf "$restore_tmp" "$link_path"; then
      rm -f -- "$restore_tmp" || true
      echo "警告：无法恢复发布链接：$link_path" >&2
      return 1
    fi
  else
    if [[ -e "$link_path" || -L "$link_path" ]]; then
      if [[ ! -L "$link_path" ]]; then
        echo "警告：发布链接被替换为非符号链接，拒绝清理：$link_path" >&2
        return 1
      fi
      rm -f -- "$link_path" || { echo "警告：无法移除新增发布链接：$link_path" >&2; return 1; }
    fi
  fi
}
cleanup_link() {
  local restore_failed=0
  rm -f -- "$link_tmp" "$previous_tmp" "$current_restore_tmp" "$previous_restore_tmp" || true
  if (( link_transaction_armed == 1 && link_transaction_committed == 0 )); then
    restore_release_link_state "$CURRENT_LINK" "$current_link_original_present" "$current_link_original_target" "$current_restore_tmp" || restore_failed=1
    restore_release_link_state "$PREVIOUS_LINK" "$previous_link_original_present" "$previous_link_original_target" "$previous_restore_tmp" || restore_failed=1
    (( restore_failed == 0 )) || echo "严重警告：发布链接事务回滚不完整，请保持维护模式并人工核对 current/previous" >&2
  fi
  link_transaction_armed=0
  return "$restore_failed"
}

# BEGIN release-link-transaction
ln -s "$target" "$link_tmp"
if [[ -n "$previous_target" ]]; then
  ln -s "$previous_target" "$previous_tmp"
fi
link_transaction_armed=1
if [[ -n "$previous_target" ]]; then
  mv -Tf "$previous_tmp" "$PREVIOUS_LINK"
fi
mv -Tf "$link_tmp" "$CURRENT_LINK"
link_transaction_committed=1
link_transaction_armed=0
# END release-link-transaction

systemctl reset-failed wangzhe-backend.service || true
if ! systemctl restart wangzhe-backend.service; then
  systemctl stop wangzhe-backend.service || true
  echo "新版本启动失败。它可能已经提交迁移，因此保持维护模式、新版本链接并停止服务，不自动启动旧代码。" >&2
  [[ -n "$previous_target" ]] && echo "上一版仍保存在 ${PREVIOUS_LINK}；核对迁移兼容性后再人工回滚。" >&2
  exit 1
fi

if ! wait_for_backend; then
  systemctl stop wangzhe-backend.service || true
  echo "新版本在 ${READY_TIMEOUT_SECONDS}s 内未就绪，请检查 journalctl -u wangzhe-backend" >&2
  echo "服务可能已经提交迁移；保持维护模式、新版本链接且已停服，不自动启动旧代码。" >&2
  exit 1
fi

if ! env -i PATH=/usr/bin:/bin HOME=/root \
  BACKEND_URL="http://${BACKEND_SERVER_BIND}:${BACKEND_SERVER_PORT}" BACKUP_DIR="$BACKUP_DIR" \
  PITR_WAL_DIR="$PITR_WAL_DIR" PITR_BASEBACKUP_DIR="$PITR_BASEBACKUP_DIR" ALLOW_MAINTENANCE_503=1 \
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
# Initialized by sourced maintenance-edge.sh.
# shellcheck disable=SC2154
if (( maintenance_was_active == 1 && phase_one_maintenance_adopted == 0 )); then
  echo "发布前已有维护标记，已按原样保留。"
fi
