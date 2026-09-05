package wallet

import (
	"time"

	"gorm.io/gorm"
)

// PaymentChannel is an admin-managed deposit / withdraw channel.
// CreditType aligns with application.payment_type (alipay / wechat / bank / ...).
type PaymentChannel struct {
	ID              uint64         `gorm:"primaryKey" json:"id"`
	WorkspaceID     uint64         `gorm:"not null;default:0;index" json:"workspace_id"`
	Provider        string         `gorm:"size:80;not null" json:"provider"`
	Name            string         `gorm:"size:80;not null" json:"name"`
	MerchantNo      string         `gorm:"size:120" json:"merchant_no"`
	CreditType      string         `gorm:"size:30;not null;index" json:"credit_type"`
	FeeRate         float64        `gorm:"not null;default:0" json:"fee_rate"`
	MinAmount       float64        `gorm:"not null;default:0" json:"min_amount"`
	MaxAmount       float64        `gorm:"not null;default:0" json:"max_amount"`
	Status          string         `gorm:"size:20;not null;default:enabled;index" json:"status"`
	Remark          string         `gorm:"size:500" json:"remark"`
	SortOrder       int            `gorm:"not null;default:0" json:"sort_order"`
	Mode            string         `gorm:"size:20;not null;default:manual;index" json:"mode"`
	APIBase         string         `gorm:"size:500" json:"api_base"`
	CreateOrderPath string         `gorm:"size:300" json:"create_order_path"`
	QueryOrderPath  string         `gorm:"size:300" json:"query_order_path"`
	CallbackPath    string         `gorm:"size:300" json:"callback_path"`
	SecretKey       string         `gorm:"type:text" json:"-"`
	TimeoutSeconds  int            `gorm:"not null;default:10" json:"timeout_seconds"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

func (PaymentChannel) TableName() string { return "wallet_payment_channels" }

// MemberPaymentAccount is a member-owned receiving account used for debit
// applications. The account number is intentionally kept separate from the
// platform-managed deposit channels above.
type MemberPaymentAccount struct {
	ID          uint64         `gorm:"primaryKey" json:"id"`
	WorkspaceID uint64         `gorm:"not null;default:0;index" json:"workspace_id"`
	UserID      uint64         `gorm:"not null;index" json:"user_id"`
	AccountType string         `gorm:"size:30;not null;index" json:"account_type"`
	Label       string         `gorm:"size:80;not null" json:"label"`
	AccountName string         `gorm:"size:100;not null" json:"account_name"`
	AccountNo   string         `gorm:"type:text;not null" json:"-"`
	HolderName  string         `gorm:"size:80" json:"holder_name"`
	QRCodeFile  *string        `gorm:"size:64" json:"-"`
	IsDefault   bool           `gorm:"not null;default:false" json:"is_default"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (MemberPaymentAccount) TableName() string { return "member_payment_accounts" }
