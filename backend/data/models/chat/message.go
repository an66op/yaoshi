package chat

import "time"

// Message is a lightweight member chat record for group/service rooms.
type Message struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	UserID   uint64 `gorm:"not null;index" json:"user_id"`
	Username string `gorm:"size:50;not null" json:"username"`
	Nickname string `gorm:"size:80;not null" json:"nickname"`
	RoomType string `gorm:"size:20;not null;index" json:"room_type"` // group / service
	// Scope identifies the audience allowed to read this message. Examples:
	// agent:42, lobby, and user:17 (one-to-one service chat).
	Scope     string     `gorm:"size:64;not null;default:lobby;index" json:"-"`
	Content   string     `gorm:"size:500;not null" json:"content"`
	CreatedAt time.Time  `gorm:"index" json:"created_at"`
	DeletedAt *time.Time `gorm:"index" json:"deleted_at,omitempty"`
	DeletedBy string     `gorm:"size:80" json:"-"`
}

func (Message) TableName() string { return "member_chat_messages" }
