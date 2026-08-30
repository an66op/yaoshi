package services

import (
	"backend/data/models/lottery"
	"testing"
	"time"
)

func TestGameTimingSummaryUsesRoomWindowAndOneIssueSnapshot(t *testing.T) {
	drawAt := time.Date(2026, 8, 30, 5, 31, 15, 0, time.UTC)
	game := &lottery.Game{ID: "speed-fly", DrawInterval: 75}
	window := newIssueWindow(5, game, "54776109", drawAt, 10)
	// The platform closes 30 seconds early; this room closes 10 seconds early.
	lifecycle := &lottery.Issue{GameID: game.ID, Issue: window.Issue, Status: lottery.IssueStatusSealed, SealAt: drawAt.Add(-30 * time.Second), ScheduledDrawAt: &drawAt}
	for _, tc := range []struct {
		name   string
		before time.Duration
		status string
	}{
		{"before acceptance", 76 * time.Second, lottery.IssueStatusPending},
		{"room still accepting despite platform seal", 20 * time.Second, lottery.IssueStatusAccepting},
		{"exact room cutoff", 10 * time.Second, lottery.IssueStatusSealed},
		{"exact draw boundary", 0, lottery.IssueStatusAwaiting},
		{"delayed source does not reopen", -2 * time.Minute, lottery.IssueStatusAwaiting},
	} {
		t.Run(tc.name, func(t *testing.T) {
			summary := GameSummary{ID: game.ID, Issue: "54776108", DrawInterval: 180, SealSeconds: 30, TimingSource: "observed"}
			applyGameTimingSummary(&summary, lifecycle, &window, drawAt.Add(-tc.before))
			if summary.IssueStatus != tc.status {
				t.Fatalf("status = %q, want %q", summary.IssueStatus, tc.status)
			}
			if summary.CurrentIssue != window.Issue || summary.Issue != "54776108" || !summary.NextDrawAt.Equal(drawAt) {
				t.Fatalf("mixed issue/draw snapshots: %+v", summary)
			}
			if summary.DrawInterval != 75 || summary.SealSeconds != 10 || summary.SealAt == nil || !summary.SealAt.Equal(window.SealAt) || summary.AcceptAt == nil || !summary.AcceptAt.Equal(window.AcceptAt) {
				t.Fatalf("room timing was not applied: %+v", summary)
			}
		})
	}
}

func TestGameTimingSummaryKeepsAuthoritativeClosedStates(t *testing.T) {
	drawAt := time.Date(2026, 8, 30, 5, 31, 15, 0, time.UTC)
	game := &lottery.Game{ID: "speed-fly", DrawInterval: 75}
	window := newIssueWindow(5, game, "54776109", drawAt, 0)
	for _, status := range []string{lottery.IssueStatusError, lottery.IssueStatusSettling, lottery.IssueStatusSettled} {
		t.Run(status, func(t *testing.T) {
			summary := GameSummary{ID: game.ID}
			lifecycle := &lottery.Issue{GameID: game.ID, Issue: window.Issue, Status: status}
			applyGameTimingSummary(&summary, lifecycle, &window, drawAt.Add(-20*time.Second))
			if summary.IssueStatus != status || summary.SealSeconds != 0 {
				t.Fatalf("closed state was replaced: %+v", summary)
			}
		})
	}
	summary := GameSummary{ID: game.ID}
	lifecycle := &lottery.Issue{GameID: game.ID, Issue: window.Issue, Status: lottery.IssueStatusAccepting, DrawAt: &drawAt}
	applyGameTimingSummary(&summary, lifecycle, &window, drawAt.Add(-20*time.Second))
	if summary.IssueStatus != lottery.IssueStatusSettling {
		t.Fatalf("a published result must stop acceptance: %q", summary.IssueStatus)
	}
}

func TestGameTimingSummaryDoesNotExposeMismatchedOrMissingWindow(t *testing.T) {
	drawAt := time.Date(2026, 8, 30, 5, 31, 15, 0, time.UTC)
	lifecycle := &lottery.Issue{GameID: "speed-fly", Issue: "54776109", Status: lottery.IssueStatusAccepting}
	wrongIssue := newIssueWindow(5, &lottery.Game{ID: "speed-fly", DrawInterval: 75}, "54776108", drawAt, 30)
	wrongGame := newIssueWindow(5, &lottery.Game{ID: "speed-racing", DrawInterval: 75}, lifecycle.Issue, drawAt, 30)
	for _, window := range []*lottery.IssueWindow{nil, &wrongIssue, &wrongGame} {
		summary := GameSummary{ID: lifecycle.GameID, NextDrawAt: drawAt, AcceptAt: &drawAt, SealAt: &drawAt}
		applyGameTimingSummary(&summary, lifecycle, window, drawAt.Add(-40*time.Second))
		if summary.IssueStatus != lottery.IssueStatusAwaiting || !summary.NextDrawAt.IsZero() || summary.AcceptAt != nil || summary.SealAt != nil {
			t.Fatalf("missing/mismatched window leaked an accepting schedule: %+v", summary)
		}
	}
}
