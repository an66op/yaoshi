package chat

import "time"

// RedPacket is one persisted envelope sent into one room conversation.
// Money and claim counts are stored in integer units so concurrent claims can
// be settled transactionally without rounding drift.
type RedPacket struct {
	ID             uint64 `gorm:"primaryKey" json:"id"`
	MessageID      uint64 `gorm:"not null;uniqueIndex" json:"message_id"`
	Scope          string `gorm:"size:64;not null;index" json:"scope"`
	RoomScope      string `gorm:"size:64;not null;index" json:"room_scope"`
	GameID         string `gorm:"size:40;not null;index" json:"game_id"`
	TotalCents     int64  `gorm:"not null" json:"-"`
	RemainingCents int64  `gorm:"not null" json:"-"`
	PacketCount    int    `gorm:"not null" json:"packet_count"`
	ClaimedCount   int    `gorm:"not null;default:0" json:"claimed_count"`
	// MinDailyTurnoverCents is the settled stake required on the claim day.
	// A zero value keeps the envelope open to every member in the room.
	MinDailyTurnoverCents int64     `gorm:"not null;default:0" json:"-"`
	Greeting              string    `gorm:"size:60;not null" json:"greeting"`
	Cover                 string    `gorm:"size:24;not null" json:"cover"`
	Status                string    `gorm:"size:20;not null;default:active;index" json:"status"`
	CreatedAt             time.Time `gorm:"index" json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

func (RedPacket) TableName() string { return "chat_red_packets" }

// RedPacketClaim guarantees one member can claim one envelope only once.
type RedPacketClaim struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	PacketID    uint64    `gorm:"not null;uniqueIndex:idx_chat_packet_member,priority:1;index" json:"packet_id"`
	UserID      uint64    `gorm:"not null;uniqueIndex:idx_chat_packet_member,priority:2;index" json:"user_id"`
	AmountCents int64     `gorm:"not null" json:"-"`
	CreatedAt   time.Time `gorm:"index" json:"created_at"`
}

func (RedPacketClaim) TableName() string { return "chat_red_packet_claims" }
