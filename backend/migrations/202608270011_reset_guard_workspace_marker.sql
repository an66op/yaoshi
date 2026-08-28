-- Workspace bootstrap historically created this one-shot marker table after
-- versioned migrations had already refreshed destructive-operation guards.
-- Version the table itself so a fresh database protects it before bootstrap
-- can write the first marker.

CREATE TABLE IF NOT EXISTS public.workspace_migration_markers (
    key varchar(120) PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

SELECT public.install_application_truncate_guards();
