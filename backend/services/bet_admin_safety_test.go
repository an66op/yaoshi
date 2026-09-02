package services

import (
	"backend/data/models/lottery"
	"math"
	"testing"
)

func TestMoneyCentsRejectsNonFiniteAndOutOfRangeValues(t *testing.T) {
	invalid := []float64{
		math.NaN(),
		math.Inf(1),
		math.Inf(-1),
		-1,
		0,
		0.004,
		math.Ldexp(1, 63) / 100,
		math.MaxFloat64,
	}
	for _, value := range invalid {
		if cents, err := positiveMoneyCents(value, "下注金额"); err == nil {
			t.Fatalf("positiveMoneyCents(%v) = %d, want rejection", value, cents)
		}
	}

	cents, err := positiveMoneyCents(12.345, "下注金额")
	if err != nil || cents != 1235 {
		t.Fatalf("positiveMoneyCents(12.345) = %d, %v; want 1235", cents, err)
	}
	if cents, err := nonNegativeMoneyCents(0, "飞单金额"); err != nil || cents != 0 {
		t.Fatalf("zero fly amount = %d, %v; want zero", cents, err)
	}
}

func TestRequestedFlyAmountFailsClosed(t *testing.T) {
	if amount, err := requestedFlyAmount(nil); err != nil || amount != -1 {
		t.Fatalf("nil fly amount = %v, %v; want auto sentinel", amount, err)
	}
	invalid := []float64{math.NaN(), math.Inf(1), -0.01, math.Ldexp(1, 63) / 100}
	for _, value := range invalid {
		value := value
		if amount, err := requestedFlyAmount(&value); err == nil {
			t.Fatalf("requestedFlyAmount(%v) = %v, want rejection", value, amount)
		}
	}
	value := 1.239
	if amount, err := requestedFlyAmount(&value); err != nil || amount != 1.24 {
		t.Fatalf("requestedFlyAmount(1.239) = %v, %v; want 1.24", amount, err)
	}
}

func TestSafeAddInt64RejectsLedgerOverflow(t *testing.T) {
	if total, ok := safeAddInt64(maxSignedInt64-10, 10); !ok || total != maxSignedInt64 {
		t.Fatalf("safe boundary add = %d/%v", total, ok)
	}
	if _, ok := safeAddInt64(maxSignedInt64, 1); ok {
		t.Fatal("positive int64 overflow was accepted")
	}
	if _, ok := safeAddInt64(minSignedInt64, -1); ok {
		t.Fatal("negative int64 overflow was accepted")
	}
	if total, ok := safeAddInt64(100, -25); !ok || total != 75 {
		t.Fatalf("ordinary signed add = %d/%v", total, ok)
	}
}

func TestValidateFiveBallAndThreeBallCatalogPlays(t *testing.T) {
	fiveBall := &lottery.Game{ID: "bingo-ssc-1", Name: "宾果时时彩(一)", Category: "时时彩"}
	threeBall := &lottery.Game{ID: "official-fc3d", Name: "福彩3D", Category: "全国彩"}

	valid := []struct {
		game      *lottery.Game
		playCode  string
		position  int
		selection string
	}{
		{fiveBall, "ball_1_5", 5, "0"},
		{fiveBall, "two_sided", 3, "双"},
		{fiveBall, "dragon_tiger", 1, "虎"},
		{fiveBall, "dragon_tiger_tie", 1, "和"},
		{fiveBall, "leopard", 1, "豹子"},
		{fiveBall, "straight", 2, "yes"},
		{fiveBall, "pair", 3, "对子"},
		{threeBall, "ball_1_5", 3, "9"},
		{threeBall, "dragon_tiger", 1, "龙"},
		{threeBall, "pair", 1, "对子"},
		{threeBall, "half_straight", 1, "中"},
		{threeBall, "mixed", 1, "mixed"},
	}
	for _, item := range valid {
		if err := validateBetChoice(item.game, item.playCode, item.position, item.selection); err != nil {
			t.Fatalf("valid %s/%d/%s for %s rejected: %v", item.playCode, item.position, item.selection, item.game.Name, err)
		}
	}
}

func TestValidateBetChoiceRejectsCatalogAndShapeConfusion(t *testing.T) {
	fiveBall := &lottery.Game{ID: "bingo-ssc-1", Name: "宾果时时彩(一)", Category: "时时彩"}
	racing := &lottery.Game{ID: "speed-racing", Name: "极速赛车", Category: "赛车"}
	invalid := []struct {
		game      *lottery.Game
		playCode  string
		position  int
		selection string
	}{
		{fiveBall, "unknown", 1, "1"},
		{fiveBall, "leopard", 1, "straight"},
		{fiveBall, "straight", 1, "豹子"},
		{fiveBall, "ball_1_5", 1, "10"},
		{fiveBall, "ball_1_5", 1, "1e0"},
		{fiveBall, "two_sided", 1, "龙"},
		{fiveBall, "dragon_tiger", 3, "龙"},
		{fiveBall, "dragon_tiger", 2, "虎"},
		{fiveBall, "dragon_tiger_tie", 1, "龙"},
		{fiveBall, "dragon_tiger_tie", 2, "和"},
		{fiveBall, "sum", 6, "大"},
		{fiveBall, "sum", 6, "10"},
		{&lottery.Game{ID: "official-fc3d", Name: "福彩3D", Category: "全国彩"}, "ball_1_5", 4, "1"},
		{&lottery.Game{ID: "official-fc3d", Name: "福彩3D", Category: "全国彩"}, "dragon_tiger", 2, "龙"},
		{racing, "leopard", 1, "豹子"},
	}
	for _, item := range invalid {
		if err := validateBetChoice(item.game, item.playCode, item.position, item.selection); err == nil {
			t.Fatalf("unsafe %s/%d/%s for %s was accepted", item.playCode, item.position, item.selection, item.game.Name)
		}
	}
}

func TestNormalizeBetSelectionPreventsEquivalentKeyVariants(t *testing.T) {
	fiveBall := &lottery.Game{ID: "bingo-ssc-1", Name: "宾果时时彩(一)", Category: "时时彩"}
	racing := &lottery.Game{ID: "speed-racing", Name: "极速赛车", Category: "赛车"}
	tests := []struct {
		game      *lottery.Game
		playCode  string
		selection string
		want      string
	}{
		{racing, "ball_1_5", "01", "1"},
		{racing, "ball_1_5", "0", "10"},
		{racing, "two_sided", "BIG", "大"},
		{racing, "dragon_tiger", "dragon", "龙"},
		{racing, "sum", "03", "3"},
		{fiveBall, "leopard", "豹子", "leopard"},
		{fiveBall, "leopard", "yes", "leopard"},
		{fiveBall, "leopard", "straight", "straight"},
	}
	for _, test := range tests {
		if got := normalizeBetSelection(test.game, test.playCode, test.selection); got != test.want {
			t.Fatalf("normalize %s/%q = %q, want %q", test.playCode, test.selection, got, test.want)
		}
	}
}
