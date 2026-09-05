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
	"time"
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

func TestDevelopmentOddsInventoryAcceptsOnlyCanonicalInertForeignPlaceholders(t *testing.T) {
	profileGames := map[string]struct{}{"speed-racing": {}}
	configuredAt := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	activeProfile := odds.PlayLimit{
		ID: 1, GameID: "speed-racing", PlayCode: "sum", Odds: 9.9,
		ExplicitlyConfigured: true, RuleVersion: "racing-v2",
		ConfigurationSource: oddsSourceAdminSave, ConfiguredAt: &configuredAt,
	}
	legacyForeign := odds.PlayLimit{
		ID: 2, GameID: "official-fc3d", PlayCode: "sum", Odds: 50,
		ConfigurationSource: oddsSourceUnconfigured,
	}

	inventory, err := inspectDevelopmentOddsInventory([]odds.PlayLimit{activeProfile, legacyForeign}, profileGames)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.TotalRows != 2 || inventory.ProfileRows != 1 || inventory.NonInertRows != 1 || inventory.ForeignNonInertRows != 0 {
		t.Fatalf("unexpected inventory: %+v", inventory)
	}

	tests := []struct {
		name   string
		mutate func(*odds.PlayLimit)
	}{
		{"explicit confirmation", func(row *odds.PlayLimit) { row.ExplicitlyConfigured = true }},
		{"stale rule version", func(row *odds.PlayLimit) { row.RuleVersion = "digits3-v1" }},
		{"noncanonical source", func(row *odds.PlayLimit) { row.ConfigurationSource = "legacy_default" }},
		{"blank source", func(row *odds.PlayLimit) { row.ConfigurationSource = "" }},
		{"configuration timestamp", func(row *odds.PlayLimit) { row.ConfiguredAt = &configuredAt }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			row := legacyForeign
			test.mutate(&row)
			got, err := inspectDevelopmentOddsInventory([]odds.PlayLimit{row}, profileGames)
			if err != nil {
				t.Fatal(err)
			}
			if got.ForeignNonInertRows != 1 || got.NonInertRows != 1 {
				t.Fatalf("unsafe foreign row was accepted as inert: %+v / %+v", row, got)
			}
		})
	}
}

func TestDevelopmentOddsInventoryRejectsDuplicateGamePlayRows(t *testing.T) {
	row := odds.PlayLimit{GameID: "official-fc3d", PlayCode: "sum", ConfigurationSource: oddsSourceUnconfigured}
	if _, err := inspectDevelopmentOddsInventory([]odds.PlayLimit{row, row}, map[string]struct{}{}); err == nil {
		t.Fatal("duplicate game/play rows were accepted")
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
	// Simulate an upgrade from the pre-confirmation schema. Its numeric value
	// is intentionally preserved, but the explicit activation metadata keeps
	// it unavailable and must not prevent the first local profile install.
	legacyPlaceholder := odds.PlayLimit{
		GameID: "official-fc3d", PlayCode: "sum", PlayName: "旧总和", Odds: 50,
		MinBet: 1, MaxBet: 1000, MaxUserPeriod: 1000, MaxPeriodTotal: 10000,
		ConfigurationSource: oddsSourceUnconfigured,
	}
	if err := db.Create(&legacyPlaceholder).Error; err != nil {
		t.Fatal("insert isolated inert legacy placeholder:", err)
	}

	first, err := ApplyDevelopmentAcceptanceProfile(db, "debug")
	if err != nil {
		t.Fatal("apply fresh development profile:", err)
	}
	if first.HumanAccounts != 4 || first.RobotAccounts != 30 || first.Workspaces != 3 ||
		first.ActiveAccounts != 34 || first.ActiveMemberships != 34 || first.ConfiguredGames != 22 || first.ConfiguredPlayQuotes != 1437 ||
		first.AgentRoomCode != demoRoomCode || first.AgentRoomOpenGames != 22 ||
		first.AgentRoomRobotQuota != MaxWorkspaceRobotQuota || first.AgentRoomRobots != MaxWorkspaceRobotQuota {
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
	var preserved odds.PlayLimit
	if err := db.First(&preserved, legacyPlaceholder.ID).Error; err != nil {
		t.Fatal("read preserved inert legacy placeholder:", err)
	}
	if preserved.Odds != legacyPlaceholder.Odds || preserved.ExplicitlyConfigured || preserved.RuleVersion != "" ||
		preserved.ConfigurationSource != oddsSourceUnconfigured || preserved.ConfiguredAt != nil {
		t.Fatalf("inert legacy placeholder was rewritten or activated: %+v", preserved)
	}
	var allOddsRows int64
	if err := db.Model(&odds.PlayLimit{}).Count(&allOddsRows).Error; err != nil {
		t.Fatal(err)
	}
	if allOddsRows != int64(first.ConfiguredPlayQuotes)+1 {
		t.Fatalf("physical odds rows = %d, want %d configured plus one inert placeholder", allOddsRows, first.ConfiguredPlayQuotes)
	}

	if err := db.Model(&legacyPlaceholder).Update("configuration_source", "legacy_default").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := AuditDevelopmentAcceptanceProfile(db, "debug"); err == nil {
		t.Fatal("expected audit to reject a noncanonical out-of-profile legacy row")
	}
	if err := db.Model(&legacyPlaceholder).Update("configuration_source", oddsSourceUnconfigured).Error; err != nil {
		t.Fatal(err)
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
