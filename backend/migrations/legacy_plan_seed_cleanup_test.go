package migrations

import (
	"strings"
	"testing"
)

func TestLegacyPlanSeedCleanupUsesTheCompleteOriginalSignature(t *testing.T) {
	raw, err := migrationFiles.ReadFile("202609050008_remove_legacy_plan_seed.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{
		"DROP TRIGGER IF EXISTS trg_reject_viewed_plan_recommendation_change",
		"WITH legacy_templates",
		"'speed-racing', '青云老师', '综合趋势', '#2aa9b3', '1,5,9', '大', '单', 10",
		"'canada-28', '北斗数据师', '冷热分析', '#6e70df', '6,11,19', '小', '双', 20",
		"'au-lucky-10', '锦鲤计划师', '节奏追踪', '#e58b45', '3,6,10', '大', '双', 30",
		"recommendation.result = 'pending'",
		"recommendation.source = 'manual'",
		"recommendation.note = '房间人工计划，由后台维护'",
		"recommendation.enabled = true",
		"recommendation.deleted_at IS NULL",
		"CREATE TRIGGER trg_reject_viewed_plan_recommendation_change",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("legacy plan seed cleanup missing exact boundary %q", fragment)
		}
	}
	if strings.Contains(sql, "DELETE FROM plan_recommendations\nWHERE source = 'manual'") {
		t.Fatal("legacy cleanup contains a broad manual-publication delete")
	}
}

func TestLegacyPlanSeedCleanupRunsAfterPublicationGuards(t *testing.T) {
	items, err := loadMigrations(migrationFiles)
	if err != nil {
		t.Fatal(err)
	}
	positions := map[string]int{}
	for index, item := range items {
		positions[item.Version] = index
	}
	cleanup, cleanupOK := positions["202609050008_remove_legacy_plan_seed.sql"]
	guards, guardsOK := positions["202609050006_plan_publication_guards.sql"]
	if !cleanupOK || !guardsOK || cleanup <= guards {
		t.Fatalf("legacy cleanup migration order is unsafe: guards=%d/%t cleanup=%d/%t", guards, guardsOK, cleanup, cleanupOK)
	}
}
