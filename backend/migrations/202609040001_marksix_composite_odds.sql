-- Mark Six 三中二 and 二中特 are one charged ticket with two mutually
-- exclusive payout tiers. Freeze the alternate tier and linked-market pricing
-- identity on the ticket; settlement must never read a later live quote.
ALTER TABLE lottery_bets
    ADD COLUMN IF NOT EXISTS odds_terms jsonb NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE lottery_bet_archives
    ADD COLUMN IF NOT EXISTS odds_terms jsonb NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE lottery_bets DROP CONSTRAINT IF EXISTS chk_bet_odds_terms_object;
ALTER TABLE lottery_bets ADD CONSTRAINT chk_bet_odds_terms_object
    CHECK (jsonb_typeof(odds_terms) = 'object');

ALTER TABLE lottery_bet_archives DROP CONSTRAINT IF EXISTS chk_bet_archive_odds_terms_object;
ALTER TABLE lottery_bet_archives ADD CONSTRAINT chk_bet_archive_odds_terms_object
    CHECK (jsonb_typeof(odds_terms) = 'object');

CREATE OR REPLACE FUNCTION guard_bet_odds_terms_snapshot() RETURNS trigger AS $$
BEGIN
    IF OLD.odds_terms IS DISTINCT FROM NEW.odds_terms THEN
        RAISE EXCEPTION 'bet odds terms are an immutable placement snapshot' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS guard_bet_odds_terms_snapshot ON lottery_bets;
CREATE TRIGGER guard_bet_odds_terms_snapshot
    BEFORE UPDATE OF odds_terms ON lottery_bets
    FOR EACH ROW EXECUTE FUNCTION guard_bet_odds_terms_snapshot();
