ALTER TABLE IF EXISTS admin_audit_logs
    ADD COLUMN IF NOT EXISTS event_id varchar(96) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS target_ref varchar(240) NOT NULL DEFAULT '';

UPDATE admin_audit_logs
SET event_id = 'legacy:' || id::text
WHERE event_id = '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_admin_audit_event_id
    ON admin_audit_logs (event_id)
    WHERE event_id <> '';
CREATE INDEX IF NOT EXISTS idx_admin_audit_target_ref
    ON admin_audit_logs (target_ref);
