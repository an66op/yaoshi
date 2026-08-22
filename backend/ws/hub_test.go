package ws

import "testing"

func TestHubTargetsOnlyRequestedUsers(t *testing.T) {
	hub := NewHub()
	first := &client{userID: 1, send: make(chan []byte, 1)}
	second := &client{userID: 2, send: make(chan []byte, 1)}
	hub.register(first)
	hub.register(second)
	hub.broadcast([]byte(`{"type":"test"}`), 1)
	select {
	case <-first.send:
	default:
		t.Fatal("target user did not receive event")
	}
	select {
	case <-second.send:
		t.Fatal("non-target user received event")
	default:
	}
}
