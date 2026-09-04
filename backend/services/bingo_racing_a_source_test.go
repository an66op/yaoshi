package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"backend/data/models/lottery"
)

var bingoOrderedIssue115049561 = []int{10, 39, 59, 2, 47, 74, 45, 3, 55, 19, 69, 23, 63, 35, 26, 42, 27, 18, 9, 80}

func bingoOrderedFixtureJSON(t *testing.T, issue string, numbers []int) []byte {
	t.Helper()
	row := bingoOrderedHistoryRow{
		Period: issue, DrawTime: "2026-09-02T01:26:13.000Z",
		Numbers: append([]int(nil), numbers...), SuperNumber: numbers[len(numbers)-1],
		SumVal: 745, SumSize: "Small", SumParity: "Odd", SuperSize: "Big",
		SuperParity: "Even", PlateUpDown: "Up", PlateOddEven: "Odd",
	}
	body, err := json.Marshal([]bingoOrderedHistoryRow{row})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func bingo168SortedSourceFixture(t *testing.T) sourceDraw {
	t.Helper()
	row := api168Row{
		Issue: "115049561", Time: "2026-09-02 09:25:00",
		Code:      "02,03,09,10,18,19,23,26,27,35,39,42,45,47,55,59,63,69,74,80,80",
		NextIssue: "115049562", NextTime: "2026-09-02 09:29:07",
	}
	draws, err := sourceDrawsFrom168Payload(bingo168Payload(t, row), api168KL8, "10047", nil)
	if err != nil || len(draws) != 1 {
		t.Fatalf("168 fixture rejected: rows=%+v err=%v", draws, err)
	}
	return draws[0]
}

func TestBingoOrderedHistoryFetcherIsBoundedInjectableAndStrict(t *testing.T) {
	calls := 0
	draws, err := fetchBingoOrderedHistory(context.Background(), func(_ context.Context, endpoint string) ([]byte, error) {
		calls++
		parsed, parseErr := url.Parse(endpoint)
		if parseErr != nil || parsed.Scheme != "https" || parsed.Host != "jyb.one" || parsed.Path != "/api/history" || parsed.Query().Get("limit") != "500" {
			t.Fatalf("unexpected ordered-source endpoint: %s (%v)", endpoint, parseErr)
		}
		return bingoOrderedFixtureJSON(t, "115049561", bingoOrderedIssue115049561), nil
	})
	if err != nil || calls != 1 || len(draws) != 1 || draws[0].Issue != "115049561" || !reflect.DeepEqual(draws[0].Numbers, bingoOrderedIssue115049561) {
		t.Fatalf("ordered source not parsed exactly: calls=%d rows=%+v err=%v", calls, draws, err)
	}
	if draws[0].DrawAt != time.Date(2026, 9, 2, 1, 26, 13, 0, time.UTC) {
		t.Fatalf("ordered draw time changed: %s", draws[0].DrawAt)
	}

	for _, test := range []struct {
		name string
		body []byte
	}{
		{"malformed", []byte(`[{`)},
		{"trailing json", append(bingoOrderedFixtureJSON(t, "115049561", bingoOrderedIssue115049561), []byte(` {}`)...)},
		{"unknown schema", []byte(`[{
			"period":"115049561","drawTime":"2026-09-02T01:26:13Z","numbers":[10,39,59,2,47,74,45,3,55,19,69,23,63,35,26,42,27,18,9,80],
			"superNumber":80,"sumVal":745,"sumSize":"Small","sumParity":"Odd","superSize":"Big","superParity":"Even","plateUpDown":"Up","plateOddEven":"Odd","unexpected":true
		}]`)},
		{"empty", []byte(`[]`)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got, parseErr := parseBingoOrderedHistory(test.body); parseErr == nil || got != nil {
				t.Fatalf("unsafe ordered response accepted: rows=%+v err=%v", got, parseErr)
			}
		})
	}
}

func TestBingoOrderedHistoryRejectsInvalidIssueBallsAndSuperNumber(t *testing.T) {
	valid := bingoOrderedIssue115049561
	tests := []struct {
		name    string
		period  string
		numbers []int
		super   int
	}{
		{"invalid issue", "11504x561", valid, 80},
		{"nineteen balls", "115049561", valid[:19], 9},
		{"duplicate ball", "115049561", append(append([]int(nil), valid[:19]...), valid[0]), valid[0]},
		{"out of range", "115049561", append(append([]int(nil), valid[:19]...), 81), 81},
		{"wrong super", "115049561", valid, 79},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			row := bingoOrderedHistoryRow{
				Period: test.period, DrawTime: "2026-09-02T01:26:13Z", Numbers: test.numbers, SuperNumber: test.super,
				SumVal: 745, SumSize: "Small", SumParity: "Odd", SuperSize: "Big", SuperParity: "Even", PlateUpDown: "Up", PlateOddEven: "Odd",
			}
			body, marshalErr := json.Marshal([]bingoOrderedHistoryRow{row})
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if got, parseErr := parseBingoOrderedHistory(body); parseErr == nil || got != nil {
				t.Fatalf("unsafe row accepted: rows=%+v err=%v", got, parseErr)
			}
		})
	}
}

func TestBingoRacingADualSourceCrossCheckAndRankV1(t *testing.T) {
	authoritative := bingo168SortedSourceFixture(t)
	ordered, err := parseBingoOrderedHistory(bingoOrderedFixtureJSON(t, authoritative.Issue, bingoOrderedIssue115049561))
	if err != nil {
		t.Fatal(err)
	}
	verified, err := crossValidate168BingoOrder([]sourceDraw{authoritative}, ordered)
	if err != nil || len(verified) != 1 || !verified[0].BingoOrderVerified || !reflect.DeepEqual(verified[0].Numbers, bingoOrderedIssue115049561) {
		t.Fatalf("matching period/set/tail did not recover ordered raw: rows=%+v err=%v", verified, err)
	}
	if verified[0].NextIssue != authoritative.NextIssue || verified[0].NextDrawAt != authoritative.NextDrawAt || verified[0].DrawAt != authoritative.DrawAt {
		t.Fatalf("168 schedule metadata was not retained: got=%+v want=%+v", verified[0], authoritative)
	}
	wantRanks := []int{3, 5, 9, 1, 7, 10, 6, 2, 8, 4}
	if got := bingoRacingARankV1Numbers(verified[0].Numbers); !reflect.DeepEqual(got, wantRanks) {
		t.Fatalf("%s rank mapping = %v, want %v", bingoRacingAConversionVersion, got, wantRanks)
	}
	if got := bingoRacingARankV1Numbers(authoritative.Numbers); !reflect.DeepEqual(got, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}) {
		t.Fatalf("fixture no longer proves why the sorted 168 set cannot supply draw order: %v", got)
	}
	if got, transformErr := transform168BingoDraws("bingo-racing-a", []sourceDraw{authoritative}, bingoRacingARankV1Numbers); !errors.Is(transformErr, err168BingoOrderMismatch) || got != nil {
		t.Fatalf("sorted 168 result bypassed the ordered-source gate: rows=%+v err=%v", got, transformErr)
	}
	if got, transformErr := transform168BingoDraws("bingo-racing-a", verified, bingoRacingARankV1Numbers); transformErr != nil || len(got) != 1 ||
		!reflect.DeepEqual(got[0].Numbers, wantRanks) || got[0].SourceRevision != bingoOrderedSourceRevision ||
		got[0].ConversionRevision != bingoRacingAConversionVersion {
		t.Fatalf("verified ordered result did not transform: rows=%+v err=%v", got, transformErr)
	}
}

func TestBingoOrderCrossCheckFailsClosedForEveryAuthorityAnomaly(t *testing.T) {
	authoritative := bingo168SortedSourceFixture(t)
	ordered, err := parseBingoOrderedHistory(bingoOrderedFixtureJSON(t, authoritative.Issue, bingoOrderedIssue115049561))
	if err != nil {
		t.Fatal(err)
	}
	missing := append([]sourceDraw(nil), ordered...)
	missing[0].Issue = "115049560"
	setMismatch := append([]sourceDraw(nil), ordered...)
	setMismatch[0].Numbers = append([]int(nil), setMismatch[0].Numbers...)
	setMismatch[0].Numbers[0] = 1
	noTail := authoritative
	noTail.HasBingoSourceTail = false
	wrongTail := authoritative
	wrongTail.BingoSourceTail = 79
	for _, test := range []struct {
		name      string
		authority []sourceDraw
		ordered   []sourceDraw
	}{
		{"missing issue", []sourceDraw{authoritative}, missing},
		{"set mismatch", []sourceDraw{authoritative}, setMismatch},
		{"missing 168 tail", []sourceDraw{noTail}, ordered},
		{"wrong 168 tail", []sourceDraw{wrongTail}, ordered},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, checkErr := crossValidate168BingoOrder(test.authority, test.ordered)
			if !errors.Is(checkErr, err168BingoOrderMismatch) || got != nil {
				t.Fatalf("unsafe dual-source result accepted: rows=%+v err=%v", got, checkErr)
			}
		})
	}
}

func TestBingoRacingAOnlyOpensOnExplicit163DualSourceAndRacingContracts(t *testing.T) {
	var binding *api168BingoBinding
	for index := range api168BingoBindings {
		if api168BingoBindings[index].GameID == "bingo-racing-a" {
			binding = &api168BingoBindings[index]
			break
		}
	}
	if binding == nil || !binding.RequiresOrderedSource || binding.ConversionVersion != bingoRacingAConversionVersion {
		t.Fatalf("Bingo Racing A lost its ordered/versioned source contract: %+v", binding)
	}
	profile, ready := rulesForGame(&lottery.Game{ID: "bingo-racing-a"})
	if !ready || profile.Version != "racing-v2" || !gameSupportsRuleVersion("bingo-racing-a", "racing-v2") {
		t.Fatalf("Bingo Racing A rule contract not explicitly bound: profile=%+v ready=%v", profile, ready)
	}
	kind, name, sourceURL, status := defaultLotterySource("bingo-racing-a")
	if kind != "external" || name != bingo163OrderedSourceName || sourceURL != bingo163SourceURL || status != "stale" || !strings.Contains(name, "双源") {
		t.Fatalf("Bingo Racing A hides an upstream: %q %q %q %q", kind, name, sourceURL, status)
	}
	if sourceHealthyForGame(&lottery.Game{SourceKind: kind, SyncStatus: status, LastSyncError: bingo163PendingMessage}) {
		t.Fatal("Bingo Racing A became healthy before its first successful dual-source validation")
	}
	if sourceHealthyForGame(&lottery.Game{SourceKind: kind, SyncStatus: "syncing", LastSyncError: bingo163PendingMessage}) {
		t.Fatal("Bingo Racing A reopened while its first dual-source validation was in flight")
	}
	if !sourceHealthyForGame(&lottery.Game{SourceKind: kind, SyncStatus: "ok"}) {
		t.Fatal("Bingo Racing A stayed unavailable after a complete dual-source sync cleared the pending error")
	}
	bKind, bName, bURL, bStatus := defaultLotterySource("bingo-racing-b")
	if bKind != "external" || bName != bingo163OrderedSourceName || bURL != bingo163SourceURL || bStatus != "stale" ||
		sourceHealthyForGame(&lottery.Game{SourceKind: bKind, SyncStatus: bStatus, LastSyncError: bingo163PendingMessage}) {
		t.Fatalf("Bingo Racing B did not start fail-closed on the verified 163 ordered source: %q %q %q %q", bKind, bName, bURL, bStatus)
	}
}

func TestBingoOrderedSourceRevisionUpgradeIsOneWayAndFailClosed(t *testing.T) {
	legacy := lottery.Game{
		ID: "bingo-racing-a", SourceKind: "external", SourceName: "168开奖网",
		SourceURL: "https://kj138138.com/view/api/index.html", SyncStatus: "ok",
	}
	updates, required := bingoOrderedSourceRevisionUpdates(legacy)
	if !required || updates["source_name"] != bingoVerifiedSourceName || updates["source_url"] != bingoVerifiedSourceURL ||
		updates["sync_status"] != "stale" || updates["last_sync_error"] != bingoOrderPendingMessage {
		t.Fatalf("legacy single-source row was not closed for revision upgrade: required=%v updates=%+v", required, updates)
	}

	fresh := lottery.Game{
		ID: "bingo-racing-a", SourceKind: "external", SourceName: bingoVerifiedSourceName,
		SourceURL: bingoVerifiedSourceURL, SyncStatus: "stale",
	}
	updates, required = bingoOrderedSourceRevisionUpdates(fresh)
	if !required || updates["sync_status"] != "stale" || updates["last_sync_error"] != bingoOrderPendingMessage {
		t.Fatalf("fresh dual-source row did not persist its pending gate: required=%v updates=%+v", required, updates)
	}

	for _, game := range []lottery.Game{
		{SourceKind: "external", SourceName: bingoVerifiedSourceName, SourceURL: bingoVerifiedSourceURL, SyncStatus: "ok"},
		{SourceKind: "external", SourceName: bingoVerifiedSourceName, SourceURL: bingoVerifiedSourceURL, SyncStatus: "error", LastSyncError: "ordered source timeout"},
		{SourceKind: "external", SourceName: bingoVerifiedSourceName, SourceURL: bingoVerifiedSourceURL, SyncStatus: "stale", LastSyncError: bingoOrderPendingMessage},
		{SourceKind: "external", SourceName: bingoVerifiedSourceName, SourceURL: bingoVerifiedSourceURL, SyncStatus: "paused", LastSyncError: "operator pause"},
	} {
		if updates, required := bingoOrderedSourceRevisionUpdates(game); required || updates != nil {
			t.Fatalf("restart overwrote established dual-source state %+v with %+v", game, updates)
		}
	}
}

func TestBingoOrderedSourceFailureIsIsolatedByExplicitBinding(t *testing.T) {
	raw := []sourceDraw{{Issue: "raw"}}
	ordered := []sourceDraw{{Issue: "ordered", BingoOrderVerified: true}}
	orderedFailure := errors.New("ordered feed unavailable")
	wantOrdered := map[string]bool{
		"bingo-ssc-1": true, "bingo-racing-a": true, "bingo-mark-six": true,
	}
	for _, binding := range api168BingoBindings {
		t.Run(binding.GameID, func(t *testing.T) {
			_, _, bootstrapStatus, bootstrapError := bingoBindingSourceDefaults(binding)
			got, err := bingoSourceInputForBinding(binding, raw, ordered, orderedFailure)
			if wantOrdered[binding.GameID] {
				if bootstrapStatus != "stale" || bootstrapError != bingoOrderPendingMessage {
					t.Fatalf("ordered game starts healthy before validation: status=%q error=%q", bootstrapStatus, bootstrapError)
				}
				if !errors.Is(err, orderedFailure) || got != nil || !binding.RequiresOrderedSource {
					t.Fatalf("ordered-dependent game did not fail closed: input=%+v err=%v binding=%+v", got, err, binding)
				}
				got, err = bingoSourceInputForBinding(binding, raw, ordered, nil)
				if err != nil || !reflect.DeepEqual(got, ordered) {
					t.Fatalf("verified ordered input not selected: input=%+v err=%v", got, err)
				}
				return
			}
			if bootstrapStatus != "idle" || bootstrapError != "" {
				t.Fatalf("independent game inherited the order gate: status=%q error=%q", bootstrapStatus, bootstrapError)
			}
			if err != nil || !reflect.DeepEqual(got, raw) || binding.RequiresOrderedSource {
				t.Fatalf("independent game was coupled to ordered outage: input=%+v err=%v binding=%+v", got, err, binding)
			}
		})
	}
}

func TestOrderedBingoRecoveryRevisionContractMatchesSettlementGate(t *testing.T) {
	legacyOrderedCount := 0
	for _, binding := range bingo163Bindings {
		if !orderedBingoDrawRevisionCurrent(binding.GameID, binding.SourceRevision, binding.ConversionVersion) ||
			orderedBingoDrawRevisionCurrent(binding.GameID, "", binding.ConversionVersion) ||
			orderedBingoDrawRevisionCurrent(binding.GameID, binding.SourceRevision, "legacy") {
			t.Fatalf("current 163 settlement revision gate is not exact: %+v", binding)
		}
		if bingo163LegacyRequiredOrder(binding.GameID) {
			legacyOrderedCount++
			if !orderedBingoDrawRevisionCurrent(binding.GameID, bingoOrderedSourceRevision, binding.ConversionVersion) {
				t.Fatalf("verified 168+jyb history lost trust after cutover: %+v", binding)
			}
		} else if orderedBingoDrawRevisionCurrent(binding.GameID, bingoOrderedSourceRevision, binding.ConversionVersion) {
			t.Fatalf("set-only game accepted an ordered legacy contract it never used: %+v", binding)
		}
	}
	if !orderedBingoDrawRevisionCurrent("unknown-mark-six", "", "") {
		t.Fatal("unrelated unversioned game inherited the Bingo source gate")
	}
	query, args := orderedBingoRecoveryRevisionSQL("bets.game_id", "draws")
	contractCount := len(trustedDrawRevisionContracts("sg-ssc")) + len(source163MirrorBindings) + len(source163PC28Bindings) + len(source163MarkSixBindings) + len(bingo163Bindings) + legacyOrderedCount // SG current+legacy + current 163 mirrors + PC28/Mark Six variants + current Bingo seven + legacy ordered three.
	wantArgs := 1 + contractCount*3 + 2                                                                                                                                                                   // versioned IDs + contracts + cutover guards.
	if !strings.Contains(query, "bets.game_id NOT IN ?") || !strings.Contains(query, "draws.source_revision = ?") ||
		!strings.Contains(query, "draws.conversion_revision = ?") || len(args) != wantArgs {
		t.Fatalf("bounded recovery predicate lost an ordered product or revision: query=%q args=%+v", query, args)
	}
}

func TestVerifiedBingoStoredDrawComparisonIsStrictAndOperational(t *testing.T) {
	verified := []int{3, 5, 9, 1, 7, 10, 6, 2, 8, 4}
	for _, stored := range []string{
		"3,5,9,1,7,10,6,2,8,4",
		"03,05,09,01,07,10,06,02,08,04",
		" 3, 5, 9, 1, 7, 10, 6, 2, 8, 4 ",
	} {
		if !storedDrawNumbersEqual(stored, verified) {
			t.Fatalf("equivalent stored draw was treated as a conflict: %q", stored)
		}
	}
	for _, stored := range []string{
		"1,2,3,4,5,6,7,8,9,10",
		"3,5,9,1,7,10,6,2,8",
		"3,5,9,1,7,10,6,2,8,broken",
		"3,5,9,1,7,10,6,2,8,4,",
	} {
		if storedDrawNumbersEqual(stored, verified) {
			t.Fatalf("different or malformed legacy draw was treated as verified: %q", stored)
		}
	}
	err := verifiedBingoDrawConflictError("bingo-racing-a", "115049561", "1,2,3,4,5,6,7,8,9,10", verified)
	if !errors.Is(err, errVerifiedBingoDrawConflict) || !strings.Contains(err.Error(), "不会自动覆盖") || !strings.Contains(err.Error(), "人工对账") {
		t.Fatalf("conflict is not actionable or fail-closed: %v", err)
	}
}
