package services

import "testing"

func TestNormalizeAdminContextRejectsCrossRoomGroup(t *testing.T) {
	service := &ChatAdminService{}
	if _, _, err := service.normalizeAdminContext("agent:9", "group", "agent:10", "lobby"); err == nil {
		t.Fatal("expected mismatched group room to be rejected")
	}
}

func TestNormalizeAdminContextAcceptsSelectedGroupRoom(t *testing.T) {
	service := &ChatAdminService{}
	roomScope, gameID, err := service.normalizeAdminContext("agent:9", "group", "agent:9", "speed-racing")
	if err != nil {
		t.Fatalf("normalizeAdminContext() error = %v", err)
	}
	if roomScope != "agent:9" || gameID != "speed-racing" {
		t.Fatalf("normalizeAdminContext() = %q, %q", roomScope, gameID)
	}
}
