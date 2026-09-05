package migrations

import (
	"strings"
	"testing"
)

func TestPlanPublicationViewsMigrationIsIdempotentAndOnlyRemovesMarkedBackfill(t *testing.T) {
	raw, err := migrationFiles.ReadFile("202609050004_plan_publication_views.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS plan_publication_views",
		"UNIQUE(workspace_id,user_id,game_id,issue,position,plan_key)",
		"viewed_at timestamptz NOT NULL DEFAULT clock_timestamp()",
		"source = 'demo'",
		"note = '系统补充的历史展示记录，不计入命中率。'",
		"WHERE source = 'manual' AND result IN ('hit', 'miss')",
		"reject_viewed_plan_recommendation_change",
		"reject_plan_stream_payload_change",
		"reject_plan_publication_view_mutation",
		"SELECT public.install_application_truncate_guards()",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("missing publication audit boundary %q", fragment)
		}
	}
	dropGuard := strings.Index(sql, "DROP TRIGGER IF EXISTS trg_reject_viewed_plan_recommendation_change")
	cleanup := strings.Index(sql, "DELETE FROM plan_recommendations")
	recreateGuard := strings.LastIndex(sql, "CREATE TRIGGER trg_reject_viewed_plan_recommendation_change")
	if dropGuard < 0 || cleanup < 0 || recreateGuard < 0 || !(dropGuard < cleanup && cleanup < recreateGuard) {
		t.Fatal("retry-safe guard must be dropped before exact cleanup and recreated afterwards")
	}
	for _, forbidden := range []string{
		"DELETE FROM plan_recommendations\nWHERE note =",
		"DELETE FROM plan_recommendations\nWHERE source = 'manual'",
		"SET numbers =",
		"SET issue =",
		"UPDATE plan_stream_cycles SET payload_json",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("migration can rewrite genuine publication data: %q", forbidden)
		}
	}
}
