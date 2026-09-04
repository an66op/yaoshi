package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

// These tests use the existing opt-in, transaction-rolled-back loopback fixture.
// Source responses are injected; no live HTTP or application database is used.
func sgSSCRuntimePostgresSync(db *gorm.DB, fail bool) SourceSyncResult {
	return NewLotteryService(db).syncOfficialGameWithPublisher(context.Background(), "sg-ssc", func(context.Context) ([]sourceDraw, error) {
		if fail {
			return nil, errors.New("fixture 115 temporarily unavailable")
		}
		return sgSSCIntegrationBatch(time.Now()), nil
	}, func(lottery.Game) {})
}

func TestSGSSCRuntimePostgresTransientFailureResumesBeforeSeal(t *testing.T) {
	db, service, member, game := sgSSCPlacementPostgresFixture(t)
	first, err := service.Place(placementPostgresInput(member, game, "2", 20))
	if err != nil {
		t.Fatal(err)
	}
	before := timingPostgresMoney(t, db, member.UserID)
	if result := sgSSCRuntimePostgresSync(db, true); result.Status != "error" {
		t.Fatalf("source failure was not recorded: %+v", result)
	}
	if _, err := service.Place(placementPostgresInput(member, game, "4", 20)); apperrors.GetErrorCode(err) != "SOURCE_UNAVAILABLE" {
		t.Fatalf("outage did not stop new tickets: %v", err)
	}
	result, err := service.RecoverSettlementBacklog(context.Background(), 100, "SG short outage fixture")
	if err != nil || result.DeferredIssues != 1 || result.MarkedAbnormalBets != 0 || len(result.Failures) != 0 {
		t.Fatalf("future source outage was prematurely reconciled: %+v %v", result, err)
	}
	stored := placementPostgresRow(t, db, first.ID)
	issue := rolloverPostgresIssue(t, db, game.ID, game.NextIssue)
	if stored.Status != "pending" || stored.ReconciliationStatus == "abnormal" || strings.HasPrefix(issue.LastError, "对账异常：") {
		t.Fatalf("temporary error became durable settlement debt: bet=%+v issue=%+v", stored, issue)
	}
	if after := timingPostgresMoney(t, db, member.UserID); !reflect.DeepEqual(before, after) {
		t.Fatalf("failed sync/recovery changed money: before=%+v after=%+v", before, after)
	}
	if result := sgSSCRuntimePostgresSync(db, false); result.Status != "ok" {
		t.Fatalf("verified recovery failed: %+v", result)
	}
	if err := db.First(game, "id = ?", game.ID).Error; err != nil {
		t.Fatal(err)
	}
	issue = rolloverPostgresIssue(t, db, game.ID, game.NextIssue)
	if !sourceHealthyForGame(game) || issue.Status != lottery.IssueStatusAccepting || issue.LastError != "" {
		t.Fatalf("verified recovery before seal did not reopen the same safe issue: game=%+v issue=%+v", game, issue)
	}
	if _, err := service.Place(placementPostgresInput(member, game, "4", 20)); err != nil {
		t.Fatalf("new verified ticket after recovery rejected: %v", err)
	}
}

func TestSGSSCRuntimePostgresRecoveryNeverExtendsSealedWindow(t *testing.T) {
	db, service, member, game := sgSSCPlacementPostgresFixture(t)
	if _, err := service.Place(placementPostgresInput(member, game, "2", 20)); err != nil {
		t.Fatal(err)
	}
	// Materialize an already-closed platform/room cutoff without sleeping until
	// the real draw boundary. The authoritative ScheduledDrawAt stays unchanged.
	sealAt := time.Now().UTC().Truncate(time.Second).Add(-time.Second)
	if err := db.Model(&lottery.IssueWindow{}).Where("game_id = ? AND issue = ?", game.ID, game.NextIssue).
		Updates(map[string]any{"seal_at": sealAt, "seal_seconds": int(game.NextDrawAt.Sub(sealAt) / time.Second)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&lottery.Issue{}).Where("game_id = ? AND issue = ?", game.ID, game.NextIssue).
		Updates(map[string]any{"status": lottery.IssueStatusSealed, "seal_at": sealAt}).Error; err != nil {
		t.Fatal(err)
	}
	before := timingPostgresMoney(t, db, member.UserID)
	if result := sgSSCRuntimePostgresSync(db, true); result.Status != "error" {
		t.Fatal(result)
	}
	if result, err := service.RecoverSettlementBacklog(context.Background(), 100, "SG sealed recovery fixture"); err != nil || result.MarkedAbnormalBets != 0 {
		t.Fatalf("future draw was prematurely reconciled after room seal: %+v %v", result, err)
	}
	if result := sgSSCRuntimePostgresSync(db, false); result.Status != "ok" {
		t.Fatal(result)
	}
	issue := rolloverPostgresIssue(t, db, game.ID, game.NextIssue)
	if issue.Status != lottery.IssueStatusSealed || !issue.SealAt.Equal(sealAt) || issue.ScheduledDrawAt == nil || !issue.ScheduledDrawAt.Equal(game.NextDrawAt) {
		t.Fatalf("recovery extended or reopened a frozen cutoff: %+v", issue)
	}
	if _, err := service.Place(placementPostgresInput(member, game, "4", 20)); apperrors.GetErrorCode(err) != "ISSUE_CLOSED" {
		t.Fatalf("sealed room reopened after source recovery: %v", err)
	}
	if after := timingPostgresMoney(t, db, member.UserID); !reflect.DeepEqual(before, after) {
		t.Fatalf("sealed recovery changed money: before=%+v after=%+v", before, after)
	}
}

func TestSGSSCRuntimePostgresRecoveryPreservesReconciliationAndLegacy(t *testing.T) {
	for _, scenario := range []string{"existing reconciliation", "platform lifecycle", "legacy ticket"} {
		t.Run(scenario, func(t *testing.T) {
			db, service, member, game := sgSSCPlacementPostgresFixture(t)
			first, err := service.Place(placementPostgresInput(member, game, "2", 20))
			if err != nil {
				t.Fatal(err)
			}
			message := "对账异常：fixture requires explicit historical review"
			switch scenario {
			case "existing reconciliation":
				err = db.Model(&lottery.Issue{}).Where("game_id = ? AND issue = ?", game.ID, game.NextIssue).
					Updates(map[string]any{"status": lottery.IssueStatusError, "last_error": message}).Error
			case "platform lifecycle":
				err = db.Model(&lottery.Issue{}).Where("game_id = ? AND issue = ?", game.ID, game.NextIssue).Update("source_mode", "platform").Error
			case "legacy ticket":
				err = db.Model(&bet.Bet{}).Where("id = ?", first.ID).Update("draw_source_revision", "").Error
			}
			if err != nil {
				t.Fatal(err)
			}
			before := timingPostgresMoney(t, db, member.UserID)
			result := sgSSCRuntimePostgresSync(db, false)
			issue := rolloverPostgresIssue(t, db, game.ID, game.NextIssue)
			if scenario == "existing reconciliation" {
				if result.Status != "ok" || issue.Status != lottery.IssueStatusError || issue.LastError != message {
					t.Fatalf("successful source sync cleared existing reconciliation: %+v %+v", result, issue)
				}
			} else if result.Status != "error" || apperrors.GetErrorCode(sgSSCIssueEvidenceError(db, game.NextIssue, &issue)) != "DRAW_SOURCE_UNVERIFIED" {
				t.Fatalf("source recovery claimed a legacy identity: %+v %+v", result, issue)
			}
			if _, err := service.Place(placementPostgresInput(member, game, "4", 20)); err == nil {
				t.Fatal("reconciled or legacy period reopened")
			}
			if after := timingPostgresMoney(t, db, member.UserID); !reflect.DeepEqual(before, after) {
				t.Fatalf("unsafe recovery changed money: before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestSGSSCRuntimePostgresOutOfWindowGapRemainsUnsettled(t *testing.T) {
	db, service, member, game := sgSSCPlacementPostgresFixture(t)
	batch := sgSSCIntegrationBatch(time.Now())
	oldAt := batch[0].DrawAt.Add(-sgSSCInterval) // Exactly one period beyond the automatic window.
	oldIssue := sgSSCIssueAt(oldAt)
	issue := lottery.Issue{GameID: game.ID, Issue: oldIssue, SourceMode: "external", Status: lottery.IssueStatusAwaiting,
		AcceptAt: oldAt.Add(-sgSSCInterval), SealAt: oldAt.Add(-30 * time.Second), ScheduledDrawAt: &oldAt}
	if err := db.Create(&issue).Error; err != nil {
		t.Fatal(err)
	}
	ticket := sgSSCPlacementLegacyTicket(member, game)
	ticket.Issue, ticket.DrawSourceRevision, ticket.RequestReference = oldIssue, sgSSCSourceRevision, "sg-out-of-window"
	if err := db.Create(&ticket).Error; err != nil {
		t.Fatal(err)
	}
	before := timingPostgresMoney(t, db, member.UserID)
	if result := sgSSCRuntimePostgresSync(db, false); result.Status != "ok" {
		t.Fatal(result)
	}
	result, err := service.RecoverSettlementBacklog(context.Background(), 100, "SG out-of-window fixture")
	if err != nil || result.MarkedAbnormalBets != 1 || result.Health.UnrecoverableBetCount < 1 || result.Health.Healthy {
		t.Fatalf("missing older result must remain explicit settlement debt: %+v %v", result, err)
	}
	// A later healthy live poll must not manufacture or reclassify that gap.
	if result := sgSSCRuntimePostgresSync(db, false); result.Status != "ok" {
		t.Fatal(result)
	}
	stored := placementPostgresRow(t, db, ticket.ID)
	var drawCount int64
	if err := db.Model(&lottery.Draw{}).Where("game_id = ? AND issue = ?", game.ID, oldIssue).Count(&drawCount).Error; err != nil {
		t.Fatal(err)
	}
	if drawCount != 0 || stored.Status != "pending" || stored.ReconciliationStatus != "abnormal" {
		t.Fatalf("older gap silently filled or settled: draws=%d ticket=%+v", drawCount, stored)
	}
	oldInput := placementPostgresInput(member, game, "4", 20)
	oldInput.Issue = oldIssue
	if _, err := service.Place(oldInput); err == nil {
		t.Fatal("live recovery reopened an expired, out-of-window issue")
	}
	if after := timingPostgresMoney(t, db, member.UserID); !reflect.DeepEqual(before, after) {
		t.Fatalf("out-of-window recovery changed money: before=%+v after=%+v", before, after)
	}
}

func TestSGSSCRuntimePostgresExpiredDrawCannotReopenFromStaleResponse(t *testing.T) {
	db, service, member, game := sgSSCPlacementPostgresFixture(t)
	oldAt := time.Now().UTC().Truncate(sgSSCInterval)
	oldIssue := sgSSCIssueAt(oldAt)
	issue := lottery.Issue{GameID: game.ID, Issue: oldIssue, SourceMode: "external", Status: lottery.IssueStatusError,
		AcceptAt: oldAt.Add(-sgSSCInterval), SealAt: oldAt.Add(-30 * time.Second), ScheduledDrawAt: &oldAt,
		LastError: "fixture source unavailable at draw boundary"}
	if err := db.Create(&issue).Error; err != nil {
		t.Fatal(err)
	}
	ticket := sgSSCPlacementLegacyTicket(member, game)
	ticket.Issue, ticket.DrawSourceRevision, ticket.RequestReference = oldIssue, sgSSCSourceRevision, "sg-expired-current"
	if err := db.Create(&ticket).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(game).Updates(map[string]any{"next_issue": oldIssue, "next_draw_at": oldAt, "sync_status": "error", "last_sync_error": issue.LastError}).Error; err != nil {
		t.Fatal(err)
	}
	before := timingPostgresMoney(t, db, member.UserID)
	if result, err := service.RecoverSettlementBacklog(context.Background(), 100, "SG expired draw fixture"); err != nil || result.MarkedAbnormalBets != 1 {
		t.Fatalf("overdue source error did not enter reconciliation: %+v %v", result, err)
	}
	stale := sgSSCIntegrationBatch(oldAt.Add(-sgSSCInterval))
	result := NewLotteryService(db).syncOfficialGameWithPublisher(context.Background(), game.ID, func(context.Context) ([]sourceDraw, error) {
		return stale, nil
	}, func(lottery.Game) {})
	if result.Status != "error" || result.Imported != 0 {
		t.Fatalf("cached old source response reopened the expired current issue: %+v", result)
	}
	if err := db.First(game, "id = ?", game.ID).Error; err != nil {
		t.Fatal(err)
	}
	stored := rolloverPostgresIssue(t, db, game.ID, oldIssue)
	if sourceHealthyForGame(game) || game.NextIssue != oldIssue || !game.NextDrawAt.Equal(oldAt) ||
		stored.Status != lottery.IssueStatusError || !strings.HasPrefix(stored.LastError, "对账异常：") {
		t.Fatalf("expired/reconciled period was extended or cleared: game=%+v issue=%+v", game, stored)
	}
	if _, err := service.Place(placementPostgresInput(member, game, "4", 20)); err == nil {
		t.Fatal("expired source period accepted a new ticket")
	}
	if after := timingPostgresMoney(t, db, member.UserID); !reflect.DeepEqual(before, after) {
		t.Fatalf("expired recovery changed money: before=%+v after=%+v", before, after)
	}
}
