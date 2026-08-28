package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"strings"
	"time"

	"gorm.io/gorm/clause"
)

// EnsureCurrentIssue materializes and refreshes the lifecycle of the currently
// advertised period without changing any historic bet or draw rows.
func (s *BetAdminService) EnsureCurrentIssue(game *lottery.Game) (*lottery.Issue, error) {
	if game == nil {
		return nil, apperrors.NewBusinessError("GAME_NOT_FOUND", "游戏不存在")
	}
	issueNo, err := s.CurrentIssue(game.ID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	interval := time.Duration(maxInt(game.DrawInterval, 60)) * time.Second
	sealAt := game.NextDrawAt.UTC().Add(-3 * time.Second)
	if game.NextDrawAt.IsZero() {
		sealAt = now.Add(interval - 3*time.Second)
	}
	acceptAt := sealAt.Add(-interval + 3*time.Second)
	mode := "platform"
	if game.SourceKind == "external" || game.SourceKind == "official" {
		mode = "external"
	}

	row := lottery.Issue{
		GameID: game.ID, Issue: issueNo, Status: lottery.IssueStatusPending,
		SourceMode: mode, AcceptAt: acceptAt, SealAt: sealAt,
	}
	if err := s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "game_id"}, {Name: "issue"}},
		DoUpdates: clause.Assignments(map[string]any{
			"source_mode": mode, "accept_at": acceptAt, "seal_at": sealAt,
		}),
	}).Create(&row).Error; err != nil {
		return nil, apperrors.NewSystemError("ISSUE_SAVE_FAILED", "保存期号状态失败", err)
	}
	if err := s.db.Where("game_id = ? AND issue = ?", game.ID, issueNo).First(&row).Error; err != nil {
		return nil, apperrors.NewSystemError("ISSUE_READ_FAILED", "读取期号状态失败", err)
	}

	status := row.Status
	lastError := row.LastError
	var draw lottery.Draw
	drawQuery := s.db.Where("game_id = ? AND issue = ?", game.ID, issueNo).Limit(1).Find(&draw)
	if drawQuery.Error != nil {
		return nil, apperrors.NewSystemError("DRAW_READ_FAILED", "读取开奖结果失败", drawQuery.Error)
	}
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
	} else if !now.Before(sealAt) {
		status = lottery.IssueStatusAwaiting
		lastError = ""
	} else if now.Before(acceptAt) {
		status = lottery.IssueStatusPending
		lastError = ""
	} else {
		status = lottery.IssueStatusAccepting
		lastError = ""
	}

	updates := map[string]any{"status": status, "last_error": lastError}
	if row.DrawAt != nil {
		updates["draw_at"] = row.DrawAt
	}
	if row.SettledAt != nil {
		updates["settled_at"] = row.SettledAt
	}
	if err := s.db.Model(&lottery.Issue{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
		return nil, apperrors.NewSystemError("ISSUE_SAVE_FAILED", "更新期号状态失败", err)
	}
	row.Status = status
	row.LastError = lastError
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
