package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBingoMarkSixDrawAndModeContract(t *testing.T) {
	game := &lottery.Game{ID: "bingo-mark-six"}
	profile, ok := rulesForGame(game)
	if !ok || profile.Version != markSixRuleVersion || !profile.MarkSix || profile.BallCount != 7 || profile.MinNumber != 1 || profile.MaxNumber != 49 || !profile.Unique {
		t.Fatalf("unexpected profile: %+v ready=%v", profile, ok)
	}
	if err := profile.validateDraw([]int{1, 2, 3, 4, 5, 6, 49}); err != nil {
		t.Fatal(err)
	}
	for _, draw := range [][]int{
		{1, 2, 3, 4, 5, 6}, {1, 2, 3, 4, 5, 6, 7, 8},
		{0, 2, 3, 4, 5, 6, 7}, {1, 2, 3, 4, 5, 6, 50}, {1, 2, 3, 4, 5, 6, 6},
	} {
		if err := profile.validateDraw(draw); err == nil {
			t.Fatalf("invalid draw accepted: %v", draw)
		}
	}
	if err := ensurePlacementBetMode(profile, "web"); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"", "chat", "assistant", "WEB"} {
		if code := apperrors.GetErrorCode(ensurePlacementBetMode(profile, mode)); code != "BET_MODE_UNAVAILABLE" {
			t.Fatalf("mode %q returned %s", mode, code)
		}
	}
	if _, err := parseAssistantBetForGame(game, "7/49/10"); apperrors.GetErrorCode(err) != "BET_MODE_UNAVAILABLE" {
		t.Fatalf("assistant parser accepted Mark Six: %v", err)
	}
}

func TestBingoMarkSixCatalogRequiresExplicitPricesAndCurrentContract(t *testing.T) {
	coreCodes := make([]string, 0, len(markSixCoreSpecs))
	for _, spec := range markSixCoreSpecs {
		coreCodes = append(coreCodes, spec.Play.Code)
	}
	wantCore := []string{
		"marksix_special_a_number", "marksix_special_b_number", "marksix_regular_number",
		"marksix_regular_position_number", "marksix_regular_special_number",
		"marksix_combo_4_all", "marksix_combo_3_all", "marksix_combo_2_all",
		"marksix_combo_special_pair", "marksix_not_in", "marksix_combo_3_2", "marksix_combo_2_special",
		"marksix_combo_3_2_exact2", "marksix_combo_3_2_exact3",
		"marksix_combo_2_special_regular", "marksix_combo_2_special_mixed",
	}
	if !reflect.DeepEqual(coreCodes, wantCore) {
		t.Fatalf("core markets changed: %v", coreCodes)
	}
	for _, version := range []string{"", "mark6-v1", "unknown"} {
		if specs := markSixSpecsForVersion(version); len(specs) != 0 {
			t.Fatalf("unsupported contract %q exposed markets: %v", version, specs)
		}
	}
	catalog := PlayCatalogForGame("bingo-mark-six")
	if len(markSixV2Specs) != 252 || len(catalog) <= len(markSixCoreSpecs) {
		t.Fatalf("current catalog incomplete: %d", len(catalog))
	}
	if len(catalog) != 242 {
		t.Fatalf("unexpected atomic Mark Six catalog size: %d", len(catalog))
	}
	seen := make(map[string]bool)
	for _, item := range catalog {
		if seen[item.PlayCode] {
			t.Fatalf("duplicate play code: %s", item.PlayCode)
		}
		seen[item.PlayCode] = true
		if strings.Contains(item.Description, "首次后台默认赔率") {
			t.Fatalf("catalog advertises an automatic price: %+v", item)
		}
	}
	payload, err := json.Marshal(catalog)
	if err != nil || strings.Contains(string(payload), "default_odds") {
		t.Fatalf("catalog must not advertise code-owned prices: %s, err=%v", payload, err)
	}
	profile, _ := rulesForGame(&lottery.Game{ID: "bingo-mark-six"})
	for _, code := range []string{"marksix_special_zodiac", "marksix_one_zodiac", "marksix_total_zodiac", "marksix_seven_color_wave"} {
		if profile.supportsPlay(code) {
			t.Fatalf("complex unsupported play exposed: %s", code)
		}
	}
	for _, code := range []string{
		"marksix_special_zodiac_horse", "marksix_one_zodiac_horse", "marksix_total_zodiac_6",
		"marksix_total_zodiac_odd", "marksix_seven_color_red", "marksix_seven_color_draw",
		"marksix_link_zodiac_2", "marksix_link_tail_5", "marksix_combo_3_2", "marksix_combo_2_special",
	} {
		if !profile.supportsPlay(code) {
			t.Fatalf("verified atomic play missing: %s", code)
		}
	}
	for _, code := range []string{"marksix_link_zodiac_2", "marksix_link_tail_5", "marksix_combo_3_2", "marksix_combo_2_special"} {
		if seen[code] {
			t.Fatalf("public composite parent leaked into administrator pricing rows: %s", code)
		}
	}
	for _, code := range []string{"marksix_link_zodiac_2_rat", "marksix_link_tail_5_9", "marksix_combo_3_2_exact2", "marksix_combo_3_2_exact3", "marksix_combo_2_special_mixed", "marksix_combo_2_special_regular"} {
		if !seen[code] {
			t.Fatalf("administrator pricing row missing: %s", code)
		}
	}
}

func TestBingoMarkSixChoiceCanonicalizationAndValidation(t *testing.T) {
	game := &lottery.Game{ID: "bingo-mark-six"}
	for _, test := range []struct {
		code      string
		position  int
		selection string
	}{
		{"marksix_special_a_number", 7, "49"},
		{"marksix_special_b_number", 7, "1"},
		{"marksix_regular_number", 0, "25"},
		{"marksix_regular_position_number", 1, "1"},
		{"marksix_regular_position_number", 6, "49"},
		{"marksix_regular_special_number", 3, "18"},
		{"marksix_combo_4_all", 0, "1,2,3,49"},
		{"marksix_combo_3_all", 0, "1,2,49"},
		{"marksix_combo_2_all", 0, "1,49"},
		{"marksix_combo_special_pair", 0, "1,49"},
		{"marksix_not_in", 0, "1,2,3,4,49"},
		{"marksix_special_zodiac_horse", 7, "马"},
		{"marksix_one_zodiac_horse", 0, "马"},
		{"marksix_total_zodiac_5", 0, "5肖"},
		{"marksix_total_zodiac_odd", 0, "总肖单"},
		{"marksix_seven_color_red", 0, "红波"},
		{"marksix_seven_color_draw", 0, "和局"},
		{"marksix_one_tail_0", 0, "0尾"},
		{"marksix_combined_zodiac_2", 7, "鼠,牛"},
		{"marksix_regular_zodiac_horse", 0, "马"},
		{"marksix_link_zodiac_2", 0, "鼠,牛"},
		{"marksix_link_tail_2", 0, "0尾,1尾"},
		{"marksix_not_in_11", 0, "1,2,3,4,5,6,7,8,9,10,11"},
		{"marksix_combo_3_2", 0, "1,2,3"},
		{"marksix_combo_2_special", 0, "1,2"},
	} {
		if err := validateBetChoice(game, test.code, test.position, test.selection); err != nil {
			t.Fatalf("valid %+v rejected: %v", test, err)
		}
	}
	if got := normalizeBetSelection(game, "marksix_combo_4_all", "49，3, 2,1"); got != "1,2,3,49" {
		t.Fatalf("combination not canonicalized: %q", got)
	}
	if got := normalizeBetSelection(game, "marksix_link_zodiac_3", "虎，鼠,牛肖"); got != "鼠,牛,虎" {
		t.Fatalf("zodiac list not canonicalized: %q", got)
	}
	if got := normalizeBetSelection(game, "marksix_link_tail_3", "9尾，1,0尾"); got != "0尾,1尾,9尾" {
		t.Fatalf("tail list not canonicalized: %q", got)
	}
	for _, test := range []struct {
		code      string
		position  int
		selection string
	}{
		{"marksix_special_a_number", 0, "49"},
		{"marksix_special_a_number", 7, "0"},
		{"marksix_special_a_number", 7, "50"},
		{"marksix_regular_number", 1, "1"},
		{"marksix_regular_position_number", 0, "1"},
		{"marksix_regular_position_number", 7, "1"},
		{"marksix_combo_4_all", 0, "1,2,3"},
		{"marksix_combo_4_all", 0, "1,2,3,3"},
		{"marksix_combo_3_all", 0, "1,2,50"},
		{"marksix_combo_2_all", 0, "2,1"},
		{"marksix_not_in", 0, "1,2,3,4,4"},
		{"marksix_combined_zodiac_2", 0, "鼠,牛"},
		{"marksix_link_zodiac_2", 0, "鼠,鼠"},
		{"marksix_link_tail_2", 0, "0尾,10尾"},
		{"marksix_not_in_11", 0, "1,2,3,4,5,6,7,8,9,10"},
		{"marksix_combo_3_2_exact2", 0, "1,2,3"},
	} {
		if err := validateBetChoice(game, test.code, test.position, test.selection); err == nil {
			t.Fatalf("invalid %+v accepted", test)
		}
	}
}

func TestBingoMarkSixCoreSettlementTruthTable(t *testing.T) {
	game := &lottery.Game{ID: "bingo-mark-six"}
	draw := []int{1, 2, 3, 4, 5, 6, 49}
	for _, test := range []struct {
		code      string
		position  int
		selection string
		want      bool
	}{
		{"marksix_special_a_number", 7, "49", true},
		{"marksix_special_b_number", 7, "48", false},
		{"marksix_regular_number", 0, "1", true},
		{"marksix_regular_number", 0, "49", false},
		{"marksix_regular_position_number", 3, "3", true},
		{"marksix_regular_special_number", 6, "5", false},
		{"marksix_combo_4_all", 0, "1,2,3,4", true},
		{"marksix_combo_4_all", 0, "1,2,3,49", false},
		{"marksix_combo_3_all", 0, "2,3,4", true},
		{"marksix_combo_2_all", 0, "5,6", true},
		{"marksix_combo_special_pair", 0, "1,49", true},
		{"marksix_combo_special_pair", 0, "1,2", false},
		{"marksix_not_in", 0, "7,8,9,10,11", true},
		{"marksix_not_in", 0, "1,7,8,9,10", false},
	} {
		got, _, err := evaluateBetForRuleVersion(game, markSixRuleVersion, draw, test.code, test.position, test.selection)
		if err != nil || got != test.want {
			t.Fatalf("%+v got=%v err=%v", test, got, err)
		}
	}
	for _, version := range []string{"", "mark6-v1"} {
		if _, _, err := evaluateBetForRuleVersion(game, version, draw, "marksix_special_a_number", 7, "49"); apperrors.GetErrorCode(err) != "RULES_NOT_READY" {
			t.Fatalf("unsupported contract %q was reinterpreted: %v", version, err)
		}
	}
	if _, _, err := evaluateBetForRuleVersion(game, markSixRuleVersion, []int{1, 2, 3, 4, 5, 6, 6}, "marksix_special_a_number", 7, "6"); apperrors.GetErrorCode(err) != "INVALID_DRAW" {
		t.Fatalf("invalid draw was settled: %v", err)
	}
}

func TestBingoMarkSixV2SideTruthTableAnd49Semantics(t *testing.T) {
	game := &lottery.Game{ID: "bingo-mark-six"}
	drawAt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	fortyNine := []int{1, 2, 3, 4, 5, 6, 49}
	for _, test := range []struct {
		code, selection string
		position        int
		want            markSixBetOutcome
	}{
		{"marksix_special_big_small", "大", 7, markSixOutcomePush},
		{"marksix_special_odd_even", "单", 7, markSixOutcomePush},
		{"marksix_special_sum_big_small", "合大", 7, markSixOutcomePush},
		{"marksix_special_sum_odd_even", "合双", 7, markSixOutcomePush},
		{"marksix_special_heaven_earth", "天肖", 7, markSixOutcomePush},
		{"marksix_special_front_back", "前肖", 7, markSixOutcomePush},
		{"marksix_special_domestic_wild", "家肖", 7, markSixOutcomePush},
		{"marksix_special_tail_big_small", "尾大", 7, markSixOutcomePush},
		{"marksix_special_half", "大单", 7, markSixOutcomeLost},
		{"marksix_total_odd_even", "总和双", 0, markSixOutcomeWon},
		{"marksix_total_big_small", "总和小", 0, markSixOutcomeWon},
		{"marksix_regular_position_big_small", "小", 1, markSixOutcomeWon},
		{"marksix_regular_position_odd_even", "单", 1, markSixOutcomeWon},
		{"marksix_regular_position_sum_big_small", "合小", 1, markSixOutcomeWon},
		{"marksix_regular_position_sum_odd_even", "合单", 1, markSixOutcomeWon},
		{"marksix_regular_position_tail_big_small", "尾小", 1, markSixOutcomeWon},
	} {
		got, _, err := evaluateBetOutcomeForRuleVersionAt(game, markSixRuleVersion, fortyNine, test.code, test.position, test.selection, drawAt)
		if err != nil || got != test.want {
			t.Fatalf("%s/%s got=%s want=%s err=%v", test.code, test.selection, got, test.want, err)
		}
	}
}

func TestBingoMarkSixV2AtomicMarketsAndDrawTimeZodiac(t *testing.T) {
	game := &lottery.Game{ID: "bingo-mark-six"}
	draw := []int{35, 34, 23, 30, 22, 6, 20}
	afterNewYear := time.Date(2026, 2, 17, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	beforeNewYear := afterNewYear.Add(-24 * time.Hour)
	if zodiac, err := markSixNumberZodiac(1, beforeNewYear); err != nil || zodiac != "蛇" {
		t.Fatalf("pre-new-year zodiac=%q err=%v", zodiac, err)
	}
	if zodiac, err := markSixNumberZodiac(1, afterNewYear); err != nil || zodiac != "马" {
		t.Fatalf("new-year zodiac=%q err=%v", zodiac, err)
	}
	boundaryDraw := []int{2, 3, 4, 5, 6, 7, 1}
	beforeOutcome, _, beforeErr := evaluateBetOutcomeForRuleVersionAt(game, markSixRuleVersion, boundaryDraw, "marksix_special_zodiac_horse", 7, "马", beforeNewYear)
	afterOutcome, _, afterErr := evaluateBetOutcomeForRuleVersionAt(game, markSixRuleVersion, boundaryDraw, "marksix_special_zodiac_horse", 7, "马", afterNewYear)
	if beforeErr != nil || afterErr != nil || beforeOutcome != markSixOutcomeLost || afterOutcome != markSixOutcomeWon {
		t.Fatalf("draw timestamp did not freeze zodiac year: before=%s/%v after=%s/%v", beforeOutcome, beforeErr, afterOutcome, afterErr)
	}
	for _, test := range []struct {
		code, selection string
		position        int
	}{
		{"marksix_special_zodiac_pig", "猪", 7},
		{"marksix_special_heaven_earth", "天肖", 7},
		{"marksix_special_domestic_wild", "家肖", 7},
		{"marksix_color_wave_blue", "蓝波", 7},
		{"marksix_half_wave_blue_small", "蓝小", 7},
		{"marksix_halfhalf_blue_small_even", "蓝小双", 7},
		{"marksix_special_head_2", "2头", 7},
		{"marksix_special_tail_0", "0尾", 7},
		{"marksix_five_element_metal", "金", 7},
		{"marksix_regular_color_red", "红波", 1},
	} {
		got, _, err := evaluateBetOutcomeForRuleVersionAt(game, markSixRuleVersion, draw, test.code, test.position, test.selection, afterNewYear)
		if err != nil || got != markSixOutcomeWon {
			t.Fatalf("%s/%s got=%s err=%v", test.code, test.selection, got, err)
		}
	}
	if _, _, err := evaluateBetOutcomeForRuleVersionAt(game, markSixRuleVersion, draw, "marksix_special_zodiac_pig", 7, "猪", time.Time{}); apperrors.GetErrorCode(err) != "RULES_NOT_READY" {
		t.Fatalf("zodiac settled without draw time: %v", err)
	}
}

func TestBingoMarkSixV2OneAndTotalZodiacUseAllSevenNumbersOnce(t *testing.T) {
	game := &lottery.Game{ID: "bingo-mark-six"}
	drawAt := time.Date(2026, 3, 1, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	// 1/13/25 are all the current-year horse zodiac. They count as one
	// distinct zodiac and one winning 一肖 occurrence, never three payouts.
	draw := []int{1, 13, 25, 2, 3, 4, 5}
	for _, test := range []struct {
		code, selection string
		want            markSixBetOutcome
	}{
		{"marksix_one_zodiac_horse", "马", markSixOutcomeWon},
		{"marksix_one_zodiac_ox", "牛", markSixOutcomeLost},
		{"marksix_total_zodiac_5", "5肖", markSixOutcomeWon},
		{"marksix_total_zodiac_6", "6肖", markSixOutcomeLost},
		{"marksix_total_zodiac_odd", "总肖单", markSixOutcomeWon},
		{"marksix_total_zodiac_even", "总肖双", markSixOutcomeLost},
	} {
		got, _, err := evaluateBetOutcomeForRuleVersionAt(game, markSixRuleVersion, draw, test.code, 0, test.selection, drawAt)
		if err != nil || got != test.want {
			t.Fatalf("%s/%s got=%s want=%s err=%v", test.code, test.selection, got, test.want, err)
		}
	}
	// 49 belongs to the current-year zodiac and participates normally in 一肖.
	withFortyNine := []int{2, 3, 4, 5, 6, 7, 49}
	got, _, err := evaluateBetOutcomeForRuleVersionAt(game, markSixRuleVersion, withFortyNine, "marksix_one_zodiac_horse", 0, "马", drawAt)
	if err != nil || got != markSixOutcomeWon {
		t.Fatalf("49 did not participate in 一肖: got=%s err=%v", got, err)
	}
	if _, _, err := evaluateBetOutcomeForRuleVersionAt(game, markSixRuleVersion, draw, "marksix_total_zodiac_5", 0, "5肖", time.Time{}); apperrors.GetErrorCode(err) != "RULES_NOT_READY" {
		t.Fatalf("total zodiac settled without immutable draw time: %v", err)
	}
}

func TestBingoMarkSixCompletedOriginalMarketsTruthTable(t *testing.T) {
	game := &lottery.Game{ID: "bingo-mark-six"}
	drawAt := time.Date(2026, 3, 1, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	// 2026 马年：1/13/25/37/49属马，2/14/26/38属蛇。
	draw := []int{1, 13, 2, 3, 4, 5, 7}
	for _, test := range []struct {
		code, selection string
		position        int
		want            markSixBetOutcome
	}{
		{"marksix_one_tail_1", "1尾", 0, markSixOutcomeWon},
		{"marksix_one_tail_9", "9尾", 0, markSixOutcomeLost},
		{"marksix_combined_zodiac_2", "鼠,牛", 7, markSixOutcomeWon}, // special 7 is rat
		{"marksix_combined_zodiac_2", "牛,虎", 7, markSixOutcomeLost},
		{"marksix_link_zodiac_2", "蛇,马", 0, markSixOutcomeWon},
		{"marksix_link_zodiac_3", "鼠,牛,猪", 0, markSixOutcomeLost},
		{"marksix_link_tail_3", "1尾,2尾,3尾", 0, markSixOutcomeWon},
		{"marksix_link_tail_2", "8尾,9尾", 0, markSixOutcomeLost},
		{"marksix_regular_zodiac_horse", "马", 0, markSixOutcomeWon},
		{"marksix_regular_zodiac_pig", "猪", 0, markSixOutcomeLost},
		{"marksix_not_in_6", "20,21,22,23,24,25", 0, markSixOutcomeWon},
		{"marksix_not_in_11", "1,20,21,22,23,24,25,26,27,28,29", 0, markSixOutcomeLost},
	} {
		got, _, err := evaluateBetOutcomeForRuleVersionAt(game, markSixRuleVersion, draw, test.code, test.position, test.selection, drawAt)
		if err != nil || got != test.want {
			t.Fatalf("%s/%s got=%s want=%s err=%v", test.code, test.selection, got, test.want, err)
		}
	}
	pushDraw := []int{1, 2, 3, 4, 5, 6, 49}
	got, _, err := evaluateBetOutcomeForRuleVersionAt(game, markSixRuleVersion, pushDraw, "marksix_combined_zodiac_2", 7, "鼠,牛", drawAt)
	if err != nil || got != markSixOutcomePush {
		t.Fatalf("合肖49 semantics got=%s err=%v", got, err)
	}
}

func TestBingoMarkSixLinkedMarketsExposeStableParentsAndAtomicPricingCandidates(t *testing.T) {
	for _, test := range []struct {
		playCode, selection string
		want                []string
	}{
		{"marksix_link_zodiac_3", "鼠,牛,虎", []string{"marksix_link_zodiac_3_rat", "marksix_link_zodiac_3_ox", "marksix_link_zodiac_3_tiger"}},
		{"marksix_link_tail_2", "0尾,9尾", []string{"marksix_link_tail_2_0", "marksix_link_tail_2_9"}},
	} {
		got, linked, err := markSixLinkedPricingCandidates(markSixRuleVersion, test.playCode, test.selection)
		if err != nil || !linked || !reflect.DeepEqual(got, test.want) {
			t.Fatalf("%s candidates=%v linked=%v err=%v", test.playCode, got, linked, err)
		}
	}
	game := &lottery.Game{ID: "bingo-mark-six"}
	for _, pricingCode := range []string{"marksix_link_zodiac_3_rat", "marksix_link_tail_2_0", "marksix_combo_3_2_exact2"} {
		if _, _, err := InferPlayForGame(game, pricingCode, "", 0, "鼠,牛,虎"); err == nil {
			t.Fatalf("internal pricing code accepted as public bet: %s", pricingCode)
		}
	}
}

func TestBingoMarkSixCompositeTicketsFreezeBothPayoutTiersAndChargeOnce(t *testing.T) {
	gameID := "bingo-mark-six"
	drawAt := time.Date(2026, 3, 1, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	threeTerms, err := encodeMarkSixOddsTerms("marksix_combo_3_2", 125)
	if err != nil {
		t.Fatal(err)
	}
	three := bet.Bet{
		GameID: gameID, RuleVersion: markSixRuleVersion, PlayCode: "marksix_combo_3_2", Selection: "1,2,3",
		AmountCents: 1000, Odds: 20.1, OddsTerms: threeTerms,
	}
	for _, test := range []struct {
		draw   []int
		want   markSixBetOutcome
		odds   float64
		policy string
	}{
		{[]int{1, 2, 8, 9, 10, 11, 49}, markSixOutcomeWon, 20.1, "marksix_combo_3_2_exact2"},
		{[]int{1, 2, 3, 9, 10, 11, 49}, markSixOutcomeWon, 125, "marksix_combo_3_2_exact3"},
		{[]int{1, 8, 9, 10, 11, 12, 49}, markSixOutcomeLost, 20.1, "marksix_standard"},
	} {
		decision, decisionErr := decideMarkSixSettlement(gameID, three, test.draw, drawAt)
		if decisionErr != nil || decision.Outcome != test.want || decision.EffectiveOdds != test.odds || decision.Policy != test.policy || decision.ValidTurnoverCents != three.AmountCents {
			t.Fatalf("three-tier decision=%+v want=%s/%.4f/%s err=%v", decision, test.want, test.odds, test.policy, decisionErr)
		}
	}

	twoTerms, err := encodeMarkSixOddsTerms("marksix_combo_2_special", 55)
	if err != nil {
		t.Fatal(err)
	}
	two := bet.Bet{
		GameID: gameID, RuleVersion: markSixRuleVersion, PlayCode: "marksix_combo_2_special", Selection: "1,49",
		AmountCents: 1000, Odds: 25, OddsTerms: twoTerms,
	}
	mixed, mixedErr := decideMarkSixSettlement(gameID, two, []int{1, 2, 3, 4, 5, 6, 49}, drawAt)
	if mixedErr != nil || mixed.Outcome != markSixOutcomeWon || mixed.EffectiveOdds != 25 || mixed.Policy != "marksix_combo_2_special_mixed" {
		t.Fatalf("mixed tier=%+v err=%v", mixed, mixedErr)
	}
	two.Selection = "1,2"
	regular, regularErr := decideMarkSixSettlement(gameID, two, []int{1, 2, 3, 4, 5, 6, 49}, drawAt)
	if regularErr != nil || regular.Outcome != markSixOutcomeWon || regular.EffectiveOdds != 55 || regular.Policy != "marksix_combo_2_special_regular" {
		t.Fatalf("regular tier=%+v err=%v", regular, regularErr)
	}
	three.OddsTerms = "{}"
	if _, missingErr := decideMarkSixSettlement(gameID, three, []int{1, 2, 3, 9, 10, 11, 49}, drawAt); apperrors.GetErrorCode(missingErr) != "RULES_NOT_READY" {
		t.Fatalf("missing alternate odds was not rejected: %v", missingErr)
	}
	three.OddsTerms = `{"version":2,"exact_three_odds":125}`
	if _, versionErr := decideMarkSixSettlement(gameID, three, []int{1, 2, 3, 9, 10, 11, 49}, drawAt); apperrors.GetErrorCode(versionErr) != "RULES_NOT_READY" {
		t.Fatalf("unknown odds-terms version was not rejected: %v", versionErr)
	}
}

func TestBingoMarkSixDecisionRejectsMalformedDrawBeforeIndexing(t *testing.T) {
	item := bet.Bet{
		GameID: "bingo-mark-six", RuleVersion: markSixRuleVersion, PlayCode: "marksix_not_in", Selection: "1,2,3,4,5",
		AmountCents: 1000, Odds: 2.1,
	}
	for _, numbers := range [][]int{
		{1, 2, 3, 4, 5, 6},
		{1, 2, 3, 4, 5, 6, 6},
		{1, 2, 3, 4, 5, 6, 50},
	} {
		if _, err := decideMarkSixSettlement(item.GameID, item, numbers, time.Now()); apperrors.GetErrorCode(err) != "INVALID_DRAW" {
			t.Fatalf("malformed draw %v returned %v", numbers, err)
		}
	}
}

func TestBingoMarkSixRegularZodiacMultipliesNetWinFromFrozenOdds(t *testing.T) {
	drawAt := time.Date(2026, 3, 1, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	item := bet.Bet{
		GameID: "bingo-mark-six", RuleVersion: markSixRuleVersion, PlayCode: "marksix_regular_zodiac_horse", Selection: "马",
		AmountCents: 10000, Odds: 1.95,
	}
	decision, err := decideMarkSixSettlement(item.GameID, item, []int{1, 13, 25, 2, 3, 4, 5}, drawAt)
	if err != nil || decision.Outcome != markSixOutcomeWon || decision.EffectiveOdds != 3.85 || decision.Policy != "marksix_regular_zodiac_3_hits" {
		t.Fatalf("regular zodiac decision=%+v err=%v", decision, err)
	}
}

func TestBingoMarkSixV2SevenColorWaveWinDrawAndPush(t *testing.T) {
	game := &lottery.Game{ID: "bingo-mark-six"}
	drawAt := time.Date(2026, 3, 1, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	regularWinner := []int{1, 2, 7, 3, 4, 5, 8}
	for _, test := range []struct {
		code, selection string
		want            markSixBetOutcome
	}{
		{"marksix_seven_color_red", "红波", markSixOutcomeWon},
		{"marksix_seven_color_blue", "蓝波", markSixOutcomeLost},
		{"marksix_seven_color_draw", "和局", markSixOutcomeLost},
	} {
		got, _, err := evaluateBetOutcomeForRuleVersionAt(game, markSixRuleVersion, regularWinner, test.code, 0, test.selection, drawAt)
		if err != nil || got != test.want {
			t.Fatalf("regular seven-color %s got=%s want=%s err=%v", test.code, got, test.want, err)
		}
	}

	// Three blue and three green regular balls tie for the lead while the
	// special ball is red: color bets push and the explicit draw option wins.
	drawResult := []int{3, 4, 9, 5, 6, 11, 1}
	for _, test := range []struct {
		code, selection string
		want            markSixBetOutcome
	}{
		{"marksix_seven_color_red", "红波", markSixOutcomePush},
		{"marksix_seven_color_blue", "蓝波", markSixOutcomePush},
		{"marksix_seven_color_green", "绿波", markSixOutcomePush},
		{"marksix_seven_color_draw", "和局", markSixOutcomeWon},
	} {
		got, _, err := evaluateBetOutcomeForRuleVersionAt(game, markSixRuleVersion, drawResult, test.code, 0, test.selection, drawAt)
		if err != nil || got != test.want {
			t.Fatalf("draw seven-color %s got=%s want=%s err=%v", test.code, got, test.want, err)
		}
	}
}

func TestBingoMarkSixSettlementLabelsNeverRenderZeroBall(t *testing.T) {
	for _, item := range []bet.Bet{
		{RuleVersion: markSixRuleVersion, PlayCode: "marksix_regular_number", PlayName: "伪造", Position: 0},
		{RuleVersion: markSixRuleVersion, PlayCode: "marksix_combo_2_all", PlayName: "伪造", Position: 0},
		{RuleVersion: markSixRuleVersion, PlayCode: "marksix_regular_position_number", Position: 3},
		{RuleVersion: markSixRuleVersion, PlayCode: "marksix_regular_position_big_small", Position: 4},
		{RuleVersion: markSixRuleVersion, PlayCode: "marksix_regular_color_green", Position: 6},
	} {
		item.GameID = "bingo-mark-six"
		label := settlementBetLabel(item)
		if strings.Contains(label, "第0") || label == "" {
			t.Fatalf("bad settlement label %q for %+v", label, item)
		}
	}
	if got := settlementBetLabel(bet.Bet{GameID: "bingo-mark-six", RuleVersion: markSixRuleVersion, PlayCode: "marksix_regular_position_big_small", Position: 4}); got != "正码1-6大小 第4位" {
		t.Fatalf("regular side label lost its position: %q", got)
	}
	if got := settlementBetLabel(bet.Bet{GameID: "bingo-mark-six", RuleVersion: markSixRuleVersion, PlayCode: "marksix_regular_color_green", Position: 6}); got != "正码1-6绿波 第6位" {
		t.Fatalf("regular color label lost its position: %q", got)
	}
}
