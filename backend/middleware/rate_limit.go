package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type rateWindow struct {
	started time.Time
	count   int
}

// fixedWindowLimiter is intentionally small and local. It protects login and
// connection-ticket endpoints without adding a Redis dependency. For multiple
// backend instances, replace it with a shared Redis limiter during deployment.
type fixedWindowLimiter struct {
	mu      sync.Mutex
	limit   int
	period  time.Duration
	windows map[string]rateWindow
}

func newFixedWindowLimiter(limit int, period time.Duration) *fixedWindowLimiter {
	return &fixedWindowLimiter{limit: limit, period: period, windows: make(map[string]rateWindow)}
}

func (l *fixedWindowLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	window, ok := l.windows[key]
	if !ok || now.Sub(window.started) >= l.period {
		l.windows[key] = rateWindow{started: now, count: 1}
		return true
	}
	if window.count >= l.limit {
		return false
	}
	window.count++
	l.windows[key] = window
	return true
}

func (l *fixedWindowLimiter) middleware(namespace string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if l.allow(namespace+":"+c.ClientIP(), time.Now()) {
			c.Next()
			return
		}
		c.Header("Retry-After", "60")
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"message": "请求过于频繁，请稍后再试"})
	}
}

var (
	authLimiter      = newFixedWindowLimiter(10, time.Minute)
	wsTicketLimiter  = newFixedWindowLimiter(30, time.Minute)
	wsConnectLimiter = newFixedWindowLimiter(30, time.Minute)
	memberBetLimiter = newFixedWindowLimiter(60, time.Minute)
)

// AuthRateLimit limits login and registration attempts per verified client IP.
func AuthRateLimit() gin.HandlerFunc { return authLimiter.middleware("auth") }

// WSTicketRateLimit limits creation of short-lived connection credentials.
func WSTicketRateLimit() gin.HandlerFunc { return wsTicketLimiter.middleware("ws-ticket") }

// WSConnectRateLimit protects the WebSocket upgrade endpoint from churn.
func WSConnectRateLimit() gin.HandlerFunc { return wsConnectLimiter.middleware("ws-connect") }

// MemberBetRateLimit limits financial write requests. It is intentionally more
// generous than auth protection so normal multi-line tickets remain usable.
func MemberBetRateLimit() gin.HandlerFunc { return memberBetLimiter.middleware("member-bet") }
