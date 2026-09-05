package services

import (
	"backend/data/models/lottery"
	"backend/data/models/plan"
	"reflect"
	"testing"
	"time"
)

func TestRacingPlanDrawResultUsesActualPositionAndDirection(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	period := plan.StreamPeriod{Issue: "34137173", CreatedAt: now.Add(-2 * time.Minute), ScheduledDrawAt: now.Add(-time.Minute)}
	draw := lottery.Draw{GameID: "speed-racing", Issue: period.Issue, Numbers: "6,8,10,1,2,4,9,7,3,5", DrawAt: now.Add(-time.Minute)}
	for _, test := range []struct {
		kind            string
		position        int
		numbers         []int
		direction, want string
	}{
		{"numbers", 1, []int{1, 2, 3, 4, 6}, "", "hit"},
		{"numbers", 1, []int{1, 2, 3, 4, 5}, "", "miss"},
		{"numbers", 10, []int{5}, "", "hit"},
		{"size", 1, nil, "大", "hit"}, {"size", 10, nil, "大", "miss"},
		{"parity", 1, nil, "双", "hit"}, {"parity", 1, nil, "单", "miss"},
		{"dragon_tiger", 1, nil, "龙", "hit"}, {"dragon_tiger", 10, nil, "虎", "hit"},
		{"dragon_tiger", 6, nil, "龙", "hit"}, {"dragon_tiger", 6, nil, "虎", "miss"},
	} {
		pick := PlanRecommendationView{GameID: draw.GameID, Issue: period.Issue, Kind: test.kind, Position: test.position, Numbers: test.numbers,
			Size: test.direction, Parity: test.direction, DragonTiger: test.direction, CycleStatus: "completed", Result: "pending"}
		result, numbers, at := racingPlanDrawResult(pick, period, draw, now)
		if result != test.want || !reflect.DeepEqual(numbers, []int{6, 8, 10, 1, 2, 4, 9, 7, 3, 5}) || at == nil || !at.Equal(draw.DrawAt) {
			t.Fatalf("%+v: %s %v %v", test, result, numbers, at)
		}
	}
}

func TestRacingPlanDrawResultSupportsEveryVerifiedRacingProduct(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	for _, gameID := range racingPlanGameIDs {
		period := plan.StreamPeriod{Issue: "verified-100", CreatedAt: now.Add(-2 * time.Minute), ScheduledDrawAt: now.Add(-time.Minute)}
		draw := lottery.Draw{GameID: gameID, Issue: period.Issue, Numbers: "1,2,3,4,5,6,7,8,9,10", DrawAt: now.Add(-time.Minute)}
		pick := PlanRecommendationView{GameID: gameID, Issue: period.Issue, Kind: "numbers", Position: 10, Numbers: []int{10}, Result: "pending"}
		if result, numbers, at := racingPlanDrawResult(pick, period, draw, now); result != "hit" || len(numbers) != 10 || at == nil {
			t.Fatalf("%s did not settle from its own verified ten-ball draw: result=%s numbers=%v at=%v", gameID, result, numbers, at)
		}
	}
}

func TestRacingPlanDrawResultFailsClosedWithoutTimelyValidEvidence(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name   string
		change func(*PlanRecommendationView, *plan.StreamPeriod, *lottery.Draw)
	}{
		{"missing draw", func(_ *PlanRecommendationView, _ *plan.StreamPeriod, d *lottery.Draw) { *d = lottery.Draw{} }},
		{"wrong game", func(_ *PlanRecommendationView, _ *plan.StreamPeriod, d *lottery.Draw) { d.GameID = "speed-fly" }},
		{"wrong issue", func(_ *PlanRecommendationView, _ *plan.StreamPeriod, d *lottery.Draw) { d.Issue = "older" }},
		{"future draw", func(_ *PlanRecommendationView, _ *plan.StreamPeriod, d *lottery.Draw) {
			d.DrawAt = now.Add(time.Minute)
		}},
		{"late publication", func(_ *PlanRecommendationView, p *plan.StreamPeriod, _ *lottery.Draw) {
			p.CreatedAt = p.ScheduledDrawAt
		}},
		{"after early actual draw", func(_ *PlanRecommendationView, p *plan.StreamPeriod, d *lottery.Draw) { d.DrawAt = p.CreatedAt }},
		{"incomplete draw", func(_ *PlanRecommendationView, _ *plan.StreamPeriod, d *lottery.Draw) { d.Numbers = "1,2,3" }},
		{"duplicate draw", func(_ *PlanRecommendationView, _ *plan.StreamPeriod, d *lottery.Draw) {
			d.Numbers = "1,2,3,4,5,6,7,8,9,9"
		}},
		{"malformed draw", func(_ *PlanRecommendationView, _ *plan.StreamPeriod, d *lottery.Draw) {
			d.Numbers = "1,2,3,x,4,5,6,7,8,9,10"
		}},
		{"invalid position", func(p *PlanRecommendationView, _ *plan.StreamPeriod, _ *lottery.Draw) { p.Position = 11 }},
		{"empty picks", func(p *PlanRecommendationView, _ *plan.StreamPeriod, _ *lottery.Draw) { p.Numbers = nil }},
		{"invalid picks", func(p *PlanRecommendationView, _ *plan.StreamPeriod, _ *lottery.Draw) { p.Numbers = []int{11} }},
		{"duplicate picks", func(p *PlanRecommendationView, _ *plan.StreamPeriod, _ *lottery.Draw) { p.Numbers = []int{1, 1} }},
		{"missing direction", func(p *PlanRecommendationView, _ *plan.StreamPeriod, _ *lottery.Draw) { p.Kind = "size" }},
		{"unknown kind", func(p *PlanRecommendationView, _ *plan.StreamPeriod, _ *lottery.Draw) { p.Kind = "unknown" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			period := plan.StreamPeriod{Issue: "34137173", CreatedAt: now.Add(-2 * time.Minute), ScheduledDrawAt: now.Add(-time.Minute)}
			draw := lottery.Draw{GameID: "speed-racing", Issue: period.Issue, Numbers: "1,2,3,4,5,6,7,8,9,10", DrawAt: now.Add(-time.Minute)}
			pick := PlanRecommendationView{GameID: draw.GameID, Issue: period.Issue, Kind: "numbers", Position: 1, Numbers: []int{1}, Result: "hit", CycleStatus: "completed"}
			test.change(&pick, &period, &draw)
			if result, numbers, at := racingPlanDrawResult(pick, period, draw, now); result != "pending" || numbers != nil || at != nil {
				t.Fatalf("unproven result: %s %v %v", result, numbers, at)
			}
		})
	}
}
