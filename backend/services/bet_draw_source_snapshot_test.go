package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"encoding/json"
	"reflect"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestPlacementDrawSourceRevisionFreezesVersionedExternalFeeds(t *testing.T) {
	if got := placementDrawSourceRevision("sg-ssc"); got == "" || got != sgSSCSourceRevision {
		t.Fatalf("new SG placement did not freeze the verified source: %q", got)
	}
	for _, binding := range source163MirrorBindings {
		if got := placementDrawSourceRevision(binding.GameID); got != binding.Revision {
			t.Fatalf("new %s placement source=%q want=%q", binding.GameID, got, binding.Revision)
		}
	}
	for _, binding := range source163PC28Bindings {
		if got := placementDrawSourceRevision(binding.GameID); got != binding.Revision {
			t.Fatalf("new %s placement source=%q want=%q", binding.GameID, got, binding.Revision)
		}
	}
	for _, binding := range source163MarkSixBindings {
		if got := placementDrawSourceRevision(binding.GameID); got != binding.SourceRevision {
			t.Fatalf("new %s placement source=%q want=%q", binding.GameID, got, binding.SourceRevision)
		}
	}
	for _, binding := range bingo163Bindings {
		if got := placementDrawSourceRevision(binding.GameID); got != binding.SourceRevision {
			t.Fatalf("new %s placement source=%q want=%q", binding.GameID, got, binding.SourceRevision)
		}
	}
	for _, gameID := range []string{"", "unknown-mark-six"} {
		if got := placementDrawSourceRevision(gameID); got != "" {
			t.Fatalf("unrelated game %q gained a source contract: %q", gameID, got)
		}
	}
}

func TestSGSSCPlacementSourceGateRunsUnderGameLockBeforeWrites(t *testing.T) {
	for _, kind := range []string{"platform", "external", ""} {
		t.Run(kind, func(t *testing.T) {
			db := robotDryRunDB(t)
			var queries, locks []string
			if err := db.Callback().Query().After("gorm:query").Register("test:sg_placement_source", func(tx *gorm.DB) {
				queries = append(queries, tx.Statement.Table)
				if lock, ok := tx.Statement.Clauses["FOR"].Expression.(clause.Locking); ok {
					locks = append(locks, tx.Statement.Table+":"+lock.Strength)
				}
				if tx.Statement.Table == "lottery_games" {
					*tx.Statement.Dest.(*lottery.Game) = lottery.Game{ID: "sg-ssc", Enabled: true, SourceKind: kind, SyncStatus: "ok"}
				}
			}); err != nil {
				t.Fatal(err)
			}
			if err := lockBettingIssue(db, "sg-ssc", "20260903023"); apperrors.GetErrorCode(err) != "SOURCE_UNAVAILABLE" {
				t.Fatalf("unverified binding passed the final placement gate: %v", err)
			}
			if !reflect.DeepEqual(queries, []string{"lottery_games"}) || !reflect.DeepEqual(locks, []string{"lottery_games:SHARE"}) {
				t.Fatalf("source failure reached issue/wallet work or lacked game lock: queries=%v locks=%v", queries, locks)
			}
		})
	}
}

func TestBetViewRetainsPersistedDrawSourceRevision(t *testing.T) {
	for _, revision := range []string{"", "historic-source-v0", sgSSCSourceRevision} {
		row := bet.Bet{GameID: "sg-ssc", DrawSourceRevision: revision}
		view := toBetView(row)
		if view.DrawSourceRevision != revision || row.DrawSourceRevision != revision {
			t.Fatalf("view inferred/replaced stored source %q: %+v", revision, view)
		}
		encoded, err := json.Marshal(view)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatal(err)
		}
		value, exists := fields["draw_source_revision"]
		if revision == "" && exists || revision != "" && (!exists || value != revision) {
			t.Fatalf("source snapshot JSON changed %q: %s", revision, encoded)
		}
	}
}
