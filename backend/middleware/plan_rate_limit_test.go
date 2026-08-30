package middleware

import (
	"backend/cluster"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
)

func TestPlanActivationRateLimitIsSharedUserAndRoomScoped(t *testing.T) {
	server := miniredis.RunT(t)
	if err := cluster.Init(context.Background(), cluster.Options{Addr: server.Addr(), Prefix: "plan-activation-test"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cluster.Close(); _ = cluster.Init(context.Background(), cluster.Options{}) })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		room, _ := strconv.ParseUint(c.GetHeader("Test-Room"), 10, 64)
		c.Set("user_id", uint64(42))
		c.Set("workspace_id", room)
	})
	router.POST("/activate", PlanActivationRateLimit(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := func(room string) int {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/activate", nil)
		r.Header.Set("Test-Room", room)
		router.ServeHTTP(w, r)
		return w.Code
	}
	for i := 0; i < 30; i++ {
		if got := request("7"); got != http.StatusNoContent {
			t.Fatalf("request %d status %d", i, got)
		}
	}
	if got := request("7"); got != http.StatusTooManyRequests {
		t.Fatalf("31st confirmation not rate limited: %d", got)
	}
	if got := request("8"); got != http.StatusNoContent {
		t.Fatalf("another room shared the wrong quota: %d", got)
	}
}
