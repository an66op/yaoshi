package services

import (
	"backend/data/models/lottery"
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const sgSSCDiscoveryLimit = 144

// Validate the issue's calendar date before the bounded candidate LIMIT. An
// invalid or not-yet-drawn pending ticket must not repeatedly occupy a slot
// ahead of recoverable debt. CASE protects the date casts from malformed data.
const sgSSCValidPastIssueSQL = `CASE WHEN bets.issue ~ '^[0-9]{4}(0[1-9]|1[0-2])(0[1-9]|[12][0-9]|3[01])(00[1-9]|0[1-9][0-9]|1[0-9]{2}|2[0-7][0-9]|28[0-8])$'
    AND LEFT(bets.issue,4) <> '0000' THEN
    to_char(make_date(LEFT(bets.issue,4)::int,SUBSTRING(bets.issue,5,2)::int,1) + (SUBSTRING(bets.issue,7,2)::int-1),'YYYYMMDD') = LEFT(bets.issue,8)
    AND ((make_date(LEFT(bets.issue,4)::int,SUBSTRING(bets.issue,5,2)::int,1) + (SUBSTRING(bets.issue,7,2)::int-1))::timestamp
         + RIGHT(bets.issue,3)::int * interval '5 minutes') AT TIME ZONE 'Asia/Shanghai' < ?
    ELSE FALSE END`

type SGSSCBackfillSummary struct {
	PendingIssues          int64 `json:"pending_issues"`
	RunningIssues          int64 `json:"running_issues"`
	RetryIssues            int64 `json:"retry_issues"`
	BlockedIssues          int64 `json:"blocked_issues"`
	CompletedIssues        int64 `json:"completed_issues"`
	UntrackedPendingIssues int64 `json:"untracked_pending_issues"`
}

type SGSSCBackfillStatus struct {
	GameID         string                         `json:"game_id"`
	Enabled        bool                           `json:"enabled"`
	SourceBound    bool                           `json:"source_bound"`
	Message        string                         `json:"message"`
	MaxAgeDays     int                            `json:"max_age_days"`
	BatchLimit     int                            `json:"batch_limit"`
	Summary        SGSSCBackfillSummary           `json:"summary"`
	Gaps           []lottery.SGSSCBackfillItem    `json:"gaps"`
	HasMoreGaps    bool                           `json:"has_more_gaps"`
	Records        []lottery.SGSSCBackfillAttempt `json:"records"`
	NextBeforeID   uint64                         `json:"next_before_id,omitempty"`
	HasMoreRecords bool                           `json:"has_more_records"`
}

type SGSSCBackfillQueued struct {
	QueuedIssues int64  `json:"queued_issues"`
	Message      string `json:"message"`
}

// Read-only: opening or polling the admin panel cannot enqueue, import, clear
// an error, retry settlement or update a live source's health.
func (s *LotteryService) SGSSCBackfillStatus(ctx context.Context, before uint64, limit int) (SGSSCBackfillStatus, error) {
	result := SGSSCBackfillStatus{GameID: "sg-ssc", MaxAgeDays: int(sgSSCBackfillMaxAge / (24 * time.Hour)), BatchLimit: sgSSCBackfillMaxIssues,
		Gaps: []lottery.SGSSCBackfillItem{}, Records: []lottery.SGSSCBackfillAttempt{}}
	db := s.db.WithContext(ctx)
	var game lottery.Game
	if err := db.Where("id = ?", "sg-ssc").Limit(1).Find(&game).Error; err != nil {
		return result, err
	}
	result.Enabled, result.SourceBound = game.Enabled, sgSSCSourceBound(&game)
	result.Message = "优先恢复当前来源版本待结注单；163母源只证明接口实际返回的有限历史，超出窗口会明确封存待人工核查。补采不改变实时期号、封盘或来源健康。"
	if !result.SourceBound {
		result.Message = "SG时时彩尚未绑定163:64母源与115校验源，补采暂停；历史记录仍保留。"
	} else if !result.Enabled {
		result.Message = "SG时时彩已关闭，补采暂停；历史记录仍保留。"
	}
	if err := db.Raw(`SELECT
		COUNT(*) FILTER (WHERE status = 'pending') AS pending_issues,
		COUNT(*) FILTER (WHERE status = 'running') AS running_issues,
		COUNT(*) FILTER (WHERE status IN ('retry','settlement_retry')) AS retry_issues,
		COUNT(*) FILTER (WHERE status = 'blocked') AS blocked_issues,
		COUNT(*) FILTER (WHERE status = 'completed') AS completed_issues
		FROM lottery_sgssc_backfill_items`).Scan(&result.Summary).Error; err != nil {
		return result, err
	}
	if err := sgSSCUntrackedPending(db).Where(sgSSCValidPastIssueSQL, time.Now()).Distinct("bets.issue").Count(&result.Summary.UntrackedPendingIssues).Error; err != nil {
		return result, err
	}
	if err := db.Where("status <> ?", "completed").Order("CASE WHEN status = 'blocked' THEN 1 ELSE 0 END, draw_at ASC, issue ASC").Limit(51).Find(&result.Gaps).Error; err != nil {
		return result, err
	}
	result.HasMoreGaps = len(result.Gaps) > 50
	if result.HasMoreGaps {
		result.Gaps = result.Gaps[:50]
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	query := db.Order("id DESC").Limit(limit + 1)
	if before > 0 {
		query = query.Where("id < ?", before)
	}
	if err := query.Find(&result.Records).Error; err != nil {
		return result, err
	}
	result.HasMoreRecords = len(result.Records) > limit
	if result.HasMoreRecords {
		result.Records = result.Records[:limit]
		result.NextBeforeID = result.Records[len(result.Records)-1].ID
	}
	return result, nil
}

func sgSSCUntrackedPending(db *gorm.DB) *gorm.DB {
	return db.Table("lottery_bets AS bets").
		Joins("LEFT JOIN lottery_sgssc_backfill_items AS work ON work.issue = bets.issue").
		Where("bets.game_id = ? AND bets.status = ? AND bets.draw_source_revision = ? AND work.issue IS NULL", "sg-ssc", "pending", sgSSCSourceRevision)
}

// The HTTP action only queues work. Its audit intent and the durable queue
// survive client timeouts; upstream I/O/settlement belong to the bounded worker.
func (s *LotteryService) QueueSGSSCBackfill(ctx context.Context, operator, requestID string) (SGSSCBackfillQueued, error) {
	count, err := s.discoverSGSSCBackfill(ctx, time.Now().UTC(), operator, requestID, "admin", true)
	return SGSSCBackfillQueued{QueuedIssues: count, Message: "已登记可恢复缺期；后台每分钟处理一批，完成状态以恢复记录为准。运行中和需人工核对的期不会重置。"}, err
}

// A discovery pass has bounded inserts. Pending receipts take precedence;
// missing history is inferred only BETWEEN current-revision stored anchors,
// never before the verified source cutover or beyond a known result.
func (s *LotteryService) discoverSGSSCBackfill(ctx context.Context, now time.Time, operator, requestID, trigger string, retry bool) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var game lottery.Game
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&game, "id = ?", "sg-ssc").Error; err != nil {
			if err == gorm.ErrRecordNotFound && trigger == "auto" {
				return nil
			}
			return err
		}
		if !game.Enabled || !sgSSCSourceBound(&game) {
			if trigger == "auto" {
				return nil
			}
			return fmt.Errorf("SG时时彩未启用163:64母源与115校验源，不能登记补采")
		}
		operator = limitDBText(defaultString(strings.TrimSpace(operator), "系统SG历史补采"), 100)
		requestID = limitDBText(defaultString(strings.TrimSpace(requestID), fmt.Sprintf("sg-history:%d", now.UnixNano())), 100)
		seed := func(issue, reason string) error {
			if count >= sgSSCDiscoveryLimit {
				return nil
			}
			_, _, at, err := parseSGSSCIssue(issue)
			if err != nil || !at.Before(now) {
				return nil
			}
			row := lottery.SGSSCBackfillItem{Issue: issue, DrawAt: at, Status: "pending", Reason: reason, NextRetryAt: now,
				RequestedBy: operator, RequestTrigger: trigger, RequestID: requestID, CreatedAt: now, UpdatedAt: now}
			insert := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
			count += insert.RowsAffected
			return insert.Error
		}
		var pending []string
		if err := sgSSCUntrackedPending(tx).Where(sgSSCValidPastIssueSQL, now).Select("bets.issue").Group("bets.issue").Order("MIN(bets.created_at) ASC, bets.issue ASC").Limit(sgSSCDiscoveryLimit).Scan(&pending).Error; err != nil {
			return err
		}
		for _, issue := range pending {
			if err := seed(issue, "pending_bet"); err != nil {
				return err
			}
		}
		var known []string
		if err := tx.Table("lottery_issues AS periods").Select("periods.issue").
			Joins("LEFT JOIN lottery_draws AS draws ON draws.game_id = periods.game_id AND draws.issue = periods.issue").
			Joins("LEFT JOIN lottery_sgssc_backfill_items AS work ON work.issue = periods.issue").
			Where("periods.game_id = ? AND periods.source_mode = ? AND periods.scheduled_draw_at < ? AND periods.scheduled_draw_at >= ? AND draws.id IS NULL AND work.issue IS NULL", "sg-ssc", "external", now, now.Add(-sgSSCBackfillMaxAge)).
			Order("periods.scheduled_draw_at ASC").Limit(sgSSCDiscoveryLimit).Scan(&known).Error; err != nil {
			return err
		}
		for _, issue := range known {
			if err := seed(issue, "recorded_issue"); err != nil {
				return err
			}
		}
		var anchors []lottery.Draw
		if err := tx.Select("issue", "draw_at").Where("game_id = ? AND source_revision = ? AND conversion_revision = ? AND draw_at >= ? AND draw_at < ?", "sg-ssc", sgSSCSourceRevision, sgSSCConversionRevision, now.Add(-sgSSCBackfillMaxAge), now).
			Order("draw_at ASC").Limit(sgSSCPeriodsPerDay * 31).Find(&anchors).Error; err != nil {
			return err
		}
		// Preserve the single trusted anchor immediately before the fetch
		// horizon; otherwise an outage spanning the cutoff loses valid gaps.
		var preceding lottery.Draw
		if err := tx.Select("issue", "draw_at").Where("game_id = ? AND source_revision = ? AND conversion_revision = ? AND draw_at < ?", "sg-ssc", sgSSCSourceRevision, sgSSCConversionRevision, now.Add(-sgSSCBackfillMaxAge)).
			Order("draw_at DESC").Limit(1).Find(&preceding).Error; err != nil {
			return err
		}
		if preceding.Issue != "" {
			anchors = append([]lottery.Draw{preceding}, anchors...)
		}
		var tracked []string
		if err := tx.Model(&lottery.SGSSCBackfillItem{}).Where("draw_at >= ?", now.Add(-sgSSCBackfillMaxAge)).Pluck("issue", &tracked).Error; err != nil {
			return err
		}
		seen := make(map[string]bool, len(tracked))
		for _, issue := range tracked {
			seen[issue] = true
		}
		for index := 1; index < len(anchors) && count < sgSSCDiscoveryLimit; index++ {
			left, right := anchors[index-1], anchors[index]
			_, _, leftAt, leftErr := parseSGSSCIssue(left.Issue)
			_, _, rightAt, rightErr := parseSGSSCIssue(right.Issue)
			if leftErr != nil || rightErr != nil || !leftAt.Equal(left.DrawAt) || !rightAt.Equal(right.DrawAt) {
				continue
			}
			start := leftAt.Add(sgSSCInterval)
			cutoff := now.Add(-sgSSCBackfillMaxAge)
			if start.Before(cutoff) {
				start = cutoff.Truncate(sgSSCInterval)
				if start.Before(cutoff) {
					start = start.Add(sgSSCInterval)
				}
			}
			for at := start; at.Before(rightAt) && count < sgSSCDiscoveryLimit; at = at.Add(sgSSCInterval) {
				issue := sgSSCIssueAt(at)
				if seen[issue] {
					continue
				}
				if err := seed(issue, "draw_gap"); err != nil {
					return err
				}
				seen[issue] = true
			}
		}
		if retry {
			// Deliberate manual retry can shorten backoff, but not steal a lease,
			// rewrite a finished journal entry or override a conflict/legacy block.
			var rows []lottery.SGSSCBackfillItem
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("status IN ?", []string{"retry", "settlement_retry"}).Order("draw_at ASC").Limit(sgSSCBackfillMaxIssues).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				if err := tx.Model(&row).Updates(map[string]any{"next_retry_at": now, "requested_by": operator, "request_trigger": trigger, "request_id": requestID, "updated_at": now}).Error; err != nil {
					return err
				}
				count++
			}
		}
		return nil
	})
	return count, err
}

func sgSSCBackfillRetryAt(now time.Time, attempts int) time.Time {
	shift := min(max(attempts-1, 0), 6)
	return now.Add(time.Duration(1<<shift) * 5 * time.Minute)
}

func sortedSGSSCHistoryTargets(items []lottery.SGSSCBackfillItem) []string {
	issues := make([]string, len(items))
	for index, item := range items {
		issues[index] = item.Issue
	}
	sort.Strings(issues)
	return issues
}
