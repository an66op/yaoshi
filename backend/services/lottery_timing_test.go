package services

import (
	"backend/data/models/lottery"
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

func TestConfiguredSealSecondsRoomAndPerGame(t *testing.T) {
	for _, test := range []struct {
		name, raw, game string
		want            int
	}{
		{"default", `{}`, "speed-racing", 30},
		{"room", `{"seal_seconds":45}`, "speed-racing", 45},
		{"zero", `{"seal_seconds":0}`, "speed-racing", 0},
		{"game", `{"seal_seconds":45,"game_timing_overrides":{"speed-racing":{"seal_seconds":10}}}`, "speed-racing", 10},
		{"other game", `{"seal_seconds":45,"game_timing_overrides":{"speed-racing":{"seal_seconds":10}}}`, "speed-fly", 45},
		{"bad legacy value", `{"seal_seconds":-1}`, "speed-racing", 30},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := configuredSealSeconds(test.raw, test.game); got != test.want {
				t.Fatalf("configuredSealSeconds = %d, want %d", got, test.want)
			}
		})
	}
}

func TestValidateGameTimingSettings(t *testing.T) {
	for _, raw := range []string{`{"seal_seconds":-1}`, `{"seal_seconds":1.5}`, `{"seal_seconds":86401}`, `{"seal_seconds":"30"}`, `{"seal_seconds":null}`, `{"game_timing_overrides":[]}`, `{"game_timing_overrides":{"speed-racing":{"seal_seconds":-2}}}`} {
		if err := validateGameTimingSettings(json.RawMessage(raw)); err == nil {
			t.Fatalf("invalid timing settings accepted: %s", raw)
		}
	}
	for _, raw := range []string{`{}`, `{"seal_seconds":0}`, `{"seal_seconds":86400}`, `{"game_timing_overrides":{"speed-racing":{"seal_seconds":15}}}`} {
		if err := validateGameTimingSettings(json.RawMessage(raw)); err != nil {
			t.Fatalf("valid settings rejected: %s: %v", raw, err)
		}
	}
}

func TestIssueWindowPhaseBoundaries(t *testing.T) {
	drawAt := time.Date(2026, 8, 30, 6, 0, 0, 0, time.UTC)
	game := lottery.Game{ID: "speed-racing", DrawInterval: 75}
	window := newIssueWindow(2, &game, "34136803", drawAt, 30)
	for _, test := range []struct {
		before time.Duration
		want   string
	}{
		{76 * time.Second, lottery.IssueStatusPending},
		{75 * time.Second, lottery.IssueStatusAccepting},
		{30*time.Second + time.Nanosecond, lottery.IssueStatusAccepting},
		{30 * time.Second, lottery.IssueStatusSealed},
		{time.Nanosecond, lottery.IssueStatusSealed},
		{0, lottery.IssueStatusAwaiting},
		{-time.Minute, lottery.IssueStatusAwaiting},
	} {
		if got := windowStatus(&window, drawAt.Add(-test.before)); got != test.want {
			t.Fatalf("phase at draw-%v = %s, want %s", test.before, got, test.want)
		}
	}
	zero := newIssueWindow(3, &game, "34136803", drawAt, 0)
	if windowStatus(&zero, drawAt.Add(-time.Nanosecond)) != lottery.IssueStatusAccepting || windowStatus(&zero, drawAt) != lottery.IssueStatusAwaiting {
		t.Fatal("zero-second sealing must accept until (but never at) draw boundary")
	}
}

func TestIssueWindowCannotExtendOrReopen(t *testing.T) {
	drawAt := time.Date(2026, 8, 30, 6, 0, 0, 0, time.UTC)
	game := lottery.Game{ID: "speed-fly", DrawInterval: 75}
	stored := newIssueWindow(2, &game, "54776127", drawAt, 30)
	later := newIssueWindow(2, &game, "54776127", drawAt.Add(time.Minute), 0)
	got := shortenIssueWindow(stored, later)
	if !got.SealAt.Equal(stored.SealAt) || !got.ScheduledDrawAt.Equal(drawAt) {
		t.Fatalf("same issue extended: %+v", got)
	}
	lessSeal := newIssueWindow(2, &game, stored.Issue, drawAt, 10)
	if got := shortenIssueWindow(stored, lessSeal); got.SealSeconds != 30 {
		t.Fatalf("reducing configured seal reopened current window: %+v", got)
	}
	earlier := newIssueWindow(2, &game, stored.Issue, drawAt, 45)
	if got := shortenIssueWindow(stored, earlier); got.SealSeconds != 45 || !got.SealAt.Equal(earlier.SealAt) {
		t.Fatalf("earlier closure was not honored: %+v", got)
	}
}

func TestSharedIssueLockDoesNotApplyPlatformSealToOtherRooms(t *testing.T) {
	now := time.Date(2026, 8, 30, 6, 0, 0, 0, time.UTC)
	drawAt := now.Add(20 * time.Second)
	row := lottery.Issue{Status: lottery.IssueStatusSealed, SealAt: now.Add(-10 * time.Second), ScheduledDrawAt: &drawAt}
	if !sharedIssueOpen(&row, now) {
		t.Fatal("platform T-30 sealing must not block a room with a T-10 window")
	}
	for _, status := range []string{lottery.IssueStatusSettled, lottery.IssueStatusSettling, lottery.IssueStatusError, "unknown"} {
		row.Status = status
		if sharedIssueOpen(&row, now) {
			t.Fatalf("terminal/unknown status %q accepted", status)
		}
	}
	row.Status = lottery.IssueStatusAccepting
	if sharedIssueOpen(&row, drawAt) {
		t.Fatal("exact draw boundary accepted")
	}
	row.DrawAt = &now
	if sharedIssueOpen(&row, now) {
		t.Fatal("published result accepted")
	}
}

func timingDrawFixtures(interval int) []sourceDraw {
	anchor := time.Date(2026, 8, 30, 5, 41, 48, 0, time.UTC)
	rows := make([]sourceDraw, 8)
	for index := range rows {
		rows[index] = sourceDraw{Issue: strconv.Itoa(34136802 - index), DrawAt: anchor.Add(-time.Duration(index*interval) * time.Second)}
	}
	return rows
}

func TestObservedDrawIntervalUsesConsecutivePublishedIssues(t *testing.T) {
	for _, interval := range []int{75, 300} {
		if got := observedDrawInterval(timingDrawFixtures(interval)); got != interval {
			t.Fatalf("observed interval = %d, want %d", got, interval)
		}
	}
	rows := timingDrawFixtures(75)
	rows = []sourceDraw{rows[0], rows[2], rows[4], rows[6]}
	if got := observedDrawInterval(rows); got != 0 {
		t.Fatalf("skipped periods inferred a false cadence: %d", got)
	}
	if got := observedDrawInterval(timingDrawFixtures(75)[:3]); got != 0 {
		t.Fatalf("less than three intervals treated as enough evidence: %d", got)
	}
	if got := observedDrawInterval(timingDrawFixtures(86400)); got != 0 {
		t.Fatalf("calendar schedule treated as a fixed high-frequency cadence: %d", got)
	}
}

func TestSourceScheduleCorrectsSeedIntervalWithoutRollingExpiredIssue(t *testing.T) {
	rows := timingDrawFixtures(75)
	game := lottery.Game{ID: "speed-racing", DrawInterval: 180}
	schedule, err := scheduleFromDraws(game, rows)
	if err != nil {
		t.Fatal(err)
	}
	if schedule.Interval != 75 || schedule.Source != "observed" || schedule.Issue != "34136803" || !schedule.DrawAt.Equal(rows[0].DrawAt.Add(75*time.Second)) {
		t.Fatalf("incorrect schedule: %+v", schedule)
	}
	// There is deliberately no 'now' input. Re-reading a stale history cannot
	// roll the same issue forward by another interval or reopen its bets.
	repeated, err := scheduleFromDraws(game, rows)
	if err != nil || repeated != schedule {
		t.Fatalf("stale schedule drifted: %+v %v", repeated, err)
	}
}

func TestSourceScheduleHonorsExplicitUpcomingIssueAtDailyRollover(t *testing.T) {
	drawAt := time.Date(2026, 8, 29, 16, 0, 0, 0, time.UTC)
	game := lottery.Game{ID: "sg-fly", DrawInterval: 300}
	rows := []sourceDraw{{Issue: "20260829288", DrawAt: drawAt, NextIssue: "20260830001", NextDrawAt: drawAt.Add(5 * time.Minute)}}
	schedule, err := scheduleFromDraws(game, rows)
	if err != nil || schedule.Issue != "20260830001" || schedule.Source != "upstream" {
		t.Fatalf("explicit daily rollover not preserved: %+v %v", schedule, err)
	}
	rows[0].NextIssue, rows[0].NextDrawAt = "", time.Time{}
	schedule, err = scheduleFromDraws(game, rows)
	if err != nil || schedule.Issue != "" {
		t.Fatalf("invented midnight issue: %+v %v", schedule, err)
	}
}

func TestSourceIssueInferenceRejectsFixturesAndUnknownTiming(t *testing.T) {
	next := time.Date(2026, 8, 30, 6, 0, 0, 0, time.UTC)
	for _, issue := range []string{"", "SPEED_RACING-999", "abc", "0", "18446744073709551615"} {
		if got := inferredNextSourceIssue(issue, next); got != "" {
			t.Fatalf("inferred %q from %q", got, issue)
		}
	}
	if got := inferredNextSourceIssue("34136802", time.Time{}); got != "" {
		t.Fatalf("unknown time inferred issue %q", got)
	}
	if got := inferredNextSourceIssue("34136802", next); got != "34136803" {
		t.Fatalf("consecutive issue = %q", got)
	}
	if validNextSourceIssue("34136802", "34136801") || validNextSourceIssue("34136802", "34136802") {
		t.Fatal("old upstream nextIssue accepted")
	}
}

func Test168TimingFieldsAndInvalidTimestamp(t *testing.T) {
	rows := parseAPI168Rows(json.RawMessage(`{"preDrawIssue":34136802,"preDrawTime":"2026-08-30 13:41:48","preDrawCode":"1,2,3,4,5,6,7,8,9,10","drawIssue":34136803,"drawTime":"2026-08-30 13:43:03"}`))
	if len(rows) != 1 || api168IssueText(rows[0].NextIssue) != "34136803" || parse168DrawTime(rows[0].NextTime).Sub(parse168DrawTime(rows[0].Time)) != 75*time.Second {
		t.Fatalf("upstream schedule metadata was lost: %+v", rows)
	}
	for _, value := range []string{"", "not-a-date", "2026-99-30 13:00:00"} {
		if got := parse168DrawTime(value); !got.IsZero() {
			t.Fatalf("invalid source time %q silently became %s", value, got)
		}
	}
	if got := parse168DrawTime("1787469820000").Unix(); got != 1787469820 {
		t.Fatalf("millisecond timestamp = %d", got)
	}
}

func TestInitialPlatformIssueUsesFixedScheduleNotReadTime(t *testing.T) {
	drawAt := time.Date(2026, 8, 30, 6, 1, 15, 0, time.UTC)
	if got := initialPlatformIssue(drawAt); got != "20260830140115" {
		t.Fatalf("scheduled initial issue = %q", got)
	}
	if initialPlatformIssue(drawAt) != initialPlatformIssue(drawAt.In(time.FixedZone("CST", 8*3600))) {
		t.Fatal("same event in a different timezone created another issue")
	}
	if got := initialPlatformIssue(time.Time{}); got != "" {
		t.Fatalf("unscheduled platform invented issue %q", got)
	}
}
