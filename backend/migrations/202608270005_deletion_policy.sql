-- Deletion-policy hardening is deliberately additive and transaction-safe.
-- migrations.Run executes this whole file in one transaction, so a failed
-- constraint or trigger creation rolls back every change in this version.

-- Member receiving accounts and lobby categories are recoverable records.
-- Their public DELETE actions now become GORM soft deletes.
ALTER TABLE IF EXISTS member_payment_accounts
    ADD COLUMN IF NOT EXISTS deleted_at timestamptz NULL;

ALTER TABLE IF EXISTS lottery_lobby_categories
    ADD COLUMN IF NOT EXISTS deleted_at timestamptz NULL;

UPDATE member_payment_accounts
SET is_default = false
WHERE deleted_at IS NOT NULL AND is_default;

DROP INDEX IF EXISTS idx_member_payment_account_one_default;
CREATE UNIQUE INDEX IF NOT EXISTS idx_member_payment_account_one_default
    ON member_payment_accounts (user_id)
    WHERE is_default AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_member_payment_accounts_deleted_at
    ON member_payment_accounts (deleted_at);
CREATE INDEX IF NOT EXISTS idx_lottery_lobby_categories_deleted_at
    ON lottery_lobby_categories (deleted_at);

-- A physical user deletion must never cascade away receiving-account or
-- notification history. Normal account removal remains an UPDATE of deleted_at.
ALTER TABLE member_payment_accounts
    DROP CONSTRAINT IF EXISTS fk_member_payment_account_user;
ALTER TABLE member_payment_accounts
    ADD CONSTRAINT fk_member_payment_account_user
    FOREIGN KEY (user_id) REFERENCES "user" (user_id) ON DELETE RESTRICT NOT VALID;
ALTER TABLE member_payment_accounts
    VALIDATE CONSTRAINT fk_member_payment_account_user;

ALTER TABLE member_notifications
    DROP CONSTRAINT IF EXISTS fk_member_notification_user;
ALTER TABLE member_notifications
    ADD CONSTRAINT fk_member_notification_user
    FOREIGN KEY (user_id) REFERENCES "user" (user_id) ON DELETE RESTRICT NOT VALID;
ALTER TABLE member_notifications
    VALIDATE CONSTRAINT fk_member_notification_user;

-- Robot-authored ordinary text can be retired independently from real-member
-- conversations. It is disabled by default and uses reversible soft deletion.
ALTER TABLE data_retention_policies
    DROP CONSTRAINT IF EXISTS ck_retention_policy_class;
ALTER TABLE data_retention_policies
    ADD CONSTRAINT ck_retention_policy_class CHECK (
        data_class IN (
            'chat_messages', 'robot_chat_messages', 'notifications',
            'audit_logs', 'robot_test_data'
        )
    );

INSERT INTO data_retention_policies
    (workspace_id, data_class, enabled, retention_days, action)
VALUES (0, 'robot_chat_messages', false, 30, 'soft_delete')
ON CONFLICT (workspace_id, data_class) DO NOTHING;

-- These tables are either immutable business evidence or have an explicit
-- soft-delete/status lifecycle. Rejecting DELETE does not affect settlement,
-- which only UPDATEs rows. Bets, balance ledgers and hot audit rows are not in
-- this trigger list because their verified cold/archive workflows perform the
-- only sanctioned hot-table removal.
CREATE OR REPLACE FUNCTION reject_protected_hard_delete() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'hard DELETE is disabled for protected table %', TG_TABLE_NAME
        USING ERRCODE = '55000';
END;
$$ LANGUAGE plpgsql;

DO $$
DECLARE
    protected_table text;
    protected_relation regclass;
BEGIN
    FOREACH protected_table IN ARRAY ARRAY[
        'user',
        'user_applications',
        'lottery_draws',
        'lottery_issues',
        'activity_participations',
        'ops_activities',
        'member_chat_messages',
        'chat_red_packets',
        'chat_red_packet_claims',
        'member_notifications',
        'admin_notifications',
        'rebate_daily_records',
        'agent_profit_share_records',
        'special_number_grants',
        'wallet_payment_channels',
        'member_payment_accounts',
        'lottery_lobby_categories'
    ] LOOP
        protected_relation := to_regclass(format('%I', protected_table));
        IF protected_relation IS NOT NULL AND NOT EXISTS (
            SELECT 1
            FROM pg_trigger
            WHERE tgrelid = protected_relation
              AND tgname = 'trg_reject_protected_hard_delete'
              AND NOT tgisinternal
        ) THEN
            EXECUTE format(
                'CREATE TRIGGER trg_reject_protected_hard_delete '
                'BEFORE DELETE ON %I FOR EACH ROW '
                'EXECUTE FUNCTION reject_protected_hard_delete()',
                protected_table
            );
        END IF;
    END LOOP;
END $$;
