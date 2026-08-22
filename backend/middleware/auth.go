package middleware

import (
	"backend/constants"
	"backend/data/models/user"
	"backend/utils"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
			c.Abort()
			return
		}

		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) == 2 && strings.ToLower(tokenParts[0]) == "bearer" {
			authHeader = tokenParts[1]
		}

		claims, err := utils.ParseToken(authHeader)
		if err != nil {
			constants.SendError(c, http.StatusUnauthorized, "登录已失效，请重新登录", err)
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Next()
	}
}

// AdminMiddleware requires an authenticated user with role=admin and status=1.
func AdminMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawID, ok := c.Get("user_id")
		if !ok {
			constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
			c.Abort()
			return
		}
		userID, ok := rawID.(uint64)
		if !ok || userID == 0 {
			constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
			c.Abort()
			return
		}
		var account user.User
		if err := db.Select("user_id", "username", "nickname", "email", "role", "status").First(&account, userID).Error; err != nil {
			constants.SendError(c, http.StatusUnauthorized, "账号不存在或已失效", err)
			c.Abort()
			return
		}
		if account.Status != 1 {
			constants.SendError(c, http.StatusUnauthorized, "账号已被禁用", nil)
			c.Abort()
			return
		}
		if account.Role != "admin" {
			constants.SendError(c, http.StatusForbidden, "需要管理员权限", nil)
			c.Abort()
			return
		}
		c.Set("admin_user", account)
		c.Set("username", account.Username)
		c.Next()
	}
}

// MemberMiddleware requires an authenticated member or agent (not admin).
func MemberMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawID, ok := c.Get("user_id")
		if !ok {
			constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
			c.Abort()
			return
		}
		userID, ok := rawID.(uint64)
		if !ok || userID == 0 {
			constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
			c.Abort()
			return
		}
		var account user.User
		if err := db.Select("user_id", "username", "nickname", "email", "role", "status").
			First(&account, userID).Error; err != nil {
			constants.SendError(c, http.StatusUnauthorized, "账号不存在或已失效", err)
			c.Abort()
			return
		}
		if account.Status != 1 {
			constants.SendError(c, http.StatusUnauthorized, "账号已被禁用", nil)
			c.Abort()
			return
		}
		if account.Role == "admin" {
			constants.SendError(c, http.StatusForbidden, "请使用管理后台", nil)
			c.Abort()
			return
		}
		c.Set("member_user", account)
		c.Set("username", account.Username)
		c.Next()
	}
}
