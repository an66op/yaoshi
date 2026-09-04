package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type sgSSCTestFixture struct {
	now       time.Time
	latest    map[string]map[string]any
	histories map[string][]map[string]any
	overrides map[string][]byte
	mu        sync.Mutex
	calls     []string
}

func sgSSCTestEnvelope(data any) []byte {
	body, err := json.Marshal(map[string]any{
		"errorCode": 0, "result": map[string]any{"businessCode": 0, "data": data},
	})
	if err != nil {
		panic(err)
	}
	return body
}

func sgSSCTestRawRow(issue string) map[string]any {
	_, ordinal, at, err := parseSGSSCIssue(issue)
	if err != nil {
		panic(err)
	}
	return map[string]any{
		"preDrawIssue": json.Number(issue),
		"preDrawTime":  at.In(sgSSCLocation).Format("2006-01-02 15:04:05"),
		"preDrawCode":  fmt.Sprintf("%d,%d,%d,%d,%d", ordinal%10, (ordinal+1)%10, (ordinal+2)%10, (ordinal+3)%10, (ordinal+4)%10),
	}
}

func sgSSCTestLatestRow(issue string, station sgSSCStation) map[string]any {
	row := sgSSCTestRawRow(issue)
	_, _, at, _ := parseSGSSCIssue(issue)
	row["lotCode"], row["lotName"], row["totalCount"] = station.lotCode, "SG时时彩", 288
	row["drawIssue"] = sgSSCIssueAt(at.Add(sgSSCInterval))
	row["drawTime"] = at.Add(sgSSCInterval).In(sgSSCLocation).Format("2006-01-02 15:04:05")
	row["serverTime"] = "2099-01-01 00:00:00" // Never a scheduling authority.
	row["sumNum"] = "wrong-derived-field"
	row["dragonTiger"] = station.name // Sites' computed fields need not agree.
	if station.name == "168" {
		row["lotCode"] = 10075
		row["drawIssue"] = json.Number(row["drawIssue"].(string))
	} else {
		row["preDrawIssue"] = issue
	}
	return row
}

func newSGSSCTestFixture(issue string) *sgSSCTestFixture {
	day, ordinal, at, err := parseSGSSCIssue(issue)
	if err != nil {
		panic(err)
	}
	f := &sgSSCTestFixture{
		now: at.Add(30 * time.Second), latest: map[string]map[string]any{},
		histories: map[string][]map[string]any{}, overrides: map[string][]byte{},
	}
	for _, station := range sgSSCStations() {
		f.latest[sgSSCEndpoint(station, "")] = sgSSCTestLatestRow(issue, station)
		for offset := 0; offset <= 1; offset++ {
			if offset == 1 && ordinal >= sgSSCWindowSize {
				break
			}
			date := day.AddDate(0, 0, -offset)
			last := ordinal
			if offset == 1 {
				last = 288
			}
			endpoint := sgSSCEndpoint(station, date.Format("2006-01-02"))
			for number := last; number >= 1; number-- {
				f.histories[endpoint] = append(f.histories[endpoint], sgSSCTestRawRow(fmt.Sprintf("%s%03d", date.Format("20060102"), number)))
			}
		}
	}
	return f
}

func (f *sgSSCTestFixture) request(ctx context.Context, endpoint string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > sgSSCTotalTimeout {
		return nil, errors.New("request missing bounded total deadline")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, endpoint)
	parsed, parseErr := url.Parse(endpoint)
	if parseErr == nil && parsed.Path == source163LatestPath && parsed.Query().Get("iGameId") == "64" {
		legacyLatest := sgSSCEndpoint(sgSSCStations()[0], "")
		if body, found := f.overrides[legacyLatest]; found {
			return body, nil
		}
		row, found := f.latest[legacyLatest]
		if !found {
			return nil, fmt.Errorf("unexpected URL: %s", endpoint)
		}
		return sgSSCTest163Envelope(sgSSCTest163Row(row, true)), nil
	}
	if parseErr == nil && parsed.Path == source163HistoryPath && parsed.Query().Get("iGameId") == "64" {
		legacy := sgSSCStations()[0]
		keys := make([]string, 0)
		for key := range f.histories {
			if strings.HasPrefix(key, legacy.base+sgSSCHistoryPath) {
				keys = append(keys, key)
			}
		}
		sort.Sort(sort.Reverse(sort.StringSlice(keys)))
		rows := make([]map[string]any, 0, source163MirrorHistoryLimit)
		for _, key := range keys {
			if body, found := f.overrides[key]; found {
				return body, nil
			}
			for _, row := range f.histories[key] {
				rows = append(rows, sgSSCTest163Row(row, false))
				if len(rows) == source163MirrorHistoryLimit {
					break
				}
			}
			if len(rows) == source163MirrorHistoryLimit {
				break
			}
		}
		if len(rows) == 0 {
			return nil, fmt.Errorf("unexpected URL: %s", endpoint)
		}
		return sgSSCTest163Envelope(rows), nil
	}
	if body, found := f.overrides[endpoint]; found {
		return body, nil
	}
	if row, found := f.latest[endpoint]; found {
		return sgSSCTestEnvelope(row), nil
	}
	if rows, found := f.histories[endpoint]; found {
		return sgSSCTestEnvelope(rows), nil
	}
	return nil, fmt.Errorf("unexpected URL: %s", endpoint)
}

func sgSSCTest163Envelope(result any) []byte {
	body, err := json.Marshal(map[string]any{"success": true, "result": result})
	if err != nil {
		panic(err)
	}
	return body
}

func sgSSCTest163Row(row map[string]any, latest bool) map[string]any {
	issue := fmt.Sprint(row["preDrawIssue"])
	code, _ := row["preDrawCode"].(string)
	result := map[string]any{
		"igameid": 64, "sgameperiod": issue,
		"sopennum": strings.ReplaceAll(code, ",", "|"), "dopentime": row["preDrawTime"],
	}
	if latest {
		_, _, at, err := parseSGSSCIssue(issue)
		if err == nil {
			result["nextGamePeriod"] = sgSSCIssueAt(at.Add(sgSSCInterval))
			result["nextPeriodOpenTime"] = at.Add(sgSSCInterval).UnixMilli()
		}
	}
	return result
}

func (f *sgSSCTestFixture) fetch() ([]sourceDraw, error) {
	return fetchSGSSCVerifiedWithRequest(context.Background(), func() time.Time { return f.now }, f.request)
}

func TestSGSSCVerifiedWindowAndMidnight(t *testing.T) {
	for _, test := range []struct {
		issue, first string
		calls        int
	}{
		{"20260903030", "20260903007", 4},
		{"20260903024", "20260903001", 4},
		{"20260903023", "20260902288", 5},
		{"20260903001", "20260902266", 5},
		{"20260902288", "20260902265", 4},
		{"20270101001", "20261231266", 5},
		{"20240301001", "20240229266", 5},
	} {
		t.Run(test.issue, func(t *testing.T) {
			fixture := newSGSSCTestFixture(test.issue)
			// Both source ordering conventions are accepted; results are always ascending.
			for endpoint, rows := range fixture.histories {
				if strings.Contains(endpoint, "115kai") {
					for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
						rows[i], rows[j] = rows[j], rows[i]
					}
				}
			}
			draws, err := fixture.fetch()
			if err != nil {
				t.Fatal(err)
			}
			if len(draws) != 24 || draws[0].Issue != test.first || draws[23].Issue != test.issue {
				t.Fatalf("unexpected verified window: %+v", draws)
			}
			if err := validateSGSSCVerifiedBatch(draws); err != nil {
				t.Fatal(err)
			}
			if len(fixture.calls) != test.calls {
				t.Fatalf("requests=%d want=%d: %v", len(fixture.calls), test.calls, fixture.calls)
			}
			seen := map[string]bool{}
			for _, endpoint := range fixture.calls {
				if seen[endpoint] {
					t.Fatalf("duplicate network request %s", endpoint)
				}
				seen[endpoint] = true
				if strings.Contains(endpoint, "api.api168168.com") || strings.Contains(endpoint, "lotCode=10075") || strings.Contains(endpoint, "10059") {
					t.Fatalf("wrong source identity: %s", endpoint)
				}
			}
			for i, draw := range draws {
				if draw.SourceRevision != sgSSCSourceRevision || draw.ConversionRevision != sgSSCConversionRevision || draw.HasBingoSourceTail || draw.BingoOrderVerified {
					t.Fatalf("wrong raw SG provenance at %d: %+v", i, draw)
				}
				if draw.DrawAt.Location() != time.UTC {
					t.Fatalf("timestamps must be normalized UTC: %v", draw.DrawAt)
				}
			}
			if test.issue == "20260902288" {
				station := sgSSCValidationStation()
				if !seen[sgSSCEndpoint(station, "2026-09-02")] || seen[sgSSCEndpoint(station, "2026-09-03")] {
					t.Fatal("00:00 period 288 must request the preceding issue date from the verifier")
				}
			}
		})
	}
}

func TestSGSSC163MotherIdentityAndScheduleFailClosed(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(map[string]any) []byte
	}{
		{"wrong game identity", func(row map[string]any) []byte { row["igameid"] = 169; return sgSSCTest163Envelope(row) }},
		{"missing next issue", func(row map[string]any) []byte { delete(row, "nextGamePeriod"); return sgSSCTest163Envelope(row) }},
		{"skipped next issue", func(row map[string]any) []byte {
			row["nextGamePeriod"] = "20260903032"
			return sgSSCTest163Envelope(row)
		}},
		{"wrong absolute next time", func(row map[string]any) []byte {
			row["nextPeriodOpenTime"] = time.Date(2026, 9, 3, 2, 40, 0, 0, sgSSCLocation).UnixMilli()
			return sgSSCTest163Envelope(row)
		}},
		{"duplicate JSON identity", func(map[string]any) []byte { return []byte(`{"success":true,"result":{"igameid":64,"igameid":169}}`) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newSGSSCTestFixture("20260903030")
			legacyLatest := sgSSCEndpoint(sgSSCStations()[0], "")
			fixture.overrides[legacyLatest] = test.mutate(sgSSCTest163Row(fixture.latest[legacyLatest], true))
			if draws, err := fixture.fetch(); err == nil || draws != nil {
				t.Fatalf("invalid 163 mother response was accepted: draws=%v err=%v", draws, err)
			}
		})
	}
}

func TestSGSSCWindowFailsClosed(t *testing.T) {
	first, second := sgSSCStations()[0], sgSSCStations()[1]
	firstLatest, secondLatest := sgSSCEndpoint(first, ""), sgSSCEndpoint(second, "")
	firstHistory, secondHistory := sgSSCEndpoint(first, "2026-09-03"), sgSSCEndpoint(second, "2026-09-03")
	for _, test := range []struct {
		name   string
		mutate func(*sgSSCTestFixture)
	}{
		{"missing station", func(f *sgSSCTestFixture) { delete(f.latest, secondLatest) }},
		{"empty latest", func(f *sgSSCTestFixture) { f.overrides[secondLatest] = nil }},
		{"oversize latest", func(f *sgSSCTestFixture) { f.overrides[firstLatest] = make([]byte, sgSSCMaxResponseSize+1) }},
		{"different latest issue", func(f *sgSSCTestFixture) { f.latest[secondLatest] = sgSSCTestLatestRow("20260903029", second) }},
		{"different latest balls", func(f *sgSSCTestFixture) { f.latest[secondLatest]["preDrawCode"] = "1,2,3,4,5" }},
		{"same balls different order", func(f *sgSSCTestFixture) { f.latest[secondLatest]["preDrawCode"] = "4,3,2,1,0" }},
		{"different latest time", func(f *sgSSCTestFixture) { f.latest[secondLatest]["preDrawTime"] = "2026-09-03 02:30:01" }},
		{"different next time", func(f *sgSSCTestFixture) { f.latest[secondLatest]["drawTime"] = "2026-09-03 02:40:00" }},
		{"latest absent from history", func(f *sgSSCTestFixture) { f.histories[firstHistory] = f.histories[firstHistory][1:] }},
		{"latest disagrees with own history", func(f *sgSSCTestFixture) { f.histories[firstHistory][0]["preDrawCode"] = "1,1,1,1,1" }},
		{"history disagreement", func(f *sgSSCTestFixture) { f.histories[secondHistory][5]["preDrawCode"] = "9,9,9,9,9" }},
		{"gap in required window", func(f *sgSSCTestFixture) {
			f.histories[secondHistory] = append(f.histories[secondHistory][:8], f.histories[secondHistory][9:]...)
		}},
		{"duplicate even identical", func(f *sgSSCTestFixture) {
			f.histories[firstHistory] = append(f.histories[firstHistory], f.histories[firstHistory][0])
		}},
		{"history ahead of latest", func(f *sgSSCTestFixture) {
			f.histories[firstHistory] = append([]map[string]any{sgSSCTestRawRow("20260903031")}, f.histories[firstHistory]...)
		}},
		{"too many daily rows", func(f *sgSSCTestFixture) {
			for len(f.histories[firstHistory]) <= 288 {
				f.histories[firstHistory] = append(f.histories[firstHistory], sgSSCTestRawRow("20260903001"))
			}
		}},
		{"malformed outside window also rejected", func(f *sgSSCTestFixture) { f.histories[firstHistory][29]["preDrawCode"] = "bad" }},
		{"stale at next boundary", func(f *sgSSCTestFixture) { f.now = f.now.Add(4*time.Minute + 30*time.Second) }},
		{"stale by one whole period", func(f *sgSSCTestFixture) { f.now = f.now.Add(5 * time.Minute) }},
		{"future raw draw despite cached server time", func(f *sgSSCTestFixture) { f.now = f.now.Add(-31 * time.Second) }},
		{"zero local clock", func(f *sgSSCTestFixture) { f.now = time.Time{} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newSGSSCTestFixture("20260903030")
			test.mutate(fixture)
			if draws, err := fixture.fetch(); err == nil || draws != nil {
				t.Fatalf("must fail with no partial or single-site result: draws=%v err=%v", draws, err)
			}
		})
	}
}

func TestSGSSCWindowDoesNotImportOlderGaps(t *testing.T) {
	fixture := newSGSSCTestFixture("20260903030")
	endpoint := sgSSCEndpoint(sgSSCStations()[1], "2026-09-03")
	// Only 007..030 are required. A missing 006 is not silently manufactured,
	// imported, or confused with proof that the whole issue day is continuous.
	fixture.histories[endpoint] = append(fixture.histories[endpoint][:24], fixture.histories[endpoint][25:]...)
	draws, err := fixture.fetch()
	if err != nil || len(draws) != 24 || draws[0].Issue != "20260903007" {
		t.Fatalf("bounded window should remain valid: %+v %v", draws, err)
	}
}

func TestSGSSCWindowNeedsPreviousIssueDate(t *testing.T) {
	fixture := newSGSSCTestFixture("20260903001")
	delete(fixture.histories, sgSSCEndpoint(sgSSCStations()[1], "2026-09-02"))
	if draws, err := fixture.fetch(); err == nil || draws != nil {
		t.Fatalf("must not accept a shortened post-midnight window: %+v %v", draws, err)
	}
}

func TestSGSSCRechecksFreshnessAfterNetwork(t *testing.T) {
	fixture := newSGSSCTestFixture("20260903030")
	clockReads := 0
	draws, err := fetchSGSSCVerifiedWithRequest(context.Background(), func() time.Time {
		clockReads++
		if clockReads == 1 {
			return fixture.now
		}
		return fixture.now.Add(5 * time.Minute)
	}, fixture.request)
	if err == nil || draws != nil || clockReads != 2 || len(fixture.calls) != 4 {
		t.Fatalf("boundary during fetch must reject stale result: reads=%d draws=%v err=%v", clockReads, draws, err)
	}
}

func TestSGSSCStrictBusinessEnvelope(t *testing.T) {
	for _, body := range []string{
		"", "null", "[]", "{}", "{", `{"errorCode":1,"result":{"businessCode":0,"data":{}}}`,
		`{"result":{"businessCode":0,"data":{}}}`, `{"errorCode":null,"result":{"businessCode":0,"data":{}}}`,
		`{"errorCode":"0","result":{"businessCode":0,"data":{}}}`, `{"errorCode":0.0,"result":{"businessCode":0,"data":{}}}`,
		`{"errorCode":0,"result":null}`, `{"errorCode":0,"result":{"data":{}}}`,
		`{"errorCode":0,"result":{"businessCode":null,"data":{}}}`, `{"errorCode":0,"result":{"businessCode":1,"data":{}}}`,
		`{"errorCode":0,"result":{"businessCode":"0","data":{}}}`, `{"errorCode":0,"result":{"businessCode":0,"data":{}}} {}`,
		`{"errorCode":1,"errorCode":0,"result":{"businessCode":0,"data":{}}}`,
		`{"ErrorCode":1,"errorCode":0,"result":{"businessCode":0,"data":{}}}`,
		`{"errorCode":0,"result":{"businessCode":1,"businessCode":0,"data":{}}}`,
		`{"errorCode":0,"result":{"businessCode":0,"data":{"preDrawIssue":1,"preDrawIssue":2}}}`,
		`{"errorCode":0,"result":{"businessCode":0,"data":[{"preDrawCode":"1","preDrawCode":"2"}]}}`,
	} {
		t.Run(body, func(t *testing.T) {
			if _, err := decodeSGSSCData([]byte(body)); err == nil {
				t.Fatalf("accepted invalid envelope: %s", body)
			}
		})
	}
	if _, err := decodeSGSSCData(sgSSCTestEnvelope(map[string]any{"raw": "data"})); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeSGSSCData([]byte(strings.Repeat("[", 34) + "0" + strings.Repeat("]", 34))); err == nil {
		t.Fatal("excessively nested JSON must be rejected")
	}
}

func TestSGSSCStrictLatestContract(t *testing.T) {
	station := sgSSCStations()[0]
	for _, test := range []struct {
		name, field string
		value       any
	}{
		{"wrong numeric product", "lotCode", 10059}, {"wrong string product", "lotCode", "sgssc"},
		{"null product", "lotCode", nil}, {"wrong name", "lotName", "幸运时时彩"}, {"missing name", "lotName", nil},
		{"wrong period count", "totalCount", 300}, {"missing period count", "totalCount", nil},
		{"wrong issue length", "preDrawIssue", "202609030030"}, {"zero ordinal", "preDrawIssue", "20260903000"},
		{"overflow ordinal", "preDrawIssue", "20260903289"}, {"invalid date", "preDrawIssue", "20260230030"},
		{"fractional issue", "preDrawIssue", json.Number("20260903030.0")}, {"exponential issue", "preDrawIssue", json.Number("2.0260903030e10")},
		{"boolean issue", "preDrawIssue", true}, {"padded issue", "preDrawIssue", " 20260903030"},
		{"missing issue", "preDrawIssue", nil}, {"bad day time", "preDrawTime", "2026-09-02 02:30:00"},
		{"bad seconds", "preDrawTime", "2026-09-03 02:30:01"}, {"noncanonical seconds", "preDrawTime", "2026-09-03 02:30:00.000"},
		{"timezone suffix", "preDrawTime", "2026-09-03 02:30:00+08:00"}, {"wrong time type", "preDrawTime", 1788373800},
		{"missing next issue", "drawIssue", nil}, {"skipped next issue", "drawIssue", "20260903032"},
		{"old next issue", "drawIssue", "20260903030"}, {"missing next time", "drawTime", nil},
		{"wrong next time", "drawTime", "2026-09-03 02:40:00"}, {"fractional next seconds", "drawTime", "2026-09-03 02:35:00.000"},
		{"four balls", "preDrawCode", "0,1,2,3"}, {"six balls", "preDrawCode", "0,1,2,3,4,5"},
		{"out of range", "preDrawCode", "0,1,2,3,10"}, {"negative ball", "preDrawCode", "0,1,2,3,-1"},
		{"padded ball", "preDrawCode", "0,1,2,3, 4"}, {"two digit ball", "preDrawCode", "0,1,2,3,04"},
		{"decimal ball", "preDrawCode", "0,1,2,3,4.0"}, {"empty ball", "preDrawCode", "0,1,2,3,"},
		{"wrong separators", "preDrawCode", "0 1 2 3 4"}, {"unicode ball", "preDrawCode", "0,1,2,3,４"},
		{"non-string raw code", "preDrawCode", []int{0, 1, 2, 3, 4}},
	} {
		t.Run(test.name, func(t *testing.T) {
			row := sgSSCTestLatestRow("20260903030", station)
			row[test.field] = test.value
			if draw, err := parseSGSSCLatest(sgSSCTestEnvelope(row), station); err == nil {
				t.Fatalf("accepted %s: %+v", test.name, draw)
			}
		})
	}
	for _, value := range []any{nil, []any{}, "wrong", 2} {
		if _, err := parseSGSSCLatest(sgSSCTestEnvelope(value), station); err == nil {
			t.Fatalf("accepted invalid latest data %v", value)
		}
	}
	row := sgSSCTestLatestRow("20260903030", station)
	row["preDrawCode"] = "0,0,0,0,0"
	draw, err := parseSGSSCLatest(sgSSCTestEnvelope(row), station)
	if err != nil || !reflect.DeepEqual(draw.Numbers, []int{0, 0, 0, 0, 0}) {
		t.Fatalf("SG repeats and zeros are valid raw balls: %+v %v", draw, err)
	}
}

func TestSGSSCStrictHistoryData(t *testing.T) {
	station := sgSSCStations()[0]
	latest, err := parseSGSSCLatest(sgSSCTestEnvelope(sgSSCTestLatestRow("20260903030", station)), station)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range []any{nil, []any{}, map[string]any{}, "bad", []any{nil}, []any{"bad"}} {
		if _, err := parseSGSSCHistory(sgSSCTestEnvelope(data), station, "2026-09-03", latest); err == nil {
			t.Fatalf("accepted invalid history data %v", data)
		}
	}
}

func TestSGSSCPeriodRoundTrip(t *testing.T) {
	for _, test := range []struct{ issue, at string }{
		{"20260902288", "2026-09-03 00:00:00"}, {"20260903001", "2026-09-03 00:05:00"},
		{"20261231288", "2027-01-01 00:00:00"}, {"20240229288", "2024-03-01 00:00:00"},
	} {
		_, _, at, err := parseSGSSCIssue(test.issue)
		if err != nil || at.In(sgSSCLocation).Format("2006-01-02 15:04:05") != test.at || sgSSCIssueAt(at) != test.issue {
			t.Fatalf("issue midnight contract: %s %v %v", test.issue, at, err)
		}
	}
	for ordinal := 1; ordinal <= 288; ordinal++ {
		issue := fmt.Sprintf("20260903%03d", ordinal)
		_, actualOrdinal, at, err := parseSGSSCIssue(issue)
		if err != nil || actualOrdinal != ordinal || sgSSCIssueAt(at) != issue {
			t.Fatalf("daily issue roundtrip failed: %s %v", issue, err)
		}
	}
}

func TestSGSSCVerifiedBatchImportGuard(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func([]sourceDraw) []sourceDraw
	}{
		{"empty", func([]sourceDraw) []sourceDraw { return nil }},
		{"partial window", func(d []sourceDraw) []sourceDraw { return d[1:] }},
		{"unbounded window", func(d []sourceDraw) []sourceDraw { return append(d, d[0]) }},
		{"wrong source revision", func(d []sourceDraw) []sourceDraw { d[0].SourceRevision = "platform"; return d }},
		{"wrong conversion revision", func(d []sourceDraw) []sourceDraw { d[0].ConversionRevision = ""; return d }},
		{"invalid issue", func(d []sourceDraw) []sourceDraw { d[0].Issue = "bad"; return d }},
		{"draw time mismatch", func(d []sourceDraw) []sourceDraw { d[0].DrawAt = d[0].DrawAt.Add(time.Second); return d }},
		{"ball count", func(d []sourceDraw) []sourceDraw { d[0].Numbers = []int{1, 2, 3}; return d }},
		{"ball negative", func(d []sourceDraw) []sourceDraw { d[0].Numbers[0] = -1; return d }},
		{"ball overflow", func(d []sourceDraw) []sourceDraw { d[0].Numbers[0] = 10; return d }},
		{"unverified next metadata", func(d []sourceDraw) []sourceDraw { d[23].NextIssue = ""; return d }},
		{"next time missing", func(d []sourceDraw) []sourceDraw { d[23].NextDrawAt = time.Time{}; return d }},
		{"duplicate row", func(d []sourceDraw) []sourceDraw { d[1] = d[0]; return d }},
		{"wrong order", func(d []sourceDraw) []sourceDraw { d[0], d[1] = d[1], d[0]; return d }},
	} {
		t.Run(test.name, func(t *testing.T) {
			draws, err := newSGSSCTestFixture("20260903030").fetch()
			if err != nil {
				t.Fatal(err)
			}
			if err := validateSGSSCVerifiedBatch(test.mutate(draws)); err == nil {
				t.Fatal("invalid imported batch must be rejected")
			}
		})
	}
}

func TestSGSSCRequestCancellationAndDependencies(t *testing.T) {
	fixture := newSGSSCTestFixture("20260903030")
	clock := func() time.Time { return fixture.now }
	for _, test := range []struct {
		name    string
		ctx     context.Context
		now     func() time.Time
		request sgSSCRequest
	}{
		{"nil context", nil, clock, fixture.request}, {"nil clock", context.Background(), nil, fixture.request},
		{"nil request", context.Background(), clock, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			if draws, err := fetchSGSSCVerifiedWithRequest(test.ctx, test.now, test.request); err == nil || draws != nil {
				t.Fatalf("invalid dependency: %v %v", draws, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls atomic.Int32
	request := func(ctx context.Context, _ string) ([]byte, error) {
		calls.Add(1)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if draws, err := fetchSGSSCVerifiedWithRequest(ctx, clock, request); !errors.Is(err, context.Canceled) || draws != nil || calls.Load() != 0 {
		t.Fatalf("pre-canceled poll should not request: %v %v calls=%d", draws, err, calls.Load())
	}
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	if draws, err := fetchSGSSCVerifiedWithRequest(ctx, clock, request); !errors.Is(err, context.DeadlineExceeded) || draws != nil || time.Since(started) > time.Second {
		t.Fatalf("request cancellation not propagated promptly: %v %v", draws, err)
	}
}

func TestSGSSCStationFailureCancelsPeer(t *testing.T) {
	peerStarted, peerCanceled := make(chan struct{}), make(chan struct{})
	request := func(ctx context.Context, endpoint string) ([]byte, error) {
		if strings.Contains(endpoint, "115kai") {
			close(peerStarted)
			<-ctx.Done()
			close(peerCanceled)
			return nil, ctx.Err()
		}
		select {
		case <-peerStarted:
			return nil, errors.New("168 unavailable")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if draws, err := fetchSGSSCVerifiedWithRequest(ctx, time.Now, request); err == nil || draws != nil {
		t.Fatalf("station failure must fail closed: %v %v", draws, err)
	}
	select {
	case <-peerCanceled:
	default:
		t.Fatal("peer request was not canceled")
	}
}

type sgSSCTestRoundTripper func(*http.Request) (*http.Response, error)

func (f sgSSCTestRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestSGSSCHTTPContract(t *testing.T) {
	for _, station := range sgSSCStations() {
		for _, date := range []string{"", "2026-09-02"} {
			endpoint := sgSSCEndpoint(station, date)
			client := &http.Client{Timeout: time.Hour, Transport: sgSSCTestRoundTripper(func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodGet || request.URL.String() != endpoint || request.Header.Get("Referer") != station.referer || request.Header.Get("User-Agent") == "" || request.Header.Get("Authorization") != "" {
					t.Fatalf("unexpected public request contract: %+v", request)
				}
				deadline, ok := request.Context().Deadline()
				if !ok || time.Until(deadline) > sgSSCRequestTimeout {
					t.Fatal("per-request timeout is not bounded")
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}")), Header: http.Header{}}, nil
			})}
			body, err := requestSGSSCJSONWithClient(context.Background(), endpoint, client)
			if err != nil || string(body) != "{}" || client.Timeout != time.Hour {
				t.Fatalf("request or copied-client contract failed: %s %v", body, err)
			}
		}
	}
}

func TestSGSSCHTTPRejectsUnexpectedDestinations(t *testing.T) {
	base := "https://api.api168168.com"
	for _, endpoint := range []string{
		"http://api.api168168.com" + sgSSCLatestPath + "?lotCode=10075",
		"https://api.api16868.com" + sgSSCLatestPath + "?lotCode=10075",
		base + sgSSCLatestPath + "?lotCode=10059", base + sgSSCLatestPath + "?lotCode=10075&lotCode=10059",
		base + sgSSCLatestPath + "?lotCode=10075&date=2026-09-02", base + sgSSCLatestPath + "?lotCode=10075&apiKey=unused",
		base + sgSSCLatestPath + "?lotCode=10075&broken=%zz", base + sgSSCLatestPath + "?lotCode=10075#fragment",
		base + "/unlisted?lotCode=10075", base + sgSSCHistoryPath + "?lotCode=10075",
		base + sgSSCHistoryPath + "?lotCode=10075&date=2026-02-30", base + sgSSCHistoryPath + "?lotCode=10075&date=2026-09-02&date=2026-09-03",
		"https://name@api.api168168.com" + sgSSCLatestPath + "?lotCode=10075",
		"https://api.api168168.com:443" + sgSSCLatestPath + "?lotCode=10075",
		base + "/CQShiCai%2fgetBaseCQShiCai.do?lotCode=10075", "%",
	} {
		t.Run(endpoint, func(t *testing.T) {
			client := &http.Client{Transport: sgSSCTestRoundTripper(func(*http.Request) (*http.Response, error) {
				t.Fatal("invalid URL must not cause a network request")
				return nil, nil
			})}
			if body, err := requestSGSSCJSONWithClient(context.Background(), endpoint, client); err == nil || body != nil {
				t.Fatalf("accepted unlisted endpoint: %s %v", body, err)
			}
		})
	}
}

func TestSGSSCHTTPRejectsBadResponsesAndRedirects(t *testing.T) {
	for _, test := range []struct {
		name string
		code int
		body string
	}{
		{"non-success status", 503, `{}`}, {"other 2xx", 201, `{}`}, {"empty response", 200, ""},
		{"oversize response", 200, strings.Repeat("x", sgSSCMaxResponseSize+1)}, {"redirect", 302, `{}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: sgSSCTestRoundTripper(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: test.code, Body: io.NopCloser(strings.NewReader(test.body)), Header: http.Header{"Location": {"https://unexpected.example/"}}}, nil
			})}
			body, err := requestSGSSCJSONWithClient(context.Background(), sgSSCEndpoint(sgSSCStations()[0], ""), client)
			if err == nil || body != nil || calls != 1 {
				t.Fatalf("bad response accepted or redirect followed: calls=%d body=%s err=%v", calls, body, err)
			}
		})
	}
}

// Explicitly opt in to public, read-only network checks; normal tests are fully
// offline and never need a database, API key or deployment credentials.
func TestSGSSCLiveVerifiedWindow(t *testing.T) {
	if os.Getenv("SGSSC_LIVE_TEST") != "1" {
		t.Skip("set SGSSC_LIVE_TEST=1 to verify the public 168/115 recent window")
	}
	var mu sync.Mutex
	var requests []string
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	draws, err := fetchSGSSCVerifiedWithRequest(ctx, time.Now, func(ctx context.Context, endpoint string) ([]byte, error) {
		mu.Lock()
		requests = append(requests, endpoint)
		mu.Unlock()
		body, err := requestSGSSCJSON(ctx, endpoint)
		if err == nil && strings.Contains(endpoint, sgSSCLatestPath+"?") {
			data, parseErr := decodeSGSSCData(body)
			var row sgSSCRow
			if parseErr == nil && json.Unmarshal(data, &row) == nil {
				t.Logf("raw latest %s: lotCode=%s preDrawIssue=%s preDrawTime=%q preDrawCode=%q drawIssue=%s drawTime=%q",
					endpoint, row.LotCode, row.Issue, row.DrawTime, row.Code, row.NextIssue, row.NextDrawTime)
			}
		}
		return body, err
	})
	if err != nil {
		t.Fatalf("public source verification failed closed: %v; requests=%v", err, requests)
	}
	if len(draws) != 24 || len(requests) < 4 || len(requests) > 6 {
		t.Fatalf("unexpected live bounded result: draws=%d requests=%d", len(draws), len(requests))
	}
	for _, endpoint := range requests {
		parsed, _ := url.Parse(endpoint)
		t.Logf("public GET host=%s path=%s query=%s", parsed.Host, parsed.Path, parsed.RawQuery)
	}
	first, latest := draws[0], draws[len(draws)-1]
	t.Logf("verified %d consecutive periods: %s (%s) to %s (%s), latest balls=%v, next=%s (%s); %d GETs; not proof of independent upstreams or whole-day continuity",
		len(draws), first.Issue, first.DrawAt.In(sgSSCLocation).Format(time.DateTime), latest.Issue,
		latest.DrawAt.In(sgSSCLocation).Format(time.DateTime), latest.Numbers, latest.NextIssue,
		latest.NextDrawAt.In(sgSSCLocation).Format(time.DateTime), len(requests))
}
