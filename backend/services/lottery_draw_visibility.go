package services

import (
	"strings"

	"gorm.io/gorm"
)

// trustedDrawsForGame is the public/lifecycle read boundary for lottery draws.
//
// Ordered Bingo products may retain a legacy draw so an already-settled period
// remains auditable. Such a row is deliberately not promoted to the current
// dual-source revision when it conflicts with the verified result. It must not,
// however, be presented as an authoritative result or drive a live lifecycle.
// Administrative reconciliation code intentionally reads lottery_draws
// directly and therefore still sees every retained row.
func trustedDrawsForGame(db *gorm.DB, gameID string) *gorm.DB {
	gameID = strings.TrimSpace(gameID)
	query := db.Where("game_id = ?", gameID)
	sourceRevision, conversionRevision, ordered := trustedDrawRevision(gameID)
	if !ordered {
		return query
	}
	return query.Where(
		"source_revision = ? AND conversion_revision = ?",
		sourceRevision,
		conversionRevision,
	)
}

func trustedDrawRevision(gameID string) (sourceRevision, conversionRevision string, ordered bool) {
	binding, found := api168BingoBindingForGame(strings.TrimSpace(gameID))
	if !found || !binding.RequiresOrderedSource {
		return "", "", false
	}
	return bingoOrderedSourceRevision, binding.ConversionVersion, true
}
