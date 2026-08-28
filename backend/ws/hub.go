package ws

import (
	"backend/cluster"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Event is a JSON message pushed to connected clients.
type Event struct {
	ID          string    `json:"event_id"`
	Type        string    `json:"type"`
	WorkspaceID uint64    `json:"workspace_id,omitempty"`
	RoomScope   string    `json:"room_scope,omitempty"`
	GameID      string    `json:"game_id,omitempty"`
	Issue       string    `json:"issue,omitempty"`
	ServerAt    time.Time `json:"server_at"`
	Data        any       `json:"data"`
}

var eventSequence atomic.Uint64

func prepareEvent(event Event) Event {
	if event.ID == "" {
		now := time.Now().UTC()
		// Include the process identity so two backend instances publishing in the
		// same millisecond cannot generate the same reconnect/deduplication key.
		event.ID = fmt.Sprintf("%s-%d-%d", cluster.InstanceID(), now.UnixMilli(), eventSequence.Add(1))
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
		if event.WorkspaceID == 0 {
			switch value := data["workspace_id"].(type) {
			case uint64:
				event.WorkspaceID = value
			case int:
				event.WorkspaceID = uint64(value)
			case float64:
				event.WorkspaceID = uint64(value)
			}
		}
	}
	return event
}

type client struct {
	identity SessionIdentity
	send     chan []byte
	done     chan struct{}
	stateMu  sync.Mutex
	closed   bool
}

// closeSession and enqueue share the same lock so a logout that has finished
// disconnecting a client cannot race with a publisher and leave a payload in
// the socket's queue.  The queue itself is deliberately never closed: senders
// can hold a stale client pointer after it has been removed from the hub.
func (c *client) closeSession() {
	c.stateMu.Lock()
	if !c.closed {
		c.closed = true
		close(c.done)
	}
	c.stateMu.Unlock()
}

func (c *client) enqueue(payload []byte) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.closed {
		return false
	}
	select {
	case c.send <- payload:
		return true
	default:
		return false
	}
}

// writeIfOpen serializes the final socket write with revocation.  A write that
// started before logout may finish first, but after disconnectUser returns no
// queued event can be written by the revoked client.
func (c *client) writeIfOpen(write func() error) (bool, error) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.closed {
		return false, nil
	}
	return true, write()
}

// Hub tracks live WebSocket connections and fan-out events.
type Hub struct {
	mu                  sync.RWMutex
	clients             map[*client]struct{}
	sessionValidator    SessionValidator
	validatorConfigured bool
}

var defaultHub = NewHub()

func Default() *Hub { return defaultHub }

func NewHub() *Hub {
	return &Hub{clients: make(map[*client]struct{})}
}

func (h *Hub) setSessionValidator(validator SessionValidator) {
	h.mu.Lock()
	h.sessionValidator = validator
	h.validatorConfigured = validator != nil
	h.mu.Unlock()
}

func (h *Hub) validate(identity SessionIdentity) bool {
	h.mu.RLock()
	validator := h.sessionValidator
	configured := h.validatorConfigured
	h.mu.RUnlock()
	return configured && validator != nil && validator(identity)
}

func (h *Hub) hasSessionValidator() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.validatorConfigured && h.sessionValidator != nil
}

func (h *Hub) register(c *client) {
	if c.done == nil {
		c.done = make(chan struct{})
	}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) unregister(c *client) {
	h.mu.Lock()
	_, existed := h.clients[c]
	if existed {
		delete(h.clients, c)
	}
	h.mu.Unlock()
	if existed {
		c.closeSession()
	}
}

func (h *Hub) broadcast(payload []byte, userID, workspaceID uint64) {
	h.mu.RLock()
	clients := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		if userID > 0 && c.identity.UserID != userID {
			continue
		}
		if workspaceID > 0 && c.identity.WorkspaceID != workspaceID {
			continue
		}
		clients = append(clients, c)
	}
	h.mu.RUnlock()
	for _, c := range clients {
		if !h.validate(c.identity) {
			h.unregister(c)
			continue
		}
		c.enqueue(payload)
	}
}

func (h *Hub) disconnectUser(userID uint64) {
	if userID == 0 {
		return
	}
	h.mu.RLock()
	clients := make([]*client, 0)
	for c := range h.clients {
		if c.identity.UserID == userID {
			clients = append(clients, c)
		}
	}
	h.mu.RUnlock()
	for _, c := range clients {
		h.unregister(c)
	}
}

// disconnectUserGeneration closes only sockets authenticated by the revoked
// credential generation. PostgreSQL outbox replay is at-least-once and may be
// delayed; a user who has since logged in with a newer generation must remain
// connected when an older event is delivered again.
func (h *Hub) disconnectUserGeneration(userID, revokedAuthVersion uint64) {
	if userID == 0 || revokedAuthVersion == 0 {
		return
	}
	h.mu.RLock()
	clients := make([]*client, 0)
	for c := range h.clients {
		if c.identity.UserID == userID && c.identity.AuthVersion == revokedAuthVersion {
			clients = append(clients, c)
		}
	}
	h.mu.RUnlock()
	for _, c := range clients {
		h.unregister(c)
	}
}

func (h *Hub) broadcastUsers(payload []byte, userIDs []uint64, workspaceID uint64) {
	if len(userIDs) == 0 {
		return
	}
	recipients := make(map[uint64]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if userID != 0 {
			recipients[userID] = struct{}{}
		}
	}
	h.mu.RLock()
	clients := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		if _, ok := recipients[c.identity.UserID]; !ok {
			continue
		}
		if workspaceID > 0 && c.identity.WorkspaceID != workspaceID {
			continue
		}
		clients = append(clients, c)
	}
	h.mu.RUnlock()
	for _, c := range clients {
		if !h.validate(c.identity) {
			h.unregister(c)
			continue
		}
		c.enqueue(payload)
	}
}

// Publish sends an event to every connected client.
func Publish(event Event) {
	event = prepareEvent(event)
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	defaultHub.broadcast(payload, 0, event.WorkspaceID)
	publishClusterEvent(clusterEnvelope{Action: clusterActionBroadcast, WorkspaceID: event.WorkspaceID, Payload: payload})
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
	defaultHub.broadcast(payload, userID, event.WorkspaceID)
	publishClusterEvent(clusterEnvelope{Action: clusterActionUsers, WorkspaceID: event.WorkspaceID, UserIDs: []uint64{userID}, Payload: payload})
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
	defaultHub.broadcastUsers(payload, userIDs, event.WorkspaceID)
	publishClusterEvent(clusterEnvelope{Action: clusterActionUsers, WorkspaceID: event.WorkspaceID, UserIDs: userIDs, Payload: payload})
}

// DisconnectUser immediately closes this instance's active sockets. Security
// mutations are propagated independently by the transactional PostgreSQL
// revocation outbox and Redis Stream; keeping transport out of this call means
// a committed mutation cannot be lost between a request and an in-memory queue.
func DisconnectUser(userID uint64) {
	if userID == 0 {
		return
	}
	defaultHub.disconnectUser(userID)
}
