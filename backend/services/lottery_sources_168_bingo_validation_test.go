package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func valid168BingoRawNumbers() []int {
	return []int{5, 7, 8, 9, 11, 14, 16, 21, 23, 27, 30, 32, 44, 46, 66, 67, 68, 70, 71, 80}
}

func bingo168Payload(t *testing.T, rows ...api168Row) api168Envelope {
	t.Helper()
	data, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	var payload api168Envelope
	payload.Result.Data = data
	return payload
}

func valid168BingoRow(issue string) api168Row {
	return api168Row{
		Issue: issue, Time: "2026-09-01 12:00:00", Code: joinNumbers(valid168BingoRawNumbers()),
		NextIssue: "00115099833", NextTime: "2026-09-01 12:05:00",
	}
}

func TestParse168BingoNumbersStrict(t *testing.T) {
	want := valid168BingoRawNumbers()
	valid := joinNumbers(want)
	for _, code := range []string{valid, " 05, 07,08,09,11,14,16,21,23,27,30,32,44,46,66,67,68,70,71,80 \n"} {
		got, err := parse168BingoNumbers(code)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("valid source order or values changed: got=%v err=%v", got, err)
		}
	}
	for _, duplicate := range []int{want[0], want[5], want[19]} {
		got, err := parse168BingoNumbers(valid + "," + strconv.Itoa(duplicate))
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("known trailing-duplicate provider defect was not normalized: duplicate=%d got=%v err=%v", duplicate, got, err)
		}
	}
	for _, test := range []struct{ name, code string }{
		{"no numbers", ""},
		{"nineteen balls", joinNumbers(want[:19])},
		{"twenty-one distinct balls", valid + ",1"},
		{"twenty-two balls", valid + ",5,7"},
		{"invalid trailing duplicate token", valid + ",broken"},
		{"empty trailing duplicate token", valid + ","},
		{"out-of-range trailing duplicate token", valid + ",81"},
		{"first twenty duplicate even with duplicate tail", strings.Replace(valid, ",7,", ",5,", 1) + ",5"},
		{"extra broken token previously dropped", valid + ",broken"},
		{"leading empty item", "," + valid},
		{"trailing empty item", valid + ","},
		{"extra empty item", strings.Replace(valid, ",", ",,", 1)},
		{"empty first item", strings.TrimPrefix(valid, "5")},
		{"empty last item", strings.TrimSuffix(valid, "80")},
		{"empty middle item", strings.Replace(valid, ",7,", ",,", 1)},
		{"whitespace item", strings.Replace(valid, ",7,", ", \t ,", 1)},
		{"broken token", strings.Replace(valid, ",7,", ",broken,", 1)},
		{"fractional token", strings.Replace(valid, ",7,", ",7.0,", 1)},
		{"signed token", strings.Replace(valid, ",7,", ",+7,", 1)},
		{"non-ASCII digit", strings.Replace(valid, ",7,", ",７,", 1)},
		{"overflow token", strings.Replace(valid, ",7,", ",999999999999999999999999999999,", 1)},
		{"zero", strings.Replace(valid, ",7,", ",0,", 1)},
		{"negative", strings.Replace(valid, ",7,", ",-7,", 1)},
		{"above eighty", strings.Replace(valid, ",7,", ",81,", 1)},
		{"duplicate", strings.Replace(valid, ",7,", ",5,", 1)},
		{"zero-padded duplicate", strings.Replace(valid, ",7,", ",05,", 1)},
		{"non-source separator", strings.ReplaceAll(valid, ",", "+")},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := parse168BingoNumbers(test.code)
			if !errors.Is(err, err168BingoRawInvalid) || got != nil {
				t.Fatalf("damaged source was accepted or partly returned: got=%v err=%v", got, err)
			}
		})
	}
	boundary := make([]int, 20)
	for i := range boundary {
		boundary[i] = i + 1
	}
	boundary[19] = 80
	if got, err := parse168BingoNumbers(joinNumbers(boundary)); err != nil || !reflect.DeepEqual(got, boundary) {
		t.Fatalf("inclusive 1/80 boundaries rejected: %v err=%v", got, err)
	}
}

func Test168BingoTransformsRejectInvalidRaw(t *testing.T) {
	valid := valid168BingoRawNumbers()
	invalid := [][]int{nil, valid[:5], valid[:7], valid[:10], valid[:19], append(append([]int(nil), valid...), 1), make([]int, 20)}
	for _, replacement := range []int{0, -1, 81, valid[0]} {
		raw := append([]int(nil), valid...)
		raw[19] = replacement
		invalid = append(invalid, raw)
	}
	for _, binding := range api168BingoBindings {
		t.Run(binding.GameID, func(t *testing.T) {
			for _, raw := range invalid {
				if got := binding.Transform(raw); got != nil {
					t.Fatalf("invalid raw was converted or filled into a draw: raw=%v got=%v", raw, got)
				}
			}
		})
	}
}

func Test168BingoTransformsPreserveValidatedOutput(t *testing.T) {
	want := map[string][]int{
		"bingo-ssc-1":    {5, 7, 8, 9, 1},
		"bingo-ssc-2":    {7, 8, 9, 1, 4},
		"bingo-ssc-3":    {8, 9, 1, 4, 6},
		"bingo-ssc-4":    {9, 1, 4, 6, 1},
		"bingo-racing-a": {1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		"bingo-racing-b": {1, 3, 5, 7, 8, 9, 2, 6, 10, 4},
		"bingo-mark-six": {5, 7, 8, 9, 11, 14, 16},
	}
	if len(want) != len(api168BingoBindings) {
		t.Fatal("every Bingo product must have an explicit compatibility fixture")
	}
	for _, binding := range api168BingoBindings {
		t.Run(binding.GameID, func(t *testing.T) {
			raw := valid168BingoRawNumbers()
			before := append([]int(nil), raw...)
			if got := binding.Transform(raw); !reflect.DeepEqual(got, want[binding.GameID]) {
				t.Fatalf("existing conversion changed: got=%v want=%v", got, want[binding.GameID])
			}
			if !reflect.DeepEqual(raw, before) {
				t.Fatalf("source order was mutated: %v", raw)
			}
		})
	}
	// Distinct raw balls may have equal residues. Preserve the existing fill
	// rule for valid sources; only malformed raw data must stop being filled.
	raw := []int{1, 11, 21, 31, 41, 51, 61, 71, 2, 12, 22, 32, 42, 52, 62, 72, 3, 13, 23, 33}
	for offset, expected := range map[int][]int{
		0: {2, 3, 4, 1, 5, 6, 7, 8, 9, 10}, 10: {3, 4, 2, 1, 5, 6, 7, 8, 9, 10},
	} {
		if got := bingoRacingNumbers(offset)(raw); !reflect.DeepEqual(got, expected) {
			t.Fatalf("valid-source racing fill changed: offset=%d got=%v want=%v", offset, got, expected)
		}
	}
}

func TestBingoMarkSixFiltersOneToFortyNineInSourceOrder(t *testing.T) {
	raw := []int{80, 49, 65, 3, 50, 22, 64, 1, 48, 70, 7, 55, 9, 60, 11, 61, 52, 53, 54, 56}
	if got, want := bingoMarkSixNumbers(raw), []int{49, 3, 22, 1, 48, 7, 9}; !reflect.DeepEqual(got, want) {
		t.Fatalf("source-order filter mismatch: got=%v want=%v", got, want)
	}
	insufficient := []int{1, 2, 3, 4, 5, 6, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63}
	if got := bingoMarkSixNumbers(insufficient); got != nil {
		t.Fatalf("fewer than seven eligible balls produced a draw: %v", got)
	}
}

func TestTransform168BingoDrawsRejectsWholeBatchWhenOneIssueCannotMap(t *testing.T) {
	valid := sourceDraw{
		Issue: "00115099832", Numbers: valid168BingoRawNumbers(),
		DrawAt:             time.Date(2026, 9, 1, 12, 0, 0, 0, time.Local),
		BingoOrderVerified: true,
		SourceRevision:     bingoOrderedSourceRevision,
	}
	insufficient := sourceDraw{
		Issue:              "00115099831",
		Numbers:            []int{1, 2, 3, 4, 5, 6, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63},
		BingoOrderVerified: true,
		SourceRevision:     bingoOrderedSourceRevision,
	}
	got, err := transform168BingoDraws("bingo-mark-six", []sourceDraw{valid, insufficient}, bingoMarkSixNumbers)
	if !errors.Is(err, err168BingoRawInvalid) || got != nil {
		t.Fatalf("partially convertible batch was published: got=%+v err=%v", got, err)
	}
	if !strings.Contains(err.Error(), insufficient.Issue) {
		t.Fatalf("conversion error lost the failing issue: %v", err)
	}

	got, err = transform168BingoDraws("bingo-mark-six", []sourceDraw{valid}, bingoMarkSixNumbers)
	if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0].Numbers, []int{5, 7, 8, 9, 11, 14, 16}) || got[0].DrawAt != valid.DrawAt {
		t.Fatalf("valid source metadata or mapping changed: got=%+v err=%v", got, err)
	}
}

func Test168BingoPayloadRejectsWholeResponseBeforeTransform(t *testing.T) {
	good := valid168BingoRow("00115099832")
	bad := valid168BingoRow("00115099831")
	bad.Code += ",broken"
	duplicate := bad
	duplicate.Issue = good.Issue
	missingIssue := good
	missingIssue.Issue = ""
	for _, test := range []struct {
		name string
		rows []api168Row
	}{
		{"bad first", []api168Row{bad, good}},
		{"bad later", []api168Row{good, bad}},
		{"bad duplicate issue", []api168Row{good, duplicate}},
		{"missing issue", []api168Row{good, missingIssue}},
		{"empty row", []api168Row{good, {}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			got, err := sourceDrawsFrom168Payload(bingo168Payload(t, test.rows...), api168KL8, "10047", func(raw []int) []int {
				calls++
				return bingoRacingNumbers(0)(raw)
			})
			if !errors.Is(err, err168BingoRawInvalid) || got != nil || calls != 0 {
				t.Fatalf("bad response reached conversion: rows=%v err=%v transform calls=%d", got, err, calls)
			}
		})
	}
	for _, raw := range []string{`[{`, `[{"preDrawIssue":"123","preDrawCode":12}]`, `[null]`, `"broken"`} {
		var payload api168Envelope
		payload.Result.Data = json.RawMessage(raw)
		if got, err := sourceDrawsFrom168Payload(payload, api168KL8, "10047", nil); !errors.Is(err, err168BingoRawInvalid) || got != nil {
			t.Fatalf("malformed record payload accepted: payload=%s rows=%v err=%v", raw, got, err)
		}
	}
}

func Test168BingoPayloadPreservesSourceMetadataAndDeduplication(t *testing.T) {
	row := valid168BingoRow("00115099832")
	duplicate := row
	duplicate.NextIssue, duplicate.NextTime = nil, ""
	payload := bingo168Payload(t, row, duplicate)
	got, err := sourceDrawsFrom168Payload(payload, api168KL8, "10047", nil)
	want := sourceDraw{
		Issue: "00115099832", Numbers: valid168BingoRawNumbers(), DrawAt: parse168DrawTime(row.Time),
		NextIssue: "00115099833", NextDrawAt: parse168DrawTime(row.NextTime),
	}
	if err != nil || !reflect.DeepEqual(got, []sourceDraw{want}) {
		t.Fatalf("source issue, order or boundary changed: got=%+v err=%v", got, err)
	}
	object, marshalErr := json.Marshal(row)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	payload.Result.Data = object
	if got, err := sourceDrawsFrom168Payload(payload, api168KL8, "10047", nil); err != nil || !reflect.DeepEqual(got, []sourceDraw{want}) {
		t.Fatalf("latest object response changed: got=%+v err=%v", got, err)
	}
}

func Test168BingoPayloadRejectsConflictingDuplicateIssue(t *testing.T) {
	base := valid168BingoRow("00115099832")
	base.Code += "," + strconv.Itoa(valid168BingoRawNumbers()[5])
	for _, test := range []struct {
		name string
		edit func(*api168Row)
	}{
		{"number sequence", func(row *api168Row) {
			numbers := valid168BingoRawNumbers()
			numbers[0], numbers[1] = numbers[1], numbers[0]
			row.Code = joinNumbers(numbers) + "," + strconv.Itoa(numbers[5])
		}},
		{"source tail", func(row *api168Row) {
			row.Code = joinNumbers(valid168BingoRawNumbers()) + "," + strconv.Itoa(valid168BingoRawNumbers()[6])
		}},
		{"draw time", func(row *api168Row) { row.Time = "2026-09-01 12:00:01" }},
		{"next issue", func(row *api168Row) { row.NextIssue = "00115099899" }},
		{"next draw time", func(row *api168Row) { row.NextTime = "2026-09-01 12:06:00" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			conflict := base
			test.edit(&conflict)
			got, err := sourceDrawsFrom168Payload(bingo168Payload(t, base, conflict), api168KL8, "10047", nil)
			if !errors.Is(err, err168BingoRawInvalid) || got != nil {
				t.Fatalf("conflicting same-period rows were deduplicated: rows=%+v err=%v", got, err)
			}
		})
	}
}

func Test168BingoPayloadNormalizesTrailingDuplicateBeforeTransform(t *testing.T) {
	row := valid168BingoRow("00115099832")
	row.Code += "," + strconv.Itoa(valid168BingoRawNumbers()[5])
	want := sourceDraw{
		Issue: "00115099832", Numbers: []int{5, 7, 8, 9, 1}, DrawAt: parse168DrawTime(row.Time),
		NextIssue: "00115099833", NextDrawAt: parse168DrawTime(row.NextTime),
		BingoSourceTail: valid168BingoRawNumbers()[5], HasBingoSourceTail: true,
	}
	for _, objectPayload := range []bool{false, true} {
		payload := bingo168Payload(t, row)
		if objectPayload {
			object, err := json.Marshal(row)
			if err != nil {
				t.Fatal(err)
			}
			payload.Result.Data = object
		}
		got, err := sourceDrawsFrom168Payload(payload, api168KL8, "10047", bingoSSCNumbers(0))
		if err != nil || !reflect.DeepEqual(got, []sourceDraw{want}) {
			t.Fatalf("safe trailing duplicate was not normalized before transform: object=%v rows=%+v err=%v", objectPayload, got, err)
		}
	}
}

func Test168BingoRecentRejectsInvalidLatestOrEitherHistoryBatch(t *testing.T) {
	now := time.Date(2026, 9, 1, 4, 5, 0, 0, time.UTC)
	for _, invalidCall := range []int{1, 2, 3} {
		t.Run("invalid response "+strconv.Itoa(invalidCall), func(t *testing.T) {
			calls := 0
			got, err := fetch168RecentWithRequest(context.Background(), api168KL8, "10047", nil, now,
				func(_ context.Context, endpoint string, payload *api168Envelope) error {
					calls++
					parsed, err := url.Parse(endpoint)
					if err != nil || parsed.Query().Get("lotCode") != "10047" {
						t.Fatalf("wrong Bingo endpoint: %s", endpoint)
					}
					row := valid168BingoRow("00115099832") // Duplicate periods must also be checked.
					if calls == invalidCall {
						row.Code = strings.TrimSuffix(row.Code, ",80")
					}
					*payload = bingo168Payload(t, row)
					return nil
				})
			if !errors.Is(err, err168BingoRawInvalid) || got != nil || calls != invalidCall {
				t.Fatalf("invalid raw left publishable draws: calls=%d rows=%+v err=%v", calls, got, err)
			}
		})
	}
}

func Test168BingoRecentAcceptsTrailingDuplicateInLatestAndHistory(t *testing.T) {
	calls := 0
	got, err := fetch168RecentWithRequest(context.Background(), api168KL8, "10047", nil, time.Date(2026, 9, 1, 4, 5, 0, 0, time.UTC),
		func(_ context.Context, endpoint string, payload *api168Envelope) error {
			calls++
			parsed, parseErr := url.Parse(endpoint)
			if parseErr != nil || parsed.Query().Get("lotCode") != "10047" {
				t.Fatalf("wrong Bingo endpoint: %s", endpoint)
			}
			row := valid168BingoRow("00115099832")
			row.Code += "," + strconv.Itoa(valid168BingoRawNumbers()[5])
			*payload = bingo168Payload(t, row)
			return nil
		})
	if err != nil || calls != 3 || len(got) != 1 || !reflect.DeepEqual(got[0].Numbers, valid168BingoRawNumbers()) {
		t.Fatalf("safe trailing duplicate failed in latest/history chain: rows=%+v calls=%d err=%v", got, calls, err)
	}
}

func Test168BingoRecentRejectsCrossResponseDuplicateConflict(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*api168Row)
	}{
		{"number sequence", func(row *api168Row) {
			numbers := valid168BingoRawNumbers()
			numbers[0], numbers[1] = numbers[1], numbers[0]
			row.Code = joinNumbers(numbers) + "," + strconv.Itoa(numbers[5])
		}},
		{"source tail", func(row *api168Row) {
			row.Code = joinNumbers(valid168BingoRawNumbers()) + "," + strconv.Itoa(valid168BingoRawNumbers()[6])
		}},
		{"draw time", func(row *api168Row) { row.Time = "2026-09-01 12:00:01" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			got, err := fetch168RecentWithRequest(context.Background(), api168KL8, "10047", nil, time.Date(2026, 9, 1, 4, 5, 0, 0, time.UTC),
				func(_ context.Context, _ string, payload *api168Envelope) error {
					calls++
					row := valid168BingoRow("00115099832")
					row.Code += "," + strconv.Itoa(valid168BingoRawNumbers()[5])
					if calls > 1 {
						test.edit(&row)
					}
					*payload = bingo168Payload(t, row)
					return nil
				})
			if !errors.Is(err, err168BingoRawInvalid) || got != nil || calls != 3 {
				t.Fatalf("latest/history conflict was ignored: rows=%+v calls=%d err=%v", got, calls, err)
			}
		})
	}
}

func Test168BingoRecentDoesNotIgnoreMalformedHistoryJSON(t *testing.T) {
	for _, requestError := range []bool{false, true} {
		t.Run(fmt.Sprint("decoder error ", requestError), func(t *testing.T) {
			calls := 0
			got, err := fetch168RecentWithRequest(context.Background(), api168KL8, "10047", nil, time.Now(),
				func(_ context.Context, _ string, payload *api168Envelope) error {
					calls++
					if calls == 1 {
						*payload = bingo168Payload(t, valid168BingoRow("123"))
						return nil
					}
					if requestError {
						var envelope api168Envelope
						return json.Unmarshal([]byte(`{"result":`), &envelope)
					}
					payload.Result.Data = json.RawMessage(`[{"preDrawCode":false}]`)
					return nil
				})
			if !errors.Is(err, err168BingoRawInvalid) || got != nil || calls != 2 {
				t.Fatalf("malformed history was ignored: rows=%v calls=%d err=%v", got, calls, err)
			}
		})
	}
}

func Test168BingoRecentRetainsValidLatestOnHistoryTransportFailure(t *testing.T) {
	calls := 0
	got, err := fetch168RecentWithRequest(context.Background(), api168KL8, "10047", nil, time.Now(),
		func(_ context.Context, _ string, payload *api168Envelope) error {
			calls++
			if calls == 1 {
				*payload = bingo168Payload(t, valid168BingoRow("123"))
				return nil
			}
			return errors.New("history transport unavailable")
		})
	if err != nil || calls != 3 || len(got) != 1 || !reflect.DeepEqual(got[0].Numbers, valid168BingoRawNumbers()) {
		t.Fatalf("valid latest availability changed on transport-only failure: rows=%v calls=%d err=%v", got, calls, err)
	}
}

func Test168DirectSourcePayloadsAreNotSubjectToBingoRawValidation(t *testing.T) {
	for _, test := range []struct {
		series  api168Series
		lotCode string
		code    string
		want    []int
	}{
		{api168PK10, "10037", "10,2,9,1,6,3,4,8,5,7", []int{10, 2, 9, 1, 6, 3, 4, 8, 5, 7}},
		{api168SSC, "10010", "0,9,9,1,2", []int{0, 9, 9, 1, 2}},
		{api168LHC, "10091", "12,9,34,25,40,7,29", []int{12, 9, 34, 25, 40, 7, 29}},
		{api168KL8, "other", "1,2,3", []int{1, 2, 3}},
	} {
		t.Run(string(test.series)+test.lotCode, func(t *testing.T) {
			row := valid168BingoRow("123")
			row.Code = test.code
			got, err := fetch168Latest(context.Background(), test.series, test.lotCode, nil,
				func(_ context.Context, _ string, payload *api168Envelope) error {
					*payload = bingo168Payload(t, row)
					return nil
				})
			if err != nil || len(got) != 1 || got[0].Issue != "123" || !reflect.DeepEqual(got[0].Numbers, test.want) {
				t.Fatalf("direct-source compatibility changed: rows=%v err=%v", got, err)
			}
		})
	}
}
