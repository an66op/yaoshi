package migrations

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const defaultLobbyAssignmentMigration = "202608300002_default_lobby_assignments.sql"

func TestDefaultLobbyAssignmentMigrationPreservesOperatorConfiguration(t *testing.T) {
	contents, err := migrationFiles.ReadFile(defaultLobbyAssignmentMigration)
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"INSERT INTO public.lottery_lobby_categories (name, sort_order, created_at, updated_at)",
		"ON CONFLICT (name) DO NOTHING",
		"UPDATE public.lottery_games AS game",
		"WHEN game.lobby_sort_order = 0 THEN placements.lobby_sort_order",
		"ELSE game.lobby_sort_order",
		"ON category.name = placements.lobby_category",
		"AND category.deleted_at IS NULL",
		"WHERE game.id = placements.game_id",
		"AND btrim(COALESCE(game.lobby_category, '')) = ''",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("default lobby repair is missing safeguard %q", fragment)
		}
	}
	upper := strings.ToUpper(regexp.MustCompile(`(?m)--[^\n]*`).ReplaceAllString(sql, ""))
	for _, forbidden := range []string{
		"ENABLED =", "SET DELETED_AT", "DO UPDATE", "DELETE FROM", "TRUNCATE",
		"LOTTERY_DRAWS", "LOTTERY_ISSUES", "LOTTERY_BETS", "ODDS", "WORKSPACES",
		"PUBLIC.USER", "PASSWORD", "INSERT INTO PUBLIC.LOTTERY_GAMES",
	} {
		if strings.Contains(upper, forbidden) {
			t.Fatalf("default lobby repair changes unrelated/operator state with %q", forbidden)
		}
	}
}

func TestDefaultLobbyAssignmentMigrationContainsExactBuiltInInventory(t *testing.T) {
	contents, err := migrationFiles.ReadFile(defaultLobbyAssignmentMigration)
	if err != nil {
		t.Fatal(err)
	}
	// Each list also fixes the default order within that shelf. Existing
	// non-zero operator orders are preserved by the CASE checked above.
	want := map[string][]string{
		"彩票": {
			"speed-racing", "speed-fly", "speed-ssc", "sg-fly", "sg-ssc", "fly-racing", "au-lucky-5", "au-lucky-10",
			"official-fc3d", "official-kl8", "official-pl3", "official-qxc", "official-tw-super-lotto", "official-tw-daily539", "official-tw-lotto649",
		},
		"宾果":  {"bingo-mark-six", "bingo-racing-a", "bingo-racing-b", "bingo-ssc-1", "bingo-ssc-2", "bingo-ssc-3", "bingo-ssc-4", "official-tw-bingo"},
		"PC":  {"pc-canada", "canada-28", "canada-20"},
		"六合彩": {"hong-kong-mark-six", "happy8-mark-six", "new-macau-mark-six", "old-macau-mark-six"},
	}
	rows := regexp.MustCompile(`\('([^']+)', '([^']+)', ([0-9]+)\)`).FindAllStringSubmatch(string(contents), -1)
	if len(rows) != 30 {
		t.Fatalf("repair contains %d game placements, want 30", len(rows))
	}
	seen := make(map[string]bool)
	for _, row := range rows {
		id, category := row[1], row[2]
		order, err := strconv.Atoi(row[3])
		if err != nil {
			t.Fatal(err)
		}
		if seen[id] {
			t.Fatalf("duplicate default placement for %s", id)
		}
		seen[id] = true
		ids := want[category]
		if order < 1 || order > len(ids) || ids[order-1] != id {
			t.Fatalf("unexpected default placement: %s / %s / %d", id, category, order)
		}
	}
}
