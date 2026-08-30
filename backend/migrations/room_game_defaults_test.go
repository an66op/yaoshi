package migrations

import (
	"strings"
	"testing"
)

func TestRoomGameDefaultMigrationPreservesExplicitChoicesAndOnlyBackfillsExistingRooms(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608300003_room_game_defaults.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"room_game.workspace_id = 0",
		"room_game.agent_id = room.owner_user_id",
		"SELECT room.id, room.owner_user_id, game.id, TRUE",
		"CROSS JOIN lottery_games AS game",
		"WHERE room.type IN ('tenant', 'agent')",
		"ON CONFLICT DO NOTHING",
		"ALTER COLUMN enabled SET DEFAULT FALSE",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("room game migration omitted %q", fragment)
		}
	}
	if strings.Index(sql, "UPDATE room_game_settings") > strings.Index(sql, "INSERT INTO room_game_settings") {
		t.Fatal("historic agent-owned switches must be scoped before the implicit-on backfill")
	}
	for _, forbidden := range []string{"DO UPDATE", "SET enabled = TRUE", "DELETE FROM", "TRUNCATE"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("room defaults migration overwrites existing data: %s", forbidden)
		}
	}
}
