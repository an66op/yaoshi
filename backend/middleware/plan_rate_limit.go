package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
)

var planActivationLimiter = newFixedWindowLimiter(30, time.Minute)

// Subscription confirmations are writes and invoke bounded generation work.
// The shared window is scoped to both authenticated identity and selected room.
func PlanActivationRateLimit() gin.HandlerFunc {
	return planActivationLimiter.middlewareWithSubject("plan-activation", func(c *gin.Context) string {
		return fmt.Sprintf("%d:%d", c.GetUint64("user_id"), c.GetUint64("workspace_id"))
	})
}
