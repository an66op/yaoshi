#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib/backend-env.sh
source "$ROOT_DIR/scripts/lib/backend-env.sh"

usage() {
  echo "用法：scripts/local-init.sh [ENV_FILE]" >&2
}

if (( $# > 1 )); then
  usage
  exit 1
fi

load_optional_backend_env "${1:-}"
apply_local_backend_defaults
require_local_backend_target

for command_name in go node npm grep od lsof ps ln mkfifo; do
  command -v "$command_name" >/dev/null 2>&1 || {
    echo "缺少命令：$command_name" >&2
    exit 1
  }
done

node_version="$(node --version)"
node_version="${node_version#v}"
IFS=. read -r node_major node_minor _ <<<"${node_version%%-*}"
npm_version="$(npm --version)"
IFS=. read -r npm_major npm_minor _ <<<"${npm_version%%-*}"
if (( 10#$node_major != 24 || 10#$node_minor < 20 )); then
  echo "Node.js 版本必须为 >=24.20.0 且 <25（当前 ${node_version}）；请在仓库根目录执行 nvm use" >&2
  exit 1
fi
if (( 10#$npm_major != 11 || 10#$npm_minor < 19 )); then
  echo "npm 版本必须为 >=11.19.0 且 <12（当前 ${npm_version}）；请在仓库根目录执行 nvm use" >&2
  exit 1
fi

psql_cmd="$(find_postgres_tool psql)"
createdb_cmd="$(find_postgres_tool createdb)"
postgres_ready_cmd="$(find_postgres_tool pg_isready)"
development_marker_namespace="wangzhe-local-development-v1"
state_dir="$ROOT_DIR/.local-state"
receipt_path=""
pending_nonce=""
pending_database_oid=""
temporary_receipt_path=""
init_lock_pid=""
init_lock_fifo=""
init_lock_output=""
init_lock_error=""
init_lock_token=""
init_lock_backend_pid=""
init_lock_acquired="false"

if [[ -e "$state_dir" && ( ! -d "$state_dir" || -L "$state_dir" ) ]]; then
  echo "本地初始化状态路径必须是普通目录且不能是符号链接：$state_dir" >&2
  exit 1
fi
mkdir -p "$state_dir"
chmod 700 "$state_dir"
state_dir_owner="$(backend_env_stat '%u' '%u' "$state_dir")"
expected_state_dir_owner="${EUID:-$(id -u)}"
[[ "$state_dir_owner" == "$expected_state_dir_owner" ]] || {
  echo "本地初始化状态目录必须属于当前用户：$state_dir" >&2
  exit 1
}

cleanup_local_init_state() {
  if [[ -n "$temporary_receipt_path" && -f "$temporary_receipt_path" && ! -L "$temporary_receipt_path" ]]; then
    rm -f -- "$temporary_receipt_path"
  fi
  if [[ -n "$init_lock_pid" ]]; then
    printf '\\q\n' >&9 2>/dev/null || true
    exec 9>&-
    wait "$init_lock_pid" 2>/dev/null || true
  fi
  local init_lock_artifact
  for init_lock_artifact in "$init_lock_fifo" "$init_lock_output" "$init_lock_error"; do
    if [[ -n "$init_lock_artifact" && -e "$init_lock_artifact" && ! -L "$init_lock_artifact" ]]; then
      rm -f -- "$init_lock_artifact"
    fi
  done
}

trap cleanup_local_init_state EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

acquire_local_init_lock() {
  init_lock_token="$(od -An -N16 -tx1 /dev/urandom | tr -d '[:space:]')"
  [[ "$init_lock_token" =~ ^[0-9a-f]{32}$ ]] || {
    echo "无法生成本地初始化互斥锁标识" >&2
    return 1
  }
  local lock_prefix="$state_dir/${LOCAL_POSTGRES_SYSTEM_IDENTIFIER}.${BACKEND_DATABASE_DBNAME}.lock.${init_lock_token}"
  init_lock_fifo="${lock_prefix}.in"
  init_lock_output="${lock_prefix}.out"
  init_lock_error="${lock_prefix}.err"
  (
    umask 077
    mkfifo "$init_lock_fifo"
    : > "$init_lock_output"
    : > "$init_lock_error"
  )
  exec 9<>"$init_lock_fifo"
  (
    exec 9>&-
    PGPASSWORD="$BACKEND_DATABASE_PASSWORD" PGSSLMODE="$BACKEND_DATABASE_SSLMODE" \
      PGAPPNAME="wangzhe-local-init-${init_lock_token}" \
      exec "$psql_cmd" -X --no-psqlrc "${postgres_args[@]}" --dbname postgres \
      --set ON_ERROR_STOP=1 --quiet --tuples-only --no-align \
      < "$init_lock_fifo" > "$init_lock_output" 2> "$init_lock_error"
  ) &
  init_lock_pid="$!"
  printf "WITH lock_attempt AS (SELECT pg_try_advisory_lock(hashtextextended('wangzhe-local-init:%s', 0)) AS acquired) SELECT CASE WHEN acquired THEN 'acquired:%s:' || pg_backend_pid()::text ELSE 'busy' END FROM lock_attempt;\n" \
    "$BACKEND_DATABASE_DBNAME" "$init_lock_token" >&9

  local _ lock_confirmation
  for _ in {1..100}; do
    lock_confirmation="$(grep -E "^acquired:${init_lock_token}:[0-9]+$" "$init_lock_output" 2>/dev/null || true)"
    if [[ -n "$lock_confirmation" ]]; then
      init_lock_backend_pid="${lock_confirmation##*:}"
      init_lock_acquired="true"
      return 0
    fi
    if grep -Fqx 'busy' "$init_lock_output"; then
      echo "同一 PostgreSQL 集群和数据库已有本地初始化进程，拒绝并发执行：$BACKEND_DATABASE_DBNAME" >&2
      return 1
    fi
    if ! kill -0 "$init_lock_pid" 2>/dev/null; then
      echo "无法建立 PostgreSQL 本地初始化互斥锁" >&2
      if [[ -s "$init_lock_error" ]]; then
        tail -n 20 "$init_lock_error" >&2
      fi
      return 1
    fi
    sleep 0.1
  done
  echo "等待 PostgreSQL 本地初始化互斥锁响应超时" >&2
  return 1
}

require_local_init_lock() {
  local lock_process_state advisory_lock_count
  [[ "$init_lock_acquired" == "true" && -n "$init_lock_pid" && "$init_lock_backend_pid" =~ ^[0-9]+$ ]] &&
    kill -0 "$init_lock_pid" 2>/dev/null || {
    echo "PostgreSQL 本地初始化互斥锁连接已断开，拒绝继续" >&2
    return 1
  }
  lock_process_state="$(ps -p "$init_lock_pid" -o stat= 2>/dev/null || true)"
  [[ -n "$lock_process_state" && "$lock_process_state" != *Z* ]] || {
    echo "PostgreSQL 本地初始化互斥锁进程已经退出，拒绝继续" >&2
    return 1
  }
  advisory_lock_count="$(
    PGPASSWORD="$BACKEND_DATABASE_PASSWORD" PGSSLMODE="$BACKEND_DATABASE_SSLMODE" \
      "$psql_cmd" -X --no-psqlrc "${postgres_args[@]}" --dbname postgres \
      --tuples-only --no-align \
      --command "SELECT COUNT(*) FROM pg_stat_activity AS activity JOIN pg_locks AS held ON held.pid = activity.pid WHERE activity.pid = ${init_lock_backend_pid} AND activity.application_name = 'wangzhe-local-init-${init_lock_token}' AND held.locktype = 'advisory' AND held.granted" \
      2>/dev/null
  )" || {
    echo "无法复核 PostgreSQL 本地初始化互斥锁，拒绝继续" >&2
    return 1
  }
  [[ "$advisory_lock_count" == "1" ]] || {
    echo "PostgreSQL 本地初始化互斥锁已经丢失，拒绝继续" >&2
    return 1
  }
}

load_pending_receipt() {
  [[ -f "$receipt_path" && ! -L "$receipt_path" ]] || {
    echo "初始化中的数据库缺少本机恢复凭证：$receipt_path" >&2
    return 1
  }
  local mode owner mode_value expected_owner
  mode="$(backend_env_stat '%a' '%Lp' "$receipt_path")"
  owner="$(backend_env_stat '%u' '%u' "$receipt_path")"
  mode_value=$((8#$mode))
  expected_owner="${EUID:-$(id -u)}"
  (( (mode_value & 077) == 0 )) && [[ "$owner" == "$expected_owner" ]] || {
    echo "本机恢复凭证必须属于当前用户且权限为 600/400" >&2
    return 1
  }
  local line key value receipt_version="" receipt_phase="" receipt_cluster="" receipt_database=""
  local receipt_database_oid="" receipt_nonce="" seen=$'\n'
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" == *=* ]] || return 1
    key="${line%%=*}"
    value="${line#*=}"
    [[ "$seen" != *$'\n'"$key"$'\n'* ]] || return 1
    seen+="$key"$'\n'
    case "$key" in
      version) receipt_version="$value" ;;
      phase) receipt_phase="$value" ;;
      system_identifier) receipt_cluster="$value" ;;
      database_name) receipt_database="$value" ;;
      database_oid) receipt_database_oid="$value" ;;
      nonce) receipt_nonce="$value" ;;
      *) return 1 ;;
    esac
  done < "$receipt_path"
  [[ "$receipt_version" == "2" && "$receipt_phase" == "bound" &&
    "$receipt_cluster" == "$LOCAL_POSTGRES_SYSTEM_IDENTIFIER" &&
    "$receipt_database" == "$BACKEND_DATABASE_DBNAME" && "$receipt_database_oid" =~ ^[0-9]+$ &&
    "$receipt_nonce" =~ ^[0-9a-f]{32}$ ]] || {
    echo "本机恢复凭证与当前 PostgreSQL 集群或数据库不匹配" >&2
    return 1
  }
  pending_nonce="$receipt_nonce"
  pending_database_oid="$receipt_database_oid"
}

create_pending_receipt() {
  local database_oid="$1"
  [[ "$database_oid" =~ ^[0-9]+$ ]] || {
    echo "不能为没有数据库 OID 的目标建立恢复凭证" >&2
    return 1
  }
  if [[ -e "$receipt_path" ]]; then
    load_pending_receipt
    return
  fi
  pending_nonce="$(od -An -N16 -tx1 /dev/urandom | tr -d '[:space:]')"
  [[ "$pending_nonce" =~ ^[0-9a-f]{32}$ ]] || {
    echo "无法生成本地初始化恢复凭证" >&2
    return 1
  }
  temporary_receipt_path="${receipt_path}.tmp.${pending_nonce}"
  (
    umask 077
    set -o noclobber
    printf 'version=2\nphase=bound\nsystem_identifier=%s\ndatabase_name=%s\ndatabase_oid=%s\nnonce=%s\n' \
      "$LOCAL_POSTGRES_SYSTEM_IDENTIFIER" "$BACKEND_DATABASE_DBNAME" "$database_oid" "$pending_nonce" \
      > "$temporary_receipt_path"
  )
  if ! ln "$temporary_receipt_path" "$receipt_path" 2>/dev/null; then
    rm -f -- "$temporary_receipt_path"
    temporary_receipt_path=""
    if [[ -e "$receipt_path" ]]; then
      load_pending_receipt
      return
    fi
    echo "无法原子建立本地初始化恢复凭证" >&2
    return 1
  fi
  rm -f -- "$temporary_receipt_path"
  temporary_receipt_path=""
  pending_database_oid="$database_oid"
}

export PGPASSWORD="$BACKEND_DATABASE_PASSWORD"
export PGSSLMODE="$BACKEND_DATABASE_SSLMODE"
postgres_args=(
  --host "$BACKEND_DATABASE_HOST"
  --port "$BACKEND_DATABASE_PORT"
  --username "$BACKEND_DATABASE_USER"
)

echo "[1/4] 检查本机 PostgreSQL、监听进程与应用角色"
"$postgres_ready_cmd" "${postgres_args[@]}" --dbname postgres >/dev/null || {
  echo "PostgreSQL 未就绪，或账号无法连接维护库 postgres；请先启动服务并检查 BACKEND_DATABASE_*" >&2
  exit 1
}
require_local_postgres_server "$psql_cmd"
receipt_path="$state_dir/${LOCAL_POSTGRES_SYSTEM_IDENTIFIER}.${BACKEND_DATABASE_DBNAME}.pending"
acquire_local_init_lock

echo "[2/4] 安装 Go 与前端锁定依赖"
dependency_env=(env -u PGPASSWORD -u ENV_FILE)
while IFS= read -r backend_environment_name; do
  [[ "$backend_environment_name" == BACKEND_* ]] || continue
  dependency_env+=(-u "$backend_environment_name")
done < <(compgen -e)
(
  cd "$ROOT_DIR/backend"
  exec 9>&-
  "${dependency_env[@]}" go mod download
)
(
  cd "$ROOT_DIR/new"
  exec 9>&-
  "${dependency_env[@]}" npm ci --ignore-scripts
)
(
  cd "$ROOT_DIR/new-back"
  exec 9>&-
  "${dependency_env[@]}" npm ci --ignore-scripts
)

echo "[3/4] 准备独立应用数据库 $BACKEND_DATABASE_DBNAME"
# Everything from this recheck through the completed marker is protected by
# the PostgreSQL session lock acquired above. The server releases it
# automatically if this script is killed or the machine loses power.
require_local_init_lock
database_names="$("$psql_cmd" -X --no-psqlrc "${postgres_args[@]}" --dbname postgres --tuples-only --no-align --command 'SELECT datname FROM pg_database')"
database_created_now="false"
if grep -Fqx -- "$BACKEND_DATABASE_DBNAME" <<<"$database_names"; then
  echo "数据库已存在，仅验证初始化凭证；不会删除、清空或覆盖"
else
  if [[ -e "$receipt_path" ]]; then
    echo "目标数据库不存在，但仍有旧恢复凭证；拒绝让未来同名数据库继承该凭证：$receipt_path" >&2
    exit 1
  fi
  # Do not create a receipt before createdb. If createdb fails or its outcome
  # is uncertain, any database it may have created remains deliberately
  # unclaimable by a later run instead of inheriting a name-only receipt.
  if ! "$createdb_cmd" "${postgres_args[@]}" --maintenance-db postgres --template template0 -- "$BACKEND_DATABASE_DBNAME"; then
    echo "创建数据库失败或结果不确定；未留下恢复凭证。若数据库实际已出现，本脚本不会自动接管它" >&2
    exit 1
  fi
  database_created_now="true"
  echo "已创建空数据库 ${BACKEND_DATABASE_DBNAME}（所有者为 ${BACKEND_DATABASE_USER}）"
fi
database_oid="$(
  "$psql_cmd" -X --no-psqlrc "${postgres_args[@]}" --dbname postgres \
    --tuples-only --no-align \
    --command "SELECT oid::text FROM pg_database WHERE datname = '$BACKEND_DATABASE_DBNAME'"
)"
[[ "$database_oid" =~ ^[0-9]+$ ]] || {
  echo "无法把恢复凭证绑定到目标数据库 OID；拒绝继续" >&2
  exit 1
}
if [[ -e "$receipt_path" ]]; then
  load_pending_receipt
  [[ "$pending_database_oid" == "$database_oid" ]] || {
    echo "本机恢复凭证绑定的是另一个数据库实例；拒绝接管当前同名数据库" >&2
    exit 1
  }
elif grep -Fqx -- "$BACKEND_DATABASE_DBNAME" <<<"$database_names"; then
  : # Existing databases without a receipt are evaluated by the marker below.
else
  create_pending_receipt "$database_oid"
fi
if [[ "$database_created_now" == "true" ]]; then
  development_initializing_marker="${development_marker_namespace}:initializing:${LOCAL_POSTGRES_SYSTEM_IDENTIFIER}:${pending_nonce}"
  require_local_init_lock
  "$psql_cmd" -X --no-psqlrc "${postgres_args[@]}" --dbname postgres \
    --command "COMMENT ON DATABASE \"$BACKEND_DATABASE_DBNAME\" IS '$development_initializing_marker'" >/dev/null
fi
"$psql_cmd" -X --no-psqlrc "${postgres_args[@]}" --dbname "$BACKEND_DATABASE_DBNAME" --tuples-only --no-align --command 'SELECT 1' >/dev/null

database_marker="$(
  "$psql_cmd" -X --no-psqlrc "${postgres_args[@]}" --dbname "$BACKEND_DATABASE_DBNAME" \
    --tuples-only --no-align \
    --command "SELECT COALESCE(shobj_description(oid, 'pg_database'), '') FROM pg_database WHERE datname = current_database()"
)"
if [[ -z "$database_marker" ]]; then
  [[ -e "$receipt_path" ]] || {
    echo "已有数据库没有 local-init 初始化凭证；为避免接管其他数据库，请改用一个尚不存在的新库名" >&2
    exit 1
  }
  load_pending_receipt
  existing_objects="$(
    "$psql_cmd" -X --no-psqlrc "${postgres_args[@]}" --dbname "$BACKEND_DATABASE_DBNAME" \
      --tuples-only --no-align --command "
        WITH user_namespaces AS (
          SELECT oid FROM pg_namespace
          WHERE nspname NOT IN ('pg_catalog', 'information_schema')
            AND nspname NOT LIKE 'pg_toast%'
        )
        SELECT
          (SELECT COUNT(*) FROM pg_namespace WHERE oid IN (SELECT oid FROM user_namespaces) AND nspname <> 'public') +
          (SELECT COUNT(*) FROM pg_class WHERE relnamespace IN (SELECT oid FROM user_namespaces)) +
          (SELECT COUNT(*) FROM pg_proc WHERE pronamespace IN (SELECT oid FROM user_namespaces)) +
          (SELECT COUNT(*) FROM pg_type WHERE typnamespace IN (SELECT oid FROM user_namespaces)) +
          (SELECT COUNT(*) FROM pg_extension WHERE extname <> 'plpgsql')"
  )"
  if [[ "$existing_objects" != "0" ]]; then
    echo "待恢复数据库已含 ${existing_objects} 个用户对象；拒绝把它当作 createdb 中断后的空库" >&2
    exit 1
  fi
  development_initializing_marker="${development_marker_namespace}:initializing:${LOCAL_POSTGRES_SYSTEM_IDENTIFIER}:${pending_nonce}"
  require_local_init_lock
  "$psql_cmd" -X --no-psqlrc "${postgres_args[@]}" --dbname "$BACKEND_DATABASE_DBNAME" \
    --command "COMMENT ON DATABASE \"$BACKEND_DATABASE_DBNAME\" IS '$development_initializing_marker'" >/dev/null
  database_marker="$development_initializing_marker"
  echo "已为本机空数据库建立可恢复的初始化凭证"
fi
if [[ "$database_marker" == "${development_marker_namespace}:initializing:"* ]]; then
  load_pending_receipt
  expected_initializing_marker="${development_marker_namespace}:initializing:${LOCAL_POSTGRES_SYSTEM_IDENTIFIER}:${pending_nonce}"
  [[ "$database_marker" == "$expected_initializing_marker" ]] || {
    echo "数据库初始化凭证与本机恢复凭证不一致" >&2
    exit 1
  }
elif ! valid_completed_local_database_marker "$database_marker" "$LOCAL_POSTGRES_SYSTEM_IDENTIFIER"; then
  echo "数据库注释不是受支持的 local-init 初始化凭证；拒绝覆盖原注释或写入数据" >&2
  exit 1
elif [[ -n "$pending_nonce" && "$database_marker" != "${development_marker_namespace}:complete:${LOCAL_POSTGRES_SYSTEM_IDENTIFIER}:${pending_nonce}:"* ]]; then
  echo "数据库完成凭证与现有本机恢复凭证不一致；拒绝删除或覆盖该恢复凭证" >&2
  exit 1
fi

echo "[4/4] 执行迁移、四个体验账号、层级关系、赔率和房间验收初始化"
require_local_init_lock
(
  cd "$ROOT_DIR/backend"
  BACKEND_LOCAL_INIT_CLUSTER_ID="$LOCAL_POSTGRES_SYSTEM_IDENTIFIER" \
    BACKEND_LOCAL_INIT_NONCE="$pending_nonce" \
    BACKEND_SEED_EXPERIENCE_ACCOUNTS=true \
    go run ./cmd/dev-bootstrap --confirm-local-development
)

require_local_init_lock
require_completed_local_database "$psql_cmd"
completed_marker="$LOCAL_DEVELOPMENT_DATABASE_MARKER"
if [[ -n "$pending_nonce" && "$completed_marker" != "${development_marker_namespace}:complete:${LOCAL_POSTGRES_SYSTEM_IDENTIFIER}:${pending_nonce}:"* ]]; then
  echo "初始化命令返回成功，但数据库完成凭证与本机恢复凭证不一致" >&2
  exit 1
fi
if [[ -f "$receipt_path" && ! -L "$receipt_path" ]]; then
  require_local_init_lock
  rm -f -- "$receipt_path"
fi
unset PGPASSWORD PGSSLMODE

echo "本地初始化成功；没有清除任何已有数据。"
if [[ -n "${1:-${ENV_FILE:-}}" ]]; then
  echo "下一步：ENV_FILE='${1:-${ENV_FILE:-}}' make dev"
else
  echo "下一步：make dev"
fi
