package migrations

import (
	"strings"
	"testing"
)

func TestPublicRoomCodeMigrationPreservesWorkspaceScopedHistory(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608280007_public_room_codes.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"8801 -> 88001",
		"UPDATE public.workspaces",
		"UPDATE public.system_settings",
		"UPDATE public.special_number_resources",
		"chk_workspace_public_room_code",
		"chk_agent_public_room_code",
		"room_code IS NULL",
		"agent_room_code IS NULL",
		"'^[0-9]{5,12}$'",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("public-room migration is missing %q", fragment)
		}
	}
	upper := strings.ToUpper(sql)
	for _, forbidden := range []string{
		"UPDATE PUBLIC.LOTTERY_BETS",
		"UPDATE PUBLIC.MEMBER_CHAT_MESSAGES",
		"UPDATE PUBLIC.USER_APPLICATIONS",
		"UPDATE PUBLIC.USER_BALANCE_TRANSACTIONS",
		"TRUNCATE",
		"DELETE FROM",
	} {
		if strings.Contains(upper, forbidden) {
			t.Fatalf("public-room migration mutates historic business data with %q", forbidden)
		}
	}
}
