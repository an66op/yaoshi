package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBingoMarkSixV1DrawAndModeContract(t *testing.T) {
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

func TestBingoMarkSixV1IsImmutableAndV2CatalogMarksUnpricedAtoms(t *testing.T) {
	v1Codes := make([]string, 0, len(markSixV1Specs))
	for _, spec := range markSixV1Specs {
		v1Codes = append(v1Codes, spec.Play.Code)
	}
	wantV1 := []string{
		"marksix_special_a_number", "marksix_special_b_number", "marksix_regular_number",
		"marksix_regular_position_number", "marksix_regular_special_number",
		"marksix_combo_4_all", "marksix_combo_3_all", "marksix_combo_2_all",
		"marksix_combo_special_pair", "marksix_not_in",
	}
	if !reflect.DeepEqual(v1Codes, wantV1) {
		t.Fatalf("v1 changed: %v", v1Codes)
	}
	if _, ok := markSixSpecByCode(markSixLegacyRuleVersion, "marksix_special_big_small"); ok {
		t.Fatal("v2 play leaked into immutable v1")
	}
	catalog := PlayCatalogForGame("bingo-mark-six")
	if len(catalog) != len(markSixV2Specs) || len(markSixDefaultPlays) <= len(markSixV1Specs) {
		t.Fatalf("v2 catalog/defaults incomplete: catalog=%d priced=%d", len(catalog), len(markSixDefaultPlays))
	}
	seenUnpriced := false
	for _, item := range catalog {
		if item.DefaultOdds == 0 {
			seenUnpriced = true
			if !strings.Contains(item.Description, "赔率需后台") {
				t.Fatalf("unpriced atom lacks explicit warning: %+v", item)
			}
		} else if item.DefaultOdds <= 1 {
			t.Fatalf("invalid seeded odds: %+v", item)
		}
	}
	if !seenUnpriced {
		t.Fatal("v2 did not expose configurable atomic markets")
	}
	profile, _ := rulesForGame(&lottery.Game{ID: "bingo-mark-six"})
	for _, code := range []string{"marksix_combo_3_2", "marksix_combo_2_special", "marksix_one_zodiac", "marksix_total_zodiac", "marksix_seven_color_wave", "marksix_link_zodiac_2"} {
		if profile.supportsPlay(code) {
			t.Fatalf("complex unsupported play exposed: %s", code)
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
	} {
		if err := validateBetChoice(game, test.code, test.position, test.selection); err != nil {
			t.Fatalf("valid %+v rejected: %v", test, err)
		}
	}
	if got := normalizeBetSelection(game, "marksix_combo_4_all", "49，3, 2,1"); got != "1,2,3,49" {
		t.Fatalf("combination not canonicalized: %q", got)
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
	} {
		if err := validateBetChoice(game, test.code, test.position, test.selection); err == nil {
			t.Fatalf("invalid %+v accepted", test)
		}
	}
}

func TestBingoMarkSixV1SettlementTruthTable(t *testing.T) {
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
		got, _, err := evaluateBetForRuleVersion(game, markSixLegacyRuleVersion, draw, test.code, test.position, test.selection)
		if err != nil || got != test.want {
			t.Fatalf("v1 %+v got=%v err=%v", test, got, err)
		}
		gotV2, _, err := evaluateBetForRuleVersion(game, markSixRuleVersion, draw, test.code, test.position, test.selection)
		if err != nil || gotV2 != test.want {
			t.Fatalf("v2 compatibility %+v got=%v err=%v", test, gotV2, err)
		}
	}
	if won, _, err := evaluateBetForRuleVersion(game, markSixLegacyRuleVersion, draw, "marksix_special_a_number", 7, "49"); err != nil || !won {
		t.Fatalf("v1 historical ticket no longer settles: won=%v err=%v", won, err)
	}
	if _, _, err := evaluateBetForRuleVersion(game, markSixLegacyRuleVersion, draw, "marksix_special_big_small", 7, "大"); apperrors.GetErrorCode(err) != "INVALID_REQUEST" {
		t.Fatalf("v2 code leaked into v1: %v", err)
	}
	if _, _, err := evaluateBetForRuleVersion(game, "", draw, "marksix_special_a_number", 7, "49"); apperrors.GetErrorCode(err) != "RULES_NOT_READY" {
		t.Fatalf("legacy empty-version Mark Six ticket was reinterpreted: %v", err)
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
	beforeOutcome, _, beforeErr := evaluateBetOutcomeForRuleVersionAt(game, markSixRuleVersion, boundaryDraw, "marksix_special_zodiac", 7, "马", beforeNewYear)
	afterOutcome, _, afterErr := evaluateBetOutcomeForRuleVersionAt(game, markSixRuleVersion, boundaryDraw, "marksix_special_zodiac", 7, "马", afterNewYear)
	if beforeErr != nil || afterErr != nil || beforeOutcome != markSixOutcomeLost || afterOutcome != markSixOutcomeWon {
		t.Fatalf("draw timestamp did not freeze zodiac year: before=%s/%v after=%s/%v", beforeOutcome, beforeErr, afterOutcome, afterErr)
	}
	for _, test := range []struct {
		code, selection string
		position        int
	}{
		{"marksix_special_zodiac", "猪", 7},
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
	if _, _, err := evaluateBetOutcomeForRuleVersionAt(game, markSixRuleVersion, draw, "marksix_special_zodiac", 7, "猪", time.Time{}); apperrors.GetErrorCode(err) != "RULES_NOT_READY" {
		t.Fatalf("zodiac settled without draw time: %v", err)
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
		label := settlementBetLabel(item)
		if strings.Contains(label, "第0") || label == "" {
			t.Fatalf("bad settlement label %q for %+v", label, item)
		}
	}
	if got := settlementBetLabel(bet.Bet{RuleVersion: markSixRuleVersion, PlayCode: "marksix_regular_position_big_small", Position: 4}); got != "正码1-6大小 第4位" {
		t.Fatalf("regular side label lost its position: %q", got)
	}
	if got := settlementBetLabel(bet.Bet{RuleVersion: markSixRuleVersion, PlayCode: "marksix_regular_color_green", Position: 6}); got != "正码1-6绿波 第6位" {
		t.Fatalf("regular color label lost its position: %q", got)
	}
}
