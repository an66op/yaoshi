package services

import (
	"backend/data/models/lottery"
	"backend/data/models/plan"
	"backend/data/models/settings"
	apperrors "backend/errors"
	"context"
	"fmt"
	"gorm.io/gorm"
	"reflect"
	"testing"
	"time"
)

func streamPostgresSetup(t *testing.T) (*gorm.DB, uint64) {
	t.Helper()
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "stream_plan_room", "732401")
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "speed-racing", true); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&settings.SystemConfig{}).Where("workspace_id = ?", room.ID).Updates(map[string]any{"room_enabled": true, "prediction_enabled": true, "game_settings_json": `{"seal_seconds":30}`}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&lottery.Game{}).Where("id = ?", "speed-racing").Updates(map[string]any{"enabled": true, "source_kind": "external", "timing_source": "upstream", "sync_status": "ok", "last_sync_error": "", "next_issue": "", "lobby_category": "彩票", "draw_interval": 300}).Error; err != nil {
		t.Fatal(err)
	}
	on := true
	if _, err := NewPlanAutomationService(db).Save(room.ID, PlanAutomationInput{Enabled: &on, GameIDs: []string{"speed-racing"}}); err != nil {
		t.Fatal(err)
	}
	return db, room.ID
}

func streamPostgresIssue(t *testing.T, db *gorm.DB, n int) lottery.Issue {
	t.Helper()
	drawAt := time.Now().UTC().Add(2*time.Minute + time.Duration(n)*time.Second)
	issue := lottery.Issue{GameID: "speed-racing", Issue: fmt.Sprintf("confirmed-stream-%d", n), SourceMode: "external", Status: lottery.IssueStatusAccepting, AcceptAt: drawAt.Add(-5 * time.Minute), SealAt: drawAt.Add(-30 * time.Second), ScheduledDrawAt: &drawAt}
	if err := db.Create(&issue).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&lottery.Game{}).Where("id = ?", "speed-racing").Updates(map[string]any{"next_issue": issue.Issue, "next_draw_at": drawAt}).Error; err != nil {
		t.Fatal(err)
	}
	return issue
}

func agePlanVisit(t *testing.T, db *gorm.DB, roomID uint64) {
	t.Helper()
	if err := db.Model(&plan.Stream{}).Where("workspace_id = ?", roomID).UpdateColumn("updated_at", time.Now().UTC().Add(-6*time.Second)).Error; err != nil {
		t.Fatal(err)
	}
}

func TestPlanStreamsPostgresVisitOnlyTargetCoalescingAndReadOnly(t *testing.T) {
	db, roomID := streamPostgresSetup(t)
	svc, automation := NewPlanContentService(db), NewPlanAutomationService(db)
	ctx := context.Background()
	before, err := svc.StreamDetail(roomID, 1, DefaultPlanKey)
	if err != nil || before.Stream.Active || !before.Stream.ActivationRequired || !before.AutomationEnabled || before.GenerationMode != "on_visit" || before.HistoryLimit != 6 {
		t.Fatal("default requires visit", before, err)
	}
	streamPostgresIssue(t, db, 1)
	if _, err := automation.RunWorkspace(ctx, roomID); apperrors.GetErrorCode(err) != "PLAN_VISIT_REQUIRED" {
		t.Fatal("room-wide generation remains", err)
	}
	if _, err := svc.Catalog(roomID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StreamDetail(roomID, 1, DefaultPlanKey); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&plan.Stream{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("read or worker wrote stream", count, err)
	}
	initial, err := svc.ActivateStream(ctx, roomID, 1, DefaultPlanKey)
	if err != nil || len(initial.Recommendations) != 3 || initial.Stream.ActiveCount != 1 {
		t.Fatal(initial, err)
	}
	var original plan.Stream
	if err := db.First(&original, initial.Stream.ID).Error; err != nil {
		t.Fatal(err)
	}
	if original.ActiveUntil == nil || original.ActiveUntil.Sub(original.UpdatedAt) > time.Minute+time.Second {
		t.Fatal("lease exceeds minute", original)
	}
	if _, err := NewPlanContentService(db).ActivateStream(ctx, roomID, 1, DefaultPlanKey); err != nil {
		t.Fatal(err)
	}
	var after plan.Stream
	if err := db.First(&after, original.ID).Error; err != nil || !after.UpdatedAt.Equal(original.UpdatedAt) || !after.ActiveUntil.Equal(*original.ActiveUntil) {
		t.Fatal("repeat not coalesced", after, err)
	}
	second, err := svc.ActivateStream(ctx, roomID, 2, "three-period-six-codes")
	if err != nil || len(second.Recommendations) != 3 || len(second.Recommendations[0].Numbers) != 6 {
		t.Fatal(second, err)
	}
	streamPostgresIssue(t, db, 2)
	agePlanVisit(t, db, roomID)
	second, err = svc.ActivateStream(ctx, roomID, 2, "three-period-six-codes")
	if err != nil || second.Recommendations[0].CyclePeriod != 2 {
		t.Fatal(second, err)
	}
	first, err := svc.StreamDetail(roomID, 1, DefaultPlanKey)
	if err != nil || len(first.Recommendations) != 0 || len(first.History) != 3 {
		t.Fatal("other stream advanced", first, err)
	}
	if err := db.Model(&plan.StreamPeriod{}).Count(&count).Error; err != nil || count != 3 {
		t.Fatal("redundant periods", count, err)
	}
	if err := db.Model(&plan.Recommendation{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("rich wrote legacy", count, err)
	}
	if _, err := svc.StreamDetail(roomID, 2, "three-period-six-codes"); err != nil {
		t.Fatal(err)
	}
	var readAfter plan.Stream
	if err := db.First(&readAfter, second.Stream.ID).Error; err != nil || !readAfter.ActiveUntil.Equal(*second.Stream.ActiveUntil) {
		t.Fatal("GET renewed lease", err)
	}
}

func TestPlanStreamsPostgresCyclesRetentionAndHighWater(t *testing.T) {
	db, roomID := streamPostgresSetup(t)
	svc := NewPlanContentService(db)
	ctx := context.Background()
	var firstIssue lottery.Issue
	var firstPick PlanRecommendationView
	var last PlanStreamDetail
	for n := 1; n <= 28; n++ {
		issue := streamPostgresIssue(t, db, n)
		if n == 1 {
			firstIssue = issue
		}
		agePlanVisit(t, db, roomID)
		detail, err := svc.ActivateStream(ctx, roomID, 1, DefaultPlanKey)
		if err != nil || len(detail.Recommendations) != 3 {
			t.Fatal(n, detail, err)
		}
		pick := detail.Recommendations[0]
		if n == 1 {
			firstPick = pick
		}
		if n <= 4 && (pick.CycleID != firstPick.CycleID || pick.CyclePeriod != n || !reflect.DeepEqual(pick.Numbers, firstPick.Numbers)) {
			t.Fatal("cycle not reused", n, pick)
		}
		if n == 5 && (pick.CycleID == firstPick.CycleID || pick.CyclePeriod != 1) {
			t.Fatal("cycle not rotated", pick)
		}
		last = detail
	}
	var periods, cycles, issues int64
	if err := db.Model(&plan.StreamPeriod{}).Where("stream_id = ?", last.Stream.ID).Count(&periods).Error; err != nil || periods != 20 {
		t.Fatal("retention unbounded", periods, err)
	}
	if err := db.Model(&plan.StreamCycle{}).Where("stream_id = ?", last.Stream.ID).Count(&cycles).Error; err != nil || cycles != 5 {
		t.Fatal("unused cycles retained", cycles, err)
	}
	if err := db.Model(&lottery.Issue{}).Where("game_id = ?", "speed-racing").Count(&issues).Error; err != nil || issues != 28 {
		t.Fatal("pruning touched real issues", issues, err)
	}
	if len(last.History) != 18 || last.HistoryLimit != 6 {
		t.Fatal("default history not six", len(last.History))
	}
	max, err := svc.StreamDetail(roomID, 1, DefaultPlanKey, 10)
	if err != nil || len(max.History) != 30 {
		t.Fatal("ten-period history", len(max.History), err)
	}
	if capped, err := svc.StreamDetail(roomID, 1, DefaultPlanKey, 999); err != nil || len(capped.History) != 30 {
		t.Fatal("history cap escaped", err)
	}
	// The current cycle is a durable high-water mark even after old period rows are pruned.
	if err := db.Model(&lottery.Game{}).Where("id = ?", "speed-racing").Updates(map[string]any{"next_issue": firstIssue.Issue, "next_draw_at": *firstIssue.ScheduledDrawAt}).Error; err != nil {
		t.Fatal(err)
	}
	agePlanVisit(t, db, roomID)
	regressed, err := svc.ActivateStream(ctx, roomID, 1, DefaultPlanKey)
	if err != nil || len(regressed.Recommendations) != 0 || regressed.LatestRecommendations[0].Issue != "confirmed-stream-28" {
		t.Fatal("old issue revived", regressed, err)
	}
	if err := db.Model(&plan.StreamPeriod{}).Where("issue_id = ?", firstIssue.ID).Count(&periods).Error; err != nil || periods != 0 {
		t.Fatal("pruned receipt resurrected", periods, err)
	}
	// A schedule correction cannot turn an old immutable issue ID into a new
	// publication either. Both issue identity and scheduled time are watermarks.
	later := time.Now().UTC().Add(4 * time.Minute)
	if err := db.Model(&firstIssue).Updates(map[string]any{"scheduled_draw_at": later, "seal_at": later.Add(-30 * time.Second)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&lottery.Game{}).Where("id = ?", "speed-racing").Update("next_draw_at", later).Error; err != nil {
		t.Fatal(err)
	}
	agePlanVisit(t, db, roomID)
	if _, err := svc.ActivateStream(ctx, roomID, 1, DefaultPlanKey); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&plan.StreamPeriod{}).Where("issue_id = ?", firstIssue.ID).Count(&periods).Error; err != nil || periods != 0 {
		t.Fatal("schedule correction revived old issue", periods, err)
	}
	automation := NewPlanAutomationService(db)
	off, on := false, true
	if _, err := automation.Save(roomID, PlanAutomationInput{Enabled: &off}); err != nil {
		t.Fatal(err)
	}
	if _, err := automation.Save(roomID, PlanAutomationInput{Enabled: &on}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ActivateStream(ctx, roomID, 1, DefaultPlanKey); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&plan.StreamPeriod{}).Where("issue_id = ?", firstIssue.ID).Count(&periods).Error; err != nil || periods != 0 {
		t.Fatal("revocation and reopen lost high-water", periods, err)
	}
}

func TestPlanStreamsPostgresQuotaRevocationAndSixtySecondTTL(t *testing.T) {
	db, roomID := streamPostgresSetup(t)
	svc, automation := NewPlanContentService(db), NewPlanAutomationService(db)
	ctx := context.Background()
	var firstID uint64
	keys := []string{"size-five-periods", "size-four-periods"}
	for n := 0; n < 20; n++ {
		result, err := svc.ActivateStream(ctx, roomID, n/2+1, keys[n%2])
		if err != nil {
			t.Fatal("twenty equal slots", n, err)
		}
		if n == 0 {
			firstID = result.Stream.ID
		}
	}
	if _, err := svc.ActivateStream(ctx, roomID, 1, DefaultPlanKey); apperrors.GetErrorCode(err) != "PLAN_STREAM_LIMIT" {
		t.Fatal("default bypassed cap", err)
	}
	if err := db.Model(&plan.Stream{}).Where("id = ?", firstID).UpdateColumn("updated_at", time.Now().UTC().Add(-61*time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	active, err := svc.ActivateStream(ctx, roomID, 1, DefaultPlanKey)
	if err != nil || active.Stream.ActiveCount != 20 {
		t.Fatal("expired slot unavailable", active.Stream, err)
	}
	on := true
	if _, err := automation.Save(roomID, PlanAutomationInput{Enabled: &on, PlanKeys: []string{"parity-five-periods"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := automation.Save(roomID, PlanAutomationInput{Enabled: &on, PlanKeys: defaultPlanKeys()}); err != nil {
		t.Fatal(err)
	}
	view, err := svc.StreamDetail(roomID, 1, DefaultPlanKey)
	if err != nil || view.Stream.Active || view.Stream.ActiveCount != 0 || len(view.History) != 0 {
		t.Fatal("restore revived subscription", view, err)
	}
	var count int64
	if err := db.Model(&plan.Stream{}).Where("revoked = false").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("GET revived default", count, err)
	}
}

func TestPlanStreamsPostgresNoVisitNoProgressAndMissedPeriodInterrupted(t *testing.T) {
	db, roomID := streamPostgresSetup(t)
	svc := NewPlanContentService(db)
	ctx := context.Background()
	streamPostgresIssue(t, db, 1)
	initial, err := svc.ActivateStream(ctx, roomID, 1, DefaultPlanKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&plan.Stream{}).Where("id = ?", initial.Stream.ID).Update("active_until", time.Now().UTC().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	streamPostgresIssue(t, db, 2)
	view, err := svc.StreamDetail(roomID, 1, DefaultPlanKey)
	if err != nil || view.Stream.Active || len(view.History) != 3 || len(view.Recommendations) != 0 {
		t.Fatal("unvisited progressed", view, err)
	}
	streamPostgresIssue(t, db, 3)
	resumed, err := svc.ActivateStream(ctx, roomID, 1, DefaultPlanKey)
	if err != nil || len(resumed.Recommendations) != 3 || resumed.Recommendations[0].CycleID == initial.Recommendations[0].CycleID || resumed.Recommendations[0].CyclePeriod != 1 {
		t.Fatal("missed counted continuous", resumed, err)
	}
	var old plan.StreamCycle
	if err := db.First(&old, initial.Recommendations[0].CycleID).Error; err != nil || old.Status != "interrupted" {
		t.Fatal("cycle not interrupted", old, err)
	}
	var count int64
	if err := db.Model(&plan.StreamPeriod{}).Where("issue = ?", "confirmed-stream-2").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("unvisited backfill", count, err)
	}
	sealed := streamPostgresIssue(t, db, 4)
	if err := db.Model(&sealed).Updates(map[string]any{"status": lottery.IssueStatusSealed, "seal_at": time.Now().UTC().Add(-time.Second)}).Error; err != nil {
		t.Fatal(err)
	}
	agePlanVisit(t, db, roomID)
	if _, err := svc.ActivateStream(ctx, roomID, 1, DefaultPlanKey); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&plan.StreamPeriod{}).Where("issue_id = ?", sealed.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("sealed received picks", count, err)
	}
}
