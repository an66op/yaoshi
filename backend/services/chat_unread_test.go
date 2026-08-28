package services

import (
	"backend/data/models/chat"
	"strings"
	"testing"
)

func TestUnreadServiceMessageQueryIsOperatorRoomAndMemberScoped(t *testing.T) {
	db := identityDryRunDB(t)
	var rows []chat.Message
	statement := unreadServiceMessageQuery(db, 71, "agent:9").Find(&rows).Statement
	sql := statement.SQL.String()
	for _, fragment := range []string{
		"cursor.operator_user_id =", "cursor.workspace_id = message.workspace_id",
		"message.room_type =", "message.user_id <> 0", "account.role = 'member'",
		"owning_workspace.scope = message.room_scope", "message.scope = CONCAT('user:', message.user_id)",
		"workspace_robot_profiles", "message.room_scope =",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("unread service query omitted %q: %s", fragment, sql)
		}
	}
	for _, forbidden := range []string{
		"account.workspace_id = message.workspace_id",
		"owning_workspace.id = account.workspace_id",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("unread service query re-derived history from the member's current room via %q: %s", forbidden, sql)
		}
	}
	want := []any{uint64(71), "service", "support", "agent:9"}
	if len(statement.Vars) < len(want) {
		t.Fatalf("unread query vars = %#v", statement.Vars)
	}
	for index := range want {
		if statement.Vars[index] != want[index] {
			t.Fatalf("unread query var %d = %#v, want %#v", index, statement.Vars[index], want[index])
		}
	}
}

func TestServiceConversationAccessRequiresFrozenHistoryOrCurrentMembership(t *testing.T) {
	db := identityDryRunDB(t)
	var allowed bool
	statement := serviceConversationAccessQuery(db, 44, 7, "user:7", "agent:9", "service").Scan(&allowed).Statement
	sql := statement.SQL.String()
	for _, fragment := range []string{
		"message.workspace_id =", "message.scope =", "message.room_scope =", "message.game_id =",
		"workspace_memberships", "membership.workspace_id =", "membership.user_id =", "membership.status = 1",
		"account.workspace_id = membership.workspace_id", "account.role = 'member'", "account.status = 1",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("service access query omitted %q: %s", fragment, sql)
		}
	}
	want := []any{uint64(44), uint64(7), "user:7", "agent:9", "service", uint64(44), uint64(7)}
	if len(statement.Vars) != len(want) {
		t.Fatalf("service access vars = %#v, want %#v", statement.Vars, want)
	}
	for index := range want {
		if statement.Vars[index] != want[index] {
			t.Fatalf("service access var %d = %#v, want %#v", index, statement.Vars[index], want[index])
		}
	}
}

func TestArbitraryRoomWithoutConversationOrMembershipIsRejected(t *testing.T) {
	if err := requireServiceConversationAccess(false); err == nil {
		t.Fatal("arbitrary room without frozen conversation/current membership was allowed")
	}
	if err := requireServiceConversationAccess(true); err != nil {
		t.Fatalf("authorized historical/current room was rejected: %v", err)
	}
}

func TestServiceConversationQueriesKeepOldAndNewRoomsIndependent(t *testing.T) {
	db := identityDryRunDB(t)
	var rows []chat.Message
	oldStatement := serviceConversationMessageQuery(db, 44, "user:7", "agent:9", "service").Find(&rows).Statement
	newStatement := serviceConversationMessageQuery(db, 81, "user:7", "agent:12", "service").Find(&rows).Statement

	oldVars, newVars := oldStatement.Vars, newStatement.Vars
	if len(oldVars) < 5 || oldVars[0] != uint64(44) || oldVars[1] != "user:7" || oldVars[3] != "agent:9" {
		t.Fatalf("old-room conversation key is not frozen: %#v", oldVars)
	}
	if len(newVars) < 5 || newVars[0] != uint64(81) || newVars[1] != "user:7" || newVars[3] != "agent:12" {
		t.Fatalf("new-room conversation key is not independent: %#v", newVars)
	}
	if oldVars[0] == newVars[0] || oldVars[3] == newVars[3] {
		t.Fatalf("room switch collapsed two service conversations: old=%#v new=%#v", oldVars, newVars)
	}
}

func TestServiceConversationMessageQueryUsesCompleteConversationKey(t *testing.T) {
	db := identityDryRunDB(t)
	var rows []chat.Message
	statement := serviceConversationMessageQuery(db, 44, "user:7", "agent:9", "service").Where("id = ?", uint64(103)).Find(&rows).Statement
	sql := statement.SQL.String()
	for _, fragment := range []string{"workspace_id =", "scope =", "room_type =", "room_scope =", "game_id =", "deleted_at IS NULL", "id ="} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("conversation message query omitted %q: %s", fragment, sql)
		}
	}
	want := []any{uint64(44), "user:7", "service", "agent:9", "service", uint64(103)}
	if len(statement.Vars) < len(want) {
		t.Fatalf("conversation query vars = %#v", statement.Vars)
	}
	for index := range want {
		if statement.Vars[index] != want[index] {
			t.Fatalf("conversation query var %d = %#v, want %#v", index, statement.Vars[index], want[index])
		}
	}
}
