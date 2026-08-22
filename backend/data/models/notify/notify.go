package notify

import "time"

// Notification is an admin-visible system notice.
type Notification struct {
	ID        uint64     `gorm:"primaryKey" json:"id"`
	Title     string     `gorm:"size:120;not null" json:"title"`
	Content   string     `gorm:"size:500;not null" json:"content"`
	Level     string     `gorm:"size:20;not null;default:info" json:"level"` // info / success / warning / error
	Link      string     `gorm:"size:120" json:"link"`
	Read      bool       `gorm:"not null;default:false;index" json:"read"`
	CreatedAt time.Time  `gorm:"index" json:"created_at"`
	ReadAt    *time.Time `json:"read_at"`
}

func (Notification) TableName() string { return "admin_notifications" }
