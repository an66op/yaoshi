package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/data/models/odds"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The shared fixture accepts only the explicitly supplied, empty loopback
// wangzhe_timing_test database and rolls back all schema/data at test end.
// These tests never read the application's database configuration.
func placementPostgresFixture(t *testing.T) (*gorm.DB, *BetAdminService, workspacemodel.Workspace, user.User, *lottery.Game) {
	t.Helper()
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "placement_lifecycle_room", "783021")
	member := timingPostgresMember(t, db, room, "placement_lifecycle_member")
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "speed-fly", true); err != nil {
		t.Fatal(err)
	}
	timingPostgresSettings(t, db, room.ID, `{"seal_seconds":30}`)
	game := timingPostgresSchedule(t, db, "972201")
	service := NewBetAdminService(db)
	service.suppressNotifications = true
	return db, service, room, member, game
}

func placementPostgresInput(member user.User, game *lottery.Game, selection string, amount float64) PlaceBetInput {
	return PlaceBetInput{GameID: game.ID, Issue: game.NextIssue, UserID: member.UserID,
		PlayCode: "ball_1_5", Position: 1, Selection: selection, Amount: amount, Operator: "placement-lifecycle-fixture"}
}

func placementPostgresRow(t *testing.T, db *gorm.DB, id uint64) bet.Bet {
	t.Helper()
	var row bet.Bet
	if err := db.First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func placementPostgresAssertDebitEvidence(t *testing.T, db *gorm.DB, row bet.Bet, amountCents int64) {
	t.Helper()
	var ledger user.BalanceTransaction
	if row.RequestReference == "" {
		t.Fatal("new bet retained legacy empty request reference")
	}
	if err := db.Where("user_id = ? AND reference = ? AND type = ?", row.UserID, row.RequestReference, "bet").First(&ledger).Error; err != nil {
		t.Fatal("bet has no matching debit evidence:", err)
	}
	if ledger.WorkspaceID != row.WorkspaceID || ledger.AmountCents != -amountCents {
		t.Fatalf("bet/debit scope or amount mismatch: bet=%+v ledger=%+v", row, ledger)
	}
}

func TestPlacementLifecyclePostgresFinancialSnapshots(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(fmt.Sprintf("batch=%t", batch), func(t *testing.T) {
			db, service, room, member, game := placementPostgresFixture(t)
			setTerms := func(price, rebate, share float64) {
				t.Helper()
				if err := db.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "workspace_id"}, {Name: "user_id"}, {Name: "game_id"}, {Name: "play_code"}},
					DoUpdates: clause.AssignmentColumns([]string{"odds"}),
				}).Create(&odds.UserPlayOdds{WorkspaceID: room.ID, UserID: member.UserID, GameID: game.ID, PlayCode: "ball_1_5", Odds: price}).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.Model(&user.User{}).Where("user_id = ?", room.OwnerUserID).
					Updates(map[string]any{"room_rebate_rate": rebate, "room_profit_share_rate": share}).Error; err != nil {
					t.Fatal(err)
				}
			}
			place := func(input PlaceBetInput) *BetView {
				t.Helper()
				if batch {
					rows, err := service.PlaceBatch([]PlaceBetInput{input})
					if err != nil || len(rows) != 1 {
						t.Fatalf("batch placement: %+v / %v", rows, err)
					}
					return &rows[0]
				}
				row, err := service.Place(input)
				if err != nil {
					t.Fatal(err)
				}
				return row
			}
			input := placementPostgresInput(member, game, "2", 100)
			setTerms(9.9, 0.5, 30)
			first := place(input)
			original := placementPostgresRow(t, db, first.ID)
			setTerms(8, 2, 40)
			second := place(input)
			latest := placementPostgresRow(t, db, second.ID)
			if first.ID == second.ID || first.Amount != 100 || second.Amount != 100 || original.RequestReference == latest.RequestReference {
				t.Fatalf("separate financial operations were accumulated: %+v / %+v", first, second)
			}
			if original.Odds != 9.9 || original.RebateRateSnapshot != 0.5 || original.AgentShareRateSnapshot != 30 ||
				latest.Odds != 8 || latest.RebateRateSnapshot != 2 || latest.AgentShareRateSnapshot != 40 {
				t.Fatalf("financial snapshots changed/averaged: %+v / %+v", original, latest)
			}
			if after := placementPostgresRow(t, db, first.ID); !reflect.DeepEqual(after, original) {
				t.Fatalf("old pending contract rewritten: before=%+v after=%+v", original, after)
			}
			placementPostgresAssertDebitEvidence(t, db, original, 10000)
			placementPostgresAssertDebitEvidence(t, db, latest, 10000)
			money := timingPostgresMoney(t, db, member.UserID)
			if money.BalanceCents != member.BalanceCents-20000 || money.Bets != 2 || money.Pending != 2 || money.LedgerRows != 2 {
				t.Fatalf("independent stakes were not atomically charged: %+v", money)
			}
		})
	}
}

func TestPlacementLifecyclePostgresNeverReusesCancelledOrSettledRows(t *testing.T) {
	db, service, _, member, game := placementPostgresFixture(t)
	input := placementPostgresInput(member, game, "2", 100)
	first, err := service.Place(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Cancel(first.ID, "fixture-cancel"); err != nil {
		t.Fatal(err)
	}
	cancelled := placementPostgresRow(t, db, first.ID)
	if cancelled.Status != "cancelled" || timingPostgresMoney(t, db, member.UserID).BalanceCents != member.BalanceCents {
		t.Fatal("fixture cancellation did not refund")
	}
	input.Amount = 20
	second, err := service.Place(input)
	if err != nil || second.ID == first.ID || second.Status != "pending" || second.Amount != 20 {
		t.Fatalf("post-cancel placement reused old row: %+v / %v", second, err)
	}
	if after := placementPostgresRow(t, db, first.ID); !reflect.DeepEqual(after, cancelled) {
		t.Fatalf("cancelled contract changed: before=%+v after=%+v", cancelled, after)
	}
	// Legacy empty-reference rows are kept byte-for-byte, whether pending,
	// cancelled, won, or lost. New Place and PlaceBatch must never target them.
	for index, status := range []string{"pending", "cancelled", "won", "lost"} {
		selection := fmt.Sprint(index + 3)
		legacy := bet.Bet{
			WorkspaceID: member.WorkspaceID, UserID: member.UserID, Username: member.Username,
			RoomScope: betRoomScope(member), GameID: game.ID, Issue: game.NextIssue,
			PlayCode: "ball_1_5", PlayName: "冠军", Position: 1, Selection: selection,
			RuleVersion: "racing-v2", Status: status, AmountCents: 1000, Odds: 7.5,
		}
		if err := db.Create(&legacy).Error; err != nil {
			t.Fatal(err)
		}
		legacy = placementPostgresRow(t, db, legacy.ID)
		newInput := placementPostgresInput(member, game, selection, 20)
		single, err := service.Place(newInput)
		if err != nil || single.ID == legacy.ID || single.Amount != 20 || single.Status != "pending" {
			t.Fatalf("Place reused %s historical row: %+v / %v", status, single, err)
		}
		batch, err := service.PlaceBatch([]PlaceBetInput{newInput})
		if err != nil || len(batch) != 1 || batch[0].ID == legacy.ID || batch[0].ID == single.ID || batch[0].Amount != 20 || batch[0].Status != "pending" {
			t.Fatalf("PlaceBatch reused %s historical row: %+v / %v", status, batch, err)
		}
		if after := placementPostgresRow(t, db, legacy.ID); !reflect.DeepEqual(after, legacy) {
			t.Fatalf("legacy %s row changed: before=%+v after=%+v", status, legacy, after)
		}
	}
}

func TestPlacementLifecyclePostgresBatchAtomicityAndIdempotency(t *testing.T) {
	db, service, _, member, game := placementPostgresFixture(t)
	first := placementPostgresInput(member, game, "2", 20)
	second := placementPostgresInput(member, game, "3", 30)
	duplicate := first
	duplicate.Amount = 40
	rows, err := service.PlaceBatch([]PlaceBetInput{first, second, duplicate})
	if err != nil || len(rows) != 3 || rows[0].ID != rows[2].ID || rows[0].ID == rows[1].ID || rows[0].Amount != 60 || rows[1].Amount != 30 {
		t.Fatalf("batch duplicate mapping/amount: %+v / %v", rows, err)
	}
	firstRow := placementPostgresRow(t, db, rows[0].ID)
	secondRow := placementPostgresRow(t, db, rows[1].ID)
	if firstRow.RequestReference != secondRow.RequestReference {
		t.Fatal("one batch split its debit reference")
	}
	placementPostgresAssertDebitEvidence(t, db, firstRow, 9000)

	// A failure at the later INSERT must roll back both the earlier row and
	// the debit, not just fail pre-validation before exercising the transaction.
	before := timingPostgresMoney(t, db, member.UserID)
	const callbackName = "fixture:fail_later_placement"
	if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if row, ok := tx.Statement.Dest.(*bet.Bet); ok && row.Selection == "9" {
			tx.AddError(fmt.Errorf("fixture rejected later bet insert"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, failed := service.PlaceBatch([]PlaceBetInput{first, placementPostgresInput(member, game, "9", 20)})
	if err := db.Callback().Create().Remove(callbackName); err != nil {
		t.Fatal(err)
	}
	if failed == nil || timingPostgresMoney(t, db, member.UserID) != before {
		t.Fatalf("failed later insert partially committed: %v / %+v", failed, timingPostgresMoney(t, db, member.UserID))
	}

	first.LedgerReference = "fixture:explicit-single-operation"
	explicit, err := service.Place(first)
	if err != nil {
		t.Fatal(err)
	}
	original := placementPostgresRow(t, db, explicit.ID)
	before = timingPostgresMoney(t, db, member.UserID)
	if _, err := service.Place(first); err == nil || timingPostgresMoney(t, db, member.UserID) != before {
		t.Fatal("duplicate explicit debit reference charged again")
	}
	if _, err := service.PlaceBatch([]PlaceBetInput{first}); err == nil || timingPostgresMoney(t, db, member.UserID) != before {
		t.Fatal("duplicate batch debit reference charged again")
	}
	if after := placementPostgresRow(t, db, explicit.ID); !reflect.DeepEqual(after, original) {
		t.Fatal("duplicate explicit reference changed prior bet")
	}
	if _, err := service.PlaceBatch([]PlaceBetInput{first, second}); apperrors.GetErrorCode(err) != "INVALID_REQUEST" || timingPostgresMoney(t, db, member.UserID) != before {
		t.Fatal("mixed references did not reject atomically:", err)
	}
	first.LedgerReference = ""
	direct, err := service.PlaceIdempotent(first, "placement-direct-request")
	if err != nil {
		t.Fatal(err)
	}
	before = timingPostgresMoney(t, db, member.UserID)
	if replay, err := service.PlaceIdempotent(first, "placement-direct-request"); err != nil || replay.ID != direct.ID || timingPostgresMoney(t, db, member.UserID) != before {
		t.Fatalf("direct request lost idempotency: %+v / %v", replay, err)
	}
	assistant := NewBetAssistantService(db)
	accepted, err := assistant.Place(member.UserID, game.ID, game.NextIssue, "1/23/20", "fixture-assistant", "placement-assistant-request")
	if err != nil || accepted.BetCount != 2 || accepted.Total != 40 {
		t.Fatalf("assistant batch submission: %+v / %v", accepted, err)
	}
	before = timingPostgresMoney(t, db, member.UserID)
	if replay, err := assistant.Place(member.UserID, game.ID, game.NextIssue, "1/23/20", "fixture-assistant", "placement-assistant-request"); err != nil || replay.Total != accepted.Total || timingPostgresMoney(t, db, member.UserID) != before {
		t.Fatalf("assistant request lost idempotency: %+v / %v", replay, err)
	}
}

func TestPlacementLifecyclePostgresRobotArchiveRoundTrip(t *testing.T) {
	db, service, room, member, game := placementPostgresFixture(t)
	if err := db.Create(&workspacemodel.RobotProfile{WorkspaceID: room.ID, UserID: member.UserID, Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	input := placementPostgresInput(member, game, "2", 20)
	first, err := service.Place(input)
	if err != nil {
		t.Fatal(err)
	}
	// Mark this isolated zero-payout robot fixture completed and old enough
	// for archive eligibility. Keep the fixture issue accepting so the next
	// step exercises archive dedupe separately from the issue-time gate.
	oldTime := time.Now().UTC().Add(-48 * time.Hour)
	if err := db.Model(&bet.Bet{}).Where("id = ?", first.ID).
		Updates(map[string]any{"created_at": oldTime, "status": "lost", "settled_at": oldTime.Add(time.Minute)}).Error; err != nil {
		t.Fatal(err)
	}
	snapshot := robotBetHotJSON(t, db, first.ID)
	old := placementPostgresRow(t, db, first.ID)
	if !strings.HasPrefix(old.RequestReference, "internal_bet:") || old.FlyCents != 0 || old.RebateRateSnapshot != 0 || old.AgentShareRateSnapshot != 0 {
		t.Fatalf("robot acquired real financial terms or lost request evidence: %+v", old)
	}
	const archiveRequest = "placement-robot-archive"
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := allowLifecycleDeletes(tx); err != nil {
			return err
		}
		count, err := NewDataLifecycleService(tx).archiveRobotBets(tx, normalizedCleanupCriteria{WorkspaceID: room.ID}, archiveRequest, oldTime.Add(time.Hour), 100)
		if err == nil && count != 1 {
			return fmt.Errorf("archived %d rows, want 1", count)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// The immutable archived request stays blocked, but a genuinely new
	// robot operation on the same selection is a separate pending contract.
	duplicate := old
	duplicate.ID = 0
	if err := db.Transaction(func(tx *gorm.DB) error { return tx.Create(&duplicate).Error }); err == nil || !strings.Contains(err.Error(), "cold archive") {
		t.Fatalf("archive accepted duplicate contract: %v", err)
	}
	second, err := service.Place(input)
	if err != nil || second.ID == first.ID || second.Status != "pending" {
		t.Fatalf("archive blocked/mutated a new operation: %+v / %v", second, err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := allowLifecycleDeletes(tx); err != nil {
			return err
		}
		count, err := restoreRobotBetArchive(tx, archiveRequest)
		if err == nil && count != 1 {
			return fmt.Errorf("restored %d rows, want 1", count)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if restored := robotBetHotJSON(t, db, first.ID); restored != snapshot {
		t.Fatalf("restore changed generated request or contract:\n%s\n%s", snapshot, restored)
	}
	placementPostgresAssertDebitEvidence(t, db, placementPostgresRow(t, db, second.ID), 2000)
}
