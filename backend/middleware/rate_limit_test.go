package middleware

import (
	"backend/cluster"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
)

func TestFixedWindowLimiter(t *testing.T) {
	limiter := newFixedWindowLimiter(2, time.Minute)
	now := time.Now()
	if !limiter.allow("client", now) || !limiter.allow("client", now) {
		t.Fatal("first two requests should pass")
	}
	if limiter.allow("client", now) {
		t.Fatal("third request should be limited")
	}
	if !limiter.allow("client", now.Add(time.Minute)) {
		t.Fatal("a new window should permit the request")
	}
}

func TestRobotRunRateLimitUsesSharedOperatorWindow(t *testing.T) {
	redisServer := miniredis.RunT(t)
	if err := cluster.Init(context.Background(), cluster.Options{Addr: redisServer.Addr(), Prefix: "robot-run-rate-test"}); err != nil {
		t.Fatalf("initialize test Redis: %v", err)
	}
	t.Cleanup(func() {
		_ = cluster.Close()
		_ = cluster.Init(context.Background(), cluster.Options{})
	})

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		userID, _ := strconv.ParseUint(c.GetHeader("X-Test-User"), 10, 64)
		c.Set("user_id", userID)
		c.Next()
	})
	engine.POST("/robots/run-once", RobotRunRateLimit(), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := func(userID string, requestNumber int) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/robots/run-once", nil)
		req.Header.Set("X-Test-User", userID)
		req.RemoteAddr = "192.0.2." + strconv.Itoa(requestNumber+1) + ":4321"
		engine.ServeHTTP(recorder, req)
		return recorder
	}

	for index := 0; index < 6; index++ {
		if status := request("44001", index).Code; status != http.StatusNoContent {
			t.Fatalf("request %d status = %d, want %d", index+1, status, http.StatusNoContent)
		}
	}
	limited := request("44001", 7)
	if limited.Code != http.StatusTooManyRequests || limited.Header().Get("Retry-After") == "" {
		t.Fatalf("seventh request status/retry = %d/%q", limited.Code, limited.Header().Get("Retry-After"))
	}
	if status := request("44002", 8).Code; status != http.StatusNoContent {
		t.Fatalf("another operator shared the rate window: status = %d", status)
	}
}
