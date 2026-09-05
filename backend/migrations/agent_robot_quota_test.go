package migrations

import (
	"strings"
	"testing"
)

func TestAgentRobotQuotaMigration(t *testing.T) {
	raw, err := migrationFiles.ReadFile("202609050001_agent_robot_quota.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, fragment := range []string{"add column robot_quota", "default 10", "robot_quota >= 0", "robot_quota <= 10"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("robot quota migration missing %q", fragment)
		}
	}
}
