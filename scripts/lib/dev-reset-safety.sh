#!/usr/bin/env bash

reset_sha256() {
  local value="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    printf '%s' "$value" | sha256sum | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    printf '%s' "$value" | shasum -a 256 | awk '{print $1}'
  else
    echo "缺少 sha256sum 或 shasum" >&2
    return 1
  fi
}

reset_psql() {
  PGPASSWORD="$BACKEND_DATABASE_PASSWORD" \
  PGSSLMODE="$BACKEND_DATABASE_SSLMODE" \
  psql \
    --host "$BACKEND_DATABASE_HOST" \
    --port "$BACKEND_DATABASE_PORT" \
    --username "$BACKEND_DATABASE_USER" \
    --dbname "$BACKEND_DATABASE_DBNAME" \
    --no-psqlrc --set ON_ERROR_STOP=1 "$@"
}

reset_assert_backend_port_stopped() {
  if lsof -nP -iTCP:"$BACKEND_SERVER_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "后端端口 $BACKEND_SERVER_PORT 仍有监听进程；请先停止后端" >&2
    return 1
  fi
}

# Emits one tab-separated line:
# system_identifier, server_address, server_port, user_count,
# total_balance_cents, balance_transaction_count.
reset_verified_identity() {
  local token_sha256="$1"
  local require_quiescent="${2:-true}"
  [[ "$token_sha256" =~ ^[0-9a-f]{64}$ ]] || { echo "开发 sentinel token 摘要不正确" >&2; return 1; }
  [[ "$require_quiescent" == "true" || "$require_quiescent" == "false" ]] || return 1

  PGAPPNAME="wangzhe-dev-reset-identity" reset_psql \
    --quiet --tuples-only --no-align --field-separator=$'\t' \
    --set expected_database="$BACKEND_DATABASE_DBNAME" \
    --set sentinel_token_sha256="$token_sha256" \
    --set require_quiescent="$require_quiescent" <<'SQL'
SET search_path = pg_catalog, public;
-- Store the guard values without emitting result rows.  This function's stdout
-- is a machine-readable single TSV row; ordinary SELECT set_config(...) calls
-- would prepend three lines and corrupt the caller's snapshot comparison.
SELECT pg_catalog.set_config('wangzhe.reset_expected_database', :'expected_database', false) AS reset_expected_database
\gset
SELECT pg_catalog.set_config('wangzhe.reset_expected_token_sha256', :'sentinel_token_sha256', false) AS reset_expected_token
\gset
SELECT pg_catalog.set_config('wangzhe.reset_require_quiescent', :'require_quiescent', false) AS reset_require_quiescent
\gset

DO $$
DECLARE
    live_system_identifier text;
    live_address text;
    live_port integer;
    other_sessions integer;
BEGIN
    IF pg_catalog.current_database() <> pg_catalog.current_setting('wangzhe.reset_expected_database') THEN
        RAISE EXCEPTION 'database identity does not match the authorized target';
    END IF;
    IF pg_catalog.to_regclass('wangzhe_meta.development_reset_sentinel') IS NULL THEN
        RAISE EXCEPTION 'development reset sentinel is not initialized';
    END IF;
    IF pg_catalog.to_regclass('public.schema_migrations') IS NULL OR NOT EXISTS (
        SELECT 1 FROM public.schema_migrations
        WHERE version = '202608270012_reset_identity_receipts.sql'
    ) THEN
        RAISE EXCEPTION 'latest development reset migrations are not applied';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM pg_catalog.pg_class relation
        JOIN pg_catalog.pg_namespace namespace ON namespace.oid = relation.relnamespace
        WHERE relation.relkind IN ('r', 'p')
          AND namespace.nspname NOT IN ('public', 'wangzhe_meta', 'pg_catalog', 'information_schema')
          AND namespace.nspname NOT LIKE 'pg_toast%'
          AND namespace.nspname NOT LIKE 'pg_temp%'
          AND (
              relation.relname ~ '^(lottery|workspace|member|chat|user|data|admin|agent|room|ops|special|entertainment|wallet|rebate|plan)_'
              OR relation.relname IN ('user', 'workspaces', 'system_settings', 'schema_migrations', 'activity_participations')
          )
    ) THEN
        RAISE EXCEPTION 'Wangzhe application tables exist outside public schema';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM pg_catalog.pg_class relation
        JOIN pg_catalog.pg_namespace namespace ON namespace.oid = relation.relnamespace
        WHERE namespace.nspname = 'wangzhe_meta'
          AND relation.relkind IN ('r', 'p')
          AND relation.relname <> 'development_reset_sentinel'
    ) THEN
        RAISE EXCEPTION 'wangzhe_meta contains an unapproved table';
    END IF;

    SELECT system_identifier::text INTO live_system_identifier
    FROM pg_catalog.pg_control_system();
    live_address := COALESCE(pg_catalog.inet_server_addr()::text, 'local-socket');
    live_port := COALESCE(pg_catalog.inet_server_port(), 0);
    IF NOT EXISTS (
        SELECT 1 FROM wangzhe_meta.development_reset_sentinel sentinel
        WHERE sentinel.singleton
          AND sentinel.database_name = pg_catalog.current_database()
          AND sentinel.system_identifier = live_system_identifier
          AND sentinel.server_address = live_address
          AND sentinel.server_port = live_port
          AND sentinel.token_sha256 = pg_catalog.current_setting('wangzhe.reset_expected_token_sha256')
    ) THEN
        RAISE EXCEPTION 'development reset sentinel does not match this physical database';
    END IF;

    IF pg_catalog.current_setting('wangzhe.reset_require_quiescent') = 'true' THEN
        SELECT COUNT(*) INTO other_sessions
        FROM pg_catalog.pg_stat_activity
        WHERE datname = pg_catalog.current_database()
          AND pid <> pg_catalog.pg_backend_pid()
          AND backend_type = 'client backend';
        IF other_sessions <> 0 THEN
            RAISE EXCEPTION 'database still has % other client session(s)', other_sessions;
        END IF;
    END IF;
END $$;

SELECT control.system_identifier::text,
       COALESCE(pg_catalog.inet_server_addr()::text, 'local-socket'),
       COALESCE(pg_catalog.inet_server_port(), 0),
       (SELECT COUNT(*) FROM public."user"),
       (SELECT COALESCE(SUM(balance_cents), 0) FROM public."user"),
       (SELECT COUNT(*) FROM public.user_balance_transactions)
FROM pg_catalog.pg_control_system() control;
SQL
}

reset_identity_matches() {
  local before="$1" after="$2"
  [[ -n "$before" && "$before" == "$after" ]] || {
    echo "备份期间数据库身份或关键账务计数发生变化，拒绝重置" >&2
    return 1
  }
}
