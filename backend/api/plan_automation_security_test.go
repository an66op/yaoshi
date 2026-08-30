package api

import (
	"backend/config"
	"backend/data/models/user"
	"backend/utils"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPlanAutomationRoutesRequireAuthenticationAndRejectForeignOrigins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := config.Config
	config.Config = &config.Configuration{Server: config.ServerConfig{Mode: "test", AllowedOrigins: []string{"https://operator.example"}}}
	t.Cleanup(func() { config.Config = previous })
	engine := gin.New()
	engine.Use(Cors())
	LoadRoutes(engine, &gorm.DB{}, nil)
	for _, test := range []struct{ method, path string }{
		{http.MethodGet, "/api/admin/plan-automation?workspace_id=17"},
		{http.MethodPut, "/api/admin/plan-automation"},
		{http.MethodPost, "/api/admin/plan-automation/preview"},
	} {
		for _, origin := range []string{"https://operator.example", "https://foreign.example"} {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(`{"workspace_id":17,"enabled":true}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", origin)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			want := http.StatusUnauthorized
			if origin == "https://foreign.example" {
				want = http.StatusForbidden
			}
			if response.Code != want {
				t.Fatalf("%s %s origin=%s status=%d want=%d", test.method, test.path, origin, response.Code, want)
			}
		}
	}
}

func TestPlanAutomationRoutesRejectAuthenticatedTenantAgentAndMember(t *testing.T) {
	gin.SetMode(gin.TestMode)
	utils.InitJWT("plan-automation-role-boundary-test-secret", 3600)
	token, err := utils.GenerateToken(42, 7)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"tenant", "agent", "member"} {
		t.Run(role, func(t *testing.T) {
			db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable"}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
			if err != nil {
				t.Fatal(err)
			}
			// Populate only the role middleware's account read. Any handler or
			// audit write would be a regression; no real database is contacted.
			if err := db.Callback().Query().Before("gorm:query").Register("test:plan_role", func(tx *gorm.DB) {
				account, ok := tx.Statement.Dest.(*user.User)
				if !ok {
					t.Fatalf("non-admin reached business query: %T", tx.Statement.Dest)
				}
				*account = user.User{WorkspaceID: 17, Role: role, Status: 1, AuthVersion: 7}
				account.UserID = 42
			}); err != nil {
				t.Fatal(err)
			}
			engine := gin.New()
			LoadRoutes(engine, db, nil)
			for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPost} {
				path := "/api/admin/plan-automation"
				if method == http.MethodPost {
					path += "/preview"
				}
				request := httptest.NewRequest(method, path, strings.NewReader(`{"workspace_id":17,"enabled":true}`))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Authorization", "Bearer "+token)
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				if response.Code != http.StatusForbidden {
					t.Fatalf("role=%s %s status=%d body=%s", role, method, response.Code, response.Body.String())
				}
			}
		})
	}
}
