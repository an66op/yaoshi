package services

import (
	"backend/data/models/lottery"
	"backend/data/models/plan"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestPlanStreamMatrixHasSeventeenOperationalOptions(t *testing.T) {
	if len(racingPlanOptions) != 17 || len(planPositions()) != 10 {
		t.Fatal("incomplete racing matrix")
	}
	keys := map[string]bool{}
	for _, option := range racingPlanOptions {
		if keys[option.Key] || option.Periods < 1 || option.Periods > 5 {
			t.Fatalf("invalid option %#v", option)
		}
		keys[option.Key] = true
		for position := 1; position <= 10; position++ {
			picks := planCyclePicks(7, position, option, "actual-100")
			if len(picks) != 3 {
				t.Fatal("expected three experts")
			}
			if !reflect.DeepEqual(picks, planCyclePicks(7, position, option, "actual-100")) {
				t.Fatal("picks are not deterministic")
			}
			for _, pick := range picks {
				if pick.Result != "pending" || pick.MasterHitRate != nil || pick.Source != "demo" {
					t.Fatal("fabricated result")
				}
				if len(pick.Numbers) != option.NumberCount {
					t.Fatalf("wrong count: %#v", pick)
				}
				seen := map[int]bool{}
				for _, n := range pick.Numbers {
					if n < 1 || n > 10 || seen[n] {
						t.Fatal("invalid unique racing pick")
					}
					seen[n] = true
				}
				switch option.Kind {
				case "numbers":
					if pick.Size != "" || pick.Parity != "" || pick.DragonTiger != "" {
						t.Fatal("number plan has directions")
					}
				case "size":
					if pick.Size != "大" && pick.Size != "小" {
						t.Fatal("empty size button")
					}
				case "parity":
					if pick.Parity != "单" && pick.Parity != "双" {
						t.Fatal("empty parity button")
					}
				case "dragon_tiger":
					if pick.DragonTiger != "龙" && pick.DragonTiger != "虎" {
						t.Fatal("empty dragon/tiger button")
					}
				default:
					t.Fatal("unsupported option")
				}
			}
		}
	}
	for _, p := range planPositions() {
		if p.OpponentPosition != 11-p.Position {
			t.Fatalf("wrong dragon/tiger pairing: %#v", p)
		}
	}
	option, _ := planOption(DefaultPlanKey)
	if reflect.DeepEqual(planCyclePicks(7, 1, option, "actual-100"), planCyclePicks(7, 2, option, "actual-100")) {
		t.Fatal("positions reused one payload")
	}
}

func TestPlanStreamRacingProductsExactlyMatchVerifiedTenPositionRules(t *testing.T) {
	want := []string{"speed-racing", "speed-fly", "sg-fly", "fly-racing", "au-lucky-10", "bingo-racing-a", "bingo-racing-b"}
	if !reflect.DeepEqual(racingPlanGameIDs, want) {
		t.Fatalf("rich plan products = %v, want %v", racingPlanGameIDs, want)
	}
	config := PlanAutomationView{Enabled: true, GameIDs: append([]string{}, want...), Positions: defaultPlanPositions(), PlanKeys: defaultPlanKeys()}
	option, _ := planOption(DefaultPlanKey)
	for _, gameID := range want {
		profile, ready := rulesForGame(&lottery.Game{ID: gameID})
		if !ready || profile.Version != "racing-v2" || !profile.Racing || !profile.Unique || profile.BallCount != 10 || profile.MinNumber != 1 || profile.MaxNumber != 10 {
			t.Fatalf("%s is not a verified ten-position racing-v2 product: %#v ready=%v", gameID, profile, ready)
		}
		if !IsRacingPlanGame(gameID) || !planRequestedStreamAllowed(config, gameID, 10, DefaultPlanKey) {
			t.Fatalf("%s did not route through the full position/type plan matrix", gameID)
		}
		picks := planCyclePicksForGame(7, gameID, 10, option, "actual-100")
		if len(picks) != len(planDemoMasters) || len(picks[0].Numbers) != option.NumberCount {
			t.Fatalf("%s did not create a complete rich plan payload: %#v", gameID, picks)
		}
	}
	for _, gameID := range []string{"speed-ssc", "canada-28", "bingo-mark-six", "official-tw-bingo", "unknown-racing"} {
		if IsRacingPlanGame(gameID) {
			t.Fatalf("non-racing-v2 product %s entered the rich racing plan contract", gameID)
		}
	}
}

func TestPlanStreamTTLExpiresWithoutDefaultOrCycleKeepalive(t *testing.T) {
	now := time.Now().UTC()
	past, future := now.Add(-time.Minute), now.Add(time.Minute)
	stream := plan.Stream{Position: 2, PlanKey: DefaultPlanKey, ActiveUntil: &past}
	if planStreamActive(stream, plan.StreamCycle{}, now) {
		t.Fatal("expired unused stream remained active")
	}
	if planStreamActive(stream, plan.StreamCycle{ID: 1, Status: "active", PublishedPeriods: 2, Periods: 4}, now) {
		t.Fatal("unfinished cycle kept an unvisited stream active")
	}
	if planStreamActive(stream, plan.StreamCycle{ID: 1, Status: "completed", LastScheduledAt: past}, now) {
		t.Fatal("finished cycle never expired")
	}
	if planStreamActive(stream, plan.StreamCycle{ID: 1, Status: "completed", LastScheduledAt: future}, now) {
		t.Fatal("future draw time kept an unvisited stream active")
	}
	stream.Position = 1
	if planStreamActive(stream, plan.StreamCycle{}, now) {
		t.Fatal("default stream remained permanent")
	}
	stream.ActiveUntil, stream.UpdatedAt = &future, now
	if !planStreamActive(stream, plan.StreamCycle{}, now) {
		t.Fatal("fresh visit has no lease")
	}
	stream.UpdatedAt = now.Add(-61 * time.Second)
	if planStreamActive(stream, plan.StreamCycle{}, now) {
		t.Fatal("legacy 30-minute lease escaped 60s cap")
	}
}

func TestPlanStreamConfigurationRevocationIsAuthoritative(t *testing.T) {
	view, err := planAutomationView(plan.Automation{Enabled: true, GameIDsJSON: `["speed-racing"]`})
	if err != nil || !planStreamAllowed(view, 1, DefaultPlanKey) {
		t.Fatal("default matrix missing", err)
	}
	for _, change := range []func(*PlanAutomationView){
		func(v *PlanAutomationView) { v.Enabled = false }, func(v *PlanAutomationView) { v.GameIDs = []string{} },
		func(v *PlanAutomationView) { v.Positions = []int{2} }, func(v *PlanAutomationView) { v.PlanKeys = []string{"size-five-periods"} },
	} {
		copy := view
		change(&copy)
		if planStreamAllowed(copy, 1, DefaultPlanKey) {
			t.Fatal("revoked selection remained allowed")
		}
	}
	disabled := view
	disabled.Enabled = false
	if planStreamAllowed(disabled, 1, DefaultPlanKey) || !planStreamConfiguredForGame(disabled, "speed-racing", 1, DefaultPlanKey) {
		t.Fatal("generation switch did not preserve the browseable configured matrix")
	}
	for _, test := range []struct {
		p []int
		k []string
	}{{[]int{0}, []string{DefaultPlanKey}}, {[]int{11}, []string{DefaultPlanKey}}, {[]int{1}, []string{"fake-plan"}}} {
		if _, _, err := normalizePlanMatrix(test.p, test.k); err == nil {
			t.Fatal("invalid matrix accepted")
		}
	}
	raw, _ := json.Marshal(view)
	if len(raw) == 0 || view.Options == nil || view.AvailablePositions == nil || view.Positions == nil || view.PlanKeys == nil {
		t.Fatal("configuration arrays must be present")
	}
}
