package chat

import "time"

// ReadCursor stores one operator's durable read position for one exact chat
// conversation. Keeping this separate from notifications prevents one admin
// from clearing another admin's customer-service unread state.
type ReadCursor struct {
	ID                uint64    `gorm:"primaryKey" json:"id"`
	OperatorUserID    uint64    `gorm:"not null;index;uniqueIndex:idx_chat_read_cursor,priority:1" json:"operator_user_id"`
	WorkspaceID       uint64    `gorm:"not null;index;uniqueIndex:idx_chat_read_cursor,priority:2" json:"workspace_id"`
	Scope             string    `gorm:"size:64;not null;uniqueIndex:idx_chat_read_cursor,priority:3" json:"scope"`
	RoomScope         string    `gorm:"size:64;not null;uniqueIndex:idx_chat_read_cursor,priority:4" json:"room_scope"`
	GameID            string    `gorm:"size:40;not null;uniqueIndex:idx_chat_read_cursor,priority:5" json:"game_id"`
	RoomType          string    `gorm:"size:20;not null;uniqueIndex:idx_chat_read_cursor,priority:6" json:"room_type"`
	LastReadMessageID uint64    `gorm:"not null;default:0" json:"last_read_message_id"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (ReadCursor) TableName() string { return "member_chat_read_cursors" }
