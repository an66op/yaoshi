package services

import (
	"backend/data/models/user"
	"errors"

	"gorm.io/gorm"
)

// ensureSeededBalance repairs the ledger baseline for system-created accounts
// and records every automatic top-up. The caller must hold a row lock for the
// account and run inside the same transaction as any balance update.
func ensureSeededBalance(tx *gorm.DB, account *user.User, minimumCents, targetCents int64, label string) error {
	var last user.BalanceTransaction
	err := tx.Where("user_id = ?", account.UserID).Order("id desc").First(&last).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		if account.BalanceCents != 0 {
			if err := tx.Create(&user.BalanceTransaction{
				UserID: account.UserID, AmountCents: account.BalanceCents,
				BeforeCents: 0, AfterCents: account.BalanceCents,
				Type: "opening_balance", Remark: label + "期初余额", Operator: "系统初始化",
			}).Error; err != nil {
				return err
			}
		}
	case err != nil:
		return err
	case last.AfterCents != account.BalanceCents:
		// Older versions seeded these accounts without a ledger. Preserve the
		// real current balance while explicitly recording the historic repair.
		if err := tx.Create(&user.BalanceTransaction{
			UserID: account.UserID, AmountCents: account.BalanceCents - last.AfterCents,
			BeforeCents: last.AfterCents, AfterCents: account.BalanceCents,
			Type: "seed_reconciliation", Remark: label + "历史余额补录", Operator: "系统迁移",
		}).Error; err != nil {
			return err
		}
	}

	if account.BalanceCents >= minimumCents {
		return nil
	}
	if targetCents < minimumCents || targetCents < account.BalanceCents {
		return errors.New("invalid seeded balance target")
	}
	before := account.BalanceCents
	if err := tx.Model(account).Update("balance_cents", targetCents).Error; err != nil {
		return err
	}
	if err := tx.Create(&user.BalanceTransaction{
		UserID: account.UserID, AmountCents: targetCents - before,
		BeforeCents: before, AfterCents: targetCents,
		Type: "system_topup", Remark: label + "自动补充", Operator: "系统初始化",
	}).Error; err != nil {
		return err
	}
	account.BalanceCents = targetCents
	return nil
}
