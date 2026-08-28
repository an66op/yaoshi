package middleware

import (
	"backend/sessionauth"
	"backend/utils"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuthMiddlewarePrefersCookieAndSupportsBearerMigration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	utils.InitJWT("auth-middleware-cookie-test-secret-long-enough", 3600)
	validToken, err := utils.GenerateToken(42, 7)
	if err != nil {
		t.Fatal(err)
	}

	request := func(cookieValue, bearer string) *httptest.ResponseRecorder {
		engine := gin.New()
		engine.Use(AuthMiddleware())
		engine.GET("/api/member/me", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"user_id": c.GetUint64("user_id")})
		})
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/member/me", nil)
		if cookieValue != "" {
			req.AddCookie(&http.Cookie{Name: sessionauth.MemberCookieName, Value: cookieValue})
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		engine.ServeHTTP(recorder, req)
		return recorder
	}

	if response := request(validToken, ""); response.Code != http.StatusOK {
		t.Fatalf("cookie status = %d, want 200", response.Code)
	}
	if response := request("", validToken); response.Code != http.StatusOK {
		t.Fatalf("bearer status = %d, want 200", response.Code)
	}
	// A stale browser cookie must not be bypassed by injecting a different
	// Authorization header into the same request.
	if response := request("invalid-cookie", validToken); response.Code != http.StatusUnauthorized {
		t.Fatalf("cookie precedence status = %d, want 401", response.Code)
	}
}

func TestRequireCurrentAuthVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("matching version continues", func(t *testing.T) {
		response := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(response)
		context.Set("auth_version", uint64(3))
		if !requireCurrentAuthVersion(context, 3) {
			t.Fatal("matching auth version was rejected")
		}
		if context.IsAborted() {
			t.Fatal("matching auth version aborted request")
		}
	})

	for _, test := range []struct {
		name           string
		claimVersion   any
		accountVersion uint64
	}{
		{name: "old token", claimVersion: uint64(2), accountVersion: 3},
		{name: "missing claim", claimVersion: nil, accountVersion: 3},
		{name: "wrong claim type", claimVersion: int(3), accountVersion: 3},
		{name: "zero account version", claimVersion: uint64(1), accountVersion: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			if test.claimVersion != nil {
				context.Set("auth_version", test.claimVersion)
			}
			if requireCurrentAuthVersion(context, test.accountVersion) {
				t.Fatal("invalid auth version was accepted")
			}
			if !context.IsAborted() {
				t.Fatal("invalid auth version did not abort request")
			}
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestRequireWorkspaceBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	if !requireWorkspaceBinding(context, 8) {
		t.Fatal("bound workspace was rejected")
	}
	if context.IsAborted() {
		t.Fatal("bound workspace aborted request")
	}

	response = httptest.NewRecorder()
	context, _ = gin.CreateTestContext(response)
	if requireWorkspaceBinding(context, 0) {
		t.Fatal("unbound room account was accepted")
	}
	if !context.IsAborted() {
		t.Fatal("unbound room account did not abort request")
	}
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}
