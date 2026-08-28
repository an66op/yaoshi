package services

import (
	"backend/data/models/plan"
	"testing"
)

func TestCanonicalPlanNumbersRejectsInvalidAndDeduplicates(t *testing.T) {
	got, err := canonicalPlanNumbers([]int{1, 5, 1, 9})
	if err != nil {
		t.Fatalf("canonicalPlanNumbers() error = %v", err)
	}
	if got != "1,5,9" {
		t.Fatalf("canonicalPlanNumbers() = %q, want 1,5,9", got)
	}
	for _, values := range [][]int{{}, {-1}, {100}, {1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}} {
		if _, err := canonicalPlanNumbers(values); err == nil {
			t.Fatalf("invalid recommendation numbers were accepted: %v", values)
		}
	}
}

func TestPlanHitRateUsesOnlyPersistedSettledResults(t *testing.T) {
	rows := []plan.Recommendation{
		{GameID: "speed-racing", MasterName: "青云老师", Result: plan.ResultPending},
		{GameID: "speed-racing", MasterName: "青云老师", Result: plan.ResultHit},
		{GameID: "speed-racing", MasterName: "青云老师", Result: plan.ResultMiss},
		{GameID: "canada-28", MasterName: "青云老师", Result: plan.ResultPending},
	}
	rates := planHitRates(rows)
	if rate := rates["speed-racing\x00青云老师"]; rate == nil || *rate != 50 {
		t.Fatalf("settled hit rate = %v, want 50", rate)
	}
	if rate := rates["canada-28\x00青云老师"]; rate != nil {
		t.Fatalf("pending-only recommendation fabricated a hit rate: %v", *rate)
	}
}

func TestValidatePlanInputNeverPromotesPendingToHit(t *testing.T) {
	row, err := validatePlanInput(PlanRecommendationInput{
		GameID: "speed-racing", Issue: "34130317", MasterName: "青云老师", Numbers: []int{1, 5, 9},
	})
	if err != nil {
		t.Fatalf("validatePlanInput() error = %v", err)
	}
	if row.Result != plan.ResultPending {
		t.Fatalf("default result = %q, want pending", row.Result)
	}
}
