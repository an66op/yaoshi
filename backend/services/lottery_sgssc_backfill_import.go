package services

import (
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func sgSSCBackfillConflict(issue, message string) error {
	return apperrors.NewBusinessError("SG_HISTORY_CONFLICT", fmt.Sprintf("SG时时彩第%s期%s，禁止覆盖，需人工核对", issue, message))
}

// Called INSIDE the existing settlement financial transaction, before any
// ticket/account locks. A pause, source change or reclaimed generation that
// wins the Game lock prevents payment, including after a successful preflight.
func sgSSCBackfillSettlementGate(item lottery.SGSSCBackfillItem, now func() time.Time) func(*gorm.DB) error {
	return func(tx *gorm.DB) error {
		var game lottery.Game
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&game, "id = ?", "sg-ssc").Error; err != nil {
			return err
		}
		if !game.Enabled || !sgSSCSourceBound(&game) {
			return errSGSSCBackfillPaused
		}
		if err := lockSGSSCBackfillClaim(tx, item, now()); err != nil {
			return err
		}
		return sgSSCIssueEvidenceError(tx, item.Issue, nil)
	}
}

func validateSGSSCStoredHistory(draw lottery.Draw, issue string) error {
	_, _, expected, err := parseSGSSCIssue(issue)
	numbers := parseNumbers(draw.Numbers)
	if err != nil || draw.GameID != "sg-ssc" || draw.Issue != issue || !draw.DrawAt.Equal(expected) ||
		draw.SourceRevision != sgSSCSourceRevision || draw.ConversionRevision != sgSSCConversionRevision ||
		len(numbers) != 5 || !storedDrawNumbersEqual(draw.Numbers, numbers) {
		return sgSSCBackfillConflict(issue, "已存历史的期号、时间或版本无效")
	}
	for _, number := range numbers {
		if number < 0 || number > 9 {
			return sgSSCBackfillConflict(issue, "已存历史五球越界")
		}
	}
	return nil
}

// Shared game-before-queue lock ordering with discovery/normal draw import.
// Only a missing Draw is inserted. No realtime Game fields, Issue windows,
// historical bets, archived snapshots or existing numbers are rewritten.
// With verified=nil this is a preflight: use an already trusted draw (even
// older than the fetch horizon) without making another upstream request.
func (s *LotteryService) prepareSGSSCBackfill(ctx context.Context, item lottery.SGSSCBackfillItem, verified *sourceDraw, now time.Time) (bool, error) {
	ready := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var game lottery.Game
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&game, "id = ?", "sg-ssc").Error; err != nil {
			return err
		}
		if !game.Enabled || !sgSSCSourceBound(&game) {
			return errSGSSCBackfillPaused
		}
		if err := lockSGSSCBackfillClaim(tx, item, now); err != nil {
			return err
		}
		_, _, expected, err := parseSGSSCIssue(item.Issue)
		if err != nil || !expected.Equal(item.DrawAt) || !expected.Before(now) {
			return sgSSCBackfillConflict(item.Issue, "补采期号或时间无效")
		}
		if err := sgSSCIssueEvidenceError(tx, item.Issue, nil); err != nil {
			return err
		}
		var stored lottery.Draw
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("game_id = ? AND issue = ?", "sg-ssc", item.Issue).Limit(1).Find(&stored).Error; err != nil {
			return err
		}
		imported := false
		if stored.ID != 0 {
			if err := validateSGSSCStoredHistory(stored, item.Issue); err != nil {
				return err
			}
			if verified != nil && (!stored.DrawAt.Equal(verified.DrawAt) || !storedDrawNumbersEqual(stored.Numbers, verified.Numbers)) {
				return sgSSCBackfillConflict(item.Issue, "双站结果与已存可信历史冲突")
			}
		} else if verified != nil {
			if err := validateSGSSCVerifiedHistoryBatch([]sourceDraw{*verified}, []string{item.Issue}, now); err != nil {
				return err
			}
			stored = lottery.Draw{GameID: "sg-ssc", Issue: item.Issue, Numbers: joinNumbers(verified.Numbers), DrawAt: verified.DrawAt.UTC(), SourceRevision: sgSSCSourceRevision, ConversionRevision: sgSSCConversionRevision}
			if err := tx.Create(&stored).Error; err != nil {
				return err
			}
			imported = true
		} else {
			return nil
		}
		updates := map[string]any{"numbers": stored.Numbers}
		if imported {
			updates["imported"] = true
		}
		receipt := tx.Model(&lottery.SGSSCBackfillAttempt{}).Where("issue = ? AND attempt = ? AND status = 'running'", item.Issue, item.Attempts).Updates(updates)
		if receipt.Error != nil {
			return receipt.Error
		}
		if receipt.RowsAffected != 1 {
			return errSGSSCBackfillLeaseLost
		}
		ready = true
		return nil
	})
	return ready, err
}
