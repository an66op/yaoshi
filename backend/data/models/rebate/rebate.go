package rebate

import "time"

// DailyRecord stores one user's rebate settlement for a calendar day (CST).
type DailyRecord struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	WorkspaceID   uint64    `gorm:"not null;default:0;index;uniqueIndex:idx_rebate_user_day,priority:1" json:"workspace_id"`
	BizDate       string    `gorm:"size:10;not null;uniqueIndex:idx_rebate_user_day,priority:2" json:"biz_date"`
	UserID        uint64    `gorm:"not null;uniqueIndex:idx_rebate_user_day,priority:3" json:"user_id"`
	Username      string    `gorm:"size:50;not null;index" json:"username"`
	TurnoverCents int64     `gorm:"not null;default:0" json:"-"`
	RatePercent   float64   `gorm:"not null;default:0" json:"rate_percent"`
	AmountCents   int64     `gorm:"not null;default:0" json:"-"`
	Status        string    `gorm:"size:20;not null;default:credited" json:"status"`
	Operator      string    `gorm:"size:80" json:"operator"`
	CreatedAt     time.Time `json:"created_at"`
}

func (DailyRecord) TableName() string { return "rebate_daily_records" }
