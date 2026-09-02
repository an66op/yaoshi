package services

import "testing"

func TestMemberOddsRuleStatusMatchesFinancialRuleReadiness(t *testing.T) {
	for _, gameID := range []string{"speed-racing", "bingo-racing-a", "speed-ssc", "bingo-ssc-1", "official-fc3d", "pc-canada", "canada-28", "canada-20"} {
		ready, version, message, modes := memberOddsRuleStatus(gameID)
		if !ready || version == "" || message != "" || !modes.Chat || !modes.Web {
			t.Fatalf("supported game %s returned ready=%v version=%q message=%q modes=%+v", gameID, ready, version, message, modes)
		}
	}
	for _, gameID := range []string{"unknown"} {
		ready, version, message, modes := memberOddsRuleStatus(gameID)
		if ready || version != "" || message == "" || modes.Chat || modes.Web {
			t.Fatalf("blocked game %s returned ready=%v version=%q message=%q modes=%+v", gameID, ready, version, message, modes)
		}
	}
	ready, version, message, modes := memberOddsRuleStatus("bingo-mark-six")
	if !ready || version != markSixRuleVersion || message != "" || modes.Chat || !modes.Web {
		t.Fatalf("Bingo Mark Six modes are not web-only: ready=%v version=%q message=%q modes=%+v", ready, version, message, modes)
	}
}
