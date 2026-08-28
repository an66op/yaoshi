package admin

import (
	"net/http/httptest"
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
