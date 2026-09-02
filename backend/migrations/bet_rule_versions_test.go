package migrations

import (
	"strings"
	"testing"
)

func TestBetRuleMigrationPreservesLegacyFinancialData(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202608310002_bet_rule_versions.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"ADD COLUMN IF NOT EXISTS rule_version varchar(32) NOT NULL DEFAULT ''",
		"ALTER TABLE lottery_bet_archives", "request_reference, rule_version)",
		"(COALESCE(source_json ->> 'request_reference', '')), rule_version)",
		"COALESCE(archived.source_json ->> 'request_reference', '') = NEW.request_reference",
		"archived.rule_version = NEW.rule_version",
		"OLD.rule_version IS DISTINCT FROM NEW.rule_version",
		"BEFORE UPDATE OF rule_version ON lottery_bets",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("rule snapshot migration lacks %q", fragment)
		}
	}
	for _, forbidden := range []string{"UPDATE lottery_bets", "UPDATE lottery_bet_archives", "UPDATE user", "DELETE FROM", "TRUNCATE", "SET rule_version"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("migration rewrites historic records: %q", forbidden)
		}
	}
}
