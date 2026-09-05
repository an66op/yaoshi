package api

import (
	"backend/cluster"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
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
		"POST /api/member/games/:id/web-bets",
		"GET /api/member/chat/redpackets/available",
		"POST /api/admin/reconciliation/bets/:id/refund",
		"GET /api/admin/system-logs",
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
		"POST /api/agent/robots/run-once",
		"GET /api/agent/games",
		"GET /api/tenant/applications",
		"GET /api/tenant/chat/messages",
		"GET /api/tenant/robots",
		"POST /api/tenant/robots/reset",
		"POST /api/tenant/robots/run-once",
		"GET /api/tenant/games",
		"POST /api/admin/robots/reset",
		"POST /api/admin/room-activity/run-once",
		"POST /api/admin/sources/:group/test",
		"GET /api/admin/source-diagnostics",
		"POST /api/admin/source-diagnostics/probe",
		"GET /api/admin/sources/sg-ssc/backfill",
		"POST /api/admin/sources/sg-ssc/backfill",
		"GET /api/admin/robot-workspaces",
		"GET /api/admin/robot-workspaces/:id/games",
		"GET /api/member/plans",
		"GET /api/member/plans/:gameID",
		"POST /api/member/plans/:gameID/activate",
		"GET /api/member/payment-accounts/:id/qr-code",
		"POST /api/member/payment-accounts",
		"POST /api/member/password",
		"GET /api/admin/plans",
		"POST /api/admin/plans",
		"PUT /api/admin/plans/:id",
		"DELETE /api/admin/plans/:id",
		"GET /api/admin/plan-automation",
		"PUT /api/admin/plan-automation",
		"POST /api/admin/plan-automation/preview",
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
		"GET /api/tenant/source-diagnostics",
		"POST /api/tenant/source-diagnostics/probe",
		"GET /api/agent/source-diagnostics",
		"POST /api/agent/source-diagnostics/probe",
		"GET /api/member/source-diagnostics",
		"POST /api/member/source-diagnostics/probe",
		"POST /api/agent/sources/:group/test",
		"GET /api/tenant/sources/sg-ssc/backfill",
		"POST /api/tenant/sources/sg-ssc/backfill",
		"GET /api/agent/sources/sg-ssc/backfill",
		"POST /api/agent/sources/sg-ssc/backfill",
		"GET /api/member/sources/sg-ssc/backfill",
		"POST /api/member/sources/sg-ssc/backfill",
		"GET /api/tenant/rooms/:agentID/applications",
		"GET /api/tenant/rooms/:agentID/chat/messages",
		"GET /api/tenant/rooms/:agentID/robots",
		"GET /api/tenant/rooms/:agentID/games",
		"GET /api/tenant/rooms/:agentID/plans",
		"GET /api/agent/rooms/:workspaceID/plans",
		"GET /api/tenant/plan-automation",
		"PUT /api/tenant/plan-automation",
		"POST /api/tenant/plan-automation/preview",
		"GET /api/agent/plan-automation",
		"PUT /api/agent/plan-automation",
		"POST /api/agent/plan-automation/preview",
		"PUT /api/member/plan-automation",
	} {
		if _, ok := registered[forbidden]; ok {
			t.Errorf("forbidden cross-scope route is registered: %s", forbidden)
		}
	}
}

func TestMemberSensitiveWriteRoutesCarryIndependentRateLimits(t *testing.T) {
	payment := memberPaymentAccountCreateRoute(func(c *gin.Context) { c.Status(http.StatusNoContent) })
	if payment.Method != http.MethodPost || payment.Pattern != "/payment-accounts" || len(payment.Middlewares) != 1 {
		t.Fatalf("member payment account create route is not independently limited: %#v", payment)
	}
	qrRead := memberPaymentQRCodeReadRoute(func(c *gin.Context) { c.Status(http.StatusNoContent) })
	if qrRead.Method != http.MethodGet || qrRead.Pattern != "/payment-accounts/:id/qr-code" || len(qrRead.Middlewares) != 1 {
		t.Fatalf("member payment QR read route is not independently limited: %#v", qrRead)
	}
	password := memberPasswordChangeRoute(func(c *gin.Context) { c.Status(http.StatusNoContent) })
	if password.Method != http.MethodPost || password.Pattern != "/password" || len(password.Middlewares) != 2 {
		t.Fatalf("member password route lacks per-user and trusted-client limits: %#v", password)
	}
}

func TestAdminRoomActivityRunOnceRouteUsesSharedOperatorRateLimit(t *testing.T) {
	redisServer := miniredis.RunT(t)
	if err := cluster.Init(context.Background(), cluster.Options{Addr: redisServer.Addr(), Prefix: "admin-room-activity-rate-test"}); err != nil {
		t.Fatalf("initialize test Redis: %v", err)
	}
	t.Cleanup(func() {
		_ = cluster.Close()
		_ = cluster.Init(context.Background(), cluster.Options{})
	})

	route := adminRoomActivityRunOnceRoute(func(c *gin.Context) { c.Status(http.StatusNoContent) })
	if route.Method != http.MethodPost || route.Pattern != "/admin/room-activity/run-once" || len(route.Middlewares) != 1 {
		t.Fatalf("admin room activity route is not protected: %#v", route)
	}

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		userID, _ := strconv.ParseUint(c.GetHeader("X-Test-User"), 10, 64)
		c.Set("user_id", userID)
		c.Next()
	})
	handlers := append([]gin.HandlerFunc{}, route.Middlewares...)
	handlers = append(handlers, route.Handler)
	engine.POST(route.Pattern, handlers...)

	request := func(operator string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, route.Pattern, nil)
		req.Header.Set("X-Test-User", operator)
		engine.ServeHTTP(recorder, req)
		return recorder
	}
	for index := 0; index < 6; index++ {
		if status := request("99001").Code; status != http.StatusNoContent {
			t.Fatalf("request %d status = %d", index+1, status)
		}
	}
	limited := request("99001")
	if limited.Code != http.StatusTooManyRequests || limited.Header().Get("Retry-After") == "" {
		t.Fatalf("seventh request status/retry = %d/%q", limited.Code, limited.Header().Get("Retry-After"))
	}
	if status := request("99002").Code; status != http.StatusNoContent {
		t.Fatalf("another admin inherited the first admin's window: %d", status)
	}
}

func TestRegistrationRoutesRespectServerMode(t *testing.T) {
	for _, test := range []struct {
		mode               string
		wantLegacyRegister bool
	}{
		{mode: gin.DebugMode, wantLegacyRegister: true},
		{mode: gin.TestMode, wantLegacyRegister: true},
		{mode: gin.ReleaseMode, wantLegacyRegister: false},
	} {
		t.Run(test.mode, func(t *testing.T) {
			engine := gin.New()
			LoadRoutesForMode(engine, &gorm.DB{}, nil, test.mode)

			registered := make(map[string]struct{}, len(engine.Routes()))
			for _, route := range engine.Routes() {
				registered[route.Method+" "+route.Path] = struct{}{}
			}

			_, hasLegacyRegister := registered["POST /api/register"]
			if hasLegacyRegister != test.wantLegacyRegister {
				t.Fatalf("legacy register route present = %v, want %v", hasLegacyRegister, test.wantLegacyRegister)
			}
			if _, ok := registered["POST /api/member/register"]; !ok {
				t.Fatal("member registration route must remain available in every server mode")
			}
		})
	}
}
