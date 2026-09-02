#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
用法：
  scripts/dev-reset-verify-bootstrap.sh [ENV_FILE]

只读验收“完整重建 public schema 后第一次 debug bootstrap”。
迁移版本、SHA-256 和表清单从当前 backend/migrations/*.sql 读取。
不传 ENV_FILE 时，必须由当前进程显式提供全部 BACKEND_* 数据库变量。
只允许本机 debug 数据库；REPEATABLE READ + READ ONLY 事务不会修改数据。
USAGE
}

env_file=""
case "$#" in
  0) ;;
  1)
    case "$1" in
      -h|--help) usage; exit 0 ;;
      *) env_file="$1" ;;
    esac
    ;;
  *) usage >&2; exit 2 ;;
esac

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/backend-env.sh
source "$script_dir/lib/backend-env.sh"
if [[ -n "$env_file" ]]; then
  load_backend_env "$env_file"
fi

: "${BACKEND_SERVER_MODE:?必须明确设置 BACKEND_SERVER_MODE}"
: "${BACKEND_DATABASE_HOST:?缺少 BACKEND_DATABASE_HOST}"
: "${BACKEND_DATABASE_PORT:?缺少 BACKEND_DATABASE_PORT}"
: "${BACKEND_DATABASE_USER:?缺少 BACKEND_DATABASE_USER}"
: "${BACKEND_DATABASE_PASSWORD:?缺少 BACKEND_DATABASE_PASSWORD}"
: "${BACKEND_DATABASE_DBNAME:?缺少 BACKEND_DATABASE_DBNAME}"
: "${BACKEND_DATABASE_SSLMODE:?缺少 BACKEND_DATABASE_SSLMODE}"

[[ "$BACKEND_SERVER_MODE" == "debug" ]] || { echo "只读重建验收仅允许 debug 环境" >&2; exit 1; }
case "$BACKEND_DATABASE_HOST" in
  127.0.0.1|localhost|::1) ;;
  *) echo "只读重建验收只允许连接本机 PostgreSQL" >&2; exit 1 ;;
esac
[[ "$BACKEND_DATABASE_PORT" =~ ^[0-9]{1,5}$ ]] &&
  (( 10#$BACKEND_DATABASE_PORT > 0 && 10#$BACKEND_DATABASE_PORT <= 65535 )) || { echo "数据库端口不正确" >&2; exit 1; }
[[ "$BACKEND_DATABASE_DBNAME" =~ ^[A-Za-z0-9_.-]+$ ]] || { echo "数据库名不正确" >&2; exit 1; }

psql_bin="${PSQL_BIN:-}"
if [[ -z "$psql_bin" ]]; then
  if command -v psql >/dev/null 2>&1; then
    psql_bin="$(command -v psql)"
  elif [[ -x /Library/PostgreSQL/17/bin/psql ]]; then
    psql_bin=/Library/PostgreSQL/17/bin/psql
  else
    echo "未找到 psql，可通过 PSQL_BIN 指定" >&2
    exit 1
  fi
fi
[[ -x "$psql_bin" ]] || { echo "psql 不可执行：$psql_bin" >&2; exit 1; }

migration_dir="$script_dir/../backend/migrations"
migration_inventory="["
separator=""
for migration_file in "$migration_dir"/*.sql; do
  [[ -f "$migration_file" ]] || { echo "迁移清单为空" >&2; exit 1; }
  version="${migration_file##*/}"
  [[ "$version" =~ ^[0-9]{12}_[a-z0-9_]+\.sql$ ]] || { echo "迁移文件名不正确：$version" >&2; exit 1; }
  if command -v sha256sum >/dev/null 2>&1; then
    checksum="$(sha256sum "$migration_file")"
  else
    checksum="$(shasum -a 256 "$migration_file")"
  fi
  checksum="${checksum%% *}"
  [[ "$checksum" =~ ^[0-9a-f]{64}$ ]] || { echo "无法计算迁移 SHA-256：$version" >&2; exit 1; }
  migration_inventory+="${separator}{\"version\":\"$version\",\"checksum\":\"$checksum\"}"
  separator=","
done
migration_inventory+="]"

# Checked-in CREATE TABLE declarations are deliberately single-line and use
# public (or the connection's public search_path). Reject unrecognized forms
# rather than quietly dropping a table from the read-only acceptance inventory.
table_declarations="$(grep -Eih '^[[:space:]]*CREATE[[:space:]]+TABLE([[:space:]]|$)' "$migration_dir"/*.sql)"
table_names="$(printf '%s\n' "$table_declarations" |
  sed -nE 's/^[[:space:]]*CREATE TABLE( IF NOT EXISTS)? ("?public"?\.)?"?([a-z_][a-z0-9_]*)"?[[:space:]]*\(.*/\3/p')"
[[ -n "$table_names" && "$(printf '%s\n' "$table_declarations" | wc -l | tr -d ' ')" == "$(printf '%s\n' "$table_names" | wc -l | tr -d ' ')" ]] || {
  echo "无法完整解析 SQL 表清单；新增建表语法需同步只读验收" >&2
  exit 1
}
table_inventory='["schema_migrations"'
while IFS= read -r table_name; do
  table_inventory+=",\"$table_name\""
done < <(printf '%s\n' "$table_names" | LC_ALL=C sort -u)
table_inventory+=']'

# Do not let inherited libpq service/host options redirect this local check.
unset PGHOSTADDR PGSERVICE PGSERVICEFILE PGOPTIONS
export PGPASSWORD="$BACKEND_DATABASE_PASSWORD"
export PGSSLMODE="$BACKEND_DATABASE_SSLMODE"
export PGAPPNAME="wangzhe-bootstrap-readonly-verify"

echo "开始只读验收：$BACKEND_DATABASE_HOST:$BACKEND_DATABASE_PORT/$BACKEND_DATABASE_DBNAME"
trap 'echo "BOOTSTRAP_ACCEPTANCE=FAIL" >&2' ERR

"$psql_bin" \
  --host "$BACKEND_DATABASE_HOST" \
  --port "$BACKEND_DATABASE_PORT" \
  --username "$BACKEND_DATABASE_USER" \
  --dbname "$BACKEND_DATABASE_DBNAME" \
  --no-psqlrc --set ON_ERROR_STOP=1 \
  --set expected_database="$BACKEND_DATABASE_DBNAME" \
  --set migration_inventory="$migration_inventory" \
  --set table_inventory="$table_inventory" <<'SQL'
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SET LOCAL search_path = public, pg_catalog;
SET LOCAL statement_timeout = '30s';
SELECT set_config('wangzhe.verify_expected_database', :'expected_database', true) AS verify_database,
       set_config('wangzhe.verify_migration_inventory', :'migration_inventory', true) AS verify_migrations,
       set_config('wangzhe.verify_table_inventory', :'table_inventory', true) AS verify_tables \gset

\echo '[1/6] 当前 SQL 清单、校验和与数据库结构保护'
DO $$
DECLARE
  bad_tables text[];
  bad_migrations text[];
  unguarded_tables text[];
  index_count bigint;
BEGIN
  IF current_database() <> current_setting('wangzhe.verify_expected_database') THEN
    RAISE EXCEPTION '数据库身份不匹配：%', current_database();
  END IF;
  WITH expected(name) AS (
    SELECT jsonb_array_elements_text(current_setting('wangzhe.verify_table_inventory')::jsonb)
  ), actual AS (
    SELECT tablename AS name FROM pg_catalog.pg_tables WHERE schemaname = 'public'
  )
  SELECT array_agg(COALESCE(expected.name, actual.name) ORDER BY COALESCE(expected.name, actual.name))
    INTO bad_tables FROM expected FULL JOIN actual USING (name)
    WHERE expected.name IS NULL OR actual.name IS NULL;
  IF bad_tables IS NOT NULL THEN
    RAISE EXCEPTION '表清单与当前 SQL 不一致（缺失或多余）：%', bad_tables;
  END IF;
  WITH expected AS (
    SELECT * FROM jsonb_to_recordset(current_setting('wangzhe.verify_migration_inventory')::jsonb)
      AS item(version text, checksum text)
  )
  SELECT array_agg(COALESCE(expected.version, applied.version) ORDER BY COALESCE(expected.version, applied.version))
    INTO bad_migrations FROM expected FULL JOIN schema_migrations AS applied USING (version)
    WHERE expected.version IS NULL OR applied.version IS NULL OR applied.checksum <> expected.checksum;
  IF bad_migrations IS NOT NULL THEN
    RAISE EXCEPTION '迁移缺失、多余或 checksum 不匹配：%', bad_migrations;
  END IF;
  SELECT array_agg(relation.relname ORDER BY relation.relname) INTO unguarded_tables
  FROM pg_catalog.pg_class AS relation
  JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
  WHERE namespace.nspname = 'public' AND relation.relkind IN ('r', 'p')
    AND NOT EXISTS (
      SELECT 1 FROM pg_catalog.pg_trigger AS guard
      WHERE guard.tgrelid = relation.oid AND NOT guard.tgisinternal AND guard.tgenabled IN ('O', 'A')
        AND guard.tgname IN ('trg_reject_unapproved_application_truncate', 'trg_reject_development_reset_receipt_truncate')
    );
  IF unguarded_tables IS NOT NULL THEN
    RAISE EXCEPTION '表缺少有效的 TRUNCATE 保护：%', unguarded_tables;
  END IF;
  SELECT count(*) INTO index_count FROM pg_catalog.pg_index AS definition
  JOIN pg_catalog.pg_class AS index_relation ON index_relation.oid = definition.indexrelid
  JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = index_relation.relnamespace
  WHERE namespace.nspname = 'public' AND definition.indisunique AND definition.indisvalid AND definition.indisready
    AND index_relation.relname IN (
      'idx_user_username_global_ci', 'idx_workspace_public_room_code', 'idx_user_applications_user_request',
      'idx_application_one_pending_join', 'idx_workspace_one_active_membership',
      'idx_room_odds_game_play', 'idx_user_odds_game_play'
    );
  IF index_count <> 7 THEN
    RAISE EXCEPTION '缺少迁移管理的并发唯一索引：%/7', index_count;
  END IF;
END $$;

\echo '[2/6] 本地基线账号、随机会员号与机器人隔离'
DO $$
DECLARE
  bad_accounts text[];
BEGIN
  WITH expected(username, role_name, balance) AS (VALUES
    ('admin', 'admin', 0::bigint), ('wangzhetenant', 'tenant', 0::bigint),
    ('suyang', 'agent', 0::bigint), ('wangzhe88', 'member', 1000000000::bigint)
  )
  SELECT array_agg(expected.username ORDER BY expected.username) INTO bad_accounts
  FROM expected LEFT JOIN "user" AS account ON lower(account.username) = lower(expected.username) AND account.deleted_at IS NULL
  WHERE account.user_id IS NULL OR account.role <> expected.role_name OR account.status <> 1 OR account.balance_cents <> expected.balance;
  IF bad_accounts IS NOT NULL THEN
    RAISE EXCEPTION '默认账号缺失或属性不正确：%', bad_accounts;
  END IF;
  IF EXISTS (SELECT 1 FROM "user" WHERE deleted_at IS NOT NULL OR auth_version <= 1 OR public_id NOT BETWEEN 1000000 AND 9999999)
     OR (SELECT count(DISTINCT public_id) FROM "user") <> (SELECT count(*) FROM "user")
     OR (SELECT count(*) FROM "user") <> 4 + (SELECT count(*) FROM workspace_robot_profiles) THEN
    RAISE EXCEPTION '账号清单、auth_version 或七位随机会员号不正确';
  END IF;
  IF EXISTS (
    SELECT 1 FROM workspace_robot_profiles AS profile
    LEFT JOIN "user" AS account ON account.user_id = profile.user_id
    LEFT JOIN workspaces AS room ON room.id = profile.workspace_id
    WHERE account.user_id IS NULL OR room.id IS NULL OR account.workspace_id <> profile.workspace_id
      OR account.role <> 'member' OR account.balance_cents < 0 OR account.status <> 1 OR profile.enabled IS NOT TRUE
  ) THEN
    RAISE EXCEPTION '初始机器人应为独立房间成员、非负余额且未跨房间';
  END IF;
END $$;

\echo '[3/6] 平台→租户→代理工作区、入房审核与默认关闭'
DO $$
BEGIN
  IF (SELECT count(*) FROM workspaces) <> 3
     OR NOT EXISTS (SELECT 1 FROM workspaces room JOIN "user" owner ON owner.user_id=room.owner_user_id WHERE room.type='platform' AND room.code='00000' AND room.room_code='' AND room.parent_id IS NULL AND owner.username='admin')
     OR NOT EXISTS (SELECT 1 FROM workspaces room JOIN workspaces parent ON parent.id=room.parent_id JOIN "user" owner ON owner.user_id=room.owner_user_id WHERE room.type='tenant' AND parent.type='platform' AND room.room_code ~ '^[0-9]{5,12}$' AND owner.username='wangzhetenant')
     OR NOT EXISTS (SELECT 1 FROM workspaces room JOIN workspaces parent ON parent.id=room.parent_id JOIN "user" owner ON owner.user_id=room.owner_user_id WHERE room.type='agent' AND parent.type='tenant' AND room.room_code='88001' AND owner.username='suyang' AND owner.parent_tenant_id=parent.owner_user_id) THEN
    RAISE EXCEPTION '工作区树、公开房间号或所属账号不正确';
  END IF;
  IF EXISTS (SELECT 1 FROM workspaces WHERE status <> 1)
     OR (SELECT count(*) FROM workspace_memberships) <> (SELECT count(*) FROM "user")
     OR EXISTS (
       SELECT 1 FROM "user" account LEFT JOIN workspace_memberships membership ON membership.user_id=account.user_id
       LEFT JOIN workspaces room ON room.id=account.workspace_id
       WHERE room.id IS NULL OR membership.id IS NULL OR membership.workspace_id <> account.workspace_id
         OR membership.role <> account.role OR membership.status <> 1
     ) THEN
    RAISE EXCEPTION '账号与活动房间成员关系不一致';
  END IF;
  IF (SELECT count(*) FROM system_settings) <> (SELECT count(*) FROM workspaces)
     OR (SELECT count(*) FROM workspace_robot_settings) <> (SELECT count(*) FROM workspaces)
     OR EXISTS (
       SELECT 1 FROM workspaces room LEFT JOIN system_settings settings ON settings.workspace_id=room.id
       LEFT JOIN workspace_robot_settings robots ON robots.workspace_id=room.id
       WHERE settings.id IS NULL OR settings.room_code <> room.room_code OR settings.room_enabled IS NOT TRUE
         OR (room.type IN ('tenant','agent') AND settings.require_join_review IS NOT TRUE)
         OR robots.workspace_id IS NULL OR robots.enabled IS NOT FALSE
         OR NOT EXISTS (SELECT 1 FROM workspace_robot_profiles profile WHERE profile.workspace_id=room.id)
     ) THEN
    RAISE EXCEPTION '房间设置、入房审核或机器人默认关闭状态不正确';
  END IF;
  IF EXISTS (
    SELECT 1 FROM workspaces room CROSS JOIN lottery_games game
    LEFT JOIN room_game_settings setting ON setting.workspace_id=room.id AND setting.game_id=game.id
    WHERE room.type IN ('tenant','agent') AND (setting.id IS NULL OR setting.enabled IS NOT FALSE)
  ) THEN
    RAISE EXCEPTION '新房间必须显式保存所有彩种并默认关闭';
  END IF;
END $$;

\echo '[4/6] 期初余额与不可变账务逐账号对平'
DO $$
BEGIN
  IF (SELECT count(*) FROM user_balance_transactions) <> (SELECT count(*) FROM "user" WHERE balance_cents > 0)
     OR EXISTS (
       SELECT 1 FROM user_balance_transactions ledger LEFT JOIN "user" account ON account.user_id=ledger.user_id
       WHERE account.user_id IS NULL OR ledger.type <> 'opening_balance' OR ledger.before_cents <> 0
         OR ledger.amount_cents <> account.balance_cents OR ledger.after_cents <> account.balance_cents
         OR ledger.workspace_id <> account.workspace_id
     )
     OR EXISTS (
       SELECT 1 FROM "user" account WHERE account.balance_cents < 0 OR
         (account.balance_cents > 0 AND (SELECT count(*) FROM user_balance_transactions ledger WHERE ledger.user_id=account.user_id) <> 1)
     )
     OR (SELECT COALESCE(sum(amount_cents),0) FROM user_balance_transactions) <>
        (SELECT COALESCE(sum(balance_cents),0) FROM "user") THEN
    RAISE EXCEPTION '期初余额与开户流水不一致';
  END IF;
END $$;

\echo '[5/6] 目录已初始化，清理策略与娱乐连接默认关闭'
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM lottery_games) OR NOT EXISTS (SELECT 1 FROM lottery_lobby_categories WHERE deleted_at IS NULL)
     OR NOT EXISTS (SELECT 1 FROM entertainment_platforms)
     OR EXISTS (SELECT 1 FROM entertainment_platforms WHERE status='enabled')
     OR EXISTS (
       SELECT 1 FROM workspaces room
       WHERE NOT EXISTS (SELECT 1 FROM ops_activities activity WHERE activity.workspace_id=room.id AND activity.deleted_at IS NULL)
          OR NOT EXISTS (SELECT 1 FROM wallet_payment_channels channel WHERE channel.workspace_id=room.id AND channel.deleted_at IS NULL)
     ) THEN
    RAISE EXCEPTION '当前业务目录未完整初始化或娱乐连接已启用';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM data_retention_policies)
     OR EXISTS (SELECT 1 FROM data_retention_policies WHERE enabled) THEN
    RAISE EXCEPTION '新库清理策略必须存在且默认关闭';
  END IF;
END $$;

\echo '[6/6] 无旧业务、未确认赔率或演示计划'
DO $$
DECLARE
  table_name text;
  row_count bigint;
BEGIN
  FOREACH table_name IN ARRAY ARRAY[
    'development_reset_receipts', 'workspace_robot_reset_receipts', 'user_applications',
    'lottery_bets', 'lottery_assistant_requests', 'lottery_bet_requests', 'member_payment_accounts',
    'activity_participations', 'special_number_resources', 'special_number_campaigns', 'special_number_grants',
    'admin_notifications', 'member_notifications', 'member_chat_messages', 'member_chat_read_cursors',
    'chat_red_packets', 'chat_red_packet_claims', 'rebate_daily_records', 'agent_profit_share_records',
    'admin_audit_logs', 'data_cleanup_runs', 'admin_audit_log_archives', 'lottery_bet_archives',
    'user_balance_transaction_archives', 'lottery_play_limits', 'user_play_odds', 'room_play_odds',
    'workspace_robot_games', 'plan_recommendations', 'plan_automations', 'plan_generation_receipts',
    'plan_streams', 'plan_stream_cycles', 'plan_stream_periods'
  ] LOOP
    EXECUTE format('SELECT count(*) FROM public.%I', table_name) INTO row_count;
    IF row_count <> 0 THEN
      RAISE EXCEPTION '新库表 % 不应包含旧业务或未配置数据，行数=%', table_name, row_count;
    END IF;
  END LOOP;
END $$;

COMMIT;
SQL

trap - ERR
echo "BOOTSTRAP_ACCEPTANCE=PASS"
