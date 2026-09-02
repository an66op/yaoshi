package migrations

import (
	"strings"
	"testing"
)

func TestExplicitOddsConfirmationMigrationKeepsUpgradedLegacyRowsClosed(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202609020003_explicit_odds_confirmation.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"explicitly_configured boolean NOT NULL DEFAULT false",
		"rule_version varchar(32) NOT NULL DEFAULT ''",
		"configuration_source varchar(32) NOT NULL DEFAULT 'unconfigured'",
		"configured_at timestamptz",
		"SET explicitly_configured = false",
		"rule_version = ''",
		"configuration_source = 'unconfigured'",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("explicit odds confirmation migration lacks %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"UPDATE lottery_play_limits SET explicitly_configured = true",
		"DELETE FROM lottery_play_limits",
		"TRUNCATE lottery_play_limits",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("migration can activate or destroy upgraded legacy rows: %q", forbidden)
		}
	}
}
