package services

import (
	"backend/data/models/lottery"
	"backend/data/models/plan"
	apperrors "backend/errors"
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

func TestAutomaticPlanPublicationCannotBeEditedAsManualContent(t *testing.T) {
	row := plan.Recommendation{Source: "demo"}
	if err := ensurePlanRecommendationEditable(row); err == nil || apperrors.GetErrorCode(err) != "PLAN_PUBLICATION_IMMUTABLE" {
		t.Fatalf("automatic publication edit error = %v", err)
	}
	if err := ensurePlanRecommendationEditable(plan.Recommendation{Source: "manual"}); err != nil {
		t.Fatalf("manual publication unexpectedly immutable: %v", err)
	}
}

func TestPlanNumberContractUsesEachGamesVerifiedTargetRange(t *testing.T) {
	for _, test := range []struct {
		gameID, numbers string
		valid           bool
	}{
		{gameID: "speed-ssc", numbers: "0,5,9", valid: true},
		{gameID: "speed-ssc", numbers: "0,10", valid: false},
		{gameID: "canada-28", numbers: "0,14,27", valid: true},
		{gameID: "canada-28", numbers: "28", valid: false},
		{gameID: "hong-kong-mark-six", numbers: "1,25,49", valid: true},
		{gameID: "hong-kong-mark-six", numbers: "0,50", valid: false},
		{gameID: "unknown", numbers: "1,2,3", valid: false},
	} {
		err := validatePlanNumberContract(lottery.Game{ID: test.gameID}, test.numbers)
		if (err == nil) != test.valid {
			t.Fatalf("%s numbers %q valid=%v, error=%v", test.gameID, test.numbers, test.valid, err)
		}
	}
	if err := validatePlanNumberContract(lottery.Game{ID: "unknown"}, "1,2,3"); apperrors.GetErrorCode(err) != "RULES_NOT_READY" {
		t.Fatalf("unsupported game error = %v", err)
	}
}

func TestPlanHitStatisticsUseOnlyDerivedSettledResultsAndExposeSample(t *testing.T) {
	rows := []plan.Recommendation{
		{GameID: "speed-racing", MasterName: "青云老师", Result: plan.ResultPending},
		{GameID: "speed-racing", MasterName: "青云老师", Result: plan.ResultHit},
		{GameID: "speed-racing", MasterName: "青云老师", Result: plan.ResultMiss},
		{GameID: "canada-28", MasterName: "青云老师", Result: plan.ResultPending},
	}
	statistics := planHitStatistics(rows)
	if statistic := statistics["speed-racing\x00manual\x00青云老师"]; statistic.Rate == nil || *statistic.Rate != 50 || statistic.SampleCount != 2 {
		t.Fatalf("settled hit statistics = %+v, want 50%% over 2 samples", statistic)
	}
	if statistic := statistics["canada-28\x00manual\x00青云老师"]; statistic.Rate != nil || statistic.SampleCount != 0 {
		t.Fatalf("pending-only recommendation fabricated statistics: %+v", statistic)
	}
}

func TestValidatePlanInputNeverPromotesPendingToHit(t *testing.T) {
	row, err := validatePlanInput(PlanRecommendationInput{
		GameID: "canada-28", Issue: "34130317", MasterName: "1号专家", Numbers: []int{1, 3, 5, 7, 9},
	})
	if err != nil {
		t.Fatalf("validatePlanInput() error = %v", err)
	}
	if row.Result != plan.ResultPending {
		t.Fatalf("default result = %q, want pending", row.Result)
	}
	for _, result := range []string{plan.ResultHit, plan.ResultMiss, "80%"} {
		input := PlanRecommendationInput{GameID: "canada-28", Issue: "real-open-1", MasterName: "人工专家", Numbers: []int{1, 3, 5}, Result: result}
		if _, err := validatePlanInput(input); err == nil {
			t.Fatalf("administrator could manually claim result %q", result)
		}
	}
}

func TestRacingPlanInputRequiresRichAutomaticMatrix(t *testing.T) {
	for _, gameID := range racingPlanGameIDs {
		input := PlanRecommendationInput{GameID: gameID, Issue: "confirmed-500", MasterName: "自定义专家", Numbers: []int{1, 3, 5, 7, 10}}
		if _, err := validatePlanInput(input); err == nil || !strings.Contains(err.Error(), "名次和方案矩阵") {
			t.Fatalf("%s accepted a hidden generic manual recommendation: %v", gameID, err)
		}
	}
	other := PlanRecommendationInput{GameID: "canada-28", Issue: "confirmed-500", MasterName: "自定义专家", Numbers: []int{1, 5, 9}, Size: "大", Parity: "单"}
	if row, err := validatePlanInput(other); err != nil || row.Numbers != "1,5,9" || row.Size != "大" || row.Parity != "单" {
		t.Fatalf("non-racing manual contract changed: %#v %v", row, err)
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
		if view.Size != "" || view.Parity != "" || view.MasterHitRate != nil || view.MasterSampleCount != 0 || !reflect.DeepEqual(row, original) {
			t.Fatalf("racing dimensions/rate leaked or stored row was changed: %#v", view)
		}
		row.Source, row.GameID = "manual", "speed-fly"
		manualView := planView(row, nil)
		if manualView.MasterName != row.MasterName || manualView.MasterTitle != row.MasterTitle || manualView.Note != row.Note || manualView.Size != row.Size || manualView.Parity != row.Parity {
			t.Fatalf("custom manual content was overwritten: %#v", manualView)
		}
	}
}
