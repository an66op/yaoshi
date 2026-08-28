-- Workspace-scoped public identity used by member profiles and lottery chat.
-- These fields are display metadata only; they never participate in
-- authorization, accounting or workspace ownership decisions.

ALTER TABLE public."user"
    ADD COLUMN IF NOT EXISTS avatar text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS public_title varchar(80) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS public_badge varchar(40) NOT NULL DEFAULT '';

ALTER TABLE public.system_settings
    ADD COLUMN IF NOT EXISTS chat_avatar text NOT NULL DEFAULT '';

COMMENT ON COLUMN public."user".avatar IS
    'Member-facing avatar path or validated image data URL.';
COMMENT ON COLUMN public."user".public_title IS
    'Optional public room title; never used as an authorization role.';
COMMENT ON COLUMN public."user".public_badge IS
    'Optional public room badge; never used as an authorization role.';
COMMENT ON COLUMN public.system_settings.chat_avatar IS
    'Workspace-scoped operator/draw-assistant avatar; room logo is the fallback.';
