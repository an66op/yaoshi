package services

import (
	"backend/data/models/lottery"
	"backend/data/models/settings"
	apperrors "backend/errors"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const defaultSealSeconds = 30

// The public names deliberately match the existing settings JSON and model.
// Per-game overrides are optional; rooms without them retain their room-wide
// seal_seconds. Draw cadence itself belongs to the shared upstream schedule.
func configuredSealSeconds(raw, gameID string) int {
	result := defaultSealSeconds
	var config map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &config) != nil {
		return result
	}
	if value, ok := timingSeconds(config["seal_seconds"]); ok {
		result = value
	}
	var overrides map[string]map[string]json.RawMessage
	if json.Unmarshal(config["game_timing_overrides"], &overrides) == nil {
		if value, ok := timingSeconds(overrides[gameID]["seal_seconds"]); ok {
			result = value
		}
	}
	return result
}

func timingSeconds(raw json.RawMessage) (int, bool) {
	var value float64
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &value) != nil ||
		value < 0 || value > 86400 || math.Trunc(value) != value {
		return 0, false
	}
	return int(value), true
}

func validateGameTimingSettings(raw json.RawMessage) error {
	var config map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &config) != nil {
		return nil // The existing settings decoder owns malformed top-level JSON.
	}
	if value, exists := config["seal_seconds"]; exists {
		if _, ok := timingSeconds(value); !ok {
			return apperrors.NewBusinessError("INVALID_SETTINGS", "封盘秒数必须是 0 至 86400 的整数")
		}
	}
	if value, exists := config["game_timing_overrides"]; exists {
		var overrides map[string]map[string]json.RawMessage
		if json.Unmarshal(value, &overrides) != nil {
			return apperrors.NewBusinessError("INVALID_SETTINGS", "彩种时间配置格式不正确")
		}
		for gameID, item := range overrides {
			if strings.TrimSpace(gameID) == "" || len(gameID) > 40 {
				return apperrors.NewBusinessError("INVALID_SETTINGS", "彩种时间配置的彩种编号不正确")
			}
			if seconds, exists := item["seal_seconds"]; exists {
				if _, ok := timingSeconds(seconds); !ok {
					return apperrors.NewBusinessError("INVALID_SETTINGS", "彩种封盘秒数必须是 0 至 86400 的整数")
				}
			}
		}
	}
	return nil
}

func readTimingSettings(db *gorm.DB, workspaceID uint64) (string, uint64, error) {
	var row settings.SystemConfig
	query := db.Select("workspace_id", "game_settings_json")
	if workspaceID == 0 {
		query = query.Where("workspace_id = COALESCE((SELECT id FROM workspaces WHERE type = ? ORDER BY id LIMIT 1), 0)", "platform")
	} else {
		query = query.Where("workspace_id = ?", workspaceID)
	}
	err := query.First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return "{}", workspaceID, nil
	}
	if err != nil {
		return "", workspaceID, apperrors.NewSystemError("SETTINGS_READ_FAILED", "读取封盘配置失败", err)
	}
	return row.GameSettingsJSON, row.WorkspaceID, nil
}

func effectiveDrawInterval(game *lottery.Game) int {
	if game == nil || game.DrawInterval <= 0 {
		return 300
	}
	return game.DrawInterval
}

func newIssueWindow(workspaceID uint64, game *lottery.Game, issue string, drawAt time.Time, sealSeconds int) lottery.IssueWindow {
	interval := effectiveDrawInterval(game)
	drawAt = drawAt.UTC()
	return lottery.IssueWindow{
		WorkspaceID: workspaceID, GameID: game.ID, Issue: issue,
		AcceptAt:        drawAt.Add(-time.Duration(interval) * time.Second),
		SealAt:          drawAt.Add(-time.Duration(sealSeconds) * time.Second),
		ScheduledDrawAt: drawAt, DrawInterval: interval, SealSeconds: sealSeconds,
	}
}

// shortenIssueWindow preserves a closed window even when settings or an
// upstream estimate are changed later. Increasing acceptance time takes effect
// on the next issue; an earlier cutoff can always make the current one safer.
func shortenIssueWindow(stored, candidate lottery.IssueWindow) lottery.IssueWindow {
	if candidate.ScheduledDrawAt.Before(stored.ScheduledDrawAt) {
		stored.ScheduledDrawAt = candidate.ScheduledDrawAt
		stored.DrawInterval = candidate.DrawInterval
		stored.AcceptAt = candidate.AcceptAt
	}
	if candidate.SealAt.Before(stored.SealAt) {
		stored.SealAt = candidate.SealAt
	}
	stored.SealSeconds = int(stored.ScheduledDrawAt.Sub(stored.SealAt) / time.Second)
	return stored
}

func ensureIssueWindow(db *gorm.DB, workspaceID uint64, game *lottery.Game, issue string, drawAt time.Time, rawSettings string) (*lottery.IssueWindow, error) {
	if game == nil || strings.TrimSpace(issue) == "" || drawAt.IsZero() {
		return nil, apperrors.NewBusinessError("ISSUE_CLOSED", "尚未取得有效开奖时间，请等待同步")
	}
	candidate := newIssueWindow(workspaceID, game, issue, drawAt, configuredSealSeconds(rawSettings, game.ID))
	var stored lottery.IssueWindow
	err := db.Where("workspace_id = ? AND game_id = ? AND issue = ?", workspaceID, game.ID, issue).First(&stored).Error
	if err == gorm.ErrRecordNotFound {
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; err != nil {
			return nil, err
		}
		if err := db.Where("workspace_id = ? AND game_id = ? AND issue = ?", workspaceID, game.ID, issue).First(&stored).Error; err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	shortened := shortenIssueWindow(stored, candidate)
	if !shortened.SealAt.Equal(stored.SealAt) || !shortened.ScheduledDrawAt.Equal(stored.ScheduledDrawAt) {
		// LEAST also prevents a concurrent settings read from extending a cutoff
		// which another request has already shortened.
		if err := db.Model(&lottery.IssueWindow{}).Where("id = ?", stored.ID).Updates(map[string]any{
			"seal_at":           gorm.Expr("LEAST(seal_at, ?)", shortened.SealAt),
			"scheduled_draw_at": gorm.Expr("LEAST(scheduled_draw_at, ?)", shortened.ScheduledDrawAt),
			"accept_at":         shortened.AcceptAt, "draw_interval": shortened.DrawInterval,
			"seal_seconds": shortened.SealSeconds,
		}).Error; err != nil {
			return nil, err
		}
		if err := db.First(&stored, stored.ID).Error; err != nil {
			return nil, err
		}
	}
	// Compute from the frozen instants, including if concurrent updates raced.
	stored.SealSeconds = int(stored.ScheduledDrawAt.Sub(stored.SealAt) / time.Second)
	return &stored, nil
}

func windowStatus(window *lottery.IssueWindow, now time.Time) string {
	if window == nil || window.ScheduledDrawAt.IsZero() {
		return lottery.IssueStatusAwaiting
	}
	if !now.Before(window.ScheduledDrawAt) {
		return lottery.IssueStatusAwaiting
	}
	if !now.Before(window.SealAt) {
		return lottery.IssueStatusSealed
	}
	if now.Before(window.AcceptAt) {
		return lottery.IssueStatusPending
	}
	return lottery.IssueStatusAccepting
}

func ensureWorkspaceIssueOpen(db *gorm.DB, workspaceID uint64, game *lottery.Game, row *lottery.Issue) error {
	if !sharedIssueOpen(row, time.Now().UTC()) {
		return apperrors.NewBusinessError("ISSUE_CLOSED", "当前期已封盘，请等待下一期")
	}
	raw, actualWorkspaceID, err := readTimingSettings(db, workspaceID)
	if err != nil {
		return err
	}
	window, err := ensureIssueWindow(db, actualWorkspaceID, game, row.Issue, *row.ScheduledDrawAt, raw)
	if err != nil {
		return err
	}
	if windowStatus(window, time.Now().UTC()) != lottery.IssueStatusAccepting {
		return apperrors.NewBusinessError("ISSUE_CLOSED", "当前期已封盘，请等待下一期")
	}
	return nil
}

// The shared issue lock protects settlement, not one particular room's cutoff.
// A platform marked sealed at T-30 must not close a room configured for T-10.
// Conversely, no room may accept at/after the fixed shared draw boundary.
func sharedIssueOpen(row *lottery.Issue, now time.Time) bool {
	if row == nil || row.ScheduledDrawAt == nil || row.ScheduledDrawAt.IsZero() || row.DrawAt != nil ||
		!now.Before(*row.ScheduledDrawAt) {
		return false
	}
	switch row.Status {
	case lottery.IssueStatusPending, lottery.IssueStatusAccepting, lottery.IssueStatusSealed, lottery.IssueStatusAwaiting:
		return true
	default:
		return false
	}
}

func checkWorkspaceIssueWindow(db *gorm.DB, workspaceID uint64, game *lottery.Game, issue string) error {
	var row lottery.Issue
	if err := db.Where("game_id = ? AND issue = ?", game.ID, issue).First(&row).Error; err != nil {
		return err
	}
	return ensureWorkspaceIssueOpen(db, workspaceID, game, &row)
}

// Observe only consecutive, numeric issues and require three agreeing
// intervals. A skipped response, midnight reset or a fixture must not turn
// into a fabricated cadence. Restrict this to high-frequency schedules.
func observedDrawInterval(draws []sourceDraw) int {
	ordered := append([]sourceDraw(nil), draws...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].DrawAt.After(ordered[j].DrawAt) })
	counts := map[int]int{}
	checked := 0
	for i := 0; i+1 < len(ordered) && checked < 12; i++ {
		later, earlier := ordered[i], ordered[i+1]
		if later.Issue == earlier.Issue || later.DrawAt.IsZero() || earlier.DrawAt.IsZero() {
			continue
		}
		checked++
		laterIssue, errLater := strconv.ParseUint(strings.TrimSpace(later.Issue), 10, 64)
		earlierIssue, errEarlier := strconv.ParseUint(strings.TrimSpace(earlier.Issue), 10, 64)
		if errLater != nil || errEarlier != nil || laterIssue <= earlierIssue || laterIssue-earlierIssue != 1 {
			continue
		}
		delta := later.DrawAt.Sub(earlier.DrawAt)
		if delta < 10*time.Second || delta > 6*time.Hour || delta%time.Second != 0 {
			continue
		}
		counts[int(delta/time.Second)]++
	}
	best, count := 0, 0
	for interval, observations := range counts {
		if observations > count || (observations == count && interval < best) {
			best, count = interval, observations
		}
	}
	if count < 3 {
		return 0
	}
	return best
}

type sourceSchedule struct {
	Issue    string
	DrawAt   time.Time
	Interval int
	Source   string
}

func scheduleFromDraws(game lottery.Game, draws []sourceDraw) (sourceSchedule, error) {
	var latest sourceDraw
	for _, draw := range draws {
		if latest.DrawAt.IsZero() || draw.DrawAt.After(latest.DrawAt) {
			latest = draw
		}
	}
	if latest.DrawAt.IsZero() || strings.TrimSpace(latest.Issue) == "" {
		return sourceSchedule{}, fmt.Errorf("未取得可验证的开奖时间")
	}
	schedule := sourceSchedule{Interval: effectiveDrawInterval(&game), Source: "configured"}
	observedInterval := observedDrawInterval(draws)
	if interval := observedInterval; interval > 0 {
		schedule.Interval, schedule.Source = interval, "observed"
	}
	if validNextSourceIssue(latest.Issue, latest.NextIssue) && latest.NextDrawAt.After(latest.DrawAt) {
		schedule.Issue, schedule.DrawAt, schedule.Source = latest.NextIssue, latest.NextDrawAt.UTC(), "upstream"
		// If history is too short, one explicit next-period boundary is still
		// more authoritative than a development seed's arbitrary 180 seconds.
		if observedInterval == 0 {
			delta := latest.NextDrawAt.Sub(latest.DrawAt)
			if delta >= 10*time.Second && delta <= 6*time.Hour && delta%time.Second == 0 {
				schedule.Interval = int(delta / time.Second)
			}
		}
		return schedule, nil
	}
	schedule.DrawAt = latest.DrawAt.UTC().Add(time.Duration(schedule.Interval) * time.Second)
	schedule.Issue = inferredNextSourceIssue(latest.Issue, schedule.DrawAt)
	return schedule, nil
}

func inferredNextSourceIssue(issue string, nextAt time.Time) string {
	issue = strings.TrimSpace(issue)
	if value, err := strconv.ParseUint(issue, 10, 64); err != nil || value == 0 || value == ^uint64(0) || nextAt.IsZero() {
		return "" // Development fixtures and arbitrary text are not live periods.
	}
	if len(issue) >= 11 {
		if day, err := time.Parse("20060102", issue[:8]); err == nil {
			nextDay := nextAt.In(time.FixedZone("CST", 8*3600)).Format("20060102")
			if day.Format("20060102") != nextDay {
				return "" // A daily reset needs the upstream's explicit drawIssue.
			}
		}
	}
	return nextIssue(issue)
}

func validNextSourceIssue(current, next string) bool {
	currentNumber, currentErr := strconv.ParseUint(strings.TrimSpace(current), 10, 64)
	nextNumber, nextErr := strconv.ParseUint(strings.TrimSpace(next), 10, 64)
	return currentErr == nil && nextErr == nil && currentNumber > 0 && nextNumber > currentNumber
}

func initialPlatformIssue(drawAt time.Time) string {
	if drawAt.IsZero() {
		return ""
	}
	// Platform-operated games may bootstrap their own period identity, but it
	// is tied to the scheduled event, not the minute when a reader arrives.
	return drawAt.In(time.FixedZone("CST", 8*3600)).Format("20060102150405")
}
