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
	ID                uint64     `json:"id"`
	WorkspaceID       uint64     `json:"workspace_id"`
	GameID            string     `json:"game_id"`
	Issue             string     `json:"issue"`
	MasterName        string     `json:"master_name"`
	MasterTitle       string     `json:"master_title"`
	MasterColor       string     `json:"master_color"`
	Numbers           []int      `json:"numbers"`
	Size              string     `json:"size"`
	Parity            string     `json:"parity"`
	Result            string     `json:"result"`
	Source            string     `json:"source"`
	Note              string     `json:"note"`
	Enabled           bool       `json:"enabled"`
	SortOrder         int        `json:"sort_order"`
	MasterHitRate     *float64   `json:"master_hit_rate"`
	MasterSampleCount int        `json:"master_sample_count"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	Position          int        `json:"position,omitempty"`
	PlanKey           string     `json:"plan_key,omitempty"`
	Kind              string     `json:"kind,omitempty"`
	DragonTiger       string     `json:"dragon_tiger,omitempty"`
	CycleID           uint64     `json:"cycle_id,omitempty"`
	CyclePeriod       int        `json:"cycle_period,omitempty"`
	CyclePeriods      int        `json:"cycle_periods,omitempty"`
	CycleStartIssue   string     `json:"cycle_start_issue,omitempty"`
	CycleStatus       string     `json:"cycle_status,omitempty"`
	DrawNumbers       []int      `json:"draw_numbers,omitempty"`
	DrawAt            *time.Time `json:"draw_at,omitempty"`
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
	{GameID: "canada-28", MasterName: "1号专家", MasterTitle: "系统自动推荐", MasterColor: "#2aa9b3", Numbers: "3,14,22", Size: "大", Parity: "单", SortOrder: 10},
	{GameID: "canada-28", MasterName: "2号专家", MasterTitle: "系统自动推荐", MasterColor: "#6e70df", Numbers: "6,11,19", Size: "小", Parity: "双", SortOrder: 20},
	{GameID: "canada-28", MasterName: "3号专家", MasterTitle: "系统自动推荐", MasterColor: "#e58b45", Numbers: "8,17,25", Size: "大", Parity: "双", SortOrder: 30},
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

func validatePlanNumberContract(game lottery.Game, rawNumbers string) error {
	minimum, maximum, supported := planDemoNumberRange(game)
	if !supported {
		return apperrors.NewBusinessError("RULES_NOT_READY", "该彩种尚未配置可验证的推荐号码规则")
	}
	if _, valid := strictPlanPickNumbers(rawNumbers, minimum, maximum); !valid {
		return apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("该彩种推荐号码只能填写 %d 至 %d 的不重复整数", minimum, maximum))
	}
	return nil
}

func (s *PlanContentService) ensurePlanNumberContract(row plan.Recommendation) error {
	var game lottery.Game
	if err := s.db.First(&game, "id = ?", row.GameID).Error; err != nil {
		return err
	}
	return validatePlanNumberContract(game, row.Numbers)
}

func ensurePlanRecommendationEditable(row plan.Recommendation) error {
	if canonicalPlanSource(row.Source) == "demo" {
		return apperrors.NewBusinessError("PLAN_PUBLICATION_IMMUTABLE", "系统自动推荐不能手工修改或删除；请通过自动计划配置管理")
	}
	return nil
}

func validatePlanInput(input PlanRecommendationInput) (plan.Recommendation, error) {
	if racingPlanGameID(strings.TrimSpace(input.GameID)) {
		return plan.Recommendation{}, apperrors.NewBusinessError("INVALID_REQUEST", "赛车类彩种请使用自动计划的名次和方案矩阵，不能发布通用手工推荐")
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
	if row.Result != "" && row.Result != plan.ResultPending {
		return plan.Recommendation{}, apperrors.NewBusinessError("INVALID_REQUEST", "推荐结果只能由已验证开奖自动计算")
	}
	row.Result = plan.ResultPending
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

func (s *PlanContentService) ensureIssue(workspaceID uint64, gameID, issue string) error {
	var row lottery.Issue
	if err := s.db.Where("game_id = ? AND issue = ?", strings.TrimSpace(gameID), strings.TrimSpace(issue)).First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NewBusinessError("NOT_FOUND", "该彩种期号不存在")
		}
		return err
	}
	now := time.Now().UTC()
	var draws int64
	if err := s.db.Model(&lottery.Draw{}).Where("game_id = ? AND issue = ?", row.GameID, row.Issue).Count(&draws).Error; err != nil {
		return err
	}
	if draws > 0 || row.DrawAt != nil || row.Status != lottery.IssueStatusAccepting || row.ScheduledDrawAt == nil || row.ScheduledDrawAt.IsZero() {
		return apperrors.NewBusinessError("PLAN_PUBLICATION_CLOSED", "该期已封盘或已开奖，不能回填推荐")
	}
	var game lottery.Game
	if err := s.db.First(&game, "id = ?", row.GameID).Error; err != nil {
		return err
	}
	rawSettings, actualWorkspaceID, err := readTimingSettings(s.db, workspaceID)
	if err != nil {
		return err
	}
	window, err := ensureIssueWindow(s.db, actualWorkspaceID, &game, row.Issue, *row.ScheduledDrawAt, rawSettings)
	if err != nil {
		return err
	}
	sealAt := effectivePlanSealAt(row, window)
	acceptAt := row.AcceptAt
	if window.AcceptAt.After(acceptAt) {
		acceptAt = window.AcceptAt
	}
	if now.Before(acceptAt) || !now.Before(sealAt) || !now.Before(*row.ScheduledDrawAt) {
		return apperrors.NewBusinessError("PLAN_PUBLICATION_CLOSED", "该期已封盘或已开奖，不能回填推荐")
	}
	return nil
}

func (s *PlanContentService) ensureGenericPublicationUnviewed(workspaceID uint64, gameID, issue string) error {
	var count int64
	if err := s.db.Model(&plan.PublicationView{}).
		Where("workspace_id = ? AND game_id = ? AND issue = ? AND position = 0", workspaceID, strings.TrimSpace(gameID), strings.TrimSpace(issue)).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return apperrors.NewBusinessError("PLAN_PUBLICATION_LOCKED", "该期推荐已被会员查看，不能再新增")
	}
	return nil
}

func planPublicationLockedDatabaseError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "viewed plan publication is immutable") ||
		strings.Contains(message, "plan publication insert is locked")
}

func planView(row plan.Recommendation, statistics map[string]planHitStatistic) PlanRecommendationView {
	source := canonicalPlanSource(row.Source)
	statistic := statistics[planMasterStatisticKey(row.GameID, source, row.MasterName)]
	if source == "demo" {
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
		row.Note = PlanDemoNotice
	}
	if row.GameID == "speed-racing" {
		row.Size, row.Parity = "", ""
	}
	return PlanRecommendationView{
		ID: row.ID, WorkspaceID: row.WorkspaceID, GameID: row.GameID, Issue: row.Issue,
		MasterName: row.MasterName, MasterTitle: row.MasterTitle, MasterColor: row.MasterColor,
		Numbers: parsePlanNumbers(row.Numbers), Size: row.Size, Parity: row.Parity,
		Result: row.Result, Source: source, Note: row.Note, Enabled: row.Enabled, SortOrder: row.SortOrder,
		MasterHitRate: statistic.Rate, MasterSampleCount: statistic.SampleCount, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
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
	if err := deriveTrustedPlanResults(s.db, rows, time.Now().UTC()); err != nil {
		return nil, err
	}
	statistics := planHitStatistics(rows)
	result := make([]PlanRecommendationView, 0, len(rows))
	for _, row := range rows {
		result = append(result, planView(row, statistics))
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
		// Racing-v2 products are represented exclusively by immutable stream
		// cycles. Never let retired generic/editorial rows create or replace a
		// rich racing catalog card.
		if racingPlanGameID(item.GameID) {
			continue
		}
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
	if racingPlanGameID(result.GameID) {
		return result, nil
	}
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
	if err := deriveTrustedPlanResults(s.db, rows, time.Now().UTC()); err != nil {
		return result, err
	}
	statistics := planHitStatistics(rows)
	seenMasters := map[string]bool{}
	for _, row := range rows {
		view := planView(row, statistics)
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
	if err := s.ensurePlanNumberContract(row); err != nil {
		return PlanRecommendationView{}, err
	}
	if err := s.ensureGenericPublicationUnviewed(workspaceID, row.GameID, row.Issue); err != nil {
		return PlanRecommendationView{}, err
	}
	if err := s.ensureIssue(workspaceID, row.GameID, row.Issue); err != nil {
		return PlanRecommendationView{}, err
	}
	row.WorkspaceID = workspaceID
	row.Source = "manual"
	if err := s.db.Create(&row).Error; err != nil {
		if planPublicationLockedDatabaseError(err) {
			return PlanRecommendationView{}, apperrors.NewBusinessError("PLAN_PUBLICATION_LOCKED", "该期推荐已被会员查看或已经封盘，不能再新增")
		}
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return PlanRecommendationView{}, apperrors.NewBusinessError("INVALID_REQUEST", "该期该大师的推荐已经存在")
		}
		return PlanRecommendationView{}, err
	}
	return planView(row, nil), nil
}

func (s *PlanContentService) Update(workspaceID, id uint64, input PlanRecommendationInput) (PlanRecommendationView, error) {
	patch, err := validatePlanInput(input)
	if err != nil {
		return PlanRecommendationView{}, err
	}
	var row plan.Recommendation
	if err := s.db.Where("id = ? AND workspace_id = ?", id, workspaceID).First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return PlanRecommendationView{}, apperrors.NewBusinessError("NOT_FOUND", "推荐不存在")
		}
		return PlanRecommendationView{}, err
	}
	if patch.GameID != row.GameID || patch.Issue != row.Issue {
		return PlanRecommendationView{}, apperrors.NewBusinessError("INVALID_REQUEST", "已发布推荐不能更换彩种或期号")
	}
	if err := ensurePlanRecommendationEditable(row); err != nil {
		return PlanRecommendationView{}, err
	}
	if err := s.ensureScope(workspaceID, row.GameID); err != nil {
		return PlanRecommendationView{}, err
	}
	if err := s.ensurePlanNumberContract(patch); err != nil {
		return PlanRecommendationView{}, err
	}
	if err := s.ensureIssue(workspaceID, row.GameID, row.Issue); err != nil {
		return PlanRecommendationView{}, err
	}
	viewed, err := recommendationPublicationViewed(s.db, row)
	if err != nil {
		return PlanRecommendationView{}, err
	}
	if viewed {
		return PlanRecommendationView{}, apperrors.NewBusinessError("PLAN_PUBLICATION_LOCKED", "该期推荐已被会员查看，不能再修改")
	}
	updates := map[string]any{
		"game_id": patch.GameID, "issue": patch.Issue, "master_name": patch.MasterName,
		"master_title": patch.MasterTitle, "master_color": patch.MasterColor, "numbers": patch.Numbers,
		"size": patch.Size, "parity": patch.Parity, "result": patch.Result, "note": patch.Note,
		"enabled": patch.Enabled, "sort_order": patch.SortOrder,
	}
	if err := s.db.Model(&row).Updates(updates).Error; err != nil {
		if planPublicationLockedDatabaseError(err) {
			return PlanRecommendationView{}, apperrors.NewBusinessError("PLAN_PUBLICATION_LOCKED", "该期推荐已被会员查看或已经封盘，不能再修改")
		}
		return PlanRecommendationView{}, fmt.Errorf("update recommendation: %w", err)
	}
	if err := s.db.First(&row, id).Error; err != nil {
		return PlanRecommendationView{}, err
	}
	return planView(row, nil), nil
}

func (s *PlanContentService) Delete(workspaceID, id uint64) error {
	if err := s.ensureScope(workspaceID, ""); err != nil {
		return err
	}
	var row plan.Recommendation
	if err := s.db.Where("id = ? AND workspace_id = ?", id, workspaceID).First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NewBusinessError("NOT_FOUND", "推荐不存在")
		}
		return err
	}
	if err := ensurePlanRecommendationEditable(row); err != nil {
		return err
	}
	if err := s.ensureIssue(workspaceID, row.GameID, row.Issue); err != nil {
		return err
	}
	viewed, err := recommendationPublicationViewed(s.db, row)
	if err != nil {
		return err
	}
	if viewed {
		return apperrors.NewBusinessError("PLAN_PUBLICATION_LOCKED", "该期推荐已被会员查看，不能再删除")
	}
	result := s.db.Where("id = ? AND workspace_id = ?", id, workspaceID).Delete(&plan.Recommendation{})
	if result.Error != nil {
		if planPublicationLockedDatabaseError(result.Error) {
			return apperrors.NewBusinessError("PLAN_PUBLICATION_LOCKED", "该期推荐已被会员查看或已经封盘，不能再删除")
		}
		return result.Error
	}
	if result.RowsAffected != 1 {
		return apperrors.NewBusinessError("NOT_FOUND", "推荐不存在")
	}
	return nil
}
