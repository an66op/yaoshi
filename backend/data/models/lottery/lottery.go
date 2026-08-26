package lottery

import "time"

// Game is the operational configuration displayed in the admin dashboard.
// Financial limits and play rules will be kept in separate models so this
// table stays safe to query frequently from the live dashboard.
type Game struct {
	ID            string     `gorm:"primaryKey;size:40" json:"id"`
	Code          string     `gorm:"size:40;uniqueIndex;not null" json:"code"`
	Name          string     `gorm:"size:80;not null" json:"name"`
	Category      string     `gorm:"size:40;not null" json:"category"`
	Badge         string     `gorm:"size:24;not null" json:"badge"`
	BadgeColor    string     `gorm:"size:24;not null" json:"badge_color"`
	Enabled       bool       `gorm:"not null;default:true" json:"enabled"`
	SortOrder     int        `gorm:"not null;default:0" json:"sort_order"`
	DrawInterval  int        `gorm:"not null;default:300" json:"draw_interval"`
	NextDrawAt    time.Time  `json:"next_draw_at"`
	SourceKind    string     `gorm:"size:20;not null;default:platform" json:"source_kind"`
	SourceName    string     `gorm:"size:80" json:"source_name"`
	SourceURL     string     `gorm:"size:320" json:"source_url"`
	SyncStatus    string     `gorm:"size:20;not null;default:idle" json:"sync_status"`
	LastSyncAt    *time.Time `json:"last_sync_at"`
	LastSyncError string     `gorm:"size:500" json:"last_sync_error"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (Game) TableName() string { return "lottery_games" }

const (
	IssueStatusPending   = "pending"
	IssueStatusAccepting = "accepting"
	IssueStatusSealed    = "sealed"
	IssueStatusAwaiting  = "awaiting_draw"
	IssueStatusSettling  = "settling"
	IssueStatusSettled   = "settled"
	IssueStatusError     = "error"
)

// Issue is the durable lifecycle record for one game period.  It is additive
// to the historic draw/bet tables: old records are preserved and can be
// reconciled without guessing a result or changing a member balance.
type Issue struct {
	ID         uint64     `gorm:"primaryKey" json:"id"`
	GameID     string     `gorm:"size:40;not null;index;uniqueIndex:idx_lottery_issue_game_issue" json:"game_id"`
	Issue      string     `gorm:"size:64;not null;index;uniqueIndex:idx_lottery_issue_game_issue" json:"issue"`
	Status     string     `gorm:"size:24;not null;default:pending;index" json:"status"`
	SourceMode string     `gorm:"size:20;not null;default:platform" json:"source_mode"`
	AcceptAt   time.Time  `gorm:"not null" json:"accept_at"`
	SealAt     time.Time  `gorm:"not null;index" json:"seal_at"`
	DrawAt     *time.Time `json:"draw_at,omitempty"`
	SettledAt  *time.Time `json:"settled_at,omitempty"`
	LastError  string     `gorm:"size:500" json:"last_error,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (Issue) TableName() string { return "lottery_issues" }

// Draw stores immutable, published draw results. Numbers are kept as a small
// comma-separated value so the schema remains portable between local Postgres
// and production Postgres deployments.
type Draw struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	GameID    string    `gorm:"size:40;not null;index;uniqueIndex:idx_lottery_draw_game_issue" json:"game_id"`
	Issue     string    `gorm:"size:64;not null;index;uniqueIndex:idx_lottery_draw_game_issue" json:"issue"`
	Numbers   string    `gorm:"size:120;not null" json:"-"`
	DrawAt    time.Time `gorm:"not null;index" json:"draw_at"`
	CreatedAt time.Time `json:"created_at"`
}

func (Draw) TableName() string { return "lottery_draws" }
