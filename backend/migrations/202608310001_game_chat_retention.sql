-- Game-room display retention is opt-in. Installing this migration does not
-- delete records or enable automatic cleanup in any existing workspace.
ALTER TABLE data_retention_policies
    ADD COLUMN IF NOT EXISTS purge_after_days integer NOT NULL DEFAULT 0;

ALTER TABLE data_retention_policies DROP CONSTRAINT IF EXISTS ck_retention_policy_class;
ALTER TABLE data_retention_policies ADD CONSTRAINT ck_retention_policy_class CHECK (
    data_class IN ('chat_messages', 'robot_chat_messages', 'game_chat_messages',
                   'notifications', 'audit_logs', 'robot_test_data')
);
ALTER TABLE data_retention_policies DROP CONSTRAINT IF EXISTS ck_retention_policy_purge_days;
ALTER TABLE data_retention_policies ADD CONSTRAINT ck_retention_policy_purge_days CHECK (
    purge_after_days BETWEEN 0 AND 3650
    AND (purge_after_days = 0 OR data_class = 'game_chat_messages')
);

INSERT INTO data_retention_policies
    (workspace_id, data_class, enabled, retention_days, purge_after_days, action)
VALUES (0, 'game_chat_messages', false, 7, 0, 'soft_delete')
ON CONFLICT (workspace_id, data_class) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_chat_active_room_cursor
    ON member_chat_messages (workspace_id, room_type, room_scope, game_id, id)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_chat_active_retention
    ON member_chat_messages (workspace_id, created_at, id)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_chat_deleted_retention
    ON member_chat_messages (workspace_id, deleted_at, id)
    WHERE deleted_at IS NOT NULL;
