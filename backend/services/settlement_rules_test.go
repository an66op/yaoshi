package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"strconv"
	"strings"
	"testing"
)

func TestVersionedDigitSumExhaustive(t *testing.T) {
	for _, test := range []struct {
		gameID, version string
		count, bigFrom  int
	}{
		{"speed-ssc", "digits5-v2", 5, 23},
		{"official-fc3d", "digits3-v2", 3, 14},
	} {
		t.Run(test.version, func(t *testing.T) {
			game := &lottery.Game{ID: test.gameID}
			limit := 1
			for i := 0; i < test.count; i++ {
				limit *= 10
			}
			for value := 0; value < limit; value++ {
				balls := make([]int, test.count)
				remaining, total := value, 0
				for i := range balls {
					balls[i] = remaining % 10
					remaining /= 10
					total += balls[i]
				}
				for selection, want := range map[string]bool{
					"大": total >= test.bigFrom, "小": total < test.bigFrom,
					"单": total%2 != 0, "双": total%2 == 0,
					strconv.Itoa(total % 10):       true,
					strconv.Itoa((total + 1) % 10): false,
				} {
					won, reason, err := evaluateBetForRuleVersion(game, test.version, balls, "sum", 6, selection)
					if err != nil || won != want {
						t.Fatalf("draw=%v sum=%d selection=%s won=%v want=%v reason=%s err=%v", balls, total, selection, won, want, reason, err)
					}
				}
			}
		})
	}
}

func TestVersionedRacingSumExhaustiveOrderedPairs(t *testing.T) {
	game := &lottery.Game{ID: "speed-racing", Name: "renamed digits", Category: "时时彩"}
	for first := 1; first <= 10; first++ {
		for second := 1; second <= 10; second++ {
			if first == second {
				continue
			}
			balls := []int{first, second}
			for number := 1; number <= 10; number++ {
				if number != first && number != second {
					balls = append(balls, number)
				}
			}
			total := first + second
			selections := map[string]bool{"大": total >= 12, "小": total < 12, "单": total%2 != 0, "双": total%2 == 0}
			for selected := 3; selected <= 19; selected++ {
				selections[strconv.Itoa(selected)] = selected == total
			}
			for selection, want := range selections {
				won, reason, err := evaluateBetForRuleVersion(game, "racing-v2", balls, "sum", 6, selection)
				if err != nil || won != want {
					t.Fatalf("draw=%v selection=%s won=%v want=%v reason=%s err=%v", balls, selection, won, want, reason, err)
				}
			}
		}
	}
}

func TestVersionedSettlementKeepsLegacySumContract(t *testing.T) {
	game := &lottery.Game{ID: "speed-ssc"}
	balls := []int{9, 9, 9, 9, 9}
	for _, test := range []struct {
		version, selection string
		want               bool
	}{
		{"", "大", false}, {"", "小", true}, {"", "8", true}, {"", "5", false},
		{"digits5-v2", "大", true}, {"digits5-v2", "小", false}, {"digits5-v2", "8", false}, {"digits5-v2", "5", true},
	} {
		won, reason, err := evaluateBetForRuleVersion(game, test.version, balls, "sum", 6, test.selection)
		if err != nil || won != test.want {
			t.Fatalf("version=%q selection=%s won=%v reason=%s err=%v", test.version, test.selection, won, reason, err)
		}
	}
	// Legacy position 6 ball-number tickets retain their old full-sum tail;
	// the same noncanonical ticket must not silently enter the new contract.
	if won, _, err := evaluateBetForRuleVersion(game, "", balls, "ball_1_5", 6, "5"); err != nil || !won {
		t.Fatalf("legacy sixth-slot tail changed: won=%v err=%v", won, err)
	}
	if _, _, err := evaluateBetForRuleVersion(game, "digits5-v2", balls, "ball_1_5", 6, "5"); err == nil {
		t.Fatal("new digits ticket accepted a legacy noncanonical sixth slot")
	}
}

func TestVersionedSettlementRejectsUnknownRulesAndMalformedDraws(t *testing.T) {
	twenty := make([]int, 20)
	for i := range twenty {
		twenty[i] = i + 1
	}
	for _, test := range []struct {
		name, gameID, version, code string
		balls                       []int
	}{
		{"unknown version", "speed-ssc", "digits5-v99", "RULES_NOT_READY", []int{1, 2, 3, 4, 5}},
		{"wrong game version", "speed-ssc", "racing-v2", "RULES_NOT_READY", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
		{"legacy keno", "official-kl8", "", "RULES_NOT_READY", twenty},
		{"legacy bingo", "official-tw-bingo", "", "RULES_NOT_READY", twenty},
		{"bingo cannot claim racing", "official-tw-bingo", "racing-v2", "RULES_NOT_READY", twenty},
		{"legacy mark six", "hong-kong-mark-six", "", "RULES_NOT_READY", []int{1, 2, 3, 4, 5, 6, 7}},
		{"unmodelled named racing", "unknown-racing", "racing-v2", "RULES_NOT_READY", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
		{"legacy twenty is not racing", "speed-racing", "", "INVALID_DRAW", twenty},
		{"new twenty is not racing", "speed-racing", "racing-v2", "INVALID_DRAW", twenty},
		{"legacy incomplete racing", "speed-racing", "", "INVALID_DRAW", []int{1, 2, 3, 4, 5}},
		{"duplicate racing", "speed-racing", "racing-v2", "INVALID_DRAW", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 9}},
		{"out of range digit", "speed-ssc", "digits5-v2", "INVALID_DRAW", []int{1, 2, 3, 4, 10}},
		{"negative digit", "official-fc3d", "digits3-v2", "INVALID_DRAW", []int{1, -1, 3}},
		{"legacy wrong digit count", "official-fc3d", "", "INVALID_DRAW", []int{1, 2, 3, 4, 5}},
	} {
		t.Run(test.name, func(t *testing.T) {
			won, reason, err := evaluateBetForRuleVersion(&lottery.Game{ID: test.gameID, Name: "极速赛车", Category: "赛车"}, test.version, test.balls, "sum", 6, "小")
			if err == nil || apperrors.GetErrorCode(err) != test.code || won || reason != "" {
				t.Fatalf("won=%v reason=%q err=%v code=%s, want error %s without a financial outcome", won, reason, err, apperrors.GetErrorCode(err), test.code)
			}
		})
	}
}

func TestVersionedSettlementRejectsInvalidChoicesWithoutLosingResult(t *testing.T) {
	game := &lottery.Game{ID: "speed-ssc"}
	for _, test := range []struct {
		code, selection string
		position        int
	}{
		{"ball_1_5", "1", 0}, {"ball_1_5", "10", 1}, {"sum", "9", 1},
		{"sum", "45", 6}, {"two_sided", "龙", 1}, {"dragon_tiger", "虎", 3},
		{"pair", "pair", 2}, {"leopard", "pair", 1}, {"unknown", "1", 1},
	} {
		won, reason, err := evaluateBetForRuleVersion(game, "digits5-v2", []int{1, 2, 3, 4, 5}, test.code, test.position, test.selection)
		if err == nil || won || reason != "" {
			t.Fatalf("%+v produced outcome won=%v reason=%q err=%v", test, won, reason, err)
		}
	}
}

func TestVersionedAndLegacyRacingFiveLossDescriptionUsesSmall(t *testing.T) {
	balls := []int{5, 1, 2, 3, 4, 6, 7, 8, 9, 10}
	for _, version := range []string{"", "racing-v2"} {
		won, reason, err := evaluateBetForRuleVersion(&lottery.Game{ID: "speed-racing"}, version, balls, "two_sided", 1, "大")
		if err != nil || won || !strings.Contains(reason, "5(小/单)") {
			t.Fatalf("version=%q won=%v reason=%s err=%v", version, won, reason, err)
		}
	}
}

func TestVersionedFrontPatternsPartitionAllDigitTriples(t *testing.T) {
	game := &lottery.Game{ID: "official-fc3d"}
	for encoded := 0; encoded < 1000; encoded++ {
		balls := []int{encoded / 100, encoded / 10 % 10, encoded % 10}
		wins := 0
		for _, play := range []string{"leopard", "straight", "pair", "half_straight", "mixed"} {
			won, reason, err := evaluateBetForRuleVersion(game, "digits3-v2", balls, play, 1, play)
			if err != nil {
				t.Fatalf("draw=%v play=%s reason=%s err=%v", balls, play, reason, err)
			}
			if won {
				wins++
			}
		}
		if wins != 1 {
			t.Fatalf("draw=%v matches %d patterns, want exactly one", balls, wins)
		}
	}
}

func TestSettlementLabelsFreezeRuleVersionThroughRoomFormatting(t *testing.T) {
	for _, test := range []struct {
		version, code, selection, want string
		position                       int
	}{
		{"", "sum", "大", "冠亚和", 6},
		{"", "ball_1_5", "0", "冠军", 1},
		{"racing-v2", "sum", "12", "冠亚和", 6},
		{"racing-v2", "two_sided", "小", "亚军", 2},
		{"digits5-v2", "sum", "大", "总和", 6},
		{"digits5-v2", "sum", "5", "总和尾", 6},
		{"digits5-v2", "ball_1_5", "0", "第1球", 1},
		{"digits3-v2", "two_sided", "大", "第3球", 3},
		{"digits5-v2", "dragon_tiger", "龙", "第2球", 2},
		{"digits3-v2", "leopard", "leopard", "前三豹子", 1},
		{"digits5-v2", "pair", "pair", "前三对子", 1},
		{"digits5-v3", "leopard", "leopard", "前三豹子", 1},
		{"digits5-v3", "pair", "pair", "中三对子", 2},
		{"digits5-v3", "straight", "straight", "后三顺子", 3},
		{"digits5-v3", "dragon_tiger", "龙", "第1球龙虎", 1},
		{"digits5-v3", "dragon_tiger_tie", "和", "第1球龙虎和", 1},
		{markSixRuleVersion, "marksix_regular_number", "18", "正码", 0},
		{markSixRuleVersion, "marksix_combo_2_all", "1,2", "二全中", 0},
	} {
		item := bet.Bet{RuleVersion: test.version, PlayCode: test.code, Position: test.position, Selection: test.selection, PlayName: "old racing label"}
		label := settlementBetLabel(item)
		if label != test.want {
			t.Fatalf("version=%q code=%s label=%q want=%q", test.version, test.code, label, test.want)
		}
		message := formatRoomSettlement("测试彩种", "123", []roomSettlementPlayer{{
			nickname: "测试会员", stakeCents: 100,
			details: []NotificationBetDetail{{PlayCode: test.code, PlayName: label, Position: test.position, Selection: test.selection, Amount: 1}},
		}})
		if !strings.Contains(message, "\n"+test.want+" [") {
			t.Fatalf("room formatter overwrote frozen %q label: %s", test.want, message)
		}
	}
}

func TestUnversionedUpgradedDigits5LabelsKeepLegacyDisplayOnly(t *testing.T) {
	for _, gameID := range []string{"speed-ssc", "au-lucky-5", "bingo-ssc-1"} {
		for _, test := range []struct {
			code, storedName, selection, want string
			position                          int
		}{
			{code: "pair", storedName: "对子", selection: "pair", position: 1, want: "前三对子"},
			// Even malformed position metadata cannot turn an empty-version old
			// shape into a newly introduced middle-three market.
			{code: "straight", storedName: "顺子", selection: "straight", position: 2, want: "前三顺子"},
			{code: "dragon_tiger", storedName: "龙虎", selection: "虎", position: 2, want: "第2球"},
			{code: "ball_1_5", storedName: "指定球位号码", selection: "7", position: 5, want: "第5球"},
			{code: "two_sided", storedName: "两面", selection: "大", position: 3, want: "第3球"},
			{code: "sum", storedName: "旧总和", selection: "大", position: 6, want: "总和"},
			{code: "sum", storedName: "旧总和尾", selection: "7", position: 6, want: "总和尾"},
			// This code did not exist before v3. An empty-version row must use its
			// stored label instead of being silently promoted to the new market.
			{code: "dragon_tiger_tie", storedName: "历史未识别玩法", selection: "和", position: 1, want: "历史未识别玩法"},
		} {
			item := bet.Bet{
				GameID: gameID, RuleVersion: "", PlayCode: test.code, PlayName: test.storedName,
				Position: test.position, Selection: test.selection,
			}
			if got := settlementBetLabel(item); got != test.want {
				t.Fatalf("%s empty-version %s label=%q want=%q", gameID, test.code, got, test.want)
			}
			view := BetView{
				GameID: item.GameID, RuleVersion: item.RuleVersion, PlayCode: item.PlayCode,
				PlayName: item.PlayName, Position: item.Position, Selection: item.Selection,
			}
			if got := BetDisplayLabel(view); got != test.want {
				t.Fatalf("room query changed %s empty-version %s label=%q want=%q", gameID, test.code, got, test.want)
			}
			if item.RuleVersion != "" || view.RuleVersion != "" {
				t.Fatal("display compatibility backfilled a historical rule version")
			}
		}
	}

	// The bridge is intentionally scoped to the two upgraded games. An empty
	// snapshot from another family keeps the pre-existing generic fallback.
	if got := settlementBetLabel(bet.Bet{GameID: "speed-racing", PlayCode: "ball_1_5", Position: 1}); got != "冠军" {
		t.Fatalf("legacy racing label changed: %q", got)
	}
}
