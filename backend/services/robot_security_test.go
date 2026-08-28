package services

import (
	"backend/data/models/lottery"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"backend/utils"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func robotDryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestReservedRobotUsernamesCannotBeAllocatedToHumans(t *testing.T) {
	for _, username := range []string{"room_robot_9_01", "ROOM_ROBOT_9_01", "room_activity_member", " Room_Activity_test "} {
		if err := validateHumanUsername(username); err == nil || apperrors.GetErrorCode(err) != "RESERVED_USERNAME" {
			t.Fatalf("validateHumanUsername(%q) error = %v", username, err)
		}
	}
	for _, username := range []string{"roomy_robot_9", "activity_member", "member_001"} {
		if err := validateHumanUsername(username); err != nil {
			t.Fatalf("validateHumanUsername(%q) unexpected error = %v", username, err)
		}
	}
}

func TestRobotCredentialHashesAreRandomAndRejectLegacyFormula(t *testing.T) {
	first, err := newRobotPasswordHash()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newRobotPasswordHash()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("two robot credentials unexpectedly produced the same password hash")
	}
	if utils.CheckPasswordHash("RoomRobot-9-01!", first) || utils.CheckPasswordHash("RoomRobot-9-01!", second) {
		t.Fatal("new robot credential still accepts the legacy deterministic password")
	}
}

func TestMemberLoginQueryExcludesRobotProfiles(t *testing.T) {
	db := robotDryRunDB(t)
	var account user.User
	statement := memberLoginAccountQuery(db, "agent:9", "room_robot_9_01").First(&account).Statement
	sql := statement.SQL.String()
	for _, fragment := range []string{"login_scope =", "LOWER(username)", "NOT EXISTS", "workspace_robot_profiles", `robot.user_id = "user".user_id`} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("member login query omitted %q: %s", fragment, sql)
		}
	}
}

func TestLegacyRobotBackfillRequiresPrefixAndMarker(t *testing.T) {
	db := robotDryRunDB(t)
	var accounts []user.User
	statement := legacyRobotAccountsQuery(db).Find(&accounts).Statement
	sql := statement.SQL.String()
	for _, fragment := range []string{"role =", "workspace_id > 0", "remark =", "LOWER(username) LIKE"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("legacy robot query omitted %q: %s", fragment, sql)
		}
	}
	if strings.Contains(sql, " OR ") {
		t.Fatalf("legacy robot query must not promote an account from either marker alone: %s", sql)
	}
	want := []any{"member", roomActivityRemark, "room_activity\\_%"}
	if len(statement.Vars) < len(want) {
		t.Fatalf("legacy robot query vars = %#v", statement.Vars)
	}
	for index := range want {
		if statement.Vars[index] != want[index] {
			t.Fatalf("legacy robot query var %d = %#v, want %#v", index, statement.Vars[index], want[index])
		}
	}
}

func TestRobotTopUpTargetsOnlyActiveWorkspacesAndValidAccounts(t *testing.T) {
	db := robotDryRunDB(t)
	var workspaces []workspacemodel.Workspace
	workspaceStatement := managedRobotWorkspacesQuery(db).Find(&workspaces).Statement
	if !strings.Contains(workspaceStatement.SQL.String(), "status =") || !strings.Contains(workspaceStatement.SQL.String(), "type IN") {
		t.Fatalf("managed workspace query is not active/type scoped: %s", workspaceStatement.SQL.String())
	}
	var count int64
	countStatement := validWorkspaceRobotProfilesQuery(db, 37).Count(&count).Statement
	countSQL := countStatement.SQL.String()
	for _, fragment := range []string{`JOIN "user" AS account`, "account.workspace_id =", "account.role =", "account.deleted_at IS NULL"} {
		if !strings.Contains(countSQL, fragment) {
			t.Fatalf("valid robot count omitted %q: %s", fragment, countSQL)
		}
	}
}

func TestWorkspaceEnabledGamesQueryAppliesRoomSwitch(t *testing.T) {
	db := robotDryRunDB(t)
	var games []lottery.Game
	statement := workspaceEnabledGamesQuery(db, 37).Find(&games).Statement
	sql := statement.SQL.String()
	for _, fragment := range []string{"lottery_games.enabled =", "NOT EXISTS", "room_game_settings", "room_game.workspace_id =", "room_game.enabled ="} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("workspace game query omitted %q: %s", fragment, sql)
		}
	}
	want := []any{true, uint64(37), false}
	if len(statement.Vars) < len(want) {
		t.Fatalf("workspace game query vars = %#v", statement.Vars)
	}
	for index := range want {
		if statement.Vars[index] != want[index] {
			t.Fatalf("workspace game query var %d = %#v, want %#v", index, statement.Vars[index], want[index])
		}
	}
}
