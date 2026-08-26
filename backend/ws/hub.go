package ws

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Event is a JSON message pushed to connected clients.
type Event struct {
	ID        string    `json:"event_id"`
	Type      string    `json:"type"`
	RoomScope string    `json:"room_scope,omitempty"`
	GameID    string    `json:"game_id,omitempty"`
	Issue     string    `json:"issue,omitempty"`
	ServerAt  time.Time `json:"server_at"`
	Data      any       `json:"data"`
}

var eventSequence atomic.Uint64

func prepareEvent(event Event) Event {
	if event.ID == "" {
		now := time.Now().UTC()
		event.ID = fmt.Sprintf("%d-%d", now.UnixMilli(), eventSequence.Add(1))
		event.ServerAt = now
	}
	if data, ok := event.Data.(map[string]any); ok {
		if event.RoomScope == "" {
			event.RoomScope, _ = data["room_scope"].(string)
		}
		if event.GameID == "" {
			event.GameID, _ = data["game_id"].(string)
		}
		if event.Issue == "" {
			event.Issue, _ = data["issue"].(string)
		}
	}
	return event
}

type client struct {
	userID uint64
	send   chan []byte
}

// Hub tracks live WebSocket connections and fan-out events.
type Hub struct {
	mu      sync.RWMutex
	clients map[*client]struct{}
}

var defaultHub = NewHub()

func Default() *Hub { return defaultHub }

func NewHub() *Hub {
	return &Hub{clients: make(map[*client]struct{})}
}

func (h *Hub) register(c *client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) unregister(c *client) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	close(c.send)
}

func (h *Hub) broadcast(payload []byte, userID uint64) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if userID > 0 && c.userID != userID {
			continue
		}
		select {
		case c.send <- payload:
		default:
		}
	}
}

// Publish sends an event to every connected client.
func Publish(event Event) {
	event = prepareEvent(event)
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	defaultHub.broadcast(payload, 0)
}

// PublishToUser sends an event to a single user's connections.
func PublishToUser(userID uint64, event Event) {
	if userID == 0 {
		return
	}
	event = prepareEvent(event)
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	defaultHub.broadcast(payload, userID)
}

// PublishToUsers sends an event only to the listed users. Duplicate IDs are
// ignored so one recipient with multiple room roles receives one event.
func PublishToUsers(userIDs []uint64, event Event) {
	if len(userIDs) == 0 {
		return
	}
	event = prepareEvent(event)
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	recipients := make(map[uint64]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if userID != 0 {
			recipients[userID] = struct{}{}
		}
	}
	defaultHub.mu.RLock()
	defer defaultHub.mu.RUnlock()
	for c := range defaultHub.clients {
		if _, ok := recipients[c.userID]; !ok {
			continue
		}
		select {
		case c.send <- payload:
		default:
		}
	}
}
