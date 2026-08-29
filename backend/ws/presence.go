package ws

import (
	"backend/cluster"
	"context"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	presenceTTL          = 75 * time.Second
	presenceKeyTTL       = 2 * presenceTTL
	presenceRedisTimeout = 250 * time.Millisecond
)

var presenceSequence atomic.Uint64

// Redis time is used for both writes and reads so clock skew between backend
// instances cannot make a live socket expire early or a dead socket linger.
var touchPresenceScript = redis.NewScript(`
local current = redis.call('TIME')
local now_ms = tonumber(current[1]) * 1000 + math.floor(tonumber(current[2]) / 1000)
redis.call('ZADD', KEYS[1], now_ms + tonumber(ARGV[1]), ARGV[3])
redis.call('PEXPIRE', KEYS[1], ARGV[2])
return now_ms
`)

var onlineUsersScript = redis.NewScript(`
local current = redis.call('TIME')
local now_ms = tonumber(current[1]) * 1000 + math.floor(tonumber(current[2]) / 1000)
local result = {}
for index, key in ipairs(KEYS) do
  redis.call('ZREMRANGEBYSCORE', key, '-inf', now_ms)
  if redis.call('ZCARD', key) > 0 then
    result[index] = 1
  else
    result[index] = 0
  end
end
return result
`)

func presenceKey(userID uint64) string {
	return cluster.Key("ws-presence", strconv.FormatUint(userID, 10))
}

// touchPresence publishes one independently expiring member per socket. The
// client state lock intentionally covers the Redis write: unregister first
// marks the client closed and then removes the token, so an in-flight heartbeat
// can never re-add a token after removal has completed.
func (h *Hub) touchPresence(c *client) {
	if h == nil || c == nil || c.identity.UserID == 0 {
		return
	}
	current := cluster.Client()
	if current == nil {
		return
	}

	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.closed {
		return
	}
	if c.presenceToken == "" {
		c.presenceToken = cluster.InstanceID() + ":" + strconv.FormatUint(c.identity.UserID, 10) + ":" + strconv.FormatUint(presenceSequence.Add(1), 10)
	}
	ctx, cancel := context.WithTimeout(context.Background(), presenceRedisTimeout)
	defer cancel()
	_ = touchPresenceScript.Run(ctx, current, []string{presenceKey(c.identity.UserID)}, presenceTTL.Milliseconds(), presenceKeyTTL.Milliseconds(), c.presenceToken).Err()
}

func removePresence(c *client) {
	if c == nil || c.identity.UserID == 0 {
		return
	}
	c.stateMu.Lock()
	token := c.presenceToken
	c.stateMu.Unlock()
	if token == "" {
		return
	}
	current := cluster.Client()
	if current == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), presenceRedisTimeout)
	defer cancel()
	_ = current.ZRem(ctx, presenceKey(c.identity.UserID), token).Err()
}

// OnlineUsers returns live presence for all requested users with at most one
// Redis round trip. Local Hub membership wins immediately; when Redis is not
// configured (the supported single-instance development mode), the local Hub
// remains the exact source of truth.
func (h *Hub) OnlineUsers(userIDs []uint64) map[uint64]bool {
	result := make(map[uint64]bool, len(userIDs))
	for _, userID := range userIDs {
		if userID != 0 {
			result[userID] = false
		}
	}
	if h == nil || len(result) == 0 {
		return result
	}

	h.mu.RLock()
	for c := range h.clients {
		if _, requested := result[c.identity.UserID]; requested {
			result[c.identity.UserID] = true
		}
	}
	h.mu.RUnlock()

	current := cluster.Client()
	if current == nil {
		return result
	}
	pending := make([]uint64, 0, len(result))
	keys := make([]string, 0, len(result))
	for userID, online := range result {
		if !online {
			pending = append(pending, userID)
			keys = append(keys, presenceKey(userID))
		}
	}
	if len(pending) == 0 {
		return result
	}

	ctx, cancel := context.WithTimeout(context.Background(), presenceRedisTimeout)
	defer cancel()
	values, err := onlineUsersScript.Run(ctx, current, keys).Slice()
	if err != nil {
		return result
	}
	for index, userID := range pending {
		if index >= len(values) {
			break
		}
		switch value := values[index].(type) {
		case int64:
			result[userID] = value > 0
		case string:
			result[userID] = value == "1"
		}
	}
	return result
}

// IsUserOnline reports authenticated WebSocket presence across backend
// instances when Redis is enabled, and exact local presence otherwise.
func (h *Hub) IsUserOnline(userID uint64) bool {
	return h.OnlineUsers([]uint64{userID})[userID]
}

// OnlineUsers queries presence through the process-wide WebSocket Hub.
func OnlineUsers(userIDs []uint64) map[uint64]bool {
	return defaultHub.OnlineUsers(userIDs)
}

// IsUserOnline queries one user's presence through the process-wide Hub.
func IsUserOnline(userID uint64) bool {
	return defaultHub.IsUserOnline(userID)
}
