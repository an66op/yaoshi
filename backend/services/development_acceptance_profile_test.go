package services

import (
	"backend/constants"
	"backend/data/models/chat"
	"backend/data/models/lottery"
	"backend/data/models/odds"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	"backend/utils"
	"testing"
)

func TestDevelopmentAcceptanceProfileContract(t *testing.T) {
	profile, err := loadDevelopmentOddsProfile()
	if err != nil {
		t.Fatal(err)
	}
	if profile.Version != developmentAcceptanceProfileVersion {
		t.Fatalf("profile version = %q", profile.Version)
	}
	games, quotes := 0, 0
	for _, template := range profile.Templates {
		games += len(template.Games)
		quotes += len(template.Games) * len(template.Odds)
	}
	if games != len(defaultGames) || games != 22 {
		t.Fatalf("configured games = %d, want 22", games)
	}
	if quotes != 1437 {
		t.Fatalf("configured quotes = %d, want 1437", quotes)
	}
}

func TestDevelopmentAcceptanceProfilePostgresFreshIdempotentAndNonOverwriting(t *testing.T) {
	db := timingPostgresDatabase(t)
	var bootstrapAdmin user.User
	if err := db.Where("username = ?", "timing_platform").First(&bootstrapAdmin).Error; err != nil {
		t.Fatal(err)
	}
	adminPassword, err := utils.HashPassword(constants.DefaultAdminPassword)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&bootstrapAdmin).Updates(map[string]any{
		"username": constants.DefaultAdminUsername,
		"password": adminPassword,
		"nickname": constants.DefaultAdminNickname,
	}).Error; err != nil {
		t.Fatal("normalize isolated bootstrap administrator:", err)
	}
	if err := Bootstrap(db, BootstrapOptions{Mode: "debug", SeedExperienceAccounts: true}); err != nil {
		t.Fatal("seed experience hierarchy:", err)
	}

	first, err := ApplyDevelopmentAcceptanceProfile(db, "debug")
	if err != nil {
		t.Fatal("apply fresh development profile:", err)
	}
	if first.HumanAccounts != 4 || first.RobotAccounts != 30 || first.Workspaces != 3 ||
		first.ActiveAccounts != 34 || first.ActiveMemberships != 34 || first.ConfiguredGames != 22 || first.ConfiguredPlayQuotes != 1437 ||
		first.AgentRoomCode != demoRoomCode || first.AgentRoomOpenGames != 22 {
		t.Fatalf("unexpected fresh profile report: %+v", first)
	}

	second, err := ApplyDevelopmentAcceptanceProfile(db, "debug")
	if err != nil {
		t.Fatal("repeat development profile:", err)
	}
	if *second != *first {
		t.Fatalf("repeat changed report: first=%+v second=%+v", first, second)
	}
	audited, err := AuditDevelopmentAcceptanceProfile(db, "debug")
	if err != nil {
		t.Fatal("read-only development profile audit:", err)
	}
	if *audited != *first {
		t.Fatalf("audit changed report: first=%+v audit=%+v", first, audited)
	}

	var agent user.User
	if err := db.Where("username = ?", demoAgentUsername).First(&agent).Error; err != nil {
		t.Fatal(err)
	}
	var room workspacemodel.Workspace
	if err := db.Where("owner_user_id = ?", agent.UserID).First(&room).Error; err != nil {
		t.Fatal(err)
	}
	var enabled int64
	if err := db.Model(&chat.RoomGameSetting{}).Where("workspace_id = ? AND enabled = ?", room.ID, true).Count(&enabled).Error; err != nil {
		t.Fatal(err)
	}
	if enabled != 22 {
		t.Fatalf("enabled games = %d, want 22", enabled)
	}
	if err := db.Model(&lottery.Game{}).Where("id = ?", "official-fc3d").Updates(map[string]any{
		"enabled": true, "lobby_category": "彩票",
	}).Error; err != nil {
		t.Fatal("add out-of-profile public game:", err)
	}
	if _, err := AuditDevelopmentAcceptanceProfile(db, "debug"); err == nil {
		t.Fatal("expected audit to reject an extra enabled and categorized platform game")
	}
	if err := db.Model(&lottery.Game{}).Where("id = ?", "official-fc3d").Updates(map[string]any{
		"enabled": false, "lobby_category": "",
	}).Error; err != nil {
		t.Fatal("restore out-of-profile public game:", err)
	}

	var quote odds.PlayLimit
	if err := db.Where("game_id = ?", "speed-racing").Order("id").First(&quote).Error; err != nil {
		t.Fatal(err)
	}
	operatorValue := 1.2345
	if err := db.Model(&quote).Update("odds", operatorValue).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyDevelopmentAcceptanceProfile(db, "debug"); err == nil {
		t.Fatal("expected changed operator price to be rejected")
	}
	var unchanged odds.PlayLimit
	if err := db.First(&unchanged, quote.ID).Error; err != nil {
		t.Fatal(err)
	}
	if unchanged.Odds != operatorValue {
		t.Fatalf("operator price was overwritten: got %.4f want %.4f", unchanged.Odds, operatorValue)
	}
	if _, err := AuditDevelopmentAcceptanceProfile(db, "debug"); err == nil {
		t.Fatal("expected read-only audit to reject changed operator price")
	}
}
