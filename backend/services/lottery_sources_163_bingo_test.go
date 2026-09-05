package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"backend/data/models/bet"
	"backend/data/models/lottery"
)

var bingo163FixtureSet = []int{4, 6, 9, 14, 15, 19, 22, 29, 30, 31, 32, 34, 40, 43, 52, 55, 57, 60, 62, 72}
var bingo163FixtureOrder = []int{6, 43, 4, 32, 57, 34, 60, 55, 30, 62, 14, 19, 31, 22, 40, 15, 52, 72, 9, 29}

func bingo163TestTime(hour, minute int) time.Time {
	return time.Date(2026, 9, 4, hour, minute, 0, 0, sgSSCLocation)
}

func bingo163TestRow(issue string, at time.Time, numbers []int) map[string]any {
	return bingo163TestProductRow(bingo163SetUpstreamGameID, issue, at, numbers)
}

func bingo163TestProductRow(gameID int, issue string, at time.Time, numbers []int) map[string]any {
	parts := make([]string, len(numbers))
	for index, number := range numbers {
		parts[index] = strconv.Itoa(number)
	}
	return map[string]any{
		"igameid":     gameID,
		"sgameperiod": issue,
		"sopennum":    strings.Join(parts, "|"),
		"dopentime":   at.In(sgSSCLocation).Format(time.DateTime),
	}
}

func bingo163TestPayload(t *testing.T, result any) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"success": true, "result": result})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestBingo163BindingsAllRequireVerifiedOrder(t *testing.T) {
	if len(bingo163Bindings) != 7 {
		t.Fatalf("bindings=%d want=7", len(bingo163Bindings))
	}
	seen := map[string]bool{}
	for _, binding := range bingo163Bindings {
		if seen[binding.GameID] || binding.SourceRevision == "" || binding.ConversionVersion == "" || binding.Transform == nil {
			t.Fatalf("invalid binding: %+v", binding)
		}
		seen[binding.GameID] = true
		if !binding.RequiresOrderedSource || binding.SourceRevision != bingo163VerifiedSourceRevision {
			t.Fatalf("binding %s is not pinned to verified ordered source: %+v", binding.GameID, binding)
		}
		found, ok := bingo163BindingForGame(" " + binding.GameID + " ")
		if !ok || found.GameID != binding.GameID {
			t.Fatalf("binding lookup failed for %s", binding.GameID)
		}
	}
	if _, ok := bingo163BindingForGame("official-tw-bingo"); ok {
		t.Fatal("official Taiwan Bingo is not one of the seven derived bindings")
	}
}

func TestBingo163SourceBindingRequiresExactCurrentMetadata(t *testing.T) {
	for _, binding := range bingo163Bindings {
		name, endpoint, status, message := bingo163BindingSourceDefaults(binding)
		if endpoint != bingo163SourceURL || status != "stale" || message == "" {
			t.Fatalf("defaults(%s)=(%q,%q,%q,%q)", binding.GameID, name, endpoint, status, message)
		}
		if name != bingo163OrderedSourceName {
			t.Fatalf("source name does not expose current ordered mother for %s", binding.GameID)
		}
		game := lottery.Game{ID: binding.GameID, SourceKind: "external", SourceName: name, SourceURL: endpoint}
		if !bingo163SourceBound(&game, binding) {
			t.Fatalf("valid binding rejected for %s", binding.GameID)
		}
		changed := game
		changed.SourceURL += "changed"
		if bingo163SourceBound(&changed, binding) {
			t.Fatalf("changed binding accepted for %s", binding.GameID)
		}
	}
}

func TestDecode163BingoRowsRequiresFixedIdentityAndSortedSet(t *testing.T) {
	valid := bingo163TestRow("115049938", bingo163TestTime(12, 0), bingo163FixtureSet)
	draws, err := decode163BingoRows(bingo163TestPayload(t, valid), false)
	if err != nil || len(draws) != 1 || draws[0].Issue != "115049938" ||
		draws[0].SourceRevision != bingo163SetSourceRevision || !reflect.DeepEqual(draws[0].Numbers, bingo163FixtureSet) {
		t.Fatalf("valid decode=%+v err=%v", draws, err)
	}

	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"wrong identity", func(row map[string]any) { row["igameid"] = 136 }},
		{"bad issue", func(row map[string]any) { row["sgameperiod"] = "period" }},
		{"short set", func(row map[string]any) { row["sopennum"] = "1|2|3" }},
		{"duplicate", func(row map[string]any) { row["sopennum"] = strings.Replace(row["sopennum"].(string), "|72", "|62", 1) }},
		{"unsorted", func(row map[string]any) { row["sopennum"] = "6|4|9|14|15|19|22|29|30|31|32|34|40|43|52|55|57|60|62|72" }},
		{"off session grid", func(row map[string]any) { row["dopentime"] = "2026-09-04 12:01:00" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			row := bingo163TestRow("115049938", bingo163TestTime(12, 0), bingo163FixtureSet)
			test.mutate(row)
			if _, err := decode163BingoRows(bingo163TestPayload(t, row), false); err == nil {
				t.Fatal("malformed row accepted")
			}
		})
	}

	duplicate := []any{valid, valid}
	if _, err := decode163BingoRows(bingo163TestPayload(t, duplicate), true); err == nil {
		t.Fatal("duplicate history issue accepted")
	}
	if _, err := decode163BingoRows([]byte(`{"success":false,"result":[]}`), true); err == nil {
		t.Fatal("unsuccessful envelope accepted")
	}
}

func TestDecode163BingoOrderedRowsRequiresID185AndStrictTwentyBallSequence(t *testing.T) {
	valid := bingo163TestProductRow(bingo163OrderedUpstreamGameID, "115049938", bingo163TestTime(12, 0), bingo163FixtureOrder)
	draws, err := decode163BingoProductRows(bingo163TestPayload(t, valid), false, bingo163OrderedUpstreamGameID, true, bingo163OrderedRawSourceRevision)
	if err != nil || len(draws) != 1 || draws[0].SourceRevision != bingo163OrderedRawSourceRevision || !reflect.DeepEqual(draws[0].Numbers, bingo163FixtureOrder) {
		t.Fatalf("valid ordered decode=%+v err=%v", draws, err)
	}
	wrongID := bingo163TestProductRow(bingo163SetUpstreamGameID, "115049938", bingo163TestTime(12, 0), bingo163FixtureOrder)
	if _, err := decode163BingoProductRows(bingo163TestPayload(t, wrongID), false, bingo163OrderedUpstreamGameID, true, bingo163OrderedRawSourceRevision); err == nil {
		t.Fatal("ID135 accepted as ID185 ordered mother")
	}
	duplicate := append([]int(nil), bingo163FixtureOrder...)
	duplicate[19] = duplicate[18]
	bad := bingo163TestProductRow(bingo163OrderedUpstreamGameID, "115049938", bingo163TestTime(12, 0), duplicate)
	if _, err := decode163BingoProductRows(bingo163TestPayload(t, bad), false, bingo163OrderedUpstreamGameID, true, bingo163OrderedRawSourceRevision); err == nil {
		t.Fatal("ordered mother accepted duplicate ball")
	}
}

func TestFetch163BingoAuthorityValidatesHistoryAndSetsNextBoundary(t *testing.T) {
	latestAt := bingo163TestTime(12, 0)
	latest := bingo163TestRow("115049938", latestAt, bingo163FixtureSet)
	history := []any{
		latest,
		bingo163TestRow("115049937", latestAt.Add(-5*time.Minute), bingo163FixtureSet),
		bingo163TestRow("115049936", latestAt.Add(-10*time.Minute), bingo163FixtureSet),
		bingo163TestRow("115049935", latestAt.Add(-15*time.Minute), bingo163FixtureSet),
	}
	requests := 0
	draws, err := fetch163BingoAuthorityWithRequest(context.Background(), func() time.Time { return latestAt.Add(time.Minute) }, bytes.NewReader(make([]byte, 20)), func(_ context.Context, endpoint string) ([]byte, error) {
		requests++
		parsed, parseErr := url.Parse(endpoint)
		if parseErr != nil || parsed.Query().Get("iGameId") != "135" {
			t.Fatalf("unexpected endpoint: %q err=%v", endpoint, parseErr)
		}
		switch parsed.Path {
		case source163LatestPath:
			return bingo163TestPayload(t, latest), nil
		case source163HistoryPath:
			if parsed.Query().Get("count") != strconv.Itoa(bingo163HistoryLimit) {
				t.Fatalf("history count=%q", parsed.Query().Get("count"))
			}
			return bingo163TestPayload(t, history), nil
		default:
			t.Fatalf("unexpected path %q", parsed.Path)
			return nil, nil
		}
	})
	if err != nil || requests != 2 || len(draws) != 4 {
		t.Fatalf("draws=%+v requests=%d err=%v", draws, requests, err)
	}
	if draws[0].NextIssue != "115049939" || !draws[0].NextDrawAt.Equal(bingo163TestTime(12, 5).UTC()) {
		t.Fatalf("next=%s@%s", draws[0].NextIssue, draws[0].NextDrawAt)
	}
	for _, draw := range draws {
		if draw.SourceRevision != bingo163SetSourceRevision || draw.ConversionRevision != "" || draw.BingoOrderVerified {
			t.Fatalf("authority provenance changed: %+v", draw)
		}
	}
}

func TestFetch163BingoAuthorityFailsClosedOnHistoryOrFreshness(t *testing.T) {
	latestAt := bingo163TestTime(12, 0)
	latest := bingo163TestRow("115049938", latestAt, bingo163FixtureSet)
	baseHistory := []any{
		latest,
		bingo163TestRow("115049937", latestAt.Add(-5*time.Minute), bingo163FixtureSet),
		bingo163TestRow("115049936", latestAt.Add(-10*time.Minute), bingo163FixtureSet),
		bingo163TestRow("115049935", latestAt.Add(-15*time.Minute), bingo163FixtureSet),
	}
	for _, test := range []struct {
		name    string
		now     time.Time
		history []any
	}{
		{"history missing latest", latestAt.Add(time.Minute), baseHistory[1:]},
		{"history gap", latestAt.Add(time.Minute), []any{baseHistory[0], baseHistory[1], baseHistory[2], bingo163TestRow("115049934", latestAt.Add(-20*time.Minute), bingo163FixtureSet)}},
		{"stale during session", latestAt.Add(8 * time.Minute), baseHistory},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := fetch163BingoAuthorityWithRequest(context.Background(), func() time.Time { return test.now }, bytes.NewReader(make([]byte, 20)), func(_ context.Context, endpoint string) ([]byte, error) {
				parsed, _ := url.Parse(endpoint)
				if parsed.Path == source163LatestPath {
					return bingo163TestPayload(t, latest), nil
				}
				return bingo163TestPayload(t, test.history), nil
			})
			if err == nil {
				t.Fatal("invalid upstream state accepted")
			}
		})
	}
}

func TestFetch163BingoVerifiedAuthorityUsesOnlyID135AndID185(t *testing.T) {
	latestAt := bingo163TestTime(12, 0)
	issues := []string{"115049938", "115049937", "115049936", "115049935"}
	sortedHistory := make([]any, 0, len(issues))
	orderedHistory := make([]any, 0, len(issues))
	for index, issue := range issues {
		at := latestAt.Add(-time.Duration(index) * 5 * time.Minute)
		sortedHistory = append(sortedHistory, bingo163TestProductRow(bingo163SetUpstreamGameID, issue, at, bingo163FixtureSet))
		orderedHistory = append(orderedHistory, bingo163TestProductRow(bingo163OrderedUpstreamGameID, issue, at, bingo163FixtureOrder))
	}
	requests := map[int]int{}
	draws, err := fetch163BingoVerifiedAuthorityWithRequest(context.Background(), func() time.Time { return latestAt.Add(time.Minute) }, bytes.NewReader(make([]byte, 40)), func(_ context.Context, endpoint string) ([]byte, error) {
		parsed, parseErr := url.Parse(endpoint)
		if parseErr != nil {
			return nil, parseErr
		}
		gameID, _ := strconv.Atoi(parsed.Query().Get("iGameId"))
		requests[gameID]++
		var rows []any
		switch gameID {
		case bingo163SetUpstreamGameID:
			rows = sortedHistory
		case bingo163OrderedUpstreamGameID:
			rows = orderedHistory
		default:
			t.Fatalf("unexpected upstream game ID %d", gameID)
		}
		if parsed.Path == source163LatestPath {
			return bingo163TestPayload(t, rows[0]), nil
		}
		if parsed.Path != source163HistoryPath {
			t.Fatalf("unexpected path %q", parsed.Path)
		}
		return bingo163TestPayload(t, rows), nil
	})
	if err != nil || len(draws) != 4 || requests[bingo163SetUpstreamGameID] != 2 || requests[bingo163OrderedUpstreamGameID] != 2 {
		t.Fatalf("verified=%+v requests=%v err=%v", draws, requests, err)
	}
	for _, draw := range draws {
		if draw.SourceRevision != bingo163VerifiedSourceRevision || !draw.BingoOrderVerified || !reflect.DeepEqual(draw.Numbers, bingo163FixtureOrder) {
			t.Fatalf("unverified production draw: %+v", draw)
		}
	}
}

func TestBingo163SessionHas203PeriodsAndSkipsOvernight(t *testing.T) {
	issue := "115049001"
	at := bingo163TestTime(7, 5)
	count := 1
	for at.In(sgSSCLocation).Hour() != 23 || at.In(sgSSCLocation).Minute() != 55 {
		var err error
		issue, at, err = bingo163NextSchedule(issue, at)
		if err != nil {
			t.Fatal(err)
		}
		count++
	}
	if count != 203 {
		t.Fatalf("periods=%d want=203", count)
	}
	nextIssueNumber, nextAt, err := bingo163NextSchedule(issue, at)
	if err != nil || nextIssueNumber != "115049204" || nextAt.In(sgSSCLocation).Format("2006-01-02 15:04:05") != "2026-09-05 07:05:00" {
		t.Fatalf("overnight next=%s@%s err=%v", nextIssueNumber, nextAt.In(sgSSCLocation), err)
	}
	latest := sourceDraw{Issue: issue, Numbers: append([]int(nil), bingo163FixtureSet...), DrawAt: at, SourceRevision: bingo163SetSourceRevision}
	if err := validate163BingoFreshness(latest, time.Date(2026, 9, 5, 2, 0, 0, 0, sgSSCLocation)); err != nil {
		t.Fatalf("overnight latest should remain valid: %v", err)
	}
	if err := validate163BingoFreshness(latest, time.Date(2026, 9, 5, 7, 8, 0, 0, sgSSCLocation)); err == nil {
		t.Fatal("previous session latest remained healthy after morning publication grace")
	}
}

func TestCrossValidate163BingoOrderUses163IdentitySetAndTime(t *testing.T) {
	authorityAt := bingo163TestTime(23, 55)
	nextIssueNumber, nextAt, err := bingo163NextSchedule("115049938", authorityAt)
	if err != nil {
		t.Fatal(err)
	}
	authority := sourceDraw{
		Issue: "115049938", Numbers: append([]int(nil), bingo163FixtureSet...), DrawAt: authorityAt.UTC(),
		NextIssue: nextIssueNumber, NextDrawAt: nextAt, SourceRevision: bingo163SetSourceRevision,
	}
	ordered := sourceDraw{Issue: authority.Issue, Numbers: append([]int(nil), bingo163FixtureOrder...), DrawAt: authorityAt.UTC(), SourceRevision: bingo163OrderedRawSourceRevision}
	verified, err := crossValidate163BingoOrder([]sourceDraw{authority}, []sourceDraw{ordered})
	if err != nil || len(verified) != 1 {
		t.Fatalf("verified=%+v err=%v", verified, err)
	}
	got := verified[0]
	if !reflect.DeepEqual(got.Numbers, bingo163FixtureOrder) || !got.DrawAt.Equal(authority.DrawAt) || got.NextIssue != authority.NextIssue || !got.NextDrawAt.Equal(authority.NextDrawAt) ||
		!got.BingoOrderVerified || got.SourceRevision != bingo163VerifiedSourceRevision {
		t.Fatalf("verified source facts not preserved: %+v", got)
	}
}

func TestCrossValidate163BingoOrderRejectsMissingSetTimeAndRevisionEvidence(t *testing.T) {
	at := bingo163TestTime(12, 0)
	authority := sourceDraw{Issue: "115049938", Numbers: append([]int(nil), bingo163FixtureSet...), DrawAt: at.UTC(), SourceRevision: bingo163SetSourceRevision}
	ordered := sourceDraw{Issue: authority.Issue, Numbers: append([]int(nil), bingo163FixtureOrder...), DrawAt: at.UTC(), SourceRevision: bingo163OrderedRawSourceRevision}
	for _, test := range []struct {
		name      string
		authority []sourceDraw
		ordered   []sourceDraw
	}{
		{"missing issue", []sourceDraw{authority}, []sourceDraw{{Issue: "115049937", Numbers: ordered.Numbers, DrawAt: ordered.DrawAt, SourceRevision: bingo163OrderedRawSourceRevision}}},
		{"set mismatch", []sourceDraw{authority}, []sourceDraw{{Issue: ordered.Issue, Numbers: append(append([]int(nil), ordered.Numbers[:19]...), 71), DrawAt: ordered.DrawAt, SourceRevision: bingo163OrderedRawSourceRevision}}},
		{"time mismatch", []sourceDraw{authority}, []sourceDraw{{Issue: ordered.Issue, Numbers: ordered.Numbers, DrawAt: at.Add(-time.Second).UTC(), SourceRevision: bingo163OrderedRawSourceRevision}}},
		{"wrong authority revision", []sourceDraw{{Issue: authority.Issue, Numbers: authority.Numbers, DrawAt: authority.DrawAt, SourceRevision: "legacy"}}, []sourceDraw{ordered}},
		{"wrong ordered revision", []sourceDraw{authority}, []sourceDraw{{Issue: ordered.Issue, Numbers: ordered.Numbers, DrawAt: ordered.DrawAt, SourceRevision: "legacy"}}},
		{"duplicate ordered issue", []sourceDraw{authority}, []sourceDraw{ordered, ordered}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := crossValidate163BingoOrder(test.authority, test.ordered); err == nil {
				t.Fatal("invalid dual-source evidence accepted")
			}
		})
	}
}

func TestBingo163AllProductsFailTogetherAndUseGoldenConversions(t *testing.T) {
	authority := sourceDraw{Issue: "115049938", Numbers: append([]int(nil), bingo163FixtureSet...), DrawAt: bingo163TestTime(12, 0).UTC(), SourceRevision: bingo163SetSourceRevision}
	ordered := sourceDraw{Issue: authority.Issue, Numbers: append([]int(nil), bingo163FixtureOrder...), DrawAt: authority.DrawAt, SourceRevision: bingo163VerifiedSourceRevision, BingoOrderVerified: true}
	orderedFailure := errors.New("ordered feed unavailable")

	want := map[string][]int{
		"bingo-ssc-1":    {6, 3, 4, 2, 7},
		"bingo-ssc-2":    {4, 0, 5, 0, 2},
		"bingo-ssc-3":    {4, 9, 1, 2, 0},
		"bingo-ssc-4":    {5, 2, 2, 9, 9},
		"bingo-racing-a": {2, 6, 1, 4, 8, 5, 9, 7, 3, 10},
		"bingo-racing-b": {2, 4, 7, 5, 8, 3, 9, 10, 1, 6},
		"bingo-mark-six": {6, 43, 4, 32, 34, 30, 14},
	}
	for _, binding := range bingo163Bindings {
		input, err := bingo163SourceInputForBinding(binding, []sourceDraw{authority}, []sourceDraw{ordered}, orderedFailure)
		if !errors.Is(err, orderedFailure) {
			t.Fatalf("%s did not fail closed with ordered source: %v", binding.GameID, err)
		}
		input, err = bingo163SourceInputForBinding(binding, []sourceDraw{authority}, []sourceDraw{ordered}, nil)
		if err != nil {
			t.Fatalf("input(%s): %v", binding.GameID, err)
		}
		draws, err := transform163BingoDraws(binding, input)
		if err != nil || len(draws) != 1 || !reflect.DeepEqual(draws[0].Numbers, want[binding.GameID]) || draws[0].SourceRevision != binding.SourceRevision || draws[0].ConversionRevision != binding.ConversionVersion {
			t.Fatalf("transform(%s)=%+v err=%v want=%v", binding.GameID, draws, err, want[binding.GameID])
		}
	}
}

func TestTransform163BingoDrawsRejectsWrongSourceClass(t *testing.T) {
	orderedBinding, _ := bingo163BindingForGame("bingo-ssc-1")
	setDraw := sourceDraw{Issue: "115049938", Numbers: bingo163FixtureSet, DrawAt: bingo163TestTime(12, 0).UTC(), SourceRevision: bingo163SetSourceRevision}
	if _, err := transform163BingoDraws(orderedBinding, []sourceDraw{setDraw}); err == nil {
		t.Fatal("ordered product accepted sorted set")
	}
	legacyOrdered := sourceDraw{Issue: setDraw.Issue, Numbers: bingo163FixtureOrder, DrawAt: setDraw.DrawAt, SourceRevision: bingo163OrderSourceRevision, BingoOrderVerified: true}
	if _, err := transform163BingoDraws(orderedBinding, []sourceDraw{legacyOrdered}); err == nil {
		t.Fatal("current product accepted legacy jyb revision")
	}
}

func TestTransform163BingoMarkSixSkipsValidNoResultPeriodAndKeepsMotherBoundary(t *testing.T) {
	binding, ok := bingo163BindingForGame("bingo-mark-six")
	if !ok {
		t.Fatal("missing bingo mark-six binding")
	}
	previousAt := bingo163TestTime(12, 0).UTC()
	latestAt := previousAt.Add(5 * time.Minute)
	nextAt := latestAt.Add(5 * time.Minute)
	valid := sourceDraw{
		Issue: "115049938", Numbers: append([]int(nil), bingo163FixtureOrder...), DrawAt: previousAt,
		SourceRevision: binding.SourceRevision, BingoOrderVerified: true,
	}
	// This is a structurally valid, unique 20-ball draw but only five values
	// are within 01-49. It therefore has no seven-ball mark-six result.
	noResult := sourceDraw{
		Issue:   "115049939",
		Numbers: []int{45, 6, 42, 58, 74, 54, 67, 80, 75, 76, 65, 37, 70, 78, 51, 66, 72, 73, 12, 50},
		DrawAt:  latestAt, NextIssue: "115049940", NextDrawAt: nextAt,
		SourceRevision: binding.SourceRevision, BingoOrderVerified: true,
	}
	draws, err := transform163BingoDraws(binding, []sourceDraw{noResult, valid})
	if err != nil || len(draws) != 1 {
		t.Fatalf("transform=%+v err=%v", draws, err)
	}
	if draws[0].Issue != valid.Issue || draws[0].NextIssue != noResult.NextIssue || !draws[0].NextDrawAt.Equal(nextAt) {
		t.Fatalf("derived row did not retain authoritative next boundary: %+v", draws[0])
	}
	if !reflect.DeepEqual(draws[0].Numbers, []int{6, 43, 4, 32, 34, 30, 14}) {
		t.Fatalf("unexpected retained result: %v", draws[0].Numbers)
	}

	malformed := noResult
	malformed.Issue = "115049941"
	malformed.Numbers = []int{45, 6, 42}
	if _, err := transform163BingoDraws(binding, []sourceDraw{malformed}); err == nil {
		t.Fatal("malformed mother row was mistaken for a legitimate no-result period")
	}
	if _, err := transform163BingoDraws(binding, []sourceDraw{noResult}); err == nil {
		t.Fatal("batch containing no usable derived result was accepted")
	}
}

func TestBingo163SourceRevisionUpdatesOnlyExactLegacyOrIncompleteCurrentBinding(t *testing.T) {
	for _, gameID := range []string{"bingo-ssc-2", "bingo-racing-a"} {
		binding, ok := bingo163BindingForGame(gameID)
		if !ok {
			t.Fatalf("missing binding for %s", gameID)
		}
		legacy := lottery.Game{ID: gameID, SourceKind: "external"}
		if bingo163LegacyRequiredOrder(gameID) {
			legacy.SourceName, legacy.SourceURL = bingoVerifiedSourceName, bingoVerifiedSourceURL
		} else {
			legacy.SourceName, legacy.SourceURL = "168开奖网", "https://kj138138.com/view/api/index.html"
		}
		updates, changed := bingo163SourceRevisionUpdates(legacy, binding)
		name, endpoint, _, _ := bingo163BindingSourceDefaults(binding)
		if !changed || updates["source_name"] != name || updates["source_url"] != endpoint ||
			updates["sync_status"] != "stale" || updates["last_sync_error"] != bingo163PendingMessage {
			t.Fatalf("legacy migration(%s)=%v changed=%v", gameID, updates, changed)
		}

		prior163Name := bingo163SetSourceName
		if bingo163LegacyRequiredOrder(gameID) {
			prior163Name = bingo163LegacyOrderedSourceName
		}
		prior163 := lottery.Game{ID: gameID, SourceKind: "external", SourceName: prior163Name, SourceURL: bingo163SourceURL, SyncStatus: "ok"}
		updates, changed = bingo163SourceRevisionUpdates(prior163, binding)
		if !changed || updates["source_name"] != bingo163OrderedSourceName || updates["sync_status"] != "stale" || updates["last_sync_at"] != nil {
			t.Fatalf("previous 163 migration(%s)=%v changed=%v", gameID, updates, changed)
		}

		current := lottery.Game{ID: gameID, SourceKind: "external", SourceName: name, SourceURL: endpoint, SyncStatus: "idle"}
		updates, changed = bingo163SourceRevisionUpdates(current, binding)
		if !changed || !reflect.DeepEqual(updates, map[string]any{"sync_status": "stale", "last_sync_error": bingo163PendingMessage}) {
			t.Fatalf("incomplete current binding(%s)=%v changed=%v", gameID, updates, changed)
		}
		for _, status := range []string{"ok", "error", "paused"} {
			current.SyncStatus = status
			if updates, changed := bingo163SourceRevisionUpdates(current, binding); changed || updates != nil {
				t.Fatalf("operational current binding %s/%s was reset: %v", gameID, status, updates)
			}
		}
		current.SyncStatus, current.LastSyncError = "stale", "operator-visible prior diagnostic"
		if updates, changed := bingo163SourceRevisionUpdates(current, binding); changed || updates != nil {
			t.Fatalf("current stale diagnostic was overwritten: %v", updates)
		}

		custom := legacy
		custom.SourceName, custom.SourceURL = "operator source", "https://operator.invalid/draws"
		if updates, changed := bingo163SourceRevisionUpdates(custom, binding); changed || updates != nil {
			t.Fatalf("operator binding was overwritten: %v", updates)
		}
	}
}

func TestInsert163BingoDrawsPostgresIsStrictIdempotentAndPreservesLegacyProvenance(t *testing.T) {
	db := timingPostgresDatabase(t)
	binding, ok := bingo163BindingForGame("bingo-ssc-2")
	if !ok {
		t.Fatal("missing direct set binding")
	}
	drawAt := bingo163TestTime(12, 0).UTC()
	verified := func(issue string, numbers []int, at time.Time) sourceDraw {
		return sourceDraw{
			Issue: issue, Numbers: append([]int(nil), numbers...), DrawAt: at,
			SourceRevision: binding.SourceRevision, ConversionRevision: binding.ConversionVersion, BingoOrderVerified: true,
		}
	}
	numbers := []int{6, 9, 4, 5, 9}

	legacyMatching := lottery.Draw{
		GameID: binding.GameID, Issue: "163-legacy-matching", Numbers: joinNumbers(numbers),
		DrawAt: drawAt.Add(-53 * time.Second), SourceRevision: "168-legacy", ConversionRevision: "legacy-conversion",
	}
	if err := db.Create(&legacyMatching).Error; err != nil {
		t.Fatal(err)
	}
	batch := []sourceDraw{
		verified(legacyMatching.Issue, numbers, drawAt),
		verified("163-new", numbers, drawAt.Add(5*time.Minute)),
	}
	if imported, err := insert163BingoDraws(db, binding, batch); err != nil || imported != 1 {
		t.Fatalf("first strict import: imported=%d err=%v", imported, err)
	}
	if imported, err := insert163BingoDraws(db, binding, batch); err != nil || imported != 0 {
		t.Fatalf("idempotent retry: imported=%d err=%v", imported, err)
	}
	var preserved, inserted lottery.Draw
	if err := db.First(&preserved, legacyMatching.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&inserted, "game_id = ? AND issue = ?", binding.GameID, "163-new").Error; err != nil {
		t.Fatal(err)
	}
	if preserved.DrawAt != legacyMatching.DrawAt || preserved.SourceRevision != legacyMatching.SourceRevision || preserved.ConversionRevision != legacyMatching.ConversionRevision {
		t.Fatalf("matching legacy provenance was relabelled: %+v", preserved)
	}
	if inserted.SourceRevision != binding.SourceRevision || inserted.ConversionRevision != binding.ConversionVersion || !inserted.DrawAt.Equal(drawAt.Add(5*time.Minute)) {
		t.Fatalf("new row did not freeze 163 provenance: %+v", inserted)
	}

	currentWrongTime := lottery.Draw{
		GameID: binding.GameID, Issue: "163-current-wrong-time", Numbers: joinNumbers(numbers), DrawAt: drawAt.Add(-time.Minute),
		SourceRevision: binding.SourceRevision, ConversionRevision: binding.ConversionVersion,
	}
	if err := db.Create(&currentWrongTime).Error; err != nil {
		t.Fatal(err)
	}
	if imported, err := insert163BingoDraws(db, binding, []sourceDraw{verified(currentWrongTime.Issue, numbers, drawAt)}); imported != 0 || !errors.Is(err, err163BingoDrawConflict) {
		t.Fatalf("current revision time conflict was accepted: imported=%d err=%v", imported, err)
	}

	settledLegacy := lottery.Draw{GameID: binding.GameID, Issue: "163-settled-legacy", Numbers: "0,0,0,0,0", DrawAt: drawAt}
	if err := db.Create(&settledLegacy).Error; err != nil {
		t.Fatal(err)
	}
	settledAt := drawAt.Add(time.Minute)
	if err := db.Create(&lottery.Issue{
		GameID: binding.GameID, Issue: settledLegacy.Issue, Status: lottery.IssueStatusSettled, SourceMode: "external",
		AcceptAt: drawAt.Add(-5 * time.Minute), SealAt: drawAt.Add(-3 * time.Second), DrawAt: &drawAt, SettledAt: &settledAt,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if imported, err := insert163BingoDraws(db, binding, []sourceDraw{verified(settledLegacy.Issue, numbers, drawAt)}); err != nil || imported != 0 {
		t.Fatalf("settled legacy row must remain audit-only: imported=%d err=%v", imported, err)
	}
	var settledAfter lottery.Draw
	if err := db.First(&settledAfter, settledLegacy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if settledAfter.Numbers != settledLegacy.Numbers || settledAfter.SourceRevision != "" {
		t.Fatalf("settled legacy history was rewritten: %+v", settledAfter)
	}

	unresolved := lottery.Draw{GameID: binding.GameID, Issue: "163-unresolved", Numbers: "0,0,0,0,0", DrawAt: drawAt}
	if err := db.Create(&unresolved).Error; err != nil {
		t.Fatal(err)
	}
	if imported, err := insert163BingoDraws(db, binding, []sourceDraw{verified(unresolved.Issue, numbers, drawAt)}); imported != 0 || !errors.Is(err, err163BingoDrawConflict) {
		t.Fatalf("unsettled legacy mismatch was accepted: imported=%d err=%v", imported, err)
	}

	if imported, err := insert163BingoDraws(db, binding, []sourceDraw{
		verified("163-duplicate", numbers, drawAt), verified("163-duplicate", numbers, drawAt),
	}); imported != 0 || !errors.Is(err, err163BingoDrawConflict) {
		t.Fatalf("duplicate batch issue was accepted: imported=%d err=%v", imported, err)
	}

	// A bet is explicit financial evidence as well as an unresolved draw. It
	// must remain untouched even if a future refactor relaxes the no-bet branch.
	room := timingPostgresRoom(t, db, "bingo_163_conflict_room", "792604")
	member := timingPostgresMember(t, db, room, "bingo_163_conflict_member")
	withBet := lottery.Draw{GameID: binding.GameID, Issue: "163-with-bet", Numbers: "0,0,0,0,0", DrawAt: drawAt}
	if err := db.Create(&withBet).Error; err != nil {
		t.Fatal(err)
	}
	ticket := bet.Bet{
		WorkspaceID: room.ID, GameID: binding.GameID, Issue: withBet.Issue, RoomScope: room.Scope,
		UserID: member.UserID, Username: member.Username, PlayCode: "number", PlayName: "号码", Position: 1,
		Selection: "6", RuleVersion: "digits5-v3", RequestReference: "bingo-163-conflict", AmountCents: 100, Odds: 2, Status: "pending",
	}
	if err := db.Create(&ticket).Error; err != nil {
		t.Fatal(err)
	}
	if imported, err := insert163BingoDraws(db, binding, []sourceDraw{verified(withBet.Issue, numbers, drawAt)}); imported != 0 || !errors.Is(err, err163BingoDrawConflict) || !strings.Contains(err.Error(), "1 条") {
		t.Fatalf("financial evidence conflict was accepted: imported=%d err=%v", imported, err)
	}
}
