package services

import (
	"backend/config"
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/data/models/settings"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"backend/migrations"
	"encoding/json"
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

// These opt-in tests use a fresh, dedicated loopback database. They never read
// the developer's config.yaml connection or persist their schema/fixture data.
// Do not run them in parallel: config.Config is intentionally scoped to a test.
func timingPostgresDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("BACKEND_TIMING_TEST_DSN")
	if dsn == "" {
		t.Skip("set BACKEND_TIMING_TEST_DSN to an empty local wangzhe_timing_test database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Fatal("BACKEND_TIMING_TEST_DSN must be a PostgreSQL URL")
	}
	if host := parsed.Hostname(); host != "localhost" && host != "127.0.0.1" && host != "::1" {
		t.Fatal("timing integration tests only accept a loopback database host")
	}
	if parsed.Path != "/wangzhe_timing_test" || parsed.Fragment != "" {
		t.Fatal("timing integration tests require the dedicated wangzhe_timing_test database")
	}
	// A URL query must not override the dedicated host/database or load a
	// service definition pointing at a different database.
	for key := range parsed.Query() {
		if key != "sslmode" && key != "connect_timeout" {
			t.Fatal("timing test DSN only permits sslmode and connect_timeout query options")
		}
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("connect to disposable timing database:", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	var tables int64
	if err := db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public'").Scan(&tables).Error; err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Fatal("refusing to initialize a nonempty timing test database")
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() {
		if err := tx.Rollback().Error; err != nil {
			t.Error("rollback disposable timing schema:", err)
		}
		var remaining int64
		if err := db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public'").Scan(&remaining).Error; err != nil {
			t.Error(err)
		} else if remaining != 0 {
			t.Errorf("test schema did not roll back: %d tables remain", remaining)
		}
	})
	previousConfig := config.Config
	config.Config = &config.Configuration{Server: config.ServerConfig{Mode: "test", Bind: "127.0.0.1", Port: 18089}}
	t.Cleanup(func() { config.Config = previousConfig })
	if err := migrations.Run(tx); err != nil {
		t.Fatal("fresh timing migrations:", err)
	}
	if _, err := CreateBootstrapAdmin(tx, BootstrapAdminInput{
		Username: "timing_platform", Password: "TimingFixture#2026_a9Z", Nickname: "计时测试平台",
	}); err != nil {
		t.Fatal("bootstrap timing administrator:", err)
	}
	if err := Bootstrap(tx, BootstrapOptions{Mode: "test"}); err != nil {
		t.Fatal("fresh timing bootstrap:", err)
	}
	return tx
}

func timingPostgresRoom(t *testing.T, db *gorm.DB, name, roomCode string) workspacemodel.Workspace {
	t.Helper()
	owner, err := NewTenantAdminService(db).Create(TenantPayload{
		Username: name, Password: "TimingFixture#2026_a9Z", Nickname: "计时测试租户", RoomCode: roomCode, Status: 1,
	})
	if err != nil {
		t.Fatal("create isolated timing room:", err)
	}
	var room workspacemodel.Workspace
	if err := db.Where("owner_user_id = ?", owner.ID).First(&room).Error; err != nil {
		t.Fatal(err)
	}
	return room
}

func timingPostgresSettings(t *testing.T, db *gorm.DB, workspaceID uint64, raw string) {
	t.Helper()
	result := db.Model(&settings.SystemConfig{}).Where("workspace_id = ?", workspaceID).Update("game_settings_json", raw)
	if result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("update isolated timing settings: affected=%d, error=%v", result.RowsAffected, result.Error)
	}
}

func timingPostgresWindow(t *testing.T, db *gorm.DB, workspaceID uint64, game *lottery.Game, issue string, drawAt time.Time) *lottery.IssueWindow {
	t.Helper()
	raw, actualWorkspaceID, err := readTimingSettings(db, workspaceID)
	if err != nil {
		t.Fatal("read room timing settings:", err)
	}
	if actualWorkspaceID != workspaceID {
		t.Fatalf("timing settings crossed workspaces: got %d, want %d", actualWorkspaceID, workspaceID)
	}
	window, err := ensureIssueWindow(db, workspaceID, game, issue, drawAt, raw)
	if err != nil {
		t.Fatal("materialize room issue window:", err)
	}
	return window
}

func TestLotteryTimingPostgresFreshBootstrap(t *testing.T) {
	db := timingPostgresDatabase(t)
	if err := migrations.VerifyApplied(db); err != nil {
		t.Fatal(err)
	}
	type migrationStamp struct {
		Version   string
		Checksum  string
		AppliedAt time.Time
	}
	var before, after []migrationStamp
	if err := db.Table("schema_migrations").Order("version").Find(&before).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(db); err != nil {
		t.Fatal("repeat timing migrations:", err)
	}
	if err := Bootstrap(db, BootstrapOptions{Mode: "test"}); err != nil {
		t.Fatal("repeat timing bootstrap:", err)
	}
	if err := migrations.VerifyApplied(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Table("schema_migrations").Order("version").Find(&after).Error; err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("repeat migrations changed inventory: %d -> %d", len(before), len(after))
	}
	for i := range before {
		if before[i].Version != after[i].Version || before[i].Checksum != after[i].Checksum || !before[i].AppliedAt.Equal(after[i].AppliedAt) {
			t.Fatalf("repeat migrations rewrote metadata for %s", before[i].Version)
		}
	}
	for _, table := range []string{"lottery_draws", "lottery_bets", "lottery_issue_windows"} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("fresh bootstrap fabricated %s records: %d, %v", table, count, err)
		}
	}
	var platform workspacemodel.Workspace
	if err := db.Where("type = ?", workspacemodel.TypePlatform).First(&platform).Error; err != nil {
		t.Fatal(err)
	}
	_, platformID, err := readTimingSettings(db, 0)
	if err != nil || platformID != platform.ID {
		t.Fatalf("default timing settings did not resolve the platform: id=%d, error=%v", platformID, err)
	}
	t.Run("platform first issue is tied to schedule not the read clock", func(t *testing.T) {
		drawAt := time.Date(2026, 8, 30, 6, 30, 0, 0, time.UTC)
		if err := db.Model(&lottery.Game{}).Where("id = ?", "pc-canada").Updates(map[string]any{
			"source_kind": "platform", "next_issue": "", "next_draw_at": drawAt,
		}).Error; err != nil {
			t.Fatal(err)
		}
		for attempt := 0; attempt < 2; attempt++ {
			issue, err := NewBetAdminService(db).CurrentIssue("pc-canada")
			if err != nil || issue != "20260830143000" {
				t.Fatalf("first platform issue drifted from its fixed schedule: issue=%q, err=%v", issue, err)
			}
		}
	})
	t.Run("external feed without history does not invent a period", func(t *testing.T) {
		if err := db.Model(&lottery.Game{}).Where("id = ?", "speed-fly").Updates(map[string]any{
			"source_kind": "external", "next_issue": "",
			"next_draw_at": time.Now().UTC().Add(time.Hour),
		}).Error; err != nil {
			t.Fatal(err)
		}
		issue, err := NewBetAdminService(db).CurrentIssue("speed-fly")
		if err != nil || issue != "" {
			t.Fatalf("missing external result fabricated issue %q: %v", issue, err)
		}
		status, err := NewBetAssistantService(db).Status("speed-fly")
		if err != nil || status.Issue != "" || status.Accepting || !status.NextDrawAt.IsZero() || status.IssueStatus != lottery.IssueStatusAwaiting {
			t.Fatalf("missing external schedule was advertised as accepting: status=%+v, err=%v", status, err)
		}
		var count int64
		if err := db.Model(&lottery.Issue{}).Where("game_id = ?", "speed-fly").Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("external status read fabricated lifecycle rows: %d, %v", count, err)
		}
	})
}

func TestLotteryTimingPostgresRoomWindows(t *testing.T) {
	db := timingPostgresDatabase(t)
	roomA := timingPostgresRoom(t, db, "timing_room_a", "76301")
	roomB := timingPostgresRoom(t, db, "timing_room_b", "76302")
	timingPostgresSettings(t, db, roomA.ID, `{"seal_seconds":30,"game_timing_overrides":{"speed-fly":{"seal_seconds":45}}}`)
	timingPostgresSettings(t, db, roomB.ID, `{"seal_seconds":10}`)
	game := &lottery.Game{ID: "speed-fly", DrawInterval: 75}
	drawAt := time.Date(2026, 8, 30, 6, 30, 0, 0, time.UTC)

	t.Run("room and per-game settings stay isolated", func(t *testing.T) {
		a := timingPostgresWindow(t, db, roomA.ID, game, "70001", drawAt)
		b := timingPostgresWindow(t, db, roomB.ID, game, "70001", drawAt)
		if a.ID == b.ID || a.SealSeconds != 45 || b.SealSeconds != 10 ||
			!a.SealAt.Equal(drawAt.Add(-45*time.Second)) || !b.SealAt.Equal(drawAt.Add(-10*time.Second)) {
			t.Fatalf("room cutoffs were mixed: a=%+v, b=%+v", a, b)
		}
		if !a.AcceptAt.Equal(drawAt.Add(-75*time.Second)) || !b.AcceptAt.Equal(a.AcceptAt) {
			t.Fatal("acceptance time does not start at the shared draw interval boundary")
		}
		otherGame := &lottery.Game{ID: "speed-racing", DrawInterval: 75}
		other := timingPostgresWindow(t, db, roomA.ID, otherGame, "70001", drawAt)
		if other.SealSeconds != 30 {
			t.Fatalf("game-specific seal leaked into another game: %d", other.SealSeconds)
		}
	})

	t.Run("zero seal goes directly from accepting to awaiting", func(t *testing.T) {
		timingPostgresSettings(t, db, roomB.ID, `{"seal_seconds":0}`)
		window := timingPostgresWindow(t, db, roomB.ID, game, "70002", drawAt)
		if window.SealSeconds != 0 || !window.SealAt.Equal(drawAt) {
			t.Fatalf("explicit zero seal was replaced with a default: %+v", window)
		}
		if got := windowStatus(window, drawAt.Add(-time.Nanosecond)); got != lottery.IssueStatusAccepting {
			t.Fatalf("before draw with zero seal: %s", got)
		}
		if got := windowStatus(window, drawAt); got != lottery.IssueStatusAwaiting {
			t.Fatalf("at draw with zero seal: %s", got)
		}
	})

	t.Run("existing issue never reopens when settings or upstream move later", func(t *testing.T) {
		timingPostgresSettings(t, db, roomB.ID, `{"seal_seconds":30}`)
		original := timingPostgresWindow(t, db, roomB.ID, game, "70003", drawAt)
		timingPostgresSettings(t, db, roomB.ID, `{"seal_seconds":0}`)
		later := timingPostgresWindow(t, db, roomB.ID, game, "70003", drawAt.Add(75*time.Second))
		if later.ID != original.ID || !later.SealAt.Equal(original.SealAt) ||
			!later.ScheduledDrawAt.Equal(original.ScheduledDrawAt) || !later.AcceptAt.Equal(original.AcceptAt) {
			t.Fatalf("existing issue's time window was extended: before=%+v, after=%+v", original, later)
		}
		if got := windowStatus(later, drawAt.Add(-10*time.Second)); got != lottery.IssueStatusSealed {
			t.Fatalf("closed issue reopened after settings changed: %s", got)
		}
		if got := windowStatus(later, drawAt.Add(time.Second)); got != lottery.IssueStatusAwaiting {
			t.Fatalf("expired issue was moved into a future slot: %s", got)
		}
		next := timingPostgresWindow(t, db, roomB.ID, game, "70004", drawAt.Add(75*time.Second))
		if next.SealSeconds != 0 || !next.SealAt.Equal(next.ScheduledDrawAt) {
			t.Fatalf("new issue did not take the updated setting: %+v", next)
		}
	})

	t.Run("an earlier cutoff shortens the persisted window", func(t *testing.T) {
		timingPostgresSettings(t, db, roomB.ID, `{"seal_seconds":10}`)
		original := timingPostgresWindow(t, db, roomB.ID, game, "70005", drawAt)
		timingPostgresSettings(t, db, roomB.ID, `{"seal_seconds":45}`)
		shortened := timingPostgresWindow(t, db, roomB.ID, game, "70005", drawAt)
		if shortened.ID != original.ID || !shortened.SealAt.Equal(drawAt.Add(-45*time.Second)) || shortened.SealSeconds != 45 {
			t.Fatalf("earlier cutoff did not persist: %+v", shortened)
		}
		timingPostgresSettings(t, db, roomB.ID, `{"seal_seconds":10}`)
		refreshed := timingPostgresWindow(t, db, roomB.ID, game, "70005", drawAt)
		if !refreshed.SealAt.Equal(shortened.SealAt) {
			t.Fatal("refresh undid an already shortened cutoff")
		}
		var stored lottery.IssueWindow
		if err := db.First(&stored, shortened.ID).Error; err != nil {
			t.Fatal(err)
		}
		if !stored.SealAt.Equal(shortened.SealAt) || stored.SealSeconds != shortened.SealSeconds {
			t.Fatalf("returned window disagrees with persisted cutoff: %+v", stored)
		}
	})

	t.Run("every clock boundary is deterministic", func(t *testing.T) {
		timingPostgresSettings(t, db, roomB.ID, `{"seal_seconds":30}`)
		window := timingPostgresWindow(t, db, roomB.ID, game, "70006", drawAt)
		for _, test := range []struct {
			name string
			now  time.Time
			want string
		}{
			{"before accepting", window.AcceptAt.Add(-time.Nanosecond), lottery.IssueStatusPending},
			{"accept boundary", window.AcceptAt, lottery.IssueStatusAccepting},
			{"last accepting instant", window.SealAt.Add(-time.Nanosecond), lottery.IssueStatusAccepting},
			{"seal boundary", window.SealAt, lottery.IssueStatusSealed},
			{"last sealed instant", drawAt.Add(-time.Nanosecond), lottery.IssueStatusSealed},
			{"draw boundary", drawAt, lottery.IssueStatusAwaiting},
			{"overdue draw", drawAt.Add(time.Hour), lottery.IssueStatusAwaiting},
		} {
			t.Run(test.name, func(t *testing.T) {
				if got := windowStatus(window, test.now); got != test.want {
					t.Fatalf("status=%s, want %s", got, test.want)
				}
			})
		}
	})
}

func timingPostgresMember(t *testing.T, db *gorm.DB, room workspacemodel.Workspace, name string) user.User {
	t.Helper()
	member := user.User{
		Username: name, LoginScope: room.Scope, WorkspaceID: room.ID,
		Password: "fixture-no-login", Nickname: name, Role: "member", Status: 1,
		BalanceCents: 100_000, ParentTenantID: &room.OwnerUserID,
	}
	if err := db.Create(&member).Error; err != nil {
		t.Fatal(err)
	}
	return member
}

type timingFinancialSnapshot struct {
	BalanceCents, Bets, Pending, Cancelled, LedgerRows int64
}

func timingPostgresMoney(t *testing.T, db *gorm.DB, memberID uint64) timingFinancialSnapshot {
	t.Helper()
	var snapshot timingFinancialSnapshot
	var account user.User
	if err := db.First(&account, memberID).Error; err != nil {
		t.Fatal(err)
	}
	snapshot.BalanceCents = account.BalanceCents
	for _, query := range []struct {
		db    *gorm.DB
		count *int64
	}{
		{db.Model(&bet.Bet{}).Where("user_id = ?", memberID), &snapshot.Bets},
		{db.Model(&bet.Bet{}).Where("user_id = ? AND status = ?", memberID, "pending"), &snapshot.Pending},
		{db.Model(&bet.Bet{}).Where("user_id = ? AND status = ?", memberID, "cancelled"), &snapshot.Cancelled},
		{db.Model(&user.BalanceTransaction{}).Where("user_id = ?", memberID), &snapshot.LedgerRows},
	} {
		if err := query.db.Count(query.count).Error; err != nil {
			t.Fatal(err)
		}
	}
	return snapshot
}

func timingPostgresSchedule(t *testing.T, db *gorm.DB, issue string) *lottery.Game {
	t.Helper()
	var game lottery.Game
	if err := db.First(&game, "id = ?", "speed-fly").Error; err != nil {
		t.Fatal(err)
	}
	// Financial scenarios opt in to an explicit administrator quote once.
	// Repeated schedules must not reopen a market deliberately cleared later.
	if game.OddsConfigRevision == 0 {
		configureTestGameOdds(t, db, game.ID, map[string]float64{
			"ball_1_5": 9.9, "two_sided": 1.993, "dragon_tiger": 1.993, "sum": 1.993,
		})
		game.OddsConfigRevision = 1
	}
	// Give these financial tests ample real-clock headroom; 75-second exact
	// cadence and instant boundary behavior are covered by the window tests.
	game.NextIssue = issue
	game.NextDrawAt = time.Now().UTC().Truncate(time.Second).Add(10 * time.Minute)
	game.DrawInterval = 3600
	if err := db.Model(&game).Updates(map[string]any{
		"next_issue": game.NextIssue, "next_draw_at": game.NextDrawAt,
		"draw_interval": game.DrawInterval, "timing_source": "upstream",
		"enabled": true, "sync_status": "ok", "last_sync_error": "",
	}).Error; err != nil {
		t.Fatal(err)
	}
	return &game
}

func TestLotteryTimingPostgresBetAndCancelGates(t *testing.T) {
	db := timingPostgresDatabase(t)
	roomA := timingPostgresRoom(t, db, "timing_gate_a", "76311")
	roomB := timingPostgresRoom(t, db, "timing_gate_b", "76312")
	for _, room := range []workspacemodel.Workspace{roomA, roomB} {
		if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "speed-fly", true); err != nil {
			t.Fatal(err)
		}
	}
	_, platformID, err := readTimingSettings(db, 0)
	if err != nil {
		t.Fatal(err)
	}
	// The platform has already sealed 15 minutes before the shared draw, while
	// the tested room can still accept until its own configured cutoff.
	timingPostgresSettings(t, db, platformID, `{"seal_seconds":900}`)
	service := NewBetAdminService(db)
	service.suppressNotifications = true
	input := func(game *lottery.Game, member user.User, position int) PlaceBetInput {
		return PlaceBetInput{
			GameID: game.ID, Issue: game.NextIssue, UserID: member.UserID,
			PlayCode: "ball_1_5", PlayName: "指定名次号码", Position: position,
			Selection: "4", Amount: 2, Operator: "timing-integration",
		}
	}
	assertClosedWithoutMoneyChange := func(t *testing.T, memberID uint64, action func() error) {
		t.Helper()
		before := timingPostgresMoney(t, db, memberID)
		if err := action(); apperrors.GetErrorCode(err) != "ISSUE_CLOSED" {
			t.Fatalf("closed room gate returned %v, want ISSUE_CLOSED", err)
		}
		if after := timingPostgresMoney(t, db, memberID); after != before {
			t.Fatalf("rejected operation changed money/orders: before=%+v, after=%+v", before, after)
		}
	}

	t.Run("platform seal does not override room placement windows", func(t *testing.T) {
		game := timingPostgresSchedule(t, db, "71001")
		timingPostgresSettings(t, db, roomA.ID, `{"seal_seconds":1200}`)
		timingPostgresSettings(t, db, roomB.ID, `{"seal_seconds":30}`)
		a := timingPostgresMember(t, db, roomA, "timing_gate_member_a")
		b := timingPostgresMember(t, db, roomB, "timing_gate_member_b")
		if _, err := service.Place(input(game, b, 1)); err != nil {
			t.Fatal("open room rejected a single bet after platform seal:", err)
		}
		if rows, err := service.PlaceBatch([]PlaceBetInput{input(game, b, 2), input(game, b, 3)}); err != nil || len(rows) != 2 {
			t.Fatalf("open room rejected a batch after platform seal: rows=%d, err=%v", len(rows), err)
		}
		if state := timingPostgresMoney(t, db, b.UserID); state.BalanceCents != 99_400 || state.Pending != 3 || state.LedgerRows != 2 {
			t.Fatalf("accepted single+batch financial state is incorrect: %+v", state)
		}
		assertClosedWithoutMoneyChange(t, a.UserID, func() error {
			_, err := service.Place(input(game, a, 1))
			return err
		})
		assertClosedWithoutMoneyChange(t, a.UserID, func() error {
			_, err := service.PlaceBatch([]PlaceBetInput{input(game, a, 2), input(game, a, 3)})
			return err
		})
	})

	t.Run("member refunds use the bet workspace after a room switch", func(t *testing.T) {
		game := timingPostgresSchedule(t, db, "71002")
		timingPostgresSettings(t, db, roomA.ID, `{"seal_seconds":30}`)
		timingPostgresSettings(t, db, roomB.ID, `{"seal_seconds":0}`)
		member := timingPostgresMember(t, db, roomA, "timing_switch_member")
		placed, err := service.Place(input(game, member, 1))
		if err != nil {
			t.Fatal(err)
		}
		timingPostgresSettings(t, db, roomA.ID, `{"seal_seconds":1200}`)
		timingPostgresWindow(t, db, roomA.ID, game, game.NextIssue, game.NextDrawAt)
		if err := db.Model(&user.User{}).Where("user_id = ?", member.UserID).Updates(map[string]any{
			"workspace_id": roomB.ID, "login_scope": roomB.Scope, "parent_tenant_id": roomB.OwnerUserID,
		}).Error; err != nil {
			t.Fatal(err)
		}
		assertClosedWithoutMoneyChange(t, member.UserID, func() error {
			_, err := service.CancelOwned(placed.ID, member.UserID, "timing-integration")
			return err
		})
		assertClosedWithoutMoneyChange(t, member.UserID, func() error {
			_, err := service.CancelCurrentIssue(member.UserID, game.ID, "timing-integration")
			return err
		})
	})

	t.Run("open room cancellation refunds once", func(t *testing.T) {
		game := timingPostgresSchedule(t, db, "71003")
		timingPostgresSettings(t, db, roomB.ID, `{"seal_seconds":0}`)
		member := timingPostgresMember(t, db, roomB, "timing_refund_member")
		placed, err := service.PlaceBatch([]PlaceBetInput{input(game, member, 1), input(game, member, 2), input(game, member, 3)})
		if err != nil || len(placed) != 3 {
			t.Fatalf("place refund fixtures: %v", err)
		}
		cancelled, err := service.CancelOwned(placed[0].ID, member.UserID, "timing-integration")
		if err != nil || cancelled.Status != "cancelled" {
			t.Fatalf("open room single refund rejected: %v", err)
		}
		afterSingle := timingPostgresMoney(t, db, member.UserID)
		if _, err := service.CancelOwned(placed[0].ID, member.UserID, "timing-integration"); apperrors.GetErrorCode(err) != "INVALID_REQUEST" {
			t.Fatalf("duplicate single refund was not rejected: %v", err)
		}
		if after := timingPostgresMoney(t, db, member.UserID); after != afterSingle {
			t.Fatalf("duplicate cancellation changed financial state: %+v -> %+v", afterSingle, after)
		}
		bulk, err := service.CancelCurrentIssue(member.UserID, game.ID, "timing-integration")
		if err != nil || bulk.Count != 2 || bulk.Refund != 4 {
			t.Fatalf("open room bulk refund rejected or incorrect: result=%+v, err=%v", bulk, err)
		}
		final := timingPostgresMoney(t, db, member.UserID)
		if final.BalanceCents != member.BalanceCents || final.Pending != 0 || final.Cancelled != 3 || final.LedgerRows != 3 {
			t.Fatalf("refunds did not balance exactly: %+v", final)
		}
		if _, err := service.CancelCurrentIssue(member.UserID, game.ID, "timing-integration"); apperrors.GetErrorCode(err) != "NO_PENDING_BETS" {
			t.Fatalf("empty repeat bulk refund returned %v", err)
		}
		if after := timingPostgresMoney(t, db, member.UserID); after != final {
			t.Fatalf("repeat bulk refund changed financial state: %+v -> %+v", final, after)
		}
	})

	t.Run("lowering seal settings cannot reopen actual bet and cancel gates", func(t *testing.T) {
		game := timingPostgresSchedule(t, db, "71004")
		timingPostgresSettings(t, db, roomA.ID, `{"seal_seconds":30}`)
		member := timingPostgresMember(t, db, roomA, "timing_frozen_member")
		placed, err := service.Place(input(game, member, 1))
		if err != nil {
			t.Fatal(err)
		}
		timingPostgresSettings(t, db, roomA.ID, `{"seal_seconds":1200}`)
		timingPostgresWindow(t, db, roomA.ID, game, game.NextIssue, game.NextDrawAt)
		timingPostgresSettings(t, db, roomA.ID, `{"seal_seconds":0}`)
		if err := db.Model(&lottery.Game{}).Where("id = ?", game.ID).Update("next_draw_at", game.NextDrawAt.Add(time.Hour)).Error; err != nil {
			t.Fatal(err)
		}
		assertClosedWithoutMoneyChange(t, member.UserID, func() error {
			_, err := service.Place(input(game, member, 2))
			return err
		})
		assertClosedWithoutMoneyChange(t, member.UserID, func() error {
			_, err := service.CancelOwned(placed.ID, member.UserID, "timing-integration")
			return err
		})
	})
}

func TestLotteryTimingPostgresRoomAPIConsistency(t *testing.T) {
	db := timingPostgresDatabase(t)
	roomA := timingPostgresRoom(t, db, "timing_api_a", "76321")
	roomB := timingPostgresRoom(t, db, "timing_api_b", "76322")
	a := timingPostgresMember(t, db, roomA, "timing_api_member_a")
	b := timingPostgresMember(t, db, roomB, "timing_api_member_b")
	for _, room := range []workspacemodel.Workspace{roomA, roomB} {
		if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "speed-fly", true); err != nil {
			t.Fatal(err)
		}
	}
	_, platformID, err := readTimingSettings(db, 0)
	if err != nil {
		t.Fatal(err)
	}
	timingPostgresSettings(t, db, platformID, `{"seal_seconds":60}`)
	timingPostgresSettings(t, db, roomA.ID, `{"seal_seconds":45}`)
	timingPostgresSettings(t, db, roomB.ID, `{"seal_seconds":10}`)
	game := timingPostgresSchedule(t, db, "73001")
	game.DrawInterval = 75
	game.NextDrawAt = time.Now().UTC().Truncate(time.Second).Add(40 * time.Second)
	if err := db.Model(&lottery.Game{}).Where("id = ?", game.ID).Updates(map[string]any{
		"draw_interval": game.DrawInterval, "next_draw_at": game.NextDrawAt,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&lottery.Draw{
		GameID: game.ID, Issue: "73000", Numbers: "1,2,3,4,5,6,7,8,9,10",
		DrawAt: game.NextDrawAt.Add(-75 * time.Second),
	}).Error; err != nil {
		t.Fatal(err)
	}
	var openSnapshot GameSummary
	for _, test := range []struct {
		room   workspacemodel.Workspace
		member user.User
		seal   int
		status string
	}{
		{roomA, a, 45, lottery.IssueStatusSealed},
		{roomB, b, 10, lottery.IssueStatusAccepting},
	} {
		views, err := NewWorkspaceGameService(db).List(test.room.ID)
		if err != nil {
			t.Fatal("room administration catalogue:", err)
		}
		var summary *GameSummary
		for i := range views {
			if views[i].ID == game.ID {
				summary = &views[i].GameSummary
				break
			}
		}
		if summary == nil {
			t.Fatal("room catalogue did not include enabled fixture game")
		}
		if summary.CurrentIssue != "73001" || summary.Issue != "73000" || summary.DrawInterval != 75 ||
			summary.SealSeconds != test.seal || summary.IssueStatus != test.status || summary.TimingSource != "upstream" ||
			!summary.NextDrawAt.Equal(game.NextDrawAt) || summary.AcceptAt == nil || summary.SealAt == nil ||
			!summary.AcceptAt.Equal(game.NextDrawAt.Add(-75*time.Second)) ||
			!summary.SealAt.Equal(game.NextDrawAt.Add(-time.Duration(test.seal)*time.Second)) {
			t.Fatalf("room %d API did not expose its matching issue/cadence/cutoff: %+v", test.room.ID, summary)
		}
		memberGames, err := NewWorkspaceGameService(db).ListEnabledForMember(test.member.UserID)
		if err != nil || len(memberGames) != 1 {
			t.Fatalf("member catalogue: count=%d, err=%v", len(memberGames), err)
		}
		if memberGames[0].CurrentIssue != summary.CurrentIssue || memberGames[0].IssueStatus != summary.IssueStatus ||
			memberGames[0].SealSeconds != summary.SealSeconds || !memberGames[0].NextDrawAt.Equal(summary.NextDrawAt) {
			t.Fatalf("member and administration timing disagree: member=%+v, room=%+v", memberGames[0], summary)
		}
		status, err := NewBetAssistantService(db).StatusForUser(test.member.UserID, game.ID)
		if err != nil {
			t.Fatal("member assistant status:", err)
		}
		if status.Issue != summary.CurrentIssue || status.LatestIssue != summary.Issue || status.DrawInterval != 75 ||
			status.SealSeconds != test.seal || status.IssueStatus != summary.IssueStatus ||
			status.Accepting != (test.status == lottery.IssueStatusAccepting) ||
			!status.NextDrawAt.Equal(summary.NextDrawAt) || status.SealAt == nil || !status.SealAt.Equal(*summary.SealAt) {
			t.Fatalf("assistant and catalogue timing disagree: assistant=%+v, room=%+v", status, summary)
		}
		if test.room.ID == roomB.ID {
			openSnapshot = *summary
		}
	}
	platformStatus, err := NewBetAssistantService(db).Status(game.ID)
	if err != nil || platformStatus.IssueStatus != lottery.IssueStatusSealed || platformStatus.Accepting {
		t.Fatalf("fixture must prove room acceptance while platform is sealed: status=%+v, err=%v", platformStatus, err)
	}
	// Simulate collection advancing after a response was read. Lobby enrichment
	// must retain that response's issue/time pair instead of re-querying the new
	// issue while keeping the old countdown.
	if err := db.Model(&lottery.Game{}).Where("id = ?", game.ID).Updates(map[string]any{
		"next_issue": "73002", "next_draw_at": game.NextDrawAt.Add(75 * time.Second),
	}).Error; err != nil {
		t.Fatal(err)
	}
	enriched, err := NewLotteryService(db).EnrichForLobby([]GameSummary{openSnapshot})
	if err != nil || len(enriched) != 1 || enriched[0].CurrentIssue != "73001" || enriched[0].Issue != "73000" ||
		!enriched[0].NextDrawAt.Equal(openSnapshot.NextDrawAt) || !enriched[0].SealAt.Equal(*openSnapshot.SealAt) {
		t.Fatalf("lobby enrichment replaced the response's timing snapshot: %+v, err=%v", enriched, err)
	}
}

func timingPostgresAgentRoom(t *testing.T, db *gorm.DB, tenant workspacemodel.Workspace, name, roomCode string) workspacemodel.Workspace {
	t.Helper()
	agent, err := NewAgentAdminService(db).CreateForTenant(tenant.OwnerUserID, CreateAgentInput{
		Username: name, Password: "TimingFixture#2026_a9Z", Nickname: "默认值测试代理", RoomCode: roomCode, Status: 1,
	})
	if err != nil {
		t.Fatal("create isolated default-settings agent:", err)
	}
	var room workspacemodel.Workspace
	if err := db.Where("owner_user_id = ?", agent.ID).First(&room).Error; err != nil {
		t.Fatal(err)
	}
	return room
}

func assertTimingPostgresDefaults(t *testing.T, db *gorm.DB, room workspacemodel.Workspace, seal int, overrides map[string]int) {
	t.Helper()
	var stored settings.SystemConfig
	if err := db.Where("workspace_id = ?", room.ID).First(&stored).Error; err != nil {
		t.Fatal("read persisted room initialization settings:", err)
	}
	var decoded struct {
		SealSeconds         *int `json:"seal_seconds"`
		MaxOpenGames        int  `json:"max_open_games"`
		RoomActivityEnabled bool `json:"room_activity_enabled"`
		Overrides           map[string]struct {
			SealSeconds *int `json:"seal_seconds"`
		} `json:"game_timing_overrides"`
	}
	if err := json.Unmarshal([]byte(stored.GameSettingsJSON), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SealSeconds == nil || *decoded.SealSeconds != seal {
		t.Fatalf("room %d initialized seal=%v, want %d", room.ID, decoded.SealSeconds, seal)
	}
	actualOverrides := map[string]int{}
	for gameID, override := range decoded.Overrides {
		if override.SealSeconds == nil {
			t.Fatalf("room %d has an override without an explicit cutoff for %s", room.ID, gameID)
		}
		actualOverrides[gameID] = *override.SealSeconds
	}
	if !reflect.DeepEqual(actualOverrides, overrides) {
		t.Fatalf("room %d overrides=%v, want %v", room.ID, actualOverrides, overrides)
	}
	if decoded.MaxOpenGames != 0 || decoded.RoomActivityEnabled {
		t.Fatalf("timing initialization copied unrelated platform operation switches: max_open_games=%d, robots=%v", decoded.MaxOpenGames, decoded.RoomActivityEnabled)
	}
	var enabled int64
	if err := db.Table("room_game_settings").Where("workspace_id = ? AND enabled = TRUE", room.ID).Count(&enabled).Error; err != nil || enabled != 0 {
		t.Fatalf("initial timing defaults unexpectedly opened games in room %d: %d, %v", room.ID, enabled, err)
	}
	if allowed, err := WorkspaceGameEnabled(db, room.ID, "speed-fly"); err != nil || allowed {
		t.Fatalf("initial timing defaults bypassed the closed-game gate: %v, %v", allowed, err)
	}
}

func TestLotteryTimingPostgresInitialRoomDefaults(t *testing.T) {
	db := timingPostgresDatabase(t)
	_, platformID, err := readTimingSettings(db, 0)
	if err != nil {
		t.Fatal(err)
	}
	legacy := timingPostgresRoom(t, db, "timing_defaults_legacy", "76401")
	timingPostgresSettings(t, db, legacy.ID, `{"seal_seconds":30,"game_timing_overrides":{"speed-fly":{"seal_seconds":5}}}`)
	initialOverrides := map[string]int{"speed-fly": 12, "speed-racing": 0, "future-lottery": 9}
	newOverrides := map[string]int{"speed-fly": 8, "speed-racing": 4, "future-lottery": 6}
	roomOverrides := map[string]int{"speed-fly": 7, "speed-racing": 0}
	timingPostgresSettings(t, db, platformID, `{"seal_seconds":45,"game_timing_overrides":{"speed-fly":{"seal_seconds":12},"speed-racing":{"seal_seconds":0},"future-lottery":{"seal_seconds":9}},"max_open_games":99,"room_activity_enabled":true}`)
	roomA := timingPostgresRoom(t, db, "timing_defaults_a", "76402")
	roomB := timingPostgresRoom(t, db, "timing_defaults_b", "76403")
	agentA := timingPostgresAgentRoom(t, db, roomA, "timing_defaults_agent_a", "76404")
	for _, room := range []workspacemodel.Workspace{roomA, roomB, agentA} {
		assertTimingPostgresDefaults(t, db, room, 45, initialOverrides)
	}

	// A room owns its copied values. A child agent takes the platform's current
	// initialization template, not its tenant's subsequently customized cutoff.
	timingPostgresSettings(t, db, roomB.ID, `{"seal_seconds":10,"game_timing_overrides":{"speed-fly":{"seal_seconds":7},"speed-racing":{"seal_seconds":0}}}`)
	agentB := timingPostgresAgentRoom(t, db, roomB, "timing_defaults_agent_b", "76405")
	assertTimingPostgresDefaults(t, db, agentB, 45, initialOverrides)
	assertTimingPostgresDefaults(t, db, roomB, 10, roomOverrides)

	timingPostgresSettings(t, db, platformID, `{"seal_seconds":20,"game_timing_overrides":{"speed-fly":{"seal_seconds":8},"speed-racing":{"seal_seconds":4},"future-lottery":{"seal_seconds":6}}}`)
	newRoom := timingPostgresRoom(t, db, "timing_defaults_new", "76406")
	newAgent := timingPostgresAgentRoom(t, db, roomA, "timing_defaults_agent_new", "76407")
	for _, room := range []workspacemodel.Workspace{newRoom, newAgent} {
		assertTimingPostgresDefaults(t, db, room, 20, newOverrides)
	}
	for _, room := range []workspacemodel.Workspace{roomA, agentA, agentB} {
		assertTimingPostgresDefaults(t, db, room, 45, initialOverrides)
	}
	assertTimingPostgresDefaults(t, db, roomB, 10, roomOverrides)
	assertTimingPostgresDefaults(t, db, legacy, 30, map[string]int{"speed-fly": 5})

	// Re-running startup is additive: neither platform edits nor bootstrap may
	// change explicit historical 30s, inherited 45s, or customized 10s values.
	if err := Bootstrap(db, BootstrapOptions{Mode: "test"}); err != nil {
		t.Fatal("repeat bootstrap after platform default change:", err)
	}
	for _, room := range []workspacemodel.Workspace{roomA, agentA, agentB} {
		assertTimingPostgresDefaults(t, db, room, 45, initialOverrides)
	}
	assertTimingPostgresDefaults(t, db, roomB, 10, roomOverrides)
	assertTimingPostgresDefaults(t, db, legacy, 30, map[string]int{"speed-fly": 5})
	assertTimingPostgresDefaults(t, db, newRoom, 20, newOverrides)
	assertTimingPostgresDefaults(t, db, newAgent, 20, newOverrides)
	for _, room := range []workspacemodel.Workspace{legacy, roomA, roomB, agentA, agentB, newRoom, newAgent} {
		var total, closed int64
		if err := db.Table("room_game_settings").Where("workspace_id = ?", room.ID).Count(&total).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Table("room_game_settings").Where("workspace_id = ? AND enabled = FALSE", room.ID).Count(&closed).Error; err != nil {
			t.Fatal(err)
		}
		if total != 30 || closed != total {
			t.Fatalf("room %d must retain all 30 closed game switches: total=%d closed=%d", room.ID, total, closed)
		}
	}
}

// Build an isolated legacy-style workspace without its settings row. This
// fixture inserts only new test rows and does not delete or truncate anything.
func timingPostgresRoomWithoutSettings(t *testing.T, db *gorm.DB, platformID uint64, name, roomCode string) workspacemodel.Workspace {
	t.Helper()
	owner := user.User{Username: name, Password: "fixture-no-login", LoginScope: platformLoginScope, Nickname: "缺配置测试租户", Role: "tenant", Status: 1}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	room := workspacemodel.Workspace{
		Code: "fixture-" + roomCode, RoomCode: roomCode, Type: workspacemodel.TypeTenant,
		OwnerUserID: owner.UserID, ParentID: &platformID, Scope: fmt.Sprintf("tenant:%d", owner.UserID), Name: "缺配置测试房间", Status: 1,
	}
	if err := db.Create(&room).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&owner).Update("workspace_id", room.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := EnsureWorkspaceGameDefaults(db, room); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&settings.SystemConfig{}).Where("workspace_id = ?", room.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("missing-settings fixture unexpectedly has a settings row: %d, %v", count, err)
	}
	return room
}

func TestLotteryTimingPostgresMissingSettingsDefaults(t *testing.T) {
	db := timingPostgresDatabase(t)
	_, platformID, err := readTimingSettings(db, 0)
	if err != nil {
		t.Fatal(err)
	}
	timingPostgresSettings(t, db, platformID, `{"seal_seconds":45,"game_timing_overrides":{"speed-fly":{"seal_seconds":11}}}`)
	apiRoom := timingPostgresRoomWithoutSettings(t, db, platformID, "timing_missing_api", "76491")
	if _, err := NewSettingsAdminService(db).GetForWorkspace(apiRoom.ID); err != nil {
		t.Fatal("first settings read should copy the current platform template:", err)
	}
	assertTimingPostgresDefaults(t, db, apiRoom, 45, map[string]int{"speed-fly": 11})
	timingPostgresSettings(t, db, platformID, `{"seal_seconds":20,"game_timing_overrides":{"speed-fly":{"seal_seconds":6}}}`)
	if _, err := NewSettingsAdminService(db).GetForWorkspace(apiRoom.ID); err != nil {
		t.Fatal(err)
	}
	assertTimingPostgresDefaults(t, db, apiRoom, 45, map[string]int{"speed-fly": 11})
	secondAPIRoom := timingPostgresRoomWithoutSettings(t, db, platformID, "timing_missing_api_new", "76492")
	if _, err := NewSettingsAdminService(db).GetForWorkspace(secondAPIRoom.ID); err != nil {
		t.Fatal("later first settings read should copy the new platform template:", err)
	}
	assertTimingPostgresDefaults(t, db, secondAPIRoom, 20, map[string]int{"speed-fly": 6})
	hierarchyRoom := timingPostgresRoomWithoutSettings(t, db, platformID, "timing_missing_bootstrap", "76493")
	if err := EnsureWorkspaceHierarchy(db); err != nil {
		t.Fatal("startup missing-settings repair:", err)
	}
	assertTimingPostgresDefaults(t, db, hierarchyRoom, 20, map[string]int{"speed-fly": 6})
	assertTimingPostgresDefaults(t, db, apiRoom, 45, map[string]int{"speed-fly": 11})
	assertTimingPostgresDefaults(t, db, secondAPIRoom, 20, map[string]int{"speed-fly": 6})
}
