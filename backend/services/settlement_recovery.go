package services

import (
	"backend/cluster"
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultRecoveryLimit       = 100
	maxRecoveryLimit           = 500
	missingIssueStaleAfter     = 15 * time.Minute
	awaitingDrawStaleAfter     = 30 * time.Minute
	settlementStaleAfter       = 5 * time.Minute
	settlementRetryAttempts    = 3
	settlementRetryBaseDelay   = 35 * time.Millisecond
	settlementRecoveryInterval = time.Minute
)

var settlementRecoveryMu sync.Mutex

// SettlementHealthSummary reports durable settlement debt. A recently
// successful upstream request alone is not enough to report a healthy draw
// service while old periods are unresolved or stuck in settlement.
type SettlementHealthSummary struct {
	Healthy                  bool      `json:"healthy"`
	GeneratedAt              time.Time `json:"generated_at"`
	UnresolvedBetCount       int64     `json:"unresolved_bet_count"`
	RecoverableBetCount      int64     `json:"recoverable_bet_count"`
	UnrecoverableBetCount    int64     `json:"unrecoverable_bet_count"`
	MissingIssueBetCount     int64     `json:"missing_issue_bet_count"`
	DisabledGamePendingCount int64     `json:"disabled_game_pending_count"`
	AbnormalBetCount         int64     `json:"abnormal_bet_count"`
	StaleIssueCount          int64     `json:"stale_issue_count"`
	StalePendingIssueCount   int64     `json:"stale_pending_issue_count"`
	StaleAwaitingIssueCount  int64     `json:"stale_awaiting_issue_count"`
	StaleSettlingIssueCount  int64     `json:"stale_settling_issue_count"`
	ErrorIssueCount          int64     `json:"error_issue_count"`
	SourceErrorGameCount     int64     `json:"source_error_game_count"`
}

type SettlementRecoveryFailure struct {
	GameID string `json:"game_id"`
	Issue  string `json:"issue"`
	Error  string `json:"error"`
}

type SettlementRecoveryResult struct {
	StartedAt          time.Time                   `json:"started_at"`
	FinishedAt         time.Time                   `json:"finished_at"`
	AlreadyRunning     bool                        `json:"already_running"`
	ScannedIssues      int                         `json:"scanned_issues"`
	SettledIssues      int                         `json:"settled_issues"`
	SettledBets        int64                       `json:"settled_bets"`
	MarkedErrorIssues  int                         `json:"marked_error_issues"`
	MarkedAbnormalBets int64                       `json:"marked_abnormal_bets"`
	DeferredIssues     int                         `json:"deferred_issues"`
	Failures           []SettlementRecoveryFailure `json:"failures"`
	Health             SettlementHealthSummary     `json:"health"`
}

type settlementCandidate struct {
	GameID               string
	Issue                string
	Pending              int64
	OldestBetAt          time.Time
	GameExists           bool
	GameEnabled          bool
	SourceKind           string
	IssueID              uint64
	IssueStatus          string
	IssueSealAt          *time.Time
	IssueSourceMode      string
	IssueScheduledDrawAt *time.Time
	IssueLastError       string
	DrawID               uint64
}

type recoveryAction int

const (
	recoveryDefer recoveryAction = iota
	recoverySettle
	recoveryMarkAbnormal
)

// SettleIssue retries only database concurrency failures. Business errors and
// invalid draw data are never retried or replaced with a generated result.
func (s *BetAdminService) SettleIssue(gameID, issue, operator string) (*SettlementResult, error) {
	return s.settleIssueGuarded(gameID, issue, operator, nil)
}

// Historical recovery can fence its own worker at the real financial commit
// boundary. Ordinary settlement retains its existing policy and idempotency.
func (s *BetAdminService) settleIssueGuarded(gameID, issue, operator string, gate func(*gorm.DB) error) (*SettlementResult, error) {
	var lastErr error
	for attempt := 0; attempt < settlementRetryAttempts; attempt++ {
		result, err := s.settleIssueOnce(gameID, issue, operator, gate)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !isRetryableTransactionError(err) || attempt == settlementRetryAttempts-1 {
			return nil, err
		}
		time.Sleep(time.Duration(attempt+1) * settlementRetryBaseDelay)
	}
	return nil, lastErr
}

func isRetryableTransactionError(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "40P01", "40001", "55P03":
			return true
		}
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "deadlock detected") ||
		strings.Contains(message, "could not serialize access") ||
		strings.Contains(message, "serialization failure")
}

func limitDBText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func (s *LotteryService) SettlementHealth(now time.Time) (SettlementHealthSummary, error) {
	return NewBetAdminService(s.db).SettlementHealth(now)
}

func (s *BetAdminService) SettlementHealth(now time.Time) (SettlementHealthSummary, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	result := SettlementHealthSummary{GeneratedAt: now}
	base := s.db.Model(&bet.Bet{}).
		Joins("LEFT JOIN lottery_issues AS issue_row ON issue_row.game_id = lottery_bets.game_id AND issue_row.issue = lottery_bets.issue").
		Joins("LEFT JOIN lottery_games AS game_row ON game_row.id = lottery_bets.game_id").
		Joins("LEFT JOIN lottery_draws AS draw_row ON draw_row.game_id = lottery_bets.game_id AND draw_row.issue = lottery_bets.issue").
		Where("lottery_bets.status = ?", "pending")
	unresolvedWhere := `draw_row.id IS NOT NULL OR issue_row.id IS NULL OR game_row.id IS NULL OR game_row.enabled = FALSE OR issue_row.status IN ? OR issue_row.seal_at < ?`
	unresolvedStatuses := []string{lottery.IssueStatusAwaiting, lottery.IssueStatusSettling, lottery.IssueStatusSettled, lottery.IssueStatusError}
	if err := base.Session(&gorm.Session{}).Where(unresolvedWhere, unresolvedStatuses, now.Add(-awaitingDrawStaleAfter)).Count(&result.UnresolvedBetCount).Error; err != nil {
		return result, err
	}
	verifiedSQL, verifiedArgs := orderedBingoRecoveryRevisionSQL("lottery_bets.game_id", "draw_row")
	if err := base.Session(&gorm.Session{}).Where("draw_row.id IS NOT NULL").Where(verifiedSQL, verifiedArgs...).Count(&result.RecoverableBetCount).Error; err != nil {
		return result, err
	}
	if err := base.Session(&gorm.Session{}).Where("issue_row.id IS NULL").Count(&result.MissingIssueBetCount).Error; err != nil {
		return result, err
	}
	if err := base.Session(&gorm.Session{}).Where("game_row.id IS NULL OR game_row.enabled = FALSE").Count(&result.DisabledGamePendingCount).Error; err != nil {
		return result, err
	}
	if err := s.db.Model(&bet.Bet{}).Where("reconciliation_status = ?", "abnormal").Count(&result.AbnormalBetCount).Error; err != nil {
		return result, err
	}
	if err := s.db.Model(&lottery.Issue{}).
		Where("status IN ? AND seal_at < ?", []string{lottery.IssueStatusPending, lottery.IssueStatusAccepting, lottery.IssueStatusSealed}, now.Add(-awaitingDrawStaleAfter)).
		Count(&result.StalePendingIssueCount).Error; err != nil {
		return result, err
	}
	if err := s.db.Model(&lottery.Issue{}).Where("status = ? AND seal_at < ?", lottery.IssueStatusAwaiting, now.Add(-awaitingDrawStaleAfter)).Count(&result.StaleAwaitingIssueCount).Error; err != nil {
		return result, err
	}
	if err := s.db.Model(&lottery.Issue{}).Where("status = ? AND COALESCE(draw_at, seal_at) < ?", lottery.IssueStatusSettling, now.Add(-settlementStaleAfter)).Count(&result.StaleSettlingIssueCount).Error; err != nil {
		return result, err
	}
	if err := s.db.Model(&lottery.Issue{}).Where("status = ?", lottery.IssueStatusError).Count(&result.ErrorIssueCount).Error; err != nil {
		return result, err
	}
	if err := s.db.Model(&lottery.Game{}).
		Where("enabled = ? AND source_kind IN ? AND sync_status = ?", true, []string{"external", "official"}, "error").
		Count(&result.SourceErrorGameCount).Error; err != nil {
		return result, err
	}
	result.StaleIssueCount = result.StalePendingIssueCount + result.StaleAwaitingIssueCount + result.StaleSettlingIssueCount
	result.UnrecoverableBetCount = result.UnresolvedBetCount - result.RecoverableBetCount
	if result.UnrecoverableBetCount < 0 {
		result.UnrecoverableBetCount = 0
	}
	result.Healthy = result.UnresolvedBetCount == 0 && result.StaleIssueCount == 0 && result.ErrorIssueCount == 0 && result.SourceErrorGameCount == 0
	return result, nil
}

// RecoverSettlementBacklog safely advances only issues that already have an
// immutable draw. Missing results are recorded for reconciliation and never
// replaced with random numbers. Repeated calls are idempotent.
func (s *BetAdminService) RecoverSettlementBacklog(ctx context.Context, limit int, operator string) (SettlementRecoveryResult, error) {
	result := SettlementRecoveryResult{StartedAt: time.Now().UTC(), Failures: make([]SettlementRecoveryFailure, 0)}
	if !settlementRecoveryMu.TryLock() {
		result.AlreadyRunning = true
		result.FinishedAt = time.Now().UTC()
		health, err := s.SettlementHealth(result.FinishedAt)
		result.Health = health
		return result, err
	}
	defer settlementRecoveryMu.Unlock()
	if limit <= 0 {
		limit = defaultRecoveryLimit
	}
	if limit > maxRecoveryLimit {
		limit = maxRecoveryLimit
	}
	operator = defaultString(strings.TrimSpace(operator), "系统对账恢复")

	candidates, err := s.pendingSettlementCandidates(ctx, limit)
	if err != nil {
		return result, err
	}
	now := time.Now().UTC()
	for _, candidate := range candidates {
		result.ScannedIssues++
		action, reason := recoveryActionForCandidate(candidate, now)
		switch action {
		case recoverySettle:
			settled, settleErr := s.SettleIssue(candidate.GameID, candidate.Issue, operator)
			if settleErr != nil {
				result.addFailure(candidate.GameID, candidate.Issue, settleErr)
				continue
			}
			result.SettledIssues++
			result.SettledBets += settled.Won + settled.Lost + settled.Push
		case recoveryMarkAbnormal:
			markedIssue, markedBets, markErr := s.markUnsafeSettlement(ctx, candidate, reason)
			if markErr != nil {
				result.addFailure(candidate.GameID, candidate.Issue, markErr)
				continue
			}
			if markedIssue {
				result.MarkedErrorIssues++
			}
			result.MarkedAbnormalBets += markedBets
		default:
			result.DeferredIssues++
		}
	}

	// Also recover stale lifecycle rows which have no pending bets. This clears
	// periods left in settling after a process crash without inventing a draw.
	marked, failures, staleErr := s.recoverStaleIssueRows(ctx, limit, operator)
	result.MarkedErrorIssues += marked
	result.Failures = append(result.Failures, failures...)
	if staleErr != nil {
		return result, staleErr
	}
	result.FinishedAt = time.Now().UTC()
	health, healthErr := s.SettlementHealth(result.FinishedAt)
	result.Health = health
	return result, healthErr
}

func (r *SettlementRecoveryResult) addFailure(gameID, issue string, err error) {
	if len(r.Failures) >= 50 {
		return
	}
	r.Failures = append(r.Failures, SettlementRecoveryFailure{GameID: gameID, Issue: issue, Error: limitDBText(err.Error(), 500)})
}

func (s *BetAdminService) pendingSettlementCandidates(ctx context.Context, limit int) ([]settlementCandidate, error) {
	rows := make([]settlementCandidate, 0, limit)
	verifiedDrawSQL, verifiedDrawArgs := orderedBingoRecoveryRevisionSQL("bets.game_id", "draws")
	query := fmt.Sprintf(`
		SELECT bets.game_id,
		       bets.issue,
		       COUNT(*) AS pending,
		       MIN(bets.created_at) AS oldest_bet_at,
		       (games.id IS NOT NULL) AS game_exists,
		       COALESCE(games.enabled, FALSE) AS game_enabled,
		       COALESCE(games.source_kind, '') AS source_kind,
		       COALESCE(issues.id, 0) AS issue_id,
		       COALESCE(issues.status, '') AS issue_status,
		       issues.seal_at AS issue_seal_at,
		       COALESCE(issues.source_mode, '') AS issue_source_mode,
		       issues.scheduled_draw_at AS issue_scheduled_draw_at,
		       COALESCE(issues.last_error, '') AS issue_last_error,
		       COALESCE(draws.id, 0) AS draw_id
		FROM lottery_bets AS bets
		LEFT JOIN lottery_games AS games ON games.id = bets.game_id
		LEFT JOIN lottery_issues AS issues ON issues.game_id = bets.game_id AND issues.issue = bets.issue
		LEFT JOIN lottery_draws AS draws ON draws.game_id = bets.game_id AND draws.issue = bets.issue
		WHERE bets.status = 'pending'
		  AND (bets.reconciliation_status <> 'abnormal'
		       OR (draws.id IS NOT NULL AND %s))
		GROUP BY bets.game_id, bets.issue, games.id, games.enabled, games.source_kind,
		         issues.id, issues.status, issues.seal_at, issues.source_mode,
		         issues.scheduled_draw_at, issues.last_error, draws.id
		ORDER BY MIN(bets.created_at) ASC, bets.game_id ASC, bets.issue ASC
		LIMIT ?
	`, verifiedDrawSQL)
	args := append(verifiedDrawArgs, limit)
	err := s.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error
	return rows, err
}

// orderedBingoRecoveryRevisionSQL mirrors the service-level settlement gate
// inside candidate selection. Once an unverified ordered draw has been marked
// abnormal it must stay in the manual-reconciliation queue without occupying
// every bounded recovery pass. A later verified import can update/replace the
// safe row and make it eligible again; unrelated games retain the legacy retry
// policy.
func orderedBingoRecoveryRevisionSQL(gameExpression, drawAlias string) (string, []any) {
	versionedIDs := []string{"sg-ssc"}
	contractCases := make([]string, 0)
	contractArgs := make([]any, 0)
	for _, contract := range trustedDrawRevisionContracts("sg-ssc") {
		contractCases = append(contractCases, fmt.Sprintf("(%s = ? AND %s.source_revision = ? AND %s.conversion_revision = ?)", gameExpression, drawAlias, drawAlias))
		contractArgs = append(contractArgs, "sg-ssc", contract.SourceRevision, contract.ConversionRevision)
	}
	// Both SG revisions require an exact immutable ticket snapshot. This keeps
	// old tickets settleable against old verified draws without allowing either
	// side of the cutover to acquire the other's identity.
	currentSources := []string{sgSSCSourceRevision, sgSSCLegacySourceRevision, bingo163SetSourceRevision, bingo163OrderSourceRevision, bingo163VerifiedSourceRevision}
	for _, binding := range source163MirrorBindings {
		versionedIDs = append(versionedIDs, binding.GameID)
		contractCases = append(contractCases, fmt.Sprintf("(%s = ? AND %s.source_revision = ? AND %s.conversion_revision = ?)", gameExpression, drawAlias, drawAlias))
		contractArgs = append(contractArgs, binding.GameID, binding.Revision, source163MirrorConversionVersion)
		currentSources = append(currentSources, binding.Revision)
	}
	for _, binding := range source163PC28Bindings {
		versionedIDs = append(versionedIDs, binding.GameID)
		contractCases = append(contractCases, fmt.Sprintf("(%s = ? AND %s.source_revision = ? AND %s.conversion_revision = ?)", gameExpression, drawAlias, drawAlias))
		contractArgs = append(contractArgs, binding.GameID, binding.Revision, source163MirrorConversionVersion)
		currentSources = append(currentSources, binding.Revision)
	}
	for _, binding := range source163MarkSixBindings {
		versionedIDs = append(versionedIDs, binding.GameID)
		contractCases = append(contractCases, fmt.Sprintf("(%s = ? AND %s.source_revision = ? AND %s.conversion_revision = ?)", gameExpression, drawAlias, drawAlias))
		contractArgs = append(contractArgs, binding.GameID, binding.SourceRevision, binding.ConversionRevision)
		currentSources = append(currentSources, binding.SourceRevision)
	}
	for _, binding := range bingo163Bindings {
		versionedIDs = append(versionedIDs, binding.GameID)
		for _, contract := range trustedDrawRevisionContracts(binding.GameID) {
			contractCases = append(contractCases, fmt.Sprintf("(%s = ? AND %s.source_revision = ? AND %s.conversion_revision = ?)", gameExpression, drawAlias, drawAlias))
			contractArgs = append(contractArgs, binding.GameID, contract.SourceRevision, contract.ConversionRevision)
		}
	}
	if len(versionedIDs) == 0 {
		return "TRUE", nil
	}
	query := fmt.Sprintf("(%s NOT IN ? OR (%s))", gameExpression, strings.Join(contractCases, " OR "))
	args := []any{versionedIDs}
	args = append(args, contractArgs...)

	// A newly versioned draw cannot give a legacy/blank ticket a new source
	// identity. Old verified Bingo revisions remain readable and settleable;
	// only the current 163/SG contracts require the placement snapshot.
	query += fmt.Sprintf(` AND (%s NOT IN ? OR %s.source_revision NOT IN ? OR (
		NOT EXISTS (SELECT 1 FROM lottery_issues legacy_issue WHERE legacy_issue.game_id = %s AND legacy_issue.issue = %s.issue AND legacy_issue.source_mode <> 'external')
		AND NOT EXISTS (SELECT 1 FROM lottery_bets legacy_bet WHERE legacy_bet.game_id = %s AND legacy_bet.issue = %s.issue AND COALESCE(legacy_bet.draw_source_revision, '') <> %s.source_revision)
		AND NOT EXISTS (SELECT 1 FROM lottery_bet_archives legacy_archive WHERE legacy_archive.game_id = %s AND legacy_archive.issue = %s.issue AND COALESCE(legacy_archive.draw_source_revision, '') <> %s.source_revision)
	))`, gameExpression, drawAlias, gameExpression, drawAlias, gameExpression, drawAlias, drawAlias, gameExpression, drawAlias, drawAlias)
	args = append(args, versionedIDs, currentSources)
	return query, args
}

func recoveryActionForCandidate(candidate settlementCandidate, now time.Time) (recoveryAction, string) {
	if candidate.DrawID > 0 && candidate.GameExists {
		return recoverySettle, ""
	}
	if !candidate.GameExists {
		return recoveryMarkAbnormal, "游戏配置不存在，且没有可验证的开奖结果"
	}
	if !candidate.GameEnabled {
		return recoveryMarkAbnormal, "彩种已停用，且该期没有可验证的开奖结果"
	}
	if candidate.IssueID == 0 {
		if !candidate.OldestBetAt.IsZero() && now.Sub(candidate.OldestBetAt) >= missingIssueStaleAfter {
			return recoveryMarkAbnormal, "历史注单缺少期号生命周期，且没有可验证的开奖结果"
		}
		return recoveryDefer, "等待期号记录"
	}
	if sgSSCSourceFailureCanWait(candidate, now) {
		return recoveryDefer, "SG时时彩本期尚未到已记录开奖时间，等待双站恢复"
	}
	switch candidate.IssueStatus {
	case lottery.IssueStatusError, lottery.IssueStatusSettled:
		return recoveryMarkAbnormal, "期号已关闭但没有可验证的开奖结果"
	case lottery.IssueStatusSettling:
		if candidate.IssueSealAt == nil || now.Sub(candidate.IssueSealAt.UTC()) >= settlementStaleAfter {
			return recoveryMarkAbnormal, "结算任务超时，且没有可验证的开奖结果"
		}
	case lottery.IssueStatusAwaiting:
		if candidate.IssueSealAt == nil || now.Sub(candidate.IssueSealAt.UTC()) >= awaitingDrawStaleAfter {
			return recoveryMarkAbnormal, "等待开奖超时，尚未取得可验证的开奖结果"
		}
	default:
		if candidate.IssueSealAt != nil && now.Sub(candidate.IssueSealAt.UTC()) >= awaitingDrawStaleAfter {
			return recoveryMarkAbnormal, "期号封盘后长时间未取得可验证的开奖结果"
		}
	}
	return recoveryDefer, "仍在安全等待窗口内"
}

func (s *BetAdminService) markUnsafeSettlement(ctx context.Context, candidate settlementCandidate, reason string) (bool, int64, error) {
	reason = reconciliationIssueError(reason)
	markedIssue := false
	var markedBets int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if candidate.GameExists {
			issueRow := lottery.Issue{
				GameID: candidate.GameID, Issue: candidate.Issue, Status: lottery.IssueStatusError,
				SourceMode: sourceMode(candidate.SourceKind), AcceptAt: candidate.OldestBetAt.UTC(), SealAt: candidate.OldestBetAt.UTC(), LastError: reason,
			}
			if candidate.GameID == "sg-ssc" && candidate.IssueID == 0 {
				blocked, err := sgSSCLegacyIssues(tx, []string{candidate.Issue})
				if err != nil {
					return err
				}
				if blocked[candidate.Issue] {
					issueRow.SourceMode = "legacy"
				}
			}
			if issueRow.AcceptAt.IsZero() {
				issueRow.AcceptAt = time.Now().UTC()
				issueRow.SealAt = issueRow.AcceptAt
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "game_id"}, {Name: "issue"}},
				DoUpdates: clause.Assignments(map[string]any{"status": lottery.IssueStatusError, "last_error": reason}),
			}).Create(&issueRow).Error; err != nil {
				return err
			}
			markedIssue = candidate.IssueID == 0 || candidate.IssueStatus != lottery.IssueStatusError
		}
		updated := tx.Model(&bet.Bet{}).
			Where("game_id = ? AND issue = ? AND status = ?", candidate.GameID, candidate.Issue, "pending").
			Where("reconciliation_status <> ? OR reconciliation_note <> ?", "abnormal", reason).
			Updates(map[string]any{"reconciliation_status": "abnormal", "reconciliation_note": reason})
		markedBets = updated.RowsAffected
		return updated.Error
	})
	if err != nil {
		return false, 0, err
	}
	return markedIssue, markedBets, nil
}

func sourceMode(kind string) string {
	if kind == "external" || kind == "official" {
		return "external"
	}
	return "platform"
}

func reconciliationIssueError(reason string) string {
	reason = strings.TrimSpace(reason)
	if !strings.HasPrefix(reason, "对账异常：") {
		reason = "对账异常：" + reason
	}
	return limitDBText(reason, 500)
}

type staleIssueCandidate struct {
	GameID      string
	Issue       string
	Status      string
	SourceMode  string
	LastError   string
	DrawID      uint64
	PendingBets int64
}

func (s *BetAdminService) recoverStaleIssueRows(ctx context.Context, limit int, operator string) (int, []SettlementRecoveryFailure, error) {
	rows := make([]staleIssueCandidate, 0, limit)
	now := time.Now().UTC()
	verifiedDrawSQL, verifiedDrawArgs := orderedBingoRecoveryRevisionSQL("issues.game_id", "draws")
	query := fmt.Sprintf(`
		SELECT issues.game_id,
		       issues.issue,
		       issues.status,
		       issues.source_mode,
		       issues.last_error,
		       COALESCE(draws.id, 0) AS draw_id,
		       COUNT(bets.id) FILTER (WHERE bets.status = 'pending') AS pending_bets
		FROM lottery_issues AS issues
		LEFT JOIN lottery_draws AS draws ON draws.game_id = issues.game_id AND draws.issue = issues.issue
		LEFT JOIN lottery_bets AS bets ON bets.game_id = issues.game_id AND bets.issue = issues.issue
		WHERE (issues.status = ? AND COALESCE(issues.draw_at, issues.seal_at) < ?)
		   OR (issues.status = ? AND issues.seal_at < ?)
		   OR (issues.status IN ? AND issues.seal_at < ?)
		   OR (issues.status = ? AND draws.id IS NOT NULL AND %s)
		GROUP BY issues.id, draws.id
		ORDER BY issues.seal_at ASC
		LIMIT ?
	`, verifiedDrawSQL)
	args := []any{
		lottery.IssueStatusSettling, now.Add(-settlementStaleAfter), lottery.IssueStatusAwaiting, now.Add(-awaitingDrawStaleAfter),
		[]string{lottery.IssueStatusPending, lottery.IssueStatusAccepting, lottery.IssueStatusSealed}, now.Add(-awaitingDrawStaleAfter), lottery.IssueStatusError,
	}
	args = append(args, verifiedDrawArgs...)
	args = append(args, limit)
	if err := s.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return 0, nil, err
	}
	marked := 0
	failures := make([]SettlementRecoveryFailure, 0)
	for _, row := range rows {
		if row.DrawID > 0 {
			if _, err := s.SettleIssue(row.GameID, row.Issue, operator); err != nil {
				failures = append(failures, SettlementRecoveryFailure{GameID: row.GameID, Issue: row.Issue, Error: limitDBText(err.Error(), 500)})
			}
			continue
		}
		reason := reconciliationIssueError("开奖生命周期超时，尚未取得可验证的开奖结果")
		if err := s.setIssueStatus(row.GameID, row.Issue, lottery.IssueStatusError, reason, nil, nil); err != nil {
			failures = append(failures, SettlementRecoveryFailure{GameID: row.GameID, Issue: row.Issue, Error: limitDBText(err.Error(), 500)})
			continue
		}
		marked++
	}
	return marked, failures, nil
}

// StartSettlementRecovery runs a bounded audit after startup and periodically
// thereafter. It never deletes history and never generates external results.
func StartSettlementRecovery(ctx context.Context, db *gorm.DB) {
	go func() {
		run := func() {
			_, err := cluster.RunWithLease(ctx, "scheduler:settlement-recovery", 10*time.Minute, func() error {
				result, recoveryErr := NewBetAdminService(db).RecoverSettlementBacklog(ctx, defaultRecoveryLimit, "系统自动对账")
				if recoveryErr != nil {
					return recoveryErr
				}
				if result.SettledIssues > 0 || result.MarkedAbnormalBets > 0 || len(result.Failures) > 0 {
					log.Printf("结算积压检查完成: settled_issues=%d settled_bets=%d abnormal_bets=%d failures=%d", result.SettledIssues, result.SettledBets, result.MarkedAbnormalBets, len(result.Failures))
				}
				return nil
			})
			if err != nil {
				log.Printf("结算积压调度跳过或执行失败: %v", err)
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
		ticker := time.NewTicker(settlementRecoveryInterval)
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
