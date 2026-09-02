package services

import (
	"os"
	"strings"
	"testing"
)

func TestRobotBetArchiveRetainsRequestAndRuleSnapshotChecks(t *testing.T) {
	contents, err := os.ReadFile("data_lifecycle.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(contents)
	start := strings.Index(source, "func restoreRobotBetArchive(")
	end := strings.Index(source, "func restoreRobotLedgerArchive(")
	if start < 0 || end <= start {
		t.Fatal("bet archive restore function was not found")
	}
	restore := source[start:end]
	for _, fragment := range []string{
		"rule_version, request_reference, amount_cents",
		"valid_turnover_cents, settlement_odds, user_issue_stake_cents_snapshot, settlement_policy, pc28_gray_push",
		"COALESCE(source_json ->> 'request_reference', '')",
		"rule_version IS DISTINCT FROM COALESCE(source_json ->> 'rule_version', '')",
		"valid_turnover_cents IS DISTINCT FROM (source_json ->> 'valid_turnover_cents')::bigint",
		"settlement_odds IS DISTINCT FROM (source_json ->> 'settlement_odds')::numeric",
		"user_issue_stake_cents_snapshot IS DISTINCT FROM (source_json ->> 'user_issue_stake_cents_snapshot')::bigint",
		"settlement_policy IS DISTINCT FROM COALESCE(source_json ->> 'settlement_policy', '')",
		"pc28_gray_push IS DISTINCT FROM COALESCE((source_json ->> 'pc28_gray_push')::boolean, false)",
		"row_hash <> md5(source_json::text)",
		"WHEN archive.source_json ? 'request_reference'",
		"WHEN archive.source_json ? 'rule_version'",
		"WHEN archive.source_json ? 'valid_turnover_cents'",
		"WHEN archive.source_json ? 'settlement_odds'",
		"WHEN archive.source_json ? 'user_issue_stake_cents_snapshot'",
		"WHEN archive.source_json ? 'settlement_policy'",
		"WHEN archive.source_json ? 'pc28_gray_push'",
		"archive.row_hash <> md5((to_jsonb(hot)",
	} {
		if !strings.Contains(restore, fragment) {
			t.Fatalf("restore lost immutable evidence check %q", fragment)
		}
	}
	for _, financial := range []string{"amount_cents", "payout_cents", "odds", "rebate_cents", "agent_share_cents"} {
		if strings.Contains(restore, "ARRAY['"+financial+"']") {
			t.Fatalf("restore excludes historic financial field %s from integrity check", financial)
		}
	}
	if !strings.Contains(source, "RETURNING id, row_hash") || !strings.Contains(source, "DELETE FROM lottery_bets hot USING candidates, archived archive") {
		t.Fatal("bet archive DELETE must verify the INSERT's returned evidence in the same statement")
	}
}
