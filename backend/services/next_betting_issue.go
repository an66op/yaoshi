package services

import (
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BettingWindow is separate from the draw currently being followed. While a
// result is being collected, a proven subsequent period may already accept
// bets; publishing that window must not move the old draw's polling deadline.
type BettingWindow struct {
	Issue         string    `json:"issue"`
	IssueStatus   string    `json:"issue_status"`
	AcceptAt      time.Time `json:"accept_at"`
	SealAt        time.Time `json:"seal_at"`
	NextDrawAt    time.Time `json:"next_draw_at"`
	DrawInterval  int       `json:"draw_interval"`
	SealSeconds   int       `json:"seal_seconds"`
	SourceHealthy bool      `json:"source_healthy"`
}

// nextBettingSchedule permits one bounded, server-side schedule projection,
// never a synthetic draw. Only the continuous speed feeds are eligible. Four
// consecutive immutable results must prove all three intervals, and the live
// upstream boundary must match that same sequence. Calendar/daily-reset games,
// stale feeds and an additional missing result all fail closed.
func nextBettingSchedule(game *lottery.Game, current *lottery.Issue, draws []lottery.Draw, now time.Time) (*lottery.Game, bool) {
	if game == nil || current == nil || !game.Enabled || !sourceHealthyForGame(game) ||
		(game.SourceKind != "external" && game.SourceKind != "official") ||
		(game.TimingSource != "upstream" && game.TimingSource != "observed") ||
		(game.SyncStatus != "ok" && game.SyncStatus != "syncing") || strings.TrimSpace(game.LastSyncError) != "" {
		return nil, false
	}
	switch game.ID {
	case "speed-racing", "speed-fly", "speed-ssc":
	default:
		return nil, false
	}
	if current.GameID != game.ID || current.Issue != game.NextIssue || current.Status != lottery.IssueStatusAwaiting ||
		current.DrawAt != nil || current.ScheduledDrawAt == nil || !current.ScheduledDrawAt.Equal(game.NextDrawAt) ||
		game.DrawInterval < 10 || game.DrawInterval > 300 || game.LastSyncAt == nil || len(draws) != 4 {
		return nil, false
	}
	interval := time.Duration(game.DrawInterval) * time.Second
	if now.Before(game.NextDrawAt) || !now.Before(game.NextDrawAt.Add(interval)) ||
		game.LastSyncAt.After(now.Add(5*time.Second)) || now.Sub(*game.LastSyncAt) > interval {
		return nil, false
	}
	for index, draw := range draws {
		// These feeds use an uninterrupted numeric sequence, not date-prefixed
		// identifiers. Do not guess a daily reset even if its recent cadence fits.
		if draw.GameID != game.ID || len(strings.TrimSpace(draw.Issue)) >= 11 || draw.DrawAt.IsZero() {
			return nil, false
		}
		if index > 0 && (inferredNextSourceIssue(draw.Issue, draws[index-1].DrawAt) != draws[index-1].Issue ||
			!draw.DrawAt.Add(interval).Equal(draws[index-1].DrawAt)) {
			return nil, false
		}
	}
	if !draws[0].DrawAt.Add(interval).Equal(game.NextDrawAt) ||
		inferredNextSourceIssue(draws[0].Issue, game.NextDrawAt) != game.NextIssue {
		return nil, false
	}
	next := *game
	next.NextDrawAt = game.NextDrawAt.Add(interval)
	next.NextIssue = inferredNextSourceIssue(game.NextIssue, next.NextDrawAt)
	if next.NextIssue == "" {
		return nil, false
	}
	return &next, true
}

// bettingIssueForGame never changes lottery_games or publishes/settles a draw.
// Its only possible write is the same immutable issue/window materialization
// used by the existing timing service. Once source synchronization supplies the
// next period, it adopts this already-frozen window instead of reopening it.
func (s *BetAdminService) bettingIssueForGame(game *lottery.Game, current *lottery.Issue) (*lottery.Issue, error) {
	if current == nil {
		var err error
		current, err = s.EnsureCurrentIssue(game)
		if err != nil {
			return nil, err
		}
	}
	if current.Status != lottery.IssueStatusAwaiting || game == nil ||
		(game.ID != "speed-racing" && game.ID != "speed-fly" && game.ID != "speed-ssc") {
		return current, nil
	}
	var draws []lottery.Draw
	if err := trustedDrawsForGame(s.db, game.ID).Order("draw_at desc, id desc").Limit(4).Find(&draws).Error; err != nil {
		return nil, apperrors.NewSystemError("DRAW_READ_FAILED", "读取下一期时间依据失败", err)
	}
	next, ok := nextBettingSchedule(game, current, draws, time.Now().UTC())
	if !ok {
		return current, nil
	}
	var existing lottery.Issue
	err := s.db.Where("game_id = ? AND issue = ?", game.ID, next.NextIssue).First(&existing).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	if err == nil && (existing.DrawAt != nil || existing.Status == lottery.IssueStatusError ||
		existing.Status == lottery.IssueStatusSettling || existing.Status == lottery.IssueStatusSettled) {
		return &existing, nil // A projection cannot repair/reopen a terminal period.
	}
	return s.EnsureCurrentIssue(next)
}

// BettingIssue is the target for an input without an explicit period. An input
// naming the closed/old period is deliberately never retargeted silently.
func (s *BetAdminService) BettingIssue(gameID string) (string, error) {
	game, err := s.loadGame(gameID)
	if err != nil {
		return "", err
	}
	current, err := s.bettingIssueForGame(game, nil)
	if err != nil {
		return "", err
	}
	return current.Issue, nil
}

func (s *BetAdminService) nextBettingWindow(game *lottery.Game, current *lottery.Issue, workspaceID uint64, rawSettings string) (*BettingWindow, error) {
	target, err := s.bettingIssueForGame(game, current)
	if err != nil {
		return nil, err
	}
	if current == nil || target.Issue == current.Issue || !sharedIssueOpen(target, time.Now().UTC()) {
		return nil, nil
	}
	window, err := ensureIssueWindow(s.db, workspaceID, game, target.Issue, *target.ScheduledDrawAt, rawSettings)
	if err != nil {
		return nil, err
	}
	if windowStatus(window, time.Now().UTC()) != lottery.IssueStatusAccepting {
		return nil, nil
	}
	return &BettingWindow{Issue: target.Issue, IssueStatus: lottery.IssueStatusAccepting,
		AcceptAt: window.AcceptAt, SealAt: window.SealAt, NextDrawAt: window.ScheduledDrawAt,
		DrawInterval: window.DrawInterval, SealSeconds: window.SealSeconds, SourceHealthy: true}, nil
}

// Lock the live game before the issue, matching the source synchronizer's lock
// order. A source failure/disable/schedule correction between status and submit
// must be revalidated before any money moves, even for an already-created next
// window. The shared lock remains held through the placement transaction.
func lockBettingGame(db *gorm.DB, gameID string) (*lottery.Game, error) {
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "请选择游戏")
	}
	var game lottery.Game
	if err := db.Clauses(clause.Locking{Strength: "SHARE"}).First(&game, "id = ?", gameID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("GAME_NOT_FOUND", "游戏不存在")
		}
		return nil, err
	}
	if !game.Enabled {
		return nil, apperrors.NewBusinessError("GAME_DISABLED", "该彩种暂未开放投注")
	}
	return &game, nil
}

func lockBettingIssue(db *gorm.DB, gameID, issue string) error {
	game, err := lockBettingGame(db, gameID)
	if err != nil {
		return err
	}
	if !sourceHealthyForGame(game) {
		return apperrors.NewBusinessError("SOURCE_UNAVAILABLE", "开奖数据暂时异常，本期已暂停投注")
	}
	if err := NewBetAdminService(db).ensureIssueOpen(game, issue); err != nil {
		return err
	}
	if err := lockAcceptingIssue(db, gameID, issue); err != nil {
		return err
	}
	if game.ID == "sg-ssc" {
		// The Game SHARE and Issue UPDATE locks remain held until placement
		// commits. Never bind a new verified ticket to a legacy platform issue.
		var row lottery.Issue
		if err := db.Where("game_id = ? AND issue = ?", game.ID, issue).First(&row).Error; err != nil {
			return err
		}
		return sgSSCIssueEvidenceError(db, issue, &row)
	}
	return nil
}
