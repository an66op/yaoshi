package services

import (
	"backend/data/models/chat"
	"backend/data/models/lifecycle"
	"backend/data/models/lottery"
	workspacemodel "backend/data/models/workspace"
	"context"
	"testing"
	"time"
)

// Uses the disposable, loopback-only fixture, never the developer's business DB.
func TestGameChatSchedulerPostgresOptInBatchingAndIdleRooms(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "chat_scheduler", "782651")
	other := timingPostgresRoom(t, db, "chat_scheduler_off", "782652")
	var platform workspacemodel.Workspace
	if err := db.Where("type = ?", workspacemodel.TypePlatform).First(&platform).Error; err != nil {
		t.Fatal(err)
	}
	actor := LifecycleActor{UserID: platform.OwnerUserID, Username: "scheduler_fixture", WorkspaceID: platform.ID}
	now := time.Now().UTC().Truncate(time.Second)
	draw := lottery.Draw{GameID: "speed-racing", Issue: "scheduler-fixture-1", Numbers: "1,2,3,4,5,6,7,8,9,10", DrawAt: now.Add(-time.Minute)}
	if err := db.Create(&draw).Error; err != nil {
		t.Fatal(err)
	}
	createMessage := func(workspace workspacemodel.Workspace, command bool) chat.Message {
		t.Helper()
		row := chat.Message{WorkspaceID: workspace.ID, UserID: workspace.OwnerUserID, Username: "fixture", Nickname: "测试", RoomType: "group", Scope: workspace.Scope, RoomScope: workspace.Scope, GameID: "speed-racing", Content: "普通聊天", MessageType: "text", CreatedAt: now.AddDate(0, 0, -18)}
		if command {
			row.RequestID = "chat-scheduler-protected-command"
			row.Content = "1/2/20"
		}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		return row
	}
	var ids []uint64
	for i := 0; i < 5; i++ {
		ids = append(ids, createMessage(room, false).ID)
	}
	protected := createMessage(room, true)
	otherMessage := createMessage(other, false)
	assertCounts := func(live, total, runs int64) {
		t.Helper()
		for _, check := range []struct {
			name  string
			query string
			want  int64
			args  []any
		}{
			{"live", "SELECT COUNT(*) FROM member_chat_messages WHERE id IN ? AND deleted_at IS NULL", live, []any{ids}},
			{"total", "SELECT COUNT(*) FROM member_chat_messages WHERE id IN ?", total, []any{ids}},
			{"runs", "SELECT COUNT(*) FROM data_cleanup_runs", runs, nil},
		} {
			var got int64
			if err := db.Raw(check.query, check.args...).Scan(&got).Error; err != nil || got != check.want {
				t.Fatalf("%s got=%d want=%d error=%v", check.name, got, check.want, err)
			}
		}
	}
	runAt := func(at time.Time) {
		t.Helper()
		if err := runScheduledGameChatLifecycle(context.Background(), db, at); err != nil {
			t.Fatal(err)
		}
	}
	runAt(now) // New policies are disabled even when old content exists.
	assertCounts(5, 5, 0)
	service := NewDataLifecycleService(db)
	setPolicy := func(workspaceID uint64, enabled bool, purgeDays int) {
		t.Helper()
		if _, err := service.UpdatePolicy(lifecycle.ClassGameChatMessages, UpdateRetentionPolicyInput{WorkspaceID: workspaceID, Enabled: enabled, RetentionDays: 7, PurgeAfterDays: &purgeDays}, actor); err != nil {
			t.Fatal(err)
		}
	}
	setPolicy(0, true, 0)         // Inherit the explicitly enabled platform default.
	setPolicy(other.ID, false, 0) // A room override must still opt out.
	batch := func(mode, id string, want int64) {
		t.Helper()
		got, err := executeGameChatCleanupBatch(db, room.ID, mode, id, 2, actor)
		if err != nil || got != want {
			t.Fatalf("batch %s got=%d want=%d error=%v", id, got, want, err)
		}
	}
	batch(DeleteModeSoft, "scheduler-bounded-batch", 2)
	assertCounts(3, 5, 1)
	batch(DeleteModeSoft, "scheduler-bounded-batch", 2) // Replay with backlog must not consume another batch.
	assertCounts(3, 5, 1)
	runAt(now)
	assertCounts(0, 5, 2)
	runAt(now)
	runAt(now.Add(time.Hour))
	assertCounts(0, 5, 2) // Idle rooms must not accumulate empty cleanup tasks.
	if err := db.Unscoped().Model(&chat.Message{}).Where("id IN ?", ids).UpdateColumn("deleted_at", now.AddDate(0, 0, -3)).Error; err != nil {
		t.Fatal(err)
	}
	batch(DeleteModeHard, "scheduler-not-authorized", 0)
	assertCounts(0, 5, 2) // Soft retention never implies automatic permanent deletion.
	setPolicy(room.ID, true, 2)
	setPolicy(room.ID, false, 2)
	batch(DeleteModeHard, "scheduler-disabled-later", 0) // A queued batch must re-read the disabled policy.
	assertCounts(0, 5, 2)
	setPolicy(room.ID, true, 2)
	runAt(now.Add(2 * time.Hour))
	assertCounts(0, 0, 3)
	runAt(now.Add(3 * time.Hour))
	assertCounts(0, 0, 3)
	for _, id := range []uint64{protected.ID, otherMessage.ID} {
		var row chat.Message
		if err := db.Unscoped().First(&row, id).Error; err != nil || row.DeletedAt.Valid {
			t.Fatalf("protected/disabled-room message %d deleted: %v", id, err)
		}
	}
}
