package services

import (
	"backend/data/models/chat"
	"backend/data/models/lottery"
	"backend/data/models/user"
	apperrors "backend/errors"
	"backend/utils"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRoomVisitPostgresMessageBoundaryAndPagination(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "visit_tenant", "76801")
	other := timingPostgresRoom(t, db, "visit_other", "76802")
	member := timingPostgresMember(t, db, room, "visit_member")
	if err := db.Model(&lottery.Game{}).Where("id = ?", "speed-racing").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "speed-racing", true); err != nil {
		t.Fatal(err)
	}
	anchor := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
	insert := func(workspaceID uint64, scope, game string, at time.Time, content string) chat.Message {
		row := chat.Message{WorkspaceID: workspaceID, Scope: scope, RoomScope: scope, RoomType: "group", GameID: game,
			UserID: member.UserID, Username: member.Username, Nickname: member.Nickname, MessageType: "text", CreatedAt: at, Content: content}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		return row
	}
	insert(room.ID, room.Scope, "speed-racing", anchor.Add(-time.Second), "older than entry")
	insert(other.ID, other.Scope, "speed-racing", anchor, "another room")
	insert(room.ID, room.Scope, "speed-fly", anchor, "another game")
	var wanted []uint64
	for index := 0; index < 125; index++ {
		// Tied and out-of-order timestamps must not break the ID cursor.
		at := anchor.Add(time.Duration(index%3) * time.Second)
		row := insert(room.ID, room.Scope, "speed-racing", at, fmt.Sprintf("message-%03d", index))
		wanted = append(wanted, row.ID)
	}
	service := NewMemberChatService(db)
	var cursor uint64
	var seen []uint64
	for pageNumber := 0; pageNumber < 3; pageNumber++ {
		page, err := service.List(member.UserID, "group", "speed-racing", 50, 0, cursor, anchor)
		if err != nil {
			t.Fatal(err)
		}
		if page.HasMore != (pageNumber < 2) {
			t.Fatalf("wrong continuation flag on page %d: %+v", pageNumber, page)
		}
		for _, item := range page.Items {
			seen = append(seen, item.ID)
			cursor = item.ID
		}
	}
	if fmt.Sprint(seen) != fmt.Sprint(wanted) {
		t.Fatalf("message gap/room leak: got %v want %v", seen, wanted)
	}
	newMessage := insert(room.ID, room.Scope, "speed-racing", anchor.Add(10*time.Second), "live push")
	next, err := service.List(member.UserID, "group", "speed-racing", 50, 0, cursor, anchor)
	if err != nil || len(next.Items) != 1 || next.Items[0].ID != newMessage.ID {
		t.Fatalf("incremental read: %+v %v", next, err)
	}
	reentered, err := service.List(member.UserID, "group", "speed-racing", 50, 0, 0, anchor.Add(5*time.Second))
	if err != nil || len(reentered.Items) != 1 || reentered.Items[0].ID != newMessage.ID {
		t.Fatalf("re-entry boundary: %+v %v", reentered, err)
	}
}

func TestManagementAccountPostgresLoginWithoutOwner(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "account_only_tenant", "76811")
	const password = "TimingFixture#2026_a9Z"
	agent, err := NewAgentAdminService(db).CreateForTenant(room.OwnerUserID, CreateAgentInput{
		Username: "account_only_agent", Password: password, Nickname: "账号测试代理", RoomCode: "76812", Status: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	utils.InitJWT("isolated-account-only-test-signing-key", 3600)
	t.Cleanup(func() { utils.InitJWT("", 0) })
	service := NewAuthService(db)
	for _, tc := range []struct{ username, role string }{{"timing_platform", "admin"}, {"account_only_tenant", "tenant"}, {"account_only_agent", "agent"}} {
		account, token, loginErr := service.Login(strings.ToUpper(tc.username), password, "")
		if loginErr != nil || account.Role != tc.role || account.Username != tc.username || token == "" {
			t.Fatalf("owner-free %s login: %+v %v", tc.role, account, loginErr)
		}
		if account.Password != "" {
			t.Fatal("password hash exposed")
		}
		if _, _, loginErr := service.Login(tc.username, "incorrect-password", ""); apperrors.GetErrorCode(loginErr) != "INVALID_CREDENTIALS" {
			t.Fatalf("password bypass: %v", loginErr)
		}
	}
	if err := ensureUsernameInScope(db, "other-scope", "ACCOUNT_ONLY_AGENT", 0); apperrors.GetErrorCode(err) != "USERNAME_EXISTS" {
		t.Fatalf("case-insensitive global duplicate accepted: %v", err)
	}
	if err := db.Model(&user.User{}).Where("user_id = ?", room.OwnerUserID).Update("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, loginErr := service.Login(agent.Username, password, ""); apperrors.GetErrorCode(loginErr) != "USER_DISABLED" {
		t.Fatalf("disabled parent bypassed without owner: %v", loginErr)
	}
}
