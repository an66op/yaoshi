#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${1:-/etc/wangzhe/backend.env}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/backend-env.sh
source "$SCRIPT_DIR/lib/backend-env.sh"
load_backend_env "$ENV_FILE"
command -v jq >/dev/null 2>&1 || { echo "缺少生产配置校验命令: jq" >&2; exit 1; }

required=(
  BACKEND_SERVER_BIND BACKEND_SERVER_PORT BACKEND_SERVER_MODE
  BACKEND_SERVER_ALLOWED_ORIGINS BACKEND_SERVER_TRUSTED_PROXIES
  BACKEND_DATABASE_HOST BACKEND_DATABASE_PORT BACKEND_DATABASE_USER
  BACKEND_DATABASE_PASSWORD BACKEND_DATABASE_DBNAME BACKEND_DATABASE_SSLMODE
  BACKEND_REDIS_ADDR BACKEND_REDIS_USERNAME BACKEND_REDIS_PASSWORD BACKEND_REDIS_DB BACKEND_REDIS_TLS BACKEND_REDIS_PREFIX
  BACKEND_JWT_SECRET BACKEND_JWT_EXPIRE BACKEND_SECURITY_DATA_ENCRYPTION_KEY
  BACKEND_UPLOAD_DIR BACKEND_AUDIT_FALLBACK_FILE BACKEND_ROOM_ACTIVITY
  BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES
)
for key in "${required[@]}"; do
  [[ -n "${!key:-}" ]] || { echo "生产环境缺少 $key" >&2; exit 1; }
done

[[ "$BACKEND_SERVER_MODE" == "release" ]] || { echo "BACKEND_SERVER_MODE 必须为 release" >&2; exit 1; }
[[ "$BACKEND_SERVER_BIND" == "127.0.0.1" ]] || {
  echo "当前 Nginx 模板要求后端只监听 127.0.0.1" >&2
  exit 1
}
[[ "$BACKEND_SERVER_PORT" == "8080" ]] || {
  echo "当前 Nginx 模板要求 BACKEND_SERVER_PORT=8080" >&2
  exit 1
}
[[ "$BACKEND_JWT_EXPIRE" =~ ^[0-9]+$ ]] && (( BACKEND_JWT_EXPIRE >= 300 && BACKEND_JWT_EXPIRE <= 86400 )) || {
  echo "BACKEND_JWT_EXPIRE 必须在 300-86400 秒之间" >&2
  exit 1
}

for key in BACKEND_DATABASE_PASSWORD BACKEND_JWT_SECRET BACKEND_SECURITY_DATA_ENCRYPTION_KEY; do
  value="${!key}"
  lower_value="$(printf '%s' "$value" | tr '[:upper:]' '[:lower:]')"
  case "$lower_value" in
    *change_me*|*changeme*|*replace_with*|*example*|*password123*|*123456*)
      echo "$key 仍是示例值或弱口令" >&2
      exit 1
      ;;
  esac
done
(( ${#BACKEND_DATABASE_PASSWORD} >= 16 )) || { echo "数据库密码少于 16 位" >&2; exit 1; }
(( ${#BACKEND_JWT_SECRET} >= 32 )) || { echo "JWT 密钥少于 32 位" >&2; exit 1; }
(( ${#BACKEND_SECURITY_DATA_ENCRYPTION_KEY} >= 32 )) || { echo "数据加密密钥少于 32 位" >&2; exit 1; }
[[ "$BACKEND_JWT_SECRET" != "$BACKEND_SECURITY_DATA_ENCRYPTION_KEY" ]] || {
  echo "JWT 与数据加密必须使用不同密钥" >&2
  exit 1
}
previous_data_keys="${BACKEND_SECURITY_DATA_ENCRYPTION_PREVIOUS_KEYS:-[]}"
if ! jq -e '
    type == "array" and length <= 8 and
    all(.[];
      type == "string" and length >= 32 and
      (explode | all(. >= 32 and . != 127)) and
      ((ascii_downcase | test("change_me|changeme|replace_with|example|password123|123456") | not))
    ) and
    ((unique | length) == length)
  ' <<<"$previous_data_keys" >/dev/null; then
  echo "历史数据加密密钥必须是至多8项的不重复高强度 JSON 字符串数组，且不得复用当前密钥或其他凭据" >&2
  exit 1
fi
while IFS= read -r previous_data_key; do
  if [[ "$previous_data_key" == "$BACKEND_SECURITY_DATA_ENCRYPTION_KEY" ||
        "$previous_data_key" == "$BACKEND_JWT_SECRET" ||
        "$previous_data_key" == "$BACKEND_DATABASE_PASSWORD" ]]; then
    echo "历史数据加密密钥不得复用当前密钥或其他凭据" >&2
    exit 1
  fi
done < <(jq -r '.[]' <<<"$previous_data_keys")

case "$BACKEND_DATABASE_SSLMODE" in
  disable|verify-ca|verify-full) ;;
  *) echo "生产数据库 SSLMODE 只能是 disable、verify-ca 或 verify-full" >&2; exit 1 ;;
esac
case "$BACKEND_DATABASE_HOST" in
  localhost|127.0.0.1|::1)
    ;;
  *)
    [[ "$BACKEND_DATABASE_SSLMODE" == "verify-ca" || "$BACKEND_DATABASE_SSLMODE" == "verify-full" ]] || {
      echo "远程 PostgreSQL 必须校验证书（verify-ca/verify-full）" >&2
      exit 1
    }
    ;;
esac

database_max_idle="${BACKEND_DATABASE_MAX_IDLE_CONNS:-10}"
database_max_open="${BACKEND_DATABASE_MAX_OPEN_CONNS:-50}"
database_lifetime="${BACKEND_DATABASE_CONN_MAX_LIFETIME_SECONDS:-3600}"
for entry in \
  "BACKEND_DATABASE_MAX_IDLE_CONNS:$database_max_idle" \
  "BACKEND_DATABASE_MAX_OPEN_CONNS:$database_max_open" \
  "BACKEND_DATABASE_CONN_MAX_LIFETIME_SECONDS:$database_lifetime"; do
  key="${entry%%:*}"
  value="${entry#*:}"
  [[ "$value" =~ ^[0-9]+$ ]] || { echo "$key 必须是正整数" >&2; exit 1; }
done
(( 10#$database_max_idle >= 1 && 10#$database_max_idle <= 10#$database_max_open )) || {
  echo "数据库空闲连接数必须在 1 至最大连接数之间" >&2
  exit 1
}
(( 10#$database_max_open >= 1 && 10#$database_max_open <= 10000 )) || {
  echo "数据库最大连接数必须在 1-10000 之间" >&2
  exit 1
}
(( 10#$database_lifetime >= 60 && 10#$database_lifetime <= 86400 )) || {
  echo "数据库连接最长生命周期必须在 60-86400 秒之间" >&2
  exit 1
}

case "$BACKEND_REDIS_TLS" in
  true|false) ;;
  *) echo "BACKEND_REDIS_TLS 必须是 true 或 false" >&2; exit 1 ;;
esac
[[ "$BACKEND_REDIS_USERNAME" =~ ^[A-Za-z0-9_.-]{1,64}$ && "$BACKEND_REDIS_USERNAME" != "default" ]] || {
  echo "生产 Redis 必须使用非 default 的独立 ACL 用户" >&2
  exit 1
}
redis_password_lower="$(printf '%s' "$BACKEND_REDIS_PASSWORD" | tr '[:upper:]' '[:lower:]')"
(( ${#BACKEND_REDIS_PASSWORD} >= 24 )) || { echo "Redis 密码少于 24 位" >&2; exit 1; }
case "$redis_password_lower" in
  *change_me*|*changeme*|*replace_with*|*example*|*123456*) echo "Redis 密码仍是示例值或弱口令" >&2; exit 1 ;;
esac
case "$BACKEND_REDIS_ADDR" in
  localhost:*|127.0.0.1:*|::1:*|'[::1]':*)
    ;;
  *)
    [[ "$BACKEND_REDIS_TLS" == "true" ]] || { echo "远程 Redis 必须启用 TLS" >&2; exit 1; }
    ;;
esac
[[ "$BACKEND_REDIS_DB" =~ ^[0-9]+$ ]] || { echo "BACKEND_REDIS_DB 必须是非负整数" >&2; exit 1; }
prefix_lower="$(printf '%s' "$BACKEND_REDIS_PREFIX" | tr '[:upper:]' '[:lower:]')"
case "$prefix_lower" in
  *production*) ;;
  *) echo "BACKEND_REDIS_PREFIX 必须使用明确的 production 隔离前缀" >&2; exit 1 ;;
esac

IFS=',' read -r -a allowed_origins <<<"$BACKEND_SERVER_ALLOWED_ORIGINS"
(( ${#allowed_origins[@]} >= 1 )) || { echo "至少需要一个 CORS 来源" >&2; exit 1; }
for origin in "${allowed_origins[@]}"; do
  origin="$(trim_backend_env_value "$origin")"
  [[ "$origin" =~ ^https://[A-Za-z0-9.-]+(:[0-9]+)?$ ]] || {
    echo "release CORS 只允许明确的 HTTPS Origin：$origin" >&2
    exit 1
  }
done
expected_production_origins='https://wz6688.app,https://www.wz6688.app,https://admin.wz888.site'
[[ "$BACKEND_SERVER_ALLOWED_ORIGINS" == "$expected_production_origins" ]] || {
  echo "生产 CORS 来源必须精确匹配正式会员/管理域名：$expected_production_origins" >&2
  exit 1
}

IFS=',' read -r -a trusted_proxies <<<"$BACKEND_SERVER_TRUSTED_PROXIES"
for proxy in "${trusted_proxies[@]}"; do
  proxy="$(trim_backend_env_value "$proxy")"
  case "$proxy" in
    127.0.0.1|127.0.0.1/32|::1|::1/128) ;;
    *) echo "单机 Nginx 部署只应信任回环代理：$proxy" >&2; exit 1 ;;
  esac
done

for path_key in BACKEND_UPLOAD_DIR BACKEND_AUDIT_FALLBACK_FILE; do
  value="${!path_key}"
  [[ "$value" == /* && "$value" != "/" ]] || { echo "$path_key 必须是非根绝对路径" >&2; exit 1; }
done
[[ "$BACKEND_UPLOAD_DIR" == /var/lib/wangzhe/* ]] || {
  echo "BACKEND_UPLOAD_DIR 必须位于 /var/lib/wangzhe/ 下" >&2
  exit 1
}
[[ "$BACKEND_AUDIT_FALLBACK_FILE" == /var/lib/wangzhe/* ]] || {
  echo "BACKEND_AUDIT_FALLBACK_FILE 必须位于 /var/lib/wangzhe/ 下" >&2
  exit 1
}
case "$BACKEND_ROOM_ACTIVITY" in
  0|1) ;;
  *) echo "BACKEND_ROOM_ACTIVITY 只能是 0 或 1" >&2; exit 1 ;;
esac
[[ "$BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES" =~ ^(0|[1-9][0-9]{0,2})$ ]] || {
  echo "BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES 必须是 0-100 的十进制整数" >&2
  exit 1
}
(( 10#$BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES <= 100 )) || {
  echo "BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES 不能超过 100" >&2
  exit 1
}
if [[ "$BACKEND_ROOM_ACTIVITY" == "1" ]]; then
  (( 10#$BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES >= 1 )) || {
    echo "启用生产机器人时必须设置正数 BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES" >&2
    exit 1
  }
fi

echo "生产环境配置检查通过（未输出任何密钥）"
