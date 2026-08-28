-- TRUNCATE bypasses row-level DELETE triggers. Protect every application table
-- from an accidental console/ORM truncate and reserve a narrowly scoped,
-- transaction-local override for the offline development reset utility.

CREATE TABLE IF NOT EXISTS development_reset_receipts (
    request_id varchar(96) PRIMARY KEY,
    database_name varchar(120) NOT NULL,
    backup_filename varchar(255) NOT NULL,
    backup_sha256 char(64) NOT NULL,
    executed_by varchar(120) NOT NULL,
    reset_scope varchar(40) NOT NULL DEFAULT 'business_data',
    cleared_tables text[] NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_development_reset_receipt_sha256
        CHECK (backup_sha256 ~ '^[0-9a-f]{64}$')
);

CREATE OR REPLACE FUNCTION reject_unapproved_application_truncate() RETURNS trigger AS $$
DECLARE
    expected_token text := 'confirmed:' || current_database();
BEGIN
    IF COALESCE(current_setting('wangzhe.dev_reset', true), '') <> expected_token THEN
        RAISE EXCEPTION 'TRUNCATE is disabled for application table %', TG_TABLE_NAME
            USING ERRCODE = '55000';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION reject_development_reset_receipt_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'development reset receipts are immutable'
        USING ERRCODE = '55000';
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION install_application_truncate_guards() RETURNS void AS $$
DECLARE
    guarded record;
BEGIN
    FOR guarded IN
        SELECT namespace.nspname AS schema_name, relation.relname AS table_name,
               relation.oid AS relation_id
        FROM pg_class relation
        JOIN pg_namespace namespace ON namespace.oid = relation.relnamespace
        WHERE namespace.nspname = 'public'
          AND relation.relkind IN ('r', 'p')
          AND relation.relname <> 'development_reset_receipts'
    LOOP
        IF NOT EXISTS (
            SELECT 1 FROM pg_trigger
            WHERE tgrelid = guarded.relation_id
              AND tgname = 'trg_reject_unapproved_application_truncate'
              AND NOT tgisinternal
        ) THEN
            EXECUTE format(
                'CREATE TRIGGER trg_reject_unapproved_application_truncate '
                'BEFORE TRUNCATE ON %I.%I FOR EACH STATEMENT '
                'EXECUTE FUNCTION reject_unapproved_application_truncate()',
                guarded.schema_name, guarded.table_name
            );
        END IF;
    END LOOP;

    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgrelid = 'development_reset_receipts'::regclass
          AND tgname = 'trg_reject_development_reset_receipt_update'
          AND NOT tgisinternal
    ) THEN
        CREATE TRIGGER trg_reject_development_reset_receipt_update
        BEFORE UPDATE OR DELETE ON development_reset_receipts
        FOR EACH ROW EXECUTE FUNCTION reject_development_reset_receipt_mutation();
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgrelid = 'development_reset_receipts'::regclass
          AND tgname = 'trg_reject_development_reset_receipt_truncate'
          AND NOT tgisinternal
    ) THEN
        CREATE TRIGGER trg_reject_development_reset_receipt_truncate
        BEFORE TRUNCATE ON development_reset_receipts
        FOR EACH STATEMENT EXECUTE FUNCTION reject_development_reset_receipt_mutation();
    END IF;
END;
$$ LANGUAGE plpgsql;

SELECT install_application_truncate_guards();
