package services

import (
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestMemberInviteRewardSummaryIsScopedToAuthenticatedMember(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}

	var summary memberInviteRewardSummary
	statement := memberInviteRewardSummaryQuery(db, 73).
		Select("COUNT(*) AS invited_count, COALESCE(SUM(reward_cents), 0) AS total_reward_cents").
		Scan(&summary).Statement
	sql := strings.ToLower(statement.SQL.String())
	for _, fragment := range []string{"user_id =", "action =", "count(*)", "sum(reward_cents)"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("invite summary query omitted %q: %s", fragment, sql)
		}
	}
	if len(statement.Vars) != 2 || statement.Vars[0] != uint64(73) || statement.Vars[1] != "invite_referral" {
		t.Fatalf("invite summary query has wrong boundary: %#v", statement.Vars)
	}
}
