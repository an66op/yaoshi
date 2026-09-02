package migrations

import (
	"strings"
	"testing"
)

func TestOddsConfigurationRevisionMigrationNeverActivatesPrices(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202609030002_odds_configuration_revision.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"odds_config_revision bigint NOT NULL DEFAULT 0",
		"CHECK (odds_config_revision >= 0)",
		"ALTER TABLE lottery_play_limits ALTER COLUMN odds SET DEFAULT 0",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("odds revision migration lacks %q", fragment)
		}
	}
	for _, forbidden := range []string{"UPDATE lottery_play_limits", "DELETE FROM", "TRUNCATE", "explicitly_configured = true"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("odds revision migration mutates existing prices: %q", forbidden)
		}
	}
}
