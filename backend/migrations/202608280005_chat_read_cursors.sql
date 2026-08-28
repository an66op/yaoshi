CREATE TABLE IF NOT EXISTS member_chat_read_cursors (
    id bigserial PRIMARY KEY,
    operator_user_id bigint NOT NULL,
    workspace_id bigint NOT NULL,
    scope varchar(64) NOT NULL,
    room_scope varchar(64) NOT NULL,
    game_id varchar(40) NOT NULL,
    room_type varchar(20) NOT NULL,
    last_read_message_id bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT member_chat_read_cursors_operator_fk
        FOREIGN KEY (operator_user_id) REFERENCES "user"(user_id) ON DELETE CASCADE,
    CONSTRAINT member_chat_read_cursors_room_type_check
        CHECK (room_type IN ('service', 'group'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_read_cursor
    ON member_chat_read_cursors (operator_user_id, workspace_id, scope, room_scope, game_id, room_type);

CREATE INDEX IF NOT EXISTS idx_member_chat_read_cursors_workspace_id
    ON member_chat_read_cursors (workspace_id);

CREATE INDEX IF NOT EXISTS idx_member_chat_read_cursors_operator_user_id
    ON member_chat_read_cursors (operator_user_id);

-- Deployment baseline: existing history predates the unread feature and must
-- not wake every operator on first load. Each platform admin gets an
-- independent cursor for every existing service conversation.
INSERT INTO member_chat_read_cursors (
    operator_user_id, workspace_id, scope, room_scope, game_id, room_type,
    last_read_message_id, created_at, updated_at
)
SELECT operator.user_id, message.workspace_id, message.scope, message.room_scope,
       message.game_id, message.room_type, MAX(message.id), now(), now()
FROM "user" AS operator
CROSS JOIN member_chat_messages AS message
WHERE operator.role = 'admin'
  AND message.room_type = 'service'
GROUP BY operator.user_id, message.workspace_id, message.scope,
         message.room_scope, message.game_id, message.room_type
ON CONFLICT (operator_user_id, workspace_id, scope, room_scope, game_id, room_type)
DO UPDATE SET
    last_read_message_id = GREATEST(
        member_chat_read_cursors.last_read_message_id,
        EXCLUDED.last_read_message_id
    ),
    updated_at = now();

-- Agent and tenant operators are baselined only against the workspace they
-- own. In particular, a tenant does not inherit cursors for child-agent rooms.
INSERT INTO member_chat_read_cursors (
    operator_user_id, workspace_id, scope, room_scope, game_id, room_type,
    last_read_message_id, created_at, updated_at
)
SELECT operator.user_id, message.workspace_id, message.scope, message.room_scope,
       message.game_id, message.room_type, MAX(message.id), now(), now()
FROM "user" AS operator
JOIN workspaces AS workspace
  ON workspace.owner_user_id = operator.user_id
JOIN member_chat_messages AS message
  ON message.workspace_id = workspace.id
 AND message.room_scope = workspace.scope
WHERE operator.role IN ('agent', 'tenant')
  AND message.room_type = 'service'
GROUP BY operator.user_id, message.workspace_id, message.scope,
         message.room_scope, message.game_id, message.room_type
ON CONFLICT (operator_user_id, workspace_id, scope, room_scope, game_id, room_type)
DO UPDATE SET
    last_read_message_id = GREATEST(
        member_chat_read_cursors.last_read_message_id,
        EXCLUDED.last_read_message_id
    ),
    updated_at = now();
