package services

import (
	"backend/cluster"
	"backend/data/models/bet"
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
)

type sgSSCHistoryFetcher func(context.Context, []string) (SGSSCHistoryVerification, error)

type sgSSCBackfillRun struct{ Claimed, Recovered, Deferred int }

// Independent of the realtime polling lease: downtime debt must not delay a
// current-period sync or make a historical success reopen live betting.
func StartSGSSCBackfill(ctx context.Context, db *gorm.DB) {
	go func() {
		timer := time.NewTimer(20 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				_, err := cluster.RunWithLease(ctx, "scheduler:sgssc-history-backfill", sgSSCBackfillLease, func(leaseCtx context.Context) error {
					workCtx, cancel := context.WithTimeout(leaseCtx, 90*time.Second)
					defer cancel()
					result, err := NewLotteryService(db).runSGSSCBackfill(workCtx, time.Now, fetchSGSSCVerifiedHistory)
					if result.Claimed > 0 {
						log.Printf("SG历史补采: 处理%d期，恢复%d期，保留%d期待核对/重试", result.Claimed, result.Recovered, result.Deferred)
					}
					return err
				})
				if err != nil && ctx.Err() == nil {
					log.Printf("SG历史补采: %v", err)
				}
				timer.Reset(time.Minute)
			}
		}
	}()
}

func validateSGSSCHistoryCoverage(result SGSSCHistoryVerification, targets []string, now time.Time) error {
	if err := validateSGSSCVerifiedHistoryBatch(result.Draws, targets, now); err != nil {
		return err
	}
	remaining := make(map[string]bool, len(targets))
	for _, issue := range targets {
		remaining[issue] = true
	}
	for _, draw := range result.Draws {
		delete(remaining, draw.Issue)
	}
	for _, failure := range result.Failures {
		if !remaining[failure.Issue] || failure.Error == "" {
			return fmt.Errorf("SG历史核对失败记录重复、超出范围或缺少原因")
		}
		delete(remaining, failure.Issue)
	}
	if len(remaining) > 0 {
		return fmt.Errorf("SG历史核对未覆盖全部请求期号")
	}
	return nil
}

func (s *LotteryService) runSGSSCBackfill(ctx context.Context, now func() time.Time, fetch sgSSCHistoryFetcher) (sgSSCBackfillRun, error) {
	result := sgSSCBackfillRun{}
	if ctx == nil || now == nil || fetch == nil {
		return result, fmt.Errorf("SG历史补采依赖不可用")
	}
	if _, err := s.discoverSGSSCBackfill(ctx, now(), "系统SG历史补采", "", "auto", false); err != nil {
		return result, err
	}
	items, err := s.claimSGSSCBackfills(ctx, now())
	if err != nil {
		return result, err
	}
	result.Claimed = len(items)
	var journalErrors []error
	finish := func(item lottery.SGSSCBackfillItem, state, outcome, message string, settled int64) {
		// On cancellation leave a durable running receipt for lease recovery.
		// Do not detach live DB work from shutdown just to paint it complete.
		if err := s.finishSGSSCBackfill(ctx, item, now(), state, outcome, message, settled); err != nil {
			journalErrors = append(journalErrors, err)
			result.Deferred++
		} else if state == "completed" {
			result.Recovered++
		} else {
			result.Deferred++
		}
	}
	failed := func(item lottery.SGSSCBackfillItem, err error) {
		state, outcome := "retry", "source_error"
		if code := apperrors.GetErrorCode(err); code == "DRAW_SOURCE_UNVERIFIED" || code == "SG_HISTORY_CONFLICT" {
			state, outcome = "blocked", "conflict"
		}
		finish(item, state, outcome, err.Error(), 0)
	}
	settle := func(item lottery.SGSSCBackfillItem) {
		// Recheck the claim/enabled binding immediately before settling. The
		// existing financial receipt transaction is idempotent with ordinary
		// settlement recovery; never re-price or re-debit an accepted ticket.
		if _, err := s.prepareSGSSCBackfill(ctx, item, nil, now()); err != nil {
			failed(item, err)
			return
		}
		settleCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		settled, err := NewBetAdminService(s.db.WithContext(settleCtx)).settleIssueGuarded("sg-ssc", item.Issue, item.RequestedBy, sgSSCBackfillSettlementGate(item, now))
		if err != nil {
			finish(item, "settlement_retry", "settlement_error", err.Error(), 0)
			return
		}
		var pending int64
		if err := s.db.WithContext(ctx).Model(&bet.Bet{}).Where("game_id = ? AND issue = ? AND status = ?", "sg-ssc", item.Issue, "pending").Count(&pending).Error; err != nil {
			finish(item, "settlement_retry", "settlement_error", err.Error(), 0)
			return
		}
		if pending > 0 {
			finish(item, "settlement_retry", "settlement_error", "该期仍有待结注单，保留恢复任务", 0)
			return
		}
		finish(item, "completed", "recovered", "", settled.Won+settled.Lost+settled.Push)
	}
	toFetch := make([]lottery.SGSSCBackfillItem, 0, len(items))
	for _, item := range items {
		ready, err := s.prepareSGSSCBackfill(ctx, item, nil, now())
		if err != nil {
			failed(item, err)
			continue
		}
		if ready {
			settle(item)
			continue
		}
		if item.DrawAt.Before(now().Add(-sgSSCBackfillMaxAge)) {
			finish(item, "blocked", "blocked", "缺期已超过30天自动核对范围；未获取可信历史，不结算，需人工核查上游记录", 0)
			continue
		}
		toFetch = append(toFetch, item)
	}
	if len(toFetch) > 0 {
		targets := sortedSGSSCHistoryTargets(toFetch)
		verified, fetchErr := fetch(ctx, targets)
		if fetchErr == nil {
			fetchErr = ctx.Err()
		}
		if fetchErr == nil {
			fetchErr = validateSGSSCHistoryCoverage(verified, targets, now())
		}
		if fetchErr != nil {
			for _, item := range toFetch {
				failed(item, fetchErr)
			}
		} else {
			draws, failures := make(map[string]sourceDraw), make(map[string]SGSSCHistoryFailure)
			for _, draw := range verified.Draws {
				draws[draw.Issue] = draw
			}
			for _, failure := range verified.Failures {
				failures[failure.Issue] = failure
			}
			for _, item := range toFetch {
				if failure, missing := failures[item.Issue]; missing {
					if failure.Permanent {
						finish(item, "blocked", "blocked", failure.Error+"；已超出163母源当前可证明的有限历史窗口，需人工核查", 0)
					} else {
						finish(item, "retry", "source_error", failure.Error, 0)
					}
					continue
				}
				draw := draws[item.Issue]
				if _, err := s.prepareSGSSCBackfill(ctx, item, &draw, now()); err != nil {
					failed(item, err)
					continue
				}
				settle(item)
			}
		}
	}
	return result, errors.Join(journalErrors...)
}
