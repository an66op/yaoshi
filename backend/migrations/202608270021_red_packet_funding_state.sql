ALTER TABLE IF EXISTS chat_red_packets
    ADD COLUMN IF NOT EXISTS funding_status varchar(24) NOT NULL DEFAULT 'reserved';

UPDATE chat_red_packets
SET funding_status = CASE
    WHEN funding_user_id = 0 THEN 'legacy_unfunded'
    WHEN refunded_cents > 0 THEN 'refunded'
    WHEN remaining_cents = 0 THEN 'released'
    WHEN claimed_count > 0 THEN 'partially_released'
    ELSE 'reserved'
END;

CREATE INDEX IF NOT EXISTS idx_chat_red_packets_funding_status
    ON chat_red_packets (workspace_id, funding_status, created_at);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_chat_red_packet_funding_status'
          AND conrelid = 'chat_red_packets'::regclass
    ) THEN
        ALTER TABLE chat_red_packets
            ADD CONSTRAINT chk_chat_red_packet_funding_status
            CHECK (funding_status IN ('reserved', 'partially_released', 'released', 'refunded', 'legacy_unfunded'));
    END IF;
END $$;
