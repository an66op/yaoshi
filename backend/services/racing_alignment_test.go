package services

import (
	"backend/data/models/lottery"
	"testing"
)

func TestFiveRacingGamesFreezeRuleAnd168SourceBindings(t *testing.T) {
	wantSources := map[string]string{
		"speed-racing": "10037",
		"speed-fly":    "10035",
		"sg-fly":       "10058",
		"fly-racing":   "10057",
		"au-lucky-10":  "10012",
	}
	gotSources := make(map[string]string, len(wantSources))
	for _, binding := range api168HighFreqBindings {
		if _, inScope := wantSources[binding.GameID]; !inScope {
			continue
		}
		if binding.Series != api168PK10 {
			t.Fatalf("%s uses non-PK10 series %q", binding.GameID, binding.Series)
		}
		if previous, duplicate := gotSources[binding.GameID]; duplicate {
			t.Fatalf("%s has duplicate source bindings %s and %s", binding.GameID, previous, binding.LotCode)
		}
		gotSources[binding.GameID] = binding.LotCode
	}
	for gameID, lotCode := range wantSources {
		if gotSources[gameID] != lotCode {
			t.Fatalf("%s source = %q, want 168 %s", gameID, gotSources[gameID], lotCode)
		}
		profile, ready := rulesForGame(&lottery.Game{ID: gameID, Name: "任意改名", Category: "任意分类"})
		if !ready || profile.Version != "racing-v2" || !profile.Racing || !profile.Unique ||
			profile.BallCount != 10 || profile.MinNumber != 1 || profile.MaxNumber != 10 ||
			profile.PositionBigFrom != 6 || profile.SumBigFrom != 12 || !gameSupportsRuleVersion(gameID, "racing-v2") {
			t.Fatalf("%s racing contract changed: %+v ready=%v", gameID, profile, ready)
		}
		if err := profile.validateDraw([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}); err != nil {
			t.Fatalf("%s rejected a valid racing draw: %v", gameID, err)
		}
		if err := profile.validateDraw([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 9}); err == nil {
			t.Fatalf("%s accepted duplicate racing numbers", gameID)
		}
		if err := profile.validateDraw([]int{0, 2, 3, 4, 5, 6, 7, 8, 9, 10}); err == nil {
			t.Fatalf("%s accepted an out-of-range racing number", gameID)
		}
	}
}
