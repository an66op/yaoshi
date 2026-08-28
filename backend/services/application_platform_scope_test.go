package services

import (
	"backend/data/models/application"
	apperrors "backend/errors"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func applicationPlatformDryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestPlatformApplicationQueryExcludesJoin(t *testing.T) {
	db := applicationPlatformDryRunDB(t)
	var rows []application.Application
	statement := scopedApplicationQuery(db, 0, true).Find(&rows).Statement
	sql := strings.ToLower(statement.SQL.String())
	if !strings.Contains(sql, "workspace_id >") || !strings.Contains(sql, "request_type <>") {
		t.Fatalf("platform application scope is not fail-closed: %s", sql)
	}
	if len(statement.Vars) != 1 || statement.Vars[0] != "join" {
		t.Fatalf("platform exclusion vars = %#v, want [join]", statement.Vars)
	}
}

func TestRoomApplicationQueryKeepsJoin(t *testing.T) {
	db := applicationPlatformDryRunDB(t)
	var rows []application.Application
	statement := scopedApplicationQuery(db, 41, false).Find(&rows).Statement
	sql := strings.ToLower(statement.SQL.String())
	if !strings.Contains(sql, "workspace_id =") {
		t.Fatalf("room application scope is missing: %s", sql)
	}
	if strings.Contains(sql, "request_type <>") {
		t.Fatalf("room owner lost its join queue: %s", sql)
	}
	if len(statement.Vars) != 1 || statement.Vars[0] != uint64(41) {
		t.Fatalf("room scope vars = %#v, want [41]", statement.Vars)
	}
}

func TestPlatformCannotCreateOrProcessJoin(t *testing.T) {
	if platformMayProcessApplication("join") {
		t.Fatal("platform unexpectedly allowed to process join applications")
	}
	if !platformMayProcessApplication("credit") {
		t.Fatal("platform wallet application behavior changed")
	}
	service := &ApplicationAdminService{}
	if _, err := service.CreateForPlatform(CreateApplicationInput{RequestType: "join"}); err == nil || !apperrors.IsBusinessError(err) {
		t.Fatalf("platform join creation error = %#v, want business rejection", err)
	}
}

func TestJoinApplicationEventTargetsRoomOwnerOnlyAndCarriesType(t *testing.T) {
	if applicationEventIncludesPlatformAdmins("join") {
		t.Fatal("join event would still notify platform administrators")
	}
	if !applicationEventIncludesPlatformAdmins("debit") {
		t.Fatal("non-join application event behavior changed")
	}
	payload := applicationEventPayload(8, 13, "pending", "join")
	if payload["workspace_id"] != uint64(8) || payload["application_id"] != uint64(13) || payload["status"] != "pending" || payload["request_type"] != "join" {
		t.Fatalf("application event payload = %#v", payload)
	}
}

func TestOnlyMembersCanEnterAnotherRoom(t *testing.T) {
	if !roomEntryApplicantRoleAllowed("member") {
		t.Fatal("ordinary member was unexpectedly blocked from room entry")
	}
	for _, role := range []string{"agent", "tenant", "admin", ""} {
		if roomEntryApplicantRoleAllowed(role) {
			t.Fatalf("management role %q can still move through member room entry", role)
		}
	}
}
