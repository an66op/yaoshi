package services

import (
	"testing"

	"gorm.io/gorm"
)

func oddsUpdateInput(view *GameOddsLimits) UpdateOddsLimitsInput {
	if view == nil {
		return UpdateOddsLimitsInput{}
	}
	return UpdateOddsLimitsInput{
		ExpectedRuleVersion: view.RuleVersion,
		ExpectedRevision:    view.ConfigRevision,
		Items:               view.Items,
	}
}

func oddsMutationGuard(view *GameOddsLimits) OddsMutationGuard {
	if view == nil {
		return OddsMutationGuard{}
	}
	return OddsMutationGuard{
		ExpectedRuleVersion: view.RuleVersion,
		ExpectedRevision:    view.ConfigRevision,
	}
}

// Test-only quotes are explicitly saved through the administrator boundary.
// No production bootstrap or catalogue may provide these numeric fixtures.
func configureTestGameOdds(t *testing.T, db *gorm.DB, gameID string, prices map[string]float64) *GameOddsLimits {
	t.Helper()
	service := NewOddsAdminService(db)
	view, err := service.Get(gameID)
	if err != nil {
		t.Fatal("read test odds catalogue:", err)
	}
	for index := range view.Items {
		if value, ok := prices[view.Items[index].PlayCode]; ok {
			view.Items[index].Odds = value
		}
	}
	result, err := service.Update(gameID, oddsUpdateInput(view))
	if err != nil {
		t.Fatal("explicitly save test odds:", err)
	}
	return result
}
