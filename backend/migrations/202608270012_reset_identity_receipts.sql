-- Bind every future development reset receipt to the physical PostgreSQL
-- cluster and endpoint that was verified immediately before execution.

ALTER TABLE public.development_reset_receipts
    ADD COLUMN IF NOT EXISTS server_system_identifier varchar(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS server_address varchar(80) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS server_port integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS sentinel_token_sha256 char(64) NOT NULL DEFAULT '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_catalog.pg_constraint
        WHERE conname = 'ck_development_reset_receipt_server_port'
          AND conrelid = 'public.development_reset_receipts'::regclass
    ) THEN
        ALTER TABLE public.development_reset_receipts
            ADD CONSTRAINT ck_development_reset_receipt_server_port
            CHECK (server_port = 0 OR server_port BETWEEN 1 AND 65535) NOT VALID;
    END IF;
END $$;

ALTER TABLE public.development_reset_receipts
    VALIDATE CONSTRAINT ck_development_reset_receipt_server_port;
