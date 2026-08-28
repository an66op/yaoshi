package migrations

import (
	"strings"
	"testing"
)

func TestRoomChatIdentityMigrationIsAdditive(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608280004_room_chat_identity.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		`ALTER TABLE public."user"`,
		`ADD COLUMN IF NOT EXISTS avatar`,
		`ADD COLUMN IF NOT EXISTS public_title`,
		`ADD COLUMN IF NOT EXISTS public_badge`,
		`ALTER TABLE public.system_settings`,
		`ADD COLUMN IF NOT EXISTS chat_avatar`,
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("room chat identity migration is missing %q", fragment)
		}
	}
	upper := strings.ToUpper(sql)
	for _, forbidden := range []string{"DROP TABLE", "TRUNCATE", "DELETE FROM"} {
		if strings.Contains(upper, forbidden) {
			t.Fatalf("room chat identity migration contains destructive SQL %q", forbidden)
		}
	}
}
