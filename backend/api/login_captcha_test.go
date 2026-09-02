package api

import (
	"backend/cluster"
	"backend/config"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestReleaseCaptchaRoutesNeverUseLocalFallback(t *testing.T) {
	previous := config.Config
	config.Config = &config.Configuration{Server: config.ServerConfig{Mode: "test"}}
	gin.SetMode(gin.TestMode)
	_ = cluster.Init(context.Background(), cluster.Options{})
	t.Cleanup(func() { config.Config = previous; _ = cluster.Init(context.Background(), cluster.Options{}) })
	engine := gin.New()
	LoadRoutesForMode(engine, &gorm.DB{}, nil, gin.ReleaseMode)
	for _, path := range []string{"/api/login/captcha", "/api/member/login/captcha"} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != 503 || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("explicit release route used local storage at %s: %d", path, response.Code)
		}
	}
	// Real shared storage may serve release challenges even when global mode
	// differs; neither login endpoint may authenticate without proof.
	server := miniredis.RunT(t)
	if err := cluster.Init(context.Background(), cluster.Options{Addr: server.Addr(), Prefix: "captcha-routes"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/login/captcha", "/api/member/login/captcha"} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != 200 || !strings.Contains(response.Body.String(), "data:image/png;base64,") {
			t.Fatalf("challenge route failed at %s: %d", path, response.Code)
		}
	}
	for _, path := range []string{"/api/login", "/api/member/login"} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"username":"someone","password":"Password#2026"}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		if response.Code != 400 || !strings.Contains(response.Body.String(), "验证码") {
			t.Fatalf("login route bypassed captcha at %s: %d", path, response.Code)
		}
	}
}
