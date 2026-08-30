package admin

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAdminPlanWorkspaceRequiresExplicitValidatedTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest("GET", "/api/admin/plans?workspace_id=23", nil)

	workspaceID, ok := adminPlanWorkspace(context, 0)
	if !ok || workspaceID != 23 {
		t.Fatalf("admin plan workspace = %d, %t; want 23, true", workspaceID, ok)
	}
	if target, exists := context.Get("target_workspace_id"); !exists || target != uint64(23) {
		t.Fatalf("audit target workspace = %#v, %t", target, exists)
	}
}

func TestPlanAutomationHandlersRejectMissingExplicitRoomBeforeDatabaseAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewPlanHandler(nil)
	for _, test := range []struct {
		method, body string
		call         gin.HandlerFunc
	}{
		{"GET", "", handler.Automation},
		{"PUT", `{"enabled":true,"game_ids":["speed-racing"]}`, handler.SaveAutomation},
		{"POST", `{}`, handler.PreviewAutomation},
		{"PUT", `invalid json`, handler.SaveAutomation},
		{"POST", `invalid json`, handler.PreviewAutomation},
	} {
		response := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(response)
		context.Request = httptest.NewRequest(test.method, "/api/admin/plan-automation", strings.NewReader(test.body))
		context.Request.Header.Set("Content-Type", "application/json")
		test.call(context)
		if response.Code != 400 {
			t.Fatalf("%s %s status=%d", test.method, test.body, response.Code)
		}
	}
}

func TestAdminPlanWorkspaceRejectsMissingTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest("GET", "/api/admin/plans", nil)

	if _, ok := adminPlanWorkspace(context, 0); ok {
		t.Fatal("admin plan request without a target room was accepted")
	}
	if response.Code != 400 {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}
