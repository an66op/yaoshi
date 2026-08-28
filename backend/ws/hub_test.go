package ws

import (
	"encoding/json"
	"sync"
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
	defaultHub.setSessionValidator(func(SessionIdentity) bool { return true })

	roomA := &client{identity: SessionIdentity{UserID: 101, AuthVersion: 1, WorkspaceID: 9}, send: make(chan []byte, 1)}
	roomB := &client{identity: SessionIdentity{UserID: 202, AuthVersion: 1, WorkspaceID: 10}, send: make(chan []byte, 1)}
	defaultHub.register(roomA)
	defaultHub.register(roomB)

	PublishToUsers([]uint64{101}, Event{
		Type: "chat_message", WorkspaceID: 9, RoomScope: "agent:9", GameID: "speed-racing",
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

func TestGameCatalogUpdateStaysInsideWorkspace(t *testing.T) {
	previous := defaultHub
	defaultHub = NewHub()
	t.Cleanup(func() { defaultHub = previous })
	defaultHub.setSessionValidator(func(SessionIdentity) bool { return true })

	roomA := &client{identity: SessionIdentity{UserID: 101, AuthVersion: 1, WorkspaceID: 9}, send: make(chan []byte, 1)}
	roomB := &client{identity: SessionIdentity{UserID: 202, AuthVersion: 1, WorkspaceID: 10}, send: make(chan []byte, 1)}
	defaultHub.register(roomA)
	defaultHub.register(roomB)

	NotifyGameCatalogChanged(9, "agent:7", "88001", "speed-racing", false)

	select {
	case payload := <-roomA.send:
		var event Event
		if err := json.Unmarshal(payload, &event); err != nil {
			t.Fatal(err)
		}
		data, ok := event.Data.(map[string]any)
		if !ok || event.Type != "game_catalog_update" || event.WorkspaceID != 9 || data["room_code"] != "88001" || data["enabled"] != false {
			t.Fatalf("unexpected room catalogue event: %+v", event)
		}
	default:
		t.Fatal("target room did not receive catalogue update")
	}

	select {
	case payload := <-roomB.send:
		t.Fatalf("unrelated room received catalogue update: %s", payload)
	default:
	}
}

func TestPlatformGameCatalogUpdateReachesEveryWorkspace(t *testing.T) {
	previous := defaultHub
	defaultHub = NewHub()
	t.Cleanup(func() { defaultHub = previous })
	defaultHub.setSessionValidator(func(SessionIdentity) bool { return true })

	roomA := &client{identity: SessionIdentity{UserID: 101, AuthVersion: 1, WorkspaceID: 9}, send: make(chan []byte, 1)}
	roomB := &client{identity: SessionIdentity{UserID: 202, AuthVersion: 1, WorkspaceID: 10}, send: make(chan []byte, 1)}
	defaultHub.register(roomA)
	defaultHub.register(roomB)

	NotifyGameCatalogChanged(0, "*", "", "speed-racing", false)
	for name, connection := range map[string]*client{"room A": roomA, "room B": roomB} {
		select {
		case payload := <-connection.send:
			var event Event
			if err := json.Unmarshal(payload, &event); err != nil {
				t.Fatal(err)
			}
			if event.Type != "game_catalog_update" || event.WorkspaceID != 0 || event.RoomScope != "*" {
				t.Fatalf("%s received malformed platform catalogue event: %+v", name, event)
			}
		default:
			t.Fatalf("%s did not receive platform catalogue update", name)
		}
	}
}

func TestNotifyChatSeparatesDeliveryAndSourceWorkspace(t *testing.T) {
	previous := defaultHub
	defaultHub = NewHub()
	t.Cleanup(func() { defaultHub = previous })
	defaultHub.setSessionValidator(func(SessionIdentity) bool { return true })

	admin := &client{identity: SessionIdentity{UserID: 55, AuthVersion: 1, WorkspaceID: 1}, send: make(chan []byte, 1)}
	defaultHub.register(admin)
	NotifyChat([]uint64{55}, 1, 44, "service", "agent:9", "service", "user:7", 103, "created", "member", "text")

	select {
	case payload := <-admin.send:
		var event Event
		if err := json.Unmarshal(payload, &event); err != nil {
			t.Fatal(err)
		}
		data, ok := event.Data.(map[string]any)
		if !ok {
			t.Fatalf("chat event data = %#v", event.Data)
		}
		if event.WorkspaceID != 1 || data["source_workspace_id"] != float64(44) || data["sender_kind"] != "member" || data["operation"] != "created" {
			t.Fatalf("unexpected chat event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("platform-bound admin did not receive room chat event")
	}
}

func TestNotifyChatPublishesUpdatedOperation(t *testing.T) {
	previous := defaultHub
	defaultHub = NewHub()
	t.Cleanup(func() { defaultHub = previous })
	defaultHub.setSessionValidator(func(SessionIdentity) bool { return true })

	member := &client{identity: SessionIdentity{UserID: 71, AuthVersion: 1, WorkspaceID: 44}, send: make(chan []byte, 1)}
	otherRoom := &client{identity: SessionIdentity{UserID: 72, AuthVersion: 1, WorkspaceID: 45}, send: make(chan []byte, 1)}
	defaultHub.register(member)
	defaultHub.register(otherRoom)
	NotifyChat([]uint64{71, 72}, 44, 44, "group", "agent:9", "lobby", "agent:9", 103, "updated", "staff", "redpacket")

	select {
	case payload := <-member.send:
		var event Event
		if err := json.Unmarshal(payload, &event); err != nil {
			t.Fatal(err)
		}
		data, ok := event.Data.(map[string]any)
		if !ok || event.Type != "chat_message" || data["operation"] != "updated" || data["message_type"] != "redpacket" || data["message_id"] != float64(103) {
			t.Fatalf("unexpected red-packet update event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("room member did not receive red-packet update")
	}
	select {
	case payload := <-otherRoom.send:
		t.Fatalf("red-packet update crossed workspace boundary: %s", payload)
	default:
	}
}

func TestPublishDisconnectsCredentialOrWorkspaceMismatch(t *testing.T) {
	previous := defaultHub
	defaultHub = NewHub()
	t.Cleanup(func() { defaultHub = previous })

	current := SessionIdentity{UserID: 101, AuthVersion: 7, WorkspaceID: 8801}
	defaultHub.setSessionValidator(func(identity SessionIdentity) bool { return identity == current })
	connection := &client{identity: current, send: make(chan []byte, 2)}
	defaultHub.register(connection)

	PublishToUser(101, Event{Type: "notification"})
	select {
	case <-connection.send:
	case <-time.After(time.Second):
		t.Fatal("valid session did not receive event")
	}

	current.AuthVersion++ // password change/logout/disable generation
	PublishToUser(101, Event{Type: "notification"})
	select {
	case payload := <-connection.send:
		t.Fatalf("revoked session received event: %s", payload)
	default:
	}
	select {
	case <-connection.done:
	case <-time.After(time.Second):
		t.Fatal("revoked session was not actively disconnected")
	}
}

func TestWorkspaceMoveInvalidatesOldSocket(t *testing.T) {
	hub := NewHub()
	currentWorkspace := uint64(8802)
	hub.setSessionValidator(func(identity SessionIdentity) bool {
		return identity.UserID == 11 && identity.AuthVersion == 4 && identity.WorkspaceID == currentWorkspace
	})
	old := &client{identity: SessionIdentity{UserID: 11, AuthVersion: 4, WorkspaceID: 8801}, send: make(chan []byte, 1)}
	hub.register(old)
	hub.broadcast([]byte(`{"type":"balance"}`), 11, 0)
	select {
	case payload := <-old.send:
		t.Fatalf("old-room socket received event: %s", payload)
	default:
	}
	select {
	case <-old.done:
	case <-time.After(time.Second):
		t.Fatal("old-room socket was not disconnected")
	}
}

func TestPublishRejectsRecipientAfterWorkspaceMove(t *testing.T) {
	previous := defaultHub
	defaultHub = NewHub()
	t.Cleanup(func() { defaultHub = previous })
	defaultHub.setSessionValidator(func(SessionIdentity) bool { return true })

	// The recipient ID may have been collected before the member moved.  The
	// event's frozen workspace must prevent delivery to the new-room socket.
	moved := &client{
		identity: SessionIdentity{UserID: 101, AuthVersion: 8, WorkspaceID: 8802},
		send:     make(chan []byte, 1),
	}
	defaultHub.register(moved)
	PublishToUsers([]uint64{101}, Event{Type: "chat_message", WorkspaceID: 8801})
	select {
	case payload := <-moved.send:
		t.Fatalf("old-workspace event reached moved member: %s", payload)
	default:
	}
}

func TestDisconnectAndPublishCannotQueueAfterRevocation(t *testing.T) {
	hub := NewHub()
	hub.setSessionValidator(func(SessionIdentity) bool { return true })
	connection := &client{
		identity: SessionIdentity{UserID: 27, AuthVersion: 2, WorkspaceID: 8801},
		send:     make(chan []byte, 256),
	}
	hub.register(connection)

	var publishers sync.WaitGroup
	for i := 0; i < 32; i++ {
		publishers.Add(1)
		go func() {
			defer publishers.Done()
			for attempt := 0; attempt < 64; attempt++ {
				connection.enqueue([]byte(`{"type":"balance"}`))
			}
		}()
	}
	hub.disconnectUser(27)
	publishers.Wait()

	queuedBeforeCheck := len(connection.send)
	for i := 0; i < 32; i++ {
		if connection.enqueue([]byte(`{"type":"late"}`)) {
			t.Fatal("a revoked connection accepted a later event")
		}
	}
	if len(connection.send) != queuedBeforeCheck {
		t.Fatal("the socket queue changed after disconnect completed")
	}
}
