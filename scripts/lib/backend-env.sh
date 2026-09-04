#!/usr/bin/env bash

# Load the systemd-compatible BACKEND_* environment file without evaluating it
# as shell code. This is intentionally a parser, not `source`: a database
# password containing `$()` or backticks must remain data even when a readiness
# or backup command is run as root.

trim_backend_env_value() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

backend_env_stat() {
  local format_linux="$1"
  local format_bsd="$2"
  local file="$3"
  if stat -c "$format_linux" "$file" >/dev/null 2>&1; then
    stat -c "$format_linux" "$file"
  else
    stat -f "$format_bsd" "$file"
  fi
}

load_backend_env() {
  local env_file="$1"
  [[ -n "$env_file" && -f "$env_file" && ! -L "$env_file" ]] || {
    echo "环境文件不存在、不是普通文件或是符号链接：$env_file" >&2
    return 1
  }

  local mode owner mode_value expected_owner
  mode="$(backend_env_stat '%a' '%Lp' "$env_file")"
  owner="$(backend_env_stat '%u' '%u' "$env_file")"
  [[ "$mode" =~ ^[0-7]{3,4}$ ]] || { echo "无法确认环境文件权限：$env_file" >&2; return 1; }
  mode_value=$((8#$mode))
  (( (mode_value & 077) == 0 )) || {
    echo "环境文件不能允许组或其他用户访问（要求 600/400）：$env_file" >&2
    return 1
  }
  expected_owner="${EUID:-$(id -u)}"
  [[ "$owner" == "$expected_owner" ]] || {
    echo "环境文件必须属于执行检查的用户（当前 uid=${expected_owner}，文件 uid=${owner}）" >&2
    return 1
  }

  # An explicit file is authoritative. Clear inherited BACKEND_* variables so
  # a missing backup/app setting cannot silently fall back to a more privileged
  # caller environment. Callers that deliberately use current environment
  # values bypass this loader explicitly (for example --current-env).
  local inherited_key
  while IFS= read -r inherited_key; do
    [[ "$inherited_key" == BACKEND_* ]] && unset "$inherited_key"
  done < <(compgen -e)

  local line key value first last line_number=0
  local seen_backend_env_keys=$'\n'
  while IFS= read -r line || [[ -n "$line" ]]; do
    line_number=$((line_number + 1))
    line="${line%$'\r'}"
    line="$(trim_backend_env_value "$line")"
    [[ -z "$line" || "$line" == \#* ]] && continue
    [[ "$line" == *=* ]] || { echo "环境文件第 $line_number 行缺少 =" >&2; return 1; }
    key="$(trim_backend_env_value "${line%%=*}")"
    value="$(trim_backend_env_value "${line#*=}")"
    [[ "$key" =~ ^BACKEND_[A-Z0-9_]+$ ]] || {
      echo "环境文件第 $line_number 行包含不允许的变量名：$key" >&2
      return 1
    }
    [[ "$seen_backend_env_keys" != *$'\n'"$key"$'\n'* ]] || {
      echo "环境文件第 $line_number 行重复定义变量：$key" >&2
      return 1
    }
    seen_backend_env_keys+="$key"$'\n'
    if (( ${#value} >= 2 )); then
      first="${value:0:1}"
      last="${value: -1}"
      if [[ ( "$first" == '"' && "$last" == '"' ) || ( "$first" == "'" && "$last" == "'" ) ]]; then
        value="${value:1:${#value}-2}"
      fi
    fi
    export "$key=$value"
  done < "$env_file"
}

# Local commands share one environment contract. An optional file is parsed by
# load_backend_env above (never sourced/evaluated), then missing values receive
# debug-only defaults. Keep experience fixtures out of this helper: only the
# explicit local initializer/auditor may enable them for its own subprocess.
load_optional_backend_env() {
  local explicit_env_file="${1:-${ENV_FILE:-}}"
  if [[ -n "$explicit_env_file" ]]; then
    load_backend_env "$explicit_env_file"
  fi
}

apply_local_backend_defaults() {
  export BACKEND_SERVER_BIND="${BACKEND_SERVER_BIND:-0.0.0.0}"
  export BACKEND_SERVER_PORT="${BACKEND_SERVER_PORT:-8080}"
  export BACKEND_SERVER_MODE="${BACKEND_SERVER_MODE:-debug}"
  export BACKEND_DATABASE_HOST="${BACKEND_DATABASE_HOST:-localhost}"
  export BACKEND_DATABASE_PORT="${BACKEND_DATABASE_PORT:-5432}"
  export BACKEND_DATABASE_USER="${BACKEND_DATABASE_USER:-postgres}"
  # An explicitly empty password is valid for a local trust/peer-auth setup;
  # only an unset variable receives the convenience default.
  if [[ -z "${BACKEND_DATABASE_PASSWORD+x}" ]]; then
    export BACKEND_DATABASE_PASSWORD="123456"
  fi
  export BACKEND_DATABASE_DBNAME="${BACKEND_DATABASE_DBNAME:-wangzhe}"
  export BACKEND_DATABASE_SSLMODE="${BACKEND_DATABASE_SSLMODE:-disable}"
  export BACKEND_JWT_SECRET="${BACKEND_JWT_SECRET:-backend_jwt_secret_key_2024}"
  export BACKEND_JWT_EXPIRE="${BACKEND_JWT_EXPIRE:-86400}"
  export BACKEND_SECURITY_DATA_ENCRYPTION_KEY="${BACKEND_SECURITY_DATA_ENCRYPTION_KEY:-local-data-encryption-key-7xlottery-dev-2026}"
  export BACKEND_SEED_DETERMINISTIC_LOTTERY_HISTORY="${BACKEND_SEED_DETERMINISTIC_LOTTERY_HISTORY:-false}"
}

# A loopback hostname alone does not prove that PostgreSQL is local: it may be
# an SSH tunnel to a remote server. When the operating system lets the current
# user inspect the exact TCP listener, lsof must identify it as PostgreSQL.
# PostgreSQL installed as a dedicated macOS system user is commonly hidden from
# unprivileged lsof, so the authoritative proof remains the local checkpointer
# PID plus matching cluster identity/start time/version through a Unix socket.
require_local_postgres_server() {
  local psql_cmd="$1" endpoint socket_directories endpoint_identifier endpoint_checkpointer_pid
  local endpoint_server_address endpoint_server_port endpoint_start_time endpoint_server_version
  endpoint="$(
    PGPASSWORD="$BACKEND_DATABASE_PASSWORD" PGSSLMODE="$BACKEND_DATABASE_SSLMODE" \
      "$psql_cmd" -X --no-psqlrc \
      --host "$BACKEND_DATABASE_HOST" --port "$BACKEND_DATABASE_PORT" \
      --username "$BACKEND_DATABASE_USER" --dbname postgres \
      --tuples-only --no-align --field-separator $'\t' \
      --command "SELECT current_setting('unix_socket_directories'), system_identifier::text, (SELECT pid::text FROM pg_stat_activity WHERE backend_type = 'checkpointer' LIMIT 1), inet_server_addr()::text, inet_server_port()::text, pg_postmaster_start_time()::text, current_setting('server_version') FROM pg_control_system()" \
      2>/dev/null
  )" || {
    echo "无法读取 PostgreSQL 本机身份；请确认维护库权限及 pg_control_system() 可用" >&2
    return 1
  }
  IFS=$'\t' read -r socket_directories endpoint_identifier endpoint_checkpointer_pid endpoint_server_address endpoint_server_port endpoint_start_time endpoint_server_version <<< "$endpoint"
  [[ -n "$socket_directories" && "$endpoint_identifier" =~ ^[0-9]+$ &&
    "$endpoint_checkpointer_pid" =~ ^[0-9]+$ && -n "$endpoint_server_address" &&
    "$endpoint_server_port" == "$BACKEND_DATABASE_PORT" && -n "$endpoint_start_time" &&
    -n "$endpoint_server_version" ]] || {
    echo "PostgreSQL 没有返回可验证的本机身份" >&2
    return 1
  }

  local lsof_server_address="$endpoint_server_address"
  if [[ "$lsof_server_address" == *:* ]]; then
    lsof_server_address="[$lsof_server_address]"
  fi
  local listener_pids listener_pid listener_command listener_count=0
  listener_pids="$(
    lsof -nP -a "-iTCP@${lsof_server_address}:${endpoint_server_port}" -sTCP:LISTEN -t 2>/dev/null || true
  )"
  if [[ -n "$listener_pids" ]]; then
    while IFS= read -r listener_pid; do
      [[ "$listener_pid" =~ ^[0-9]+$ ]] || continue
      listener_command="$(ps -p "$listener_pid" -o command= 2>/dev/null || true)"
      case "$listener_command" in
        postgres|postgres\ *|postgres:*|*/postgres|*/postgres\ *|*/postgres:*)
          listener_count=$((listener_count + 1))
          ;;
        *)
          echo "拒绝初始化：${endpoint_server_address}:${endpoint_server_port} 的可见 LISTEN 进程不是 PostgreSQL" >&2
          return 1
          ;;
      esac
    done <<< "$listener_pids"
    (( listener_count > 0 )) || {
      echo "拒绝初始化：lsof 返回了监听端点，但没有可验证的 PostgreSQL 进程" >&2
      return 1
    }
  fi

  local checkpointer_command
  checkpointer_command="$(ps -p "$endpoint_checkpointer_pid" -o command= 2>/dev/null || true)"
  case "$checkpointer_command" in
    postgres:*checkpointer*|*/postgres:*checkpointer*) ;;
    *)
      echo "拒绝初始化：配置端点返回的 PostgreSQL 后台进程不在本机" >&2
      return 1
      ;;
  esac

  local socket_dir socket_identity socket_identifier socket_start_time socket_server_version
  while IFS= read -r socket_dir; do
    socket_dir="$(trim_backend_env_value "$socket_dir")"
    socket_dir="${socket_dir#\'}"
    socket_dir="${socket_dir%\'}"
    socket_dir="${socket_dir#\"}"
    socket_dir="${socket_dir%\"}"
    [[ "$socket_dir" == /* || "$socket_dir" == @* ]] || continue
    socket_identity="$(
      PGPASSWORD="$BACKEND_DATABASE_PASSWORD" PGSSLMODE=disable \
        "$psql_cmd" -X --no-psqlrc \
        --host "$socket_dir" --port "$BACKEND_DATABASE_PORT" \
        --username "$BACKEND_DATABASE_USER" --dbname postgres \
        --tuples-only --no-align --field-separator $'\t' \
        --command "SELECT system_identifier::text, pg_postmaster_start_time()::text, current_setting('server_version') FROM pg_control_system()" \
        2>/dev/null
    )" || continue
    IFS=$'\t' read -r socket_identifier socket_start_time socket_server_version <<< "$socket_identity"
    if [[ "$socket_identifier" == "$endpoint_identifier" &&
      "$socket_start_time" == "$endpoint_start_time" &&
      "$socket_server_version" == "$endpoint_server_version" ]]; then
      LOCAL_POSTGRES_SYSTEM_IDENTIFIER="$endpoint_identifier"
      export LOCAL_POSTGRES_SYSTEM_IDENTIFIER
      return 0
    fi
  done < <(printf '%s\n' "$socket_directories" | tr ',' '\n')

  echo "拒绝初始化：无法证明 ${BACKEND_DATABASE_HOST}:${BACKEND_DATABASE_PORT} 是本机 PostgreSQL（SSH 隧道或远端转发不受支持）" >&2
  return 1
}

valid_completed_local_database_marker() {
  local marker="$1"
  local cluster_identifier="${2:-${LOCAL_POSTGRES_SYSTEM_IDENTIFIER:-}}"
  local marker_pattern
  [[ "$cluster_identifier" =~ ^[0-9]+$ ]] || return 1
  marker_pattern="^wangzhe-local-development-v1:complete:${cluster_identifier}:[0-9a-f]{32}:development-acceptance-odds-v1:[0-9a-f]{64}:[0-9a-f]{64}$"
  [[ "$marker" =~ $marker_pattern ]]
}

require_completed_local_database() {
  local psql_cmd="$1" marker
  [[ "${LOCAL_POSTGRES_SYSTEM_IDENTIFIER:-}" =~ ^[0-9]+$ ]] || {
    echo "尚未验证本机 PostgreSQL 集群身份" >&2
    return 1
  }
  marker="$(
    PGPASSWORD="$BACKEND_DATABASE_PASSWORD" PGSSLMODE="$BACKEND_DATABASE_SSLMODE" \
      "$psql_cmd" -X --no-psqlrc \
      --host "$BACKEND_DATABASE_HOST" --port "$BACKEND_DATABASE_PORT" \
      --username "$BACKEND_DATABASE_USER" --dbname "$BACKEND_DATABASE_DBNAME" \
      --tuples-only --no-align \
      --command "SELECT COALESCE(shobj_description(oid, 'pg_database'), '') FROM pg_database WHERE datname = current_database()" \
      2>/dev/null
  )" || {
    echo "无法读取应用数据库的 local-init 完成凭证；首次克隆请先运行 make dev-init" >&2
    return 1
  }
  valid_completed_local_database_marker "$marker" "$LOCAL_POSTGRES_SYSTEM_IDENTIFIER" || {
    echo "应用数据库没有与当前本机 PostgreSQL 集群匹配的有效完成凭证；请先运行 make dev-init" >&2
    return 1
  }
  LOCAL_DEVELOPMENT_DATABASE_MARKER="$marker"
  export LOCAL_DEVELOPMENT_DATABASE_MARKER
}

require_loopback_http_origin() {
  local variable_name="$1"
  local value="$2"
  local expected_port="$3"
  local origin_pattern='^http://(localhost|127\.0\.0\.1|\[::1\]):([0-9]+)$'
  [[ "$value" =~ $origin_pattern ]] || {
    echo "${variable_name} 必须是无路径、无凭据的本机 HTTP 地址（localhost、127.0.0.1 或 [::1]）" >&2
    return 1
  }
  [[ "${BASH_REMATCH[2]}" == "$expected_port" ]] || {
    echo "${variable_name} 端口必须为 ${expected_port}" >&2
    return 1
  }
}

require_local_backend_target() {
  [[ "${BACKEND_SERVER_MODE:-}" == "debug" ]] || {
    echo "本地命令只允许 BACKEND_SERVER_MODE=debug" >&2
    return 1
  }
  [[ "${BACKEND_SERVER_PORT:-}" =~ ^[0-9]+$ ]] &&
    (( 10#$BACKEND_SERVER_PORT >= 1 && 10#$BACKEND_SERVER_PORT <= 65535 )) || {
      echo "BACKEND_SERVER_PORT 必须是 1 到 65535 的整数" >&2
      return 1
    }
  case "${BACKEND_DATABASE_HOST:-}" in
    localhost|127.0.0.1|::1) ;;
    *)
      echo "本地命令只允许连接 localhost、127.0.0.1 或 ::1 PostgreSQL" >&2
      return 1
      ;;
  esac
  [[ "${BACKEND_DATABASE_PORT:-}" =~ ^[0-9]+$ ]] || {
    echo "BACKEND_DATABASE_PORT 必须是整数" >&2
    return 1
  }
  (( 10#$BACKEND_DATABASE_PORT >= 1 && 10#$BACKEND_DATABASE_PORT <= 65535 )) || {
    echo "BACKEND_DATABASE_PORT 必须在 1 到 65535 之间" >&2
    return 1
  }
  [[ "${BACKEND_DATABASE_DBNAME:-}" =~ ^[_A-Za-z][_A-Za-z0-9-]{0,62}$ ]] || {
    echo "本地数据库名称只能包含字母、数字、下划线或连字符，且长度不超过 63" >&2
    return 1
  }
  case "${BACKEND_DATABASE_DBNAME}" in
    [Pp][Oo][Ss][Tt][Gg][Rr][Ee][Ss]|[Tt][Ee][Mm][Pp][Ll][Aa][Tt][Ee]0|[Tt][Ee][Mm][Pp][Ll][Aa][Tt][Ee]1)
      echo "本地命令必须使用独立应用数据库，不能使用 ${BACKEND_DATABASE_DBNAME}" >&2
      return 1
      ;;
  esac
}

find_postgres_tool() {
  local tool="$1" candidate postgres_bin_dir
  candidate="$(command -v "$tool" 2>/dev/null || true)"
  if [[ -n "$candidate" ]]; then
    printf '%s\n' "$candidate"
    return 0
  fi
  for postgres_bin_dir in /Library/PostgreSQL/*/bin; do
    if [[ -x "$postgres_bin_dir/$tool" ]]; then
      printf '%s\n' "$postgres_bin_dir/$tool"
      return 0
    fi
  done
  echo "未找到 PostgreSQL 命令：$tool" >&2
  return 1
}
