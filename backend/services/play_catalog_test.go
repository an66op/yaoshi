package services

import (
	"backend/data/models/lottery"
	"testing"
)

func TestInferPlayForGameKeepsSixthRacingRankSeparateFromCrownSum(t *testing.T) {
	for _, gameID := range []string{"speed-racing", "speed-fly", "sg-fly", "fly-racing", "au-lucky-10", "bingo-racing-b"} {
		t.Run(gameID, func(t *testing.T) {
			game := &lottery.Game{ID: gameID}
			for _, test := range []struct{ code, name, selection, want string }{
				{"", "", "大", "two_sided"}, {"", "", "5", "ball_1_5"}, {"", "", "10", "ball_1_5"},
				{"", "冠亚和", "大", "sum"}, {"", "冠亚", "14", "sum"}, {"sum", "", "14", "sum"},
				{"two_sided", "", "小", "two_sided"}, {"ball_1_5", "", "0", "ball_1_5"},
			} {
				code, _, err := InferPlayForGame(game, test.code, test.name, 6, test.selection)
				if err != nil || code != test.want {
					t.Fatalf("%+v -> %s %v", test, code, err)
				}
			}
			if _, _, err := InferPlayForGame(game, "", "总和", 6, "大"); err == nil {
				t.Fatal("racing must not guess a generic digit total")
			}
			if _, _, err := InferPlayForGame(game, "", "", 1, "豹子"); err == nil {
				t.Fatal("racing must reject shape inference")
			}
		})
	}
}

func TestInferPlayForGameDigitTotalsRequireExplicitMeaning(t *testing.T) {
	for _, gameID := range []string{"speed-ssc", "sg-ssc", "au-lucky-5", "bingo-ssc-1", "bingo-ssc-2", "bingo-ssc-3", "bingo-ssc-4", "official-fc3d", "official-pl3"} {
		t.Run(gameID, func(t *testing.T) {
			game := &lottery.Game{ID: gameID}
			v3 := isUpgradedDigits5Game(gameID)
			for _, test := range []struct{ code, name, selection, wantCode, wantName string }{
				{"", "总和", "大", "sum", "总和"}, {"", "总和尾", "7", "sum", "总和尾"},
				{"sum", "", "大", "sum", "总和"}, {"sum", "", "7", "sum", "总和尾"},
				{"", "", "大", "two_sided", "两面"}, {"", "", "7", "ball_1_5", "指定球位号码"},
				{"", "", "豹子", "leopard", "前三豹子"}, {"", "前三对子", "中", "pair", "前三对子"},
			} {
				code, name, err := InferPlayForGame(game, test.code, test.name, 6, test.selection)
				if v3 && test.wantCode == "sum" {
					if err == nil {
						t.Fatalf("digits5-v3 inferred closed sum market from %+v: %s/%s", test, code, name)
					}
					continue
				}
				if err != nil || code != test.wantCode || name != test.wantName {
					t.Fatalf("%+v -> %s/%s %v", test, code, name, err)
				}
			}
			for _, code := range []string{"", "sum"} {
				if _, _, err := InferPlayForGame(game, code, "冠亚和", 6, "大"); err == nil {
					t.Fatal("digit game must reject racing sum name")
				}
			}
			if _, _, err := InferPlayForGame(game, "sum", "总和尾", 6, "大"); err == nil {
				t.Fatal("a tail name must not silently become a total side bet")
			}
			for _, name := range []string{"中三豹子", "后三顺子"} {
				_, _, err := InferPlayForGame(game, "leopard", name, 1, "中")
				if isUpgradedDigits5Game(gameID) {
					if err == nil {
						t.Fatal("scoped shape name with a mismatched position was accepted")
					}
				} else if err == nil {
					t.Fatal("v2 shape scope must not silently become front-three")
				}
			}
		})
	}
}

func TestInferPlayForGameUnknownRulesFailClosed(t *testing.T) {
	for _, game := range []*lottery.Game{nil, {ID: "hong-kong-mark-six"}, {ID: "truly-unknown"}, {Name: "极速赛车", Category: "赛车"}} {
		if _, _, err := InferPlayForGame(game, "ball_1_5", "", 1, "1"); err == nil {
			t.Fatalf("unknown profile must not infer: %+v", game)
		}
	}
	pc := &lottery.Game{ID: "pc-canada"}
	if _, _, err := InferPlayForGame(pc, "", "", 0, "13"); err == nil {
		t.Fatal("PC28 must require a typed atomic play code")
	}
	code := pc28ExactCode(13)
	if actual, name, err := InferPlayForGame(pc, code, "", 0, "13"); err != nil || actual != code || name == "" {
		t.Fatalf("PC28 atomic play was not recognized: %s/%s %v", actual, name, err)
	}
	if _, _, err := InferPlayForGame(&lottery.Game{ID: "speed-racing"}, "unknown", "", 1, "1"); err == nil {
		t.Fatal("unknown play code must not fall back to numbers")
	}
}
