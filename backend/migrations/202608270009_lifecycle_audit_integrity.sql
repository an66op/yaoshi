-- Lifecycle receipts and cold archives are audit evidence. Keep the run's
-- state machine writable only for its documented transitions, while making
-- identity, frozen criteria and completed receipts tamper resistant.

ALTER TABLE data_cleanup_runs
    ADD COLUMN IF NOT EXISTS executed_by_id bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS executed_by_name varchar(80) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS soft_restored_by_id bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS soft_restored_by_name varchar(80) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS financial_restored_by_id bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS financial_restored_by_name varchar(80) NOT NULL DEFAULT '';

CREATE OR REPLACE FUNCTION lifecycle_guard_cleanup_run_update() RETURNS trigger AS $$
BEGIN
    IF NEW.request_id IS DISTINCT FROM OLD.request_id
       OR NEW.workspace_id IS DISTINCT FROM OLD.workspace_id
       OR NEW.all_workspaces IS DISTINCT FROM OLD.all_workspaces
       OR NEW.actor_id IS DISTINCT FROM OLD.actor_id
       OR NEW.actor_name IS DISTINCT FROM OLD.actor_name
       OR NEW.batch_limit IS DISTINCT FROM OLD.batch_limit
       OR NEW.criteria_json IS DISTINCT FROM OLD.criteria_json
       OR NEW.preview_json IS DISTINCT FROM OLD.preview_json
       OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'cleanup run identity and frozen preview are immutable'
            USING ERRCODE = '55000';
    END IF;

    IF NEW.status IS DISTINCT FROM OLD.status AND NOT (
        (OLD.status = 'previewed' AND NEW.status IN ('running', 'failed'))
        OR (OLD.status = 'running' AND NEW.status = 'completed')
    ) THEN
        RAISE EXCEPTION 'invalid cleanup run transition % -> %', OLD.status, NEW.status
            USING ERRCODE = '55000';
    END IF;

    IF OLD.executed_by_id <> 0 AND (
        NEW.executed_by_id IS DISTINCT FROM OLD.executed_by_id
        OR NEW.executed_by_name IS DISTINCT FROM OLD.executed_by_name
    ) THEN
        RAISE EXCEPTION 'cleanup executor is immutable' USING ERRCODE = '55000';
    END IF;
    IF OLD.executed_by_id = 0 AND (
        NEW.executed_by_id IS DISTINCT FROM OLD.executed_by_id
        OR NEW.executed_by_name IS DISTINCT FROM OLD.executed_by_name
    ) THEN
        IF NEW.executed_by_id = 0 OR NEW.executed_by_name = '' OR NOT (
            OLD.status = 'previewed' AND NEW.status IN ('running', 'failed')
        ) THEN
            RAISE EXCEPTION 'cleanup executor may only be recorded when execution starts'
                USING ERRCODE = '55000';
        END IF;
    END IF;

    IF OLD.started_at IS NOT NULL AND NEW.started_at IS DISTINCT FROM OLD.started_at THEN
        RAISE EXCEPTION 'cleanup start time is immutable' USING ERRCODE = '55000';
    END IF;
    IF OLD.started_at IS NULL AND NEW.started_at IS NOT NULL AND NOT (
        OLD.status = 'previewed' AND NEW.status IN ('running', 'failed')
    ) THEN
        RAISE EXCEPTION 'cleanup start time has an invalid transition' USING ERRCODE = '55000';
    END IF;
    IF OLD.completed_at IS NOT NULL AND NEW.completed_at IS DISTINCT FROM OLD.completed_at THEN
        RAISE EXCEPTION 'cleanup completion time is immutable' USING ERRCODE = '55000';
    END IF;
    IF OLD.completed_at IS NULL AND NEW.completed_at IS NOT NULL AND NOT (
        OLD.status = 'running' AND NEW.status = 'completed'
    ) THEN
        RAISE EXCEPTION 'cleanup completion time has an invalid transition' USING ERRCODE = '55000';
    END IF;
    IF NEW.result_json IS DISTINCT FROM OLD.result_json AND NOT (
        OLD.status = 'running' AND NEW.status = 'completed'
    ) THEN
        RAISE EXCEPTION 'cleanup result may only be written on completion'
            USING ERRCODE = '55000';
    END IF;
    IF NEW.last_error IS DISTINCT FROM OLD.last_error AND NOT (
        (OLD.status = 'previewed' AND NEW.status = 'failed')
        OR (OLD.status = 'running' AND NEW.status = 'completed')
    ) THEN
        RAISE EXCEPTION 'cleanup error receipt has an invalid transition'
            USING ERRCODE = '55000';
    END IF;

    IF OLD.soft_restored_at IS NOT NULL AND (
        NEW.soft_restored_at IS DISTINCT FROM OLD.soft_restored_at
        OR NEW.soft_restored_by_id IS DISTINCT FROM OLD.soft_restored_by_id
        OR NEW.soft_restored_by_name IS DISTINCT FROM OLD.soft_restored_by_name
        OR NEW.soft_restore_result_json IS DISTINCT FROM OLD.soft_restore_result_json
    ) THEN
        RAISE EXCEPTION 'soft-delete restore receipt is immutable' USING ERRCODE = '55000';
    END IF;
    IF OLD.soft_restored_at IS NULL AND NEW.soft_restored_at IS NOT NULL AND (
        OLD.status <> 'completed' OR NEW.status <> 'completed'
        OR NEW.soft_restored_by_id = 0 OR NEW.soft_restored_by_name = ''
    ) THEN
        RAISE EXCEPTION 'soft-delete restore receipt is incomplete' USING ERRCODE = '55000';
    END IF;
    IF OLD.soft_restored_at IS NULL AND NEW.soft_restored_at IS NULL AND (
        NEW.soft_restored_by_id IS DISTINCT FROM OLD.soft_restored_by_id
        OR NEW.soft_restored_by_name IS DISTINCT FROM OLD.soft_restored_by_name
        OR NEW.soft_restore_result_json IS DISTINCT FROM OLD.soft_restore_result_json
    ) THEN
        RAISE EXCEPTION 'soft-delete restore receipt cannot be staged' USING ERRCODE = '55000';
    END IF;

    IF OLD.financial_restored_at IS NOT NULL AND (
        NEW.financial_restored_at IS DISTINCT FROM OLD.financial_restored_at
        OR NEW.financial_restored_by_id IS DISTINCT FROM OLD.financial_restored_by_id
        OR NEW.financial_restored_by_name IS DISTINCT FROM OLD.financial_restored_by_name
        OR NEW.financial_restore_result_json IS DISTINCT FROM OLD.financial_restore_result_json
    ) THEN
        RAISE EXCEPTION 'financial restore receipt is immutable' USING ERRCODE = '55000';
    END IF;
    IF OLD.financial_restored_at IS NULL AND NEW.financial_restored_at IS NOT NULL AND (
        OLD.status <> 'completed' OR NEW.status <> 'completed'
        OR NEW.financial_restored_by_id = 0 OR NEW.financial_restored_by_name = ''
    ) THEN
        RAISE EXCEPTION 'financial restore receipt is incomplete' USING ERRCODE = '55000';
    END IF;
    IF OLD.financial_restored_at IS NULL AND NEW.financial_restored_at IS NULL AND (
        NEW.financial_restored_by_id IS DISTINCT FROM OLD.financial_restored_by_id
        OR NEW.financial_restored_by_name IS DISTINCT FROM OLD.financial_restored_by_name
        OR NEW.financial_restore_result_json IS DISTINCT FROM OLD.financial_restore_result_json
    ) THEN
        RAISE EXCEPTION 'financial restore receipt cannot be staged' USING ERRCODE = '55000';
    END IF;

    IF NEW.restored_by_id IS DISTINCT FROM OLD.restored_by_id THEN
        RAISE EXCEPTION 'legacy restore actor field is immutable' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION reject_lifecycle_receipt_delete() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'lifecycle receipt % is immutable', TG_TABLE_NAME
        USING ERRCODE = '55000';
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION reject_lifecycle_archive_update() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'lifecycle archive % is immutable', TG_TABLE_NAME
        USING ERRCODE = '55000';
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION protect_service_welcome_from_soft_delete() RETURNS trigger AS $$
BEGIN
    IF OLD.message_type = 'welcome'
       AND OLD.deleted_at IS NULL
       AND NEW.deleted_at IS NOT NULL THEN
        RAISE EXCEPTION 'durable service welcome cannot be soft deleted'
            USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
DECLARE
    archive_table text;
    archive_relation regclass;
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgrelid = 'data_cleanup_runs'::regclass
          AND tgname = 'trg_lifecycle_guard_cleanup_run_update'
          AND NOT tgisinternal
    ) THEN
        CREATE TRIGGER trg_lifecycle_guard_cleanup_run_update
        BEFORE UPDATE ON data_cleanup_runs
        FOR EACH ROW EXECUTE FUNCTION lifecycle_guard_cleanup_run_update();
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgrelid = 'data_cleanup_runs'::regclass
          AND tgname = 'trg_reject_lifecycle_receipt_delete'
          AND NOT tgisinternal
    ) THEN
        CREATE TRIGGER trg_reject_lifecycle_receipt_delete
        BEFORE DELETE ON data_cleanup_runs
        FOR EACH ROW EXECUTE FUNCTION reject_lifecycle_receipt_delete();
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgrelid = 'member_chat_messages'::regclass
          AND tgname = 'trg_protect_service_welcome_from_soft_delete'
          AND NOT tgisinternal
    ) THEN
        CREATE TRIGGER trg_protect_service_welcome_from_soft_delete
        BEFORE UPDATE OF deleted_at ON member_chat_messages
        FOR EACH ROW EXECUTE FUNCTION protect_service_welcome_from_soft_delete();
    END IF;

    FOREACH archive_table IN ARRAY ARRAY[
        'admin_audit_log_archives',
        'lottery_bet_archives',
        'user_balance_transaction_archives'
    ] LOOP
        archive_relation := to_regclass(format('%I', archive_table));
        IF archive_relation IS NOT NULL AND NOT EXISTS (
            SELECT 1 FROM pg_trigger
            WHERE tgrelid = archive_relation
              AND tgname = 'trg_reject_lifecycle_archive_update'
              AND NOT tgisinternal
        ) THEN
            EXECUTE format(
                'CREATE TRIGGER trg_reject_lifecycle_archive_update '
                'BEFORE UPDATE ON %I FOR EACH ROW '
                'EXECUTE FUNCTION reject_lifecycle_archive_update()',
                archive_table
            );
        END IF;
    END LOOP;
END $$;
