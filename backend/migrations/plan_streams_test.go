package migrations

import (
	"strings"
	"testing"
)

func TestPlanStreamMigrationPreservesLegacyAndHasCompactIdentity(t *testing.T) {
	raw, err := migrationFiles.ReadFile("202608300007_plan_streams.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{"UNIQUE(workspace_id, game_id, position, plan_key)", "UNIQUE(stream_id,issue_id)", "payload_json text NOT NULL", "revoked boolean NOT NULL DEFAULT false", "CHECK(status IN ('active','completed','interrupted'))"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("missing stream safety boundary %q", fragment)
		}
	}
	for _, forbidden := range []string{"INSERT INTO plan_recommendations", "UPDATE plan_recommendations", "UPDATE plan_automations SET enabled"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("migration modifies historical content: %q", forbidden)
		}
	}
}
