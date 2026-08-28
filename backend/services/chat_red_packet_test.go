package services

import (
	"backend/data/models/chat"
	"testing"
)

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

func TestSendRedPacketRejectsNonLobbyConversation(t *testing.T) {
	service := NewChatAdminService(identityDryRunDB(t))
	_, err := service.SendRedPacket("agent:9", "agent:9", "speed-racing", ChatRedPacketInput{
		Count: 1, TotalAmount: 1, Greeting: "恭喜发财", Cover: "classic",
	}, "测试管理员")
	if err == nil || err.Error() != "房间红包只能发送到房间大厅" {
		t.Fatalf("non-lobby room packet error = %v", err)
	}
}

func TestPlanRedPacketClosePreservesFunds(t *testing.T) {
	cases := []struct {
		name       string
		packet     chat.RedPacket
		refund     int64
		refunded   int64
		finalState string
		wantError  bool
	}{
		{name: "unused reserve", packet: chat.RedPacket{TotalCents: 1000, RemainingCents: 1000, FundingUserID: 9, FundingStatus: "reserved"}, refund: 1000, refunded: 1000, finalState: "refunded"},
		{name: "partly claimed", packet: chat.RedPacket{TotalCents: 1000, RemainingCents: 350, FundingUserID: 9, FundingStatus: "partially_released"}, refund: 350, refunded: 350, finalState: "refunded"},
		{name: "legacy never mints", packet: chat.RedPacket{TotalCents: 1000, RemainingCents: 1000, FundingStatus: "legacy_unfunded"}, refund: 0, refunded: 0, finalState: "legacy_unfunded"},
		{name: "unknown fails closed", packet: chat.RedPacket{TotalCents: 1000, RemainingCents: 1000, FundingUserID: 9, FundingStatus: "mystery"}, wantError: true},
		{name: "refunded reserve cannot refund twice", packet: chat.RedPacket{TotalCents: 1000, RemainingCents: 100, RefundedCents: 900, FundingUserID: 9, FundingStatus: "refunded"}, wantError: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			plan, err := planRedPacketClose(test.packet)
			if test.wantError {
				if err == nil {
					t.Fatal("unsafe funding state was accepted")
				}
				return
			}
			if err != nil {
				t.Fatalf("plan failed: %v", err)
			}
			if plan.RefundCents != test.refund || plan.FinalRefundedCents != test.refunded || plan.FinalFundingStatus != test.finalState {
				t.Fatalf("unexpected plan: %#v", plan)
			}
		})
	}
}
