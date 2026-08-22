package special

import "time"

// NumberResource is a vanity room number kept in the pool.
// When granted to an agent it becomes that agent's public room code.
type NumberResource struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	Number      string    `gorm:"size:40;not null;uniqueIndex" json:"number"`
	Level       string    `gorm:"size:20;not null;default:normal" json:"level"` // normal / rare / epic
	Status      string    `gorm:"size:20;not null;default:available;index" json:"status"` // available / reserved / granted
	OwnerUserID *uint64   `gorm:"index" json:"owner_user_id"`
	PriceCents  int64     `gorm:"not null;default:0" json:"-"`
	Remark      string    `gorm:"size:300" json:"remark"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (NumberResource) TableName() string { return "special_number_resources" }

// Campaign is a room-number giveaway / sales campaign.
type Campaign struct {
	ID           uint64     `gorm:"primaryKey" json:"id"`
	Title        string     `gorm:"size:120;not null" json:"title"`
	Status       string     `gorm:"size:20;not null;default:draft;index" json:"status"`
	RuleText     string     `gorm:"size:500" json:"rule_text"`
	GrantedCount int64      `gorm:"not null;default:0" json:"granted_count"`
	StartsAt     *time.Time `json:"starts_at"`
	EndsAt       *time.Time `json:"ends_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (Campaign) TableName() string { return "special_number_campaigns" }

// GrantRecord records which agent received which room number.
type GrantRecord struct {
	ID         uint64    `gorm:"primaryKey" json:"id"`
	CampaignID uint64    `gorm:"not null;index" json:"campaign_id"`
	ResourceID uint64    `gorm:"not null;index" json:"resource_id"`
	Number     string    `gorm:"size:40;not null" json:"number"`
	UserID     uint64    `gorm:"not null;index" json:"user_id"`
	Username   string    `gorm:"size:50;not null" json:"username"`
	Operator   string    `gorm:"size:80" json:"operator"`
	CreatedAt  time.Time `json:"created_at"`
}

func (GrantRecord) TableName() string { return "special_number_grants" }
