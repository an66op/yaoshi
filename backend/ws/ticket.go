package ws

import (
	"backend/cluster"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const ticketTTL = 90 * time.Second

type connectionTicket struct {
	identity  SessionIdentity
	expiresAt time.Time
}

// ticketStore keeps short-lived, one-time WebSocket credentials in-process.
// It is deliberately separate from JWTs so a browser never puts its login JWT
// into a WebSocket URL (which can otherwise be retained by access logs).
type ticketStore struct {
	mu      sync.Mutex
	tickets map[string]connectionTicket
}

var wsTickets = ticketStore{tickets: make(map[string]connectionTicket)}

func (s *ticketStore) create(identity SessionIdentity) (string, time.Time, error) {
	if !identity.valid() {
		return "", time.Time{}, errInvalidSessionIdentity
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", time.Time{}, err
	}
	value := base64.RawURLEncoding.EncodeToString(buf)
	expiresAt := time.Now().Add(ticketTTL)
	if cluster.Enabled() {
		payload, err := json.Marshal(identity)
		if err != nil {
			return "", time.Time{}, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := cluster.Client().Set(ctx, cluster.Key("ws-ticket", value), payload, ticketTTL).Err(); err == nil {
			return value, expiresAt, nil
		} else if cluster.Required() {
			return "", time.Time{}, fmt.Errorf("store websocket ticket: %w", err)
		}
	} else if cluster.Required() {
		return "", time.Time{}, cluster.ErrUnavailable
	}
	// Debug/test fallback is intentionally process-local. Release mode never
	// reaches this path, so a load balancer may safely route the upgrade to any
	// backend instance when Redis is required.
	s.mu.Lock()
	s.removeExpiredLocked(time.Now())
	s.tickets[value] = connectionTicket{identity: identity, expiresAt: expiresAt}
	s.mu.Unlock()
	return value, expiresAt, nil
}

func (s *ticketStore) consume(value string) (SessionIdentity, bool, error) {
	if value == "" {
		return SessionIdentity{}, false, nil
	}
	if cluster.Enabled() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		payload, err := cluster.Client().GetDel(ctx, cluster.Key("ws-ticket", value)).Bytes()
		if err == nil {
			var identity SessionIdentity
			if decodeErr := json.Unmarshal(payload, &identity); decodeErr != nil || !identity.valid() {
				return SessionIdentity{}, false, nil
			}
			return identity, true, nil
		}
		if err == redis.Nil {
			return SessionIdentity{}, false, nil
		}
		if cluster.Required() {
			return SessionIdentity{}, false, fmt.Errorf("consume websocket ticket: %w", err)
		}
	} else if cluster.Required() {
		return SessionIdentity{}, false, cluster.ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.tickets[value]
	delete(s.tickets, value) // one attempt only, including expired values.
	if !ok || time.Now().After(entry.expiresAt) {
		return SessionIdentity{}, false, nil
	}
	return entry.identity, true, nil
}

func (s *ticketStore) removeExpiredLocked(now time.Time) {
	for key, entry := range s.tickets {
		if !now.Before(entry.expiresAt) {
			delete(s.tickets, key)
		}
	}
}

// HandleTicket creates a one-time credential for the member WebSocket.
// The surrounding member auth middleware supplies user_id.
func HandleTicket(c *gin.Context) {
	rawUserID, ok := c.Get("user_id")
	userID, ok := rawUserID.(uint64)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "请先登录"})
		return
	}
	authVersion, versionOK := uint64ContextValue(c, "auth_version")
	workspaceID, workspaceOK := uint64ContextValue(c, "workspace_id")
	if !versionOK || !workspaceOK {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "登录状态不完整，请重新登录"})
		return
	}
	identity := SessionIdentity{UserID: userID, AuthVersion: authVersion, WorkspaceID: workspaceID}
	ticket, expiresAt, err := wsTickets.create(identity)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "创建实时连接凭据失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "message": "ok", "data": gin.H{
		"ticket": ticket, "expires_at": expiresAt.UTC(),
	}})
}
