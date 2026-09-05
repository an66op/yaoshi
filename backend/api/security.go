package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// Most API payloads are compact JSON. Capping them before a decoder runs
	// prevents an unauthenticated client from making handlers buffer an
	// unbounded request body.
	defaultMaxRequestBodyBytes int64 = 1 << 20
	// Activity artwork allows 8 MiB. Leave room for multipart framing while
	// still bounding temporary disk use.
	uploadMaxRequestBodyBytes int64 = 10 << 20
	// Member QR codes are capped at 4 MiB, with a small allowance for the
	// multipart envelope and account fields.
	paymentQRCodeRequestBodyBytes int64 = 5 << 20
)

// SecurityHeaders applies browser-facing protections to every response. The
// API is not an HTML application and must never be framed or content-sniffed.
// HSTS intentionally belongs to the TLS-terminating edge proxy; emitting it
// here as well would create duplicate, potentially conflicting policies.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "0")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Header("Content-Security-Policy", "default-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
		// Authenticated JSON responses may contain account and financial data.
		// Public uploads and explicitly cacheable draw-feed handlers are left
		// alone; draw-feed handlers can also overwrite this conservative default.
		if strings.HasPrefix(c.Request.URL.Path, "/api/") && !strings.HasPrefix(c.Request.URL.Path, "/api/public/") {
			c.Header("Cache-Control", "no-store")
		}
		c.Next()
	}
}

// RequestBodyLimit bounds request bodies before Gin's JSON or multipart
// binders can buffer them. ContentLength catches ordinary oversized requests
// with a clear 413 response; MaxBytesReader also protects chunked requests.
func RequestBodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil {
			c.Next()
			return
		}
		limit := defaultMaxRequestBodyBytes
		if c.Request.Method == http.MethodPost && c.Request.URL.Path == "/api/admin/activities/upload" {
			limit = uploadMaxRequestBodyBytes
		} else if c.Request.Method == http.MethodPost && c.Request.URL.Path == "/api/member/payment-accounts" &&
			strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "multipart/form-data;") {
			limit = paymentQRCodeRequestBodyBytes
		}
		if c.Request.ContentLength > limit {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"message": "请求内容过大"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}

// SafeRequestLogger preserves useful request telemetry without persisting raw
// query strings. WebSocket tickets and future secret-bearing query parameters
// must not leak into application logs.
func SafeRequestLogger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		latency := param.Latency
		if latency > time.Minute {
			latency = latency.Truncate(time.Second)
		}
		return fmt.Sprintf("[GIN] %v | %3d | %13v | %15s | %-7s %#v\n%s",
			param.TimeStamp.Format("2006/01/02 - 15:04:05"),
			param.StatusCode,
			latency,
			param.ClientIP,
			param.Method,
			param.Request.URL.Path,
			param.ErrorMessage,
		)
	})
}
