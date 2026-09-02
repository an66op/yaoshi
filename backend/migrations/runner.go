package migrations

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// SQL migrations are intentionally kept outside config/database.go. They are
// the single schema source for both empty local databases and future releases;
// every cross-table/data change has an immutable version and checksum.
//
//go:embed *.sql
var migrationFiles embed.FS

const migrationLockID int64 = 729421117

const coreSchemaBaselineVersion = "202608260000_core_schema.sql"

var (
	baselineTablePattern  = regexp.MustCompile(`(?ms)CREATE TABLE "public"\."([^"]+)" \(\n(.*?)\n\);`)
	baselineColumnPattern = regexp.MustCompile(`(?m)^\s+"([^"]+)"\s+`)
)

type migration struct {
	Version  string
	SQL      string
	Checksum string
}

// Requirement is the immutable identity of one migration embedded in this
// binary. Readiness uses the same inventory as Run, so adding a new .sql file
// cannot accidentally leave /ready reporting an older schema as complete.
type Requirement struct {
	Version  string
	Checksum string
}

// Run applies pending SQL migrations in lexical order. PostgreSQL's
// transaction-scoped advisory lock prevents concurrent application instances
// from applying the same migration twice.
func Run(db *gorm.DB) error {
	items, err := loadMigrations(migrationFiles)
	if err != nil {
		return err
	}
	var baselineSQL string
	for _, item := range items {
		if item.Version == coreSchemaBaselineVersion {
			baselineSQL = item.SQL
		}
		if err := apply(db, item); err != nil {
			return fmt.Errorf("migration %s: %w", item.Version, err)
		}
	}
	if baselineSQL == "" {
		return fmt.Errorf("required core schema baseline %s is missing", coreSchemaBaselineVersion)
	}
	if err := verifyCoreSchema(db, baselineSQL); err != nil {
		return fmt.Errorf("verify core schema: %w", err)
	}
	// Migration 010 installs this idempotent function. Re-running it after the
	// complete inventory means a table introduced by a later migration cannot
	// silently miss the database-level destructive-operation guard.
	if err := db.Exec(`SELECT public.install_application_truncate_guards()`).Error; err != nil {
		return fmt.Errorf("refresh destructive-operation guards: %w", err)
	}
	return nil
}

// Requirements returns a stable copy of every migration required by this
// build. SQL text is intentionally not exposed to callers.
func Requirements() ([]Requirement, error) {
	items, err := loadMigrations(migrationFiles)
	if err != nil {
		return nil, err
	}
	required := make([]Requirement, 0, len(items))
	for _, item := range items {
		required = append(required, Requirement{Version: item.Version, Checksum: item.Checksum})
	}
	return required, nil
}

// VerifyApplied confirms both presence and checksum of every migration
// required by this binary. Extra historic versions are harmless and ignored.
func VerifyApplied(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	required, err := Requirements()
	if err != nil {
		return err
	}
	versions := make([]string, 0, len(required))
	for _, item := range required {
		versions = append(versions, item.Version)
	}
	type appliedMigration struct {
		Version  string
		Checksum string
	}
	var rows []appliedMigration
	if err := db.Table("schema_migrations").
		Select("version, checksum").
		Where("version IN ?", versions).
		Find(&rows).Error; err != nil {
		return err
	}
	applied := make(map[string]string, len(rows))
	for _, row := range rows {
		applied[row.Version] = row.Checksum
	}
	for _, item := range required {
		checksum, ok := applied[item.Version]
		if !ok {
			return fmt.Errorf("missing migration %s", item.Version)
		}
		if checksum != item.Checksum {
			return fmt.Errorf("checksum mismatch for %s", item.Version)
		}
	}
	return nil
}

func apply(db *gorm.DB, item migration) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(?)`, migrationLockID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`
			CREATE TABLE IF NOT EXISTS schema_migrations (
				version varchar(180) PRIMARY KEY,
				checksum char(64) NOT NULL,
				applied_at timestamptz NOT NULL DEFAULT now()
			)
		`).Error; err != nil {
			return err
		}

		var stored string
		if err := tx.Raw(`SELECT COALESCE(MAX(checksum), '') FROM schema_migrations WHERE version = ?`, item.Version).Scan(&stored).Error; err != nil {
			return err
		}
		if stored != "" {
			if stored != item.Checksum {
				return fmt.Errorf("checksum mismatch: database=%s source=%s", stored, item.Checksum)
			}
			return nil
		}
		if err := tx.Exec(item.SQL).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO schema_migrations (version, checksum) VALUES (?, ?)`, item.Version, item.Checksum).Error
	})
}

type baselineColumn struct {
	Table  string
	Column string
}

func baselineColumns(sql string) ([]baselineColumn, error) {
	tables := baselineTablePattern.FindAllStringSubmatch(sql, -1)
	if len(tables) == 0 {
		return nil, fmt.Errorf("core schema baseline contains no public tables")
	}
	result := make([]baselineColumn, 0)
	for _, table := range tables {
		columns := baselineColumnPattern.FindAllStringSubmatch(table[2], -1)
		if len(columns) == 0 {
			return nil, fmt.Errorf("core schema baseline table %s contains no columns", table[1])
		}
		for _, column := range columns {
			result = append(result, baselineColumn{Table: table[1], Column: column[1]})
		}
	}
	return result, nil
}

func verifyCoreSchema(db *gorm.DB, sql string) error {
	expected, err := baselineColumns(sql)
	if err != nil {
		return err
	}
	var actual []baselineColumn
	if err := db.Raw(`
		SELECT table_name AS "table", column_name AS "column"
		FROM information_schema.columns
		WHERE table_schema = 'public'
	`).Scan(&actual).Error; err != nil {
		return err
	}
	available := make(map[string]struct{}, len(actual))
	for _, column := range actual {
		available[column.Table+"\x00"+column.Column] = struct{}{}
	}
	missing := make([]string, 0)
	for _, column := range expected {
		if _, ok := available[column.Table+"\x00"+column.Column]; !ok {
			missing = append(missing, column.Table+"."+column.Column)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	const limit = 12
	display := missing
	if len(display) > limit {
		display = display[:limit]
	}
	return fmt.Errorf(
		"database is missing baseline columns %s (total %d); recreate the unpublished local database from the checked-in migration inventory",
		strings.Join(display, ", "),
		len(missing),
	)
}

func loadMigrations(source fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(source, ".")
	if err != nil {
		return nil, err
	}
	items := make([]migration, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		contents, err := fs.ReadFile(source, name)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(string(contents)) == "" {
			return nil, fmt.Errorf("empty migration %s", name)
		}
		sum := sha256.Sum256(contents)
		items = append(items, migration{Version: name, SQL: string(contents), Checksum: hex.EncodeToString(sum[:])})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Version < items[j].Version })
	return items, nil
}
