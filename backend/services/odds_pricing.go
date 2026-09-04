package services

import (
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	bingoRacingAGameID = "bingo-racing-a"
	bingoRacingBGameID = "bingo-racing-b"
)

func isBingoRacingGame(gameID string) bool {
	return gameID == bingoRacingAGameID || gameID == bingoRacingBGameID
}

// oddsPricingCode maps a public, settlement-stable play/selection pair to the
// administrator-facing price row. Both Bingo Racing products share the same
// selection-specific crown-sum market. Their draw windows are independent, but
// a flat price for every sum would expose impossible risk and contradict the
// documented A contract; B intentionally adopts it as a platform policy.
func oddsPricingCode(gameID, playCode, selection string) (string, error) {
	gameID = strings.TrimSpace(gameID)
	playCode = strings.ToLower(strings.TrimSpace(playCode))
	if profile, ready := rulesForGame(&lottery.Game{ID: gameID}); ready && profile.MarkSix {
		if primary, _, composite := markSixCompositePricingCodes(playCode); composite {
			return primary, nil
		}
	}
	if !isBingoRacingGame(gameID) || playCode != "sum" {
		return playCode, nil
	}
	normalized := normalizeSelection(selection)
	switch normalized {
	case "big", "small", "odd", "even":
		return "sum_" + normalized, nil
	}
	value, err := strconv.Atoi(normalized)
	if err != nil || value < 3 || value > 19 {
		return "", apperrors.NewBusinessError("INVALID_REQUEST", "宾果赛车冠亚和只能选择3至19或大、小、单、双")
	}
	return fmt.Sprintf("sum_%d", value), nil
}
