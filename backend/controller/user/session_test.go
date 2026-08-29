package user

import (
	"backend/config"
	"backend/sessionauth"
	"backend/utils"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestWriteSessionCookieAdaptsSecureFlagToEnvironment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := config.Config
	t.Cleanup(func() { config.Config = previous })

	for _, test := range []struct {
		name       string
		mode       string
		requestURL string
		secure     bool
	}{
		{name: "local LAN HTTP", mode: "debug", requestURL: "http://192.168.31.84/api/member/login", secure: false},
		{name: "release behind TLS proxy", mode: "release", requestURL: "http://127.0.0.1/api/member/login", secure: true},
		{name: "direct local TLS", mode: "debug", requestURL: "https://localhost/api/member/login", secure: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			config.Config = &config.Configuration{
				Server: config.ServerConfig{Mode: test.mode},
				JWT:    config.JWTConfig{Expire: 3600},
			}
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Request = httptest.NewRequest(http.MethodPost, test.requestURL, nil)

			writeSessionCookie(context, sessionauth.ScopeMember, "signed-token")

			cookies := response.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatalf("cookies = %d, want 1", len(cookies))
			}
			cookie := cookies[0]
			if cookie.Name != sessionauth.MemberCookieName || cookie.Value != "signed-token" {
				t.Fatalf("unexpected session cookie: %#v", cookie)
			}
			if cookie.Secure != test.secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
				t.Fatalf("cookie protections = %#v", cookie)
			}
			if got := response.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("Cache-Control = %q", got)
			}
		})
	}
}

func TestClearSessionCookieUsesMatchingScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := config.Config
	config.Config = &config.Configuration{Server: config.ServerConfig{Mode: "debug"}}
	t.Cleanup(func() { config.Config = previous })

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPost, "http://localhost/api/logout", nil)
	clearSessionCookie(context, sessionauth.ScopeManagement)

	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionauth.ManagementCookieName || cookies[0].MaxAge >= 0 {
		t.Fatalf("unexpected expired cookie: %#v", cookies)
	}
}

func TestWriteVersionedSessionCookieUsesCommittedAuthVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := config.Config
	config.Config = &config.Configuration{
		Server: config.ServerConfig{Mode: "release"},
		JWT:    config.JWTConfig{Expire: 3600},
	}
	t.Cleanup(func() { config.Config = previous })
	utils.InitJWT("room-activation-cookie-test-secret-long-enough", 3600)

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/member/room/join", nil)
	if err := writeVersionedSessionCookie(context, sessionauth.ScopeMember, 42, 9); err != nil {
		t.Fatalf("writeVersionedSessionCookie: %v", err)
	}

	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	claims, err := utils.ParseToken(cookies[0].Value)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.UserID != 42 || claims.AuthVersion != 9 {
		t.Fatalf("claims = user %d version %d, want user 42 version 9", claims.UserID, claims.AuthVersion)
	}
	if !cookies[0].Secure || !cookies[0].HttpOnly || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("rotated cookie protections are incomplete: %#v headers=%v", cookies[0], response.Header())
	}
}
