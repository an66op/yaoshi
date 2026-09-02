package admin

import (
	"reflect"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPlatformDashboardMemberCountsExcludeRobots(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable"}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, activeOnly := range []bool{false, true} {
		var count int64
		statement := platformMemberCountQuery(db, activeOnly).Count(&count).Statement
		sql := strings.ToLower(statement.SQL.String())
		for _, predicate := range []string{"not exists", "workspace_robot_profiles", `robot_profile.workspace_id = "user".workspace_id`, `robot_profile.user_id = "user".user_id`, `"user".role =`, `coalesce("user".remark`} {
			if !strings.Contains(sql, predicate) {
				t.Fatalf("activeOnly=%v missing %q: %s", activeOnly, predicate, sql)
			}
		}
		if strings.Contains(sql, `"user".status =`) != activeOnly {
			t.Fatalf("active filter mismatch: %s", sql)
		}
		want := []any{"member", "测试机器人专用账号%"}
		if activeOnly {
			want = append(want, 1)
		}
		if !reflect.DeepEqual(statement.Vars, want) {
			t.Fatalf("activeOnly=%v vars=%#v", activeOnly, statement.Vars)
		}
	}
}
