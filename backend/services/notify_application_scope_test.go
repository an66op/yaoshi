package services

import (
	"backend/data/models/application"
	"strings"
	"testing"
)

func TestPlatformReminderExcludesJoin(t *testing.T) {
	db := applicationPlatformDryRunDB(t)
	var rows []application.Application
	statement := pendingApplicationReminderQuery(db, 1, true).Find(&rows).Statement
	sql := strings.ToLower(statement.SQL.String())
	if !strings.Contains(sql, "workspace_id >") || !strings.Contains(sql, "request_type <>") {
		t.Fatalf("platform reminder query is not isolated: %s", sql)
	}
	if len(statement.Vars) != 2 || statement.Vars[0] != "pending" || statement.Vars[1] != "join" {
		t.Fatalf("platform reminder vars = %#v", statement.Vars)
	}
}

func TestRoomReminderKeepsJoin(t *testing.T) {
	db := applicationPlatformDryRunDB(t)
	var rows []application.Application
	statement := pendingApplicationReminderQuery(db, 8, false).Find(&rows).Statement
	sql := strings.ToLower(statement.SQL.String())
	if !strings.Contains(sql, "workspace_id =") || strings.Contains(sql, "request_type <>") {
		t.Fatalf("room reminder query lost room applications: %s", sql)
	}
	want := []any{"pending", uint64(8)}
	if len(statement.Vars) != len(want) {
		t.Fatalf("room reminder vars = %#v", statement.Vars)
	}
	for index := range want {
		if statement.Vars[index] != want[index] {
			t.Fatalf("room reminder var %d = %#v, want %#v", index, statement.Vars[index], want[index])
		}
	}
}
