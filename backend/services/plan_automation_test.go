package services

import (
	"backend/data/models/lottery"
	"backend/data/models/plan"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestPlanAutomationDefaultsRemainDisabledAndClearlyAutomatic(t *testing.T) {
	view, err := planAutomationView(plan.Automation{WorkspaceID: 42})
	if err != nil || view.Enabled || view.Mode != "demo" || len(view.GameIDs) != 0 || len(view.Masters) != 3 || view.UpdatedAt != nil {
		t.Fatalf("unexpected default: %#v error=%v", view, err)
	}
	if view.Notice != "系统自动生成，仅供娱乐参考，不保证命中。" {
		t.Fatalf("automatic recommendation disclaimer omitted: %q", view.Notice)
	}
	wantNames := []string{"1号专家", "2号专家", "3号专家"}
	wantKeys := []string{"demo-qingyun", "demo-beidou", "demo-jinli"}
	for i, master := range view.Masters {
		if master.Name != wantNames[i] || master.Key != wantKeys[i] || master.Title != "系统自动推荐" {
			t.Fatalf("template changed its public label or idempotency key: %#v", master)
		}
	}
	view.Masters[0].Name = "changed"
	if planDemoMasters[0].Name == "changed" {
		t.Fatal("view mutated the shared template")
	}
	for _, raw := range []string{"null", "{}", "broken"} {
		if _, err := planAutomationView(plan.Automation{GameIDsJSON: raw}); err == nil {
			t.Fatalf("accepted damaged configuration %q", raw)
		}
	}
	row := plan.Automation{WorkspaceID: 42}
	statement := robotDryRunDB(t).Session(&gorm.Session{SkipDefaultTransaction: true}).Create(&row).Statement
	if statement.Error != nil || row.Enabled || statement.Schema.LookUpField("Enabled").DefaultValue != "false" {
		t.Fatalf("GORM enabled a default configuration: %#v err=%v", row, statement.Error)
	}
}

func TestPlanAutomationNumbersAreDeterministicUniqueAndGameBounded(t *testing.T) {
	for _, test := range []struct {
		id, category            string
		minimum, maximum, count int
	}{
		{"speed-racing", "赛车", 1, 10, 5}, {"speed-fly", "飞艇", 1, 10, 3},
		{"speed-ssc", "时时彩", 0, 9, 3}, {"canada-28", "PC", 0, 27, 3},
		{"hong-kong-mark-six", "六合彩", 1, 49, 3},
	} {
		t.Run(test.id, func(t *testing.T) {
			game := lottery.Game{ID: test.id, Category: test.category}
			for _, master := range planDemoMasters {
				first, err := planDemoNumbers(5, game, "confirmed-500", master.Key)
				second, secondErr := planDemoNumbers(5, game, "confirmed-500", master.Key)
				if err != nil || secondErr != nil || len(first) != test.count || !reflect.DeepEqual(first, second) {
					t.Fatalf("non-deterministic fixture: %v %v %v", first, second, err)
				}
				seen := map[int]bool{}
				for _, n := range first {
					if n < test.minimum || n > test.maximum || seen[n] {
						t.Fatalf("invalid %s fixture: %v", test.id, first)
					}
					seen[n] = true
				}
			}
		})
	}
	if _, err := planDemoNumbers(5, lottery.Game{ID: "unknown", Category: "六合彩"}, "500", "demo"); err == nil {
		t.Fatal("unsupported game silently received a number range")
	}
	first, _ := planDemoNumbers(5, lottery.Game{ID: "speed-racing", Category: "赛车"}, "500", "demo")
	different := false
	for room := uint64(6); room < 20; room++ {
		numbers, _ := planDemoNumbers(room, lottery.Game{ID: "speed-racing", Category: "赛车"}, "500", "demo")
		different = different || !reflect.DeepEqual(first, numbers)
	}
	if !different {
		t.Fatal("room identity does not affect generated fixtures")
	}
}

func TestPlanAutomationCompatibilityKeysAndHashSeedStayStable(t *testing.T) {
	for _, test := range []struct {
		id, category string
		want         []int
	}{
		{"speed-fly", "飞艇", []int{3, 5, 7}},
		{"speed-ssc", "时时彩", []int{3, 7, 8}},
		{"canada-28", "PC", []int{2, 9, 21}},
		{"speed-racing", "赛车", []int{1, 2, 5, 7, 8}},
	} {
		got, err := planDemoNumbers(5, lottery.Game{ID: test.id, Category: test.category}, "confirmed-500", "demo-qingyun")
		if err != nil || !reflect.DeepEqual(got, test.want) {
			t.Fatalf("%s changed its compatibility seed: got %v want %v error=%v", test.id, got, test.want, err)
		}
	}
}

func TestPlanAutomationNeverUsesGuessedClosedOrDrawnIssues(t *testing.T) {
	now := time.Now().UTC()
	drawAt := now.Add(time.Minute)
	game := lottery.Game{ID: "speed-racing", SourceKind: "external", TimingSource: "upstream", NextIssue: "500", NextDrawAt: drawAt, SyncStatus: "ok"}
	issue := lottery.Issue{GameID: game.ID, Issue: "500", SourceMode: "external", Status: lottery.IssueStatusAccepting, AcceptAt: now.Add(-time.Minute), SealAt: now.Add(30 * time.Second), ScheduledDrawAt: &drawAt}
	if !planAutomationIssueEligible(game, issue, now) {
		t.Fatal("confirmed open upstream issue rejected")
	}
	observed := game
	observed.TimingSource = "observed"
	if !planAutomationIssueEligible(observed, issue, now) {
		t.Fatal("open external issue with observed feed timing rejected")
	}
	for name, change := range map[string]func(*lottery.Game, *lottery.Issue){
		"configured schedule":   func(g *lottery.Game, _ *lottery.Issue) { g.TimingSource = "configured" },
		"simulated source":      func(g *lottery.Game, _ *lottery.Issue) { g.SourceKind = "platform" },
		"missing issue":         func(g *lottery.Game, _ *lottery.Issue) { g.NextIssue = "" },
		"old issue":             func(g *lottery.Game, _ *lottery.Issue) { g.NextIssue = "501" },
		"wrong game":            func(_ *lottery.Game, i *lottery.Issue) { i.GameID = "canada-28" },
		"simulated issue":       func(_ *lottery.Game, i *lottery.Issue) { i.SourceMode = "platform" },
		"expired status flag":   func(_ *lottery.Game, i *lottery.Issue) { i.SealAt = now },
		"not accepting yet":     func(_ *lottery.Game, i *lottery.Issue) { i.AcceptAt = now.Add(time.Second) },
		"sealed":                func(_ *lottery.Game, i *lottery.Issue) { i.Status = lottery.IssueStatusSealed },
		"already drawn":         func(_ *lottery.Game, i *lottery.Issue) { i.DrawAt = &now },
		"missing draw boundary": func(_ *lottery.Game, i *lottery.Issue) { i.ScheduledDrawAt = nil },
		"draw boundary elapsed": func(_ *lottery.Game, i *lottery.Issue) { i.ScheduledDrawAt = &now },
		"feed failed":           func(g *lottery.Game, _ *lottery.Issue) { g.SyncStatus = "error" },
	} {
		t.Run(name, func(t *testing.T) {
			g, i := game, issue
			change(&g, &i)
			if planAutomationIssueEligible(g, i, now) {
				t.Fatal("unsafe issue accepted for automatic publication")
			}
		})
	}
}

func TestPlanAutomationConfigInputRequiresExplicitOptIn(t *testing.T) {
	for _, input := range []PlanAutomationInput{{}, {Mode: "prediction"}} {
		if _, err := NewPlanAutomationService(nil).Save(1, input); err == nil {
			t.Fatal("missing opt-in or unsupported mode accepted")
		}
	}
	got, err := normalizePlanAutomationGames([]string{" speed-fly ", "speed-racing", "speed-fly"})
	if err != nil || !reflect.DeepEqual(got, []string{"speed-fly", "speed-racing"}) {
		t.Fatalf("normalize games = %v, %v", got, err)
	}
	for _, values := range [][]string{{""}, {strings.Repeat("x", 41)}, make([]string, 61)} {
		if _, err := normalizePlanAutomationGames(values); err == nil {
			t.Fatalf("accepted invalid games: %v", values)
		}
	}
}

func TestPlanDemoRowsNeverContributeToHitRate(t *testing.T) {
	manual := plan.Recommendation{GameID: "speed-racing", MasterName: "same name", Source: "manual", Result: plan.ResultMiss}
	demo := manual
	demo.Source, demo.Result = "demo", plan.ResultHit
	rates := planHitRates([]plan.Recommendation{manual, demo})
	if rate := rates["speed-racing\x00same name"]; rate == nil || *rate != 0 {
		t.Fatalf("demo result entered manual hit rate: %v", rate)
	}
	if view := planView(demo, rates); view.Source != "demo" || view.MasterHitRate != nil {
		t.Fatalf("demo row displayed a hit rate: %#v", view)
	}
}
