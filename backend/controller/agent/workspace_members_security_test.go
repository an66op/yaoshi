package agent

import (
	"backend/data/models/user"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAgentMembersRejectInvalidIdentityBeforeDatabaseAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name  string
		value any
	}{
		{name: "missing account"},
		{name: "wrong context type", value: uint64(41)},
		{name: "empty account", value: user.User{}},
		{name: "missing user ID", value: user.User{WorkspaceID: 7, Role: "agent"}},
		{name: "missing workspace", value: user.User{UserID: 41, Role: "agent"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Request = httptest.NewRequest(http.MethodGet, "/api/agent/users?workspace_id=999&agent_id=999&room_code=99999", nil)
			if test.value != nil {
				context.Set("agent_user", test.value)
			}
			context.Set("agent_id", uint64(999))
			context.Set("workspace_id", uint64(999))
			// A nil database also proves rejected identities never reach a query.
			NewWorkspaceHandler(nil).Users(context)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401; response = %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestAgentMembersRejectInvalidUserIDBeforeDatabaseAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name  string
		value string
	}{
		{name: "zero", value: "0"},
		{name: "not numeric", value: "not-a-number"},
		{name: "negative", value: "-1"},
		{name: "fractional", value: "1.5"},
		{name: "signed database ID overflow", value: "9223372036854775808"},
		{name: "unsigned overflow", value: "18446744073709551616"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Request = httptest.NewRequest(http.MethodGet, "/api/agent/users?user_id="+test.value+"&workspace_id=999&room_code=99999", nil)
			context.Set("agent_user", user.User{UserID: 41, WorkspaceID: 7, Role: "agent", Status: 1})
			// An invalid explicit ID must not fall back to listing every member.
			NewWorkspaceHandler(nil).Users(context)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; response = %s", response.Code, response.Body.String())
			}
		})
	}
}

type agentMemberQueryCapture struct {
	SQL  string
	Vars []any
}

func captureAgentMemberHandlerQueries(t *testing.T, path string) []agentMemberQueryCapture {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	var captures []agentMemberQueryCapture
	if err := db.Callback().Query().After("gorm:query").Register("test:agent_member_scope", func(tx *gorm.DB) {
		captures = append(captures, agentMemberQueryCapture{SQL: tx.Statement.SQL.String(), Vars: append([]any(nil), tx.Statement.Vars...)})
		if account, ok := tx.Statement.Dest.(*user.User); ok {
			// Supply only the authenticated standalone agent lookup. No query is
			// executed: the roster count below stops before its result scan.
			*account = user.User{UserID: 41, WorkspaceID: 7, Role: "agent", Status: 1}
			tx.RowsAffected = 1
			return
		}
		tx.AddError(errors.New("test stopped after capturing member roster query"))
	}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, path, nil)
	context.Set("agent_user", user.User{UserID: 41, WorkspaceID: 7, Role: "agent", Status: 1})
	context.Set("agent_id", uint64(999))
	context.Set("workspace_id", uint64(999))
	context.Set("room_scope", "agent:999")
	NewWorkspaceHandler(db).Users(context)
	if response.Code != http.StatusInternalServerError || len(captures) != 2 {
		t.Fatalf("expected agent lookup then intercepted roster query; status = %d, queries = %#v, response = %s", response.Code, captures, response.Body.String())
	}
	return captures
}

func TestAgentMembersIgnoreClientRoomSelectors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	baseline := captureAgentMemberHandlerQueries(t, "/api/agent/users?query=alice&status=all&page=2&page_size=5")
	forged := captureAgentMemberHandlerQueries(t, "/api/agent/users?query=alice&status=all&page=2&page_size=5&workspace_id=999&agent_id=999&tenant_id=999&room_code=99999&room_scope=agent:999")
	if !reflect.DeepEqual(baseline, forged) {
		t.Fatalf("client room selectors changed authorized queries:\nbaseline = %#v\nforged = %#v", baseline, forged)
	}
	var agentIDBound bool
	for _, value := range forged[0].Vars {
		if value == uint64(41) {
			agentIDBound = true
		}
	}
	if !agentIDBound {
		t.Fatalf("authenticated agent ID missing from lookup: %#v", forged[0])
	}
	roster := forged[1]
	if !strings.Contains(roster.SQL, "workspace_memberships") || !strings.Contains(roster.SQL, "workspace_id") {
		t.Fatalf("roster omitted exact room membership boundary: %#v", roster)
	}
	var roomBindings int
	for _, value := range roster.Vars {
		if value == uint64(7) {
			roomBindings++
		}
		if value == uint64(999) || value == "999" || value == "99999" || value == "agent:999" {
			t.Fatalf("client room selector reached roster query: %#v", roster)
		}
	}
	if roomBindings < 2 {
		t.Fatalf("current and historical roster scopes must both bind authenticated workspace 7: %#v", roster)
	}
}

func TestAgentMembersExactUserIDPreservesAuthenticatedRoomScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	baseline := captureAgentMemberHandlerQueries(t, "/api/agent/users?user_id=1234")
	forged := captureAgentMemberHandlerQueries(t, "/api/agent/users?user_id=1234&workspace_id=999&agent_id=999&tenant_id=999&room_code=99999&room_scope=agent:999")
	if !reflect.DeepEqual(baseline, forged) {
		t.Fatalf("client room selectors changed exact member query:\nbaseline = %#v\nforged = %#v", baseline, forged)
	}
	roster := forged[1]
	if !strings.Contains(roster.SQL, "workspace_memberships") || !strings.Contains(roster.SQL, `AND "user".user_id =`) {
		t.Fatalf("exact member filter must narrow, not replace, the room roster: %#v", roster)
	}
	var roomBindings, memberBindings int
	for _, value := range roster.Vars {
		switch value {
		case uint64(7):
			roomBindings++
		case uint64(1234):
			memberBindings++
		case uint64(999), "999", "99999", "agent:999":
			t.Fatalf("client room selector reached exact member query: %#v", roster)
		}
	}
	if roomBindings < 2 || memberBindings != 1 {
		t.Fatalf("exact member query must retain both authenticated room bindings and member ID 1234: %#v", roster)
	}
}
