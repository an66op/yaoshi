package services

import (
	"backend/cluster"
	"backend/data/models/bet"
	"backend/data/models/user"
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	idempotencyRecoveryInterval = time.Minute
	idempotencyRecoveryBatch    = 200
)

// IdempotencyRecoveryResult records how abandoned reservations were closed.
// A request is completed only when an immutable debit ledger proves that it
// was charged. Missing evidence becomes a terminal failure so no reservation
// can remain in "processing" forever. Inconsistent evidence also fails closed
// and is surfaced in Errors for operator investigation.
type IdempotencyRecoveryResult struct {
	Scanned   int
	Completed int
	Failed    int
	Errors    int
}

func (s *BetAdminService) RecoverStaleIdempotencyRequests(ctx context.Context, limit int) (IdempotencyRecoveryResult, error) {
	result := IdempotencyRecoveryResult{}
	if limit <= 0 {
		limit = idempotencyRecoveryBatch
	}
	if limit > 1000 {
		limit = 1000
	}
	cutoff := time.Now().UTC().Add(-idempotencyReservationTimeout)

	var directIDs []uint64
	if err := s.db.WithContext(ctx).Model(&bet.BetRequest{}).
		Where("status = ? AND (updated_at <= ? OR (updated_at IS NULL AND (created_at <= ? OR created_at IS NULL)))", "processing", cutoff, cutoff).
		Order("COALESCE(updated_at, created_at) ASC NULLS FIRST, id ASC").Limit(limit).Pluck("id", &directIDs).Error; err != nil {
		return result, err
	}
	for _, id := range directIDs {
		completed, failed, inconsistent, recoverErr := s.recoverDirectReservation(ctx, id)
		if shouldQuarantineIdempotencyRecovery(ctx, recoverErr) {
			quarantined, quarantineErr := s.quarantineIdempotencyReservation(ctx, &bet.BetRequest{}, id)
			if quarantineErr != nil {
				recoverErr = fmt.Errorf("recover direct request: %v; quarantine: %w", recoverErr, quarantineErr)
			} else if quarantined {
				log.Printf("下注幂等恢复异常已隔离 kind=direct id=%d: %v", id, recoverErr)
				completed, failed, inconsistent, recoverErr = false, true, true, nil
			}
		}
		accumulateIdempotencyRecovery(&result, id, "direct", completed, failed, inconsistent, recoverErr)
	}

	var assistantIDs []uint64
	if err := s.db.WithContext(ctx).Model(&bet.AssistantRequest{}).
		Where("status = ? AND (updated_at <= ? OR (updated_at IS NULL AND (created_at <= ? OR created_at IS NULL)))", "processing", cutoff, cutoff).
		Order("COALESCE(updated_at, created_at) ASC NULLS FIRST, id ASC").Limit(limit).Pluck("id", &assistantIDs).Error; err != nil {
		return result, err
	}
	for _, id := range assistantIDs {
		completed, failed, inconsistent, recoverErr := s.recoverAssistantReservation(ctx, id)
		if shouldQuarantineIdempotencyRecovery(ctx, recoverErr) {
			quarantined, quarantineErr := s.quarantineIdempotencyReservation(ctx, &bet.AssistantRequest{}, id)
			if quarantineErr != nil {
				recoverErr = fmt.Errorf("recover assistant request: %v; quarantine: %w", recoverErr, quarantineErr)
			} else if quarantined {
				log.Printf("下注幂等恢复异常已隔离 kind=assistant id=%d: %v", id, recoverErr)
				completed, failed, inconsistent, recoverErr = false, true, true, nil
			}
		}
		accumulateIdempotencyRecovery(&result, id, "assistant", completed, failed, inconsistent, recoverErr)
	}
	return result, nil
}

func shouldQuarantineIdempotencyRecovery(ctx context.Context, err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return ctx == nil || ctx.Err() == nil
}

func (s *BetAdminService) quarantineIdempotencyReservation(ctx context.Context, model any, id uint64) (bool, error) {
	updated := s.db.WithContext(ctx).Model(model).Where("id = ? AND status = ?", id, "processing").
		Updates(map[string]any{
			"status":      "failed",
			"last_error":  "自动恢复异常，需人工核对",
			"result_json": "",
		})
	if updated.Error != nil {
		return false, updated.Error
	}
	return updated.RowsAffected == 1, nil
}

func accumulateIdempotencyRecovery(result *IdempotencyRecoveryResult, id uint64, kind string, completed, failed, inconsistent bool, err error) {
	if result == nil {
		return
	}
	result.Scanned++
	if err != nil {
		result.Errors++
		log.Printf("隔离下注幂等恢复异常 kind=%s id=%d: %v", kind, id, err)
		return
	}
	if completed {
		result.Completed++
	}
	if failed {
		result.Failed++
	}
	if inconsistent {
		result.Errors++
	}
}

func (s *BetAdminService) recoverDirectReservation(ctx context.Context, id uint64) (completed, failed, inconsistent bool, resultErr error) {
	resultErr = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row bet.BetRequest
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).First(&row, id).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
		if row.Status != "processing" || !idempotencyReservationExpired(row.UpdatedAt, time.Now().UTC()) {
			return nil
		}

		var ledger user.BalanceTransaction
		err := tx.Where("user_id = ? AND reference = ?", row.UserID, directBetRequestReference(row.ID)).First(&ledger).Error
		if err == gorm.ErrRecordNotFound {
			failed = true
			return finishDirectReservation(tx, row.ID, "failed", "请求处理超时且未发生扣分，请重新提交", "")
		}
		if err != nil {
			return err
		}
		if err := validateIdempotencyRequestLedger(ledger, row.UserID, row.WorkspaceID, directBetRequestReference(row.ID)); err != nil {
			failed, inconsistent = true, true
			return finishDirectReservation(tx, row.ID, "failed", "请求账务校验异常，请联系管理员", "")
		}
		var bets []bet.Bet
		if err := tx.Where("workspace_id = ? AND user_id = ? AND request_reference = ?", row.WorkspaceID, row.UserID, directBetRequestReference(row.ID)).Find(&bets).Error; err != nil {
			return err
		}
		if err := validateIdempotencyBetEvidence(ledger, bets, row.UserID, row.WorkspaceID, directBetRequestReference(row.ID), true); err != nil {
			failed, inconsistent = true, true
			return finishDirectReservation(tx, row.ID, "failed", "请求注单证据校验异常，请联系管理员", "")
		}
		_, payload, err := recoveredDirectBetView(ledger)
		if err != nil {
			return err
		}
		completed = true
		return finishDirectReservation(tx, row.ID, "completed", "", string(payload))
	})
	return
}

func (s *BetAdminService) recoverAssistantReservation(ctx context.Context, id uint64) (completed, failed, inconsistent bool, resultErr error) {
	resultErr = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row bet.AssistantRequest
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).First(&row, id).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
		if row.Status != "processing" || !idempotencyReservationExpired(row.UpdatedAt, time.Now().UTC()) {
			return nil
		}

		var ledger user.BalanceTransaction
		err := tx.Where("user_id = ? AND reference = ?", row.UserID, assistantBetRequestReference(row.ID)).First(&ledger).Error
		if err == gorm.ErrRecordNotFound {
			failed = true
			return finishAssistantReservation(tx, row.ID, "failed", "请求处理超时且未发生扣分，请重新提交", "")
		}
		if err != nil {
			return err
		}
		if err := validateIdempotencyRequestLedger(ledger, row.UserID, row.WorkspaceID, assistantBetRequestReference(row.ID)); err != nil {
			failed, inconsistent = true, true
			return finishAssistantReservation(tx, row.ID, "failed", "请求账务校验异常，请联系管理员", "")
		}
		var bets []bet.Bet
		if err := tx.Where("workspace_id = ? AND user_id = ? AND request_reference = ?", row.WorkspaceID, row.UserID, assistantBetRequestReference(row.ID)).Find(&bets).Error; err != nil {
			return err
		}
		if err := validateIdempotencyBetEvidence(ledger, bets, row.UserID, row.WorkspaceID, assistantBetRequestReference(row.ID), false); err != nil {
			failed, inconsistent = true, true
			return finishAssistantReservation(tx, row.ID, "failed", "请求注单证据校验异常，请联系管理员", "")
		}
		_, payload, err := recoveredAssistantResult(ledger)
		if err != nil {
			return err
		}
		completed = true
		return finishAssistantReservation(tx, row.ID, "completed", "", string(payload))
	})
	return
}

func validateIdempotencyBetEvidence(ledger user.BalanceTransaction, rows []bet.Bet, userID, workspaceID uint64, reference string, requireSingle bool) error {
	if err := validateIdempotencyRequestLedger(ledger, userID, workspaceID, reference); err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("idempotency request has no matching bets")
	}
	if requireSingle && len(rows) != 1 {
		return fmt.Errorf("direct idempotency request has %d matching bets", len(rows))
	}
	var total int64
	gameID, issue := strings.TrimSpace(rows[0].GameID), strings.TrimSpace(rows[0].Issue)
	if gameID == "" || issue == "" {
		return fmt.Errorf("idempotency bet game or issue is empty")
	}
	for _, row := range rows {
		if row.WorkspaceID != workspaceID || row.UserID != userID || strings.TrimSpace(row.RequestReference) != strings.TrimSpace(reference) {
			return fmt.Errorf("idempotency bet scope does not match request")
		}
		if strings.TrimSpace(row.GameID) != gameID || strings.TrimSpace(row.Issue) != issue || row.AmountCents <= 0 {
			return fmt.Errorf("idempotency bet issue or amount is inconsistent")
		}
		if row.AmountCents > math.MaxInt64-total {
			return fmt.Errorf("idempotency bet amount overflow")
		}
		total += row.AmountCents
	}
	if total != -ledger.AmountCents {
		return fmt.Errorf("idempotency bet total does not match debit")
	}
	return nil
}

func finishDirectReservation(tx *gorm.DB, id uint64, status, lastError, resultJSON string) error {
	updated := tx.Model(&bet.BetRequest{}).Where("id = ? AND status = ?", id, "processing").
		Updates(map[string]any{"status": status, "last_error": lastError, "result_json": resultJSON})
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected != 1 {
		return fmt.Errorf("投注幂等请求恢复状态发生冲突")
	}
	return nil
}

func finishAssistantReservation(tx *gorm.DB, id uint64, status, lastError, resultJSON string) error {
	updated := tx.Model(&bet.AssistantRequest{}).Where("id = ? AND status = ?", id, "processing").
		Updates(map[string]any{"status": status, "last_error": lastError, "result_json": resultJSON})
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected != 1 {
		return fmt.Errorf("开奖助手幂等请求恢复状态发生冲突")
	}
	return nil
}

// StartIdempotencyRecovery runs shortly after startup and once per minute.
// Redis provides the cross-instance lease in release deployments; a local
// development instance without Redis executes the same bounded database job.
func StartIdempotencyRecovery(ctx context.Context, db *gorm.DB) {
	go func() {
		run := func() {
			_, err := cluster.RunWithLease(ctx, "scheduler:idempotency-recovery", 3*time.Minute, func() error {
				result, recoverErr := NewBetAdminService(db).RecoverStaleIdempotencyRequests(ctx, idempotencyRecoveryBatch)
				if recoverErr != nil {
					return recoverErr
				}
				if result.Scanned > 0 {
					message := fmt.Sprintf("下注幂等请求自动恢复完成: scanned=%d completed=%d failed=%d", result.Scanned, result.Completed, result.Failed)
					if result.Errors > 0 {
						message += fmt.Sprintf(" inconsistent=%d", result.Errors)
					}
					log.Print(strings.TrimSpace(message))
				}
				return nil
			})
			if err != nil && ctx.Err() == nil {
				log.Printf("下注幂等恢复调度跳过或执行失败: %v", err)
			}
		}

		initial := time.NewTimer(10 * time.Second)
		defer initial.Stop()
		select {
		case <-ctx.Done():
			return
		case <-initial.C:
			run()
		}

		ticker := time.NewTicker(idempotencyRecoveryInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}
