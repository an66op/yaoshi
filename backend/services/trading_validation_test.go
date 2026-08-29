package services

import (
	"math"
	"testing"
)

func TestTradingNumericValidationRejectsNonFiniteValues(t *testing.T) {
	for _, value := range []float64{0, 25.5, 100} {
		if !isFinitePercent(value) {
			t.Fatalf("isFinitePercent(%v) = false, want true", value)
		}
	}
	for _, value := range []float64{-0.01, 100.01, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if isFinitePercent(value) {
			t.Fatalf("isFinitePercent(%v) = true, want false", value)
		}
	}
	for _, value := range []float64{1, 0, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if isValidOddsOverride(value) {
			t.Fatalf("isValidOddsOverride(%v) = true, want false", value)
		}
	}
	if !isValidOddsOverride(1.001) {
		t.Fatal("a finite odds override above one was rejected")
	}
}
