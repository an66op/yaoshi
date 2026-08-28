ALTER TABLE lottery_assistant_requests
    ADD COLUMN IF NOT EXISTS payload_hash varchar(64) NOT NULL DEFAULT '';

ALTER TABLE lottery_bet_requests
    ADD COLUMN IF NOT EXISTS payload_hash varchar(64) NOT NULL DEFAULT '';

ALTER TABLE lottery_assistant_requests
    DROP CONSTRAINT IF EXISTS ck_lottery_assistant_request_payload_hash;
ALTER TABLE lottery_assistant_requests
    ADD CONSTRAINT ck_lottery_assistant_request_payload_hash
    CHECK (payload_hash = '' OR payload_hash ~ '^[0-9a-f]{64}$');

ALTER TABLE lottery_bet_requests
    DROP CONSTRAINT IF EXISTS ck_lottery_bet_request_payload_hash;
ALTER TABLE lottery_bet_requests
    ADD CONSTRAINT ck_lottery_bet_request_payload_hash
    CHECK (payload_hash = '' OR payload_hash ~ '^[0-9a-f]{64}$');

COMMENT ON COLUMN lottery_assistant_requests.payload_hash IS
    'Canonical business payload hash; the same request id cannot be reused for different content.';
COMMENT ON COLUMN lottery_bet_requests.payload_hash IS
    'Canonical business payload hash; the same request id cannot be reused for different content.';
