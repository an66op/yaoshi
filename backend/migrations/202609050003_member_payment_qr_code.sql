ALTER TABLE public.member_payment_accounts
    ADD COLUMN IF NOT EXISTS qr_code_file character varying(64);

ALTER TABLE public.member_payment_accounts
    DROP CONSTRAINT IF EXISTS chk_member_payment_account_qr_code_file;

ALTER TABLE public.member_payment_accounts
    ADD CONSTRAINT chk_member_payment_account_qr_code_file
    CHECK (qr_code_file IS NULL OR qr_code_file ~ '^[0-9a-f]{32}\.png$');
