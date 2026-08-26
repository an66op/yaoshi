package services

import "testing"

func TestAgentIDFromScope(t *testing.T) {
	if id, ok := agentIDFromScope("agent:23"); !ok || id != 23 {
		t.Fatalf("agent scope parsed as %d, %v", id, ok)
	}
	for _, value := range []string{"lobby", "agent:", "agent:abc", "agent:0"} {
		if _, ok := agentIDFromScope(value); ok {
			t.Fatalf("invalid scope %q must be rejected", value)
		}
	}
}

func TestOutstandingProfitShareIsIdempotent(t *testing.T) {
	if got := outstandingProfitShare(10_000, 0); got != 10_000 {
		t.Fatalf("first payout delta = %d", got)
	}
	if got := outstandingProfitShare(10_000, 10_000); got != 0 {
		t.Fatalf("same batch must not pay twice, got %d", got)
	}
	if got := outstandingProfitShare(12_500, 10_000); got != 2_500 {
		t.Fatalf("late settlement delta = %d", got)
	}
	if got := outstandingProfitShare(9_000, 10_000); got != 0 {
		t.Fatalf("paid amount must never be clawed back implicitly, got %d", got)
	}
}
