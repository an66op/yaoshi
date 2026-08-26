package ws

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPrepareEventAddsStableEnvelope(t *testing.T) {
	event := prepareEvent(Event{Type: "chat_message", Data: map[string]any{
		"room_scope": "agent:9",
		"game_id":    "speed-racing",
		"issue":      "34129990",
	}})
	if event.ID == "" || event.ServerAt.IsZero() {
		t.Fatalf("event envelope is incomplete: %+v", event)
	}
	if event.RoomScope != "agent:9" || event.GameID != "speed-racing" || event.Issue != "34129990" {
		t.Fatalf("scope fields were not promoted into the envelope: %+v", event)
	}

	next := prepareEvent(Event{Type: "chat_message", Data: map[string]any{}})
	if next.ID == event.ID {
		t.Fatalf("event ids must be unique: %q", next.ID)
	}
}

func TestPublishToUsersDoesNotCrossRoomRecipients(t *testing.T) {
	previous := defaultHub
	defaultHub = NewHub()
	t.Cleanup(func() { defaultHub = previous })

	roomA := &client{userID: 101, send: make(chan []byte, 1)}
	roomB := &client{userID: 202, send: make(chan []byte, 1)}
	defaultHub.register(roomA)
	defaultHub.register(roomB)

	PublishToUsers([]uint64{101}, Event{
		Type: "chat_message", RoomScope: "agent:9", GameID: "speed-racing",
		Data: map[string]any{"message_id": uint64(88)},
	})

	select {
	case payload := <-roomA.send:
		var event Event
		if err := json.Unmarshal(payload, &event); err != nil {
			t.Fatal(err)
		}
		if event.RoomScope != "agent:9" || event.GameID != "speed-racing" || event.ID == "" {
			t.Fatalf("unexpected event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("intended room member did not receive the event")
	}

	select {
	case payload := <-roomB.send:
		t.Fatalf("unrelated room member received event: %s", payload)
	default:
	}
}
