package services

import (
	"fmt"
	"testing"
	"time"

	"backend/data/models/lottery"
)

func Test163PC28CurrentContractsFailClosedAndFreezeTickets(t *testing.T) {
	for _, binding := range source163PC28Bindings {
		t.Run(binding.GameID, func(t *testing.T) {
			source, conversion, versioned := trustedDrawRevision(binding.GameID)
			if !versioned || source != binding.Revision || conversion != source163MirrorConversionVersion {
				t.Fatalf("current contract=%q/%q/%v", source, conversion, versioned)
			}
			if got := placementDrawSourceRevision(binding.GameID); got != binding.Revision {
				t.Fatalf("placement revision=%q want=%q", got, binding.Revision)
			}
			if !trustedDrawRevisionMatches(binding.GameID, binding.Revision, source163MirrorConversionVersion) {
				t.Fatal("current PC28 draw contract is not trusted")
			}
			if trustedDrawRevisionMatches(binding.GameID, "", "") || trustedDrawRevisionMatches(binding.GameID, binding.Revision, "") {
				t.Fatal("blank or partial PC28 draw contract was trusted")
			}

			issue := fmt.Sprintf("pc28-%d", binding.UpstreamGameID)
			if err := betDrawRevisionError(binding.GameID, issue, binding.Revision, binding.Revision); err != nil {
				t.Fatalf("matching ticket/draw rejected: %v", err)
			}
			if err := betDrawRevisionError(binding.GameID, issue, "", binding.Revision); err == nil {
				t.Fatal("blank legacy ticket acquired the new PC28 source identity")
			}
		})
	}
}

func Test163PC28DefaultCatalogUsesOneVerifiedSourceAndCadence(t *testing.T) {
	now := time.Date(2026, time.September, 4, 8, 0, 0, 0, time.UTC)
	seen := map[string]bool{}
	for index, item := range defaultGames {
		binding, ok := source163PC28BindingForGame(item.ID)
		if !ok {
			continue
		}
		seen[item.ID] = true
		if item.Interval != source163PC28Interval {
			t.Fatalf("%s seed interval=%d want=%d", item.ID, item.Interval, source163PC28Interval)
		}
		game := defaultGameCatalogRow(item, index, now)
		if !source163PC28Bound(&game, binding) || game.DrawInterval != source163PC28Interval ||
			game.SyncStatus != "stale" || game.LastSyncError != source163MirrorPendingMessage || sourceHealthyForGame(&game) {
			t.Fatalf("%s unsafe default: %+v", item.ID, game)
		}
	}
	if len(seen) != len(source163PC28Bindings) {
		t.Fatalf("seed coverage=%d want=%d", len(seen), len(source163PC28Bindings))
	}
}

func Test163PC28HealthRejectsBindingDrift(t *testing.T) {
	for _, binding := range source163PC28Bindings {
		valid := lottery.Game{ID: binding.GameID, SourceKind: "external", SourceName: source163MirrorName, SourceURL: source163MirrorURL, SyncStatus: "ok"}
		if !sourceHealthyForGame(&valid) {
			t.Fatalf("%s valid binding rejected", binding.GameID)
		}
		changed := valid
		changed.SourceName = "another source"
		if sourceHealthyForGame(&changed) {
			t.Fatalf("%s accepted changed source binding", binding.GameID)
		}
		changed = valid
		changed.SyncStatus = "error"
		if sourceHealthyForGame(&changed) {
			t.Fatalf("%s accepted failed source", binding.GameID)
		}
	}
}
