-- Give compact-input idempotency reservations an explicit terminal state.
-- This lets the recovery worker close abandoned requests without inventing a
-- successful receipt or allowing a potentially charged request to run twice.
ALTER TABLE lottery_assistant_requests
    ADD COLUMN IF NOT EXISTS status varchar(20) NOT NULL DEFAULT 'processing',
    ADD COLUMN IF NOT EXISTS last_error varchar(500);

UPDATE lottery_assistant_requests
SET status = 'completed'
WHERE COALESCE(result_json, '') <> ''
  AND status <> 'completed';

UPDATE lottery_assistant_requests
SET status = 'processing'
WHERE COALESCE(result_json, '') = ''
  AND status NOT IN ('processing', 'failed');

CREATE INDEX IF NOT EXISTS idx_lottery_assistant_requests_status
    ON lottery_assistant_requests(status);

CREATE INDEX IF NOT EXISTS idx_lottery_assistant_requests_recovery
    ON lottery_assistant_requests(status, updated_at, id);

CREATE INDEX IF NOT EXISTS idx_lottery_bet_requests_recovery
    ON lottery_bet_requests(status, updated_at, id);
