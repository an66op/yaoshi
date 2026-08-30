package ws

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Draws are shared public facts, not personal bet acknowledgements. Merely
// watching a room must receive every completed issue without placing a bet.
func TestNotifyDrawPublishesEveryIssueToSpectators(t *testing.T) {
	previous := defaultHub
	defaultHub = NewHub()
	t.Cleanup(func() { defaultHub = previous })
	defaultHub.setSessionValidator(func(SessionIdentity) bool { return true })

	watchers := []*client{
		{identity: SessionIdentity{UserID: 101, AuthVersion: 1, WorkspaceID: 9}, send: make(chan []byte, 2)},
		{identity: SessionIdentity{UserID: 202, AuthVersion: 1, WorkspaceID: 10}, send: make(chan []byte, 2)},
	}
	for _, watcher := range watchers {
		defaultHub.register(watcher)
		t.Cleanup(func() { defaultHub.unregister(watcher) })
	}
	issues := []string{"34137153", "34137154"}
	draws := [][]int{
		{8, 5, 1, 9, 7, 6, 10, 3, 2, 4},
		{1, 2, 7, 4, 9, 8, 10, 6, 3, 5},
	}
	for index, issue := range issues {
		NotifyDraw("speed-racing", issue, draws[index])
	}

	for _, watcher := range watchers {
		previousID := ""
		for index, issue := range issues {
			select {
			case payload := <-watcher.send:
				var frame struct {
					Event
					Data struct {
						GameID  string `json:"game_id"`
						Issue   string `json:"issue"`
						Numbers []int  `json:"numbers"`
					} `json:"data"`
				}
				if err := json.Unmarshal(payload, &frame); err != nil {
					t.Fatal(err)
				}
				if frame.Type != "draw_update" || frame.WorkspaceID != 0 || frame.RoomScope != "*" ||
					frame.GameID != "speed-racing" || frame.Issue != issue || frame.Data.GameID != frame.GameID || frame.Data.Issue != issue {
					t.Fatalf("spectator %d received an incomplete or room-bound draw: %s", watcher.identity.UserID, payload)
				}
				if !reflect.DeepEqual(frame.Data.Numbers, draws[index]) {
					t.Fatalf("issue %s was replaced by another draw: got %v, want %v", issue, frame.Data.Numbers, draws[index])
				}
				if frame.ID == "" || frame.ID == previousID || frame.ServerAt.IsZero() {
					t.Fatalf("successive draws must have distinct push identities: %s", payload)
				}
				previousID = frame.ID
			default:
				t.Fatalf("spectator %d missed issue %s", watcher.identity.UserID, issue)
			}
		}
	}
}

func TestSettlementInboxEventRemainsPrivate(t *testing.T) {
	previous := defaultHub
	defaultHub = NewHub()
	t.Cleanup(func() { defaultHub = previous })
	defaultHub.setSessionValidator(func(SessionIdentity) bool { return true })
	owner := &client{identity: SessionIdentity{UserID: 101, AuthVersion: 1, WorkspaceID: 9}, send: make(chan []byte, 1)}
	spectator := &client{identity: SessionIdentity{UserID: 102, AuthVersion: 1, WorkspaceID: 9}, send: make(chan []byte, 1)}
	defaultHub.register(owner)
	defaultHub.register(spectator)
	t.Cleanup(func() { defaultHub.unregister(owner); defaultHub.unregister(spectator) })

	NotifyUser(101, "notification", map[string]any{
		"workspace_id": uint64(9), "room_scope": "agent:9", "game_id": "speed-racing",
		"issue": "34137173", "category": "winning", "payout_amount": 267.30,
	})
	select {
	case payload := <-owner.send:
		var frame Event
		if err := json.Unmarshal(payload, &frame); err != nil {
			t.Fatal(err)
		}
		data, ok := frame.Data.(map[string]any)
		if !ok || frame.Type != "notification" || data["category"] != "winning" || data["payout_amount"] != 267.30 {
			t.Fatalf("financial inbox event was lost or converted into a room message: %s", payload)
		}
	default:
		t.Fatal("winning notice must remain available to its owner's notification centre")
	}
	select {
	case payload := <-spectator.send:
		t.Fatalf("spectator received another member's private financial notice: %s", payload)
	default:
	}
}
