package notify

import (
	"time"

	"gorm.io/gorm"
)

// Notification is an admin-visible system notice.
type Notification struct {
	ID               uint64         `gorm:"primaryKey" json:"id"`
	WorkspaceID      uint64         `gorm:"not null;default:0;index" json:"workspace_id"`
	Title            string         `gorm:"size:120;not null" json:"title"`
	Content          string         `gorm:"size:500;not null" json:"content"`
	Level            string         `gorm:"size:20;not null;default:info" json:"level"` // info / success / warning / error
	Link             string         `gorm:"size:120" json:"link"`
	Read             bool           `gorm:"not null;default:false;index" json:"read"`
	CreatedAt        time.Time      `gorm:"index" json:"created_at"`
	ReadAt           *time.Time     `json:"read_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
	DeletedBy        string         `gorm:"size:80;not null;default:''" json:"-"`
	CleanupRequestID string         `gorm:"size:96;not null;default:'';index" json:"-"`
}

func (Notification) TableName() string { return "admin_notifications" }
