-- Link idempotent balance debits to the exact bet rows committed by the same
-- request. Including the reference in the dedupe key preserves legacy
-- aggregation for empty references while keeping distinct client requests
-- independently auditable and recoverable.
ALTER TABLE lottery_bets
    ADD COLUMN IF NOT EXISTS request_reference varchar(180) NOT NULL DEFAULT '';

DROP INDEX IF EXISTS idx_bet_dedupe;
CREATE UNIQUE INDEX idx_bet_dedupe
    ON lottery_bets (game_id, issue, room_scope, user_id, play_code, position, selection, request_reference);

DROP INDEX IF EXISTS idx_bet_request_evidence;
CREATE INDEX idx_bet_request_evidence
    ON lottery_bets (workspace_id, user_id, request_reference)
    WHERE request_reference <> '';
