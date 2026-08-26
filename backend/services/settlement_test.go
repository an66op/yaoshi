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
