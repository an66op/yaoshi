-- Data lifecycle policy and immutable cleanup execution ledger.
-- Financial ledgers and lottery bets are deliberately absent from every
-- DELETE statement in this migration and in the matching service.

CREATE TABLE IF NOT EXISTS data_retention_policies (
    id bigserial PRIMARY KEY,
    workspace_id bigint NOT NULL DEFAULT 0,
    data_class varchar(40) NOT NULL,
    enabled boolean NOT NULL DEFAULT false,
    retention_days integer NOT NULL,
    action varchar(32) NOT NULL,
    updated_by_id bigint NOT NULL DEFAULT 0,
    updated_by_name varchar(80) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_retention_policy_workspace_class UNIQUE (workspace_id, data_class),
    CONSTRAINT ck_retention_policy_days CHECK (retention_days BETWEEN 1 AND 3650),
    CONSTRAINT ck_retention_policy_class CHECK (data_class IN ('chat_messages', 'notifications', 'audit_logs', 'robot_test_data')),
    CONSTRAINT ck_retention_policy_action CHECK (action IN ('soft_delete', 'archive_then_purge_hot', 'cold_archive'))
);

CREATE INDEX IF NOT EXISTS idx_retention_policy_workspace ON data_retention_policies (workspace_id);

CREATE TABLE IF NOT EXISTS data_cleanup_runs (
    id bigserial PRIMARY KEY,
    request_id varchar(96) NOT NULL UNIQUE,
    workspace_id bigint NOT NULL DEFAULT 0,
    all_workspaces boolean NOT NULL DEFAULT false,
    actor_id bigint NOT NULL,
    actor_name varchar(80) NOT NULL,
    status varchar(24) NOT NULL,
    batch_limit integer NOT NULL DEFAULT 5000,
    criteria_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    preview_json jsonb NOT NULL DEFAULT '[]'::jsonb,
    result_json jsonb NOT NULL DEFAULT '[]'::jsonb,
    last_error varchar(500) NOT NULL DEFAULT '',
    started_at timestamptz NULL,
    completed_at timestamptz NULL,
    soft_restored_at timestamptz NULL,
    financial_restored_at timestamptz NULL,
    restored_by_id bigint NOT NULL DEFAULT 0,
    soft_restore_result_json jsonb NOT NULL DEFAULT '[]'::jsonb,
    financial_restore_result_json jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_cleanup_run_status CHECK (status IN ('previewed', 'running', 'completed', 'failed')),
    CONSTRAINT ck_cleanup_run_scope CHECK ((all_workspaces AND workspace_id = 0) OR (NOT all_workspaces AND workspace_id > 0)),
    CONSTRAINT ck_cleanup_run_batch CHECK (batch_limit BETWEEN 1 AND 20000)
);

CREATE INDEX IF NOT EXISTS idx_cleanup_run_created ON data_cleanup_runs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_cleanup_run_workspace ON data_cleanup_runs (workspace_id, created_at DESC);

-- Audit records move out of the hot table only after an immutable copy exists.
CREATE TABLE IF NOT EXISTS admin_audit_log_archives (
    source_id bigint PRIMARY KEY,
    workspace_id bigint NOT NULL DEFAULT 0,
    actor_id bigint NOT NULL,
    actor_name varchar(80) NOT NULL,
    actor_role varchar(20) NOT NULL,
    room_scope varchar(64) NOT NULL DEFAULT '',
    method varchar(10) NOT NULL,
    path varchar(240) NOT NULL,
    status_code integer NOT NULL,
    request_id varchar(96) NOT NULL DEFAULT '',
    ip varchar(80) NOT NULL DEFAULT '',
    source_created_at timestamptz NOT NULL,
    archived_at timestamptz NOT NULL DEFAULT now(),
    cleanup_request_id varchar(96) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_archive_workspace_created ON admin_audit_log_archives (workspace_id, source_created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_archive_cleanup ON admin_audit_log_archives (cleanup_request_id);

-- Robot financial rows use cold archive tables. Every original column and
-- primary key is retained together with a canonical PostgreSQL JSONB hash and
-- cleanup request. The service only removes a hot row after the archived hash
-- has been verified. Pending, abnormal and real-member rows are never selected.
CREATE TABLE IF NOT EXISTS lottery_bet_archives (
    id bigint PRIMARY KEY,
    workspace_id bigint NOT NULL,
    game_id varchar(40) NOT NULL,
    issue varchar(64) NOT NULL,
    room_scope varchar(64) NOT NULL,
    user_id bigint NOT NULL,
    username varchar(50) NOT NULL,
    play_code varchar(40) NOT NULL,
    play_name varchar(40) NOT NULL,
    position bigint NOT NULL,
    selection varchar(40) NOT NULL,
    amount_cents bigint NOT NULL,
    odds numeric NOT NULL,
    status varchar(20) NOT NULL,
    payout_cents bigint NOT NULL,
    fly_cents bigint NOT NULL,
    rebate_rate_snapshot numeric NOT NULL,
    rebate_cents bigint NOT NULL,
    agent_share_rate_snapshot numeric NOT NULL,
    agent_share_cents bigint NOT NULL,
    settled_at timestamptz NULL,
    remark varchar(300) NULL,
    operator varchar(80) NULL,
    reconciliation_status varchar(24) NOT NULL,
    reconciliation_note varchar(500) NULL,
    created_at timestamptz NULL,
    updated_at timestamptz NULL,
    source_json jsonb NOT NULL,
    row_hash char(32) NOT NULL,
    archived_at timestamptz NOT NULL DEFAULT now(),
    cleanup_request_id varchar(96) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_bet_archive_workspace_created ON lottery_bet_archives (workspace_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_bet_archive_user_created ON lottery_bet_archives (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_bet_archive_cleanup ON lottery_bet_archives (cleanup_request_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_bet_archive_dedupe
ON lottery_bet_archives (room_scope, game_id, issue, user_id, play_code, position, selection);

CREATE TABLE IF NOT EXISTS user_balance_transaction_archives (
    id bigint PRIMARY KEY,
    workspace_id bigint NOT NULL,
    user_id bigint NOT NULL,
    reference varchar(180) NOT NULL,
    amount_cents bigint NOT NULL,
    before_cents bigint NOT NULL,
    after_cents bigint NOT NULL,
    type varchar(30) NOT NULL,
    remark varchar(300) NULL,
    operator varchar(80) NULL,
    created_at timestamptz NULL,
    source_json jsonb NOT NULL,
    row_hash char(32) NOT NULL,
    archived_at timestamptz NOT NULL DEFAULT now(),
    cleanup_request_id varchar(96) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_balance_archive_workspace_created ON user_balance_transaction_archives (workspace_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_balance_archive_user_created ON user_balance_transaction_archives (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_balance_archive_cleanup ON user_balance_transaction_archives (cleanup_request_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_balance_archive_reference
ON user_balance_transaction_archives (user_id, reference) WHERE reference <> '';

-- Archiving must not weaken the original idempotency constraints. These
-- triggers reject a future insert whose business identity already exists in a
-- cold archive, using PostgreSQL's unique-violation error code.
CREATE OR REPLACE FUNCTION lifecycle_guard_archived_bet() RETURNS trigger AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM lottery_bet_archives archived
        WHERE archived.room_scope = NEW.room_scope
          AND archived.game_id = NEW.game_id
          AND archived.issue = NEW.issue
          AND archived.user_id = NEW.user_id
          AND archived.play_code = NEW.play_code
          AND archived.position = NEW.position
          AND archived.selection = NEW.selection
    ) THEN
        RAISE EXCEPTION 'bet already exists in cold archive' USING ERRCODE = '23505';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION lifecycle_guard_archived_balance_reference() RETURNS trigger AS $$
BEGIN
    IF NEW.reference <> '' AND EXISTS (
        SELECT 1 FROM user_balance_transaction_archives archived
        WHERE archived.user_id = NEW.user_id AND archived.reference = NEW.reference
    ) THEN
        RAISE EXCEPTION 'balance reference already exists in cold archive' USING ERRCODE = '23505';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF to_regclass('lottery_bets') IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'trg_lifecycle_guard_archived_bet'
    ) THEN
        CREATE TRIGGER trg_lifecycle_guard_archived_bet
        BEFORE INSERT ON lottery_bets FOR EACH ROW EXECUTE FUNCTION lifecycle_guard_archived_bet();
    END IF;
    IF to_regclass('user_balance_transactions') IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'trg_lifecycle_guard_archived_balance_reference'
    ) THEN
        CREATE TRIGGER trg_lifecycle_guard_archived_balance_reference
        BEFORE INSERT ON user_balance_transactions FOR EACH ROW EXECUTE FUNCTION lifecycle_guard_archived_balance_reference();
    END IF;
END $$;

-- Notification cleanup is a reversible soft delete. Existing installations
-- receive these columns here; fresh databases also receive them from the model
-- when AutoMigrate creates the notification tables after this migration.
ALTER TABLE IF EXISTS member_notifications ADD COLUMN IF NOT EXISTS deleted_at timestamptz NULL;
ALTER TABLE IF EXISTS member_notifications ADD COLUMN IF NOT EXISTS deleted_by varchar(80) NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS member_notifications ADD COLUMN IF NOT EXISTS cleanup_request_id varchar(96) NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS admin_notifications ADD COLUMN IF NOT EXISTS deleted_at timestamptz NULL;
ALTER TABLE IF EXISTS admin_notifications ADD COLUMN IF NOT EXISTS deleted_by varchar(80) NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS admin_notifications ADD COLUMN IF NOT EXISTS cleanup_request_id varchar(96) NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS member_chat_messages ADD COLUMN IF NOT EXISTS deleted_at timestamptz NULL;
ALTER TABLE IF EXISTS member_chat_messages ADD COLUMN IF NOT EXISTS deleted_by varchar(80) NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS member_chat_messages ADD COLUMN IF NOT EXISTS cleanup_request_id varchar(96) NOT NULL DEFAULT '';

DO $$
BEGIN
    IF to_regclass('member_notifications') IS NOT NULL THEN
        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_member_notifications_deleted_at ON member_notifications (deleted_at) WHERE deleted_at IS NOT NULL';
        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_member_notifications_cleanup_request ON member_notifications (cleanup_request_id)';
    END IF;
    IF to_regclass('admin_notifications') IS NOT NULL THEN
        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_admin_notifications_deleted_at ON admin_notifications (deleted_at) WHERE deleted_at IS NOT NULL';
        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_admin_notifications_cleanup_request ON admin_notifications (cleanup_request_id)';
    END IF;
    IF to_regclass('member_chat_messages') IS NOT NULL THEN
        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_member_chat_cleanup_request ON member_chat_messages (cleanup_request_id)';
    END IF;
END $$;

INSERT INTO data_retention_policies (workspace_id, data_class, enabled, retention_days, action)
VALUES
    (0, 'chat_messages', false, 180, 'soft_delete'),
    (0, 'notifications', false, 180, 'soft_delete'),
    (0, 'audit_logs', false, 730, 'archive_then_purge_hot'),
    (0, 'robot_test_data', false, 90, 'cold_archive')
ON CONFLICT (workspace_id, data_class) DO NOTHING;
