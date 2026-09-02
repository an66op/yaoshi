#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
用法：
  scripts/dev-reset-database.sh --dry-run [ENV_FILE]
  scripts/dev-reset-database.sh --execute \
    --confirm 'DROP:<数据库名>:REBUILD-PUBLIC-SCHEMA' \
    --backup-dir /绝对/备份目录 [ENV_FILE]

此工具会删除并重新创建本地开发数据库的 public schema。全部业务数据、
账号、配置、旧迁移记录及孤立旧表都会清空；后端下次启动时重新建表、
执行全部版本化迁移并生成本地初始账号。工具不会删除数据库本身。
不传 ENV_FILE 时，必须由当前进程环境显式提供全部 BACKEND_* 变量。
USAGE
}

mode="dry-run"
confirm_value=""
backup_dir=""
env_file=""
while (($#)); do
  case "$1" in
    --dry-run) mode="dry-run"; shift ;;
    --execute) mode="execute"; shift ;;
    --confirm) [[ $# -ge 2 ]] || { usage >&2; exit 2; }; confirm_value="$2"; shift 2 ;;
    --backup-dir) [[ $# -ge 2 ]] || { usage >&2; exit 2; }; backup_dir="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    --*) echo "未知参数：$1" >&2; usage >&2; exit 2 ;;
    *) [[ -z "$env_file" ]] || { echo "只能指定一个环境文件" >&2; exit 2; }; env_file="$1"; shift ;;
  esac
done

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/backend-env.sh
source "$script_dir/lib/backend-env.sh"
# shellcheck source=lib/dev-reset-safety.sh
source "$script_dir/lib/dev-reset-safety.sh"
if [[ -n "$env_file" ]]; then
  # File mode must be self-contained. Do not let an omitted value silently
  # inherit a reset authorization or target from the invoking shell.
  unset BACKEND_SERVER_MODE BACKEND_DATABASE_HOST BACKEND_DATABASE_PORT
  unset BACKEND_DATABASE_USER BACKEND_DATABASE_PASSWORD BACKEND_DATABASE_DBNAME
  unset BACKEND_DATABASE_SSLMODE BACKEND_SERVER_PORT BACKEND_ALLOW_DEVELOPMENT_RESET
  unset BACKEND_DEVELOPMENT_RESET_DATABASE BACKEND_DEVELOPMENT_RESET_SENTINEL_TOKEN
  load_backend_env "$env_file"
fi

: "${BACKEND_SERVER_MODE:?必须明确设置 BACKEND_SERVER_MODE}"
: "${BACKEND_DATABASE_HOST:?缺少 BACKEND_DATABASE_HOST}"
: "${BACKEND_DATABASE_PORT:?缺少 BACKEND_DATABASE_PORT}"
: "${BACKEND_DATABASE_USER:?缺少 BACKEND_DATABASE_USER}"
: "${BACKEND_DATABASE_PASSWORD:?缺少 BACKEND_DATABASE_PASSWORD}"
: "${BACKEND_DATABASE_DBNAME:?缺少 BACKEND_DATABASE_DBNAME}"
: "${BACKEND_DATABASE_SSLMODE:?缺少 BACKEND_DATABASE_SSLMODE}"
: "${BACKEND_SERVER_PORT:?缺少 BACKEND_SERVER_PORT}"

[[ "$BACKEND_SERVER_MODE" == "debug" ]] || {
  echo "完整重建仅允许 debug 环境，当前模式：$BACKEND_SERVER_MODE" >&2
  exit 1
}
case "$BACKEND_DATABASE_HOST" in
  127.0.0.1|localhost|::1) ;;
  *) echo "开发整库重建只允许连接本机 PostgreSQL，当前主机：$BACKEND_DATABASE_HOST" >&2; exit 1 ;;
esac
[[ "$BACKEND_DATABASE_PORT" =~ ^[0-9]+$ ]] && (( BACKEND_DATABASE_PORT >= 1 && BACKEND_DATABASE_PORT <= 65535 )) || {
  echo "数据库端口不正确" >&2
  exit 1
}
[[ "$BACKEND_SERVER_PORT" =~ ^[0-9]+$ ]] && (( BACKEND_SERVER_PORT >= 1 && BACKEND_SERVER_PORT <= 65535 )) || { echo "后端端口不正确" >&2; exit 1; }
[[ "$BACKEND_DATABASE_DBNAME" =~ ^[A-Za-z0-9_.-]+$ ]] || { echo "数据库名格式不安全" >&2; exit 1; }
case "$BACKEND_DATABASE_SSLMODE" in
  disable|allow|prefer|require|verify-ca|verify-full) ;;
  *) echo "BACKEND_DATABASE_SSLMODE 不正确" >&2; exit 1 ;;
esac

echo "目标数据库：$BACKEND_DATABASE_HOST:$BACKEND_DATABASE_PORT/$BACKEND_DATABASE_DBNAME"
echo "动作：完整备份后 DROP SCHEMA public CASCADE，再创建空 public schema。"
echo "结果：账号、工作区、彩票数据、聊天、注单、流水、配置、迁移记录和孤立旧表全部清空。"
echo "数据库本身不会删除；后端必须保持停止，随后由 bootstrap 完整重建。"

if [[ "$mode" == "dry-run" ]]; then
  echo "仅预览：没有连接数据库、没有备份、没有修改任何 schema 或数据。"
  exit 0
fi

[[ "${BACKEND_ALLOW_DEVELOPMENT_RESET:-}" == "YES" ]] || {
  echo "必须显式设置 BACKEND_ALLOW_DEVELOPMENT_RESET=YES" >&2
  exit 1
}
[[ "${BACKEND_DEVELOPMENT_RESET_DATABASE:-}" == "$BACKEND_DATABASE_DBNAME" ]] || {
  echo "BACKEND_DEVELOPMENT_RESET_DATABASE 必须与目标数据库名完全一致" >&2
  exit 1
}
expected_confirmation="DROP:${BACKEND_DATABASE_DBNAME}:REBUILD-PUBLIC-SCHEMA"
[[ "$confirm_value" == "$expected_confirmation" ]] || {
  echo "确认口令不匹配；需要：$expected_confirmation" >&2
  exit 1
}
[[ -n "$backup_dir" && "$backup_dir" == /* ]] || { echo "--backup-dir 必须是明确的绝对路径" >&2; exit 1; }
case "$backup_dir" in
  /|/home|/Users|"$HOME") echo "拒绝使用过宽的备份目录：$backup_dir" >&2; exit 1 ;;
esac
sentinel_token="${BACKEND_DEVELOPMENT_RESET_SENTINEL_TOKEN:-}"
(( ${#sentinel_token} >= 32 && ${#sentinel_token} <= 256 )) || { echo "sentinel token 必须为 32-256 个字符" >&2; exit 1; }
for command_name in psql pg_restore createdb dropdb awk basename date mv sed tail lsof; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done

reset_assert_backend_port_stopped
token_sha256="$(reset_sha256 "$sentinel_token")"
unset sentinel_token BACKEND_DEVELOPMENT_RESET_SENTINEL_TOKEN
identity_before="$(reset_verified_identity "$token_sha256" true)"
IFS=$'\t' read -r server_system_identifier server_address server_port source_user_count source_balance_cents source_ledger_count <<<"$identity_before"

echo "开始创建整库重建前完整备份……"
backup_source="${env_file:---current-env}"
backup_output="$(BACKUP_DIR="$backup_dir" BACKUP_RETENTION_DAYS=3650 "$script_dir/postgres-backup.sh" "$backup_source")"
printf '%s\n' "$backup_output"
backup_file="$(printf '%s\n' "$backup_output" | sed -n 's/^数据库备份完成：//p' | tail -n 1)"
[[ -n "$backup_file" && -f "$backup_file" && -f "$backup_file.sha256" ]] || {
  echo "无法确认完整备份及校验文件，拒绝继续" >&2
  exit 1
}
backup_sha256="$(awk 'NR == 1 { print $1 }' "$backup_file.sha256")"
[[ "$backup_sha256" =~ ^[0-9a-f]{64}$ ]] || { echo "备份 SHA-256 不正确" >&2; exit 1; }
backup_name="$(basename "$backup_file")"
request_id="dev-full-reset-$(date -u +%Y%m%dT%H%M%SZ)-$$-${RANDOM}"
receipt_file="$backup_file.full-reset-receipt"
receipt_partial="$receipt_file.partial"
scratch_database="wz_reset_verify_$$_${RANDOM}"
[[ "$scratch_database" =~ ^wz_reset_verify_[0-9]+_[0-9]+$ ]] || { echo "临时验库名不安全" >&2; exit 1; }

reset_assert_backend_port_stopped
identity_after_backup="$(reset_verified_identity "$token_sha256" true)"
reset_identity_matches "$identity_before" "$identity_after_backup"

export PGPASSWORD="$BACKEND_DATABASE_PASSWORD"
export PGSSLMODE="$BACKEND_DATABASE_SSLMODE"
cleanup_scratch() {
  if [[ -n "${scratch_database:-}" && "$scratch_database" =~ ^wz_reset_verify_[0-9]+_[0-9]+$ ]]; then
    dropdb --if-exists --host "$BACKEND_DATABASE_HOST" --port "$BACKEND_DATABASE_PORT" \
      --username "$BACKEND_DATABASE_USER" --maintenance-db "$BACKEND_DATABASE_DBNAME" \
      "$scratch_database" >/dev/null 2>&1 || true
  fi
}
trap cleanup_scratch EXIT INT TERM
createdb --host "$BACKEND_DATABASE_HOST" --port "$BACKEND_DATABASE_PORT" \
  --username "$BACKEND_DATABASE_USER" --maintenance-db "$BACKEND_DATABASE_DBNAME" \
  --owner "$BACKEND_DATABASE_USER" "$scratch_database"
pg_restore --exit-on-error --no-owner --no-privileges \
  --host "$BACKEND_DATABASE_HOST" --port "$BACKEND_DATABASE_PORT" \
  --username "$BACKEND_DATABASE_USER" --dbname "$scratch_database" "$backup_file"
scratch_snapshot="$(PGAPPNAME=wangzhe-reset-restore-verifier psql \
  --host "$BACKEND_DATABASE_HOST" --port "$BACKEND_DATABASE_PORT" \
  --username "$BACKEND_DATABASE_USER" --dbname "$scratch_database" \
  --no-psqlrc --set ON_ERROR_STOP=1 --quiet --tuples-only --no-align --field-separator=$'\t' <<'SQL'
SET search_path = pg_catalog, public;
DO $$ BEGIN
  IF pg_catalog.to_regclass('public.schema_migrations') IS NULL OR NOT EXISTS (
    SELECT 1 FROM public.schema_migrations WHERE version = '202608270012_reset_identity_receipts.sql'
  ) OR pg_catalog.to_regclass('public."user"') IS NULL
     OR pg_catalog.to_regclass('public.user_balance_transactions') IS NULL THEN
    RAISE EXCEPTION 'restored backup is missing required Wangzhe schema';
  END IF;
END $$;
SELECT (SELECT COUNT(*) FROM public."user"),
       (SELECT COALESCE(SUM(balance_cents), 0) FROM public."user"),
       (SELECT COUNT(*) FROM public.user_balance_transactions);
SQL
)"
IFS=$'\t' read -r restored_user_count restored_balance_cents restored_ledger_count <<<"$scratch_snapshot"
[[ "$restored_user_count" == "$source_user_count" && "$restored_balance_cents" == "$source_balance_cents" && "$restored_ledger_count" == "$source_ledger_count" ]] || {
  echo "临时恢复数据库的账号或账务计数与源库不一致，拒绝重置" >&2
  exit 1
}
dropdb --host "$BACKEND_DATABASE_HOST" --port "$BACKEND_DATABASE_PORT" \
  --username "$BACKEND_DATABASE_USER" --maintenance-db "$BACKEND_DATABASE_DBNAME" "$scratch_database"
scratch_database=""
trap - EXIT INT TERM
reset_assert_backend_port_stopped
identity_after_restore="$(reset_verified_identity "$token_sha256" true)"
reset_identity_matches "$identity_before" "$identity_after_restore"

write_external_receipt() {
  local reset_status="$1"
  umask 077
  {
    printf 'request_id=%s\n' "$request_id"
    printf 'database=%s\n' "$BACKEND_DATABASE_DBNAME"
    printf 'backup=%s\n' "$backup_name"
    printf 'backup_sha256=%s\n' "$backup_sha256"
    printf 'scope=public_schema_rebuild\n'
    printf 'server_system_identifier=%s\n' "$server_system_identifier"
    printf 'server_address=%s\n' "$server_address"
    printf 'server_port=%s\n' "$server_port"
    printf 'sentinel_token_sha256=%s\n' "$token_sha256"
    printf 'source_user_count=%s\n' "$source_user_count"
    printf 'source_balance_cents=%s\n' "$source_balance_cents"
    printf 'source_ledger_count=%s\n' "$source_ledger_count"
    printf 'scratch_restore_verified=true\n'
    printf 'status=%s\n' "$reset_status"
    printf 'recorded_at_utc=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  } >"$receipt_partial"
  mv "$receipt_partial" "$receipt_file"
}

# Persist authorization evidence before the destructive transaction. If psql
# fails or is interrupted, this receipt remains explicitly pending rather than
# falsely claiming the schema was rebuilt.
write_external_receipt "schema_reset_authorized_pending"

export PGAPPNAME="wangzhe-dev-full-reset"

psql \
  --host "$BACKEND_DATABASE_HOST" \
  --port "$BACKEND_DATABASE_PORT" \
  --username "$BACKEND_DATABASE_USER" \
  --dbname "$BACKEND_DATABASE_DBNAME" \
  --no-psqlrc --set ON_ERROR_STOP=1 \
  --set expected_database="$BACKEND_DATABASE_DBNAME" \
  --set expected_system_identifier="$server_system_identifier" \
  --set expected_server_address="$server_address" \
  --set expected_server_port="$server_port" \
  --set sentinel_token_sha256="$token_sha256" <<'SQL'
BEGIN;
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';
SET LOCAL search_path = pg_catalog, public;
SELECT pg_advisory_xact_lock(729421121);
SELECT set_config('wangzhe.reset_expected_database', :'expected_database', true);
SELECT set_config('wangzhe.reset_expected_system_identifier', :'expected_system_identifier', true);
SELECT set_config('wangzhe.reset_expected_server_address', :'expected_server_address', true);
SELECT set_config('wangzhe.reset_expected_server_port', :'expected_server_port', true);
SELECT set_config('wangzhe.reset_expected_token_sha256', :'sentinel_token_sha256', true);

DO $$
DECLARE
    active_sessions integer;
    guard_applied boolean;
BEGIN
    IF current_database() <> current_setting('wangzhe.reset_expected_database') THEN
        RAISE EXCEPTION 'database identity changed during full reset';
    END IF;
    IF pg_catalog.to_regclass('public.schema_migrations') IS NULL THEN
        RAISE EXCEPTION 'database schema is not a current Wangzhe development database';
    END IF;
    SELECT EXISTS (
        SELECT 1 FROM public.schema_migrations
        WHERE version = '202608270012_reset_identity_receipts.sql'
    ) INTO guard_applied;
    IF NOT guard_applied THEN
        RAISE EXCEPTION 'latest development reset guard migrations are not applied';
    END IF;
    IF (SELECT system_identifier::text FROM pg_catalog.pg_control_system()) <> current_setting('wangzhe.reset_expected_system_identifier')
       OR COALESCE(pg_catalog.inet_server_addr()::text, 'local-socket') <> current_setting('wangzhe.reset_expected_server_address')
       OR COALESCE(pg_catalog.inet_server_port(), 0)::text <> current_setting('wangzhe.reset_expected_server_port') THEN
        RAISE EXCEPTION 'physical database identity changed during full reset';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM wangzhe_meta.development_reset_sentinel sentinel
        WHERE sentinel.singleton AND sentinel.database_name = current_database()
          AND sentinel.system_identifier = current_setting('wangzhe.reset_expected_system_identifier')
          AND sentinel.server_address = current_setting('wangzhe.reset_expected_server_address')
          AND sentinel.server_port::text = current_setting('wangzhe.reset_expected_server_port')
          AND sentinel.token_sha256 = current_setting('wangzhe.reset_expected_token_sha256')
    ) THEN
        RAISE EXCEPTION 'development reset sentinel authorization changed';
    END IF;
    SELECT COUNT(*) INTO active_sessions
    FROM pg_catalog.pg_stat_activity
    WHERE datname = current_database()
      AND pid <> pg_backend_pid()
      AND backend_type = 'client backend';
    IF active_sessions <> 0 THEN
        RAISE EXCEPTION 'database still has % other session(s); stop backend/admin/member processes first', active_sessions;
    END IF;
END $$;

DROP SCHEMA public CASCADE;
CREATE SCHEMA public AUTHORIZATION CURRENT_USER;
COMMENT ON SCHEMA public IS 'Recreated by guarded local development reset; bootstrap pending';
COMMIT;
SQL

write_external_receipt "bootstrap_pending"

echo "本地开发数据库 public schema 已重建为空。"
echo "备份：$backup_file"
echo "外部凭证：$receipt_file"
echo "下一步：启动后端，等待全部版本化 SQL 迁移和本地数据初始化完成。"
echo "完成后运行 dev-reset-complete-receipt.sh；只有严格只读验收通过后凭证才会变为 complete。"
