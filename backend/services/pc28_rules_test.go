package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestPC28CatalogUsesOnlyAtomicUnpricedMarkets(t *testing.T) {
	items := pc28PlayCatalog()
	if len(items) != 32 {
		t.Fatalf("PC28 catalog has %d items, want 32", len(items))
	}
	encoded, err := json.Marshal(items)
	if err != nil || strings.Contains(string(encoded), "default_odds") {
		t.Fatalf("catalogue leaked default-price metadata: %s / %v", encoded, err)
	}
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.PlayCode == "" || item.PlayName == "" {
			t.Fatalf("non-atomic or fabricated catalog item: %+v", item)
		}
		if _, exists := seen[item.PlayCode]; exists {
			t.Fatalf("duplicate PC28 atomic market: %s", item.PlayCode)
		}
		seen[item.PlayCode] = struct{}{}
	}
	for total := 0; total <= 27; total++ {
		code := pc28ExactCode(total)
		if _, exists := seen[code]; !exists {
			t.Fatalf("sum %d has no atomic price code", total)
		}
		if err := pc28ValidateChoice(code, 0, strconv.Itoa(total)); err != nil {
			t.Fatalf("sum %d rejected from %s: %v", total, code, err)
		}
	}
}

func TestPC28AssistantParserAndRepeatUseTypedAtomicContracts(t *testing.T) {
	game := &lottery.Game{ID: "pc-canada"}
	lines, err := parseAssistantBetForGame(game, "1/5#27/6#特码/3/1/2/7#13/89/8#1大9#大/10#大单/11#极小12#红波/13#豹子14")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 13 {
		t.Fatalf("parsed %d lines: %+v", len(lines), lines)
	}
	wantPrefix := []AssistantBetLine{
		{Position: 0, Selection: "1", PlayCode: pc28ExactCode(1), Amount: 5},
		{Position: 0, Selection: "27", PlayCode: pc28ExactCode(27), Amount: 6},
		{Position: 0, Selection: "1,2,3", PlayCode: pc28PackageThree, Amount: 7},
		{Position: 1, Selection: "8", PlayCode: pc28PositionNumber, Amount: 8},
		{Position: 1, Selection: "9", PlayCode: pc28PositionNumber, Amount: 8},
		{Position: 3, Selection: "8", PlayCode: pc28PositionNumber, Amount: 8},
		{Position: 3, Selection: "9", PlayCode: pc28PositionNumber, Amount: 8},
		{Position: 1, Selection: "大", PlayCode: pc28PositionTwoSided, Amount: 9},
	}
	for index, want := range wantPrefix {
		got := lines[index]
		if got.Position != want.Position || got.Selection != want.Selection || got.PlayCode != want.PlayCode || got.Amount != want.Amount || got.PlayName == "" || got.Label == "" {
			t.Fatalf("line %d: got=%+v want=%+v", index, got, want)
		}
	}
	if _, err := parseAssistantBetForGame(game, "15"); apperrors.GetErrorCode(err) != "INVALID_REQUEST" {
		t.Fatalf("pure numeric PC28 chat command was accepted without /: %v", err)
	}
	content, err := AssistantRepeatContent(game.ID, lines)
	if err != nil || content == "" {
		t.Fatalf("repeat content: %q / %v", content, err)
	}
	repeated, err := parseAssistantBetForGame(game, content)
	if err != nil || !reflect.DeepEqual(repeated, lines) {
		t.Fatalf("repeat changed atomic terms:\ncontent=%s\noriginal=%+v\nrepeated=%+v\nerror=%v", content, lines, repeated, err)
	}
}

func TestPC28SelectionValidationRejectsCodeSelectionDrift(t *testing.T) {
	for _, test := range []struct {
		code      string
		position  int
		selection string
	}{
		{pc28ExactCode(1), 0, "2"},
		{pc28PackageThree, 0, "1,1,2"},
		{pc28PositionNumber, 4, "1"},
		{pc28PositionNumber, 1, "10"},
		{pc28PositionTwoSided, 1, "龙"},
		{pc28DragonTiger, 1, "和"},
		{pc28DragonTigerTie, 1, "龙"},
		{pc28ComboBigOdd, 0, "小单"},
		{pc28ColorRed, 0, "蓝波"},
		{pc28Straight, 0, "对子"},
	} {
		if err := pc28ValidateChoice(test.code, test.position, test.selection); err == nil {
			t.Fatalf("invalid typed choice accepted: %+v", test)
		}
	}
}

func TestPC28ReverseRestrictionOnlyCoversAggregateSumMarkets(t *testing.T) {
	profile, ok := rulesForVersion(pc28RuleV1)
	if !ok {
		t.Fatal("missing pc28-v1 profile")
	}
	// No database access is required here: neither positioning market belongs
	// to the aggregate sum-size/sum-parity reverse restriction.
	if err := validatePC28PlacementConstraints(nil, profile, 1, "tenant:1", "pc-canada", "1", 9, []betLimitEntry{
		{PlayCode: pc28PositionTwoSided, Position: 1, Selection: "大", AmountCents: 100},
		{PlayCode: pc28PositionTwoSided, Position: 1, Selection: "小", AmountCents: 100},
		{PlayCode: pc28DragonTiger, Position: 1, Selection: "龙", AmountCents: 100},
		{PlayCode: pc28DragonTiger, Position: 1, Selection: "虎", AmountCents: 100},
	}); err != nil {
		t.Fatalf("position/dragon markets inherited the sum reverse restriction: %v", err)
	}
}

func TestPC28EvaluationCoversDrawMarketsAndOriginalWrappingStraight(t *testing.T) {
	profile, ok := rulesForVersion(pc28RuleV1)
	if !ok {
		t.Fatal("missing pc28-v1 profile")
	}
	for _, values := range [][]int{{8, 9, 0}, {9, 0, 1}, {0, 1, 9}, {0, 9, 8}, {1, 9, 0}} {
		if shape := pc28Shape(values); shape != pc28Straight {
			t.Fatalf("original wrapping sequence %v got shape %s", values, shape)
		}
	}
	for _, values := range [][]int{{0, 1, 2}, {2, 0, 1}, {7, 8, 9}} {
		if shape := pc28Shape(values); shape != pc28Straight {
			t.Fatalf("ordinary sequence %v got shape %s", values, shape)
		}
	}
	checks := []struct {
		numbers          []int
		code             string
		position         int
		selection        string
		grayPush, winner bool
		outcome          markSixBetOutcome
	}{
		{[]int{1, 2, 3}, pc28ExactCode(6), 0, "6", false, true, markSixOutcomeWon},
		{[]int{1, 2, 3}, pc28PackageThree, 0, "5,6,7", false, true, markSixOutcomeWon},
		{[]int{1, 2, 3}, pc28PositionNumber, 2, "2", false, true, markSixOutcomeWon},
		{[]int{9, 2, 3}, pc28PositionTwoSided, 1, "大", false, true, markSixOutcomeWon},
		{[]int{1, 2, 3}, pc28DragonTiger, 1, "虎", false, true, markSixOutcomeWon},
		{[]int{1, 2, 1}, pc28DragonTigerTie, 1, "和", false, true, markSixOutcomeWon},
		{[]int{1, 2, 3}, pc28ColorRed, 0, "红波", false, true, markSixOutcomeWon},
		{[]int{3, 3, 3}, pc28Leopard, 0, "豹子", false, true, markSixOutcomeWon},
		{[]int{3, 3, 4}, pc28Pair, 0, "对子", false, true, markSixOutcomeWon},
		{[]int{8, 9, 0}, pc28Straight, 0, "顺子", false, true, markSixOutcomeWon},
		{[]int{4, 4, 5}, pc28ColorRed, 0, "红波", true, false, markSixOutcomePush},
	}
	for _, check := range checks {
		outcome, reason, err := evaluatePC28Bet(profile, check.numbers, check.code, check.position, check.selection, check.grayPush)
		if err != nil || outcome != check.outcome || (outcome == markSixOutcomeWon) != check.winner || reason == "" {
			t.Fatalf("%+v -> outcome=%s reason=%q error=%v", check, outcome, reason, err)
		}
	}
}

func TestPC28Variant13And14SettlementStrictBoundaries(t *testing.T) {
	draw := []int{9, 5, 0} // 14, big/even.
	base := bet.Bet{AmountCents: 100, Odds: 1.993, Position: 0, PlayCode: pc28SumSize, Selection: "大"}
	for _, test := range []struct {
		name, gameID, version, playCode, selection, policy string
		stakeCents                                         int64
		wantOutcome                                        markSixBetOutcome
		wantOdds                                           float64
		wantValid                                          int64
	}{
		{"v1 exactly one keeps price but invalid turnover", "pc-canada", pc28RuleV1, pc28SumSize, "大", "pc28_standard", 100, markSixOutcomeWon, 1.993, 0},
		{"v1 above one two-sided", "pc-canada", pc28RuleV1, pc28SumSize, "大", "pc28_v1_13_14_two_sided_gt1", 101, markSixOutcomeWon, 1.5, 0},
		{"v1 exactly 9999 remains 1.5", "pc-canada", pc28RuleV1, pc28SumSize, "大", "pc28_v1_13_14_two_sided_gt1", 999900, markSixOutcomeWon, 1.5, 0},
		{"v1 above 9999 is one", "pc-canada", pc28RuleV1, pc28SumSize, "大", "pc28_v1_13_14_two_sided_gt9999", 999901, markSixOutcomeWon, 1, 0},
		{"v2 combo exactly one is ordinary", "canada-28", pc28RuleV2, pc28ComboBigEven, "大双", "pc28_standard", 100, markSixOutcomeWon, 1.993, 100},
		{"v2 combo above one dealer take", "canada-28", pc28RuleV2, pc28ComboBigEven, "大双", "pc28_v2_13_14_combo_dealer_take", 101, markSixOutcomeLost, 0, 100},
		{"v3 two-sided above one", "canada-20", pc28RuleV3, pc28SumSize, "大", "pc28_v3_13_14_two_sided_gt1", 101, markSixOutcomeWon, 1.98, 100},
		{"v3 combo above one", "canada-20", pc28RuleV3, pc28ComboBigEven, "大双", "pc28_v3_13_14_combo_gt1", 101, markSixOutcomeWon, 3.65, 100},
	} {
		t.Run(test.name, func(t *testing.T) {
			item := base
			item.RuleVersion, item.PlayCode, item.Selection = test.version, test.playCode, test.selection
			decision, err := decidePC28Settlement(test.gameID, item, draw, test.stakeCents)
			if err != nil || decision.Outcome != test.wantOutcome || decision.EffectiveOdds != test.wantOdds ||
				decision.ValidTurnoverCents != test.wantValid || decision.UserIssueStakeCents != test.stakeCents || decision.Policy != test.policy {
				t.Fatalf("decision=%+v error=%v", decision, err)
			}
		})
	}
	loser := base
	loser.RuleVersion, loser.Selection = pc28RuleV1, "小"
	if decision, err := decidePC28Settlement("pc-canada", loser, draw, 101); err != nil || decision.Outcome != markSixOutcomeLost || decision.ValidTurnoverCents != 0 {
		t.Fatalf("v1 losing 13/14 ticket retained turnover: %+v / %v", decision, err)
	}
}

func TestPC28GrayPushAndFinancialTurnoverKeepActualGGR(t *testing.T) {
	item := bet.Bet{
		RuleVersion: pc28RuleV3, PlayCode: pc28ColorRed, Position: 0, Selection: "红波",
		AmountCents: 1000, Odds: 2.8, PC28GrayPush: true,
	}
	decision, err := decidePC28Settlement("canada-20", item, []int{4, 4, 5}, 1000)
	if err != nil || decision.Outcome != markSixOutcomePush || decision.EffectiveOdds != 1 || decision.ValidTurnoverCents != 0 || decision.Policy != "pc28_gray_push" {
		t.Fatalf("gray-wave snapshot was not honored: %+v / %v", decision, err)
	}
	zero := int64(0)
	item.ValidTurnoverCents = &zero
	item.RebateRateSnapshot = 5
	item.AgentShareRateSnapshot = 10
	rebate, share := settledBetFinancialAmounts(item, 0, false)
	if rebate != 0 || share != 100 {
		t.Fatalf("valid turnover and actual GGR were conflated: rebate=%d share=%d", rebate, share)
	}
}

func TestPC28RejectsMismatchedRuleVersions(t *testing.T) {
	item := bet.Bet{RuleVersion: pc28RuleV1, PlayCode: pc28ExactCode(6), Position: 0, Selection: "6", AmountCents: 100, Odds: 9}
	for _, gameID := range []string{"canada-28", "canada-20", "unknown"} {
		if _, err := decidePC28Settlement(gameID, item, []int{1, 2, 3}, 100); apperrors.GetErrorCode(err) != "RULES_NOT_READY" || !strings.Contains(err.Error(), "版本") {
			t.Fatalf("%s accepted pc28-v1: %v", gameID, err)
		}
	}
}
