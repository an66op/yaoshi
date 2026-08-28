package api

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestPrivilegedRouteBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	LoadRoutes(engine, &gorm.DB{}, nil)

	routes := engine.Routes()
	registered := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		registered[route.Method+" "+route.Path] = struct{}{}
		if strings.Contains(route.Path, "/data-lifecycle") && !strings.HasPrefix(route.Path, "/api/admin/data-lifecycle") {
			t.Fatalf("data lifecycle route escaped the platform admin surface: %s %s", route.Method, route.Path)
		}
		if strings.HasPrefix(route.Path, "/api/agent/") &&
			(strings.Contains(route.Path, ":agentID") || strings.Contains(route.Path, ":workspaceID") || strings.Contains(route.Path, ":roomScope")) {
			t.Fatalf("agent route accepts a browser-selected room identity: %s %s", route.Method, route.Path)
		}
		if strings.HasPrefix(route.Path, "/api/tenant/rooms/:agentID/") && route.Path != "/api/tenant/rooms/:agentID/settings" {
			t.Fatalf("tenant route exposes a child agent's operational data: %s %s", route.Method, route.Path)
		}
	}

	required := []string{
		"POST /api/logout",
		"GET /api/session",
		"POST /api/session/refresh",
		"POST /api/member/logout",
		"POST /api/member/session/refresh",
		"GET /api/member/chat/redpackets/available",
		"POST /api/admin/reconciliation/bets/:id/refund",
		"GET /api/admin/data-lifecycle/policies",
		"GET /api/admin/data-lifecycle/summary",
		"PUT /api/admin/data-lifecycle/policies/:dataClass",
		"POST /api/admin/data-lifecycle/preview",
		"POST /api/admin/data-lifecycle/execute",
		"GET /api/admin/data-lifecycle/runs",
		"GET /api/admin/data-lifecycle/runs/:requestID",
		"GET /api/admin/data-lifecycle/runs/:requestID/archives",
		"POST /api/admin/data-lifecycle/runs/:requestID/restore-soft-deleted",
		"POST /api/admin/data-lifecycle/runs/:requestID/restore-robot-archive",
		"GET /api/agent/applications",
		"GET /api/agent/chat/messages",
		"GET /api/agent/robots",
		"POST /api/agent/robots/reset",
		"GET /api/agent/games",
		"GET /api/tenant/applications",
		"GET /api/tenant/chat/messages",
		"GET /api/tenant/robots",
		"POST /api/tenant/robots/reset",
		"GET /api/tenant/games",
		"POST /api/admin/robots/reset",
		"POST /api/admin/sources/:group/test",
		"GET /api/admin/robot-workspaces",
		"GET /api/admin/robot-workspaces/:id/games",
		"GET /api/member/plans",
		"GET /api/member/plans/:gameID",
		"GET /api/admin/plans",
		"POST /api/admin/plans",
		"PUT /api/admin/plans/:id",
		"DELETE /api/admin/plans/:id",
		"GET /api/tenant/plans",
		"POST /api/tenant/plans",
		"PUT /api/tenant/plans/:id",
		"DELETE /api/tenant/plans/:id",
		"GET /api/agent/plans",
		"POST /api/agent/plans",
		"PUT /api/agent/plans/:id",
		"DELETE /api/agent/plans/:id",
	}
	for _, route := range required {
		if _, ok := registered[route]; !ok {
			t.Errorf("required scoped route is not registered: %s", route)
		}
	}

	for _, forbidden := range []string{
		"POST /api/tenant/reconciliation/bets/:id/refund",
		"POST /api/agent/reconciliation/bets/:id/refund",
		"GET /api/tenant/data-lifecycle/policies",
		"POST /api/tenant/data-lifecycle/preview",
		"POST /api/tenant/data-lifecycle/execute",
		"GET /api/agent/data-lifecycle/policies",
		"POST /api/agent/data-lifecycle/preview",
		"POST /api/agent/data-lifecycle/execute",
		"POST /api/tenant/sources/:group/test",
		"POST /api/agent/sources/:group/test",
		"GET /api/tenant/rooms/:agentID/applications",
		"GET /api/tenant/rooms/:agentID/chat/messages",
		"GET /api/tenant/rooms/:agentID/robots",
		"GET /api/tenant/rooms/:agentID/games",
		"GET /api/tenant/rooms/:agentID/plans",
		"GET /api/agent/rooms/:workspaceID/plans",
	} {
		if _, ok := registered[forbidden]; ok {
			t.Errorf("forbidden cross-scope route is registered: %s", forbidden)
		}
	}
}
