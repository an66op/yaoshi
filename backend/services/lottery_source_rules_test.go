package services

import (
	"backend/data/models/lottery"
	"reflect"
	"testing"
	"time"
)

func TestSourceDrawValidationUsesExplicitDigitProfiles(t *testing.T) {
	for _, test := range []struct {
		gameID string
		valid  []int
		bad    [][]int
	}{
		{"speed-ssc", []int{0, 9, 9, 1, 2}, [][]int{{1, 2, 3, 4}, {1, 2, 3, 4, 5, 6}, {1, 2, 3, 4, 10}, {1, 2, 3, 4, -1}}},
		{"official-fc3d", []int{0, 9, 9}, [][]int{{1, 2}, {1, 2, 3, 4}, {1, 2, 10}}},
		{"official-pl3", []int{0, 0, 0}, [][]int{{1, 2}, {1, 2, 3, 4}, {-1, 2, 3}}},
	} {
		game := lottery.Game{ID: test.gameID, Name: "renamed racing", Category: "赛车"}
		valid := sourceDraw{Issue: "123", Numbers: test.valid, DrawAt: time.Unix(1_700_000_000, 0)}
		if err := validateSourceDrawBatch(game, []sourceDraw{valid}); err != nil {
			t.Fatalf("game=%s valid numbers rejected: %v", game.ID, err)
		}
		for _, bad := range test.bad {
			invalid := sourceDraw{Issue: "124", Numbers: bad, DrawAt: valid.DrawAt.Add(time.Minute)}
			if err := validateSourceDrawBatch(game, []sourceDraw{valid, invalid}); err == nil {
				t.Fatalf("game=%s malformed later batch row accepted: %v", game.ID, bad)
			}
		}
	}
}

func TestSourceBingoProfilesValidateTransformedNotRawTwentyBalls(t *testing.T) {
	raw := []int{5, 7, 8, 9, 11, 14, 16, 21, 23, 27, 30, 32, 44, 46, 66, 67, 68, 70, 71, 80}
	for _, binding := range api168BingoBindings {
		game := lottery.Game{ID: binding.GameID}
		if _, modelled := rulesForGame(&game); !modelled {
			continue
		}
		if err := validateOfficialDraws(game, []sourceDraw{{Issue: "123", Numbers: raw}}); err == nil {
			t.Fatalf("game=%s accepted raw 20-ball data as its final draw", game.ID)
		}
		transformed := binding.Transform(raw)
		if err := validateOfficialDraws(game, []sourceDraw{{Issue: "123", Numbers: transformed}}); err != nil {
			t.Fatalf("game=%s transformed=%v err=%v", game.ID, transformed, err)
		}
	}
	if got := bingoMarkSixNumbers(raw); !reflect.DeepEqual(got, []int{5, 7, 8, 9, 11, 14, 16}) {
		t.Fatalf("Mark Six source-order filter changed: %v", got)
	}
}

func TestSourceUnmodelledHistoryNeverUsesRacingShapeByNameOrLength(t *testing.T) {
	raw := make([]int, 20)
	for i := range raw {
		raw[i] = i + 1
	}
	for _, gameID := range []string{"official-kl8", "official-tw-bingo", "unknown"} {
		game := lottery.Game{ID: gameID, Name: "极速赛车", Category: "赛车"}
		if err := validateSourceDrawBatch(game, []sourceDraw{{Issue: "123", Numbers: raw, DrawAt: time.Unix(1_700_000_000, 0)}}); err != nil {
			t.Fatalf("unmodelled history was incorrectly treated as racing: %v", err)
		}
		if _, _, err := evaluateBetForRuleVersion(&game, "", raw, "sum", 6, "小"); err == nil {
			t.Fatalf("unmodelled history unexpectedly allowed financial settlement: %s", gameID)
		}
	}
}
