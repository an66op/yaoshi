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

func TestPlanStreamActivationRouteRequiresAuthenticationAndTrustedOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := config.Config
	config.Config = &config.Configuration{Server: config.ServerConfig{Mode: "test", AllowedOrigins: []string{"https://member.example"}}}
	t.Cleanup(func() { config.Config = previous })
	engine := gin.New()
	engine.Use(Cors())
	LoadRoutes(engine, &gorm.DB{}, nil)
	for _, test := range []struct {
		origin string
		want   int
	}{
		{origin: "https://member.example", want: http.StatusUnauthorized},
		{origin: "https://foreign.example", want: http.StatusForbidden},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/member/plans/speed-racing/activate", strings.NewReader(`{"workspace_id":999,"position":1,"plan_key":"four-period-five-codes"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", test.origin)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		if response.Code != test.want {
			t.Fatalf("origin=%s status=%d want=%d", test.origin, response.Code, test.want)
		}
	}
}

func TestPlanStreamActivationRouteRejectsAdminAndTenantSessions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	utils.InitJWT("plan-stream-role-boundary-test-secret", 3600)
	token, err := utils.GenerateToken(42, 7)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"admin", "tenant"} {
		t.Run(role, func(t *testing.T) {
			db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable"}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Callback().Query().Before("gorm:query").Register("test:plan_stream_role", func(tx *gorm.DB) {
				account, ok := tx.Statement.Dest.(*user.User)
				if !ok {
					t.Fatalf("management role reached business query: %T", tx.Statement.Dest)
				}
				*account = user.User{UserID: 42, WorkspaceID: 17, Role: role, Status: 1, AuthVersion: 7}
			}); err != nil {
				t.Fatal(err)
			}
			engine := gin.New()
			LoadRoutes(engine, db, nil)
			request := httptest.NewRequest(http.MethodPost, "/api/member/plans/speed-racing/activate", strings.NewReader(`{"position":1,"plan_key":"four-period-five-codes"}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("role=%s status=%d body=%s", role, response.Code, response.Body.String())
			}
		})
	}
}
