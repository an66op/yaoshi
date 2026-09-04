package services

import (
	"backend/data/models/lottery"
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const sgSSCBackfillLease = 3 * time.Minute

var errSGSSCBackfillLeaseLost = errors.New("SG历史补采执行权已过期，等待下一次恢复")
var errSGSSCBackfillPaused = errors.New("SG时时彩已关闭或来源绑定已改变，暂停补采")

// Claim generations are checked again after upstream I/O. A process that
// resumes after its lease expired cannot import or close a newer receipt.
func (s *LotteryService) claimSGSSCBackfills(ctx context.Context, now time.Time) ([]lottery.SGSSCBackfillItem, error) {
	claimed := make([]lottery.SGSSCBackfillItem, 0, sgSSCBackfillMaxIssues)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var game lottery.Game
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&game, "id = ?", "sg-ssc").Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		var abandoned []lottery.SGSSCBackfillItem
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = 'running' AND (lease_until IS NULL OR lease_until <= ?)", now).
			Order("draw_at ASC").Limit(sgSSCDiscoveryLimit).Find(&abandoned).Error; err != nil {
			return err
		}
		for _, item := range abandoned {
			if err := finishSGSSCBackfillTx(tx, item, now, "retry", "interrupted", "上次执行中断或超时，保留已导入记录并等待重试", 0); err != nil {
				return err
			}
		}
		if !game.Enabled || !sgSSCSourceBound(&game) {
			return nil
		}
		var candidates []lottery.SGSSCBackfillItem
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ? AND next_retry_at <= ?", []string{"pending", "retry", "settlement_retry"}, now).
			Order(clause.Expr{SQL: `CASE WHEN EXISTS (SELECT 1 FROM lottery_bets b WHERE b.game_id = 'sg-ssc' AND b.issue = lottery_sgssc_backfill_items.issue AND b.status = 'pending' AND b.draw_source_revision = ?) THEN 0 ELSE 1 END`, Vars: []any{sgSSCSourceRevision}, WithoutParentheses: true}).
			Order("draw_at ASC, issue ASC").Limit(sgSSCDiscoveryLimit).Find(&candidates).Error; err != nil {
			return err
		}
		dates := make(map[string]bool)
		for _, item := range candidates {
			if len(claimed) == sgSSCBackfillMaxIssues {
				break
			}
			date := item.Issue[:8] // Database CHECK guarantees eleven digits.
			if !dates[date] && len(dates) == sgSSCBackfillMaxDates {
				continue
			}
			dates[date] = true
			until := now.Add(sgSSCBackfillLease)
			item.Status, item.Attempts, item.LeaseUntil, item.UpdatedAt = "running", item.Attempts+1, &until, now
			if err := tx.Model(&item).Updates(map[string]any{"status": item.Status, "attempts": item.Attempts, "lease_until": until, "updated_at": now}).Error; err != nil {
				return err
			}
			attempt := lottery.SGSSCBackfillAttempt{Issue: item.Issue, Attempt: item.Attempts, Status: "running", Trigger: item.RequestTrigger,
				Operator: item.RequestedBy, RequestID: item.RequestID, StartedAt: now,
				SourceRevision: sgSSCSourceRevision, ConversionRevision: sgSSCConversionRevision}
			if err := tx.Create(&attempt).Error; err != nil {
				return err
			}
			claimed = append(claimed, item)
		}
		return nil
	})
	return claimed, err
}

func lockSGSSCBackfillClaim(tx *gorm.DB, claim lottery.SGSSCBackfillItem, now time.Time) error {
	var current lottery.SGSSCBackfillItem
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "issue = ?", claim.Issue).Error; err != nil {
		return err
	}
	if current.Status != "running" || current.Attempts != claim.Attempts || current.LeaseUntil == nil || !current.LeaseUntil.After(now) {
		return errSGSSCBackfillLeaseLost
	}
	return nil
}

// Both journal completion and queue movement commit together. The database
// also forbids changes to finished receipts, so manual retry creates evidence
// rather than replacing the earlier failure with an optimistic success.
func finishSGSSCBackfillTx(tx *gorm.DB, item lottery.SGSSCBackfillItem, now time.Time, state, outcome, message string, settled int64) error {
	message = limitDBText(message, 500)
	journal := tx.Model(&lottery.SGSSCBackfillAttempt{}).Where("issue = ? AND attempt = ? AND status = 'running'", item.Issue, item.Attempts).
		Updates(map[string]any{"status": outcome, "finished_at": now, "error": message, "settled_bets": settled})
	if journal.Error != nil {
		return journal.Error
	}
	if journal.RowsAffected != 1 {
		return fmt.Errorf("SG历史补采第%s期执行回执不存在或已结束", item.Issue)
	}
	updates := map[string]any{"status": state, "lease_until": nil, "last_error": message, "updated_at": now, "next_retry_at": sgSSCBackfillRetryAt(now, item.Attempts)}
	if state == "completed" {
		updates["completed_at"] = now
	}
	row := tx.Model(&lottery.SGSSCBackfillItem{}).Where("issue = ? AND attempts = ? AND status = 'running'", item.Issue, item.Attempts).Updates(updates)
	if row.Error != nil {
		return row.Error
	}
	if row.RowsAffected != 1 {
		return errSGSSCBackfillLeaseLost
	}
	return nil
}

func (s *LotteryService) finishSGSSCBackfill(ctx context.Context, item lottery.SGSSCBackfillItem, now time.Time, state, outcome, message string, settled int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSGSSCBackfillClaim(tx, item, now); err != nil {
			return err
		}
		return finishSGSSCBackfillTx(tx, item, now, state, outcome, message, settled)
	})
}
