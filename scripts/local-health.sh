#!/usr/bin/env bash
set -euo pipefail

BACKEND_URL="${BACKEND_URL:-http://127.0.0.1:8080}"
MEMBER_URL="${MEMBER_URL:-http://127.0.0.1:5173}"
ADMIN_URL="${ADMIN_URL:-http://127.0.0.1:5174}"
PG_HOST="${BACKEND_DATABASE_HOST:-localhost}"
PG_PORT="${BACKEND_DATABASE_PORT:-5432}"
PG_USER="${BACKEND_DATABASE_USER:-postgres}"
PG_DB="${BACKEND_DATABASE_DBNAME:-backend}"

if command -v pg_isready >/dev/null 2>&1; then
  pg_isready -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d "$PG_DB" >/dev/null
elif [[ -x /Library/PostgreSQL/17/bin/pg_isready ]]; then
  /Library/PostgreSQL/17/bin/pg_isready -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d "$PG_DB" >/dev/null
else
  echo "未找到 pg_isready，跳过 PostgreSQL 命令行健康检查"
fi

curl -fsS "$BACKEND_URL/health" >/dev/null
enabled_count="$(curl -fsS "$BACKEND_URL/api/public/lottery/games/enabled" | jq '.data | length')"
if [[ "$enabled_count" != "22" ]]; then
  echo "启用彩种数量异常：期望 22，实际 $enabled_count" >&2
  exit 1
fi
curl -fsS "$MEMBER_URL" >/dev/null
curl -fsS "$ADMIN_URL" >/dev/null
echo "健康检查通过：PostgreSQL、后端 8080、用户端 5173、后台 5174；启用彩种 22 个"
