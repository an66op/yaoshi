package middleware

import (
	"backend/accesscontrol"
	"backend/constants"
	"backend/data/models/user"
	"backend/services"
	"backend/sessionauth"
	"backend/utils"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, _ := sessionauth.TokenFromRequest(c.Request)
		if token == "" {
			constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
			c.Abort()
			return
		}

		claims, err := utils.ParseToken(token)
		if err != nil {
			constants.SendError(c, http.StatusUnauthorized, "登录已失效，请重新登录", err)
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("auth_version", claims.AuthVersion)
		c.Next()
	}
}

// requireCurrentAuthVersion binds a parsed JWT to the account's current
// credential state. Every role middleware calls it after loading the account,
// so password changes revoke old tokens across all application surfaces.
func requireCurrentAuthVersion(c *gin.Context, accountVersion uint64) bool {
	rawVersion, ok := c.Get("auth_version")
	claimVersion, typeOK := rawVersion.(uint64)
	if !ok || !typeOK || claimVersion == 0 || accountVersion == 0 || claimVersion != accountVersion {
		constants.SendError(c, http.StatusUnauthorized, "登录已失效，请重新登录", nil)
		c.Abort()
		return false
	}
	return true
}

// requireWorkspaceBinding makes room-scoped administration fail closed. Some
// platform services intentionally interpret workspace_id=0 as an all-workspace
// query for platform administrators, so tenant and agent entry points must
// never pass an unbound account into those services.
func requireWorkspaceBinding(c *gin.Context, workspaceID uint64) bool {
	if workspaceID == 0 {
		constants.SendError(c, http.StatusForbidden, "账号尚未绑定房间工作区，请联系平台管理员", nil)
		c.Abort()
		return false
	}
	return true
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
		if err := db.Select("user_id", "workspace_id", "username", "nickname", "email", "role", "status", "auth_version").First(&account, userID).Error; err != nil {
			constants.SendError(c, http.StatusUnauthorized, "账号不存在或已失效", err)
			c.Abort()
			return
		}
		if !requireCurrentAuthVersion(c, account.AuthVersion) {
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
		c.Set("workspace_id", account.WorkspaceID)
		c.Set("username", account.Username)
		c.Next()
	}
}

// AgentMiddleware is a hard authorization boundary for the room workbench.
// The room identity always comes from the authenticated account; handlers must
// never accept an agent id or room scope supplied by the browser.
func AgentMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawID, ok := c.Get("user_id")
		userID, idOK := rawID.(uint64)
		if !ok || !idOK || userID == 0 {
			constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
			c.Abort()
			return
		}
		var account user.User
		if err := db.Select("user_id", "workspace_id", "public_id", "username", "nickname", "email", "role", "status", "auth_version", "agent_room_code", "parent_tenant_id").First(&account, userID).Error; err != nil {
			constants.SendError(c, http.StatusUnauthorized, "账号不存在或已失效", err)
			c.Abort()
			return
		}
		if !requireCurrentAuthVersion(c, account.AuthVersion) {
			return
		}
		if account.Status != 1 {
			constants.SendError(c, http.StatusUnauthorized, "账号已被禁用", nil)
			c.Abort()
			return
		}
		if account.Role != "agent" || strings.TrimSpace(account.AgentRoomCode) == "" {
			constants.SendError(c, http.StatusForbidden, "需要房间代理权限", nil)
			c.Abort()
			return
		}
		if !requireWorkspaceBinding(c, account.WorkspaceID) {
			return
		}
		hierarchyActive, err := accesscontrol.AgentHierarchyActive(db, account)
		if err != nil {
			constants.SendError(c, http.StatusInternalServerError, "读取代理权限失败", err)
			c.Abort()
			return
		}
		if !hierarchyActive {
			constants.SendError(c, http.StatusForbidden, "所属租户已停用，代理工作台不可用", nil)
			c.Abort()
			return
		}
		c.Set("agent_user", account)
		c.Set("agent_id", account.UserID)
		c.Set("workspace_id", account.WorkspaceID)
		c.Set("room_scope", "agent:"+fmt.Sprint(account.UserID))
		c.Set("username", account.Username)
		c.Next()
	}
}

// TenantMiddleware protects the tenant workbench. Tenant ownership is always
// read from the authenticated account and is never accepted from query/body.
func TenantMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawID, ok := c.Get("user_id")
		userID, idOK := rawID.(uint64)
		if !ok || !idOK || userID == 0 {
			constants.SendError(c, http.StatusUnauthorized, "请先登录", nil)
			c.Abort()
			return
		}
		var account user.User
		if err := db.Select("user_id", "workspace_id", "public_id", "username", "nickname", "email", "role", "status", "auth_version").First(&account, userID).Error; err != nil {
			constants.SendError(c, http.StatusUnauthorized, "账号不存在或已失效", err)
			c.Abort()
			return
		}
		if !requireCurrentAuthVersion(c, account.AuthVersion) {
			return
		}
		if account.Status != 1 {
			constants.SendError(c, http.StatusUnauthorized, "账号已被禁用", nil)
			c.Abort()
			return
		}
		if account.Role != "tenant" {
			constants.SendError(c, http.StatusForbidden, "需要租户权限", nil)
			c.Abort()
			return
		}
		if !requireWorkspaceBinding(c, account.WorkspaceID) {
			return
		}
		c.Set("tenant_user", account)
		c.Set("tenant_id", account.UserID)
		c.Set("workspace_id", account.WorkspaceID)
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
		if err := db.Select("user_id", "workspace_id", "username", "nickname", "email", "role", "status", "auth_version", "parent_agent_id", "parent_tenant_id").
			First(&account, userID).Error; err != nil {
			constants.SendError(c, http.StatusUnauthorized, "账号不存在或已失效", err)
			c.Abort()
			return
		}
		if !requireCurrentAuthVersion(c, account.AuthVersion) {
			return
		}
		if account.Status != 1 {
			constants.SendError(c, http.StatusUnauthorized, "账号已被禁用", nil)
			c.Abort()
			return
		}
		if account.Role == "admin" || account.Role == "tenant" {
			constants.SendError(c, http.StatusForbidden, "请使用管理后台", nil)
			c.Abort()
			return
		}
		roomActive, err := accesscontrol.AccountRoomActive(db, account)
		if err != nil {
			constants.SendError(c, http.StatusInternalServerError, "读取房间权限失败", err)
			c.Abort()
			return
		}
		if !roomActive {
			constants.SendError(c, http.StatusForbidden, "所属房间已停用，请联系管理员", nil)
			c.Abort()
			return
		}
		c.Set("member_user", account)
		c.Set("workspace_id", account.WorkspaceID)
		if workspace, err := services.WorkspaceForAccount(db, account); err == nil {
			c.Set("room_scope", workspace.Scope)
		}
		c.Set("username", account.Username)
		c.Next()
	}
}
