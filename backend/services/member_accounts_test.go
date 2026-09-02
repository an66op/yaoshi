package services

import (
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestHumanMemberQueryPreservesScopeAndExcludesAllRobotProfiles(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable"}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	var count int64
	statement := HumanMemberQuery(db.Where(`"user".workspace_id = ? AND "user".parent_agent_id = ?`, 42, 88)).Where(`"user".status = ?`, 1).Count(&count).Statement
	sql := strings.ToLower(statement.SQL.String())
	for _, predicate := range []string{`"user".workspace_id =`, `"user".parent_agent_id =`, `"user".status =`, `"user".role =`, `coalesce("user".remark`, "not like", "not exists", "workspace_robot_profiles", `robot_profile.workspace_id = "user".workspace_id`, `robot_profile.user_id = "user".user_id`} {
		if !strings.Contains(sql, predicate) {
			t.Fatalf("missing %q in %s", predicate, sql)
		}
	}
	for _, forbidden := range []string{"robot_profile.enabled", "robot_profile.status", "username"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("unsafe robot identity filter %q in %s", forbidden, sql)
		}
	}
	want := []any{42, 88, "member", "测试机器人专用账号%", 1}
	if len(statement.Vars) != len(want) {
		t.Fatalf("vars=%#v", statement.Vars)
	}
	for i := range want {
		if statement.Vars[i] != want[i] {
			t.Fatalf("var[%d]=%#v want=%#v", i, statement.Vars[i], want[i])
		}
	}
}
