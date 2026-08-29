#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_DIR="$ROOT_DIR/.local-logs"
mkdir -p "$LOG_DIR"

# A clean checkout intentionally has no tracked config.yaml. Keep all local
# defaults here so `make dev` is reproducible, while still allowing developers
# to override any value from their shell. These credentials are debug-only;
# release validation rejects them.
export BACKEND_SERVER_BIND="${BACKEND_SERVER_BIND:-0.0.0.0}"
export BACKEND_SERVER_PORT="${BACKEND_SERVER_PORT:-8080}"
export BACKEND_SERVER_MODE="${BACKEND_SERVER_MODE:-debug}"
export BACKEND_DATABASE_HOST="${BACKEND_DATABASE_HOST:-localhost}"
export BACKEND_DATABASE_PORT="${BACKEND_DATABASE_PORT:-5432}"
export BACKEND_DATABASE_USER="${BACKEND_DATABASE_USER:-postgres}"
export BACKEND_DATABASE_PASSWORD="${BACKEND_DATABASE_PASSWORD:-123456}"
export BACKEND_DATABASE_DBNAME="${BACKEND_DATABASE_DBNAME:-backend}"
export BACKEND_DATABASE_SSLMODE="${BACKEND_DATABASE_SSLMODE:-disable}"
export BACKEND_JWT_SECRET="${BACKEND_JWT_SECRET:-backend_jwt_secret_key_2024}"
export BACKEND_JWT_EXPIRE="${BACKEND_JWT_EXPIRE:-86400}"
export BACKEND_SECURITY_DATA_ENCRYPTION_KEY="${BACKEND_SECURITY_DATA_ENCRYPTION_KEY:-local-data-encryption-key-7xlottery-dev-2026}"

for command_name in go npm curl jq; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done

postgres_ready_cmd="$(command -v pg_isready || true)"
if [[ -z "$postgres_ready_cmd" ]]; then
  for postgres_bin_dir in /Library/PostgreSQL/*/bin; do
    if [[ -x "$postgres_bin_dir/pg_isready" ]]; then
      postgres_ready_cmd="$postgres_bin_dir/pg_isready"
      break
    fi
  done
fi

if [[ -n "$postgres_ready_cmd" ]]; then
  "$postgres_ready_cmd" -h "$BACKEND_DATABASE_HOST" -p "$BACKEND_DATABASE_PORT" -U "$BACKEND_DATABASE_USER" -d "$BACKEND_DATABASE_DBNAME" >/dev/null
else
  echo "未找到 pg_isready，请先确认 PostgreSQL 已启动" >&2
  exit 1
fi

pids=()
start_if_free() {
  local port="$1"
  local name="$2"
  local directory="$3"
  shift 3
  if lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "$name 已在端口 $port 运行"
    return
  fi
  (
    cd "$directory"
    exec "$@"
  ) >"$LOG_DIR/$name.log" 2>&1 &
  pids+=("$!")
  echo "$name 正在启动：端口 ${port}，日志 $LOG_DIR/$name.log"
}

start_if_free 8080 backend "$ROOT_DIR/backend" go run main.go
start_if_free 5173 member "$ROOT_DIR/new" npm run dev -- --host 0.0.0.0 --port 5173
start_if_free 5174 admin "$ROOT_DIR/new-back" npm run dev -- --host 0.0.0.0 --port 5174

for _ in {1..30}; do
  if curl -fsS http://127.0.0.1:8080/health >/dev/null 2>&1 \
    && curl -fsS http://127.0.0.1:5173 >/dev/null 2>&1 \
    && curl -fsS http://127.0.0.1:5174 >/dev/null 2>&1; then
    "$ROOT_DIR/scripts/local-health.sh"
    break
  fi
  sleep 1
done

echo "用户端：http://127.0.0.1:5173  后台：http://127.0.0.1:5174  API：http://127.0.0.1:8080"
if [[ "${#pids[@]}" -eq 0 ]]; then
  exit 0
fi
trap 'kill "${pids[@]}" 2>/dev/null || true' INT TERM EXIT
wait
