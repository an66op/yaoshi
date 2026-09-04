package services

import (
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"fmt"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func Test163MirrorTicketRevisionMustMatchDrawRevision(t *testing.T) {
	for _, binding := range source163MirrorBindings {
		t.Run(binding.GameID, func(t *testing.T) {
			issue := fmt.Sprintf("contract-%d", binding.UpstreamGameID)
			if err := betDrawRevisionError(binding.GameID, issue, binding.Revision, binding.Revision); err != nil {
				t.Fatalf("matching placement/draw revisions were rejected: %v", err)
			}
			for _, mismatch := range []struct {
				name, ticket, draw string
			}{
				{name: "blank ticket", ticket: "", draw: binding.Revision},
				{name: "legacy ticket", ticket: "legacy-168-v1", draw: binding.Revision},
				{name: "blank draw", ticket: binding.Revision, draw: ""},
				{name: "different draw", ticket: binding.Revision, draw: "legacy-168-v1"},
			} {
				t.Run(mismatch.name, func(t *testing.T) {
					if err := betDrawRevisionError(binding.GameID, issue, mismatch.ticket, mismatch.draw); err == nil {
						t.Fatalf("accepted mismatched ticket=%q draw=%q", mismatch.ticket, mismatch.draw)
					}
				})
			}
		})
	}
	if err := betDrawRevisionError("unknown-mark-six", "unversioned", "", ""); err != nil {
		t.Fatalf("unversioned game inherited the 163 cutover gate: %v", err)
	}
}

func TestPublishDrawRejectsManualAndRandomWritesForEvery163MirrorGame(t *testing.T) {
	for _, binding := range source163MirrorBindings {
		for _, test := range []struct {
			name    string
			numbers []int
		}{
			{name: "random", numbers: nil},
			{name: "manual", numbers: make([]int, binding.Count)},
		} {
			t.Run(binding.GameID+"/"+test.name, func(t *testing.T) {
				db := robotDryRunDB(t)
				writes := 0
				if err := db.Callback().Query().After("gorm:query").Register("test:163_publish_load_game", func(tx *gorm.DB) {
					if tx.Statement.Table == "lottery_games" {
						*tx.Statement.Dest.(*lottery.Game) = lottery.Game{
							ID: binding.GameID, Enabled: true, SourceKind: "external",
							SourceName: source163MirrorName, SourceURL: source163MirrorURL, SyncStatus: "ok",
						}
					}
				}); err != nil {
					t.Fatal(err)
				}
				for _, callback := range []struct {
					name string
					add  func(string, func(*gorm.DB)) error
				}{
					{name: "create", add: db.Callback().Create().Before("gorm:create").Register},
					{name: "update", add: db.Callback().Update().Before("gorm:update").Register},
					{name: "delete", add: db.Callback().Delete().Before("gorm:delete").Register},
				} {
					if err := callback.add("test:163_publish_no_"+callback.name, func(*gorm.DB) { writes++ }); err != nil {
						t.Fatal(err)
					}
				}
				result, err := NewBetAdminService(db).PublishDraw(binding.GameID, "manual-forbidden", test.numbers, "test")
				if result != nil || apperrors.GetErrorCode(err) != "EXTERNAL_DRAW_MANUAL_FORBIDDEN" {
					t.Fatalf("result=%+v err=%v", result, err)
				}
				if writes != 0 {
					t.Fatalf("rejected publish reached %d write callbacks", writes)
				}
			})
		}
	}
}

func Test163MirrorRecoverySQLContainsCurrentContractsAndTicketIsolation(t *testing.T) {
	query, args := orderedBingoRecoveryRevisionSQL("bets.game_id", "draws")
	for _, fragment := range []string{
		"draws.source_revision = ?",
		"draws.conversion_revision = ?",
		"COALESCE(legacy_bet.draw_source_revision, '') <> draws.source_revision",
		"COALESCE(legacy_archive.draw_source_revision, '') <> draws.source_revision",
		"legacy_issue.source_mode <> 'external'",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("recovery predicate omitted %q: %s", fragment, query)
		}
	}
	flattened := fmt.Sprint(args)
	for _, binding := range source163MirrorBindings {
		for _, value := range []string{binding.GameID, binding.Revision, source163MirrorConversionVersion} {
			if !strings.Contains(flattened, value) {
				t.Fatalf("recovery predicate omitted %s current contract value %q: args=%+v", binding.GameID, value, args)
			}
		}
	}
	if len(args) < 2 {
		t.Fatalf("recovery predicate has no cutover isolation arguments: %+v", args)
	}
	versionedIDs, ok := args[len(args)-2].([]string)
	if !ok {
		t.Fatalf("ticket isolation IDs type=%T want []string", args[len(args)-2])
	}
	currentSources, ok := args[len(args)-1].([]string)
	if !ok {
		t.Fatalf("current source revisions type=%T want []string", args[len(args)-1])
	}
	for _, binding := range source163MirrorBindings {
		if !containsExactString(versionedIDs, binding.GameID) || !containsExactString(currentSources, binding.Revision) {
			t.Fatalf("%s is outside ticket isolation: ids=%v sources=%v", binding.GameID, versionedIDs, currentSources)
		}
	}
}

func Test163MirrorSourceHealthFailsClosedOnAnyBindingMismatch(t *testing.T) {
	for _, binding := range source163MirrorBindings {
		t.Run(binding.GameID, func(t *testing.T) {
			valid := lottery.Game{
				ID: binding.GameID, SourceKind: "external", SourceName: source163MirrorName,
				SourceURL: source163MirrorURL, SyncStatus: "ok",
			}
			if !sourceHealthyForGame(&valid) {
				t.Fatal("exact healthy 163 binding was rejected")
			}
			mutations := []struct {
				name   string
				mutate func(*lottery.Game)
			}{
				{name: "kind", mutate: func(game *lottery.Game) { game.SourceKind = "platform" }},
				{name: "name", mutate: func(game *lottery.Game) { game.SourceName = legacy168HighFreqName }},
				{name: "url", mutate: func(game *lottery.Game) { game.SourceURL = legacy168HighFreqURL }},
				{name: "stale", mutate: func(game *lottery.Game) { game.SyncStatus = "stale" }},
				{name: "error", mutate: func(game *lottery.Game) { game.SyncStatus = "error" }},
				{name: "syncing with error", mutate: func(game *lottery.Game) { game.SyncStatus, game.LastSyncError = "syncing", "upstream failed" }},
			}
			for _, mutation := range mutations {
				t.Run(mutation.name, func(t *testing.T) {
					changed := valid
					mutation.mutate(&changed)
					if sourceHealthyForGame(&changed) {
						t.Fatalf("accepted unsafe binding/state: %+v", changed)
					}
				})
			}
		})
	}
}

func containsExactString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
