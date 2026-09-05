package services

import (
	"backend/data/models/lottery"
	"backend/data/models/plan"
	"strings"
	"time"
)

// Publication completion is not a win/loss. Only a matching, already-published
// complete draw may grade a recommendation saved before its draw boundary.
// Returning pending for missing/invalid evidence avoids inventing a result.
func racingPlanDrawResult(pick PlanRecommendationView, period plan.StreamPeriod, draw lottery.Draw, now time.Time) (string, []int, *time.Time) {
	if !racingPlanGameID(pick.GameID) || draw.GameID != pick.GameID || draw.Issue != pick.Issue || period.Issue != pick.Issue ||
		pick.Position < 1 || pick.Position > 10 || period.CreatedAt.IsZero() || period.ScheduledDrawAt.IsZero() ||
		draw.DrawAt.IsZero() || draw.DrawAt.After(now) || !period.CreatedAt.Before(period.ScheduledDrawAt) || !period.CreatedAt.Before(draw.DrawAt) {
		return "pending", nil, nil
	}
	numbers := parseNumbers(draw.Numbers)
	if len(numbers) != 10 || len(strings.Split(draw.Numbers, ",")) != 10 {
		return "pending", nil, nil
	}
	seen := map[int]bool{}
	for _, number := range numbers {
		if number < 1 || number > 10 || seen[number] {
			return "pending", nil, nil
		}
		seen[number] = true
	}
	value := numbers[pick.Position-1]
	hit := false
	switch pick.Kind {
	case "numbers":
		if len(pick.Numbers) == 0 || len(pick.Numbers) > 10 {
			return "pending", nil, nil
		}
		selected := map[int]bool{}
		for _, number := range pick.Numbers {
			if number < 1 || number > 10 || selected[number] {
				return "pending", nil, nil
			}
			selected[number] = true
			hit = hit || number == value
		}
	case "size":
		if pick.Size != "大" && pick.Size != "小" {
			return "pending", nil, nil
		}
		hit = (pick.Size == "大") == (value >= 6)
	case "parity":
		if pick.Parity != "单" && pick.Parity != "双" {
			return "pending", nil, nil
		}
		hit = (pick.Parity == "单") == (value%2 == 1)
	case "dragon_tiger":
		if pick.DragonTiger != "龙" && pick.DragonTiger != "虎" {
			return "pending", nil, nil
		}
		hit = (pick.DragonTiger == "龙") == (value > numbers[10-pick.Position])
	default:
		return "pending", nil, nil
	}
	result := "miss"
	if hit {
		result = "hit"
	}
	drawAt := draw.DrawAt
	return result, numbers, &drawAt
}
