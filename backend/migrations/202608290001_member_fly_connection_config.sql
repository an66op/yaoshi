-- Keep per-member fly-order policy on the existing user record. These fields
-- are preparation metadata only: this release has no external order connector
-- and therefore never persists or reports a connected state.
ALTER TABLE "user"
    ADD COLUMN IF NOT EXISTS fly_target_platform varchar(80) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS fly_target_account varchar(120) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS fly_endpoint_label varchar(160) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS fly_single_limit_cents bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS fly_daily_limit_cents bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS fly_connection_remark varchar(500) NOT NULL DEFAULT '';

ALTER TABLE "user"
    DROP CONSTRAINT IF EXISTS ck_user_fly_external_limits;
ALTER TABLE "user"
    ADD CONSTRAINT ck_user_fly_external_limits CHECK (
        fly_single_limit_cents >= 0
        AND fly_daily_limit_cents >= 0
        AND (
            fly_single_limit_cents = 0
            OR fly_daily_limit_cents = 0
            OR fly_daily_limit_cents >= fly_single_limit_cents
        )
    );

COMMENT ON COLUMN "user".fly_target_platform IS
    'Non-secret external follow platform label. Configuration only; no connector is installed.';
COMMENT ON COLUMN "user".fly_target_account IS
    'Non-secret external account identifier. Passwords, tokens and API keys must not be stored here.';
COMMENT ON COLUMN "user".fly_endpoint_label IS
    'Human-readable endpoint identifier, not a credential-bearing URL.';
COMMENT ON COLUMN "user".fly_single_limit_cents IS
    'Prepared external-follow per-order cap in cents; zero means unspecified.';
COMMENT ON COLUMN "user".fly_daily_limit_cents IS
    'Prepared external-follow daily cap in cents; zero means unspecified.';
COMMENT ON COLUMN "user".fly_connection_remark IS
    'Operator-only preparation note; external connectivity is always not_connected in this release.';
