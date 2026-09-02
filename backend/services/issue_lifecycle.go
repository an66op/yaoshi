package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EnsureCurrentIssue materializes and refreshes the lifecycle of the currently
// advertised period without changing any historic bet or draw rows.
func (s *BetAdminService) EnsureCurrentIssue(game *lottery.Game) (*lottery.Issue, error) {
	if game == nil {
		return nil, apperrors.NewBusinessError("GAME_NOT_FOUND", "游戏不存在")
	}
	issueNo, err := s.currentIssueForGame(game)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	mode := "platform"
	if game.SourceKind == "external" || game.SourceKind == "official" {
		mode = "external"
	}

	if issueNo == "" || game.NextDrawAt.IsZero() {
		// An absent upstream schedule is not permission to invent an issue or
		// start a new interval from the time this endpoint happens to be read.
		return &lottery.Issue{GameID: game.ID, SourceMode: mode, Status: lottery.IssueStatusAwaiting}, nil
	}
	var row lottery.Issue
	readErr := s.db.Where("game_id = ? AND issue = ?", game.ID, issueNo).First(&row).Error
	if readErr != nil && readErr != gorm.ErrRecordNotFound {
		return nil, apperrors.NewSystemError("ISSUE_READ_FAILED", "读取期号状态失败", readErr)
	}
	drawAt := game.NextDrawAt.UTC()
	if row.ScheduledDrawAt != nil && row.ScheduledDrawAt.Before(drawAt) {
		drawAt = row.ScheduledDrawAt.UTC()
	} else if readErr == nil && row.ScheduledDrawAt == nil &&
		(row.Status == lottery.IssueStatusSealed || row.Status == lottery.IssueStatusAwaiting) && row.SealAt.Before(drawAt) {
		// Upgrade safety: an old closed issue must stay closed. Its legacy seal
		// is a conservative upper bound, never a reason to grant more time.
		drawAt = row.SealAt.UTC()
	}
	rawSettings, platformID, err := readTimingSettings(s.db, 0)
	if err != nil {
		return nil, err
	}
	window, err := ensureIssueWindow(s.db, platformID, game, issueNo, drawAt, rawSettings)
	if err != nil {
		return nil, err
	}
	drawAt = window.ScheduledDrawAt
	if readErr == gorm.ErrRecordNotFound {
		row = lottery.Issue{GameID: game.ID, Issue: issueNo, Status: lottery.IssueStatusPending,
			SourceMode: mode, AcceptAt: window.AcceptAt, SealAt: window.SealAt, ScheduledDrawAt: &drawAt}
		if err := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return nil, apperrors.NewSystemError("ISSUE_SAVE_FAILED", "保存期号状态失败", err)
		}
		if err := s.db.Where("game_id = ? AND issue = ?", game.ID, issueNo).First(&row).Error; err != nil {
			return nil, err
		}
	}

	status := row.Status
	lastError := row.LastError
	var draw lottery.Draw
	drawQuery := trustedDrawsForGame(s.db, game.ID).Where("issue = ?", issueNo).Limit(1).Find(&draw)
	if drawQuery.Error != nil {
		return nil, apperrors.NewSystemError("DRAW_READ_FAILED", "读取开奖结果失败", drawQuery.Error)
	}
	_, _, orderedDrawContract := trustedDrawRevision(game.ID)
	clearUntrustedLifecycleDraw := drawQuery.RowsAffected == 0 && orderedDrawContract && (row.DrawAt != nil || row.SettledAt != nil)
	if drawQuery.RowsAffected > 0 {
		var pending int64
		if err := s.db.Model(&bet.Bet{}).Where("game_id = ? AND issue = ? AND status = ?", game.ID, issueNo, "pending").Count(&pending).Error; err != nil {
			return nil, apperrors.NewSystemError("BET_READ_FAILED", "读取待结算注单失败", err)
		}
		drawAt := draw.DrawAt.UTC()
		row.DrawAt = &drawAt
		if pending == 0 {
			status = lottery.IssueStatusSettled
			row.SettledAt = &drawAt
		} else {
			status = lottery.IssueStatusSettling
		}
		lastError = ""
	} else if row.Status == lottery.IssueStatusError && strings.HasPrefix(strings.TrimSpace(row.LastError), "对账异常：") {
		// A successful HTTP sync must not silently reopen a period which still
		// has no verifiable draw. Only importing that draw or an explicit repair
		// can clear a reconciliation error.
		status = lottery.IssueStatusError
		lastError = row.LastError
	} else if mode == "external" && (game.SyncStatus == "error" || (game.SyncStatus == "syncing" && strings.TrimSpace(game.LastSyncError) != "")) {
		status = lottery.IssueStatusError
		lastError = strings.TrimSpace(game.LastSyncError)
	} else {
		status = windowStatus(window, now)
		lastError = ""
	}

	updates := map[string]any{}
	if row.Status != status || row.LastError != lastError {
		updates["status"], updates["last_error"] = status, lastError
	}
	if row.ScheduledDrawAt == nil || !row.ScheduledDrawAt.Equal(drawAt) || !row.SealAt.Equal(window.SealAt) || !row.AcceptAt.Equal(window.AcceptAt) {
		updates["scheduled_draw_at"] = drawAt
		updates["seal_at"], updates["accept_at"] = window.SealAt, window.AcceptAt
	}
	if clearUntrustedLifecycleDraw {
		// An earlier single-source build may already have copied an unverified
		// draw into the current issue row.  Filtering lottery_draws alone is not
		// enough: sharedIssueOpen treats any DrawAt as terminal.  Clear only the
		// current lifecycle pointers after the ordered contract has no trusted
		// draw; the raw draw remains intact for administrative reconciliation.
		row.DrawAt = nil
		row.SettledAt = nil
		updates["draw_at"] = nil
		updates["settled_at"] = nil
	} else if row.DrawAt != nil {
		updates["draw_at"] = row.DrawAt
	}
	if !clearUntrustedLifecycleDraw && row.SettledAt != nil {
		updates["settled_at"] = row.SettledAt
	}
	if len(updates) > 0 {
		// A catalogue reader must not overwrite a settlement/source-error state
		// which changed after its snapshot. In that case return the winner's
		// durable row instead of reopening an issue from stale local state.
		updated := s.db.Model(&lottery.Issue{}).
			Where("id = ? AND status = ? AND updated_at = ?", row.ID, row.Status, row.UpdatedAt).Updates(updates)
		if updated.Error != nil {
			return nil, apperrors.NewSystemError("ISSUE_SAVE_FAILED", "更新期号状态失败", updated.Error)
		}
		if updated.RowsAffected == 0 {
			if err := s.db.First(&row, row.ID).Error; err != nil {
				return nil, err
			}
			return &row, nil
		}
	}
	row.Status = status
	row.LastError = lastError
	row.ScheduledDrawAt = &drawAt
	row.AcceptAt, row.SealAt = window.AcceptAt, window.SealAt
	return &row, nil
}

func (s *BetAdminService) setIssueStatus(gameID, issue, status, message string, drawAt, settledAt *time.Time) error {
	updates := map[string]any{"status": status, "last_error": limitDBText(message, 500)}
	if drawAt != nil {
		updates["draw_at"] = drawAt.UTC()
	}
	if settledAt != nil {
		updates["settled_at"] = settledAt.UTC()
	}
	return s.db.Model(&lottery.Issue{}).Where("game_id = ? AND issue = ?", gameID, issue).Updates(updates).Error
}

func issueAccepting(row *lottery.Issue) bool {
	return row != nil && row.Status == lottery.IssueStatusAccepting
}
