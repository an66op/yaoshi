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
	ID              uint64     `json:"id"`
	WorkspaceID     uint64     `json:"workspace_id"`
	GameID          string     `json:"game_id"`
	Issue           string     `json:"issue"`
	MasterName      string     `json:"master_name"`
	MasterTitle     string     `json:"master_title"`
	MasterColor     string     `json:"master_color"`
	Numbers         []int      `json:"numbers"`
	Size            string     `json:"size"`
	Parity          string     `json:"parity"`
	Result          string     `json:"result"`
	Source          string     `json:"source"`
	Note            string     `json:"note"`
	Enabled         bool       `json:"enabled"`
	SortOrder       int        `json:"sort_order"`
	MasterHitRate   *float64   `json:"master_hit_rate"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	Position        int        `json:"position,omitempty"`
	PlanKey         string     `json:"plan_key,omitempty"`
	Kind            string     `json:"kind,omitempty"`
	DragonTiger     string     `json:"dragon_tiger,omitempty"`
	CycleID         uint64     `json:"cycle_id,omitempty"`
	CyclePeriod     int        `json:"cycle_period,omitempty"`
	CyclePeriods    int        `json:"cycle_periods,omitempty"`
	CycleStartIssue string     `json:"cycle_start_issue,omitempty"`
	CycleStatus     string     `json:"cycle_status,omitempty"`
	DrawNumbers     []int      `json:"draw_numbers,omitempty"`
	DrawAt          *time.Time `json:"draw_at,omitempty"`
}

type PlanGameSummary struct {
	GameID       string    `json:"game_id"`
	CurrentIssue string    `json:"current_issue"`
	LatestIssue  string    `json:"latest_issue"`
	HistoryOnly  bool      `json:"history_only"`
	MasterCount  int       `json:"master_count"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type PlanDetail struct {
	GameID                string                   `json:"game_id"`
	CurrentIssue          string                   `json:"current_issue"`
	Recommendations       []PlanRecommendationView `json:"recommendations"`
	History               []PlanRecommendationView `json:"history"`
	LatestRecommendations []PlanRecommendationView `json:"latest_recommendations"`
	GenerationMode        string                   `json:"generation_mode"`
	AutomationEnabled     bool                     `json:"automation_enabled"`
	HistoryLimit          int                      `json:"history_limit"`
	RefreshSeconds        int                      `json:"refresh_seconds"`
}

type PlanContentService struct{ db *gorm.DB }

func NewPlanContentService(db *gorm.DB) *PlanContentService { return &PlanContentService{db: db} }

type defaultPlanTemplate struct {
	GameID, MasterName, MasterTitle, MasterColor, Numbers, Size, Parity string
	SortOrder                                                           int
}

var debugPlanTemplates = []defaultPlanTemplate{
	{GameID: "speed-racing", MasterName: "1号专家", MasterTitle: "系统自动推荐", MasterColor: "#2aa9b3", Numbers: "1,3,5,7,9", SortOrder: 10},
	{GameID: "speed-racing", MasterName: "2号专家", MasterTitle: "系统自动推荐", MasterColor: "#6e70df", Numbers: "2,4,6,8,10", SortOrder: 20},
	{GameID: "speed-racing", MasterName: "3号专家", MasterTitle: "系统自动推荐", MasterColor: "#e58b45", Numbers: "1,2,4,7,10", SortOrder: 30},
	{GameID: "canada-28", MasterName: "1号专家", MasterTitle: "系统自动推荐", MasterColor: "#2aa9b3", Numbers: "3,14,22", Size: "大", Parity: "单", SortOrder: 10},
	{GameID: "canada-28", MasterName: "2号专家", MasterTitle: "系统自动推荐", MasterColor: "#6e70df", Numbers: "6,11,19", Size: "小", Parity: "双", SortOrder: 20},
	{GameID: "canada-28", MasterName: "3号专家", MasterTitle: "系统自动推荐", MasterColor: "#e58b45", Numbers: "8,17,25", Size: "大", Parity: "双", SortOrder: 30},
	{GameID: "au-lucky-10", MasterName: "1号专家", MasterTitle: "系统自动推荐", MasterColor: "#2aa9b3", Numbers: "1,4,8", Size: "大", Parity: "单", SortOrder: 10},
	{GameID: "au-lucky-10", MasterName: "2号专家", MasterTitle: "系统自动推荐", MasterColor: "#6e70df", Numbers: "2,5,9", Size: "小", Parity: "双", SortOrder: 20},
	{GameID: "au-lucky-10", MasterName: "3号专家", MasterTitle: "系统自动推荐", MasterColor: "#e58b45", Numbers: "3,6,10", Size: "大", Parity: "双", SortOrder: 30},
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
				Result: plan.ResultPending, Source: "demo", Note: PlanDemoNotice, Enabled: true, SortOrder: template.SortOrder,
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
	if strings.TrimSpace(input.GameID) == "speed-racing" {
		if len(input.Numbers) != 5 {
			return plan.Recommendation{}, apperrors.NewBusinessError("INVALID_REQUEST", "极速赛车推荐必须填写5个不重复的1至10号码")
		}
		seen := make(map[int]bool, 5)
		for _, value := range input.Numbers {
			if value < 1 || value > 10 || seen[value] {
				return plan.Recommendation{}, apperrors.NewBusinessError("INVALID_REQUEST", "极速赛车推荐必须填写5个不重复的1至10号码")
			}
			seen[value] = true
		}
		if strings.TrimSpace(input.Size) != "" || strings.TrimSpace(input.Parity) != "" {
			return plan.Recommendation{}, apperrors.NewBusinessError("INVALID_REQUEST", "极速赛车只支持号码推荐，不支持大小或单双")
		}
	}
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
		if row.Source == "demo" {
			continue
		}
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
	source := row.Source
	if source == "" {
		source = "manual"
	}
	rate := rates[row.GameID+"\x00"+row.MasterName]
	if source == "demo" {
		rate = nil
		// Presentation-only aliases keep published numbers, issue identity and
		// timestamps immutable while old and new automatic rows share a label.
		for index, previous := range []string{"青云演示师", "北斗演示师", "锦鲤演示师"} {
			if row.MasterName == previous || row.MasterName == planDemoMasters[index].Name {
				row.MasterName = planDemoMasters[index].Name
				row.MasterTitle = planDemoMasters[index].Title
				break
			}
		}
		row.MasterTitle = "系统自动推荐"
		if row.Note != PlanHistoryBackfillNotice {
			row.Note = PlanDemoNotice
		}
	}
	if row.GameID == "speed-racing" {
		row.Size, row.Parity = "", ""
	}
	return PlanRecommendationView{
		ID: row.ID, WorkspaceID: row.WorkspaceID, GameID: row.GameID, Issue: row.Issue,
		MasterName: row.MasterName, MasterTitle: row.MasterTitle, MasterColor: row.MasterColor,
		Numbers: parsePlanNumbers(row.Numbers), Size: row.Size, Parity: row.Parity,
		Result: row.Result, Source: source, Note: row.Note, Enabled: row.Enabled, SortOrder: row.SortOrder,
		MasterHitRate: rate, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

// ListAdmin returns the latest 300 room-scoped rows, including disabled content. The caller
// must provide the authenticated room workspace; no browser-selected fallback
// to a global workspace is allowed.
func (s *PlanContentService) ListAdmin(workspaceID uint64) ([]PlanRecommendationView, error) {
	if err := s.ensureScope(workspaceID, ""); err != nil {
		return nil, err
	}
	var rows []plan.Recommendation
	if err := s.db.Joins("LEFT JOIN lottery_issues AS published_issue ON published_issue.game_id = plan_recommendations.game_id AND published_issue.issue = plan_recommendations.issue").
		Joins("LEFT JOIN lottery_draws AS published_draw ON published_draw.game_id = plan_recommendations.game_id AND published_draw.issue = plan_recommendations.issue").
		Where("plan_recommendations.workspace_id = ?", workspaceID).
		Order("COALESCE(published_issue.scheduled_draw_at, published_draw.draw_at) DESC NULLS LAST, plan_recommendations.sort_order, plan_recommendations.id DESC").Limit(300).Find(&rows).Error; err != nil {
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
	// Retain a shelf for every published game without loading its unbounded
	// automatic history into the application on every catalog refresh.
	var summaries []PlanGameSummary
	if err := s.db.Raw(`WITH latest AS (
		SELECT DISTINCT ON (recommendation.game_id) recommendation.game_id, recommendation.issue
		FROM plan_recommendations AS recommendation
		LEFT JOIN lottery_issues AS published_issue ON published_issue.game_id = recommendation.game_id AND published_issue.issue = recommendation.issue
		LEFT JOIN lottery_draws AS published_draw ON published_draw.game_id = recommendation.game_id AND published_draw.issue = recommendation.issue
		WHERE recommendation.workspace_id = ? AND recommendation.enabled = true AND recommendation.deleted_at IS NULL
		ORDER BY recommendation.game_id, COALESCE(published_issue.scheduled_draw_at, published_draw.draw_at) DESC NULLS LAST, recommendation.id DESC
	) SELECT latest.game_id, latest.issue AS latest_issue, COUNT(DISTINCT recommendation.master_name) AS master_count, MAX(recommendation.updated_at) AS updated_at
	FROM latest JOIN plan_recommendations AS recommendation ON recommendation.game_id = latest.game_id AND recommendation.issue = latest.issue
	WHERE recommendation.workspace_id = ? AND recommendation.enabled = true AND recommendation.deleted_at IS NULL
	GROUP BY latest.game_id, latest.issue`, workspaceID, workspaceID).Scan(&summaries).Error; err != nil {
		return nil, err
	}
	result := make([]PlanGameSummary, 0, len(summaries))
	for _, item := range summaries {
		current, err := s.currentOpenPlanIssue(workspaceID, item.GameID)
		if err != nil {
			return nil, err
		}
		item.CurrentIssue, item.HistoryOnly = current, current == "" || current != item.LatestIssue
		result = append(result, item)
	}
	result, err := s.appendStreamCatalog(workspaceID, result)
	if err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	return result, nil
}

func (s *PlanContentService) Detail(workspaceID uint64, gameID string, historyLimits ...int) (PlanDetail, error) {
	if err := s.ensureScope(workspaceID, gameID); err != nil {
		return PlanDetail{}, err
	}
	result := PlanDetail{GameID: strings.TrimSpace(gameID), Recommendations: []PlanRecommendationView{}, History: []PlanRecommendationView{}, LatestRecommendations: []PlanRecommendationView{}}
	result.GenerationMode, result.HistoryLimit, result.RefreshSeconds = "on_visit", planHistoryLimit(historyLimits), 15
	config, err := NewPlanAutomationService(s.db).Get(workspaceID)
	if err != nil {
		return result, err
	}
	if planRequestedStreamAllowed(config, gameID, 1, singlePeriodPlanKey) {
		result.AutomationEnabled, err = planStreamRoomAvailable(s.db, workspaceID, gameID)
		if err != nil {
			return result, err
		}
	}
	result.CurrentIssue, err = s.currentOpenPlanIssue(workspaceID, result.GameID)
	if err != nil {
		return result, err
	}
	var rows []plan.Recommendation
	recentIssues := s.db.Model(&plan.Recommendation{}).Select("plan_recommendations.issue").
		Joins("LEFT JOIN lottery_issues AS published_issue ON published_issue.game_id = plan_recommendations.game_id AND published_issue.issue = plan_recommendations.issue").
		Joins("LEFT JOIN lottery_draws AS published_draw ON published_draw.game_id = plan_recommendations.game_id AND published_draw.issue = plan_recommendations.issue").
		Where("plan_recommendations.workspace_id = ? AND plan_recommendations.game_id = ? AND plan_recommendations.enabled = ?", workspaceID, result.GameID, true).
		Group("plan_recommendations.issue, published_issue.scheduled_draw_at, published_draw.draw_at").
		Order("COALESCE(published_issue.scheduled_draw_at, published_draw.draw_at) DESC NULLS LAST").Limit(result.HistoryLimit)
	if err := s.db.Joins("LEFT JOIN lottery_issues AS published_issue ON published_issue.game_id = plan_recommendations.game_id AND published_issue.issue = plan_recommendations.issue").
		Joins("LEFT JOIN lottery_draws AS published_draw ON published_draw.game_id = plan_recommendations.game_id AND published_draw.issue = plan_recommendations.issue").
		Where("plan_recommendations.workspace_id = ? AND plan_recommendations.game_id = ? AND plan_recommendations.enabled = ?", workspaceID, result.GameID, true).
		Where("plan_recommendations.issue IN (?)", recentIssues).
		Order("COALESCE(published_issue.scheduled_draw_at, published_draw.draw_at) DESC NULLS LAST, plan_recommendations.sort_order, plan_recommendations.id").Limit(300).Find(&rows).Error; err != nil {
		return result, err
	}
	if len(rows) == 0 {
		return result, nil
	}
	rates := planHitRates(rows)
	seenMasters := map[string]bool{}
	for _, row := range rows {
		view := planView(row, rates)
		masterIdentity := view.Source + "\x00" + view.MasterName
		if !seenMasters[masterIdentity] {
			seenMasters[masterIdentity] = true
			result.LatestRecommendations = append(result.LatestRecommendations, view)
		}
		if result.CurrentIssue != "" && row.Issue == result.CurrentIssue {
			result.Recommendations = append(result.Recommendations, view)
		}
		result.History = append(result.History, view)
	}
	return result, nil
}

// A stale status flag is not a current recommendation. Reads never create a
// lifecycle row or guess the next issue, and respect an earlier room cutoff.
func (s *PlanContentService) currentOpenPlanIssue(workspaceID uint64, gameID string) (string, error) {
	now := time.Now().UTC()
	var issue lottery.Issue
	err := s.db.Where("game_id = ? AND status = ? AND accept_at <= ? AND seal_at > ? AND draw_at IS NULL", gameID, lottery.IssueStatusAccepting, now, now).
		Where("NOT EXISTS (SELECT 1 FROM lottery_draws AS published_draw WHERE published_draw.game_id = lottery_issues.game_id AND published_draw.issue = lottery_issues.issue)").
		Order("seal_at DESC, id DESC").First(&issue).Error
	if err == gorm.ErrRecordNotFound {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if issue.ScheduledDrawAt != nil {
		var game lottery.Game
		if err := s.db.First(&game, "id = ?", gameID).Error; err != nil {
			return "", err
		}
		raw, _, err := readTimingSettings(s.db, workspaceID)
		if err != nil {
			return "", err
		}
		window := newIssueWindow(workspaceID, &game, issue.Issue, *issue.ScheduledDrawAt, configuredSealSeconds(raw, gameID))
		var frozen lottery.IssueWindow
		err = s.db.Where("workspace_id = ? AND game_id = ? AND issue = ?", workspaceID, gameID, issue.Issue).First(&frozen).Error
		if err == nil {
			window = shortenIssueWindow(frozen, window)
		} else if err != gorm.ErrRecordNotFound {
			return "", err
		}
		if windowStatus(&window, now) != lottery.IssueStatusAccepting {
			return "", nil
		}
	}
	return issue.Issue, nil
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
	row.Source = "manual"
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
	if row.Source == "demo" {
		if patch.Result != plan.ResultPending {
			return PlanRecommendationView{}, apperrors.NewBusinessError("INVALID_REQUEST", "自动推荐不统计命中结果")
		}
		patch.Note = PlanDemoNotice
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
