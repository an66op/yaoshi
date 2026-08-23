package activity

import "time"

// Activity is an operational campaign managed from the admin console.
type Activity struct {
	ID                 uint64    `gorm:"primaryKey" json:"id"`
	Type               string    `gorm:"size:30;not null;index" json:"type"` // checkin / banner / invite / redpacket
	Title              string    `gorm:"size:120;not null" json:"title"`
	Subtitle           string    `gorm:"size:200" json:"subtitle"`
	Status             string    `gorm:"size:20;not null;default:draft;index" json:"status"` // draft / active / ended
	Cover              string    `gorm:"size:320" json:"cover"`
	RewardCents        int64     `gorm:"not null;default:0" json:"-"`
	PoolTotalCents     int64     `gorm:"not null;default:0" json:"-"`
	PoolRemainingCents int64     `gorm:"not null;default:0" json:"-"`
	ConfigJSON         string    `gorm:"type:text" json:"-"`
	Participants       int64     `gorm:"not null;default:0" json:"participants"`
	SortOrder          int       `gorm:"not null;default:0" json:"sort_order"`
	StartsAt           *time.Time `json:"starts_at"`
	EndsAt             *time.Time `json:"ends_at"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (Activity) TableName() string { return "ops_activities" }
