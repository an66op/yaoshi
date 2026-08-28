ALTER TABLE chat_red_packets
    ADD COLUMN IF NOT EXISTS request_id varchar(96) NOT NULL DEFAULT '';

-- Old rows did not have a client retry key. Only non-empty keys participate
-- in the room-scoped uniqueness guarantee.
CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_red_packets_workspace_request
    ON chat_red_packets (workspace_id, request_id)
    WHERE request_id <> '';
