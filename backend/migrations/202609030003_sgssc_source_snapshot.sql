-- An empty value deliberately preserves the unknown/legacy draw contract.
-- Only new verified SG placements may record a current source revision.
ALTER TABLE lottery_bets
    ADD COLUMN IF NOT EXISTS draw_source_revision varchar(64) NOT NULL DEFAULT '';

ALTER TABLE lottery_bet_archives
    ADD COLUMN IF NOT EXISTS draw_source_revision varchar(64) NOT NULL DEFAULT '';

-- Source verification looks up periods across rooms, including archive-only
-- legacy tickets. The room-prefixed dedupe index cannot serve this lookup.
CREATE INDEX IF NOT EXISTS idx_bet_archive_game_issue
    ON lottery_bet_archives (game_id, issue);
