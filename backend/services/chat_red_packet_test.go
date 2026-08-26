package services

import "testing"

func TestDrawChatRedPacketRewardKeepsOneCentPerRemainingPacket(t *testing.T) {
	remaining := int64(10_000)
	for slots := 100; slots > 0; slots-- {
		reward, err := drawChatRedPacketReward(remaining, slots)
		if err != nil {
			t.Fatalf("drawChatRedPacketReward(%d, %d): %v", remaining, slots, err)
		}
		if reward < 1 {
			t.Fatalf("reward must be positive, got %d", reward)
		}
		if remaining-reward < int64(slots-1) {
			t.Fatalf("reward %d leaves %d cents for %d packets", reward, remaining-reward, slots-1)
		}
		remaining -= reward
	}
	if remaining != 0 {
		t.Fatalf("final packet must consume all funds, %d cents left", remaining)
	}
}

func TestDrawChatRedPacketRewardRejectsInvalidPool(t *testing.T) {
	if _, err := drawChatRedPacketReward(2, 3); err == nil {
		t.Fatal("expected pool smaller than packet count to be rejected")
	}
}
