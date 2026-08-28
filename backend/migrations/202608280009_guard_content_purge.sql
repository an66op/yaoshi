-- Permanent content purge is intentionally separate from the verified
-- financial/archive lifecycle. The flag is transaction-local and is set only
-- by an executed, frozen hard-delete preview.
CREATE OR REPLACE FUNCTION reject_protected_hard_delete() RETURNS trigger AS $$
BEGIN
    IF current_setting('wangzhe.lifecycle_content_purge', true) = 'on'
       AND TG_TABLE_NAME IN (
           'member_chat_messages',
           'member_notifications',
           'admin_notifications'
       ) THEN
        -- Only these three relations have the lifecycle deleted_at column.
        -- Keeping this field access inside the table allow-list branch also
        -- keeps the shared trigger safe for immutable tables without it.
        IF OLD.deleted_at IS NOT NULL THEN
            RETURN OLD;
        END IF;
    END IF;

    RAISE EXCEPTION 'hard DELETE is disabled for protected table %', TG_TABLE_NAME
        USING ERRCODE = '55000';
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION reject_protected_hard_delete() IS
    'Fail-closed delete guard. Only transaction-local lifecycle_content_purge may remove an already soft-deleted chat or notification row.';
