package middleware

import (
	"backend/data/models/audit"
	"backend/data/models/user"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PrivilegedAudit records every privileged mutation after authorization. It
// writes an intent before executing the handler; if neither PostgreSQL nor the
// fsync spool is available, it fails closed instead of allowing an unaudited
// high-risk operation.
func PrivilegedAudit(db *gorm.DB, role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if method == "GET" || method == "HEAD" || method == "OPTIONS" {
			c.Next()
			return
		}
		rawID, _ := c.Get("user_id")
		actorID, _ := rawID.(uint64)
		actorName, _ := c.Get("username")
		roomScope, _ := c.Get("room_scope")
		workspaceRaw, _ := c.Get("workspace_id")
		workspaceID, _ := workspaceRaw.(uint64)
		requestID := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if requestID == "" {
			requestID = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
		}
		entry := audit.Log{
			EventID:     newAuditEventID(),
			WorkspaceID: workspaceID, ActorID: actorID, ActorName: stringValue(actorName), ActorRole: role,
			RoomScope: stringValue(roomScope), Method: method, Path: c.FullPath(),
			TargetRef: auditTargetRef(c), StatusCode: http.StatusProcessing, RequestID: requestID, IP: c.ClientIP(),
		}
		primaryPersisted := db != nil && db.Create(&entry).Error == nil
		if !primaryPersisted {
			if fallbackErr := persistAuditFallback(entry); fallbackErr != nil {
				log.Printf("严重：管理审计预写及本地保底均失败 event=%s fallback=%v", entry.EventID, fallbackErr)
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": "AUDIT_UNAVAILABLE", "message": "安全审计暂不可用，请稍后重试"})
				return
			}
		}

		c.Next()
		entry.StatusCode = c.Writer.Status()
		if targetRaw, exists := c.Get("target_workspace_id"); exists {
			entry.WorkspaceID = auditWorkspaceID(targetRaw, entry.WorkspaceID)
		}
		entry.TargetRef = auditTargetRef(c)
		if primaryPersisted && updateAuditRecord(db, entry) == nil {
			return
		}
		// Appending the completed event is safe: recovery coalesces duplicate
		// event IDs and persists the latest state.
		if err := persistAuditFallback(entry); err != nil {
			log.Printf("严重：管理审计完成状态保底失败 event=%s err=%v", entry.EventID, err)
		}
	}
}

func auditWorkspaceID(value any, fallback uint64) uint64 {
	switch target := value.(type) {
	case uint64:
		if target > 0 {
			return target
		}
	case uint:
		if target > 0 {
			return uint64(target)
		}
	case int:
		if target > 0 {
			return uint64(target)
		}
	}
	return fallback
}

func auditTargetRef(c *gin.Context) string {
	parts := make([]string, 0, len(c.Params))
	for _, param := range c.Params {
		value := strings.TrimSpace(param.Value)
		if value != "" {
			parts = append(parts, param.Key+"="+value)
		}
	}
	return strings.Join(parts, ",")
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
