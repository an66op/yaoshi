package services

import (
	"backend/data/models/application"
	workspacemodel "backend/data/models/workspace"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func memberRoomDryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestRoomEntryReviewRequiredOnlyForFirstVisit(t *testing.T) {
	tests := []struct {
		name          string
		reviewEnabled bool
		hasHistory    bool
		want          bool
	}{
		{name: "first visit to reviewed room", reviewEnabled: true, hasHistory: false, want: true},
		{name: "inactive historic membership reenters directly", reviewEnabled: true, hasHistory: true, want: false},
		{name: "first visit to open room", reviewEnabled: false, hasHistory: false, want: false},
		{name: "historic membership in open room", reviewEnabled: false, hasHistory: true, want: false},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			if got := roomEntryReviewRequired(item.reviewEnabled, item.hasHistory); got != item.want {
				t.Fatalf("roomEntryReviewRequired(%v, %v) = %v, want %v", item.reviewEnabled, item.hasHistory, got, item.want)
			}
		})
	}
}

func TestHistoricalWorkspaceMembershipLookupIncludesInactiveRows(t *testing.T) {
	db := memberRoomDryRunDB(t)
	var membership workspacemodel.Membership
	statement := historicalWorkspaceMembershipQuery(db, 81, 92).Take(&membership).Statement
	sql := strings.ToLower(statement.SQL.String())
	if !strings.Contains(sql, "workspace_id =") || !strings.Contains(sql, "user_id =") {
		t.Fatalf("historical membership lookup is not room/user scoped: %s", sql)
	}
	if strings.Contains(sql, "status =") {
		t.Fatalf("inactive historical membership was excluded: %s", sql)
	}
	want := []any{uint64(81), uint64(92)}
	if len(statement.Vars) < len(want) {
		t.Fatalf("historical membership vars = %#v", statement.Vars)
	}
	for index := range want {
		if statement.Vars[index] != want[index] {
			t.Fatalf("historical membership var %d = %#v, want %#v", index, statement.Vars[index], want[index])
		}
	}
}

func TestHistoricalWorkspaceMembershipLookupDoesNotLeakAcrossRooms(t *testing.T) {
	db := memberRoomDryRunDB(t)
	var first, second workspacemodel.Membership
	firstStatement := historicalWorkspaceMembershipQuery(db, 81, 92).Take(&first).Statement
	secondStatement := historicalWorkspaceMembershipQuery(db, 82, 92).Take(&second).Statement
	if len(firstStatement.Vars) < 2 || len(secondStatement.Vars) < 2 {
		t.Fatalf("membership queries are missing room/user boundaries: %#v / %#v", firstStatement.Vars, secondStatement.Vars)
	}
	if firstStatement.Vars[0] != uint64(81) || secondStatement.Vars[0] != uint64(82) {
		t.Fatalf("membership history did not retain the target workspace: %#v / %#v", firstStatement.Vars, secondStatement.Vars)
	}
	if firstStatement.Vars[1] != uint64(92) || secondStatement.Vars[1] != uint64(92) {
		t.Fatalf("membership history did not retain the authenticated user: %#v / %#v", firstStatement.Vars, secondStatement.Vars)
	}
}

func TestPendingHistoricalJoinCleanupIsTargetRoomScoped(t *testing.T) {
	db := memberRoomDryRunDB(t)
	var rows []application.Application
	statement := pendingHistoricalJoinApplicationsQuery(db, 81, 92).Find(&rows).Statement
	sql := strings.ToLower(statement.SQL.String())
	for _, fragment := range []string{"workspace_id =", "user_id =", "request_type =", "status ="} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("pending historical join cleanup omitted %q: %s", fragment, sql)
		}
	}
	want := []any{uint64(81), uint64(92), "join", "pending"}
	if len(statement.Vars) < len(want) {
		t.Fatalf("pending historical join vars = %#v", statement.Vars)
	}
	for index := range want {
		if statement.Vars[index] != want[index] {
			t.Fatalf("pending historical join var %d = %#v, want %#v", index, statement.Vars[index], want[index])
		}
	}
}
