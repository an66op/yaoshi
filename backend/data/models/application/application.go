package application

import "time"

// Application records a user request that must be reviewed by an operator.
// Monetary values use integer cents so approval and balance settlement stay
// exact and auditable.
type Application struct {
	ID             uint64     `gorm:"primaryKey" json:"id"`
	UserID         uint64     `gorm:"not null;index" json:"user_id"`
	Username       string     `gorm:"size:50;not null;index" json:"username"`
	AccountType    string     `gorm:"size:20;not null" json:"account_type"`
	RequestType    string     `gorm:"size:20;not null;index" json:"request_type"`
	PaymentType    string     `gorm:"size:30;not null" json:"payment_type"`
	RequestedCents int64      `gorm:"not null;default:0" json:"-"`
	ReceivedCents  int64      `gorm:"not null;default:0" json:"-"`
	Remark         string     `gorm:"size:500" json:"remark"`
	Status         string     `gorm:"size:20;not null;default:pending;index" json:"status"`
	Operator       string     `gorm:"size:80" json:"operator"`
	ReviewRemark   string     `gorm:"size:500" json:"review_remark"`
	ReviewedAt     *time.Time `json:"reviewed_at"`
	CreatedAt      time.Time  `gorm:"index" json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (Application) TableName() string { return "user_applications" }
