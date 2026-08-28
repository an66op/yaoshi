#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
用法：
  scripts/dev-reset-verify-bootstrap.sh [ENV_FILE]

只读验收“完整重建 public schema 后第一次 debug bootstrap”。
不传 ENV_FILE 时，必须由当前进程显式提供全部 BACKEND_* 数据库变量。
脚本使用 REPEATABLE READ + READ ONLY 事务，不会修改 schema 或数据。
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
  unset BACKEND_SERVER_MODE BACKEND_DATABASE_HOST BACKEND_DATABASE_PORT
  unset BACKEND_DATABASE_USER BACKEND_DATABASE_PASSWORD BACKEND_DATABASE_DBNAME
  unset BACKEND_DATABASE_SSLMODE
  load_backend_env "$env_file"
fi

: "${BACKEND_SERVER_MODE:?必须明确设置 BACKEND_SERVER_MODE}"
: "${BACKEND_DATABASE_HOST:?缺少 BACKEND_DATABASE_HOST}"
: "${BACKEND_DATABASE_PORT:?缺少 BACKEND_DATABASE_PORT}"
: "${BACKEND_DATABASE_USER:?缺少 BACKEND_DATABASE_USER}"
: "${BACKEND_DATABASE_PASSWORD:?缺少 BACKEND_DATABASE_PASSWORD}"
: "${BACKEND_DATABASE_DBNAME:?缺少 BACKEND_DATABASE_DBNAME}"
: "${BACKEND_DATABASE_SSLMODE:?缺少 BACKEND_DATABASE_SSLMODE}"

[[ "$BACKEND_SERVER_MODE" == "debug" ]] || {
  echo "该基线只适用于 debug bootstrap，当前模式：$BACKEND_SERVER_MODE" >&2
  exit 1
}
[[ "$BACKEND_DATABASE_PORT" =~ ^[0-9]+$ ]] || { echo "数据库端口不正确" >&2; exit 1; }

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
  --set expected_database="$BACKEND_DATABASE_DBNAME" <<'SQL'
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SET LOCAL search_path = public, pg_catalog;
SET LOCAL statement_timeout = '30s';
SELECT set_config('wangzhe.verify_expected_database', :'expected_database', true);

\echo '[1/9] 数据库身份、表清单与迁移版本'
DO $$
DECLARE
  missing_tables text[];
  unexpected_tables text[];
  bad_migrations text[];
  unexpected_migrations text[];
BEGIN
  IF current_database() <> current_setting('wangzhe.verify_expected_database') THEN
    RAISE EXCEPTION '数据库身份不匹配：%', current_database();
  END IF;

  WITH expected(name) AS (VALUES
    ('schema_migrations'), ('development_reset_receipts'), ('workspace_migration_markers'),
    ('user'), ('user_balance_transactions'), ('user_applications'),
    ('lottery_games'), ('lottery_lobby_categories'), ('lottery_draws'), ('lottery_issues'),
    ('system_settings'), ('workspaces'), ('workspace_memberships'),
    ('workspace_robot_profiles'), ('workspace_robot_games'), ('workspace_robot_settings'),
    ('lottery_play_limits'), ('user_play_odds'), ('room_play_odds'), ('plan_recommendations'),
    ('wallet_payment_channels'), ('member_payment_accounts'),
    ('lottery_bets'), ('lottery_assistant_requests'), ('lottery_bet_requests'),
    ('ops_activities'), ('activity_participations'),
    ('special_number_resources'), ('special_number_campaigns'), ('special_number_grants'),
    ('entertainment_platforms'), ('admin_notifications'), ('member_notifications'),
    ('member_chat_messages'), ('chat_red_packets'), ('chat_red_packet_claims'),
    ('room_game_settings'), ('rebate_daily_records'), ('agent_profit_share_records'),
    ('admin_audit_logs'), ('data_retention_policies'), ('data_cleanup_runs'),
    ('admin_audit_log_archives'), ('lottery_bet_archives'), ('user_balance_transaction_archives')
  )
  SELECT array_agg(name ORDER BY name) INTO missing_tables
  FROM (SELECT name FROM expected EXCEPT SELECT tablename FROM pg_tables WHERE schemaname = 'public') missing;

  WITH expected(name) AS (VALUES
    ('schema_migrations'), ('development_reset_receipts'), ('workspace_migration_markers'),
    ('user'), ('user_balance_transactions'), ('user_applications'),
    ('lottery_games'), ('lottery_lobby_categories'), ('lottery_draws'), ('lottery_issues'),
    ('system_settings'), ('workspaces'), ('workspace_memberships'),
    ('workspace_robot_profiles'), ('workspace_robot_games'), ('workspace_robot_settings'),
    ('lottery_play_limits'), ('user_play_odds'), ('room_play_odds'), ('plan_recommendations'),
    ('wallet_payment_channels'), ('member_payment_accounts'),
    ('lottery_bets'), ('lottery_assistant_requests'), ('lottery_bet_requests'),
    ('ops_activities'), ('activity_participations'),
    ('special_number_resources'), ('special_number_campaigns'), ('special_number_grants'),
    ('entertainment_platforms'), ('admin_notifications'), ('member_notifications'),
    ('member_chat_messages'), ('chat_red_packets'), ('chat_red_packet_claims'),
    ('room_game_settings'), ('rebate_daily_records'), ('agent_profit_share_records'),
    ('admin_audit_logs'), ('data_retention_policies'), ('data_cleanup_runs'),
    ('admin_audit_log_archives'), ('lottery_bet_archives'), ('user_balance_transaction_archives')
  )
  SELECT array_agg(tablename ORDER BY tablename) INTO unexpected_tables
  FROM (SELECT tablename FROM pg_tables WHERE schemaname = 'public' EXCEPT SELECT name FROM expected) extra;

  IF missing_tables IS NOT NULL OR unexpected_tables IS NOT NULL THEN
    RAISE EXCEPTION 'schema 不符合基线；缺失=%，多余=%', missing_tables, unexpected_tables;
  END IF;

  WITH expected(version, checksum) AS (VALUES
    ('202608270001_data_lifecycle.sql', '18460c363db2a6c72a03e38701df1641aa7fef43dfa112180e68dfd77d2e035c'),
    ('202608270002_robot_safety_limits.sql', 'b5785a53c28c8cc8d09131e8f47b79a2d4c7bf630e1566ad1a64bbce19f2d389'),
    ('202608270003_session_version.sql', 'cbdc5c3f2d8d912e8a688b26d6f0bc04cfebc81375bd407b053712bbfba8e9f6'),
    ('202608270004_soft_delete_config.sql', 'e3a93d6c0ca7e5bf8f1333608cff757c77ebf88f8d38c84e076224a302357c72'),
    ('202608270005_deletion_policy.sql', '900f096d0642e56eacb36b5d342aa52ea27c5518f29c586fb6cbd969e4422db2'),
    ('202608270006_guard_financial_deletes.sql', '621bbc92c9653196f60de07a911143501f1ce9cd7dd5db272ff2f3ca77057bd5'),
    ('202608270007_service_welcome.sql', 'aedb439e1c97ffcc10506d2e5ba2046c00e637b5378907064e80b599220eab3c'),
    ('202608270008_plan_recommendations.sql', '0cb45d60c7e1ad04e782073991280175287eab598eeab2a90eefd6a4394f62b6'),
    ('202608270009_lifecycle_audit_integrity.sql', 'fd3d6ca01c6d5309fd25ba884690d3c339cabb8e0c5a15f269913aa090c7f8cb'),
    ('202608270010_dev_reset_guard.sql', 'a450f56f6d8e55a29e7500ae0af7bb7090047201248b1dbddb4016a75e5bea05'),
    ('202608270011_reset_guard_workspace_marker.sql', '79de36840eccd23cfe8d9fbbb53d3fcdde3b48b47d35ef13f48a20c2fe3df4cb'),
    ('202608270012_reset_identity_receipts.sql', '3d1e70e3dfac18a963fada882783d5f09653da7e3f743c5bfde04ea7f038a5de')
  )
  SELECT array_agg(expected.version ORDER BY expected.version) INTO bad_migrations
  FROM expected LEFT JOIN schema_migrations applied USING (version)
  WHERE applied.version IS NULL OR applied.checksum <> expected.checksum;

  WITH expected(version) AS (VALUES
    ('202608270001_data_lifecycle.sql'), ('202608270002_robot_safety_limits.sql'),
    ('202608270003_session_version.sql'), ('202608270004_soft_delete_config.sql'),
    ('202608270005_deletion_policy.sql'), ('202608270006_guard_financial_deletes.sql'),
    ('202608270007_service_welcome.sql'), ('202608270008_plan_recommendations.sql'),
    ('202608270009_lifecycle_audit_integrity.sql'), ('202608270010_dev_reset_guard.sql'),
    ('202608270011_reset_guard_workspace_marker.sql'), ('202608270012_reset_identity_receipts.sql')
  )
  SELECT array_agg(version ORDER BY version) INTO unexpected_migrations
  FROM (SELECT version FROM schema_migrations EXCEPT SELECT version FROM expected) extra;

  IF bad_migrations IS NOT NULL OR unexpected_migrations IS NOT NULL OR (SELECT COUNT(*) FROM schema_migrations) <> 12 THEN
    RAISE EXCEPTION '迁移不完整或 checksum 不匹配；异常=%，多余=%', bad_migrations, unexpected_migrations;
  END IF;
END $$;

\echo '[2/9] 默认账号、auth_version 与公开会员号'
DO $$
DECLARE
  bad_accounts text[];
BEGIN
  IF (SELECT COUNT(*) FROM "user" WHERE deleted_at IS NULL) <> 16 THEN
    RAISE EXCEPTION '新库应有 16 个账号（4 个基线账号 + 12 个机器人），实际=%',
      (SELECT COUNT(*) FROM "user" WHERE deleted_at IS NULL);
  END IF;
  WITH expected(username, role_name, balance) AS (VALUES
    ('admin', 'admin', 0::bigint),
    ('wangzhetenant', 'tenant', 0::bigint),
    ('suyang', 'agent', 0::bigint),
    ('wangzhe88', 'member', 1000000000::bigint)
  )
  SELECT array_agg(expected.username ORDER BY expected.username) INTO bad_accounts
  FROM expected LEFT JOIN "user" account ON lower(account.username) = lower(expected.username) AND account.deleted_at IS NULL
  WHERE account.user_id IS NULL OR account.role <> expected.role_name OR account.status <> 1
     OR account.balance_cents <> expected.balance;
  IF bad_accounts IS NOT NULL THEN
    RAISE EXCEPTION '默认账号缺失或属性不正确：%', bad_accounts;
  END IF;
  IF EXISTS (SELECT 1 FROM "user" WHERE deleted_at IS NOT NULL OR auth_version <= 1) THEN
    RAISE EXCEPTION '存在软删除账号或 auth_version <= 1，旧 JWT 可能未失效';
  END IF;
  IF EXISTS (SELECT 1 FROM "user" WHERE public_id < 1000000)
     OR (SELECT COUNT(DISTINCT public_id) FROM "user") <> 16 THEN
    RAISE EXCEPTION '公开会员号未从 7 位数开始或存在重复';
  END IF;
  IF (SELECT COUNT(*) FROM "user" WHERE remark = '房间活跃账号') <> 12 THEN
    RAISE EXCEPTION '应有 12 个房间机器人账号';
  END IF;
END $$;

\echo '[3/9] 平台→租户→代理→会员工作区隔离'
DO $$
DECLARE
  admin_id bigint;
  tenant_id bigint;
  agent_id bigint;
  member_id bigint;
  platform_ws bigint;
  tenant_ws bigint;
  agent_ws bigint;
BEGIN
  SELECT user_id INTO admin_id FROM "user" WHERE username = 'admin';
  SELECT user_id INTO tenant_id FROM "user" WHERE username = 'wangzhetenant';
  SELECT user_id INTO agent_id FROM "user" WHERE username = 'suyang';
  SELECT user_id INTO member_id FROM "user" WHERE username = 'wangzhe88';
  SELECT id INTO platform_ws FROM workspaces WHERE type = 'platform';
  SELECT id INTO tenant_ws FROM workspaces WHERE type = 'tenant';
  SELECT id INTO agent_ws FROM workspaces WHERE type = 'agent';

  IF (SELECT COUNT(*) FROM workspaces) <> 3
     OR platform_ws IS NULL OR tenant_ws IS NULL OR agent_ws IS NULL THEN
    RAISE EXCEPTION '工作区必须且只能是 platform/tenant/agent 各 1 个';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM workspaces WHERE id=platform_ws AND code='00000' AND room_code='' AND owner_user_id=admin_id AND parent_id IS NULL AND status=1)
     OR NOT EXISTS (SELECT 1 FROM workspaces WHERE id=tenant_ws AND code='00001' AND room_code='100001' AND owner_user_id=tenant_id AND parent_id=platform_ws AND status=1)
     OR NOT EXISTS (SELECT 1 FROM workspaces WHERE id=agent_ws AND code='8801' AND room_code='8801' AND owner_user_id=agent_id AND parent_id=tenant_ws AND status=1) THEN
    RAISE EXCEPTION '工作区树、内部编号或公开房间号不符合新库基线';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM "user" WHERE user_id=admin_id AND workspace_id=platform_ws)
     OR NOT EXISTS (SELECT 1 FROM "user" WHERE user_id=tenant_id AND workspace_id=tenant_ws)
     OR NOT EXISTS (SELECT 1 FROM "user" WHERE user_id=agent_id AND workspace_id=agent_ws AND parent_tenant_id=tenant_id)
     OR NOT EXISTS (SELECT 1 FROM "user" WHERE user_id=member_id AND workspace_id=agent_ws AND parent_agent_id=agent_id AND parent_tenant_id=tenant_id) THEN
    RAISE EXCEPTION '基线账号与工作区归属不正确';
  END IF;
  IF (SELECT COUNT(*) FROM workspace_memberships) <> 16
     OR EXISTS (SELECT 1 FROM workspace_memberships WHERE status <> 1)
     OR EXISTS (
       SELECT 1 FROM workspace_memberships membership
       JOIN "user" account ON account.user_id = membership.user_id
       WHERE membership.workspace_id <> account.workspace_id OR membership.role <> account.role
     ) THEN
    RAISE EXCEPTION '每个账号必须只有一条与当前工作区匹配的有效会员关系';
  END IF;
  IF EXISTS (SELECT 1 FROM "user" WHERE workspace_id = 0)
     OR EXISTS (SELECT 1 FROM workspace_robot_profiles WHERE workspace_id NOT IN (tenant_ws, agent_ws)) THEN
    RAISE EXCEPTION '存在未归属或跨房间的初始账号/机器人';
  END IF;
  IF (SELECT COUNT(*) FROM workspace_robot_profiles WHERE workspace_id=tenant_ws) <> 6
     OR (SELECT COUNT(*) FROM workspace_robot_profiles WHERE workspace_id=agent_ws) <> 6 THEN
    RAISE EXCEPTION '租户直属房间和代理房间应各有 6 个独立机器人';
  END IF;
END $$;

\echo '[4/9] 房间设置、入房审核与机器人默认关闭'
DO $$
BEGIN
  IF (SELECT COUNT(*) FROM system_settings) <> 3
     OR EXISTS (
       SELECT 1 FROM workspaces workspace
       LEFT JOIN system_settings settings ON settings.workspace_id = workspace.id
       WHERE settings.id IS NULL OR settings.room_code <> workspace.room_code OR settings.room_enabled IS NOT TRUE
     ) THEN
    RAISE EXCEPTION '每个工作区必须只有一套同步的系统设置';
  END IF;
  IF EXISTS (
    SELECT 1 FROM workspaces workspace
    JOIN system_settings settings ON settings.workspace_id = workspace.id
    WHERE workspace.type IN ('tenant','agent') AND settings.require_join_review IS NOT TRUE
  ) THEN
    RAISE EXCEPTION '正式房间必须默认开启入房审核';
  END IF;
  IF (SELECT COUNT(*) FROM workspace_migration_markers WHERE key='20260827_enable_join_review_for_formal_rooms') <> 1 THEN
    RAISE EXCEPTION '入房审核一次性迁移标记缺失';
  END IF;
  IF (SELECT COUNT(*) FROM workspace_robot_settings) <> 3
     OR EXISTS (SELECT 1 FROM workspace_robot_settings WHERE enabled IS TRUE) THEN
    RAISE EXCEPTION '所有工作区的机器人调度器必须默认关闭';
  END IF;
  IF (SELECT COUNT(*) FROM workspace_robot_profiles) <> 12
     OR EXISTS (SELECT 1 FROM workspace_robot_profiles WHERE enabled IS NOT TRUE)
     OR (SELECT COUNT(*) FROM workspace_robot_games) <> 0 THEN
    RAISE EXCEPTION '机器人身份应已就绪且默认跟随全部开放彩种；只能由调度器总开关控制是否运行';
  END IF;
END $$;

\echo '[5/9] 余额、opening ledger 与积分守恒'
DO $$
BEGIN
  IF (SELECT COUNT(*) FROM "user" WHERE balance_cents > 0) <> 13
     OR (SELECT COALESCE(SUM(balance_cents),0) FROM "user") <> 13000000000 THEN
    RAISE EXCEPTION '体验会员 + 12 个机器人的总余额应为 13000000000 分';
  END IF;
  IF (SELECT COUNT(*) FROM user_balance_transactions) <> 13
     OR EXISTS (
       SELECT 1 FROM user_balance_transactions ledger
       JOIN "user" account ON account.user_id = ledger.user_id
       WHERE ledger.type <> 'opening_balance' OR ledger.before_cents <> 0
          OR ledger.amount_cents <> account.balance_cents
          OR ledger.after_cents <> account.balance_cents
          OR ledger.workspace_id <> account.workspace_id
     )
     OR EXISTS (
       SELECT 1 FROM "user" account
       WHERE account.balance_cents > 0
         AND (SELECT COUNT(*) FROM user_balance_transactions ledger WHERE ledger.user_id=account.user_id) <> 1
     ) THEN
    RAISE EXCEPTION '新库必须只有 13 条与当前余额/工作区一致的 opening_balance 流水';
  END IF;
  IF (SELECT COALESCE(SUM(amount_cents),0) FROM user_balance_transactions) <>
     (SELECT COALESCE(SUM(balance_cents),0) FROM "user") THEN
    RAISE EXCEPTION '开户流水总额与账号余额不守恒';
  END IF;
END $$;

\echo '[6/9] 30 个彩种、22 开放、8 关闭、4 个分类与默认赔率'
DO $$
DECLARE
  bad_categories text[];
BEGIN
  IF (SELECT COUNT(*) FROM lottery_games) <> 30
     OR (SELECT COUNT(*) FROM lottery_games WHERE enabled) <> 22
     OR (SELECT COUNT(*) FROM lottery_games WHERE NOT enabled) <> 8 THEN
    RAISE EXCEPTION '彩种数量应为总数 30 / 开放 22 / 关闭 8';
  END IF;
  IF (SELECT COUNT(*) FROM lottery_games WHERE source_kind='external') <> 17
     OR (SELECT COUNT(*) FROM lottery_games WHERE source_kind='platform') <> 5
     OR (SELECT COUNT(*) FROM lottery_games WHERE source_kind='official') <> 8 THEN
    RAISE EXCEPTION '开奖源分组应为 external 17 / platform 5 / official 8';
  END IF;
  IF EXISTS (SELECT 1 FROM lottery_games WHERE source_kind='official' AND enabled) THEN
    RAISE EXCEPTION '8 个官方备选彩种必须保持关闭';
  END IF;
  IF (SELECT COUNT(*) FROM lottery_lobby_categories WHERE deleted_at IS NULL) <> 4
     OR EXISTS (SELECT 1 FROM lottery_lobby_categories WHERE deleted_at IS NOT NULL) THEN
    RAISE EXCEPTION '前台必须只有 4 个未删除分类';
  END IF;
  WITH expected(name, sort_order, game_count) AS (VALUES
    ('彩票',10,8::bigint), ('宾果',20,7::bigint), ('PC',30,3::bigint), ('六合彩',40,4::bigint)
  ), actual AS (
    SELECT category.name, category.sort_order, COUNT(game.id) FILTER (WHERE game.enabled) AS game_count
    FROM lottery_lobby_categories category
    LEFT JOIN lottery_games game ON game.lobby_category=category.name
    WHERE category.deleted_at IS NULL
    GROUP BY category.name, category.sort_order
  )
  SELECT array_agg(expected.name ORDER BY expected.sort_order) INTO bad_categories
  FROM expected LEFT JOIN actual USING(name)
  WHERE actual.name IS NULL OR actual.sort_order <> expected.sort_order OR actual.game_count <> expected.game_count;
  IF bad_categories IS NOT NULL THEN
    RAISE EXCEPTION '分类顺序或开放彩种数不正确：%', bad_categories;
  END IF;
  IF (SELECT COUNT(*) FROM lottery_play_limits) <> 270
     OR EXISTS (
       SELECT 1 FROM lottery_games game
       WHERE (SELECT COUNT(*) FROM lottery_play_limits limits WHERE limits.game_id=game.id) <> 9
     ) THEN
    RAISE EXCEPTION '30 个彩种应各有 9 组默认赔率/限额，合计 270 条';
  END IF;
END $$;

\echo '[7/9] debug 初始目录与 fixture（不能当成旧业务数据）'
DO $$
BEGIN
  IF (SELECT COUNT(*) FROM ops_activities) <> 39
     OR EXISTS (
       SELECT 1 FROM workspaces workspace
       WHERE (SELECT COUNT(*) FROM ops_activities activity WHERE activity.workspace_id=workspace.id) <> 13
     ) THEN
    RAISE EXCEPTION '每个工作区应有 13 条默认活动目录，合计 39 条';
  END IF;
  IF (SELECT COUNT(*) FROM wallet_payment_channels) <> 15
     OR EXISTS (
       SELECT 1 FROM workspaces workspace
       WHERE (SELECT COUNT(*) FROM wallet_payment_channels channel WHERE channel.workspace_id=workspace.id) <> 5
     ) THEN
    RAISE EXCEPTION '每个工作区应有 5 条默认收款目录，合计 15 条';
  END IF;
  IF (SELECT COUNT(*) FROM entertainment_platforms) <> 17
     OR EXISTS (SELECT 1 FROM entertainment_platforms WHERE status='enabled') THEN
    RAISE EXCEPTION '应有 17 条未启用的娱乐供应商目录';
  END IF;
  IF (SELECT COUNT(*) FROM plan_recommendations WHERE deleted_at IS NULL) <> 18
     OR EXISTS (
       SELECT 1 FROM workspaces workspace
       WHERE workspace.type IN ('tenant','agent')
         AND (SELECT COUNT(*) FROM plan_recommendations plan WHERE plan.workspace_id=workspace.id AND plan.deleted_at IS NULL) <> 9
     )
     OR EXISTS (
       SELECT 1 FROM plan_recommendations plan
       LEFT JOIN lottery_issues issue ON issue.game_id=plan.game_id AND issue.issue=plan.issue
       WHERE plan.deleted_at IS NULL AND issue.id IS NULL
     ) THEN
    RAISE EXCEPTION 'debug 应为两个正式房间各创建 9 条带真实期号的计划，合计 18 条';
  END IF;
  IF (SELECT COUNT(*) FROM lottery_draws) < 264
     OR EXISTS (
       SELECT 1 FROM lottery_games game
       WHERE game.enabled AND (SELECT COUNT(*) FROM lottery_draws draw WHERE draw.game_id=game.id) < 12
     ) THEN
    RAISE EXCEPTION 'debug fixture 应为 22 个开放彩种各保留至少 12 期初始开奖（合计至少 264）';
  END IF;
END $$;

\echo '[8/9] 清理策略默认关闭'
DO $$
DECLARE
  bad_policies text[];
BEGIN
  WITH expected(data_class, days, action) AS (VALUES
    ('chat_messages',180,'soft_delete'),
    ('robot_chat_messages',30,'soft_delete'),
    ('notifications',180,'soft_delete'),
    ('audit_logs',730,'archive_then_purge_hot'),
    ('robot_test_data',90,'cold_archive')
  )
  SELECT array_agg(expected.data_class ORDER BY expected.data_class) INTO bad_policies
  FROM expected LEFT JOIN data_retention_policies policy
    ON policy.workspace_id=0 AND policy.data_class=expected.data_class
  WHERE policy.id IS NULL OR policy.enabled IS TRUE OR policy.retention_days <> expected.days OR policy.action <> expected.action;
  IF (SELECT COUNT(*) FROM data_retention_policies) <> 5 OR bad_policies IS NOT NULL THEN
    RAISE EXCEPTION '清理策略必须只有 5 条平台默认且全部关闭；异常=%', bad_policies;
  END IF;
END $$;

\echo '[9/9] 旧业务、旧缓存与历史孤儿必须为 0'
DO $$
DECLARE
  dirty_tables text[];
BEGIN
  SELECT array_agg(table_name ORDER BY table_name) INTO dirty_tables
  FROM (
    SELECT 'development_reset_receipts' AS table_name, COUNT(*) AS row_count FROM development_reset_receipts
    UNION ALL SELECT 'user_applications', COUNT(*) FROM user_applications
    UNION ALL SELECT 'lottery_bets', COUNT(*) FROM lottery_bets
    UNION ALL SELECT 'lottery_assistant_requests', COUNT(*) FROM lottery_assistant_requests
    UNION ALL SELECT 'lottery_bet_requests', COUNT(*) FROM lottery_bet_requests
    UNION ALL SELECT 'member_payment_accounts', COUNT(*) FROM member_payment_accounts
    UNION ALL SELECT 'activity_participations', COUNT(*) FROM activity_participations
    UNION ALL SELECT 'special_number_resources', COUNT(*) FROM special_number_resources
    UNION ALL SELECT 'special_number_campaigns', COUNT(*) FROM special_number_campaigns
    UNION ALL SELECT 'special_number_grants', COUNT(*) FROM special_number_grants
    UNION ALL SELECT 'admin_notifications', COUNT(*) FROM admin_notifications
    UNION ALL SELECT 'member_notifications', COUNT(*) FROM member_notifications
    UNION ALL SELECT 'member_chat_messages', COUNT(*) FROM member_chat_messages
    UNION ALL SELECT 'chat_red_packets', COUNT(*) FROM chat_red_packets
    UNION ALL SELECT 'chat_red_packet_claims', COUNT(*) FROM chat_red_packet_claims
    UNION ALL SELECT 'rebate_daily_records', COUNT(*) FROM rebate_daily_records
    UNION ALL SELECT 'agent_profit_share_records', COUNT(*) FROM agent_profit_share_records
    UNION ALL SELECT 'admin_audit_logs', COUNT(*) FROM admin_audit_logs
    UNION ALL SELECT 'data_cleanup_runs', COUNT(*) FROM data_cleanup_runs
    UNION ALL SELECT 'admin_audit_log_archives', COUNT(*) FROM admin_audit_log_archives
    UNION ALL SELECT 'lottery_bet_archives', COUNT(*) FROM lottery_bet_archives
    UNION ALL SELECT 'user_balance_transaction_archives', COUNT(*) FROM user_balance_transaction_archives
    UNION ALL SELECT 'user_play_odds', COUNT(*) FROM user_play_odds
    UNION ALL SELECT 'room_play_odds', COUNT(*) FROM room_play_odds
    UNION ALL SELECT 'room_game_settings', COUNT(*) FROM room_game_settings
    UNION ALL SELECT 'workspace_robot_games', COUNT(*) FROM workspace_robot_games
  ) counts
  WHERE row_count <> 0;
  IF dirty_tables IS NOT NULL THEN
    RAISE EXCEPTION '新库仍包含旧业务或未配置覆盖数据：%', dirty_tables;
  END IF;
END $$;

COMMIT;
SQL

trap - ERR
echo "BOOTSTRAP_ACCEPTANCE=PASS"
