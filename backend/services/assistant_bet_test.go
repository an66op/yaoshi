package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func TestParseAssistantBetCompactTicket(t *testing.T) {
	lines, err := ParseAssistantBet("买12345/1000#3大/2000#6/123456/100")
	if err != nil {
		t.Fatalf("ParseAssistantBet returned error: %v", err)
	}
	if len(lines) != 12 {
		t.Fatalf("expected 12 parsed lines, got %d", len(lines))
	}
	var total float64
	for _, line := range lines {
		total += line.Amount
	}
	if math.Abs(total-7600) > 0.00001 {
		t.Fatalf("expected exact ticket total 7600, got %.2f", total)
	}
	if lines[0].Position != 1 || lines[0].Selection != "1" || lines[0].Amount != 1000 {
		t.Fatalf("unexpected first compact line: %#v", lines[0])
	}
	if lines[5].Position != 3 || lines[5].Selection != "大" || lines[5].Amount != 2000 {
		t.Fatalf("unexpected side-bet line: %#v", lines[5])
	}
}

func TestAssistantRulesStatusDoesNotConflateRulesAndSourceHealth(t *testing.T) {
	for _, gameID := range []string{"hong-kong-mark-six", "unknown"} {
		status := &AssistantDrawStatus{Accepting: true, SourceHealthy: true, Issue: "123", IssueStatus: "accepting", NextDrawAt: time.Unix(123, 0), BettingWindow: &BettingWindow{}}
		applyAssistantRulesStatus(&lottery.Game{ID: gameID}, status)
		if status.Accepting || status.BettingWindow != nil || status.RulesReady || status.RuleVersion != "" || status.RulesMessage == "" {
			t.Fatalf("unknown rules still accepting: %+v", status)
		}
		if !status.SourceHealthy || status.Issue != "123" || status.IssueStatus != "accepting" || !status.NextDrawAt.Equal(time.Unix(123, 0)) {
			t.Fatalf("rules gate changed draw/source lifecycle: %+v", status)
		}
	}
	for gameID, version := range map[string]string{"pc-canada": pc28RuleV1, "canada-28": pc28RuleV2, "canada-20": pc28RuleV3} {
		status := &AssistantDrawStatus{Accepting: true, SourceHealthy: true}
		applyAssistantRulesStatus(&lottery.Game{ID: gameID}, status)
		if !status.Accepting || !status.RulesReady || status.RuleVersion != version || status.RulesMessage != "" {
			t.Fatalf("supported PC28 rules changed availability: %+v", status)
		}
	}
	for _, accepting := range []bool{true, false} {
		status := &AssistantDrawStatus{Accepting: accepting, SourceHealthy: true}
		applyAssistantRulesStatus(&lottery.Game{ID: "speed-ssc"}, status)
		if status.Accepting != accepting || !status.RulesReady || status.RuleVersion != "digits5-v3" || status.RulesMessage != "" {
			t.Fatalf("supported rules changed existing availability: %+v", status)
		}
	}
}

func TestCompletedAssistantRequestKeepsReceiptWithoutReparsing(t *testing.T) {
	for _, receipt := range []AssistantBetResult{
		{GameID: "pc-canada", Content: "1/2/20", Total: 20},
		{GameID: "speed-racing", Content: "冠亚/99/20", Total: 40},
		{GameID: "speed-racing", Content: "1/2/1.234", Total: 1.23},
		{GameID: "speed-ssc", RuleVersion: "digits5-v3", Content: "中三/豹子/20", Total: 20},
	} {
		payload, err := json.Marshal(receipt)
		if err != nil {
			t.Fatal(err)
		}
		service := NewBetAssistantService(nil)
		got, handled, err := service.resolveExistingAssistantRequest(nil, bet.AssistantRequest{Status: "completed", ResultJSON: string(payload)}, time.Now())
		if err != nil || !handled || got == nil || got.GameID != receipt.GameID || got.RuleVersion != receipt.RuleVersion || got.Content != receipt.Content || got.Total != receipt.Total {
			t.Fatalf("completed request was rejected/reinterpreted: got=%+v handled=%v err=%v", got, handled, err)
		}
	}
}

func TestAssistantRepeatContentPreservesExplicitDigitScopes(t *testing.T) {
	front := []AssistantBetLine{{Position: 1, Selection: "leopard", PlayCode: "leopard", PlayName: "前三豹子", Amount: 20}}
	content, err := AssistantRepeatContent("speed-ssc", front)
	if err != nil || content != "前三/豹子/20" {
		t.Fatalf("front shape was not made explicit: %q %v", content, err)
	}
	lines, err := parseAssistantBetForGame(&lottery.Game{ID: "speed-ssc"}, content)
	if err != nil || len(lines) != 1 || lines[0].Position != 1 || lines[0].PlayCode != "leopard" {
		t.Fatalf("front shape expanded into unrelated windows: %+v %v", lines, err)
	}

	v3 := []AssistantBetLine{
		{Position: 1, Selection: "pair", PlayCode: "pair", PlayName: "三段对子", Amount: 20},
		{Position: 2, Selection: "pair", PlayCode: "pair", PlayName: "三段对子", Amount: 20},
		{Position: 3, Selection: "pair", PlayCode: "pair", PlayName: "三段对子", Amount: 20},
	}
	content, err = AssistantRepeatContent("speed-ssc", v3)
	if err != nil || content != "前三/对子/20#中三/对子/20#后三/对子/20" {
		t.Fatalf("v3 shape windows were not preserved: %q %v", content, err)
	}
	lines, err = parseAssistantBetForGame(&lottery.Game{ID: "speed-ssc"}, content)
	if err != nil || len(lines) != 3 || lines[0].Position != 1 || lines[1].Position != 2 || lines[2].Position != 3 {
		t.Fatalf("v3 shape repeat changed windows: %+v %v", lines, err)
	}

	retired := []AssistantBetLine{{Position: 2, Selection: "龙", PlayCode: "dragon_tiger", PlayName: "龙虎", Amount: 20}}
	content, err = AssistantRepeatContent("speed-ssc", retired)
	if err != nil || content != "2/龙/20" {
		t.Fatalf("unsupported dragon position was moved before validation: %q %v", content, err)
	}
	if lines, err = parseAssistantBetForGame(&lottery.Game{ID: "speed-ssc"}, content); err == nil || lines != nil {
		t.Fatalf("retired second-vs-fourth dragon was silently accepted: %+v %v", lines, err)
	}
	if _, err := AssistantRepeatContent("speed-ssc", nil); err == nil {
		t.Fatal("history without authoritative accepted lines was reparsed")
	}
}

func TestParseAssistantBetAmountAppliesToEverySelection(t *testing.T) {
	lines, err := ParseAssistantBet("6/123456/100")
	if err != nil {
		t.Fatalf("ParseAssistantBet returned error: %v", err)
	}
	if len(lines) != 6 {
		t.Fatalf("expected six selections, got %d", len(lines))
	}
	var cents int64
	for _, line := range lines {
		cents += int64(math.Round(line.Amount * 100))
	}
	if cents != 60000 {
		t.Fatalf("expected six 100-point selections, got %d cents", cents)
	}
}

func TestParseAssistantBetMultipleRacingPositions(t *testing.T) {
	lines, err := ParseAssistantBet("1/12345/100#6/大/200#7/67890/100")
	if err != nil {
		t.Fatalf("ParseAssistantBet returned error: %v", err)
	}
	if len(lines) != 11 {
		t.Fatalf("expected eleven selections across three positions, got %d", len(lines))
	}
	var total float64
	positions := map[int]int{}
	for _, line := range lines {
		total += line.Amount
		positions[line.Position]++
	}
	if math.Abs(total-1200) > 0.00001 {
		t.Fatalf("expected exact ticket total 1200, got %.2f", total)
	}
	if positions[1] != 5 || positions[6] != 1 || positions[7] != 5 || len(positions) != 3 {
		t.Fatalf("expected 5/1/5 selections for positions 1/6/7, got %#v", positions)
	}
	if lines[0].Position != 1 || lines[0].Selection != "1" || lines[0].Amount != 100 {
		t.Fatalf("unexpected first-position line: %#v", lines[0])
	}
	if lines[5].Position != 6 || lines[5].Selection != "大" || lines[5].PlayCode != "two_sided" || lines[5].Amount != 200 {
		t.Fatalf("unexpected sixth-position side line: %#v", lines[5])
	}
	if lines[10].Position != 7 || lines[10].Selection != "0" || lines[10].Amount != 100 {
		t.Fatalf("unexpected seventh-position line: %#v", lines[10])
	}
}

func TestEvaluateRacingPositionsSixToTen(t *testing.T) {
	game := &lottery.Game{ID: "speed-racing"}
	numbers := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if won, reason, err := evaluateBetForRuleVersion(game, "racing-v2", numbers, "two_sided", 6, "大"); err != nil || !won {
		t.Fatalf("expected sixth position big to win, got %s, err=%v", reason, err)
	}
	if won, reason, err := evaluateBetForRuleVersion(game, "racing-v2", numbers, "ball_1_5", 7, "7"); err != nil || !won {
		t.Fatalf("expected seventh position number to win, got %s, err=%v", reason, err)
	}
	if won, reason, err := evaluateBetForRuleVersion(game, "racing-v2", numbers, "ball_1_5", 10, "10"); err != nil || !won {
		t.Fatalf("expected tenth position number to win, got %s, err=%v", reason, err)
	}
}

func TestParseAssistantBetRejectsInvalidSyntax(t *testing.T) {
	if _, err := ParseAssistantBet("冠军大"); err == nil {
		t.Fatal("expected missing amount to be rejected")
	}
}

func TestParseAssistantBetSumWithSeparators(t *testing.T) {
	lines, err := ParseAssistantBet("冠亚和/大/200")
	if err != nil {
		t.Fatalf("expected separated sum syntax to parse: %v", err)
	}
	if len(lines) != 1 || lines[0].PlayCode != "sum" || lines[0].Selection != "大" || lines[0].Amount != 200 {
		t.Fatalf("unexpected sum line: %#v", lines)
	}
}

func TestParseAssistantBetCrossesPositionsAndSelections(t *testing.T) {
	lines, err := ParseAssistantBet("34/大虎/236#489/0178/48")
	if err != nil {
		t.Fatalf("expected multi-position syntax to parse: %v", err)
	}
	if len(lines) != 16 {
		t.Fatalf("expected 16 selections, got %d", len(lines))
	}
	if lines[0].Position != 3 || lines[0].Selection != "大" || lines[0].Amount != 236 {
		t.Fatalf("unexpected first crossed selection: %#v", lines[0])
	}
	if lines[15].Position != 9 || lines[15].Selection != "8" || lines[15].Amount != 48 {
		t.Fatalf("unexpected final crossed selection: %#v", lines[15])
	}
}

func TestParseAssistantBetDefaultChampionAndDuplicateWeights(t *testing.T) {
	lines, err := ParseAssistantBet("800/7")
	if err != nil {
		t.Fatalf("expected champion shorthand to parse: %v", err)
	}
	if len(lines) != 2 || lines[0].Position != 1 || lines[0].Selection != "8" || lines[0].Amount != 7 || lines[1].Selection != "0" || lines[1].Amount != 14 {
		t.Fatalf("unexpected duplicate selection merge: %#v", lines)
	}
}

func TestParseAssistantBetReferenceRoomSyntax(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantLines  int
		wantTotal  float64
		wantPos    int
		wantSelect string
		wantCode   string
	}{
		{name: "crown sum number", content: "冠亚/9/9", wantLines: 1, wantTotal: 9, wantPos: 6, wantSelect: "9", wantCode: "sum"},
		{name: "two digit crown sum", content: "冠亚/14/9", wantLines: 1, wantTotal: 9, wantPos: 6, wantSelect: "14", wantCode: "sum"},
		{name: "crown sum side", content: "冠亚/小/96", wantLines: 1, wantTotal: 96, wantPos: 6, wantSelect: "小", wantCode: "sum"},
		{name: "tenth place", content: "0/38/12", wantLines: 2, wantTotal: 24, wantPos: 10, wantSelect: "3", wantCode: "ball_1_5"},
		{name: "default champion", content: "1234578/100", wantLines: 7, wantTotal: 700, wantPos: 1, wantSelect: "1", wantCode: "ball_1_5"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lines, err := ParseAssistantBet(test.content)
			if err != nil {
				t.Fatalf("ParseAssistantBet(%q) returned error: %v", test.content, err)
			}
			if len(lines) != test.wantLines {
				t.Fatalf("expected %d lines, got %#v", test.wantLines, lines)
			}
			var total float64
			for _, line := range lines {
				total += line.Amount
			}
			if math.Abs(total-test.wantTotal) > 0.00001 {
				t.Fatalf("expected total %.2f, got %.2f", test.wantTotal, total)
			}
			if lines[0].Position != test.wantPos || lines[0].Selection != test.wantSelect || lines[0].PlayCode != test.wantCode {
				t.Fatalf("unexpected first line: %#v", lines[0])
			}
		})
	}
}

func TestNormalizeAssistantAllInReferenceSyntax(t *testing.T) {
	for _, content := range []string{"大梭哈", "大单梭哈", "12345/梭哈"} {
		normalized, allIn, err := normalizeAssistantAllIn(content)
		if err != nil || !allIn {
			t.Fatalf("expected %q to be recognized as all-in: normalized=%q allIn=%v err=%v", content, normalized, allIn, err)
		}
		if _, err := ParseAssistantBet(normalized); err != nil {
			t.Fatalf("normalized %q should parse: %v", normalized, err)
		}
	}
	if _, _, err := normalizeAssistantAllIn("123梭哈"); err == nil {
		t.Fatal("multi-number shorthand without amount slash must be rejected")
	}
	if _, _, err := normalizeAssistantAllIn("1/1/20#2/2/梭哈"); err == nil {
		t.Fatal("all-in must not be mixed with an explicitly priced segment")
	}
}

func TestAllInAmountsPreserveDuplicateWeights(t *testing.T) {
	lines, err := ParseAssistantBet("800/1")
	if err != nil {
		t.Fatal(err)
	}
	if !applyAllInAmounts(lines, 2100) {
		t.Fatal("expected whole-point all-in allocation")
	}
	if lines[0].Amount != 7 || lines[1].Amount != 14 {
		t.Fatalf("expected 7/14 weighted all-in split, got %#v", lines)
	}
}

func TestAllInAmountsUseEqualWholePointsAndLeaveRemainder(t *testing.T) {
	three, err := ParseAssistantBet("1/123/1")
	if err != nil || !applyAllInAmounts(three, 10000) {
		t.Fatalf("three-way all-in failed: %+v %v", three, err)
	}
	for _, line := range three {
		if line.Amount != 33 {
			t.Fatalf("100 points over three selections must be 33 each, got %+v", three)
		}
	}
	two, err := ParseAssistantBet("大单/1")
	if err != nil || !applyAllInAmounts(two, 10050) {
		t.Fatalf("two-way all-in failed: %+v %v", two, err)
	}
	if two[0].Amount != 50 || two[1].Amount != 50 {
		t.Fatalf("100.50 points over two selections must leave .50, got %+v", two)
	}
	if applyAllInAmounts(three, 250) {
		t.Fatal("less than one whole point per selection must be rejected")
	}
}

func TestDigits5V3AllInExpandsBeforeEqualAllocation(t *testing.T) {
	for _, gameID := range []string{"speed-ssc", "au-lucky-5", "bingo-ssc-1"} {
		game := &lottery.Game{ID: gameID}
		for _, test := range []struct {
			content     string
			balance     int64
			wantLines   int
			wantPerLine float64
		}{
			{content: "大梭哈", balance: 10050, wantLines: 5, wantPerLine: 20},
			{content: "1/123/梭哈", balance: 10000, wantLines: 3, wantPerLine: 33},
		} {
			normalized, allIn, err := normalizeAssistantAllIn(test.content)
			if err != nil || !allIn {
				t.Fatalf("%s %q normalization: %q/%v/%v", gameID, test.content, normalized, allIn, err)
			}
			lines, err := parseAssistantBetForGame(game, normalized)
			if err != nil || len(lines) != test.wantLines || !applyAllInAmounts(lines, test.balance) {
				t.Fatalf("%s %q allocation: %+v %v", gameID, test.content, lines, err)
			}
			for _, line := range lines {
				if line.Amount != test.wantPerLine {
					t.Fatalf("%s %q line=%+v want %.2f", gameID, test.content, line, test.wantPerLine)
				}
			}
		}
	}
}

func TestParseAssistantBetDocumentedRacingAliases(t *testing.T) {
	game := &lottery.Game{ID: "speed-racing"}
	tests := []struct {
		input      string
		positions  []int
		selections []string
	}{
		{"3/大/5", []int{3}, []string{"大"}},
		{"123大/5", []int{1, 1, 1, 1}, []string{"1", "2", "3", "大"}},
		{"1大5", []int{1}, []string{"大"}},
		{"和/大/5", []int{6}, []string{"大"}},
		{"和/345/5", []int{6, 6, 6}, []string{"3", "4", "5"}},
		{"和345/5", []int{6, 6, 6}, []string{"3", "4", "5"}},
		{"0/1/5", []int{10}, []string{"1"}},
		{"10大5", []int{1, 10}, []string{"大", "大"}},
		{"前三/2/5", []int{1, 2, 3}, []string{"2", "2", "2"}},
		{"后三/2/5", []int{8, 9, 10}, []string{"2", "2", "2"}},
		{"前五/2/5", []int{1, 2, 3, 4, 5}, []string{"2", "2", "2", "2", "2"}},
		{"后五/2/5", []int{6, 7, 8, 9, 10}, []string{"2", "2", "2", "2", "2"}},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			lines, err := parseAssistantBetForGame(game, test.input)
			if err != nil {
				t.Fatal(err)
			}
			if len(lines) != len(test.positions) {
				t.Fatalf("got %+v", lines)
			}
			for index, line := range lines {
				if line.Position != test.positions[index] || line.Selection != test.selections[index] || line.Amount != 5 {
					t.Fatalf("line %d = %+v", index, line)
				}
			}
		})
	}
	if lines, err := parseAssistantBetForGame(game, "1235"); err == nil || lines != nil {
		t.Fatalf("pure numeric compact text must remain ambiguous and rejected: %+v %v", lines, err)
	}
}

func TestParseAssistantBetFiveDigitUnpositionedSelectionsUseAllBalls(t *testing.T) {
	for _, gameID := range []string{"speed-ssc", "au-lucky-5", "bingo-ssc-1"} {
		game := &lottery.Game{ID: gameID}
		lines, err := parseAssistantBetForGame(game, "大/20#12/5")
		if err != nil {
			t.Fatal(err)
		}
		// 大/20 -> 5 and 12/5 -> 10.
		if len(lines) != 15 {
			t.Fatalf("%s got %d lines: %+v", gameID, len(lines), lines)
		}
		for index := 0; index < 5; index++ {
			if lines[index].Position != index+1 || lines[index].Selection != "大" || lines[index].Amount != 20 {
				t.Fatalf("%s unpositioned side line %d = %+v", gameID, index, lines[index])
			}
		}
		for position := 1; position <= 5; position++ {
			for offset, selection := range []string{"1", "2"} {
				line := lines[5+(position-1)*2+offset]
				if line.Position != position || line.Selection != selection || line.Amount != 5 || line.PlayCode != "ball_1_5" {
					t.Fatalf("%s unpositioned number line = %+v", gameID, line)
				}
			}
		}
		compact, err := parseAssistantBetForGame(game, "1大5")
		if err != nil || len(compact) != 1 || compact[0].Position != 1 || compact[0].Selection != "大" || compact[0].Amount != 5 {
			t.Fatalf("%s compact explicit first ball = %+v, %v", gameID, compact, err)
		}
	}
}

func TestEvaluateRacingDragonTigerAndCrownSum(t *testing.T) {
	game := &lottery.Game{ID: "speed-racing"}
	numbers := []int{4, 10, 1, 6, 7, 8, 2, 3, 9, 5}
	if won, reason, err := evaluateBetForRuleVersion(game, "racing-v2", numbers, "dragon_tiger", 1, "虎"); err != nil || !won {
		t.Fatalf("expected champion tiger (4 < 5), got %s, err=%v", reason, err)
	}
	if won, reason, err := evaluateBetForRuleVersion(game, "racing-v2", numbers, "dragon_tiger", 2, "龙"); err != nil || !won {
		t.Fatalf("expected runner-up dragon (10 > 9), got %s, err=%v", reason, err)
	}
	if won, reason, err := evaluateBetForRuleVersion(game, "racing-v2", numbers, "sum", 6, "14"); err != nil || !won {
		t.Fatalf("expected exact crown sum 14, got %s, err=%v", reason, err)
	}
	if won, reason, err := evaluateBetForRuleVersion(game, "racing-v2", numbers, "sum", 6, "4"); err != nil || won {
		t.Fatalf("racing crown sum must not settle by tail: %s, err=%v", reason, err)
	}
}

func TestValidateRacingBetChoice(t *testing.T) {
	game := &lottery.Game{ID: "speed-racing", Name: "极速赛车", Category: "赛车"}
	valid := []struct {
		playCode  string
		position  int
		selection string
	}{
		{"ball_1_5", 10, "10"},
		{"two_sided", 6, "大"},
		{"dragon_tiger", 5, "虎"},
		{"sum", 6, "3"},
		{"sum", 6, "19"},
	}
	for _, item := range valid {
		if err := validateBetChoice(game, item.playCode, item.position, item.selection); err != nil {
			t.Fatalf("expected valid choice %#v, got %v", item, err)
		}
	}
	if normalized := normalizeBetSelection(game, "ball_1_5", "0"); normalized != "10" {
		t.Fatalf("racing zero alias normalized to %q, want 10", normalized)
	} else if err := validateBetChoice(game, "ball_1_5", 10, normalized); err != nil {
		t.Fatalf("normalized racing number 10 must be accepted: %v", err)
	}
	invalid := []struct {
		playCode  string
		position  int
		selection string
	}{
		{"ball_1_5", 1, "11"},
		{"two_sided", 1, "龙"},
		{"dragon_tiger", 6, "龙"},
		{"dragon_tiger", 1, "大"},
		{"sum", 6, "2"},
		{"sum", 6, "20"},
		{"leopard", 1, "中"},
	}
	for _, item := range invalid {
		if err := validateBetChoice(game, item.playCode, item.position, item.selection); err == nil {
			t.Fatalf("expected invalid choice %#v to be rejected", item)
		}
	}
}

func TestParseAssistantBetRejectsOutOfRangeCrownSumsWithoutSplitting(t *testing.T) {
	for _, content := range []string{
		"冠亚/99/20", "冠亚/34/20", "冠亚/2/20", "冠亚/20/20", "冠亚/0/20",
		"冠亚/99999999999999999999999999/20", "冠亚/大14/20", "冠亚/3大/20",
		"1/123/20#冠亚/99/20",
	} {
		t.Run(content, func(t *testing.T) {
			if lines, err := ParseAssistantBet(content); err == nil || lines != nil {
				t.Fatalf("invalid sum must reject the whole ticket: %+v %v", lines, err)
			}
		})
	}
	for _, content := range []string{"冠亚/3/20", "冠亚/19/20", "冠亚/14/20", "冠亚/3/20#冠亚/4/20", "冠亚/大小单双/20"} {
		if lines, err := ParseAssistantBet(content); err != nil || len(lines) == 0 {
			t.Fatalf("valid sum rejected: %q %+v %v", content, lines, err)
		}
	}
}

func TestAssistantMoneyCentsRejectsExtraPrecisionAndNonDecimalForms(t *testing.T) {
	for _, amount := range []string{"1.234", "1.230", "0.001", "0.009", "1e3", "1E3", "+1", "-1", ".5", "1.", "NaN", "Inf", "1,000", "0", "0.00", "92233720368547758.07", "999999999999999999999999999999999999"} {
		t.Run(amount, func(t *testing.T) {
			if _, err := assistantMoneyCents(amount); err == nil {
				t.Fatalf("amount %q should be rejected, never rounded", amount)
			}
			if lines, err := ParseAssistantBet("1/2/20#2/3/" + amount); err == nil || lines != nil {
				t.Fatalf("invalid amount partially parsed: %+v %v", lines, err)
			}
		})
	}
	for amount, want := range map[string]int64{"0.01": 1, "0.10": 10, "1": 100, "1.2": 120, "1.23": 123, "20.00": 2000, "001.05": 105, "12.01": 1201, "999999.99": 99999999} {
		if got, err := assistantMoneyCents(amount); err != nil || got != want {
			t.Fatalf("amount %q: got %d, want %d, err=%v", amount, got, want, err)
		}
	}
}

func TestParseAssistantBetForEveryRacingGame(t *testing.T) {
	for _, gameID := range []string{"speed-racing", "speed-fly", "sg-fly", "fly-racing", "au-lucky-10", "bingo-racing-a"} {
		t.Run(gameID, func(t *testing.T) {
			game := &lottery.Game{ID: gameID}
			lines, err := parseAssistantBetForGame(game, "0/0/1.25#6/大/20#冠亚/14/9#2/大小单双12/20")
			if err != nil || len(lines) != 9 {
				t.Fatalf("racing parser: %+v %v", lines, err)
			}
			if lines[0].Position != 10 || lines[0].Selection != "0" || lines[1].Position != 6 || lines[1].PlayCode != "two_sided" || lines[2].PlayCode != "sum" || lines[2].Selection != "14" {
				t.Fatalf("rank/zero/sum meanings changed: %+v", lines)
			}
			for _, input := range []string{"豹子/20", "前三/豹子/20", "总和/大/20", "总和尾/9/20", "6/龙/20", "0龙/20", "冠亚/99/20"} {
				if _, err := parseAssistantBetForGame(game, input); err == nil {
					t.Fatalf("racing accepted unsupported input %q", input)
				}
			}
		})
	}
}

func TestParseAssistantBetForEveryFiveDigitGame(t *testing.T) {
	for _, gameID := range []string{"speed-ssc", "sg-ssc", "au-lucky-5", "bingo-ssc-1"} {
		t.Run(gameID, func(t *testing.T) {
			game := &lottery.Game{ID: gameID}
			lines, err := parseAssistantBetForGame(game, "1/0/20#5/大小单双12/1.25#前三/豹子/20")
			if err != nil || len(lines) != 8 {
				t.Fatalf("five-ball parser: %+v %v", lines, err)
			}
			if lines[0].Selection != "0" || lines[0].Label != "第1球[0/20]" || lines[7].PlayCode != "leopard" {
				t.Fatalf("five-ball meaning/labels incorrect: %+v", lines)
			}
			unsupported := []string{"冠亚/14/20", "冠亚和大/20", "冠亚/大/20", "0/1/20", "6/大/20", "10/1/20", "冠军/1/20", "总和尾/大/20", "总和/14/20", "总和尾/99/20", "总和/7大/20"}
			unsupported = append(unsupported, "总和/大/20", "总和尾/7/20", "总和大20", "总和尾7/20")
			for _, input := range unsupported {
				if _, err := parseAssistantBetForGame(game, input); err == nil {
					t.Fatalf("five-ball accepted unsupported input %q", input)
				}
			}
		})
	}
}

func TestParseAssistantBetThreeDigitTotalsAndFrontShapes(t *testing.T) {
	game := &lottery.Game{ID: "official-fc3d"}
	for _, input := range []string{"总和/0/20", "总和尾0/20", "总和大/20", "总和/大小单双/20", "第1球/09/20", "第3球9/20"} {
		if lines, err := parseAssistantBetForGame(game, input); err != nil || len(lines) == 0 {
			t.Fatalf("valid digit input %q: %+v %v", input, lines, err)
		}
	}
	for name, code := range map[string]string{"豹子": "leopard", "顺子": "straight", "对子": "pair", "半顺": "half_straight", "杂六": "mixed"} {
		for _, input := range []string{"前三/" + name + "/20", "前三" + name + "/20"} {
			lines, err := parseAssistantBetForGame(game, input)
			if err != nil || len(lines) != 1 || lines[0].Position != 1 || lines[0].PlayCode != code || lines[0].Selection != code || lines[0].Label != "前三["+name+"/20]" {
				t.Fatalf("shape input %q: %+v %v", input, lines, err)
			}
		}
		lines, err := parseAssistantBetForGame(game, name+"/20")
		if err != nil || len(lines) != 1 {
			t.Fatalf("unscoped shape %q: %+v %v", name, lines, err)
		}
		if lines[0].Position != 1 || lines[0].PlayCode != code || lines[0].Label != "前三["+name+"/20]" {
			t.Fatalf("three-digit unscoped shape %q: %+v", name, lines[0])
		}
		compact, compactErr := parseAssistantBetForGame(game, name+"5")
		if compactErr != nil || len(compact) != 1 {
			t.Fatalf("compact unscoped shape %q: %+v %v", name, compact, compactErr)
		}
	}
}

func TestParseAssistantBetV3MiddleBackAndFirstLastTie(t *testing.T) {
	for _, gameID := range []string{"speed-ssc", "sg-ssc", "au-lucky-5", "bingo-ssc-1"} {
		game := &lottery.Game{ID: gameID}
		lines, err := parseAssistantBetForGame(game, "中三顺子/5#后三/对子/5#1/龙虎和/5")
		if err != nil || len(lines) != 5 {
			t.Fatalf("%s v3 parser: %+v %v", gameID, lines, err)
		}
		if lines[0].Position != 2 || lines[0].PlayCode != "straight" || lines[0].Label != "中三[顺子/5]" ||
			lines[1].Position != 3 || lines[1].PlayCode != "pair" || lines[1].Label != "后三[对子/5]" {
			t.Fatalf("%s shape scopes changed: %+v", gameID, lines)
		}
		for index, want := range []struct{ code, selection string }{
			{"dragon_tiger", "龙"}, {"dragon_tiger", "虎"}, {"dragon_tiger_tie", "和"},
		} {
			line := lines[index+2]
			if line.Position != 1 || line.PlayCode != want.code || line.Selection != want.selection {
				t.Fatalf("%s first/last outcome %d: %+v", gameID, index, line)
			}
		}
		if compact, compactErr := parseAssistantBetForGame(game, "1和5"); compactErr != nil || len(compact) != 1 || compact[0].PlayCode != "dragon_tiger_tie" {
			t.Fatalf("%s compact tie: %+v %v", gameID, compact, compactErr)
		}
	}
	for _, gameID := range []string{"bingo-ssc-2", "bingo-ssc-3", "bingo-ssc-4"} {
		for _, input := range []string{"中三顺子/5", "后三/对子/5", "豹子/5", "1/和/5"} {
			lines, err := parseAssistantBetForGame(&lottery.Game{ID: gameID}, input)
			if err == nil {
				t.Fatalf("%s accepted v3 syntax %q: %+v", gameID, input, lines)
			}
		}
	}
}

func TestParseAssistantBetThreeDigitProfilesAndUnknownGames(t *testing.T) {
	for _, gameID := range []string{"official-fc3d", "official-pl3"} {
		game := &lottery.Game{ID: gameID}
		lines, err := parseAssistantBetForGame(game, "3/09/20#总和/单/20#总和尾/7/20#对子/20")
		if err != nil || len(lines) != 5 || lines[0].Label != "第3球[0/20]" {
			t.Fatalf("three-ball parser %s: %+v %v", gameID, lines, err)
		}
		for _, input := range []string{"4/1/20", "第4球/1/20", "冠亚/7/20", "2/龙/20", "后三/对子/20"} {
			if _, err := parseAssistantBetForGame(game, input); err == nil {
				t.Fatalf("three-ball accepted %q", input)
			}
		}
	}
	for _, game := range []*lottery.Game{nil, {ID: "hong-kong-mark-six"}, {ID: "unknown", Name: "极速赛车", Category: "赛车"}, {Name: "极速赛车", Category: "赛车"}} {
		if lines, err := parseAssistantBetForGame(game, "1/2/20"); err == nil || lines != nil || !strings.Contains(err.Error(), "尚未配置完整玩法") {
			t.Fatalf("unknown game must fail closed: game=%+v lines=%+v err=%v", game, lines, err)
		}
	}
}
