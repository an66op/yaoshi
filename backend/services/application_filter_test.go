package services

import (
	"backend/data/models/application"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func applicationFilterStatement(t *testing.T, filter ApplicationFilter) *gorm.Statement {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	var rows []application.Application
	return applyApplicationFilters(db.Model(&application.Application{}), filter).Find(&rows).Statement
}

func TestApplyApplicationFiltersUsesShanghaiDayForDate(t *testing.T) {
	statement := applicationFilterStatement(t, ApplicationFilter{Date: "2026-08-28"})
	if occurrences := strings.Count(statement.SQL.String(), "created_at"); occurrences != 2 {
		t.Fatalf("date filter SQL = %q, want one lower and one upper created_at bound", statement.SQL.String())
	}
	want := []time.Time{
		time.Date(2026, 8, 27, 16, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 28, 16, 0, 0, 0, time.UTC),
	}
	if len(statement.Vars) != len(want) {
		t.Fatalf("date filter vars = %#v, want %#v", statement.Vars, want)
	}
	for index := range want {
		got, ok := statement.Vars[index].(time.Time)
		if !ok || !got.Equal(want[index]) {
			t.Fatalf("date bound %d = %#v, want %s", index, statement.Vars[index], want[index])
		}
	}
}

func TestApplyApplicationFiltersUsesShanghaiInclusiveRange(t *testing.T) {
	statement := applicationFilterStatement(t, ApplicationFilter{Start: "2026-08-27", End: "2026-08-28"})
	want := []time.Time{
		time.Date(2026, 8, 26, 16, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 28, 16, 0, 0, 0, time.UTC),
	}
	if len(statement.Vars) != len(want) {
		t.Fatalf("range filter vars = %#v, want %#v", statement.Vars, want)
	}
	for index := range want {
		got, ok := statement.Vars[index].(time.Time)
		if !ok || !got.Equal(want[index]) {
			t.Fatalf("range bound %d = %#v, want %s", index, statement.Vars[index], want[index])
		}
	}
}

func TestApplyApplicationFiltersPrefersExplicitRangeOverDate(t *testing.T) {
	statement := applicationFilterStatement(t, ApplicationFilter{
		Date: "2026-08-01", Start: "2026-08-27", End: "2026-08-28",
	})
	if len(statement.Vars) != 2 {
		t.Fatalf("combined date filter added duplicate bounds: SQL=%q vars=%#v", statement.SQL.String(), statement.Vars)
	}
	first, ok := statement.Vars[0].(time.Time)
	if !ok || !first.Equal(time.Date(2026, 8, 26, 16, 0, 0, 0, time.UTC)) {
		t.Fatalf("explicit range was not preferred: %#v", statement.Vars)
	}
}
