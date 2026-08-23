package notify

import "time"

// MemberNotification is a user-scoped inbox item for the mobile app.
type MemberNotification struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	UserID    uint64    `gorm:"not null;index" json:"user_id"`
	Title     string    `gorm:"size:120;not null" json:"title"`
	Content   string    `gorm:"type:text" json:"content"`
	Level     string    `gorm:"size:20;not null;default:info" json:"level"`
	Category  string    `gorm:"size:30;not null;default:system;index" json:"category"` // system / activity / winning
	Link      string    `gorm:"size:300" json:"link"`
	Read      bool      `gorm:"not null;default:false;index" json:"read"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

func (MemberNotification) TableName() string { return "member_notifications" }
