#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${1:-/etc/wangzhe/backend.env}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/backend-env.sh
source "$SCRIPT_DIR/lib/backend-env.sh"
load_backend_env "$ENV_FILE"

required=(
  BACKEND_SERVER_BIND BACKEND_SERVER_PORT BACKEND_SERVER_MODE
  BACKEND_SERVER_ALLOWED_ORIGINS BACKEND_SERVER_TRUSTED_PROXIES
  BACKEND_DATABASE_HOST BACKEND_DATABASE_PORT BACKEND_DATABASE_USER
  BACKEND_DATABASE_PASSWORD BACKEND_DATABASE_DBNAME BACKEND_DATABASE_SSLMODE
  BACKEND_REDIS_ADDR BACKEND_REDIS_DB BACKEND_REDIS_TLS BACKEND_REDIS_PREFIX
  BACKEND_JWT_SECRET BACKEND_JWT_EXPIRE BACKEND_SECURITY_DATA_ENCRYPTION_KEY
  BACKEND_UPLOAD_DIR BACKEND_AUDIT_FALLBACK_FILE BACKEND_ROOM_ACTIVITY
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

case "$BACKEND_REDIS_TLS" in
  true|false) ;;
  *) echo "BACKEND_REDIS_TLS 必须是 true 或 false" >&2; exit 1 ;;
esac
case "$BACKEND_REDIS_ADDR" in
  localhost:*|127.0.0.1:*|::1:*|'[::1]':*)
    ;;
  *)
    [[ "$BACKEND_REDIS_TLS" == "true" ]] || { echo "远程 Redis 必须启用 TLS" >&2; exit 1; }
    [[ -n "${BACKEND_REDIS_PASSWORD:-}" ]] || { echo "远程 Redis 必须配置密码" >&2; exit 1; }
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
[[ "$BACKEND_ROOM_ACTIVITY" == "0" ]] || { echo "首次上线前 BACKEND_ROOM_ACTIVITY 必须保持 0" >&2; exit 1; }

echo "生产环境配置检查通过（未输出任何密钥）"
