package services

import (
	modeluser "backend/data/models/user"
	"backend/data/models/wallet"
	apperrors "backend/errors"
	"backend/utils"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MemberPaymentAccountService struct{ db *gorm.DB }

type MemberPaymentAccountView struct {
	ID          uint64 `json:"id"`
	AccountType string `json:"account_type"`
	Label       string `json:"label"`
	AccountName string `json:"account_name"`
	AccountNo   string `json:"account_no"`
	HolderName  string `json:"holder_name"`
	IsDefault   bool   `json:"is_default"`
}

type CreateMemberPaymentAccountInput struct {
	AccountType string `json:"account_type"`
	Label       string `json:"label"`
	AccountName string `json:"account_name"`
	AccountNo   string `json:"account_no"`
	HolderName  string `json:"holder_name"`
	IsDefault   bool   `json:"is_default"`
}

func NewMemberPaymentAccountService(db *gorm.DB) *MemberPaymentAccountService {
	return &MemberPaymentAccountService{db: db}
}

func (s *MemberPaymentAccountService) List(userID uint64) ([]MemberPaymentAccountView, error) {
	var rows []wallet.MemberPaymentAccount
	if err := s.db.Where("user_id = ?", userID).Order("is_default desc, id desc").Find(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("PAYMENT_ACCOUNT_READ_FAILED", "读取收款方式失败", err)
	}
	items := make([]MemberPaymentAccountView, 0, len(rows))
	for _, row := range rows {
		view, err := memberPaymentAccountView(row)
		if err != nil {
			return nil, apperrors.NewSystemError("PAYMENT_ACCOUNT_DECRYPT_FAILED", "读取收款方式失败", err)
		}
		items = append(items, view)
	}
	return items, nil
}

func (s *MemberPaymentAccountService) Create(userID uint64, input CreateMemberPaymentAccountInput) (*MemberPaymentAccountView, error) {
	accountType, label, accountName, accountNo, holderName, err := validateMemberPaymentAccount(input)
	if err != nil {
		return nil, err
	}
	encryptedAccountNo, err := utils.EncryptSensitive(accountNo)
	if err != nil {
		return nil, apperrors.NewSystemError("PAYMENT_ACCOUNT_ENCRYPT_FAILED", "保存收款方式失败", err)
	}
	var created wallet.MemberPaymentAccount
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Serialize all account-list mutations for one member.  Without this
		// lock two concurrent "first account" requests can both observe count=0
		// and leave two defaults behind.
		var owner modeluser.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("user_id").First(&owner, userID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
			}
			return err
		}
		var count int64
		if err := tx.Model(&wallet.MemberPaymentAccount{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
			return err
		}
		isDefault := input.IsDefault || count == 0
		if isDefault {
			if err := tx.Model(&wallet.MemberPaymentAccount{}).Where("user_id = ?", userID).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		created = wallet.MemberPaymentAccount{
			UserID: userID, AccountType: accountType, Label: label, AccountName: accountName,
			AccountNo: encryptedAccountNo, HolderName: holderName, IsDefault: isDefault,
		}
		return tx.Create(&created).Error
	})
	if err != nil {
		return nil, apperrors.NewSystemError("PAYMENT_ACCOUNT_CREATE_FAILED", "新增收款方式失败", err)
	}
	view, err := memberPaymentAccountView(created)
	if err != nil {
		return nil, apperrors.NewSystemError("PAYMENT_ACCOUNT_DECRYPT_FAILED", "读取收款方式失败", err)
	}
	return &view, nil
}

func (s *MemberPaymentAccountService) Delete(userID, id uint64) error {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var owner modeluser.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("user_id").First(&owner, userID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
			}
			return err
		}
		var row wallet.MemberPaymentAccount
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", id, userID).First(&row).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("PAYMENT_ACCOUNT_NOT_FOUND", "收款方式不存在")
			}
			return err
		}
		if err := tx.Delete(&row).Error; err != nil {
			return err
		}
		if !row.IsDefault {
			return nil
		}
		// Keep the product invariant "accounts exist => one default" after a
		// member removes the current default.
		var replacement wallet.MemberPaymentAccount
		if err := tx.Where("user_id = ?", userID).Order("id DESC").First(&replacement).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
		return tx.Model(&replacement).Update("is_default", true).Error
	})
	if err != nil {
		if _, ok := err.(*apperrors.AppError); ok {
			return err
		}
		return apperrors.NewSystemError("PAYMENT_ACCOUNT_DELETE_FAILED", "删除收款方式失败", err)
	}
	return nil
}

func (s *MemberPaymentAccountService) GetOwned(userID, id uint64) (*wallet.MemberPaymentAccount, error) {
	if id == 0 {
		return nil, apperrors.NewBusinessError("PAYMENT_ACCOUNT_REQUIRED", "请先选择收款方式")
	}
	var row wallet.MemberPaymentAccount
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("PAYMENT_ACCOUNT_NOT_FOUND", "收款方式不存在或不属于当前账号")
		}
		return nil, apperrors.NewSystemError("PAYMENT_ACCOUNT_READ_FAILED", "读取收款方式失败", err)
	}
	plainAccountNo, err := utils.DecryptSensitive(row.AccountNo)
	if err != nil {
		return nil, apperrors.NewSystemError("PAYMENT_ACCOUNT_DECRYPT_FAILED", "读取收款方式失败", err)
	}
	row.AccountNo = plainAccountNo
	return &row, nil
}

func validateMemberPaymentAccount(input CreateMemberPaymentAccountInput) (string, string, string, string, string, error) {
	accountType := strings.ToLower(strings.TrimSpace(input.AccountType))
	if _, ok := allowedCreditTypes[accountType]; !ok || accountType == "manual" {
		return "", "", "", "", "", apperrors.NewBusinessError("INVALID_PAYMENT_ACCOUNT", "请选择微信、支付宝、银行卡或 USDT")
	}
	accountName := strings.TrimSpace(input.AccountName)
	accountNo := strings.TrimSpace(input.AccountNo)
	holderName := strings.TrimSpace(input.HolderName)
	if accountName == "" || accountNo == "" {
		return "", "", "", "", "", apperrors.NewBusinessError("INVALID_PAYMENT_ACCOUNT", "请填写收款账号和账户名称")
	}
	if utf8.RuneCountInString(accountName) > 100 || utf8.RuneCountInString(accountNo) > 180 || utf8.RuneCountInString(holderName) > 80 {
		return "", "", "", "", "", apperrors.NewBusinessError("INVALID_PAYMENT_ACCOUNT", "收款方式信息过长")
	}
	label := strings.TrimSpace(input.Label)
	if label == "" {
		label = allowedCreditTypes[accountType]
	}
	if utf8.RuneCountInString(label) > 80 {
		return "", "", "", "", "", apperrors.NewBusinessError("INVALID_PAYMENT_ACCOUNT", "收款方式名称过长")
	}
	return accountType, label, accountName, accountNo, holderName, nil
}

func memberPaymentAccountView(row wallet.MemberPaymentAccount) (MemberPaymentAccountView, error) {
	accountNo, err := utils.DecryptSensitive(row.AccountNo)
	if err != nil {
		return MemberPaymentAccountView{}, err
	}
	return MemberPaymentAccountView{
		ID: row.ID, AccountType: row.AccountType, Label: row.Label, AccountName: row.AccountName,
		AccountNo: maskPaymentAccountNo(accountNo), HolderName: row.HolderName, IsDefault: row.IsDefault,
	}, nil
}

func maskPaymentAccountNo(value string) string {
	chars := []rune(strings.TrimSpace(value))
	if len(chars) <= 4 {
		return strings.Repeat("•", len(chars))
	}
	keep := 4
	return strings.Repeat("•", len(chars)-keep) + string(chars[len(chars)-keep:])
}
