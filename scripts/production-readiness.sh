#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${1:-/etc/wangzhe/backend.env}"
READINESS_API_URL="${BACKEND_URL:-http://127.0.0.1:8080}"
ALLOW_MAINTENANCE_503="${ALLOW_MAINTENANCE_503:-0}"
EXPECTED_GAMES="${EXPECTED_ENABLED_GAMES:-22}"
MAX_STALE_PENDING="${MAX_STALE_PENDING:-0}"
MAX_ABNORMAL_BETS="${MAX_ABNORMAL_BETS:-0}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/wangzhe/database}"
UPLOAD_BACKUP_DIR="${UPLOAD_BACKUP_DIR:-/var/backups/wangzhe/uploads}"
PITR_CLUSTER_ID_FILE="${PITR_CLUSTER_ID_FILE:-/etc/wangzhe/pitr-cluster-id}"
PITR_WAL_DIR="${PITR_WAL_DIR:-}"
PITR_BASEBACKUP_DIR="${PITR_BASEBACKUP_DIR:-}"
MAX_BACKUP_AGE_MINUTES="${MAX_BACKUP_AGE_MINUTES:-1560}"
MAX_WAL_ARCHIVE_AGE_MINUTES="${MAX_WAL_ARCHIVE_AGE_MINUTES:-15}"
MAX_BASEBACKUP_AGE_MINUTES="${MAX_BASEBACKUP_AGE_MINUTES:-11520}"
PUBLIC_URL="${PUBLIC_URL:-https://wz6688.app}"
PUBLIC_WWW_URL="${PUBLIC_WWW_URL:-https://www.wz6688.app}"
ADMIN_URL="${ADMIN_URL:-https://admin.wz888.site}"
PUBLIC_TLS_CERT_FILE="${PUBLIC_TLS_CERT_FILE:-/etc/letsencrypt/live/wz6688.app/fullchain.pem}"
ADMIN_TLS_CERT_FILE="${ADMIN_TLS_CERT_FILE:-/etc/letsencrypt/live/admin.wz888.site/fullchain.pem}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/backend-env.sh
source "$SCRIPT_DIR/lib/backend-env.sh"
# shellcheck source=lib/safe-integer.sh
source "$SCRIPT_DIR/lib/safe-integer.sh"
# shellcheck source=lib/encrypted-backup.sh
source "$SCRIPT_DIR/lib/encrypted-backup.sh"
"$SCRIPT_DIR/production-config-check.sh" "$ENV_FILE"
load_backend_env "$ENV_FILE"

# Hiding a public test password does not revoke it: both the preset and the
# published accounts must be disabled before this formal readiness gate passes.
[[ ! -e /etc/wangzhe/test-login.enabled && ! -L /etc/wangzhe/test-login.enabled ]] || {
  echo "测试账号填充仍在启用，拒绝正式上线" >&2
  exit 1
}

for command_name in awk basename curl find id jq openssl psql runuser sha256sum sort stat tail timeout; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done

if [[ -z "$PITR_WAL_DIR" || -z "$PITR_BASEBACKUP_DIR" ]]; then
  [[ -f "$PITR_CLUSTER_ID_FILE" && ! -L "$PITR_CLUSTER_ID_FILE" && "$(stat -c '%u' "$PITR_CLUSTER_ID_FILE")" == 0 && -z "$(find "$PITR_CLUSTER_ID_FILE" -perm /022 -print -quit)" ]] || { echo "PITR 集群标识文件必须由 root 保护：$PITR_CLUSTER_ID_FILE" >&2; exit 1; }
  read -r pitr_cluster_id pitr_cluster_extra <"$PITR_CLUSTER_ID_FILE" || { echo "PITR 集群标识不可读" >&2; exit 1; }
  [[ "$pitr_cluster_id" =~ ^[0-9]{10,30}$ && -z "${pitr_cluster_extra:-}" ]] || { echo "PITR 集群标识必须是 PostgreSQL system identifier" >&2; exit 1; }
  PITR_WAL_DIR="/var/backups/wangzhe/wal/$pitr_cluster_id"
  PITR_BASEBACKUP_DIR="/var/backups/wangzhe/base/$pitr_cluster_id"
else
  [[ "$(basename "$PITR_WAL_DIR")" =~ ^[0-9]{10,30}$ && "$(basename "$PITR_WAL_DIR")" == "$(basename "$PITR_BASEBACKUP_DIR")" ]] || {
    echo "PITR WAL 与基础备份目录必须绑定同一个 PostgreSQL system identifier" >&2
    exit 1
  }
fi

[[ "${BACKEND_SERVER_MODE:-}" == "release" ]] || { echo "后端不是 release 模式" >&2; exit 1; }
: "${BACKEND_SERVER_BIND:?缺少 BACKEND_SERVER_BIND}"
: "${BACKEND_DATABASE_HOST:?缺少 BACKEND_DATABASE_HOST}"
: "${BACKEND_DATABASE_PORT:?缺少 BACKEND_DATABASE_PORT}"
: "${BACKEND_DATABASE_USER:?缺少 BACKEND_DATABASE_USER}"
: "${BACKEND_DATABASE_DBNAME:?缺少 BACKEND_DATABASE_DBNAME}"
: "${BACKEND_DATABASE_PASSWORD:?缺少 BACKEND_DATABASE_PASSWORD}"
: "${BACKEND_DATABASE_SSLMODE:?缺少 BACKEND_DATABASE_SSLMODE}"
: "${BACKEND_REDIS_ADDR:?缺少 BACKEND_REDIS_ADDR}"
: "${BACKEND_REDIS_USERNAME:?缺少 BACKEND_REDIS_USERNAME}"
: "${BACKEND_REDIS_PREFIX:?缺少 BACKEND_REDIS_PREFIX}"
: "${BACKEND_JWT_SECRET:?缺少 BACKEND_JWT_SECRET}"
: "${BACKEND_SECURITY_DATA_ENCRYPTION_KEY:?缺少 BACKEND_SECURITY_DATA_ENCRYPTION_KEY}"
: "${BACKEND_SERVER_ALLOWED_ORIGINS:?缺少 BACKEND_SERVER_ALLOWED_ORIGINS}"
: "${BACKEND_SERVER_TRUSTED_PROXIES:?缺少 BACKEND_SERVER_TRUSTED_PROXIES}"
: "${BACKEND_UPLOAD_DIR:?缺少 BACKEND_UPLOAD_DIR}"
: "${BACKEND_ROOM_ACTIVITY:?缺少 BACKEND_ROOM_ACTIVITY}"
: "${BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES:?缺少 BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES}"

EXPECTED_GAMES="$(require_decimal_count EXPECTED_ENABLED_GAMES "$EXPECTED_GAMES")"
MAX_STALE_PENDING="$(require_decimal_count MAX_STALE_PENDING "$MAX_STALE_PENDING")"
MAX_ABNORMAL_BETS="$(require_decimal_count MAX_ABNORMAL_BETS "$MAX_ABNORMAL_BETS")"
MAX_ENABLED_ROBOT_WORKSPACES="$(require_decimal_count BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES "$BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES")"
MAX_BACKUP_AGE_MINUTES="$(require_decimal_count MAX_BACKUP_AGE_MINUTES "$MAX_BACKUP_AGE_MINUTES")"
MAX_WAL_ARCHIVE_AGE_MINUTES="$(require_decimal_count MAX_WAL_ARCHIVE_AGE_MINUTES "$MAX_WAL_ARCHIVE_AGE_MINUTES")"
MAX_BASEBACKUP_AGE_MINUTES="$(require_decimal_count MAX_BASEBACKUP_AGE_MINUTES "$MAX_BASEBACKUP_AGE_MINUTES")"
(( 10#$MAX_BACKUP_AGE_MINUTES >= 1 )) || { echo "MAX_BACKUP_AGE_MINUTES 必须是正整数" >&2; exit 1; }
(( 10#$MAX_WAL_ARCHIVE_AGE_MINUTES >= 1 )) || { echo "MAX_WAL_ARCHIVE_AGE_MINUTES 必须是正整数" >&2; exit 1; }
(( 10#$MAX_BASEBACKUP_AGE_MINUTES >= 1 )) || { echo "MAX_BASEBACKUP_AGE_MINUTES 必须是正整数" >&2; exit 1; }
[[ "$ALLOW_MAINTENANCE_503" == "0" || "$ALLOW_MAINTENANCE_503" == "1" ]] || { echo "ALLOW_MAINTENANCE_503 只能是 0 或 1" >&2; exit 1; }
[[ "$BACKEND_SERVER_BIND" == "127.0.0.1" || "$BACKEND_SERVER_BIND" == "::1" ]] || { echo "后端必须只监听本机，由 Nginx 对外服务" >&2; exit 1; }
case "${BACKEND_DATABASE_HOST,,}" in
  localhost|127.0.0.1|::1) ;;
  *) [[ "$BACKEND_DATABASE_SSLMODE" == "verify-ca" || "$BACKEND_DATABASE_SSLMODE" == "verify-full" ]] || {
    echo "远程 PostgreSQL 必须校验证书（verify-ca/verify-full）" >&2
    exit 1
  } ;;
esac
[[ "$BACKEND_UPLOAD_DIR" == /* && "$BACKEND_UPLOAD_DIR" != "/" ]] || { echo "上传目录必须是非根绝对路径" >&2; exit 1; }
[[ -d "$BACKEND_UPLOAD_DIR" && ! -L "$BACKEND_UPLOAD_DIR" ]] || { echo "上传目录不存在或是符号链接：$BACKEND_UPLOAD_DIR" >&2; exit 1; }
(( ${#BACKEND_JWT_SECRET} >= 32 )) || { echo "JWT 密钥少于 32 位" >&2; exit 1; }
(( ${#BACKEND_SECURITY_DATA_ENCRYPTION_KEY} >= 32 )) || { echo "数据加密密钥少于 32 位" >&2; exit 1; }
[[ "$BACKEND_JWT_SECRET" != "$BACKEND_SECURITY_DATA_ENCRYPTION_KEY" ]] || { echo "JWT 与数据加密必须使用不同密钥" >&2; exit 1; }
previous_data_keys="${BACKEND_SECURITY_DATA_ENCRYPTION_PREVIOUS_KEYS:-[]}"
if ! jq -e '
    type == "array" and length <= 8 and
    all(.[]; type == "string" and length >= 32 and (explode | all(. >= 32 and . != 127))) and
    ((unique | length) == length)
  ' <<<"$previous_data_keys" >/dev/null; then
  echo "历史数据加密密钥配置无效" >&2
  exit 1
fi
while IFS= read -r previous_data_key; do
  if [[ "$previous_data_key" == "$BACKEND_SECURITY_DATA_ENCRYPTION_KEY" ||
        "$previous_data_key" == "$BACKEND_JWT_SECRET" ||
        "$previous_data_key" == "$BACKEND_DATABASE_PASSWORD" ]]; then
    echo "历史数据加密密钥复用了当前密钥或其他凭据" >&2
    exit 1
  fi
done < <(jq -r '.[]' <<<"$previous_data_keys")
case "${BACKEND_JWT_SECRET,,}:${BACKEND_SECURITY_DATA_ENCRYPTION_KEY,,}:${BACKEND_DATABASE_PASSWORD,,}" in
  *change_me*|*changeme*|*replace_with*|*example*) echo "仍存在示例密钥或密码" >&2; exit 1 ;;
esac
IFS=',' read -r -a allowed_origins <<<"$BACKEND_SERVER_ALLOWED_ORIGINS"
for origin in "${allowed_origins[@]}"; do
  [[ "$origin" == https://* ]] || { echo "release CORS 只允许 HTTPS：$origin" >&2; exit 1; }
done
[[ ",$BACKEND_SERVER_TRUSTED_PROXIES," != *,0.0.0.0/0,* && ",$BACKEND_SERVER_TRUSTED_PROXIES," != *,::/0,* ]] || { echo "受信任代理范围过宽" >&2; exit 1; }

REDIS_CHECK_ENV_FILE="${REDIS_CHECK_ENV_FILE:-/etc/wangzhe/redis-check.env}"
[[ -f "$REDIS_CHECK_ENV_FILE" && ! -L "$REDIS_CHECK_ENV_FILE" ]] || { echo "Redis 监控凭据文件无效：$REDIS_CHECK_ENV_FILE" >&2; exit 1; }
id wangzhe-monitor >/dev/null 2>&1 || { echo "缺少系统用户 wangzhe-monitor" >&2; exit 1; }
# Read only the two non-secret expectations back from the monitor-owned file.
# The subprocess owns and then discards REDIS_PASSWORD; it never imports that
# credential into this root readiness process or prints it to stdout.
redis_expected_identity="$(timeout 5 runuser --user wangzhe-monitor -- env -i PATH=/usr/bin:/bin HOME=/var/lib/wangzhe-monitor \
  bash -c '
    set -euo pipefail
    source "$1"
    load_strict_env "$2" "^REDIS_[A-Z0-9_]+$"
    : "${REDIS_EXPECTED_APP_USERNAME:?}"
    : "${REDIS_EXPECTED_APP_PREFIX:?}"
    [[ "$REDIS_EXPECTED_APP_USERNAME" =~ ^[A-Za-z0-9_.-]{1,64}$ ]]
    [[ "$REDIS_EXPECTED_APP_PREFIX" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{2,95}$ ]]
    printf "%s:%s" "$REDIS_EXPECTED_APP_USERNAME" "$REDIS_EXPECTED_APP_PREFIX"
  ' wangzhe-redis-identity "$SCRIPT_DIR/lib/strict-env.sh" "$REDIS_CHECK_ENV_FILE")" || {
  echo "无法安全读取 Redis 应用 ACL 期望值" >&2
  exit 1
}
[[ "$redis_expected_identity" == "$BACKEND_REDIS_USERNAME:$BACKEND_REDIS_PREFIX" ]] || {
  echo "Redis ACL 期望用户/前缀与后端运行配置不一致" >&2
  exit 1
}
unset redis_expected_identity
timeout 15 runuser --user wangzhe-monitor -- env -i PATH=/usr/bin:/bin HOME=/var/lib/wangzhe-monitor \
  "$SCRIPT_DIR/redis-production-check.sh" "$REDIS_CHECK_ENV_FILE" >/dev/null

curl -fsS --max-time 10 "$READINESS_API_URL/health" >/dev/null
curl -fsS --max-time 10 "$READINESS_API_URL/ready" >/dev/null
curl -fsS --max-time 120 "$READINESS_API_URL/ready/encryption" >/dev/null || {
  echo "敏感字段存在明文、未知信封或当前密钥配置无法鉴权的数据，拒绝上线" >&2
  exit 1
}
curl -fsS --max-time 10 "$READINESS_API_URL/ready/odds" >/dev/null || {
  echo "会员可达房间已开放彩种的生产赔率不完整，拒绝上线" >&2
  exit 1
}
check_tls_certificate() {
  local label="$1" certificate="$2"
  [[ -f "$certificate" ]] || { echo "找不到${label} TLS 证书：$certificate" >&2; exit 1; }
  openssl x509 -checkend 1209600 -noout -in "$certificate" >/dev/null || {
    echo "${label} TLS 证书将在 14 天内过期，拒绝上线" >&2
    exit 1
  }
}
check_tls_certificate "会员端" "$PUBLIC_TLS_CERT_FILE"
check_tls_certificate "管理端" "$ADMIN_TLS_CERT_FILE"

check_https_endpoint() {
  local url="$1" headers lower_headers redirect_headers lower_redirect_headers status_line expected_status
  [[ "$url" =~ ^https://[A-Za-z0-9.-]+(:[0-9]+)?$ ]] || { echo "外部检查地址必须是 HTTPS Origin：$url" >&2; exit 1; }
  headers="$(curl -sSI --max-time 10 "$url/")" || { echo "$url 无法连接" >&2; exit 1; }
  lower_headers="$(printf '%s' "$headers" | tr -d '\r' | tr '[:upper:]' '[:lower:]')"
  status_line="$(printf '%s\n' "$lower_headers" | awk '/^http\// { status=$2 } END { print status }')"
  expected_status=200
  [[ "$ALLOW_MAINTENANCE_503" == "1" ]] && expected_status=503
  [[ "$status_line" == "$expected_status" ]] || { echo "$url 状态异常：期望 ${expected_status}，实际 ${status_line:-未知}" >&2; exit 1; }
  grep -q '^strict-transport-security:.*max-age=' <<<"$lower_headers" || { echo "$url 缺少 HSTS" >&2; exit 1; }
  grep -q '^content-security-policy:' <<<"$lower_headers" || { echo "$url 缺少 CSP" >&2; exit 1; }
  grep -q '^x-content-type-options:[[:space:]]*nosniff' <<<"$lower_headers" || { echo "$url 缺少 nosniff" >&2; exit 1; }
  redirect_headers="$(curl -fsSI --max-time 10 "http://${url#https://}/")"
  lower_redirect_headers="$(printf '%s' "$redirect_headers" | tr -d '\r' | tr '[:upper:]' '[:lower:]')"
  grep -q "^location:[[:space:]]*$url/" <<<"$lower_redirect_headers" || { echo "$url 的 HTTP 入口没有固定跳转到 HTTPS" >&2; exit 1; }
}
check_https_endpoint "$PUBLIC_URL"
check_https_endpoint "$PUBLIC_WWW_URL"
check_https_endpoint "$ADMIN_URL"

games_payload="$(curl -fsS --max-time 10 "$READINESS_API_URL/api/public/lottery/games/enabled")"
enabled_count="$(printf '%s' "$games_payload" | jq -er '.data | arrays | length')"
enabled_count="$(require_decimal_count 启用彩种数 "$enabled_count")"
[[ "$enabled_count" == "$EXPECTED_GAMES" ]] || { echo "启用彩种异常：期望 ${EXPECTED_GAMES}，实际 $enabled_count" >&2; exit 1; }
status_payload="$(curl -fsS --max-time 10 "$READINESS_API_URL/api/public/lottery/status")"
read_lottery_health_count() {
  local field="$1" label="$2" value
  value="$(printf '%s' "$status_payload" | jq -er ".data.health.$field | numbers")" || {
    echo "开奖健康指标缺失或格式错误：$label" >&2
    exit 1
  }
  require_decimal_count "$label" "$value"
}
source_errors="$(read_lottery_health_count source_error_game_count 开奖源异常数)"
error_issues="$(read_lottery_health_count error_issue_count 开奖错误期号数)"
stale_issues="$(read_lottery_health_count stale_pending_issue_count 开奖超时期号数)"
unrecoverable_bets="$(read_lottery_health_count unrecoverable_bet_count 不可恢复注单数)"
health_abnormal_bets="$(read_lottery_health_count abnormal_bet_count 开奖健康异常注单数)"

export PGPASSWORD="$BACKEND_DATABASE_PASSWORD"
export PGSSLMODE="$BACKEND_DATABASE_SSLMODE"
export PGCONNECT_TIMEOUT=5
psql_base=(
  timeout 10 psql --no-psqlrc --tuples-only --no-align
  --host "$BACKEND_DATABASE_HOST"
  --port "$BACKEND_DATABASE_PORT"
  --username "$BACKEND_DATABASE_USER"
  --dbname "$BACKEND_DATABASE_DBNAME"
)

active_test_accounts="$("${psql_base[@]}" --command "SELECT count(*) FROM \"user\" WHERE remark LIKE 'test-site-accounts:v1:%' AND status = 1 AND deleted_at IS NULL;")"
active_test_accounts="$(require_decimal_count 活跃公开测试账号数 "$active_test_accounts")"
(( 10#$active_test_accounts == 0 )) || { echo "仍有公开测试账号未停用，拒绝正式上线" >&2; exit 1; }

stale_pending="$("${psql_base[@]}" --command "SELECT count(*) FROM lottery_bets WHERE status IN ('pending','accepted','settling') AND created_at < now() - interval '1 hour';")"
orphan_bets="$("${psql_base[@]}" --command "SELECT count(*) FROM lottery_bets WHERE workspace_id = 0;")"
orphan_users="$("${psql_base[@]}" --command "SELECT count(*) FROM \"user\" WHERE workspace_id = 0 AND deleted_at IS NULL;")"
abnormal_bets="$("${psql_base[@]}" --command "SELECT count(*) FROM lottery_bets WHERE reconciliation_status = 'abnormal';")"
enabled_robots="$("${psql_base[@]}" --command "SELECT count(*) FROM workspace_robot_settings WHERE enabled = true;")"
failed_cleanup="$("${psql_base[@]}" --command "SELECT count(*) FROM data_cleanup_runs WHERE status = 'failed' AND created_at > now() - interval '24 hours';")"
custom_tablespaces="$("${psql_base[@]}" --command "SELECT count(*) FROM pg_tablespace WHERE spcname NOT IN ('pg_default','pg_global');")"
unset PGPASSWORD PGSSLMODE PGCONNECT_TIMEOUT

stale_pending="$(require_decimal_count 超时未结注单数 "$stale_pending")"
orphan_bets="$(require_decimal_count 无归属注单数 "$orphan_bets")"
orphan_users="$(require_decimal_count 无归属账号数 "$orphan_users")"
abnormal_bets="$(require_decimal_count 异常注单数 "$abnormal_bets")"
enabled_robots="$(require_decimal_count 启用机器人数 "$enabled_robots")"
failed_cleanup="$(require_decimal_count 清理失败任务数 "$failed_cleanup")"
custom_tablespaces="$(require_decimal_count 自定义表空间数 "$custom_tablespaces")"

echo "启用彩种=$enabled_count  开奖源异常=$source_errors  错误期号=$error_issues  超时期号=$stale_issues  不可恢复注单=$unrecoverable_bets  超时未结=$stale_pending  异常注单=$abnormal_bets  无归属注单=$orphan_bets  无归属账号=$orphan_users  机器人调度=$BACKEND_ROOM_ACTIVITY  启用机器人工作区=$enabled_robots/$MAX_ENABLED_ROBOT_WORKSPACES  近24小时清理失败=$failed_cleanup  自定义表空间=$custom_tablespaces"
(( 10#$source_errors == 0 )) || { echo "存在开奖源异常，拒绝上线" >&2; exit 1; }
(( 10#$error_issues == 0 )) || { echo "存在开奖错误期号，拒绝上线" >&2; exit 1; }
(( 10#$stale_issues == 0 )) || { echo "存在开奖超时期号，拒绝上线" >&2; exit 1; }
(( 10#$unrecoverable_bets == 0 )) || { echo "存在不可恢复注单，拒绝上线" >&2; exit 1; }
(( 10#$health_abnormal_bets == 0 )) || { echo "开奖健康指标存在异常注单，拒绝上线" >&2; exit 1; }
(( 10#$stale_pending <= 10#$MAX_STALE_PENDING )) || { echo "超时未结注单超过阈值，拒绝上线" >&2; exit 1; }
(( 10#$abnormal_bets <= 10#$MAX_ABNORMAL_BETS )) || { echo "异常注单超过阈值，拒绝上线" >&2; exit 1; }
(( 10#$orphan_bets == 0 )) || { echo "存在无工作区注单，拒绝上线" >&2; exit 1; }
(( 10#$orphan_users == 0 )) || { echo "存在无工作区账号，拒绝上线" >&2; exit 1; }
(( 10#$enabled_robots <= 10#$MAX_ENABLED_ROBOT_WORKSPACES )) || { echo "启用机器人工作区数量超过生产配置上限" >&2; exit 1; }
(( 10#$failed_cleanup == 0 )) || { echo "存在数据维护失败任务，拒绝上线" >&2; exit 1; }
(( 10#$custom_tablespaces == 0 )) || { echo "当前 PITR 流程不支持自定义 PostgreSQL 表空间，拒绝上线" >&2; exit 1; }

check_recent_encrypted_offsite_backup() {
  local directory="$1" pattern="$2" max_age="$3" label="$4" require_wal_source="${5:-0}"
  local recent marker_checksum marker_remote marker_extra actual_checksum
  local source_checksum source_wal_name source_cluster source_extra
  [[ -d "$directory" && ! -L "$directory" ]] || { echo "$label 目录不存在或是符号链接：$directory" >&2; exit 1; }
  recent="$(find "$directory" -maxdepth 1 -type f -name "$pattern" -mmin "-$max_age" -print 2>/dev/null | LC_ALL=C sort | tail -n 1)"
  [[ -n "$recent" ]] || { echo "没有找到足够新的$label" >&2; exit 1; }
  validate_encrypted_backup_and_manifest "$recent" || { echo "$label SHA-256 校验失败" >&2; exit 1; }
  [[ -f "$recent.offsite-ok" && ! -L "$recent.offsite-ok" ]] || { echo "$label 缺少异机回读凭证" >&2; exit 1; }
  read -r marker_checksum marker_remote marker_extra <"$recent.offsite-ok" || { echo "$label 异机凭证不可读" >&2; exit 1; }
  actual_checksum="$(sha256sum "$recent" | awk '{print $1}')"
  [[ "$marker_checksum" == "$actual_checksum" && "$marker_remote" == *:* && -z "${marker_extra:-}" ]] || {
    echo "$label 异机凭证与本地加密制品不一致" >&2
    exit 1
  }
  if [[ "$require_wal_source" == 1 ]]; then
    [[ -f "$recent.source.sha256" && ! -L "$recent.source.sha256" ]] || { echo "$label 缺少源WAL与集群绑定凭证" >&2; exit 1; }
    read -r source_checksum source_wal_name source_cluster source_extra <"$recent.source.sha256" || { echo "$label 源WAL凭证不可读" >&2; exit 1; }
    [[ "$source_checksum" =~ ^[0-9a-f]{64}$ && "$source_wal_name.age" == "$(basename "$recent")" && "$source_cluster" == "$(basename "$directory")" && -z "${source_extra:-}" ]] || {
      echo "$label 源WAL与 PostgreSQL 集群标识不匹配" >&2
      exit 1
    }
  fi
}
check_recent_encrypted_offsite_backup "$BACKUP_DIR" "${BACKEND_DATABASE_DBNAME}-*.dump.age" "$MAX_BACKUP_AGE_MINUTES" "数据库加密备份"
check_recent_encrypted_offsite_backup "$UPLOAD_BACKUP_DIR" 'uploads-*.tar.age' "$MAX_BACKUP_AGE_MINUTES" "上传目录加密备份"
check_recent_encrypted_offsite_backup "$PITR_WAL_DIR" '*.age' "$MAX_WAL_ARCHIVE_AGE_MINUTES" "PITR WAL归档" 1
check_recent_encrypted_offsite_backup "$PITR_BASEBACKUP_DIR" 'basebackup-*.tar.age' "$MAX_BASEBACKUP_AGE_MINUTES" "PITR基础备份"

echo "生产就绪检查通过"
