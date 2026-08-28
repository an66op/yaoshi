package profitshare

import "time"

// DailyRecord is the durable reconciliation record for one agent room and
// settlement day. AccruedShareCents comes from immutable bet snapshots, while
// PaidShareCents records what has actually reached the agent balance. Keeping
// both values makes late settlements visible and lets another run credit only
// the outstanding delta.
type DailyRecord struct {
	ID                uint64     `gorm:"primaryKey" json:"id"`
	WorkspaceID       uint64     `gorm:"not null;default:0;index;uniqueIndex:idx_profit_share_agent_day,priority:1" json:"workspace_id"`
	BizDate           string     `gorm:"size:10;not null;uniqueIndex:idx_profit_share_agent_day,priority:2" json:"biz_date"`
	AgentID           uint64     `gorm:"not null;uniqueIndex:idx_profit_share_agent_day,priority:3;index" json:"agent_id"`
	RoomScope         string     `gorm:"size:80;not null;index" json:"room_scope"`
	AgentUsername     string     `gorm:"size:50;not null;index" json:"agent_username"`
	RoomCode          string     `gorm:"size:40;index" json:"room_code"`
	BetCount          int64      `gorm:"not null;default:0" json:"bet_count"`
	TurnoverCents     int64      `gorm:"not null;default:0" json:"-"`
	PayoutCents       int64      `gorm:"not null;default:0" json:"-"`
	GrossProfitCents  int64      `gorm:"not null;default:0" json:"-"`
	RebateCents       int64      `gorm:"not null;default:0" json:"-"`
	AccruedShareCents int64      `gorm:"not null;default:0" json:"-"`
	PaidShareCents    int64      `gorm:"not null;default:0" json:"-"`
	LastTransactionID uint64     `gorm:"index" json:"last_transaction_id,omitempty"`
	RunCount          int        `gorm:"not null;default:0" json:"run_count"`
	Status            string     `gorm:"size:20;not null;default:pending;index" json:"status"`
	Operator          string     `gorm:"size:80" json:"operator"`
	LastPaidAt        *time.Time `json:"last_paid_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (DailyRecord) TableName() string { return "agent_profit_share_records" }
