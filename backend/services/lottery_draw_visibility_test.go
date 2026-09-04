package services

import (
	"backend/data/models/lottery"
	"reflect"
	"strings"
	"testing"
)

func TestTrustedDrawRevisionUsesCurrent163ContractAndRetainsVerifiedOrderedHistory(t *testing.T) {
	for _, binding := range source163MirrorBindings {
		source, conversion, versioned := trustedDrawRevision(binding.GameID)
		if !versioned || source != binding.Revision || conversion != source163MirrorConversionVersion {
			t.Fatalf("%s current 163 mirror revision = %q/%q/%v", binding.GameID, source, conversion, versioned)
		}
		contracts := trustedDrawRevisionContracts(binding.GameID)
		if len(contracts) != 1 || contracts[0].SourceRevision != binding.Revision || contracts[0].ConversionRevision != source163MirrorConversionVersion {
			t.Fatalf("%s visibility contracts=%+v, want only the current 163 contract", binding.GameID, contracts)
		}
		if !trustedDrawRevisionMatches(binding.GameID, binding.Revision, source163MirrorConversionVersion) ||
			trustedDrawRevisionMatches(binding.GameID, binding.Revision, "") ||
			trustedDrawRevisionMatches(binding.GameID, "legacy-168", source163MirrorConversionVersion) {
			t.Fatalf("%s visibility gate is not an exact source/conversion pair", binding.GameID)
		}
	}
	for _, binding := range source163PC28Bindings {
		source, conversion, versioned := trustedDrawRevision(binding.GameID)
		if !versioned || source != binding.Revision || conversion != source163MirrorConversionVersion {
			t.Fatalf("%s current 163 PC28 revision = %q/%q/%v", binding.GameID, source, conversion, versioned)
		}
		contracts := trustedDrawRevisionContracts(binding.GameID)
		if len(contracts) != 1 || contracts[0].SourceRevision != binding.Revision || contracts[0].ConversionRevision != source163MirrorConversionVersion {
			t.Fatalf("%s visibility contracts=%+v, want only the current 163 contract", binding.GameID, contracts)
		}
	}
	for _, binding := range bingo163Bindings {
		source, conversion, versioned := trustedDrawRevision(binding.GameID)
		if !versioned || source != binding.SourceRevision || conversion != binding.ConversionVersion {
			t.Fatalf("%s current trusted revision = %q/%q/%v", binding.GameID, source, conversion, versioned)
		}
		contracts := trustedDrawRevisionContracts(binding.GameID)
		wantCount := 1
		if bingo163LegacyRequiredOrder(binding.GameID) {
			wantCount = 2
			if !trustedDrawRevisionMatches(binding.GameID, bingoOrderedSourceRevision, binding.ConversionVersion) {
				t.Fatalf("%s no longer trusts verified 168+jyb history", binding.GameID)
			}
		} else if trustedDrawRevisionMatches(binding.GameID, bingoOrderedSourceRevision, binding.ConversionVersion) {
			t.Fatalf("%s accepted an inapplicable ordered legacy contract", binding.GameID)
		}
		if len(contracts) != wantCount {
			t.Fatalf("%s contracts=%+v want count=%d", binding.GameID, contracts, wantCount)
		}
	}
	for _, gameID := range []string{"unknown-mark-six", ""} {
		if source, conversion, versioned := trustedDrawRevision(gameID); versioned || source != "" || conversion != "" {
			t.Fatalf("unrelated game %s gained a Bingo source contract: %q/%q/%v", gameID, source, conversion, versioned)
		}
	}
	for _, binding := range source163MarkSixBindings {
		source, conversion, versioned := trustedDrawRevision(binding.GameID)
		if !versioned || source != binding.SourceRevision || conversion != binding.ConversionRevision {
			t.Fatalf("%s current 163 Mark Six revision = %q/%q/%v", binding.GameID, source, conversion, versioned)
		}
	}
}

func Test163MirrorTrustedDrawQueryUsesOnlyCurrentExactRevision(t *testing.T) {
	for _, binding := range source163MirrorBindings {
		db := robotDryRunDB(t)
		var draws []lottery.Draw
		statement := trustedDrawsForGame(db, binding.GameID).Find(&draws).Statement
		query := statement.SQL.String()
		if !strings.Contains(query, "source_revision =") || !strings.Contains(query, "conversion_revision =") {
			t.Fatalf("%s trusted draw query has no exact revision filter: %s", binding.GameID, query)
		}
		wantVars := []any{binding.GameID, binding.Revision, source163MirrorConversionVersion}
		if !reflect.DeepEqual(statement.Vars, wantVars) {
			t.Fatalf("%s trusted draw query vars=%#v want=%#v", binding.GameID, statement.Vars, wantVars)
		}
	}
}
