package services

import (
	"backend/data/models/lottery"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func nextBettingFixture() (lottery.Game, lottery.Issue, []lottery.Draw, time.Time) {
	now := time.Date(2026, 8, 30, 12, 0, 1, 0, time.UTC)
	drawAt := now.Add(-time.Second)
	syncAt := now.Add(-2 * time.Second)
	game := lottery.Game{ID: "speed-racing", Enabled: true, SourceKind: "external", SyncStatus: "ok",
		TimingSource: "upstream", NextIssue: "34137173", NextDrawAt: drawAt, DrawInterval: 75, LastSyncAt: &syncAt}
	current := lottery.Issue{GameID: game.ID, Issue: game.NextIssue, Status: lottery.IssueStatusAwaiting, ScheduledDrawAt: &drawAt}
	draws := make([]lottery.Draw, 4)
	for i := range draws {
		draws[i] = lottery.Draw{GameID: game.ID, Issue: strconv.Itoa(34137172 - i), DrawAt: drawAt.Add(-time.Duration(i+1) * 75 * time.Second)}
	}
	return game, current, draws, now
}

func TestNextBettingScheduleIsSeparateAndBounded(t *testing.T) {
	for _, gameID := range []string{"speed-racing", "speed-fly", "speed-ssc"} {
		t.Run(gameID, func(t *testing.T) {
			game, current, draws, now := nextBettingFixture()
			game.ID, current.GameID = gameID, gameID
			for i := range draws {
				draws[i].GameID = gameID
			}
			before := game
			next, ok := nextBettingSchedule(&game, &current, draws, now)
			if !ok || next.NextIssue != "34137174" || !next.NextDrawAt.Equal(game.NextDrawAt.Add(75*time.Second)) {
				t.Fatalf("confirmed next period not exposed: %+v, %v", next, ok)
			}
			if !reflect.DeepEqual(game, before) || current.Status != lottery.IssueStatusAwaiting {
				t.Fatal("next acceptance replaced the old draw tracker")
			}
			for attempt := 0; attempt < 3; attempt++ {
				again, yes := nextBettingSchedule(&game, &current, draws, now)
				if !yes || !reflect.DeepEqual(again, next) {
					t.Fatal("re-reading advanced the projection again")
				}
			}
			if _, ok := nextBettingSchedule(&game, &current, draws, next.NextDrawAt); ok {
				t.Fatal("a second missing result advanced by another period")
			}
			if _, ok := nextBettingSchedule(&game, &current, draws, game.NextDrawAt.Add(-time.Nanosecond)); ok {
				t.Fatal("next-period acceptance started before the old draw boundary")
			}
			if _, ok := nextBettingSchedule(&game, &current, draws, game.NextDrawAt); !ok {
				t.Fatal("exact old draw boundary should open the confirmed next window")
			}
		})
	}
}

func TestNextBettingScheduleRequiresFreshUnambiguousEvidence(t *testing.T) {
	tests := []struct {
		name string
		edit func(*lottery.Game, *lottery.Issue, *[]lottery.Draw, *time.Time)
	}{
		{"disabled", func(g *lottery.Game, _ *lottery.Issue, _ *[]lottery.Draw, _ *time.Time) { g.Enabled = false }},
		{"unknown game", func(g *lottery.Game, _ *lottery.Issue, _ *[]lottery.Draw, _ *time.Time) { g.ID = "other-racing" }},
		{"daily reset game", func(g *lottery.Game, _ *lottery.Issue, _ *[]lottery.Draw, _ *time.Time) { g.ID = "sg-fly" }},
		{"calendar game", func(g *lottery.Game, _ *lottery.Issue, _ *[]lottery.Draw, _ *time.Time) { g.ID = "hong-kong-mark-six" }},
		{"platform seed", func(g *lottery.Game, _ *lottery.Issue, _ *[]lottery.Draw, _ *time.Time) { g.SourceKind = "platform" }},
		{"configured seed cadence", func(g *lottery.Game, _ *lottery.Issue, _ *[]lottery.Draw, _ *time.Time) {
			g.TimingSource = "configured"
		}},
		{"source error", func(g *lottery.Game, _ *lottery.Issue, _ *[]lottery.Draw, _ *time.Time) { g.SyncStatus = "error" }},
		{"stale", func(g *lottery.Game, _ *lottery.Issue, _ *[]lottery.Draw, _ *time.Time) { g.SyncStatus = "stale" }},
		{"paused", func(g *lottery.Game, _ *lottery.Issue, _ *[]lottery.Draw, _ *time.Time) { g.SyncStatus = "paused" }},
		{"sync retry with error", func(g *lottery.Game, _ *lottery.Issue, _ *[]lottery.Draw, _ *time.Time) {
			g.SyncStatus, g.LastSyncError = "syncing", "timeout"
		}},
		{"no sync sample", func(g *lottery.Game, _ *lottery.Issue, _ *[]lottery.Draw, _ *time.Time) { g.LastSyncAt = nil }},
		{"old sync sample", func(g *lottery.Game, _ *lottery.Issue, _ *[]lottery.Draw, n *time.Time) {
			at := n.Add(-76 * time.Second)
			g.LastSyncAt = &at
		}},
		{"future sync sample", func(g *lottery.Game, _ *lottery.Issue, _ *[]lottery.Draw, n *time.Time) {
			at := n.Add(time.Hour)
			g.LastSyncAt = &at
		}},
		{"too few intervals", func(_ *lottery.Game, _ *lottery.Issue, d *[]lottery.Draw, _ *time.Time) { *d = (*d)[:3] }},
		{"gap in history", func(_ *lottery.Game, _ *lottery.Issue, d *[]lottery.Draw, _ *time.Time) { (*d)[2].Issue = "34137100" }},
		{"changing cadence", func(_ *lottery.Game, _ *lottery.Issue, d *[]lottery.Draw, _ *time.Time) {
			(*d)[2].DrawAt = (*d)[2].DrawAt.Add(time.Second)
		}},
		{"wrong game history", func(_ *lottery.Game, _ *lottery.Issue, d *[]lottery.Draw, _ *time.Time) { (*d)[0].GameID = "speed-fly" }},
		{"unknown source period", func(_ *lottery.Game, _ *lottery.Issue, d *[]lottery.Draw, _ *time.Time) { (*d)[0].Issue = "SAMPLE-1" }},
		{"date-prefixed period", func(_ *lottery.Game, _ *lottery.Issue, d *[]lottery.Draw, _ *time.Time) {
			(*d)[0].Issue = "20260830001"
		}},
		{"reconciliation error", func(_ *lottery.Game, c *lottery.Issue, _ *[]lottery.Draw, _ *time.Time) {
			c.Status, c.LastError = lottery.IssueStatusError, "对账异常：fixture"
		}},
		{"old draw already published", func(_ *lottery.Game, c *lottery.Issue, _ *[]lottery.Draw, n *time.Time) { c.DrawAt = n }},
		{"boundary mismatch", func(g *lottery.Game, _ *lottery.Issue, _ *[]lottery.Draw, _ *time.Time) {
			g.NextDrawAt = g.NextDrawAt.Add(time.Second)
		}},
		{"issue mismatch", func(g *lottery.Game, _ *lottery.Issue, _ *[]lottery.Draw, _ *time.Time) { g.NextIssue = "34137199" }},
		{"last boundary not proven", func(g *lottery.Game, c *lottery.Issue, _ *[]lottery.Draw, _ *time.Time) {
			g.NextDrawAt = g.NextDrawAt.Add(-time.Second)
			c.ScheduledDrawAt = &g.NextDrawAt
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			game, current, draws, now := nextBettingFixture()
			test.edit(&game, &current, &draws, &now)
			if next, ok := nextBettingSchedule(&game, &current, draws, now); ok {
				t.Fatalf("unverified next period accepted: %+v", next)
			}
		})
	}
}
