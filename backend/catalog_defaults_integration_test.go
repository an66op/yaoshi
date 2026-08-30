package main

import (
	"backend/config"
	"backend/data/models/chat"
	"backend/data/models/lottery"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"backend/migrations"
	"backend/services"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// These tests run the real migration/bootstrap/services against an explicitly
// supplied EMPTY disposable PostgreSQL database. They never use config.yaml
// or the developer database, and roll back the entire schema at the end.
func catalogTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("BACKEND_CATALOG_TEST_DSN")
	if dsn == "" {
		t.Skip("set BACKEND_CATALOG_TEST_DSN to an empty local wangzhe_catalog_test database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Fatal("BACKEND_CATALOG_TEST_DSN must be a PostgreSQL URL")
	}
	if host := parsed.Hostname(); host != "localhost" && host != "127.0.0.1" && host != "::1" {
		t.Fatal("catalog integration tests only accept a loopback database host")
	}
	if parsed.Path != "/wangzhe_catalog_test" || parsed.Query().Get("host") != "" || parsed.Query().Get("dbname") != "" {
		t.Fatal("catalog integration tests require the dedicated wangzhe_catalog_test database without connection overrides")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("connect to disposable catalog database:", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	var tableCount int64
	if err := db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public'").Scan(&tableCount).Error; err != nil {
		t.Fatal(err)
	}
	if tableCount != 0 {
		t.Fatal("refusing to initialize a nonempty catalog test database")
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() {
		if err := tx.Rollback().Error; err != nil {
			t.Error("rollback disposable catalog schema:", err)
		}
	})
	previousConfig := config.Config
	config.Config = &config.Configuration{Server: config.ServerConfig{Mode: "test", Bind: "127.0.0.1", Port: 18089}}
	t.Cleanup(func() { config.Config = previousConfig })
	if err := migrations.Run(tx); err != nil {
		t.Fatal("fresh migrations:", err)
	}
	if _, err := services.CreateBootstrapAdmin(tx, services.BootstrapAdminInput{
		Username: "catalog_platform", Password: "FixtureInit#2026_a9Z", Nickname: "分类测试平台",
	}); err != nil {
		t.Fatal("bootstrap administrator:", err)
	}
	if err := services.Bootstrap(tx, services.BootstrapOptions{Mode: "test"}); err != nil {
		t.Fatal("fresh bootstrap:", err)
	}
	return tx
}

func catalogTestRoom(t *testing.T, db *gorm.DB, ownerID uint64) workspacemodel.Workspace {
	t.Helper()
	var room workspacemodel.Workspace
	if err := db.Where("owner_user_id = ?", ownerID).First(&room).Error; err != nil {
		t.Fatal(err)
	}
	return room
}

func catalogTestMember(t *testing.T, db *gorm.DB, room workspacemodel.Workspace, name string) user.User {
	t.Helper()
	member := user.User{
		Username: name, LoginScope: room.Scope, WorkspaceID: room.ID,
		Password: "test-only-no-login", Nickname: name, Role: "member", Status: 1, BalanceCents: 100_000,
	}
	if room.Type == workspacemodel.TypeAgent {
		member.ParentAgentID = &room.OwnerUserID
	} else {
		member.ParentTenantID = &room.OwnerUserID
	}
	if err := db.Create(&member).Error; err != nil {
		t.Fatal(err)
	}
	return member
}

func catalogDefaultPlacement(gameID string) (string, int) {
	for category, ids := range map[string][]string{
		"彩票":  {"speed-racing", "speed-fly", "speed-ssc", "sg-fly", "sg-ssc", "fly-racing", "au-lucky-5", "au-lucky-10"},
		"PC":  {"pc-canada", "canada-28", "canada-20"},
		"宾果":  {"bingo-mark-six", "bingo-racing-a", "bingo-racing-b", "bingo-ssc-1", "bingo-ssc-2", "bingo-ssc-3", "bingo-ssc-4"},
		"六合彩": {"hong-kong-mark-six", "happy8-mark-six", "new-macau-mark-six", "old-macau-mark-six"},
	} {
		for index, id := range ids {
			if id == gameID {
				return category, index + 1
			}
		}
	}
	return "", 0
}

func assertCatalogRoom(t *testing.T, db *gorm.DB, room workspacemodel.Workspace, wantOpen ...string) {
	t.Helper()
	views, err := services.NewWorkspaceGameService(db).List(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 30 {
		t.Fatalf("room %s has %d games, want 30", room.Scope, len(views))
	}
	wanted := make(map[string]bool, len(wantOpen))
	for _, id := range wantOpen {
		wanted[id] = true
	}
	var settings []chat.RoomGameSetting
	if err := db.Where("workspace_id = ?", room.ID).Find(&settings).Error; err != nil || len(settings) != 30 {
		t.Fatalf("room %s stored switches=%d, want 30: %v", room.Scope, len(settings), err)
	}
	for _, setting := range settings {
		if setting.Enabled != wanted[setting.GameID] {
			t.Errorf("room %s stored switch %s=%v, want %v", room.Scope, setting.GameID, setting.Enabled, wanted[setting.GameID])
		}
	}
	for _, game := range views {
		category, order := catalogDefaultPlacement(game.ID)
		if game.LobbyCategory != category || game.LobbySortOrder != order {
			t.Errorf("room %s game %s placement=(%q,%d), want (%q,%d)", room.Scope, game.ID, game.LobbyCategory, game.LobbySortOrder, category, order)
		}
		if game.RoomEnabled != wanted[game.ID] || game.Enabled != wanted[game.ID] {
			t.Errorf("room %s %s: raw=%v effective=%v, want %v", room.Scope, game.ID, game.RoomEnabled, game.Enabled, wanted[game.ID])
		}
		allowed, err := services.WorkspaceGameEnabled(db, room.ID, game.ID)
		if err != nil || allowed != game.Enabled {
			t.Errorf("room %s %s: direct check=%v err=%v does not match catalogue=%v", room.Scope, game.ID, allowed, err, game.Enabled)
		}
	}
}

func TestCatalogDefaultsFreshPostgres(t *testing.T) {
	db := catalogTestDatabase(t)
	var games []lottery.Game
	if err := db.Find(&games).Error; err != nil {
		t.Fatal(err)
	}
	if len(games) != 30 {
		t.Fatalf("fresh catalogue has %d games, want 30", len(games))
	}
	groups := map[string]int{}
	var official, enabled int
	for _, game := range games {
		category, order := catalogDefaultPlacement(game.ID)
		if game.LobbyCategory != category || game.LobbySortOrder != order {
			t.Errorf("fresh game %s placement=(%q,%d), want (%q,%d)", game.ID, game.LobbyCategory, game.LobbySortOrder, category, order)
		}
		groups[game.LobbyCategory]++
		if game.Enabled {
			enabled++
		}
		if game.SourceKind == "official" {
			official++
			if game.Enabled {
				t.Errorf("official game %s unexpectedly enabled", game.ID)
			}
		}
	}
	for name, count := range map[string]int{"彩票": 8, "宾果": 7, "PC": 3, "六合彩": 4, "": 8} {
		if groups[name] != count {
			t.Errorf("default category %s has %d games, want %d", name, groups[name], count)
		}
	}
	if official != 8 || enabled != 22 {
		t.Fatalf("fresh platform enabled=%d official=%d, want 22/8", enabled, official)
	}
	var drawCount int64
	if err := db.Model(&lottery.Draw{}).Count(&drawCount).Error; err != nil || drawCount != 0 {
		t.Fatalf("base configuration fabricated draw history: %d, %v", drawCount, err)
	}
	tenant, err := services.NewTenantAdminService(db).Create(services.TenantPayload{
		Username: "catalog_tenant", Password: "TenantTest#2026_a9", Nickname: "分类测试租户", RoomCode: "76201", Status: 1,
	})
	if err != nil {
		t.Fatal("create tenant:", err)
	}
	agent, err := services.NewAgentAdminService(db).CreateForTenant(tenant.ID, services.CreateAgentInput{
		Username: "catalog_agent", Password: "AgentTest#2026_a9", Nickname: "分类测试代理", RoomCode: "76202", Status: 1,
	})
	if err != nil {
		t.Fatal("create tenant agent:", err)
	}
	tenantRoom := catalogTestRoom(t, db, tenant.ID)
	agentRoom := catalogTestRoom(t, db, agent.ID)
	for _, room := range []workspacemodel.Workspace{tenantRoom, agentRoom} {
		assertCatalogRoom(t, db, room)
		member := catalogTestMember(t, db, room, fmt.Sprintf("catalog_member_%d", room.ID))
		memberGames, err := services.NewWorkspaceGameService(db).ListEnabledForMember(member.UserID)
		if err != nil || len(memberGames) != 0 {
			t.Fatalf("new room member has enabled games: %d, %v", len(memberGames), err)
		}
		chatService := services.NewMemberChatService(db)
		if _, err := chatService.List(member.UserID, "group", "speed-racing", 10, 0, 0); apperrors.GetErrorCode(err) != "LOTTERY_ROOM_CLOSED" {
			t.Fatalf("%s closed game chat returned %v", room.Type, err)
		}
		if _, err := chatService.Post(member.UserID, "group", "speed-racing", "closed fixture"); apperrors.GetErrorCode(err) != "LOTTERY_ROOM_CLOSED" {
			t.Fatalf("%s closed game post returned %v", room.Type, err)
		}
		if _, err := services.NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "speed-racing", true); err != nil {
			t.Fatal(err)
		}
		assertCatalogRoom(t, db, room, "speed-racing")
		memberGames, err = services.NewWorkspaceGameService(db).ListEnabledForMember(member.UserID)
		if err != nil || len(memberGames) != 1 || memberGames[0].ID != "speed-racing" {
			t.Fatalf("member did not see exactly the opened game: %+v, %v", memberGames, err)
		}
		if _, err := chatService.Post(member.UserID, "group", "speed-racing", "open fixture"); err != nil {
			t.Fatalf("%s opened game post: %v", room.Type, err)
		}
		messages, err := chatService.List(member.UserID, "group", "speed-racing", 10, 0, 0)
		if err != nil || len(messages.Items) != 1 || messages.Items[0].Content != "open fixture" {
			t.Fatalf("%s opened game chat: %+v, %v", room.Type, messages, err)
		}
		if room.Type == workspacemodel.TypeTenant {
			assertCatalogRoom(t, db, agentRoom) // A tenant opening its room never opens its agents' rooms.
		}
	}
	if err := services.EnsureWorkspaceGameDefaults(db, agentRoom); err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(db); err != nil {
		t.Fatal(err)
	}
	if err := services.Bootstrap(db, services.BootstrapOptions{Mode: "test"}); err != nil {
		t.Fatal("repeat bootstrap:", err)
	}
	assertCatalogRoom(t, db, agentRoom, "speed-racing")
	assertCatalogRoom(t, db, tenantRoom, "speed-racing")

	// Direct access must respect the same category/platform/room gates as the
	// member catalogue; a true room switch must never bypass another gate.
	for _, change := range []map[string]any{{"enabled": false}, {"enabled": true, "lobby_category": ""}} {
		if err := db.Model(&lottery.Game{}).Where("id = ?", "speed-racing").Updates(change).Error; err != nil {
			t.Fatal(err)
		}
		if allowed, err := services.WorkspaceGameEnabled(db, agentRoom.ID, "speed-racing"); err != nil || allowed {
			t.Fatalf("platform/category gate bypassed: %v, %v", allowed, err)
		}
	}
	if err := db.Model(&lottery.Game{}).Where("id = ?", "speed-racing").Update("lobby_category", "彩票").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&agentRoom).Update("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	for _, workspaceID := range []uint64{agentRoom.ID, 0, 999999} {
		if allowed, err := services.WorkspaceGameEnabled(db, workspaceID, "speed-racing"); err != nil || allowed {
			t.Fatalf("inactive/missing room %d bypassed: %v, %v", workspaceID, allowed, err)
		}
	}
	if err := db.Model(&agentRoom).Update("status", 1).Error; err != nil {
		t.Fatal(err)
	}
	futureGame := lottery.Game{ID: "catalog-future", Code: "CATALOG_FUTURE", Name: "新增测试彩", Category: "PK10", LobbyCategory: "彩票", Enabled: true, DrawInterval: 300, NextDrawAt: time.Now().Add(5 * time.Minute)}
	if err := db.Create(&futureGame).Error; err != nil {
		t.Fatal(err)
	}
	for _, room := range []workspacemodel.Workspace{tenantRoom, agentRoom} {
		if allowed, err := services.WorkspaceGameEnabled(db, room.ID, futureGame.ID); err != nil || allowed {
			t.Fatalf("future game silently opened in %s: %v, %v", room.Scope, allowed, err)
		}
	}
}

func TestCatalogDefaultsUpgradePostgres(t *testing.T) {
	t.Run("pre_default_migrations", testCatalogLegacyDefaultsUpgrade)
	t.Run("correct_legacy_official_placements", testCatalogOfficialDefaultsUpgrade)
	t.Run("preserve_operator_placements", testCatalogCustomDefaultsUpgrade)
}

func testCatalogLegacyDefaultsUpgrade(t *testing.T) {
	db := catalogTestDatabase(t)
	agent, err := services.NewAgentAdminService(db).Create(services.CreateAgentInput{
		Username: "legacy_catalog_agent", Password: "LegacyFixture#2026_a9", Nickname: "原有房间", RoomCode: "76203", Status: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	room := catalogTestRoom(t, db, agent.ID)
	// Reconstruct the pre-upgrade state ONLY inside this disposable, rollback-only
	// database: missing room switches meant on, and known games had no shelf.
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{"DELETE FROM schema_migrations WHERE version IN (?, ?, ?)", []any{"202608300002_default_lobby_assignments.sql", "202608300003_room_game_defaults.sql", "202608300005_default_lobby_scope.sql"}},
		{"DELETE FROM room_game_settings WHERE workspace_id = ?", []any{room.ID}},
		{"UPDATE lottery_games SET lobby_category = '', lobby_sort_order = 0, enabled = TRUE", nil},
		{"UPDATE lottery_games SET lobby_category = '自定义专区', lobby_sort_order = 77 WHERE id = 'speed-racing'", nil},
		{"UPDATE lottery_games SET lobby_sort_order = 37, enabled = FALSE WHERE id = 'speed-ssc'", nil},
		{"UPDATE lottery_lobby_categories SET deleted_at = CURRENT_TIMESTAMP WHERE name = 'PC'", nil},
	} {
		if err := db.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Cover both modern explicit switches and legacy agent-only ownership.
	for _, setting := range []chat.RoomGameSetting{
		{WorkspaceID: room.ID, AgentID: agent.ID, GameID: "speed-racing", Enabled: false},
		{WorkspaceID: 0, AgentID: agent.ID, GameID: "pc-canada", Enabled: true},
		{WorkspaceID: 0, AgentID: agent.ID, GameID: "speed-fly", Enabled: false},
	} {
		if err := db.Create(&setting).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := migrations.Run(db); err != nil {
		t.Fatal("upgrade migrations:", err)
	}
	if err := services.Bootstrap(db, services.BootstrapOptions{Mode: "test"}); err != nil {
		t.Fatal("upgrade bootstrap:", err)
	}
	var games []lottery.Game
	if err := db.Find(&games).Error; err != nil {
		t.Fatal(err)
	}
	for _, game := range games {
		if game.Enabled != (game.ID != "speed-ssc") {
			t.Errorf("upgrade changed platform switch for %s", game.ID)
		}
		switch game.ID {
		case "speed-racing":
			if game.LobbyCategory != "自定义专区" || game.LobbySortOrder != 77 {
				t.Errorf("upgrade overwrote custom placement: %+v", game)
			}
		case "pc-canada", "canada-28", "canada-20":
			if game.LobbyCategory != "" {
				t.Errorf("upgrade revived deleted PC category for %s", game.ID)
			}
		case "official-fc3d", "official-kl8", "official-pl3", "official-qxc", "official-tw-super-lotto", "official-tw-daily539", "official-tw-lotto649", "official-tw-bingo":
			if game.LobbyCategory != "" || game.LobbySortOrder != 0 {
				t.Errorf("upgrade retained excluded default classification for %s: (%q,%d)", game.ID, game.LobbyCategory, game.LobbySortOrder)
			}
		default:
			if game.LobbyCategory == "" {
				t.Errorf("upgrade left %s unclassified", game.ID)
			}
			if game.ID == "speed-ssc" && game.LobbySortOrder != 37 {
				t.Error("upgrade overwrote nonzero ordering")
			}
		}
	}
	var categories []lottery.LobbyCategory
	if err := db.Find(&categories).Error; err != nil || len(categories) != 3 {
		t.Fatalf("bootstrap resurrected deleted category: %d, %v", len(categories), err)
	}
	var settings []chat.RoomGameSetting
	if err := db.Where("workspace_id = ?", room.ID).Find(&settings).Error; err != nil || len(settings) != 30 {
		t.Fatalf("upgrade room defaults: %d, %v", len(settings), err)
	}
	for _, setting := range settings {
		if setting.Enabled != (setting.GameID != "speed-racing" && setting.GameID != "speed-fly") {
			t.Errorf("upgrade changed prior explicit/implicit switch: %+v", setting)
		}
	}
	// Owner choices AFTER the one-time repair must survive subsequent startup.
	if err := db.Model(&lottery.Game{}).Where("id = ?", "speed-fly").Updates(map[string]any{"lobby_category": "", "lobby_sort_order": 91, "enabled": false}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := services.NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "speed-ssc", false); err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(db); err != nil {
		t.Fatal(err)
	}
	if err := services.Bootstrap(db, services.BootstrapOptions{Mode: "test"}); err != nil {
		t.Fatal(err)
	}
	var edited lottery.Game
	if err := db.First(&edited, "id = ?", "speed-fly").Error; err != nil || edited.LobbyCategory != "" || edited.LobbySortOrder != 91 || edited.Enabled {
		t.Fatalf("restart overwrote operator placement: %+v, %v", edited, err)
	}
	var editedSwitch chat.RoomGameSetting
	if err := db.Where("workspace_id = ? AND game_id = ?", room.ID, "speed-ssc").First(&editedSwitch).Error; err != nil || editedSwitch.Enabled {
		t.Fatalf("restart reopened an explicitly closed game: %+v, %v", editedSwitch, err)
	}
	newAgent, err := services.NewAgentAdminService(db).Create(services.CreateAgentInput{
		Username: "after_catalog_upgrade", Password: "NewFixture#2026_a9", Nickname: "升级后新房间", RoomCode: "76204", Status: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	var opened int64
	if err := db.Model(&chat.RoomGameSetting{}).Where("workspace_id = ? AND enabled = TRUE", newAgent.WorkspaceID).Count(&opened).Error; err != nil || opened != 0 {
		t.Fatalf("post-upgrade room inherited legacy on default: %d, %v", opened, err)
	}
}

type catalogTestPlacement struct {
	category string
	order    int
}

func catalogLegacyOfficialPlacements() map[string]catalogTestPlacement {
	return map[string]catalogTestPlacement{
		"official-fc3d":           {"彩票", 9},
		"official-kl8":            {"彩票", 10},
		"official-pl3":            {"彩票", 11},
		"official-qxc":            {"彩票", 12},
		"official-tw-super-lotto": {"彩票", 13},
		"official-tw-daily539":    {"彩票", 14},
		"official-tw-lotto649":    {"彩票", 15},
		"official-tw-bingo":       {"宾果", 8},
	}
}

func catalogSwitchSnapshot(t *testing.T, db *gorm.DB) map[string]bool {
	t.Helper()
	var games []lottery.Game
	var settings []chat.RoomGameSetting
	if err := db.Find(&games).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Find(&settings).Error; err != nil {
		t.Fatal(err)
	}
	result := make(map[string]bool, len(games)+len(settings))
	for _, game := range games {
		result["platform/"+game.ID] = game.Enabled
	}
	for _, setting := range settings {
		result[fmt.Sprintf("room/%d/%s", setting.WorkspaceID, setting.GameID)] = setting.Enabled
	}
	return result
}

func catalogRunScopeUpgrade(t *testing.T, db *gorm.DB) {
	t.Helper()
	// Rewind this one migration only in the guarded, rollback-only test database.
	result := db.Exec("DELETE FROM schema_migrations WHERE version = ?", "202608300005_default_lobby_scope.sql")
	if result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("reconstruct pending scope migration: affected=%d err=%v", result.RowsAffected, result.Error)
	}
	if err := migrations.Run(db); err != nil {
		t.Fatal("scope upgrade migration:", err)
	}
	if err := services.Bootstrap(db, services.BootstrapOptions{Mode: "test"}); err != nil {
		t.Fatal("scope upgrade bootstrap:", err)
	}
}

func catalogCreateClosedRooms(t *testing.T, db *gorm.DB, suffix string) []workspacemodel.Workspace {
	t.Helper()
	codes, ok := map[string][2]string{
		"before": {"76210", "76211"},
		"after":  {"76212", "76213"},
		"custom": {"76214", "76215"},
	}[suffix]
	if !ok {
		t.Fatalf("unknown scope room fixture %q", suffix)
	}
	tenant, err := services.NewTenantAdminService(db).Create(services.TenantPayload{
		Username: "scope_tenant_" + suffix, Password: "TenantScope#2026_a9", Nickname: "默认分类租户", RoomCode: codes[0], Status: 1,
	})
	if err != nil {
		t.Fatal("create scope tenant:", err)
	}
	agent, err := services.NewAgentAdminService(db).CreateForTenant(tenant.ID, services.CreateAgentInput{
		Username: "scope_agent_" + suffix, Password: "AgentScope#2026_a9", Nickname: "默认分类代理", RoomCode: codes[1], Status: 1,
	})
	if err != nil {
		t.Fatal("create scope agent:", err)
	}
	rooms := []workspacemodel.Workspace{catalogTestRoom(t, db, tenant.ID), catalogTestRoom(t, db, agent.ID)}
	var catalog []lottery.Game
	if err := db.Find(&catalog).Error; err != nil || len(catalog) != 30 {
		t.Fatalf("scope catalog has %d games: %v", len(catalog), err)
	}
	byID := make(map[string]lottery.Game, len(catalog))
	for _, game := range catalog {
		byID[game.ID] = game
	}
	for _, room := range rooms {
		views, err := services.NewWorkspaceGameService(db).List(room.ID)
		if err != nil || len(views) != 30 {
			t.Fatalf("new %s catalog has %d games: %v", room.Type, len(views), err)
		}
		for _, view := range views {
			shared, ok := byID[view.ID]
			if !ok || view.LobbyCategory != shared.LobbyCategory || view.LobbySortOrder != shared.LobbySortOrder {
				t.Errorf("new %s did not inherit shared placement for %s", room.Type, view.ID)
			}
			if view.RoomEnabled || view.Enabled {
				t.Errorf("new %s silently opened %s", room.Type, view.ID)
			}
		}
		var switches []chat.RoomGameSetting
		if err := db.Where("workspace_id = ?", room.ID).Find(&switches).Error; err != nil || len(switches) != 30 {
			t.Fatalf("new %s stored switches=%d: %v", room.Type, len(switches), err)
		}
		for _, setting := range switches {
			if setting.Enabled {
				t.Errorf("new %s enabled stored switch %s", room.Type, setting.GameID)
			}
		}
	}
	return rooms
}

func testCatalogOfficialDefaultsUpgrade(t *testing.T) {
	db := catalogTestDatabase(t)
	for id, placement := range catalogLegacyOfficialPlacements() {
		if err := db.Model(&lottery.Game{}).Where("id = ?", id).Updates(map[string]any{
			"lobby_category": placement.category, "lobby_sort_order": placement.order, "enabled": placement.order%2 == 0,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, room := range catalogCreateClosedRooms(t, db, "before") {
		if err := db.Model(&chat.RoomGameSetting{}).Where("workspace_id = ? AND game_id IN ?", room.ID,
			[]string{"speed-racing", "official-kl8", "official-tw-bingo"}).Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
	}
	before := catalogSwitchSnapshot(t, db)
	catalogRunScopeUpgrade(t, db)
	var games []lottery.Game
	if err := db.Find(&games).Error; err != nil {
		t.Fatal(err)
	}
	for _, game := range games {
		category, order := catalogDefaultPlacement(game.ID)
		if game.LobbyCategory != category || game.LobbySortOrder != order {
			t.Errorf("scope migration game %s placement=(%q,%d), want (%q,%d)", game.ID, game.LobbyCategory, game.LobbySortOrder, category, order)
		}
	}
	if after := catalogSwitchSnapshot(t, db); !reflect.DeepEqual(after, before) {
		t.Fatal("classification-only migration changed platform or existing room switches")
	}
	// Later operator assignment remains authoritative; migration/bootstrap are not
	// recurring synchronization and new rooms still inherit the shared structure.
	if err := db.Model(&lottery.Game{}).Where("id = ?", "official-fc3d").Updates(map[string]any{
		"lobby_category": "彩票", "lobby_sort_order": 9,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(db); err != nil {
		t.Fatal(err)
	}
	if err := services.Bootstrap(db, services.BootstrapOptions{Mode: "test"}); err != nil {
		t.Fatal(err)
	}
	var edited lottery.Game
	if err := db.First(&edited, "id = ?", "official-fc3d").Error; err != nil || edited.LobbyCategory != "彩票" || edited.LobbySortOrder != 9 {
		t.Fatalf("restart reapplied scope repair over an operator choice: %+v, %v", edited, err)
	}
	if after := catalogSwitchSnapshot(t, db); !reflect.DeepEqual(after, before) {
		t.Fatal("repeat startup changed platform or existing room switches")
	}
	catalogCreateClosedRooms(t, db, "after")
}

func testCatalogCustomDefaultsUpgrade(t *testing.T) {
	db := catalogTestDatabase(t)
	placements := map[string]catalogTestPlacement{
		"official-fc3d":           {"自定义专区", 77}, // Custom shelf and order.
		"official-kl8":            {"彩票", 91},    // Same shelf, customized order.
		"official-pl3":            {"", 0},       // Explicitly unclassified.
		"official-qxc":            {"宾果", 12},    // Changed shelf, same old order.
		"official-tw-super-lotto": {"彩票", 0},
		"official-tw-daily539":    {"彩票", 14}, // The only exact legacy tuple here.
		"official-tw-lotto649":    {"六合彩", 15},
		"official-tw-bingo":       {"宾果", 0},
	}
	for id, placement := range placements {
		if err := db.Model(&lottery.Game{}).Where("id = ?", id).Updates(map[string]any{
			"lobby_category": placement.category, "lobby_sort_order": placement.order, "enabled": placement.order%2 == 0,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	before := catalogSwitchSnapshot(t, db)
	catalogRunScopeUpgrade(t, db)
	placements["official-tw-daily539"] = catalogTestPlacement{"", 0}
	for id, want := range placements {
		var game lottery.Game
		if err := db.First(&game, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		if game.LobbyCategory != want.category || game.LobbySortOrder != want.order {
			t.Errorf("scope migration overwrote operator placement for %s: got (%q,%d), want (%q,%d)", id, game.LobbyCategory, game.LobbySortOrder, want.category, want.order)
		}
	}
	if after := catalogSwitchSnapshot(t, db); !reflect.DeepEqual(after, before) {
		t.Fatal("custom-placement migration changed platform or room switches")
	}
	catalogCreateClosedRooms(t, db, "custom")
}
