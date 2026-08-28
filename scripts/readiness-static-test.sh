#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=lib/backend-env.sh
source "$ROOT_DIR/scripts/lib/backend-env.sh"

fixture_dir="$(mktemp -d)"
cleanup_fixture() { rm -rf -- "$fixture_dir"; }
trap cleanup_fixture EXIT INT TERM

env_file="$fixture_dir/backend.env"
marker="$fixture_dir/must-not-exist"
printf '%s\n' \
  'BACKEND_DATABASE_HOST=127.0.0.1' \
  'BACKEND_DATABASE_PASSWORD=$(touch should-not-run)' \
  >"$env_file"
chmod 600 "$env_file"
(
  cd "$fixture_dir"
  load_backend_env "$env_file"
  [[ "$BACKEND_DATABASE_PASSWORD" == '$(touch should-not-run)' ]]
)
[[ ! -e "$fixture_dir/should-not-run" && ! -e "$marker" ]]

chmod 644 "$env_file"
if (load_backend_env "$env_file" >/dev/null 2>&1); then
  echo "宽松权限的环境文件被错误接受" >&2
  exit 1
fi

! rg -n '127\.0\.0\.1:8089|BACKEND_SERVER_ALLOWED_ORIGINS=.*http://' "$ROOT_DIR/deploy"
rg -q 'migrations\.VerifyApplied' "$ROOT_DIR/backend/api/health.go"
rg -q 'BACKEND_SERVER_BIND=127\.0\.0\.1' "$ROOT_DIR/deploy/env/backend.env.example"

# Fresh debug databases must provision the exact identities exercised by the
# smoke test; acceptance may not depend on accounts left in a developer DB.
for fixture in \
  'demoAgentUsername  = "suyang"' \
  'demoAgentPassword  = "Room8801"' \
  'demoTenantUsername = "wangzhetenant"' \
  'demoTenantPassword = "WzTenant8801"'; do
  rg -Fq "$fixture" "$ROOT_DIR/backend/services/demo_member.go"
done
rg -Fq '"username":"suyang","password":"Room8801"' "$ROOT_DIR/scripts/local-smoke.sh"
rg -Fq '"username":"wangzhetenant","password":"WzTenant8801"' "$ROOT_DIR/scripts/local-smoke.sh"
rg -Fq 'export BACKEND_SERVER_BIND="${BACKEND_SERVER_BIND:-0.0.0.0}"' "$ROOT_DIR/scripts/local-dev.sh"
rg -Fq 'export BACKEND_DATABASE_DBNAME="${BACKEND_DATABASE_DBNAME:-backend}"' "$ROOT_DIR/scripts/local-dev.sh"

echo "发布配置静态检查通过"
