package services

import (
	"backend/data/models/lottery"
	"testing"
)

var completedDigits5V3Prices = map[string]float64{
	"two_sided":        1.993,
	"ball_1_5":         9.9,
	"dragon_tiger":     1.993,
	"dragon_tiger_tie": 8.7,
	"leopard":          80,
	"straight":         15.08,
	"pair":             3.38,
	"half_straight":    2.58,
	"mixed":            3.08,
}

type digits5V3CompletionCase struct {
	code, selection string
	position        int
	draw            []int
}

var digits5V3CompletionCases = []digits5V3CompletionCase{
	{code: "two_sided", selection: "大", position: 1, draw: []int{9, 0, 1, 2, 3}},
	{code: "ball_1_5", selection: "9", position: 1, draw: []int{9, 0, 1, 2, 3}},
	{code: "dragon_tiger", selection: "龙", position: 1, draw: []int{9, 0, 1, 2, 3}},
	{code: "dragon_tiger_tie", selection: "和", position: 1, draw: []int{9, 0, 1, 2, 9}},
	{code: "leopard", selection: "豹子", position: 1, draw: []int{5, 5, 5, 1, 2}},
	{code: "straight", selection: "顺子", position: 1, draw: []int{1, 2, 3, 8, 9}},
	{code: "pair", selection: "对子", position: 1, draw: []int{1, 1, 4, 8, 9}},
	{code: "half_straight", selection: "半顺", position: 1, draw: []int{1, 2, 4, 8, 9}},
	{code: "mixed", selection: "杂六", position: 1, draw: []int{1, 3, 5, 8, 9}},
}

func TestCompletedDigits5V3CatalogueMatchesPlacementAndSettlementCodes(t *testing.T) {
	if len(completedDigits5V3Prices) != 9 || len(digits5V3CompletionCases) != 9 {
		t.Fatalf("completed digits5-v3 inventory is not 9/9: prices=%d cases=%d", len(completedDigits5V3Prices), len(digits5V3CompletionCases))
	}
	for _, gameID := range []string{"speed-ssc", "sg-ssc", "au-lucky-5"} {
		game := &lottery.Game{ID: gameID}
		profile, ready := rulesForGame(game)
		catalog := PlayCatalogForGame(gameID)
		if !ready || profile.Version != "digits5-v3" || len(catalog) != 9 {
			t.Fatalf("%s digits5-v3 catalogue = ready:%t version:%q rows:%d", gameID, ready, profile.Version, len(catalog))
		}
		for _, item := range catalog {
			if price, ok := completedDigits5V3Prices[item.PlayCode]; !ok || price <= 1 {
				t.Fatalf("%s catalogue play %s has no approved price", gameID, item.PlayCode)
			}
		}
		for _, test := range digits5V3CompletionCases {
			code, _, err := InferPlayForGame(game, "", "", test.position, test.selection)
			if err != nil || code != test.code {
				t.Fatalf("%s placement inferred %q for %s, want %q: %v", gameID, code, test.selection, test.code, err)
			}
			if err := profile.validateChoice(code, test.position, test.selection); err != nil {
				t.Fatalf("%s placement rejected %s/%s: %v", gameID, code, test.selection, err)
			}
			won, _, err := evaluateBetForRuleVersion(game, profile.Version, test.draw, code, test.position, test.selection)
			if err != nil || !won {
				t.Fatalf("%s settlement disagrees with placement code %s/%s: won=%t err=%v", gameID, code, test.selection, won, err)
			}
		}
	}
}

func TestCompletedDigits5V3OddsPostgresExposeNineResolvableMarketsPerGame(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "digits5_v3_complete_odds", "782091")
	member := timingPostgresMember(t, db, room, "digits5_v3_complete_member")
	trading := NewTradingAdminService(db)

	for _, gameID := range []string{"speed-ssc", "sg-ssc", "au-lucky-5"} {
		view := configureTestGameOdds(t, db, gameID, completedDigits5V3Prices)
		if view.RuleVersion != "digits5-v3" || len(view.Items) != 9 || len(view.RiskWarnings) != 0 {
			t.Fatalf("%s completed view = version:%q rows:%d warnings:%+v", gameID, view.RuleVersion, len(view.Items), view.RiskWarnings)
		}
		for _, item := range view.Items {
			want, ok := completedDigits5V3Prices[item.PlayCode]
			if !ok || !item.Configured || item.Odds != want || item.RuleVersion != "digits5-v3" || item.ConfigurationSource != oddsSourceAdminSave || item.ConfiguredAt == nil {
				t.Fatalf("%s incomplete auditable quote: %+v want=%v", gameID, item, want)
			}
		}
		for _, test := range digits5V3CompletionCases {
			resolved, err := trading.Resolve(member.UserID, gameID, test.code, 20, 999, 0)
			if err != nil || resolved.PricingCode != test.code || resolved.Odds != completedDigits5V3Prices[test.code] {
				t.Fatalf("%s pricing did not resolve placement code %s: %+v / %v", gameID, test.code, resolved, err)
			}
		}
	}
}
