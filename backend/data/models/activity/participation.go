package activity

import "time"

// Participation records a member action on an activity (checkin, redpacket, etc.).
type Participation struct {
	ID             uint64    `gorm:"primaryKey" json:"id"`
	UserID         uint64    `gorm:"not null;uniqueIndex:idx_participation_daily_unique,priority:1" json:"user_id"`
	ActivityID     uint64    `gorm:"not null;uniqueIndex:idx_participation_daily_unique,priority:2" json:"activity_id"`
	Action         string    `gorm:"size:30;not null;uniqueIndex:idx_participation_daily_unique,priority:3" json:"action"` // checkin / redpacket
	BizDate        string    `gorm:"size:10;not null;uniqueIndex:idx_participation_daily_unique,priority:4" json:"biz_date"`
	RewardCents    int64     `gorm:"not null;default:0" json:"-"`
	Streak         int       `gorm:"not null;default:0" json:"streak"`
	ParticipatedAt time.Time `gorm:"not null;index" json:"participated_at"`
	CreatedAt      time.Time `json:"created_at"`
}

func (Participation) TableName() string { return "activity_participations" }
