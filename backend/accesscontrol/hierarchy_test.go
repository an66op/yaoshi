package accesscontrol

import (
	"backend/data/models/user"
	"testing"
)

func TestStandaloneAgentHierarchy(t *testing.T) {
	agent := user.User{UserID: 9, Role: "agent", Status: 1, AgentRoomCode: "88001"}
	active, err := AgentHierarchyActive(nil, agent)
	if err != nil || !active {
		t.Fatalf("standalone active agent should be valid: active=%v err=%v", active, err)
	}

	agent.Status = 0
	active, err = AgentHierarchyActive(nil, agent)
	if err != nil || active {
		t.Fatalf("disabled agent must be rejected: active=%v err=%v", active, err)
	}
}

func TestLobbyAndAgentRoomShape(t *testing.T) {
	lobbyMember := user.User{UserID: 30, Role: "member", Status: 1}
	active, err := AccountRoomActive(nil, lobbyMember)
	if err != nil || !active {
		t.Fatalf("active lobby member should remain valid: active=%v err=%v", active, err)
	}

	agentWithoutRoom := user.User{UserID: 9, Role: "agent", Status: 1}
	active, err = AccountRoomActive(nil, agentWithoutRoom)
	if err != nil || active {
		t.Fatalf("agent without a room number must be rejected: active=%v err=%v", active, err)
	}
}
