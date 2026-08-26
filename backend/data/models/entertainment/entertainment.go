package entertainment

import "time"

// Platform stores third-party entertainment merchant configuration.
// Real vendor SDKs are not wired; this keeps enablement / health editable.
type Platform struct {
	ID         uint64    `gorm:"primaryKey" json:"id"`
	Code       string    `gorm:"size:40;not null;uniqueIndex" json:"code"` // kaiyuan / pg / ag / im
	Name       string    `gorm:"size:80;not null" json:"name"`
	Category   string    `gorm:"size:40;not null" json:"category"`
	MerchantNo string    `gorm:"size:120" json:"merchant_no"`
	APIBase    string    `gorm:"size:320" json:"api_base"`
	LaunchPath string    `gorm:"size:320" json:"launch_path"`
	SecretKey  string    `gorm:"type:text" json:"-"`
	Status     string    `gorm:"size:20;not null;default:disabled;index" json:"status"` // enabled / maintenance / disabled
	Remark     string    `gorm:"size:500" json:"remark"`
	SortOrder  int       `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (Platform) TableName() string { return "entertainment_platforms" }
