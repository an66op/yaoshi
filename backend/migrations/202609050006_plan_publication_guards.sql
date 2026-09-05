-- Existing installations may already have applied the publication-view table
-- migration. Keep the stricter insert and room-cutoff boundaries in their own
-- forward-only version so a normal restart upgrades those databases too.
CREATE OR REPLACE FUNCTION lock_plan_publication_game(p_workspace_id bigint, p_game_id varchar)
RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    -- Serialize the first rendered snapshot with every publication mutation for
    -- one room/game. A trigger-only visibility check is insufficient when an
    -- INSERT and the first view receipt are still uncommitted concurrently.
    PERFORM pg_advisory_xact_lock(hashtextextended(
        p_workspace_id::text || ':' || length(p_game_id)::text || ':' || p_game_id,
        0
    ));
END $$;

CREATE OR REPLACE FUNCTION reject_locked_plan_recommendation_insert()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM lock_plan_publication_game(NEW.workspace_id, NEW.game_id);
    IF EXISTS (
        SELECT 1 FROM plan_publication_views AS viewed
        WHERE viewed.workspace_id = NEW.workspace_id
          AND viewed.game_id = NEW.game_id
          AND viewed.issue = NEW.issue
          AND viewed.position = 0
    ) OR EXISTS (
        SELECT 1 FROM lottery_draws AS draw
        WHERE draw.game_id = NEW.game_id AND draw.issue = NEW.issue
    ) OR EXISTS (
        SELECT 1 FROM lottery_issues AS issue
        WHERE issue.game_id = NEW.game_id AND issue.issue = NEW.issue
          AND issue.seal_at <= clock_timestamp()
    ) OR EXISTS (
        SELECT 1 FROM lottery_issue_windows AS room_window
        WHERE room_window.workspace_id = NEW.workspace_id
          AND room_window.game_id = NEW.game_id AND room_window.issue = NEW.issue
          AND room_window.seal_at <= clock_timestamp()
    ) THEN
        RAISE EXCEPTION 'plan publication insert is locked' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS trg_reject_locked_plan_recommendation_insert ON plan_recommendations;
CREATE TRIGGER trg_reject_locked_plan_recommendation_insert
BEFORE INSERT ON plan_recommendations
FOR EACH ROW EXECUTE FUNCTION reject_locked_plan_recommendation_insert();

CREATE OR REPLACE FUNCTION reject_viewed_plan_recommendation_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM lock_plan_publication_game(OLD.workspace_id, OLD.game_id);
    IF TG_OP = 'UPDATE' AND (
        NEW.workspace_id IS DISTINCT FROM OLD.workspace_id
        OR NEW.game_id IS DISTINCT FROM OLD.game_id
        OR NEW.issue IS DISTINCT FROM OLD.issue
    ) THEN
        RAISE EXCEPTION 'plan publication identity is immutable' USING ERRCODE = '55000';
    END IF;
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
    ) OR EXISTS (
        SELECT 1 FROM lottery_issue_windows AS room_window
        WHERE room_window.workspace_id = OLD.workspace_id
          AND room_window.game_id = OLD.game_id AND room_window.issue = OLD.issue
          AND room_window.seal_at <= clock_timestamp()
    ) THEN
        RAISE EXCEPTION 'viewed plan publication is immutable' USING ERRCODE = '55000';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END $$;
