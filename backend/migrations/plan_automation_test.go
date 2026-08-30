package migrations

import (
	"strings"
	"testing"
)

func TestPlanAutomationMigrationIsOptInAndHasDurableIdentity(t *testing.T) {
	raw, err := migrationFiles.ReadFile("202608300006_plan_automation.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{
		"enabled boolean NOT NULL DEFAULT false", "CHECK (mode = 'demo')", "REFERENCES workspaces(id)",
		"UNIQUE (workspace_id, game_id, issue, master_key)", "source varchar(16) NOT NULL DEFAULT 'manual'",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration omits safety boundary %q", fragment)
		}
	}
	if strings.Contains(strings.ToUpper(sql), "INSERT INTO") || strings.Contains(strings.ToUpper(sql), "UPDATE PLAN_") {
		t.Fatal("migration auto-enabled or seeded a room")
	}
}
