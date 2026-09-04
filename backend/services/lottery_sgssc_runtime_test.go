package services

import (
	"backend/data/models/lottery"
	"fmt"
	"testing"
	"time"
)

func TestSGSSCRuntimeRecoveryKeepsFutureSourceFailureRetryable(t *testing.T) {
	drawAt := time.Date(2026, 9, 3, 16, 0, 0, 0, time.UTC) // Issue 20260903288, next calendar midnight.
	sealAt := drawAt.Add(-30 * time.Second)
	candidate := settlementCandidate{
		GameID: "sg-ssc", Issue: "20260903288", Pending: 1,
		GameExists: true, GameEnabled: true, SourceKind: "external", IssueID: 1,
		IssueStatus: lottery.IssueStatusError, IssueSealAt: &sealAt,
		IssueSourceMode: "external", IssueScheduledDrawAt: &drawAt, IssueLastError: "fixture 115 temporarily unavailable",
		OldestBetAt: drawAt.Add(-4 * time.Minute),
	}
	for _, beforeDraw := range []time.Duration{time.Minute, 15 * time.Second} {
		now := drawAt.Add(-beforeDraw)
		action, reason := recoveryActionForCandidate(candidate, now)
		if action != recoveryDefer {
			t.Fatalf("a temporary source error before the actual SG draw must remain retryable, not permanently reconcile the active period: action=%v reason=%q", action, reason)
		}
	}
}

func TestSGSSCRuntimeRecoveryPreservesClosedAndUnknownBoundaries(t *testing.T) {
	drawAt := time.Date(2026, 9, 3, 16, 0, 0, 0, time.UTC)
	sealAt := drawAt.Add(-30 * time.Second)
	base := settlementCandidate{
		GameID: "sg-ssc", Issue: "20260903288", Pending: 1, GameExists: true, GameEnabled: true,
		SourceKind: "external", IssueID: 1, IssueStatus: lottery.IssueStatusError, IssueSealAt: &sealAt,
		IssueSourceMode: "external", IssueScheduledDrawAt: &drawAt, IssueLastError: "fixture source error",
	}
	for _, test := range []struct {
		name   string
		change func(*settlementCandidate)
		now    time.Time
	}{
		{"draw boundary", func(*settlementCandidate) {}, drawAt},
		{"past draw boundary", func(*settlementCandidate) {}, drawAt.Add(time.Second)},
		{"already reconciled", func(c *settlementCandidate) { c.IssueLastError = " 对账异常：历史冲突" }, drawAt.Add(-time.Minute)},
		{"platform lifecycle", func(c *settlementCandidate) { c.IssueSourceMode = "platform" }, drawAt.Add(-time.Minute)},
		{"legacy lifecycle", func(c *settlementCandidate) { c.IssueSourceMode = "legacy" }, drawAt.Add(-time.Minute)},
		{"missing lifecycle mode", func(c *settlementCandidate) { c.IssueSourceMode = "" }, drawAt.Add(-time.Minute)},
		{"wrong game source", func(c *settlementCandidate) { c.SourceKind = "platform" }, drawAt.Add(-time.Minute)},
		{"missing recorded draw boundary", func(c *settlementCandidate) { c.IssueScheduledDrawAt = nil }, drawAt.Add(-time.Minute)},
		{"shifted recorded boundary", func(c *settlementCandidate) { shifted := drawAt.Add(sgSSCInterval); c.IssueScheduledDrawAt = &shifted }, drawAt.Add(-time.Minute)},
		{"invalid issue", func(c *settlementCandidate) { c.Issue = "20260903289" }, drawAt.Add(-time.Minute)},
		{"unknown error", func(c *settlementCandidate) { c.IssueLastError = "" }, drawAt.Add(-time.Minute)},
		{"disabled game", func(c *settlementCandidate) { c.GameEnabled = false }, drawAt.Add(-time.Minute)},
		{"unrelated game unchanged", func(c *settlementCandidate) { c.GameID = "speed-ssc" }, drawAt.Add(-time.Minute)},
		{"terminal lifecycle unchanged", func(c *settlementCandidate) { c.IssueStatus = lottery.IssueStatusSettled }, drawAt.Add(-time.Minute)},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			test.change(&candidate)
			if action, reason := recoveryActionForCandidate(candidate, test.now); action != recoveryMarkAbnormal {
				t.Fatalf("unsafe/terminal candidate must keep its reconciliation path: action=%v reason=%q", action, reason)
			}
		})
	}
	base.DrawID = 9
	if action, _ := recoveryActionForCandidate(base, drawAt.Add(time.Minute)); action != recoverySettle {
		t.Fatal("a recovered draw must still enter the existing revision-gated settlement path")
	}
}

func TestSGSSCRuntimeMidnightWindowAndFreshnessTransition(t *testing.T) {
	for _, issue := range []string{"20260903287", "20260903288", "20260904001", "20260904023", "20260904024"} {
		t.Run(issue, func(t *testing.T) {
			fixture := newSGSSCTestFixture(issue)
			draws, err := fixture.fetch()
			if err != nil {
				t.Fatal(err)
			}
			schedule, err := scheduleFromDraws(lottery.Game{ID: "sg-ssc", DrawInterval: 300}, draws)
			if err != nil || schedule.Source != "upstream" || schedule.Interval != 300 || schedule.Issue != draws[23].NextIssue || !schedule.DrawAt.Equal(draws[23].NextDrawAt) {
				t.Fatalf("midnight schedule lost the validated next metadata: %+v %v", schedule, err)
			}
			game := sgSSCIntegrationHealthyGame(fixture.now)
			game.NextIssue, game.NextDrawAt = schedule.Issue, schedule.DrawAt
			if !sgSSCSourceHealthyAt(&game, fixture.now) {
				t.Fatal("fresh matched current window must be healthy")
			}
			if sgSSCSourceHealthyAt(&game, schedule.DrawAt) {
				t.Fatal("old successful sync must not carry betting through the draw boundary")
			}
			game.SyncStatus, game.LastSyncError = "error", "fixture station unavailable"
			if sgSSCSourceHealthyAt(&game, fixture.now) {
				t.Fatal("single-station failure must close source health")
			}
			game.SyncStatus = "syncing"
			if sgSSCSourceHealthyAt(&game, fixture.now) {
				t.Fatal("retry must retain the failure until the full window verifies")
			}
			game.SyncStatus, game.LastSyncError = "ok", ""
			if !sgSSCSourceHealthyAt(&game, fixture.now) {
				t.Fatal("a fully verified recovery should restore source health")
			}
		})
	}
}

func TestSGSSCRuntimeAutomaticRecoveryStopsAt24MissedPeriods(t *testing.T) {
	lastGoodAt := time.Date(2026, 9, 3, 15, 50, 0, 0, time.UTC)
	firstMissed := sgSSCIssueAt(lastGoodAt.Add(sgSSCInterval))
	for _, missedPeriods := range []int{1, 24, 25, 289} {
		t.Run(fmt.Sprintf("missed=%d", missedPeriods), func(t *testing.T) {
			latestAt := lastGoodAt.Add(time.Duration(missedPeriods) * sgSSCInterval)
			fixture := newSGSSCTestFixture(sgSSCIssueAt(latestAt))
			draws, err := fixture.fetch()
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, draw := range draws {
				found = found || draw.Issue == firstMissed
			}
			if found != (missedPeriods <= 24) || len(draws) != 24 || len(fixture.calls) > 6 {
				t.Fatalf("bounded recovery manufactured history or lost an in-range missed period: found=%v rows=%d calls=%d", found, len(draws), len(fixture.calls))
			}
		})
	}
}
