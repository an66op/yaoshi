package chat

import (
	"time"

	"gorm.io/gorm"
)

// Message is a lightweight member chat record for group/service rooms.
type Message struct {
	ID          uint64 `gorm:"primaryKey" json:"id"`
	WorkspaceID uint64 `gorm:"not null;default:0;index;index:idx_chat_workspace_game_created,priority:1" json:"workspace_id"`
	UserID      uint64 `gorm:"not null;index" json:"user_id"`
	Username    string `gorm:"size:50;not null" json:"username"`
	Nickname    string `gorm:"size:80;not null" json:"nickname"`
	RoomType    string `gorm:"size:20;not null;index" json:"room_type"` // group / service
	// Scope identifies the audience allowed to read this message. Examples:
	// agent:42, lobby, and user:17 (one-to-one service chat).
	Scope string `gorm:"size:64;not null;default:lobby;index" json:"-"`
	// RoomScope freezes the owning room when the message is sent. GameID is the
	// lottery conversation inside that room; lobby and service are explicit
	// values rather than implicit catch-all channels.
	RoomScope   string `gorm:"size:64;not null;default:legacy;index;index:idx_chat_room_game_created,priority:1" json:"room_scope"`
	GameID      string `gorm:"size:40;not null;default:legacy;index;index:idx_chat_room_game_created,priority:2;index:idx_chat_workspace_game_created,priority:2" json:"game_id"`
	Content     string `gorm:"size:500;not null" json:"content"`
	MessageType string `gorm:"size:20;not null;default:text;index" json:"message_type"`
	ReferenceID uint64 `gorm:"not null;default:0;index" json:"reference_id,omitempty"`
	// RequestID is supplied only by the member command endpoint. A partial
	// database unique index on (user_id, request_id) makes network retries reuse
	// the original timeline message instead of creating a second command.
	RequestID string `gorm:"size:96;not null;default:'';index" json:"request_id,omitempty"`
	// Red-packet display data is snapshotted on the message so message history
	// can render without issuing one lookup per envelope.
	RedPacketCount            int            `gorm:"not null;default:0" json:"red_packet_count,omitempty"`
	RedPacketTotalCents       int64          `gorm:"not null;default:0" json:"-"`
	RedPacketMinTurnoverCents int64          `gorm:"not null;default:0" json:"-"`
	RedPacketCover            string         `gorm:"size:24;not null;default:''" json:"red_packet_cover,omitempty"`
	CreatedAt                 time.Time      `gorm:"index;index:idx_chat_room_game_created,priority:3;index:idx_chat_workspace_game_created,priority:3" json:"created_at"`
	DeletedAt                 gorm.DeletedAt `gorm:"index" json:"-"`
	DeletedBy                 string         `gorm:"size:80" json:"-"`
	CleanupRequestID          string         `gorm:"size:96;not null;default:'';index" json:"-"`
}

func (Message) TableName() string { return "member_chat_messages" }
