package services

import (
	"backend/data/models/lottery"
	"backend/data/models/odds"
	"testing"
)

func TestPlayLimitItemsForProfileExposeMissingRacingOddsAsUnconfigured(t *testing.T) {
	profile, ready := rulesForVersion("racing-v2")
	if !ready {
		t.Fatal("racing-v2 profile missing")
	}
	rows := []odds.PlayLimit{
		{
			GameID: "speed-racing", PlayCode: "ball_1_5", PlayName: "自定义号码", Odds: 8.7654,
			MinBet: 2, MaxBet: 300, MaxUserPeriod: 400, MaxPeriodTotal: 500, SortOrder: 99,
		},
		{GameID: "speed-racing", PlayCode: "pair", PlayName: "旧的不支持玩法", Odds: 8.8},
	}
	items := playLimitItemsForProfile("speed-racing", profile, PlayCatalogForGame("speed-racing"), rows)
	if len(items) != 4 {
		t.Fatalf("racing catalog has %d items, want 4: %+v", len(items), items)
	}
	byCode := make(map[string]PlayLimitItem, len(items))
	for _, item := range items {
		byCode[item.PlayCode] = item
	}
	configured := byCode["ball_1_5"]
	if !configured.Configured || configured.Odds != 8.7654 || configured.MinBet != 2 || configured.MaxBet != 300 ||
		configured.MaxUserPeriod != 400 || configured.MaxPeriodTotal != 500 {
		t.Fatalf("saved racing price or limits changed: %+v", configured)
	}
	for _, code := range []string{"two_sided", "dragon_tiger", "sum"} {
		item, exists := byCode[code]
		if !exists || item.Configured || item.Odds != 0 {
			t.Fatalf("missing %s was not exposed as an unavailable zero-price item: %+v", code, item)
		}
	}
	if _, leaked := byCode["pair"]; leaked {
		t.Fatal("unsupported stale odds row leaked into the racing catalog")
	}
	if risks := playLimitOddsRisks("speed-racing", items); len(risks) != 0 {
		t.Fatalf("unconfigured placeholders were treated as live risky offers: %+v", risks)
	}
}

func TestBingoSSC1UpgradeDoesNotReuseRetiredSumOddsForTie(t *testing.T) {
	profile, ready := rulesForGame(&lottery.Game{ID: "bingo-ssc-1"})
	if !ready || profile.Version != "digits5-v3" {
		t.Fatalf("bingo-ssc-1 profile = %+v/%v", profile, ready)
	}
	rows := []odds.PlayLimit{
		{GameID: "bingo-ssc-1", PlayCode: "dragon_tiger", PlayName: "龙虎", Odds: 1.98, ExplicitlyConfigured: true, ConfigurationSource: oddsSourceAdminSave},
		// This row may exist from the former v2 contract. It must not price or
		// resurrect the new independent 和 outcome.
		{GameID: "bingo-ssc-1", PlayCode: "sum", PlayName: "旧总和", Odds: 1.99},
	}
	items := playLimitItemsForProfile("bingo-ssc-1", profile, PlayCatalogForGame("bingo-ssc-1"), rows)
	byCode := make(map[string]PlayLimitItem, len(items))
	for _, item := range items {
		byCode[item.PlayCode] = item
	}
	if _, leaked := byCode["sum"]; leaked {
		t.Fatal("retired v2 sum odds leaked into the v3 catalogue")
	}
	if tie, ok := byCode["dragon_tiger_tie"]; !ok || tie.Configured || tie.Odds != 0 {
		t.Fatalf("missing independent tie price did not fail closed: %+v/%v", tie, ok)
	}
	if dragon := byCode["dragon_tiger"]; !dragon.Configured || dragon.Odds != 1.98 {
		t.Fatalf("saved dragon/tiger price changed: %+v", dragon)
	}
}

func TestUpgradedLegacyOddsRowsRemainVisibleButUnconfigured(t *testing.T) {
	for _, gameID := range []string{"pc-canada", "speed-ssc", "bingo-racing-a"} {
		profile, ready := rulesForGame(&lottery.Game{ID: gameID})
		if !ready {
			t.Fatalf("missing rules for %s", gameID)
		}
		catalog := PlayCatalogForGame(gameID)
		if len(catalog) == 0 {
			t.Fatalf("missing odds catalog for %s", gameID)
		}
		code := catalog[0].PlayCode
		rows := []odds.PlayLimit{{GameID: gameID, PlayCode: code, PlayName: catalog[0].PlayName, Odds: 9.7}}
		items := playLimitItemsForProfile(gameID, profile, catalog, rows)
		if !items[0].Configured && items[0].Odds == 0 && items[0].ConfigurationSource == oddsSourceLegacyUnconfirmed {
			continue
		}
		t.Fatalf("%s legacy row became a live quote: %+v", gameID, items[0])
	}
}

func TestUpgradedGamesNeverSeedHardcodedOdds(t *testing.T) {
	for _, gameID := range []string{"speed-ssc", "au-lucky-5", "bingo-ssc-1", "bingo-racing-a", "pc-canada", "canada-28", "canada-20"} {
		profile, ready := rulesForGame(&lottery.Game{ID: gameID})
		if !ready || !requiresExplicitOddsConfiguration(gameID, profile) {
			t.Fatalf("%s could still receive code-level default odds: profile=%+v ready=%v", gameID, profile, ready)
		}
	}
	for _, gameID := range []string{"speed-racing", "bingo-racing-b", "bingo-ssc-2", "bingo-ssc-3", "bingo-ssc-4"} {
		profile, ready := rulesForGame(&lottery.Game{ID: gameID})
		if !ready || requiresExplicitOddsConfiguration(gameID, profile) {
			t.Fatalf("existing %s default policy changed: profile=%+v ready=%v", gameID, profile, ready)
		}
	}
}
