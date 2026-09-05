package api

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(SecurityHeaders())
	engine.GET("/api/member/me", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/member/me", nil))

	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"X-XSS-Protection":       "0",
		"Referrer-Policy":        "no-referrer",
		"Cache-Control":          "no-store",
	} {
		if got := response.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if got := response.Header().Get("Content-Security-Policy"); !strings.Contains(got, "frame-ancestors 'none'") {
		t.Errorf("Content-Security-Policy = %q", got)
	}
	if got := response.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("application duplicated edge-owned HSTS policy: %q", got)
	}
}

func TestSecurityHeadersDoNotMakePublicUploadsPrivate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(SecurityHeaders())
	engine.GET("/api/public/uploads/activities/image.png", func(c *gin.Context) { c.Status(http.StatusOK) })
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/public/uploads/activities/image.png", nil))
	if got := response.Header().Get("Cache-Control"); got != "" {
		t.Fatalf("public upload Cache-Control = %q, want unset", got)
	}
}

func TestRequestBodyLimitRejectsOversizedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(RequestBodyLimit())
	called := false
	engine.POST("/api/member/room/join", func(c *gin.Context) { called = true; c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodPost, "/api/member/room/join", bytes.NewReader(make([]byte, defaultMaxRequestBodyBytes+1)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", response.Code)
	}
	if called {
		t.Fatal("oversized request reached its handler")
	}
}

func TestRequestBodyLimitProtectsChunkedRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(RequestBodyLimit())
	engine.POST("/api/member/room/join", func(c *gin.Context) {
		_, err := io.Copy(io.Discard, c.Request.Body)
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.Status(http.StatusRequestEntityTooLarge)
			return
		}
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/api/member/room/join", bytes.NewReader(make([]byte, defaultMaxRequestBodyBytes+1)))
	request.ContentLength = -1
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("chunked status = %d, want 413", response.Code)
	}
}

func TestRequestBodyLimitAllowsBoundedActivityUpload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(RequestBodyLimit())
	engine.POST("/api/admin/activities/upload", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodPost, "/api/admin/activities/upload", bytes.NewReader(make([]byte, defaultMaxRequestBodyBytes+1)))
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("bounded upload status = %d, want 204", response.Code)
	}
}

func TestRequestBodyLimitAllowsOnlyBoundedMemberQRCodeMultipart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(RequestBodyLimit())
	called := 0
	engine.POST("/api/member/payment-accounts", func(c *gin.Context) { called++; c.Status(http.StatusNoContent) })

	allowed := httptest.NewRequest(http.MethodPost, "/api/member/payment-accounts", bytes.NewReader(make([]byte, defaultMaxRequestBodyBytes+1)))
	allowed.Header.Set("Content-Type", "multipart/form-data; boundary=safe-boundary")
	allowedResponse := httptest.NewRecorder()
	engine.ServeHTTP(allowedResponse, allowed)
	if allowedResponse.Code != http.StatusNoContent {
		t.Fatalf("bounded QR upload status = %d, want 204", allowedResponse.Code)
	}

	oversized := httptest.NewRequest(http.MethodPost, "/api/member/payment-accounts", bytes.NewReader(make([]byte, paymentQRCodeRequestBodyBytes+1)))
	oversized.Header.Set("Content-Type", "multipart/form-data; boundary=safe-boundary")
	oversizedResponse := httptest.NewRecorder()
	engine.ServeHTTP(oversizedResponse, oversized)
	if oversizedResponse.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized QR upload status = %d, want 413", oversizedResponse.Code)
	}
	if called != 1 {
		t.Fatalf("member payment handler calls = %d, want 1", called)
	}
}

func TestSafeRequestLoggerRedactsQueryString(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := gin.DefaultWriter
	var output bytes.Buffer
	gin.DefaultWriter = &output
	t.Cleanup(func() { gin.DefaultWriter = previous })

	engine := gin.New()
	engine.Use(SafeRequestLogger())
	engine.GET("/api/ws", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/ws?ticket=do-not-log-this", nil))

	logged := output.String()
	if !strings.Contains(logged, "/api/ws") {
		t.Fatalf("request path missing from log: %q", logged)
	}
	if strings.Contains(logged, "do-not-log-this") || strings.Contains(logged, "ticket=") {
		t.Fatalf("query string leaked into log: %q", logged)
	}
}
