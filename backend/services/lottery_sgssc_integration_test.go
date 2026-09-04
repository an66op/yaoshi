package services

import (
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func sgSSCIntegrationBatch(latestAt time.Time) []sourceDraw {
	latestAt = latestAt.UTC().Truncate(sgSSCInterval)
	rows := make([]sourceDraw, sgSSCWindowSize)
	for index := range rows {
		at := latestAt.Add(-time.Duration(sgSSCWindowSize-1-index) * sgSSCInterval)
		rows[index] = sourceDraw{Issue: sgSSCIssueAt(at), DrawAt: at, Numbers: []int{6, 5, 8, 3, 0},
			NextIssue: sgSSCIssueAt(at.Add(sgSSCInterval)), NextDrawAt: at.Add(sgSSCInterval),
			SourceRevision: sgSSCSourceRevision, ConversionRevision: sgSSCConversionRevision}
	}
	return rows
}

func sgSSCIntegrationHealthyGame(now time.Time) lottery.Game {
	batch := sgSSCIntegrationBatch(now)
	latest := batch[len(batch)-1]
	return lottery.Game{ID: "sg-ssc", SourceKind: "external", SourceName: sgSSCVerifiedSourceName, SourceURL: sgSSCVerifiedSourceURL,
		SyncStatus: "ok", LastSyncAt: &now, TimingSource: "upstream", DrawInterval: 300, NextIssue: latest.NextIssue, NextDrawAt: latest.NextDrawAt}
}

func TestSGSSCIntegrationHealthAndSourceBinding(t *testing.T) {
	now := time.Date(2026, 9, 3, 9, 31, 0, 0, time.UTC)
	game := sgSSCIntegrationHealthyGame(now)
	if !sgSSCSourceHealthyAt(&game, now) || len(sgSSCSourceBindingUpdates(game)) != 0 {
		t.Fatal("verified source was not healthy or stable across bootstrap")
	}
	for name, change := range map[string]func(*lottery.Game){
		"platform downgrade":    func(g *lottery.Game) { g.SourceKind = "platform" },
		"other product URL":     func(g *lottery.Game) { g.SourceURL += "?lotCode=10059" },
		"failed source":         func(g *lottery.Game) { g.SyncStatus = "error" },
		"unverified retry":      func(g *lottery.Game) { g.SyncStatus = "syncing"; g.LastSyncError = "mismatch" },
		"never synced":          func(g *lottery.Game) { g.LastSyncAt = nil },
		"invented schedule":     func(g *lottery.Game) { g.TimingSource = "configured" },
		"wrong interval":        func(g *lottery.Game) { g.DrawInterval = 60 },
		"expired period":        func(g *lottery.Game) { g.NextDrawAt = now },
		"missing next issue":    func(g *lottery.Game) { g.NextIssue = "" },
		"mismatched next issue": func(g *lottery.Game) { g.NextIssue = "20260903001" },
		"future schedule":       func(g *lottery.Game) { g.NextDrawAt = now.Add(time.Hour); g.NextIssue = sgSSCIssueAt(g.NextDrawAt) },
	} {
		t.Run(name, func(t *testing.T) {
			copy := game
			change(&copy)
			if sgSSCSourceHealthyAt(&copy, now) {
				t.Fatal("unverified/stale source allowed betting")
			}
		})
	}
	if sgSSCSourceHealthyAt(&game, now.Add(61*time.Second)) || sgSSCSourceHealthyAt(&game, now.Add(-time.Second)) {
		t.Fatal("worker staleness/future timestamps accepted")
	}
	game.SyncStatus = "syncing"
	if !sgSSCSourceHealthyAt(&game, now.Add(time.Second)) {
		t.Fatal("normal in-flight verified retry unnecessarily closed")
	}
	game.SourceKind = "platform"
	updates := sgSSCSourceBindingUpdates(game)
	if updates["next_issue"] != "" || updates["timing_source"] != "pending" || updates["source_kind"] != "external" {
		t.Fatalf("cutover reused the platform schedule: %+v", updates)
	}
	for _, forbidden := range []string{"enabled", "odds_config_revision", "lobby_category", "sort_order"} {
		if _, exists := updates[forbidden]; exists {
			t.Fatalf("cutover changes unrelated configuration: %s", forbidden)
		}
	}
	legacy := lottery.Game{ID: "sg-ssc", SourceKind: "external", SourceName: sgSSCLegacySourceName, SourceURL: sgSSCLegacySourceURL}
	legacyUpdates := sgSSCSourceBindingUpdates(legacy)
	if legacyUpdates["source_name"] != sgSSCVerifiedSourceName || legacyUpdates["source_url"] != sgSSCVerifiedSourceURL || legacyUpdates["sync_status"] != "stale" {
		t.Fatalf("exact legacy binding was not cut over fail-closed: %+v", legacyUpdates)
	}
	custom := lottery.Game{ID: "sg-ssc", SourceKind: "external", SourceName: "operator source", SourceURL: "https://operator.invalid/"}
	if updates := sgSSCSourceBindingUpdates(custom); len(updates) != 0 {
		t.Fatalf("operator source was silently overwritten: %+v", updates)
	}
}

func TestSGSSCCutoverContractsKeepHistoryExactAndNewPlacementCurrent(t *testing.T) {
	source, conversion, versioned := trustedDrawRevision("sg-ssc")
	if !versioned || source != sgSSCSourceRevision || conversion != sgSSCConversionRevision || placementDrawSourceRevision("sg-ssc") != sgSSCSourceRevision {
		t.Fatalf("current placement contract=%q/%q versioned=%v", source, conversion, versioned)
	}
	contracts := trustedDrawRevisionContracts("sg-ssc")
	want := []drawRevisionContract{
		{SourceRevision: sgSSCSourceRevision, ConversionRevision: sgSSCConversionRevision},
		{SourceRevision: sgSSCLegacySourceRevision, ConversionRevision: sgSSCConversionRevision},
	}
	if !reflect.DeepEqual(contracts, want) {
		t.Fatalf("SG visibility contracts=%+v want=%+v", contracts, want)
	}
	for _, revision := range []string{sgSSCSourceRevision, sgSSCLegacySourceRevision} {
		if err := betDrawRevisionError("sg-ssc", "20260903030", revision, revision); err != nil {
			t.Fatalf("matching immutable SG history was rejected: %v", err)
		}
	}
	for _, pair := range [][2]string{{"", sgSSCSourceRevision}, {sgSSCLegacySourceRevision, sgSSCSourceRevision}, {sgSSCSourceRevision, sgSSCLegacySourceRevision}} {
		if err := betDrawRevisionError("sg-ssc", "20260903030", pair[0], pair[1]); err == nil {
			t.Fatalf("cross-cutover ticket/draw pair accepted: %q/%q", pair[0], pair[1])
		}
	}
	db := robotDryRunDB(t)
	var rows []lottery.Draw
	statement := trustedDrawsForGame(db, "sg-ssc").Find(&rows).Statement
	if strings.Count(statement.SQL.String(), "source_revision =") != 2 || !reflect.DeepEqual(statement.Vars, []any{"sg-ssc", sgSSCSourceRevision, sgSSCConversionRevision, sgSSCLegacySourceRevision, sgSSCConversionRevision}) {
		t.Fatalf("SG visibility SQL lost an exact contract: sql=%s vars=%#v", statement.SQL.String(), statement.Vars)
	}
}

func TestSGSSCIntegrationImportScheduleAndManualGuard(t *testing.T) {
	batch := sgSSCIntegrationBatch(time.Date(2026, 9, 3, 16, 0, 0, 0, time.UTC))
	game := lottery.Game{ID: "sg-ssc", DrawInterval: 60}
	if err := validateSourceDrawBatch(game, batch); err != nil {
		t.Fatal(err)
	}
	schedule, err := scheduleFromDraws(game, batch)
	if err != nil || schedule.Source != "upstream" || schedule.Interval != 300 || schedule.Issue != "20260904001" {
		t.Fatalf("midnight schedule guessed or misdated: %+v %v", schedule, err)
	}
	for _, kind := range []string{"", "platform", "simulated", "external", "official"} {
		result, err := generateDrawNumbers(&lottery.Game{ID: "sg-ssc", SourceKind: kind}, failingDrawEntropy{errors.New("must not generate")})
		if result != nil || apperrors.GetErrorCode(err) != "DRAW_NOT_FOUND" {
			t.Fatalf("SG source downgrade generated a draw: kind=%s result=%v err=%v", kind, result, err)
		}
	}
	copy := append([]sourceDraw(nil), batch...)
	copy[0].SourceRevision = ""
	if err := validateSourceDrawBatch(game, copy); err == nil || !reflect.DeepEqual(batch[0].Numbers, []int{6, 5, 8, 3, 0}) {
		t.Fatal("missing verification revision accepted")
	}
}
