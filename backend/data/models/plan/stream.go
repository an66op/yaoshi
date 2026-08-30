package plan

import "time"

// A stream is one room/position/strategy subscription. Picks live once per
// cycle; each observed real period stores only a reference, not 3 expert rows.
type Stream struct {
	ID          uint64 `gorm:"primaryKey"`
	WorkspaceID uint64 `gorm:"not null;uniqueIndex:idx_plan_stream_identity"`
	GameID      string `gorm:"size:40;not null;uniqueIndex:idx_plan_stream_identity"`
	Position    int    `gorm:"not null;uniqueIndex:idx_plan_stream_identity"`
	PlanKey     string `gorm:"size:48;not null;uniqueIndex:idx_plan_stream_identity"`
	ActiveUntil *time.Time
	Revoked     bool   `gorm:"not null;default:false"`
	CycleID     uint64 `gorm:"not null;default:0"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Stream) TableName() string { return "plan_streams" }

type StreamCycle struct {
	ID               uint64 `gorm:"primaryKey"`
	StreamID         uint64 `gorm:"not null;index"`
	Periods          int    `gorm:"not null"`
	PublishedPeriods int    `gorm:"not null;default:0"`
	Status           string `gorm:"size:16;not null;default:active"`
	StartIssue       string `gorm:"size:64;not null"`
	LastIssueID      uint64 `gorm:"not null;default:0"`
	LastScheduledAt  time.Time
	PayloadJSON      string `gorm:"type:text;not null"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (StreamCycle) TableName() string { return "plan_stream_cycles" }

type StreamPeriod struct {
	ID              uint64 `gorm:"primaryKey"`
	StreamID        uint64 `gorm:"not null;uniqueIndex:idx_plan_stream_period"`
	IssueID         uint64 `gorm:"not null;uniqueIndex:idx_plan_stream_period"`
	Issue           string `gorm:"size:64;not null"`
	CycleID         uint64 `gorm:"not null;index"`
	PeriodIndex     int    `gorm:"not null"`
	ScheduledDrawAt time.Time
	CreatedAt       time.Time
}

func (StreamPeriod) TableName() string { return "plan_stream_periods" }
