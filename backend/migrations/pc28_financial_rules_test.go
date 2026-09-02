package migrations

import (
	"strings"
	"testing"
)

func TestPC28FinancialRulesMigrationIsAdditiveAndPreservesLegacyRows(t *testing.T) {
	contents, err := migrationFiles.ReadFile("202609020001_pc28_financial_rules.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"ALTER TABLE lottery_bets",
		"ALTER TABLE lottery_bet_archives",
		"ADD COLUMN IF NOT EXISTS valid_turnover_cents bigint NULL",
		"ADD COLUMN IF NOT EXISTS settlement_odds numeric NULL",
		"ADD COLUMN IF NOT EXISTS user_issue_stake_cents_snapshot bigint NULL",
		"ADD COLUMN IF NOT EXISTS settlement_policy varchar(64) NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS pc28_gray_push boolean NOT NULL DEFAULT false",
		"valid_turnover_cents <= amount_cents",
		"settlement_odds IS NULL OR settlement_odds >= 0",
		"user_issue_stake_cents_snapshot >= amount_cents",
		"OLD.pc28_gray_push IS DISTINCT FROM NEW.pc28_gray_push",
		"BEFORE UPDATE OF pc28_gray_push ON lottery_bets",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("PC28 financial migration lacks %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"UPDATE lottery_bets", "UPDATE lottery_bet_archives", "DELETE FROM", "TRUNCATE",
		"SET valid_turnover_cents", "SET settlement_odds", "SET user_issue_stake_cents_snapshot",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("PC28 migration rewrites or destroys historical rows: %q", forbidden)
		}
	}
}
