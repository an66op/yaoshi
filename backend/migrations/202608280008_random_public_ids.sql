-- Public account IDs are display-only identifiers. Keep every assigned value,
-- but stop exposing the insertion order for newly-created accounts.
--
-- The transaction-scoped advisory lock closes the race between checking a
-- candidate and the INSERT that consumes it: all callers using the default
-- wait for the preceding account transaction to commit before choosing. The
-- loop retries ordinary random collisions, while the unique index remains the
-- final database invariant for every insert path.
CREATE OR REPLACE FUNCTION public.next_member_public_id()
RETURNS bigint
LANGUAGE plpgsql
VOLATILE
SET search_path = pg_catalog, public
AS $$
DECLARE
    candidate bigint;
BEGIN
    PERFORM pg_advisory_xact_lock(24587624048118084);

    FOR attempt IN 1..256 LOOP
        candidate := 1000000 + floor(random() * 9000000)::bigint;
        IF NOT EXISTS (
            SELECT 1
            FROM public."user" AS account
            WHERE account.public_id = candidate
        ) THEN
            RETURN candidate;
        END IF;
    END LOOP;

    RAISE EXCEPTION 'unable to allocate a unique seven-digit member public ID after 256 attempts'
        USING ERRCODE = '54000';
END;
$$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_public_id
    ON public."user" (public_id);

ALTER TABLE public."user"
    ALTER COLUMN public_id SET DEFAULT public.next_member_public_id();
