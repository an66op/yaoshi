package services

import (
	"backend/data/models/lottery"
	"reflect"
	"sort"
	"testing"
	"time"

	"gorm.io/gorm"
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
	for _, template := range officialGames {
		t.Run(template.ID, func(t *testing.T) {
			game := template
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
			if values["lobby_category"] != "" || values["lobby_sort_order"] != 0 {
				t.Fatalf("official game received an unrequested default classification: %#v", values)
			}
		})
	}
}

func TestOfficialGameSeedPreservesExplicitPlacementAndOrder(t *testing.T) {
	game := officialGames[0]
	game.LobbyCategory = "自定义分类"
	game.LobbySortOrder = 77
	values := officialGameSeedValues(game)
	if values["lobby_category"] != game.LobbyCategory || values["lobby_sort_order"] != game.LobbySortOrder {
		t.Fatalf("explicit catalog placement changed: %#v", values)
	}

	// An assigned order is independent of a missing category and must survive
	// initialization, even for a game that has no default shelf.
	game.LobbyCategory = ""
	values = officialGameSeedValues(game)
	if values["lobby_category"] != "" || values["lobby_sort_order"] != 77 {
		t.Fatalf("initialization changed the unclassified game or its explicit order: %#v", values)
	}
}

func TestDefaultLobbyPlacementClassifiesOnlyConfiguredBuiltInGames(t *testing.T) {
	ids := make([]string, 0, len(defaultGames)+len(officialGames))
	for _, game := range defaultGames {
		ids = append(ids, game.ID)
	}
	for _, game := range officialGames {
		ids = append(ids, game.ID)
	}
	if len(ids) != 30 {
		t.Fatalf("catalog contains %d games, want 30", len(ids))
	}
	wantShelves := map[string][]string{
		"彩票":  {"speed-racing", "speed-fly", "speed-ssc", "sg-fly", "sg-ssc", "fly-racing", "au-lucky-5", "au-lucky-10"},
		"PC":  {"pc-canada", "canada-28", "canada-20"},
		"宾果":  {"bingo-mark-six", "bingo-racing-a", "bingo-racing-b", "bingo-ssc-1", "bingo-ssc-2", "bingo-ssc-3", "bingo-ssc-4"},
		"六合彩": {"hong-kong-mark-six", "happy8-mark-six", "new-macau-mark-six", "old-macau-mark-six"},
	}
	wantUnclassified := map[string]bool{
		"official-fc3d": true, "official-kl8": true, "official-pl3": true, "official-qxc": true,
		"official-tw-super-lotto": true, "official-tw-daily539": true, "official-tw-lotto649": true,
		"official-tw-bingo": true,
	}
	wantCounts := map[string]int{"彩票": 8, "宾果": 7, "PC": 3, "六合彩": 4, "": 8}
	gotCounts := make(map[string]int)
	slots := make(map[string]map[int]string)
	seenIDs := make(map[string]bool)
	for _, id := range ids {
		if seenIDs[id] {
			t.Fatalf("duplicate catalog ID %s", id)
		}
		seenIDs[id] = true
		category, order := defaultLobbyPlacement(id)
		if wantUnclassified[id] {
			if category != "" || order != 0 {
				t.Fatalf("catalog game %s should remain unclassified: %q / %d", id, category, order)
			}
			gotCounts[""]++
			continue
		}
		wantIDs := wantShelves[category]
		if order < 1 || order > len(wantIDs) || wantIDs[order-1] != id {
			t.Fatalf("catalog game %s has an unexpected default placement: %q / %d", id, category, order)
		}
		if slots[category] == nil {
			slots[category] = make(map[int]string)
		}
		if previous, exists := slots[category][order]; exists {
			t.Fatalf("default shelf %s has a duplicate order %d: %s and %s", category, order, previous, id)
		}
		slots[category][order] = id
		gotCounts[category]++
	}
	if !reflect.DeepEqual(gotCounts, wantCounts) {
		t.Fatalf("default shelf counts = %#v, want %#v", gotCounts, wantCounts)
	}
	if category, order := defaultLobbyPlacement("operator-created-game"); category != "" || order != 0 {
		t.Fatalf("unknown game was forced into a built-in shelf: %q / %d", category, order)
	}
}

func TestMissingDefaultLobbyCategoriesRespectsRetiredShelf(t *testing.T) {
	existing := []lottery.LobbyCategory{
		{Name: "彩票", SortOrder: 10},
		{Name: "宾果", SortOrder: 20, DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true}},
		{Name: "PC", SortOrder: 30},
		{Name: "六合彩", SortOrder: 40},
	}
	if missing := missingDefaultLobbyCategories(existing); len(missing) != 0 {
		t.Fatalf("restart would recreate retired shelves: %#v", missing)
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
