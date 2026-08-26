package middleware

import (
	"backend/data/models/audit"
	"backend/data/models/user"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PrivilegedAudit records every admin/agent mutation after authorization.
// Audit persistence must not change the outcome of the business request.
func PrivilegedAudit(db *gorm.DB, role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if method == "GET" || method == "HEAD" || method == "OPTIONS" {
			c.Next()
			return
		}
		c.Next()
		rawID, _ := c.Get("user_id")
		actorID, _ := rawID.(uint64)
		actorName, _ := c.Get("username")
		roomScope, _ := c.Get("room_scope")
		requestID := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if requestID == "" {
			requestID = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
		}
		_ = db.Create(&audit.Log{
			ActorID: actorID, ActorName: stringValue(actorName), ActorRole: role,
			RoomScope: stringValue(roomScope), Method: method, Path: c.FullPath(),
			StatusCode: c.Writer.Status(), RequestID: requestID, IP: c.ClientIP(),
		}).Error
	}
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	if account, ok := value.(user.User); ok {
		return account.Username
	}
	return ""
}
