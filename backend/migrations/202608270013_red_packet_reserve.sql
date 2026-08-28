-- Room red packets are funded before publication. Unclaimed points remain a
-- recorded liability and are returned to the room owner when closed/expired.
ALTER TABLE IF EXISTS chat_red_packets
    ADD COLUMN IF NOT EXISTS funding_user_id bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS refunded_cents bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS expires_at timestamptz,
    ADD COLUMN IF NOT EXISTS closed_at timestamptz,
    ADD COLUMN IF NOT EXISTS closed_by varchar(80) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS close_reason varchar(240) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_chat_red_packets_funding_user
    ON chat_red_packets (funding_user_id);
CREATE INDEX IF NOT EXISTS idx_chat_red_packets_expiry
    ON chat_red_packets (status, expires_at)
    WHERE status = 'active' AND expires_at IS NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_chat_red_packet_amounts'
          AND conrelid = 'chat_red_packets'::regclass
    ) THEN
        ALTER TABLE chat_red_packets
            ADD CONSTRAINT chk_chat_red_packet_amounts
            CHECK (
                total_cents > 0
                AND remaining_cents >= 0
                AND refunded_cents >= 0
                AND remaining_cents + refunded_cents <= total_cents
            );
    END IF;
END $$;

-- Historic envelopes were created without a funding debit. They remain
-- readable/claimable for compatibility, but receive an expiry so they cannot
-- stay as an unbounded liability forever. New sends always set a funding user.
UPDATE chat_red_packets
SET expires_at = created_at + interval '24 hours'
WHERE expires_at IS NULL AND status = 'active';
