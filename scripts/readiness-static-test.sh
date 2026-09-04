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

fixture_dir="$(mktemp -d)"
cleanup_fixture() { rm -rf -- "$fixture_dir"; }
trap cleanup_fixture EXIT INT TERM

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
  verify_maintenance_edge https://wz6688.app https://admin.wz6688.app
)
if (
  curl() {
    printf '%s\n' 'HTTP/2 200' 'Strict-Transport-Security: max-age=31536000' 'Content-Security-Policy: default-src '\''self'\''' 'X-Content-Type-Options: nosniff'
  }
  verify_maintenance_edge https://wz6688.app https://admin.wz6688.app >/dev/null 2>&1
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
  verify_maintenance_edge https://wz6688.app https://admin.wz6688.app >/dev/null 2>&1
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
  verify_maintenance_edge https://wz6688.app https://admin.wz6688.app >/dev/null 2>&1
); then
  echo "本机 Nginx 未维护但公网已维护时被错误放行" >&2
  exit 1
fi
if (
  curl() {
    if [[ "$*" == *admin.wz6688.app* ]]; then
      printf '%s\n' 'HTTP/2 503' 'Strict-Transport-Security: max-age=31536000' 'Content-Security-Policy: default-src '\''self'\''' 'X-Content-Type-Options: nosniff'
    else
      printf '%s\n' 'HTTP/2 200' 'Strict-Transport-Security: max-age=31536000' 'Content-Security-Policy: default-src '\''self'\''' 'X-Content-Type-Options: nosniff'
    fi
  }
  verify_maintenance_edge https://wz6688.app https://admin.wz6688.app >/dev/null 2>&1
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
  verify_maintenance_edge https://wz6688.app https://admin.wz6688.app >/dev/null 2>&1
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
  verify_maintenance_edge https://wz6688.app https://admin.wz6688.app
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
BACKEND_SERVER_ALLOWED_ORIGINS=https://wz6688.app,https://admin.wz6688.app
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
BACKEND_UPLOAD_DIR=/var/lib/wangzhe/uploads
BACKEND_AUDIT_FALLBACK_FILE=/var/lib/wangzhe/audit-fallback.jsonl
BACKEND_ROOM_ACTIVITY=0
BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES=0
EOF
chmod 600 "$valid_env"
bash "$ROOT_DIR/scripts/production-config-check.sh" "$valid_env" >/dev/null

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
rg -q 'BACKEND_SERVER_BIND=127\.0\.0\.1' "$ROOT_DIR/deploy/env/backend.env.example"

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
rg -q "connect-src 'self' wss://admin\.wz6688\.app" "$nginx_config"
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
split_log_block="$(sed -n '/log_format wz_current_safe/p' "$split_nginx_config")"
! grep -Eq '\$args([^_A-Za-z0-9]|$)|\$request([^_A-Za-z0-9_]|$)' <<<"$split_log_block"
rg -q 'Strict-Transport-Security.*max-age=' "$ROOT_DIR/deploy/nginx/snippets/wangzhe-security-headers.conf"
rg -q 'ssl_protocols TLSv1\.2 TLSv1\.3;' "$ROOT_DIR/deploy/nginx/snippets/wangzhe-tls.conf"
rg -q 'openssl x509 -checkend 1209600' "$ROOT_DIR/scripts/production-readiness.sh"
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
for executable in production-config-check.sh production-deploy.sh production-rollback.sh release-integrity.sh; do
  [[ -x "$ROOT_DIR/scripts/$executable" ]] || { echo "部署脚本不可执行：$executable" >&2; exit 1; }
done
[[ "$(sed -n '3p' "$deploy_script")" == 'export PATH=/usr/sbin:/usr/bin:/sbin:/bin' ]]
[[ "$(sed -n '3p' "$rollback_script")" == 'export PATH=/usr/sbin:/usr/bin:/sbin:/bin' ]]
deploy_command_check="$(rg '^for command_name in ' "$deploy_script" | head -n1)"
rollback_command_check="$(rg '^for command_name in ' "$rollback_script" | head -n1)"
for required_command in date env sleep; do
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
[[ -n "$backup_line" && -n "$switch_line" && "$backup_line" -lt "$switch_line" ]] || {
  echo "发布脚本没有保证先备份再切换版本" >&2
  exit 1
}
maintenance_line="$(rg -n 'ensure_maintenance_marker "\$MAINTENANCE_FLAG"' "$deploy_script" | head -n1 | cut -d: -f1)"
gate_line="$(rg -n 'production-readiness\.sh' "$deploy_script" | tail -n1 | cut -d: -f1)"
remove_maintenance_line="$(rg -n 'finish_maintenance_marker "\$MAINTENANCE_FLAG"' "$deploy_script" | tail -n1 | cut -d: -f1)"
[[ -n "$maintenance_line" && -n "$gate_line" && -n "$remove_maintenance_line" && "$maintenance_line" -lt "$switch_line" && "$switch_line" -lt "$gate_line" && "$gate_line" -lt "$remove_maintenance_line" ]] || {
  echo "发布脚本没有在完整门禁期间保持维护模式" >&2
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
mkdir -p "$contract_candidate/bin" "$contract_candidate/member/assets"
printf '%s\n' "$current_mark_six_rule" >"$contract_candidate/bin/wangzhe-backend"
chmod 700 "$contract_candidate/bin/wangzhe-backend"
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
edge_check_line="$(rg -n 'verify_maintenance_edge "\$PUBLIC_URL" "\$ADMIN_URL"' "$deploy_script" | head -n1 | cut -d: -f1)"
[[ -n "$edge_check_line" && "$edge_check_line" -lt "$switch_line" ]] || {
  echo "发布脚本没有在切换版本前实测公网维护模式" >&2
  exit 1
}

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
rg -q '同一张.*证书.*wz6688\.app.*admin\.wz6688\.app' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
rg -q 'chown -R root:root /tmp/wangzhe-release' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
rg -Fq 'chmod -R a+rX,go-w /tmp/wangzhe-release' "$ROOT_DIR/PRODUCTION_OPERATIONS.md"
rg -Fq 'RELEASE_GOOS ?= linux' "$ROOT_DIR/Makefile"
rg -Fq 'RELEASE_GOARCH ?= amd64' "$ROOT_DIR/Makefile"
rg -q '^release: verify readiness-test$' "$ROOT_DIR/Makefile"
rg -q '^release-contract-check:' "$ROOT_DIR/Makefile"
rg -Fq '$(MAKE) release-contract-check' "$ROOT_DIR/Makefile"
rg -Fq 'GOOS=$(RELEASE_GOOS) GOARCH=$(RELEASE_GOARCH)' "$ROOT_DIR/Makefile"
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
rg -q 'verify_maintenance_edge "\$PUBLIC_URL" "\$ADMIN_URL"' "$ROOT_DIR/scripts/production-rollback.sh"
rg -q 'capture_maintenance_marker_state "\$MAINTENANCE_FLAG"' "$ROOT_DIR/scripts/production-rollback.sh"
rg -q 'ensure_maintenance_marker "\$MAINTENANCE_FLAG"' "$ROOT_DIR/scripts/production-rollback.sh"
! rg -q 'finish_maintenance_marker "\$MAINTENANCE_FLAG"' "$ROOT_DIR/scripts/production-rollback.sh"
rg -q 'production-readiness\.sh.*人工删除' "$ROOT_DIR/scripts/production-rollback.sh"
! rg -q 'rm -f -- "\$MAINTENANCE_FLAG"|install .*MAINTENANCE_FLAG' "$ROOT_DIR/scripts/production-rollback.sh"
rollback_capture_line="$(rg -n 'capture_maintenance_marker_state "\$MAINTENANCE_FLAG"' "$rollback_script" | head -n1 | cut -d: -f1)"
rollback_maintenance_line="$(rg -n 'ensure_maintenance_marker "\$MAINTENANCE_FLAG"' "$rollback_script" | head -n1 | cut -d: -f1)"
rollback_switch_line="$(rg -n 'mv -Tf.*CURRENT_LINK' "$rollback_script" | head -n1 | cut -d: -f1)"
[[ -n "$rollback_capture_line" && -n "$rollback_maintenance_line" && -n "$rollback_switch_line" && "$rollback_capture_line" -lt "$rollback_maintenance_line" && "$rollback_maintenance_line" -lt "$rollback_switch_line" ]] || {
  echo "回滚脚本没有在操作前记录状态并在切换版本前进入维护模式" >&2
  exit 1
}
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
