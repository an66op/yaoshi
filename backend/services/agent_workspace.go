package services

import (
	"backend/accesscontrol"
	"backend/data/models/application"
	"backend/data/models/bet"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// AgentWorkspaceService is deliberately separate from the admin services.
// Every query starts from the authenticated agent id, so a caller cannot
// widen access by changing a user id, room number, or room scope in a request.
type AgentWorkspaceService struct{ db *gorm.DB }

type AgentWorkspaceDashboard struct {
	AgentID             uint64  `json:"agent_id"`
	RoomCode            string  `json:"room_code"`
	RoomName            string  `json:"room_name"`
	RoomLogo            string  `json:"room_logo"`
	MemberCount         int64   `json:"member_count"`
	ActiveMemberCount   int64   `json:"active_member_count"`
	MemberBalance       float64 `json:"member_balance"`
	TodayStake          float64 `json:"today_stake"`
	TodayPayout         float64 `json:"today_payout"`
	TodayNet            float64 `json:"today_net"`
	TodayRebate         float64 `json:"today_rebate"`
	TodayAgentShare     float64 `json:"today_agent_share"`
	TodayPlatformProfit float64 `json:"today_platform_profit"`
	PendingTurnover     float64 `json:"pending_turnover"`
	PendingBets         int64   `json:"pending_bets"`
	PendingApplications int64   `json:"pending_applications"`
}

func NewAgentWorkspaceService(db *gorm.DB) *AgentWorkspaceService {
	return &AgentWorkspaceService{db: db}
}

func agentWorkspaceFinancialBetQuery(db *gorm.DB, workspaceID uint64) *gorm.DB {
	return excludeRobotProfileBets(db.Model(&bet.Bet{}).Where("workspace_id = ?", workspaceID))
}

func (s *AgentWorkspaceService) DashboardForWorkspace(workspaceID uint64) (*AgentWorkspaceDashboard, error) {
	var workspace workspacemodel.Workspace
	if err := s.db.Where("id = ? AND status = ?", workspaceID, 1).First(&workspace).Error; err != nil {
		return nil, apperrors.NewBusinessError("WORKSPACE_NOT_FOUND", "房间不存在或已停用")
	}
	result := &AgentWorkspaceDashboard{AgentID: workspace.OwnerUserID, RoomCode: workspace.RoomCode, RoomName: workspace.Name, RoomLogo: workspace.Logo}
	members := excludeRobotProfileUsers(s.db.Model(&user.User{})).Where("workspace_id = ? AND role = ?", workspaceID, "member")
	if err := members.Count(&result.MemberCount).Error; err != nil {
		return nil, err
	}
	if err := excludeRobotProfileUsers(s.db.Model(&user.User{})).Where("workspace_id = ? AND role = ? AND status = 1", workspaceID, "member").Count(&result.ActiveMemberCount).Error; err != nil {
		return nil, err
	}
	var balance int64
	if err := members.Select("COALESCE(SUM(balance_cents),0)").Scan(&balance).Error; err != nil {
		return nil, err
	}
	result.MemberBalance = centsToAmount(balance)
	start := startOfDayCST(time.Now())
	var money struct{ Stake, Payout, Rebate, AgentShare int64 }
	if err := agentWorkspaceFinancialBetQuery(s.db, workspaceID).Where("COALESCE(settled_at,updated_at,created_at) >= ? AND status IN ?", start, []string{"won", "lost"}).
		Select("COALESCE(SUM(amount_cents),0) AS stake, COALESCE(SUM(payout_cents),0) AS payout, COALESCE(SUM(rebate_cents),0) AS rebate, COALESCE(SUM(agent_share_cents),0) AS agent_share").Scan(&money).Error; err != nil {
		return nil, err
	}
	result.TodayStake, result.TodayPayout = centsToAmount(money.Stake), centsToAmount(money.Payout)
	result.TodayNet, result.TodayRebate, result.TodayAgentShare = result.TodayStake-result.TodayPayout, centsToAmount(money.Rebate), centsToAmount(money.AgentShare)
	result.TodayPlatformProfit = centsToAmount(money.Stake - money.Payout - money.Rebate - money.AgentShare)
	var pending int64
	if err := agentWorkspaceFinancialBetQuery(s.db, workspaceID).Where("status = ?", "pending").Select("COALESCE(SUM(amount_cents),0)").Scan(&pending).Error; err != nil {
		return nil, err
	}
	result.PendingTurnover = centsToAmount(pending)
	if err := agentWorkspaceFinancialBetQuery(s.db, workspaceID).Where("status = ?", "pending").Count(&result.PendingBets).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&application.Application{}).Where("workspace_id = ? AND status = ?", workspaceID, "pending").Count(&result.PendingApplications).Error; err != nil {
		return nil, err
	}
	return result, nil
}

func (s *AgentWorkspaceService) Dashboard(agentID uint64) (*AgentWorkspaceDashboard, error) {
	agent, err := s.agent(agentID)
	if err != nil {
		return nil, err
	}
	workspace, err := WorkspaceForAccount(s.db, *agent)
	if err != nil {
		return nil, err
	}
	return s.DashboardForWorkspace(workspace.ID)
}

func (s *AgentWorkspaceService) Users(agentID uint64, filter UserListFilter) (*UserList, error) {
	if _, err := s.agent(agentID); err != nil {
		return nil, err
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	query := excludeRobotProfileUsers(s.db.Model(&user.User{})).Where("parent_agent_id = ?", agentID)
	if keyword := strings.TrimSpace(filter.Query); keyword != "" {
		like := "%" + strings.ToLower(keyword) + "%"
		query = query.Where("LOWER(username) LIKE ? OR LOWER(nickname) LIKE ? OR CAST(public_id AS TEXT) LIKE ?", like, like, "%"+keyword+"%")
	}
	if filter.Status == "active" {
		query = query.Where("status = 1")
	}
	if filter.Status == "disabled" {
		query = query.Where("status = 0")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []user.User
	if err := query.Order("created_at DESC, user_id DESC").Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]AdminUser, 0, len(rows))
	for _, row := range rows {
		items = append(items, adminUser(row))
	}
	populateAdminUserPresence(items)
	return &UserList{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (s *AgentWorkspaceService) Bets(agentID uint64, filter BetListFilter) (*BetListResult, error) {
	if _, err := s.agent(agentID); err != nil {
		return nil, err
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	query := s.db.Model(&bet.Bet{}).Where("room_scope = ?", fmt.Sprintf("agent:%d", agentID))
	if q := strings.TrimSpace(filter.Query); q != "" {
		like := "%" + q + "%"
		query = query.Where("username ILIKE ? OR issue ILIKE ? OR selection ILIKE ? OR play_name ILIKE ?", like, like, like, like)
	}
	if filter.GameID != "" && filter.GameID != "all" {
		query = query.Where("game_id = ?", filter.GameID)
	}
	if filter.Issue != "" {
		query = query.Where("issue = ?", filter.Issue)
	}
	if filter.UserID > 0 {
		if err := s.EnsureOwnedUser(agentID, filter.UserID); err != nil {
			return nil, err
		}
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.Status != "" && filter.Status != "all" {
		if filter.Status == "settled" {
			query = query.Where("status IN ?", []string{"won", "lost", "cancelled"})
		} else {
			query = query.Where("status = ?", filter.Status)
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []bet.Bet
	if err := query.Order("id DESC").Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]BetView, 0, len(rows))
	for _, row := range rows {
		items = append(items, toBetView(row))
	}
	return &BetListResult{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (s *AgentWorkspaceService) Applications(agentID uint64, filter ApplicationFilter) (*ApplicationList, error) {
	if _, err := s.agent(agentID); err != nil {
		return nil, err
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	query := applyApplicationFilters(s.applicationQuery(agentID), filter)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []application.Application
	if err := query.Order("CASE WHEN status = 'pending' THEN 0 ELSE 1 END, CASE WHEN status = 'pending' THEN created_at END ASC, created_at DESC").Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]AdminApplication, 0, len(rows))
	for _, row := range rows {
		items = append(items, adminApplication(row))
	}
	return &ApplicationList{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (s *AgentWorkspaceService) ReviewApplication(agentID, applicationID uint64, input ReviewApplicationInput) (*AdminApplication, error) {
	input.Operator = fmt.Sprintf("房间 %d 代理", agentID)
	return NewApplicationAdminService(s.db).ReviewOwned(applicationID, agentID, input)
}

func (s *AgentWorkspaceService) AdjustBalance(agentID, userID uint64, amount float64, remark, operator string) (*AdminUser, error) {
	return NewUserAdminService(s.db).AdjustBalanceOwned(userID, agentID, amount, remark, operator)
}

func (s *AgentWorkspaceService) SetUserStatus(agentID, userID uint64, status int) (*AdminUser, error) {
	return NewUserAdminService(s.db).SetStatusOwned(userID, agentID, status)
}

func (s *AgentWorkspaceService) EnsureOwnedUser(agentID, userID uint64) error {
	var count int64
	if err := s.db.Model(&user.User{}).Where("user_id = ? AND parent_agent_id = ?", userID, agentID).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return apperrors.NewBusinessError("FORBIDDEN", "该用户不属于当前房间")
	}
	return nil
}

func (s *AgentWorkspaceService) agent(agentID uint64) (*user.User, error) {
	var row user.User
	if err := s.db.Where("user_id = ? AND role = ? AND status = 1", agentID, "agent").First(&row).Error; err != nil {
		return nil, apperrors.NewBusinessError("FORBIDDEN", "房间代理账号不可用")
	}
	hierarchyActive, hierarchyErr := accesscontrol.AgentHierarchyActive(s.db, row)
	if hierarchyErr != nil {
		return nil, hierarchyErr
	}
	if !hierarchyActive {
		return nil, apperrors.NewBusinessError("FORBIDDEN", "所属租户已停用，房间代理账号不可用")
	}
	return &row, nil
}

func (s *AgentWorkspaceService) UpdateRoomProfile(agentID uint64, roomName, roomLogo string) (*AgentWorkspaceDashboard, error) {
	agent, err := s.agent(agentID)
	if err != nil {
		return nil, err
	}
	workspace, err := WorkspaceForAccount(s.db, *agent)
	if err != nil {
		return nil, err
	}
	if _, err := NewSettingsAdminService(s.db).UpdateRoomProfileForWorkspace(workspace.ID, roomName, roomLogo); err != nil {
		return nil, err
	}
	return s.DashboardForWorkspace(workspace.ID)
}

func (s *AgentWorkspaceService) roomMemberIDs(agentID uint64) *gorm.DB {
	return s.db.Model(&user.User{}).Select("user_id").Where("parent_agent_id = ?", agentID)
}

func (s *AgentWorkspaceService) applicationQuery(agentID uint64) *gorm.DB {
	var workspaceID uint64
	_ = s.db.Model(&user.User{}).Where("user_id = ?", agentID).Pluck("workspace_id", &workspaceID).Error
	return s.db.Model(&application.Application{}).Where("workspace_id = ?", workspaceID)
}
