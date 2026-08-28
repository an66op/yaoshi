-- Robot reset idempotency must outlive hot balance-ledger retention. Keep one
-- immutable receipt per workspace/request pair in a table that is not part of
-- the normal data-lifecycle archive policy.

CREATE TABLE IF NOT EXISTS workspace_robot_reset_receipts (
    id bigserial PRIMARY KEY,
    workspace_id bigint NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    request_id_hash char(64) NOT NULL,
    payload_hash char(32) NOT NULL,
    mode varchar(12) NOT NULL,
    robot_count integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_workspace_robot_reset_request_hash
        CHECK (request_id_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_workspace_robot_reset_payload_hash
        CHECK (payload_hash ~ '^[0-9a-f]{32}$'),
    CONSTRAINT ck_workspace_robot_reset_mode
        CHECK (mode IN ('random', 'custom')),
    CONSTRAINT ck_workspace_robot_reset_count
        CHECK (robot_count > 0),
    CONSTRAINT uq_workspace_robot_reset_request
        UNIQUE (workspace_id, request_id_hash)
);

CREATE INDEX IF NOT EXISTS idx_workspace_robot_reset_created
    ON workspace_robot_reset_receipts (workspace_id, created_at DESC);

CREATE OR REPLACE FUNCTION reject_robot_reset_receipt_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'robot reset receipts are immutable'
        USING ERRCODE = '55000';
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgrelid = 'workspace_robot_reset_receipts'::regclass
          AND tgname = 'trg_reject_robot_reset_receipt_mutation'
          AND NOT tgisinternal
    ) THEN
        CREATE TRIGGER trg_reject_robot_reset_receipt_mutation
        BEFORE UPDATE OR DELETE ON workspace_robot_reset_receipts
        FOR EACH ROW EXECUTE FUNCTION reject_robot_reset_receipt_mutation();
    END IF;
END $$;
