package audit

import "time"

// SystemEvent is an append-only operational transition emitted by the draw
// scheduler. Unlike Log, it never represents an administrator or an HTTP
// mutation. Only transitions are persisted, so frequent healthy polling does
// not turn this table into a raw request dump.
type SystemEvent struct {
	ID                uint64    `gorm:"primaryKey" json:"id"`
	Category          string    `gorm:"size:24;not null;index" json:"category"`
	EventType         string    `gorm:"size:40;not null;index" json:"event_type"`
	Level             string    `gorm:"size:12;not null" json:"level"`
	Status            string    `gorm:"size:20;not null;index" json:"status"`
	SourceGroup       string    `gorm:"size:48;not null;default:'';index" json:"source_group,omitempty"`
	GameID            string    `gorm:"size:40;not null;default:'';index" json:"game_id,omitempty"`
	JobID             string    `gorm:"size:64;not null;default:''" json:"job_id,omitempty"`
	Message           string    `gorm:"size:500;not null;default:''" json:"message"`
	Imported          int       `gorm:"not null;default:0" json:"imported"`
	LatestIssue       string    `gorm:"size:64;not null;default:''" json:"latest_issue,omitempty"`
	ConsecutiveErrors int       `gorm:"not null;default:0" json:"consecutive_errors"`
	CreatedAt         time.Time `gorm:"index" json:"created_at"`
}

func (SystemEvent) TableName() string { return "system_event_logs" }
