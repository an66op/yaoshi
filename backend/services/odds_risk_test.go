package services

import (
	"math"
	"testing"
)

func TestFrontThreeRiskOnlyUsesProvenVersionContracts(t *testing.T) {
	for _, version := range []string{"digits3-v2", "digits5-v3"} {
		profile, ok := rulesForVersion(version)
		if !ok || !hasExclusiveFrontThreeShapes(profile) {
			t.Fatalf("missing proven contract %s", version)
		}
	}
	for _, profile := range []gameRuleProfile{{}, {Version: "digits5-v2", Patterns: true}, {Version: "digits5-v99", Patterns: true}, {Version: "racing-v2"}, {Version: "digits3-v2"}} {
		if hasExclusiveFrontThreeShapes(profile) {
			t.Fatalf("unproven shape partition inherited coverage guard: %+v", profile)
		}
	}
}

func TestFrontThreeOddsCoverageRisk(t *testing.T) {
	for _, test := range []struct {
		name, gameID string
		values       []float64
		want         string
	}{
		{"current base portfolio", "speed-ssc", []float64{50, 15, 8, 6, 4}, "SHAPE_COVERAGE_RISK"},
		{"three digit contract", "official-fc3d", []float64{50, 15, 8, 6, 4}, "SHAPE_COVERAGE_RISK"},
		{"balanced test prices", "speed-ssc", []float64{50, 15, 2.8, 2.4, 3}, ""},
		{"fair exact decimal boundary", "speed-ssc", []float64{5, 5, 5, 5, 5}, ""},
		{"one precision step beyond fair", "speed-ssc", []float64{5, 5, 5, 5, 5.0001}, "SHAPE_COVERAGE_RISK"},
		{"rounded fair boundary", "speed-ssc", []float64{5, 5, 5, 5, 5.00001}, ""},
		{"missing last outcome", "speed-ssc", []float64{50, 15, 8, 6}, ""},
		{"nonfinite odds", "speed-ssc", []float64{50, 15, math.Inf(1), 6, 4}, "INVALID_SHAPE_ODDS"},
		{"not a number", "speed-ssc", []float64{50, 15, math.NaN(), 6, 4}, "INVALID_SHAPE_ODDS"},
		{"invalid multiplier result", "speed-ssc", []float64{1, 15, 8, 6, 4}, "INVALID_SHAPE_ODDS"},
		{"racing does not offer shapes", "speed-racing", []float64{50, 15, 8, 6, 4}, ""},
		{"PC28 does not offer front-three digit shapes", "pc-canada", []float64{50, 15, 8, 6, 4}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			offers := map[string]float64{}
			for index, value := range test.values {
				offers[frontThreeShapeCodes[index]] = value
			}
			warnings := frontThreeOddsRisks(test.gameID, offers)
			if test.want == "" {
				if len(warnings) != 0 {
					t.Fatalf("unexpected risk: %+v", warnings)
				}
				return
			}
			if len(warnings) != 1 || warnings[0].Code != test.want || len(warnings[0].PlayCodes) != 5 || warnings[0].Message == "" {
				t.Fatalf("risk=%+v, want %s", warnings, test.want)
			}
		})
	}
}

func TestFrontThreeRiskIsConsistentWithActualOutcomePartition(t *testing.T) {
	stakes := map[string]int{"leopard": 12, "straight": 40, "pair": 75, "half_straight": 100, "mixed": 150}
	prices := map[string]float64{"leopard": 50, "straight": 15, "pair": 8, "half_straight": 6, "mixed": 4}
	for first := 0; first <= 9; first++ {
		for second := 0; second <= 9; second++ {
			for third := 0; third <= 9; third++ {
				outcome := frontPattern([]int{first, second, third})
				if !isFrontThreeShape(outcome) || float64(stakes[outcome])*prices[outcome] != 600 {
					t.Fatalf("uncovered outcome %d%d%d -> %s", first, second, third, outcome)
				}
			}
		}
	}
	if warnings := frontThreeOddsRisks("speed-ssc", prices); len(warnings) != 1 {
		t.Fatalf("mathematically unsafe coverage was not detected: %+v", warnings)
	}
}
