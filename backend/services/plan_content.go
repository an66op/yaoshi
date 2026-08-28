package services

import (
	"backend/data/models/lottery"
	"backend/data/models/plan"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PlanRecommendationInput struct {
	WorkspaceID uint64 `json:"workspace_id"`
	GameID      string `json:"game_id"`
	Issue       string `json:"issue"`
	MasterName  string `json:"master_name"`
	MasterTitle string `json:"master_title"`
	MasterColor string `json:"master_color"`
	Numbers     []int  `json:"numbers"`
	Size        string `json:"size"`
	Parity      string `json:"parity"`
	Result      string `json:"result"`
	Note        string `json:"note"`
	Enabled     bool   `json:"enabled"`
	SortOrder   int    `json:"sort_order"`
}

type PlanRecommendationView struct {
	ID            uint64    `json:"id"`
	WorkspaceID   uint64    `json:"workspace_id"`
	GameID        string    `json:"game_id"`
	Issue         string    `json:"issue"`
	MasterName    string    `json:"master_name"`
	MasterTitle   string    `json:"master_title"`
	MasterColor   string    `json:"master_color"`
	Numbers       []int     `json:"numbers"`
	Size          string    `json:"size"`
	Parity        string    `json:"parity"`
	Result        string    `json:"result"`
	Note          string    `json:"note"`
	Enabled       bool      `json:"enabled"`
	SortOrder     int       `json:"sort_order"`
	MasterHitRate *float64  `json:"master_hit_rate"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type PlanGameSummary struct {
	GameID       string    `json:"game_id"`
	CurrentIssue string    `json:"current_issue"`
	MasterCount  int       `json:"master_count"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type PlanDetail struct {
	GameID          string                   `json:"game_id"`
	CurrentIssue    string                   `json:"current_issue"`
	Recommendations []PlanRecommendationView `json:"recommendations"`
	History         []PlanRecommendationView `json:"history"`
}

type PlanContentService struct{ db *gorm.DB }

func NewPlanContentService(db *gorm.DB) *PlanContentService { return &PlanContentService{db: db} }

type defaultPlanTemplate struct {
	GameID, MasterName, MasterTitle, MasterColor, Numbers, Size, Parity string
	SortOrder                                                           int
}

var debugPlanTemplates = []defaultPlanTemplate{
	{GameID: "speed-racing", MasterName: "青云老师", MasterTitle: "综合趋势", MasterColor: "#2aa9b3", Numbers: "1,5,9", Size: "大", Parity: "单", SortOrder: 10},
	{GameID: "speed-racing", MasterName: "北斗数据师", MasterTitle: "冷热分析", MasterColor: "#6e70df", Numbers: "2,6,10", Size: "小", Parity: "双", SortOrder: 20},
	{GameID: "speed-racing", MasterName: "锦鲤计划师", MasterTitle: "节奏追踪", MasterColor: "#e58b45", Numbers: "3,4,8", Size: "大", Parity: "双", SortOrder: 30},
	{GameID: "canada-28", MasterName: "青云老师", MasterTitle: "综合趋势", MasterColor: "#2aa9b3", Numbers: "3,14,22", Size: "大", Parity: "单", SortOrder: 10},
	{GameID: "canada-28", MasterName: "北斗数据师", MasterTitle: "冷热分析", MasterColor: "#6e70df", Numbers: "6,11,19", Size: "小", Parity: "双", SortOrder: 20},
	{GameID: "canada-28", MasterName: "锦鲤计划师", MasterTitle: "节奏追踪", MasterColor: "#e58b45", Numbers: "8,17,25", Size: "大", Parity: "双", SortOrder: 30},
	{GameID: "au-lucky-10", MasterName: "青云老师", MasterTitle: "综合趋势", MasterColor: "#2aa9b3", Numbers: "1,4,8", Size: "大", Parity: "单", SortOrder: 10},
	{GameID: "au-lucky-10", MasterName: "北斗数据师", MasterTitle: "冷热分析", MasterColor: "#6e70df", Numbers: "2,5,9", Size: "小", Parity: "双", SortOrder: 20},
	{GameID: "au-lucky-10", MasterName: "锦鲤计划师", MasterTitle: "节奏追踪", MasterColor: "#e58b45", Numbers: "3,6,10", Size: "大", Parity: "双", SortOrder: 30},
}

// SeedDebugPlanRecommendations creates local editorial fixtures only for a
// persisted issue which is genuinely accepting bets. It is intentionally not
// called by release bootstrap and never fabricates an issue or a result.
func SeedDebugPlanRecommendations(db *gorm.DB) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("数据库连接不可用")
	}
	gameIssues := make(map[string]string)
	bets := NewBetAdminService(db)
	for _, template := range debugPlanTemplates {
		if _, exists := gameIssues[template.GameID]; exists {
			continue
		}
		var game lottery.Game
		if err := db.Where("id = ? AND enabled = ?", template.GameID, true).First(&game).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				continue
			}
			return 0, err
		}
		issue, err := bets.EnsureCurrentIssue(&game)
		if err != nil {
			return 0, err
		}
		if issue.Status == lottery.IssueStatusAccepting && time.Now().UTC().Before(issue.SealAt.UTC()) {
			gameIssues[template.GameID] = issue.Issue
		}
	}

	var workspaces []workspacemodel.Workspace
	if err := db.Where("type IN ? AND status = ? AND room_code <> ''", []string{workspacemodel.TypeTenant, workspacemodel.TypeAgent}, 1).
		Order("id ASC").Find(&workspaces).Error; err != nil {
		return 0, err
	}
	var created int64
	for _, workspace := range workspaces {
		for _, template := range debugPlanTemplates {
			issue := gameIssues[template.GameID]
			if issue == "" {
				continue
			}
			row := plan.Recommendation{
				WorkspaceID: workspace.ID, GameID: template.GameID, Issue: issue,
				MasterName: template.MasterName, MasterTitle: template.MasterTitle, MasterColor: template.MasterColor,
				Numbers: template.Numbers, Size: template.Size, Parity: template.Parity,
				Result: plan.ResultPending, Enabled: true, SortOrder: template.SortOrder,
			}
			result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
			if result.Error != nil {
				return created, result.Error
			}
			created += result.RowsAffected
		}
	}
	return created, nil
}

func parsePlanNumbers(raw string) []int {
	result := make([]int, 0)
	for _, part := range strings.Split(raw, ",") {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err == nil {
			result = append(result, value)
		}
	}
	return result
}

func canonicalPlanNumbers(values []int) (string, error) {
	if len(values) == 0 || len(values) > 12 {
		return "", apperrors.NewBusinessError("INVALID_REQUEST", "推荐号码需要填写 1 至 12 个")
	}
	seen := make(map[int]struct{}, len(values))
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if value < 0 || value > 99 {
			return "", apperrors.NewBusinessError("INVALID_REQUEST", "推荐号码范围不正确")
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		parts = append(parts, strconv.Itoa(value))
	}
	return strings.Join(parts, ","), nil
}

func validatePlanInput(input PlanRecommendationInput) (plan.Recommendation, error) {
	numbers, err := canonicalPlanNumbers(input.Numbers)
	if err != nil {
		return plan.Recommendation{}, err
	}
	row := plan.Recommendation{
		GameID: strings.TrimSpace(input.GameID), Issue: strings.TrimSpace(input.Issue),
		MasterName: strings.TrimSpace(input.MasterName), MasterTitle: strings.TrimSpace(input.MasterTitle),
		MasterColor: strings.TrimSpace(input.MasterColor), Numbers: numbers,
		Size: strings.TrimSpace(input.Size), Parity: strings.TrimSpace(input.Parity),
		Result: strings.TrimSpace(input.Result), Note: strings.TrimSpace(input.Note),
		Enabled: input.Enabled, SortOrder: input.SortOrder,
	}
	if row.GameID == "" || row.Issue == "" || row.MasterName == "" {
		return plan.Recommendation{}, apperrors.NewBusinessError("INVALID_REQUEST", "彩种、期号和大师名称不能为空")
	}
	if row.MasterColor == "" {
		row.MasterColor = "#2aa9b3"
	}
	if row.Size != "" && row.Size != "大" && row.Size != "小" {
		return plan.Recommendation{}, apperrors.NewBusinessError("INVALID_REQUEST", "大小推荐只能填写大或小")
	}
	if row.Parity != "" && row.Parity != "单" && row.Parity != "双" {
		return plan.Recommendation{}, apperrors.NewBusinessError("INVALID_REQUEST", "单双推荐只能填写单或双")
	}
	if row.Result == "" {
		row.Result = plan.ResultPending
	}
	if row.Result != plan.ResultPending && row.Result != plan.ResultHit && row.Result != plan.ResultMiss {
		return plan.Recommendation{}, apperrors.NewBusinessError("INVALID_REQUEST", "推荐结果不正确")
	}
	return row, nil
}

func (s *PlanContentService) ensureScope(workspaceID uint64, gameID string) error {
	if workspaceID == 0 {
		return apperrors.NewBusinessError("INVALID_REQUEST", "请选择房间")
	}
	var workspaceCount int64
	if err := s.db.Model(&workspacemodel.Workspace{}).Where("id = ? AND status = ? AND type IN ?", workspaceID, 1, []string{workspacemodel.TypeTenant, workspacemodel.TypeAgent}).Count(&workspaceCount).Error; err != nil {
		return err
	}
	if workspaceCount != 1 {
		return apperrors.NewBusinessError("ROOM_NOT_FOUND", "房间不存在或已停用")
	}
	if strings.TrimSpace(gameID) != "" {
		var gameCount int64
		if err := s.db.Model(&lottery.Game{}).Where("id = ?", strings.TrimSpace(gameID)).Count(&gameCount).Error; err != nil {
			return err
		}
		if gameCount != 1 {
			return apperrors.NewBusinessError("GAME_NOT_FOUND", "彩种不存在")
		}
	}
	return nil
}

func (s *PlanContentService) ensureIssue(gameID, issue string) error {
	var count int64
	if err := s.db.Model(&lottery.Issue{}).
		Where("game_id = ? AND issue = ?", strings.TrimSpace(gameID), strings.TrimSpace(issue)).
		Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return apperrors.NewBusinessError("NOT_FOUND", "该彩种期号不存在")
	}
	return nil
}

func planHitRates(rows []plan.Recommendation) map[string]*float64 {
	type score struct{ hits, settled int }
	scores := map[string]score{}
	for _, row := range rows {
		key := row.GameID + "\x00" + row.MasterName
		value := scores[key]
		switch row.Result {
		case plan.ResultHit:
			value.hits++
			value.settled++
		case plan.ResultMiss:
			value.settled++
		}
		scores[key] = value
	}
	result := make(map[string]*float64, len(scores))
	for key, value := range scores {
		if value.settled == 0 {
			result[key] = nil
			continue
		}
		rate := float64(value.hits) * 100 / float64(value.settled)
		result[key] = &rate
	}
	return result
}

func planView(row plan.Recommendation, rates map[string]*float64) PlanRecommendationView {
	return PlanRecommendationView{
		ID: row.ID, WorkspaceID: row.WorkspaceID, GameID: row.GameID, Issue: row.Issue,
		MasterName: row.MasterName, MasterTitle: row.MasterTitle, MasterColor: row.MasterColor,
		Numbers: parsePlanNumbers(row.Numbers), Size: row.Size, Parity: row.Parity,
		Result: row.Result, Note: row.Note, Enabled: row.Enabled, SortOrder: row.SortOrder,
		MasterHitRate: rates[row.GameID+"\x00"+row.MasterName], CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

// ListAdmin returns room-scoped rows, including disabled content.  The caller
// must provide the authenticated room workspace; no browser-selected fallback
// to a global workspace is allowed.
func (s *PlanContentService) ListAdmin(workspaceID uint64) ([]PlanRecommendationView, error) {
	if err := s.ensureScope(workspaceID, ""); err != nil {
		return nil, err
	}
	var rows []plan.Recommendation
	if err := s.db.Where("workspace_id = ?", workspaceID).Order("game_id, issue DESC, sort_order, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	rates := planHitRates(rows)
	result := make([]PlanRecommendationView, 0, len(rows))
	for _, row := range rows {
		result = append(result, planView(row, rates))
	}
	return result, nil
}

func (s *PlanContentService) Catalog(workspaceID uint64) ([]PlanGameSummary, error) {
	if err := s.ensureScope(workspaceID, ""); err != nil {
		return nil, err
	}
	var rows []plan.Recommendation
	if err := s.db.
		Joins("JOIN lottery_issues AS current_issue ON current_issue.game_id = plan_recommendations.game_id AND current_issue.issue = plan_recommendations.issue AND current_issue.status = ?", lottery.IssueStatusAccepting).
		Where("plan_recommendations.workspace_id = ? AND plan_recommendations.enabled = ?", workspaceID, true).
		Order("plan_recommendations.updated_at DESC, plan_recommendations.id DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	type group struct {
		issue   string
		updated time.Time
		masters map[string]struct{}
	}
	groups := map[string]*group{}
	for _, row := range rows {
		item, exists := groups[row.GameID]
		if !exists {
			item = &group{issue: row.Issue, updated: row.UpdatedAt, masters: map[string]struct{}{}}
			groups[row.GameID] = item
		}
		if row.Issue == item.issue {
			item.masters[row.MasterName] = struct{}{}
		}
	}
	result := make([]PlanGameSummary, 0, len(groups))
	for gameID, item := range groups {
		result = append(result, PlanGameSummary{GameID: gameID, CurrentIssue: item.issue, MasterCount: len(item.masters), UpdatedAt: item.updated})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	return result, nil
}

func (s *PlanContentService) Detail(workspaceID uint64, gameID string) (PlanDetail, error) {
	if err := s.ensureScope(workspaceID, gameID); err != nil {
		return PlanDetail{}, err
	}
	result := PlanDetail{GameID: strings.TrimSpace(gameID), Recommendations: []PlanRecommendationView{}, History: []PlanRecommendationView{}}
	var currentIssue lottery.Issue
	issueError := s.db.Where("game_id = ? AND status = ?", result.GameID, lottery.IssueStatusAccepting).
		Order("seal_at DESC, id DESC").First(&currentIssue).Error
	if issueError != nil && issueError != gorm.ErrRecordNotFound {
		return result, issueError
	}
	if issueError == nil {
		result.CurrentIssue = currentIssue.Issue
	}
	var rows []plan.Recommendation
	if err := s.db.Where("workspace_id = ? AND game_id = ? AND enabled = ?", workspaceID, result.GameID, true).Order("updated_at DESC, sort_order, id").Limit(300).Find(&rows).Error; err != nil {
		return result, err
	}
	if len(rows) == 0 {
		return result, nil
	}
	rates := planHitRates(rows)
	for _, row := range rows {
		view := planView(row, rates)
		if result.CurrentIssue != "" && row.Issue == result.CurrentIssue {
			result.Recommendations = append(result.Recommendations, view)
		}
		result.History = append(result.History, view)
	}
	return result, nil
}

func (s *PlanContentService) Create(workspaceID uint64, input PlanRecommendationInput) (PlanRecommendationView, error) {
	row, err := validatePlanInput(input)
	if err != nil {
		return PlanRecommendationView{}, err
	}
	if err := s.ensureScope(workspaceID, row.GameID); err != nil {
		return PlanRecommendationView{}, err
	}
	if err := s.ensureIssue(row.GameID, row.Issue); err != nil {
		return PlanRecommendationView{}, err
	}
	row.WorkspaceID = workspaceID
	if err := s.db.Create(&row).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return PlanRecommendationView{}, apperrors.NewBusinessError("INVALID_REQUEST", "该期该大师的推荐已经存在")
		}
		return PlanRecommendationView{}, err
	}
	return planView(row, planHitRates([]plan.Recommendation{row})), nil
}

func (s *PlanContentService) Update(workspaceID, id uint64, input PlanRecommendationInput) (PlanRecommendationView, error) {
	patch, err := validatePlanInput(input)
	if err != nil {
		return PlanRecommendationView{}, err
	}
	if err := s.ensureScope(workspaceID, patch.GameID); err != nil {
		return PlanRecommendationView{}, err
	}
	if err := s.ensureIssue(patch.GameID, patch.Issue); err != nil {
		return PlanRecommendationView{}, err
	}
	var row plan.Recommendation
	if err := s.db.Where("id = ? AND workspace_id = ?", id, workspaceID).First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return PlanRecommendationView{}, apperrors.NewBusinessError("NOT_FOUND", "推荐不存在")
		}
		return PlanRecommendationView{}, err
	}
	updates := map[string]any{
		"game_id": patch.GameID, "issue": patch.Issue, "master_name": patch.MasterName,
		"master_title": patch.MasterTitle, "master_color": patch.MasterColor, "numbers": patch.Numbers,
		"size": patch.Size, "parity": patch.Parity, "result": patch.Result, "note": patch.Note,
		"enabled": patch.Enabled, "sort_order": patch.SortOrder,
	}
	if err := s.db.Model(&row).Updates(updates).Error; err != nil {
		return PlanRecommendationView{}, fmt.Errorf("update recommendation: %w", err)
	}
	if err := s.db.First(&row, id).Error; err != nil {
		return PlanRecommendationView{}, err
	}
	return planView(row, planHitRates([]plan.Recommendation{row})), nil
}

func (s *PlanContentService) Delete(workspaceID, id uint64) error {
	if err := s.ensureScope(workspaceID, ""); err != nil {
		return err
	}
	result := s.db.Where("id = ? AND workspace_id = ?", id, workspaceID).Delete(&plan.Recommendation{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return apperrors.NewBusinessError("NOT_FOUND", "推荐不存在")
	}
	return nil
}
