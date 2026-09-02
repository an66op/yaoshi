package tenant

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

func TestTenantMembersRejectInvalidIdentityBeforeDatabaseAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name  string
		value any
	}{
		{name: "missing account"},
		{name: "wrong context type", value: uint64(52)},
		{name: "empty account", value: user.User{}},
		{name: "missing user ID", value: user.User{WorkspaceID: 7, Role: "tenant"}},
		{name: "missing workspace", value: user.User{UserID: 52, Role: "tenant"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Request = httptest.NewRequest(http.MethodGet, "/api/tenant/users?workspace_id=999&tenant_id=999&room_code=99999", nil)
			if test.value != nil {
				context.Set("tenant_user", test.value)
			}
			context.Set("tenant_id", uint64(999))
			context.Set("workspace_id", uint64(999))
			// A nil database also proves rejected identities never reach a query.
			NewWorkspaceHandler(nil).DirectUsers(context)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401; response = %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestTenantMembersRejectInvalidUserIDBeforeDatabaseAccess(t *testing.T) {
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
			context.Request = httptest.NewRequest(http.MethodGet, "/api/tenant/users?user_id="+test.value+"&workspace_id=999&room_code=99999", nil)
			context.Set("tenant_user", user.User{UserID: 52, WorkspaceID: 7, Role: "tenant", Status: 1})
			// An invalid explicit ID must not fall back to listing every member.
			NewWorkspaceHandler(nil).DirectUsers(context)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; response = %s", response.Code, response.Body.String())
			}
		})
	}
}

type tenantMemberQueryCapture struct {
	SQL  string
	Vars []any
}

func captureTenantMemberHandlerQuery(t *testing.T, path string) tenantMemberQueryCapture {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	var captures []tenantMemberQueryCapture
	if err := db.Callback().Query().After("gorm:query").Register("test:tenant_member_scope", func(tx *gorm.DB) {
		captures = append(captures, tenantMemberQueryCapture{SQL: tx.Statement.SQL.String(), Vars: append([]any(nil), tx.Statement.Vars...)})
		// Stop the real handler/service path at its first query. DryRun prevents
		// network access and this sentinel prevents the later result scan.
		tx.AddError(errors.New("test stopped after capturing member roster query"))
	}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, path, nil)
	context.Set("tenant_user", user.User{UserID: 52, WorkspaceID: 7, Role: "tenant", Status: 1})
	context.Set("tenant_id", uint64(999))
	context.Set("workspace_id", uint64(999))
	context.Set("room_scope", "agent:999")
	NewWorkspaceHandler(db).DirectUsers(context)
	if response.Code != http.StatusInternalServerError || len(captures) != 1 {
		t.Fatalf("expected intercepted roster query; status = %d, queries = %#v, response = %s", response.Code, captures, response.Body.String())
	}
	return captures[0]
}

func TestTenantMembersIgnoreClientRoomSelectors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	baseline := captureTenantMemberHandlerQuery(t, "/api/tenant/users?query=alice&status=all&page=2&page_size=5")
	forged := captureTenantMemberHandlerQuery(t, "/api/tenant/users?query=alice&status=all&page=2&page_size=5&workspace_id=999&agent_id=999&tenant_id=999&room_code=99999&room_scope=agent:999")
	if !reflect.DeepEqual(baseline, forged) {
		t.Fatalf("client room selectors changed authorized query:\nbaseline = %#v\nforged = %#v", baseline, forged)
	}
	if !strings.Contains(forged.SQL, "workspace_memberships") || !strings.Contains(forged.SQL, "workspace_id") {
		t.Fatalf("roster omitted exact room membership boundary: %#v", forged)
	}
	var roomBindings int
	for _, value := range forged.Vars {
		if value == uint64(7) {
			roomBindings++
		}
		if value == uint64(999) || value == "999" || value == "99999" || value == "agent:999" {
			t.Fatalf("client room selector reached roster query: %#v", forged)
		}
	}
	if roomBindings < 2 {
		t.Fatalf("current and historical roster scopes must both bind authenticated workspace 7: %#v", forged)
	}
}

func TestTenantMembersExactUserIDPreservesAuthenticatedRoomScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	baseline := captureTenantMemberHandlerQuery(t, "/api/tenant/users?user_id=1234")
	forged := captureTenantMemberHandlerQuery(t, "/api/tenant/users?user_id=1234&workspace_id=999&agent_id=999&tenant_id=999&room_code=99999&room_scope=agent:999")
	if !reflect.DeepEqual(baseline, forged) {
		t.Fatalf("client room selectors changed exact member query:\nbaseline = %#v\nforged = %#v", baseline, forged)
	}
	if !strings.Contains(forged.SQL, "workspace_memberships") || !strings.Contains(forged.SQL, `AND "user".user_id =`) {
		t.Fatalf("exact member filter must narrow, not replace, the room roster: %#v", forged)
	}
	var roomBindings, memberBindings int
	for _, value := range forged.Vars {
		switch value {
		case uint64(7):
			roomBindings++
		case uint64(1234):
			memberBindings++
		case uint64(999), "999", "99999", "agent:999":
			t.Fatalf("client room selector reached exact member query: %#v", forged)
		}
	}
	if roomBindings < 2 || memberBindings != 1 {
		t.Fatalf("exact member query must retain both authenticated room bindings and member ID 1234: %#v", forged)
	}
}
