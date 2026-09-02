package services

import "testing"

func TestTrustedDrawRevisionAppliesOnlyToOrderedBingoProducts(t *testing.T) {
	for gameID, conversion := range map[string]string{
		"bingo-racing-a": bingoRacingAConversionVersion,
		"bingo-ssc-1":    bingoSSC1ConversionVersion,
		"bingo-mark-six": bingoMarkSixConversionVersion,
	} {
		source, gotConversion, ordered := trustedDrawRevision(gameID)
		if !ordered || source != bingoOrderedSourceRevision || gotConversion != conversion {
			t.Fatalf("%s trusted revision = %q/%q/%v", gameID, source, gotConversion, ordered)
		}
	}
	for _, gameID := range []string{
		"bingo-racing-b", "bingo-ssc-2", "bingo-ssc-3", "bingo-ssc-4", "speed-racing", "",
	} {
		if source, conversion, ordered := trustedDrawRevision(gameID); ordered || source != "" || conversion != "" {
			t.Fatalf("independent game %s was filtered as ordered: %q/%q/%v", gameID, source, conversion, ordered)
		}
	}
}
