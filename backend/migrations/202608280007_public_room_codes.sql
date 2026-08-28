-- Public room numbers are user-facing identifiers.  Internal ownership and
-- every historic bet/chat/ledger row remain keyed by immutable workspace_id;
-- changing a public number therefore updates identity shadows only.

DO $$
DECLARE
    room RECORD;
    account RECORD;
    next_code text;
BEGIN
    -- Normalize existing tenant/agent workspaces first.  The current local
    -- experience room has an intentional, stable upgrade path: 8801 -> 88001.
    FOR room IN
        SELECT id, owner_user_id, type, room_code
        FROM public.workspaces
        WHERE type IN ('tenant', 'agent')
          AND (room_code IS NULL OR room_code !~ '^[0-9]{5,12}$')
        ORDER BY id
    LOOP
        next_code := NULL;
        IF room.room_code = '8801'
           AND NOT EXISTS (
                SELECT 1 FROM public.workspaces
                WHERE room_code = '88001' AND id <> room.id
           )
           AND NOT EXISTS (
                SELECT 1 FROM public."user"
                WHERE agent_room_code = '88001'
                  AND user_id <> room.owner_user_id
                  AND deleted_at IS NULL
           )
           AND NOT EXISTS (
                SELECT 1 FROM public.special_number_resources
                WHERE number = '88001'
           ) THEN
            next_code := '88001';
        ELSE
            SELECT candidate::text
            INTO next_code
            FROM generate_series(10000, 999999) AS candidate
            WHERE NOT EXISTS (
                    SELECT 1 FROM public.workspaces
                    WHERE room_code = candidate::text
                )
              AND NOT EXISTS (
                    SELECT 1 FROM public."user"
                    WHERE agent_room_code = candidate::text
                      AND deleted_at IS NULL
                )
              AND NOT EXISTS (
                    SELECT 1 FROM public.special_number_resources
                    WHERE number = candidate::text
                )
            ORDER BY candidate
            LIMIT 1;
        END IF;

        IF next_code IS NULL THEN
            RAISE EXCEPTION 'no valid public room code is available for workspace %', room.id;
        END IF;

        UPDATE public.workspaces
        SET room_code = next_code,
            updated_at = CURRENT_TIMESTAMP
        WHERE id = room.id;

        IF room.type = 'agent' THEN
			-- A granted vanity-number row is the agent room's current identity,
			-- not historic business evidence. Move that compatibility shadow with
			-- the room; immutable grant records keep their original snapshot.
			UPDATE public.special_number_resources
			SET number = next_code,
				updated_at = CURRENT_TIMESTAMP
			WHERE number IS NOT DISTINCT FROM room.room_code
			  AND owner_user_id = room.owner_user_id
			  AND status = 'granted';

            UPDATE public."user"
            SET agent_room_code = next_code,
                updated_at = CURRENT_TIMESTAMP
            WHERE user_id = room.owner_user_id
              AND role = 'agent';
        END IF;
    END LOOP;

    -- Workspace is authoritative.  Heal any stale legacy agent shadow before
    -- allocating codes for pre-workspace agent accounts.
    UPDATE public."user" AS account_row
    SET agent_room_code = workspace.room_code,
        updated_at = CURRENT_TIMESTAMP
    FROM public.workspaces AS workspace
    WHERE workspace.owner_user_id = account_row.user_id
      AND workspace.type = 'agent'
      AND account_row.role = 'agent'
      AND account_row.agent_room_code IS DISTINCT FROM workspace.room_code;

    FOR account IN
        SELECT user_id, agent_room_code
        FROM public."user"
        WHERE role = 'agent'
          AND (agent_room_code IS NULL OR agent_room_code !~ '^[0-9]{5,12}$')
        ORDER BY user_id
    LOOP
        next_code := NULL;
        IF account.agent_room_code = '8801'
           AND NOT EXISTS (SELECT 1 FROM public.workspaces WHERE room_code = '88001')
           AND NOT EXISTS (
                SELECT 1 FROM public."user"
                WHERE agent_room_code = '88001'
                  AND user_id <> account.user_id
                  AND deleted_at IS NULL
           )
           AND NOT EXISTS (
                SELECT 1 FROM public.special_number_resources
                WHERE number = '88001'
           ) THEN
            next_code := '88001';
        ELSE
            SELECT candidate::text
            INTO next_code
            FROM generate_series(10000, 999999) AS candidate
            WHERE NOT EXISTS (
                    SELECT 1 FROM public.workspaces
                    WHERE room_code = candidate::text
                )
              AND NOT EXISTS (
                    SELECT 1 FROM public."user"
                    WHERE agent_room_code = candidate::text
                      AND deleted_at IS NULL
                )
              AND NOT EXISTS (
                    SELECT 1 FROM public.special_number_resources
                    WHERE number = candidate::text
                )
            ORDER BY candidate
            LIMIT 1;
        END IF;

        IF next_code IS NULL THEN
            RAISE EXCEPTION 'no valid public room code is available for agent %', account.user_id;
        END IF;

		UPDATE public.special_number_resources
		SET number = next_code,
			updated_at = CURRENT_TIMESTAMP
		WHERE number IS NOT DISTINCT FROM account.agent_room_code
		  AND owner_user_id = account.user_id
		  AND status = 'granted';

        UPDATE public."user"
        SET agent_room_code = next_code,
            updated_at = CURRENT_TIMESTAMP
        WHERE user_id = account.user_id;
    END LOOP;

    -- Keep current configuration identity in sync.  Application room-code
    -- snapshots and historic financial/report rows are intentionally untouched.
    UPDATE public.system_settings AS config
    SET room_code = workspace.room_code,
        updated_at = CURRENT_TIMESTAMP
    FROM public.workspaces AS workspace
    WHERE config.workspace_id = workspace.id
      AND config.room_code IS DISTINCT FROM workspace.room_code;

    -- A code may appear in the workspace and its owning legacy agent shadow,
    -- but never in a different room/account pair.
    IF EXISTS (
        SELECT 1
        FROM public.workspaces AS workspace
        JOIN public."user" AS account_row
          ON account_row.agent_room_code = workspace.room_code
         AND account_row.deleted_at IS NULL
        WHERE workspace.room_code <> ''
          AND (workspace.type <> 'agent' OR workspace.owner_user_id <> account_row.user_id)
    ) THEN
        RAISE EXCEPTION 'public room code registry contains cross-owner conflicts';
    END IF;
END
$$;

ALTER TABLE public.workspaces
    DROP CONSTRAINT IF EXISTS chk_workspace_public_room_code;
ALTER TABLE public.workspaces
    ADD CONSTRAINT chk_workspace_public_room_code
    CHECK (
        type = 'platform'
        OR (room_code IS NOT NULL AND room_code ~ '^[0-9]{5,12}$')
    );

ALTER TABLE public."user"
    DROP CONSTRAINT IF EXISTS chk_agent_public_room_code;
ALTER TABLE public."user"
    ADD CONSTRAINT chk_agent_public_room_code
    CHECK (
        role <> 'agent'
        OR (agent_room_code IS NOT NULL AND agent_room_code ~ '^[0-9]{5,12}$')
    );

-- Platform settings do not expose a public room number.  Retire the original
-- four-digit column default so new compatibility rows cannot reintroduce it.
ALTER TABLE public.system_settings
    ALTER COLUMN room_code SET DEFAULT '';
