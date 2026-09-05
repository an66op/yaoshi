package services

import (
	"backend/data/models/lottery"
	"backend/data/models/plan"
	apperrors "backend/errors"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MaxPlanHistoryBackfillPeriods = 6

// PlanHistoryBackfillReport makes this explicit maintenance operation auditable.
// Recommendations are saved after their draw and stay ungraded; they are display
// history, never represented as predictions published before the result.
type PlanHistoryBackfillReport struct {
	WorkspaceID            uint64            `json:"workspace_id"`
	RequestedPeriods       int               `json:"requested_periods"`
	ConfiguredGames        int               `json:"configured_games"`
	CoveredGames           int               `json:"covered_games"`
	DisplayVerifiedGames   int               `json:"display_verified_games"`
	AvailablePeriodsByGame map[string]int    `json:"available_periods_by_game"`
	DisplayPeriodsByGame   map[string]int    `json:"display_periods_by_game"`
	CreatedRecommendations int64             `json:"created_recommendations"`
	CreatedStreamPeriods   int64             `json:"created_stream_periods"`
	SkippedGames           map[string]string `json:"skipped_games"`
	CompletedAt            time.Time         `json:"completed_at"`
}

type planHistoricalIssue struct {
	Issue lottery.Issue
	Draw  lottery.Draw
}

func planHistoricalIssues(db *gorm.DB, gameID string, limit int, requireLifecycle bool) ([]planHistoricalIssue, error) {
	var draws []lottery.Draw
	if err := trustedDrawsForGame(db, gameID).Order("draw_at DESC, id DESC").Limit(limit * 3).Find(&draws).Error; err != nil {
		return nil, err
	}
	result := make([]planHistoricalIssue, 0, limit)
	for _, draw := range draws {
		var issue lottery.Issue
		err := db.First(&issue, "game_id = ? AND issue = ?", gameID, draw.Issue).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if requireLifecycle {
				continue
			}
			result = append(result, planHistoricalIssue{Draw: draw})
			if len(result) == limit {
				break
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		result = append(result, planHistoricalIssue{Issue: issue, Draw: draw})
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

func backfillGenericPlanHistory(tx *gorm.DB, workspaceID uint64, game lottery.Game, periods int) (int, int64, error) {
	rows, err := planHistoricalIssues(tx, game.ID, periods, false)
	if err != nil {
		return 0, 0, err
	}
	createdIssues := map[string]bool{}
	var created int64
	now := time.Now().UTC()
	for _, historical := range rows {
		issue := historical.Draw.Issue
		if historical.Issue.Issue != "" {
			issue = historical.Issue.Issue
		}
		for _, master := range planDemoMasters {
			numbers, err := planDemoNumbers(workspaceID, game, issue, master.Key)
			if err != nil {
				return 0, created, err
			}
			rawNumbers, err := canonicalPlanNumbers(numbers)
			if err != nil {
				return 0, created, err
			}
			recommendation := plan.Recommendation{
				WorkspaceID: workspaceID, GameID: game.ID, Issue: issue,
				MasterName: master.Name, MasterTitle: master.Title, MasterColor: master.Color,
				Numbers: rawNumbers, Result: plan.ResultPending, Source: "demo",
				Note: PlanHistoryBackfillNotice, Enabled: true, SortOrder: master.SortOrder,
				CreatedAt: now, UpdatedAt: now,
			}
			insert := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&recommendation)
			if insert.Error != nil {
				return 0, created, insert.Error
			}
			created += insert.RowsAffected
			if insert.RowsAffected > 0 {
				createdIssues[issue] = true
			}
		}
	}
	return len(createdIssues), created, nil
}

func backfillRacingPlanHistory(tx *gorm.DB, workspaceID uint64, periods int) (int, int64, error) {
	rows, err := planHistoricalIssues(tx, "speed-racing", periods, true)
	if err != nil || len(rows) == 0 {
		return 0, 0, err
	}
	// Cycles are written oldest-to-newest while the member view reads periods
	// newest-first. Existing rows are left untouched, making retries idempotent.
	sort.Slice(rows, func(i, j int) bool { return rows[i].Draw.DrawAt.Before(rows[j].Draw.DrawAt) })
	stream := plan.Stream{WorkspaceID: workspaceID, GameID: "speed-racing", Position: 1, PlanKey: DefaultPlanKey}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&stream).Error; err != nil {
		return 0, 0, err
	}
	if err := tx.First(&stream, "workspace_id = ? AND game_id = ? AND position = ? AND plan_key = ?", workspaceID, "speed-racing", 1, DefaultPlanKey).Error; err != nil {
		return 0, 0, err
	}
	var existing []uint64
	if err := tx.Model(&plan.StreamPeriod{}).Where("stream_id = ?", stream.ID).Pluck("issue_id", &existing).Error; err != nil {
		return 0, 0, err
	}
	seen := make(map[uint64]bool, len(existing))
	for _, id := range existing {
		seen[id] = true
	}
	missing := make([]planHistoricalIssue, 0, len(rows))
	for _, row := range rows {
		if !seen[row.Issue.ID] {
			missing = append(missing, row)
		}
	}
	option, _ := planOption(DefaultPlanKey)
	now := time.Now().UTC()
	var created int64
	createdIssues := 0
	for start := 0; start < len(missing); start += option.Periods {
		end := start + option.Periods
		if end > len(missing) {
			end = len(missing)
		}
		group := missing[start:end]
		picks := planCyclePicks(workspaceID, 1, option, group[0].Issue.Issue)
		for index := range picks {
			picks[index].Note = PlanHistoryBackfillNotice
		}
		payload, _ := json.Marshal(picks)
		status := "interrupted"
		if len(group) == option.Periods {
			status = "completed"
		}
		last := group[len(group)-1]
		lastScheduledAt := last.Draw.DrawAt.UTC()
		if last.Issue.ScheduledDrawAt != nil {
			lastScheduledAt = last.Issue.ScheduledDrawAt.UTC()
		}
		cycle := plan.StreamCycle{
			StreamID: stream.ID, Periods: option.Periods, PublishedPeriods: len(group), Status: status,
			StartIssue: group[0].Issue.Issue, LastIssueID: last.Issue.ID, LastScheduledAt: lastScheduledAt,
			PayloadJSON: string(payload), CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&cycle).Error; err != nil {
			return createdIssues, created, err
		}
		for index, historical := range group {
			scheduledAt := historical.Draw.DrawAt.UTC()
			if historical.Issue.ScheduledDrawAt != nil {
				scheduledAt = historical.Issue.ScheduledDrawAt.UTC()
			}
			period := plan.StreamPeriod{StreamID: stream.ID, IssueID: historical.Issue.ID, Issue: historical.Issue.Issue,
				CycleID: cycle.ID, PeriodIndex: index + 1, ScheduledDrawAt: scheduledAt, CreatedAt: now}
			insert := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&period)
			if insert.Error != nil {
				return createdIssues, created, insert.Error
			}
			created += insert.RowsAffected
			if insert.RowsAffected > 0 {
				createdIssues++
			}
		}
	}
	return createdIssues, created, nil
}

// BackfillHistory adds recent, real issue identities to every configured game.
// It is intentionally explicit rather than part of GET or normal startup.
func (s *PlanAutomationService) BackfillHistory(ctx context.Context, workspaceID uint64, periods int) (PlanHistoryBackfillReport, error) {
	report := PlanHistoryBackfillReport{WorkspaceID: workspaceID, RequestedPeriods: periods,
		AvailablePeriodsByGame: map[string]int{}, DisplayPeriodsByGame: map[string]int{}, SkippedGames: map[string]string{}}
	if periods < 1 || periods > MaxPlanHistoryBackfillPeriods {
		return report, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("历史展示期数必须为 1 至 %d", MaxPlanHistoryBackfillPeriods))
	}
	if err := NewPlanContentService(s.db).ensureScope(workspaceID, ""); err != nil {
		return report, err
	}
	config, err := s.Get(workspaceID)
	if err != nil {
		return report, err
	}
	if !config.Enabled {
		return report, apperrors.NewBusinessError("PLAN_AUTOMATION_DISABLED", "计划群尚未开启")
	}
	report.ConfiguredGames = len(config.GameIDs)
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, gameID := range config.GameIDs {
			if err := ctx.Err(); err != nil {
				return err
			}
			available, err := planStreamRoomAvailable(tx, workspaceID, gameID)
			if err != nil {
				return err
			}
			if !available {
				report.SkippedGames[gameID] = "房间未开放该彩种或计划群"
				continue
			}
			var game lottery.Game
			if err := tx.First(&game, "id = ?", gameID).Error; err != nil {
				return err
			}
			if !game.Enabled || strings.TrimSpace(game.LobbyCategory) == "" {
				report.SkippedGames[gameID] = "彩种未开放"
				continue
			}
			if _, _, supported := planDemoNumberRange(game); !supported {
				report.SkippedGames[gameID] = "计划规则不支持"
				continue
			}
			var issueCount int
			var created int64
			if gameID == "speed-racing" {
				issueCount, created, err = backfillRacingPlanHistory(tx, workspaceID, periods)
				report.CreatedStreamPeriods += created
			} else {
				issueCount, created, err = backfillGenericPlanHistory(tx, workspaceID, game, periods)
				report.CreatedRecommendations += created
			}
			if err != nil {
				return err
			}
			// Existing records count toward coverage on an idempotent retry.
			var availablePeriods int64
			if gameID == "speed-racing" {
				if err := tx.Model(&plan.StreamPeriod{}).Joins("JOIN plan_streams ON plan_streams.id = plan_stream_periods.stream_id").
					Where("plan_streams.workspace_id = ? AND plan_streams.game_id = ?", workspaceID, gameID).Distinct("plan_stream_periods.issue").Count(&availablePeriods).Error; err != nil {
					return err
				}
			} else if err := tx.Model(&plan.Recommendation{}).Where("workspace_id = ? AND game_id = ? AND enabled = true", workspaceID, gameID).
				Distinct("issue").Count(&availablePeriods).Error; err != nil {
				return err
			}
			if availablePeriods > int64(periods) {
				availablePeriods = int64(periods)
			}
			issueCount = int(availablePeriods)
			if issueCount >= periods {
				report.CoveredGames++
				report.AvailablePeriodsByGame[gameID] = issueCount
			} else if issueCount > 0 {
				report.AvailablePeriodsByGame[gameID] = issueCount
				report.SkippedGames[gameID] = fmt.Sprintf("目前仅有 %d/%d 期可展示", issueCount, periods)
			} else {
				report.SkippedGames[gameID] = "没有可用的真实开奖期"
			}
		}
		return nil
	})
	if err != nil {
		return report, err
	}
	content := NewPlanContentService(s.db)
	for _, gameID := range config.GameIDs {
		seen := map[string]bool{}
		if gameID == "speed-racing" {
			detail, err := content.StreamDetail(workspaceID, 1, DefaultPlanKey, periods)
			if err != nil {
				return report, err
			}
			for _, row := range detail.History {
				seen[row.Issue] = true
			}
		} else {
			detail, err := content.Detail(workspaceID, gameID, periods)
			if err != nil {
				return report, err
			}
			for _, row := range detail.History {
				seen[row.Issue] = true
			}
		}
		report.DisplayPeriodsByGame[gameID] = len(seen)
		if len(seen) < periods {
			return report, fmt.Errorf("彩种 %s 只显示 %d/%d 期计划记录", gameID, len(seen), periods)
		}
		report.DisplayVerifiedGames++
	}
	report.CompletedAt = time.Now().UTC()
	return report, nil
}
