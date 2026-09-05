package migrations

import (
	"strings"
	"testing"
)

func TestActivitySeedParticipantsMigrationSubtractsLegacyBaseFromRecognizableSeeds(t *testing.T) {
	raw, err := migrationFiles.ReadFile("202609050007_activity_seed_participants.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{
		"COUNT(participation.id)::bigint AS actual_participants",
		"participation.activity_id = activity.id",
		"activity.title = '每日签到'",
		"activity.title = '幸运红包'",
		"CASE activity.type WHEN 'checkin' THEN 128 ELSE 56 END",
		"+ COUNT(participation.id)",
		"SET participants = legacy_candidate.actual_participants",
		"legacy_candidate.legacy_base + legacy_candidate.actual_participants",
		"activity.config_json =",
		"activity.deleted_at IS NULL",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("legacy activity seed migration missing %q", fragment)
		}
	}
	for _, forbidden := range []string{"TRUNCATE", "DELETE FROM", "NOT EXISTS", "participants >", "participants <>"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("legacy seed repair has an overbroad mutation: %q", forbidden)
		}
	}
}
