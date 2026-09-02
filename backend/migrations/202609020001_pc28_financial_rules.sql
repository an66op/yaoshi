-- PC28 settlement evidence is additive and deliberately separate from the
-- quoted odds and actual stake. NULL retains the legacy meaning for rows that
-- predate this migration; report queries use COALESCE only for those rows.
ALTER TABLE lottery_bets
    ADD COLUMN IF NOT EXISTS valid_turnover_cents bigint NULL,
    ADD COLUMN IF NOT EXISTS settlement_odds numeric NULL,
    ADD COLUMN IF NOT EXISTS user_issue_stake_cents_snapshot bigint NULL,
    ADD COLUMN IF NOT EXISTS settlement_policy varchar(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pc28_gray_push boolean NOT NULL DEFAULT false;

ALTER TABLE lottery_bet_archives
    ADD COLUMN IF NOT EXISTS valid_turnover_cents bigint NULL,
    ADD COLUMN IF NOT EXISTS settlement_odds numeric NULL,
    ADD COLUMN IF NOT EXISTS user_issue_stake_cents_snapshot bigint NULL,
    ADD COLUMN IF NOT EXISTS settlement_policy varchar(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pc28_gray_push boolean NOT NULL DEFAULT false;

ALTER TABLE lottery_bets DROP CONSTRAINT IF EXISTS chk_bet_valid_turnover;
ALTER TABLE lottery_bets ADD CONSTRAINT chk_bet_valid_turnover
    CHECK (valid_turnover_cents IS NULL OR (valid_turnover_cents >= 0 AND valid_turnover_cents <= amount_cents));
ALTER TABLE lottery_bets DROP CONSTRAINT IF EXISTS chk_bet_settlement_odds;
ALTER TABLE lottery_bets ADD CONSTRAINT chk_bet_settlement_odds
    CHECK (settlement_odds IS NULL OR settlement_odds >= 0);
ALTER TABLE lottery_bets DROP CONSTRAINT IF EXISTS chk_bet_user_issue_stake;
ALTER TABLE lottery_bets ADD CONSTRAINT chk_bet_user_issue_stake
    CHECK (user_issue_stake_cents_snapshot IS NULL OR user_issue_stake_cents_snapshot >= amount_cents);

CREATE OR REPLACE FUNCTION guard_bet_pc28_placement_snapshot() RETURNS trigger AS $$
BEGIN
    IF OLD.pc28_gray_push IS DISTINCT FROM NEW.pc28_gray_push THEN
        RAISE EXCEPTION 'PC28 gray push setting is an immutable placement snapshot' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS guard_bet_pc28_placement_snapshot ON lottery_bets;
CREATE TRIGGER guard_bet_pc28_placement_snapshot
    BEFORE UPDATE OF pc28_gray_push ON lottery_bets
    FOR EACH ROW EXECUTE FUNCTION guard_bet_pc28_placement_snapshot();
