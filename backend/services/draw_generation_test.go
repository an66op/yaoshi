package services

import (
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"bytes"
	cryptorand "crypto/rand"
	"errors"
	"io"
	"reflect"
	"testing"
)

func TestGenerateDrawNumbersUsesIndependentDigitsAndRejectionSampling(t *testing.T) {
	for _, test := range []struct {
		name, gameID string
		entropy      []byte
		want         []int
	}{
		{"independent five digits", "sg-ssc", []byte{0, 1, 2, 3, 4}, []int{0, 1, 2, 3, 4}},
		{"duplicates remain legal", "speed-ssc", []byte{0, 0, 0, 0, 0}, []int{0, 0, 0, 0, 0}},
		{"three digits", "official-fc3d", []byte{9, 8, 7}, []int{9, 8, 7}},
		{"pc28 three digits", "pc-canada", []byte{9, 8, 7}, []int{9, 8, 7}},
		{"reject rather than modulo", "speed-ssc", []byte{15, 14, 10, 9, 1, 2, 3, 4}, []int{9, 1, 2, 3, 4}},
		{"unbiased shuffle zero choices", "speed-racing", make([]byte, 10), []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// A misleading display name must not change the ID-bound contract.
			got, err := generateDrawNumbers(&lottery.Game{ID: test.gameID, SourceKind: "platform", Name: "六合彩", Category: "六合彩"}, bytes.NewReader(test.entropy))
			if err != nil || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("got=%v err=%v want=%v", got, err, test.want)
			}
		})
	}
}

type failingDrawEntropy struct{ err error }

func (reader failingDrawEntropy) Read([]byte) (int, error) { return 0, reader.err }

func TestGenerateDrawNumbersFailsClosedOnEntropyFailure(t *testing.T) {
	failure := errors.New("entropy unavailable")
	for _, gameID := range []string{"speed-racing", "speed-ssc", "official-pl3"} {
		for _, test := range []struct {
			name   string
			reader io.Reader
			cause  error
		}{
			{"reader failure", failingDrawEntropy{failure}, failure},
			{"partial result discarded", bytes.NewReader([]byte{1}), io.EOF},
			{"missing entropy", nil, nil},
		} {
			t.Run(gameID+"/"+test.name, func(t *testing.T) {
				got, err := generateDrawNumbers(&lottery.Game{ID: gameID, SourceKind: "platform"}, test.reader)
				if got != nil || err == nil || apperrors.GetErrorCode(err) != "DRAW_RANDOM_FAILED" {
					t.Fatalf("partial/fallback numbers=%v err=%v", got, err)
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("entropy error not retained: %v", err)
				}
			})
		}
	}
}

func TestGenerateDrawNumbersRefusesUnmodelledGames(t *testing.T) {
	for _, gameID := range []string{"happy8-mark-six", "hong-kong-mark-six", "official-kl8", "official-tw-bingo", "bingo-racing-b", "bingo-ssc-2", "bingo-ssc-3", "bingo-ssc-4", "unknown"} {
		got, err := generateDrawNumbers(&lottery.Game{ID: gameID, Name: "极速赛车", Category: "赛车"}, failingDrawEntropy{errors.New("must not read entropy")})
		if got != nil || apperrors.GetErrorCode(err) != "RULES_NOT_READY" {
			t.Fatalf("game=%s got=%v err=%v", gameID, got, err)
		}
	}
}

func TestGenerateDrawNumbersNeverInventsBingoMarkSix(t *testing.T) {
	got, err := generateDrawNumbers(&lottery.Game{ID: "bingo-mark-six"}, failingDrawEntropy{errors.New("must not read entropy")})
	if got != nil || apperrors.GetErrorCode(err) != "DRAW_NOT_FOUND" {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestGenerateDrawNumbersNeverInventsExternalResults(t *testing.T) {
	for _, sourceKind := range []string{"", "official", "external", "unrecognized"} {
		got, err := generateDrawNumbers(&lottery.Game{ID: "official-fc3d", SourceKind: sourceKind}, failingDrawEntropy{errors.New("must not read entropy")})
		if got != nil || apperrors.GetErrorCode(err) != "DRAW_NOT_FOUND" {
			t.Fatalf("source=%s got=%v err=%v", sourceKind, got, err)
		}
	}
	for _, sourceKind := range []string{"platform", "simulated"} {
		got, err := generateDrawNumbers(&lottery.Game{ID: "sg-ssc", SourceKind: sourceKind}, bytes.NewReader([]byte{1, 2, 3, 4, 5}))
		if err != nil || !reflect.DeepEqual(got, []int{1, 2, 3, 4, 5}) {
			t.Fatalf("source=%q got=%v err=%v", sourceKind, got, err)
		}
	}
}

func TestGenerateDrawNumbersCryptoReaderProducesValidProfiles(t *testing.T) {
	for _, gameID := range []string{"speed-racing", "bingo-racing-a", "sg-ssc", "official-fc3d"} {
		game := &lottery.Game{ID: gameID, SourceKind: "platform"}
		profile, _ := rulesForGame(game)
		for i := 0; i < 16; i++ {
			got, err := generateDrawNumbers(game, cryptorand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			if err := profile.validateDraw(got); err != nil {
				t.Fatalf("game=%s numbers=%v err=%v", gameID, got, err)
			}
		}
	}
}
