package migrations

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadMigrationsOrdersAndChecksumsFiles(t *testing.T) {
	source := fstest.MapFS{
		"002_second.sql": &fstest.MapFile{Data: []byte("SELECT 2;")},
		"001_first.sql":  &fstest.MapFile{Data: []byte("SELECT 1;")},
		"README.md":      &fstest.MapFile{Data: []byte("ignored")},
	}
	items, err := loadMigrations(source)
	if err != nil {
		t.Fatalf("loadMigrations() error = %v", err)
	}
	if len(items) != 2 || items[0].Version != "001_first.sql" || items[1].Version != "002_second.sql" {
		t.Fatalf("unexpected order: %#v", items)
	}
	if len(items[0].Checksum) != 64 || items[0].Checksum == items[1].Checksum {
		t.Fatalf("checksums were not generated correctly: %#v", items)
	}
}

func TestLoadMigrationsRejectsEmptySQL(t *testing.T) {
	_, err := loadMigrations(fstest.MapFS{"001_empty.sql": &fstest.MapFile{Data: []byte("  \n")}})
	if err == nil {
		t.Fatal("expected empty migration to be rejected")
	}
}

func TestRequirementsMatchEveryEmbeddedSQLFile(t *testing.T) {
	required, err := Requirements()
	if err != nil {
		t.Fatalf("Requirements() error = %v", err)
	}
	entries, err := migrationFiles.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	want := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			want++
		}
	}
	if len(required) != want {
		t.Fatalf("Requirements() returned %d migrations, want %d", len(required), want)
	}
	for _, item := range required {
		if !strings.HasSuffix(item.Version, ".sql") || len(item.Checksum) != 64 {
			t.Fatalf("invalid migration requirement: %#v", item)
		}
	}
}

func TestCoreSchemaBaselineIsFirstAndSelfContained(t *testing.T) {
	items, err := loadMigrations(migrationFiles)
	if err != nil {
		t.Fatalf("loadMigrations() error = %v", err)
	}
	if len(items) == 0 || items[0].Version != coreSchemaBaselineVersion {
		t.Fatalf("first migration = %q, want %q", items[0].Version, coreSchemaBaselineVersion)
	}
	baseline := items[0].SQL
	for _, fragment := range []string{
		`CREATE TABLE "public"."user"`,
		`CREATE TABLE "public"."workspaces"`,
		`CREATE TABLE "public"."lottery_bets"`,
		`CREATE TABLE "public"."member_chat_messages"`,
		`CREATE TABLE "public"."user_balance_transactions"`,
	} {
		if !strings.Contains(baseline, fragment) {
			t.Fatalf("core baseline is missing %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"transaction_timeout",
		"set_config('search_path'",
		"schema_migrations",
	} {
		if strings.Contains(strings.ToLower(baseline), strings.ToLower(forbidden)) {
			t.Fatalf("core baseline contains runner-owned/session SQL %q", forbidden)
		}
	}
}

func TestBaselineInventoryCoversTablesAndColumns(t *testing.T) {
	contents, err := migrationFiles.ReadFile(coreSchemaBaselineVersion)
	if err != nil {
		t.Fatalf("read core baseline: %v", err)
	}
	tables, err := baselineTables(string(contents))
	if err != nil {
		t.Fatalf("baselineTables() error = %v", err)
	}
	columns, err := baselineColumns(string(contents))
	if err != nil {
		t.Fatalf("baselineColumns() error = %v", err)
	}
	if len(tables) < 35 {
		t.Fatalf("baseline contains only %d tables", len(tables))
	}
	if len(columns) < 300 {
		t.Fatalf("baseline contains only %d columns", len(columns))
	}
	want := map[string]bool{
		"user.user_id":                      false,
		"user.auth_version":                 false,
		"workspaces.room_code":              false,
		"lottery_bets.workspace_id":         false,
		"member_chat_messages.workspace_id": false,
	}
	for _, column := range columns {
		key := column.Table + "." + column.Column
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Fatalf("baseline inventory is missing %s", key)
		}
	}
}

func TestDataLifecycleMigrationIsReversibleAndDisabledByDefault(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608270001_data_lifecycle.sql")
	if err != nil {
		t.Fatalf("read lifecycle migration: %v", err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS data_cleanup_runs",
		"CREATE TABLE IF NOT EXISTS lottery_bet_archives",
		"CREATE TABLE IF NOT EXISTS user_balance_transaction_archives",
		"member_chat_messages ADD COLUMN IF NOT EXISTS deleted_at",
		"member_notifications ADD COLUMN IF NOT EXISTS deleted_at",
		"admin_notifications ADD COLUMN IF NOT EXISTS deleted_at",
		"soft_restored_at",
		"financial_restored_at",
		"(0, 'chat_messages', false",
		"(0, 'notifications', false",
		"(0, 'audit_logs', false",
		"(0, 'robot_test_data', false",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("lifecycle migration is missing %q", fragment)
		}
	}
}

func TestDeletionPolicyMigrationIsIdempotentAndFailClosed(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608270005_deletion_policy.sql")
	if err != nil {
		t.Fatalf("read deletion-policy migration: %v", err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"member_payment_accounts\n    ADD COLUMN IF NOT EXISTS deleted_at",
		"lottery_lobby_categories\n    ADD COLUMN IF NOT EXISTS deleted_at",
		"WHERE is_default AND deleted_at IS NULL",
		"ON DELETE RESTRICT NOT VALID",
		"'robot_chat_messages'",
		"VALUES (0, 'robot_chat_messages', false, 30, 'soft_delete')",
		"CREATE OR REPLACE FUNCTION reject_protected_hard_delete()",
		"'user_applications'",
		"'lottery_draws'",
		"'member_chat_messages'",
		"'chat_red_packet_claims'",
		"'member_payment_accounts'",
		"IF NOT EXISTS",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("deletion-policy migration is missing %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"TRUNCATE ",
		"DELETE FROM lottery_bets",
		"DELETE FROM user_balance_transactions",
		"DELETE FROM user_applications",
	} {
		if strings.Contains(strings.ToUpper(sql), strings.ToUpper(forbidden)) {
			t.Fatalf("deletion-policy migration contains forbidden SQL %q", forbidden)
		}
	}
}

func TestServiceWelcomeMigrationKeepsPrivateConversationsIndependent(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608270007_service_welcome.sql")
	if err != nil {
		t.Fatalf("read service-welcome migration: %v", err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_member_service_welcome",
		"workspace_id, scope, room_scope, game_id, message_type",
		"room_type = 'service'",
		"message_type = 'welcome'",
		"deleted_at IS NULL",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("service-welcome migration is missing %q", fragment)
		}
	}
}

func TestFinancialDeleteGuardRequiresLifecycleTransaction(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608270006_guard_financial_deletes.sql")
	if err != nil {
		t.Fatalf("read financial-delete guard migration: %v", err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"current_setting('wangzhe.lifecycle_delete', true)",
		"'lottery_bets'",
		"'user_balance_transactions'",
		"'admin_audit_logs'",
		"'lottery_bet_archives'",
		"'user_balance_transaction_archives'",
		"'admin_audit_log_archives'",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("financial-delete guard migration is missing %q", fragment)
		}
	}
}

func TestContentPurgeGuardIsSeparateAndFailClosed(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608280009_guard_content_purge.sql")
	if err != nil {
		t.Fatalf("read content-purge guard migration: %v", err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"current_setting('wangzhe.lifecycle_content_purge', true)",
		"OLD.deleted_at IS NOT NULL",
		"'member_chat_messages'",
		"'member_notifications'",
		"'admin_notifications'",
		"RAISE EXCEPTION 'hard DELETE is disabled for protected table %'",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("content-purge guard migration is missing %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"wangzhe.lifecycle_delete",
		"lottery_bets",
		"user_balance_transactions",
		"admin_audit_logs",
		"user_applications",
		"chat_red_packets",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("content-purge guard unexpectedly grants access to %q", forbidden)
		}
	}
}

func TestContentPurgeReceiptsDisableFalseRestoreClaims(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608280010_content_purge_receipts.sql")
	if err != nil {
		t.Fatalf("read content-purge receipt migration: %v", err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"content_purged_at timestamptz NULL",
		"content_purge_count bigint NOT NULL DEFAULT 0",
		"last_content_purge_request_id varchar(96) NOT NULL DEFAULT ''",
		"current_setting('wangzhe.lifecycle_content_purge', true) IS DISTINCT FROM 'on'",
		"NEW.content_purge_count <= OLD.content_purge_count",
		"trg_guard_cleanup_content_purge_receipt",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("content-purge receipt migration is missing %q", fragment)
		}
	}
}

func TestLifecycleAuditIntegrityMigrationIsFailClosed(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608270009_lifecycle_audit_integrity.sql")
	if err != nil {
		t.Fatalf("read lifecycle audit integrity migration: %v", err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"executed_by_id bigint NOT NULL DEFAULT 0",
		"soft_restored_by_id bigint NOT NULL DEFAULT 0",
		"financial_restored_by_id bigint NOT NULL DEFAULT 0",
		"lifecycle_guard_cleanup_run_update",
		"trg_reject_lifecycle_receipt_delete",
		"reject_lifecycle_archive_update",
		"admin_audit_log_archives",
		"lottery_bet_archives",
		"user_balance_transaction_archives",
		"trg_protect_service_welcome_from_soft_delete",
		"durable service welcome cannot be soft deleted",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("lifecycle audit integrity migration is missing %q", fragment)
		}
	}
}

func TestMonitorLedgerIndexesSupportBoundedAccountingAudit(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608300001_monitor_ledger_index.sql")
	if err != nil {
		t.Fatalf("read monitor ledger index migration: %v", err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"idx_balance_archive_user_id_id",
		"idx_balance_ledger_monitor_chain",
		"INCLUDE (before_cents, after_cents)",
		"idx_balance_ledger_monitor_arithmetic_error",
		"WHERE after_cents <> before_cents + amount_cents",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("monitor ledger index migration is missing %q", fragment)
		}
	}
}

func TestDevelopmentResetGuardClosesTruncateBypass(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608270010_dev_reset_guard.sql")
	if err != nil {
		t.Fatalf("read development reset guard migration: %v", err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS development_reset_receipts",
		"reject_unapproved_application_truncate",
		"current_setting('wangzhe.dev_reset', true)",
		"'confirmed:' || current_database()",
		"install_application_truncate_guards",
		"BEFORE TRUNCATE ON %I.%I",
		"relation.relname <> 'development_reset_receipts'",
		"development reset receipts are immutable",
		"BEFORE UPDATE OR DELETE ON development_reset_receipts",
		"BEFORE TRUNCATE ON development_reset_receipts",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("development reset guard migration is missing %q", fragment)
		}
	}
}

func TestWorkspaceMarkerIsVersionedBeforeBootstrap(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608270011_reset_guard_workspace_marker.sql")
	if err != nil {
		t.Fatalf("read workspace marker guard migration: %v", err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS public.workspace_migration_markers",
		"key varchar(120) PRIMARY KEY",
		"SELECT public.install_application_truncate_guards()",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("workspace marker guard migration is missing %q", fragment)
		}
	}
}

func TestResetReceiptsBindPhysicalDatabaseIdentity(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608270012_reset_identity_receipts.sql")
	if err != nil {
		t.Fatalf("read reset identity receipt migration: %v", err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"server_system_identifier varchar(32)",
		"server_address varchar(80)",
		"server_port integer",
		"sentinel_token_sha256 char(64)",
		"ck_development_reset_receipt_server_port",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("reset identity receipt migration is missing %q", fragment)
		}
	}
}

func TestPlanRecommendationsSeedOnlyRealAcceptingIssues(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608270008_plan_recommendations.sql")
	if err != nil {
		t.Fatalf("read plan-recommendations migration: %v", err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS plan_recommendations",
		"workspace_id bigint NOT NULL REFERENCES workspaces(id)",
		"JOIN LATERAL",
		"issue.status = 'accepting'",
		"template.master_color",
		"'pending'",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("plan migration is missing %q", fragment)
		}
	}
	for _, forbidden := range []string{"待更新", "lottery_draws", "'hit'", "'miss'"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("plan migration must not seed fabricated issue/results via %q", forbidden)
		}
	}
}

func TestWebSocketRevocationOutboxIsTransactionalAndVersioned(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608280003_ws_session_revocation_outbox.sql")
	if err != nil {
		t.Fatalf("read WebSocket revocation migration: %v", err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS ws_session_revocation_outbox",
		"WHERE delivered_at IS NULL",
		"idx_ws_session_revocation_delivered",
		"WHERE delivered_at IS NOT NULL",
		"CREATE OR REPLACE FUNCTION bump_user_session_generation()",
		"NEW.auth_version := OLD.auth_version + 1",
		"BEFORE UPDATE OF password, status, role, workspace_id, login_scope",
		"CREATE OR REPLACE FUNCTION enqueue_user_session_revocation()",
		"INSERT INTO ws_session_revocation_outbox (user_id, revoked_auth_version)",
		"VALUES (OLD.user_id, OLD.auth_version)",
		"AFTER UPDATE OF password, status, role, workspace_id, login_scope",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("WebSocket revocation migration is missing %q", fragment)
		}
	}
}

func TestRobotResetReceiptsAreDurableAndImmutable(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608280006_robot_reset_receipts.sql")
	if err != nil {
		t.Fatalf("read robot reset receipt migration: %v", err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS workspace_robot_reset_receipts",
		"UNIQUE (workspace_id, request_id_hash)",
		"payload_hash char(32) NOT NULL",
		"robot reset receipts are immutable",
		"BEFORE UPDATE OR DELETE ON workspace_robot_reset_receipts",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("robot reset receipt migration is missing %q", fragment)
		}
	}
}
