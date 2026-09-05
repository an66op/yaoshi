#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib/backend-env.sh
source "$ROOT_DIR/scripts/lib/backend-env.sh"
if (( $# > 1 )); then
  echo "用法：scripts/local-health.sh [ENV_FILE]" >&2
  exit 1
fi
load_optional_backend_env "${1:-}"
apply_local_backend_defaults
require_local_backend_target

for command_name in curl jq; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done

BACKEND_URL="${BACKEND_URL:-http://127.0.0.1:${BACKEND_SERVER_PORT}}"
MEMBER_URL="${MEMBER_URL:-http://127.0.0.1:5173}"
ADMIN_URL="${ADMIN_URL:-http://127.0.0.1:5174}"
PG_HOST="${BACKEND_DATABASE_HOST:-localhost}"
PG_PORT="${BACKEND_DATABASE_PORT:-5432}"
PG_USER="${BACKEND_DATABASE_USER:-postgres}"
PG_DB="${BACKEND_DATABASE_DBNAME:-wangzhe}"

postgres_ready_cmd="$(find_postgres_tool pg_isready)"
PGPASSWORD="$BACKEND_DATABASE_PASSWORD" PGSSLMODE="$BACKEND_DATABASE_SSLMODE" \
  "$postgres_ready_cmd" -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d "$PG_DB" >/dev/null

curl -fsS "$BACKEND_URL/health" >/dev/null
for captcha_path in /api/login/captcha /api/member/login/captcha; do
  captcha_payload="$(curl -fsS -H "Origin: $ADMIN_URL" "$BACKEND_URL$captcha_path")"
  jq -e '
    (.data // .) as $captcha |
    ($captcha.id | type == "string" and length == 32) and
    ($captcha.image | type == "string" and startswith("data:image/png;base64,")) and
    ($captcha.expires_in | type == "number" and . > 0)
  ' <<<"$captcha_payload" >/dev/null || {
    echo "验证码接口异常：$captcha_path" >&2
    exit 1
  }
done
enabled_count="$(curl -fsS "$BACKEND_URL/api/public/lottery/games/enabled" | jq '.data | length')"
if [[ "$enabled_count" != "22" ]]; then
  echo "启用彩种数量异常：期望 22，实际 $enabled_count" >&2
  exit 1
fi
curl -fsS "$MEMBER_URL" >/dev/null
curl -fsS "$ADMIN_URL" >/dev/null
echo "健康检查通过：PostgreSQL、后端 ${BACKEND_SERVER_PORT}、双端验证码、用户端 5173、后台 5174；启用彩种 22 个"
