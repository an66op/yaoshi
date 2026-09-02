package services

import (
	"backend/data/models/odds"
	"testing"
)

func TestExplicitOddsGamesRemainUnconfiguredAcrossBootstrapSync(t *testing.T) {
	db := timingPostgresDatabase(t)
	gameIDs := []string{"speed-ssc", "au-lucky-5", "bingo-ssc-1", "bingo-racing-a", "pc-canada", "canada-28", "canada-20"}
	assertMissing := func(stage string) {
		t.Helper()
		for _, gameID := range gameIDs {
			var count int64
			if err := db.Model(&odds.PlayLimit{}).Where("game_id = ?", gameID).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("%s created %d hard-coded odds rows for %s", stage, count, gameID)
			}
		}
	}

	assertMissing("bootstrap")
	if _, err := NewOddsAdminService(db).SyncAllGames(); err != nil {
		t.Fatal(err)
	}
	assertMissing("catalog sync")

	// A numeric legacy row remains stored for audit/migration compatibility but
	// must not become an active quote without the administrator save boundary.
	row := odds.PlayLimit{GameID: "bingo-ssc-1", PlayCode: "dragon_tiger_tie", PlayName: "龙虎和", Odds: 8.7, MinBet: 1, MaxBet: 50000, MaxUserPeriod: 50000, MaxPeriodTotal: 100000}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := NewOddsAdminService(db).SyncAllGames(); err != nil {
		t.Fatal(err)
	}
	var stored odds.PlayLimit
	stored = odds.PlayLimit{}
	if err := db.Where("game_id = ? AND play_code = ?", row.GameID, row.PlayCode).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Odds != row.Odds {
		t.Fatalf("catalog sync changed legacy odds evidence: got %v want %v", stored.Odds, row.Odds)
	}
	view, err := NewOddsAdminService(db).Get(row.GameID)
	if err != nil {
		t.Fatal(err)
	}
	for index := range view.Items {
		if view.Items[index].PlayCode != row.PlayCode {
			continue
		}
		if view.Items[index].Configured || view.Items[index].Odds != 0 || view.Items[index].ConfigurationSource != oddsSourceLegacyUnconfirmed {
			t.Fatalf("legacy target row became a live quote: %+v", view.Items[index])
		}
		view.Items[index].Odds = 8.7
	}
	configured, err := NewOddsAdminService(db).Update(row.GameID, UpdateOddsLimitsInput{Items: view.Items})
	if err != nil {
		t.Fatal(err)
	}
	var activated PlayLimitItem
	for _, item := range configured.Items {
		if item.PlayCode == row.PlayCode {
			activated = item
		}
	}
	if !activated.Configured || activated.Odds != 8.7 || activated.ConfigurationSource != oddsSourceAdminSave || activated.ConfiguredAt == nil {
		t.Fatalf("administrator save did not activate an auditable quote: %+v", activated)
	}
	stored = odds.PlayLimit{}
	if err := db.Where("game_id = ? AND play_code = ?", row.GameID, row.PlayCode).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.ExplicitlyConfigured || stored.ConfigurationSource != oddsSourceAdminSave || stored.ConfiguredAt == nil {
		t.Fatalf("persisted activation provenance missing: %+v", stored)
	}
}
