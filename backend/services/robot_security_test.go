package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"backend/utils"
	"strings"
	"testing"
	"time"

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

func TestRobotFinancialQueriesFailClosed(t *testing.T) {
	db := robotDryRunDB(t)
	account := user.User{UserID: 91, WorkspaceID: 37}
	var count int64
	profileStatement := robotProfileFinancialQuery(db, account).Count(&count).Statement
	profileSQL := profileStatement.SQL.String()
	for _, fragment := range []string{"workspace_robot_profiles", "workspace_id =", "user_id ="} {
		if !strings.Contains(profileSQL, fragment) {
			t.Fatalf("robot financial identity query omitted %q: %s", fragment, profileSQL)
		}
	}
	terms := betFinancialTerms{isRobot: true, rebateRate: 25, shareRate: 40}
	if terms.flyRequest(-1) != 0 {
		t.Fatal("robot financial policy allowed automatic fly amount")
	}

	var rows []bet.Bet
	exclusionStatement := excludeRobotProfileBets(db.Model(&bet.Bet{})).Find(&rows).Statement
	exclusionSQL := exclusionStatement.SQL.String()
	for _, fragment := range []string{"NOT EXISTS", "workspace_robot_profiles", "robot_profile.workspace_id = lottery_bets.workspace_id", "robot_profile.user_id = lottery_bets.user_id"} {
		if !strings.Contains(exclusionSQL, fragment) {
			t.Fatalf("financial aggregate robot exclusion omitted %q: %s", fragment, exclusionSQL)
		}
	}
}

func TestAgentWorkspaceFinancialAggregatesExcludeRobotProfiles(t *testing.T) {
	db := robotDryRunDB(t)
	const workspaceID uint64 = 37
	start := time.Date(2026, time.August, 30, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))

	var money struct{ Stake, Payout, Rebate, AgentShare int64 }
	settledStatement := agentWorkspaceFinancialBetQuery(db, workspaceID).
		Where("COALESCE(settled_at,updated_at,created_at) >= ? AND status IN ?", start, []string{"won", "lost"}).
		Select("COALESCE(SUM(amount_cents),0) AS stake, COALESCE(SUM(payout_cents),0) AS payout, COALESCE(SUM(rebate_cents),0) AS rebate, COALESCE(SUM(agent_share_cents),0) AS agent_share").
		Scan(&money).Statement

	var pendingTurnover int64
	pendingAmountStatement := agentWorkspaceFinancialBetQuery(db, workspaceID).
		Where("status = ?", "pending").
		Select("COALESCE(SUM(amount_cents),0)").
		Scan(&pendingTurnover).Statement

	var pendingBets int64
	pendingCountStatement := agentWorkspaceFinancialBetQuery(db, workspaceID).
		Where("status = ?", "pending").
		Count(&pendingBets).Statement

	for name, statement := range map[string]*gorm.Statement{
		"today financial totals": settledStatement,
		"pending turnover":       pendingAmountStatement,
		"pending bet count":      pendingCountStatement,
	} {
		sql := statement.SQL.String()
		for _, fragment := range []string{
			"workspace_id =",
			"NOT EXISTS",
			"workspace_robot_profiles",
			"robot_profile.workspace_id = lottery_bets.workspace_id",
			"robot_profile.user_id = lottery_bets.user_id",
		} {
			if !strings.Contains(sql, fragment) {
				t.Fatalf("%s query omitted %q: %s", name, fragment, sql)
			}
		}
		if strings.Contains(strings.ToLower(sql), "remark") {
			t.Fatalf("%s query must not identify robots by mutable remark: %s", name, sql)
		}
		if len(statement.Vars) == 0 || statement.Vars[0] != workspaceID {
			t.Fatalf("%s query is not scoped to workspace %d: vars=%#v", name, workspaceID, statement.Vars)
		}
	}
}

func assertRobotProfilePairExclusion(t *testing.T, name string, statement *gorm.Statement, workspaceColumn, userColumn string) {
	t.Helper()
	sql := statement.SQL.String()
	for _, fragment := range []string{
		"NOT EXISTS",
		"workspace_robot_profiles",
		"robot_profile.workspace_id = " + workspaceColumn,
		"robot_profile.user_id = " + userColumn,
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("%s omitted %q: %s", name, fragment, sql)
		}
	}
	if strings.Contains(strings.ToLower(sql), "remark") {
		t.Fatalf("%s still identifies robots through mutable remark: %s", name, sql)
	}
}

func TestFinancialAndOperatingReportsUseAuthoritativeRobotIdentity(t *testing.T) {
	db := robotDryRunDB(t)
	period := reportPeriod{
		Start: time.Date(2026, time.August, 29, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, time.August, 30, 0, 0, 0, 0, time.UTC),
	}

	var ledgerRows []user.BalanceTransaction
	financialLedger := NewFinancialReportService(db).
		filteredLedger(FinancialReportFilter{WorkspaceID: 37}, period).
		Find(&ledgerRows).Statement
	assertRobotProfilePairExclusion(t, "financial ledger", financialLedger, "t.workspace_id", "t.user_id")

	var bets []bet.Bet
	operatingBets := NewOperatingReportService(db).
		filteredBets(OperatingReportFilter{WorkspaceID: 37}, period, true).
		Find(&bets).Statement
	assertRobotProfilePairExclusion(t, "operating bets", operatingBets, "b.workspace_id", "b.user_id")

	var welfareCents int64
	welfareLedger := NewOperatingReportService(db).
		welfareQuery(OperatingReportFilter{WorkspaceID: 37}, period).
		Select("COALESCE(SUM(t.amount_cents),0)").Scan(&welfareCents).Statement
	assertRobotProfilePairExclusion(t, "operating welfare", welfareLedger, "t.workspace_id", "t.user_id")

	reportCenterBets := NewReportCenterService(db).
		betBase(ReportCenterFilter{WorkspaceID: 37}, period).
		Find(&bets).Statement
	assertRobotProfilePairExclusion(t, "report center bets", reportCenterBets, "lottery_bets.workspace_id", "lottery_bets.user_id")

	var accounts []user.User
	scoreboard := roomScorePlayersQuery(db, 37).Find(&accounts).Statement
	assertRobotProfilePairExclusion(t, "room scoreboard", scoreboard, `"user".workspace_id`, `"user".user_id`)
}

func TestSettlementRobotIdentitySurvivesRemarkEditsAndUsesWorkspacePair(t *testing.T) {
	robotAccount := user.User{UserID: 91, WorkspaceID: 37, Remark: "管理员已经修改备注"}
	robots := map[robotBetIdentity]struct{}{
		{workspaceID: robotAccount.WorkspaceID, userID: robotAccount.UserID}: {},
	}
	if !isRobotSettlementRecipient(robots, settlementRecipient{WorkspaceID: robotAccount.WorkspaceID, UserID: robotAccount.UserID}) {
		t.Fatal("profile-backed robot became a real player after its remark changed")
	}
	if isRobotSettlementRecipient(robots, settlementRecipient{WorkspaceID: 38, UserID: robotAccount.UserID}) {
		t.Fatal("robot identity leaked into a different workspace")
	}
	humanWithLegacyRemark := user.User{UserID: 92, WorkspaceID: 37, Remark: roomActivityRemark}
	if isRobotSettlementRecipient(robots, settlementRecipient{WorkspaceID: humanWithLegacyRemark.WorkspaceID, UserID: humanWithLegacyRemark.UserID}) {
		t.Fatal("a human was classified as robot from its mutable remark")
	}
}
