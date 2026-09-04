package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/data/models/user"
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"gorm.io/gorm"
)

type sgSSCBackfillGuardPostgresFixture struct {
	db      *gorm.DB
	now     time.Time
	claim   lottery.SGSSCBackfillItem
	member  user.User
	ticket  bet.Bet
	service *BetAdminService
}

// Deliberately no live placement/source poll: accepted historic receipts and
// their verified Draw are seeded in the opt-in rollback-only timing database.
func newSGSSCBackfillGuardPostgresFixture(t *testing.T, withTicket bool) sgSSCBackfillGuardPostgresFixture {
	t.Helper()
	db := timingPostgresDatabase(t)
	now := time.Now().UTC().Truncate(time.Second)
	at := now.Truncate(sgSSCInterval).Add(-3 * time.Hour)
	issue := sgSSCIssueAt(at)
	if err := db.Model(&lottery.Game{}).Where("id = ?", "sg-ssc").Updates(map[string]any{
		"enabled": true, "source_kind": "external", "source_name": sgSSCVerifiedSourceName, "source_url": sgSSCVerifiedSourceURL,
	}).Error; err != nil {
		t.Fatal(err)
	}
	room := timingPostgresRoom(t, db, "sg_guard_room", "783041")
	member := timingPostgresMember(t, db, room, "sg_guard_member")
	draw := lottery.Draw{GameID: "sg-ssc", Issue: issue, DrawAt: at, Numbers: "6,5,8,3,0", SourceRevision: sgSSCSourceRevision, ConversionRevision: sgSSCConversionRevision}
	if err := db.Create(&draw).Error; err != nil {
		t.Fatal(err)
	}
	until := now.Add(sgSSCBackfillLease)
	claim := lottery.SGSSCBackfillItem{Issue: issue, DrawAt: at, Status: "running", Reason: "pending_bet", Attempts: 1,
		NextRetryAt: now, LeaseUntil: &until, RequestedBy: "SG guard fixture", RequestTrigger: "admin", RequestID: "sg-guard-request",
		CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&claim).Error; err != nil {
		t.Fatal(err)
	}
	journal := lottery.SGSSCBackfillAttempt{Issue: issue, Attempt: 1, Status: "running", Trigger: claim.RequestTrigger,
		Operator: claim.RequestedBy, RequestID: claim.RequestID, StartedAt: now, SourceRevision: sgSSCSourceRevision, ConversionRevision: sgSSCConversionRevision}
	if err := db.Create(&journal).Error; err != nil {
		t.Fatal(err)
	}
	fixture := sgSSCBackfillGuardPostgresFixture{db: db, now: now, claim: claim, member: member, service: NewBetAdminService(db)}
	fixture.service.suppressNotifications = true
	if withTicket {
		fixture.ticket = bet.Bet{WorkspaceID: room.ID, GameID: "sg-ssc", Issue: issue, RoomScope: room.Scope, UserID: member.UserID,
			Username: member.Username, PlayCode: "two_sided", PlayName: "两面", Position: 1, Selection: "大", AmountCents: 100,
			Odds: 2, Status: "pending", RuleVersion: "digits5-v3", DrawSourceRevision: sgSSCSourceRevision, RequestReference: "sg-guard-bet"}
		if err := db.Create(&fixture.ticket).Error; err != nil {
			t.Fatal(err)
		}
	}
	ready, err := NewLotteryService(db).prepareSGSSCBackfill(context.Background(), claim, nil, now)
	if err != nil || !ready {
		t.Fatalf("trusted preflight failed: ready=%t err=%v", ready, err)
	}
	return fixture
}

func sgSSCBackfillGuardPostgresMutation(db *gorm.DB, claim lottery.SGSSCBackfillItem, now time.Time, mutation string) error {
	switch mutation {
	case "disabled":
		return db.Model(&lottery.Game{}).Where("id = ?", "sg-ssc").Update("enabled", false).Error
	case "platform source":
		return db.Model(&lottery.Game{}).Where("id = ?", "sg-ssc").Updates(map[string]any{"source_kind": "platform", "source_name": "old fixture source", "source_url": ""}).Error
	case "changed external source":
		return db.Model(&lottery.Game{}).Where("id = ?", "sg-ssc").Update("source_url", "https://invalid.example/other-product").Error
	case "expired claim":
		return db.Model(&lottery.SGSSCBackfillItem{}).Where("issue = ?", claim.Issue).Update("lease_until", now.Add(-time.Second)).Error
	default:
		return fmt.Errorf("unknown fixture mutation %q", mutation)
	}
}

func TestSGSSCBackfillGuardPostgresRejectsBeforeLifecycleInitialization(t *testing.T) {
	for _, mutation := range []string{"disabled", "platform source", "changed external source", "expired claim"} {
		t.Run(mutation, func(t *testing.T) {
			f := newSGSSCBackfillGuardPostgresFixture(t, true)
			before := timingPostgresMoney(t, f.db, f.member.UserID)
			if err := sgSSCBackfillGuardPostgresMutation(f.db, f.claim, f.now, mutation); err != nil {
				t.Fatal(err)
			}
			gate := sgSSCBackfillSettlementGate(f.claim, func() time.Time { return f.now })
			settled, err := f.service.settleIssueGuarded("sg-ssc", f.claim.Issue, "old fixture worker", gate)
			if err == nil || settled != nil {
				t.Fatalf("invalidated preflight reached settlement: %+v err=%v", settled, err)
			}
			var count int64
			if err := f.db.Model(&lottery.Issue{}).Where("game_id = ? AND issue = ?", "sg-ssc", f.claim.Issue).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("invalidated worker created a lifecycle, possibly poisoning SG history as platform: %d", count)
			}
			if after := timingPostgresMoney(t, f.db, f.member.UserID); !reflect.DeepEqual(after, before) {
				t.Fatalf("invalidated worker changed receipts or money: before=%+v after=%+v", before, after)
			}
		})
	}
}

// The pending query occurs after lifecycle initialization and before the money
// transaction. Injecting there simulates a competing state change without
// another connection (the shared fixture's schema itself is uncommitted).
func sgSSCBackfillGuardAfterPendingRead(t *testing.T, db *gorm.DB, callback func() error) *bool {
	t.Helper()
	fired := false
	name := "test:sg_backfill_guard_after_pending_read"
	if err := db.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		if fired || tx.Error != nil {
			return
		}
		if _, pendingRows := tx.Statement.Dest.(*[]bet.Bet); !pendingRows {
			return
		}
		fired = true
		if err := callback(); err != nil {
			tx.AddError(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(name) })
	return &fired
}

func TestSGSSCBackfillGuardPostgresRechecksBeforeMoneyAndEmptyCompletion(t *testing.T) {
	for _, withTicket := range []bool{true, false} {
		for _, mutation := range []string{"disabled", "platform source", "changed external source", "expired claim"} {
			t.Run(fmt.Sprintf("ticket=%t/%s", withTicket, mutation), func(t *testing.T) {
				f := newSGSSCBackfillGuardPostgresFixture(t, withTicket)
				before := timingPostgresMoney(t, f.db, f.member.UserID)
				fired := sgSSCBackfillGuardAfterPendingRead(t, f.db, func() error {
					return sgSSCBackfillGuardPostgresMutation(f.db, f.claim, f.now, mutation)
				})
				calls := 0
				base := sgSSCBackfillSettlementGate(f.claim, func() time.Time { return f.now })
				gate := func(tx *gorm.DB) error { calls++; return base(tx) }
				settled, err := f.service.settleIssueGuarded("sg-ssc", f.claim.Issue, "late fixture worker", gate)
				if !*fired || calls < 2 || err == nil || settled != nil {
					t.Fatalf("money/completion gate was bypassed: injected=%t checks=%d settled=%+v err=%v", *fired, calls, settled, err)
				}
				issue := rolloverPostgresIssue(t, f.db, "sg-ssc", f.claim.Issue)
				if issue.SourceMode != "external" || issue.Status != lottery.IssueStatusSettling || issue.LastError != "" || issue.SettledAt != nil {
					t.Fatalf("rejected gate changed lifecycle or wrote an unfenced failure: %+v", issue)
				}
				if after := timingPostgresMoney(t, f.db, f.member.UserID); !reflect.DeepEqual(after, before) {
					t.Fatalf("late invalidation paid or settled: before=%+v after=%+v", before, after)
				}
			})
		}
	}
}

func TestSGSSCBackfillGuardPostgresOldGenerationCannotOverwriteNewerSettlement(t *testing.T) {
	f := newSGSSCBackfillGuardPostgresFixture(t, true)
	before := timingPostgresMoney(t, f.db, f.member.UserID)
	var newerClaim lottery.SGSSCBackfillItem
	fired := sgSSCBackfillGuardAfterPendingRead(t, f.db, func() error {
		lotteryService := NewLotteryService(f.db)
		expiredAt := f.now.Add(sgSSCBackfillLease + time.Second)
		if _, err := lotteryService.claimSGSSCBackfills(context.Background(), expiredAt); err != nil {
			return err
		}
		newNow := expiredAt.Add(6 * time.Minute) // Past the abandoned attempt's initial retry delay.
		claims, err := lotteryService.claimSGSSCBackfills(context.Background(), newNow)
		if err != nil || len(claims) != 1 || claims[0].Attempts != f.claim.Attempts+1 {
			return fmt.Errorf("newer fixture claim: claims=%+v err=%v", claims, err)
		}
		newerClaim = claims[0]
		if ready, err := lotteryService.prepareSGSSCBackfill(context.Background(), newerClaim, nil, newNow); err != nil || !ready {
			return fmt.Errorf("newer fixture preflight: ready=%t err=%v", ready, err)
		}
		newerGate := sgSSCBackfillSettlementGate(newerClaim, func() time.Time { return newNow })
		newer, err := f.service.settleIssueGuarded("sg-ssc", f.claim.Issue, "newer fixture worker", newerGate)
		if err != nil || newer == nil || newer.Won != 1 {
			return fmt.Errorf("newer fixture settlement: %+v err=%v", newer, err)
		}
		return lotteryService.finishSGSSCBackfill(context.Background(), newerClaim, newNow, "completed", "recovered", "", newer.Won+newer.Lost+newer.Push)
	})
	oldGate := sgSSCBackfillSettlementGate(f.claim, func() time.Time { return f.now })
	old, err := f.service.settleIssueGuarded("sg-ssc", f.claim.Issue, "superseded fixture worker", oldGate)
	if !*fired || err == nil || old != nil || newerClaim.Attempts != 2 {
		t.Fatalf("old generation unexpectedly completed: injected=%t newer=%+v old=%+v err=%v", *fired, newerClaim, old, err)
	}
	issue := rolloverPostgresIssue(t, f.db, "sg-ssc", f.claim.Issue)
	if issue.Status != lottery.IssueStatusSettled || issue.LastError != "" || issue.SettledAt == nil || issue.SourceMode != "external" {
		t.Fatalf("old worker downgraded the newer settled lifecycle: %+v", issue)
	}
	after := timingPostgresMoney(t, f.db, f.member.UserID)
	if after.BalanceCents != before.BalanceCents+200 || after.LedgerRows != before.LedgerRows+1 || after.Pending != before.Pending-1 {
		t.Fatalf("new generation must pay exactly once and old generation never pay: before=%+v after=%+v", before, after)
	}
	var attempts []lottery.SGSSCBackfillAttempt
	if err := f.db.Where("issue = ?", f.claim.Issue).Order("attempt ASC").Find(&attempts).Error; err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 2 || attempts[0].Status != "interrupted" || attempts[1].Status != "recovered" || attempts[1].SettledBets != 1 {
		t.Fatalf("reclaimed generations lost their independent audit: %+v", attempts)
	}
}

func TestSGSSCBackfillGuardPostgresSuccessfulSettlementAndImmutableAttempt(t *testing.T) {
	f := newSGSSCBackfillGuardPostgresFixture(t, true)
	before := timingPostgresMoney(t, f.db, f.member.UserID)
	gate := sgSSCBackfillSettlementGate(f.claim, func() time.Time { return f.now })
	settled, err := f.service.settleIssueGuarded("sg-ssc", f.claim.Issue, "successful fixture worker", gate)
	if err != nil || settled == nil || settled.Won != 1 {
		t.Fatalf("valid generation did not settle: %+v err=%v", settled, err)
	}
	after := timingPostgresMoney(t, f.db, f.member.UserID)
	if after.BalanceCents != before.BalanceCents+200 || after.LedgerRows != before.LedgerRows+1 {
		t.Fatalf("trusted guard did not preserve exact financial receipt: before=%+v after=%+v", before, after)
	}
	if again, err := f.service.settleIssueGuarded("sg-ssc", f.claim.Issue, "same fixture generation retry", gate); err != nil || again == nil || again.Won != 0 {
		t.Fatalf("same generation did not retain idempotency: %+v err=%v", again, err)
	}
	if got := timingPostgresMoney(t, f.db, f.member.UserID); !reflect.DeepEqual(got, after) {
		t.Fatalf("retry changed financial receipts: before=%+v after=%+v", after, got)
	}
	if err := NewLotteryService(f.db).finishSGSSCBackfill(context.Background(), f.claim, f.now, "completed", "recovered", "", 1); err != nil {
		t.Fatal(err)
	}
	var frozen lottery.SGSSCBackfillAttempt
	if err := f.db.Where("issue = ? AND attempt = ?", f.claim.Issue, f.claim.Attempts).First(&frozen).Error; err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*gorm.DB) error{
		"rewrite error": func(tx *gorm.DB) error {
			return tx.Model(&lottery.SGSSCBackfillAttempt{}).Where("id = ?", frozen.ID).Update("error", "rewritten").Error
		},
		"reopen attempt": func(tx *gorm.DB) error {
			return tx.Model(&lottery.SGSSCBackfillAttempt{}).Where("id = ?", frozen.ID).Updates(map[string]any{"status": "running", "finished_at": nil}).Error
		},
		"delete attempt": func(tx *gorm.DB) error {
			return tx.Where("id = ?", frozen.ID).Delete(&lottery.SGSSCBackfillAttempt{}).Error
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := f.db.Transaction(mutate); err == nil {
				t.Fatal("database allowed mutation of a finished attempt")
			}
			var unchanged lottery.SGSSCBackfillAttempt
			if err := f.db.First(&unchanged, frozen.ID).Error; err != nil || !reflect.DeepEqual(frozen, unchanged) {
				t.Fatalf("immutable journal changed: before=%+v after=%+v err=%v", frozen, unchanged, err)
			}
		})
	}
}

func TestSGSSCBackfillGuardPostgresFailureDoesNotDowngradeSettledIssue(t *testing.T) {
	f := newSGSSCBackfillGuardPostgresFixture(t, true)
	before := timingPostgresMoney(t, f.db, f.member.UserID)
	fired := sgSSCBackfillGuardAfterPendingRead(t, f.db, func() error {
		// Ordinary recovery may win without changing this history claim. The
		// old pending snapshot remains in the outer worker, but the row is paid.
		settled, err := f.service.SettleIssue("sg-ssc", f.claim.Issue, "ordinary recovery wins")
		if err != nil || settled == nil || settled.Won != 1 {
			return fmt.Errorf("ordinary fixture recovery: %+v err=%v", settled, err)
		}
		return nil
	})
	checks := 0
	base := sgSSCBackfillSettlementGate(f.claim, func() time.Time { return f.now })
	gate := func(tx *gorm.DB) error {
		checks++
		if err := base(tx); err != nil {
			return err
		}
		if checks == 2 {
			return errors.New("injected financial transaction failure after competing settlement")
		}
		return nil
	}
	settled, err := f.service.settleIssueGuarded("sg-ssc", f.claim.Issue, "failed stale snapshot", gate)
	if !*fired || checks != 3 || err == nil || settled != nil {
		t.Fatalf("fixture missed the authorized error-write gate: injected=%t checks=%d result=%+v err=%v", *fired, checks, settled, err)
	}
	issue := rolloverPostgresIssue(t, f.db, "sg-ssc", f.claim.Issue)
	after := timingPostgresMoney(t, f.db, f.member.UserID)
	if issue.Status != lottery.IssueStatusSettled || issue.LastError != "" || issue.SettledAt == nil ||
		after.BalanceCents != before.BalanceCents+200 || after.LedgerRows != before.LedgerRows+1 || after.Pending != before.Pending-1 {
		t.Fatalf("authorized failure handler overwrote completed settlement: issue=%+v before=%+v after=%+v", issue, before, after)
	}
}
