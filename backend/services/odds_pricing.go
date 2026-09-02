package services

import (
	apperrors "backend/errors"
	"fmt"
	"strconv"
	"strings"
)

const bingoRacingAGameID = "bingo-racing-a"

// oddsPricingCode maps a public, settlement-stable play/selection pair to the
// administrator-facing price row.  Only Bingo Racing A needs this extra level:
// all other products retain their historic play-code pricing contract.
func oddsPricingCode(gameID, playCode, selection string) (string, error) {
	gameID = strings.TrimSpace(gameID)
	playCode = strings.ToLower(strings.TrimSpace(playCode))
	if gameID != bingoRacingAGameID || playCode != "sum" {
		return playCode, nil
	}
	normalized := normalizeSelection(selection)
	switch normalized {
	case "big", "small", "odd", "even":
		return "sum_" + normalized, nil
	}
	value, err := strconv.Atoi(normalized)
	if err != nil || value < 3 || value > 19 {
		return "", apperrors.NewBusinessError("INVALID_REQUEST", "宾果赛车(A)冠亚和只能选择3至19或大、小、单、双")
	}
	return fmt.Sprintf("sum_%d", value), nil
}
