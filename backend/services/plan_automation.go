package services

import (
	"backend/data/models/chat"
	"backend/data/models/lottery"
	"backend/data/models/plan"
	apperrors "backend/errors"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const PlanDemoNotice = "系统自动生成，仅供娱乐参考，不保证命中。"

type PlanDemoMaster struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Title     string `json:"title"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
}

var planDemoMasters = []PlanDemoMaster{
	{Key: "demo-qingyun", Name: "1号专家", Title: "系统自动推荐", Color: "#2aa9b3", SortOrder: 10},
	{Key: "demo-beidou", Name: "2号专家", Title: "系统自动推荐", Color: "#6e70df", SortOrder: 20},
	{Key: "demo-jinli", Name: "3号专家", Title: "系统自动推荐", Color: "#e58b45", SortOrder: 30},
}

var planDemoCategories = []string{"赛车", "飞艇", "幸运10", "时时彩", "幸运5", "PC"}

type PlanAutomationInput struct {
	WorkspaceID uint64   `json:"workspace_id"`
	Enabled     *bool    `json:"enabled"`
	Mode        string   `json:"mode"`
	GameIDs     []string `json:"game_ids"`
	Positions   []int    `json:"positions"`
	PlanKeys    []string `json:"plan_keys"`
}

type PlanAutomationView struct {
	WorkspaceID         uint64           `json:"workspace_id"`
	Enabled             bool             `json:"enabled"`
	Mode                string           `json:"mode"`
	GameIDs             []string         `json:"game_ids"`
	Masters             []PlanDemoMaster `json:"masters"`
	SupportedCategories []string         `json:"supported_categories"`
	Notice              string           `json:"notice"`
	LastRunAt           *time.Time       `json:"last_run_at"`
	LastCreatedCount    int64            `json:"last_created_count"`
	LastError           string           `json:"last_error"`
	UpdatedAt           *time.Time       `json:"updated_at"`
	Positions           []int            `json:"positions"`
	PlanKeys            []string         `json:"plan_keys"`
	Options             []PlanOption     `json:"options"`
	AvailablePositions  []PlanPosition   `json:"available_positions"`
	MaxActiveStreams    int              `json:"max_active_streams"`
	GenerationMode      string           `json:"generation_mode"`
	StreamTTLSeconds    int              `json:"stream_ttl_seconds"`
	HistoryDefault      int              `json:"history_default_periods"`
	HistoryMax          int              `json:"history_max_periods"`
	HistoryRetention    int              `json:"history_retention_periods"`
}

type PlanAutomationRun struct {
	WorkspaceID       uint64    `json:"workspace_id"`
	CreatedCount      int64     `json:"created_count"`
	EligibleGameCount int       `json:"eligible_game_count"`
	SkippedGameIDs    []string  `json:"skipped_game_ids"`
	RanAt             time.Time `json:"ran_at"`
	Notice            string    `json:"notice"`
}

type PlanAutomationService struct{ db *gorm.DB }

func NewPlanAutomationService(db *gorm.DB) *PlanAutomationService {
	return &PlanAutomationService{db: db}
}

func planAutomationView(row plan.Automation) (PlanAutomationView, error) {
	view := PlanAutomationView{
		WorkspaceID: row.WorkspaceID, Enabled: row.Enabled, Mode: "demo", GameIDs: []string{},
		Masters: append([]PlanDemoMaster{}, planDemoMasters...), SupportedCategories: append([]string{}, planDemoCategories...),
		Notice: PlanDemoNotice, LastRunAt: row.LastRunAt, LastCreatedCount: row.LastCreatedCount, LastError: strings.ReplaceAll(row.LastError, "演示", "自动"),
	}
	if row.GameIDsJSON != "" {
		if err := json.Unmarshal([]byte(row.GameIDsJSON), &view.GameIDs); err != nil || view.GameIDs == nil {
			return view, fmt.Errorf("自动推荐彩种配置损坏，请重新保存")
		}
	}
	if !row.UpdatedAt.IsZero() {
		view.UpdatedAt = &row.UpdatedAt
	}
	var err error
	view.Positions, view.PlanKeys, err = decodePlanMatrix(row)
	if err != nil {
		return view, err
	}
	view.Options, view.AvailablePositions, view.MaxActiveStreams = append([]PlanOption{}, racingPlanOptions...), planPositions(), MaxActivePlanStreams
	view.GenerationMode, view.StreamTTLSeconds = "on_visit", 60
	view.HistoryDefault, view.HistoryMax, view.HistoryRetention = 6, 10, 20
	return view, nil
}

func (s *PlanAutomationService) Get(workspaceID uint64) (PlanAutomationView, error) {
	if err := NewPlanContentService(s.db).ensureScope(workspaceID, ""); err != nil {
		return PlanAutomationView{}, err
	}
	row := plan.Automation{WorkspaceID: workspaceID}
	if err := s.db.First(&row, "workspace_id = ?", workspaceID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return PlanAutomationView{}, err
	}
	return planAutomationView(row)
}

func normalizePlanAutomationGames(values []string) ([]string, error) {
	if len(values) > 60 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "自动推荐最多选择 60 个彩种")
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 40 {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "彩种编号不正确")
		}
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result, nil
}

func (s *PlanAutomationService) Save(workspaceID uint64, input PlanAutomationInput) (PlanAutomationView, error) {
	if input.Enabled == nil || (input.Mode != "" && input.Mode != "demo") {
		return PlanAutomationView{}, apperrors.NewBusinessError("INVALID_REQUEST", "请明确选择开关；自动推荐配置不正确")
	}
	if err := NewPlanContentService(s.db).ensureScope(workspaceID, ""); err != nil {
		return PlanAutomationView{}, err
	}
	var saved plan.Automation
	err := s.db.Transaction(func(tx *gorm.DB) error {
		initialKeys, _ := json.Marshal(defaultPlanKeys())
		initial := plan.Automation{WorkspaceID: workspaceID, Mode: "demo", GameIDsJSON: "[]", PlanKeysJSON: string(initialKeys)}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&initial).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&saved, "workspace_id = ?", workspaceID).Error; err != nil {
			return err
		}
		gameIDs := input.GameIDs
		positions, keys, err := decodePlanMatrix(saved)
		if err != nil {
			return err
		}
		if input.Positions != nil {
			positions = input.Positions
		}
		if input.PlanKeys != nil {
			keys = input.PlanKeys
		}
		positions, keys, err = normalizePlanMatrix(positions, keys)
		if err != nil {
			return err
		}
		if gameIDs == nil {
			if err := json.Unmarshal([]byte(saved.GameIDsJSON), &gameIDs); err != nil {
				return err
			}
		}
		gameIDs, err = normalizePlanAutomationGames(gameIDs)
		if err != nil {
			return err
		}
		if *input.Enabled && len(gameIDs) == 0 {
			return apperrors.NewBusinessError("INVALID_REQUEST", "开启自动推荐前请至少选择一个彩种")
		}
		for _, gameID := range gameIDs {
			if !*input.Enabled {
				break // A removed/reclassified game must never prevent stopping the worker.
			}
			var game lottery.Game
			if err := tx.First(&game, "id = ?", gameID).Error; err != nil {
				return apperrors.NewBusinessError("INVALID_REQUEST", "所选彩种不存在")
			}
			if _, _, ok := planDemoNumberRange(game); !ok {
				return apperrors.NewBusinessError("INVALID_REQUEST", "所选彩种暂不支持自动推荐")
			}
		}
		raw, _ := json.Marshal(gameIDs)
		positionsRaw, _ := json.Marshal(positions)
		keysRaw, _ := json.Marshal(keys)
		if err := tx.Model(&saved).Updates(map[string]any{"enabled": *input.Enabled, "mode": "demo", "game_ids_json": string(raw), "positions_json": string(positionsRaw), "plan_keys_json": string(keysRaw), "last_error": ""}).Error; err != nil {
			return err
		}
		if err := tx.First(&saved, "workspace_id = ?", workspaceID).Error; err != nil {
			return err
		}
		updated, err := planAutomationView(saved)
		if err != nil {
			return err
		}
		return revokeDisallowedPlanStreams(tx, workspaceID, updated)
	})
	if err != nil {
		return PlanAutomationView{}, err
	}
	return planAutomationView(saved)
}

func planDemoNumberRange(game lottery.Game) (int, int, bool) {
	if game.ID == "speed-racing" {
		return 1, 10, true
	}
	switch strings.TrimSpace(game.Category) {
	case "赛车", "飞艇", "幸运10":
		return 1, 10, true
	case "时时彩", "幸运5":
		return 0, 9, true
	case "PC":
		return 0, 27, true
	default:
		return 0, 0, false
	}
}

// Keys and the plan-demo-v1 salt are compatibility identities: changing labels
// must not regenerate an already published issue or change other games' picks.
// This is a display fixture, not an analysis of draws. It only depends on the
// room, advertised issue, game and template identity; it never reads outcomes.
func planDemoNumbers(workspaceID uint64, game lottery.Game, issue, masterKey string) ([]int, error) {
	minimum, maximum, supported := planDemoNumberRange(game)
	if !supported {
		return nil, fmt.Errorf("不支持的自动推荐彩种")
	}
	pool := make([]int, maximum-minimum+1)
	for i := range pool {
		pool[i] = minimum + i
	}
	count := 3
	if game.ID == "speed-racing" {
		count = 5
	}
	for i := 0; i < count; i++ {
		digest := sha256.Sum256([]byte(fmt.Sprintf("plan-demo-v1\x00%d\x00%s\x00%s\x00%s\x00%d", workspaceID, game.ID, issue, masterKey, i)))
		j := i + int(binary.BigEndian.Uint64(digest[:8])%uint64(len(pool)-i))
		pool[i], pool[j] = pool[j], pool[i]
	}
	result := append([]int{}, pool[:count]...)
	sort.Ints(result)
	return result, nil
}

func planAutomationIssueEligible(game lottery.Game, issue lottery.Issue, now time.Time) bool {
	return (game.SourceKind == "external" || game.SourceKind == "official") && game.TimingSource == "upstream" &&
		game.SyncStatus != "error" && !(game.SyncStatus == "syncing" && game.LastSyncError != "") &&
		strings.TrimSpace(game.NextIssue) != "" && game.NextIssue == issue.Issue && game.ID == issue.GameID &&
		issue.SourceMode == "external" && issue.Status == lottery.IssueStatusAccepting && issue.DrawAt == nil &&
		!issue.AcceptAt.IsZero() && !now.Before(issue.AcceptAt) && now.Before(issue.SealAt) &&
		issue.ScheduledDrawAt != nil && now.Before(*issue.ScheduledDrawAt) && now.Before(game.NextDrawAt)
}

// Kept for old callers only. Generation requires a member visit selecting one
// game/stream; an administrator or a scheduler cannot publish an entire room.
func (s *PlanAutomationService) RunWorkspace(ctx context.Context, workspaceID uint64) (PlanAutomationRun, error) {
	return PlanAutomationRun{}, apperrors.NewBusinessError("PLAN_VISIT_REQUIRED", "计划仅在会员打开对应页面时按需生成")
}

func generatePlanDemoGame(tx *gorm.DB, workspaceID uint64, gameID, rawSettings string, stream plan.Stream) (int64, bool, error) {
	var roomGame chat.RoomGameSetting
	err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&roomGame, "workspace_id = ? AND game_id = ?", workspaceID, gameID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && !roomGame.Enabled) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	var game lottery.Game
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&game, "id = ?", gameID).Error; err != nil {
		return 0, false, err
	}
	if _, _, supported := planDemoNumberRange(game); !supported || !game.Enabled || strings.TrimSpace(game.LobbyCategory) == "" || game.NextIssue == "" {
		return 0, false, nil
	}
	var issue lottery.Issue
	err = tx.Clauses(clause.Locking{Strength: "SHARE"}).
		Where("NOT EXISTS (SELECT 1 FROM lottery_draws AS published_draw WHERE published_draw.game_id = lottery_issues.game_id AND published_draw.issue = lottery_issues.issue)").
		First(&issue, "game_id = ? AND issue = ?", gameID, game.NextIssue).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && !planAutomationIssueEligible(game, issue, time.Now().UTC())) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	window, err := ensureIssueWindow(tx, workspaceID, &game, issue.Issue, *issue.ScheduledDrawAt, rawSettings)
	if err != nil {
		return 0, false, err
	}
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(window, window.ID).Error; err != nil {
		return 0, false, err
	}
	if windowStatus(window, time.Now().UTC()) != lottery.IssueStatusAccepting {
		return 0, false, nil
	}
	if gameID == "speed-racing" {
		created, err := advancePlanStream(tx, workspaceID, game, issue, window, stream)
		return created, true, err
	}
	var created int64
	for _, master := range planDemoMasters {
		numbers, err := planDemoNumbers(workspaceID, game, issue.Issue, master.Key)
		if err != nil {
			return 0, false, err
		}
		rawNumbers, _ := canonicalPlanNumbers(numbers)
		// clock_timestamp (not transaction-start CURRENT_TIMESTAMP) prevents a
		// transaction delayed by locks from publishing after either cutoff.
		insert := tx.Exec(`WITH claimed AS (
			INSERT INTO plan_generation_receipts (workspace_id, game_id, issue, master_key, created_at)
			SELECT ?, ?, ?, ?, clock_timestamp()
			WHERE clock_timestamp() >= ? AND clock_timestamp() < ? AND clock_timestamp() < ?
			  AND NOT EXISTS (SELECT 1 FROM lottery_draws WHERE game_id = ? AND issue = ?)
			ON CONFLICT DO NOTHING RETURNING id
		) INSERT INTO plan_recommendations
			(workspace_id, game_id, issue, master_name, master_title, master_color, numbers,
			 size, parity, result, source, note, enabled, sort_order, created_at, updated_at)
		SELECT ?, ?, ?, ?, ?, ?, ?, '', '', 'pending', 'demo', ?, true, ?, clock_timestamp(), clock_timestamp()
		FROM claimed WHERE clock_timestamp() < ? AND clock_timestamp() < ? ON CONFLICT DO NOTHING`,
			workspaceID, gameID, issue.Issue, master.Key, window.AcceptAt, window.SealAt, issue.SealAt, gameID, issue.Issue,
			workspaceID, gameID, issue.Issue, master.Name, master.Title, master.Color, rawNumbers, PlanDemoNotice, master.SortOrder, window.SealAt, issue.SealAt)
		if insert.Error != nil {
			return 0, false, insert.Error
		}
		created += insert.RowsAffected
	}
	return created, true, nil
}
