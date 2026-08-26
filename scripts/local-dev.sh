#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_DIR="$ROOT_DIR/.local-logs"
mkdir -p "$LOG_DIR"

for command_name in go npm curl jq; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done

if command -v pg_isready >/dev/null 2>&1; then
  pg_isready -h "${BACKEND_DATABASE_HOST:-localhost}" -p "${BACKEND_DATABASE_PORT:-5432}" -U "${BACKEND_DATABASE_USER:-postgres}" -d "${BACKEND_DATABASE_DBNAME:-backend}" >/dev/null
elif [[ -x /Library/PostgreSQL/17/bin/pg_isready ]]; then
  /Library/PostgreSQL/17/bin/pg_isready -h "${BACKEND_DATABASE_HOST:-localhost}" -p "${BACKEND_DATABASE_PORT:-5432}" -U "${BACKEND_DATABASE_USER:-postgres}" -d "${BACKEND_DATABASE_DBNAME:-backend}" >/dev/null
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
  echo "$name 正在启动：端口 $port，日志 $LOG_DIR/$name.log"
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
