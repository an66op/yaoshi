package services

import (
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

func devAcceptanceSafety() DevAcceptanceFundingSafety {
	return DevAcceptanceFundingSafety{Mode: "debug", DatabaseHost: "127.0.0.1", DatabaseName: "wangzhe_timing_test"}
}

func TestDevAcceptanceFundingValidatesSafetyAndInputBeforeDatabaseAccess(t *testing.T) {
	validSafety := devAcceptanceSafety()
	validInput := DevAcceptanceFundingInput{
		ResetRequestID: "dev-reset-20260905T010203Z-42-7",
		LoginScope:     "agent:2",
		Username:       "acceptance_member",
		AmountCents:    250000,
	}
	for name, safety := range map[string]DevAcceptanceFundingSafety{
		"release":         {Mode: "release", DatabaseHost: "127.0.0.1", DatabaseName: "wangzhe_timing_test"},
		"test":            {Mode: "test", DatabaseHost: "127.0.0.1", DatabaseName: "wangzhe_timing_test"},
		"spaced-debug":    {Mode: " debug ", DatabaseHost: "127.0.0.1", DatabaseName: "wangzhe_timing_test"},
		"remote-host":     {Mode: "debug", DatabaseHost: "db.example.test", DatabaseName: "wangzhe_timing_test"},
		"private-address": {Mode: "debug", DatabaseHost: "192.168.1.20", DatabaseName: "wangzhe_timing_test"},
		"empty-database":  {Mode: "debug", DatabaseHost: "localhost", DatabaseName: ""},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateDevAcceptanceFundingSafety(safety); err == nil {
				t.Fatal("unsafe environment accepted")
			}
			if _, err := FundDevAcceptanceAccount(nil, safety, validInput); err == nil || err.Error() == "数据库连接不可用" {
				t.Fatal("database was reached before safety validation")
			}
		})
	}
	for _, host := range []string{"localhost", "127.0.0.1", "::1", "/var/run/postgresql"} {
		safety := validSafety
		safety.DatabaseHost = host
		if err := ValidateDevAcceptanceFundingSafety(safety); err != nil {
			t.Fatalf("local PostgreSQL host %q rejected: %v", host, err)
		}
	}
	// PostgreSQL's inet text representation includes a CIDR suffix, which is
	// why the live identity query must normalize through host(inet).
	if devAcceptanceLocalHost("127.0.0.1/32") || devAcceptanceLocalHost("::1/128") {
		t.Fatal("raw CIDR text must not bypass config host validation")
	}

	for name, input := range map[string]DevAcceptanceFundingInput{
		"short-request":  {ResetRequestID: "short", LoginScope: validInput.LoginScope, Username: validInput.Username, AmountCents: validInput.AmountCents},
		"spaced-request": {ResetRequestID: " " + validInput.ResetRequestID, LoginScope: validInput.LoginScope, Username: validInput.Username, AmountCents: validInput.AmountCents},
		"bad-request":    {ResetRequestID: "dev-reset/unsafe", LoginScope: validInput.LoginScope, Username: validInput.Username, AmountCents: validInput.AmountCents},
		"empty-scope":    {ResetRequestID: validInput.ResetRequestID, LoginScope: "", Username: validInput.Username, AmountCents: validInput.AmountCents},
		"spaced-scope":   {ResetRequestID: validInput.ResetRequestID, LoginScope: " agent:2 ", Username: validInput.Username, AmountCents: validInput.AmountCents},
		"empty-user":     {ResetRequestID: validInput.ResetRequestID, LoginScope: validInput.LoginScope, Username: "", AmountCents: validInput.AmountCents},
		"spaced-user":    {ResetRequestID: validInput.ResetRequestID, LoginScope: validInput.LoginScope, Username: " member ", AmountCents: validInput.AmountCents},
		"zero-amount":    {ResetRequestID: validInput.ResetRequestID, LoginScope: validInput.LoginScope, Username: validInput.Username, AmountCents: 0},
		"debit":          {ResetRequestID: validInput.ResetRequestID, LoginScope: validInput.LoginScope, Username: validInput.Username, AmountCents: -1},
		"too-large":      {ResetRequestID: validInput.ResetRequestID, LoginScope: validInput.LoginScope, Username: validInput.Username, AmountCents: maxDevAcceptanceAmountCents + 1},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := FundDevAcceptanceAccount(nil, validSafety, input); err == nil || err.Error() == "数据库连接不可用" {
				t.Fatal("database was reached before input validation")
			}
		})
	}
	if _, err := FundDevAcceptanceAccount(nil, validSafety, validInput); err == nil || err.Error() != "数据库连接不可用" {
		t.Fatalf("valid input was not accepted before the nil database check: %v", err)
	}
}

var devAcceptanceTestRequestSequence atomic.Uint64

func devAcceptanceTestRequestID(suffix string) string {
	return fmt.Sprintf("dev-reset-test-%d-%d-%s", time.Now().UTC().UnixNano(), devAcceptanceTestRequestSequence.Add(1), suffix)
}

func insertDevAcceptanceReceipt(t *testing.T, db *gorm.DB, requestID, databaseName, scope string, createdAt time.Time) {
	t.Helper()
	if err := db.Exec(`
		INSERT INTO public.development_reset_receipts (
			request_id, database_name, backup_filename, backup_sha256,
			executed_by, reset_scope, cleared_tables, created_at
		) VALUES (?, ?, 'acceptance-fixture.sql.gz', repeat('a', 64),
		          'acceptance-test', ?, ARRAY['user_balance_transactions']::text[], ?)
	`, requestID, databaseName, scope, createdAt).Error; err != nil {
		t.Fatal("insert reset receipt:", err)
	}
}

func createDevAcceptanceMember(t *testing.T, db *gorm.DB, room workspacemodel.Workspace, username string, balance int64) user.User {
	t.Helper()
	member := user.User{
		Username: username, LoginScope: room.Scope, WorkspaceID: room.ID,
		Password: "fixture-no-login", Nickname: "验收会员", Role: "member", Status: 1,
		BalanceCents: balance, ParentTenantID: &room.OwnerUserID, Remark: "保留账号配置",
	}
	if err := db.Create(&member).Error; err != nil {
		t.Fatal(err)
	}
	if err := ActivateWorkspaceMembership(db, &member, room); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&member, member.UserID).Error; err != nil {
		t.Fatal(err)
	}
	return member
}

func devAcceptanceTableSnapshot(t *testing.T, db *gorm.DB, table string) string {
	t.Helper()
	allowed := map[string]bool{
		"workspaces": true, "workspace_memberships": true, "lottery_games": true,
		"lottery_play_limits": true, "room_play_odds": true, "user_play_odds": true,
		"room_game_settings": true, "system_settings": true,
	}
	if !allowed[table] {
		t.Fatalf("unsafe snapshot table %q", table)
	}
	var snapshot string
	query := fmt.Sprintf(`SELECT COALESCE(jsonb_agg(to_jsonb(row_data) ORDER BY to_jsonb(row_data)::text)::text, '[]') FROM public.%s AS row_data`, table)
	if err := db.Raw(query).Scan(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestDevAcceptanceFundingPostgresCreditsOnceAndPreservesConfiguration(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "acceptance_funding_room", "79601")
	member := createDevAcceptanceMember(t, db, room, "acceptance_funding_member", 0)
	requestID := devAcceptanceTestRequestID("credit-once")
	insertDevAcceptanceReceipt(t, db, requestID, "wangzhe_timing_test", "business_data", time.Now().UTC())

	var accountBefore user.User
	if err := db.First(&accountBefore, member.UserID).Error; err != nil {
		t.Fatal(err)
	}
	tables := []string{"workspaces", "workspace_memberships", "lottery_games", "lottery_play_limits", "room_play_odds", "user_play_odds", "room_game_settings", "system_settings"}
	configBefore := map[string]string{}
	for _, table := range tables {
		configBefore[table] = devAcceptanceTableSnapshot(t, db, table)
	}

	input := DevAcceptanceFundingInput{ResetRequestID: requestID, LoginScope: member.LoginScope, Username: strings.ToUpper(member.Username), AmountCents: 250000}
	first, err := FundDevAcceptanceAccount(db, devAcceptanceSafety(), input)
	if err != nil {
		t.Fatal(err)
	}
	wantReference := devAcceptanceReference(requestID, member.UserID)
	if first.Duplicate || first.Reference != wantReference || first.UserID != member.UserID || first.WorkspaceID != room.ID || first.BeforeCents != 0 || first.AfterCents != 250000 || first.AmountCents != 250000 || first.Username != member.Username {
		t.Fatalf("unexpected first result: %+v", first)
	}

	var accountAfter user.User
	if err := db.First(&accountAfter, member.UserID).Error; err != nil {
		t.Fatal(err)
	}
	if accountAfter.BalanceCents != 250000 {
		t.Fatalf("balance=%d, want 250000", accountAfter.BalanceCents)
	}
	accountAfter.BalanceCents = accountBefore.BalanceCents
	if !reflect.DeepEqual(accountBefore, accountAfter) {
		t.Fatalf("funding modified account configuration:\nbefore=%+v\nafter=%+v", accountBefore, accountAfter)
	}
	for _, table := range tables {
		if after := devAcceptanceTableSnapshot(t, db, table); after != configBefore[table] {
			t.Fatalf("funding modified %s configuration", table)
		}
	}

	var ledger user.BalanceTransaction
	if err := db.Where("user_id = ? AND reference = ?", member.UserID, wantReference).Take(&ledger).Error; err != nil {
		t.Fatal(err)
	}
	if ledger.WorkspaceID != room.ID || ledger.Type != devAcceptanceType || ledger.AmountCents != 250000 || ledger.BeforeCents != 0 || ledger.AfterCents != 250000 {
		t.Fatalf("unexpected ledger: %+v", ledger)
	}

	repeated, err := FundDevAcceptanceAccount(db, devAcceptanceSafety(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !repeated.Duplicate || repeated.Reference != first.Reference || repeated.AfterCents != first.AfterCents {
		t.Fatalf("repeat did not return the original outcome: %+v", repeated)
	}
	var count int64
	if err := db.Model(&user.BalanceTransaction{}).Where("user_id = ? AND reference = ?", member.UserID, wantReference).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("repeat ledger count=%d err=%v", count, err)
	}
	var repeatedBalance int64
	if err := db.Model(&user.User{}).Where("user_id = ?", member.UserID).Pluck("balance_cents", &repeatedBalance).Error; err != nil || repeatedBalance != 250000 {
		t.Fatalf("repeat balance=%d err=%v", repeatedBalance, err)
	}

	changedAmount := input
	changedAmount.AmountCents++
	if _, err := FundDevAcceptanceAccount(db, devAcceptanceSafety(), changedAmount); err == nil || !strings.Contains(err.Error(), "参数不一致") {
		t.Fatalf("request_id payload conflict was accepted: %v", err)
	}
}

func TestDevAcceptanceFundingPostgresRequiresLatestMatchingReceiptAndZeroActiveMember(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "acceptance_guard_room", "79602")
	member := createDevAcceptanceMember(t, db, room, "acceptance_guard_member", 0)
	safety := devAcceptanceSafety()
	baseTime := time.Now().UTC().Add(-time.Hour)
	validInput := DevAcceptanceFundingInput{ResetRequestID: devAcceptanceTestRequestID("guard-valid"), LoginScope: member.LoginScope, Username: member.Username, AmountCents: 1000}

	if _, err := FundDevAcceptanceAccount(db, safety, validInput); err == nil || !strings.Contains(err.Error(), "未找到") {
		t.Fatalf("missing receipt was accepted: %v", err)
	}
	insertDevAcceptanceReceipt(t, db, validInput.ResetRequestID, safety.DatabaseName, "business_data", baseTime)
	stale := validInput
	stale.ResetRequestID = devAcceptanceTestRequestID("guard-stale")
	if _, err := FundDevAcceptanceAccount(db, safety, stale); err == nil || !strings.Contains(err.Error(), "最新重置凭证") {
		t.Fatalf("non-latest request was accepted: %v", err)
	}

	nonzero := createDevAcceptanceMember(t, db, room, "acceptance_nonzero_member", 1)
	nonzeroInput := validInput
	nonzeroInput.Username = nonzero.Username
	if _, err := FundDevAcceptanceAccount(db, safety, nonzeroInput); err == nil || !strings.Contains(err.Error(), "首次注资前余额必须为 0") {
		t.Fatalf("nonzero first balance was accepted: %v", err)
	}

	noMembership := user.User{
		Username: "acceptance_no_membership", LoginScope: room.Scope, WorkspaceID: room.ID,
		Password: "fixture-no-login", Role: "member", Status: 1, BalanceCents: 0,
	}
	if err := db.Create(&noMembership).Error; err != nil {
		t.Fatal(err)
	}
	missingMembershipInput := validInput
	missingMembershipInput.Username = noMembership.Username
	if _, err := FundDevAcceptanceAccount(db, safety, missingMembershipInput); err == nil || !strings.Contains(err.Error(), "active member 关系") {
		t.Fatalf("member without active membership was accepted: %v", err)
	}

	disabled := createDevAcceptanceMember(t, db, room, "acceptance_disabled_member", 0)
	if err := db.Model(&user.User{}).Where("user_id = ?", disabled.UserID).UpdateColumn("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	disabledInput := validInput
	disabledInput.Username = disabled.Username
	if _, err := FundDevAcceptanceAccount(db, safety, disabledInput); err == nil || !strings.Contains(err.Error(), "已启用") {
		t.Fatalf("disabled member was accepted: %v", err)
	}

	wrongDatabaseRequestID := devAcceptanceTestRequestID("wrong-database")
	insertDevAcceptanceReceipt(t, db, wrongDatabaseRequestID, "wrong_database", "business_data", baseTime.Add(time.Minute))
	wrongDatabase := validInput
	wrongDatabase.ResetRequestID = wrongDatabaseRequestID
	if _, err := FundDevAcceptanceAccount(db, safety, wrongDatabase); err == nil || !strings.Contains(err.Error(), "最新重置凭证") {
		t.Fatalf("receipt for another database was accepted: %v", err)
	}
	wrongScopeRequestID := devAcceptanceTestRequestID("wrong-scope")
	insertDevAcceptanceReceipt(t, db, wrongScopeRequestID, safety.DatabaseName, "public_schema_rebuild", baseTime.Add(2*time.Minute))
	wrongScope := validInput
	wrongScope.ResetRequestID = wrongScopeRequestID
	if _, err := FundDevAcceptanceAccount(db, safety, wrongScope); err == nil || !strings.Contains(err.Error(), "business_data") {
		t.Fatalf("wrong reset scope was accepted: %v", err)
	}
}

func TestDevAcceptanceFundingPostgresRequiresActiveWorkspace(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "acceptance_inactive_room", "79603")
	member := createDevAcceptanceMember(t, db, room, "acceptance_inactive_room_member", 0)
	requestID := devAcceptanceTestRequestID("inactive-workspace")
	insertDevAcceptanceReceipt(t, db, requestID, "wangzhe_timing_test", "business_data", time.Now().UTC())
	if err := db.Model(&workspacemodel.Workspace{}).Where("id = ?", room.ID).UpdateColumn("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	input := DevAcceptanceFundingInput{ResetRequestID: requestID, LoginScope: member.LoginScope, Username: member.Username, AmountCents: 1000}
	if _, err := FundDevAcceptanceAccount(db, devAcceptanceSafety(), input); err == nil || !strings.Contains(err.Error(), "工作区不存在或未启用") {
		t.Fatalf("inactive workspace was accepted: %v", err)
	}
	var count int64
	if err := db.Model(&user.BalanceTransaction{}).Where("user_id = ?", member.UserID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("rejected funding wrote ledger rows: count=%d err=%v", count, err)
	}
}

func TestDevAcceptanceFundingPostgresRollsBackBalanceWhenLedgerInsertFails(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "acceptance_atomic_room", "79604")
	member := createDevAcceptanceMember(t, db, room, "acceptance_atomic_member", 0)
	requestID := devAcceptanceTestRequestID("atomic-rollback")
	insertDevAcceptanceReceipt(t, db, requestID, "wangzhe_timing_test", "business_data", time.Now().UTC())
	if err := db.Exec(fmt.Sprintf(`
		CREATE FUNCTION fail_dev_acceptance_ledger() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.user_id = %d THEN RAISE EXCEPTION 'fixture ledger failure'; END IF;
			RETURN NEW;
		END $$;
		CREATE TRIGGER fail_dev_acceptance_ledger
		BEFORE INSERT ON public.user_balance_transactions
		FOR EACH ROW EXECUTE FUNCTION fail_dev_acceptance_ledger()
	`, member.UserID)).Error; err != nil {
		t.Fatal(err)
	}

	input := DevAcceptanceFundingInput{ResetRequestID: requestID, LoginScope: member.LoginScope, Username: member.Username, AmountCents: 1000}
	if _, err := FundDevAcceptanceAccount(db, devAcceptanceSafety(), input); err == nil || !strings.Contains(err.Error(), "fixture ledger failure") {
		t.Fatalf("injected ledger failure was not propagated: %v", err)
	}
	var balance int64
	if err := db.Model(&user.User{}).Where("user_id = ?", member.UserID).Pluck("balance_cents", &balance).Error; err != nil || balance != 0 {
		t.Fatalf("failed ledger insert left balance=%d err=%v", balance, err)
	}
	var count int64
	if err := db.Model(&user.BalanceTransaction{}).Where("user_id = ?", member.UserID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("failed ledger insert left count=%d err=%v", count, err)
	}
}
