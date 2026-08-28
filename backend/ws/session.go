package ws

import (
	"backend/accesscontrol"
	"backend/data/models/user"
	"backend/sessionauth"
	"backend/utils"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var errInvalidSessionIdentity = errors.New("invalid websocket session identity")

// SessionIdentity freezes the credential generation and room binding that
// were current when the one-time WebSocket ticket was issued.  A socket is
// valid only while all three values still match the account row.
type SessionIdentity struct {
	UserID      uint64
	AuthVersion uint64
	WorkspaceID uint64
}

func (identity SessionIdentity) valid() bool {
	return identity.UserID > 0 && identity.AuthVersion > 0
}

type SessionValidator func(SessionIdentity) bool

// ConfigureSessionDatabase binds WebSocket authorization to the same account
// state used by HTTP middleware.  LoadRoutes calls this once during startup.
func ConfigureSessionDatabase(db *gorm.DB) {
	defaultHub.setSessionValidator(func(identity SessionIdentity) bool {
		if db == nil || !identity.valid() {
			return false
		}
		var account user.User
		if err := db.Select(
			"user_id", "workspace_id", "status", "auth_version", "role",
			"agent_room_code", "parent_agent_id", "parent_tenant_id",
		).First(&account, identity.UserID).Error; err != nil {
			return false
		}
		if account.Status != 1 || account.AuthVersion != identity.AuthVersion || account.WorkspaceID != identity.WorkspaceID {
			return false
		}
		switch account.Role {
		case "admin", "tenant":
			return true
		case "agent", "member":
			active, err := accesscontrol.AccountRoomActive(db, account)
			return err == nil && active
		default:
			return false
		}
	})
}

func uint64ContextValue(c *gin.Context, key string) (uint64, bool) {
	raw, exists := c.Get(key)
	if !exists {
		return 0, false
	}
	value, ok := raw.(uint64)
	return value, ok
}

// RevokeRequestSession invalidates every JWT and WebSocket issued for the
// credential generation found in the request cookie/Bearer token.  Logout is
// intentionally all-device: this installation does not persist JWT session
// IDs and therefore must not pretend a single stateless token is revocable.
func RevokeRequestSession(db *gorm.DB, request *http.Request) error {
	if db == nil || request == nil {
		return errInvalidSessionIdentity
	}
	token, _ := sessionauth.TokenFromRequest(request)
	if token == "" {
		return nil // logout remains idempotent after a cookie has already expired.
	}
	claims, err := utils.ParseToken(token)
	if err != nil || claims.UserID == 0 || claims.AuthVersion == 0 {
		return nil // clear the client cookie without revealing token validity.
	}
	updated := db.Model(&user.User{}).
		Where("user_id = ? AND auth_version = ?", claims.UserID, claims.AuthVersion).
		UpdateColumn("auth_version", gorm.Expr("auth_version + 1"))
	if updated.Error != nil {
		return updated.Error
	}
	DisconnectUser(claims.UserID)
	return nil
}
