package services

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"backend/data/models/lottery"

	"gorm.io/gorm"
)

func source163MarkSixTestRows(binding source163MarkSixBinding, latestAt time.Time, interval time.Duration) []map[string]any {
	rows := make([]map[string]any, 8)
	for index := range rows {
		rows[index] = map[string]any{
			"igameid": binding.UpstreamGameID, "sgameperiod": 2026246 - index,
			"sopennum":  "22|24|19|10|20|1|30",
			"dopentime": latestAt.Add(-time.Duration(index) * interval).Format("2006-01-02 15:04:05"),
		}
	}
	return rows
}

func copy163MarkSixRow(row map[string]any) map[string]any {
	result := make(map[string]any, len(row))
	for key, value := range row {
		result[key] = value
	}
	return result
}

func Test163MarkSixRegistersFourIndependentContracts(t *testing.T) {
	want := map[string]int{"hong-kong-mark-six": 18, "happy8-mark-six": 141, "new-macau-mark-six": 140, "old-macau-mark-six": 70}
	if len(source163MarkSixBindings) != len(want) {
		t.Fatalf("bindings=%+v", source163MarkSixBindings)
	}
	seenSource, seenConversion := map[string]bool{}, map[string]bool{}
	for gameID, upstream := range want {
		binding, ok := source163MarkSixBindingForGame(gameID)
		mirror := binding.mirrorBinding()
		if !ok || binding.UpstreamGameID != upstream || mirror.Count != 7 || !mirror.Unique {
			t.Fatalf("binding %s=%+v found=%v", gameID, binding, ok)
		}
		if binding.SourceRevision == "" || binding.ConversionRevision == "" || seenSource[binding.SourceRevision] || seenConversion[binding.ConversionRevision] {
			t.Fatalf("%s does not have independent revisions: %+v", gameID, binding)
		}
		seenSource[binding.SourceRevision], seenConversion[binding.ConversionRevision] = true, true
		game := lottery.Game{ID: gameID, SourceKind: "external", SourceName: source163MirrorName, SourceURL: source163MirrorURL}
		if !source163MarkSixBound(&game, binding) {
			t.Fatalf("exact binding rejected for %s", gameID)
		}
	}
}

func Test163MarkSixBettingHealthRequiresEveryExactBinding(t *testing.T) {
	for _, binding := range source163MarkSixBindings {
		t.Run(binding.GameID, func(t *testing.T) {
			valid := lottery.Game{
				ID: binding.GameID, SourceKind: "external", SourceName: source163MirrorName,
				SourceURL: source163MirrorURL, SyncStatus: "ok",
			}
			if !sourceHealthyForGame(&valid) {
				t.Fatal("exact healthy 163 Mark Six binding was rejected")
			}
			mutations := []struct {
				name   string
				mutate func(*lottery.Game)
			}{
				{name: "kind", mutate: func(game *lottery.Game) { game.SourceKind = "platform" }},
				{name: "name", mutate: func(game *lottery.Game) { game.SourceName = legacy168HighFreqName }},
				{name: "url", mutate: func(game *lottery.Game) { game.SourceURL = legacy168HighFreqURL }},
				{name: "stale", mutate: func(game *lottery.Game) { game.SyncStatus = "stale" }},
				{name: "error", mutate: func(game *lottery.Game) { game.SyncStatus = "error" }},
				{name: "syncing with error", mutate: func(game *lottery.Game) {
					game.SyncStatus, game.LastSyncError = "syncing", "upstream failed"
				}},
			}
			for _, mutation := range mutations {
				t.Run(mutation.name, func(t *testing.T) {
					changed := valid
					mutation.mutate(&changed)
					if sourceHealthyForGame(&changed) {
						t.Fatalf("accepted unsafe binding/state: %+v", changed)
					}
				})
			}
		})
	}
}

func Test163MarkSixCutoverOnlyAcceptsExactFormerDefaults(t *testing.T) {
	for _, binding := range source163MarkSixBindings {
		legacy := lottery.Game{ID: binding.GameID, SourceKind: "external", SourceName: legacy168HighFreqName, SourceURL: legacy168HighFreqURL}
		if updates, ok := source163MarkSixBindingUpdates(legacy, binding); !ok || updates["source_name"] != source163MirrorName {
			t.Fatalf("legacy %s was not cut over: %+v %v", binding.GameID, updates, ok)
		}
		custom := legacy
		custom.SourceURL = "https://operator.example/"
		if _, ok := source163MarkSixBindingUpdates(custom, binding); ok {
			t.Fatalf("custom source overwritten for %s", binding.GameID)
		}
	}
	happy, _ := source163MarkSixBindingForGame("happy8-mark-six")
	if updates, ok := source163MarkSixBindingUpdates(lottery.Game{ID: happy.GameID, SourceKind: "platform", SourceName: "王者开奖"}, happy); !ok || updates["source_kind"] != "external" {
		t.Fatalf("happy8 exact platform default not cut over: %+v %v", updates, ok)
	}
}

func TestEnsure168CannotWriteAnyRegistered163MarkSixProduct(t *testing.T) {
	db := robotDryRunDB(t)
	writes := 0
	if err := db.Callback().Update().Before("gorm:update").Register("test:no_168_marksix_write", func(*gorm.DB) { writes++ }); err != nil {
		t.Fatal(err)
	}
	if err := Ensure168SourceGames(db); err != nil {
		t.Fatal(err)
	}
	if writes != 0 {
		t.Fatalf("retired Ensure168 attempted %d Mark Six metadata writes", writes)
	}
}

func TestFetch163MarkSixUsesExplicitNextIssueAndOpenTime(t *testing.T) {
	for _, binding := range source163MarkSixBindings {
		t.Run(binding.GameID, func(t *testing.T) {
			latestAt := time.Date(2026, 9, 3, 21, 30, 0, 0, sgSSCLocation)
			interval := source163MarkSixDailyInterval
			if !binding.Daily {
				interval = 48 * time.Hour
			}
			rows := source163MarkSixTestRows(binding, latestAt, interval)
			nextAt := latestAt.Add(interval)
			rows[0]["nextGamePeriod"], rows[0]["realNextGamePeriod"] = 2026247, 2026247
			rows[0]["nextPeriodOpenTime"] = nextAt.UnixMilli()
			calls := 0
			request := func(_ context.Context, _ string) ([]byte, error) {
				calls++
				if calls == 1 {
					return source163MirrorPayload(t, rows[0]), nil
				}
				return source163MirrorPayload(t, rows), nil
			}
			now := latestAt.Add(12 * time.Hour)
			draws, err := fetch163MarkSixDrawsWithRequest(context.Background(), binding, func() time.Time { return now }, bytes.NewReader(bytes.Repeat([]byte{4, 1, 0, 8}, 16)), request)
			if err != nil {
				t.Fatal(err)
			}
			schedule, err := schedule163MarkSix(binding, draws)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 2 || schedule.Issue != "2026247" || !schedule.DrawAt.Equal(nextAt.UTC()) || schedule.Source != "upstream" {
				t.Fatalf("calls=%d schedule=%+v", calls, schedule)
			}
			for _, draw := range draws {
				if draw.SourceRevision != binding.SourceRevision || draw.ConversionRevision != binding.ConversionRevision {
					t.Fatalf("draw contract=%+v", draw)
				}
			}
		})
	}
}

func Test163MarkSixFailsClosedOnInvalidSevenBallOrNextBoundary(t *testing.T) {
	binding, _ := source163MarkSixBindingForGame("new-macau-mark-six")
	latestAt := time.Date(2026, 9, 3, 21, 30, 0, 0, sgSSCLocation)
	for _, test := range []struct {
		name string
		edit func(map[string]any)
		want string
	}{
		{"duplicate ball", func(row map[string]any) { row["sopennum"] = "22|22|19|10|20|1|30" }, "重复"},
		{"six balls", func(row map[string]any) { row["sopennum"] = "22|24|19|10|20|1" }, "数量"},
		{"wrong next issue", func(row map[string]any) { row["realNextGamePeriod"] = 2026248 }, "连续期"},
		{"missing next time", func(row map[string]any) { row["nextPeriodOpenTime"] = 0 }, "开奖时间"},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := source163MarkSixTestRows(binding, latestAt, source163MarkSixDailyInterval)
			rows[0]["nextGamePeriod"], rows[0]["realNextGamePeriod"] = 2026247, 2026247
			rows[0]["nextPeriodOpenTime"] = latestAt.Add(source163MarkSixDailyInterval).UnixMilli()
			latest := copy163MarkSixRow(rows[0])
			test.edit(latest)
			calls := 0
			request := func(_ context.Context, _ string) ([]byte, error) {
				calls++
				if calls == 1 {
					return source163MirrorPayload(t, latest), nil
				}
				return source163MirrorPayload(t, rows), nil
			}
			_, err := fetch163MarkSixDrawsWithRequest(context.Background(), binding, func() time.Time { return latestAt.Add(12 * time.Hour) }, bytes.NewReader(bytes.Repeat([]byte{1}, 64)), request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err=%v want %q", err, test.want)
			}
		})
	}
}

func Test163MarkSixLegacyDrawsAreNotTrusted(t *testing.T) {
	for _, binding := range source163MarkSixBindings {
		if trustedDrawRevisionMatches(binding.GameID, "", "") || trustedDrawRevisionMatches(binding.GameID, binding.SourceRevision, "") {
			t.Fatalf("%s accepts blank legacy revision", binding.GameID)
		}
		if !trustedDrawRevisionMatches(binding.GameID, binding.SourceRevision, binding.ConversionRevision) {
			t.Fatalf("%s rejects current exact contract", binding.GameID)
		}
		if err := betDrawRevisionError(binding.GameID, "2026247", "", binding.SourceRevision); err == nil {
			t.Fatalf("%s allowed a blank ticket source snapshot", binding.GameID)
		}
	}
}

func TestPromotable163MarkSixDrawRequiresExactBlankLegacyEvidence(t *testing.T) {
	binding, _ := source163MarkSixBindingForGame("hong-kong-mark-six")
	drawAt := time.Date(2026, 8, 27, 13, 30, 0, 0, time.UTC)
	incoming := sourceDraw{
		Issue: "2026094", Numbers: []int{12, 45, 44, 25, 17, 31, 35}, DrawAt: drawAt,
		SourceRevision: binding.SourceRevision, ConversionRevision: binding.ConversionRevision,
	}
	exact := lottery.Draw{GameID: binding.GameID, Issue: incoming.Issue, Numbers: joinNumbers(incoming.Numbers), DrawAt: drawAt}
	if !promotable163MarkSixDraw(exact, incoming, binding) {
		t.Fatal("fully matching blank legacy row was not eligible")
	}
	for _, test := range []struct {
		name string
		edit func(*lottery.Draw)
	}{
		{"different issue", func(row *lottery.Draw) { row.Issue = "2026093" }},
		{"different ordered numbers", func(row *lottery.Draw) { row.Numbers = "45,12,44,25,17,31,35" }},
		{"different draw time", func(row *lottery.Draw) { row.DrawAt = row.DrawAt.Add(24 * time.Hour) }},
		{"existing source", func(row *lottery.Draw) { row.SourceRevision = "legacy-168-v1" }},
		{"existing conversion", func(row *lottery.Draw) { row.ConversionRevision = "legacy-direct-v1" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			row := exact
			test.edit(&row)
			if promotable163MarkSixDraw(row, incoming, binding) {
				t.Fatalf("unsafe row was eligible: %+v", row)
			}
		})
	}
}

func TestPromotable163NewMacauDrawAllowsOnlyExactLegacyDirectV1(t *testing.T) {
	binding, _ := source163MarkSixBindingForGame("new-macau-mark-six")
	drawAt := time.Date(2026, 9, 4, 13, 30, 0, 0, time.UTC)
	incoming := sourceDraw{
		Issue: "2026247", Numbers: []int{12, 45, 44, 25, 17, 31, 35}, DrawAt: drawAt,
		SourceRevision: binding.SourceRevision, ConversionRevision: binding.ConversionRevision,
	}
	exactLegacy := lottery.Draw{
		GameID: binding.GameID, Issue: incoming.Issue, Numbers: joinNumbers(incoming.Numbers), DrawAt: drawAt,
		SourceRevision: binding.SourceRevision, ConversionRevision: source163MirrorConversionVersion,
	}
	if !promotable163MarkSixDraw(exactLegacy, incoming, binding) {
		t.Fatal("exact new-macau direct-v1 row was not eligible")
	}
	for _, test := range []struct {
		name         string
		editRow      func(*lottery.Draw)
		editIncoming func(*sourceDraw)
	}{
		{name: "different issue", editRow: func(row *lottery.Draw) { row.Issue = "2026246" }},
		{name: "different ordered numbers", editRow: func(row *lottery.Draw) { row.Numbers = "45,12,44,25,17,31,35" }},
		{name: "different draw time", editRow: func(row *lottery.Draw) { row.DrawAt = row.DrawAt.Add(time.Second) }},
		{name: "other source", editRow: func(row *lottery.Draw) { row.SourceRevision = "other-source-v1" }},
		{name: "other conversion", editRow: func(row *lottery.Draw) { row.ConversionRevision = "direct-v0" }},
		{name: "current conversion", editRow: func(row *lottery.Draw) { row.ConversionRevision = binding.ConversionRevision }},
		{name: "wrong incoming source", editIncoming: func(row *sourceDraw) { row.SourceRevision = "other-source-v1" }},
		{name: "wrong incoming conversion", editIncoming: func(row *sourceDraw) { row.ConversionRevision = "direct-v2" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			row, candidate := exactLegacy, incoming
			if test.editRow != nil {
				test.editRow(&row)
			}
			if test.editIncoming != nil {
				test.editIncoming(&candidate)
			}
			if promotable163MarkSixDraw(row, candidate, binding) {
				t.Fatalf("unsafe legacy row was eligible: row=%+v incoming=%+v", row, candidate)
			}
		})
	}

	hongKong, _ := source163MarkSixBindingForGame("hong-kong-mark-six")
	hongKongIncoming := incoming
	hongKongIncoming.SourceRevision = hongKong.SourceRevision
	hongKongIncoming.ConversionRevision = hongKong.ConversionRevision
	hongKongRow := exactLegacy
	hongKongRow.GameID = hongKong.GameID
	hongKongRow.SourceRevision = hongKong.SourceRevision
	if promotable163MarkSixDraw(hongKongRow, hongKongIncoming, hongKong) {
		t.Fatal("direct-v1 compatibility leaked from new-macau to hong-kong")
	}
}

func TestRepairable163HongKongIssueRequiresEverySafetyPredicate(t *testing.T) {
	now := time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC)
	oldAt, newAt := now.Add(-time.Hour), now.Add(24*time.Hour)
	row := lottery.Issue{
		GameID: source163HongKongGameID, Issue: "2026096", Status: lottery.IssueStatusError,
		LastError: " 对账异常：旧开奖边界已过期", ScheduledDrawAt: &oldAt,
	}
	schedule := sourceSchedule{Issue: row.Issue, DrawAt: newAt, Interval: int(72 * time.Hour / time.Second), Source: "upstream"}
	if !repairable163HongKongIssue(row, schedule, now, 0, 0) {
		t.Fatal("fully isolated overdue reconciliation issue was not repairable")
	}
	settled := now.Add(-time.Minute)
	for _, test := range []struct {
		name        string
		editRow     func(*lottery.Issue)
		editSource  func(*sourceSchedule)
		bets, draws int64
	}{
		{name: "other game", editRow: func(item *lottery.Issue) { item.GameID = "new-macau-mark-six" }},
		{name: "different issue", editSource: func(item *sourceSchedule) { item.Issue = "2026097" }},
		{name: "not error", editRow: func(item *lottery.Issue) { item.Status = lottery.IssueStatusAwaiting }},
		{name: "ordinary source error", editRow: func(item *lottery.Issue) { item.LastError = "上游超时" }},
		{name: "missing old boundary", editRow: func(item *lottery.Issue) { item.ScheduledDrawAt = nil }},
		{name: "old boundary still future", editRow: func(item *lottery.Issue) { value := now.Add(time.Minute); item.ScheduledDrawAt = &value }},
		{name: "new boundary not later", editSource: func(item *sourceSchedule) { item.DrawAt = oldAt }},
		{name: "new boundary expired", editSource: func(item *sourceSchedule) { item.DrawAt = now }},
		{name: "draw pointer present", editRow: func(item *lottery.Issue) { item.DrawAt = &settled }},
		{name: "settled pointer present", editRow: func(item *lottery.Issue) { item.SettledAt = &settled }},
		{name: "any bet", bets: 1},
		{name: "any raw draw", draws: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate, source := row, schedule
			if test.editRow != nil {
				test.editRow(&candidate)
			}
			if test.editSource != nil {
				test.editSource(&source)
			}
			if repairable163HongKongIssue(candidate, source, now, test.bets, test.draws) {
				t.Fatalf("unsafe lifecycle became repairable: row=%+v schedule=%+v", candidate, source)
			}
		})
	}
}
