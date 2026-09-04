package services

import (
	"errors"
	"strings"
	"time"

	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/data/models/settings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const source163HongKongGameID = "hong-kong-mark-six"

// repairable163HongKongIssue is deliberately pure so every fail-closed
// predicate can be regression tested without a database. The caller still
// repeats the material predicates in its final CAS update.
func repairable163HongKongIssue(row lottery.Issue, authoritative sourceSchedule, now time.Time, betCount, drawCount int64) bool {
	return row.GameID == source163HongKongGameID && strings.TrimSpace(row.Issue) != "" && row.Issue == strings.TrimSpace(authoritative.Issue) &&
		row.Status == lottery.IssueStatusError && strings.HasPrefix(strings.TrimSpace(row.LastError), "对账异常：") &&
		row.ScheduledDrawAt != nil && !row.ScheduledDrawAt.IsZero() && !now.Before(row.ScheduledDrawAt.UTC()) &&
		authoritative.DrawAt.After(row.ScheduledDrawAt.UTC()) && authoritative.DrawAt.After(now) &&
		row.DrawAt == nil && row.SettledAt == nil && betCount == 0 && drawCount == 0
}

// repair163HongKongIssueLifecycle is the sole exception to the normal
// never-extend-window rule. It is allowed only after the ID18 adapter has
// validated the authoritative next issue and future draw boundary. Holding the
// game and issue locks prevents placement from racing the zero-bet proof.
func repair163HongKongIssueLifecycle(tx *gorm.DB, game *lottery.Game, binding source163MarkSixBinding, authoritative sourceSchedule, now time.Time) (bool, error) {
	if tx == nil || game == nil || binding.GameID != source163HongKongGameID || binding.UpstreamGameID != 18 ||
		game.ID != binding.GameID || strings.TrimSpace(game.NextIssue) != strings.TrimSpace(authoritative.Issue) ||
		game.NextDrawAt.IsZero() || !game.NextDrawAt.UTC().Equal(authoritative.DrawAt.UTC()) || now.IsZero() {
		return false, nil
	}
	now = now.UTC()
	authoritative.DrawAt = authoritative.DrawAt.UTC()

	var row lottery.Issue
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("game_id = ? AND issue = ?", binding.GameID, authoritative.Issue).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var betCount, drawCount int64
	if err := tx.Model(&bet.Bet{}).Where("game_id = ? AND issue = ?", binding.GameID, authoritative.Issue).Count(&betCount).Error; err != nil {
		return false, err
	}
	// This is intentionally the raw draw table, not trustedDrawsForGame: even
	// an unversioned or conflicting historical result makes reopening unsafe.
	if err := tx.Model(&lottery.Draw{}).Where("game_id = ? AND issue = ?", binding.GameID, authoritative.Issue).Count(&drawCount).Error; err != nil {
		return false, err
	}
	if !repairable163HongKongIssue(row, authoritative, now, betCount, drawCount) {
		return false, nil
	}

	var windows []lottery.IssueWindow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("game_id = ? AND issue = ?", binding.GameID, authoritative.Issue).Order("workspace_id ASC").Find(&windows).Error; err != nil {
		return false, err
	}
	workspaceIDs := make([]uint64, 0, len(windows))
	for _, window := range windows {
		// A future or independently rebased room window is evidence that the
		// lifecycle no longer matches the stale snapshot being repaired.
		if window.ScheduledDrawAt.IsZero() || now.Before(window.ScheduledDrawAt.UTC()) {
			return false, nil
		}
		workspaceIDs = append(workspaceIDs, window.WorkspaceID)
	}
	configs := make(map[uint64]string, len(workspaceIDs))
	if len(workspaceIDs) > 0 {
		var rows []settings.SystemConfig
		if err := tx.Select("workspace_id", "game_settings_json").Where("workspace_id IN ?", workspaceIDs).Find(&rows).Error; err != nil {
			return false, err
		}
		for _, config := range rows {
			configs[config.WorkspaceID] = config.GameSettingsJSON
		}
	}

	// Rebase every already materialized workspace window with its own current
	// seal configuration. A failed CAS rolls the entire transaction back.
	for _, stored := range windows {
		candidate := newIssueWindow(stored.WorkspaceID, game, authoritative.Issue, authoritative.DrawAt, configuredSealSeconds(configs[stored.WorkspaceID], binding.GameID))
		updated := tx.Model(&lottery.IssueWindow{}).
			Where("id = ? AND game_id = ? AND issue = ? AND workspace_id = ? AND scheduled_draw_at = ? AND updated_at = ?",
				stored.ID, binding.GameID, authoritative.Issue, stored.WorkspaceID, stored.ScheduledDrawAt, stored.UpdatedAt).
			Updates(map[string]any{
				"accept_at": candidate.AcceptAt, "seal_at": candidate.SealAt, "scheduled_draw_at": candidate.ScheduledDrawAt,
				"draw_interval": candidate.DrawInterval, "seal_seconds": candidate.SealSeconds,
			})
		if updated.Error != nil {
			return false, updated.Error
		}
		if updated.RowsAffected != 1 {
			return false, errors.New("香港六合彩工作区封盘窗口在安全修复前发生变化")
		}
	}

	platformRaw, _, err := readTimingSettings(tx, 0)
	if err != nil {
		return false, err
	}
	platformWindow := newIssueWindow(0, game, authoritative.Issue, authoritative.DrawAt, configuredSealSeconds(platformRaw, binding.GameID))
	status := windowStatus(&platformWindow, now)
	oldDrawAt := row.ScheduledDrawAt.UTC()
	updated := tx.Model(&lottery.Issue{}).
		Where(`id = ? AND game_id = ? AND issue = ? AND status = ? AND last_error = ? AND scheduled_draw_at = ? AND updated_at = ?
			AND draw_at IS NULL AND settled_at IS NULL
			AND NOT EXISTS (SELECT 1 FROM lottery_bets AS repair_bet WHERE repair_bet.game_id = ? AND repair_bet.issue = ?)
			AND NOT EXISTS (SELECT 1 FROM lottery_draws AS repair_draw WHERE repair_draw.game_id = ? AND repair_draw.issue = ?)`,
			row.ID, binding.GameID, authoritative.Issue, lottery.IssueStatusError, row.LastError, oldDrawAt, row.UpdatedAt,
			binding.GameID, authoritative.Issue, binding.GameID, authoritative.Issue).
		Updates(map[string]any{
			"status": status, "source_mode": "external", "last_error": "",
			"accept_at": platformWindow.AcceptAt, "seal_at": platformWindow.SealAt, "scheduled_draw_at": authoritative.DrawAt,
		})
	if updated.Error != nil {
		return false, updated.Error
	}
	if updated.RowsAffected != 1 {
		return false, errors.New("香港六合彩期号在安全修复前发生变化")
	}
	return true, nil
}
