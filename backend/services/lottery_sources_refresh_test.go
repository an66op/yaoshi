package services

import (
	"backend/data/models/lottery"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
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

func Test168LiveStageCommitsBeforeHistoryAndPreservesLatestIdentity(t *testing.T) {
	ctx := context.Background()
	var order []string
	latest := SourceSyncResult{GameID: "speed-racing", Status: "ok", Imported: 1, LatestIssue: "34136855"}
	result := sync168LatestThenHistory(ctx,
		func(context.Context) SourceSyncResult {
			order = append(order, "commit-and-publish-latest")
			return latest
		},
		func(context.Context) ([]sourceDraw, error) {
			if !reflect.DeepEqual(order, []string{"commit-and-publish-latest"}) {
				t.Fatalf("history started before the latest state was ready: %v", order)
			}
			order = append(order, "fetch-history")
			return []sourceDraw{{Issue: "34136853"}, {Issue: "34136854"}}, nil
		},
		func(_ context.Context, rows []sourceDraw) (int, error) {
			order = append(order, "backfill-history")
			return len(rows), nil
		})
	if !reflect.DeepEqual(order, []string{"commit-and-publish-latest", "fetch-history", "backfill-history"}) {
		t.Fatalf("wrong stage order: %v", order)
	}
	if result.Status != "ok" || result.LatestIssue != latest.LatestIssue || result.Imported != 3 {
		t.Fatalf("backfill replaced the live result: %+v", result)
	}
}

func Test168LiveFailureDoesNotAttemptHistoryOrImport(t *testing.T) {
	failed := SourceSyncResult{GameID: "speed-racing", Status: "error", Error: "invalid latest result"}
	got := sync168LatestThenHistory(context.Background(), func(context.Context) SourceSyncResult { return failed },
		func(context.Context) ([]sourceDraw, error) {
			t.Fatal("history fetched after live failure")
			return nil, nil
		},
		func(context.Context, []sourceDraw) (int, error) {
			t.Fatal("history imported after live failure")
			return 0, nil
		})
	if got != failed {
		t.Fatalf("live failure was hidden: %+v", got)
	}
}

func Test168HistoryFailureKeepsValidLiveResultAndRetriesAvailableRows(t *testing.T) {
	for _, test := range []struct {
		name      string
		rows      []sourceDraw
		importErr error
		want      int
	}{
		{"complete history outage", nil, nil, 1},
		{"one available day", []sourceDraw{{Issue: "34136854"}}, nil, 2},
		{"history insert failed", []sourceDraw{{Issue: "34136854"}}, errors.New("backfill failed"), 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			result := sync168LatestThenHistory(context.Background(),
				func(context.Context) SourceSyncResult {
					return SourceSyncResult{GameID: "speed-racing", Status: "ok", LatestIssue: "34136855", Imported: 1}
				},
				func(context.Context) ([]sourceDraw, error) { return test.rows, errors.New("history timeout") },
				func(context.Context, []sourceDraw) (int, error) { calls++; return len(test.rows), test.importErr })
			if result.Status != "ok" || result.LatestIssue != "34136855" || result.Imported != test.want {
				t.Fatalf("history failure changed live state: %+v", result)
			}
			if len(test.rows) > 0 && calls != 1 || len(test.rows) == 0 && calls != 0 {
				t.Fatalf("unexpected import calls: %d", calls)
			}
		})
	}
}

func Test168LatestFastPathMakesOnlyOneRequest(t *testing.T) {
	calls := 0
	draws, included, err := fetch168LiveDraws(context.Background(), api168Binding{GameID: "speed-racing", Series: api168PK10, LotCode: "10037"},
		func(_ context.Context, endpoint string, payload *api168Envelope) error {
			calls++
			parsed, err := url.Parse(endpoint)
			if err != nil || parsed.Path != "/pks/getLotteryPksInfo.do" || parsed.Query().Get("lotCode") != "10037" {
				t.Fatalf("fast path unexpectedly requested history: %s", endpoint)
			}
			payload.Result.Data = json.RawMessage(`{"preDrawIssue":34136854,"preDrawTime":"2026-08-30 14:46:48","preDrawCode":"1,2,3,4,5,6,7,8,9,10","drawIssue":34136855,"drawTime":"2026-08-30 14:48:03"}`)
			return nil
		})
	if err != nil || included || calls != 1 || len(draws) != 1 || draws[0].NextIssue != "34136855" {
		t.Fatalf("latest metadata not returned immediately: calls=%d included=%v rows=%+v err=%v", calls, included, draws, err)
	}
}

func Test168MissingUpcomingBoundaryRequiresHistoricalCadence(t *testing.T) {
	for _, validHistory := range []bool{true, false} {
		t.Run(map[bool]string{true: "verified cadence", false: "cannot use arbitrary seed"}[validHistory], func(t *testing.T) {
			calls := 0
			request := func(_ context.Context, endpoint string, payload *api168Envelope) error {
				calls++
				if strings.Contains(endpoint, "getLotteryPksInfo.do") {
					payload.Result.Data = json.RawMessage(`{"preDrawIssue":"104","preDrawTime":"2026-08-30 14:46:48","preDrawCode":"1,2,3,4,5,6,7,8,9,10"}`)
					return nil
				}
				if !validHistory {
					return errors.New("history unavailable")
				}
				payload.Result.Data = json.RawMessage(`[
					{"preDrawIssue":"103","preDrawTime":"2026-08-30 14:45:33","preDrawCode":"1,2,3,4,5,6,7,8,9,10"},
					{"preDrawIssue":"102","preDrawTime":"2026-08-30 14:44:18","preDrawCode":"1,2,3,4,5,6,7,8,9,10"},
					{"preDrawIssue":"101","preDrawTime":"2026-08-30 14:43:03","preDrawCode":"1,2,3,4,5,6,7,8,9,10"}
				]`)
				return nil
			}
			draws, included, err := fetch168LiveDraws(context.Background(), api168Binding{Series: api168PK10, LotCode: "10037"}, request)
			if calls != 3 || !included {
				t.Fatalf("fallback exceeded the existing one-latest/two-history budget: calls=%d included=%v", calls, included)
			}
			if validHistory {
				if err != nil || observedDrawInterval(draws) != 75 {
					t.Fatalf("fallback lost cadence evidence: rows=%+v error=%v", draws, err)
				}
			} else if err == nil || len(draws) != 0 {
				t.Fatalf("missing schedule was accepted: rows=%+v error=%v", draws, err)
			}
		})
	}
}

func Test168HistoryKeepsTwoLocalDaysAndDeduplicatesIssues(t *testing.T) {
	var dates []string
	now := time.Date(2026, 8, 29, 16, 0, 1, 0, time.UTC)
	rows, err := fetch168History(context.Background(), api168PK10, "10037", nil, now,
		func(_ context.Context, endpoint string, payload *api168Envelope) error {
			parsed, err := url.Parse(endpoint)
			if err != nil || parsed.Path != "/pks/getPksHistoryList.do" {
				t.Fatalf("unexpected history endpoint: %s", endpoint)
			}
			dates = append(dates, parsed.Query().Get("date"))
			payload.Result.Data = json.RawMessage(`[{"preDrawIssue":"104","preDrawTime":"2026-08-30 00:00:00","preDrawCode":"1,2,3,4,5,6,7,8,9,10"}]`)
			return nil
		})
	if err != nil || len(rows) != 1 || !reflect.DeepEqual(dates, []string{"2026-08-30", "2026-08-29"}) {
		t.Fatalf("history recovery window changed: dates=%v rows=%+v err=%v", dates, rows, err)
	}
}

func Test168RecentMergePreservesLatestUpcomingBoundary(t *testing.T) {
	latest := sourceDraw{Issue: "104", NextIssue: "105", NextDrawAt: time.Now().UTC()}
	history := []sourceDraw{{Issue: "104"}, {Issue: "103"}}
	got := mergeSourceDraws([]sourceDraw{latest}, history)
	if len(got) != 2 || !reflect.DeepEqual(got[0], latest) || got[1].Issue != "103" {
		t.Fatalf("history erased the live schedule: %+v", got)
	}
}
