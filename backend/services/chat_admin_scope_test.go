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

func TestNormalizeAdminContextKeepsExplicitHistoricalServiceRoom(t *testing.T) {
	service := &ChatAdminService{}
	for _, roomScope := range []string{"agent:9", "tenant:4", "lobby"} {
		resolvedRoom, gameID, err := service.normalizeAdminContext("user:7", "service", roomScope, "ignored")
		if err != nil {
			t.Fatalf("normalize historic service room %q: %v", roomScope, err)
		}
		if resolvedRoom != roomScope || gameID != "service" {
			t.Fatalf("historic service room was reclassified: got %q/%q, want %q/service", resolvedRoom, gameID, roomScope)
		}
	}
}

func TestNormalizeAdminContextRejectsServiceWithoutFrozenRoom(t *testing.T) {
	service := &ChatAdminService{}
	for _, roomScope := range []string{"", "legacy", "user:7"} {
		if _, _, err := service.normalizeAdminContext("user:7", "service", roomScope, "service"); err == nil {
			t.Fatalf("service context accepted invalid frozen room %q", roomScope)
		}
	}
}
