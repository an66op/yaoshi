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

func TestMemberPaymentAccountCreateRateLimitIsSharedAndIdentityScoped(t *testing.T) {
	server := miniredis.RunT(t)
	if err := cluster.Init(context.Background(), cluster.Options{Addr: server.Addr(), Prefix: "member-payment-create-rate-test"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cluster.Close(); _ = cluster.Init(context.Background(), cluster.Options{}) })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		workspaceID, _ := strconv.ParseUint(c.GetHeader("Test-Workspace"), 10, 64)
		userID, _ := strconv.ParseUint(c.GetHeader("Test-User"), 10, 64)
		c.Set("workspace_id", workspaceID)
		c.Set("user_id", userID)
	})
	router.POST("/payment-accounts", MemberPaymentAccountCreateRateLimit(), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := func(workspaceID, userID string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/payment-accounts", nil)
		req.Header.Set("Test-Workspace", workspaceID)
		req.Header.Set("Test-User", userID)
		req.RemoteAddr = "192.0.2.10:4312"
		router.ServeHTTP(response, req)
		return response
	}
	for attempt := 1; attempt <= 10; attempt++ {
		if got := request("37", "91").Code; got != http.StatusNoContent {
			t.Fatalf("payment account attempt %d status = %d", attempt, got)
		}
	}
	limited := request("37", "91")
	if limited.Code != http.StatusTooManyRequests || limited.Header().Get("Retry-After") == "" {
		t.Fatalf("11th payment account attempt status/retry = %d/%q", limited.Code, limited.Header().Get("Retry-After"))
	}
	if got := request("37", "92").Code; got != http.StatusNoContent {
		t.Fatalf("another member inherited the exhausted quota: %d", got)
	}
	if got := request("38", "91").Code; got != http.StatusNoContent {
		t.Fatalf("server-resolved workspace did not separate the quota: %d", got)
	}
}

func TestMemberPaymentQRCodeReadRateLimitIsAtomicAndMemberScoped(t *testing.T) {
	server := miniredis.RunT(t)
	if err := cluster.Init(context.Background(), cluster.Options{Addr: server.Addr(), Prefix: "member-payment-qr-read-rate-test"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cluster.Close(); _ = cluster.Init(context.Background(), cluster.Options{}) })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		workspaceID, _ := strconv.ParseUint(c.GetHeader("Test-Workspace"), 10, 64)
		userID, _ := strconv.ParseUint(c.GetHeader("Test-User"), 10, 64)
		c.Set("workspace_id", workspaceID)
		c.Set("user_id", userID)
	})
	router.GET("/payment-accounts/1/qr-code", MemberPaymentQRCodeReadRateLimit(), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := func(user string) int {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/payment-accounts/1/qr-code", nil)
		req.Header.Set("Test-Workspace", "37")
		req.Header.Set("Test-User", user)
		router.ServeHTTP(response, req)
		return response.Code
	}
	start := make(chan struct{})
	statuses := make(chan int, 61)
	for index := 0; index < 61; index++ {
		go func() {
			<-start
			statuses <- request("91")
		}()
	}
	close(start)
	allowed, limited := 0, 0
	for index := 0; index < 61; index++ {
		switch status := <-statuses; status {
		case http.StatusNoContent:
			allowed++
		case http.StatusTooManyRequests:
			limited++
		default:
			t.Fatalf("unexpected concurrent QR read status %d", status)
		}
	}
	if allowed != 60 || limited != 1 {
		t.Fatalf("concurrent QR reads allowed/limited = %d/%d, want 60/1", allowed, limited)
	}
	if status := request("92"); status != http.StatusNoContent {
		t.Fatalf("another member inherited the exhausted QR quota: %d", status)
	}
}

func TestMemberPasswordChangeRateLimitsCannotBeReplenishedByChangingIP(t *testing.T) {
	server := miniredis.RunT(t)
	if err := cluster.Init(context.Background(), cluster.Options{Addr: server.Addr(), Prefix: "member-password-rate-test"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cluster.Close(); _ = cluster.Init(context.Background(), cluster.Options{}) })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		t.Fatal(err)
	}
	router.Use(func(c *gin.Context) {
		userID, _ := strconv.ParseUint(c.GetHeader("Test-User"), 10, 64)
		c.Set("user_id", userID)
	})
	router.POST("/password", MemberPasswordChangeRateLimit(), MemberPasswordChangeClientRateLimit(), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := func(userID, remoteAddr string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/password", nil)
		req.Header.Set("Test-User", userID)
		req.RemoteAddr = remoteAddr
		router.ServeHTTP(response, req)
		return response
	}
	for attempt := 1; attempt <= 3; attempt++ {
		if got := request("91", "192.0.2.10:4312").Code; got != http.StatusNoContent {
			t.Fatalf("password attempt %d status = %d", attempt, got)
		}
	}
	limited := request("91", "192.0.2.10:4312")
	if limited.Code != http.StatusTooManyRequests || limited.Header().Get("Retry-After") == "" {
		t.Fatalf("fourth same-client password attempt status/retry = %d/%q", limited.Code, limited.Header().Get("Retry-After"))
	}
	if got := request("91", "192.0.2.11:4312").Code; got != http.StatusNoContent {
		t.Fatalf("fifth member attempt from another trusted IP status = %d", got)
	}
	if got := request("91", "192.0.2.12:4312").Code; got != http.StatusTooManyRequests {
		t.Fatalf("changing trusted IP replenished the per-member quota: %d", got)
	}
	if got := request("92", "192.0.2.10:4312").Code; got != http.StatusNoContent {
		t.Fatalf("another member inherited the quota: %d", got)
	}
}
