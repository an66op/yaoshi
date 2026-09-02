package migrations

import (
	"strings"
	"testing"
)

func TestBingoDrawRevisionMigrationIsAdditiveAndLeavesLegacyRowsUnclaimed(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202609020002_bingo_draw_revisions.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"ADD COLUMN IF NOT EXISTS source_revision varchar(64) NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS conversion_revision varchar(64) NOT NULL DEFAULT ''",
		"idx_lottery_draw_revision",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("Bingo draw revision migration lacks %q", fragment)
		}
	}
	for _, forbidden := range []string{"UPDATE lottery_draws", "DELETE FROM lottery_draws", "TRUNCATE lottery_draws", "SET source_revision", "SET conversion_revision"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("migration claims or rewrites legacy draw history: %q", forbidden)
		}
	}
}
