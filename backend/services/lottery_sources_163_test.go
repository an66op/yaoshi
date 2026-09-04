package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"backend/data/models/lottery"
	apperrors "backend/errors"

	"gorm.io/gorm"
)

func Test163MirrorProductionBindingsAreFixedAndExcludeSGMigration(t *testing.T) {
	want := []struct {
		gameID string
		id     int
		count  int
	}{
		{"speed-racing", 56, 10}, {"speed-fly", 61, 10}, {"sg-fly", 58, 10}, {"fly-racing", 38, 10},
		{"au-lucky-10", 33, 10}, {"speed-ssc", 55, 5}, {"au-lucky-5", 31, 5},
	}
	if len(source163MirrorBindings) != len(want) {
		t.Fatalf("bindings=%d want=%d", len(source163MirrorBindings), len(want))
	}
	for index, expected := range want {
		got := source163MirrorBindings[index]
		if got.GameID != expected.gameID || got.UpstreamGameID != expected.id || got.Count != expected.count || got.Revision == "" {
			t.Fatalf("binding[%d]=%+v want=%+v", index, got, expected)
		}
		if strings.Contains(got.Revision, "168-115") {
			t.Fatalf("163 draw uses legacy SG revision: %q", got.Revision)
		}
	}
	if _, found := source163MirrorBindingForGame("sg-ssc"); found {
		t.Fatal("SG时时彩 must remain on its existing verified two-station writer until a versioned three-station migration exists")
	}
}

func Test163MirrorFreshnessLimitKeepsLuckyAirshipScheduledBreak(t *testing.T) {
	for _, test := range []struct {
		gameID string
		want   time.Duration
	}{
		{gameID: "fly-racing", want: source163FlyRacingMaxAge},
		{gameID: "speed-racing", want: source163MirrorMaxAge},
	} {
		binding, found := source163MirrorBindingForGame(test.gameID)
		if !found {
			t.Fatalf("missing binding for %s", test.gameID)
		}
		if got := source163MirrorFreshnessLimit(binding); got != test.want {
			t.Fatalf("%s freshness limit=%s want=%s", test.gameID, got, test.want)
		}
	}
	if got := source163MirrorFreshnessLimit(source163MirrorBinding{GameID: "fly-racing", UpstreamGameID: 164}); got != source163MirrorMaxAge {
		t.Fatalf("same-named ID164 must not inherit the verified ID38 break: %s", got)
	}
}

func Test163MirrorSourceBindingIsExact(t *testing.T) {
	binding := source163MirrorBindings[0]
	valid := lottery.Game{ID: binding.GameID, SourceKind: "external", SourceName: source163MirrorName, SourceURL: source163MirrorURL}
	if !source163MirrorBound(&valid, binding) {
		t.Fatal("expected exact binding")
	}
	for _, mutate := range []func(*lottery.Game){
		func(game *lottery.Game) { game.ID = "other" },
		func(game *lottery.Game) { game.SourceKind = "platform" },
		func(game *lottery.Game) { game.SourceName += " changed" },
		func(game *lottery.Game) { game.SourceURL += "changed" },
	} {
		changed := valid
		mutate(&changed)
		if source163MirrorBound(&changed, binding) {
			t.Fatalf("accepted changed binding: %+v", changed)
		}
	}
}

func Test163MirrorBindingMigrationIsExactAndFailClosed(t *testing.T) {
	binding := source163MirrorBindings[0]
	legacy := lottery.Game{ID: binding.GameID, SourceKind: "external", SourceName: legacy168HighFreqName, SourceURL: legacy168HighFreqURL, SyncStatus: "ok"}
	updates, required := source163MirrorBindingUpdates(legacy, binding)
	if !required || updates["source_name"] != source163MirrorName || updates["source_url"] != source163MirrorURL ||
		updates["sync_status"] != "stale" || updates["last_sync_error"] != source163MirrorPendingMessage || updates["last_sync_at"] != nil {
		t.Fatalf("legacy cutover=%#v required=%v", updates, required)
	}

	current := lottery.Game{ID: binding.GameID, SourceKind: "external", SourceName: source163MirrorName, SourceURL: source163MirrorURL, SyncStatus: "ok"}
	if updates, required := source163MirrorBindingUpdates(current, binding); required || updates != nil {
		t.Fatalf("healthy current binding was reset: %#v/%v", updates, required)
	}
	current.SyncStatus = "syncing"
	if updates, required := source163MirrorBindingUpdates(current, binding); !required || updates["sync_status"] != "stale" {
		t.Fatalf("interrupted current binding was not failed closed: %#v/%v", updates, required)
	}

	custom := legacy
	custom.SourceName, custom.SourceURL = "商户自定义源", "https://operator.example/"
	if updates, required := source163MirrorBindingUpdates(custom, binding); required || updates != nil {
		t.Fatalf("custom operator binding was overwritten: %#v/%v", updates, required)
	}
}

func source163MirrorPayload(t *testing.T, result any) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"success": true, "result": result})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func source163MirrorTestRows(binding source163MirrorBinding, now time.Time) []map[string]any {
	rows := make([]map[string]any, 5)
	for index := range rows {
		numbers := make([]string, binding.Count)
		for numberIndex := range numbers {
			if binding.Unique {
				numbers[numberIndex] = string(rune('1' + numberIndex))
				if numberIndex == 9 {
					numbers[numberIndex] = "10"
				}
			} else {
				numbers[numberIndex] = string(rune('0' + (numberIndex+index)%10))
			}
		}
		rows[index] = map[string]any{
			"igameid": binding.UpstreamGameID, "sgameperiod": 20260904120 - index,
			"sopennum": strings.Join(numbers, "|"), "dopentime": now.Add(-time.Duration(index) * 30 * time.Second).Format("2006-01-02 15:04:05"),
		}
	}
	return rows
}

func TestFetch163MirrorUsesFreshSignedRequestsAndVerifiesFiniteHistory(t *testing.T) {
	binding := source163MirrorBindings[0]
	now := time.Date(2026, 9, 4, 12, 0, 5, 0, sgSSCLocation)
	rows := source163MirrorTestRows(binding, now.Add(-5*time.Second))
	nextAt := now.Add(4*time.Minute + 55*time.Second)
	rows[0]["nextGamePeriod"], rows[0]["realNextGamePeriod"] = "20260904121", "20260904121"
	rows[0]["nextPeriodOpenTime"] = nextAt.UnixMilli()
	requests := make([]string, 0, 2)
	request := func(_ context.Context, endpoint string) ([]byte, error) {
		requests = append(requests, endpoint)
		parsed, err := url.Parse(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Query().Get("iGameId") != "56" || parsed.Query().Get("sign") == "" || parsed.Query().Get("sign2") == "" {
			t.Fatalf("missing fixed identity/signatures: %s", parsed.RawQuery)
		}
		if len(requests) == 1 {
			if parsed.Query().Get("count") != "" {
				t.Fatal("latest request unexpectedly has history count")
			}
			return source163MirrorPayload(t, rows[0]), nil
		}
		if parsed.Query().Get("count") != "32" {
			t.Fatalf("history count=%q", parsed.Query().Get("count"))
		}
		return source163MirrorPayload(t, rows), nil
	}
	draws, err := fetch163MirrorDrawsWithRequest(context.Background(), binding, func() time.Time { return now }, bytes.NewReader(bytes.Repeat([]byte{7, 3, 9, 2, 8}, 8)), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || requests[0] == requests[1] {
		t.Fatalf("requests=%v", requests)
	}
	if len(draws) != 5 || draws[0].SourceRevision != binding.Revision || draws[0].ConversionRevision != source163MirrorConversionVersion ||
		draws[0].NextIssue != "20260904121" || !draws[0].NextDrawAt.Equal(nextAt.UTC()) {
		t.Fatalf("draws=%+v", draws)
	}
	if interval := observedDrawInterval(draws); interval != 30 {
		t.Fatalf("interval=%d", interval)
	}
	game := lottery.Game{ID: binding.GameID, Name: "极速赛车"}
	if err := validate163MirrorDrawBatch(game, binding, draws); err != nil {
		t.Fatal(err)
	}
}

func TestFetch163MirrorAcceptsLuckyAirshipDuringScheduledBreakOnly(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 5, 0, sgSSCLocation)
	for _, test := range []struct {
		gameID    string
		wantError bool
	}{
		{gameID: "fly-racing"},
		{gameID: "speed-racing", wantError: true},
	} {
		t.Run(test.gameID, func(t *testing.T) {
			binding, found := source163MirrorBindingForGame(test.gameID)
			if !found {
				t.Fatal("missing binding")
			}
			rows := source163MirrorTestRows(binding, now.Add(-9*time.Hour))
			call := 0
			request := func(context.Context, string) ([]byte, error) {
				call++
				if call == 1 {
					return source163MirrorPayload(t, rows[0]), nil
				}
				return source163MirrorPayload(t, rows), nil
			}
			_, err := fetch163MirrorDrawsWithRequest(context.Background(), binding, func() time.Time { return now }, bytes.NewReader(make([]byte, 40)), request)
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), "已过期") {
					t.Fatalf("err=%v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func source163MirrorScheduleDraws(latestAt time.Time, latestSequence int) []sourceDraw {
	result := make([]sourceDraw, 5)
	for index := range result {
		result[index] = sourceDraw{
			Issue:  fmt.Sprintf("20260904%03d", latestSequence-index),
			DrawAt: latestAt.Add(-time.Duration(index) * 5 * time.Minute),
		}
	}
	return result
}

func Test163MirrorLuckyAirshipScheduledBreakClearsUnconfirmedSchedule(t *testing.T) {
	binding, found := source163MirrorBindingForGame("fly-racing")
	if !found {
		t.Fatal("missing fly-racing binding")
	}
	latestAt := time.Date(2026, 9, 5, 4, 4, 0, 0, sgSSCLocation).UTC()
	draws := source163MirrorScheduleDraws(latestAt, 180)
	game := lottery.Game{ID: binding.GameID, DrawInterval: 300}

	stopped, awaiting, err := source163MirrorSchedule(binding, game, draws)
	if err != nil || !awaiting {
		t.Fatalf("scheduled break=%+v awaiting=%v err=%v", stopped, awaiting, err)
	}
	updates := source163MirrorSuccessUpdates(stopped, awaiting, latestAt.Add(9*time.Hour))
	if updates["sync_status"] != "ok" || updates["last_sync_error"] != "" || updates["next_issue"] != "" || updates["next_draw_at"] != nil || updates["timing_source"] != "pending" {
		t.Fatalf("scheduled break updates=%#v", updates)
	}
	closed := lottery.Game{
		ID: binding.GameID, SourceKind: "external", SourceName: source163MirrorName, SourceURL: source163MirrorURL,
		SyncStatus: "ok", TimingSource: "pending",
	}
	if !sourceHealthyForGame(&closed) {
		t.Fatal("verified scheduled break was reported as a source failure")
	}

	activeAt := time.Date(2026, 9, 5, 0, 49, 0, 0, sgSSCLocation).UTC()
	activeWithNext := source163MirrorScheduleDraws(activeAt, 141)
	activeWithNext[0].NextIssue, activeWithNext[0].NextDrawAt = "20260904142", activeAt.Add(5*time.Minute)
	activeSchedule, awaiting, err := source163MirrorSchedule(binding, game, activeWithNext)
	if err != nil || awaiting || activeSchedule.Source != "upstream" || activeSchedule.Issue != "20260904142" || !activeSchedule.DrawAt.Equal(activeAt.Add(5*time.Minute)) {
		t.Fatalf("normal explicit schedule=%+v awaiting=%v err=%v", activeSchedule, awaiting, err)
	}

	activeWithoutNext := source163MirrorScheduleDraws(activeAt, 141)
	if _, awaiting, err = source163MirrorSchedule(binding, game, activeWithoutNext); err == nil || awaiting || !strings.Contains(err.Error(), "无法确定") {
		t.Fatalf("active missing metadata was mistaken for scheduled break: awaiting=%v err=%v", awaiting, err)
	}
}

func Test163MirrorLuckyAirshipAwaitingScheduleDoesNotStickBehindRegressionGuard(t *testing.T) {
	binding, found := source163MirrorBindingForGame("fly-racing")
	if !found {
		t.Fatal("missing fly-racing binding")
	}
	latestAt := time.Date(2026, 9, 5, 4, 4, 0, 0, sgSSCLocation).UTC()
	draws := source163MirrorScheduleDraws(latestAt, 180)
	schedule, awaiting, err := source163MirrorSchedule(binding, lottery.Game{ID: binding.GameID, DrawInterval: 300}, draws)
	if err != nil || !awaiting || !schedule.DrawAt.Equal(latestAt.Add(5*time.Minute)) {
		t.Fatalf("schedule=%+v awaiting=%v err=%v", schedule, awaiting, err)
	}
	for _, previous := range []lottery.Game{
		{NextIssue: "20260904180", NextDrawAt: latestAt, TimingSource: "upstream", SyncStatus: "ok"},
		{NextIssue: "20260904181", NextDrawAt: schedule.DrawAt, TimingSource: "observed", SyncStatus: "error"},
		{NextIssue: "", NextDrawAt: time.Time{}, TimingSource: "pending", SyncStatus: "ok"},
	} {
		if officialScheduleRegresses(previous, schedule) {
			t.Fatalf("normal final-period close was blocked as a regression: previous=%+v schedule=%+v", previous, schedule)
		}
	}
	newerConfirmed := lottery.Game{NextIssue: "20260905001", NextDrawAt: latestAt.Add(9 * time.Hour), TimingSource: "upstream", SyncStatus: "ok"}
	if !officialScheduleRegresses(newerConfirmed, schedule) {
		t.Fatal("a genuinely newer confirmed boundary was not protected from a stale close response")
	}
	updates := source163MirrorSuccessUpdates(schedule, awaiting, latestAt.Add(time.Minute))
	if updates["sync_status"] != "ok" || updates["last_sync_error"] != "" || updates["next_draw_at"] != nil || updates["next_issue"] != "" {
		t.Fatalf("awaiting success can retain old error/schedule: %#v", updates)
	}
}

func Test163MirrorLuckyAirshipDailyBoundaryNeverInventsNextIssue(t *testing.T) {
	binding, found := source163MirrorBindingForGame("fly-racing")
	if !found {
		t.Fatal("missing fly-racing binding")
	}
	latestAt := time.Date(2026, 9, 5, 4, 4, 0, 0, sgSSCLocation).UTC()
	draws := source163MirrorScheduleDraws(latestAt, 180)
	game := lottery.Game{ID: binding.GameID, DrawInterval: 300}
	schedule, awaiting, err := source163MirrorSchedule(binding, game, draws)
	if err != nil || !awaiting || schedule.Issue != "" {
		t.Fatalf("daily boundary schedule=%+v awaiting=%v err=%v", schedule, awaiting, err)
	}

	notID38 := binding
	notID38.UpstreamGameID = 164
	if _, _, err := source163MirrorSchedule(notID38, game, draws); err == nil || !strings.Contains(err.Error(), "无法确定") {
		t.Fatalf("same-named unverified source inherited break policy: %v", err)
	}
}

func Test163MirrorLuckyAirshipUsesConfirmedPostBreakBoundary(t *testing.T) {
	binding, found := source163MirrorBindingForGame("fly-racing")
	if !found {
		t.Fatal("missing fly-racing binding")
	}
	latestAt := time.Date(2026, 9, 5, 4, 4, 0, 0, sgSSCLocation).UTC()
	draws := source163MirrorScheduleDraws(latestAt, 180)
	draws[0].NextIssue = "20260905001"
	draws[0].NextDrawAt = latestAt.Add(9 * time.Hour)
	schedule, awaiting, err := source163MirrorSchedule(binding, lottery.Game{ID: binding.GameID, DrawInterval: 300}, draws)
	if err != nil || awaiting || schedule.Source != "upstream" || schedule.Issue != draws[0].NextIssue || !schedule.DrawAt.Equal(draws[0].NextDrawAt) {
		t.Fatalf("confirmed schedule=%+v awaiting=%v err=%v", schedule, awaiting, err)
	}
}

func Test163MirrorRejectsPartialNextBoundaryInsteadOfTreatingItAsBreak(t *testing.T) {
	binding, found := source163MirrorBindingForGame("fly-racing")
	if !found {
		t.Fatal("missing fly-racing binding")
	}
	latestAt := time.Date(2026, 9, 5, 4, 4, 0, 0, sgSSCLocation).UTC()
	draws := source163MirrorScheduleDraws(latestAt, 180)
	draws[0].NextIssue = "20260905001"
	if _, _, err := source163MirrorSchedule(binding, lottery.Game{ID: binding.GameID, DrawInterval: 300}, draws); err == nil || !strings.Contains(err.Error(), "不完整") {
		t.Fatalf("partial boundary err=%v", err)
	}
}

func Test163MirrorAwaitingScheduleDoesNotMaterializeOrOpenIssue(t *testing.T) {
	binding, found := source163MirrorBindingForGame("fly-racing")
	if !found {
		t.Fatal("missing fly-racing binding")
	}
	db := robotDryRunDB(t)
	writes := 0
	if err := db.Callback().Create().Before("gorm:create").Register("test:163_break_no_create", func(*gorm.DB) { writes++ }); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("test:163_break_no_update", func(*gorm.DB) { writes++ }); err != nil {
		t.Fatal(err)
	}
	game := lottery.Game{
		ID: binding.GameID, Enabled: true, SourceKind: "external", SourceName: source163MirrorName, SourceURL: source163MirrorURL,
		SyncStatus: "ok", TimingSource: "pending",
	}
	issue, err := NewBetAdminService(db).EnsureCurrentIssue(&game)
	if err != nil || issue == nil || issue.Issue != "" || issue.Status != lottery.IssueStatusAwaiting || issue.ScheduledDrawAt != nil {
		t.Fatalf("issue=%+v err=%v", issue, err)
	}
	if writes != 0 {
		t.Fatalf("awaiting schedule performed %d lifecycle writes", writes)
	}
	if err := NewBetAdminService(db).ensureIssueOpen(&game, "20260904180"); apperrors.GetErrorCode(err) != "ISSUE_MISMATCH" {
		t.Fatalf("old period remained bettable during scheduled break: %v", err)
	}
}

func TestDecode163MirrorNextScheduleRequiresCoherentPair(t *testing.T) {
	binding := source163MirrorBindings[0]
	drawAt := time.Date(2026, 9, 4, 12, 0, 0, 0, sgSSCLocation)
	base := source163MirrorTestRows(binding, drawAt)[0]
	base["nextGamePeriod"], base["realNextGamePeriod"] = "20260904121", "20260904121"
	base["nextPeriodOpenTime"] = drawAt.Add(5 * time.Minute).UnixMilli()
	decoded, err := decode163MirrorRows(source163MirrorPayload(t, base), false, binding)
	if err != nil || len(decoded) != 1 || decoded[0].NextIssue != "20260904121" || !decoded[0].NextDrawAt.Equal(drawAt.Add(5*time.Minute).UTC()) {
		t.Fatalf("decoded=%+v err=%v", decoded, err)
	}

	for _, test := range []struct {
		name string
		edit func(map[string]any)
		want string
	}{
		{name: "conflicting issues", edit: func(row map[string]any) { row["realNextGamePeriod"] = "20260904122" }, want: "不一致"},
		{name: "missing time", edit: func(row map[string]any) { row["nextPeriodOpenTime"] = 0 }, want: "时间缺失"},
		{name: "missing issue", edit: func(row map[string]any) { row["nextGamePeriod"], row["realNextGamePeriod"] = 0, 0 }, want: "期号缺失"},
		{name: "old boundary", edit: func(row map[string]any) { row["nextPeriodOpenTime"] = drawAt.Add(-time.Minute).UnixMilli() }, want: "边界无效"},
	} {
		t.Run(test.name, func(t *testing.T) {
			row := make(map[string]any, len(base))
			for key, value := range base {
				row[key] = value
			}
			test.edit(row)
			if _, err := decode163MirrorRows(source163MirrorPayload(t, row), false, binding); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err=%v want=%q", err, test.want)
			}
		})
	}

	base["nextGamePeriod"], base["realNextGamePeriod"], base["nextPeriodOpenTime"] = 0, 0, 0
	decoded, err = decode163MirrorRows(source163MirrorPayload(t, base), false, binding)
	if err != nil || len(decoded) != 1 || decoded[0].NextIssue != "" || !decoded[0].NextDrawAt.IsZero() {
		t.Fatalf("zero break sentinel decoded=%+v err=%v", decoded, err)
	}
}

func TestFetch163MirrorFailsClosedOnIdentityHistoryAndFreshness(t *testing.T) {
	binding := source163MirrorBindings[5]
	now := time.Date(2026, 9, 4, 12, 0, 5, 0, sgSSCLocation)
	base := source163MirrorTestRows(binding, now.Add(-5*time.Second))
	for _, test := range []struct {
		name string
		edit func([]map[string]any)
		want string
	}{
		{"wrong identity", func(rows []map[string]any) { rows[0]["igameid"] = 169 }, "彩种身份"},
		{"current history mismatch", func(rows []map[string]any) { rows[0]["sopennum"] = "9|8|7|6|5" }, "号码或时间不一致"},
		{"duplicate issue", func(rows []map[string]any) { rows[2]["sgameperiod"] = rows[1]["sgameperiod"] }, "重复"},
		{"history ahead of latest", func(rows []map[string]any) { rows[1]["dopentime"] = now.Add(time.Minute).Format("2006-01-02 15:04:05") }, "晚于当前接口"},
		{"too little history", func(rows []map[string]any) { rows[3] = nil; rows[4] = nil }, "有限历史不足"},
		{"stale latest", func(rows []map[string]any) {
			for index := range rows {
				if rows[index] != nil {
					rows[index]["dopentime"] = now.Add(-time.Hour - time.Duration(index)*30*time.Second).Format("2006-01-02 15:04:05")
				}
			}
		}, "已过期"},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := make([]map[string]any, len(base))
			for index := range base {
				rows[index] = map[string]any{}
				for key, value := range base[index] {
					rows[index][key] = value
				}
			}
			test.edit(rows)
			history := make([]map[string]any, 0, len(rows))
			for _, row := range rows {
				if row != nil {
					history = append(history, row)
				}
			}
			call := 0
			request := func(context.Context, string) ([]byte, error) {
				call++
				if call == 1 {
					if test.name == "current history mismatch" {
						return source163MirrorPayload(t, base[0]), nil
					}
					return source163MirrorPayload(t, rows[0]), nil
				}
				return source163MirrorPayload(t, history), nil
			}
			_, err := fetch163MirrorDrawsWithRequest(context.Background(), binding, func() time.Time { return now }, bytes.NewReader(make([]byte, 40)), request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err=%v want %q", err, test.want)
			}
		})
	}
}

func TestRequest163MirrorCapsBodyAndDoesNotFollowRedirects(t *testing.T) {
	large := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write(bytes.Repeat([]byte{'x'}, source163MirrorBodyLimit+1))
	}))
	defer large.Close()
	if _, err := request163Mirror(context.Background(), large.URL); err == nil || !strings.Contains(err.Error(), "1 MiB") {
		t.Fatalf("large body err=%v", err)
	}
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetCalls++ }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, target.URL, http.StatusFound)
	}))
	defer redirect.Close()
	if _, err := request163Mirror(context.Background(), redirect.URL); err == nil || !strings.Contains(err.Error(), "HTTP 302") {
		t.Fatalf("redirect err=%v", err)
	}
	if targetCalls != 0 {
		t.Fatalf("followed redirect %d times", targetCalls)
	}
}

func TestDecode163MirrorRejectsNumbersOutsideContract(t *testing.T) {
	binding := source163MirrorBindings[0]
	row := source163MirrorTestRows(binding, time.Date(2026, 9, 4, 12, 0, 0, 0, sgSSCLocation))[0]
	row["sopennum"] = "1|2|3|4|5|6|7|8|9|9"
	if _, err := decode163MirrorRows(source163MirrorPayload(t, row), false, binding); err == nil || !strings.Contains(err.Error(), "重复") {
		t.Fatalf("duplicate err=%v", err)
	}
	row["sopennum"] = "1|2|3|4|5|6|7|8|9|11"
	if _, err := decode163MirrorRows(source163MirrorPayload(t, row), false, binding); err == nil || !strings.Contains(err.Error(), "越界") {
		t.Fatalf("range err=%v", err)
	}
}

func Test163MirrorMergeKeepsCurrentIdentityIdempotently(t *testing.T) {
	binding := source163MirrorBindings[0]
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	draw := sourceDraw{Issue: "20260904120", Numbers: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, DrawAt: now, SourceRevision: binding.Revision, ConversionRevision: source163MirrorConversionVersion}
	got := mergeSourceDraws([]sourceDraw{draw}, []sourceDraw{draw})
	if !reflect.DeepEqual(got, []sourceDraw{draw}) {
		t.Fatalf("merge=%+v", got)
	}
}

func TestExisting163MirrorHistoryKeepsOldTimeAndProvenanceWhenNumbersMatch(t *testing.T) {
	binding := source163MirrorBindings[4]
	existing := lottery.Draw{
		Issue: "51347501", Numbers: "1,2,3,4,5,6,7,8,9,10", DrawAt: time.Date(2026, 9, 4, 4, 0, 0, 0, time.UTC),
		SourceRevision: "168-original-v1", ConversionRevision: "",
	}
	incoming := sourceDraw{
		Issue: existing.Issue, Numbers: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, DrawAt: existing.DrawAt.Add(20 * time.Second),
		SourceRevision: binding.Revision, ConversionRevision: source163MirrorConversionVersion,
	}
	before := existing
	if !existing163MirrorDrawCompatible(existing, incoming) {
		t.Fatal("same ordered result with provider timestamp offset must be compatible")
	}
	if !reflect.DeepEqual(existing, before) {
		t.Fatal("compatibility check changed persisted time or provenance")
	}
	incoming.Numbers[9] = 1
	if existing163MirrorDrawCompatible(existing, incoming) {
		t.Fatal("different ordered numbers must stop the source transition")
	}
}

func TestRequest163MirrorHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("canceled request reached server") }))
	defer server.Close()
	_, err := request163Mirror(ctx, server.URL)
	if err == nil {
		t.Fatal("expected canceled request error")
	}
}
