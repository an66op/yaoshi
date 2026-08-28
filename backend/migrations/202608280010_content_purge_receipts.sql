-- A hard purge can consume rows created by one or more earlier soft-delete
-- runs. Persist that relationship on the source receipt so the UI and restore
-- endpoint never claim that a partially or fully purged batch is recoverable.
ALTER TABLE data_cleanup_runs
    ADD COLUMN IF NOT EXISTS content_purged_at timestamptz NULL,
    ADD COLUMN IF NOT EXISTS content_purge_count bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_content_purge_request_id varchar(96) NOT NULL DEFAULT '';

CREATE OR REPLACE FUNCTION guard_cleanup_content_purge_receipt() RETURNS trigger AS $$
BEGIN
    IF NEW.content_purged_at IS NOT DISTINCT FROM OLD.content_purged_at
       AND NEW.content_purge_count IS NOT DISTINCT FROM OLD.content_purge_count
       AND NEW.last_content_purge_request_id IS NOT DISTINCT FROM OLD.last_content_purge_request_id THEN
        RETURN NEW;
    END IF;

    IF current_setting('wangzhe.lifecycle_content_purge', true) IS DISTINCT FROM 'on' THEN
        RAISE EXCEPTION 'content purge receipt may only be written by lifecycle hard purge'
            USING ERRCODE = '55000';
    END IF;
    IF OLD.status <> 'completed' OR NEW.status <> 'completed'
       OR NEW.content_purge_count <= OLD.content_purge_count
       OR NEW.last_content_purge_request_id = '' THEN
        RAISE EXCEPTION 'content purge receipt is incomplete'
            USING ERRCODE = '55000';
    END IF;
    IF OLD.content_purged_at IS NULL AND NEW.content_purged_at IS NULL THEN
        RAISE EXCEPTION 'content purge receipt requires a timestamp'
            USING ERRCODE = '55000';
    END IF;
    IF OLD.content_purged_at IS NOT NULL
       AND NEW.content_purged_at IS DISTINCT FROM OLD.content_purged_at THEN
        RAISE EXCEPTION 'first content purge timestamp is immutable'
            USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_guard_cleanup_content_purge_receipt ON data_cleanup_runs;
CREATE TRIGGER trg_guard_cleanup_content_purge_receipt
BEFORE UPDATE ON data_cleanup_runs
FOR EACH ROW EXECUTE FUNCTION guard_cleanup_content_purge_receipt();

COMMENT ON COLUMN data_cleanup_runs.content_purge_count IS
    'Number of this soft-delete run rows permanently removed by later hard-purge receipts.';
COMMENT ON COLUMN data_cleanup_runs.last_content_purge_request_id IS
    'Most recent hard-purge request that consumed rows from this soft-delete run.';
