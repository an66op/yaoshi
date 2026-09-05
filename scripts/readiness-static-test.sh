#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=lib/backend-env.sh
source "$ROOT_DIR/scripts/lib/backend-env.sh"
# shellcheck source=lib/safe-integer.sh
source "$ROOT_DIR/scripts/lib/safe-integer.sh"
# shellcheck source=lib/maintenance-edge.sh
source "$ROOT_DIR/scripts/lib/maintenance-edge.sh"
# shellcheck source=lib/encrypted-backup.sh
source "$ROOT_DIR/scripts/lib/encrypted-backup.sh"
# shellcheck source=lib/encryption-capabilities.sh
source "$ROOT_DIR/scripts/lib/encryption-capabilities.sh"

fixture_dir="$(mktemp -d)"
cleanup_fixture() { rm -rf -- "$fixture_dir"; }
trap cleanup_fixture EXIT INT TERM

valid_encryption_release="$fixture_dir/valid-encryption-release"
mkdir -p "$valid_encryption_release"
printf '%s\n' \
  'format_version=1' \
  'read_versions=1,2' \
  'write_version=2' \
  'previous_key_fallback=true' \
  >"$valid_encryption_release/FIELD_ENCRYPTION_CAPABILITIES"
load_release_encryption_capabilities "$valid_encryption_release"
# Initialized by load_release_encryption_capabilities from the sourced parser.
# shellcheck disable=SC2154
[[ "$encryption_cap_read_versions" == "1,2" && "$encryption_cap_write_version" == "2" && "$encryption_cap_previous_key_fallback" == "true" ]]
encryption_version_supported "$encryption_cap_read_versions" 1
encryption_version_supported "$encryption_cap_read_versions" 2
if encryption_version_supported "$encryption_cap_read_versions" 3; then
  echo "未知加密信封版本被错误识别为可读" >&2
  exit 1
fi

invalid_encryption_release="$fixture_dir/invalid-encryption-release"
mkdir -p "$invalid_encryption_release"
capability_injection_marker="$fixture_dir/capability-injection-must-not-run"
printf '%s\n' \
  'format_version=1' \
  'read_versions=$(touch '"$capability_injection_marker"')' \
  'write_version=2' \
  'previous_key_fallback=true' \
  >"$invalid_encryption_release/FIELD_ENCRYPTION_CAPABILITIES"
if load_release_encryption_capabilities "$invalid_encryption_release" >/dev/null 2>&1; then
  echo "恶意加密信封能力元数据被错误接受" >&2
  exit 1
fi
[[ ! -e "$capability_injection_marker" ]]

printf '%s\n' \
  'format_version=1' \
  'read_versions=1' \
  'write_version=2' \
  'previous_key_fallback=false' \
  >"$invalid_encryption_release/FIELD_ENCRYPTION_CAPABILITIES"
if load_release_encryption_capabilities "$invalid_encryption_release" >/dev/null 2>&1; then
  echo "无法自读写入格式的发布能力元数据被错误接受" >&2
  exit 1
fi

arithmetic_marker="$fixture_dir/arithmetic-injection-must-not-run"
malicious_count='a[$(touch '"$arithmetic_marker"')0]'
if require_decimal_count "恶意计数" "$malicious_count" >/dev/null 2>&1; then
  echo "恶意算术表达式被错误接受" >&2
  exit 1
fi
[[ ! -e "$arithmetic_marker" ]] || { echo "计数校验触发了命令替换" >&2; exit 1; }
[[ "$(require_decimal_count 正常计数 00042)" == "00042" ]]
(
  curl() {
    printf '%s\n' 'HTTP/2 503' 'Strict-Transport-Security: max-age=31536000' 'Content-Security-Policy: default-src '\''self'\''' 'X-Content-Type-Options: nosniff'
  }
  verify_maintenance_edge https://wz6688.app https://www.wz6688.app https://admin.wz888.site
)
if (
  curl() {
    printf '%s\n' 'HTTP/2 200' 'Strict-Transport-Security: max-age=31536000' 'Content-Security-Policy: default-src '\''self'\''' 'X-Content-Type-Options: nosniff'
  }
  verify_maintenance_edge https://wz6688.app https://www.wz6688.app https://admin.wz888.site >/dev/null 2>&1
); then
  echo "非 503 响应被错误识别为维护模式" >&2
  exit 1
fi
if (
  curl() {
    if [[ "$*" == *--resolve* ]]; then
      printf '%s\n' 'HTTP/2 503' 'Strict-Transport-Security: max-age=31536000' 'Content-Security-Policy: default-src '\''self'\''' 'X-Content-Type-Options: nosniff'
    else
      printf '%s\n' 'HTTP/2 200' 'Strict-Transport-Security: max-age=31536000' 'Content-Security-Policy: default-src '\''self'\''' 'X-Content-Type-Options: nosniff'
    fi
  }
  verify_maintenance_edge https://wz6688.app https://www.wz6688.app https://admin.wz888.site >/dev/null 2>&1
); then
  echo "公网未维护但本机 Nginx 已维护时被错误放行" >&2
  exit 1
fi
if (
  curl() {
    if [[ "$*" == *--resolve* ]]; then
      printf '%s\n' 'HTTP/2 200' 'Strict-Transport-Security: max-age=31536000' 'Content-Security-Policy: default-src '\''self'\''' 'X-Content-Type-Options: nosniff'
    else
      printf '%s\n' 'HTTP/2 503' 'Strict-Transport-Security: max-age=31536000' 'Content-Security-Policy: default-src '\''self'\''' 'X-Content-Type-Options: nosniff'
    fi
  }
  verify_maintenance_edge https://wz6688.app https://www.wz6688.app https://admin.wz888.site >/dev/null 2>&1
); then
  echo "本机 Nginx 未维护但公网已维护时被错误放行" >&2
  exit 1
fi
if (
  curl() {
    if [[ "$*" == *admin.wz888.site* ]]; then
      printf '%s\n' 'HTTP/2 503' 'Strict-Transport-Security: max-age=31536000' 'Content-Security-Policy: default-src '\''self'\''' 'X-Content-Type-Options: nosniff'
    else
      printf '%s\n' 'HTTP/2 200' 'Strict-Transport-Security: max-age=31536000' 'Content-Security-Policy: default-src '\''self'\''' 'X-Content-Type-Options: nosniff'
    fi
  }
  verify_maintenance_edge https://wz6688.app https://www.wz6688.app https://admin.wz888.site >/dev/null 2>&1
); then
  echo "用户端未维护、管理端已维护时被错误放行" >&2
  exit 1
fi

# Exercise the marker ownership helper used by deploy and rollback. An
# operator-owned marker must survive, an externally removed pre-existing
# marker must not be recreated, and a failed atomic move must be retryable.
existing_maintenance_marker="$fixture_dir/existing-maintenance"
printf '%s\n' 'operator-owned-marker' >"$existing_maintenance_marker"
capture_maintenance_marker_state "$existing_maintenance_marker"
# Initialized by sourced maintenance-edge.sh.
# shellcheck disable=SC2154
[[ "$maintenance_was_active" == 1 && "$maintenance_marker_created" == 0 ]]
ensure_maintenance_marker "$existing_maintenance_marker"
finish_maintenance_marker "$existing_maintenance_marker"
[[ "$(cat "$existing_maintenance_marker")" == "operator-owned-marker" ]] || {
  echo "预先存在的维护标记被修改或删除" >&2
  exit 1
}

removed_maintenance_marker="$fixture_dir/removed-maintenance"
printf '%s\n' 'operator-owned-marker' >"$removed_maintenance_marker"
capture_maintenance_marker_state "$removed_maintenance_marker"
rm -f -- "$removed_maintenance_marker"
if ensure_maintenance_marker "$removed_maintenance_marker"; then
  echo "操作开始时已有的维护标记被错误重建" >&2
  exit 1
fi
[[ ! -e "$removed_maintenance_marker" && ! -L "$removed_maintenance_marker" ]]

created_maintenance_marker="$fixture_dir/created-maintenance"
capture_maintenance_marker_state "$created_maintenance_marker"
ensure_maintenance_marker "$created_maintenance_marker"
# Initialized by sourced maintenance-edge.sh.
# shellcheck disable=SC2154
[[ "$maintenance_was_active" == 0 && "$maintenance_marker_created" == 1 ]]
# Initialized by sourced maintenance-edge.sh.
# shellcheck disable=SC2154
maintenance_marker_owned_by "$created_maintenance_marker" "$maintenance_marker_token"
[[ -z "$(find "$fixture_dir" -maxdepth 1 -name '.created-maintenance.tmp.*' -print -quit)" ]]
finish_maintenance_marker "$created_maintenance_marker"
[[ ! -e "$created_maintenance_marker" && ! -L "$created_maintenance_marker" ]]

retry_maintenance_marker="$fixture_dir/retry-maintenance"
mv_attempt_file="$fixture_dir/maintenance-mv-attempts"
printf '%s\n' 0 >"$mv_attempt_file"
mv() {
  local attempt
  attempt="$(cat "$mv_attempt_file")"
  printf '%s\n' "$((attempt + 1))" >"$mv_attempt_file"
  if (( attempt == 0 )); then
    return 1
  fi
  command mv "$@"
}
capture_maintenance_marker_state "$retry_maintenance_marker"
if ensure_maintenance_marker "$retry_maintenance_marker"; then
  echo "原子移动失败时维护 helper 被错误识别为成功" >&2
  exit 1
fi
[[ ! -e "$retry_maintenance_marker" && ! -L "$retry_maintenance_marker" ]]
[[ -z "$(find "$fixture_dir" -maxdepth 1 -name '.retry-maintenance.tmp.*' -print -quit)" ]]
ensure_maintenance_marker "$retry_maintenance_marker"
unset -f mv
[[ "$maintenance_marker_created" == 1 ]]
maintenance_marker_owned_by "$retry_maintenance_marker" "$maintenance_marker_token"
finish_maintenance_marker "$retry_maintenance_marker"
[[ ! -e "$retry_maintenance_marker" && ! -L "$retry_maintenance_marker" ]]

edge_failure_marker="$fixture_dir/edge-failure-maintenance"
capture_maintenance_marker_state "$edge_failure_marker"
ensure_maintenance_marker "$edge_failure_marker"
if (
  curl() {
    printf '%s\n' 'HTTP/2 200' 'Strict-Transport-Security: max-age=31536000' 'Content-Security-Policy: default-src '\''self'\''' 'X-Content-Type-Options: nosniff'
  }
  verify_maintenance_edge https://wz6688.app https://www.wz6688.app https://admin.wz888.site >/dev/null 2>&1
); then
  echo "维护边缘验证失败场景被错误识别为成功" >&2
  exit 1
fi
maintenance_marker_owned_by "$edge_failure_marker" "$maintenance_marker_token" || {
  echo "维护边缘验证失败后未保持 fail-closed 标记" >&2
  exit 1
}
(
  curl() {
    printf '%s\n' 'HTTP/2 503' 'Strict-Transport-Security: max-age=31536000' 'Content-Security-Policy: default-src '\''self'\''' 'X-Content-Type-Options: nosniff'
  }
  verify_maintenance_edge https://wz6688.app https://www.wz6688.app https://admin.wz888.site
)
maintenance_marker_owned_by "$edge_failure_marker" "$maintenance_marker_token"
finish_maintenance_marker "$edge_failure_marker"
[[ ! -e "$edge_failure_marker" && ! -L "$edge_failure_marker" ]]

env_file="$fixture_dir/backend.env"
marker="$fixture_dir/must-not-exist"
printf '%s\n' \
  'BACKEND_DATABASE_HOST=127.0.0.1' \
  'BACKEND_DATABASE_PASSWORD=$(touch should-not-run)' \
  >"$env_file"
chmod 600 "$env_file"
(
  cd "$fixture_dir"
  export BACKEND_JWT_SECRET=must-not-leak-from-caller
  export BACKEND_URL=http://127.0.0.1:9999
  readiness_api_url="${BACKEND_URL:-http://127.0.0.1:8080}"
  load_backend_env "$env_file"
  [[ "$BACKEND_DATABASE_PASSWORD" == '$(touch should-not-run)' ]]
  [[ -z "${BACKEND_JWT_SECRET+x}" ]]
  [[ -z "${BACKEND_URL+x}" && "$readiness_api_url" == "http://127.0.0.1:9999" ]]
)
[[ ! -e "$fixture_dir/should-not-run" && ! -e "$marker" ]]

chmod 644 "$env_file"
if (load_backend_env "$env_file" >/dev/null 2>&1); then
  echo "宽松权限的环境文件被错误接受" >&2
  exit 1
fi

valid_env="$fixture_dir/valid-backend.env"
cat >"$valid_env" <<'EOF'
BACKEND_SERVER_PORT=8080
BACKEND_SERVER_BIND=127.0.0.1
BACKEND_SERVER_MODE=release
BACKEND_SERVER_ALLOWED_ORIGINS=https://wz6688.app,https://www.wz6688.app,https://admin.wz888.site
BACKEND_SERVER_TRUSTED_PROXIES=127.0.0.1,::1
BACKEND_DATABASE_HOST=127.0.0.1
BACKEND_DATABASE_PORT=5432
BACKEND_DATABASE_USER=wangzhe
BACKEND_DATABASE_PASSWORD=correct-horse-battery-staple-2026
BACKEND_DATABASE_DBNAME=wangzhe
BACKEND_DATABASE_SSLMODE=disable
BACKEND_REDIS_ADDR=127.0.0.1:6379
BACKEND_REDIS_USERNAME=wangzhe-app
BACKEND_REDIS_PASSWORD=redis-secret-that-is-longer-than-twenty-four-characters
BACKEND_REDIS_DB=0
BACKEND_REDIS_TLS=false
BACKEND_REDIS_PREFIX=wangzhe-production
BACKEND_JWT_SECRET=jwt-secret-that-is-longer-than-thirty-two-characters-A
BACKEND_JWT_EXPIRE=3600
BACKEND_SECURITY_DATA_ENCRYPTION_KEY=data-key-that-is-longer-than-thirty-two-characters-B
BACKEND_SECURITY_DATA_ENCRYPTION_PREVIOUS_KEYS=[]
BACKEND_UPLOAD_DIR=/var/lib/wangzhe/uploads
BACKEND_AUDIT_FALLBACK_FILE=/var/lib/wangzhe/audit-fallback.jsonl
BACKEND_ROOM_ACTIVITY=0
BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES=0
EOF
chmod 600 "$valid_env"
bash "$ROOT_DIR/scripts/production-config-check.sh" "$valid_env" >/dev/null

rotating_data_key_env="$fixture_dir/rotating-data-key.env"
sed 's#BACKEND_SECURITY_DATA_ENCRYPTION_PREVIOUS_KEYS=\[\]#BACKEND_SECURITY_DATA_ENCRYPTION_PREVIOUS_KEYS='\''["retained-old-data-key-with-more-than-thirty-two-chars-C9!"]'\''#' "$valid_env" >"$rotating_data_key_env"
chmod 600 "$rotating_data_key_env"
bash "$ROOT_DIR/scripts/production-config-check.sh" "$rotating_data_key_env" >/dev/null

invalid_data_keyring_env="$fixture_dir/invalid-data-keyring.env"
sed 's#BACKEND_SECURITY_DATA_ENCRYPTION_PREVIOUS_KEYS=\[\]#BACKEND_SECURITY_DATA_ENCRYPTION_PREVIOUS_KEYS=not-json#' "$valid_env" >"$invalid_data_keyring_env"
chmod 600 "$invalid_data_keyring_env"
if bash "$ROOT_DIR/scripts/production-config-check.sh" "$invalid_data_keyring_env" >/dev/null 2>&1; then
  echo "非 JSON 的历史数据加密密钥配置被错误接受" >&2
  exit 1
fi

duplicate_data_keyring_env="$fixture_dir/duplicate-data-keyring.env"
sed 's#BACKEND_SECURITY_DATA_ENCRYPTION_PREVIOUS_KEYS=\[\]#BACKEND_SECURITY_DATA_ENCRYPTION_PREVIOUS_KEYS='\''["data-key-that-is-longer-than-thirty-two-characters-B"]'\''#' "$valid_env" >"$duplicate_data_keyring_env"
chmod 600 "$duplicate_data_keyring_env"
if bash "$ROOT_DIR/scripts/production-config-check.sh" "$duplicate_data_keyring_env" >/dev/null 2>&1; then
  echo "复用当前密钥的历史数据加密配置被错误接受" >&2
  exit 1
fi

wrong_production_domain_env="$fixture_dir/wrong-production-domain.env"
sed 's#https://www.wz6688.app,https://admin.wz888.site#https://admin.invalid.example#' "$valid_env" >"$wrong_production_domain_env"
chmod 600 "$wrong_production_domain_env"
if bash "$ROOT_DIR/scripts/production-config-check.sh" "$wrong_production_domain_env" >/dev/null 2>&1; then
  echo "非正式域名的生产 CORS 配置被错误接受" >&2
  exit 1
fi

enabled_robot_env="$fixture_dir/enabled-robot.env"
sed \
  -e 's/BACKEND_ROOM_ACTIVITY=0/BACKEND_ROOM_ACTIVITY=1/' \
  -e 's/BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES=0/BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES=2/' \
  "$valid_env" >"$enabled_robot_env"
chmod 600 "$enabled_robot_env"
bash "$ROOT_DIR/scripts/production-config-check.sh" "$enabled_robot_env" >/dev/null

uncapped_robot_env="$fixture_dir/uncapped-robot.env"
sed 's/BACKEND_ROOM_ACTIVITY=0/BACKEND_ROOM_ACTIVITY=1/' "$valid_env" >"$uncapped_robot_env"
chmod 600 "$uncapped_robot_env"
if bash "$ROOT_DIR/scripts/production-config-check.sh" "$uncapped_robot_env" >/dev/null 2>&1; then
  echo "未设置工作区上限的生产机器人被错误接受" >&2
  exit 1
fi

truthy_robot_env="$fixture_dir/truthy-robot.env"
sed 's/BACKEND_ROOM_ACTIVITY=0/BACKEND_ROOM_ACTIVITY=true/' "$valid_env" >"$truthy_robot_env"
chmod 600 "$truthy_robot_env"
if bash "$ROOT_DIR/scripts/production-config-check.sh" "$truthy_robot_env" >/dev/null 2>&1; then
  echo "非精确开关值的生产机器人被错误接受" >&2
  exit 1
fi

over_cap_robot_env="$fixture_dir/over-cap-robot.env"
sed 's/BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES=0/BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES=101/' "$valid_env" >"$over_cap_robot_env"
chmod 600 "$over_cap_robot_env"
if bash "$ROOT_DIR/scripts/production-config-check.sh" "$over_cap_robot_env" >/dev/null 2>&1; then
  echo "超过安全上限的生产机器人配置被错误接受" >&2
  exit 1
fi

duplicate_env="$fixture_dir/duplicate.env"
cp "$valid_env" "$duplicate_env"
printf '%s\n' 'BACKEND_SERVER_PORT=9090' >>"$duplicate_env"
chmod 600 "$duplicate_env"
if bash "$ROOT_DIR/scripts/production-config-check.sh" "$duplicate_env" >/dev/null 2>&1; then
  echo "重复环境变量被错误接受" >&2
  exit 1
fi

weak_env="$fixture_dir/weak.env"
sed 's/correct-horse-battery-staple-2026/123456/' "$valid_env" >"$weak_env"
chmod 600 "$weak_env"
if bash "$ROOT_DIR/scripts/production-config-check.sh" "$weak_env" >/dev/null 2>&1; then
  echo "生产弱口令被错误接受" >&2
  exit 1
fi

remote_redis_env="$fixture_dir/remote-redis.env"
sed 's/127.0.0.1:6379/redis.internal:6379/' "$valid_env" >"$remote_redis_env"
chmod 600 "$remote_redis_env"
if bash "$ROOT_DIR/scripts/production-config-check.sh" "$remote_redis_env" >/dev/null 2>&1; then
  echo "未启用 TLS 的远程 Redis 被错误接受" >&2
  exit 1
fi

default_redis_user_env="$fixture_dir/default-redis-user.env"
sed 's/BACKEND_REDIS_USERNAME=wangzhe-app/BACKEND_REDIS_USERNAME=default/' "$valid_env" >"$default_redis_user_env"
chmod 600 "$default_redis_user_env"
if bash "$ROOT_DIR/scripts/production-config-check.sh" "$default_redis_user_env" >/dev/null 2>&1; then
  echo "生产配置错误接受了 Redis default 用户" >&2
  exit 1
fi

! rg -n '127\.0\.0\.1:8089|BACKEND_SERVER_ALLOWED_ORIGINS=.*http://' "$ROOT_DIR/deploy"
rg -q 'migrations\.VerifyApplied' "$ROOT_DIR/backend/api/health.go"
rg -q 'AuditProductionOddsReadiness' "$ROOT_DIR/backend/api/health.go"
rg -Fq '"$READINESS_API_URL/ready/odds"' "$ROOT_DIR/scripts/production-readiness.sh"
rg -q 'AuditSensitiveFieldReadiness' "$ROOT_DIR/backend/api/health.go"
rg -Fq '"$READINESS_API_URL/ready/encryption"' "$ROOT_DIR/scripts/production-readiness.sh"
rg -q 'BACKEND_SERVER_BIND=127\.0\.0\.1' "$ROOT_DIR/deploy/env/backend.env.example"

# The odds audit is a loopback operations endpoint. Public Nginx hosts proxy
# only their explicit health/API locations and must never expose readiness
# inventory endpoints.
! rg -n 'location[^\{]*/ready(/(odds|encryption))?' "$ROOT_DIR/deploy/nginx/wz6688.app.conf" "$ROOT_DIR/deploy/nginx/wz6688.split-hosts.conf"

nginx_config="$ROOT_DIR/deploy/nginx/wz6688.app.conf"
rg -q 'listen 443 ssl http2;' "$nginx_config"
rg -q 'listen 443 ssl default_server;' "$nginx_config"
rg -q 'return 444;' "$nginx_config"
rg -q 'client_max_body_size 1m;' "$nginx_config"
rg -q 'location = /api/admin/activities/upload' "$nginx_config"
rg -q 'client_max_body_size 9m;' "$nginx_config"
[[ "$(rg -c 'if \(-f /etc/wangzhe/maintenance\) \{ return 503; \}' "$nginx_config")" -eq 2 ]]
rg -q 'location = /api/ws' "$nginx_config"
safe_log_block="$(sed -n '/log_format wangzhe_safe/,/;$/p' "$nginx_config")"
grep -q '\$request_method' <<<"$safe_log_block"
grep -q '\$uri' <<<"$safe_log_block"
! grep -Eq '\$args([^_A-Za-z0-9]|$)|\$request([^_A-Za-z0-9_]|$)' <<<"$safe_log_block"
[[ "$(rg -c 'access_log /var/log/nginx/wangzhe-access\.log wangzhe_safe;' "$nginx_config")" -ge 6 ]]
[[ "$(rg -c 'error_log /dev/null crit;' "$nginx_config")" -eq 2 ]]
rg -q "connect-src 'self' wss://wz6688\.app" "$nginx_config"
rg -q "connect-src 'self' wss://admin\.wz888\.site" "$nginx_config"
! rg -n "connect-src[^;]*[[:space:]]wss:[[:space:]]*;" "$nginx_config"
split_nginx_config="$ROOT_DIR/deploy/nginx/wz6688.split-hosts.conf"
rg -q 'server_name www\.wz6688\.app;' "$split_nginx_config"
rg -q 'root /opt/wangzhe/current/admin;' "$split_nginx_config"
rg -q 'root /opt/wangzhe/current/member;' "$split_nginx_config"
rg -q 'location \^~ /api/admin/ \{ return 404; \}' "$split_nginx_config"
rg -q 'location \^~ /api/tenant/ \{ return 404; \}' "$split_nginx_config"
rg -q 'location \^~ /api/agent/ \{ return 404; \}' "$split_nginx_config"
[[ "$(rg -c 'if \(-f /etc/wangzhe/maintenance\) \{ return 503; \}' "$split_nginx_config")" -eq 2 ]]
[[ "$(rg -c 'error_log /dev/null crit;' "$split_nginx_config")" -eq 2 ]]
rg -q "connect-src 'self' wss://wz6688\.app wss://www\.wz6688\.app" "$split_nginx_config"
rg -q "connect-src 'self' wss://admin\.wz888\.site" "$split_nginx_config"
split_member_block="$(sed -n '/server_name wz6688.app www.wz6688.app;/,/^}/p' "$split_nginx_config")"
grep -Fq 'root /opt/wangzhe/current/member;' <<<"$split_member_block"
grep -Fq 'alias /etc/wangzhe/test-login/member.json;' <<<"$split_member_block"
grep -Fq 'location ^~ /api/admin/ { return 404; }' <<<"$split_member_block"
! grep -Fq 'root /opt/wangzhe/current/admin;' <<<"$split_member_block"
split_admin_block="$(sed -n '/server_name admin.wz888.site;/,/^}/p' "$split_nginx_config")"
grep -Fq 'root /opt/wangzhe/current/admin;' <<<"$split_admin_block"
grep -Fq 'alias /etc/wangzhe/test-login/admin.json;' <<<"$split_admin_block"
grep -Fq 'ssl_certificate /etc/letsencrypt/live/admin.wz888.site/fullchain.pem;' <<<"$split_admin_block"
grep -Fq 'ssl_certificate_key /etc/letsencrypt/live/admin.wz888.site/privkey.pem;' <<<"$split_admin_block"
! grep -Fq 'server_name wz888.site;' "$split_nginx_config"
rg -q 'listen 80 default_server;' "$split_nginx_config"
rg -q 'listen \[::\]:80 default_server;' "$split_nginx_config"
rg -q 'listen 443 ssl default_server;' "$split_nginx_config"
rg -q 'listen \[::\]:443 ssl default_server;' "$split_nginx_config"
rg -q 'ssl_reject_handshake on;' "$split_nginx_config"
[[ "$(rg -c 'return 444;' "$split_nginx_config")" -eq 2 ]]
split_log_block="$(sed -n '/log_format wz_current_safe/,/;$/p' "$split_nginx_config")"
for log_field in '\$remote_addr' '\$host' '\$request_method' '\$uri' '\$server_protocol' '\$status' '\$body_bytes_sent' '\$request_time' '\$request_id'; do
  grep -q "$log_field" <<<"$split_log_block" || { echo "split-host 访问日志缺少字段：$log_field" >&2; exit 1; }
done
! grep -Eq '\$args([^_A-Za-z0-9]|$)|\$request([^_A-Za-z0-9_]|$)' <<<"$split_log_block"
[[ "$(rg -c 'access_log /var/log/nginx/wangzhe-access\.log wz_current_safe;' "$split_nginx_config")" -eq 7 ]]
split_proxy_count="$(rg -c 'proxy_pass http://wz_current_backend;' "$split_nginx_config")"
[[ "$split_proxy_count" -eq 6 ]]
[[ "$(rg -c 'proxy_set_header X-Request-ID \$request_id;' "$split_nginx_config")" -eq "$split_proxy_count" ]]
[[ "$(rg -c 'proxy_set_header X-Forwarded-Host \$host;' "$split_nginx_config")" -eq "$split_proxy_count" ]]
[[ "$(rg -c 'add_header X-Request-ID \$request_id always;' "$split_nginx_config")" -eq 2 ]]
rg -Fq 'server_name wz6688.app www.wz6688.app admin.wz888.site;' "$ROOT_DIR/deploy/nginx/wangzhe-acme-bootstrap.conf"
rg -Fq 'BACKEND_SERVER_ALLOWED_ORIGINS=https://wz6688.app,https://www.wz6688.app,https://admin.wz888.site' "$ROOT_DIR/deploy/env/backend.env.example"
rg -Fq 'MONITOR_TLS_HOSTS=wz6688.app,www.wz6688.app,admin.wz888.site' "$ROOT_DIR/deploy/env/monitor.env.example"
legacy_admin_domain="admin.wz6688"'.app'
if rg -Fq "$legacy_admin_domain" "$ROOT_DIR/PRODUCTION_OPERATIONS.md" "$ROOT_DIR/deploy" "$ROOT_DIR/scripts/production-deploy.sh" "$ROOT_DIR/scripts/production-rollback.sh" "$ROOT_DIR/scripts/production-readiness.sh"; then
  echo "生产配置仍引用旧管理域名" >&2
  exit 1
fi
rg -q 'Strict-Transport-Security.*max-age=' "$ROOT_DIR/deploy/nginx/snippets/wangzhe-security-headers.conf"
rg -q 'ssl_protocols TLSv1\.2 TLSv1\.3;' "$ROOT_DIR/deploy/nginx/snippets/wangzhe-tls.conf"
rg -q 'openssl x509 -checkend 1209600' "$ROOT_DIR/scripts/production-readiness.sh"
rg -Fq 'PUBLIC_WWW_URL="${PUBLIC_WWW_URL:-https://www.wz6688.app}"' "$ROOT_DIR/scripts/production-readiness.sh"
rg -Fq 'ADMIN_URL="${ADMIN_URL:-https://admin.wz888.site}"' "$ROOT_DIR/scripts/production-readiness.sh"
rg -Fq 'ADMIN_TLS_CERT_FILE="${ADMIN_TLS_CERT_FILE:-/etc/letsencrypt/live/admin.wz888.site/fullchain.pem}"' "$ROOT_DIR/scripts/production-readiness.sh"
rg -Fq 'check_tls_certificate "会员端" "$PUBLIC_TLS_CERT_FILE"' "$ROOT_DIR/scripts/production-readiness.sh"
rg -Fq 'check_tls_certificate "管理端" "$ADMIN_TLS_CERT_FILE"' "$ROOT_DIR/scripts/production-readiness.sh"
rg -Fq 'check_https_endpoint "$PUBLIC_WWW_URL"' "$ROOT_DIR/scripts/production-readiness.sh"
rg -q "pg_tablespace.*pg_default.*pg_global" "$ROOT_DIR/scripts/production-readiness.sh"
rg -q 'PITR 流程不支持自定义 PostgreSQL 表空间' "$ROOT_DIR/scripts/production-readiness.sh"

backend_unit="$ROOT_DIR/deploy/systemd/wangzhe-backend.service"
backup_unit="$ROOT_DIR/deploy/systemd/wangzhe-backup.service"
failure_alert_unit="$ROOT_DIR/deploy/systemd/wangzhe-ops-failure-alert@.service"
rg -q 'ExecStart=/opt/wangzhe/current/bin/wangzhe-backend' "$backend_unit"
rg -q '^NoNewPrivileges=true$' "$backend_unit"
rg -q '^ProtectSystem=strict$' "$backend_unit"
rg -q '^MemoryDenyWriteExecute=true$' "$backend_unit"
rg -q '^User=wangzhe-backup$' "$backup_unit"
rg -q '/etc/wangzhe/backup.env' "$backup_unit"
rg -q '^CapabilityBoundingSet=$' "$backup_unit"
rg -q '^Environment=APPLICATION_DATABASE_USER=wangzhe$' "$backup_unit"
rg -q '^StartLimitIntervalSec=15min$' "$failure_alert_unit"
rg -q '^StartLimitBurst=5$' "$failure_alert_unit"
rg -q '^Restart=on-failure$' "$failure_alert_unit"
rg -q '^RestartSec=30s$' "$failure_alert_unit"

deploy_script="$ROOT_DIR/scripts/production-deploy.sh"
rollback_script="$ROOT_DIR/scripts/production-rollback.sh"
rewrap_script="$ROOT_DIR/scripts/production-encryption-rewrap.sh"
for executable in production-config-check.sh production-deploy.sh production-rollback.sh release-integrity.sh; do
  [[ -x "$ROOT_DIR/scripts/$executable" ]] || { echo "部署脚本不可执行：$executable" >&2; exit 1; }
done
[[ "$(sed -n '3p' "$deploy_script")" == 'export PATH=/usr/sbin:/usr/bin:/sbin:/bin' ]]
[[ "$(sed -n '3p' "$rollback_script")" == 'export PATH=/usr/sbin:/usr/bin:/sbin:/bin' ]]
deploy_command_check="$(rg '^for command_name in ' "$deploy_script" | head -n1)"
rollback_command_check="$(rg '^for command_name in ' "$rollback_script" | head -n1)"
for required_command in date env psql sleep systemd-run; do
  [[ " $deploy_command_check " == *" $required_command "* ]] || { echo "发布脚本未前置检查命令：$required_command" >&2; exit 1; }
done
for required_command in chmod ln mktemp sleep; do
  [[ " $rollback_command_check " == *" $required_command "* ]] || { echo "回滚脚本未前置检查命令：$required_command" >&2; exit 1; }
done
rg -q 'release-integrity\.sh.*verify' "$deploy_script"
rg -q 'postgres-backup\.sh' "$deploy_script"
rg -q 'mv -Tf.*CURRENT_LINK' "$deploy_script"
rg -q 'PREVIOUS_LINK=/opt/wangzhe/previous' "$deploy_script"
rg -q 'env -i PATH=/usr/bin:/bin HOME=/var/backups/wangzhe' "$deploy_script"
rg -q 'EXPECTED_MANIFEST_SHA256' "$deploy_script"
rg -q '/usr/local/libexec/wangzhe' "$deploy_script"
rg -q 'systemctl --version' "$deploy_script"
rg -q 'systemd_version_decimal >= 249' "$deploy_script"
rg -Fq "trap 'exit 130' INT" "$deploy_script"
rg -Fq "trap 'exit 143' TERM" "$deploy_script"
if rg -n '^trap .*EXIT INT TERM' "$deploy_script" >/dev/null; then
  echo "发布脚本的资源清理 trap 会吞掉终止信号" >&2
  exit 1
fi

# Fresh production installation is explicit and keeps the password out of
# argv/environment. The verified candidate performs the one-shot bootstrap
# before either the current link or the normal backend can become active.
deploy_help="$(bash "$deploy_script" --help)"
grep -Fq -- '--first-install' <<<"$deploy_help"
grep -Fq -- '--first-admin-username' <<<"$deploy_help"
grep -Fq -- '--first-admin-password-file /run/wangzhe-bootstrap-admin/password' <<<"$deploy_help"
if bash "$deploy_script" --first-admin-username platform-owner "$fixture_dir" >"$fixture_dir/unpaired-first-admin.out" 2>&1; then
  echo "没有 --first-install 的管理员引导参数被错误接受" >&2
  exit 1
fi
grep -Fq '只能与 --first-install 一起使用' "$fixture_dir/unpaired-first-admin.out"
if bash "$deploy_script" --first-install --first-admin-username platform-owner "$fixture_dir" >"$fixture_dir/missing-first-password.out" 2>&1; then
  echo "没有密码文件的首次安装被错误接受" >&2
  exit 1
fi
grep -Fq '必须同时提供首位管理员账号和受保护的密码文件' "$fixture_dir/missing-first-password.out"
rg -Fq 'FIRST_ADMIN_PASSWORD_FILE=""' "$deploy_script"
! rg -q 'FIRST_ADMIN_PASSWORD(=|:)' "$deploy_script"
rg -Fq '[[ "$FIRST_ADMIN_PASSWORD_FILE" == /run/wangzhe-bootstrap-admin/password ]]' "$deploy_script"
rg -Fq 'find "$FIRST_ADMIN_PASSWORD_FILE" -perm /077' "$deploy_script"
rg -Fq 'stat -c '\''%h'\'' "$FIRST_ADMIN_PASSWORD_FILE"' "$deploy_script"
rg -Fq 'stat -c '\''%d:%i'\'' "$FIRST_ADMIN_PASSWORD_FILE"' "$deploy_script"
rg -Fq 'LoadCredential=admin-password:$FIRST_ADMIN_PASSWORD_FILE' "$deploy_script"
rg -Fq 'bootstrap_credential="/run/credentials/${bootstrap_unit}.service/admin-password"' "$deploy_script"
rg -Fq '"$target/bin/wangzhe-bootstrap-admin"' "$deploy_script"
rg -Fq -- '--username "$FIRST_ADMIN_USERNAME" --password-file "$bootstrap_credential"' "$deploy_script"
rg -Fq 'cleanup_bootstrap_unit() {' "$deploy_script"
rg -Fq 'systemctl stop "$unit_name"' "$deploy_script"
rg -Fq 'systemctl show "$unit_name" --property=LoadState --property=ActiveState' "$deploy_script"
rg -Fq 'WANGZHE_BOOTSTRAP_UNIT_CLEANUP_FAILED' "$deploy_script"
rg -Fq 'trap cleanup_deploy_exit EXIT' "$deploy_script"
rg -Fq 'cleanup_bootstrap_unit || cleanup_failed=1' "$deploy_script"
rg -Fq 'rm -f -- "$FIRST_ADMIN_PASSWORD_FILE"' "$deploy_script"
rg -Fq '首位管理员创建失败；后端尚未启动，维护模式和已安装候选版本保持不变' "$deploy_script"
rg -Fq 'initial_database_state=empty-schema' "$deploy_script"
rg -Fq 'initial_database_state=empty-users' "$deploy_script"
rg -Fq 'initial_database_state=active-admin' "$deploy_script"
rg -Fq 'FROM public.\"user\";' "$deploy_script"
rg -Fq '空生产库必须显式使用 --first-install' "$deploy_script"
rg -Fq '数据库已有账号但没有可用平台管理员' "$deploy_script"
rg -Fq 'public schema 已有非本应用关系且缺少 user 表' "$deploy_script"
for hardening_property in NoNewPrivileges PrivateTmp PrivateDevices ProtectHome ProtectSystem ProtectHostname ProtectProc ProcSubset ProtectKernelTunables ProtectKernelModules ProtectKernelLogs ProtectControlGroups ProtectClock RestrictNamespaces RestrictRealtime RestrictSUIDSGID RemoveIPC RestrictAddressFamilies CapabilityBoundingSet LockPersonality MemoryDenyWriteExecute SystemCallArchitectures; do
  rg -q -- "--property=.*${hardening_property}" "$deploy_script" || { echo "首次安装 transient service 缺少加固：$hardening_property" >&2; exit 1; }
done
bootstrap_run_line="$(rg -n '^  if ! systemd-run .*--service-type=exec' "$deploy_script" | cut -d: -f1)"
bootstrap_arm_line="$(rg -n '^  bootstrap_unit_cleanup_armed=1$' "$deploy_script" | cut -d: -f1)"
bootstrap_disarm_line="$(rg -n '^  bootstrap_unit_cleanup_armed=0$' "$deploy_script" | tail -n1 | cut -d: -f1)"
backend_restart_line="$(rg -n '^if ! systemctl restart wangzhe-backend\.service' "$deploy_script" | cut -d: -f1)"
[[ -n "$bootstrap_arm_line" && -n "$bootstrap_run_line" && -n "$bootstrap_disarm_line" && -n "$backend_restart_line" && \
   "$bootstrap_arm_line" -lt "$bootstrap_run_line" && "$bootstrap_run_line" -lt "$bootstrap_disarm_line" && \
   "$bootstrap_disarm_line" -lt "$backend_restart_line" ]] || {
  echo "首次管理员没有在正常后端启动前创建" >&2
  exit 1
}
password_remove_line="$(rg -n '^  rm -f -- "\$FIRST_ADMIN_PASSWORD_FILE"$' "$deploy_script" | cut -d: -f1)"
[[ -n "$password_remove_line" && "$bootstrap_run_line" -lt "$password_remove_line" && "$password_remove_line" -lt "$backend_restart_line" ]] || {
  echo "首次管理员密码源没有在 bootstrap 后、正常后端启动前清理" >&2
  exit 1
}
systemd_gate_line="$(rg -n '^systemd_version=' "$deploy_script" | head -n1 | cut -d: -f1)"
source_resolution_line="$(rg -n '^SOURCE_DIR=' "$deploy_script" | head -n1 | cut -d: -f1)"
[[ -n "$systemd_gate_line" && -n "$source_resolution_line" && "$systemd_gate_line" -lt "$source_resolution_line" ]] || {
  echo "systemd 249 版本门禁没有在解析发布包前执行" >&2
  exit 1
}
! rg -q '"\$SOURCE_DIR/scripts/release-integrity\.sh" verify' "$deploy_script"
rg -q 'RELEASE_ROOT/\.staging-' "$deploy_script"
rg -q 'mv -T "\$staging" "\$target"' "$deploy_script"
rg -q 'validate_secure_source_path "\$SOURCE_DIR"' "$deploy_script"
rg -q 'find "\$SOURCE_DIR" -mindepth 1 ! -user root -print -quit' "$deploy_script"
owner_fixture="$fixture_dir/non-root-release-entry"
mkdir -p "$owner_fixture"
printf '%s\n' artifact >"$owner_fixture/file"
if (( EUID == 0 )); then
  chown 65534 "$owner_fixture/file"
fi
[[ -n "$(find "$owner_fixture" -mindepth 1 ! -user root -print -quit)" ]] || {
  echo "内层非 root 所有者负例未被识别" >&2
  exit 1
}
[[ "$(rg -c 'release-integrity\.sh.*verify' "$deploy_script")" -ge 2 ]] || {
  echo "发布脚本必须在复制前后各校验一次完整性" >&2
  exit 1
}
backup_line="$(rg -n 'postgres-backup\.sh.*BACKUP_ENV' "$deploy_script" | head -n1 | cut -d: -f1)"
switch_line="$(rg -n 'mv -Tf.*CURRENT_LINK' "$deploy_script" | head -n1 | cut -d: -f1)"
previous_switch_line="$(rg -n 'mv -Tf.*PREVIOUS_LINK' "$deploy_script" | head -n1 | cut -d: -f1)"
[[ -n "$backup_line" && -n "$switch_line" && "$backup_line" -lt "$switch_line" ]] || {
  echo "发布脚本没有保证先备份再切换版本" >&2
  exit 1
}
[[ -n "$bootstrap_run_line" && "$bootstrap_run_line" -lt "$switch_line" && "$switch_line" -lt "$backend_restart_line" ]] || {
  echo "首次管理员、current 切换与正常后端启动顺序不安全" >&2
  exit 1
}
maintenance_line="$(rg -n 'ensure_maintenance_marker "\$MAINTENANCE_FLAG"' "$deploy_script" | head -n1 | cut -d: -f1)"
staging_create_line="$(rg -n '^install -d -o root -g root -m 0755 "\$staging"$' "$deploy_script" | cut -d: -f1)"
gate_line="$(rg -n 'production-readiness\.sh' "$deploy_script" | tail -n1 | cut -d: -f1)"
remove_maintenance_line="$(rg -n 'finish_maintenance_marker "\$MAINTENANCE_FLAG"' "$deploy_script" | tail -n1 | cut -d: -f1)"
[[ -n "$maintenance_line" && -n "$gate_line" && -n "$remove_maintenance_line" && "$maintenance_line" -lt "$switch_line" && "$switch_line" -lt "$gate_line" && "$gate_line" -lt "$remove_maintenance_line" ]] || {
  echo "发布脚本没有在完整门禁期间保持维护模式" >&2
  exit 1
}
[[ -n "$staging_create_line" && "$maintenance_line" -lt "$staging_create_line" ]] || {
  echo "首次安装阶段 1 没有在准备候选版本前验证维护模式" >&2
  exit 1
}
capture_maintenance_line="$(rg -n 'capture_maintenance_marker_state "\$MAINTENANCE_FLAG"' "$deploy_script" | head -n1 | cut -d: -f1)"
[[ -n "$capture_maintenance_line" && "$capture_maintenance_line" -lt "$backup_line" && "$capture_maintenance_line" -lt "$maintenance_line" ]] || {
  echo "发布脚本没有在操作前记录维护标记状态" >&2
  exit 1
}
! rg -q 'rm -f -- "\$MAINTENANCE_FLAG"|install .*MAINTENANCE_FLAG' "$deploy_script"
rg -q 'ALLOW_MAINTENANCE_503=1' "$deploy_script"
rg -q 'validate_secure_source_path "\$SOURCE_DIR"' "$deploy_script"
rg -q 'staging_manifest_digest=.*SHA256SUMS' "$deploy_script"
rg -q 'validate_member_betting_contract "\$SOURCE_DIR"' "$deploy_script"
rg -q 'validate_member_betting_contract "\$staging"' "$deploy_script"
rg -Fq "mark6-v2" "$deploy_script"
rg -Fq "mark-six-bet-board" "$deploy_script"
rg -Fq "web-bets" "$deploy_script"
! rg -Fq "mark6-v1" "$deploy_script" "$ROOT_DIR/Makefile"

# Exercise only the pure package-content predicate, never the deploy flow.
# A current-only backend must pass both package creation and deployment checks;
# requiring a retired engine would make every new build undeployable.
contract_helper="$fixture_dir/betting-contract-function.sh"
sed -n '/^validate_member_betting_contract() {$/,/^}$/p' "$deploy_script" >"$contract_helper"
# shellcheck disable=SC1090
source "$contract_helper"
declare -F validate_member_betting_contract >/dev/null
current_mark_six_rule="$(sed -nE 's/^[[:space:]]*markSixRuleVersion[[:space:]]*=[[:space:]]*"([^"]+)".*/\1/p' "$ROOT_DIR/backend/services/mark_six_rules.go")"
[[ -n "$current_mark_six_rule" ]] || { echo "无法读取当前六合彩规则版本" >&2; exit 1; }
rg -Fq "'$current_mark_six_rule'" "$deploy_script" "$ROOT_DIR/Makefile"
contract_workspace="$fixture_dir/betting-contract"
contract_candidate="$contract_workspace/release"
mkdir -p "$contract_candidate/bin" "$contract_candidate/member/assets" "$contract_candidate/scripts/lib"
printf '%s\n' "$current_mark_six_rule" >"$contract_candidate/bin/wangzhe-backend"
chmod 700 "$contract_candidate/bin/wangzhe-backend"
printf '%s\n' 'aggregate-only encryption audit fixture' >"$contract_candidate/bin/wangzhe-field-encryption-audit"
chmod 700 "$contract_candidate/bin/wangzhe-field-encryption-audit"
printf '%s\n' \
  'format_version=1' \
  'read_versions=1,2' \
  'write_version=2' \
  'previous_key_fallback=true' \
  >"$contract_candidate/FIELD_ENCRYPTION_CAPABILITIES"
printf '%s\n' '#!/usr/bin/env bash' 'exit 0' >"$contract_candidate/scripts/production-encryption-rewrap.sh"
chmod 700 "$contract_candidate/scripts/production-encryption-rewrap.sh"
printf '%s\n' '# release capability parser fixture' >"$contract_candidate/scripts/lib/encryption-capabilities.sh"
printf '%s\n' 'mark-six-bet-board web-bets' >"$contract_candidate/member/assets/member.js"
validate_member_betting_contract "$contract_candidate"
make --no-print-directory -C "$contract_workspace" -f "$ROOT_DIR/Makefile" release-contract-check
printf '%s\n' 'mark6-v1' >"$contract_candidate/bin/wangzhe-backend"
if validate_member_betting_contract "$contract_candidate" >/dev/null 2>&1; then
  echo "仅包含旧规则的后端通过了发布契约检查" >&2
  exit 1
fi
if make --no-print-directory -C "$contract_workspace" -f "$ROOT_DIR/Makefile" release-contract-check >/dev/null 2>&1; then
  echo "仅包含旧规则的后端通过了构建契约检查" >&2
  exit 1
fi
printf '%s\n' "$current_mark_six_rule" >"$contract_candidate/bin/wangzhe-backend"
for incomplete_member in mark-six-bet-board web-bets; do
  printf '%s\n' "$incomplete_member" >"$contract_candidate/member/assets/member.js"
  if validate_member_betting_contract "$contract_candidate" >/dev/null 2>&1; then
    echo "缺少面板或批量接口的会员端通过了发布契约检查" >&2
    exit 1
  fi
done
edge_check_line="$(rg -n 'verify_maintenance_edge "\$PUBLIC_URL" "\$PUBLIC_WWW_URL" "\$ADMIN_URL"' "$deploy_script" | head -n1 | cut -d: -f1)"
[[ -n "$edge_check_line" && "$edge_check_line" -lt "$switch_line" ]] || {
  echo "发布脚本没有在切换版本前实测公网维护模式" >&2
  exit 1
}
[[ -n "$previous_switch_line" && "$edge_check_line" -lt "$previous_switch_line" && "$previous_switch_line" -lt "$switch_line" ]] || {
  echo "previous 链接没有在维护入口确认后与 current 一起交换" >&2
  exit 1
}
rg -Fq 'restore_release_link_state "$CURRENT_LINK" "$current_link_original_present" "$current_link_original_target"' "$deploy_script"
rg -Fq 'restore_release_link_state "$PREVIOUS_LINK" "$previous_link_original_present" "$previous_link_original_target"' "$deploy_script"
rg -Fq 'link_transaction_armed=1' "$deploy_script"
rg -Fq 'link_transaction_committed=1' "$deploy_script"
rg -Fq 'FIRST_INSTALL_PHASE_MARKER_PREFIX=wangzhe-first-install-phase1:v2' "$deploy_script"
rg -Fq 'persist_first_install_phase_marker' "$deploy_script"
rg -Fq 'persist_first_install_pending_marker' "$deploy_script"
rg -Fq 'adopt_first_install_phase_marker' "$deploy_script"
rg -Fq 'capture_first_install_backup_binding' "$deploy_script"
rg -Fq 'backup_completed_at_epoch=%s' "$deploy_script"
rg -Fq 'database_cipher_sha256=%s' "$deploy_script"
rg -Fq 'basebackup_cipher_sha256=%s' "$deploy_script"
rg -Fq 'cleanup_phase_marker_tmp || cleanup_failed=1' "$deploy_script"
rg -Fq 'maintenance_marker_created=1' "$deploy_script"
rg -Fq 'phase_one_maintenance_adopted=1' "$deploy_script"

phase_one_complete_line="$(rg -n '^  complete_first_install_phase_one$' "$deploy_script" | cut -d: -f1)"
phase_one_exit_line="$(awk -v start="$phase_one_complete_line" 'NR > start && /^  exit 0$/ { print NR; exit }' "$deploy_script")"
[[ -n "$phase_one_complete_line" && -n "$phase_one_exit_line" && \
   "$bootstrap_run_line" -lt "$phase_one_complete_line" && "$phase_one_complete_line" -lt "$phase_one_exit_line" && \
   "$phase_one_exit_line" -lt "$switch_line" ]] || {
  echo "首次安装阶段 1 没有在 bootstrap 后备份并于 current 切换前停止" >&2
  exit 1
}
rg -Fq 'create_and_verify_backup_set 1' "$deploy_script"
rg -Fq 'run_pre_release_recovery_gates' "$deploy_script"
rg -Fq 'timeout 10m "$TRUSTED_SCRIPT_DIR/production-recovery-evidence-check.sh"' "$deploy_script"
if rg -q 'SKIP_RECOVERY|ALLOW_MISSING_RECOVERY|fake.*recovery|伪造.*恢复' "$deploy_script"; then
  echo "发布脚本包含永久绕过或伪造恢复证据的路径" >&2
  exit 1
fi

# Dynamically exercise the exact link rollback helpers and transaction body.
# The test mv shim only translates GNU mv -T for macOS; failure/signal injection
# still occurs at the real boundary between previous and current replacement.
write_deploy_link_harness() {
  local harness_path="$1"
  {
    printf '%s\n' \
      '#!/usr/bin/env bash' \
      'set -euo pipefail' \
      'test_mode="$1"' \
      'case_root="$2"' \
      'CURRENT_LINK="$case_root/current"' \
      'PREVIOUS_LINK="$case_root/previous"' \
      'previous_target="$case_root/releases/current-release"' \
      'target="$case_root/releases/new-release"' \
      'current_link_original_present=1' \
      'current_link_original_target="$(readlink "$CURRENT_LINK")"' \
      'previous_link_original_present=1' \
      'previous_link_original_target="$(readlink "$PREVIOUS_LINK")"' \
      'link_tmp="$case_root/.current-new"' \
      'previous_tmp="$case_root/.previous-new"' \
      'current_restore_tmp="$case_root/.current-restore"' \
      'previous_restore_tmp="$case_root/.previous-restore"' \
      'link_transaction_armed=0' \
      'link_transaction_committed=0' \
      'injected=0' \
      'mv() {' \
      '  if [[ "${1:-}" != -Tf ]]; then command mv "$@"; return; fi' \
      '  local source_path="$2" destination_path="$3"' \
      '  if [[ "$test_mode" == fail-current && "$destination_path" == "$CURRENT_LINK" && "$injected" == 0 ]]; then' \
      '    injected=1' \
      '    : >"$case_root/link-window-hit"' \
      '    return 1' \
      '  fi' \
      '  command rm -f -- "$destination_path"' \
      '  command mv "$source_path" "$destination_path"' \
      '  if [[ "$test_mode" == term-after-previous && "$destination_path" == "$PREVIOUS_LINK" && "$injected" == 0 ]]; then' \
      '    injected=1' \
      '    : >"$case_root/link-window-hit"' \
      '    kill -s TERM "$$"' \
      '  fi' \
      '}'
    sed -n '/^restore_release_link_state() {$/,/^}$/p' "$deploy_script"
    sed -n '/^cleanup_link() {$/,/^}$/p' "$deploy_script"
    printf '%s\n' 'trap cleanup_link EXIT'
    grep -F "trap 'exit 143' TERM" "$deploy_script" | head -n1
    sed -n '/^# BEGIN release-link-transaction$/,/^# END release-link-transaction$/p' "$deploy_script"
    printf '%s\n' ': >"$case_root/continued-after-link-transaction"'
  } >"$harness_path"
}

run_deploy_link_case() {
  local test_mode="$1" expected_status="$2"
  local case_root="$fixture_dir/deploy-link-$test_mode" harness_path status
  mkdir -p "$case_root/releases/current-release" "$case_root/releases/previous-release" "$case_root/releases/new-release"
  ln -s "$case_root/releases/current-release" "$case_root/current"
  ln -s "$case_root/releases/previous-release" "$case_root/previous"
  harness_path="$case_root/harness.sh"
  write_deploy_link_harness "$harness_path"
  set +e
  bash "$harness_path" "$test_mode" "$case_root" >/dev/null 2>&1
  status=$?
  set -e
  [[ "$status" == "$expected_status" ]] || { echo "发布链接事务 $test_mode 退出码错误：$status" >&2; exit 1; }
  case "$test_mode" in
    success)
      [[ "$(readlink "$case_root/current")" == "$case_root/releases/new-release" && \
         "$(readlink "$case_root/previous")" == "$case_root/releases/current-release" && \
         -f "$case_root/continued-after-link-transaction" ]] || {
        echo "发布链接事务成功路径没有提交成对链接" >&2
        exit 1
      }
      ;;
    fail-current|term-after-previous)
      [[ -f "$case_root/link-window-hit" && ! -e "$case_root/continued-after-link-transaction" && \
         "$(readlink "$case_root/current")" == "$case_root/releases/current-release" && \
         "$(readlink "$case_root/previous")" == "$case_root/releases/previous-release" ]] || {
        echo "发布链接事务 $test_mode 没有恢复原始 current/previous" >&2
        exit 1
      }
      ;;
    *) echo "未知发布链接事务测试：$test_mode" >&2; exit 1 ;;
  esac
}

run_deploy_link_case success 0
run_deploy_link_case fail-current 1
run_deploy_link_case term-after-previous 143

# A foreground child self-signals so macOS and Linux both exercise real
# INT/TERM handling without background shells inheriting ignored SIGINT.
write_bootstrap_signal_harness() {
  local harness_path="$1"
  {
    printf '%s\n' \
      '#!/usr/bin/env bash' \
      'set -euo pipefail' \
      'test_signal="$1"' \
      'case_root="$2"' \
      'unit_state_mode="$3"' \
      'systemctl_log="$case_root/systemctl.log"' \
      'bootstrap_unit=wangzhe-bootstrap-admin-4242' \
      'bootstrap_unit_cleanup_armed=1' \
      'systemctl() {' \
      '  printf '\''%s\n'\'' "$*" >>"$systemctl_log"' \
      '  case "${1:-}" in' \
      '    stop|kill) return 0 ;;' \
      '    show)' \
      '      if [[ "$unit_state_mode" == stuck ]]; then printf '\''LoadState=loaded\nActiveState=active\n'\''; else printf '\''LoadState=not-found\nActiveState=inactive\n'\''; fi' \
      '      return 0' \
      '      ;;' \
      '    *) return 1 ;;' \
      '  esac' \
      '}' \
      'sleep() { :; }' \
      'cleanup_first_admin_password() {' \
      '  printf '\''%s\n'\'' "$bootstrap_unit_cleanup_armed" >"$case_root/armed-after-cleanup"' \
      '  : >"$case_root/cleanup-finished"' \
      '}'
    sed -n '/^cleanup_bootstrap_unit() {$/,/^}$/p' "$deploy_script"
    sed -n '/^cleanup_deploy_exit() {$/,/^}$/p' "$deploy_script"
    grep -F 'trap cleanup_deploy_exit EXIT' "$deploy_script" | head -n1
    grep -F "trap 'exit 130' INT" "$deploy_script" | head -n1
    grep -F "trap 'exit 143' TERM" "$deploy_script" | head -n1
    printf '%s\n' \
      'if [[ "$test_signal" == EXIT ]]; then exit 0; fi' \
      'kill -s "$test_signal" "$$"' \
      ': >"$case_root/continued-after-bootstrap-signal"'
  } >"$harness_path"
}

run_bootstrap_signal_case() {
  local signal_name="$1" expected_status="$2" unit_state_mode="$3"
  local case_root="$fixture_dir/bootstrap-signal-$signal_name-$unit_state_mode" harness_path status
  mkdir -p "$case_root"
  harness_path="$case_root/harness.sh"
  write_bootstrap_signal_harness "$harness_path"
  set +e
  bash "$harness_path" "$signal_name" "$case_root" "$unit_state_mode" >"$case_root/output" 2>&1
  status=$?
  set -e
  [[ "$status" == "$expected_status" && -f "$case_root/cleanup-finished" && \
     ! -e "$case_root/continued-after-bootstrap-signal" ]] || {
    echo "首次安装 transient unit 的 $signal_name 清理未终止发布" >&2
    exit 1
  }
  grep -Fxq 'stop wangzhe-bootstrap-admin-4242.service' "$case_root/systemctl.log"
  grep -Fxq 'show wangzhe-bootstrap-admin-4242.service --property=LoadState --property=ActiveState' "$case_root/systemctl.log"
  if grep -Fv 'wangzhe-bootstrap-admin-4242.service' "$case_root/systemctl.log" >/dev/null; then
    echo "首次安装信号清理触及了本次之外的 unit" >&2
    exit 1
  fi
  if [[ "$unit_state_mode" == stuck ]]; then
    grep -Fq 'kill --kill-whom=all --signal=KILL wangzhe-bootstrap-admin-4242.service' "$case_root/systemctl.log"
    grep -Fq 'WANGZHE_BOOTSTRAP_UNIT_CLEANUP_FAILED unit=wangzhe-bootstrap-admin-4242.service' "$case_root/output"
    grep -Fq 'WANGZHE_DEPLOY_CLEANUP_FAILED original_status=' "$case_root/output"
    [[ "$(cat "$case_root/armed-after-cleanup")" == 1 ]] || { echo "未确认停止的 transient unit 被错误解除清理状态" >&2; exit 1; }
  else
    [[ "$(cat "$case_root/armed-after-cleanup")" == 0 ]] || { echo "已停止的 transient unit 没有解除清理状态" >&2; exit 1; }
  fi
}

run_bootstrap_signal_case TERM 143 inactive
run_bootstrap_signal_case INT 130 inactive
run_bootstrap_signal_case TERM 143 stuck
run_bootstrap_signal_case EXIT 1 stuck

# Exercise the exact two-phase gate functions with command doubles. Phase 1
# may only create the real post-bootstrap backup set; every phase-2 shape must
# execute the recovery evidence checker, including a host with no current yet.
deploy_phase_functions="$fixture_dir/deploy-phase-functions.sh"
{
  sed -n '/^run_pre_release_recovery_gates() {$/,/^}$/p' "$deploy_script"
  sed -n '/^complete_first_install_phase_one() {$/,/^}$/p' "$deploy_script"
} >"$deploy_phase_functions"
# shellcheck disable=SC1090
source "$deploy_phase_functions"
phase_log="$fixture_dir/deploy-phase.log"
create_and_verify_backup_set() { printf 'backup:%s\n' "$1" >>"$phase_log"; }
capture_first_install_backup_binding() { printf '%s\n' backup-binding >>"$phase_log"; }
persist_first_install_phase_marker() { printf '%s\n' phase-marker >>"$phase_log"; }
timeout() { printf 'evidence:%s\n' "$*" >>"$phase_log"; }
# These globals are consumed by the exact functions sourced just above;
# ShellCheck cannot follow that generated helper file.
# shellcheck disable=SC2034
TRUSTED_SCRIPT_DIR=/trusted
FIRST_INSTALL=1
previous_target=""
: >"$phase_log"
run_pre_release_recovery_gates >/dev/null
[[ ! -s "$phase_log" ]] || { echo "首次安装阶段 1 在 bootstrap 前伪造或读取了恢复证据" >&2; exit 1; }
complete_first_install_phase_one >/dev/null
[[ "$(cat "$phase_log")" == $'backup:1\nbackup-binding\nphase-marker' ]] || { echo "首次安装阶段 1 没有绑定完整备份集合并持久化阶段标记" >&2; exit 1; }
# shellcheck disable=SC2034
FIRST_INSTALL=0
: >"$phase_log"
run_pre_release_recovery_gates >/dev/null
grep -Fxq 'backup:1' "$phase_log"
grep -Fxq 'evidence:10m /trusted/production-recovery-evidence-check.sh' "$phase_log"
# shellcheck disable=SC2034
previous_target=/opt/wangzhe/releases/current
: >"$phase_log"
run_pre_release_recovery_gates >/dev/null
grep -Fxq 'backup:0' "$phase_log"
grep -Fxq 'evidence:10m /trusted/production-recovery-evidence-check.sh' "$phase_log"
unset -f timeout create_and_verify_backup_set capture_first_install_backup_binding persist_first_install_phase_marker run_pre_release_recovery_gates complete_first_install_phase_one

# Verify the cross-process phase marker handshake with the exact production
# persist/adopt functions. A phase-1-owned marker is removed only after phase 2
# explicitly adopts it; an operator marker survives, and a mismatched release
# digest is rejected without changing the marker.
phase_marker_functions="$fixture_dir/deploy-phase-marker-functions.sh"
{
  sed -n '/^cleanup_phase_marker_tmp() {$/,/^}$/p' "$deploy_script"
  sed -n '/^phase_marker_field() {$/,/^}$/p' "$deploy_script"
  sed -n '/^replace_first_install_phase_marker() {$/,/^}$/p' "$deploy_script"
  sed -n '/^persist_first_install_pending_marker() {$/,/^}$/p' "$deploy_script"
  sed -n '/^persist_first_install_phase_marker() {$/,/^}$/p' "$deploy_script"
  sed -n '/^adopt_first_install_phase_marker() {$/,/^}$/p' "$deploy_script"
} >"$phase_marker_functions"
# shellcheck disable=SC1090
source "$phase_marker_functions"
phase_marker_root="$fixture_dir/deploy-phase-marker"
mkdir -p "$phase_marker_root/releases/bootstrap-1"
printf '%s\n' manifest >"$phase_marker_root/releases/bootstrap-1/SHA256SUMS"
phase_manifest="$(sha256sum "$phase_marker_root/releases/bootstrap-1/SHA256SUMS" | awk '{print $1}')"
validate_installed_release() { [[ -d "$1" && -f "$1/SHA256SUMS" ]]; }
stat() {
  if [[ "${1:-}" == -c ]]; then
    case "$2" in
      %u) printf '%s\n' 0 ;;
      %a) printf '%s\n' 644 ;;
      %h) printf '%s\n' 1 ;;
      %s) wc -c <"$3" | tr -d '[:space:]' ;;
      *) return 1 ;;
    esac
  else
    command stat "$@"
  fi
}
MAINTENANCE_FLAG="$phase_marker_root/maintenance"
RELEASE_ROOT="$phase_marker_root/releases"
# Consumed by the dynamically sourced production functions.
# shellcheck disable=SC2034
PREVIOUS_LINK="$phase_marker_root/previous"
export FIRST_INSTALL_PHASE_MARKER_PREFIX=wangzhe-first-install-phase1:v2
# shellcheck disable=SC2034
EXPECTED_MANIFEST_SHA256="$phase_manifest"
RELEASE_ID=bootstrap-1
# shellcheck disable=SC2034
target="$RELEASE_ROOT/$RELEASE_ID"
FIRST_INSTALL=1
maintenance_was_active=0
maintenance_marker_created=1
maintenance_marker_token=phase-one-owner-token
printf '%s\n' "$maintenance_marker_token" >"$MAINTENANCE_FLAG"
export phase_marker_tmp=""
export phase_payload_tmp=""
export first_install_backup_completed_epoch=1700000000
export first_install_database_artifact_name=wangzhe-20260830-000000-1.dump.age
export first_install_database_cipher_sha256=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
export first_install_upload_artifact_name=uploads-20260830-000000-1.tar.age
export first_install_upload_cipher_sha256=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
export first_install_basebackup_artifact_name=basebackup-20260830-000000-1.tar.age
export first_install_basebackup_cipher_sha256=cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
export first_install_wal_inventory_sha256=dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd
persist_first_install_phase_marker >/dev/null
grep -Fxq 'status=awaiting-recovery' "$MAINTENANCE_FLAG"
grep -Fxq "manifest_sha256=${phase_manifest}" "$MAINTENANCE_FLAG"
phase_ready_token="$(head -n1 "$MAINTENANCE_FLAG")"
[[ "$phase_ready_token" =~ ^wangzhe-first-install-phase1:v2:[0-9a-f]{64}$ ]]

# Simulate the new phase-2 process and then its post-gate finish operation.
capture_maintenance_marker_state "$MAINTENANCE_FLAG"
# shellcheck disable=SC2034
export FIRST_INSTALL=0
# shellcheck disable=SC2034
previous_target=""
# shellcheck disable=SC2034
initial_database_state=active-admin
phase_one_maintenance_adopted=0
adopt_first_install_phase_marker >/dev/null
[[ "$maintenance_was_active" == 1 && "$maintenance_marker_created" == 1 && "$phase_one_maintenance_adopted" == 1 ]]
cp "$MAINTENANCE_FLAG" "$phase_marker_root/ready-marker"
finish_maintenance_marker "$MAINTENANCE_FLAG"
[[ ! -e "$MAINTENANCE_FLAG" && ! -L "$MAINTENANCE_FLAG" ]] || { echo "阶段 2 没有解除自己严格接管的阶段 1 维护标记" >&2; exit 1; }

printf '%s\n' operator-owned-marker >"$MAINTENANCE_FLAG"
capture_maintenance_marker_state "$MAINTENANCE_FLAG"
phase_marker_status=0
adopt_first_install_phase_marker >/dev/null 2>&1 || phase_marker_status=$?
[[ "$phase_marker_status" == 2 && "$maintenance_marker_created" == 0 ]]
finish_maintenance_marker "$MAINTENANCE_FLAG"
grep -Fxq operator-owned-marker "$MAINTENANCE_FLAG" || { echo "阶段 2 错误删除了操作员维护标记" >&2; exit 1; }

cp "$phase_marker_root/ready-marker" "$MAINTENANCE_FLAG"
sed -i.bak 's/^manifest_sha256=.*/manifest_sha256=0000000000000000000000000000000000000000000000000000000000000000/' "$MAINTENANCE_FLAG"
rm -f "$MAINTENANCE_FLAG.bak"
capture_maintenance_marker_state "$MAINTENANCE_FLAG"
phase_marker_status=0
adopt_first_install_phase_marker >/dev/null 2>&1 || phase_marker_status=$?
[[ "$phase_marker_status" == 1 && -f "$MAINTENANCE_FLAG" ]] || { echo "阶段 2 接受或改写了摘要不匹配的阶段标记" >&2; exit 1; }

# A durable pending marker left after bootstrap starts must never be treated as
# an ordinary restored active-admin deployment.
FIRST_INSTALL=1
maintenance_was_active=0
maintenance_marker_created=1
maintenance_marker_token=phase-one-owner-token
printf '%s\n' "$maintenance_marker_token" >"$MAINTENANCE_FLAG"
persist_first_install_pending_marker >/dev/null
capture_maintenance_marker_state "$MAINTENANCE_FLAG"
export FIRST_INSTALL=0
phase_marker_status=0
adopt_first_install_phase_marker >/dev/null 2>&1 || phase_marker_status=$?
[[ "$phase_marker_status" == 1 && -f "$MAINTENANCE_FLAG" ]] || { echo "阶段 2 将 bootstrap-pending 中断状态当作普通 active-admin" >&2; exit 1; }
unset -f stat validate_installed_release cleanup_phase_marker_tmp phase_marker_field replace_first_install_phase_marker persist_first_install_pending_marker persist_first_install_phase_marker adopt_first_install_phase_marker

release_fixture="$fixture_dir/release"
mkdir -p "$release_fixture/sub"
printf '%s\n' 'artifact' >"$release_fixture/sub/file"
bash "$ROOT_DIR/scripts/release-integrity.sh" generate "$release_fixture" >/dev/null
bash "$ROOT_DIR/scripts/release-integrity.sh" verify "$release_fixture" >/dev/null
trusted_manifest_digest="$(sha256sum "$release_fixture/SHA256SUMS" | awk '{print $1}')"
printf '%s\n' 'tampered' >"$release_fixture/sub/file"
if bash "$ROOT_DIR/scripts/release-integrity.sh" verify "$release_fixture" >/dev/null 2>&1; then
  echo "被修改的发布包通过了完整性检查" >&2
  exit 1
fi
bash "$ROOT_DIR/scripts/release-integrity.sh" generate "$release_fixture" >/dev/null
replacement_manifest_digest="$(sha256sum "$release_fixture/SHA256SUMS" | awk '{print $1}')"
[[ "$replacement_manifest_digest" != "$trusted_manifest_digest" ]] || {
  echo "整体替换发布包后可信清单摘要没有变化" >&2
  exit 1
}
rm "$release_fixture/sub/file"
printf '%s\n' 'artifact' >"$release_fixture/real-file"
ln -s real-file "$release_fixture/symlink-file"
if bash "$ROOT_DIR/scripts/release-integrity.sh" generate "$release_fixture" >/dev/null 2>&1; then
  echo "包含符号链接的发布包被错误接受" >&2
  exit 1
fi
rm "$release_fixture/symlink-file" "$release_fixture/real-file"
newline_file="$release_fixture/"$'bad\nname'
printf '%s\n' 'artifact' >"$newline_file"
if bash "$ROOT_DIR/scripts/release-integrity.sh" generate "$release_fixture" >/dev/null 2>&1; then
  echo "文件名含换行的发布包被错误接受" >&2
  exit 1
fi

rg -q '最低版本是 1\.25.*Go 1\.26\.7' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
rg -q '生产服务器运行预编译二进制，不需要安装 Go' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
rg -q '会员证书覆盖.*wz6688\.app.*www\.wz6688\.app.*管理证书.*admin\.wz888\.site' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
rg -Fq -- '--first-admin-password-file /run/wangzhe-bootstrap-admin/password' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
rg -Fq 'LoadCredential=' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
rg -Fq '阶段 1 只在公网已确认维护 503 后' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
rg -Fq 'current` 尚未切换' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
rg -q '阶段 2 .*新的 `RELEASE_ID`' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
rg -q '维护标记.*不会改写、接管或自动删除' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
rg -Fq 'WANGZHE_BOOTSTRAP_UNIT_CLEANUP_FAILED' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
rg -Fq 'wangzhe-production-encryption-rewrap --dry-run 1 100' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
rg -Fq 'wangzhe-production-encryption-rewrap --execute 1 100' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
rg -Fq '最终盘点到编辑配置之间必须保持后端停止和维护 503' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
rg -Fq 'WANGZHE_ENCRYPTION_REWRAP_CLEANUP_FAILED' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
rg -Fq '缺少能力元数据或审计工具的历史 release 也一律拒绝' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
! rg -Fq '本实现有意不自动批量重加密数据库字段' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
rg -q 'chown -R root:root /tmp/wangzhe-release' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
rg -Fq 'chmod -R a+rX,go-w /tmp/wangzhe-release' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
rg -Fq 'RELEASE_GOOS ?= linux' "$ROOT_DIR/Makefile"
rg -Fq 'RELEASE_GOARCH ?= amd64' "$ROOT_DIR/Makefile"
rg -q '^release: verify readiness-test$' "$ROOT_DIR/Makefile"
rg -q '^release-contract-check:' "$ROOT_DIR/Makefile"
rg -Fq '$(MAKE) release-contract-check' "$ROOT_DIR/Makefile"
rg -Fq 'GOOS=$(RELEASE_GOOS) GOARCH=$(RELEASE_GOARCH)' "$ROOT_DIR/Makefile"
rg -Fq 'wangzhe-field-encryption-audit ./cmd/field-encryption-audit' "$ROOT_DIR/Makefile"
rg -Fq 'release/FIELD_ENCRYPTION_CAPABILITIES' "$ROOT_DIR/Makefile"
rg -Fq 'scripts/production-encryption-rewrap.sh' "$ROOT_DIR/Makefile"
rg -Fq 'scripts/lib/encryption-capabilities.sh' "$ROOT_DIR/Makefile"
rg -q 'PGSSLMODE="\$BACKEND_DATABASE_SSLMODE"' "$ROOT_DIR/scripts/production-readiness.sh"
rg -Fq 'READINESS_API_URL="${BACKEND_URL:-' "$ROOT_DIR/scripts/production-readiness.sh"
for field in source_error_game_count error_issue_count stale_pending_issue_count unrecoverable_bet_count abnormal_bet_count; do
  rg -q "read_lottery_health_count $field" "$ROOT_DIR/scripts/production-readiness.sh"
done
! rg -q 'source_error_game_count // 0' "$ROOT_DIR/scripts/production-readiness.sh"
validation_line="$(rg -n '^source_errors=.*read_lottery_health_count' "$ROOT_DIR/scripts/production-readiness.sh" | cut -d: -f1)"
arithmetic_line="$(rg -n '10#\$source_errors == 0' "$ROOT_DIR/scripts/production-readiness.sh" | cut -d: -f1)"
[[ -n "$validation_line" && -n "$arithmetic_line" && "$validation_line" -lt "$arithmetic_line" ]] || {
  echo "外部计数没有在 Bash 算术前完成校验" >&2
  exit 1
}
rg -q 'validate_encrypted_backup_and_manifest.*recent' "$ROOT_DIR/scripts/production-readiness.sh"
rg -q '\.dump\.age' "$ROOT_DIR/scripts/production-readiness.sh"
rg -q '\.offsite-ok' "$ROOT_DIR/scripts/production-readiness.sh"
rg -q 'flock -w "\$LOCK_WAIT_SECONDS"' "$ROOT_DIR/scripts/postgres-backup.sh"
rg -q '远程 PostgreSQL 备份必须校验证书' "$ROOT_DIR/scripts/postgres-backup.sh"
rg -q 'release-integrity\.sh.*verify.*candidate' "$ROOT_DIR/scripts/production-rollback.sh"
rg -q 'CONFIRM_SCHEMA_COMPATIBLE' "$ROOT_DIR/scripts/production-rollback.sh"
rg -q 'MAINTENANCE_FLAG=/etc/wangzhe/maintenance' "$ROOT_DIR/scripts/production-rollback.sh"
rg -Fq 'bin/wangzhe-field-encryption-audit' "$rollback_script"
rg -Fq 'FIELD_ENCRYPTION_CAPABILITIES' "$rollback_script"
rg -Fq 'load_release_encryption_capabilities "$current_target"' "$rollback_script"
rg -Fq 'load_release_encryption_capabilities "$previous_target"' "$rollback_script"
rg -Fq 'encryption_version_supported "$previous_read_versions" "$current_write_version"' "$rollback_script"
rg -Fq 'encryption_version_supported "$current_read_versions" "$previous_write_version"' "$rollback_script"
rg -q 'verify_maintenance_edge "\$PUBLIC_URL" "\$PUBLIC_WWW_URL" "\$ADMIN_URL"' "$ROOT_DIR/scripts/production-rollback.sh"
rg -Fq "trap 'exit 130' INT" "$ROOT_DIR/scripts/production-rollback.sh"
rg -Fq "trap 'exit 143' TERM" "$ROOT_DIR/scripts/production-rollback.sh"
if rg -n '^trap .*EXIT INT TERM' "$ROOT_DIR/scripts/production-rollback.sh" >/dev/null; then
  echo "回滚脚本的资源清理 trap 会吞掉终止信号" >&2
  exit 1
fi
rg -q 'capture_maintenance_marker_state "\$MAINTENANCE_FLAG"' "$ROOT_DIR/scripts/production-rollback.sh"
rg -q 'ensure_maintenance_marker "\$MAINTENANCE_FLAG"' "$ROOT_DIR/scripts/production-rollback.sh"
! rg -q 'finish_maintenance_marker "\$MAINTENANCE_FLAG"' "$ROOT_DIR/scripts/production-rollback.sh"
rg -q 'production-readiness\.sh.*人工删除' "$ROOT_DIR/scripts/production-rollback.sh"
! rg -q 'rm -f -- "\$MAINTENANCE_FLAG"|install .*MAINTENANCE_FLAG' "$ROOT_DIR/scripts/production-rollback.sh"
rollback_capture_line="$(rg -n 'capture_maintenance_marker_state "\$MAINTENANCE_FLAG"' "$rollback_script" | head -n1 | cut -d: -f1)"
rollback_maintenance_line="$(rg -n 'ensure_maintenance_marker "\$MAINTENANCE_FLAG"' "$rollback_script" | head -n1 | cut -d: -f1)"
rollback_stop_line="$(rg -n '^if ! systemctl stop wangzhe-backend\.service' "$rollback_script" | cut -d: -f1)"
rollback_audit_line="$(rg -n '^if ! audit_encryption_for_rollback; then' "$rollback_script" | cut -d: -f1)"
rollback_switch_line="$(rg -n 'mv -Tf.*CURRENT_LINK' "$rollback_script" | head -n1 | cut -d: -f1)"
[[ -n "$rollback_capture_line" && -n "$rollback_maintenance_line" && -n "$rollback_stop_line" && -n "$rollback_audit_line" && -n "$rollback_switch_line" && \
   "$rollback_capture_line" -lt "$rollback_maintenance_line" && "$rollback_maintenance_line" -lt "$rollback_stop_line" && \
   "$rollback_stop_line" -lt "$rollback_audit_line" && "$rollback_audit_line" -lt "$rollback_switch_line" ]] || {
  echo "回滚脚本没有在停写和加密兼容门禁后再切换版本" >&2
  exit 1
}

[[ -x "$rewrap_script" ]] || { echo "受控敏感字段重加密脚本不可执行" >&2; exit 1; }
for rewrap_contract in \
  'bin/wangzhe-field-encryption-audit' \
  'load_release_encryption_capabilities "$current_target"' \
  'verify_maintenance_edge "$PUBLIC_URL" "$PUBLIC_WWW_URL" "$ADMIN_URL"' \
  'systemctl stop wangzhe-backend.service' \
  'LoadCredential=freeze-proof:' \
  '--execute-rewrap' \
  'restart_backend_on_exit=0'; do
  rg -Fq -- "$rewrap_contract" "$rewrap_script" || { echo "受控重加密脚本缺少门禁：$rewrap_contract" >&2; exit 1; }
done
! rg -q 'finish_maintenance_marker|rm -f -- "\$MAINTENANCE_FLAG"' "$rewrap_script"
rewrap_edge_before_line="$(rg -n '^verify_maintenance_edge "\$PUBLIC_URL" "\$PUBLIC_WWW_URL" "\$ADMIN_URL"' "$rewrap_script" | head -n1 | cut -d: -f1)"
rewrap_stop_line="$(rg -n '^systemctl stop wangzhe-backend\.service' "$rewrap_script" | cut -d: -f1)"
rewrap_freeze_before_line="$(rg -n '^if ! assert_backend_writes_frozen; then' "$rewrap_script" | cut -d: -f1)"
rewrap_execute_line="$(rg -n '^if ! run_rewrap_tool "\$mode"; then' "$rewrap_script" | cut -d: -f1)"
rewrap_freeze_after_line="$(rg -n '^if ! assert_backend_writes_frozen \|\|' "$rewrap_script" | cut -d: -f1)"
rewrap_keep_stopped_line="$(rg -n '^restart_backend_on_exit=0$' "$rewrap_script" | tail -n1 | cut -d: -f1)"
[[ -n "$rewrap_edge_before_line" && -n "$rewrap_stop_line" && -n "$rewrap_freeze_before_line" && \
   -n "$rewrap_execute_line" && -n "$rewrap_freeze_after_line" && -n "$rewrap_keep_stopped_line" && \
   "$rewrap_edge_before_line" -lt "$rewrap_stop_line" && "$rewrap_stop_line" -lt "$rewrap_freeze_before_line" && \
   "$rewrap_freeze_before_line" -lt "$rewrap_execute_line" && "$rewrap_execute_line" -lt "$rewrap_freeze_after_line" && \
   "$rewrap_freeze_after_line" -lt "$rewrap_keep_stopped_line" ]] || {
  echo "受控重加密没有在最终盘点前后持续证明停写" >&2
  exit 1
}

# Exercise the exact freeze predicate: a stale/active worker after the final
# inventory must make the wrapper fail instead of granting key removal.
rewrap_freeze_helper="$fixture_dir/rewrap-freeze-helper.sh"
sed -n '/^assert_backend_writes_frozen() {$/,/^}$/p' "$rewrap_script" >"$rewrap_freeze_helper"
# shellcheck disable=SC1090
source "$rewrap_freeze_helper"
systemctl() {
  case "$*" in
    *--property=ActiveState*) printf '%s\n' "${test_backend_active_state:?}" ;;
    *--property=MainPID*) printf '%s\n' "${test_backend_main_pid:?}" ;;
    *) return 1 ;;
  esac
}
test_backend_active_state=inactive
test_backend_main_pid=0
assert_backend_writes_frozen
test_backend_active_state=active
test_backend_main_pid=4242
if assert_backend_writes_frozen; then
  echo "最终盘点后的活动旧 worker 被错误视为已停写" >&2
  exit 1
fi
test_backend_active_state=inactive
test_backend_main_pid=4242
if assert_backend_writes_frozen; then
  echo "最终盘点后的残留 worker PID 被错误视为已停写" >&2
  exit 1
fi
unset -f systemctl assert_backend_writes_frozen

rewrap_cleanup_helper="$fixture_dir/rewrap-unit-cleanup-helper.sh"
sed -n '/^cleanup_rewrap_unit() {$/,/^}$/p' "$rewrap_script" >"$rewrap_cleanup_helper"
# shellcheck disable=SC1090
source "$rewrap_cleanup_helper"
rewrap_cleanup_log="$fixture_dir/rewrap-unit-cleanup.log"
sleep() { :; }
systemctl() {
  printf '%s\n' "$*" >>"$rewrap_cleanup_log"
  if [[ "${1:-}" == show ]]; then
    if [[ "${test_rewrap_unit_state:?}" == stuck ]]; then
      printf '%s\n' 'LoadState=loaded' 'ActiveState=active'
    else
      printf '%s\n' 'LoadState=not-found' 'ActiveState=inactive'
    fi
  fi
  return 0
}
# Consumed by the exact production cleanup function sourced above.
# shellcheck disable=SC2034
rewrap_unit=wangzhe-field-encryption-rewrap-4242
rewrap_unit_cleanup_armed=1
test_rewrap_unit_state=inactive
: >"$rewrap_cleanup_log"
cleanup_rewrap_unit
[[ "$rewrap_unit_cleanup_armed" == 0 ]]
grep -Fxq 'stop wangzhe-field-encryption-rewrap-4242.service' "$rewrap_cleanup_log"
rewrap_unit_cleanup_armed=1
test_rewrap_unit_state=stuck
: >"$rewrap_cleanup_log"
if cleanup_rewrap_unit >/dev/null 2>&1; then
  echo "未停止的重加密 transient unit 被错误确认清理完成" >&2
  exit 1
fi
[[ "$rewrap_unit_cleanup_armed" == 1 ]]
grep -Fxq 'kill --kill-whom=all --signal=KILL wangzhe-field-encryption-rewrap-4242.service' "$rewrap_cleanup_log"
if grep -Fv 'wangzhe-field-encryption-rewrap-4242.service' "$rewrap_cleanup_log" >/dev/null; then
  echo "重加密清理触及了本次之外的 unit" >&2
  exit 1
fi
unset -f sleep systemctl cleanup_rewrap_unit

rewrap_arm_line="$(rg -n '^  rewrap_unit_cleanup_armed=1$' "$rewrap_script" | cut -d: -f1)"
rewrap_systemd_line="$(rg -n '^  if systemd-run ' "$rewrap_script" | cut -d: -f1)"
rewrap_cleanup_call_line="$(rg -n '^  cleanup_rewrap_unit \|\| cleanup_failed=1$' "$rewrap_script" | cut -d: -f1)"
rewrap_restart_guard_line="$(rg -n '^  if \(\( cleanup_failed == 0 && restart_backend_on_exit == 1 \)\); then$' "$rewrap_script" | cut -d: -f1)"
[[ -n "$rewrap_arm_line" && -n "$rewrap_systemd_line" && "$rewrap_arm_line" -lt "$rewrap_systemd_line" && \
   -n "$rewrap_cleanup_call_line" && -n "$rewrap_restart_guard_line" && "$rewrap_cleanup_call_line" -lt "$rewrap_restart_guard_line" ]] || {
  echo "重加密 transient unit 生命周期没有阻止清理失败后的后端重启" >&2
  exit 1
}
rg -Fq 'WANGZHE_ENCRYPTION_REWRAP_UNIT_CLEANUP_FAILED' "$rewrap_script"
rg -Fq 'WANGZHE_ENCRYPTION_REWRAP_CLEANUP_FAILED' "$rewrap_script"
maintenance_helper="$ROOT_DIR/scripts/lib/maintenance-edge.sh"
rg -Fq 'mktemp "$marker_dir/.${marker_name}.tmp.XXXXXX"' "$maintenance_helper"
rg -Fq 'mv -n -- "$temporary" "$marker"' "$maintenance_helper"
rg -Fq 'maintenance_marker_owned_by "$marker" "$maintenance_marker_token"' "$maintenance_helper"
! rg -q '^rollback_code()' "$deploy_script"

backup_test_home="$fixture_dir/backup-home"
mkdir -p "$backup_test_home"
# macOS exposes /var as a symlink to /private/var.  Canonicalize only this
# fixture so the test exercises an exact private directory without weakening
# the production guard that rejects symlinked backup paths.
backup_test_home="$(cd "$backup_test_home" && pwd -P)"
set +e
validate_backup_directory /var/backups >/dev/null 2>&1
wide_backup_status=$?
set -e
if (( wide_backup_status == 0 )); then
  echo "过宽的备份目录被错误接受" >&2
  exit 1
fi
validate_backup_directory "$backup_test_home"
backup_guard_line="$(rg -n '^validate_backup_directory "\$BACKUP_DIR"' "$ROOT_DIR/scripts/postgres-backup.sh" | cut -d: -f1)"
work_guard_line="$(rg -n '^validate_encrypted_work_directory ' "$ROOT_DIR/scripts/postgres-backup.sh" | cut -d: -f1)"
pg_dump_line="$(rg -n '^pg_dump \\' "$ROOT_DIR/scripts/postgres-backup.sh" | cut -d: -f1)"
[[ -n "$backup_guard_line" && -n "$work_guard_line" && -n "$pg_dump_line" && "$backup_guard_line" -lt "$pg_dump_line" && "$work_guard_line" -lt "$pg_dump_line" ]] || {
  echo "备份路径和 LUKS 工作目录没有在 pg_dump 前完成验证" >&2
  exit 1
}

# The explicit local bootstrap must provision the exact identities audited by
# the database-only read-only audit; ordinary debug startup remains non-seeding.
rg -Fq 'DefaultAdminPassword = "Admin8801!"' "$ROOT_DIR/backend/constants/system.go"
for fixture in \
  'demoAgentUsername  = "suyang"' \
  'demoAgentPassword  = "Room8801"' \
  'demoTenantUsername = "wangzhetenant"' \
  'demoTenantPassword = "WzTenant8801"'; do
  rg -Fq "$fixture" "$ROOT_DIR/backend/services/demo_member.go"
done
rg -q 'SeedExperienceAccounts[[:space:]]+bool' "$ROOT_DIR/backend/services/bootstrap.go"
rg -q 'SeedExperienceAccounts:[[:space:]]+cfg\.Server\.SeedExperienceAccounts' "$ROOT_DIR/backend/main.go"
if rg -Fq 'BACKEND_SEED_EXPERIENCE_ACCOUNTS' "$ROOT_DIR/scripts/local-dev.sh"; then
  echo "普通 debug 启动错误地默认启用体验账号夹具" >&2
  exit 1
fi
rg -Fq 'source "$ROOT_DIR/scripts/lib/backend-env.sh"' "$ROOT_DIR/scripts/local-dev.sh"
rg -Fq 'apply_local_backend_defaults' "$ROOT_DIR/scripts/local-dev.sh"
rg -Fq 'export BACKEND_SERVER_BIND="${BACKEND_SERVER_BIND:-0.0.0.0}"' "$ROOT_DIR/scripts/lib/backend-env.sh"
rg -Fq 'export BACKEND_DATABASE_DBNAME="${BACKEND_DATABASE_DBNAME:-wangzhe}"' "$ROOT_DIR/scripts/lib/backend-env.sh"
rg -Fq 'go run ./cmd/dev-bootstrap --confirm-local-development' "$ROOT_DIR/scripts/local-init.sh"
rg -Fq 'go run ./cmd/dev-bootstrap --confirm-local-development --audit-only' "$ROOT_DIR/scripts/local-smoke.sh"
rg -Fq '.agent_room_code == "88001"' "$ROOT_DIR/scripts/local-smoke.sh"
rg -Fq '.agent_room_robot_quota == 10' "$ROOT_DIR/scripts/local-smoke.sh"
rg -Fq '.agent_room_robots == 10' "$ROOT_DIR/scripts/local-smoke.sh"
! rg -Fq '/login' "$ROOT_DIR/scripts/local-smoke.sh"
! rg -Fq 'curl ' "$ROOT_DIR/scripts/local-smoke.sh"
rg -Fq 'if grep -Fqx -- "$BACKEND_DATABASE_DBNAME"' "$ROOT_DIR/scripts/local-init.sh"
rg -Fq 'createdb_cmd' "$ROOT_DIR/scripts/local-init.sh"
rg -Fq -- '--template template0' "$ROOT_DIR/scripts/local-init.sh"
rg -Fq 'require_local_postgres_server "$psql_cmd"' "$ROOT_DIR/scripts/local-init.sh"
rg -Fq 'acquire_local_init_lock' "$ROOT_DIR/scripts/local-init.sh"
rg -Fq 'pg_try_advisory_lock(hashtextextended' "$ROOT_DIR/scripts/local-init.sh"
rg -Fq 'require_local_init_lock' "$ROOT_DIR/scripts/local-init.sh"
rg -Fq 'ln "$temporary_receipt_path" "$receipt_path"' "$ROOT_DIR/scripts/local-init.sh"
! rg -Fq 'mv -f "$temporary_receipt_path" "$receipt_path"' "$ROOT_DIR/scripts/local-init.sh"
rg -Fq 'version=2\nphase=bound\nsystem_identifier=%s\ndatabase_name=%s\ndatabase_oid=%s\nnonce=%s\n' "$ROOT_DIR/scripts/local-init.sh"
rg -Fq '[[ "$pending_database_oid" == "$database_oid" ]]' "$ROOT_DIR/scripts/local-init.sh"
rg -Fq 'dependency_env=(env -u PGPASSWORD -u ENV_FILE)' "$ROOT_DIR/scripts/local-init.sh"
rg -Fq 'dependency_env+=(-u "$backend_environment_name")' "$ROOT_DIR/scripts/local-init.sh"
rg -Fq '"${dependency_env[@]}" go mod download' "$ROOT_DIR/scripts/local-init.sh"
rg -Fq '"${dependency_env[@]}" npm ci --ignore-scripts' "$ROOT_DIR/scripts/local-init.sh"
[[ "$(rg -c '^[[:space:]]+exec 9>&-$' "$ROOT_DIR/scripts/local-init.sh")" -ge 4 ]] || {
  echo "local-init 子进程没有全部关闭 advisory lock FIFO 描述符" >&2
  exit 1
}
rg -Fq 'development_initializing_marker="${development_marker_namespace}:initializing:${LOCAL_POSTGRES_SYSTEM_IDENTIFIER}:${pending_nonce}"' "$ROOT_DIR/scripts/local-init.sh"
rg -Fq 'BACKEND_LOCAL_INIT_NONCE="$pending_nonce"' "$ROOT_DIR/scripts/local-init.sh"
local_init_lock_line="$(rg -n '^acquire_local_init_lock$' "$ROOT_DIR/scripts/local-init.sh" | cut -d: -f1)"
local_init_database_recheck_line="$(rg -n '^database_names=' "$ROOT_DIR/scripts/local-init.sh" | cut -d: -f1)"
local_init_marker_recheck_line="$(rg -n '^database_marker=' "$ROOT_DIR/scripts/local-init.sh" | cut -d: -f1)"
[[ -n "$local_init_lock_line" && -n "$local_init_database_recheck_line" && -n "$local_init_marker_recheck_line" &&
  "$local_init_lock_line" -lt "$local_init_database_recheck_line" &&
  "$local_init_database_recheck_line" -lt "$local_init_marker_recheck_line" ]] || {
  echo "local-init 没有在 (cluster, database) 互斥锁内重查数据库和初始化凭证" >&2
  exit 1
}
createdb_execution_line="$(rg -n '^  if ! "\$createdb_cmd"' "$ROOT_DIR/scripts/local-init.sh" | cut -d: -f1)"
receipt_bind_line="$(rg -n '^  create_pending_receipt "\$database_oid"$' "$ROOT_DIR/scripts/local-init.sh" | cut -d: -f1)"
initial_comment_line="$(rg -n -- '--command "COMMENT ON DATABASE' "$ROOT_DIR/scripts/local-init.sh" | head -n1 | cut -d: -f1)"
[[ -n "$createdb_execution_line" && -n "$receipt_bind_line" && -n "$initial_comment_line" &&
  "$createdb_execution_line" -lt "$receipt_bind_line" && "$receipt_bind_line" -lt "$initial_comment_line" ]] || {
  echo "local-init 必须在 createdb 明确成功后按 OID 绑定收据并立即建立 initializing 凭证" >&2
  exit 1
}
rg -Fq 'lsof -nP -a "-iTCP@${lsof_server_address}:${endpoint_server_port}" -sTCP:LISTEN -t' "$ROOT_DIR/scripts/lib/backend-env.sh"
rg -Fq 'socket_start_time' "$ROOT_DIR/scripts/lib/backend-env.sh"
database_gate_line="$(rg -n 'if err := EnsureDatabaseInitializationComplete\(db\)' "$ROOT_DIR/backend/config/database.go" | head -n1 | cut -d: -f1)"
database_migration_line="$(rg -n 'if err := migrations.Run\(db\)' "$ROOT_DIR/backend/config/database.go" | head -n1 | cut -d: -f1)"
[[ -n "$database_gate_line" && -n "$database_migration_line" && "$database_gate_line" -lt "$database_migration_line" ]] || {
  echo "数据库 initializing 门禁必须在普通启动迁移前执行" >&2
  exit 1
}
! rg -i -e '\b(dropdb|truncate|drop[[:space:]]+database)\b' "$ROOT_DIR/scripts/local-init.sh"
rg -Fq 'require_local_postgres_server "$psql_cmd"' "$ROOT_DIR/scripts/local-dev.sh"
rg -Fq 'require_completed_local_database "$psql_cmd"' "$ROOT_DIR/scripts/local-dev.sh"
rg -Fq 'require_loopback_http_origin BACKEND_URL "$BACKEND_URL" "$BACKEND_SERVER_PORT"' "$ROOT_DIR/scripts/local-dev.sh"
rg -Fq 'require_loopback_http_origin MEMBER_URL "$MEMBER_URL" 5173' "$ROOT_DIR/scripts/local-dev.sh"
rg -Fq 'require_loopback_http_origin ADMIN_URL "$ADMIN_URL" 5174' "$ROOT_DIR/scripts/local-dev.sh"
rg -Fq 'unset VITE_API_BASE_URL' "$ROOT_DIR/scripts/local-dev.sh"
rg -Fq 'export VITE_API_PORT="$BACKEND_SERVER_PORT"' "$ROOT_DIR/scripts/local-dev.sh"
rg -Fq 'done < <(compgen -e)' "$ROOT_DIR/scripts/local-dev.sh"
rg -Fq 'frontend_env+=(-u "$backend_environment_name")' "$ROOT_DIR/scripts/local-dev.sh"
rg -Fq '拒绝把它误认为本次' "$ROOT_DIR/scripts/local-dev.sh"
rg -Fq 'TestDevelopmentAcceptanceOuterTransactionPostgresRollsBackEveryStep' "$ROOT_DIR/Makefile"
rg -Fq 'TestUserAdminCreateMemberPostgresActivatesAgentRoomAtomically' "$ROOT_DIR/Makefile"
rg -Fq 'PG_DB="${BACKEND_DATABASE_DBNAME:-wangzhe}"' "$ROOT_DIR/scripts/local-health.sh"
rg -q '^  dbname: wangzhe$' "$ROOT_DIR/backend/config/config.example.yaml"
rg -q '^BACKEND_DATABASE_DBNAME=wangzhe$' "$ROOT_DIR/deploy/env/backend.env.example"

echo "发布配置静态检查通过"

# Test-site credentials are runtime-only, isolated between the two hostnames,
# off by default and incompatible with formal production acceptance.
test_login_nginx="$ROOT_DIR/deploy/nginx/wz6688.split-hosts.conf"
[[ "$(rg -c 'location = /test-login.json' "$test_login_nginx")" == 2 ]]
[[ "$(rg -c 'if \(!-f /etc/wangzhe/test-login.enabled\)' "$test_login_nginx")" == 2 ]]
rg -Fq 'alias /etc/wangzhe/test-login/member.json;' "$test_login_nginx"
rg -Fq 'alias /etc/wangzhe/test-login/admin.json;' "$test_login_nginx"
rg -Fq 'Cache-Control "no-store, max-age=0"' "$test_login_nginx"
rg -Fq 'test-site-accounts:v1:%' "$ROOT_DIR/scripts/production-readiness.sh"
rg -Fq '10#$active_test_accounts == 0' "$ROOT_DIR/scripts/production-readiness.sh"
rg -Fq '! -e /etc/wangzhe/test-login.enabled' "$ROOT_DIR/scripts/production-readiness.sh"
