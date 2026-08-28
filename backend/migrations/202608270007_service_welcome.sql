-- One durable greeting belongs to each private customer-service conversation.
-- The member API inserts it only when the conversation has no active messages;
-- this index closes the list/send race without merging different rooms or users.

CREATE UNIQUE INDEX IF NOT EXISTS idx_member_service_welcome
ON member_chat_messages (workspace_id, scope, room_scope, game_id, message_type)
WHERE room_type = 'service'
  AND message_type = 'welcome'
  AND deleted_at IS NULL;
