package services

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// trustedDrawsForGame is the public/lifecycle read boundary for lottery draws.
//
// Verified-source products may retain a legacy draw so an already-settled period
// remains auditable. Such a row is deliberately not promoted to the current
// dual-source revision when it conflicts with the verified result. It must not,
// however, be presented as an authoritative result or drive a live lifecycle.
// Administrative reconciliation code intentionally reads lottery_draws
// directly and therefore still sees every retained row.
func trustedDrawsForGame(db *gorm.DB, gameID string) *gorm.DB {
	gameID = strings.TrimSpace(gameID)
	query := db.Where("game_id = ?", gameID)
	contracts := trustedDrawRevisionContracts(gameID)
	if len(contracts) == 0 {
		return query
	}
	clauses := make([]string, 0, len(contracts))
	args := make([]any, 0, len(contracts)*2)
	for _, contract := range contracts {
		clauses = append(clauses, "(source_revision = ? AND conversion_revision = ?)")
		args = append(args, contract.SourceRevision, contract.ConversionRevision)
	}
	return query.Where("("+strings.Join(clauses, " OR ")+")", args...)
}

type drawRevisionContract struct {
	SourceRevision     string
	ConversionRevision string
}

// trustedDrawRevision returns the currently accepted placement contract. The
// complete read/settlement allowlist lives in trustedDrawRevisionContracts so
// a source cutover can retain immutable, already verified history without
// treating the old provider as the source for newly accepted tickets.
func trustedDrawRevision(gameID string) (sourceRevision, conversionRevision string, ordered bool) {
	gameID = strings.TrimSpace(gameID)
	if gameID == "sg-ssc" {
		return sgSSCSourceRevision, sgSSCConversionRevision, true
	}
	if binding, found := source163MirrorBindingForGame(gameID); found {
		return binding.Revision, source163MirrorConversionVersion, true
	}
	if binding, found := source163PC28BindingForGame(gameID); found {
		return binding.Revision, source163MirrorConversionVersion, true
	}
	if binding, found := source163MarkSixBindingForGame(gameID); found {
		return binding.SourceRevision, binding.ConversionRevision, true
	}
	binding, found := bingo163BindingForGame(gameID)
	if !found {
		return "", "", false
	}
	return binding.SourceRevision, binding.ConversionVersion, true
}

func trustedDrawRevisionContracts(gameID string) []drawRevisionContract {
	gameID = strings.TrimSpace(gameID)
	if gameID == "sg-ssc" {
		return []drawRevisionContract{
			{SourceRevision: sgSSCSourceRevision, ConversionRevision: sgSSCConversionRevision},
			{SourceRevision: sgSSCLegacySourceRevision, ConversionRevision: sgSSCConversionRevision},
		}
	}
	if binding, found := source163MirrorBindingForGame(gameID); found {
		return []drawRevisionContract{{SourceRevision: binding.Revision, ConversionRevision: source163MirrorConversionVersion}}
	}
	if binding, found := source163PC28BindingForGame(gameID); found {
		return []drawRevisionContract{{SourceRevision: binding.Revision, ConversionRevision: source163MirrorConversionVersion}}
	}
	if binding, found := source163MarkSixBindingForGame(gameID); found {
		return []drawRevisionContract{{SourceRevision: binding.SourceRevision, ConversionRevision: binding.ConversionRevision}}
	}
	binding, found := bingo163BindingForGame(gameID)
	if !found {
		return nil
	}
	contracts := []drawRevisionContract{{SourceRevision: binding.SourceRevision, ConversionRevision: binding.ConversionVersion}}
	if legacy, ok := api168BingoBindingForGame(gameID); ok && legacy.RequiresOrderedSource {
		contracts = append(contracts, drawRevisionContract{
			SourceRevision:     bingoOrderedSourceRevision,
			ConversionRevision: legacy.ConversionVersion,
		})
	}
	return contracts
}

func trustedDrawRevisionMatches(gameID, sourceRevision, conversionRevision string) bool {
	contracts := trustedDrawRevisionContracts(gameID)
	for _, contract := range contracts {
		if sourceRevision == contract.SourceRevision && conversionRevision == contract.ConversionRevision {
			return true
		}
	}
	return len(contracts) == 0
}

func currentDrawRevisionMatches(gameID, sourceRevision, conversionRevision string) bool {
	wantSource, wantConversion, required := trustedDrawRevision(gameID)
	return !required || sourceRevision == wantSource && conversionRevision == wantConversion
}

// betDrawRevisionError protects the cutover boundary. Legacy ordered Bingo
// history keeps its original trusted contract and can settle its historical
// tickets; every draw written by the new 163 mother source requires an exact
// placement snapshot, so an old/blank ticket cannot silently change identity.
func betDrawRevisionError(gameID, issue, ticketRevision string, drawSourceRevision string) error {
	gameID = strings.TrimSpace(gameID)
	if gameID == "sg-ssc" {
		if ticketRevision == drawSourceRevision && (drawSourceRevision == sgSSCSourceRevision || drawSourceRevision == sgSSCLegacySourceRevision) {
			return nil
		}
		return fmt.Errorf("第 %s 期注单与开奖来源版本不一致", issue)
	}
	if binding, found := source163MirrorBindingForGame(gameID); found {
		if drawSourceRevision == binding.Revision && ticketRevision == binding.Revision {
			return nil
		}
		return fmt.Errorf("第 %s 期注单与163母源开奖版本不一致", issue)
	}
	if binding, found := source163PC28BindingForGame(gameID); found {
		if drawSourceRevision == binding.Revision && ticketRevision == binding.Revision {
			return nil
		}
		return fmt.Errorf("第 %s 期注单与163加拿大28母源开奖版本不一致", issue)
	}
	if binding, found := source163MarkSixBindingForGame(gameID); found {
		if drawSourceRevision == binding.SourceRevision && ticketRevision == binding.SourceRevision {
			return nil
		}
		return fmt.Errorf("第 %s 期注单与163六合彩母源开奖版本不一致", issue)
	}
	binding, found := bingo163BindingForGame(gameID)
	if !found {
		return nil
	}
	if drawSourceRevision == bingoOrderedSourceRevision {
		// Before this cutover Bingo tickets did not persist a source snapshot.
		// The old dual-source result is immutable and remains safe for those rows.
		if ticketRevision == "" || ticketRevision == bingoOrderedSourceRevision {
			return nil
		}
		return fmt.Errorf("第 %s 期注单与旧版双源开奖不一致", issue)
	}
	if drawSourceRevision == binding.SourceRevision && ticketRevision == binding.SourceRevision {
		return nil
	}
	return fmt.Errorf("第 %s 期注单与163母源开奖版本不一致", issue)
}
