-- Workspace bootstrap must not create/drop indexes or reconcile duplicate
-- business records on every startup. Install its concurrency boundaries once,
-- before any application bootstrap data is written. Conflicting local rows
-- fail the migration instead of being silently merged or reassigned.
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_username_global_ci
    ON public."user" (LOWER(username)) WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_workspace_public_room_code
    ON public.workspaces (room_code) WHERE room_code <> '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_application_one_pending_join
    ON public.user_applications (user_id, workspace_id)
    WHERE request_type = 'join' AND status = 'pending' AND workspace_id > 0;

CREATE UNIQUE INDEX IF NOT EXISTS idx_workspace_one_active_membership
    ON public.workspace_memberships (user_id) WHERE status = 1;

-- Migration 023 already installs the same application-request constraint under
-- idx_user_applications_user_request. Remove the redundant startup-created copy.
DROP INDEX IF EXISTS public.idx_application_user_request;

-- Refresh protection for tables added after migration 010. New migrations that
-- create application tables must install these guards in the same transaction.
SELECT public.install_application_truncate_guards();
