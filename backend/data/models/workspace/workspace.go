package workspace

import "time"

const (
	TypePlatform = "platform"
	TypeTenant   = "tenant"
	TypeAgent    = "agent"
)

// Workspace is the immutable ownership boundary for configuration and
// business data. ParentID describes the organisation tree only; it never
// grants the parent permission to read the child's runtime business data.
type Workspace struct {
	ID   uint64 `gorm:"primaryKey" json:"id"`
	Code string `gorm:"size:40;not null;uniqueIndex" json:"code"`
	// RoomCode is the public number members type when entering a room. Code is
	// an internal organisation identifier and must never be exposed as a room
	// number. Platform workspaces and tenants waiting for allocation keep it
	// empty; PostgreSQL enforces uniqueness only for non-empty values.
	RoomCode    string  `gorm:"size:40;not null;default:'';index" json:"room_code,omitempty"`
	Type        string  `gorm:"size:20;not null;index" json:"type"`
	OwnerUserID uint64  `gorm:"not null;uniqueIndex" json:"owner_user_id"`
	ParentID    *uint64 `gorm:"index" json:"parent_id,omitempty"`
	Scope       string  `gorm:"size:64;not null;uniqueIndex" json:"scope"`
	Name        string  `gorm:"size:80;not null" json:"name"`
	Logo        string  `gorm:"type:text" json:"logo,omitempty"`
	Status      int     `gorm:"not null;default:1;index" json:"status"`
	// RobotQuota is the number of pre-provisioned robot identities the room
	// owner may manage and run. The platform keeps ten physical identities per
	// workspace, while an upper-level account allocates only 0-10 of them.
	RobotQuota int       `gorm:"not null;default:10" json:"robot_quota"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (Workspace) TableName() string { return "workspaces" }

// Membership allows one identity to enter multiple workspaces without
// rewriting historical ownership on bets, messages, wallets or notices.
type Membership struct {
	ID          uint64 `gorm:"primaryKey" json:"id"`
	WorkspaceID uint64 `gorm:"not null;uniqueIndex:idx_workspace_member,priority:1;index" json:"workspace_id"`
	UserID      uint64 `gorm:"not null;uniqueIndex:idx_workspace_member,priority:2;index" json:"user_id"`
	Role        string `gorm:"size:20;not null;default:member" json:"role"`
	Status      int    `gorm:"not null;default:1;index" json:"status"`
	// OddsMultiplier belongs to this room membership, not the global account.
	// It therefore follows the room when an operator approves entry and cannot
	// leak into another workspace when the member changes rooms.
	OddsMultiplier float64   `gorm:"not null;default:1" json:"odds_multiplier"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (Membership) TableName() string { return "workspace_memberships" }

// RobotProfile keeps automation concerns out of the member identity row.
// A robot is still an ordinary persisted member for betting and settlement.
type RobotProfile struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	WorkspaceID uint64    `gorm:"not null;index;uniqueIndex:idx_workspace_robot_user,priority:1" json:"workspace_id"`
	UserID      uint64    `gorm:"not null;uniqueIndex;uniqueIndex:idx_workspace_robot_user,priority:2" json:"user_id"`
	Avatar      string    `gorm:"size:300;not null;default:''" json:"avatar"`
	Enabled     bool      `gorm:"not null;default:true;index" json:"enabled"`
	ActiveStart string    `gorm:"size:5;not null;default:''" json:"active_start"`
	ActiveEnd   string    `gorm:"size:5;not null;default:''" json:"active_end"`
	MinBetCents int64     `gorm:"not null;default:100" json:"-"`
	MaxBetCents int64     `gorm:"not null;default:5000" json:"-"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (RobotProfile) TableName() string { return "workspace_robot_profiles" }

type RobotGame struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	RobotID   uint64    `gorm:"not null;uniqueIndex:idx_robot_game,priority:1;index" json:"robot_id"`
	GameID    string    `gorm:"size:40;not null;uniqueIndex:idx_robot_game,priority:2;index" json:"game_id"`
	CreatedAt time.Time `json:"created_at"`
}

func (RobotGame) TableName() string { return "workspace_robot_games" }

// RobotSetting controls one workspace scheduler. Enabled is authoritative:
// changing it starts/stops automatic execution without a second save action.
type RobotSetting struct {
	WorkspaceID    uint64     `gorm:"primaryKey" json:"workspace_id"`
	Enabled        bool       `gorm:"not null;default:false;index" json:"enabled"`
	IntervalSecs   int        `gorm:"not null;default:60" json:"interval_secs"`
	BetsPerCycle   int        `gorm:"not null;default:1" json:"bets_per_cycle"`
	DailyBetLimit  int        `gorm:"not null;default:200" json:"daily_bet_limit"`
	MaxPendingBets int        `gorm:"not null;default:50" json:"max_pending_bets"`
	PauseReason    string     `gorm:"size:240;not null;default:''" json:"pause_reason,omitempty"`
	LastRunAt      *time.Time `json:"last_run_at,omitempty"`
	LastError      string     `gorm:"size:500" json:"last_error,omitempty"`
	TodayBets      int64      `gorm:"-" json:"today_bets"`
	PendingBets    int64      `gorm:"-" json:"pending_bets"`
	RobotQuota     int        `gorm:"-" json:"robot_quota"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (RobotSetting) TableName() string { return "workspace_robot_settings" }

// RobotResetReceipt is the durable idempotency boundary for a whole-workspace
// robot reset. Unlike balance history it is never lifecycle-archived, so a
// retry remains recognizable after financial ledger rows move to cold storage.
type RobotResetReceipt struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	WorkspaceID   uint64    `gorm:"not null;uniqueIndex:idx_workspace_robot_reset_request,priority:1;index" json:"workspace_id"`
	RequestIDHash string    `gorm:"size:64;not null;uniqueIndex:idx_workspace_robot_reset_request,priority:2" json:"-"`
	PayloadHash   string    `gorm:"size:32;not null" json:"-"`
	Mode          string    `gorm:"size:12;not null" json:"mode"`
	RobotCount    int       `gorm:"not null" json:"robot_count"`
	CreatedAt     time.Time `json:"created_at"`
}

func (RobotResetReceipt) TableName() string { return "workspace_robot_reset_receipts" }
