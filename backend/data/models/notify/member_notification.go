package notify

import (
	"time"

	"gorm.io/gorm"
)

// MemberNotification is a user-scoped inbox item for the mobile app.
type MemberNotification struct {
	ID          uint64 `gorm:"primaryKey" json:"id"`
	WorkspaceID uint64 `gorm:"not null;default:0;index" json:"workspace_id"`
	UserID      uint64 `gorm:"not null;index" json:"user_id"`
	// GameID and RoomScope make a settlement notice replayable only inside the
	// game room in which the ticket was placed.  They are snapshots: moving a
	// member to another agent room must not rewrite historic settlement events.
	GameID    string `gorm:"size:40;index" json:"game_id"`
	RoomScope string `gorm:"size:64;index" json:"room_scope"`
	// EventKey is populated for idempotent domain events such as one member's
	// settlement for one game/issue/room.  Ordinary notices keep it empty.
	EventKey       string     `gorm:"size:180;index" json:"-"`
	Title          string     `gorm:"size:120;not null" json:"title"`
	Content        string     `gorm:"type:text" json:"content"`
	Level          string     `gorm:"size:20;not null;default:info" json:"level"`
	Category       string     `gorm:"size:30;not null;default:system;index" json:"category"` // system / account / activity / winning
	Link           string     `gorm:"size:300" json:"link"`
	Read           bool       `gorm:"not null;default:false;index" json:"read"`
	GameName       string     `gorm:"size:80" json:"game_name"`
	Issue          string     `gorm:"size:64" json:"issue"`
	DrawNumbers    string     `gorm:"size:120" json:"-"`
	DrawAt         *time.Time `json:"draw_at"`
	BetCount       int        `gorm:"not null;default:0" json:"bet_count"`
	WonCount       int        `gorm:"not null;default:0" json:"won_count"`
	StakeCents     int64      `gorm:"not null;default:0" json:"-"`
	PayoutCents    int64      `gorm:"not null;default:0" json:"-"`
	BetDetailsJSON string     `gorm:"type:text" json:"-"`
	CreatedAt      time.Time  `gorm:"index" json:"created_at"`
	// Notifications are user-facing history rather than accounting evidence.
	// Retention uses GORM soft deletion so an operator can restore an item and
	// so no cleanup can erase the settlement/ledger rows it references.
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
	DeletedBy        string         `gorm:"size:80;not null;default:''" json:"-"`
	CleanupRequestID string         `gorm:"size:96;not null;default:'';index" json:"-"`
}

func (MemberNotification) TableName() string { return "member_notifications" }
