package services

import (
	"backend/data/models/user"
	apperrors "backend/errors"
	"encoding/json"
	"strings"
	"testing"
)

func TestWorkspaceMembersRequireExplicitWorkspace(t *testing.T) {
	// A missing room must fail before touching the database. It must never mean
	// the global member directory, even if a caller supplies another filter.
	result, err := NewWorkspaceMemberService(nil).List(0, UserListFilter{WorkspaceID: 99, Kind: "account"})
	if err == nil || !apperrors.IsBusinessError(err) || result != nil {
		t.Fatalf("missing room returned result=%+v, error=%v", result, err)
	}
}

func TestWorkspaceHumanMemberQueryUsesJoinedHistoryWithoutRobotOrRoomLeaks(t *testing.T) {
	db := memberRoomDryRunDB(t)
	var rows []user.User
	statement := WorkspaceHumanMemberQuery(db, 42).Find(&rows).Statement
	sql := strings.ToLower(statement.SQL.String())
	for _, fragment := range []string{
		`"user".role =`, `coalesce("user".remark`, `not exists`, `workspace_robot_profiles`,
		`roster_robot.user_id = "user".user_id`, `"user".workspace_id =`, `or exists`,
		`workspace_memberships`, `roster_membership.user_id = "user".user_id`,
		`roster_membership.workspace_id =`, `roster_membership.role =`, `roster_membership.status in (0, 1)`,
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("roster query omitted %q: %s", fragment, sql)
		}
	}
	for _, forbidden := range []string{"roster_robot.workspace_id", "roster_robot.enabled", "roster_robot.status", "join workspace_memberships", "user_applications", "parent_tenant_id", "parent_agent_id"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("roster scope depends on unsafe %q: %s", forbidden, sql)
		}
	}
	want := []any{"member", "测试机器人专用账号%", uint64(42), uint64(42), "member"}
	if len(statement.Vars) != len(want) {
		t.Fatalf("query variables = %#v, want %#v", statement.Vars, want)
	}
	for index, value := range want {
		if statement.Vars[index] != value {
			t.Fatalf("query variable %d = %#v, want %#v", index, statement.Vars[index], value)
		}
	}
	zero := WorkspaceHumanMemberQuery(db, 0).Find(&rows).Statement.SQL.String()
	if !strings.Contains(zero, "1 = 0") {
		t.Fatalf("zero workspace query can become a global roster: %s", zero)
	}
}

func TestWorkspaceMemberSerializationIsRoomSafe(t *testing.T) {
	item := WorkspaceMember{ID: 7, PublicID: 1234567, Username: "former_member", Nickname: "历史会员"}
	payload := workspaceMemberJSON(t, item)
	for _, field := range []string{"balance", "status", "online"} {
		if value, exists := payload[field]; !exists || value != nil {
			t.Fatalf("historical %s must be explicit null, got %#v (present=%v)", field, value, exists)
		}
	}
	if payload["in_current_room"] != false || payload["can_manage"] != false {
		t.Fatalf("historical member was implicitly manageable: %#v", payload)
	}
	assertWorkspaceMemberNoDestination(t, payload)
}

func workspaceMemberJSON(t *testing.T, item WorkspaceMember) map[string]any {
	t.Helper()
	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func assertWorkspaceMemberNoDestination(t *testing.T, payload map[string]any) {
	t.Helper()
	for _, field := range []string{
		"workspace_id", "current_workspace_id", "room_code", "current_room_code", "room_name",
		"agent_room_code", "parent_agent_id", "parent_tenant_id", "agent_name", "tenant_name",
		"login_scope", "login_identity", "email", "password", "auth_version",
	} {
		if value, exists := payload[field]; exists {
			t.Fatalf("room member response exposes %s=%#v", field, value)
		}
	}
}
