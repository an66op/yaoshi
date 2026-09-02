package services

import (
	"backend/data/models/lottery"
	"reflect"
	"testing"
	"time"
)

func TestOrderedBingoPublicDrawReadsHideNewerLegacyRowsPostgres(t *testing.T) {
	db := timingPostgresDatabase(t)
	now := time.Now().UTC().Truncate(time.Second)
	gameID := "bingo-racing-a"
	verifiedNumbers := []int{3, 5, 9, 1, 7, 10, 6, 2, 8, 4}
	verified := lottery.Draw{
		GameID: gameID, Issue: "visibility-verified", Numbers: joinNumbers(verifiedNumbers), DrawAt: now.Add(-2 * time.Minute),
		SourceRevision: bingoOrderedSourceRevision, ConversionRevision: bingoRacingAConversionVersion,
	}
	legacy := lottery.Draw{
		GameID: gameID, Issue: "visibility-legacy-newer", Numbers: "1,2,3,4,5,6,7,8,9,10", DrawAt: now.Add(-time.Minute),
	}
	if err := db.Create(&[]lottery.Draw{verified, legacy}).Error; err != nil {
		t.Fatal(err)
	}
	nextDrawAt := now.Add(5 * time.Minute)
	if result := db.Model(&lottery.Game{}).Where("id = ?", gameID).Updates(map[string]any{
		"next_issue": "visibility-next", "next_draw_at": nextDrawAt, "timing_source": "upstream",
		"sync_status": "ok", "last_sync_error": "",
	}); result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("prepare ordered game: affected=%d err=%v", result.RowsAffected, result.Error)
	}

	service := NewLotteryService(db)
	draws, err := service.ListDraws(gameID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(draws) != 1 || draws[0].Issue != verified.Issue || !reflect.DeepEqual(draws[0].Numbers, verifiedNumbers) {
		t.Fatalf("public history exposed legacy ordered draw: %+v", draws)
	}

	games, err := service.ListGames()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, game := range games {
		if game.ID != gameID {
			continue
		}
		found = true
		if game.Issue != verified.Issue || !reflect.DeepEqual(game.LatestNumbers, verifiedNumbers) {
			t.Fatalf("game summary exposed legacy ordered draw: %+v", game)
		}
	}
	if !found {
		t.Fatal("ordered Bingo game missing from summary")
	}

	status, err := NewBetAssistantService(db).statusForWorkspace(gameID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if status.LatestIssue != verified.Issue || !reflect.DeepEqual(status.LatestNumbers, verifiedNumbers) {
		t.Fatalf("assistant status exposed legacy ordered draw: %+v", status)
	}

	var retained int64
	if err := db.Model(&lottery.Draw{}).Where("game_id = ? AND issue = ?", gameID, legacy.Issue).Count(&retained).Error; err != nil || retained != 1 {
		t.Fatalf("visibility filter changed reconciliation history: count=%d err=%v", retained, err)
	}
}

func TestOrderedBingoLifecycleAndFallbackIgnoreLegacyOnlyDrawPostgres(t *testing.T) {
	db := timingPostgresDatabase(t)
	now := time.Now().UTC().Truncate(time.Second)
	gameID := "bingo-ssc-1"
	issue := "visibility-legacy-current"
	legacy := lottery.Draw{GameID: gameID, Issue: issue, Numbers: "1,2,3,4,5", DrawAt: now.Add(-time.Minute)}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	nextDrawAt := now.Add(5 * time.Minute)
	if result := db.Model(&lottery.Game{}).Where("id = ?", gameID).Updates(map[string]any{
		"next_issue": issue, "next_draw_at": nextDrawAt, "timing_source": "upstream",
		"sync_status": "ok", "last_sync_error": "",
	}); result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("prepare ordered game: affected=%d err=%v", result.RowsAffected, result.Error)
	}
	legacyDrawAt := legacy.DrawAt.UTC()
	legacySettledAt := legacyDrawAt
	scheduledDrawAt := nextDrawAt.UTC()
	legacyLifecycle := lottery.Issue{
		GameID: gameID, Issue: issue, Status: lottery.IssueStatusSettled, SourceMode: "external",
		AcceptAt: scheduledDrawAt.Add(-5 * time.Minute), SealAt: scheduledDrawAt.Add(-3 * time.Second),
		ScheduledDrawAt: &scheduledDrawAt, DrawAt: &legacyDrawAt, SettledAt: &legacySettledAt,
	}
	if err := db.Create(&legacyLifecycle).Error; err != nil {
		t.Fatal(err)
	}
	var game lottery.Game
	if err := db.First(&game, "id = ?", gameID).Error; err != nil {
		t.Fatal(err)
	}
	lifecycle, err := NewBetAdminService(db).EnsureCurrentIssue(&game)
	if err != nil {
		t.Fatal(err)
	}
	if lifecycle.Status == lottery.IssueStatusSettled || lifecycle.Status == lottery.IssueStatusSettling || lifecycle.DrawAt != nil || lifecycle.SettledAt != nil {
		t.Fatalf("legacy-only draw closed ordered lifecycle as authoritative: %+v", lifecycle)
	}
	var persistedLifecycle lottery.Issue
	if err := db.First(&persistedLifecycle, legacyLifecycle.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persistedLifecycle.DrawAt != nil || persistedLifecycle.SettledAt != nil || persistedLifecycle.Status == lottery.IssueStatusSettled || persistedLifecycle.Status == lottery.IssueStatusSettling {
		t.Fatalf("legacy lifecycle pointers survived ordered visibility gate: %+v", persistedLifecycle)
	}
	draws, err := NewLotteryService(db).ListDraws(gameID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(draws) != 0 {
		t.Fatalf("legacy-only ordered history was exposed: %+v", draws)
	}
	games, err := NewLotteryService(db).ListGames()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range games {
		if item.ID == gameID && (item.Issue != "" || len(item.LatestNumbers) != 0) {
			t.Fatalf("legacy-only ordered draw leaked through game summary: %+v", item)
		}
	}
	status, err := NewBetAssistantService(db).statusForWorkspace(gameID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if status.LatestIssue != "" || len(status.LatestNumbers) != 0 || status.LatestDrawAt != nil {
		t.Fatalf("legacy-only ordered draw leaked through assistant status: %+v", status)
	}

	// Without a source-provided next issue, a retained legacy row is not a valid
	// schedule seed either.
	game.NextIssue = ""
	if inferred, err := NewBetAdminService(db).currentIssueForGame(&game); err != nil || inferred != "" {
		t.Fatalf("legacy-only draw drove ordered issue fallback: issue=%q err=%v", inferred, err)
	}
}
