CREATE TABLE IF NOT EXISTS plan_publication_views (
    id bigserial PRIMARY KEY,
    workspace_id bigint NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    user_id bigint NOT NULL REFERENCES "user"(user_id) ON DELETE RESTRICT,
    game_id varchar(40) NOT NULL REFERENCES lottery_games(id) ON DELETE RESTRICT,
    issue varchar(64) NOT NULL,
    position integer NOT NULL CHECK(position BETWEEN 0 AND 10),
    plan_key varchar(48) NOT NULL,
    viewed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT idx_plan_publication_view UNIQUE(workspace_id,user_id,game_id,issue,position,plan_key)
);
CREATE INDEX IF NOT EXISTS idx_plan_publication_views_workspace_recent
    ON plan_publication_views(workspace_id,viewed_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS idx_plan_publication_views_user_recent
    ON plan_publication_views(user_id,viewed_at DESC,id DESC);

CREATE OR REPLACE FUNCTION reject_plan_publication_view_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'plan publication view receipts are immutable' USING ERRCODE = '55000';
END $$;
DROP TRIGGER IF EXISTS trg_reject_plan_publication_view_mutation ON plan_publication_views;
CREATE TRIGGER trg_reject_plan_publication_view_mutation
BEFORE UPDATE OR DELETE ON plan_publication_views
FOR EACH ROW EXECUTE FUNCTION reject_plan_publication_view_mutation();

-- A previous successful run may already have installed the guard. Drop it
-- before the narrowly scoped cleanup so retrying this migration is safe.
DROP TRIGGER IF EXISTS trg_reject_viewed_plan_recommendation_change ON plan_recommendations;

-- Remove only the exact automatic rows produced by the retired historical
-- backfill command. Genuine manual publications and ordinary automatic rows
-- are deliberately outside this predicate.
DELETE FROM plan_recommendations
WHERE source = 'demo'
  AND note = '系统补充的历史展示记录，不计入命中率。';

UPDATE plan_streams
SET cycle_id = 0, updated_at = clock_timestamp()
WHERE cycle_id IN (
    SELECT id FROM plan_stream_cycles
    WHERE payload_json LIKE '%"source":"demo"%'
      AND payload_json LIKE '%系统补充的历史展示记录，不计入命中率。%'
);
DELETE FROM plan_stream_periods
WHERE cycle_id IN (
    SELECT id FROM plan_stream_cycles
    WHERE payload_json LIKE '%"source":"demo"%'
      AND payload_json LIKE '%系统补充的历史展示记录，不计入命中率。%'
);
DELETE FROM plan_stream_cycles
WHERE payload_json LIKE '%"source":"demo"%'
  AND payload_json LIKE '%系统补充的历史展示记录，不计入命中率。%';

-- Historic administration clients could manually claim hit/miss. Preserve
-- every publication and pick, but discard that unaudited outcome metadata;
-- reads now derive the result from the immutable matching lottery draw.
UPDATE plan_recommendations
SET result = 'pending', updated_at = updated_at
WHERE source = 'manual' AND result IN ('hit', 'miss');

CREATE OR REPLACE FUNCTION reject_viewed_plan_recommendation_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM plan_publication_views AS viewed
        WHERE viewed.workspace_id = OLD.workspace_id
          AND viewed.game_id = OLD.game_id
          AND viewed.issue = OLD.issue
          AND viewed.position = 0
    ) OR EXISTS (
        SELECT 1 FROM lottery_draws AS draw
        WHERE draw.game_id = OLD.game_id AND draw.issue = OLD.issue
    ) OR EXISTS (
        SELECT 1 FROM lottery_issues AS issue
        WHERE issue.game_id = OLD.game_id AND issue.issue = OLD.issue
          AND issue.seal_at <= clock_timestamp()
    ) THEN
        RAISE EXCEPTION 'viewed plan publication is immutable' USING ERRCODE = '55000';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_reject_viewed_plan_recommendation_change
BEFORE UPDATE OR DELETE ON plan_recommendations
FOR EACH ROW EXECUTE FUNCTION reject_viewed_plan_recommendation_change();

CREATE OR REPLACE FUNCTION reject_plan_stream_payload_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.payload_json IS DISTINCT FROM OLD.payload_json THEN
        RAISE EXCEPTION 'published plan stream payload is immutable' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS trg_reject_plan_stream_payload_change ON plan_stream_cycles;
CREATE TRIGGER trg_reject_plan_stream_payload_change
BEFORE UPDATE OF payload_json ON plan_stream_cycles
FOR EACH ROW EXECUTE FUNCTION reject_plan_stream_payload_change();

-- Keep reset/restore safety in sync when this table is introduced after the
-- original guard installer migration.
SELECT public.install_application_truncate_guards();
