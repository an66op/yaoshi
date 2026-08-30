package services

import (
	"backend/data/models/plan"
	"reflect"
	"strings"
	"testing"
	"time"
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
		GameID: "speed-racing", Issue: "34130317", MasterName: "1号专家", Numbers: []int{1, 3, 5, 7, 9},
	})
	if err != nil {
		t.Fatalf("validatePlanInput() error = %v", err)
	}
	if row.Result != plan.ResultPending {
		t.Fatalf("default result = %q, want pending", row.Result)
	}
}

func TestSpeedRacingPlanInputRequiresFiveUniqueNumbersOnly(t *testing.T) {
	valid := PlanRecommendationInput{GameID: "speed-racing", Issue: "confirmed-500", MasterName: "自定义专家", Numbers: []int{1, 3, 5, 7, 10}}
	if row, err := validatePlanInput(valid); err != nil || row.Numbers != "1,3,5,7,10" || row.Size != "" || row.Parity != "" {
		t.Fatalf("five-number input rejected: %#v %v", row, err)
	}
	for _, numbers := range [][]int{{1, 2, 3}, {1, 2, 3, 4}, {1, 2, 3, 4, 5, 6}, {1, 2, 3, 4, 4}, {0, 2, 3, 4, 5}, {1, 2, 3, 4, 11}} {
		input := valid
		input.Numbers = numbers
		if _, err := validatePlanInput(input); err == nil || !strings.Contains(err.Error(), "5个不重复") {
			t.Fatalf("invalid racing numbers accepted: %v error=%v", numbers, err)
		}
	}
	for _, dimension := range []string{"size", "parity"} {
		input := valid
		if dimension == "size" {
			input.Size = "大"
		} else {
			input.Parity = "单"
		}
		if _, err := validatePlanInput(input); err == nil {
			t.Fatalf("racing %s prediction was accepted", dimension)
		}
	}
	other := valid
	other.GameID, other.Numbers, other.Size, other.Parity = "speed-fly", []int{1, 5, 9}, "大", "单"
	if _, err := validatePlanInput(other); err != nil {
		t.Fatalf("other game contract changed: %v", err)
	}
}

func TestLegacyAutomaticPlanPresentationPreservesHistoricalFacts(t *testing.T) {
	created := time.Date(2026, 8, 29, 10, 20, 30, 0, time.UTC)
	for i, oldName := range []string{"青云演示师", "北斗演示师", "锦鲤演示师"} {
		row := plan.Recommendation{ID: uint64(i + 1), WorkspaceID: 42, GameID: "speed-racing", Issue: "confirmed-history-500",
			Source: "demo", MasterName: oldName, MasterTitle: "程序演示 · 号码样例", Note: "程序生成的演示推荐，非真实预测，不保证命中。",
			Numbers: "1,5,9", Size: "大", Parity: "单", Result: "pending", CreatedAt: created, UpdatedAt: created.Add(time.Minute)}
		original := row
		view := planView(row, nil)
		if view.MasterName != planDemoMasters[i].Name || view.MasterTitle != "系统自动推荐" || view.Note != PlanDemoNotice {
			t.Fatalf("legacy presentation was not normalized: %#v", view)
		}
		if view.ID != row.ID || view.WorkspaceID != row.WorkspaceID || view.GameID != row.GameID || view.Issue != row.Issue ||
			!reflect.DeepEqual(view.Numbers, []int{1, 5, 9}) || !view.CreatedAt.Equal(row.CreatedAt) || !view.UpdatedAt.Equal(row.UpdatedAt) || view.Result != row.Result {
			t.Fatalf("historical facts changed: %#v", view)
		}
		if view.Size != "" || view.Parity != "" || view.MasterHitRate != nil || !reflect.DeepEqual(row, original) {
			t.Fatalf("racing dimensions/rate leaked or stored row was changed: %#v", view)
		}
		row.Source, row.GameID = "manual", "speed-fly"
		manualView := planView(row, nil)
		if manualView.MasterName != row.MasterName || manualView.MasterTitle != row.MasterTitle || manualView.Note != row.Note || manualView.Size != row.Size || manualView.Parity != row.Parity {
			t.Fatalf("custom manual content was overwritten: %#v", manualView)
		}
	}
}
