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

// writeVersionedSessionCookie issues a cookie from the authoritative session
// generation stored in PostgreSQL. Callers use this after a security-sensitive
// account mutation that deliberately invalidates the token carried by the
// current request.
func writeVersionedSessionCookie(c *gin.Context, scope sessionauth.Scope, userID, authVersion uint64) error {
	token, err := utils.GenerateToken(userID, authVersion)
	if err != nil {
		return err
	}
	writeSessionCookie(c, scope, token)
	return nil
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
	if err := writeVersionedSessionCookie(c, scope, id, version); err != nil {
		constants.SendError(c, http.StatusInternalServerError, "刷新登录状态失败", err)
		return false
	}
	constants.SendSuccess(c, http.StatusOK, "登录状态已刷新", gin.H{"expires_in": config.GetConfig().JWT.Expire})
	return true
}
