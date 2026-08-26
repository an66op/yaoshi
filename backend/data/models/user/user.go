package user

import (
	"gorm.io/gorm"
	"time"
)

// User 用户表模型
type User struct {
	UserID uint64 `gorm:"primaryKey" json:"user_id"`
	// PublicID is the stable member number shown in the product. It must not
	// change when a member changes their nickname.
	PublicID uint64 `gorm:"not null;uniqueIndex;default:nextval('member_public_id_seq')" json:"public_id"`
	// 登录相关
	Username     string `gorm:"size:50;not null;index" json:"username"` // 登录帐号（在所属租户/代理内唯一）
	LoginScope   string `gorm:"size:80;not null;default:platform;index" json:"login_scope"`
	Email        string `gorm:"size:100" json:"email,omitempty"`   // 邮箱（可选）
	Password     string `gorm:"size:255;not null" json:"-"`        // 密码（加密存储，不返回给前端）
	Nickname     string `gorm:"size:50" json:"nickname,omitempty"` // 昵称
	Phone        string `gorm:"size:30;index" json:"phone,omitempty"`
	Role         string `gorm:"size:20;not null;default:member;index" json:"role"`
	Remark       string `gorm:"size:500" json:"remark,omitempty"`
	RiskLevel    string `gorm:"size:20;not null;default:normal;index" json:"risk_level"`
	BalanceCents int64  `gorm:"not null;default:0" json:"-"`

	// FlyMode: inherit = 跟随房间默认飞单比例; custom = 使用 FlyRate; off = 不飞单
	FlyMode string  `gorm:"size:20;not null;default:inherit" json:"fly_mode"`
	FlyRate float64 `gorm:"not null;default:0" json:"fly_rate"` // 百分比，仅 custom 生效

	// RoomRebateRate is the room default owned by an agent. Members inherit it
	// unless their RebateMode explicitly overrides or disables the rebate.
	RoomRebateRate float64 `gorm:"not null;default:0" json:"room_rebate_rate"`
	// RoomProfitShareRate is the agent's share of positive room GGR. A losing
	// period never creates a negative share that would inflate platform profit.
	// It is copied onto every bet at placement time so historic statements do
	// not change when a room contract is edited later.
	RoomProfitShareRate float64 `gorm:"not null;default:0" json:"room_profit_share_rate"`
	// RebateMode: inherit = room default; custom = RebateRate; off = disabled.
	RebateMode string  `gorm:"size:20;not null;default:inherit" json:"rebate_mode"`
	RebateRate float64 `gorm:"not null;default:0" json:"rebate_rate"`

	// AgentRoomCode is the vanity room number owned by an agent (靓号=房间号).
	AgentRoomCode string `gorm:"size:40;uniqueIndex:idx_user_agent_room_code,where:agent_room_code <> '' AND deleted_at IS NULL" json:"agent_room_code,omitempty"`
	// AgentRoomName is the public room name configured by the owning agent.
	// It is intentionally independent from the agent's personal nickname.
	AgentRoomName string `gorm:"size:50" json:"agent_room_name,omitempty"`
	// AgentRoomLogo is the public room avatar. It is stored as a compact,
	// resized image data URL so local installations do not depend on an object
	// storage service just to display a room identity.
	AgentRoomLogo string `gorm:"type:text" json:"agent_room_logo,omitempty"`
	// ParentAgentID links a member to the agent room they entered.
	ParentAgentID *uint64 `gorm:"index" json:"parent_agent_id,omitempty"`
	// ParentTenantID is the immutable ownership boundary for an agent. Members
	// inherit tenant ownership through ParentAgentID and never choose it from a
	// browser request.
	ParentTenantID *uint64 `gorm:"index" json:"parent_tenant_id,omitempty"`

	Status int `gorm:"default:1" json:"status"` // 状态：0-禁用 1-启用
	// MutedUntil only affects public group chat. Service chat remains available
	// so a muted member can still contact support.
	MutedUntil  *time.Time `gorm:"index" json:"muted_until,omitempty"`
	MuteReason  string     `gorm:"size:300" json:"mute_reason,omitempty"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	LoginCount  int        `gorm:"not null;default:0" json:"login_count"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName 指定数据库表名
func (User) TableName() string {
	return "user"
}

// BalanceTransaction is an immutable audit record for every manual balance
// adjustment made from the admin user-management page.
type BalanceTransaction struct {
	ID     uint64 `gorm:"primaryKey" json:"id"`
	UserID uint64 `gorm:"not null;index" json:"user_id"`
	// Reference identifies the domain operation that produced the immutable
	// ledger row.  Legacy/manual rows may leave it empty; automated operations
	// use it for traceability and database-level idempotency.
	Reference   string    `gorm:"size:180;not null;default:'';index" json:"reference,omitempty"`
	AmountCents int64     `gorm:"not null" json:"-"`
	BeforeCents int64     `gorm:"not null" json:"-"`
	AfterCents  int64     `gorm:"not null" json:"-"`
	Type        string    `gorm:"size:30;not null;default:manual" json:"type"`
	Remark      string    `gorm:"size:300" json:"remark"`
	Operator    string    `gorm:"size:80" json:"operator"`
	CreatedAt   time.Time `gorm:"index" json:"created_at"`
}

func (BalanceTransaction) TableName() string { return "user_balance_transactions" }
