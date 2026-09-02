package migrations

import (
	"strings"
	"testing"
)

func TestGameChatRetentionMigrationIsOptInAndIndexed(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608310001_game_chat_retention.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, fragment := range []string{"'game_chat_messages', false, 7, 0, 'soft_delete'", "purge_after_days BETWEEN 0 AND 3650", "ON CONFLICT (workspace_id, data_class) DO NOTHING", "workspace_id, room_type, room_scope, game_id, id", "WHERE deleted_at IS NULL", "workspace_id, deleted_at, id", "WHERE deleted_at IS NOT NULL"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("game retention migration lacks %q", fragment)
		}
	}
	for _, forbidden := range []string{"DELETE FROM", "TRUNCATE", "SET enabled = true"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("migration performs an unrequested mutation: %s", forbidden)
		}
	}
}
