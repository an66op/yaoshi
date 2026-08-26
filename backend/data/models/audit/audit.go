package audit

import "time"

// Log is an append-only record of a privileged mutation. It deliberately
// stores request metadata instead of request bodies so passwords and payment
// account details never enter the audit table.
type Log struct {
	ID         uint64    `gorm:"primaryKey" json:"id"`
	ActorID    uint64    `gorm:"not null;index" json:"actor_id"`
	ActorName  string    `gorm:"size:80;not null" json:"actor_name"`
	ActorRole  string    `gorm:"size:20;not null;index" json:"actor_role"`
	RoomScope  string    `gorm:"size:64;index" json:"room_scope,omitempty"`
	Method     string    `gorm:"size:10;not null" json:"method"`
	Path       string    `gorm:"size:240;not null;index" json:"path"`
	StatusCode int       `gorm:"not null" json:"status_code"`
	RequestID  string    `gorm:"size:96;index" json:"request_id,omitempty"`
	IP         string    `gorm:"size:80" json:"ip,omitempty"`
	CreatedAt  time.Time `gorm:"index" json:"created_at"`
}

func (Log) TableName() string { return "admin_audit_logs" }
