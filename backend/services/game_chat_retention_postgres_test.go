package services

import (
	"backend/data/models/application"
	"backend/data/models/bet"
	"backend/data/models/chat"
	"backend/data/models/lifecycle"
	"backend/data/models/lottery"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"gorm.io/gorm"
)

// The shared helper refuses non-loopback/nonempty/non-test databases and rolls
// back the entire schema and fixture. These tests never use config.yaml.
func TestGameChatRetentionPostgresBoundariesAndLifecycle(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "retention_room", "783451")
	other := timingPostgresRoom(t, db, "retention_other", "783452")
	var platform workspacemodel.Workspace
	if err := db.Where("type = ?", workspacemodel.TypePlatform).First(&platform).Error; err != nil {
		t.Fatal(err)
	}
	actor := LifecycleActor{UserID: platform.OwnerUserID, Username: "retention_fixture", WorkspaceID: platform.ID}
	now := time.Now().UTC().Truncate(time.Second)
	svc := NewDataLifecycleService(db)
	svc.now = func() time.Time { return now }
	old := now.AddDate(0, 0, -18)
	create := func(value any) {
		t.Helper()
		if err := db.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	draw := func(issue string, days int, status string) lottery.Draw {
		t.Helper()
		at := now.AddDate(0, 0, -days)
		row := lottery.Draw{GameID: "speed-racing", Issue: issue, Numbers: "1,2,3,4,5,6,7,8,9,10", DrawAt: at}
		create(&row)
		create(&lottery.Issue{GameID: row.GameID, Issue: issue, Status: status, SourceMode: "external", AcceptAt: at.Add(-time.Minute), SealAt: at, DrawAt: &at, SettledAt: &at})
		return row
	}
	older := draw("history-100", 20, "settled")
	pendingDraw := draw("history-101", 19, "settled")
	badDraw := draw("history-102", 18, "settled")
	errorDraw := draw("history-103", 17, "error")
	latest := draw("history-105", 9, "settled")
	imported := draw("history-099", 22, "settled") // highest id, but oldest event
	message := func(owner workspacemodel.Workspace, mutate func(*chat.Message)) chat.Message {
		t.Helper()
		row := chat.Message{WorkspaceID: owner.ID, UserID: owner.OwnerUserID, Username: "fixture", Nickname: "测试", RoomType: "group", Scope: owner.Scope, RoomScope: owner.Scope, GameID: "speed-racing", Content: "保留策略测试", MessageType: "text", CreatedAt: old}
		if mutate != nil {
			mutate(&row)
		}
		create(&row)
		return row
	}
	settlement := func(ref lottery.Draw, kind string) chat.Message {
		return message(room, func(m *chat.Message) {
			m.UserID = 0
			m.Username = "draw_assistant"
			m.MessageType = kind
			m.ReferenceID = ref.ID
		})
	}
	ordinary := message(room, nil)
	settled := settlement(older, "settlement")
	score := settlement(older, "scoreboard")
	backfill := settlement(imported, "settlement")
	protected := []chat.Message{
		message(other, nil), settlement(latest, "settlement"), settlement(latest, "scoreboard"),
		settlement(pendingDraw, "settlement"), settlement(badDraw, "scoreboard"), settlement(errorDraw, "settlement"),
		message(room, func(m *chat.Message) { m.RequestID = "chat-protected-command"; m.Content = "1/2/20" }),
		message(room, func(m *chat.Message) {
			m.UserID = 0
			m.Username = "draw_assistant"
			m.MessageType = "application"
			m.ReferenceID = 100000
		}),
		message(room, func(m *chat.Message) { m.MessageType = "redpacket" }),
		message(room, func(m *chat.Message) { m.MessageType = "welcome"; m.RoomType = "service"; m.GameID = "service" }),
		message(room, func(m *chat.Message) { m.GameID = "lobby" }),
		message(room, func(m *chat.Message) { m.GameID = "legacy" }),
		message(room, func(m *chat.Message) { m.GameID = "not-a-game" }),
		message(room, func(m *chat.Message) { m.CreatedAt = now.AddDate(0, 0, -8) }), // current draw window; old enough by TTL alone
		message(room, func(m *chat.Message) { m.GameID = "speed-fly" }),              // no draw boundary
		message(room, func(m *chat.Message) { m.RedPacketCount = 1 }),
		message(room, func(m *chat.Message) { m.RedPacketTotalCents = 1 }),
		message(room, func(m *chat.Message) { m.RedPacketMinTurnoverCents = 1 }),
		message(room, func(m *chat.Message) { m.RedPacketCover = "legacy" }),
	}
	linked := message(room, nil)
	create(&application.Application{WorkspaceID: room.ID, UserID: room.OwnerUserID, Username: "fixture", AccountType: "member", RequestType: "credit", PaymentType: "manual", RoomScope: room.Scope, GameID: "speed-racing", ChatMessageID: linked.ID, RequestedCents: 100, Status: "pending"})
	packetMessage := message(room, nil)
	create(&chat.RedPacket{WorkspaceID: room.ID, MessageID: packetMessage.ID, Scope: room.Scope, RoomScope: room.Scope, GameID: "speed-racing", FundingUserID: room.OwnerUserID, TotalCents: 100, RefundedCents: 100, PacketCount: 1, Status: "closed", FundingStatus: "refunded", Greeting: "fixture", Cover: "default"})
	protected = append(protected, linked, packetMessage)
	for i, fixture := range []struct {
		draw                   lottery.Draw
		status, reconciliation string
	}{{pendingDraw, "pending", "normal"}, {badDraw, "won", "abnormal"}, {older, "lost", "normal"}} {
		create(&bet.Bet{WorkspaceID: room.ID, RoomScope: room.Scope, GameID: "speed-racing", Issue: fixture.draw.Issue, UserID: room.OwnerUserID, Username: "fixture", PlayCode: "ball_1_5", PlayName: "号码", Position: 1, Selection: fmt.Sprint(i + 1), AmountCents: 2000, Odds: 9.9, Status: fixture.status, ReconciliationStatus: fixture.reconciliation, SettledAt: &old, CreatedAt: old})
	}
	create(&bet.AssistantRequest{WorkspaceID: room.ID, UserID: room.OwnerUserID, RequestID: "chat-protected-command", Status: "completed", ResultJSON: `{"game_id":"speed-racing"}`})
	create(&user.BalanceTransaction{WorkspaceID: room.ID, UserID: room.OwnerUserID, Reference: "retention-fixture", AmountCents: 2000, BeforeCents: 0, AfterCents: 2000, Type: "manual", CreatedAt: old})
	protectedTables := []string{"lottery_bets", "user_balance_transactions", "lottery_assistant_requests", "user_applications", "chat_red_packets", "chat_red_packet_claims", "lottery_draws", "lottery_issues", "user"}
	snapshots := map[string]string{}
	for _, table := range protectedTables {
		snapshots[table] = gameRetentionTableSnapshot(t, db, table)
	}
	policy, _, err := svc.policyForWorkspace(room.ID, lifecycle.ClassGameChatMessages)
	if err != nil || policy.Enabled || policy.RetentionDays != 7 || policy.PurgeAfterDays != 0 {
		t.Fatalf("default policy: %#v %v", policy, err)
	}
	purgeDays := 2
	if _, err := svc.UpdatePolicy(lifecycle.ClassGameChatMessages, UpdateRetentionPolicyInput{WorkspaceID: room.ID, Enabled: true, RetentionDays: 7, PurgeAfterDays: &purgeDays}, actor); err != nil {
		t.Fatal(err)
	}
	kept, err := svc.UpdatePolicy(lifecycle.ClassGameChatMessages, UpdateRetentionPolicyInput{WorkspaceID: room.ID, Enabled: true, RetentionDays: 7}, actor)
	if err != nil || kept.PurgeAfterDays != 2 {
		t.Fatalf("old client overwrote purge setting: %#v %v", kept, err)
	}
	expectedIDs := []uint64{ordinary.ID, settled.ID, score.ID, backfill.ID}
	var eligibleIDs []uint64
	if err := db.Table("member_chat_messages message").Where("message.workspace_id = ? AND "+gameChatLifecyclePredicate, room.ID).Order("message.id ASC").Pluck("message.id", &eligibleIDs).Error; err != nil {
		t.Fatal(err)
	}
	sort.Slice(expectedIDs, func(i, j int) bool { return expectedIDs[i] < expectedIDs[j] })
	if !reflect.DeepEqual(eligibleIDs, expectedIDs) {
		t.Fatalf("candidate boundary: got %v want %v", eligibleIDs, expectedIDs)
	}
	for _, predicate := range []string{genericChatLifecyclePredicate, robotChatLifecyclePredicate} {
		var overlaps int64
		if err := db.Table("member_chat_messages message").Where("message.id IN ? AND "+predicate, expectedIDs).Count(&overlaps).Error; err != nil || overlaps != 0 {
			t.Fatalf("overlapping policies: %d %v", overlaps, err)
		}
	}
	preview := func(requestID, mode string) *CleanupPreview {
		t.Helper()
		result, err := svc.Preview(CleanupPreviewInput{RequestID: requestID, WorkspaceID: &room.ID, DataClasses: []string{lifecycle.ClassGameChatMessages}, BatchLimit: 100, DeleteMode: mode}, actor)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	execute := func(p *CleanupPreview) {
		t.Helper()
		result, err := svc.Execute(CleanupExecuteInput{RequestID: p.RequestID}, actor)
		if err != nil || len(result.Items) != 1 || result.Items[0].AffectedCount != int64(len(expectedIDs)) {
			t.Fatalf("execute: %#v %v", result, err)
		}
	}
	stale := preview("game-retention-stale-001", DeleteModeSoft)
	if err := db.Model(&chat.Message{}).Where("id = ?", ordinary.ID).UpdateColumn("request_id", "chat-newly-protected").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Execute(CleanupExecuteInput{RequestID: stale.RequestID}, actor); err == nil {
		t.Fatal("stale preview deleted a newly protected command")
	}
	var unchanged chat.Message
	if err := db.Unscoped().First(&unchanged, ordinary.ID).Error; err != nil || unchanged.DeletedAt.Valid {
		t.Fatalf("failed preview partially deleted data: %#v %v", unchanged, err)
	}
	if err := db.Model(&chat.Message{}).Where("id = ?", ordinary.ID).UpdateColumn("request_id", "").Error; err != nil {
		t.Fatal(err)
	}
	soft := preview("game-retention-soft-001", DeleteModeSoft)
	if soft.Items[0].PlannedCount != 4 {
		t.Fatalf("soft preview: %#v", soft)
	}
	execute(soft)
	execute(soft) // completed task is idempotent
	summary, err := svc.Summary(actor)
	if err != nil || summary.SoftDeletedGameChatCount != 4 {
		t.Fatalf("recycle summary: %#v %v", summary, err)
	}
	reused, created, err := createRoomSettlementMessage(db, older.ID, room.ID, room.Scope, "speed-racing", "settlement", "must not recreate")
	if err != nil || created || reused.ID != settled.ID || !reused.DeletedAt.Valid {
		t.Fatalf("soft-deleted receipt was rebuilt: %#v created=%v err=%v", reused, created, err)
	}
	restored, err := svc.RestoreSoftDeleted(soft.RequestID, actor)
	if err != nil {
		t.Fatal(err)
	}
	foundGameClass := false
	for _, item := range restored.Items {
		if item.DataClass == lifecycle.ClassGameChatMessages && item.AffectedCount == 4 {
			foundGameClass = true
		}
	}
	if !foundGameClass {
		t.Fatalf("restore lost game classification: %#v", restored)
	}
	second := preview("game-retention-soft-002", DeleteModeSoft)
	execute(second)
	now = now.AddDate(0, 0, 3)
	hard := preview("game-retention-hard-001", DeleteModeHard)
	if hard.Items[0].PlannedCount != 4 || hard.Items[0].RetentionDays != 2 {
		t.Fatalf("hard preview purge delay: %#v", hard)
	}
	execute(hard)
	if _, err := svc.RestoreSoftDeleted(second.RequestID, actor); err == nil {
		t.Fatal("permanently deleted task was restored")
	}
	var remaining int64
	if err := db.Unscoped().Model(&chat.Message{}).Where("id IN ?", expectedIDs).Count(&remaining).Error; err != nil || remaining != 0 {
		t.Fatalf("hard purge left rows: %d %v", remaining, err)
	}
	for _, row := range protected {
		var current chat.Message
		if err := db.Unscoped().First(&current, row.ID).Error; err != nil || current.DeletedAt.Valid {
			t.Fatalf("protected message %d removed: %v", row.ID, err)
		}
	}
	for _, table := range protectedTables {
		if actual := gameRetentionTableSnapshot(t, db, table); actual != snapshots[table] {
			t.Fatalf("cleanup mutated protected table %s", table)
		}
	}
}

func gameRetentionTableSnapshot(t *testing.T, db *gorm.DB, table string) string {
	t.Helper()
	var snapshot string
	// Table names above are a fixed test-only allowlist, never external input.
	if err := db.Raw(`SELECT md5(COALESCE(jsonb_agg(to_jsonb(row) ORDER BY to_jsonb(row)::text)::text, '[]')) FROM "` + table + `" row`).Scan(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	return snapshot
}
