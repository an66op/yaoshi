package plan

import "time"

// Automation is opt-in for one room. Reading it must never create or enable it.
type Automation struct {
	WorkspaceID      uint64     `gorm:"primaryKey" json:"workspace_id"`
	Enabled          bool       `gorm:"not null;default:false;index" json:"enabled"`
	Mode             string     `gorm:"size:16;not null;default:demo" json:"mode"`
	GameIDsJSON      string     `gorm:"type:text;not null;default:'[]'" json:"-"`
	PositionsJSON    string     `gorm:"type:text;not null;default:'[1,2,3,4,5,6,7,8,9,10]'" json:"-"`
	PlanKeysJSON     string     `gorm:"type:text;not null;default:'[]'" json:"-"`
	LastRunAt        *time.Time `json:"last_run_at"`
	LastCreatedCount int64      `gorm:"not null;default:0" json:"last_created_count"`
	LastError        string     `gorm:"size:500;not null;default:''" json:"last_error"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (Automation) TableName() string { return "plan_automations" }

// GenerationReceipt survives recommendation soft deletion. Restarting a
// worker, retrying a preview or running another instance cannot republish an
// intentionally removed recommendation for the same room/game/issue/master.
type GenerationReceipt struct {
	ID          uint64 `gorm:"primaryKey"`
	WorkspaceID uint64 `gorm:"not null;uniqueIndex:idx_plan_generation_identity"`
	GameID      string `gorm:"size:40;not null;uniqueIndex:idx_plan_generation_identity"`
	Issue       string `gorm:"size:64;not null;uniqueIndex:idx_plan_generation_identity"`
	MasterKey   string `gorm:"size:32;not null;uniqueIndex:idx_plan_generation_identity"`
	CreatedAt   time.Time
}

func (GenerationReceipt) TableName() string { return "plan_generation_receipts" }
