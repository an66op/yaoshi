package services

import (
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The cutover only changes this game's source binding. Historical draws,
// tickets, room settings, enabled state and odds are never rewritten.
func EnsureSGSSCVerifiedSource(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var game lottery.Game
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&game, "id = ?", "sg-ssc").Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
		wasBound := sgSSCSourceBound(&game)
		updates := sgSSCSourceBindingUpdates(game)
		if len(updates) == 0 {
			return nil
		}
		if err := tx.Model(&game).Updates(updates).Error; err != nil {
			return err
		}
		if wasBound {
			return nil
		}
		// Queue rows predate the v2 mother-source contract and do not carry an
		// immutable revision of their own. Explicitly block every unfinished
		// legacy item during the same cutover transaction; never reinterpret it
		// as authorization to fetch or stamp v2 evidence.
		now := time.Now().UTC()
		message := "来源已切换为163:64母源；旧版补采任务已封存，未按新版本重新解释"
		if err := tx.Model(&lottery.SGSSCBackfillAttempt{}).Where("status = ?", "running").Updates(map[string]any{
			"status": "blocked", "finished_at": now, "error": message,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&lottery.SGSSCBackfillItem{}).Where("status <> ?", "completed").Updates(map[string]any{
			"status": "blocked", "last_error": message, "lease_until": nil, "updated_at": now,
		}).Error
	})
}

func sgSSCSourceBound(game *lottery.Game) bool {
	return game != nil && game.ID == "sg-ssc" && game.SourceKind == "external" &&
		game.SourceName == sgSSCVerifiedSourceName && game.SourceURL == sgSSCVerifiedSourceURL
}

func sgSSCSourceBindingUpdates(game lottery.Game) map[string]any {
	if !sgSSCSourceBound(&game) {
		currentIdentity := game.ID == "sg-ssc" && game.SourceName == sgSSCVerifiedSourceName && game.SourceURL == sgSSCVerifiedSourceURL
		legacyExternal := game.ID == "sg-ssc" && game.SourceKind == "external" && game.SourceName == sgSSCLegacySourceName && game.SourceURL == sgSSCLegacySourceURL
		legacyPlatform := game.ID == "sg-ssc" && game.SourceKind == "platform" && (game.SourceName == "" || game.SourceName == "王者开奖") && game.SourceURL == ""
		if !currentIdentity && !legacyExternal && !legacyPlatform {
			return nil
		}
		return map[string]any{
			"source_kind": "external", "source_name": sgSSCVerifiedSourceName, "source_url": sgSSCVerifiedSourceURL,
			"sync_status": "stale", "last_sync_error": sgSSCPendingMessage, "last_sync_at": nil,
			"next_issue": "", "next_draw_at": time.Time{}, "timing_source": "pending", "draw_interval": 300,
		}
	}
	if game.SyncStatus == "idle" || game.SyncStatus == "syncing" {
		message := game.LastSyncError
		if message == "" {
			message = sgSSCPendingMessage
		}
		return map[string]any{"sync_status": "stale", "last_sync_error": message}
	}
	return nil
}

func sgSSCSourceHealthyAt(game *lottery.Game, now time.Time) bool {
	if !sgSSCSourceBound(game) || (game.SyncStatus != "ok" && game.SyncStatus != "syncing") ||
		game.LastSyncError != "" || game.LastSyncAt == nil || game.TimingSource != "upstream" || game.DrawInterval != 300 {
		return false
	}
	// A stopped worker must close betting even when its last poll succeeded.
	age := now.Sub(*game.LastSyncAt)
	_, _, nextAt, err := parseSGSSCIssue(game.NextIssue)
	return err == nil && game.NextDrawAt.Equal(nextAt) && age >= 0 && age <= time.Minute &&
		!now.Before(nextAt.Add(-sgSSCInterval)) && now.Before(nextAt)
}

func validateSGSSCImportRevision(draws []sourceDraw) error {
	return validateSGSSCVerifiedBatch(draws)
}

// An ordinary source outage must not become a permanent reconciliation marker
// before this already-recorded period has even drawn. Betting stays closed
// until a verified sync succeeds, and that sync still enforces each room's
// original seal time. Never relax an existing reconciliation marker or defer
// an already overdue or non-external lifecycle; source/legacy gates still apply.
func sgSSCSourceFailureCanWait(candidate settlementCandidate, now time.Time) bool {
	if candidate.GameID != "sg-ssc" || candidate.SourceKind != "external" || candidate.IssueSourceMode != "external" ||
		candidate.IssueStatus != lottery.IssueStatusError || candidate.IssueScheduledDrawAt == nil || now.IsZero() {
		return false
	}
	message := strings.TrimSpace(candidate.IssueLastError)
	if message == "" || strings.HasPrefix(message, "对账异常：") {
		return false
	}
	_, _, contractedAt, err := parseSGSSCIssue(candidate.Issue)
	return err == nil && candidate.IssueScheduledDrawAt.Equal(contractedAt) && now.Before(*candidate.IssueScheduledDrawAt)
}

func sgSSCUnverifiedIssue(issue string) error {
	return apperrors.NewBusinessError("DRAW_SOURCE_UNVERIFIED", fmt.Sprintf("SG时时彩第%s期存在旧来源记录，保留待核对，不受理或自动结算", issue))
}

// This is the current-import collision detector. Any lifecycle, draw or ticket
// carrying another contract prevents the new writer from claiming that issue.
// Historical settlement uses sgSSCIssueEvidenceError below, which accepts an
// old trusted draw only when every immutable ticket snapshot matches it.
func sgSSCLegacyIssues(db *gorm.DB, issues []string) (map[string]bool, error) {
	blocked := make(map[string]bool)
	var rows []string
	if err := db.Model(&lottery.Issue{}).Where("game_id = ? AND issue IN ? AND source_mode <> ?", "sg-ssc", issues, "external").Pluck("issue", &rows).Error; err != nil {
		return nil, err
	}
	for _, issue := range rows {
		blocked[issue] = true
	}
	rows = nil
	if err := db.Model(&lottery.Draw{}).Where("game_id = ? AND issue IN ? AND (source_revision <> ? OR conversion_revision <> ?)", "sg-ssc", issues, sgSSCSourceRevision, sgSSCConversionRevision).Pluck("issue", &rows).Error; err != nil {
		return nil, err
	}
	for _, issue := range rows {
		blocked[issue] = true
	}
	for _, table := range []string{"lottery_bets", "lottery_bet_archives"} {
		rows = nil
		if err := db.Table(table).Distinct("issue").Where("game_id = ? AND issue IN ? AND draw_source_revision <> ?", "sg-ssc", issues, sgSSCSourceRevision).Pluck("issue", &rows).Error; err != nil {
			return nil, err
		}
		for _, issue := range rows {
			blocked[issue] = true
		}
	}
	return blocked, nil
}

func sgSSCIssueEvidenceError(db *gorm.DB, issue string, row *lottery.Issue) error {
	if row == nil || row.Issue == "" {
		var stored lottery.Issue
		if err := db.Where("game_id = ? AND issue = ?", "sg-ssc", issue).Limit(1).Find(&stored).Error; err != nil {
			return err
		}
		row = &stored
	}
	if row != nil && row.Issue != "" && row.SourceMode != "external" {
		return sgSSCUnverifiedIssue(issue)
	}
	var draw lottery.Draw
	if err := db.Where("game_id = ? AND issue = ?", "sg-ssc", issue).Limit(1).Find(&draw).Error; err != nil {
		return err
	}
	expectedRevision := sgSSCSourceRevision
	if draw.ID != 0 {
		if !trustedDrawRevisionMatches("sg-ssc", draw.SourceRevision, draw.ConversionRevision) {
			return sgSSCUnverifiedIssue(issue)
		}
		expectedRevision = draw.SourceRevision
	}
	for _, table := range []string{"lottery_bets", "lottery_bet_archives"} {
		var mismatched int64
		if err := db.Table(table).Where("game_id = ? AND issue = ? AND draw_source_revision <> ?", "sg-ssc", issue, expectedRevision).Count(&mismatched).Error; err != nil {
			return err
		}
		if mismatched > 0 {
			return sgSSCUnverifiedIssue(issue)
		}
	}
	return nil
}

// Caller holds the Game UPDATE lock and transaction, as do all placement
// writers. Never promote an unversioned platform draw, even when balls match.
func insertVerifiedSGSSCDraws(db *gorm.DB, draws []sourceDraw) (int, error) {
	if err := validateSGSSCImportRevision(draws); err != nil {
		return 0, err
	}
	issues := make([]string, len(draws))
	for index, draw := range draws {
		issues[index] = draw.Issue
	}
	blocked, err := sgSSCLegacyIssues(db, issues)
	if err != nil {
		return 0, err
	}
	var existing []lottery.Draw
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("game_id = ? AND issue IN ?", "sg-ssc", issues).Find(&existing).Error; err != nil {
		return 0, err
	}
	byIssue := make(map[string]lottery.Draw, len(existing))
	for _, draw := range existing {
		byIssue[draw.Issue] = draw
	}
	var pending []lottery.Draw
	for index, draw := range draws {
		stored, found := byIssue[draw.Issue]
		if found && (stored.SourceRevision != sgSSCSourceRevision || stored.ConversionRevision != sgSSCConversionRevision) {
			blocked[draw.Issue] = true
		}
		if blocked[draw.Issue] {
			if index == len(draws)-1 {
				return 0, sgSSCUnverifiedIssue(draw.Issue)
			}
			continue
		}
		if found {
			if !stored.DrawAt.Equal(draw.DrawAt) || !storedDrawNumbersEqual(stored.Numbers, draw.Numbers) {
				return 0, fmt.Errorf("SG时时彩第%s期双站数据与已核对历史冲突，禁止覆盖", draw.Issue)
			}
			continue
		}
		pending = append(pending, lottery.Draw{GameID: "sg-ssc", Issue: draw.Issue, Numbers: joinNumbers(draw.Numbers), DrawAt: draw.DrawAt.UTC(),
			SourceRevision: sgSSCSourceRevision, ConversionRevision: sgSSCConversionRevision})
	}
	if len(pending) == 0 {
		return 0, nil
	}
	// A uniqueness race is an error, not proof that the other row was verified.
	result := db.Create(&pending)
	return int(result.RowsAffected), result.Error
}
