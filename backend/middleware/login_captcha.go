package middleware

import (
	"backend/captcha"
	"backend/cluster"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var captchaWindows = struct {
	sync.Mutex
	items map[string]rateWindow
}{items: make(map[string]rateWindow)}

// RequireSharedLoginCaptcha preserves an explicit release route configuration
// even if another entrypoint has not synchronized Gin/config's global mode.
// It can only tighten the policy; it cannot disable captcha verification.
func RequireSharedLoginCaptcha(serverMode string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if serverMode == gin.ReleaseMode && cluster.Client() == nil {
			c.Header("Cache-Control", "no-store")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"message": "验证码服务暂不可用，请稍后重试"})
			return
		}
		c.Next()
	}
}

// LoginCaptchaRateLimit has its own quota: requesting/refreshing an image must
// not consume the ten-per-minute password-attempt allowance.
func LoginCaptchaRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		allowed, retry, err := cluster.AllowFixedWindow(c.Request.Context(), "login-captcha", c.ClientIP(), 20, time.Minute)
		if err != nil {
			if !captcha.LocalFallbackAllowed() {
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"message": "验证码服务暂不可用，请稍后重试"})
				return
			}
			now := time.Now()
			captchaWindows.Lock()
			for ip, window := range captchaWindows.items {
				if now.Sub(window.started) >= time.Minute {
					delete(captchaWindows.items, ip)
				}
			}
			window, exists := captchaWindows.items[c.ClientIP()]
			if !exists && len(captchaWindows.items) >= 4096 {
				captchaWindows.Unlock()
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"message": "验证码服务繁忙，请稍后重试"})
				return
			}
			if !exists {
				window.started = now
			}
			allowed = window.count < 20
			if allowed {
				window.count++
				captchaWindows.items[c.ClientIP()] = window
			}
			retry = time.Minute - now.Sub(window.started)
			captchaWindows.Unlock()
		}
		if !allowed {
			seconds := int(retry.Seconds()) + 1
			if seconds < 1 {
				seconds = 1
			}
			c.Header("Retry-After", strconv.Itoa(seconds))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"message": "验证码刷新过于频繁，请稍后再试"})
			return
		}
		c.Next()
	}
}
