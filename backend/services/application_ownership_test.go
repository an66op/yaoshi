package services

import (
	"backend/data/models/application"
	"backend/data/models/user"
	"testing"
)

func TestApplicationOwnershipUsesRoomSnapshot(t *testing.T) {
	agentA, agentB := uint64(11), uint64(22)
	movedMember := user.User{UserID: 99, Role: "member", ParentAgentID: &agentB}
	item := application.Application{UserID: movedMember.UserID, RoomScope: "agent:11"}

	if !applicationBelongsToAgent(item, movedMember, agentA) {
		t.Fatal("the room that owned the application at creation must retain it")
	}
	if applicationBelongsToAgent(item, movedMember, agentB) {
		t.Fatal("moving the member must not transfer a pending application")
	}
}

func TestApplicationOwnershipLegacyFallbackIsNarrow(t *testing.T) {
	agentA, agentB := uint64(11), uint64(22)
	member := user.User{UserID: 99, Role: "member", ParentAgentID: &agentA}
	legacy := application.Application{UserID: member.UserID}

	if !applicationBelongsToAgent(legacy, member, agentA) {
		t.Fatal("legacy empty snapshots should remain available to the current owner")
	}
	if applicationBelongsToAgent(legacy, member, agentB) {
		t.Fatal("legacy empty snapshots must never be visible to another agent")
	}
	if applicationBelongsToAgent(application.Application{RoomScope: "lobby"}, member, agentA) {
		t.Fatal("lobby applications must not be claimable by an agent")
	}
}
