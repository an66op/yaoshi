ALTER TABLE member_chat_messages
    ADD COLUMN IF NOT EXISTS request_id varchar(96) NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_member_chat_command_request
    ON member_chat_messages (user_id, request_id)
    WHERE request_id <> '';

WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY workspace_id, room_scope, game_id, reference_id
               ORDER BY id ASC
           ) AS row_number
    FROM member_chat_messages
    WHERE user_id = 0
      AND message_type = 'application'
      AND reference_id > 0
)
UPDATE member_chat_messages AS message
SET reference_id = 0
FROM ranked
WHERE message.id = ranked.id
  AND ranked.row_number > 1;

CREATE UNIQUE INDEX IF NOT EXISTS idx_member_chat_assistant_reference
    ON member_chat_messages (workspace_id, room_scope, game_id, reference_id)
    WHERE user_id = 0 AND message_type = 'application' AND reference_id > 0;

COMMENT ON COLUMN member_chat_messages.request_id IS
    'Client request id for idempotent member room commands; empty for ordinary chat.';
