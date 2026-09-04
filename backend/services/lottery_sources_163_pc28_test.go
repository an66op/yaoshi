package services

import (
	"bytes"
	"context"
	"net/url"
	"strconv"
	"testing"
	"time"

	"backend/data/models/lottery"
)

func Test163PC28BindingsShareOneExactThreeBallDraw(t *testing.T) {
	want := []string{"pc-canada", "canada-28", "canada-20"}
	if len(source163PC28Bindings) != len(want) {
		t.Fatalf("bindings=%d want=%d", len(source163PC28Bindings), len(want))
	}
	for index, gameID := range want {
		binding := source163PC28Bindings[index]
		if binding.GameID != gameID || binding.UpstreamGameID != 57 || binding.Count != 3 || binding.Min != 0 || binding.Max != 9 || binding.Unique || binding.Revision != source163PC28Revision {
			t.Fatalf("binding[%d]=%+v", index, binding)
		}
		if found, ok := source163PC28BindingForGame(gameID); !ok || found != binding {
			t.Fatalf("lookup %s=%+v/%v", gameID, found, ok)
		}
	}
	if _, ok := source163PC28BindingForGame("speed-racing"); ok {
		t.Fatal("accepted unrelated game")
	}
}

func Test163PC28CadenceMustBeExactly210Seconds(t *testing.T) {
	base := time.Date(2026, time.September, 4, 1, 30, 0, 0, time.UTC)
	draws := make([]sourceDraw, source163MirrorHistoryLimit+1)
	for index := range draws {
		draws[index] = sourceDraw{
			Issue:  strconv.Itoa(3477577 + index),
			DrawAt: base.Add(time.Duration(index) * source163PC28Interval * time.Second),
		}
	}
	if err := validate163PC28Cadence(draws); err != nil {
		t.Fatalf("valid cadence rejected: %v", err)
	}

	for edge := 0; edge < len(draws)-1; edge++ {
		t.Run("issue_gap_at_edge_"+strconv.Itoa(edge), func(t *testing.T) {
			changed := append([]sourceDraw(nil), draws...)
			for index := edge + 1; index < len(changed); index++ {
				issue, err := strconv.Atoi(changed[index].Issue)
				if err != nil {
					t.Fatal(err)
				}
				changed[index].Issue = strconv.Itoa(issue + 1)
			}
			if err := validate163PC28Cadence(changed); err == nil {
				t.Fatalf("issue gap at adjacent edge %d accepted", edge)
			}
		})
		t.Run("wrong_time_at_edge_"+strconv.Itoa(edge), func(t *testing.T) {
			changed := append([]sourceDraw(nil), draws...)
			for index := edge + 1; index < len(changed); index++ {
				changed[index].DrawAt = changed[index].DrawAt.Add(time.Second)
			}
			if err := validate163PC28Cadence(changed); err == nil {
				t.Fatalf("211-second interval at adjacent edge %d accepted", edge)
			}
		})
	}
}

func Test163PC28RecoversAfterFourPeriodChainBeyondProviderOutage(t *testing.T) {
	latestAt := time.Date(2026, time.September, 4, 19, 43, 30, 0, sgSSCLocation).UTC()
	draw := func(issue int, drawAt time.Time) sourceDraw {
		return sourceDraw{
			Issue: strconv.Itoa(issue), Numbers: []int{3, 8, 9}, DrawAt: drawAt,
			SourceRevision: source163PC28Revision, ConversionRevision: source163MirrorConversionVersion,
		}
	}
	latest := draw(3477904, latestAt)
	history := []sourceDraw{
		latest,
		draw(3477903, latestAt.Add(-210*time.Second)),
		draw(3477902, latestAt.Add(-420*time.Second)),
		draw(3477901, latestAt.Add(-630*time.Second)),
		// The provider stopped after 3477900, then resumed with only the next
		// issue number. This older row must define a restart boundary instead
		// of poisoning the recovered current run for the whole 25-row window.
		draw(3477900, time.Date(2026, time.September, 4, 18, 56, 30, 0, sgSSCLocation).UTC()),
	}
	draws, err := verified163PC28LatestAndHistory(latest, history)
	if err != nil {
		t.Fatal(err)
	}
	if len(draws) != 4 || draws[0].Issue != latest.Issue || draws[len(draws)-1].Issue != "3477901" {
		t.Fatalf("recovered run=%+v", draws)
	}
	if err := validate163PC28Cadence(draws); err != nil {
		t.Fatalf("recovered current run rejected: %v", err)
	}
}

func Test163PC28OutageBoundaryStillFailsBeforeFourRecoveredPeriods(t *testing.T) {
	latestAt := time.Date(2026, time.September, 4, 19, 40, 0, 0, sgSSCLocation).UTC()
	draw := func(issue int, drawAt time.Time) sourceDraw {
		return sourceDraw{
			Issue: strconv.Itoa(issue), Numbers: []int{3, 8, 9}, DrawAt: drawAt,
			SourceRevision: source163PC28Revision, ConversionRevision: source163MirrorConversionVersion,
		}
	}
	latest := draw(3477903, latestAt)
	history := []sourceDraw{
		latest,
		draw(3477902, latestAt.Add(-210*time.Second)),
		draw(3477901, latestAt.Add(-420*time.Second)),
		draw(3477900, time.Date(2026, time.September, 4, 18, 56, 30, 0, sgSSCLocation).UTC()),
	}
	if _, err := verified163PC28LatestAndHistory(latest, history); err == nil {
		t.Fatal("source reopened before four post-outage periods proved cadence")
	}
}

func source163PC28LagFixture() (sourceDraw, []sourceDraw) {
	latestAt := time.Date(2026, time.September, 4, 12, 3, 30, 0, sgSSCLocation).UTC()
	latest := sourceDraw{
		Issue: "3477609", Numbers: []int{1, 2, 3}, DrawAt: latestAt,
		SourceRevision: source163PC28Revision, ConversionRevision: source163MirrorConversionVersion,
	}
	history := make([]sourceDraw, source163MirrorHistoryLimit)
	for index := range history {
		history[index] = sourceDraw{
			Issue: strconv.Itoa(3477608 - index), Numbers: []int{1, 2, 3},
			DrawAt:         latestAt.Add(-time.Duration(index+1) * source163PC28Interval * time.Second),
			SourceRevision: source163PC28Revision, ConversionRevision: source163MirrorConversionVersion,
		}
	}
	return latest, history
}

func TestFetch163PC28AcceptsVerifiedImmediateLatestWhenHistoryLags(t *testing.T) {
	binding := source163PC28Bindings[0]
	latest, history := source163PC28LagFixture()
	now := latest.DrawAt.In(sgSSCLocation).Add(5 * time.Second)
	row := func(draw sourceDraw) map[string]any {
		return map[string]any{
			"igameid": binding.UpstreamGameID, "sgameperiod": draw.Issue,
			"sopennum": "1|2|3", "dopentime": draw.DrawAt.In(sgSSCLocation).Format("2006-01-02 15:04:05"),
		}
	}
	historyRows := make([]map[string]any, len(history))
	for index, draw := range history {
		historyRows[index] = row(draw)
	}
	requests := 0
	request := func(_ context.Context, endpoint string) ([]byte, error) {
		requests++
		parsed, err := url.Parse(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Query().Get("iGameId") != strconv.Itoa(source163PC28UpstreamGameID) {
			t.Fatalf("wrong upstream identity: %s", parsed.RawQuery)
		}
		if requests == 1 {
			if parsed.Query().Get("count") != "" {
				t.Fatalf("latest request has count: %s", parsed.RawQuery)
			}
			return source163MirrorPayload(t, row(latest)), nil
		}
		if parsed.Query().Get("count") != strconv.Itoa(source163MirrorHistoryLimit) {
			t.Fatalf("history count=%q", parsed.Query().Get("count"))
		}
		return source163MirrorPayload(t, historyRows), nil
	}
	draws, err := fetch163PC28DrawsWithRequest(
		context.Background(), binding, func() time.Time { return now },
		bytes.NewReader(bytes.Repeat([]byte{2, 8, 5, 7, 1}, 16)), request,
	)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(draws) != source163MirrorHistoryLimit+1 {
		t.Fatalf("requests=%d draws=%d", requests, len(draws))
	}
	if draws[0].Issue != latest.Issue || draws[0].SourceRevision != binding.Revision || draws[0].ConversionRevision != source163MirrorConversionVersion {
		t.Fatalf("latest draw=%+v", draws[0])
	}
	if err := validate163PC28Cadence(draws); err != nil {
		t.Fatalf("merged latest and lagging history rejected: %v", err)
	}
}

func Test163PC28ProductionCompletesOneRowRemoteHistoryFromExactVerifiedCache(t *testing.T) {
	latest, history := source163PC28LagFixture()
	latest.NextIssue = "remote-latest-wins"
	remoteHistory := []sourceDraw{latest}
	cached := append([]sourceDraw(nil), latest)
	cached[0].NextIssue = "cached-copy-must-not-win"
	cached = append(cached, history[:3]...)

	draws, err := merge163PC28VerifiedWindow(latest, remoteHistory, cached)
	if err != nil {
		t.Fatal(err)
	}
	if len(draws) != 4 {
		t.Fatalf("draws=%d want=4", len(draws))
	}
	if draws[0].Issue != latest.Issue || draws[0].NextIssue != "remote-latest-wins" {
		t.Fatalf("remote latest did not retain precedence: %+v", draws[0])
	}
	if err := validate163PC28Cadence(draws); err != nil {
		t.Fatalf("remote+verified cache chain rejected: %v", err)
	}
}

func Test163PC28OneRowRemoteHistoryWithoutVerifiedCacheFailsClosed(t *testing.T) {
	latest, _ := source163PC28LagFixture()
	remoteHistory := []sourceDraw{latest}
	if _, err := verified163PC28LatestAndHistory(latest, remoteHistory); err == nil {
		t.Fatal("remote-only diagnostics accepted a one-row history")
	}
	if _, err := merge163PC28VerifiedWindow(latest, remoteHistory, nil); err == nil {
		t.Fatal("first production sync accepted a one-row history without verified cache")
	}
}

func Test163PC28CacheFallbackStaysNarrowlyBehindRemoteBoundary(t *testing.T) {
	latest, history := source163PC28LagFixture()
	for _, test := range []struct {
		name          string
		remoteHistory []sourceDraw
		cache         []sourceDraw
	}{
		{
			name:          "empty remote history",
			remoteHistory: nil,
			cache:         history[:3],
		},
		{
			name:          "remote history lags multiple periods",
			remoteHistory: []sourceDraw{history[2]},
			cache:         history[:3],
		},
		{
			name:          "cache bridges an internal remote gap",
			remoteHistory: []sourceDraw{latest, history[1]},
			cache:         history[:3],
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := merge163PC28VerifiedWindow(latest, test.remoteHistory, test.cache); err == nil {
				t.Fatal("broad or cache-only fallback was accepted")
			}
		})
	}

	// Once the remote response independently proves four periods, an unrelated
	// older cache gap must not poison it. The cache is only relevant for exact
	// overlaps, or while supplying the few immediate predecessors needed to
	// reach four periods.
	remoteComplete := append([]sourceDraw{latest}, history[:3]...)
	if draws, err := merge163PC28VerifiedWindow(latest, remoteComplete, []sourceDraw{history[8]}); err != nil || len(draws) != 4 {
		t.Fatalf("healthy remote window was poisoned by unrelated old cache: draws=%d err=%v", len(draws), err)
	}

	remoteThree := append([]sourceDraw{latest}, history[:2]...)
	cacheWithIrrelevantOldGap := []sourceDraw{history[0], history[2], history[8]}
	if draws, err := merge163PC28VerifiedWindow(latest, remoteThree, cacheWithIrrelevantOldGap); err != nil || len(draws) != 4 {
		t.Fatalf("immediate cache prefix did not stop after reaching four: draws=%d err=%v", len(draws), err)
	}
}

func Test163PC28VerifiedCacheOverlapMustMatchRemoteExactly(t *testing.T) {
	latest, history := source163PC28LagFixture()
	remoteHistory := []sourceDraw{latest, history[0]}
	validCache := append([]sourceDraw(nil), history[:3]...)

	for _, test := range []struct {
		name string
		edit func([]sourceDraw)
	}{
		{
			name: "ordered numbers conflict",
			edit: func(draws []sourceDraw) { draws[0].Numbers = []int{9, 2, 3} },
		},
		{
			name: "timestamp conflict",
			edit: func(draws []sourceDraw) { draws[0].DrawAt = draws[0].DrawAt.Add(time.Second) },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := append([]sourceDraw(nil), validCache...)
			changed[0].Numbers = append([]int(nil), changed[0].Numbers...)
			test.edit(changed)
			if _, err := merge163PC28VerifiedWindow(latest, remoteHistory, changed); err == nil {
				t.Fatal("conflicting duplicate issue accepted")
			}
		})
	}
}

func Test163PC28VerifiedCacheRejectsWrongContractAndBrokenChain(t *testing.T) {
	latest, history := source163PC28LagFixture()
	remoteHistory := []sourceDraw{latest}
	for _, test := range []struct {
		name string
		edit func([]sourceDraw)
	}{
		{
			name: "legacy source revision",
			edit: func(draws []sourceDraw) { draws[0].SourceRevision = "platform-default" },
		},
		{
			name: "legacy conversion revision",
			edit: func(draws []sourceDraw) { draws[0].ConversionRevision = "" },
		},
		{
			name: "issue gap",
			edit: func(draws []sourceDraw) { draws[1].Issue = "3477605" },
		},
		{
			name: "wrong 210 second edge",
			edit: func(draws []sourceDraw) { draws[1].DrawAt = draws[1].DrawAt.Add(time.Second) },
		},
		{
			name: "cache is ahead of remote latest",
			edit: func(draws []sourceDraw) {
				draws[0].Issue = "3477610"
				draws[0].DrawAt = latest.DrawAt.Add(source163PC28Interval * time.Second)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			cached := append([]sourceDraw(nil), history[:3]...)
			test.edit(cached)
			if _, err := merge163PC28VerifiedWindow(latest, remoteHistory, cached); err == nil {
				t.Fatal("unverified or discontinuous local chain accepted")
			}
		})
	}
}

func TestFetch163PC28ObservationExposesUpstreamOneRowWithoutDatabaseFallback(t *testing.T) {
	binding := source163PC28Bindings[0]
	latest, _ := source163PC28LagFixture()
	now := latest.DrawAt.In(sgSSCLocation).Add(5 * time.Second)
	row := map[string]any{
		"igameid": binding.UpstreamGameID, "sgameperiod": latest.Issue,
		"sopennum": "1|2|3", "dopentime": latest.DrawAt.In(sgSSCLocation).Format("2006-01-02 15:04:05"),
	}
	requests := 0
	request := func(_ context.Context, _ string) ([]byte, error) {
		requests++
		if requests == 1 {
			return source163MirrorPayload(t, row), nil
		}
		return source163MirrorPayload(t, []map[string]any{row}), nil
	}
	observation, err := fetch163PC28ObservationWithRequest(
		context.Background(), binding, func() time.Time { return now },
		bytes.NewReader(bytes.Repeat([]byte{4, 2, 5, 7}, 20)), request,
	)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || observation.Latest.Issue != latest.Issue || len(observation.History) != 1 {
		t.Fatalf("requests=%d observation=%+v", requests, observation)
	}
	if _, err := verified163PC28LatestAndHistory(observation.Latest, observation.History); err == nil {
		t.Fatal("diagnostic remote-only verification silently used a cache")
	}
}

func TestFetch163PC28ObservationHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	requests := 0
	_, err := fetch163PC28ObservationWithRequest(
		ctx, source163PC28Bindings[0], time.Now,
		bytes.NewReader(bytes.Repeat([]byte{1, 2, 3, 4}, 20)),
		func(requestContext context.Context, _ string) ([]byte, error) {
			requests++
			return nil, requestContext.Err()
		},
	)
	if err == nil || requests != 1 {
		t.Fatalf("canceled observation err=%v requests=%d", err, requests)
	}
}

func TestParse163PC28StoredNumbersIsStrict(t *testing.T) {
	for _, value := range []string{"1,x,2,3", "1,,3", "1,2", "1,2,3,4", "1,-2,3", "1,10,3"} {
		if numbers, ok := parse163PC28StoredNumbers(value); ok || numbers != nil {
			t.Fatalf("malformed stored numbers %q accepted as %v", value, numbers)
		}
	}
	if numbers, ok := parse163PC28StoredNumbers(" 1, 0, 9 "); !ok || len(numbers) != 3 || numbers[0] != 1 || numbers[1] != 0 || numbers[2] != 9 {
		t.Fatalf("valid stored numbers rejected: %v/%v", numbers, ok)
	}
}

func Test163PC28LaggingLatestRejectsUnverifiedTransitions(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*sourceDraw, []sourceDraw)
	}{
		{
			name: "latest skips an issue",
			edit: func(latest *sourceDraw, _ []sourceDraw) { latest.Issue = "3477610" },
		},
		{
			name: "latest is one second late",
			edit: func(latest *sourceDraw, _ []sourceDraw) { latest.DrawAt = latest.DrawAt.Add(time.Second) },
		},
		{
			name: "latest is one second early",
			edit: func(latest *sourceDraw, _ []sourceDraw) { latest.DrawAt = latest.DrawAt.Add(-time.Second) },
		},
		{
			name: "history itself skips an issue",
			edit: func(_ *sourceDraw, history []sourceDraw) {
				for index := 2; index < len(history); index++ {
					issue, err := strconv.Atoi(history[index].Issue)
					if err != nil {
						t.Fatal(err)
					}
					history[index].Issue = strconv.Itoa(issue - 1)
				}
			},
		},
		{
			name: "history itself has a wrong time",
			edit: func(_ *sourceDraw, history []sourceDraw) {
				for index := 2; index < len(history); index++ {
					history[index].DrawAt = history[index].DrawAt.Add(-time.Second)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			latest, history := source163PC28LagFixture()
			test.edit(&latest, history)
			if _, err := verified163PC28LatestAndHistory(latest, history); err == nil {
				t.Fatal("unverified latest/history transition accepted")
			}
		})
	}
}

func Test163PC28MigrationOnlyClaimsUntouchedPlatformDefaults(t *testing.T) {
	binding := source163PC28Bindings[0]
	legacy := lottery.Game{ID: binding.GameID, SourceKind: "platform", SourceName: "王者开奖", SyncStatus: "ok", DrawInterval: 120}
	updates, required := source163PC28BindingUpdates(legacy, binding)
	if !required || updates["source_name"] != source163MirrorName || updates["source_url"] != source163MirrorURL ||
		updates["sync_status"] != "stale" || updates["last_sync_error"] != source163MirrorPendingMessage || updates["last_sync_at"] != nil ||
		updates["draw_interval"] != source163PC28Interval {
		t.Fatalf("cutover=%#v required=%v", updates, required)
	}

	current := lottery.Game{ID: binding.GameID, SourceKind: "external", SourceName: source163MirrorName, SourceURL: source163MirrorURL, SyncStatus: "ok"}
	if updates, required := source163PC28BindingUpdates(current, binding); required || updates != nil {
		t.Fatalf("healthy binding reset=%#v/%v", updates, required)
	}
	current.SyncStatus = "syncing"
	if updates, required := source163PC28BindingUpdates(current, binding); !required || updates["sync_status"] != "stale" {
		t.Fatalf("interrupted binding not closed=%#v/%v", updates, required)
	}

	for _, custom := range []lottery.Game{
		{ID: binding.GameID, SourceKind: "platform", SourceName: "商户自开"},
		{ID: binding.GameID, SourceKind: "external", SourceName: "商户外部源", SourceURL: "https://operator.example/"},
		{ID: "unrelated", SourceKind: "platform", SourceName: "王者开奖"},
	} {
		if updates, required := source163PC28BindingUpdates(custom, binding); required || updates != nil {
			t.Fatalf("custom source overwritten: %+v -> %#v/%v", custom, updates, required)
		}
	}
}

func Test163PC28PostgresCacheLoadsOnlyExactGameAndRevision(t *testing.T) {
	db := timingPostgresDatabase(t)
	binding := source163PC28Bindings[0]
	latest, history := source163PC28LagFixture()
	exact := make([]lottery.Draw, 0, 3)
	for _, draw := range history[:3] {
		exact = append(exact, lottery.Draw{
			GameID: binding.GameID, Issue: draw.Issue, Numbers: joinNumbers(draw.Numbers), DrawAt: draw.DrawAt,
			SourceRevision: binding.Revision, ConversionRevision: source163MirrorConversionVersion,
		})
	}
	noise := []lottery.Draw{
		{GameID: binding.GameID, Issue: "3477605", Numbers: "1,2,3", DrawAt: history[2].DrawAt.Add(-source163PC28Interval * time.Second), SourceRevision: "platform-default", ConversionRevision: source163MirrorConversionVersion},
		{GameID: binding.GameID, Issue: "3477604", Numbers: "1,2,3", DrawAt: history[2].DrawAt.Add(-2 * source163PC28Interval * time.Second), SourceRevision: binding.Revision, ConversionRevision: "legacy-direct"},
		{GameID: source163PC28Bindings[1].GameID, Issue: history[0].Issue, Numbers: "1,2,3", DrawAt: history[0].DrawAt, SourceRevision: binding.Revision, ConversionRevision: source163MirrorConversionVersion},
	}
	if err := db.Create(&exact).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&noise).Error; err != nil {
		t.Fatal(err)
	}

	loaded, err := load163PC28VerifiedHistory(context.Background(), db, binding)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != len(exact) {
		t.Fatalf("loaded=%d want=%d; legacy/other-game rows leaked into cache", len(loaded), len(exact))
	}
	draws, err := merge163PC28VerifiedWindow(latest, []sourceDraw{latest}, loaded)
	if err != nil {
		t.Fatal(err)
	}
	if len(draws) != 4 {
		t.Fatalf("completed draws=%d want=4", len(draws))
	}
}
