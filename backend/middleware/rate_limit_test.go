package middleware

import (
	"testing"
	"time"
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
