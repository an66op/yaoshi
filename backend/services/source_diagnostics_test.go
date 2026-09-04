package services

import (
	"backend/data/models/lottery"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type sourceDiagnosticRoundTrip func(*http.Request) (*http.Response, error)

func (f sourceDiagnosticRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Keep production endpoints fixed. A test-only transport forwards those requests
// to httptest, without any live provider, database or global transport mutation.
func sourceDiagnosticTestClient(t *testing.T, handler http.HandlerFunc) *http.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	base, _ := url.Parse(server.URL)
	return &http.Client{Transport: sourceDiagnosticRoundTrip(func(request *http.Request) (*http.Response, error) {
		copy := request.Clone(request.Context())
		endpoint := *request.URL
		endpoint.Scheme, endpoint.Host = base.Scheme, base.Host
		copy.URL = &endpoint
		copy.Host = base.Host
		return server.Client().Transport.RoundTrip(copy)
	})}
}

func sourceDiagnosticTestNow() time.Time { return time.Date(2026, 9, 4, 0, 1, 0, 0, sgSSCLocation) }
func sourceDiagnosticTestRow(id int) map[string]any {
	return map[string]any{"igameid": id, "sgameperiod": "20260903288", "sopennum": "6|0|0|2|9", "dopentime": "2026-09-04 00:00:00"}
}
func sourceDiagnosticEncode(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
func sourceDiagnosticTestProbe(ctx context.Context, key string, client *http.Client) SourceProbeResult {
	return probeSourceDiagnostic(ctx, key, sourceDiagnosticTestNow, client, bytes.NewReader(make([]byte, 100)))
}

func TestSourceDiagnosticCatalogAndCurrentBindings(t *testing.T) {
	specs := sourceDiagnosticSpecs()
	seen := map[string]bool{}
	ids := map[int]bool{}
	for _, spec := range specs {
		if seen[spec.Key] {
			t.Fatalf("duplicate key %s", spec.Key)
		}
		seen[spec.Key] = true
		if strings.ContainsAny(spec.Endpoint, "?#") || spec.GameIDs == nil || spec.Groups == nil || spec.Relation == "" {
			t.Fatalf("unsafe/invalid public contract: %+v", spec.SourceDiagnosticSource)
		}
		if spec.Provider == "163" {
			if spec.Candidate == source163MirrorProductionID(spec.UpstreamGameID) {
				t.Fatalf("163 source candidate/current classification is wrong: %+v", spec.SourceDiagnosticSource)
			}
			if source163MirrorProductionID(spec.UpstreamGameID) && spec.Relation != sourceRelationProduction {
				t.Fatalf("163 production relation is wrong: %+v", spec.SourceDiagnosticSource)
			}
			ids[spec.UpstreamGameID] = true
		}
	}
	if len(ids) != 88 {
		t.Fatalf("got %d 163 IDs", len(ids))
	}
	for _, id := range []int{4, 5, 7, 9, 10, 12, 15, 17, 23, 25, 27, 28, 42, 44, 50, 51, 52, 53, 54, 66, 142, 151} {
		if _, found := sourceDiagnosticSpecForKey(fmt.Sprintf("163:%d", id)); !found {
			t.Fatalf("current 88-item 163 directory is missing ID %d", id)
		}
	}
	for _, id := range []int{181, 182} {
		spec, found := sourceDiagnosticSpecForKey(fmt.Sprintf("163:%d", id))
		if !found || spec.Name != "三分运动会" || !spec.WarningPersistent || !strings.Contains(spec.Warning, "同名") {
			t.Fatalf("duplicate sports directory identity is not explicit: %+v", spec.SourceDiagnosticSource)
		}
	}
	canada, found := sourceDiagnosticSpecForKey("163:57")
	if !found || canada.Relation != sourceRelationProduction || canada.Candidate || !reflect.DeepEqual(canada.GameIDs, []string{"pc-canada", "canada-28", "canada-20"}) || canada.count != 3 || canada.min != 0 || canada.max != 9 || canada.staleAfter != source163MirrorMaxAge {
		t.Fatalf("Canada 28 verified candidate contract is incomplete: %+v", canada)
	}
	for _, id := range []int{37, 36, 41, 48, 137, 60} {
		spec, _ := sourceDiagnosticSpecForKey(fmt.Sprintf("163:%d", id))
		if spec.Warning == "" || spec.WarningCheckedAt == "" {
			t.Fatalf("missing dated warning %d", id)
		}
	}
	for _, id := range []int{160, 162, 163, 164, 165, 167, 168, 169} {
		spec, _ := sourceDiagnosticSpecForKey(fmt.Sprintf("163:%d", id))
		if spec.Relation != sourceRelationDifferentProduct || spec.Warning == "" || !spec.WarningPersistent || spec.WarningCheckedAt == "" {
			t.Fatalf("different product must remain explicit after a successful connectivity test: %+v", spec.SourceDiagnosticSource)
		}
	}
	for _, id := range []int{1, 2, 20, 136, 186, 187, 188, 189, 190, 191, 192} {
		spec, _ := sourceDiagnosticSpecForKey(fmt.Sprintf("163:%d", id))
		if spec.Relation != sourceRelationCrossCheck || spec.Warning == "" || !spec.WarningPersistent {
			t.Fatalf("official aggregator must remain cross-check only: %+v", spec.SourceDiagnosticSource)
		}
	}
	ordered163, found := sourceDiagnosticSpecForKey("163:185")
	if !found || ordered163.Relation != sourceRelationProduction || ordered163.Candidate || ordered163.count != 20 || ordered163.min != 1 || ordered163.max != 80 || !ordered163.unique || ordered163.staleAfter != 9*time.Hour || len(ordered163.GameIDs) != len(bingo163Bindings) {
		t.Fatalf("new ordered Taiwan Bingo candidate is incomplete: %+v", ordered163)
	}
	happy8, found := sourceDiagnosticSpecForKey("163:141")
	if !found || happy8.Relation != sourceRelationProduction || happy8.Candidate || !strings.Contains(happy8.Warning, "直接七球合同") || strings.Contains(happy8.Warning, "未验收") {
		t.Fatalf("Happy8 Mark Six production identity is stale: %+v", happy8.SourceDiagnosticSource)
	}
	oldMacau, found := sourceDiagnosticSpecForKey("163:70")
	if !found || oldMacau.Relation != sourceRelationProduction || oldMacau.Candidate || oldMacau.Warning != "" {
		t.Fatalf("old Macau Mark Six must be a confirmed production source: %+v", oldMacau.SourceDiagnosticSource)
	}
	for id, gameID := range map[int]string{186: "bingo-mark-six", 187: "bingo-racing-a", 188: "bingo-racing-b", 189: "bingo-ssc-1", 190: "bingo-ssc-2", 191: "bingo-ssc-3", 192: "bingo-ssc-4"} {
		spec, found := sourceDiagnosticSpecForKey(fmt.Sprintf("163:%d", id))
		if !found || !reflect.DeepEqual(spec.GameIDs, []string{gameID}) || spec.staleAfter != 9*time.Hour {
			t.Fatalf("163 derived comparison %d is incomplete: %+v", id, spec)
		}
	}
	bingo163, _ := sourceDiagnosticSpecForKey("163:135")
	if bingo163.Relation != sourceRelationProduction || !diagnosticContainsString(bingo163.GameIDs, "official-tw-bingo") || bingo163.GameRelations["official-tw-bingo"] != sourceRelationCrossCheck {
		t.Fatalf("Taiwan Bingo source relationships incomplete: %+v", bingo163.SourceDiagnosticSource)
	}
	sgComposite, found := sourceDiagnosticSpecForKey("sg-ssc-verified")
	if !found || sgComposite.Provider != "163＋115" || sgComposite.Endpoint != source163Base+source163LatestPath || sgComposite.Relation != sourceRelationProduction {
		t.Fatalf("SG production diagnostic still describes the legacy source: %+v", sgComposite.SourceDiagnosticSource)
	}
	sgMother, found := sourceDiagnosticSpecForKey("163:64")
	if !found || sgMother.Relation != sourceRelationProduction || sgMother.Candidate {
		t.Fatalf("SG 163:64 mother source is not classified as a production component: %+v", sgMother.SourceDiagnosticSource)
	}
	duplicate, _ := sourceDiagnosticSpecForKey("163:69")
	if len(duplicate.Groups) != 2 {
		t.Fatal("69 must retain both directory groups")
	}
	game := lottery.Game{ID: "sg-ssc", Name: "SG时时彩", SourceKind: "external", SourceName: sgSSCVerifiedSourceName, SourceURL: sgSSCVerifiedSourceURL}
	if key := configuredSourceDiagnosticKey(game); key != "sg-ssc-verified" {
		t.Fatalf("wrong SG binding: %q", key)
	}
	game.SourceURL = "https://example.invalid/?token=secret"
	if configuredSourceDiagnosticKey(game) != "" {
		t.Fatal("unknown metadata must not be called a current binding")
	}
	for _, binding := range source163MirrorBindings {
		game := lottery.Game{ID: binding.GameID, SourceKind: "external", SourceName: source163MirrorName, SourceURL: source163MirrorURL}
		if key := configuredSourceDiagnosticKey(game); key != "163:"+strconv.Itoa(binding.UpstreamGameID) {
			t.Fatalf("wrong 163 mother-source binding for %s: %q", binding.GameID, key)
		}
	}
	for _, binding := range source163PC28Bindings {
		game := lottery.Game{ID: binding.GameID, SourceKind: "external", SourceName: source163MirrorName, SourceURL: source163MirrorURL}
		if key := configuredSourceDiagnosticKey(game); key != "163:57" {
			t.Fatalf("wrong 163 Canada 28 mother-source binding for %s: %q", binding.GameID, key)
		}
	}
	for _, id := range []string{"pc-canada", "canada-28", "canada-20", "happy8-mark-six"} {
		if configuredSourceDiagnosticKey(lottery.Game{ID: id, SourceKind: "platform"}) != "" {
			t.Fatal("platform draws must not be mapped to external source")
		}
	}
	for _, game := range officialGames {
		if configuredSourceDiagnosticKey(game) != "official:"+game.ID {
			t.Fatalf("missing official source %s", game.ID)
		}
	}
	result := sourceDiagnosticsForGames([]lottery.Game{{ID: "sg-ssc", SourceKind: "external", SourceName: sgSSCVerifiedSourceName, SourceURL: sgSSCVerifiedSourceURL}, {ID: "official-kl8"}, {ID: "pc-canada"}, {ID: "canada-28"}, {ID: "canada-20"}})
	if result.Games[0].RuleVersion != "digits5-v3" || result.Games[1].RulesMessage == "" || result.Games[2].RuleVersion != "pc28-v1" {
		t.Fatalf("wrong rule summaries %+v", result.Games)
	}
	if result.Games[0].Source163Status != source163StatusCurrent || result.Games[1].Source163Status != source163StatusNotFound || result.Games[2].Source163Status != source163StatusVerifiedCandidate || result.Games[3].Source163Status != source163StatusVerifiedCandidate || result.Games[4].Source163Status != source163StatusVerifiedCandidate {
		t.Fatalf("wrong fixed-directory assessments %+v", result.Games)
	}
	if !strings.Contains(result.Games[4].Source163Message, "三款加拿大28玩法共用开奖") || !strings.Contains(result.Games[4].Source163Message, "210秒") {
		t.Fatalf("Canada shared mother-source evidence is not explicit: %+v", result.Games[4])
	}
	boundPC := sourceDiagnosticsForGames([]lottery.Game{{ID: "canada-20", SourceKind: "external", SourceName: source163MirrorName, SourceURL: source163MirrorURL}}).Games[0]
	if boundPC.SourceKey == nil || *boundPC.SourceKey != "163:57" || boundPC.Source163Status != source163StatusCurrent {
		t.Fatalf("bound Canada 28 variant is not shown as current: %+v", boundPC)
	}
	if result.Games[0].NextDrawAt != nil || result.Games[0].LastSyncAt != nil {
		t.Fatal("unknown time must remain null")
	}
}

func diagnosticContainsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestSourceDiagnosticsListUsesOnlyOneRead(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable"}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	_ = pool.Close()
	queries := 0
	if err := db.Callback().Query().Before("gorm:query").Register("diagnostic:select", func(tx *gorm.DB) {
		queries++
		rows, ok := tx.Statement.Dest.(*[]lottery.Game)
		if !ok {
			tx.AddError(errors.New("unexpected query"))
			return
		}
		*rows = []lottery.Game{{ID: "pc-canada", SourceKind: "platform"}}
	}); err != nil {
		t.Fatal(err)
	}
	deny := func(tx *gorm.DB) {
		t.Error("diagnostics attempted a write")
		tx.AddError(errors.New("write forbidden"))
	}
	_ = db.Callback().Create().Before("gorm:create").Register("diagnostic:create", deny)
	_ = db.Callback().Update().Before("gorm:update").Register("diagnostic:update", deny)
	_ = db.Callback().Delete().Before("gorm:delete").Register("diagnostic:delete", deny)
	result, err := NewLotteryService(db).SourceDiagnostics(context.Background())
	if err != nil || queries != 1 || len(result.Games) != 1 {
		t.Fatalf("not a bounded read: queries=%d result=%+v err=%v", queries, result, err)
	}
}

func TestSourceDiagnostic163SuccessAndHistoryLimit(t *testing.T) {
	calls := 0
	client := sourceDiagnosticTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodGet || r.Header.Get("X-Token") != "" || r.URL.Query().Get("iGameId") != "169" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("sign") == "" || r.URL.Query().Get("sign2") == "" {
			t.Error("missing anonymous signature")
		}
		if r.URL.Path == source163LatestPath {
			sourceDiagnosticEncode(w, map[string]any{"success": true, "result": sourceDiagnosticTestRow(169)})
			return
		}
		if r.URL.Path != source163HistoryPath || r.URL.Query().Get("count") != "3" {
			t.Error("non-allowlisted/unbounded request")
		}
		rows := []any{sourceDiagnosticTestRow(169)}
		for i := 1; i < 3; i++ {
			row := sourceDiagnosticTestRow(169)
			row["sgameperiod"] = fmt.Sprintf("20260903%03d", 288-i)
			row["dopentime"] = time.Date(2026, 9, 4, 0, -5*i, 0, 0, sgSSCLocation).Format("2006-01-02 15:04:05")
			rows = append(rows, row)
		}
		for len(rows) < 500 {
			rows = append(rows, map[string]any{"ignored_outside_local_sample": true})
		}
		sourceDiagnosticEncode(w, map[string]any{"success": true, "result": rows})
	})
	result := sourceDiagnosticTestProbe(context.Background(), "163:169", client)
	if result.Status != "success" || calls != 2 || result.HistoryCount != 3 || result.HTTPStatus == nil || *result.HTTPStatus != 200 || result.Issue == nil || *result.Issue != "20260903288" || !reflect.DeepEqual(result.Numbers, []int{6, 0, 0, 2, 9}) {
		t.Fatalf("unexpected probe %+v calls=%d", result, calls)
	}
	body, _ := json.Marshal(result)
	if strings.Contains(string(body), "sign=") || strings.Contains(string(body), "sign2=") {
		t.Fatal("signature leaked")
	}
}

func TestSourceDiagnostic163RejectsBadEvidence(t *testing.T) {
	for _, test := range []struct {
		name, status string
		mutate       func(map[string]any)
	}{
		{"empty", "empty", func(row map[string]any) { row["sgameperiod"], row["sopennum"], row["dopentime"] = "", "", "" }},
		{"identity", "error", func(row map[string]any) { row["igameid"] = 64 }},
		{"malformed numbers", "error", func(row map[string]any) { row["sopennum"] = "6|0|oops|2|9" }},
		{"range", "error", func(row map[string]any) { row["sopennum"] = "6|0|10|2|9" }},
		{"count", "error", func(row map[string]any) { row["sopennum"] = "6|0|2|9" }},
		{"bad date", "error", func(row map[string]any) { row["dopentime"] = "invalid" }},
		{"future", "error", func(row map[string]any) { row["dopentime"] = "2026-09-05 00:00:00" }},
		{"stale", "stale", func(row map[string]any) { row["dopentime"] = "2024-08-09 17:03:00" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			row := sourceDiagnosticTestRow(169)
			test.mutate(row)
			client := sourceDiagnosticTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				var result any = row
				if r.URL.Path == source163HistoryPath {
					result = []any{row}
				}
				sourceDiagnosticEncode(w, map[string]any{"success": true, "result": result})
			})
			result := sourceDiagnosticTestProbe(context.Background(), "163:169", client)
			if result.Status != test.status {
				t.Fatalf("got %+v", result)
			}
		})
	}
	for _, body := range []string{`{}`, `{"success":false,"errorMsg":"sign=should-not-leak","result":{}}`, `{"success":true,"result":[]} trailing`, `<html>token=secret</html>`} {
		t.Run(body, func(t *testing.T) {
			client := sourceDiagnosticTestClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) })
			result := sourceDiagnosticTestProbe(context.Background(), "163:169", client)
			if result.Status != "error" || strings.Contains(result.Message, "secret") || strings.Contains(result.Message, "should-not-leak") {
				t.Fatalf("unsafe result %+v", result)
			}
		})
	}
}

func TestSourceDiagnostic163SupportsDifferentBallCounts(t *testing.T) {
	for _, test := range []struct {
		id      int
		numbers string
	}{
		{160, "1|2|3|4|5|6|7|8|9|10"},
		{1, "0|0|9"}, {20, "0|1|2|3|4|5|13"}, {136, "1|2|3|4|5|38|1"},
	} {
		t.Run(strconv.Itoa(test.id), func(t *testing.T) {
			row := sourceDiagnosticTestRow(test.id)
			row["sopennum"] = test.numbers
			client := sourceDiagnosticTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				var result any = row
				if r.URL.Path == source163HistoryPath {
					result = []any{row}
				}
				sourceDiagnosticEncode(w, map[string]any{"success": true, "result": result})
			})
			result := sourceDiagnosticTestProbe(context.Background(), fmt.Sprintf("163:%d", test.id), client)
			if result.Status != "success" {
				t.Fatalf("valid multi-ball source rejected: %+v", result)
			}
		})
	}
}

func TestSourceDiagnosticCurrent163MirrorUsesProductionGate(t *testing.T) {
	binding := source163MirrorBindings[0]
	now := time.Date(2026, 9, 4, 12, 0, 5, 0, sgSSCLocation)
	rows := source163MirrorTestRows(binding, now.Add(-5*time.Second))
	probe := func(t *testing.T, history []map[string]any) SourceProbeResult {
		t.Helper()
		client := sourceDiagnosticTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("iGameId") != strconv.Itoa(binding.UpstreamGameID) {
				t.Fatalf("unexpected upstream identity %q", r.URL.Query().Get("iGameId"))
			}
			if r.URL.Path == source163LatestPath {
				_, _ = w.Write(source163MirrorPayload(t, rows[0]))
				return
			}
			if r.URL.Path != source163HistoryPath || r.URL.Query().Get("count") != strconv.Itoa(source163MirrorHistoryLimit) {
				t.Fatalf("diagnostic did not use production history gate: %s?%s", r.URL.Path, r.URL.RawQuery)
			}
			_, _ = w.Write(source163MirrorPayload(t, history))
		})
		return probeSourceDiagnostic(context.Background(), "163:56", func() time.Time { return now }, client, bytes.NewReader(make([]byte, 100)))
	}
	if result := probe(t, rows); result.Status != "success" || result.HistoryCount != len(rows) || !strings.Contains(result.Message, "生产母源") {
		t.Fatalf("production-valid mirror rejected: %+v", result)
	}
	if result := probe(t, rows[:3]); result.Status == "success" || !strings.Contains(result.Message, "历史不足") {
		t.Fatalf("diagnostic accepted a batch production would reject: %+v", result)
	}
}

func TestSourceDiagnosticCanada28UsesExactProductionCadence(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 5, 0, sgSSCLocation)
	latestAt := now.Add(-5 * time.Second)
	rows := make([]map[string]any, 5)
	for index := range rows {
		rows[index] = map[string]any{
			"igameid":     source163PC28UpstreamGameID,
			"sgameperiod": strconv.Itoa(3477599 - index),
			"sopennum":    fmt.Sprintf("%d|%d|%d", index%10, (index+1)%10, (index+2)%10),
			"dopentime":   latestAt.Add(-time.Duration(index*source163PC28Interval) * time.Second).Format(time.DateTime),
		}
	}
	probe := func(history []map[string]any) SourceProbeResult {
		client := sourceDiagnosticTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("iGameId") != strconv.Itoa(source163PC28UpstreamGameID) {
				t.Fatalf("unexpected Canada 28 identity %q", r.URL.Query().Get("iGameId"))
			}
			if r.URL.Path == source163LatestPath {
				_, _ = w.Write(source163MirrorPayload(t, rows[0]))
				return
			}
			if r.URL.Path != source163HistoryPath || r.URL.Query().Get("count") != strconv.Itoa(source163MirrorHistoryLimit) {
				t.Fatalf("diagnostic did not use Canada 28 production history gate: %s?%s", r.URL.Path, r.URL.RawQuery)
			}
			_, _ = w.Write(source163MirrorPayload(t, history))
		})
		return probeSourceDiagnostic(context.Background(), "163:57", func() time.Time { return now }, client, bytes.NewReader(make([]byte, 100)))
	}
	if result := probe(rows); result.Status != "success" || result.HistoryCount != len(rows) || !strings.Contains(result.Message, "生产母源") {
		t.Fatalf("production-valid Canada 28 rejected: %+v", result)
	}
	if result := probe(rows[1:]); result.Status != "success" || result.HistoryCount != len(rows) || result.Issue == nil || *result.Issue != fmt.Sprint(rows[0]["sgameperiod"]) {
		t.Fatalf("safe latest-before-history transition rejected by diagnostics: %+v", result)
	}
	wrongCadence := make([]map[string]any, len(rows))
	for index, row := range rows {
		wrongCadence[index] = make(map[string]any, len(row))
		for key, value := range row {
			wrongCadence[index][key] = value
		}
		wrongCadence[index]["dopentime"] = latestAt.Add(-time.Duration(index*30) * time.Second).Format(time.DateTime)
	}
	if result := probe(wrongCadence); result.Status == "success" || !strings.Contains(result.Message, "210秒") {
		t.Fatalf("diagnostic accepted a cadence production would reject: %+v", result)
	}
}

func TestSourceDiagnosticCurrent163BingoUsesProductionGates(t *testing.T) {
	latestAt := bingo163TestTime(12, 0)
	now := latestAt.Add(time.Minute)
	latestSet := bingo163TestProductRow(bingo163SetUpstreamGameID, "115049938", latestAt, bingo163FixtureSet)
	latestOrdered := bingo163TestProductRow(bingo163OrderedUpstreamGameID, "115049938", latestAt, bingo163FixtureOrder)
	setHistory := []any{
		latestSet,
		bingo163TestRow("115049937", latestAt.Add(-5*time.Minute), bingo163FixtureSet),
		bingo163TestRow("115049936", latestAt.Add(-10*time.Minute), bingo163FixtureSet),
		bingo163TestRow("115049935", latestAt.Add(-15*time.Minute), bingo163FixtureSet),
	}
	orderedHistory := []any{
		latestOrdered,
		bingo163TestProductRow(bingo163OrderedUpstreamGameID, "115049937", latestAt.Add(-5*time.Minute), bingo163FixtureOrder),
		bingo163TestProductRow(bingo163OrderedUpstreamGameID, "115049936", latestAt.Add(-10*time.Minute), bingo163FixtureOrder),
		bingo163TestProductRow(bingo163OrderedUpstreamGameID, "115049935", latestAt.Add(-15*time.Minute), bingo163FixtureOrder),
	}
	client := sourceDiagnosticTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gameID := r.URL.Query().Get("iGameId")
		switch r.URL.Path {
		case source163LatestPath:
			if gameID == strconv.Itoa(bingo163OrderedUpstreamGameID) {
				_, _ = w.Write(bingo163TestPayload(t, latestOrdered))
			} else if gameID == strconv.Itoa(bingo163SetUpstreamGameID) {
				_, _ = w.Write(bingo163TestPayload(t, latestSet))
			} else {
				t.Fatalf("unexpected Bingo identity %q", gameID)
			}
		case source163HistoryPath:
			if r.URL.Query().Get("count") != strconv.Itoa(bingo163HistoryLimit) {
				t.Fatalf("history count=%q", r.URL.Query().Get("count"))
			}
			if gameID == strconv.Itoa(bingo163OrderedUpstreamGameID) {
				_, _ = w.Write(bingo163TestPayload(t, orderedHistory))
			} else if gameID == strconv.Itoa(bingo163SetUpstreamGameID) {
				_, _ = w.Write(bingo163TestPayload(t, setHistory))
			} else {
				t.Fatalf("unexpected Bingo identity %q", gameID)
			}
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	})
	probe := func(key string) SourceProbeResult {
		return probeSourceDiagnostic(context.Background(), key, func() time.Time { return now }, client, bytes.NewReader(make([]byte, 100)))
	}
	if result := probe("163:135"); result.Status != "success" || result.HistoryCount != len(setHistory) || !reflect.DeepEqual(result.Numbers, bingo163FixtureOrder) {
		t.Fatalf("production-valid Bingo pair rejected through set component: %+v", result)
	}
	if result := probe("163:185"); result.Status != "success" || result.HistoryCount != len(orderedHistory) || !reflect.DeepEqual(result.Numbers, bingo163FixtureOrder) {
		t.Fatalf("production-valid Bingo pair rejected through ordered component: %+v", result)
	}
	if result := probe("bingo-ordered-163"); result.Status != "success" || result.HistoryCount != len(setHistory) || !reflect.DeepEqual(result.Numbers, bingo163FixtureOrder) {
		t.Fatalf("production-valid Bingo order rejected: %+v", result)
	}

	// A valid 20-ball mother set can still be unusable by an ordered derivative:
	// Bingo Mark Six needs seven numbers in 1..49. The read-only diagnostic must
	// execute that production conversion gate instead of reporting green here.
	highSet := make([]int, 20)
	highOrder := make([]int, 20)
	for index := range highSet {
		highSet[index] = 61 + index
		highOrder[index] = 80 - index
	}
	highSetLatest := bingo163TestProductRow(bingo163SetUpstreamGameID, "115049938", latestAt, highSet)
	highOrderedLatest := bingo163TestProductRow(bingo163OrderedUpstreamGameID, "115049938", latestAt, highOrder)
	highSetHistory := []any{
		highSetLatest,
		bingo163TestRow("115049937", latestAt.Add(-5*time.Minute), highSet),
		bingo163TestRow("115049936", latestAt.Add(-10*time.Minute), highSet),
		bingo163TestRow("115049935", latestAt.Add(-15*time.Minute), highSet),
	}
	highOrderedHistory := []any{
		highOrderedLatest,
		bingo163TestProductRow(bingo163OrderedUpstreamGameID, "115049937", latestAt.Add(-5*time.Minute), highOrder),
		bingo163TestProductRow(bingo163OrderedUpstreamGameID, "115049936", latestAt.Add(-10*time.Minute), highOrder),
		bingo163TestProductRow(bingo163OrderedUpstreamGameID, "115049935", latestAt.Add(-15*time.Minute), highOrder),
	}
	highClient := sourceDiagnosticTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		orderedSource := r.URL.Query().Get("iGameId") == strconv.Itoa(bingo163OrderedUpstreamGameID)
		switch r.URL.Path {
		case source163LatestPath:
			if orderedSource {
				_, _ = w.Write(bingo163TestPayload(t, highOrderedLatest))
			} else {
				_, _ = w.Write(bingo163TestPayload(t, highSetLatest))
			}
		case source163HistoryPath:
			if orderedSource {
				_, _ = w.Write(bingo163TestPayload(t, highOrderedHistory))
			} else {
				_, _ = w.Write(bingo163TestPayload(t, highSetHistory))
			}
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	})
	for _, key := range []string{"163:135", "163:185", "bingo-ordered-163"} {
		if result := probeSourceDiagnostic(context.Background(), key, func() time.Time { return now }, highClient, bytes.NewReader(make([]byte, 100))); result.Status == "success" || !strings.Contains(result.Message, "bingo-mark-six") {
			t.Fatalf("diagnostic %s accepted data rejected by an ordered production conversion: %+v", key, result)
		}
	}

	tooShortClient := sourceDiagnosticTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		orderedSource := r.URL.Query().Get("iGameId") == strconv.Itoa(bingo163OrderedUpstreamGameID)
		if r.URL.Path == source163LatestPath {
			if orderedSource {
				_, _ = w.Write(bingo163TestPayload(t, latestOrdered))
			} else {
				_, _ = w.Write(bingo163TestPayload(t, latestSet))
			}
			return
		}
		if orderedSource {
			_, _ = w.Write(bingo163TestPayload(t, orderedHistory[:3]))
		} else {
			_, _ = w.Write(bingo163TestPayload(t, setHistory[:3]))
		}
	})
	result := probeSourceDiagnostic(context.Background(), "163:135", func() time.Time { return now }, tooShortClient, bytes.NewReader(make([]byte, 100)))
	if result.Status == "success" || !strings.Contains(result.Message, "历史不足") {
		t.Fatalf("diagnostic accepted a Bingo batch production would reject: %+v", result)
	}
}

func TestSourceDiagnosticPartitionedNumberRanges(t *testing.T) {
	for _, test := range []struct {
		key     string
		numbers []int
		valid   bool
	}{
		{"163:20", []int{1, 2, 3, 4, 5, 6, 14}, true},
		{"163:20", []int{14, 2, 3, 4, 5, 6, 1}, false},
		{"163:136", []int{1, 2, 3, 4, 5, 38, 1}, true},
		{"163:136", []int{1, 2, 3, 4, 5, 38, 9}, false},
		{"163:136", []int{1, 1, 3, 4, 5, 38, 1}, false},
	} {
		spec, _ := sourceDiagnosticSpecForKey(test.key)
		err := validateSourceProbeDraw(sourceDraw{Issue: "123", Numbers: test.numbers, DrawAt: sourceDiagnosticTestNow()}, spec)
		if (err == nil) != test.valid {
			t.Fatalf("key=%s numbers=%v valid=%v err=%v", test.key, test.numbers, test.valid, err)
		}
	}
}

func TestSourceDiagnosticTransportBoundsAndNoRedirect(t *testing.T) {
	t.Run("oversize", func(t *testing.T) {
		client := sourceDiagnosticTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, strings.Repeat("x", sourceProbeBodyLimit+1))
		})
		result := sourceDiagnosticTestProbe(context.Background(), "163:169", client)
		if result.Status != "error" || !strings.Contains(result.Message, "1 MiB") {
			t.Fatalf("body was not bounded %+v", result)
		}
	})
	t.Run("redirect", func(t *testing.T) {
		calls := 0
		client := sourceDiagnosticTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			http.Redirect(w, r, "http://127.0.0.1/internal?token=secret", http.StatusFound)
		})
		result := sourceDiagnosticTestProbe(context.Background(), "163:169", client)
		if calls != 1 || result.Status != "error" || result.HTTPStatus == nil || *result.HTTPStatus != 302 || strings.Contains(result.Message, "token") {
			t.Fatalf("redirect followed or leaked %+v calls=%d", result, calls)
		}
	})
	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		client := sourceDiagnosticTestClient(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
		start := time.Now()
		result := sourceDiagnosticTestProbe(ctx, "163:169", client)
		if result.Status != "error" || time.Since(start) > time.Second {
			t.Fatalf("context not honored %+v", result)
		}
	})
	t.Run("unknown key no network", func(t *testing.T) {
		client := &http.Client{Transport: sourceDiagnosticRoundTrip(func(*http.Request) (*http.Response, error) {
			t.Error("unknown key made network request")
			return nil, errors.New("unexpected")
		})}
		for _, key := range []string{"http://127.0.0.1/", "163:99999", "163:169?url=http://localhost"} {
			result := sourceDiagnosticTestProbe(context.Background(), key, client)
			if result.Status != "error" || result.HTTPStatus != nil {
				t.Fatalf("accepted arbitrary source %+v", result)
			}
		}
	})
	t.Run("signed url error hidden", func(t *testing.T) {
		client := &http.Client{Transport: sourceDiagnosticRoundTrip(func(r *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("raw failure %s", r.URL.String())
		})}
		result := sourceDiagnosticTestProbe(context.Background(), "163:169", client)
		if result.Status != "error" || strings.Contains(result.Message, "sign") || strings.Contains(result.Message, "http") {
			t.Fatalf("signed URL exposed %+v", result)
		}
	})
}

func TestSourceDiagnostic168AndSGAreReadOnlySamples(t *testing.T) {
	t.Run("168", func(t *testing.T) {
		calls := 0
		client := sourceDiagnosticTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			row := map[string]any{"preDrawIssue": "20260903288", "preDrawTime": "2026-09-04 00:00:00", "preDrawCode": "6,0,0,2,9", "lotCode": 10036}
			var data any = row
			if strings.Contains(r.URL.Path, "List") {
				data = []any{row}
			}
			sourceDiagnosticEncode(w, map[string]any{"errorCode": 0, "result": map[string]any{"data": data}})
		})
		result := sourceDiagnosticTestProbe(context.Background(), "168:10036", client)
		if result.Status != "success" || calls != 2 || result.HistoryCount != 1 {
			t.Fatalf("bad 168 sample %+v calls=%d", result, calls)
		}
	})
	for _, test := range []struct {
		name, key string
		mismatch  bool
	}{
		{"composite production", "sg-ssc-verified", false},
		{"ID64 production component", "163:64", false},
		{"verifier mismatch", "sg-ssc-verified", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newSGSSCTestFixture("20260903030")
			if test.mismatch {
				fixture.latest[sgSSCEndpoint(sgSSCValidationStation(), "")]["preDrawCode"] = "1,1,1,1,1"
			}
			client := sourceDiagnosticTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				base := source163Base
				if r.URL.Path == sgSSCLatestPath || r.URL.Path == sgSSCHistoryPath {
					base = sgSSCValidationStation().base
				}
				requestContext, cancel := context.WithTimeout(r.Context(), sgSSCTotalTimeout)
				defer cancel()
				body, err := fixture.request(requestContext, base+r.URL.RequestURI())
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadGateway)
					return
				}
				_, _ = w.Write(body)
			})
			result := probeSourceDiagnostic(context.Background(), test.key, func() time.Time { return fixture.now }, client, bytes.NewReader(make([]byte, 100)))
			expected := "success"
			if test.mismatch {
				expected = "error"
			}
			if result.Status != expected || !test.mismatch && (result.HistoryCount != sgSSCWindowSize || !strings.Contains(result.Message, "最近24期")) {
				t.Fatalf("SG production diagnostic %+v", result)
			}
		})
	}
}

func TestSourceDiagnosticBingoOrderAndMissingOfficialTime(t *testing.T) {
	for _, wrongTail := range []bool{false, true} {
		t.Run(fmt.Sprintf("bingo tail %v", wrongTail), func(t *testing.T) {
			numbers := make([]int, 20)
			parts := make([]string, 21)
			for i := range numbers {
				numbers[i] = i + 1
				parts[i] = strconv.Itoa(i + 1)
			}
			parts[20] = "20"
			if wrongTail {
				parts[20] = "19"
			}
			client := sourceDiagnosticTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/history" {
					sourceDiagnosticEncode(w, []any{map[string]any{"period": "115049938", "drawTime": "2026-09-03T23:55:00+08:00", "numbers": numbers, "superNumber": 20}})
					return
				}
				row := map[string]any{"preDrawIssue": "115049938", "preDrawTime": "2026-09-03 23:55:00", "preDrawCode": strings.Join(parts, ",")}
				var data any = row
				if strings.Contains(r.URL.Path, "List") {
					data = []any{row}
				}
				sourceDiagnosticEncode(w, map[string]any{"errorCode": 0, "result": map[string]any{"data": data}})
			})
			result := sourceDiagnosticTestProbe(context.Background(), "bingo-ordered", client)
			expected := "success"
			if wrongTail {
				expected = "error"
			}
			if result.Status != expected {
				t.Fatalf("bingo order not checked %+v", result)
			}
		})
	}
	client := sourceDiagnosticTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		numbers := []string{}
		for i := 1; i <= 20; i++ {
			numbers = append(numbers, fmt.Sprint(i))
		}
		sourceDiagnosticEncode(w, map[string]any{"rtCode": 0, "content": map[string]any{"bingoQueryResult": []any{map[string]any{"drawTerm": 115049938, "openShowOrder": numbers}}}})
	})
	result := sourceDiagnosticTestProbe(context.Background(), "official:official-tw-bingo", client)
	if result.Status != "error" || result.DrawAt != nil || len(result.Numbers) != 20 || !strings.Contains(result.Message, "不会用当前时间替代") {
		t.Fatalf("missing source timestamp synthesized %+v", result)
	}
}

func TestSourceDiagnosticSigningChangesWithPathTimeAndEntropy(t *testing.T) {
	first, err := source163SignedURL(source163LatestPath, 169, 0, sourceDiagnosticTestNow(), bytes.NewReader(make([]byte, 10)))
	if err != nil {
		t.Fatal(err)
	}
	second, err := source163SignedURL(source163LatestPath, 64, 0, sourceDiagnosticTestNow(), bytes.NewReader(make([]byte, 10)))
	if err != nil {
		t.Fatal(err)
	}
	firstURL, _ := url.Parse(first)
	secondURL, _ := url.Parse(second)
	if firstURL.Query().Get("sign") != secondURL.Query().Get("sign") || firstURL.Query().Get("sign2") != secondURL.Query().Get("sign2") {
		t.Fatal("query must not enter anonymous signature")
	}
	third, _ := source163SignedURL(source163HistoryPath, 169, 3, sourceDiagnosticTestNow(), bytes.NewReader(make([]byte, 10)))
	thirdURL, _ := url.Parse(third)
	if firstURL.Query().Get("sign") == thirdURL.Query().Get("sign") {
		t.Fatal("path must enter signature")
	}
	if _, err := source163SignedURL("/api/homePage/clickNumber", 169, 0, sourceDiagnosticTestNow(), bytes.NewReader(make([]byte, 10))); err == nil {
		t.Fatal("write-like upstream path was accepted")
	}
	if _, err := source163SignedURL(source163LatestPath, 169, 0, sourceDiagnosticTestNow(), bytes.NewReader(nil)); err == nil {
		t.Fatal("entropy failure must stop request")
	}
}
