package middleware

import (
	"backend/cluster"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type rateWindow struct {
	started time.Time
	count   int
}

// fixedWindowLimiter is the development fallback used only when Redis is not
// configured. Release mode fails closed instead of silently applying a
// per-process limit that could be bypassed through another backend instance.
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
	return l.middlewareWithSubject(namespace, func(c *gin.Context) string { return c.ClientIP() })
}

func (l *fixedWindowLimiter) middlewareWithSubject(namespace string, subject func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := subject(c)
		allowed, retryAfter, err := cluster.AllowFixedWindow(c.Request.Context(), namespace, key, l.limit, l.period)
		if err == nil {
			if allowed {
				c.Next()
				return
			}
			seconds := int(retryAfter.Round(time.Second) / time.Second)
			if seconds < 1 {
				seconds = 1
			}
			c.Header("Retry-After", strconv.Itoa(seconds))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"message": "请求过于频繁，请稍后再试"})
			return
		}
		if cluster.Required() {
			c.Header("Retry-After", "1")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"message": "请求校验服务暂不可用，请稍后再试"})
			return
		}
		if l.allow(namespace+":"+key, time.Now()) {
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
	robotRunLimiter  = newFixedWindowLimiter(6, time.Minute)
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

// RobotRunRateLimit limits manual scheduler triggers per authenticated
// operator. The shared Redis window applies across backend instances; local
// state is only the existing non-release development fallback.
func RobotRunRateLimit() gin.HandlerFunc {
	return robotRunLimiter.middlewareWithSubject("robot-run", func(c *gin.Context) string {
		if userID, exists := c.Get("user_id"); exists {
			if value, ok := userID.(uint64); ok && value > 0 {
				return "user:" + strconv.FormatUint(value, 10)
			}
		}
		return "ip:" + c.ClientIP()
	})
}
