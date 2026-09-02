package migrations

import (
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Every fixture is rollback-only and requires a dedicated empty loopback DB.
// These tests never read application config or connect to a developer's DB.
func migrationTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("BACKEND_MIGRATIONS_TEST_DSN")
	if dsn == "" {
		t.Skip("set BACKEND_MIGRATIONS_TEST_DSN to an empty local wangzhe_migrations_test database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Fatal("BACKEND_MIGRATIONS_TEST_DSN must be a PostgreSQL URL")
	}
	if host := parsed.Hostname(); host != "127.0.0.1" && host != "localhost" && host != "::1" {
		t.Fatal("migration integration tests require a loopback host")
	}
	if parsed.Path != "/wangzhe_migrations_test" {
		t.Fatal("migration integration tests require the dedicated wangzhe_migrations_test database")
	}
	query := parsed.Query()
	for key := range query {
		if key != "sslmode" {
			t.Fatalf("migration integration tests reject connection override %q", key)
		}
	}
	query.Set("search_path", "public")
	parsed.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(parsed.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("connect to disposable migration database:", err)
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	var objectCount int64
	if err := db.Raw(`SELECT COUNT(*) FROM pg_catalog.pg_class AS relation
		JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
		WHERE namespace.nspname = 'public' AND relation.relkind IN ('r', 'p', 'v', 'm', 'S', 'f')`).Scan(&objectCount).Error; err != nil {
		t.Fatal(err)
	}
	if objectCount != 0 {
		t.Fatal("refusing to initialize a nonempty migration test database")
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() {
		if err := tx.Rollback().Error; err != nil {
			t.Error("rollback disposable migration schema:", err)
		}
	})
	return tx
}

func TestMigrationsPostgresFreshAndRepeat(t *testing.T) {
	db := migrationTestDatabase(t)
	if err := Run(db); err != nil {
		t.Fatal("fresh migrations:", err)
	}
	if err := VerifyApplied(db); err != nil {
		t.Fatal("verify fresh migrations:", err)
	}
	type stamp struct {
		Version   string
		Checksum  string
		AppliedAt time.Time
	}
	var before, after []stamp
	if err := db.Table("schema_migrations").Order("version").Find(&before).Error; err != nil {
		t.Fatal(err)
	}
	required, err := Requirements()
	if err != nil || len(before) != len(required) {
		t.Fatalf("applied migration inventory: got=%d want=%d err=%v", len(before), len(required), err)
	}
	var unguarded []string
	if err := db.Raw(`SELECT relation.relname FROM pg_catalog.pg_class AS relation
		JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
		WHERE namespace.nspname = 'public' AND relation.relkind IN ('r', 'p')
		AND NOT EXISTS (SELECT 1 FROM pg_catalog.pg_trigger AS guard
			WHERE guard.tgrelid = relation.oid AND NOT guard.tgisinternal
			AND guard.tgname IN ('trg_reject_unapproved_application_truncate', 'trg_reject_development_reset_receipt_truncate'))`).Scan(&unguarded).Error; err != nil {
		t.Fatal(err)
	}
	if len(unguarded) != 0 {
		t.Fatalf("migrations left application tables without truncate guards: %v", unguarded)
	}
	var indexCount int64
	if err := db.Raw(`SELECT COUNT(*) FROM pg_catalog.pg_indexes WHERE schemaname = 'public' AND indexname IN (
		'idx_user_username_global_ci', 'idx_workspace_public_room_code', 'idx_user_applications_user_request',
		'idx_application_one_pending_join', 'idx_workspace_one_active_membership',
		'idx_room_odds_game_play', 'idx_user_odds_game_play')`).Scan(&indexCount).Error; err != nil || indexCount != 7 {
		t.Fatalf("workspace concurrency indexes: got=%d want=7 err=%v", indexCount, err)
	}
	if err := Run(db); err != nil {
		t.Fatal("repeat migrations:", err)
	}
	if err := db.Table("schema_migrations").Order("version").Find(&after).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("repeat startup changed committed migration identities")
	}
}

func TestMigrationsPostgresRejectsUnversionedObjects(t *testing.T) {
	for name, fixture := range map[string]string{
		"application_table":  `CREATE TABLE public."user" (user_id bigint)`,
		"unrelated_table":    `CREATE TABLE public.unversioned_notes (id bigint)`,
		"unrelated_view":     `CREATE VIEW public.unversioned_notes AS SELECT 1 AS id`,
		"unrelated_sequence": `CREATE SEQUENCE public.unversioned_notes`,
	} {
		t.Run(name, func(t *testing.T) {
			db := migrationTestDatabase(t)
			if err := db.Exec(fixture).Error; err != nil {
				t.Fatal(err)
			}
			if err := Run(db); err == nil || !strings.Contains(err.Error(), "unversioned database") {
				t.Fatalf("expected explicit unversioned database rejection, got %v", err)
			}
			var metadataExists bool
			if err := db.Raw(`SELECT to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&metadataExists).Error; err != nil {
				t.Fatal(err)
			}
			if metadataExists {
				t.Fatal("failed baseline left migration metadata behind")
			}
		})
	}
}

func TestMigrationsPostgresRejectsHistoryWithoutBaseline(t *testing.T) {
	db := migrationTestDatabase(t)
	if err := db.Exec(`CREATE TABLE public.schema_migrations (version varchar(180) PRIMARY KEY, checksum char(64) NOT NULL, applied_at timestamptz NOT NULL DEFAULT now());
		INSERT INTO public.schema_migrations (version, checksum) VALUES ('202608270001_data_lifecycle.sql', repeat('0', 64))`).Error; err != nil {
		t.Fatal(err)
	}
	if err := Run(db); err == nil || !strings.Contains(err.Error(), "without the core schema baseline") {
		t.Fatalf("expected missing baseline history rejection, got %v", err)
	}
	var count int64
	if err := db.Table("schema_migrations").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("old migration history was mutated: count=%d err=%v", count, err)
	}
}

func TestMigrationsPostgresRejectsChecksumMismatch(t *testing.T) {
	db := migrationTestDatabase(t)
	if err := Run(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Table("schema_migrations").Where("version = ?", coreSchemaBaselineVersion).Update("checksum", strings.Repeat("0", 64)).Error; err != nil {
		t.Fatal(err)
	}
	if err := Run(db); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Run accepted a changed baseline checksum: %v", err)
	}
	if err := VerifyApplied(db); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("VerifyApplied accepted a changed baseline checksum: %v", err)
	}
}
