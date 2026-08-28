package services

import "testing"

func TestEvaluateBetBallAndSumTail(t *testing.T) {
	numbers := []int{3, 5, 8, 2, 1}
	if won, _ := evaluateBet(numbers, "ball_1_5", 1, "3"); !won {
		t.Fatal("expected ball1=3 win")
	}
	if won, _ := evaluateBet(numbers, "ball_1_5", 1, "4"); won {
		t.Fatal("expected ball1=4 lose")
	}
	// sum=19, tail=9
	if won, _ := evaluateBet(numbers, "ball_1_5", 6, "9"); !won {
		t.Fatal("expected sum tail 9 win")
	}
}

func TestEvaluateBetTwoSidedAndDragon(t *testing.T) {
	numbers := []int{7, 1, 2, 3, 4}
	if won, _ := evaluateBet(numbers, "two_sided", 1, "大"); !won {
		t.Fatal("expected big win")
	}
	if won, _ := evaluateBet(numbers, "dragon_tiger", 1, "龙"); !won {
		t.Fatal("expected dragon win")
	}
	if won, _ := evaluateBet(numbers, "dragon_tiger", 1, "虎"); won {
		t.Fatal("expected tiger lose")
	}
}

func TestEvaluateBetPatterns(t *testing.T) {
	if won, _ := evaluateBet([]int{5, 5, 5, 1, 2}, "leopard", 1, ""); !won {
		t.Fatal("expected leopard")
	}
	if won, _ := evaluateBet([]int{1, 2, 3, 8, 9}, "straight", 1, ""); !won {
		t.Fatal("expected straight")
	}
	if won, _ := evaluateBet([]int{1, 1, 4, 8, 9}, "pair", 1, ""); !won {
		t.Fatal("expected pair")
	}
}

func TestEvaluateRacingCrownSumBoundariesAreExclusive(t *testing.T) {
	// 4 + 7 = 11: small and odd only.
	eleven := []int{4, 7, 1, 2, 3, 5, 6, 8, 9, 10}
	for _, selection := range []string{"小", "单", "11"} {
		if won, reason := evaluateBet(eleven, "sum", 6, selection); !won {
			t.Fatalf("11 should win %s: %s", selection, reason)
		}
	}
	for _, selection := range []string{"大", "双", "12"} {
		if won, reason := evaluateBet(eleven, "sum", 6, selection); won {
			t.Fatalf("11 must lose %s: %s", selection, reason)
		}
	}

	// 5 + 7 = 12: big and even only.
	twelve := []int{5, 7, 1, 2, 3, 4, 6, 8, 9, 10}
	for _, selection := range []string{"大", "双", "12"} {
		if won, reason := evaluateBet(twelve, "sum", 6, selection); !won {
			t.Fatalf("12 should win %s: %s", selection, reason)
		}
	}
	for _, selection := range []string{"小", "单", "11"} {
		if won, reason := evaluateBet(twelve, "sum", 6, selection); won {
			t.Fatalf("12 must lose %s: %s", selection, reason)
		}
	}
}

func TestEvaluateRacingZeroAliasOnlyMeansTenForTenBallGames(t *testing.T) {
	racing := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if won, reason := evaluateBet(racing, "ball_1_5", 10, "0"); !won {
		t.Fatalf("racing 0 alias should match number 10: %s", reason)
	}
	ssc := []int{0, 2, 3, 4, 5}
	if won, reason := evaluateBet(ssc, "ball_1_5", 1, "0"); !won {
		t.Fatalf("five-ball game must keep literal zero: %s", reason)
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
