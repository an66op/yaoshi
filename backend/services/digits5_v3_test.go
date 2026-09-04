package services

import (
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"testing"
)

func TestDigits5V3UsesOnlyCurrentGameContracts(t *testing.T) {
	for _, gameID := range []string{"speed-ssc", "sg-ssc", "au-lucky-5", "bingo-ssc-1", "bingo-ssc-2", "bingo-ssc-3", "bingo-ssc-4"} {
		game := &lottery.Game{ID: gameID}
		profile, ok := rulesForGame(game)
		if !ok || profile.Version != "digits5-v3" || !profile.ThreeShapeWindows || !profile.FirstLastDragonTiger || !profile.DragonTigerTie || profile.SumMarket {
			t.Fatalf("%s current profile: %+v/%v", gameID, profile, ok)
		}
		won, _, err := evaluateBetForRuleVersion(game, "digits5-v3", []int{9, 1, 2, 3, 4}, "ball_1_5", 1, "9")
		if err != nil || !won {
			t.Fatalf("%s current version cannot settle: won=%v err=%v", gameID, won, err)
		}
		for _, version := range []string{"", "digits5-v2", "racing-v2", "digits5-v99"} {
			if _, _, err := evaluateBetForRuleVersion(game, version, []int{9, 1, 2, 3, 4}, "ball_1_5", 1, "9"); apperrors.GetErrorCode(err) != "RULES_NOT_READY" {
				t.Fatalf("%s accepted retired/missing version %q: %v", gameID, version, err)
			}
		}
	}
	if _, ok := rulesForVersion("digits5-v2"); ok {
		t.Fatal("prelaunch build retained the retired five-ball engine")
	}
}

func TestVerifiedBingoVariantsUseTheirVersionedBettingContracts(t *testing.T) {
	for _, gameID := range []string{"bingo-ssc-2", "bingo-ssc-3", "bingo-ssc-4"} {
		game := &lottery.Game{ID: gameID}
		profile, ok := rulesForGame(game)
		if !ok || profile.Version != "digits5-v3" || !profile.ThreeShapeWindows || !profile.DragonTigerTie {
			t.Fatalf("%s did not bind the verified five-ball contract: %+v/%v", gameID, profile, ok)
		}
		lines, err := parseAssistantBetForGame(game, "中三顺子/5#后三/对子/5#1/龙虎和/5")
		if err != nil || len(lines) != 5 {
			t.Fatalf("%s rejected the verified v3 syntax: %+v %v", gameID, lines, err)
		}
		won, _, err := evaluateBetForRuleVersion(game, "digits5-v3", []int{9, 1, 2, 3, 9}, "dragon_tiger_tie", 1, "和")
		if err != nil || !won {
			t.Fatalf("%s could not settle the verified v3 contract: won=%v err=%v", gameID, won, err)
		}
	}
	racing := &lottery.Game{ID: "bingo-racing-b"}
	profile, ok := rulesForGame(racing)
	if !ok || profile.Version != "racing-v2" || !profile.Racing || !profile.Unique {
		t.Fatalf("bingo-racing-b did not bind the verified racing contract: %+v/%v", profile, ok)
	}
	if lines, err := parseAssistantBetForGame(racing, "0/0/5#冠亚/14/5"); err != nil || len(lines) != 2 {
		t.Fatalf("bingo-racing-b rejected racing syntax: %+v %v", lines, err)
	}
}

func TestSGSSCVerifiedSourcePreservesFiveBallRules(t *testing.T) {
	kind, name, sourceURL, status := defaultLotterySource("sg-ssc")
	if kind != "external" || name != sgSSCVerifiedSourceName || sourceURL != sgSSCVerifiedSourceURL || status != "stale" {
		t.Fatalf("SG时时彩 did not wait for verified external data: %q/%q/%q/%q", kind, name, sourceURL, status)
	}
	for _, binding := range api168HighFreqBindings {
		if binding.GameID == "sg-ssc" {
			t.Fatalf("SG时时彩 was silently bound to 168 source %s", binding.LotCode)
		}
	}
	profile, ready := rulesForGame(&lottery.Game{ID: "sg-ssc"})
	if !ready || profile.Version != "digits5-v3" || !gameSupportsRuleVersion("sg-ssc", "digits5-v3") {
		t.Fatalf("SG时时彩 did not use the current five-ball contract: %+v ready=%v", profile, ready)
	}
}

func TestDigits5V3RejectsRetiredLocalMarketsEverywhere(t *testing.T) {
	for _, gameID := range []string{"speed-ssc", "sg-ssc", "au-lucky-5", "bingo-ssc-1"} {
		game := &lottery.Game{ID: gameID}
		profile, _ := rulesForGame(game)
		if profile.supportsPlay("sum") {
			t.Fatalf("%s v3 advertises the local sum market", gameID)
		}
		for _, content := range []string{"总和/大/20", "总和尾/7/20", "总和大20", "总和尾7/20", "2/龙/20"} {
			if lines, err := parseAssistantBetForGame(game, content); err == nil || lines != nil {
				t.Fatalf("%s v3 accepted %q: %+v %v", gameID, content, lines, err)
			}
		}
		for _, choice := range []struct {
			code, selection string
			position        int
		}{
			{"sum", "大", 6}, {"sum", "5", 6}, {"dragon_tiger", "龙", 2}, {"ball_1_5", "5", 6},
		} {
			if err := validateBetChoice(game, choice.code, choice.position, choice.selection); err == nil {
				t.Fatalf("%s direct placement accepted retired choice %+v", gameID, choice)
			}
			if _, _, err := evaluateBetForRuleVersion(game, "digits5-v3", []int{9, 9, 9, 9, 9}, choice.code, choice.position, choice.selection); err == nil {
				t.Fatalf("%s settlement accepted retired choice %+v", gameID, choice)
			}
		}
	}
}

func TestDigits5V3ShapeWindowsPartitionEveryTriple(t *testing.T) {
	shapeCodes := []string{"leopard", "straight", "pair", "half_straight", "mixed"}
	for _, gameID := range []string{"speed-ssc", "sg-ssc", "au-lucky-5", "bingo-ssc-1"} {
		game := &lottery.Game{ID: gameID}
		for position := 1; position <= 3; position++ {
			for encoded := 0; encoded < 1000; encoded++ {
				balls := []int{7, 7, 7, 7, 7}
				balls[position-1] = encoded / 100
				balls[position] = encoded / 10 % 10
				balls[position+1] = encoded % 10
				wins := 0
				for _, code := range shapeCodes {
					won, _, err := evaluateBetForRuleVersion(game, "digits5-v3", balls, code, position, code)
					if err != nil {
						t.Fatalf("%s position=%d draw=%v code=%s: %v", gameID, position, balls, code, err)
					}
					if won {
						wins++
					}
				}
				if wins != 1 {
					t.Fatalf("%s position=%d draw=%v matched %d shapes", gameID, position, balls, wins)
				}
			}
		}
	}
}

func TestDigits5V3CircularStraightBoundaryIsFrozen(t *testing.T) {
	for _, gameID := range []string{"speed-ssc", "sg-ssc", "au-lucky-5", "bingo-ssc-1"} {
		game := &lottery.Game{ID: gameID}
		for _, triple := range [][]int{{8, 9, 0}, {9, 0, 1}, {0, 1, 9}} {
			for position := 1; position <= 3; position++ {
				balls := []int{4, 4, 4, 4, 4}
				copy(balls[position-1:position+2], triple)
				won, reason, err := evaluateBetForRuleVersion(game, "digits5-v3", balls, "straight", position, "straight")
				if err != nil || !won {
					t.Fatalf("%s position=%d triple=%v must remain a circular straight: won=%v reason=%q err=%v", gameID, position, triple, won, reason, err)
				}
			}
		}
	}
}

func TestDigits5V3FirstVersusFifthDragonTigerTie(t *testing.T) {
	for _, test := range []struct {
		name      string
		draw      []int
		playCode  string
		selection string
		want      bool
	}{
		{name: "dragon", draw: []int{6, 9, 9, 9, 5}, playCode: "dragon_tiger", selection: "龙", want: true},
		{name: "dragon is not tie", draw: []int{6, 9, 9, 9, 5}, playCode: "dragon_tiger_tie", selection: "和", want: false},
		{name: "tiger", draw: []int{4, 9, 9, 9, 5}, playCode: "dragon_tiger", selection: "虎", want: true},
		{name: "tie", draw: []int{5, 9, 9, 9, 5}, playCode: "dragon_tiger_tie", selection: "和", want: true},
		{name: "tie loses dragon", draw: []int{5, 9, 9, 9, 5}, playCode: "dragon_tiger", selection: "龙", want: false},
		{name: "tie loses tiger", draw: []int{5, 9, 9, 9, 5}, playCode: "dragon_tiger", selection: "虎", want: false},
	} {
		for _, gameID := range []string{"speed-ssc", "sg-ssc", "au-lucky-5", "bingo-ssc-1"} {
			t.Run(gameID+"/"+test.name, func(t *testing.T) {
				won, _, err := evaluateBetForRuleVersion(&lottery.Game{ID: gameID}, "digits5-v3", test.draw, test.playCode, 1, test.selection)
				if err != nil || won != test.want {
					t.Fatalf("won=%v want=%v err=%v", won, test.want, err)
				}
			})
		}
	}
	for _, gameID := range []string{"speed-ssc", "sg-ssc", "au-lucky-5", "bingo-ssc-1"} {
		for _, item := range []struct {
			code, selection string
		}{
			{code: "dragon_tiger", selection: "龙"},
			{code: "dragon_tiger_tie", selection: "和"},
		} {
			if _, _, err := evaluateBetForRuleVersion(&lottery.Game{ID: gameID}, "digits5-v3", []int{9, 8, 7, 6, 5}, item.code, 2, item.selection); err == nil {
				t.Fatalf("%s v3 accepted obsolete second-vs-fourth market: %+v", gameID, item)
			}
		}
	}
}

func TestDigits5V3TieHasIndependentBackendOddsCode(t *testing.T) {
	for _, gameID := range []string{"speed-ssc", "sg-ssc", "au-lucky-5", "bingo-ssc-1"} {
		profile, _ := rulesForGame(&lottery.Game{ID: gameID})
		if !profile.supportsPlay("dragon_tiger") || !profile.supportsPlay("dragon_tiger_tie") {
			t.Fatalf("%s v3 catalog lost a dragon/tiger outcome", gameID)
		}
		if code, name, err := InferPlayForGame(&lottery.Game{ID: gameID}, "", "", 1, "和"); err != nil || code != "dragon_tiger_tie" || name != "龙虎和" {
			t.Fatalf("%s tie inference: %s/%s %v", gameID, code, name, err)
		}
	}
	tie, ok := defaultPlayByCode("dragon_tiger_tie")
	if !ok || tie.Code == "dragon_tiger" {
		t.Fatalf("tie is not an independent configurable market: %+v/%v", tie, ok)
	}
}
