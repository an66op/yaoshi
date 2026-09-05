package services

import (
	"backend/data/models/lottery"
	"backend/data/models/odds"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	"context"
	"testing"
)

func TestProductionOddsReadinessPostgresRespectsRoomSwitchAndCurrentRules(t *testing.T) {
	db := timingPostgresDatabase(t)
	if err := db.Model(&lottery.Game{}).Where("id <> ?", "speed-racing").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	var platform workspacemodel.Workspace
	if err := db.Where("type = ?", workspacemodel.TypePlatform).First(&platform).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(platform, "speed-racing", false); err != nil {
		t.Fatal(err)
	}
	report, err := AuditProductionOddsReadiness(context.Background(), db)
	if err != nil || !report.Complete || report.AuditedGames != 0 {
		t.Fatalf("closed platform room game must be outside the gate: report=%+v err=%v", report, err)
	}

	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(platform, "speed-racing", true); err != nil {
		t.Fatal(err)
	}
	report, err = AuditProductionOddsReadiness(context.Background(), db)
	if err != nil || !report.Complete || report.AuditedGames != 0 {
		t.Fatalf("bootstrap administrator alone must not deadlock first-install odds readiness: report=%+v err=%v", report, err)
	}

	platformMember := user.User{
		Username: "odds_platform_member", LoginScope: platform.Scope, WorkspaceID: platform.ID,
		Password: "test-only-no-login", Nickname: "赔率门禁会员", Role: "member", Status: 1,
	}
	if err := db.Create(&platformMember).Error; err != nil {
		t.Fatal(err)
	}
	report, err = AuditProductionOddsReadiness(context.Background(), db)
	wantQuotes := len(PlayCatalogForGame("speed-racing"))
	if err != nil || report.Complete || report.AuditedGames != 1 || report.RequiredQuotes != wantQuotes ||
		report.ValidQuotes != 0 || len(report.IncompleteGames) != 1 {
		t.Fatalf("open unpriced game did not fail closed: report=%+v err=%v", report, err)
	}

	service := NewOddsAdminService(db)
	current, err := service.Get("speed-racing")
	if err != nil {
		t.Fatal(err)
	}
	for index := range current.Items {
		current.Items[index].Odds = 2.25
		current.Items[index].MinBet = 1
		current.Items[index].MaxBet = 100
		current.Items[index].MaxUserPeriod = 1000
		current.Items[index].MaxPeriodTotal = 5000
	}
	if _, err := service.Update("speed-racing", oddsUpdateInput(current)); err != nil {
		t.Fatal(err)
	}
	report, err = AuditProductionOddsReadiness(context.Background(), db)
	if err != nil || !report.Complete || report.ValidQuotes != wantQuotes || len(report.IncompleteGames) != 0 {
		t.Fatalf("complete explicit current-rule prices were rejected: report=%+v err=%v", report, err)
	}

	var changed odds.PlayLimit
	if err := db.Where("game_id = ?", "speed-racing").Order("play_code ASC").First(&changed).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&changed).Update("rule_version", "racing-v1").Error; err != nil {
		t.Fatal(err)
	}
	report, err = AuditProductionOddsReadiness(context.Background(), db)
	if err != nil || report.Complete || report.ValidQuotes != wantQuotes-1 || len(report.IncompleteGames) != 1 ||
		len(report.IncompleteGames[0].InvalidPlayCodes) != 1 || report.IncompleteGames[0].InvalidPlayCodes[0] != changed.PlayCode {
		t.Fatalf("rule-version drift did not fail closed: report=%+v err=%v", report, err)
	}
}
