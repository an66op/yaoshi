package api

import (
	"backend/data/models/user"
	"backend/utils"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestSourceDiagnosticRoutesRequirePlatformAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	utils.InitJWT("source-diagnostic-role-fixture", 3600)
	token, err := utils.GenerateToken(42, 7)
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []struct{ method, path string }{{http.MethodGet, "/api/admin/source-diagnostics"}, {http.MethodPost, "/api/admin/source-diagnostics/probe"}} {
		t.Run("anonymous"+route.method, func(t *testing.T) {
			engine := gin.New()
			LoadRoutes(engine, &gorm.DB{}, nil)
			request := httptest.NewRequest(route.method, route.path, strings.NewReader(`{"source_key":"163:169"}`))
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("anonymous status %d", response.Code)
			}
		})
		for _, test := range []struct {
			role    string
			status  int
			version uint64
			want    int
		}{{"member", 1, 7, 403}, {"agent", 1, 7, 403}, {"tenant", 1, 7, 403}, {"admin", 0, 7, 401}, {"admin", 1, 8, 401}} {
			t.Run(test.role+route.method, func(t *testing.T) {
				account := user.User{Role: test.role, Status: test.status, AuthVersion: test.version}
				account.UserID = 42
				db, evidence := sgSSCBackfillSecurityDatabase(t, account, false)
				engine := gin.New()
				LoadRoutes(engine, db, nil)
				request := httptest.NewRequest(route.method, route.path, strings.NewReader(`{"source_key":"163:169"}`))
				request.Header.Set("Authorization", "Bearer "+token)
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				if response.Code != test.want || evidence.businessReads != 0 || evidence.businessWrites != 0 || len(evidence.auditIntents) != 0 {
					t.Fatalf("role bypass status=%d evidence=%+v", response.Code, evidence)
				}
			})
		}
	}
}

func TestSourceDiagnosticInvalidProbeKeepsOnlyGenericAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	utils.InitJWT("source-diagnostic-audit-fixture", 3600)
	token, err := utils.GenerateToken(42, 7)
	if err != nil {
		t.Fatal(err)
	}
	account := user.User{Role: "admin", Status: 1, AuthVersion: 7, Username: "fixture-admin"}
	account.UserID = 42
	db, evidence := sgSSCBackfillSecurityDatabase(t, account, true)
	engine := gin.New()
	LoadRoutes(engine, db, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/admin/source-diagnostics/probe", strings.NewReader(`{"source_key":"http://localhost","sign":"must-not-leak"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || evidence.businessReads != 0 || evidence.businessWrites != 0 || len(evidence.auditIntents) != 1 || len(evidence.auditCompletions) != 1 {
		t.Fatalf("invalid probe mutated business state or lost audit: status=%d evidence=%+v", response.Code, evidence)
	}
	if strings.Contains(response.Body.String(), "must-not-leak") {
		t.Fatal("echoed submitted signature")
	}
}
