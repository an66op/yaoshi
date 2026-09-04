package migrations

import (
	"strings"
	"testing"
)

func TestSGSSCSourceSnapshotMigrationAddsEmptyColumnsAndArchiveLookupIndex(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202609030003_sgssc_source_snapshot.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	if strings.Count(sql, "ADD COLUMN IF NOT EXISTS draw_source_revision varchar(64) NOT NULL DEFAULT ''") != 2 {
		t.Fatal("both live and archived tickets need an empty legacy source snapshot")
	}
	for _, table := range []string{"lottery_bets", "lottery_bet_archives"} {
		if !strings.Contains(sql, "ALTER TABLE "+table) {
			t.Fatalf("migration omitted %s", table)
		}
	}
	if !strings.Contains(sql, "CREATE INDEX IF NOT EXISTS idx_bet_archive_game_issue\n    ON lottery_bet_archives (game_id, issue)") ||
		strings.Count(sql, "CREATE INDEX") != 1 {
		t.Fatal("source verification needs one cross-room archive period index without duplicating live indexes")
	}
	for _, forbidden := range []string{"UPDATE ", "DELETE ", "TRUNCATE", "INSERT ", "SET draw_source_revision", "DROP "} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("source snapshot migration must not rewrite legacy evidence: %s", forbidden)
		}
	}
}
