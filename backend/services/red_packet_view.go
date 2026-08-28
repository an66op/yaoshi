package services

import (
	"backend/data/models/chat"
	"time"

	"gorm.io/gorm"
)

type redPacketViewState struct {
	Status         string
	FundingStatus  string
	ClaimedCount   int
	RemainingCents int64
	RefundedCents  int64
	ExpiresAt      *time.Time
	ClosedAt       *time.Time
	CloseReason    string
}

func loadRedPacketViewStates(db *gorm.DB, rows []chat.Message) (map[uint64]redPacketViewState, error) {
	messageIDs := make([]uint64, 0)
	for _, row := range rows {
		if row.MessageType == "redpacket" {
			messageIDs = append(messageIDs, row.ID)
		}
	}
	result := make(map[uint64]redPacketViewState, len(messageIDs))
	if len(messageIDs) == 0 {
		return result, nil
	}
	var packets []chat.RedPacket
	if err := db.Select("message_id", "status", "funding_status", "claimed_count", "remaining_cents", "refunded_cents", "expires_at", "closed_at", "close_reason").
		Where("message_id IN ?", messageIDs).Find(&packets).Error; err != nil {
		return nil, err
	}
	for _, packet := range packets {
		result[packet.MessageID] = redPacketViewState{
			Status: packet.Status, FundingStatus: packet.FundingStatus, ClaimedCount: packet.ClaimedCount,
			RemainingCents: packet.RemainingCents, RefundedCents: packet.RefundedCents,
			ExpiresAt: packet.ExpiresAt, ClosedAt: packet.ClosedAt, CloseReason: packet.CloseReason,
		}
	}
	return result, nil
}
