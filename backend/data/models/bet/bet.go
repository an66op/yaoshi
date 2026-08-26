package bet

import "time"

// Bet is a single stake placed on a game issue. Amounts use integer cents.
// Position 1-5 = ball slots, 6 = sum column used by the live monitor matrix.
type Bet struct {
	ID     uint64 `gorm:"primaryKey" json:"id"`
	GameID string `gorm:"size:40;not null;index;uniqueIndex:idx_bet_dedupe;index:idx_bet_feed_scope,priority:2" json:"game_id"`
	Issue  string `gorm:"size:64;not null;index;uniqueIndex:idx_bet_dedupe;index:idx_bet_feed_scope,priority:3" json:"issue"`
	// RoomScope is frozen at placement time. It prevents a member moving rooms
	// later from exposing a previous room's live betting feed.
	RoomScope   string  `gorm:"size:64;not null;default:legacy;index;uniqueIndex:idx_bet_dedupe;index:idx_bet_feed_scope,priority:1" json:"-"`
	UserID      uint64  `gorm:"not null;index;uniqueIndex:idx_bet_dedupe" json:"user_id"`
	Username    string  `gorm:"size:50;not null;index" json:"username"`
	PlayCode    string  `gorm:"size:40;not null;uniqueIndex:idx_bet_dedupe" json:"play_code"`
	PlayName    string  `gorm:"size:40;not null" json:"play_name"`
	Position    int     `gorm:"not null;default:0;uniqueIndex:idx_bet_dedupe" json:"position"`
	Selection   string  `gorm:"size:40;not null;uniqueIndex:idx_bet_dedupe" json:"selection"`
	AmountCents int64   `gorm:"not null" json:"-"`
	Odds        float64 `gorm:"not null;default:1.993" json:"odds"`
	Status      string  `gorm:"size:20;not null;default:pending;index" json:"status"`
	PayoutCents int64   `gorm:"not null;default:0" json:"-"`
	FlyCents    int64   `gorm:"not null;default:0" json:"-"`
	// Financial terms are immutable placement/settlement snapshots.  Reports
	// must never recalculate old business after room settings change.
	RebateRateSnapshot     float64    `gorm:"not null;default:0" json:"rebate_rate_snapshot"`
	RebateCents            int64      `gorm:"not null;default:0" json:"-"`
	AgentShareRateSnapshot float64    `gorm:"not null;default:0" json:"agent_share_rate_snapshot"`
	AgentShareCents        int64      `gorm:"not null;default:0" json:"-"`
	SettledAt              *time.Time `gorm:"index" json:"settled_at,omitempty"`
	Remark                 string     `gorm:"size:300" json:"remark"`
	Operator               string     `gorm:"size:80" json:"operator"`
	// ReconciliationStatus never changes a historic financial result. Legacy
	// rows that cannot be matched to a durable issue are queued for review.
	ReconciliationStatus string    `gorm:"size:24;not null;default:normal;index" json:"reconciliation_status"`
	ReconciliationNote   string    `gorm:"size:500" json:"reconciliation_note,omitempty"`
	CreatedAt            time.Time `gorm:"index;index:idx_bet_feed_scope,priority:4" json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

func (Bet) TableName() string { return "lottery_bets" }

// AssistantRequest makes compact-text betting safe to retry. A client can
// resend the same request_id after a transient network failure without paying
// for the ticket twice.
type AssistantRequest struct {
	ID         uint64    `gorm:"primaryKey" json:"id"`
	UserID     uint64    `gorm:"not null;uniqueIndex:idx_assistant_request" json:"user_id"`
	RequestID  string    `gorm:"size:96;not null;uniqueIndex:idx_assistant_request" json:"request_id"`
	ResultJSON string    `gorm:"type:text" json:"-"`
	CreatedAt  time.Time `gorm:"index" json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (AssistantRequest) TableName() string { return "lottery_assistant_requests" }

// BetRequest reserves a member supplied request id before a direct bet is
// charged.  A network retry therefore returns the original receipt instead of
// deducting the same stake twice.
type BetRequest struct {
	ID         uint64    `gorm:"primaryKey" json:"id"`
	UserID     uint64    `gorm:"not null;uniqueIndex:idx_bet_request" json:"user_id"`
	RequestID  string    `gorm:"size:96;not null;uniqueIndex:idx_bet_request" json:"request_id"`
	Status     string    `gorm:"size:20;not null;default:processing;index" json:"status"`
	ResultJSON string    `gorm:"type:text" json:"-"`
	LastError  string    `gorm:"size:500" json:"last_error,omitempty"`
	CreatedAt  time.Time `gorm:"index" json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (BetRequest) TableName() string { return "lottery_bet_requests" }
