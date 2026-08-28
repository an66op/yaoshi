package migrations

import (
	"strings"
	"testing"
)

func TestChatMessageOwnershipMigrationFreezesHistoricalRoom(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608280012_chat_message_room_ownership.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"CREATE OR REPLACE FUNCTION guard_chat_message_room_ownership()",
		"NEW.workspace_id IS DISTINCT FROM OLD.workspace_id",
		"NEW.room_scope IS DISTINCT FROM OLD.room_scope",
		"NEW.scope IS DISTINCT FROM OLD.scope",
		"workspace.id = NEW.workspace_id",
		"workspace.scope = NEW.room_scope",
		"BEFORE INSERT OR UPDATE OF workspace_id",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("chat ownership migration is missing %q", fragment)
		}
	}
	upper := strings.ToUpper(sql)
	for _, forbidden := range []string{"DELETE FROM", "TRUNCATE", "DROP TABLE"} {
		if strings.Contains(upper, forbidden) {
			t.Fatalf("chat ownership migration contains destructive SQL %q", forbidden)
		}
	}
}
