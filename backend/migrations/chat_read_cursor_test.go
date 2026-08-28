package migrations

import (
	"strings"
	"testing"
)

func TestChatReadCursorMigrationBaselinesWithoutCrossingTenantRooms(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608280005_chat_read_cursors.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS member_chat_read_cursors",
		"operator_user_id, workspace_id, scope, room_scope, game_id, room_type",
		"CROSS JOIN member_chat_messages AS message",
		"operator.role = 'admin'",
		"workspace.owner_user_id = operator.user_id",
		"message.room_scope = workspace.scope",
		"operator.role IN ('agent', 'tenant')",
		"message.room_type = 'service'",
		"MAX(message.id)",
		"ON CONFLICT (operator_user_id, workspace_id, scope, room_scope, game_id, room_type)",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("chat read cursor migration is missing %q", fragment)
		}
	}
	upper := strings.ToUpper(sql)
	for _, forbidden := range []string{"TRUNCATE", "DELETE FROM MEMBER_CHAT_MESSAGES", "DROP TABLE"} {
		if strings.Contains(upper, forbidden) {
			t.Fatalf("chat read cursor migration contains destructive SQL %q", forbidden)
		}
	}
}
