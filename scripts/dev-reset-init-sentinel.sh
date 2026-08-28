#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
用法：
  scripts/dev-reset-init-sentinel.sh --dry-run [ENV_FILE]
  scripts/dev-reset-init-sentinel.sh --execute \
    --confirm 'INIT:<数据库名>:DEVELOPMENT-SENTINEL' [ENV_FILE]

这是一次性授权步骤。它只在独立 wangzhe_meta schema 写入当前物理数据库
身份和 sentinel token 摘要，不清理任何业务数据，也不会打印 token。
USAGE
}

mode="dry-run"
confirm_value=""
env_file=""
while (($#)); do
  case "$1" in
    --dry-run) mode="dry-run"; shift ;;
    --execute) mode="execute"; shift ;;
    --confirm) [[ $# -ge 2 ]] || { usage >&2; exit 2; }; confirm_value="$2"; shift 2 ;;
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
  unset BACKEND_SERVER_MODE BACKEND_SERVER_PORT BACKEND_DATABASE_HOST
  unset BACKEND_DATABASE_PORT BACKEND_DATABASE_USER BACKEND_DATABASE_PASSWORD
  unset BACKEND_DATABASE_DBNAME BACKEND_DATABASE_SSLMODE
  unset BACKEND_ALLOW_DEVELOPMENT_RESET BACKEND_DEVELOPMENT_RESET_DATABASE
  unset BACKEND_INITIALIZE_DEVELOPMENT_SENTINEL
  unset BACKEND_DEVELOPMENT_RESET_SENTINEL_TOKEN
  load_backend_env "$env_file"
fi

: "${BACKEND_SERVER_MODE:?必须明确设置 BACKEND_SERVER_MODE}"
: "${BACKEND_SERVER_PORT:?缺少 BACKEND_SERVER_PORT}"
: "${BACKEND_DATABASE_HOST:?缺少 BACKEND_DATABASE_HOST}"
: "${BACKEND_DATABASE_PORT:?缺少 BACKEND_DATABASE_PORT}"
: "${BACKEND_DATABASE_USER:?缺少 BACKEND_DATABASE_USER}"
: "${BACKEND_DATABASE_PASSWORD:?缺少 BACKEND_DATABASE_PASSWORD}"
: "${BACKEND_DATABASE_DBNAME:?缺少 BACKEND_DATABASE_DBNAME}"
: "${BACKEND_DATABASE_SSLMODE:?缺少 BACKEND_DATABASE_SSLMODE}"

[[ "$BACKEND_SERVER_MODE" == "debug" ]] || { echo "sentinel 初始化仅允许 debug 环境" >&2; exit 1; }
case "$BACKEND_DATABASE_HOST" in 127.0.0.1|localhost|::1) ;; *) echo "sentinel 初始化只允许本机 PostgreSQL" >&2; exit 1 ;; esac
[[ "$BACKEND_SERVER_PORT" =~ ^[0-9]+$ ]] && (( BACKEND_SERVER_PORT >= 1 && BACKEND_SERVER_PORT <= 65535 )) || { echo "后端端口不正确" >&2; exit 1; }
[[ "$BACKEND_DATABASE_PORT" =~ ^[0-9]+$ ]] && (( BACKEND_DATABASE_PORT >= 1 && BACKEND_DATABASE_PORT <= 65535 )) || { echo "数据库端口不正确" >&2; exit 1; }
[[ "$BACKEND_DATABASE_DBNAME" =~ ^[A-Za-z0-9_.-]+$ ]] || { echo "数据库名格式不安全" >&2; exit 1; }
case "$BACKEND_DATABASE_SSLMODE" in disable|allow|prefer|require|verify-ca|verify-full) ;; *) echo "BACKEND_DATABASE_SSLMODE 不正确" >&2; exit 1 ;; esac

echo "目标数据库：$BACKEND_DATABASE_HOST:$BACKEND_DATABASE_PORT/$BACKEND_DATABASE_DBNAME"
echo "动作：一次性登记物理数据库开发重置 sentinel；不修改业务表。"
if [[ "$mode" == "dry-run" ]]; then
  echo "仅预览：没有连接数据库、没有写入 sentinel。"
  exit 0
fi

[[ "${BACKEND_ALLOW_DEVELOPMENT_RESET:-}" == "YES" ]] || { echo "必须显式设置 BACKEND_ALLOW_DEVELOPMENT_RESET=YES" >&2; exit 1; }
[[ "${BACKEND_INITIALIZE_DEVELOPMENT_SENTINEL:-}" == "YES" ]] || { echo "必须显式设置 BACKEND_INITIALIZE_DEVELOPMENT_SENTINEL=YES" >&2; exit 1; }
[[ "${BACKEND_DEVELOPMENT_RESET_DATABASE:-}" == "$BACKEND_DATABASE_DBNAME" ]] || { echo "开发重置数据库授权不匹配" >&2; exit 1; }
sentinel_token="${BACKEND_DEVELOPMENT_RESET_SENTINEL_TOKEN:-}"
(( ${#sentinel_token} >= 32 && ${#sentinel_token} <= 256 )) || { echo "sentinel token 必须为 32-256 个字符" >&2; exit 1; }
expected_confirmation="INIT:${BACKEND_DATABASE_DBNAME}:DEVELOPMENT-SENTINEL"
[[ "$confirm_value" == "$expected_confirmation" ]] || { echo "确认口令不匹配；需要：$expected_confirmation" >&2; exit 1; }
for command_name in psql lsof awk; do command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }; done
reset_assert_backend_port_stopped
token_sha256="$(reset_sha256 "$sentinel_token")"
unset sentinel_token BACKEND_DEVELOPMENT_RESET_SENTINEL_TOKEN

PGAPPNAME="wangzhe-dev-reset-sentinel-init" reset_psql \
  --quiet --set expected_database="$BACKEND_DATABASE_DBNAME" \
  --set sentinel_token_sha256="$token_sha256" <<'SQL'
BEGIN;
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';
SET LOCAL search_path = pg_catalog, public;
SELECT pg_catalog.pg_advisory_xact_lock(729421122);
SELECT pg_catalog.set_config('wangzhe.reset_expected_database', :'expected_database', true);

DO $$
DECLARE
    other_sessions integer;
BEGIN
    IF pg_catalog.current_database() <> pg_catalog.current_setting('wangzhe.reset_expected_database') THEN
        RAISE EXCEPTION 'database identity does not match the authorized target';
    END IF;
    IF pg_catalog.to_regclass('public.schema_migrations') IS NULL OR NOT EXISTS (
        SELECT 1 FROM public.schema_migrations
        WHERE version = '202608270012_reset_identity_receipts.sql'
    ) THEN
        RAISE EXCEPTION 'apply migration 012 before initializing the sentinel';
    END IF;
    SELECT COUNT(*) INTO other_sessions
    FROM pg_catalog.pg_stat_activity
    WHERE datname = pg_catalog.current_database()
      AND pid <> pg_catalog.pg_backend_pid()
      AND backend_type = 'client backend';
    IF other_sessions <> 0 THEN
        RAISE EXCEPTION 'database still has % other client session(s)', other_sessions;
    END IF;
    IF pg_catalog.to_regclass('wangzhe_meta.development_reset_sentinel') IS NOT NULL THEN
        RAISE EXCEPTION 'development reset sentinel is already initialized';
    END IF;
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_namespace WHERE nspname = 'wangzhe_meta') THEN
        RAISE EXCEPTION 'wangzhe_meta already exists without the expected sentinel; inspect it manually';
    END IF;
END $$;

CREATE SCHEMA wangzhe_meta AUTHORIZATION CURRENT_USER;
CREATE TABLE wangzhe_meta.development_reset_sentinel (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    database_name varchar(120) NOT NULL,
    system_identifier varchar(32) NOT NULL,
    server_address varchar(80) NOT NULL,
    server_port integer NOT NULL CHECK (server_port BETWEEN 1 AND 65535),
    token_sha256 char(64) NOT NULL CHECK (token_sha256 ~ '^[0-9a-f]{64}$'),
    initialized_by varchar(120) NOT NULL,
    initialized_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO wangzhe_meta.development_reset_sentinel (
    singleton, database_name, system_identifier, server_address,
    server_port, token_sha256, initialized_by
)
SELECT true, pg_catalog.current_database(), control.system_identifier::text,
       COALESCE(pg_catalog.inet_server_addr()::text, 'local-socket'),
       COALESCE(pg_catalog.inet_server_port(), 0), :'sentinel_token_sha256', current_user
FROM pg_catalog.pg_control_system() control;

CREATE FUNCTION wangzhe_meta.reject_development_sentinel_mutation() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    RAISE EXCEPTION 'development reset sentinel is immutable' USING ERRCODE = '55000';
END;
$$;
CREATE TRIGGER trg_reject_development_sentinel_update
BEFORE UPDATE OR DELETE ON wangzhe_meta.development_reset_sentinel
FOR EACH ROW EXECUTE FUNCTION wangzhe_meta.reject_development_sentinel_mutation();
CREATE TRIGGER trg_reject_development_sentinel_truncate
BEFORE TRUNCATE ON wangzhe_meta.development_reset_sentinel
FOR EACH STATEMENT EXECUTE FUNCTION wangzhe_meta.reject_development_sentinel_mutation();
COMMIT;
SQL

echo "开发重置 sentinel 初始化完成；token 请继续保存在本机安全环境中。"
