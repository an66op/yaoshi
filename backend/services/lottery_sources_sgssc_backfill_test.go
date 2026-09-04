package services

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func sgSSCHistoryTestNow() time.Time {
	return time.Date(2026, 9, 3, 17, 30, 30, 0, sgSSCLocation)
}

func newSGSSCHistoryTestFixture(now time.Time) *sgSSCTestFixture {
	fixture := newSGSSCTestFixture(sgSSCIssueAt(now.Truncate(sgSSCInterval)))
	fixture.now = now
	return fixture
}

func fetchSGSSCHistoryTestFixture(f *sgSSCTestFixture, issues []string) (SGSSCHistoryVerification, error) {
	return fetchSGSSCVerifiedHistoryWithRequest(context.Background(), issues, func() time.Time { return f.now }, f.request)
}

func removeSGSSCHistoryTestIssue(f *sgSSCTestFixture, endpoint, issue string) {
	rows := f.histories[endpoint]
	for index, row := range rows {
		if fmt.Sprint(row["preDrawIssue"]) == issue {
			f.histories[endpoint] = append(rows[:index:index], rows[index+1:]...)
			return
		}
	}
}

func TestSGSSCHistoryBackfillUsesOnly163FiniteMotherHistory(t *testing.T) {
	now := sgSSCHistoryTestNow()
	issues := []string{"20260903185", "20260903199", "20260903209"}
	f := newSGSSCHistoryTestFixture(now)
	original := append([]string(nil), issues...)
	result, err := fetchSGSSCHistoryTestFixture(f, issues)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(issues, original) || len(result.Failures) != 0 || len(result.Draws) != len(issues) {
		t.Fatalf("unexpected verified finite history: %+v", result)
	}
	for index, draw := range result.Draws {
		if draw.Issue != issues[index] || draw.SourceRevision != sgSSCSourceRevision || draw.ConversionRevision != sgSSCConversionRevision ||
			draw.NextIssue != "" || !draw.NextDrawAt.IsZero() || draw.DrawAt.Location() != time.UTC {
			t.Fatalf("history row leaked schedule or wrong provenance: %+v", draw)
		}
	}
	var saw163Latest, saw163History, saw115Latest, saw115History bool
	for _, endpoint := range f.calls {
		saw163Latest = saw163Latest || strings.Contains(endpoint, source163LatestPath) && strings.Contains(endpoint, "iGameId=64")
		saw163History = saw163History || strings.Contains(endpoint, source163HistoryPath) && strings.Contains(endpoint, "count=32")
		saw115Latest = saw115Latest || endpoint == sgSSCEndpoint(sgSSCValidationStation(), "")
		saw115History = saw115History || endpoint == sgSSCEndpoint(sgSSCValidationStation(), "2026-09-03")
		if strings.Contains(endpoint, "api.api168168.com") {
			t.Fatalf("legacy 168 source was still called: %s", endpoint)
		}
	}
	if !saw163Latest || !saw163History || !saw115Latest || !saw115History || len(f.calls) != 4 {
		t.Fatalf("unexpected source request set: %v", f.calls)
	}
}

func TestSGSSCHistoryBackfillClassifiesMotherAndVerifierFailures(t *testing.T) {
	now := sgSSCHistoryTestNow()
	motherEndpoint := sgSSCEndpoint(sgSSCStations()[0], "2026-09-03") // fixture backing data for 163
	verifierEndpoint := sgSSCEndpoint(sgSSCValidationStation(), "2026-09-03")
	for _, test := range []struct {
		name      string
		issue     string
		mutate    func(*sgSSCTestFixture)
		permanent bool
		contains  string
	}{
		{
			name: "mother missing", issue: "20260903200", permanent: true, contains: "163母源有限历史",
			mutate: func(f *sgSSCTestFixture) { removeSGSSCHistoryTestIssue(f, motherEndpoint, "20260903200") },
		},
		{
			name: "verifier missing", issue: "20260903200", permanent: false, contains: "115校验源",
			mutate: func(f *sgSSCTestFixture) { removeSGSSCHistoryTestIssue(f, verifierEndpoint, "20260903200") },
		},
		{
			name: "disagreement", issue: "20260903200", permanent: false, contains: "五球或时间不一致",
			mutate: func(f *sgSSCTestFixture) {
				for _, row := range f.histories[verifierEndpoint] {
					if fmt.Sprint(row["preDrawIssue"]) == "20260903200" {
						row["preDrawCode"] = "9,9,9,9,9"
					}
				}
			},
		},
		{
			name: "outside finite mother window", issue: "20260902085", permanent: true, contains: "163母源有限历史",
			mutate: func(f *sgSSCTestFixture) {
				station := sgSSCValidationStation()
				f.histories[sgSSCEndpoint(station, "2026-09-02")] = []map[string]any{
					sgSSCTestRawRow("20260902085"),
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newSGSSCHistoryTestFixture(now)
			test.mutate(f)
			result, err := fetchSGSSCHistoryTestFixture(f, []string{test.issue})
			if err != nil || len(result.Draws) != 0 || len(result.Failures) != 1 ||
				result.Failures[0].Permanent != test.permanent || !strings.Contains(result.Failures[0].Error, test.contains) {
				t.Fatalf("failure classification=%+v err=%v", result, err)
			}
		})
	}
}

func TestSGSSCHistoryBackfillRejectsUntrustedWholeResponses(t *testing.T) {
	now := sgSSCHistoryTestNow()
	issues := []string{"20260903200"}
	for _, test := range []struct {
		name   string
		mutate func(*sgSSCTestFixture)
	}{
		{"wrong 163 identity", func(f *sgSSCTestFixture) {
			f.latest[sgSSCEndpoint(sgSSCStations()[0], "")]["preDrawIssue"] = "bad"
		}},
		{"duplicate 163 JSON key", func(f *sgSSCTestFixture) {
			f.overrides[sgSSCEndpoint(sgSSCStations()[0], "")] = []byte(`{"success":false,"success":true,"result":{}}`)
		}},
		{"115 identity malformed", func(f *sgSSCTestFixture) {
			f.latest[sgSSCEndpoint(sgSSCValidationStation(), "")]["lotName"] = "幸运时时彩"
		}},
		{"115 response missing", func(f *sgSSCTestFixture) {
			delete(f.histories, sgSSCEndpoint(sgSSCValidationStation(), "2026-09-03"))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newSGSSCHistoryTestFixture(now)
			test.mutate(f)
			result, err := fetchSGSSCHistoryTestFixture(f, issues)
			if err == nil || len(result.Draws) != 0 || len(result.Failures) != 0 {
				t.Fatalf("untrusted whole response produced partial evidence: %+v err=%v", result, err)
			}
		})
	}
}

func TestSGSSCHistoryBackfillLimitsAndCancellation(t *testing.T) {
	now := sgSSCHistoryTestNow()
	valid := []string{"20260903200"}
	for _, test := range []struct {
		name   string
		now    time.Time
		issues []string
	}{
		{"zero time", time.Time{}, valid},
		{"empty", now, nil},
		{"duplicate", now, []string{valid[0], valid[0]}},
		{"bad issue", now, []string{"bad"}},
		{"future", now, []string{"20260903211"}},
		{"three dates", now, []string{"20260901001", "20260902001", "20260903001"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			request := func(context.Context, string) ([]byte, error) {
				calls.Add(1)
				return nil, errors.New("must validate before I/O")
			}
			result, err := fetchSGSSCVerifiedHistoryWithRequests(context.Background(), test.issues, func() time.Time { return test.now }, strings.NewReader(strings.Repeat("0", 100)), request, request)
			if err == nil || calls.Load() != 0 || len(result.Draws) != 0 || len(result.Failures) != 0 {
				t.Fatalf("invalid request scope reached I/O: %+v err=%v calls=%d", result, err, calls.Load())
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls atomic.Int32
	request := func(ctx context.Context, _ string) ([]byte, error) {
		calls.Add(1)
		return nil, ctx.Err()
	}
	result, err := fetchSGSSCVerifiedHistoryWithRequests(ctx, valid, func() time.Time { return now }, strings.NewReader(strings.Repeat("0", 100)), request, request)
	if !errors.Is(err, context.Canceled) || calls.Load() != 0 || len(result.Draws) != 0 {
		t.Fatalf("pre-canceled backfill performed I/O: %+v err=%v calls=%d", result, err, calls.Load())
	}
}

func TestSGSSCHistoryBackfillValidationSuccessfulSubset(t *testing.T) {
	now := sgSSCHistoryTestNow()
	issues := []string{"20260903190", "20260903200", "20260903209"}
	f := newSGSSCHistoryTestFixture(now)
	verified, err := fetchSGSSCHistoryTestFixture(f, issues)
	if err != nil || len(verified.Draws) != 3 {
		t.Fatalf("fixture verification failed: %+v err=%v", verified, err)
	}
	for _, subset := range [][]sourceDraw{nil, {}, verified.Draws[1:2], {verified.Draws[0], verified.Draws[2]}} {
		if err := validateSGSSCVerifiedHistoryBatch(subset, issues, now); err != nil {
			t.Fatalf("valid subset rejected: %v", err)
		}
	}
	bad := append([]sourceDraw(nil), verified.Draws...)
	bad[0].SourceRevision = sgSSCLegacySourceRevision
	if err := validateSGSSCVerifiedHistoryBatch(bad, issues, now); err == nil {
		t.Fatal("legacy evidence was relabeled as current backfill")
	}
}
