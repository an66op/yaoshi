package activity

import "time"

// Participation records a member action on an activity (checkin, redpacket, etc.).
type Participation struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	UserID      uint64    `gorm:"not null;index:idx_activity_user_day,priority:1" json:"user_id"`
	ActivityID  uint64    `gorm:"not null;index:idx_activity_user_day,priority:2" json:"activity_id"`
	Action      string    `gorm:"size:30;not null;index" json:"action"` // checkin / redpacket
	RewardCents int64     `gorm:"not null;default:0" json:"-"`
	Streak      int       `gorm:"not null;default:0" json:"streak"`
	ParticipatedAt time.Time `gorm:"not null;index:idx_activity_user_day,priority:3" json:"participated_at"`
	CreatedAt   time.Time `json:"created_at"`
}

func (Participation) TableName() string { return "activity_participations" }
