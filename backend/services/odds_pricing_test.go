package services

import (
	apperrors "backend/errors"
	"testing"
)

func TestBingoRacingAOddsPricingCodesAreSelectionSpecific(t *testing.T) {
	for selection, want := range map[string]string{
		"大": "sum_big", "小": "sum_small", "单": "sum_odd", "双": "sum_even",
		"3": "sum_3", "11": "sum_11", "19": "sum_19",
	} {
		got, err := oddsPricingCode(bingoRacingAGameID, "sum", selection)
		if err != nil || got != want {
			t.Fatalf("selection %q: got %q/%v, want %q", selection, got, err, want)
		}
	}
	if _, err := oddsPricingCode(bingoRacingAGameID, "sum", "20"); apperrors.GetErrorCode(err) != "INVALID_REQUEST" {
		t.Fatalf("invalid crown sum produced a pricing row: %v", err)
	}
	if got, err := oddsPricingCode("speed-racing", "sum", "大"); err != nil || got != "sum" {
		t.Fatalf("ordinary racing pricing changed: %q/%v", got, err)
	}
}

func TestBingoRacingAOddsCatalogHasNoFlatSumFallback(t *testing.T) {
	items := PlayCatalogForGame(bingoRacingAGameID)
	want := map[string]bool{
		"sum_big": false, "sum_small": false, "sum_odd": false, "sum_even": false,
		"sum_3": false, "sum_11": false, "sum_19": false,
	}
	for _, item := range items {
		if item.PlayCode == "sum" {
			t.Fatal("generic sum row can incorrectly open every Bingo Racing A outcome")
		}
		if _, ok := want[item.PlayCode]; ok {
			want[item.PlayCode] = true
		}
	}
	if len(items) != 24 {
		t.Fatalf("Bingo Racing A odds catalog has %d items, want 24", len(items))
	}
	for code, found := range want {
		if !found {
			t.Fatalf("missing selection price %s", code)
		}
	}
}
