#!/usr/bin/env bash
set -Eeuo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
suite="${SYSTEM_TEST_SUITE:-all}"

fail() {
  echo "release system test: $*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "缺少命令 $1"
}

case "$suite" in
  all|integration|e2e|load) ;;
  *) fail "SYSTEM_TEST_SUITE 必须是 all/integration/e2e/load" ;;
esac

[[ "${SYSTEM_TEST_ALLOW_LOCAL:-}" == "1" ]] || fail "必须显式设置 SYSTEM_TEST_ALLOW_LOCAL=1"
postgres_host="${SYSTEM_TEST_POSTGRES_HOST:-127.0.0.1}"
case "$postgres_host" in
  127.0.0.1|localhost|::1) ;;
  *) fail "fresh-DB 测试只允许本机 PostgreSQL，当前为 $postgres_host" ;;
esac

for command in createdb dropdb dropuser psql redis-cli curl openssl node; do
  require_command "$command"
done
if [[ "$suite" == "all" || "$suite" == "e2e" ]]; then
  require_command npm
fi

postgres_port="${SYSTEM_TEST_POSTGRES_PORT:-5432}"
postgres_admin_user="${SYSTEM_TEST_POSTGRES_ADMIN_USER:-wangzhe_ci_admin}"
postgres_admin_password="${SYSTEM_TEST_POSTGRES_ADMIN_PASSWORD:-}"
postgres_app_password="${SYSTEM_TEST_POSTGRES_APP_PASSWORD:-}"
postgres_admin_database="${SYSTEM_TEST_POSTGRES_ADMIN_DATABASE:-postgres}"
redis_addr="${SYSTEM_TEST_REDIS_ADDR:-127.0.0.1:6379}"
redis_username="${SYSTEM_TEST_REDIS_USERNAME:-wangzhe-app}"
redis_password="${SYSTEM_TEST_REDIS_PASSWORD:-}"
redis_monitor_username="${SYSTEM_TEST_REDIS_MONITOR_USERNAME:-wangzhe-monitor}"
redis_monitor_password="${SYSTEM_TEST_REDIS_MONITOR_PASSWORD:-}"
backend_port="${SYSTEM_TEST_BACKEND_PORT:-18080}"
member_port="${E2E_MEMBER_PORT:-4173}"
admin_port="${E2E_ADMIN_PORT:-4174}"
release_dir="${SYSTEM_TEST_RELEASE_DIR:-$repository_root/release}"
run_id="${SYSTEM_TEST_RUN_ID:-${GITHUB_RUN_ID:-local}_${GITHUB_RUN_ATTEMPT:-0}_$$}"
run_id="$(printf '%s' "$run_id" | tr '[:upper:]-.' '[:lower:]___' | tr -cd 'a-z0-9_')"
database_name="${SYSTEM_TEST_DATABASE_NAME:-wangzhe_ci_${run_id}}"
postgres_app_user="wangzhe_ci_app_${run_id}"

[[ -n "$postgres_admin_password" ]] || fail "SYSTEM_TEST_POSTGRES_ADMIN_PASSWORD 不能为空"
[[ "$postgres_app_password" =~ ^[A-Za-z0-9#_.:@%+=,-]{24,72}$ ]] || fail "SYSTEM_TEST_POSTGRES_APP_PASSWORD 必须是 24-72 位安全测试密码"
[[ -n "$redis_password" ]] || fail "SYSTEM_TEST_REDIS_PASSWORD 不能为空"
[[ -n "$redis_monitor_password" ]] || fail "SYSTEM_TEST_REDIS_MONITOR_PASSWORD 不能为空"
(( ${#redis_password} >= 24 )) || fail "SYSTEM_TEST_REDIS_PASSWORD 至少 24 位"
(( ${#redis_monitor_password} >= 24 )) || fail "SYSTEM_TEST_REDIS_MONITOR_PASSWORD 至少 24 位"
[[ "$redis_username" =~ ^[A-Za-z0-9_.-]{1,64}$ && "$redis_monitor_username" =~ ^[A-Za-z0-9_.-]{1,64}$ ]] || fail "Redis ACL 用户名无效"
[[ "$redis_username" != "$redis_monitor_username" && "$redis_password" != "$redis_monitor_password" ]] || fail "Redis 应用与监控凭据必须独立"
[[ "$postgres_port" =~ ^[0-9]+$ ]] && (( postgres_port > 0 && postgres_port <= 65535 )) || fail "PostgreSQL 端口无效"
[[ "$backend_port" =~ ^[0-9]+$ ]] && (( backend_port > 1024 && backend_port <= 65535 )) || fail "后端测试端口无效"
[[ "$member_port" =~ ^[0-9]+$ ]] && (( member_port > 1024 && member_port <= 65535 )) || fail "会员 E2E 端口无效"
[[ "$admin_port" =~ ^[0-9]+$ ]] && (( admin_port > 1024 && admin_port <= 65535 )) || fail "管理 E2E 端口无效"
[[ "$postgres_admin_database" =~ ^[a-zA-Z0-9_]+$ ]] || fail "PostgreSQL maintenance 数据库名无效"
[[ "$postgres_admin_user" =~ ^[a-zA-Z_][a-zA-Z0-9_]{0,62}$ ]] || fail "PostgreSQL 管理账号名无效"
[[ "$database_name" =~ ^wangzhe_ci_[a-z0-9_]+$ ]] || fail "测试数据库名必须以 wangzhe_ci_ 开头且只含小写字母、数字和下划线"
[[ "$postgres_app_user" =~ ^wangzhe_ci_app_[a-z0-9_]+$ ]] || fail "一次性应用角色名无效"
(( ${#database_name} <= 63 )) || fail "测试数据库名超过 PostgreSQL 63 字节限制"
(( ${#postgres_app_user} <= 63 )) || fail "一次性应用角色名超过 PostgreSQL 63 字节限制"
[[ -d "$release_dir" && ! -L "$release_dir" ]] || fail "release 目录不存在或不安全：$release_dir"
release_dir="$(cd "$release_dir" && pwd -P)"

[[ "$redis_addr" =~ ^(127\.0\.0\.1|localhost):[0-9]+$ ]] || \
  fail "fresh-DB 测试只允许 host:port 形式的本机 Redis，当前为 $redis_addr"
redis_host="${redis_addr%:*}"
redis_port="${redis_addr##*:}"
(( redis_port > 0 && redis_port <= 65535 )) || fail "Redis 端口无效"

runtime_dir="$(mktemp -d "${TMPDIR:-/tmp}/wangzhe-release-system.XXXXXX")"
database_created=0
role_created=0
backend_pid=""
edge_pid=""

stop_process() {
  local pid="$1"
  [[ -n "$pid" ]] || return 0
  if kill -0 "$pid" 2>/dev/null; then
    kill "$pid" 2>/dev/null || true
    for _ in {1..50}; do
      if ! kill -0 "$pid" 2>/dev/null; then
        wait "$pid" 2>/dev/null || true
        return 0
      fi
      sleep 0.1
    done
    kill -KILL "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
}

cleanup() {
  local exit_code=$?
  trap - EXIT INT TERM
  stop_process "$edge_pid"
  stop_process "$backend_pid"
  if (( database_created == 1 )); then
    PGPASSWORD="$postgres_admin_password" dropdb --if-exists --force \
      --host "$postgres_host" --port "$postgres_port" --username "$postgres_admin_user" \
      --maintenance-db "$postgres_admin_database" "$database_name" >/dev/null 2>&1 || \
      echo "警告：未能删除一次性测试数据库 $database_name" >&2
  fi
  if (( role_created == 1 )); then
    PGPASSWORD="$postgres_admin_password" PGDATABASE="$postgres_admin_database" dropuser --if-exists \
      --host "$postgres_host" --port "$postgres_port" --username "$postgres_admin_user" \
      "$postgres_app_user" >/dev/null 2>&1 || \
      echo "警告：未能删除一次性测试角色 $postgres_app_user" >&2
  fi
  rm -rf -- "$runtime_dir"
  exit "$exit_code"
}
trap cleanup EXIT INT TERM

echo "[1/9] 先验证待测 release/SHA256SUMS"
bash "$repository_root/scripts/release-integrity.sh" verify "$release_dir"
[[ -x "$release_dir/bin/wangzhe-backend" && -x "$release_dir/bin/wangzhe-bootstrap-admin" ]] || \
  fail "release/bin 缺少可执行后端或管理员引导工具"
[[ -f "$release_dir/member/index.html" && -f "$release_dir/admin/index.html" ]] || \
  fail "release/member 或 release/admin 缺少前端入口"

echo "[2/9] 验证真实 Redis 版本与 AOF"
export REDISCLI_AUTH="$redis_monitor_password"
redis_monitor_args=(--no-auth-warning --user "$redis_monitor_username" -h "$redis_host" -p "$redis_port")
unauthenticated_ping="$(env -u REDISCLI_AUTH redis-cli "${redis_monitor_args[@]}" --raw PING 2>/dev/null || true)"
[[ "$unauthenticated_ping" != "PONG" ]] || fail "Redis 未启用认证，拒绝以非生产形态继续"
redis_version="$(redis-cli "${redis_monitor_args[@]}" --raw INFO server | sed -n 's/^redis_version://p' | tr -d '\r' | head -n 1)"
[[ "$redis_version" =~ ^([1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})$ ]] || \
  fail "Redis 版本格式必须是有限纯数字 major.minor.patch，当前为 ${redis_version:-<empty>}"
redis_major="${BASH_REMATCH[1]}"
redis_minor="${BASH_REMATCH[2]}"
if (( redis_major < 6 || (redis_major == 6 && redis_minor < 2) )); then
  fail "Redis 必须为 6.2+，当前为 $redis_version"
fi
appendonly="$(redis-cli "${redis_monitor_args[@]}" --raw CONFIG GET appendonly | tail -n 1 | tr -d '\r')"
[[ "$appendonly" == "yes" ]] || fail "Redis AOF 未启用（appendonly=${appendonly}）"

echo "[3/9] 创建 NOSUPERUSER 一次性应用角色与真实 PostgreSQL 数据库 $database_name"
PGPASSWORD="$postgres_admin_password" psql --no-psqlrc --set ON_ERROR_STOP=1 \
  --host "$postgres_host" --port "$postgres_port" --username "$postgres_admin_user" --dbname "$postgres_admin_database" \
  --command "CREATE ROLE \"$postgres_app_user\" LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD '$postgres_app_password';"
role_created=1
PGPASSWORD="$postgres_admin_password" createdb \
  --host "$postgres_host" --port "$postgres_port" --username "$postgres_admin_user" \
  --maintenance-db "$postgres_admin_database" --owner "$postgres_app_user" "$database_name"
database_created=1
role_flags="$(PGPASSWORD="$postgres_admin_password" psql --no-psqlrc --tuples-only --no-align --set ON_ERROR_STOP=1 \
  --host "$postgres_host" --port "$postgres_port" --username "$postgres_admin_user" --dbname "$postgres_admin_database" \
  --command "SELECT rolsuper || ':' || rolcreatedb || ':' || rolcreaterole || ':' || rolreplication || ':' || rolbypassrls FROM pg_roles WHERE rolname='$postgres_app_user';" | tr -d '[:space:]')"
[[ "$role_flags" == "false:false:false:false:false" ]] || fail "一次性应用角色权限过高：$role_flags"

echo "[4/9] 使用已经校验的 release/bin，不从源码重新编译"
backend_binary="$release_dir/bin/wangzhe-backend"
bootstrap_binary="$release_dir/bin/wangzhe-bootstrap-admin"

admin_username="${E2E_ADMIN_USERNAME:-e2e_platform}"
admin_password="${E2E_ADMIN_PASSWORD:-ProdBootstrap#2026_x9Q}"
password_file="$runtime_dir/bootstrap-password"
umask 077
printf '%s\n' "$admin_password" > "$password_file"
chmod 0600 "$password_file"

backend_origin="http://127.0.0.1:$backend_port"
backend_environment=(
  "BACKEND_SERVER_BIND=127.0.0.1"
  "BACKEND_SERVER_PORT=$backend_port"
  "BACKEND_SERVER_MODE=release"
  "BACKEND_SERVER_ALLOWED_ORIGINS=https://127.0.0.1:$member_port,https://127.0.0.1:$admin_port"
  "BACKEND_SERVER_TRUSTED_PROXIES=127.0.0.1"
  "BACKEND_DATABASE_HOST=$postgres_host"
  "BACKEND_DATABASE_PORT=$postgres_port"
  "BACKEND_DATABASE_USER=$postgres_app_user"
  "BACKEND_DATABASE_PASSWORD=$postgres_app_password"
  "BACKEND_DATABASE_DBNAME=$database_name"
  "BACKEND_DATABASE_SSLMODE=disable"
  "BACKEND_REDIS_ADDR=$redis_addr"
  "BACKEND_REDIS_USERNAME=$redis_username"
  "BACKEND_REDIS_PASSWORD=$redis_password"
  "BACKEND_REDIS_DB=0"
  "BACKEND_REDIS_TLS=false"
  # Every run receives a fresh namespace on the real Redis instance. This
  # prevents rate limits and sessions persisted by AOF in an earlier run from
  # weakening or flaking the fresh-environment assertions below.
  "BACKEND_REDIS_PREFIX=wangzhe-system-test:$run_id"
  "BACKEND_JWT_SECRET=CiJWT#7uQ9mL2vR8xK4pN6sT1dF5hJ3wZ0bE"
  "BACKEND_JWT_EXPIRE=3600"
  "BACKEND_SECURITY_DATA_ENCRYPTION_KEY=CiData#9pW3nX6kR1sV8qM4tH7jL2yF5dB0cG"
  "BACKEND_UPLOAD_DIR=$runtime_dir/uploads"
  "BACKEND_AUDIT_FALLBACK_FILE=$runtime_dir/audit-fallback.jsonl"
  # Explicitly exercise the controlled production robot gate.  The fixture
  # creates no robot profiles, so the scheduler cannot place a bet; the API
  # test below verifies the one-workspace cap and transaction rollback.
  "BACKEND_ROOM_ACTIVITY=1"
  "BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES=1"
)

echo "[5/9] 在空库执行版本化迁移并安全创建首位管理员"
(cd "$runtime_dir" && env "${backend_environment[@]}" "$bootstrap_binary" \
  --username "$admin_username" --password-file "$password_file" --nickname "E2E 平台管理员")

psql_app() {
  PGPASSWORD="$postgres_app_password" psql --no-psqlrc --set ON_ERROR_STOP=1 --tuples-only --no-align \
    --host "$postgres_host" --port "$postgres_port" --username "$postgres_app_user" --dbname "$database_name" "$@"
}

expected_migrations="$(find "$repository_root/backend/migrations" -maxdepth 1 -type f -name '*.sql' | wc -l | tr -d ' ')"
applied_migrations="$(psql_app --command 'SELECT count(*) FROM schema_migrations' | tr -d '[:space:]')"
[[ "$applied_migrations" == "$expected_migrations" ]] || fail "迁移数量不一致：$applied_migrations/$expected_migrations"
admin_count="$(psql_app --command "SELECT count(*) FROM \"user\" WHERE role='admin' AND status=1" | tr -d '[:space:]')"
[[ "$admin_count" == "1" ]] || fail "首位有效管理员数量应为 1，实际为 $admin_count"
demo_count="$(psql_app --command "SELECT count(*) FROM \"user\" WHERE username IN ('admin','wangzhe88','suyang','wangzhetenant')" | tr -d '[:space:]')"
[[ "$demo_count" == "0" ]] || fail "release fresh DB 出现开发体验账号"
if (cd "$runtime_dir" && env "${backend_environment[@]}" "$bootstrap_binary" \
  --username "second_admin" --password-file "$password_file" >"$runtime_dir/second-bootstrap.log" 2>&1); then
  fail "管理员引导工具错误地创建了第二位管理员"
fi
grep -q '已存在管理员' "$runtime_dir/second-bootstrap.log" || fail "第二次管理员引导未以预期原因失败"

start_backend() {
  (cd "$runtime_dir" && exec env "${backend_environment[@]}" "$backend_binary") \
    >"$runtime_dir/backend.log" 2>&1 &
  backend_pid=$!
}

wait_for_ready() {
  for _ in {1..120}; do
    if curl --silent --show-error --fail "$backend_origin/ready" >/dev/null 2>&1; then
      return 0
    fi
    if ! kill -0 "$backend_pid" 2>/dev/null; then
      sed -n '1,240p' "$runtime_dir/backend.log" >&2
      fail "release 后端在 ready 前退出"
    fi
    sleep 0.25
  done
  sed -n '1,240p' "$runtime_dir/backend.log" >&2
  fail "release 后端未在 30 秒内 ready"
}

echo "[6/9] 启动、探测并重启 release 后端"
start_backend
wait_for_ready
curl --silent --show-error --fail "$backend_origin/health" >/dev/null
stop_process "$backend_pid"
backend_pid=""
start_backend
wait_for_ready

echo "[7/9] 验证 API、release 注册边界并创建一次性验收数据"
SYSTEM_TEST_ALLOW_LOCAL=1 SYSTEM_TEST_BACKEND_ORIGIN="$backend_origin" \
  E2E_ADMIN_USERNAME="$admin_username" E2E_ADMIN_PASSWORD="$admin_password" \
  node "$repository_root/tests/system/release-api.mjs"

robot_bet_count="$(psql_app --command 'SELECT count(*) FROM lottery_bets AS bet JOIN workspace_robot_profiles AS profile ON profile.workspace_id = bet.workspace_id AND profile.user_id = bet.user_id;' | tr -d '[:space:]')"
[[ "$robot_bet_count" =~ ^[1-9][0-9]*$ ]] || fail "fresh-DB 机器人真实下注链路没有生成注单"
robot_settled_count="$(psql_app --command "SELECT count(*) FROM lottery_bets AS bet JOIN workspace_robot_profiles AS profile ON profile.workspace_id = bet.workspace_id AND profile.user_id = bet.user_id WHERE bet.status IN ('won','lost') AND bet.settled_at IS NOT NULL;" | tr -d '[:space:]')"
[[ "$robot_settled_count" =~ ^[1-9][0-9]*$ ]] || fail "fresh-DB 机器人真实注单没有完成结算"
robot_financial_leaks="$(psql_app --command 'SELECT count(*) FROM lottery_bets AS bet JOIN workspace_robot_profiles AS profile ON profile.workspace_id = bet.workspace_id AND profile.user_id = bet.user_id WHERE bet.fly_cents <> 0 OR bet.rebate_rate_snapshot <> 0 OR bet.rebate_cents <> 0 OR bet.agent_share_rate_snapshot <> 0 OR bet.agent_share_cents <> 0;' | tr -d '[:space:]')"
[[ "$robot_financial_leaks" == "0" ]] || fail "机器人注单进入了飞单、返水或代理分成链路：$robot_financial_leaks"

if [[ "$suite" == "all" || "$suite" == "e2e" ]]; then
  [[ -x "$repository_root/tests/e2e/node_modules/.bin/playwright" ]] || \
    fail "Playwright 未安装；先运行 make production-test-install"

  echo "[8/9] 代理已经校验的 release/member 与 release/admin 并执行 HTTPS 浏览器 E2E"
  tls_key="$runtime_dir/e2e.key"
  tls_cert="$runtime_dir/e2e.crt"
  openssl req -x509 -newkey rsa:2048 -sha256 -nodes -days 1 \
    -subj '/CN=127.0.0.1' -addext 'subjectAltName=IP:127.0.0.1,DNS:localhost' \
    -keyout "$tls_key" -out "$tls_cert" >/dev/null 2>&1
  SYSTEM_TEST_BACKEND_ORIGIN="$backend_origin" SYSTEM_TEST_TLS_KEY="$tls_key" SYSTEM_TEST_TLS_CERT="$tls_cert" \
    SYSTEM_TEST_MEMBER_ROOT="$release_dir/member" SYSTEM_TEST_ADMIN_ROOT="$release_dir/admin" \
    E2E_MEMBER_PORT="$member_port" E2E_ADMIN_PORT="$admin_port" \
    node "$repository_root/tests/e2e/serve.mjs" >"$runtime_dir/edge.log" 2>&1 &
  edge_pid=$!
  edge_ready=0
  for _ in {1..80}; do
    if curl --silent --show-error --fail --insecure "https://127.0.0.1:$member_port/health" >/dev/null 2>&1 && \
       curl --silent --show-error --fail --insecure "https://127.0.0.1:$admin_port/health" >/dev/null 2>&1; then
      edge_ready=1
      break
    fi
    if ! kill -0 "$edge_pid" 2>/dev/null; then
      sed -n '1,160p' "$runtime_dir/edge.log" >&2
      fail "HTTPS E2E 代理启动失败"
    fi
    sleep 0.25
  done
  if (( edge_ready != 1 )); then
    sed -n '1,160p' "$runtime_dir/edge.log" >&2
    fail "HTTPS E2E 代理未在 20 秒内 ready"
  fi
  (cd "$repository_root/tests/e2e" && \
    E2E_MEMBER_BASE_URL="https://127.0.0.1:$member_port" E2E_ADMIN_BASE_URL="https://127.0.0.1:$admin_port" \
    E2E_ADMIN_USERNAME="$admin_username" E2E_ADMIN_PASSWORD="$admin_password" \
    npm test)
else
  echo "[8/9] 跳过浏览器 E2E（SYSTEM_TEST_SUITE=${suite}）"
fi

if [[ "$suite" == "all" || "$suite" == "load" ]]; then
  echo "[9/9] 执行安全 HTTP/WebSocket/关键读写负载烟测"
  [[ -d "$repository_root/tests/system/node_modules/ws" ]] || \
    fail "负载测试依赖未安装；先运行 make production-test-install"
  SYSTEM_TEST_ALLOW_LOCAL=1 SYSTEM_TEST_BACKEND_ORIGIN="$backend_origin" \
    node "$repository_root/tests/system/load-smoke.mjs"
else
  echo "[9/9] 跳过负载烟测（SYSTEM_TEST_SUITE=${suite}）"
fi

echo "release system test passed ($suite; PostgreSQL database $database_name; Redis $redis_version with AOF)"
