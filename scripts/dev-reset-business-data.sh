#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
用法：
  scripts/dev-reset-business-data.sh --dry-run [ENV_FILE]
  scripts/dev-reset-business-data.sh --execute --confirm 'RESET:<数据库名>:BUSINESS-DATA' \
    --backup-dir /绝对/备份目录 [ENV_FILE]

此工具只清理本地非 release 数据库的业务记录，保留 schema、迁移记录、
登录账号、工作区、彩票目录和房间配置。执行前必须停止前后端服务。
执行模式必须显式设置绝对路径 BACKEND_UPLOAD_DIR；数据库提交后只会
精确删除其 .private/member-payment-qr 下经过结构校验的会员收款二维码文件。
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
  unset BACKEND_UPLOAD_DIR
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

[[ "$BACKEND_SERVER_MODE" == "debug" ]] || { echo "业务数据重置仅允许 debug 环境" >&2; exit 1; }
case "$BACKEND_DATABASE_HOST" in
  127.0.0.1|localhost|::1) ;;
  *) echo "开发重置只允许连接本机 PostgreSQL，当前主机：$BACKEND_DATABASE_HOST" >&2; exit 1 ;;
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

# Reset receipts are immutable audit/idempotency evidence. Session revocations
# must survive until delivered; advancing auth_version below appends new ones.
# Plan automation rows are operator-owned configuration, not generated history;
# their last-run fields are reset below with the other preserved runtime state.
preserved_tables=(
  schema_migrations development_reset_receipts workspace_migration_markers
  workspace_robot_reset_receipts ws_session_revocation_outbox
  user workspaces workspace_memberships
  workspace_robot_profiles workspace_robot_games workspace_robot_settings
  lottery_games lottery_lobby_categories lottery_play_limits user_play_odds
  room_play_odds room_game_settings system_settings data_retention_policies
  ops_activities special_number_resources special_number_campaigns
  entertainment_platforms wallet_payment_channels plan_automations
)
# Derived cursors/windows must be cleared with messages/issues because the
# corresponding identities restart. Stream children and their parent streams
# are listed together so no CASCADE can widen the approved reset boundary.
# SG recovery attempts and their queue are business history; clear both in the
# same authorized reset so old completions cannot suppress recovery afterward.
cleared_tables=(
  lottery_sgssc_backfill_attempts lottery_sgssc_backfill_items
  user_balance_transactions user_applications lottery_issues lottery_draws lottery_issue_windows
  lottery_bets lottery_assistant_requests lottery_bet_requests plan_recommendations
  plan_generation_receipts plan_streams plan_stream_cycles plan_stream_periods plan_publication_views
  member_payment_accounts activity_participations special_number_grants
  admin_notifications member_notifications member_chat_messages member_chat_read_cursors chat_red_packets
  chat_red_packet_claims rebate_daily_records agent_profit_share_records
  admin_audit_logs system_event_logs data_cleanup_runs admin_audit_log_archives lottery_bet_archives
  user_balance_transaction_archives
)

echo "目标数据库：$BACKEND_DATABASE_HOST:$BACKEND_DATABASE_PORT/$BACKEND_DATABASE_DBNAME"
echo "保留表（${#preserved_tables[@]}）：${preserved_tables[*]}"
echo "清理表（${#cleared_tables[@]}）：${cleared_tables[*]}"
echo "账号余额将归零、登录会话将失效、机器人总开关将关闭。"
echo "彩票期号/同步状态、活动参与/剩余奖池、特殊号码发放计数和计划运行统计将复位。"
echo "执行模式还会精确清理 BACKEND_UPLOAD_DIR/.private/member-payment-qr 下的应用生成文件。"

if [[ "$mode" == "dry-run" ]]; then
  echo "仅预览：没有连接数据库、没有备份、没有修改任何数据。"
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
expected_confirmation="RESET:${BACKEND_DATABASE_DBNAME}:BUSINESS-DATA"
[[ "$confirm_value" == "$expected_confirmation" ]] || {
  echo "确认口令不匹配；需要：$expected_confirmation" >&2
  exit 1
}
[[ -n "$backup_dir" && "$backup_dir" == /* ]] || { echo "--backup-dir 必须是明确的绝对路径" >&2; exit 1; }
case "$backup_dir" in
  /|/home|/Users|"$HOME") echo "拒绝使用过宽的备份目录：$backup_dir" >&2; exit 1 ;;
esac
for command_name in age age-keygen psql awk basename chmod cmp date dirname find id mkdir mktemp mv rm sed sort stat tail tar lsof; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "缺少命令：$command_name" >&2; exit 1; }
done
: "${BACKEND_UPLOAD_DIR:?执行业务数据重置时必须显式设置 BACKEND_UPLOAD_DIR}"
reset_validate_payment_qr_cleanup_target "$BACKEND_UPLOAD_DIR"
case "$backup_dir/" in
  "$RESET_PAYMENT_QR_DIRECTORY/"*) echo "备份目录不能位于待清理的二维码目录内" >&2; exit 1 ;;
esac
payment_qr_directory="$RESET_PAYMENT_QR_DIRECTORY"
payment_qr_expected_count="$RESET_PAYMENT_QR_FILE_COUNT"
echo "已锁定二维码清理目标：$payment_qr_directory（$payment_qr_expected_count 个文件）"

sentinel_token="${BACKEND_DEVELOPMENT_RESET_SENTINEL_TOKEN:-}"
(( ${#sentinel_token} >= 32 && ${#sentinel_token} <= 256 )) || { echo "sentinel token 必须为 32-256 个字符" >&2; exit 1; }

reset_assert_backend_port_stopped
token_sha256="$(reset_sha256 "$sentinel_token")"
unset sentinel_token BACKEND_DEVELOPMENT_RESET_SENTINEL_TOKEN
identity_before="$(reset_verified_identity "$token_sha256" true)"
IFS=$'\t' read -r server_system_identifier server_address server_port _ _ _ <<<"$identity_before"
reset_validate_payment_qr_database_file_consistency "$BACKEND_UPLOAD_DIR"

echo "开始创建重置前完整加密备份……"
backup_output="$("$script_dir/dev-postgres-backup.sh" --backup-dir "$backup_dir")"
printf '%s\n' "$backup_output"
backup_file="$(printf '%s\n' "$backup_output" | sed -n 's/^数据库备份完成：//p' | tail -n 1)"
[[ -n "$backup_file" && -f "$backup_file" && -f "$backup_file.sha256" ]] || {
  echo "无法确认完整备份及校验文件，拒绝继续" >&2
  exit 1
}
backup_sha256="$(awk 'NR == 1 { print $1 }' "$backup_file.sha256")"
[[ "$backup_sha256" =~ ^[0-9a-f]{64}$ ]] || { echo "备份 SHA-256 不正确" >&2; exit 1; }
backup_name="$(basename "$backup_file")"
echo "开始创建与数据库备份配套的加密会员收款二维码归档……"
reset_archive_payment_qr_files "$BACKEND_UPLOAD_DIR" "$backup_file" \
  "${BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY:?缺少 BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY}"
payment_qr_archive="$RESET_PAYMENT_QR_ARCHIVE"
payment_qr_archive_name="$(basename "$payment_qr_archive")"
payment_qr_archive_sha256="$RESET_PAYMENT_QR_ARCHIVE_SHA256"
payment_qr_archived_count="$RESET_PAYMENT_QR_ARCHIVE_FILE_COUNT"
[[ "$payment_qr_archived_count" == "$payment_qr_expected_count" ]] || {
  echo "二维码归档数量与重置前快照不一致，拒绝继续" >&2
  exit 1
}
reset_validate_payment_qr_archive_database_consistency "$payment_qr_archive" "$payment_qr_archive_sha256" \
  "$payment_qr_archived_count" "$BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY"
request_id="dev-reset-$(date -u +%Y%m%dT%H%M%SZ)-$$-${RANDOM}"
receipt_file="$backup_file.reset-receipt"
receipt_partial="$receipt_file.partial"
payment_qr_removed_count="pending"
write_business_reset_receipt() {
  local reset_status="$1"
  umask 077
  {
    printf 'request_id=%s\n' "$request_id"
    printf 'database=%s\n' "$BACKEND_DATABASE_DBNAME"
    printf 'backup=%s\n' "$backup_name"
    printf 'backup_sha256=%s\n' "$backup_sha256"
    printf 'payment_qr_backup=%s\n' "$payment_qr_archive_name"
    printf 'payment_qr_backup_sha256=%s\n' "$payment_qr_archive_sha256"
    printf 'payment_qr_files_archived=%s\n' "$payment_qr_archived_count"
    printf 'payment_qr_files_expected=%s\n' "$payment_qr_expected_count"
    printf 'server_system_identifier=%s\n' "$server_system_identifier"
    printf 'server_address=%s\n' "$server_address"
    printf 'server_port=%s\n' "$server_port"
    printf 'sentinel_token_sha256=%s\n' "$token_sha256"
    printf 'payment_qr_directory=%s\n' "$payment_qr_directory"
    printf 'payment_qr_files_removed=%s\n' "$payment_qr_removed_count"
    printf 'status=%s\n' "$reset_status"
    printf 'recorded_at_utc=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  } >"$receipt_partial"
  chmod 600 "$receipt_partial"
  mv "$receipt_partial" "$receipt_file"
}
unset BACKEND_DEVELOPMENT_BACKUP_AGE_IDENTITY

reset_assert_backend_port_stopped
identity_after="$(reset_verified_identity "$token_sha256" true)"
reset_identity_matches "$identity_before" "$identity_after"

write_business_reset_receipt "business_reset_authorized_pending"

export PGPASSWORD="$BACKEND_DATABASE_PASSWORD"
export PGSSLMODE="${BACKEND_DATABASE_SSLMODE:-disable}"
export PGAPPNAME="wangzhe-dev-reset"

psql \
  --host "$BACKEND_DATABASE_HOST" \
  --port "$BACKEND_DATABASE_PORT" \
  --username "$BACKEND_DATABASE_USER" \
  --dbname "$BACKEND_DATABASE_DBNAME" \
  --no-psqlrc --set ON_ERROR_STOP=1 \
  --set request_id="$request_id" \
  --set backup_name="$backup_name" \
  --set backup_sha256="$backup_sha256" \
  --set expected_database="$BACKEND_DATABASE_DBNAME" \
  --set expected_system_identifier="$server_system_identifier" \
  --set expected_server_address="$server_address" \
  --set expected_server_port="$server_port" \
  --set sentinel_token_sha256="$token_sha256" <<'SQL'
BEGIN;
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';
SET LOCAL search_path = pg_catalog, public;
SELECT pg_advisory_xact_lock(729421120);
SELECT set_config('wangzhe.reset_expected_database', :'expected_database', true);
SELECT set_config('wangzhe.reset_expected_system_identifier', :'expected_system_identifier', true);
SELECT set_config('wangzhe.reset_expected_server_address', :'expected_server_address', true);
SELECT set_config('wangzhe.reset_expected_server_port', :'expected_server_port', true);
SELECT set_config('wangzhe.reset_expected_token_sha256', :'sentinel_token_sha256', true);

DO $$
DECLARE
    active_sessions integer;
BEGIN
    IF current_database() <> current_setting('wangzhe.reset_expected_database') THEN
        RAISE EXCEPTION 'database identity changed during reset';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM public.schema_migrations
        WHERE version = '202608270012_reset_identity_receipts.sql'
    ) THEN
        RAISE EXCEPTION 'latest development reset guard migrations are not applied';
    END IF;
    IF (SELECT system_identifier::text FROM pg_catalog.pg_control_system()) <> current_setting('wangzhe.reset_expected_system_identifier')
       OR COALESCE(pg_catalog.inet_server_addr()::text, 'local-socket') <> current_setting('wangzhe.reset_expected_server_address')
       OR COALESCE(pg_catalog.inet_server_port(), 0)::text <> current_setting('wangzhe.reset_expected_server_port') THEN
        RAISE EXCEPTION 'physical database identity changed during reset';
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

CREATE TEMP TABLE dev_reset_manifest (
    table_name text PRIMARY KEY,
    disposition text NOT NULL CHECK (disposition IN ('preserve', 'clear'))
) ON COMMIT DROP;
INSERT INTO dev_reset_manifest (table_name, disposition) VALUES
  ('schema_migrations','preserve'), ('development_reset_receipts','preserve'),
  ('workspace_migration_markers','preserve'),
  ('workspace_robot_reset_receipts','preserve'), ('ws_session_revocation_outbox','preserve'),
  ('user','preserve'), ('workspaces','preserve'), ('workspace_memberships','preserve'),
  ('workspace_robot_profiles','preserve'), ('workspace_robot_games','preserve'),
  ('workspace_robot_settings','preserve'), ('lottery_games','preserve'),
  ('lottery_lobby_categories','preserve'), ('lottery_play_limits','preserve'),
  ('user_play_odds','preserve'), ('room_play_odds','preserve'),
  ('room_game_settings','preserve'), ('system_settings','preserve'),
  ('data_retention_policies','preserve'), ('ops_activities','preserve'),
  ('special_number_resources','preserve'), ('special_number_campaigns','preserve'),
  ('entertainment_platforms','preserve'), ('wallet_payment_channels','preserve'),
  ('plan_automations','preserve'),
  ('lottery_sgssc_backfill_attempts','clear'), ('lottery_sgssc_backfill_items','clear'),
  ('user_balance_transactions','clear'), ('user_applications','clear'),
  ('lottery_issues','clear'), ('lottery_draws','clear'), ('lottery_issue_windows','clear'), ('lottery_bets','clear'),
  ('lottery_assistant_requests','clear'), ('lottery_bet_requests','clear'),
  ('plan_recommendations','clear'), ('plan_generation_receipts','clear'),
  ('plan_streams','clear'), ('plan_stream_cycles','clear'), ('plan_stream_periods','clear'),
  ('plan_publication_views','clear'),
  ('member_payment_accounts','clear'),
  ('activity_participations','clear'), ('special_number_grants','clear'),
  ('admin_notifications','clear'), ('member_notifications','clear'),
  ('member_chat_messages','clear'), ('member_chat_read_cursors','clear'), ('chat_red_packets','clear'),
  ('chat_red_packet_claims','clear'), ('rebate_daily_records','clear'),
  ('agent_profit_share_records','clear'), ('admin_audit_logs','clear'), ('system_event_logs','clear'),
  ('data_cleanup_runs','clear'), ('admin_audit_log_archives','clear'),
  ('lottery_bet_archives','clear'), ('user_balance_transaction_archives','clear');

DO $$
DECLARE
    unknown_tables text[];
    missing_tables text[];
BEGIN
    SELECT array_agg(table_name ORDER BY table_name) INTO unknown_tables
    FROM (
        SELECT tablename AS table_name FROM pg_catalog.pg_tables WHERE schemaname = 'public'
        EXCEPT SELECT table_name FROM dev_reset_manifest
    ) unknown;
    SELECT array_agg(table_name ORDER BY table_name) INTO missing_tables
    FROM (
        SELECT table_name FROM dev_reset_manifest
        EXCEPT SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname = 'public'
    ) missing;
    IF unknown_tables IS NOT NULL THEN
        RAISE EXCEPTION 'unclassified public tables: %', unknown_tables;
    END IF;
    IF missing_tables IS NOT NULL THEN
        RAISE EXCEPTION 'schema is incomplete; missing tables: %', missing_tables;
    END IF;
END $$;

SELECT set_config('wangzhe.dev_reset', 'confirmed:' || current_database(), true);
SELECT set_config('wangzhe.lifecycle_delete', 'on', true);

TRUNCATE TABLE
    public.lottery_sgssc_backfill_attempts, public.lottery_sgssc_backfill_items,
    public.user_balance_transactions, public.user_applications,
    public.lottery_issues, public.lottery_draws, public.lottery_issue_windows, public.lottery_bets,
    public.lottery_assistant_requests, public.lottery_bet_requests,
    public.plan_recommendations, public.plan_generation_receipts,
    public.plan_streams, public.plan_stream_cycles, public.plan_stream_periods, public.plan_publication_views,
    public.member_payment_accounts,
    public.activity_participations, public.special_number_grants,
    public.admin_notifications, public.member_notifications,
    public.member_chat_messages, public.member_chat_read_cursors, public.chat_red_packets,
    public.chat_red_packet_claims, public.rebate_daily_records,
    public.agent_profit_share_records, public.admin_audit_logs, public.system_event_logs,
    public.data_cleanup_runs, public.admin_audit_log_archives,
    public.lottery_bet_archives, public.user_balance_transaction_archives
RESTART IDENTITY;

UPDATE public."user"
SET balance_cents = 0,
    auth_version = auth_version + 1,
    muted_until = NULL,
    mute_reason = '',
    last_login_at = NULL,
    login_count = 0,
    updated_at = now();

UPDATE public.workspace_robot_settings
SET enabled = false,
    pause_reason = '开发业务数据重置后需人工重新启用',
    last_run_at = NULL,
    last_error = '',
    updated_at = now();

-- Preserved catalogue/configuration rows also contain derived runtime fields.
-- Reset only those fields after their corresponding history was truncated.
-- External and official games stay unavailable until a verified source sync
-- republishes an explicit next issue and draw time.
UPDATE public.lottery_games
SET next_issue = '',
    next_draw_at = NULL,
    timing_source = CASE
        WHEN lower(btrim(source_kind)) IN ('external', 'official') THEN 'pending'
        ELSE 'configured'
    END,
    sync_status = CASE
        WHEN lower(btrim(source_kind)) IN ('external', 'official') THEN 'stale'
        ELSE 'idle'
    END,
    last_sync_at = NULL,
    last_sync_error = '',
    updated_at = now();

UPDATE public.ops_activities
SET participants = 0,
    pool_remaining_cents = pool_total_cents,
    updated_at = now();

UPDATE public.special_number_campaigns
SET granted_count = 0,
    updated_at = now();

UPDATE public.plan_automations
SET last_run_at = NULL,
    last_created_count = 0,
    last_error = '',
    updated_at = now();

-- Fail closed before writing the immutable receipt. A future trigger or schema
-- change must not leave cleared business rows or stale derived state behind.
DO $$
DECLARE
    cleared_table text;
    remaining_rows bigint;
BEGIN
    FOR cleared_table IN
        SELECT table_name
        FROM dev_reset_manifest
        WHERE disposition = 'clear'
        ORDER BY table_name
    LOOP
        EXECUTE format('SELECT count(*) FROM public.%I', cleared_table)
        INTO remaining_rows;
        IF remaining_rows <> 0 THEN
            RAISE EXCEPTION 'cleared table % still contains % row(s) after reset',
                cleared_table, remaining_rows;
        END IF;
    END LOOP;

    IF EXISTS (
        SELECT 1 FROM public."user"
        WHERE balance_cents <> 0 OR muted_until IS NOT NULL
           OR COALESCE(mute_reason, '') <> '' OR last_login_at IS NOT NULL
           OR login_count <> 0
    ) THEN
        RAISE EXCEPTION 'user runtime state was not fully reset';
    END IF;
    IF EXISTS (
        SELECT 1 FROM public.workspace_robot_settings
        WHERE enabled OR pause_reason <> '开发业务数据重置后需人工重新启用'
           OR last_run_at IS NOT NULL OR COALESCE(last_error, '') <> ''
    ) THEN
        RAISE EXCEPTION 'robot runtime state was not fully reset';
    END IF;
    IF EXISTS (
        SELECT 1 FROM public.lottery_games
        WHERE next_issue <> '' OR next_draw_at IS NOT NULL
           OR last_sync_at IS NOT NULL OR COALESCE(last_sync_error, '') <> ''
           OR timing_source <> CASE
                WHEN lower(btrim(source_kind)) IN ('external', 'official') THEN 'pending'
                ELSE 'configured'
              END
           OR sync_status <> CASE
                WHEN lower(btrim(source_kind)) IN ('external', 'official') THEN 'stale'
                ELSE 'idle'
              END
    ) THEN
        RAISE EXCEPTION 'lottery runtime state was not fully reset';
    END IF;
    IF EXISTS (
        SELECT 1 FROM public.ops_activities
        WHERE participants <> 0 OR pool_remaining_cents <> pool_total_cents
    ) THEN
        RAISE EXCEPTION 'activity runtime state was not fully reset';
    END IF;
    IF EXISTS (
        SELECT 1 FROM public.special_number_campaigns WHERE granted_count <> 0
    ) THEN
        RAISE EXCEPTION 'special-number campaign counters were not fully reset';
    END IF;
    IF EXISTS (
        SELECT 1 FROM public.plan_automations
        WHERE last_run_at IS NOT NULL OR last_created_count <> 0
           OR COALESCE(last_error, '') <> ''
    ) THEN
        RAISE EXCEPTION 'plan automation runtime state was not fully reset';
    END IF;
END $$;

INSERT INTO public.development_reset_receipts (
    request_id, database_name, backup_filename, backup_sha256,
    executed_by, reset_scope, cleared_tables, server_system_identifier,
    server_address, server_port, sentinel_token_sha256
)
SELECT :'request_id', current_database(), :'backup_name', :'backup_sha256',
       current_user, 'business_data', array_agg(table_name ORDER BY table_name),
       :'expected_system_identifier', :'expected_server_address',
       :'expected_server_port'::integer, :'sentinel_token_sha256'
FROM dev_reset_manifest
WHERE disposition = 'clear';

COMMIT;
SQL

write_business_reset_receipt "business_reset_committed_qr_cleanup_pending"

# The backend and all database clients were required to be stopped before the
# reset. Revalidate the exact private subtree after the database commit, then
# remove only server-generated QR PNG paths one by one. If this fails, set -e
# prevents a misleading completed receipt; rerunning the authorized reset can
# finish cleanup without widening the deletion scope.
reset_remove_payment_qr_files "$BACKEND_UPLOAD_DIR" "$payment_qr_archived_count"
payment_qr_removed_count="$RESET_PAYMENT_QR_REMOVED_COUNT"
[[ "$payment_qr_removed_count" == "$payment_qr_archived_count" ]] || {
  echo "二维码删除数量与配套归档不一致" >&2
  exit 1
}
write_business_reset_receipt "complete"

echo "开发业务数据重置完成。备份：$backup_file"
echo "配套加密二维码归档：$payment_qr_archive（$payment_qr_archived_count 个文件）"
echo "已精确删除 $payment_qr_removed_count 个会员收款二维码文件（不会递归删除目录）。"
echo "重置凭证：$receipt_file"
echo "重新启动后端前请先确认账号余额归零、机器人关闭及迁移就绪。"
