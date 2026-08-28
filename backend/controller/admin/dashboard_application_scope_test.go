package admin

import (
	"backend/data/models/application"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPlatformDashboardPendingApplicationsExcludeJoin(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	var rows []application.Application
	statement := platformPendingApplicationQuery(db).Find(&rows).Statement
	sql := strings.ToLower(statement.SQL.String())
	if !strings.Contains(sql, "status =") || !strings.Contains(sql, "request_type <>") {
		t.Fatalf("platform dashboard application query = %s", sql)
	}
	want := []any{"pending", "join"}
	if len(statement.Vars) != len(want) {
		t.Fatalf("dashboard vars = %#v", statement.Vars)
	}
	for index := range want {
		if statement.Vars[index] != want[index] {
			t.Fatalf("dashboard var %d = %#v, want %#v", index, statement.Vars[index], want[index])
		}
	}
}
