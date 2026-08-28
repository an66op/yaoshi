package plan

import (
	"time"

	"gorm.io/gorm"
)

const (
	ResultPending = "pending"
	ResultHit     = "hit"
	ResultMiss    = "miss"
)

// Recommendation is room-owned editorial content.  It is deliberately
// persisted: member clients only render rows returned by the server and never
// synthesize a pick or a hit rate in the browser.
type Recommendation struct {
	ID          uint64         `gorm:"primaryKey" json:"id"`
	WorkspaceID uint64         `gorm:"not null;index" json:"workspace_id"`
	GameID      string         `gorm:"size:40;not null;index" json:"game_id"`
	Issue       string         `gorm:"size:64;not null;index" json:"issue"`
	MasterName  string         `gorm:"size:60;not null" json:"master_name"`
	MasterTitle string         `gorm:"size:80;not null;default:''" json:"master_title"`
	MasterColor string         `gorm:"size:16;not null;default:'#2aa9b3'" json:"master_color"`
	Numbers     string         `gorm:"size:120;not null;default:''" json:"-"`
	Size        string         `gorm:"size:4;not null;default:''" json:"size"`
	Parity      string         `gorm:"size:4;not null;default:''" json:"parity"`
	Result      string         `gorm:"size:16;not null;default:'pending';index" json:"result"`
	Note        string         `gorm:"size:500;not null;default:''" json:"note"`
	Enabled     bool           `gorm:"not null;default:true;index" json:"enabled"`
	SortOrder   int            `gorm:"not null;default:100" json:"sort_order"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Recommendation) TableName() string { return "plan_recommendations" }
