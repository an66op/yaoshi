package services

import (
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"testing"
)

func TestDigits5V3VersionMatrixKeepsHistoricalV2Tickets(t *testing.T) {
	for _, gameID := range []string{"speed-ssc", "au-lucky-5", "bingo-ssc-1"} {
		profile, ok := rulesForGame(&lottery.Game{ID: gameID})
		if !ok || profile.Version != "digits5-v3" || !profile.ThreeShapeWindows || !profile.FirstLastDragonTiger || !profile.DragonTigerTie || profile.SumMarket {
			t.Fatalf("%s current profile: %+v/%v", gameID, profile, ok)
		}
		for _, version := range []string{"digits5-v2", "digits5-v3"} {
			won, _, err := evaluateBetForRuleVersion(&lottery.Game{ID: gameID}, version, []int{9, 1, 2, 3, 4}, "ball_1_5", 1, "9")
			if err != nil || !won {
				t.Fatalf("%s historical/current version %s cannot settle: won=%v err=%v", gameID, version, won, err)
			}
		}
	}

	for _, gameID := range []string{"sg-ssc", "bingo-ssc-2", "bingo-ssc-3", "bingo-ssc-4"} {
		legacy := &lottery.Game{ID: gameID}
		if profile, ok := rulesForGame(legacy); !ok || profile.Version != "digits5-v2" || profile.ThreeShapeWindows || profile.DragonTigerTie {
			t.Fatalf("%s inherited v3: %+v/%v", gameID, profile, ok)
		}
		if _, _, err := evaluateBetForRuleVersion(legacy, "digits5-v3", []int{1, 2, 3, 4, 5}, "ball_1_5", 1, "1"); apperrors.GetErrorCode(err) != "RULES_NOT_READY" {
			t.Fatalf("%s accepted digits5-v3 settlement: %v", gameID, err)
		}
		if err := validateBetChoice(legacy, "dragon_tiger_tie", 1, "和"); err == nil {
			t.Fatalf("%s accepted the new independent tie market", gameID)
		}
	}
}

func TestBingoSSC1HistoricalV2AndEmptyTicketsKeepLegacyMeaning(t *testing.T) {
	game := &lottery.Game{ID: "bingo-ssc-1"}

	// v2 only modelled the first triple. An empty pre-migration row must also
	// keep that front-three meaning even when stale metadata says position 2;
	// it cannot silently become the new middle-three market.
	shapeDraw := []int{1, 1, 2, 3, 4}
	for _, version := range []string{"digits5-v2", ""} {
		position := 1
		if version == "" {
			position = 2
		}
		won, _, err := evaluateBetForRuleVersion(game, version, shapeDraw, "pair", position, "pair")
		if err != nil || !won {
			t.Fatalf("legacy version %q front-three pair: won=%v err=%v", version, won, err)
		}
	}
	if won, _, err := evaluateBetForRuleVersion(game, "digits5-v3", shapeDraw, "pair", 2, "pair"); err != nil || won {
		t.Fatalf("v3 must evaluate the middle window rather than old front-three: won=%v err=%v", won, err)
	}
	if won, _, err := evaluateBetForRuleVersion(game, "digits5-v3", shapeDraw, "straight", 2, "straight"); err != nil || !won {
		t.Fatalf("v3 middle-three straight was not evaluated: won=%v err=%v", won, err)
	}

	// The former v2 second-vs-fourth comparison remains settleable only on an
	// old snapshot. Current placement and v3 settlement reject that position.
	dragonDraw := []int{1, 9, 2, 3, 4}
	for _, version := range []string{"digits5-v2", ""} {
		won, _, err := evaluateBetForRuleVersion(game, version, dragonDraw, "dragon_tiger", 2, "龙")
		if err != nil || !won {
			t.Fatalf("legacy version %q second/fourth dragon: won=%v err=%v", version, won, err)
		}
	}
	if _, _, err := evaluateBetForRuleVersion(game, "digits5-v3", dragonDraw, "dragon_tiger", 2, "龙"); err == nil {
		t.Fatal("v3 reintroduced the retired second-vs-fourth dragon market")
	}
	if err := validateBetChoice(game, "sum", 6, "大"); err == nil {
		t.Fatal("new bingo-ssc-1 placement still accepts the retired v2 total")
	}
	if won, _, err := evaluateBetForRuleVersion(game, "digits5-v2", []int{9, 9, 9, 9, 9}, "sum", 6, "大"); err != nil || !won {
		t.Fatalf("historical v2 total no longer settles: won=%v err=%v", won, err)
	}
}

func TestSGSSCRemainsPlatformDrawAndOutsideExternalV3Rollout(t *testing.T) {
	kind, name, sourceURL, status := defaultLotterySource("sg-ssc")
	if kind != "platform" || name != "王者开奖" || sourceURL != "" || status != "ok" {
		t.Fatalf("SG时时彩 product identity changed without a confirmed source: %q/%q/%q/%q", kind, name, sourceURL, status)
	}
	for _, binding := range api168HighFreqBindings {
		if binding.GameID == "sg-ssc" {
			t.Fatalf("SG时时彩 was silently bound to 168 source %s", binding.LotCode)
		}
	}
	profile, ready := rulesForGame(&lottery.Game{ID: "sg-ssc"})
	if !ready || profile.Version != "digits5-v2" || gameSupportsRuleVersion("sg-ssc", "digits5-v3") {
		t.Fatalf("SG时时彩 escaped v2 isolation: %+v ready=%v", profile, ready)
	}
}

func TestDigits5V3ClosesNewLocalSumMarketsButSettlesHistoricalTickets(t *testing.T) {
	for _, gameID := range []string{"speed-ssc", "au-lucky-5", "bingo-ssc-1"} {
		game := &lottery.Game{ID: gameID}
		profile, _ := rulesForGame(game)
		if profile.supportsPlay("sum") {
			t.Fatalf("%s v3 still advertises the local sum market", gameID)
		}
		for _, content := range []string{"总和/大/20", "总和尾/7/20", "总和大20", "总和尾7/20"} {
			if lines, err := parseAssistantBetForGame(game, content); err == nil || lines != nil {
				t.Fatalf("%s v3 accepted %q: %+v %v", gameID, content, lines, err)
			}
		}
		if err := validateBetChoice(game, "sum", 6, "大"); err == nil {
			t.Fatalf("%s direct placement accepted a v3 sum", gameID)
		}
		for _, test := range []struct {
			selection string
			want      bool
		}{
			{selection: "大", want: true},
			{selection: "5", want: true},
			{selection: "小", want: false},
			{selection: "4", want: false},
		} {
			won, _, err := evaluateBetForRuleVersion(game, "digits5-v3", []int{9, 9, 9, 9, 9}, "sum", 6, test.selection)
			if err != nil || won != test.want {
				t.Fatalf("%s historical v3 sum selection=%s: won=%v want=%v err=%v", gameID, test.selection, won, test.want, err)
			}
		}
		for _, invalid := range []struct {
			position  int
			selection string
		}{{position: 1, selection: "大"}, {position: 6, selection: "10"}, {position: 6, selection: "龙虎"}} {
			if _, _, err := evaluateBetForRuleVersion(game, "digits5-v3", []int{9, 9, 9, 9, 9}, "sum", invalid.position, invalid.selection); err == nil {
				t.Fatalf("%s historical v3 compatibility accepted malformed sum %+v", gameID, invalid)
			}
		}
		won, _, err := evaluateBetForRuleVersion(game, "digits5-v2", []int{9, 9, 9, 9, 9}, "sum", 6, "大")
		if err != nil || !won {
			t.Fatalf("%s historical v2 sum no longer settles: won=%v err=%v", gameID, won, err)
		}
	}

	legacy, _ := rulesForVersion("digits5-v2")
	if !legacy.SumMarket || !legacy.supportsPlay("sum") {
		t.Fatal("digits5-v2 lost its frozen historical sum contract")
	}
}

func TestDigits5V3ShapeWindowsPartitionEveryTriple(t *testing.T) {
	shapeCodes := []string{"leopard", "straight", "pair", "half_straight", "mixed"}
	for _, gameID := range []string{"speed-ssc", "au-lucky-5", "bingo-ssc-1"} {
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
	for _, gameID := range []string{"speed-ssc", "au-lucky-5", "bingo-ssc-1"} {
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
		for _, gameID := range []string{"speed-ssc", "au-lucky-5", "bingo-ssc-1"} {
			t.Run(gameID+"/"+test.name, func(t *testing.T) {
				won, _, err := evaluateBetForRuleVersion(&lottery.Game{ID: gameID}, "digits5-v3", test.draw, test.playCode, 1, test.selection)
				if err != nil || won != test.want {
					t.Fatalf("won=%v want=%v err=%v", won, test.want, err)
				}
			})
		}
	}
	for _, gameID := range []string{"speed-ssc", "au-lucky-5", "bingo-ssc-1"} {
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
	for _, gameID := range []string{"speed-ssc", "au-lucky-5", "bingo-ssc-1"} {
		profile, _ := rulesForGame(&lottery.Game{ID: gameID})
		if !profile.supportsPlay("dragon_tiger") || !profile.supportsPlay("dragon_tiger_tie") {
			t.Fatalf("%s v3 catalog lost a dragon/tiger outcome", gameID)
		}
		if code, name, err := InferPlayForGame(&lottery.Game{ID: gameID}, "", "", 1, "和"); err != nil || code != "dragon_tiger_tie" || name != "龙虎和" {
			t.Fatalf("%s tie inference: %s/%s %v", gameID, code, name, err)
		}
	}
	tie, ok := defaultPlayByCode("dragon_tiger_tie")
	if !ok || tie.Odds != 8.7 || tie.Code == "dragon_tiger" {
		t.Fatalf("tie is not an independent configurable market: %+v/%v", tie, ok)
	}
	if _, _, err := InferPlayForGame(&lottery.Game{ID: "sg-ssc"}, "", "", 1, "和"); err == nil {
		t.Fatal("v2 digit game inferred the v3 tie market")
	}
}
