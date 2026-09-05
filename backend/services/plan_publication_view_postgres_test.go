package services

import (
	"backend/data/models/lottery"
	"backend/data/models/plan"
	"backend/data/models/settings"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm/clause"
)

func TestGenericPlanPostgresFirstActivationPublishesBeforeRecordingView(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "plan_first_activation", "735004")
	member := timingPostgresMember(t, db, room, "plan_first_activation_member")
	membership := workspacemodel.Membership{WorkspaceID: room.ID, UserID: member.UserID, Role: "member", Status: 1, OddsMultiplier: 1}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&membership).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "speed-ssc", true); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&settings.SystemConfig{}).Where("workspace_id = ?", room.ID).Updates(map[string]any{
		"room_enabled": true, "prediction_enabled": true, "game_settings_json": `{"seal_seconds":30}`,
	}).Error; err != nil {
		t.Fatal(err)
	}
	drawAt := time.Now().UTC().Add(3 * time.Minute)
	issue := lottery.Issue{GameID: "speed-ssc", Issue: "plan-first-activation-1", SourceMode: "external", Status: lottery.IssueStatusAccepting, AcceptAt: drawAt.Add(-5 * time.Minute), SealAt: drawAt.Add(-30 * time.Second), ScheduledDrawAt: &drawAt}
	if err := db.Create(&issue).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&lottery.Game{}).Where("id = ?", issue.GameID).Updates(map[string]any{
		"enabled": true, "source_kind": "external", "timing_source": "upstream", "sync_status": "ok", "last_sync_error": "",
		"next_issue": issue.Issue, "next_draw_at": drawAt, "lobby_category": "彩票", "draw_interval": 300,
	}).Error; err != nil {
		t.Fatal(err)
	}
	manual := plan.Recommendation{WorkspaceID: room.ID, GameID: issue.GameID, Issue: issue.Issue, MasterName: "人工发布专家", MasterTitle: "人工发布", MasterColor: "#2aa9b3", Numbers: "1,3,5", Result: plan.ResultPending, Source: "manual", Enabled: true, SortOrder: 5}
	if err := db.Create(&manual).Error; err != nil {
		t.Fatal(err)
	}
	enabled := true
	if _, err := NewPlanAutomationService(db).Save(room.ID, PlanAutomationInput{Enabled: &enabled, GameIDs: []string{issue.GameID}}); err != nil {
		t.Fatal(err)
	}

	detail, err := NewPlanContentService(db).ActivateGameForMember(context.Background(), member.UserID, room.ID, issue.GameID)
	if err != nil {
		t.Fatalf("first activation was blocked by its own view receipt: %v", err)
	}
	if len(detail.Recommendations) != len(planDemoMasters)+1 {
		t.Fatalf("first activation returned %d recommendations, want complete manual+automatic publication", len(detail.Recommendations))
	}
	var automaticRows int64
	if err := db.Model(&plan.Recommendation{}).Where("workspace_id = ? AND game_id = ? AND issue = ? AND source = ?", room.ID, issue.GameID, issue.Issue, "demo").Count(&automaticRows).Error; err != nil || automaticRows != int64(len(planDemoMasters)) {
		t.Fatalf("first activation persisted %d automatic rows, want %d: %v", automaticRows, len(planDemoMasters), err)
	}
	var views int64
	if err := db.Model(&plan.PublicationView{}).Where("workspace_id = ? AND user_id = ? AND game_id = ? AND issue = ? AND position = 0", room.ID, member.UserID, issue.GameID, issue.Issue).Count(&views).Error; err != nil || views != 1 {
		t.Fatalf("complete publication did not receive exactly one view receipt: count=%d err=%v", views, err)
	}
}

func TestGenericPlanPostgresAuditsEveryIssueReturnedInHistory(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "plan_history_audit", "735005")
	member := timingPostgresMember(t, db, room, "plan_history_audit_member")
	membership := workspacemodel.Membership{WorkspaceID: room.ID, UserID: member.UserID, Role: "member", Status: 1, OddsMultiplier: 1}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&membership).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for index := 1; index <= 6; index++ {
		drawAt := now.Add(time.Duration(10+index) * time.Minute)
		issue := lottery.Issue{GameID: "speed-ssc", Issue: fmt.Sprintf("plan-history-audit-%d", index), SourceMode: "external", Status: lottery.IssueStatusAccepting, AcceptAt: now.Add(-time.Minute), SealAt: drawAt.Add(-30 * time.Second), ScheduledDrawAt: &drawAt}
		if err := db.Create(&issue).Error; err != nil {
			t.Fatal(err)
		}
		row := plan.Recommendation{WorkspaceID: room.ID, GameID: issue.GameID, Issue: issue.Issue, MasterName: "人工发布专家", MasterTitle: "人工发布", MasterColor: "#2aa9b3", Numbers: "1,3,5", Result: plan.ResultPending, Source: "manual", Enabled: true, SortOrder: 10}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	detail, err := NewPlanContentService(db).ActivateGameForMember(context.Background(), member.UserID, room.ID, "speed-ssc", 6)
	if err != nil || len(detail.History) != 6 {
		t.Fatalf("six-period response = %+v, error=%v", detail, err)
	}
	var receipts []plan.PublicationView
	if err := db.Where("workspace_id = ? AND user_id = ? AND game_id = ? AND position = 0", room.ID, member.UserID, "speed-ssc").Find(&receipts).Error; err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 6 || len(visiblePlanIssues(detail)) != 6 {
		t.Fatalf("six returned issues produced %d receipts: %+v", len(receipts), receipts)
	}
}

func TestPlanPublicationViewPostgresIsIdempotentAndLocksGenericPublication(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "plan_view_generic", "735001")
	member := timingPostgresMember(t, db, room, "plan_view_generic_member")
	membership := workspacemodel.Membership{WorkspaceID: room.ID, UserID: member.UserID, Role: "member", Status: 1, OddsMultiplier: 1}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&membership).Error; err != nil {
		t.Fatal(err)
	}

	drawAt := time.Now().UTC().Add(3 * time.Minute)
	issue := lottery.Issue{GameID: "speed-ssc", Issue: "plan-view-generic-1", SourceMode: "external", Status: lottery.IssueStatusAccepting, AcceptAt: drawAt.Add(-5 * time.Minute), SealAt: drawAt.Add(-30 * time.Second), ScheduledDrawAt: &drawAt}
	if err := db.Create(&issue).Error; err != nil {
		t.Fatal(err)
	}
	recommendation := plan.Recommendation{WorkspaceID: room.ID, GameID: issue.GameID, Issue: issue.Issue, MasterName: "真实发布专家", MasterTitle: "人工发布", MasterColor: "#2aa9b3", Numbers: "1,3,5", Result: plan.ResultPending, Source: "manual", Enabled: true, SortOrder: 10}
	if err := db.Create(&recommendation).Error; err != nil {
		t.Fatal(err)
	}

	service := NewPlanContentService(db)
	first, err := service.ActivateGameForMember(context.Background(), member.UserID, room.ID, issue.GameID)
	if err != nil || len(first.Recommendations) != 1 || first.AutomationEnabled {
		t.Fatalf("read-only member visit = %+v, error=%v", first, err)
	}
	var viewed plan.PublicationView
	if err := db.Where("workspace_id = ? AND user_id = ? AND game_id = ? AND issue = ?", room.ID, member.UserID, issue.GameID, issue.Issue).First(&viewed).Error; err != nil {
		t.Fatal("first view was not persisted:", err)
	}
	if viewed.Position != 0 || viewed.PlanKey != singlePeriodPlanKey || viewed.ViewedAt.IsZero() {
		t.Fatalf("generic view identity is incomplete: %+v", viewed)
	}
	if _, err := service.ActivateGameForMember(context.Background(), member.UserID, room.ID, issue.GameID); err != nil {
		t.Fatal(err)
	}
	var views int64
	var unchanged plan.PublicationView
	if err := db.Model(&plan.PublicationView{}).Where("workspace_id = ? AND user_id = ? AND game_id = ? AND issue = ?", room.ID, member.UserID, issue.GameID, issue.Issue).Count(&views).Error; err != nil || views != 1 {
		t.Fatalf("refresh created %d view rows: %v", views, err)
	}
	if err := db.First(&unchanged, viewed.ID).Error; err != nil || !unchanged.ViewedAt.Equal(viewed.ViewedAt) {
		t.Fatalf("refresh rewrote first viewed_at: before=%v after=%v err=%v", viewed.ViewedAt, unchanged.ViewedAt, err)
	}
	blockedInput := PlanRecommendationInput{GameID: issue.GameID, Issue: issue.Issue, MasterName: "查看后新增专家", MasterTitle: "人工发布", MasterColor: "#2aa9b3", Numbers: []int{2, 4, 6}, Result: plan.ResultPending, Enabled: true, SortOrder: 30}
	if _, err := service.Create(room.ID, blockedInput); apperrors.GetErrorCode(err) != "PLAN_PUBLICATION_LOCKED" {
		t.Fatalf("service allowed a new recommendation after the issue was viewed: %v", err)
	}
	if err := db.Exec("SAVEPOINT plan_insert_guard").Error; err != nil {
		t.Fatal(err)
	}
	directInsert := plan.Recommendation{WorkspaceID: room.ID, GameID: issue.GameID, Issue: issue.Issue, MasterName: "绕过服务新增", MasterTitle: "人工发布", MasterColor: "#2aa9b3", Numbers: "2,4,6", Result: plan.ResultPending, Source: "manual", Enabled: true, SortOrder: 40}
	if err := db.Create(&directInsert).Error; err == nil || !strings.Contains(strings.ToLower(err.Error()), "plan publication insert is locked") {
		t.Fatalf("database allowed a viewed publication insert: %v", err)
	}
	if err := db.Exec("ROLLBACK TO SAVEPOINT plan_insert_guard").Error; err != nil {
		t.Fatal("recover insert guard savepoint:", err)
	}
	if err := db.Exec("SAVEPOINT plan_view_guard").Error; err != nil {
		t.Fatal(err)
	}
	if mutation := db.Model(&plan.PublicationView{}).Where("id = ?", viewed.ID).Update("viewed_at", time.Now().UTC().Add(time.Hour)); mutation.Error == nil {
		t.Fatal("view receipt remained editable")
	}
	if err := db.Exec("ROLLBACK TO SAVEPOINT plan_view_guard").Error; err != nil {
		t.Fatal("recover view guard savepoint:", err)
	}
	otherIssue := lottery.Issue{GameID: issue.GameID, Issue: "plan-view-unlocked-2", SourceMode: "external", Status: lottery.IssueStatusAccepting, AcceptAt: issue.AcceptAt, SealAt: issue.SealAt, ScheduledDrawAt: &drawAt}
	if err := db.Create(&otherIssue).Error; err != nil {
		t.Fatal(err)
	}
	unlocked := plan.Recommendation{WorkspaceID: room.ID, GameID: issue.GameID, Issue: otherIssue.Issue, MasterName: "未查看期专家", MasterTitle: "人工发布", MasterColor: "#2aa9b3", Numbers: "2,4,6", Result: plan.ResultPending, Source: "manual", Enabled: true, SortOrder: 50}
	if err := db.Create(&unlocked).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("SAVEPOINT plan_identity_guard").Error; err != nil {
		t.Fatal(err)
	}
	if mutation := db.Model(&unlocked).Update("issue", issue.Issue); mutation.Error == nil || !strings.Contains(strings.ToLower(mutation.Error.Error()), "plan publication identity is immutable") {
		t.Fatalf("database allowed an unlocked row to move into a viewed publication: %v", mutation.Error)
	}
	if err := db.Exec("ROLLBACK TO SAVEPOINT plan_identity_guard").Error; err != nil {
		t.Fatal("recover identity guard savepoint:", err)
	}
	var unchangedIdentity plan.Recommendation
	if err := db.First(&unchangedIdentity, unlocked.ID).Error; err != nil || unchangedIdentity.Issue != otherIssue.Issue {
		t.Fatalf("failed identity mutation changed the stored publication: row=%+v err=%v", unchangedIdentity, err)
	}

	input := PlanRecommendationInput{GameID: issue.GameID, Issue: issue.Issue, MasterName: recommendation.MasterName, MasterTitle: recommendation.MasterTitle, MasterColor: recommendation.MasterColor, Numbers: []int{1, 3, 5}, Result: plan.ResultPending, Enabled: true, SortOrder: 20}
	if _, err := service.Update(room.ID, recommendation.ID, input); apperrors.GetErrorCode(err) != "PLAN_PUBLICATION_LOCKED" {
		t.Fatalf("viewed publication update was not rejected: %v", err)
	}
	if err := service.Delete(room.ID, recommendation.ID); apperrors.GetErrorCode(err) != "PLAN_PUBLICATION_LOCKED" {
		t.Fatalf("viewed publication delete was not rejected: %v", err)
	}
}

func TestManualPlanPostgresUsesEarlierRoomCutoffForCreateUpdateDelete(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "plan_room_cutoff", "735003")
	now := time.Now().UTC()
	drawAt := now.Add(3 * time.Minute)
	globalSeal := now.Add(2 * time.Minute)
	issue := lottery.Issue{GameID: "speed-ssc", Issue: "plan-room-cutoff-1", SourceMode: "external", Status: lottery.IssueStatusAccepting, AcceptAt: now.Add(-time.Minute), SealAt: globalSeal, ScheduledDrawAt: &drawAt}
	if err := db.Create(&issue).Error; err != nil {
		t.Fatal(err)
	}
	existing := plan.Recommendation{WorkspaceID: room.ID, GameID: issue.GameID, Issue: issue.Issue, MasterName: "封盘前发布", MasterTitle: "人工发布", MasterColor: "#2aa9b3", Numbers: "1,3,5", Result: plan.ResultPending, Source: "manual", Enabled: true, SortOrder: 10}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}
	roomSeal := now.Add(-time.Second)
	window := lottery.IssueWindow{WorkspaceID: room.ID, GameID: issue.GameID, Issue: issue.Issue, AcceptAt: now.Add(-5 * time.Minute), SealAt: roomSeal, ScheduledDrawAt: drawAt, DrawInterval: 300, SealSeconds: int(drawAt.Sub(roomSeal) / time.Second)}
	if err := db.Create(&window).Error; err != nil {
		t.Fatal(err)
	}

	service := NewPlanContentService(db)
	input := PlanRecommendationInput{GameID: issue.GameID, Issue: issue.Issue, MasterName: "房间封盘后新增", MasterTitle: "人工发布", MasterColor: "#2aa9b3", Numbers: []int{2, 4, 6}, Result: plan.ResultPending, Enabled: true, SortOrder: 20}
	if _, err := service.Create(room.ID, input); apperrors.GetErrorCode(err) != "PLAN_PUBLICATION_CLOSED" {
		t.Fatalf("room cutoff did not close create: %v", err)
	}
	input.MasterName = existing.MasterName
	if _, err := service.Update(room.ID, existing.ID, input); apperrors.GetErrorCode(err) != "PLAN_PUBLICATION_CLOSED" {
		t.Fatalf("room cutoff did not close update: %v", err)
	}
	if err := service.Delete(room.ID, existing.ID); apperrors.GetErrorCode(err) != "PLAN_PUBLICATION_CLOSED" {
		t.Fatalf("room cutoff did not close delete: %v", err)
	}
	direct := plan.Recommendation{WorkspaceID: room.ID, GameID: issue.GameID, Issue: issue.Issue, MasterName: "直接写入", MasterTitle: "人工发布", MasterColor: "#2aa9b3", Numbers: "2,4,6", Result: plan.ResultPending, Source: "manual", Enabled: true, SortOrder: 30}
	if err := db.Create(&direct).Error; err == nil || !strings.Contains(strings.ToLower(err.Error()), "plan publication insert is locked") {
		t.Fatalf("database insert guard ignored room cutoff: %v", err)
	}
}

func TestPlanPublicationViewPostgresRecordsRichIdentityAndFreezesPayload(t *testing.T) {
	db, roomID := streamPostgresSetup(t)
	var room workspacemodel.Workspace
	if err := db.First(&room, roomID).Error; err != nil {
		t.Fatal(err)
	}
	member := timingPostgresMember(t, db, room, "plan_view_rich_member")
	membership := workspacemodel.Membership{WorkspaceID: room.ID, UserID: member.UserID, Role: "member", Status: 1, OddsMultiplier: 1}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&membership).Error; err != nil {
		t.Fatal(err)
	}
	service := NewPlanContentService(db)
	for index := 1; index <= 6; index++ {
		streamPostgresIssue(t, db, index)
		if index > 1 {
			agePlanVisit(t, db, roomID)
		}
		if _, err := service.ActivateStream(context.Background(), roomID, 10, "two-period-eight-codes"); err != nil {
			t.Fatalf("prepare rich history period %d: %v", index, err)
		}
	}
	detail, err := service.ActivateStreamForMember(context.Background(), member.UserID, roomID, "speed-racing", 10, "two-period-eight-codes")
	if err != nil || len(detail.Recommendations) != len(planDemoMasters) || len(detail.History) != 6*len(planDemoMasters) {
		t.Fatalf("rich member visit = %+v, error=%v", detail, err)
	}
	var viewed []plan.PublicationView
	if err := db.Where("workspace_id = ? AND user_id = ? AND game_id = ?", roomID, member.UserID, "speed-racing").Order("issue").Find(&viewed).Error; err != nil {
		t.Fatal(err)
	}
	expectedIssues := visiblePlanIssues(detail.PlanDetail)
	if len(viewed) != len(expectedIssues) || len(viewed) != 6 {
		t.Fatalf("rich response issues were not fully audited: views=%+v expected=%+v", viewed, expectedIssues)
	}
	expected := make(map[string]bool, len(expectedIssues))
	for _, issue := range expectedIssues {
		expected[issue] = true
	}
	for _, receipt := range viewed {
		if !expected[receipt.Issue] || receipt.Position != 10 || receipt.PlanKey != "two-period-eight-codes" {
			t.Fatalf("rich view identity does not match rendered stream: view=%+v", receipt)
		}
	}
	if _, err := service.ActivateStreamForMember(context.Background(), member.UserID, roomID, "speed-racing", 10, "two-period-eight-codes"); err != nil {
		t.Fatal(err)
	}
	var views int64
	if err := db.Model(&plan.PublicationView{}).Where("workspace_id = ? AND user_id = ? AND game_id = ?", roomID, member.UserID, "speed-racing").Count(&views).Error; err != nil || views != 6 {
		t.Fatalf("rich refresh created %d view rows: %v", views, err)
	}
	disabled := false
	var periodsBefore int64
	if err := db.Model(&plan.StreamPeriod{}).Count(&periodsBefore).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := NewPlanAutomationService(db).Save(roomID, PlanAutomationInput{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	catalog, err := service.Catalog(roomID)
	if err != nil || len(catalog) != 1 || catalog[0].GameID != "speed-racing" || !catalog[0].HistoryOnly || catalog[0].CurrentIssue != "" || catalog[0].LatestIssue == "" {
		t.Fatalf("disabled rich history disappeared from catalog: catalog=%+v error=%v", catalog, err)
	}
	archived, err := service.ActivateStreamForMember(context.Background(), member.UserID, roomID, "speed-racing", 10, "two-period-eight-codes")
	if err != nil || archived.AutomationEnabled || !archived.Stream.Allowed || len(archived.History) == 0 {
		t.Fatalf("disabled generation hid audited rich history: detail=%+v error=%v", archived, err)
	}
	if err := db.Model(&plan.PublicationView{}).Where("workspace_id = ? AND user_id = ? AND game_id = ?", roomID, member.UserID, "speed-racing").Count(&views).Error; err != nil || views != 6 {
		t.Fatalf("disabled refresh changed idempotent view count to %d: %v", views, err)
	}
	var periodsAfter int64
	if err := db.Model(&plan.StreamPeriod{}).Count(&periodsAfter).Error; err != nil || periodsAfter != periodsBefore {
		t.Fatalf("disabled read generated periods: before=%d after=%d error=%v", periodsBefore, periodsAfter, err)
	}

	if err := db.Exec("SAVEPOINT plan_payload_guard").Error; err != nil {
		t.Fatal(err)
	}
	mutation := db.Model(&plan.StreamCycle{}).Where("id = ?", detail.Recommendations[0].CycleID).Update("payload_json", "[]")
	if mutation.Error == nil {
		t.Fatal("published rich payload remained editable")
	}
	if err := db.Exec("ROLLBACK TO SAVEPOINT plan_payload_guard").Error; err != nil {
		t.Fatal("recover payload guard savepoint:", err)
	}
}

func TestGenericPlanPostgresUsesOnlyTrustedRevisionAndIgnoresStoredResult(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "plan_result_trust", "735002")
	now := time.Now().UTC()
	sourceRevision, conversionRevision, versioned := trustedDrawRevision("speed-ssc")
	if !versioned {
		t.Fatal("speed-ssc test fixture unexpectedly has no trusted draw contract")
	}

	fixtures := []struct {
		issue, master, numbers, stored, source, conversion, want string
		roomSealAfterGlobal                                      time.Duration
	}{
		{issue: "plan-trusted-1", master: "可信样本", numbers: "7,1,3,2,0", stored: plan.ResultMiss, source: sourceRevision, conversion: conversionRevision, want: plan.ResultHit},
		{issue: "plan-untrusted-1", master: "非可信样本", numbers: "8,1,3,2,0", stored: plan.ResultHit, source: "untrusted-revision", conversion: conversionRevision, want: plan.ResultPending},
		{issue: "plan-room-late-1", master: "房间封盘后样本", numbers: "7,1,3,2,0", stored: plan.ResultHit, source: sourceRevision, conversion: conversionRevision, want: plan.ResultPending, roomSealAfterGlobal: -2 * time.Minute},
	}
	for index, fixture := range fixtures {
		drawAt := now.Add(time.Duration(-3-index) * time.Minute)
		sealAt := drawAt.Add(-time.Minute)
		acceptAt := sealAt.Add(-5 * time.Minute)
		issue := lottery.Issue{GameID: "speed-ssc", Issue: fixture.issue, Status: lottery.IssueStatusSettled, SourceMode: "external", AcceptAt: acceptAt, SealAt: sealAt, ScheduledDrawAt: &drawAt, DrawAt: &drawAt, SettledAt: &drawAt}
		publicationAt := sealAt.Add(-time.Minute)
		recommendation := plan.Recommendation{WorkspaceID: room.ID, GameID: issue.GameID, Issue: issue.Issue, MasterName: fixture.master, MasterTitle: "真实发布", MasterColor: "#2aa9b3", Numbers: "1,3,7", Result: fixture.stored, Source: "manual", Enabled: true, SortOrder: 10, CreatedAt: publicationAt, UpdatedAt: publicationAt}
		if err := db.Create(&recommendation).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&issue).Error; err != nil {
			t.Fatal(err)
		}
		if fixture.roomSealAfterGlobal != 0 {
			roomSeal := sealAt.Add(fixture.roomSealAfterGlobal)
			window := lottery.IssueWindow{WorkspaceID: room.ID, GameID: issue.GameID, Issue: issue.Issue, AcceptAt: acceptAt, SealAt: roomSeal, ScheduledDrawAt: drawAt, DrawInterval: 300, SealSeconds: int(drawAt.Sub(roomSeal) / time.Second)}
			if err := db.Create(&window).Error; err != nil {
				t.Fatal(err)
			}
		}
		draw := lottery.Draw{GameID: issue.GameID, Issue: issue.Issue, Numbers: fixture.numbers, SourceRevision: fixture.source, ConversionRevision: fixture.conversion, DrawAt: drawAt}
		if err := db.Create(&draw).Error; err != nil {
			t.Fatal(err)
		}
	}

	detail, err := NewPlanContentService(db).Detail(room.ID, "speed-ssc")
	if err != nil || len(detail.History) != len(fixtures) {
		t.Fatalf("generic trusted result detail=%+v error=%v", detail, err)
	}
	byIssue := make(map[string]PlanRecommendationView, len(detail.History))
	for _, row := range detail.History {
		byIssue[row.Issue] = row
	}
	for _, fixture := range fixtures {
		row := byIssue[fixture.issue]
		if row.Result != fixture.want {
			t.Fatalf("%s derived result=%s want=%s (stored=%s)", fixture.issue, row.Result, fixture.want, fixture.stored)
		}
		wantSamples := 0
		if fixture.want != plan.ResultPending {
			wantSamples = 1
		}
		if row.MasterSampleCount != wantSamples || (wantSamples == 0 && row.MasterHitRate != nil) || (wantSamples == 1 && (row.MasterHitRate == nil || *row.MasterHitRate != 100)) {
			t.Fatalf("%s statistics are not evidence-backed: %+v", fixture.issue, row)
		}
	}
}
