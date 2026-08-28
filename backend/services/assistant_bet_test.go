package services

import (
	"backend/data/models/lottery"
	"math"
	"testing"
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
	numbers := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if won, reason := evaluateBet(numbers, "two_sided", 6, "大"); !won {
		t.Fatalf("expected sixth position big to win, got %s", reason)
	}
	if won, reason := evaluateBet(numbers, "ball_1_5", 7, "7"); !won {
		t.Fatalf("expected seventh position number to win, got %s", reason)
	}
	if won, reason := evaluateBet(numbers, "ball_1_5", 10, "10"); !won {
		t.Fatalf("expected tenth position number to win, got %s", reason)
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
	for _, content := range []string{"大梭哈", "12345/梭哈"} {
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
}

func TestAllInAmountsPreserveDuplicateWeights(t *testing.T) {
	lines, err := ParseAssistantBet("800/1")
	if err != nil {
		t.Fatal(err)
	}
	applyAllInAmounts(lines, 2100)
	if lines[0].Amount != 7 || lines[1].Amount != 14 {
		t.Fatalf("expected 7/14 weighted all-in split, got %#v", lines)
	}
}

func TestEvaluateRacingDragonTigerAndCrownSum(t *testing.T) {
	numbers := []int{4, 10, 1, 6, 7, 8, 2, 3, 9, 5}
	if won, reason := evaluateBet(numbers, "dragon_tiger", 1, "虎"); !won {
		t.Fatalf("expected champion tiger (4 < 5), got %s", reason)
	}
	if won, reason := evaluateBet(numbers, "dragon_tiger", 2, "龙"); !won {
		t.Fatalf("expected runner-up dragon (10 > 9), got %s", reason)
	}
	if won, reason := evaluateBet(numbers, "sum", 6, "14"); !won {
		t.Fatalf("expected exact crown sum 14, got %s", reason)
	}
	if won, reason := evaluateBet(numbers, "sum", 6, "4"); won {
		t.Fatalf("racing crown sum must not settle by tail: %s", reason)
	}
}

func TestValidateRacingBetChoice(t *testing.T) {
	game := &lottery.Game{Name: "极速赛车", Category: "赛车"}
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
