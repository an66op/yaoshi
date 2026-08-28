package services

import (
	"backend/data/models/bet"
	"backend/data/models/user"
	apperrors "backend/errors"
	"backend/ws"
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	reconciliationRefundLedgerType = "reconciliation_refund"
	// The existing database constraint reserves "resolved" for abnormalities
	// closed by an operator. The reconciliation note and immutable ledger type
	// preserve the more specific refunded outcome.
	reconciliationRefundedStatus = "resolved"
)

// ReconciliationBetView preserves the existing bet JSON while exposing the
// exact integer stake used by reconciliation. Refundable is deliberately
// derived by the backend and must not be inferred by an administration UI.
type ReconciliationBetView struct {
	bet.Bet
	AmountCents int64 `json:"amount_cents"`
	Refundable  bool  `json:"refundable"`
}

type ReconciliationRefundResult struct {
	BetID                uint64 `json:"bet_id"`
	WorkspaceID          uint64 `json:"workspace_id"`
	UserID               uint64 `json:"user_id"`
	AmountCents          int64  `json:"amount_cents"`
	BeforeCents          int64  `json:"before_cents"`
	AfterCents           int64  `json:"after_cents"`
	LedgerReference      string `json:"ledger_reference"`
	BetStatus            string `json:"bet_status"`
	ReconciliationStatus string `json:"reconciliation_status"`
	AlreadyRefunded      bool   `json:"already_refunded"`
}

func toReconciliationBetView(row bet.Bet) ReconciliationBetView {
	return ReconciliationBetView{
		Bet: row, AmountCents: row.AmountCents,
		Refundable: row.Status == "pending" && row.ReconciliationStatus == "abnormal",
	}
}

func reconciliationRefundReference(betID uint64) string {
	return fmt.Sprintf("reconciliation_refund:%d", betID)
}

// RefundAbnormalPendingBet closes one otherwise-unsettleable ticket without
// deleting it. The immutable ledger reference makes a repeated administrator
// request return the original outcome instead of crediting the member twice.
func (s *SystemAuditService) RefundAbnormalPendingBet(ctx context.Context, betID uint64, operator string) (*ReconciliationRefundResult, error) {
	if betID == 0 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "注单编号不正确")
	}
	operator = limitDBText(defaultString(strings.TrimSpace(operator), "平台管理员"), 80)
	var lastErr error
	for attempt := 0; attempt < settlementRetryAttempts; attempt++ {
		result, err := s.refundAbnormalPendingBetOnce(ctx, betID, operator)
		if err == nil {
			// An idempotent retry returns the original immutable ledger before/after
			// values. Do not broadcast that historical after-value: the member may
			// have completed newer balance operations since the first refund.
			if shouldBroadcastReconciliationRefund(result) {
				ws.NotifyUser(result.UserID, "balance", map[string]any{
					"workspace_id": result.WorkspaceID,
					"balance":      centsToAmount(result.AfterCents),
				})
			}
			return result, nil
		}
		lastErr = err
		if !isRetryableTransactionError(err) || attempt == settlementRetryAttempts-1 {
			return nil, err
		}
		delay := time.Duration(attempt+1) * settlementRetryBaseDelay
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, lastErr
}

func shouldBroadcastReconciliationRefund(result *ReconciliationRefundResult) bool {
	return result != nil && !result.AlreadyRefunded
}

func (s *SystemAuditService) refundAbnormalPendingBetOnce(ctx context.Context, betID uint64, operator string) (*ReconciliationRefundResult, error) {
	reference := reconciliationRefundReference(betID)
	result := &ReconciliationRefundResult{BetID: betID, LedgerReference: reference}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row bet.Bet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, betID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("NOT_FOUND", "注单不存在")
			}
			return err
		}
		result.WorkspaceID = row.WorkspaceID
		result.UserID = row.UserID
		result.AmountCents = row.AmountCents

		// The member row is locked even for an idempotent retry. This makes the
		// returned original ledger outcome stable while another balance mutation
		// is waiting and keeps lock ordering identical to settlement.
		var account user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, row.UserID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("USER_NOT_FOUND", "注单所属用户不存在")
			}
			return err
		}

		var existing user.BalanceTransaction
		ledgerErr := tx.Where("user_id = ? AND reference = ?", row.UserID, reference).First(&existing).Error
		if ledgerErr == nil {
			if err := validateExistingReconciliationRefund(row, existing); err != nil {
				return err
			}
			result.BeforeCents = existing.BeforeCents
			result.AfterCents = existing.AfterCents
			result.BetStatus = row.Status
			result.ReconciliationStatus = row.ReconciliationStatus
			result.AlreadyRefunded = true
			return nil
		}
		if ledgerErr != gorm.ErrRecordNotFound {
			return ledgerErr
		}
		if err := validateRefundableAbnormalBet(row); err != nil {
			return err
		}

		before := account.BalanceCents
		after := before + row.AmountCents
		if after < before {
			return apperrors.NewBusinessError("INVALID_REQUEST", "退款金额超出余额可表示范围")
		}
		if err := tx.Model(&account).Update("balance_cents", after).Error; err != nil {
			return err
		}
		if err := tx.Create(&user.BalanceTransaction{
			WorkspaceID: row.WorkspaceID,
			UserID:      row.UserID,
			Reference:   reference,
			AmountCents: row.AmountCents,
			BeforeCents: before,
			AfterCents:  after,
			Type:        reconciliationRefundLedgerType,
			Remark:      limitDBText(fmt.Sprintf("异常未结注单 #%d 人工退款关闭（%s/%s）", row.ID, row.GameID, row.Issue), 300),
			Operator:    operator,
		}).Error; err != nil {
			return err
		}

		note := limitDBText(fmt.Sprintf("已人工退款关闭；退款流水 %s", reference), 500)
		remark := limitDBText(strings.Trim(strings.TrimSpace(row.Remark)+" | 异常未结注单已人工退款关闭", " |"), 300)
		updated := tx.Model(&bet.Bet{}).
			Where("id = ? AND status = ? AND reconciliation_status = ?", row.ID, "pending", "abnormal").
			Updates(map[string]any{
				"status":                "cancelled",
				"operator":              operator,
				"remark":                remark,
				"reconciliation_status": reconciliationRefundedStatus,
				"reconciliation_note":   note,
				"updated_at":            time.Now().UTC(),
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return apperrors.NewBusinessError("BET_STATUS_CHANGED", "注单状态已变化，请刷新后重试")
		}

		result.BeforeCents = before
		result.AfterCents = after
		result.BetStatus = "cancelled"
		result.ReconciliationStatus = reconciliationRefundedStatus
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func validateRefundableAbnormalBet(row bet.Bet) error {
	if row.Status != "pending" || row.ReconciliationStatus != "abnormal" {
		return apperrors.NewBusinessError("INVALID_REQUEST", "仅异常且待结算的注单可以人工退款关闭")
	}
	if row.AmountCents <= 0 {
		return apperrors.NewBusinessError("INVALID_REQUEST", "注单金额异常，不能自动退款")
	}
	return nil
}

func validateExistingReconciliationRefund(row bet.Bet, ledger user.BalanceTransaction) error {
	if ledger.Reference != reconciliationRefundReference(row.ID) ||
		ledger.Type != reconciliationRefundLedgerType ||
		ledger.UserID != row.UserID ||
		ledger.WorkspaceID != row.WorkspaceID ||
		ledger.AmountCents != row.AmountCents ||
		ledger.AfterCents != ledger.BeforeCents+ledger.AmountCents ||
		row.Status != "cancelled" || row.ReconciliationStatus != reconciliationRefundedStatus {
		return apperrors.NewBusinessError("REFUND_STATE_CONFLICT", "退款流水与注单状态不一致，请人工核对")
	}
	return nil
}
