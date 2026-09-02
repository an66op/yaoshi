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

func TestSettlementRequiresExactRuleSnapshot(t *testing.T) {
	for _, gameID := range []string{"speed-racing", "speed-ssc", "sg-ssc", "official-fc3d", "pc-canada", "bingo-mark-six"} {
		for _, version := range []string{"", "digits5-v2", "mark6-v1", "unknown-v99"} {
			won, reason, err := evaluateBetForRuleVersion(&lottery.Game{ID: gameID}, version, []int{1, 2, 3, 4, 5}, "ball_1_5", 1, "1")
			if apperrors.GetErrorCode(err) != "RULES_NOT_READY" || won || reason != "" {
				t.Fatalf("game=%s version=%q inferred a payout: won=%v reason=%s err=%v", gameID, version, won, reason, err)
			}
		}
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
		{"unversioned twenty is not racing", "speed-racing", "", "RULES_NOT_READY", twenty},
		{"new twenty is not racing", "speed-racing", "racing-v2", "INVALID_DRAW", twenty},
		{"unversioned incomplete racing", "speed-racing", "", "RULES_NOT_READY", []int{1, 2, 3, 4, 5}},
		{"duplicate racing", "speed-racing", "racing-v2", "INVALID_DRAW", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 9}},
		{"out of range digit", "speed-ssc", "digits5-v3", "INVALID_DRAW", []int{1, 2, 3, 4, 10}},
		{"negative digit", "official-fc3d", "digits3-v2", "INVALID_DRAW", []int{1, -1, 3}},
		{"unversioned wrong digit count", "official-fc3d", "", "RULES_NOT_READY", []int{1, 2, 3, 4, 5}},
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
		{"pair", "pair", 4}, {"leopard", "pair", 1}, {"unknown", "1", 1},
	} {
		won, reason, err := evaluateBetForRuleVersion(game, "digits5-v3", []int{1, 2, 3, 4, 5}, test.code, test.position, test.selection)
		if err == nil || won || reason != "" {
			t.Fatalf("%+v produced outcome won=%v reason=%q err=%v", test, won, reason, err)
		}
	}
}

func TestRacingFiveLossDescriptionUsesSmall(t *testing.T) {
	balls := []int{5, 1, 2, 3, 4, 6, 7, 8, 9, 10}
	won, reason, err := evaluateBetForRuleVersion(&lottery.Game{ID: "speed-racing"}, "racing-v2", balls, "two_sided", 1, "大")
	if err != nil || won || !strings.Contains(reason, "5(小/单)") {
		t.Fatalf("won=%v reason=%s err=%v", won, reason, err)
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
	gameByVersion := map[string]string{"racing-v2": "speed-racing", "digits3-v2": "official-fc3d", "digits5-v3": "speed-ssc", markSixRuleVersion: "bingo-mark-six"}
	for _, test := range []struct {
		version, code, selection, want string
		position                       int
	}{
		{"racing-v2", "sum", "12", "冠亚和", 6},
		{"racing-v2", "two_sided", "小", "亚军", 2},
		{"digits3-v2", "sum", "大", "总和", 6},
		{"digits3-v2", "sum", "5", "总和尾", 6},
		{"digits5-v3", "ball_1_5", "0", "第1球", 1},
		{"digits3-v2", "two_sided", "大", "第3球", 3},
		{"digits3-v2", "dragon_tiger", "龙", "第1球", 1},
		{"digits3-v2", "leopard", "leopard", "前三豹子", 1},
		{"digits5-v3", "leopard", "leopard", "前三豹子", 1},
		{"digits5-v3", "pair", "pair", "中三对子", 2},
		{"digits5-v3", "straight", "straight", "后三顺子", 3},
		{"digits5-v3", "dragon_tiger", "龙", "第1球龙虎", 1},
		{"digits5-v3", "dragon_tiger_tie", "和", "第1球龙虎和", 1},
		{markSixRuleVersion, "marksix_regular_number", "18", "正码", 0},
		{markSixRuleVersion, "marksix_combo_2_all", "1,2", "二全中", 0},
	} {
		item := bet.Bet{GameID: gameByVersion[test.version], RuleVersion: test.version, PlayCode: test.code, Position: test.position, Selection: test.selection, PlayName: "stored label"}
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

func TestUnknownRuleLabelsDoNotInferAnotherContract(t *testing.T) {
	for _, gameID := range []string{"", "bingo-racing-b", "bingo-ssc-2", "speed-ssc", "sg-ssc", "speed-racing", "bingo-mark-six"} {
		for _, version := range []string{"", "digits5-v2", "mark6-v1", "unknown-v99", "racing-v2", "digits5-v3", "mark6-v2", "pc28-v1"} {
			if gameSupportsRuleVersion(gameID, version) {
				continue
			}
			item := BetView{GameID: gameID, RuleVersion: version, PlayCode: "pair", PlayName: "存储的玩法说明", Position: 2, Selection: "pair"}
			if got := BetDisplayLabel(item); got != item.PlayName {
				t.Fatalf("%s/%q inferred a rule label: %q", gameID, version, got)
			}
			item.PlayName = ""
			if got := BetDisplayLabel(item); got != "未识别玩法" {
				t.Fatalf("%s/%q invented a label for an unknown contract: %q", gameID, version, got)
			}
		}
	}
}
