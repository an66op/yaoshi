package migrations

import (
	"strings"
	"testing"
)

func TestPlanPublicationGuardsUpgradeExistingDatabases(t *testing.T) {
	raw, err := migrationFiles.ReadFile("202609050006_plan_publication_guards.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{
		"CREATE OR REPLACE FUNCTION lock_plan_publication_game",
		"pg_advisory_xact_lock",
		"PERFORM lock_plan_publication_game(NEW.workspace_id, NEW.game_id)",
		"PERFORM lock_plan_publication_game(OLD.workspace_id, OLD.game_id)",
		"CREATE OR REPLACE FUNCTION reject_locked_plan_recommendation_insert()",
		"DROP TRIGGER IF EXISTS trg_reject_locked_plan_recommendation_insert",
		"BEFORE INSERT ON plan_recommendations",
		"viewed.workspace_id = NEW.workspace_id",
		"viewed.position = 0",
		"room_window.workspace_id = NEW.workspace_id",
		"room_window.workspace_id = OLD.workspace_id",
		"CREATE OR REPLACE FUNCTION reject_viewed_plan_recommendation_change()",
		"NEW.workspace_id IS DISTINCT FROM OLD.workspace_id",
		"NEW.game_id IS DISTINCT FROM OLD.game_id",
		"NEW.issue IS DISTINCT FROM OLD.issue",
		"plan publication identity is immutable",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("forward plan publication guard migration missing %q", fragment)
		}
	}
	for _, forbidden := range []string{"DELETE FROM ", "UPDATE plan_recommendations", "TRUNCATE ", "DROP TABLE "} {
		if strings.Contains(strings.ToUpper(sql), strings.ToUpper(forbidden)) {
			t.Fatalf("guard-only upgrade mutates publication data with %q", forbidden)
		}
	}
}

func TestPlanPublicationGuardMigrationFollowsViewTableMigration(t *testing.T) {
	items, err := loadMigrations(migrationFiles)
	if err != nil {
		t.Fatal(err)
	}
	positions := map[string]int{}
	for index, item := range items {
		positions[item.Version] = index
	}
	viewIndex, viewOK := positions["202609050004_plan_publication_views.sql"]
	guardIndex, guardOK := positions["202609050006_plan_publication_guards.sql"]
	if !viewOK || !guardOK || guardIndex <= viewIndex {
		t.Fatalf("guard upgrade order is unsafe: view=%d/%v guard=%d/%v", viewIndex, viewOK, guardIndex, guardOK)
	}
}
