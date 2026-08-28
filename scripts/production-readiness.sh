#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${1:-/etc/wangzhe/backend.env}"
BACKEND_URL="${BACKEND_URL:-http://127.0.0.1:8080}"
EXPECTED_GAMES="${EXPECTED_ENABLED_GAMES:-22}"
MAX_STALE_PENDING="${MAX_STALE_PENDING:-0}"
MAX_ABNORMAL_BETS="${MAX_ABNORMAL_BETS:-0}"
MAX_ENABLED_ROBOTS="${MAX_ENABLED_ROBOTS:-0}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/wangzhe}"
MAX_BACKUP_AGE_MINUTES="${MAX_BACKUP_AGE_MINUTES:-1560}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/backend-env.sh
source "$SCRIPT_DIR/lib/backend-env.sh"
load_backend_env "$ENV_FILE"

for command_name in curl jq psql stat find sha256sum; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done

[[ "${BACKEND_SERVER_MODE:-}" == "release" ]] || { echo "后端不是 release 模式" >&2; exit 1; }
: "${BACKEND_SERVER_BIND:?缺少 BACKEND_SERVER_BIND}"
: "${BACKEND_DATABASE_HOST:?缺少 BACKEND_DATABASE_HOST}"
: "${BACKEND_DATABASE_PORT:?缺少 BACKEND_DATABASE_PORT}"
: "${BACKEND_DATABASE_USER:?缺少 BACKEND_DATABASE_USER}"
: "${BACKEND_DATABASE_DBNAME:?缺少 BACKEND_DATABASE_DBNAME}"
: "${BACKEND_DATABASE_PASSWORD:?缺少 BACKEND_DATABASE_PASSWORD}"
: "${BACKEND_DATABASE_SSLMODE:?缺少 BACKEND_DATABASE_SSLMODE}"
: "${BACKEND_REDIS_ADDR:?缺少 BACKEND_REDIS_ADDR}"
: "${BACKEND_JWT_SECRET:?缺少 BACKEND_JWT_SECRET}"
: "${BACKEND_SECURITY_DATA_ENCRYPTION_KEY:?缺少 BACKEND_SECURITY_DATA_ENCRYPTION_KEY}"
: "${BACKEND_SERVER_ALLOWED_ORIGINS:?缺少 BACKEND_SERVER_ALLOWED_ORIGINS}"
: "${BACKEND_SERVER_TRUSTED_PROXIES:?缺少 BACKEND_SERVER_TRUSTED_PROXIES}"
: "${BACKEND_UPLOAD_DIR:?缺少 BACKEND_UPLOAD_DIR}"

[[ "$EXPECTED_GAMES" =~ ^[0-9]+$ ]] || { echo "EXPECTED_ENABLED_GAMES 必须是整数" >&2; exit 1; }
[[ "$MAX_STALE_PENDING" =~ ^[0-9]+$ && "$MAX_ABNORMAL_BETS" =~ ^[0-9]+$ && "$MAX_ENABLED_ROBOTS" =~ ^[0-9]+$ ]] || {
  echo "生产阈值必须是非负整数" >&2
  exit 1
}
[[ "$MAX_BACKUP_AGE_MINUTES" =~ ^[0-9]+$ ]] && (( MAX_BACKUP_AGE_MINUTES >= 1 )) || { echo "MAX_BACKUP_AGE_MINUTES 必须是正整数" >&2; exit 1; }
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
case "${BACKEND_JWT_SECRET,,}:${BACKEND_SECURITY_DATA_ENCRYPTION_KEY,,}:${BACKEND_DATABASE_PASSWORD,,}" in
  *change_me*|*changeme*|*replace_with*|*example*) echo "仍存在示例密钥或密码" >&2; exit 1 ;;
esac
IFS=',' read -r -a allowed_origins <<<"$BACKEND_SERVER_ALLOWED_ORIGINS"
for origin in "${allowed_origins[@]}"; do
  [[ "$origin" == https://* ]] || { echo "release CORS 只允许 HTTPS：$origin" >&2; exit 1; }
done
[[ ",$BACKEND_SERVER_TRUSTED_PROXIES," != *,0.0.0.0/0,* && ",$BACKEND_SERVER_TRUSTED_PROXIES," != *,::/0,* ]] || { echo "受信任代理范围过宽" >&2; exit 1; }

curl -fsS "$BACKEND_URL/health" >/dev/null
curl -fsS "$BACKEND_URL/ready" >/dev/null
games_payload="$(curl -fsS "$BACKEND_URL/api/public/lottery/games/enabled")"
enabled_count="$(printf '%s' "$games_payload" | jq -er '.data | arrays | length')"
[[ "$enabled_count" == "$EXPECTED_GAMES" ]] || { echo "启用彩种异常：期望 $EXPECTED_GAMES，实际 $enabled_count" >&2; exit 1; }
status_payload="$(curl -fsS "$BACKEND_URL/api/public/lottery/status")"
source_errors="$(printf '%s' "$status_payload" | jq -er '.data.health.source_error_game_count // 0')"

export PGPASSWORD="$BACKEND_DATABASE_PASSWORD"
psql_base=(
  psql --no-psqlrc --tuples-only --no-align
  --host "$BACKEND_DATABASE_HOST"
  --port "$BACKEND_DATABASE_PORT"
  --username "$BACKEND_DATABASE_USER"
  --dbname "$BACKEND_DATABASE_DBNAME"
)

stale_pending="$("${psql_base[@]}" --command "SELECT count(*) FROM lottery_bets WHERE status IN ('pending','accepted','settling') AND created_at < now() - interval '1 hour';")"
orphan_bets="$("${psql_base[@]}" --command "SELECT count(*) FROM lottery_bets WHERE workspace_id = 0;")"
orphan_users="$("${psql_base[@]}" --command "SELECT count(*) FROM \"user\" WHERE workspace_id = 0 AND deleted_at IS NULL;")"
abnormal_bets="$("${psql_base[@]}" --command "SELECT count(*) FROM lottery_bets WHERE reconciliation_status = 'abnormal';")"
enabled_robots="$("${psql_base[@]}" --command "SELECT count(*) FROM workspace_robot_settings WHERE enabled = true;")"
failed_cleanup="$("${psql_base[@]}" --command "SELECT count(*) FROM data_cleanup_runs WHERE status = 'failed' AND created_at > now() - interval '24 hours';")"

echo "启用彩种=$enabled_count  开奖源异常=$source_errors  超时未结=$stale_pending  异常注单=$abnormal_bets  无归属注单=$orphan_bets  无归属账号=$orphan_users  运行机器人=$enabled_robots  近24小时清理失败=$failed_cleanup"
(( source_errors == 0 )) || { echo "存在开奖源异常，拒绝上线" >&2; exit 1; }
(( stale_pending <= MAX_STALE_PENDING )) || { echo "超时未结注单超过阈值，拒绝上线" >&2; exit 1; }
(( abnormal_bets <= MAX_ABNORMAL_BETS )) || { echo "异常注单超过阈值，拒绝上线" >&2; exit 1; }
(( orphan_bets == 0 )) || { echo "存在无工作区注单，拒绝上线" >&2; exit 1; }
(( orphan_users == 0 )) || { echo "存在无工作区账号，拒绝上线" >&2; exit 1; }
(( enabled_robots <= MAX_ENABLED_ROBOTS )) || { echo "启用机器人数量超过上线阈值" >&2; exit 1; }
(( failed_cleanup == 0 )) || { echo "存在数据维护失败任务，拒绝上线" >&2; exit 1; }

[[ -d "$BACKUP_DIR" && ! -L "$BACKUP_DIR" ]] || { echo "备份目录不存在或是符号链接：$BACKUP_DIR" >&2; exit 1; }
recent_backup="$(find "$BACKUP_DIR" -maxdepth 1 -type f -name "${BACKEND_DATABASE_DBNAME}-*.dump" -mmin "-$MAX_BACKUP_AGE_MINUTES" -print -quit 2>/dev/null || true)"
[[ -n "$recent_backup" && -f "$recent_backup.sha256" ]] || { echo "没有找到已校验且足够新的数据库备份" >&2; exit 1; }
(
  cd "$BACKUP_DIR"
  sha256sum --check --status "$(basename "$recent_backup.sha256")"
) || { echo "最近数据库备份的校验和无效" >&2; exit 1; }

echo "生产就绪检查通过"
