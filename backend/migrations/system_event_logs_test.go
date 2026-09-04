package migrations

import (
	"strings"
	"testing"
)

func TestSystemEventLogsMigrationIsAppendOnlyAndQueryable(t *testing.T) {
	raw, err := migrationFiles.ReadFile("202609040002_system_event_logs.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{
		"CREATE TABLE system_event_logs",
		"idx_system_event_logs_created_at",
		"idx_system_event_logs_event_type",
		"idx_system_event_logs_status",
		"idx_system_event_logs_game_id",
		"BEFORE UPDATE OR DELETE ON system_event_logs",
		"system event logs are append-only",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("system event migration is missing %q", fragment)
		}
	}
}
