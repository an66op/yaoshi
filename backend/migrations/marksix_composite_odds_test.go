package migrations

import (
	"strings"
	"testing"
)

func TestMarkSixCompositeOddsMigrationFreezesAdditiveJSONSnapshot(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202609040001_marksix_composite_odds.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"ALTER TABLE lottery_bets",
		"ALTER TABLE lottery_bet_archives",
		"ADD COLUMN IF NOT EXISTS odds_terms jsonb NOT NULL DEFAULT '{}'::jsonb",
		"jsonb_typeof(odds_terms) = 'object'",
		"OLD.odds_terms IS DISTINCT FROM NEW.odds_terms",
		"BEFORE UPDATE OF odds_terms ON lottery_bets",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("Mark Six composite odds migration lacks %q", fragment)
		}
	}
	for _, forbidden := range []string{"UPDATE lottery_bets", "UPDATE lottery_bet_archives", "DELETE FROM", "TRUNCATE"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("migration rewrites or destroys historical rows: %q", forbidden)
		}
	}
}
