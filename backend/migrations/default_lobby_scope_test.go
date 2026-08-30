package migrations

import (
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const defaultLobbyScopeMigration = "202608300005_default_lobby_scope.sql"

func TestDefaultLobbyScopeMigrationOnlyCorrectsPreviousDefaultPlacements(t *testing.T) {
	contents, err := migrationFiles.ReadFile(defaultLobbyScopeMigration)
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"UPDATE public.lottery_games AS game",
		"SET lobby_category = ''",
		"lobby_sort_order = 0",
		"WHERE game.id = previous_defaults.game_id",
		"AND game.lobby_category = previous_defaults.lobby_category",
		"AND game.lobby_sort_order = previous_defaults.lobby_sort_order",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("default lobby scope repair is missing safeguard %q", fragment)
		}
	}
	upper := strings.ToUpper(regexp.MustCompile(`(?m)--[^\n]*`).ReplaceAllString(sql, ""))
	for _, forbidden := range []string{
		"ENABLED", "DELETED_AT", "DELETE FROM", "TRUNCATE", "INSERT INTO", "ALTER TABLE",
		"LOTTERY_DRAWS", "LOTTERY_ISSUES", "LOTTERY_BETS", "LOTTERY_LOBBY_CATEGORIES", "ODDS",
		"WORKSPACES", "ROOM_GAME_SETTINGS", "GAME_SETTINGS_JSON", "PUBLIC.USER", "PASSWORD", "SCHEMA_MIGRATIONS",
	} {
		if strings.Contains(upper, forbidden) {
			t.Fatalf("default lobby scope repair changes unrelated state with %q", forbidden)
		}
	}
}

func TestDefaultLobbyScopeMigrationContainsExactPreviouslyAssignedOfficialInventory(t *testing.T) {
	contents, err := migrationFiles.ReadFile(defaultLobbyScopeMigration)
	if err != nil {
		t.Fatal(err)
	}
	type placement struct {
		Category string
		Order    int
	}
	want := map[string]placement{
		"official-fc3d":           {"彩票", 9},
		"official-kl8":            {"彩票", 10},
		"official-pl3":            {"彩票", 11},
		"official-qxc":            {"彩票", 12},
		"official-tw-super-lotto": {"彩票", 13},
		"official-tw-daily539":    {"彩票", 14},
		"official-tw-lotto649":    {"彩票", 15},
		"official-tw-bingo":       {"宾果", 8},
	}
	rows := regexp.MustCompile(`\('([^']+)', '([^']+)', ([0-9]+)\)`).FindAllStringSubmatch(string(contents), -1)
	got := make(map[string]placement, len(rows))
	for _, row := range rows {
		order, err := strconv.Atoi(row[3])
		if err != nil {
			t.Fatal(err)
		}
		if _, duplicate := got[row[1]]; duplicate {
			t.Fatalf("duplicate default scope repair for %s", row[1])
		}
		got[row[1]] = placement{row[2], order}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("default scope repair inventory = %#v, want %#v", got, want)
	}
}
