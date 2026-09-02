package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"testing"
)

func TestEvaluateCurrentDigitMarkets(t *testing.T) {
	for _, test := range []struct {
		name, gameID, version, play, selection string
		position                               int
		numbers                                []int
		want                                   bool
	}{
		{"ball win", "speed-ssc", "digits5-v3", "ball_1_5", "3", 1, []int{3, 5, 8, 2, 1}, true},
		{"ball lose", "speed-ssc", "digits5-v3", "ball_1_5", "4", 1, []int{3, 5, 8, 2, 1}, false},
		{"three-ball sum tail", "official-fc3d", "digits3-v2", "sum", "6", 6, []int{3, 5, 8}, true},
		{"big", "speed-ssc", "digits5-v3", "two_sided", "大", 1, []int{7, 1, 2, 3, 4}, true},
		{"dragon", "speed-ssc", "digits5-v3", "dragon_tiger", "龙", 1, []int{7, 1, 2, 3, 4}, true},
		{"tiger lose", "speed-ssc", "digits5-v3", "dragon_tiger", "虎", 1, []int{7, 1, 2, 3, 4}, false},
		{"leopard", "speed-ssc", "digits5-v3", "leopard", "leopard", 1, []int{5, 5, 5, 1, 2}, true},
		{"straight", "speed-ssc", "digits5-v3", "straight", "straight", 1, []int{1, 2, 3, 8, 9}, true},
		{"pair", "speed-ssc", "digits5-v3", "pair", "pair", 1, []int{1, 1, 4, 8, 9}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			won, reason, err := evaluateBetForRuleVersion(&lottery.Game{ID: test.gameID}, test.version, test.numbers, test.play, test.position, test.selection)
			if err != nil || won != test.want {
				t.Fatalf("won=%v, want=%v: %s, err=%v", won, test.want, reason, err)
			}
		})
	}
	if _, _, err := evaluateBetForRuleVersion(&lottery.Game{ID: "speed-ssc"}, "digits5-v3", []int{3, 5, 8, 2, 1}, "ball_1_5", 6, "9"); err == nil {
		t.Fatal("nonexistent sixth ball must not become a sum-tail bet")
	}
}

func TestEvaluateRacingCrownSumBoundariesAreExclusive(t *testing.T) {
	game := &lottery.Game{ID: "speed-racing"}
	// 4 + 7 = 11: small and odd only.
	eleven := []int{4, 7, 1, 2, 3, 5, 6, 8, 9, 10}
	for _, selection := range []string{"小", "单", "11"} {
		if won, reason, err := evaluateBetForRuleVersion(game, "racing-v2", eleven, "sum", 6, selection); err != nil || !won {
			t.Fatalf("11 should win %s: %s, err=%v", selection, reason, err)
		}
	}
	for _, selection := range []string{"大", "双", "12"} {
		if won, reason, err := evaluateBetForRuleVersion(game, "racing-v2", eleven, "sum", 6, selection); err != nil || won {
			t.Fatalf("11 must lose %s: %s, err=%v", selection, reason, err)
		}
	}

	// 5 + 7 = 12: big and even only.
	twelve := []int{5, 7, 1, 2, 3, 4, 6, 8, 9, 10}
	for _, selection := range []string{"大", "双", "12"} {
		if won, reason, err := evaluateBetForRuleVersion(game, "racing-v2", twelve, "sum", 6, selection); err != nil || !won {
			t.Fatalf("12 should win %s: %s, err=%v", selection, reason, err)
		}
	}
	for _, selection := range []string{"小", "单", "11"} {
		if won, reason, err := evaluateBetForRuleVersion(game, "racing-v2", twelve, "sum", 6, selection); err != nil || won {
			t.Fatalf("12 must lose %s: %s, err=%v", selection, reason, err)
		}
	}
}

func TestEvaluateRacingZeroAliasOnlyMeansTenForTenBallGames(t *testing.T) {
	racing := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	game := &lottery.Game{ID: "speed-racing"}
	selection := normalizeBetSelection(game, "ball_1_5", "0")
	if won, reason, err := evaluateBetForRuleVersion(game, "racing-v2", racing, "ball_1_5", 10, selection); err != nil || !won {
		t.Fatalf("normalized racing 0 alias should match number 10: %s, err=%v", reason, err)
	}
	ssc := []int{0, 2, 3, 4, 5}
	if won, reason, err := evaluateBetForRuleVersion(&lottery.Game{ID: "speed-ssc"}, "digits5-v3", ssc, "ball_1_5", 1, "0"); err != nil || !won {
		t.Fatalf("five-ball game must keep literal zero: %s, err=%v", reason, err)
	}
}

func TestSettlementEventKeyKeepsGameAndRoomIsolation(t *testing.T) {
	base := settlementEventKey("speed-racing", "34130076", 23, "agent:9")
	if base == settlementEventKey("speed-ssc", "34130076", 23, "agent:9") {
		t.Fatal("different games must not share a settlement event key")
	}
	if base == settlementEventKey("speed-racing", "34130076", 23, "agent:10") {
		t.Fatal("different rooms must not share a settlement event key")
	}
	if base != settlementEventKey("speed-racing", "34130076", 23, "agent:9") {
		t.Fatal("the same settlement event must be idempotent")
	}
}

func TestAgentProfitShareOnlyUsesPositiveGrossProfit(t *testing.T) {
	if got := agentProfitShareCents(10_000, 25); got != 2_500 {
		t.Fatalf("positive GGR share = %d, want 2500", got)
	}
	if got := agentProfitShareCents(-10_000, 25); got != 0 {
		t.Fatalf("negative GGR must not create a negative share, got %d", got)
	}
	if got := agentProfitShareCents(10_000, 150); got != 10_000 {
		t.Fatalf("share rate must be clamped to 100%%, got %d", got)
	}
}

func TestRobotSettlementCannotCreateRebateOrAgentShare(t *testing.T) {
	item := bet.Bet{AmountCents: 10_000, RebateRateSnapshot: 5, AgentShareRateSnapshot: 25}
	rebateCents, shareCents := settledBetFinancialAmounts(item, 0, true)
	if rebateCents != 0 || shareCents != 0 {
		t.Fatalf("robot settlement created rebate/share: %d/%d", rebateCents, shareCents)
	}
	rebateCents, shareCents = settledBetFinancialAmounts(item, 0, false)
	if rebateCents != 500 || shareCents != 2_500 {
		t.Fatalf("human settlement amounts = %d/%d, want 500/2500", rebateCents, shareCents)
	}
}
