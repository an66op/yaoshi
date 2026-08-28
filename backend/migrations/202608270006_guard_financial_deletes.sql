-- Financial hot tables are physically pruned only by the verified lifecycle
-- archive transaction. Ordinary SQL, accidental ORM changes and maintenance
-- sessions must fail closed instead of silently removing accounting evidence.

CREATE OR REPLACE FUNCTION reject_unguarded_lifecycle_delete() RETURNS trigger AS $$
BEGIN
    IF COALESCE(current_setting('wangzhe.lifecycle_delete', true), '') <> 'on' THEN
        RAISE EXCEPTION 'hard DELETE is disabled for lifecycle table %', TG_TABLE_NAME
            USING ERRCODE = '55000';
    END IF;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

DO $$
DECLARE
    guarded_table text;
    guarded_relation regclass;
BEGIN
    FOREACH guarded_table IN ARRAY ARRAY[
        'lottery_bets',
        'user_balance_transactions',
        'admin_audit_logs',
        'lottery_bet_archives',
        'user_balance_transaction_archives'
    ] LOOP
        guarded_relation := to_regclass(format('%I', guarded_table));
        IF guarded_relation IS NOT NULL AND NOT EXISTS (
            SELECT 1
            FROM pg_trigger
            WHERE tgrelid = guarded_relation
              AND tgname = 'trg_reject_unguarded_lifecycle_delete'
              AND NOT tgisinternal
        ) THEN
            EXECUTE format(
                'CREATE TRIGGER trg_reject_unguarded_lifecycle_delete '
                'BEFORE DELETE ON %I FOR EACH ROW '
                'EXECUTE FUNCTION reject_unguarded_lifecycle_delete()',
                guarded_table
            );
        END IF;
    END LOOP;
END $$;

-- Audit archives are permanent evidence and have no restore workflow.
DO $$
DECLARE
    archive_relation regclass := to_regclass('admin_audit_log_archives');
BEGIN
    IF archive_relation IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgrelid = archive_relation
          AND tgname = 'trg_reject_protected_hard_delete'
          AND NOT tgisinternal
    ) THEN
        CREATE TRIGGER trg_reject_protected_hard_delete
        BEFORE DELETE ON admin_audit_log_archives
        FOR EACH ROW EXECUTE FUNCTION reject_protected_hard_delete();
    END IF;
END $$;
