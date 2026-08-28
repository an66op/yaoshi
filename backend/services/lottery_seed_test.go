package services

import (
	"backend/data/models/lottery"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestMissingDefaultLobbyCategoriesRecoversPartialWithoutOverwriting(t *testing.T) {
	existing := []lottery.LobbyCategory{
		{Name: "彩票", SortOrder: 777},
		{Name: "PC", SortOrder: 888},
		{Name: "自定义", SortOrder: 1},
	}

	missing := missingDefaultLobbyCategories(existing)
	if len(missing) != 2 {
		t.Fatalf("missing category count = %d, want 2", len(missing))
	}
	if missing[0].Name != "宾果" || missing[1].Name != "六合彩" {
		t.Fatalf("missing categories = %#v, want 宾果 and 六合彩", missing)
	}
	if existing[0].SortOrder != 777 || existing[1].SortOrder != 888 {
		t.Fatalf("existing category configuration was changed: %#v", existing)
	}

	recovered := append(append([]lottery.LobbyCategory(nil), existing...), missing...)
	if second := missingDefaultLobbyCategories(recovered); len(second) != 0 {
		t.Fatalf("second recovery still wants to insert %#v", second)
	}
}

func TestOfficialGameSeedKeepsExplicitDisabledState(t *testing.T) {
	now := time.Date(2026, 8, 27, 10, 31, 0, 0, time.UTC)
	game := officialGames[0]
	game.CreatedAt = now
	game.UpdatedAt = now
	game.NextDrawAt = now.Add(time.Duration(game.DrawInterval) * time.Second)

	values := officialGameSeedValues(game)
	enabled, ok := values["enabled"].(bool)
	if !ok || enabled {
		t.Fatalf("official game enabled seed = %#v, want explicit false", values["enabled"])
	}
	if values["id"] != game.ID || values["code"] != game.Code {
		t.Fatalf("official game identity missing from seed values: %#v", values)
	}
}

func TestDeterministicFixtureRecoveryIsStableAndDoesNotOverwrite(t *testing.T) {
	anchor := time.Date(2026, 8, 27, 10, 31, 0, 0, time.UTC)
	const (
		gameID   = "speed-racing"
		gameCode = "SPEED_RACING"
		interval = 180
		seed     = 11
	)
	want := deterministicFixtureRows(gameID, gameCode, interval, seed, anchor)

	// Simulate an interrupted old bootstrap: only two fixture rows survived.
	partial := []lottery.Draw{want[3], want[9]}
	recoveredAnchor, found := deterministicFixtureAnchor(gameCode, interval, anchor.Add(45*time.Second), anchor.Add(24*time.Hour), partial)
	if !found {
		t.Fatal("partial fixture was not recognized")
	}
	if !recoveredAnchor.Equal(anchor) {
		t.Fatalf("recovered anchor = %s, want %s", recoveredAnchor, anchor)
	}
	recoveryRows := deterministicFixtureRows(gameID, gameCode, interval, seed, recoveredAnchor)
	if !reflect.DeepEqual(recoveryRows, want) {
		t.Fatal("partial recovery produced a different fixture set")
	}

	// ON CONFLICT DO NOTHING must preserve an operator-modified result that
	// already occupies a fixture issue while inserting the missing rows.
	operatorResult := want[0]
	operatorResult.Numbers = "9,9,9,9,9"
	stored := map[string]lottery.Draw{operatorResult.Issue: operatorResult}
	for _, draw := range partial {
		stored[draw.Issue] = draw
	}
	mergeFixtureRows(stored, recoveryRows)
	if len(stored) != deterministicFixtureDrawCount {
		t.Fatalf("recovered row count = %d, want %d", len(stored), deterministicFixtureDrawCount)
	}
	if got := stored[operatorResult.Issue].Numbers; got != operatorResult.Numbers {
		t.Fatalf("operator result was overwritten with %q", got)
	}

	// A complete second bootstrap derives the same anchor and keys, so it adds
	// no rows even when the database returns candidates in a different order.
	all := make([]lottery.Draw, 0, len(stored))
	for _, draw := range stored {
		all = append(all, draw)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Issue > all[j].Issue })
	secondAnchor, found := deterministicFixtureAnchor(gameCode, interval, time.Time{}, anchor.Add(48*time.Hour), all)
	if !found || !secondAnchor.Equal(anchor) {
		t.Fatalf("second anchor = %s, found=%v; want %s", secondAnchor, found, anchor)
	}
	mergeFixtureRows(stored, deterministicFixtureRows(gameID, gameCode, interval, seed, secondAnchor))
	if len(stored) != deterministicFixtureDrawCount {
		t.Fatalf("second bootstrap grew history to %d rows", len(stored))
	}
}

func TestDeterministicFixtureAnchorUsesStableGameCreationTime(t *testing.T) {
	createdAt := time.Date(2026, 8, 27, 10, 31, 49, 0, time.UTC)
	first, found := deterministicFixtureAnchor("SG_SSC", 300, createdAt, createdAt.Add(24*time.Hour), nil)
	if found {
		t.Fatal("empty history unexpectedly reported an existing fixture")
	}
	second, found := deterministicFixtureAnchor("SG_SSC", 300, createdAt, createdAt.Add(48*time.Hour), nil)
	if found || !first.Equal(second) {
		t.Fatalf("fallback changed stable anchor: first=%s second=%s", first, second)
	}
	if want := createdAt.Truncate(time.Minute); !first.Equal(want) {
		t.Fatalf("anchor = %s, want %s", first, want)
	}
}

func mergeFixtureRows(stored map[string]lottery.Draw, rows []lottery.Draw) {
	for _, draw := range rows {
		if _, exists := stored[draw.Issue]; exists {
			continue
		}
		stored[draw.Issue] = draw
	}
}
