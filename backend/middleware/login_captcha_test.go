package middleware

import (
	"backend/cluster"
	"backend/config"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
)

func TestCaptchaRateLimitIsSeparateFromPasswordAttempts(t *testing.T) {
	server := miniredis.RunT(t)
	if err := cluster.Init(context.Background(), cluster.Options{Addr: server.Addr(), Prefix: "captcha-rate", Required: true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cluster.Init(context.Background(), cluster.Options{}) })
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/captcha", LoginCaptchaRateLimit(), func(c *gin.Context) { c.Status(204) })
	engine.POST("/login", AuthRateLimit(), func(c *gin.Context) { c.Status(204) })
	request := func(method, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		return w
	}
	for i := 0; i < 20; i++ {
		if response := request("GET", "/captcha"); response.Code != 204 {
			t.Fatalf("captcha request %d status=%d", i, response.Code)
		}
	}
	if w := request("GET", "/captcha"); w.Code != 429 || w.Header().Get("Retry-After") == "" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("captcha quota not enforced")
	}
	for i := 0; i < 10; i++ {
		if response := request("POST", "/login"); response.Code != 204 {
			t.Fatalf("image consumed password quota at attempt%d", i)
		}
	}
	if w := request("POST", "/login"); w.Code != 429 {
		t.Fatal("existing auth quota changed")
	}
	server.FastForward(time.Minute)
	if w := request("GET", "/captcha"); w.Code != 204 {
		t.Fatal("captcha quota did not expire")
	}
	server.SetError("ERR unavailable")
	if w := request("GET", "/captcha"); w.Code != 503 {
		t.Fatal("captcha shared rate limiter failed open")
	}
}

func TestCaptchaRateLimitLocalFallbackIsBoundedAndExpires(t *testing.T) {
	previous := config.Config
	config.Config = &config.Configuration{Server: config.ServerConfig{Mode: "test"}}
	_ = cluster.Init(context.Background(), cluster.Options{})
	t.Cleanup(func() {
		config.Config = previous
		captchaWindows.Lock()
		captchaWindows.items = make(map[string]rateWindow)
		captchaWindows.Unlock()
	})
	captchaWindows.Lock()
	captchaWindows.items = make(map[string]rateWindow)
	for i := 0; i < 4096; i++ {
		captchaWindows.items[time.Unix(int64(i), 0).String()] = rateWindow{started: time.Now()}
	}
	captchaWindows.Unlock()
	engine := gin.New()
	engine.GET("/captcha", LoginCaptchaRateLimit(), func(c *gin.Context) { c.Status(204) })
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/captcha", nil))
	if w.Code != 503 {
		t.Fatal("unbounded local captcha rate subjects")
	}
	captchaWindows.Lock()
	for k, v := range captchaWindows.items {
		v.started = time.Now().Add(-time.Minute)
		captchaWindows.items[k] = v
	}
	captchaWindows.Unlock()
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/captcha", nil))
	if w.Code != 204 {
		t.Fatal("expired local rate entries not reclaimed")
	}
	config.Config.Server.Mode = "release"
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/captcha", nil))
	if w.Code != 503 {
		t.Fatal("release captcha limiter used local fallback")
	}
}
