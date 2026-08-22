package ws

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const ticketTTL = 90 * time.Second

type connectionTicket struct {
	userID    uint64
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

func (s *ticketStore) create(userID uint64) (string, time.Time, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", time.Time{}, err
	}
	value := base64.RawURLEncoding.EncodeToString(buf)
	expiresAt := time.Now().Add(ticketTTL)
	s.mu.Lock()
	s.removeExpiredLocked(time.Now())
	s.tickets[value] = connectionTicket{userID: userID, expiresAt: expiresAt}
	s.mu.Unlock()
	return value, expiresAt, nil
}

func (s *ticketStore) consume(value string) (uint64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.tickets[value]
	delete(s.tickets, value) // one attempt only, including expired values.
	if !ok || time.Now().After(entry.expiresAt) {
		return 0, false
	}
	return entry.userID, true
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
	ticket, expiresAt, err := wsTickets.create(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "创建实时连接凭据失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "message": "ok", "data": gin.H{
		"ticket": ticket, "expires_at": expiresAt.UTC(),
	}})
}
