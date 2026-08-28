#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
reset_script="$script_dir/dev-reset-business-data.sh"
full_reset_script="$script_dir/dev-reset-database.sh"
sentinel_script="$script_dir/dev-reset-init-sentinel.sh"
verify_script="$script_dir/dev-reset-verify-bootstrap.sh"
complete_receipt_script="$script_dir/dev-reset-complete-receipt.sh"
migration="$script_dir/../backend/migrations/202608270010_dev_reset_guard.sql"
marker_migration="$script_dir/../backend/migrations/202608270011_reset_guard_workspace_marker.sql"
identity_migration="$script_dir/../backend/migrations/202608270012_reset_identity_receipts.sql"

bash -n "$reset_script" "$full_reset_script" "$sentinel_script" "$verify_script" "$complete_receipt_script" "$script_dir/lib/dev-reset-safety.sh"
test_root="$(mktemp -d)"
trap 'rm -rf -- "$test_root"' EXIT INT TERM

write_env() {
  local target="$1" mode="$2" host="$3"
  printf '%s\n' \
    "BACKEND_SERVER_MODE=$mode" \
    "BACKEND_SERVER_PORT=8080" \
    "BACKEND_DATABASE_HOST=$host" \
    "BACKEND_DATABASE_PORT=5432" \
    "BACKEND_DATABASE_USER=developer" \
    "BACKEND_DATABASE_PASSWORD=not-used-by-dry-run" \
    "BACKEND_DATABASE_DBNAME=wangzhe_dev" \
    "BACKEND_DATABASE_SSLMODE=disable" \
    >"$target"
  chmod 600 "$target"
}

write_env "$test_root/debug.env" debug 127.0.0.1
dry_run_output="$(bash "$reset_script" --dry-run "$test_root/debug.env")"
grep -Fq '仅预览：没有连接数据库、没有备份、没有修改任何数据。' <<<"$dry_run_output"
grep -Fq 'schema_migrations' <<<"$dry_run_output"
grep -Fq 'development_reset_receipts' <<<"$dry_run_output"
grep -Fq 'workspace_migration_markers' <<<"$dry_run_output"

full_dry_run_output="$(bash "$full_reset_script" --dry-run "$test_root/debug.env")"
grep -Fq '仅预览：没有连接数据库、没有备份、没有修改任何 schema 或数据。' <<<"$full_dry_run_output"
sentinel_dry_run_output="$(bash "$sentinel_script" --dry-run "$test_root/debug.env")"
grep -Fq '仅预览：没有连接数据库、没有写入 sentinel。' <<<"$sentinel_dry_run_output"

current_env_output="$(
  BACKEND_SERVER_MODE=debug \
  BACKEND_SERVER_PORT=8080 \
  BACKEND_DATABASE_HOST=localhost \
  BACKEND_DATABASE_PORT=5432 \
  BACKEND_DATABASE_USER=developer \
  BACKEND_DATABASE_PASSWORD=not-used-by-dry-run \
  BACKEND_DATABASE_DBNAME=wangzhe_dev \
  BACKEND_DATABASE_SSLMODE=disable \
  bash "$reset_script" --dry-run
)"
grep -Fq '仅预览：没有连接数据库、没有备份、没有修改任何数据。' <<<"$current_env_output"
current_env_full_output="$(
  BACKEND_SERVER_MODE=debug \
  BACKEND_SERVER_PORT=8080 \
  BACKEND_DATABASE_HOST=::1 \
  BACKEND_DATABASE_PORT=5432 \
  BACKEND_DATABASE_USER=developer \
  BACKEND_DATABASE_PASSWORD=not-used-by-dry-run \
  BACKEND_DATABASE_DBNAME=wangzhe_dev \
  BACKEND_DATABASE_SSLMODE=disable \
  bash "$full_reset_script" --dry-run
)"
grep -Fq '仅预览：没有连接数据库、没有备份、没有修改任何 schema 或数据。' <<<"$current_env_full_output"
grep -Fq -- '--current-env' "$script_dir/postgres-backup.sh"

# File mode cannot inherit a dangerous authorization that the file omitted.
if BACKEND_ALLOW_DEVELOPMENT_RESET=YES \
  BACKEND_DEVELOPMENT_RESET_DATABASE=wangzhe_dev \
  bash "$reset_script" --execute \
    --confirm 'RESET:wangzhe_dev:BUSINESS-DATA' \
    --backup-dir "$test_root/backups" "$test_root/debug.env" \
    >"$test_root/inherited-auth.out" 2>&1; then
  echo "环境文件错误地继承了 shell 中的开发重置授权" >&2
  exit 1
fi
grep -Fq '必须显式设置 BACKEND_ALLOW_DEVELOPMENT_RESET=YES' "$test_root/inherited-auth.out"
if BACKEND_ALLOW_DEVELOPMENT_RESET=YES \
  BACKEND_DEVELOPMENT_RESET_DATABASE=wangzhe_dev \
  bash "$full_reset_script" --execute \
    --confirm 'DROP:wangzhe_dev:REBUILD-PUBLIC-SCHEMA' \
    --backup-dir "$test_root/backups" "$test_root/debug.env" \
    >"$test_root/full-inherited-auth.out" 2>&1; then
  echo "环境文件错误地继承了 shell 中的完整重置授权" >&2
  exit 1
fi
grep -Fq '必须显式设置 BACKEND_ALLOW_DEVELOPMENT_RESET=YES' "$test_root/full-inherited-auth.out"

grep -Fq 'SET LOCAL search_path = pg_catalog, public;' "$reset_script"
grep -Fq 'FROM public.schema_migrations' "$reset_script"
grep -Fq 'FROM pg_catalog.pg_stat_activity' "$reset_script"
grep -Fq 'public.workspace_robot_settings' "$reset_script"
grep -Fq 'UPDATE public."user"' "$reset_script"
grep -Fq 'INSERT INTO public.development_reset_receipts' "$reset_script"
grep -Fq 'public.user_balance_transactions' "$reset_script"
grep -Fq 'SET LOCAL search_path = pg_catalog, public;' "$full_reset_script"
grep -Fq 'FROM public.schema_migrations' "$full_reset_script"
grep -Fq 'FROM pg_catalog.pg_stat_activity' "$full_reset_script"
grep -Fq 'DROP SCHEMA public CASCADE' "$full_reset_script"
grep -Fq 'CREATE SCHEMA public AUTHORIZATION CURRENT_USER' "$full_reset_script"
if grep -Eiq 'DROP[[:space:]]+DATABASE' "$full_reset_script"; then
  echo "完整开发重置不得删除数据库本身" >&2
  exit 1
fi

write_env "$test_root/release.env" release 127.0.0.1
if bash "$reset_script" --dry-run "$test_root/release.env" >"$test_root/release.out" 2>&1; then
  echo "release 环境错误地通过了开发重置保护" >&2
  exit 1
fi
grep -Fq '业务数据重置仅允许 debug 环境' "$test_root/release.out"
if bash "$full_reset_script" --dry-run "$test_root/release.env" >"$test_root/full-release.out" 2>&1; then
  echo "release 环境错误地通过了完整开发重置保护" >&2
  exit 1
fi
grep -Fq '完整重建仅允许 debug 环境' "$test_root/full-release.out"
if bash "$sentinel_script" --dry-run "$test_root/release.env" >"$test_root/sentinel-release.out" 2>&1; then
  echo "release 环境错误地通过了 sentinel 初始化保护" >&2
  exit 1
fi
grep -Fq 'sentinel 初始化仅允许 debug 环境' "$test_root/sentinel-release.out"

write_env "$test_root/test.env" test 127.0.0.1
if bash "$reset_script" --dry-run "$test_root/test.env" >"$test_root/partial-test.out" 2>&1; then
  echo "test 环境错误地通过了业务数据重置保护" >&2
  exit 1
fi
grep -Fq '业务数据重置仅允许 debug 环境' "$test_root/partial-test.out"
if bash "$full_reset_script" --dry-run "$test_root/test.env" >"$test_root/full-test.out" 2>&1; then
  echo "test 环境错误地通过了完整开发重置保护" >&2
  exit 1
fi
grep -Fq '完整重建仅允许 debug 环境' "$test_root/full-test.out"

write_env "$test_root/remote.env" debug 192.0.2.10
if bash "$reset_script" --dry-run "$test_root/remote.env" >"$test_root/remote.out" 2>&1; then
  echo "远程数据库错误地通过了开发重置保护" >&2
  exit 1
fi
grep -Fq '只允许连接本机 PostgreSQL' "$test_root/remote.out"
if bash "$full_reset_script" --dry-run "$test_root/remote.env" >"$test_root/full-remote.out" 2>&1; then
  echo "远程数据库错误地通过了完整开发重置保护" >&2
  exit 1
fi
grep -Fq '只允许连接本机 PostgreSQL' "$test_root/full-remote.out"

grep -Fq 'reject_unapproved_application_truncate' "$migration"
grep -Fq 'development reset receipts are immutable' "$migration"
grep -Fq "relation.relname <> 'development_reset_receipts'" "$migration"
grep -Fq 'CREATE TABLE IF NOT EXISTS public.workspace_migration_markers' "$marker_migration"
grep -Fq 'SELECT public.install_application_truncate_guards()' "$marker_migration"
grep -Fq 'server_system_identifier' "$identity_migration"
grep -Fq "version = '202608270012_reset_identity_receipts.sql'" "$reset_script"
grep -Fq "version = '202608270012_reset_identity_receipts.sql'" "$full_reset_script"
grep -Fq 'wangzhe_meta.development_reset_sentinel' "$reset_script"
grep -Fq 'wangzhe_meta.development_reset_sentinel' "$full_reset_script"
grep -Fq 'createdb --host' "$full_reset_script"
grep -Fq 'pg_restore --exit-on-error' "$full_reset_script"
grep -Fq 'dropdb --host' "$full_reset_script"
grep -Fq 'scratch_restore_verified=true' "$full_reset_script"
grep -Fq 'status=complete' "$complete_receipt_script"
grep -Fq '/ready' "$complete_receipt_script"
grep -Fq 'dev-reset-verify-bootstrap.sh' "$complete_receipt_script"
grep -Fq 'BACKEND_DATABASE_SSLMODE' "$script_dir/postgres-backup.sh"
grep -Fq 'export PGSSLMODE' "$script_dir/postgres-backup.sh"
if grep -Eiq 'DROP[[:space:]]+(DATABASE|SCHEMA)|TRUNCATE[[:space:]]+TABLE[[:space:]]+schema_migrations' "$reset_script"; then
  echo "开发重置脚本不得删除数据库、schema 或迁移记录" >&2
  exit 1
fi

echo "开发数据重置静态保护检查通过"
