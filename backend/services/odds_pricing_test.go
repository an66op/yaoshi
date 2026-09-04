package services

import (
	apperrors "backend/errors"
	"testing"
)

func TestBingoRacingOddsPricingCodesAreSelectionSpecific(t *testing.T) {
	for _, gameID := range []string{bingoRacingAGameID, bingoRacingBGameID} {
		for selection, want := range map[string]string{
			"大": "sum_big", "小": "sum_small", "单": "sum_odd", "双": "sum_even",
			"3": "sum_3", "11": "sum_11", "19": "sum_19",
		} {
			got, err := oddsPricingCode(gameID, "sum", selection)
			if err != nil || got != want {
				t.Fatalf("%s selection %q: got %q/%v, want %q", gameID, selection, got, err, want)
			}
		}
		if _, err := oddsPricingCode(gameID, "sum", "20"); apperrors.GetErrorCode(err) != "INVALID_REQUEST" {
			t.Fatalf("%s invalid crown sum produced a pricing row: %v", gameID, err)
		}
	}
	if got, err := oddsPricingCode("speed-racing", "sum", "大"); err != nil || got != "sum" {
		t.Fatalf("ordinary racing pricing changed: %q/%v", got, err)
	}
}

func TestBingoRacingOddsCatalogsHaveNoFlatSumFallback(t *testing.T) {
	for _, gameID := range []string{bingoRacingAGameID, bingoRacingBGameID} {
		items := PlayCatalogForGame(gameID)
		want := map[string]bool{
			"sum_big": false, "sum_small": false, "sum_odd": false, "sum_even": false,
			"sum_3": false, "sum_11": false, "sum_19": false,
		}
		for _, item := range items {
			if item.PlayCode == "sum" {
				t.Fatalf("generic sum row can incorrectly open every %s outcome", gameID)
			}
			if _, ok := want[item.PlayCode]; ok {
				want[item.PlayCode] = true
			}
		}
		if len(items) != 24 {
			t.Fatalf("%s odds catalog has %d items, want 24", gameID, len(items))
		}
		for code, found := range want {
			if !found {
				t.Fatalf("%s missing selection price %s", gameID, code)
			}
		}
	}
}
