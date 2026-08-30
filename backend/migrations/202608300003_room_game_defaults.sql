-- Freeze the former implicit-on rule exactly once for rooms that already
-- exist. Preserve every explicit on/off choice, including legacy agent-owned
-- rows not yet attached to their immutable workspace.
UPDATE room_game_settings AS room_game
SET workspace_id = room.id
FROM workspaces AS room
WHERE room_game.workspace_id = 0
  AND room_game.agent_id = room.owner_user_id;

INSERT INTO room_game_settings (workspace_id, agent_id, game_id, enabled, created_at, updated_at)
SELECT room.id, room.owner_user_id, game.id, TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM workspaces AS room
CROSS JOIN lottery_games AS game
WHERE room.type IN ('tenant', 'agent')
ON CONFLICT DO NOTHING;

-- New tenants/agents inherit the catalogue and categories, not permission to
-- run it. Code materializes FALSE at creation and treats any future missing
-- game switch as closed. The migration runner prevents the TRUE backfill from
-- ever being repeated on a restart or a subsequent room creation.
ALTER TABLE room_game_settings ALTER COLUMN enabled SET DEFAULT FALSE;
