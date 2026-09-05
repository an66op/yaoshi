package services

import (
	"reflect"
	"testing"
	"time"

	"backend/data/models/lottery"
)

func TestOfficialGameCatalogChangedOnlyPublishesVisibleChanges(t *testing.T) {
	base := lottery.Game{
		ID: "speed-racing", Enabled: true, SourceKind: "external", SourceName: source163MirrorName, SourceURL: source163MirrorURL, SyncStatus: "ok",
		NextIssue: "34136855", NextDrawAt: time.Date(2026, 8, 30, 6, 48, 3, 0, time.UTC),
		DrawInterval: 75, TimingSource: "upstream",
	}
	for _, test := range []struct {
		name   string
		change func(*lottery.Game)
		want   bool
	}{
		{"identical result", func(*lottery.Game) {}, false},
		{"poll timestamp", func(g *lottery.Game) { now := time.Now(); g.LastSyncAt = &now }, false},
		{"transient syncing", func(g *lottery.Game) { g.SyncStatus = "syncing" }, false},
		{"next issue", func(g *lottery.Game) { g.NextIssue = "34136856" }, true},
		{"boundary", func(g *lottery.Game) { g.NextDrawAt = g.NextDrawAt.Add(75 * time.Second) }, true},
		{"cadence", func(g *lottery.Game) { g.DrawInterval = 300 }, true},
		{"timing origin", func(g *lottery.Game) { g.TimingSource = "observed" }, true},
		{"disabled", func(g *lottery.Game) { g.Enabled = false }, true},
		{"source failed", func(g *lottery.Game) { g.SyncStatus = "error" }, true},
		{"source error text", func(g *lottery.Game) { g.LastSyncError = "source unavailable" }, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := base
			test.change(&changed)
			if got := officialGameCatalogChanged(base, changed); got != test.want {
				t.Fatalf("changed = %v, want %v", got, test.want)
			}
		})
	}
	failed := base
	failed.SyncStatus, failed.LastSyncError = "error", "source unavailable"
	if !officialGameCatalogChanged(failed, base) {
		t.Fatal("source recovery without a new draw must invalidate the catalogue")
	}
	retrying := failed
	retrying.SyncStatus = "syncing"
	if officialGameCatalogChanged(failed, retrying) {
		t.Fatal("retrying the same failed source must not flood catalogue events")
	}
}

func TestOfficialScheduleNeverRewindsVerifiedDifferentIssue(t *testing.T) {
	now := time.Date(2026, 8, 30, 6, 48, 3, 0, time.UTC)
	game := lottery.Game{NextIssue: "105", NextDrawAt: now, TimingSource: "upstream"}
	for _, test := range []struct {
		name, issue string
		at          time.Time
		want        bool
	}{
		{"cached earlier period", "103", now.Add(-150 * time.Second), true},
		{"valid next period", "106", now.Add(75 * time.Second), false},
		{"same issue earlier cutoff", "105", now.Add(-5 * time.Second), false},
		{"same issue unchanged", "105", now, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := officialScheduleRegresses(game, sourceSchedule{Issue: test.issue, DrawAt: test.at}); got != test.want {
				t.Fatalf("regresses = %v, want %v", got, test.want)
			}
		})
	}
	game.TimingSource = "configured"
	if officialScheduleRegresses(game, sourceSchedule{Issue: "103", DrawAt: now.Add(-150 * time.Second)}) {
		t.Fatal("first verified schedule must be able to correct an arbitrary configured seed")
	}
}

func TestSourceDrawMergePreservesLatestUpcomingBoundary(t *testing.T) {
	latest := sourceDraw{Issue: "104", NextIssue: "105", NextDrawAt: time.Now().UTC()}
	history := []sourceDraw{{Issue: "104"}, {Issue: "103"}}
	got := mergeSourceDraws([]sourceDraw{latest}, history)
	if len(got) != 2 || !reflect.DeepEqual(got[0], latest) || got[1].Issue != "103" {
		t.Fatalf("history erased the live schedule: %+v", got)
	}
}
