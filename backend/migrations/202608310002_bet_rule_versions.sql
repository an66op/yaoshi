-- Additive rule snapshots. Existing bets deliberately keep the empty legacy
-- version: no results, stakes, payouts or balances are recalculated here.
ALTER TABLE lottery_bets
    ADD COLUMN IF NOT EXISTS rule_version varchar(32) NOT NULL DEFAULT '';
ALTER TABLE lottery_bet_archives
    ADD COLUMN IF NOT EXISTS rule_version varchar(32) NOT NULL DEFAULT '';

-- A new-version stake must never accumulate into an older rule contract.
DROP INDEX IF EXISTS idx_bet_dedupe;
CREATE UNIQUE INDEX idx_bet_dedupe
    ON lottery_bets (game_id, issue, room_scope, user_id, play_code, position, selection, request_reference, rule_version);

DROP INDEX IF EXISTS idx_bet_archive_dedupe;
-- The immutable JSON snapshot already retains request references for rows
-- archived since request-level deduplication was introduced. Older snapshots
-- deliberately retain their legacy empty reference; do not rewrite them.
CREATE UNIQUE INDEX idx_bet_archive_dedupe
    ON lottery_bet_archives (room_scope, game_id, issue, user_id, play_code, position, selection,
                            (COALESCE(source_json ->> 'request_reference', '')), rule_version);

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
          AND COALESCE(archived.source_json ->> 'request_reference', '') = NEW.request_reference
          AND archived.rule_version = NEW.rule_version
    ) THEN
        RAISE EXCEPTION 'bet already exists in cold archive' USING ERRCODE = '23505';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION guard_bet_rule_snapshot() RETURNS trigger AS $$
BEGIN
    IF OLD.rule_version IS DISTINCT FROM NEW.rule_version THEN
        RAISE EXCEPTION 'bet rule version is an immutable placement snapshot' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS guard_bet_rule_snapshot ON lottery_bets;
CREATE TRIGGER guard_bet_rule_snapshot
    BEFORE UPDATE OF rule_version ON lottery_bets
    FOR EACH ROW EXECUTE FUNCTION guard_bet_rule_snapshot();
