package user

import (
	"crypto/rand"
	"encoding/binary"
	"gorm.io/gorm"
	"time"
)

// User 用户表模型
type User struct {
	UserID uint64 `gorm:"primaryKey" json:"user_id"`
	// PublicID is the stable member number shown in the product. It must not
	// change when a member changes their nickname. PostgreSQL assigns a random
	// seven-digit value and serializes allocation before the unique insert.
	PublicID uint64 `gorm:"not null;uniqueIndex;default:public.next_member_public_id()" json:"public_id"`
	// 登录相关
	Username   string `gorm:"size:50;not null;index" json:"username"` // 登录帐号（在所属租户/代理内唯一）
	LoginScope string `gorm:"size:80;not null;default:platform;index" json:"login_scope"`
	// WorkspaceID is the account's current home workspace. Historical business
	// rows keep their own workspace snapshot and are never rewritten when a
	// member later joins another room.
	WorkspaceID uint64 `gorm:"not null;default:0;index" json:"workspace_id"`
	Email       string `gorm:"size:100" json:"email,omitempty"` // 邮箱（可选）
	Password    string `gorm:"size:255;not null" json:"-"`      // 密码（加密存储，不返回给前端）
	// AuthVersion is embedded in every JWT. Changing a password increments it,
	// which revokes every token issued with the previous credential state.
	AuthVersion uint64 `gorm:"not null;default:1" json:"-"`
	Nickname    string `gorm:"size:50" json:"nickname,omitempty"` // 昵称
	// Avatar, PublicTitle and PublicBadge are member-facing profile metadata.
	// They are intentionally independent from Remark (an operator-only note)
	// and are resolved through the message workspace before being exposed in a
	// room timeline.
	Avatar      string `gorm:"type:text;not null;default:''" json:"avatar,omitempty"`
	PublicTitle string `gorm:"size:80;not null;default:''" json:"public_title,omitempty"`
	PublicBadge string `gorm:"size:40;not null;default:''" json:"badge,omitempty"`
	Phone       string `gorm:"size:30;index" json:"phone,omitempty"`
	Role        string `gorm:"size:20;not null;default:member;index" json:"role"`
	Remark      string `gorm:"size:500" json:"remark,omitempty"`
	// RobotGameIDsJSON is only used by persisted room activity accounts. An
	// empty array means that the account follows every enabled lottery; keeping
	// the selection on the account makes operator changes survive restarts.
	RobotGameIDsJSON string `gorm:"type:text;not null;default:'[]'" json:"-"`
	RobotActiveStart string `gorm:"size:5;not null;default:''" json:"-"`
	RobotActiveEnd   string `gorm:"size:5;not null;default:''" json:"-"`
	RobotMinBetCents int64  `gorm:"not null;default:0" json:"-"`
	RobotMaxBetCents int64  `gorm:"not null;default:0" json:"-"`
	RiskLevel        string `gorm:"size:20;not null;default:normal;index" json:"risk_level"`
	BalanceCents     int64  `gorm:"not null;default:0" json:"-"`

	// FlyMode: inherit = 跟随房间默认飞单比例; custom = 使用 FlyRate; off = 不飞单
	FlyMode string  `gorm:"size:20;not null;default:inherit" json:"fly_mode"`
	FlyRate float64 `gorm:"not null;default:0" json:"fly_rate"` // 百分比，仅 custom 生效
	// External-follow fields are non-secret preparation metadata for a future
	// connector. They never imply that an outside platform is connected and
	// are deliberately kept on the existing member trading record instead of
	// creating a second, conflicting fly-order policy model.
	FlyTargetPlatform   string `gorm:"size:80;not null;default:''" json:"-"`
	FlyTargetAccount    string `gorm:"size:120;not null;default:''" json:"-"`
	FlyEndpointLabel    string `gorm:"size:160;not null;default:''" json:"-"`
	FlySingleLimitCents int64  `gorm:"not null;default:0" json:"-"`
	FlyDailyLimitCents  int64  `gorm:"not null;default:0" json:"-"`
	FlyConnectionRemark string `gorm:"size:500;not null;default:''" json:"-"`

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
	// GroupChatEnabled controls the public lobby chat owned by this agent.
	// Rooms start muted and stay muted until an operator explicitly opens them.
	GroupChatEnabled bool `gorm:"not null;default:false" json:"group_chat_enabled"`
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

// BeforeCreate gives every new account a non-repeating credential generation.
// A development database reset restarts numeric user IDs; keeping the old
// default auth_version=1 would therefore allow a JWT issued before the reset
// to authenticate as a newly-created account that reused the same ID.  The
// value stays below JavaScript's safe-integer limit in case an operator needs
// to inspect a decoded token in browser tooling.
func (u *User) BeforeCreate(_ *gorm.DB) error {
	if u.AuthVersion > 1 {
		return nil
	}
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return err
	}
	u.AuthVersion = binary.BigEndian.Uint64(raw[:]) & ((1 << 52) - 1)
	if u.AuthVersion < 2 {
		u.AuthVersion += 2
	}
	return nil
}

// BalanceTransaction is an immutable audit record for every manual balance
// adjustment made from the admin user-management page.
type BalanceTransaction struct {
	ID          uint64 `gorm:"primaryKey" json:"id"`
	WorkspaceID uint64 `gorm:"not null;default:0;index" json:"workspace_id"`
	UserID      uint64 `gorm:"not null;index" json:"user_id"`
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
