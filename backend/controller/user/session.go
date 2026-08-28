package user

import (
	"backend/config"
	"backend/constants"
	"backend/sessionauth"
	"backend/utils"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func sessionCookieSecure(c *gin.Context) bool {
	return config.GetConfig().Server.Mode == "release" || c.Request.TLS != nil
}

func writeSessionCookie(c *gin.Context, scope sessionauth.Scope, token string) {
	cfg := config.GetConfig()
	http.SetCookie(c.Writer, sessionauth.NewCookie(scope, token, sessionCookieSecure(c), time.Duration(cfg.JWT.Expire)*time.Second))
	c.Header("Cache-Control", "no-store")
}

func clearSessionCookie(c *gin.Context, scope sessionauth.Scope) {
	http.SetCookie(c.Writer, sessionauth.ExpiredCookie(scope, sessionCookieSecure(c)))
	c.Header("Cache-Control", "no-store")
}

func refreshSessionCookie(c *gin.Context, scope sessionauth.Scope) bool {
	userID, idOK := c.Get("user_id")
	authVersion, versionOK := c.Get("auth_version")
	id, validID := userID.(uint64)
	version, validVersion := authVersion.(uint64)
	if !idOK || !versionOK || !validID || !validVersion || id == 0 || version == 0 {
		constants.SendError(c, http.StatusUnauthorized, "登录已失效，请重新登录", nil)
		return false
	}
	token, err := utils.GenerateToken(id, version)
	if err != nil {
		constants.SendError(c, http.StatusInternalServerError, "刷新登录状态失败", err)
		return false
	}
	writeSessionCookie(c, scope, token)
	constants.SendSuccess(c, http.StatusOK, "登录状态已刷新", gin.H{"expires_in": config.GetConfig().JWT.Expire})
	return true
}
