package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type bettingLockObservation struct {
	gameLocks    int
	timingWrites int
}

// Instrument the real PostgreSQL execution path, not a mock SQL plan: any
// lifecycle/window write before a successful SELECT Game FOR SHARE makes the
// financial transaction fail. This catches the outer idempotency transaction
// holding Issue then waiting on Game, even when a serial happy-path test passes.
func observeBettingLockOrder(t *testing.T, db *gorm.DB) *bettingLockObservation {
	t.Helper()
	observation := &bettingLockObservation{}
	if err := db.Callback().Query().After("gorm:query").Register("test:betting_game_lock", func(tx *gorm.DB) {
		if tx.Error != nil || tx.Statement.Table != "lottery_games" {
			return
		}
		locking, ok := tx.Statement.Clauses["FOR"].Expression.(clause.Locking)
		if ok && locking.Strength == "SHARE" {
			observation.gameLocks++
		}
	}); err != nil {
		t.Fatal(err)
	}
	checkWrite := func(tx *gorm.DB) {
		if tx.Statement.Table != "lottery_issues" && tx.Statement.Table != "lottery_issue_windows" {
			return
		}
		observation.timingWrites++
		if observation.gameLocks == 0 {
			tx.AddError(fmt.Errorf("timing write preceded the outer transaction's Game SHARE lock"))
		}
	}
	if err := db.Callback().Create().Before("gorm:create").Register("test:betting_timing_create", checkWrite); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("test:betting_timing_update", checkWrite); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove("test:betting_game_lock")
		_ = db.Callback().Create().Remove("test:betting_timing_create")
		_ = db.Callback().Update().Remove("test:betting_timing_update")
	})
	return observation
}

// Opt-in integration tests use the existing fresh-loopback-only fixture guard
// and transaction rollback; they never load the application's configured DB.
func TestLotteryTimingPostgresNextBettingWindow(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "next_period_tenant", "76601")
	member := timingPostgresMember(t, db, room, "next_period_member")
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "speed-racing", true); err != nil {
		t.Fatal(err)
	}
	timingPostgresSettings(t, db, room.ID, `{"seal_seconds":30}`)
	service := NewBetAdminService(db)
	service.suppressNotifications = true
	assistant := NewBetAssistantService(db)

	seed := func(t *testing.T, issue string) *lottery.Game {
		t.Helper()
		// Keep enough headroom for a slow test runner; a 120s period is proven by
		// all fixture draws instead of being assumed from a production game ID.
		now := time.Now().UTC().Truncate(time.Second)
		drawAt := now.Add(-time.Second)
		if err := db.Model(&lottery.Game{}).Where("id = ?", "speed-racing").Updates(map[string]any{
			"enabled": true, "source_kind": "external", "sync_status": "ok", "last_sync_error": "",
			"last_sync_at": now, "next_issue": issue, "next_draw_at": drawAt, "draw_interval": 120, "timing_source": "upstream",
		}).Error; err != nil {
			t.Fatal(err)
		}
		value, _ := strconv.Atoi(issue)
		for index := 1; index <= 4; index++ {
			if err := db.Create(&lottery.Draw{GameID: "speed-racing", Issue: strconv.Itoa(value - index),
				DrawAt: drawAt.Add(-time.Duration(index) * 120 * time.Second), Numbers: "1,2,3,4,5,6,7,8,9,10"}).Error; err != nil {
				t.Fatal(err)
			}
		}
		game, err := service.loadGame("speed-racing")
		if err != nil {
			t.Fatal(err)
		}
		return game
	}
	input := func(issue string) PlaceBetInput {
		return PlaceBetInput{GameID: "speed-racing", Issue: issue, UserID: member.UserID, PlayCode: "ball_1_5",
			Position: 1, Selection: "4", Amount: 2, Operator: "next-window-integration"}
	}
	assertRejected := func(t *testing.T, issue string, code string) {
		t.Helper()
		before := timingPostgresMoney(t, db, member.UserID)
		if _, err := service.Place(input(issue)); apperrors.GetErrorCode(err) != code {
			t.Fatalf("rejection: got %v, want %s", err, code)
		}
		if after := timingPostgresMoney(t, db, member.UserID); after != before {
			t.Fatalf("rejected request changed funds/orders: before=%+v after=%+v", before, after)
		}
	}
	run := func(name string, fn func(*testing.T)) {
		t.Run(name, func(t *testing.T) {
			if err := db.SavePoint("next_betting_case").Error; err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := db.RollbackTo("next_betting_case").Error; err != nil {
					t.Error("rollback next-period fixture:", err)
				}
			})
			fn(t)
		})
	}

	run("view-only status exposes next window without advancing draw tracking", func(t *testing.T) {
		game := seed(t, "94001")
		before := timingPostgresMoney(t, db, member.UserID)
		var status *AssistantDrawStatus
		for i := 0; i < 3; i++ {
			var err error
			status, err = assistant.StatusForUser(member.UserID, game.ID)
			if err != nil || status.Issue != "94001" || status.IssueStatus != lottery.IssueStatusAwaiting ||
				!status.Accepting || status.BettingWindow == nil || status.BettingWindow.Issue != "94002" ||
				!status.BettingWindow.AcceptAt.Equal(game.NextDrawAt) || status.BettingWindow.SealSeconds != 30 {
				t.Fatalf("next window status: %+v, %v", status, err)
			}
		}
		games, err := NewWorkspaceGameService(db).ListEnabledForMember(member.UserID)
		if err != nil || len(games) != 1 || games[0].CurrentIssue != status.Issue || games[0].IssueStatus != status.IssueStatus ||
			!reflect.DeepEqual(games[0].BettingWindow, status.BettingWindow) {
			t.Fatalf("catalogue/status disagree: %+v, %v", games, err)
		}
		stored, err := service.loadGame(game.ID)
		if err != nil || stored.NextIssue != game.NextIssue || !stored.NextDrawAt.Equal(game.NextDrawAt) {
			t.Fatalf("status changed draw polling schedule: %+v, %v", stored, err)
		}
		if after := timingPostgresMoney(t, db, member.UserID); after != before {
			t.Fatal("viewing status wrote a bet or ledger")
		}
		var draws int64
		if err := db.Model(&lottery.Draw{}).Where("game_id = ?", game.ID).Count(&draws).Error; err != nil || draws != 4 {
			t.Fatalf("status created a synthetic draw: %d %v", draws, err)
		}
	})

	run("explicit next and empty issue work but closed and skipped issues fail", func(t *testing.T) {
		seed(t, "94011")
		assertRejected(t, "94011", "ISSUE_MISMATCH")
		assertRejected(t, "94013", "ISSUE_MISMATCH")
		for _, issue := range []string{"94012", ""} {
			placed, err := service.Place(input(issue))
			if err != nil || placed.Issue != "94012" {
				t.Fatalf("confirmed next issue rejected: %+v, %v", placed, err)
			}
		}
		before := timingPostgresMoney(t, db, member.UserID)
		first, err := assistant.Place(member.UserID, "speed-racing", "94012", "2/5/2", "test", "next-window-idempotent-01")
		if err != nil || first.Issue != "94012" {
			t.Fatalf("assistant next period: %+v %v", first, err)
		}
		after := timingPostgresMoney(t, db, member.UserID)
		if after.BalanceCents != before.BalanceCents-200 || after.LedgerRows != before.LedgerRows+1 {
			t.Fatalf("assistant debit: %+v -> %+v", before, after)
		}
		for i := 0; i < 2; i++ {
			repeated, err := assistant.Place(member.UserID, "speed-racing", "94012", "2/5/2", "test", "next-window-idempotent-01")
			if err != nil || !reflect.DeepEqual(first, repeated) {
				t.Fatalf("idempotent next period replay: %+v %v", repeated, err)
			}
			if replayed := timingPostgresMoney(t, db, member.UserID); replayed != after {
				t.Fatalf("retry double debited: %+v -> %+v", after, replayed)
			}
		}
		if _, err := assistant.Place(member.UserID, "speed-racing", "94013", "2/5/2", "test", "next-window-idempotent-01"); apperrors.GetErrorCode(err) != "IDEMPOTENCY_CONFLICT" {
			t.Fatalf("retry changed accepted issue: %v", err)
		}
		cancelled, err := service.CancelCurrentIssue(member.UserID, "speed-racing", "test")
		if err != nil || cancelled.Issue != "94012" || cancelled.Count != 2 || cancelled.Refund != 6 {
			t.Fatalf("next period cancellation mismatch: %+v %v", cancelled, err)
		}
		var oldBets int64
		if err := db.Model(&bet.Bet{}).Where("game_id = ? AND issue = ?", "speed-racing", "94011").Count(&oldBets).Error; err != nil || oldBets != 0 {
			t.Fatalf("next order landed in old draw: %d %v", oldBets, err)
		}
	})

	run("shortened next cutoff cannot reopen on settings refresh", func(t *testing.T) {
		game := seed(t, "94021")
		status, err := assistant.StatusForUser(member.UserID, game.ID)
		if err != nil || status.BettingWindow == nil {
			t.Fatalf("materialize next cutoff: %+v %v", status, err)
		}
		timingPostgresSettings(t, db, room.ID, `{"seal_seconds":120}`)
		// A settings refresh freezes the shorter cutoff outside a rejected
		// financial transaction, exactly as the member/catalogue API does.
		if status, err := assistant.StatusForUser(member.UserID, game.ID); err != nil || status.BettingWindow != nil {
			t.Fatalf("shortened next window is still exposed: %+v %v", status, err)
		}
		assertRejected(t, "94022", "ISSUE_CLOSED")
		timingPostgresSettings(t, db, room.ID, `{"seal_seconds":0}`)
		assertRejected(t, "94022", "ISSUE_CLOSED")
		status, err = assistant.StatusForUser(member.UserID, game.ID)
		if err != nil || status.Accepting || status.BettingWindow != nil {
			t.Fatalf("closed next cutoff advertised as accepting: %+v %v", status, err)
		}
		timingPostgresSettings(t, db, room.ID, `{"seal_seconds":30}`)
	})

	run("source failure and room switch override an already seen window", func(t *testing.T) {
		game := seed(t, "94031")
		if status, err := assistant.StatusForUser(member.UserID, game.ID); err != nil || status.BettingWindow == nil {
			t.Fatalf("next window unavailable: %+v %v", status, err)
		}
		if err := db.Model(game).Updates(map[string]any{"sync_status": "error", "last_sync_error": "fixture timeout"}).Error; err != nil {
			t.Fatal(err)
		}
		assertRejected(t, "94032", "SOURCE_UNAVAILABLE")
		if err := db.Model(game).Updates(map[string]any{"sync_status": "ok", "last_sync_error": ""}).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, game.ID, false); err != nil {
			t.Fatal(err)
		}
		assertRejected(t, "94032", "GAME_DISABLED")
		status, err := assistant.StatusForUser(member.UserID, game.ID)
		if err != nil || status.Accepting || status.BettingWindow != nil {
			t.Fatalf("disabled room leaked next acceptance: %+v %v", status, err)
		}
	})

	run("upstream advances into the same frozen next window", func(t *testing.T) {
		game := seed(t, "94041")
		status, err := assistant.StatusForUser(member.UserID, game.ID)
		if err != nil || status.BettingWindow == nil {
			t.Fatalf("next window unavailable: %+v %v", status, err)
		}
		next := *status.BettingWindow
		timingPostgresSettings(t, db, room.ID, `{"seal_seconds":0}`)
		if err := db.Model(game).Updates(map[string]any{"next_issue": next.Issue, "next_draw_at": next.NextDrawAt}).Error; err != nil {
			t.Fatal(err)
		}
		status, err = assistant.StatusForUser(member.UserID, game.ID)
		if err != nil || status.Issue != next.Issue || status.BettingWindow != nil || !status.Accepting ||
			status.SealAt == nil || !status.SealAt.Equal(next.SealAt) || status.SealSeconds != 30 {
			t.Fatalf("upstream replaced or extended projected window: %+v %v", status, err)
		}
		if placed, err := service.Place(input(next.Issue)); err != nil || placed.Issue != next.Issue {
			t.Fatalf("upstream-confirmed period failed placement: %+v %v", placed, err)
		}
		var count int64
		if err := db.Model(&lottery.IssueWindow{}).Where("workspace_id = ? AND game_id = ? AND issue = ?", room.ID, game.ID, next.Issue).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("upstream created a duplicate acceptance window: %d %v", count, err)
		}
	})

	run("projection never reopens a terminal next period", func(t *testing.T) {
		game := seed(t, "94051")
		status, err := assistant.StatusForUser(member.UserID, game.ID)
		if err != nil || status.BettingWindow == nil {
			t.Fatalf("next window unavailable: %+v %v", status, err)
		}
		if err := db.Model(&lottery.Issue{}).Where("game_id = ? AND issue = ?", game.ID, status.BettingWindow.Issue).
			Updates(map[string]any{"status": lottery.IssueStatusError, "last_error": "manual review required"}).Error; err != nil {
			t.Fatal(err)
		}
		assertRejected(t, "94052", "SOURCE_UNAVAILABLE")
		status, err = assistant.StatusForUser(member.UserID, game.ID)
		if err != nil || status.Accepting || status.BettingWindow != nil {
			t.Fatalf("terminal next period advertised as open: %+v %v", status, err)
		}
		var terminal lottery.Issue
		if err := db.Where("game_id = ? AND issue = ?", game.ID, "94052").First(&terminal).Error; err != nil || terminal.Status != lottery.IssueStatusError || terminal.LastError != "manual review required" {
			t.Fatalf("projection repaired an error without authority: %+v %v", terminal, err)
		}
	})

	for _, kind := range []string{"direct", "assistant"} {
		run("idempotent "+kind+" locks game before materializing timing", func(t *testing.T) {
			game := seed(t, "94061")
			observation := observeBettingLockOrder(t, db)
			place := func() (string, error) {
				if kind == "direct" {
					result, err := service.PlaceIdempotent(input(""), "next-lock-order-direct-01")
					if err != nil {
						return "", err
					}
					return result.Issue, nil
				}
				result, err := assistant.Place(member.UserID, game.ID, "", "1/4/2", "test", "next-lock-order-assistant-01")
				if err != nil {
					return "", err
				}
				return result.Issue, nil
			}
			if issue, err := place(); err != nil || issue != "94062" {
				t.Fatalf("outer lock order rejected %s bet: %s %v", kind, issue, err)
			}
			if observation.gameLocks == 0 || observation.timingWrites == 0 {
				t.Fatalf("test did not exercise both game locking and new timing writes: %+v", observation)
			}
			before := timingPostgresMoney(t, db, member.UserID)
			locksBeforeReplay, writesBeforeReplay := observation.gameLocks, observation.timingWrites
			if err := db.Model(game).Updates(map[string]any{"sync_status": "error", "last_sync_error": "fixture source stopped after acceptance"}).Error; err != nil {
				t.Fatal(err)
			}
			if issue, err := place(); err != nil || issue != "94062" {
				t.Fatalf("accepted request replay depended on a now-failed source: %s %v", issue, err)
			}
			if observation.gameLocks != locksBeforeReplay || observation.timingWrites != writesBeforeReplay {
				t.Fatal("cached idempotent receipt reacquired game locks or wrote timing")
			}
			if after := timingPostgresMoney(t, db, member.UserID); after != before {
				t.Fatalf("cached replay moved money: before=%+v after=%+v", before, after)
			}
		})
	}

	run("cancellation rejects an old confirmed issue without touching either period", func(t *testing.T) {
		game := seed(t, "94071")
		if _, err := service.Place(input("94072")); err != nil {
			t.Fatal("place previously-confirmed issue:", err)
		}
		// Simulate an upstream period correction occurring after a client read
		// 94072 but before its cancellation is processed. Both sets of stored
		// orders must remain untouched when the client confirms the old issue.
		if err := db.Model(game).Updates(map[string]any{"next_issue": "94073", "next_draw_at": game.NextDrawAt.Add(120 * time.Second)}).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := service.Place(input("94073")); err != nil {
			t.Fatal("place corrected issue:", err)
		}
		before := timingPostgresMoney(t, db, member.UserID)
		if _, err := service.CancelCurrentIssue(member.UserID, game.ID, "test", "94072"); apperrors.GetErrorCode(err) != "ISSUE_MISMATCH" {
			t.Fatalf("stale cancellation silently retargeted: %v", err)
		}
		if after := timingPostgresMoney(t, db, member.UserID); after != before {
			t.Fatalf("stale cancellation changed orders/funds: %+v -> %+v", before, after)
		}
		cancelled, err := service.CancelCurrentIssue(member.UserID, game.ID, "test", "94073")
		if err != nil || cancelled.Issue != "94073" || cancelled.Count != 1 || cancelled.Refund != 2 {
			t.Fatalf("explicit current cancellation failed: %+v %v", cancelled, err)
		}
		var oldPending int64
		if err := db.Model(&bet.Bet{}).Where("user_id = ? AND game_id = ? AND issue = ? AND status = ?", member.UserID, game.ID, "94072", "pending").Count(&oldPending).Error; err != nil || oldPending != 1 {
			t.Fatalf("current cancellation touched another period: %d %v", oldPending, err)
		}
		// The original 3-argument service call remains valid, but only operates
		// on the server's locked current issue; it cannot refund the older row.
		if _, err := service.CancelCurrentIssue(member.UserID, game.ID, "test"); apperrors.GetErrorCode(err) != "NO_PENDING_BETS" {
			t.Fatalf("legacy current cancellation changed its target: %v", err)
		}
	})
}
