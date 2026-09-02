package services

import (
	"backend/data/models/lottery"
	"backend/data/models/plan"
	"backend/data/models/settings"
	apperrors "backend/errors"
	"context"
	"fmt"
	"testing"
	"time"
)

// Opt-in, dedicated disposable PostgreSQL only; the shared helper refuses a
// nonempty database and rolls back every migration and fixture after the test.
func TestPlanAutomationOtherGamesPostgresPersistenceAndRoomBoundaries(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "plan_auto_room", "723451")
	other := timingPostgresRoom(t, db, "plan_other_room", "723452")
	svc := NewPlanAutomationService(db)
	visit := func() (PlanAutomationRun, error) {
		if err := db.Model(&plan.Stream{}).Where("workspace_id = ?", room.ID).UpdateColumn("updated_at", time.Now().UTC().Add(-6*time.Second)).Error; err != nil {
			t.Fatal(err)
		}
		return NewPlanContentService(db).touchPlan(context.Background(), room.ID, "speed-fly", 1, singlePeriodPlanKey)
	}
	view, err := svc.Get(room.ID)
	if err != nil || view.Enabled || len(view.GameIDs) != 0 {
		t.Fatalf("default config = %#v, %v", view, err)
	}
	var configs int64
	if err := db.Model(&plan.Automation{}).Count(&configs).Error; err != nil || configs != 0 {
		t.Fatalf("read seeded configuration: count=%d err=%v", configs, err)
	}
	if _, err := visit(); err == nil {
		t.Fatal("unconfigured room generated data")
	}
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "speed-fly", true); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&settings.SystemConfig{}).Where("workspace_id = ?", room.ID).
		Updates(map[string]any{"room_enabled": true, "prediction_enabled": true, "game_settings_json": `{"seal_seconds":30}`}).Error; err != nil {
		t.Fatal(err)
	}
	drawAt := time.Now().UTC().Add(2 * time.Minute)
	if err := db.Model(&lottery.Game{}).Where("id = ?", "speed-fly").Updates(map[string]any{
		"enabled": true, "source_kind": "external", "timing_source": "upstream", "next_issue": "confirmed-plan-500",
		"next_draw_at": drawAt, "draw_interval": 300, "sync_status": "ok", "last_sync_error": "", "lobby_category": "彩票",
	}).Error; err != nil {
		t.Fatal(err)
	}
	issue := lottery.Issue{GameID: "speed-fly", Issue: "confirmed-plan-500", Status: lottery.IssueStatusAccepting,
		SourceMode: "external", AcceptAt: drawAt.Add(-5 * time.Minute), SealAt: drawAt.Add(-30 * time.Second), ScheduledDrawAt: &drawAt}
	if err := db.Create(&issue).Error; err != nil {
		t.Fatal(err)
	}
	enabled := true
	if _, err := svc.Save(room.ID, PlanAutomationInput{Enabled: &enabled, Mode: "demo", GameIDs: []string{"speed-fly", "speed-ssc"}}); err != nil {
		t.Fatal(err)
	}
	assertNoGeneration := func(reason string) {
		t.Helper()
		run, err := visit()
		if (err != nil && apperrors.GetErrorCode(err) != "ROOM_CLOSED") || run.CreatedCount != 0 || run.EligibleGameCount != 0 {
			t.Fatalf("%s generated recommendations: %#v, %v", reason, run, err)
		}
		var receipts int64
		if err := db.Model(&plan.GenerationReceipt{}).Count(&receipts).Error; err != nil || receipts != 0 {
			t.Fatalf("%s consumed generation receipts: %d, %v", reason, receipts, err)
		}
	}
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "speed-fly", false); err != nil {
		t.Fatal(err)
	}
	assertNoGeneration("closed room game")
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "speed-fly", true); err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct {
		field             string
		blocked, restored any
	}{
		{"enabled", false, true}, {"timing_source", "observed", "upstream"}, {"source_kind", "platform", "external"},
	} {
		if err := db.Model(&lottery.Game{}).Where("id = ?", "speed-fly").Update(change.field, change.blocked).Error; err != nil {
			t.Fatal(err)
		}
		assertNoGeneration(change.field)
		if err := db.Model(&lottery.Game{}).Where("id = ?", "speed-fly").Update(change.field, change.restored).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Room timing can close before the shared platform cutoff. It must stay
	// closed even if an operator subsequently loosens the setting for this issue.
	timingPostgresSettings(t, db, room.ID, `{"seal_seconds":240}`)
	assertNoGeneration("earlier room cutoff")
	timingPostgresSettings(t, db, room.ID, `{"seal_seconds":30}`)
	assertNoGeneration("frozen room cutoff")
	issue.ID, issue.Issue = 0, "confirmed-plan-500-open"
	if err := db.Create(&issue).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&lottery.Game{}).Where("id = ?", "speed-fly").Update("next_issue", issue.Issue).Error; err != nil {
		t.Fatal(err)
	}
	run, err := visit()
	if err != nil || run.CreatedCount != 3 || run.EligibleGameCount != 1 || len(run.SkippedGameIDs) != 0 {
		t.Fatalf("first generation = %#v, %v", run, err)
	}
	var rows []plan.Recommendation
	if err := db.Where("workspace_id = ?", room.ID).Find(&rows).Error; err != nil || len(rows) != 3 {
		t.Fatalf("generated rows = %d, %v", len(rows), err)
	}
	for _, row := range rows {
		if row.Source != "demo" || row.Result != plan.ResultPending || row.Note != PlanDemoNotice || len(parsePlanNumbers(row.Numbers)) != 3 || row.Size != "" || row.Parity != "" {
			t.Fatalf("misleading generated row: %#v", row)
		}
	}
	// Simulate a pre-upgrade published row and retain its existing receipt. A
	// renamed template must neither rewrite its picks nor create a second row.
	if err := db.Model(&rows[1]).Updates(map[string]any{
		"master_name": "北斗演示师", "master_title": "程序演示 · 号码样例", "numbers": "2,6,10",
		"note": "程序生成的演示推荐，非真实预测，不保证命中。",
	}).Error; err != nil {
		t.Fatal(err)
	}
	var historicalBefore plan.Recommendation
	if err := db.First(&historicalBefore, rows[1].ID).Error; err != nil {
		t.Fatal(err)
	}
	// A new service instance has no in-memory coordination with the first one.
	second, err := visit()
	if err != nil || second.CreatedCount != 0 {
		t.Fatalf("repeat generated duplicates: %#v, %v", second, err)
	}
	var historicalAfter plan.Recommendation
	if err := db.First(&historicalAfter, rows[1].ID).Error; err != nil {
		t.Fatal(err)
	}
	if historicalAfter.Numbers != historicalBefore.Numbers || historicalAfter.MasterName != historicalBefore.MasterName || historicalAfter.Issue != historicalBefore.Issue ||
		!historicalAfter.CreatedAt.Equal(historicalBefore.CreatedAt) || !historicalAfter.UpdatedAt.Equal(historicalBefore.UpdatedAt) {
		t.Fatalf("template relabeling rewrote a published row: before=%#v after=%#v", historicalBefore, historicalAfter)
	}
	if err := NewPlanContentService(db).Delete(room.ID, rows[0].ID); err != nil {
		t.Fatal(err)
	}
	if run, err := visit(); err != nil || run.CreatedCount != 0 {
		t.Fatalf("deleted recommendation resurrected: %#v, %v", run, err)
	}
	if _, err := NewPlanContentService(db).Update(room.ID, rows[1].ID, PlanRecommendationInput{
		GameID: rows[1].GameID, Issue: rows[1].Issue, MasterName: rows[1].MasterName, Numbers: []int{1, 2, 3, 4, 5}, Result: "hit", Enabled: true,
	}); err == nil {
		t.Fatal("demo recommendation accepted a fake hit")
	}
	if _, err := NewPlanContentService(db).Update(other.ID, rows[1].ID, PlanRecommendationInput{
		GameID: rows[1].GameID, Issue: rows[1].Issue, MasterName: rows[1].MasterName, Numbers: []int{1, 2, 3, 4, 5}, Enabled: true,
	}); err == nil {
		t.Fatal("another room edited the recommendation")
	}
	if catalog, err := NewPlanContentService(db).Catalog(other.ID); err != nil || len(catalog) != 0 {
		t.Fatalf("recommendation leaked to another room: %#v, %v", catalog, err)
	}
	// Closing a period keeps its catalog/history, but it cannot masquerade as current.
	if err := db.Model(&issue).Updates(map[string]any{"status": lottery.IssueStatusSealed, "seal_at": time.Now().UTC().Add(-time.Second)}).Error; err != nil {
		t.Fatal(err)
	}
	if run, err := visit(); err != nil || run.CreatedCount != 0 || run.EligibleGameCount != 0 {
		t.Fatalf("closed issue generated: %#v, %v", run, err)
	}
	catalog, err := NewPlanContentService(db).Catalog(room.ID)
	if err != nil || len(catalog) != 1 || !catalog[0].HistoryOnly || catalog[0].CurrentIssue != "" {
		t.Fatalf("closed issue lost its historical catalog: %#v, %v", catalog, err)
	}
	detail, err := NewPlanContentService(db).Detail(room.ID, "speed-fly")
	if err != nil || len(detail.Recommendations) != 0 || len(detail.History) != 2 || len(detail.LatestRecommendations) != 2 {
		t.Fatalf("closed issue history/current separation failed: %#v, %v", detail, err)
	}
	// Another explicit real issue produces one new row per master; no period is invented.
	next := issue
	next.ID, next.Issue, next.Status, next.SealAt = 0, "confirmed-plan-501", lottery.IssueStatusAccepting, drawAt.Add(-30*time.Second)
	if err := db.Create(&next).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&lottery.Game{}).Where("id = ?", "speed-fly").Update("next_issue", next.Issue).Error; err != nil {
		t.Fatal(err)
	}
	if run, err := visit(); err != nil || run.CreatedCount != 3 {
		t.Fatalf("next real open issue missing: %#v, %v", run, err)
	}
	detail, err = NewPlanContentService(db).Detail(room.ID, "speed-fly")
	if err != nil || len(detail.LatestRecommendations) != 3 || len(detail.History) != 5 {
		t.Fatalf("old/new expert labels duplicated latest cards: %#v error=%v", detail, err)
	}
	for _, item := range detail.LatestRecommendations {
		if len(item.Numbers) != 3 || item.Issue != next.Issue {
			t.Fatalf("latest recommendation did not use new five-number row: %#v", item)
		}
	}
	// Disabling affects the next cycle and must not delete already published history.
	enabled = false
	if _, err := svc.Save(room.ID, PlanAutomationInput{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	if _, err := visit(); err == nil {
		t.Fatal("disabled configuration executed")
	}
	if view, err := svc.Get(room.ID); err != nil || view.Enabled || len(view.GameIDs) != 2 {
		t.Fatalf("disable did not preserve game selection: %#v, %v", view, err)
	}
	// Administration keeps its array contract but never downloads unbounded
	// history after automatic generation has been running for many periods.
	bulk := make([]plan.Recommendation, 301)
	for i := range bulk {
		bulk[i] = plan.Recommendation{WorkspaceID: room.ID, GameID: "speed-fly", Issue: next.Issue,
			MasterName: fmt.Sprintf("手工历史样例%d", i), Numbers: "1,2,3", Source: "manual", Result: "pending", Enabled: true, SortOrder: 100}
	}
	bulk[0].SortOrder = -1
	if err := db.Create(&bulk).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&bulk[0]).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	adminRows, err := NewPlanContentService(db).ListAdmin(room.ID)
	if err != nil || len(adminRows) != 300 || adminRows[0].ID != bulk[0].ID || adminRows[0].Enabled {
		t.Fatalf("bounded admin history lost ordering/disabled content: len=%d, error=%v", len(adminRows), err)
	}
}
