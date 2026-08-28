-- Bind every JWT to the credential state that issued it. Password changes
-- increment auth_version, invalidating all tokens carrying the old version.

ALTER TABLE IF EXISTS "user"
    ADD COLUMN IF NOT EXISTS auth_version bigint NOT NULL DEFAULT 1;

DO $$
BEGIN
    IF to_regclass('"user"') IS NOT NULL THEN
        UPDATE "user"
        SET auth_version = 1
        WHERE auth_version IS NULL OR auth_version < 1;
    END IF;
END $$;

DO $$
BEGIN
    IF to_regclass('"user"') IS NOT NULL
       AND NOT EXISTS (
           SELECT 1
           FROM pg_constraint
           WHERE conname = 'chk_user_auth_version_positive'
             AND conrelid = '"user"'::regclass
       ) THEN
        ALTER TABLE "user"
            ADD CONSTRAINT chk_user_auth_version_positive CHECK (auth_version > 0);
    END IF;
END $$;
