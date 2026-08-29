package api

import (
	"backend/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCorsAllowsCredentialedConfiguredOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := config.Config
	config.Config = &config.Configuration{Server: config.ServerConfig{
		Mode:           "release",
		AllowedOrigins: []string{"https://wz6688.app"},
	}}
	t.Cleanup(func() { config.Config = previous })

	engine := gin.New()
	engine.Use(Cors())
	engine.GET("/api/session", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	request.Header.Set("Origin", "https://wz6688.app")
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "https://wz6688.app" {
		t.Fatalf("allow origin = %q", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("allow credentials = %q", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Idempotency-Key") {
		t.Fatalf("idempotency header is missing from CORS allow-list: %q", got)
	}
}
