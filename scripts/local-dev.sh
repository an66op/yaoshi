#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_DIR="$ROOT_DIR/.local-logs"
mkdir -p "$LOG_DIR"

if (( $# > 1 )); then
  echo "用法：scripts/local-dev.sh [ENV_FILE]" >&2
  exit 1
fi

# A tracked config.yaml is intentionally not required. Parse an optional
# ENV_FILE as data (never as shell code) and share the same debug defaults as
# local-init/local-smoke.
# shellcheck source=scripts/lib/backend-env.sh
source "$ROOT_DIR/scripts/lib/backend-env.sh"
load_optional_backend_env "${1:-}"
apply_local_backend_defaults
require_local_backend_target

for command_name in go npm curl jq lsof ps; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done

export BACKEND_URL="${BACKEND_URL:-http://127.0.0.1:${BACKEND_SERVER_PORT}}"
export MEMBER_URL="${MEMBER_URL:-http://127.0.0.1:5173}"
export ADMIN_URL="${ADMIN_URL:-http://127.0.0.1:5174}"
require_loopback_http_origin BACKEND_URL "$BACKEND_URL" "$BACKEND_SERVER_PORT"
require_loopback_http_origin MEMBER_URL "$MEMBER_URL" 5173
require_loopback_http_origin ADMIN_URL "$ADMIN_URL" 5174
unset VITE_API_BASE_URL
export VITE_API_PORT="$BACKEND_SERVER_PORT"

psql_cmd="$(find_postgres_tool psql)"
postgres_ready_cmd="$(find_postgres_tool pg_isready)"
PGPASSWORD="$BACKEND_DATABASE_PASSWORD" PGSSLMODE="$BACKEND_DATABASE_SSLMODE" \
  "$postgres_ready_cmd" -h "$BACKEND_DATABASE_HOST" -p "$BACKEND_DATABASE_PORT" \
  -U "$BACKEND_DATABASE_USER" -d postgres >/dev/null || {
    echo "本机 PostgreSQL 维护库未就绪；请先启动 PostgreSQL 并检查 BACKEND_DATABASE_*" >&2
    exit 1
  }
require_local_postgres_server "$psql_cmd"
require_completed_local_database "$psql_cmd"

pids=()
cleanup() {
  if [[ "${#pids[@]}" -gt 0 ]]; then
    kill "${pids[@]}" 2>/dev/null || true
  fi
}
trap cleanup INT TERM EXIT

start_if_free() {
  local port="$1"
  local name="$2"
  local directory="$3"
  shift 3
  if lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "端口 $port 已被其他进程占用，拒绝把它误认为本次 $name；请先停止占用进程" >&2
    exit 1
  fi
  (
    cd "$directory"
    exec "$@"
  ) >"$LOG_DIR/$name.log" 2>&1 &
  pids+=("$!")
  echo "$name 正在启动：端口 ${port}，日志 $LOG_DIR/$name.log"
}

start_if_free "$BACKEND_SERVER_PORT" backend "$ROOT_DIR/backend" go run main.go
# Frontend build tools need only VITE_* values. Remove every current and future
# backend setting dynamically so adding a new secret cannot leak it to npm by
# omission from a manually maintained denylist.
frontend_env=(env -u PGPASSWORD -u ENV_FILE)
while IFS= read -r backend_environment_name; do
  [[ "$backend_environment_name" == BACKEND_* ]] || continue
  frontend_env+=(-u "$backend_environment_name")
done < <(compgen -e)
start_if_free 5173 member "$ROOT_DIR/new" "${frontend_env[@]}" npm run dev -- --host 0.0.0.0 --port 5173
start_if_free 5174 admin "$ROOT_DIR/new-back" "${frontend_env[@]}" npm run dev -- --host 0.0.0.0 --port 5174

ready=false
for _ in {1..45}; do
  if curl -fsS "$BACKEND_URL/health" >/dev/null 2>&1 \
    && curl -fsS "$MEMBER_URL" >/dev/null 2>&1 \
    && curl -fsS "$ADMIN_URL" >/dev/null 2>&1; then
    "$ROOT_DIR/scripts/local-health.sh"
    ready=true
    break
  fi
  sleep 1
done

if [[ "$ready" != "true" ]]; then
  echo "本地服务在 45 秒内没有全部就绪，启动失败。最近日志：" >&2
  for log_file in "$LOG_DIR"/backend.log "$LOG_DIR"/member.log "$LOG_DIR"/admin.log; do
    if [[ -f "$log_file" ]]; then
      echo "日志：$log_file" >&2
      tail -n 80 "$log_file" >&2
    fi
  done
  exit 1
fi

echo "用户端：$MEMBER_URL  后台：$ADMIN_URL  API：$BACKEND_URL"
if [[ "${#pids[@]}" -eq 0 ]]; then
  exit 0
fi
wait
