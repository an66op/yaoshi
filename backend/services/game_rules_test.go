package services

import (
	"backend/data/models/lottery"
	"math"
	"testing"
)

func TestGameRulesCoverExplicitCatalogWithoutNameInference(t *testing.T) {
	families := map[string][]string{
		"racing-v2":  {"speed-racing", "speed-fly", "sg-fly", "fly-racing", "au-lucky-10", "bingo-racing-a"},
		"digits5-v3": {"speed-ssc", "sg-ssc", "au-lucky-5", "bingo-ssc-1"},
		"digits3-v2": {"official-fc3d", "official-pl3"},
		"mark6-v2":   {"bingo-mark-six"},
		pc28RuleV1:   {"pc-canada"},
		pc28RuleV2:   {"canada-28"},
		pc28RuleV3:   {"canada-20"},
		"":           {"bingo-racing-b", "bingo-ssc-2", "bingo-ssc-3", "bingo-ssc-4", "hong-kong-mark-six", "new-macau-mark-six", "old-macau-mark-six", "happy8-mark-six", "official-qxc", "official-kl8", "official-tw-bingo", "official-tw-super-lotto", "official-tw-daily539", "official-tw-lotto649"},
	}
	count := 0
	for version, ids := range families {
		for _, id := range ids {
			count++
			t.Run(id, func(t *testing.T) {
				game := &lottery.Game{ID: id, Name: "重命名赛车", Category: "飞艇"}
				profile, ok := rulesForGame(game)
				if ok != (version != "") || profile.Version != version {
					t.Fatalf("game %s resolved %q/%t, want %q", id, profile.Version, ok, version)
				}
				if (ensureGameRulesSupported(game) == nil) != ok {
					t.Fatal("readiness differs from placement gate")
				}
				var summary GameSummary
				applyGameRulesSummary(&summary, game)
				if summary.RulesReady != ok || summary.RuleVersion != version || (!ok && summary.RulesMessage == "") {
					t.Fatalf("member summary lost rule status: %+v", summary)
				}
			})
		}
	}
	if count != 30 {
		t.Fatalf("audit inventory has %d games, want 30", count)
	}
	for _, game := range []*lottery.Game{nil, {}, {ID: "unknown", Name: "极速赛车", Category: "赛车"}} {
		if _, ok := rulesForGame(game); ok {
			t.Fatal("unknown game inherited a rule by display name")
		}
		if err := validateBetChoice(game, "ball_1_5", 1, "1"); err == nil {
			t.Fatal("unknown game accepted a bet")
		}
	}
}

func TestGameRulesKeepShapeWindowsVersioned(t *testing.T) {
	for _, test := range []struct {
		gameID  string
		valid   []int
		invalid []int
	}{
		{gameID: "speed-ssc", valid: []int{1, 2, 3}, invalid: []int{0, 4, 5, 6}},
		{gameID: "au-lucky-5", valid: []int{1, 2, 3}, invalid: []int{0, 4, 5, 6}},
		{gameID: "bingo-ssc-1", valid: []int{1, 2, 3}, invalid: []int{0, 4, 5, 6}},
		{gameID: "sg-ssc", valid: []int{1, 2, 3}, invalid: []int{0, 4, 5, 6}},
		{gameID: "bingo-ssc-2", invalid: []int{0, 1, 2, 3, 4, 5, 6}},
	} {
		game := &lottery.Game{ID: test.gameID}
		for _, code := range []string{"leopard", "straight", "pair", "half_straight", "mixed"} {
			for _, position := range test.valid {
				if err := validateBetChoice(game, code, position, code); err != nil {
					t.Fatalf("%s rejected %s position %d: %v", test.gameID, code, position, err)
				}
			}
			for _, position := range test.invalid {
				if err := validateBetChoice(game, code, position, code); err == nil {
					t.Fatalf("%s accepted unmodeled %s position %d", test.gameID, code, position)
				}
			}
		}
	}
}

func TestGameRulesCatalogIsGameSpecificAndParseable(t *testing.T) {
	for _, test := range []struct {
		gameID string
		count  int
		name   string
	}{
		{"speed-racing", 4, "冠亚和"},
		{"speed-ssc", 9, ""},
		{"au-lucky-5", 9, ""},
		{"bingo-ssc-1", 9, ""},
		{"sg-ssc", 9, ""},
		{"official-fc3d", 9, "总和 / 总和尾"},
		{"pc-canada", len(pc28PlaySpecs()), ""},
		{"bingo-mark-six", len(markSixV2Specs), ""},
	} {
		catalog := PlayCatalogForGame(test.gameID)
		profile, _ := rulesForGame(&lottery.Game{ID: test.gameID})
		if len(catalog) != test.count {
			t.Fatalf("%s: %d catalog entries, want %d", test.gameID, len(catalog), test.count)
		}
		for _, item := range catalog {
			if profile.ThreeShapeWindows && item.PlayCode == "sum" {
				t.Fatalf("%s v3 catalog still exposes local totals: %+v", test.gameID, item)
			}
			if item.PlayCode == "sum" && item.PlayName != test.name {
				t.Fatalf("%s mislabeled sum: %q", test.gameID, item.PlayName)
			}
			if item.PlayCode == "leopard" {
				if profile.ThreeShapeWindows && (item.PlayName != "三段豹子" || item.Example != "中三/豹子/20") {
					t.Fatalf("v3 shape catalog obscures its three windows: %+v", item)
				}
			}
		}
	}
}

func TestValidatedStakeCentsRejectsSilentPrecisionLoss(t *testing.T) {
	for _, value := range []float64{0, -1, math.NaN(), math.Inf(1), math.MaxFloat64, 1.234, 12.345, 1.000001, 0.005} {
		if _, err := validatedStakeCents(value); err == nil {
			t.Fatalf("invalid or silently rounded stake %v accepted", value)
		}
	}
	for value, want := range map[float64]int64{1: 100, 20: 2000, 1.23: 123, 12.34: 1234, 0.29: 29, 50000: 5000000} {
		if got, err := validatedStakeCents(value); err != nil || got != want {
			t.Fatalf("valid stake %v: %d/%v, want %d", value, got, err, want)
		}
	}
}
