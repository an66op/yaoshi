-- Referenced room configuration is retired rather than physically erased.
-- Existing activities retain participant history and existing payment
-- channels remain available to audits while disappearing from active lists.

ALTER TABLE IF EXISTS ops_activities
    ADD COLUMN IF NOT EXISTS deleted_at timestamptz NULL;

ALTER TABLE IF EXISTS wallet_payment_channels
    ADD COLUMN IF NOT EXISTS deleted_at timestamptz NULL;

DO $$
BEGIN
    IF to_regclass('ops_activities') IS NOT NULL THEN
        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_ops_activities_deleted_at ON ops_activities (deleted_at)';
    END IF;
    IF to_regclass('wallet_payment_channels') IS NOT NULL THEN
        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_wallet_payment_channels_deleted_at ON wallet_payment_channels (deleted_at)';
    END IF;
END $$;
