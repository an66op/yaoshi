package wallet

import "time"

// PaymentChannel is an admin-managed deposit / withdraw channel.
// CreditType aligns with application.payment_type (alipay / wechat / bank / ...).
type PaymentChannel struct {
	ID         uint64    `gorm:"primaryKey" json:"id"`
	Provider   string    `gorm:"size:80;not null" json:"provider"`
	Name       string    `gorm:"size:80;not null" json:"name"`
	MerchantNo string    `gorm:"size:120" json:"merchant_no"`
	CreditType string    `gorm:"size:30;not null;index" json:"credit_type"`
	FeeRate    float64   `gorm:"not null;default:0" json:"fee_rate"`
	MinAmount  float64   `gorm:"not null;default:0" json:"min_amount"`
	MaxAmount  float64   `gorm:"not null;default:0" json:"max_amount"`
	Status     string    `gorm:"size:20;not null;default:enabled;index" json:"status"`
	Remark     string    `gorm:"size:500" json:"remark"`
	SortOrder  int       `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (PaymentChannel) TableName() string { return "wallet_payment_channels" }
