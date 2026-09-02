-- Ordered Taiwan Bingo products used a legacy sorted single-source feed.
-- Blank revisions deliberately identify those historic rows without rewriting
-- them. The importer upgrades only exact matches or rows proven to have no
-- financial evidence; settled legacy results remain isolated and immutable.
ALTER TABLE lottery_draws
    ADD COLUMN IF NOT EXISTS source_revision varchar(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS conversion_revision varchar(64) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_lottery_draw_revision
    ON lottery_draws (game_id, source_revision, conversion_revision);
