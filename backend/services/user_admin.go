package services

import (
	"backend/data/models/chat"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"backend/utils"
	"backend/ws"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	pseudorand "math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UserAdminService struct{ db *gorm.DB }

type AdminUser struct {
	ID               uint64     `json:"id"`
	PublicID         uint64     `json:"public_id"`
	Username         string     `json:"username"`
	Email            string     `json:"email"`
	Nickname         string     `json:"nickname"`
	Avatar           string     `json:"avatar,omitempty"`
	PublicTitle      string     `json:"public_title,omitempty"`
	PublicBadge      string     `json:"badge,omitempty"`
	Phone            string     `json:"phone"`
	Role             string     `json:"role"`
	Remark           string     `json:"remark"`
	RiskLevel        string     `json:"risk_level"`
	Balance          float64    `json:"balance"`
	FlyMode          string     `json:"fly_mode"`
	FlyRate          float64    `json:"fly_rate"`
	AgentRoomCode    string     `json:"agent_room_code"`
	RoomCode         string     `json:"room_code"`
	ParentAgentID    *uint64    `json:"parent_agent_id"`
	ParentTenantID   *uint64    `json:"parent_tenant_id"`
	AgentName        string     `json:"agent_name"`
	TenantName       string     `json:"tenant_name"`
	LoginIdentity    string     `json:"login_identity"`
	Status           int        `json:"status"`
	Online           bool       `json:"online"`
	LastLoginAt      *time.Time `json:"last_login_at"`
	LoginCount       int        `json:"login_count"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	IsRobot          bool       `json:"is_robot"`
	RobotGameIDs     []string   `json:"robot_game_ids"`
	RobotActiveStart string     `json:"robot_active_start"`
	RobotActiveEnd   string     `json:"robot_active_end"`
	RobotMinBet      float64    `json:"robot_min_bet"`
	RobotMaxBet      float64    `json:"robot_max_bet"`
	RobotAvatar      string     `json:"robot_avatar"`
	WorkspaceID      uint64     `json:"workspace_id"`
}

type UserListFilter struct {
	UserID      uint64 // Exact roster lookup; it never replaces the authenticated room scope.
	Query       string
	Status      string
	Role        string
	Kind        string
	Page        int
	PageSize    int
	WorkspaceID uint64
}

type UserList struct {
	Items    []AdminUser `json:"items"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

type UserStats struct {
	Total          int64   `json:"total"`
	Active         int64   `json:"active"`
	Disabled       int64   `json:"disabled"`
	NewToday       int64   `json:"new_today"`
	Administrators int64   `json:"administrators"`
	TotalBalance   float64 `json:"total_balance"`
}

type CreateAdminUserInput struct {
	Username      string
	Password      string
	Email         string
	Nickname      string
	Phone         string
	Role          string
	Remark        string
	RiskLevel     string
	Status        int
	ParentAgentID uint64
}

type UpdateAdminUserInput struct {
	Email     string
	Nickname  string
	Phone     string
	Role      string
	Remark    string
	RiskLevel string
	Status    int
}

type UpdateRobotInput struct {
	Nickname    string
	Status      int
	GameIDs     []string
	ActiveStart string
	ActiveEnd   string
	MinBet      float64
	MaxBet      float64
	Avatar      string
}

type ResetWorkspaceRobotsInput struct {
	WorkspaceID    uint64  `json:"workspace_id"`
	RequestID      string  `json:"request_id"`
	Mode           string  `json:"mode"`
	NicknamePrefix string  `json:"nickname_prefix"`
	Balance        float64 `json:"balance"`
	BalanceMin     float64 `json:"balance_min"`
	BalanceMax     float64 `json:"balance_max"`
}

type ResetWorkspaceRobotsResult struct {
	RequestID string      `json:"request_id"`
	Mode      string      `json:"mode"`
	Count     int         `json:"count"`
	Duplicate bool        `json:"duplicate"`
	Items     []AdminUser `json:"items"`
}

type RobotWorkspaceOption struct {
	WorkspaceID uint64 `json:"workspace_id"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	RoomCode    string `json:"room_code"`
	Status      int    `json:"status"`
	RobotCount  int64  `json:"robot_count"`
	RobotQuota  int    `json:"robot_quota"`
}

type normalizedRobotReset struct {
	requestID      string
	mode           string
	nicknamePrefix string
	balanceMin     int64
	balanceMax     int64
	payloadHash    string
}

type robotResetPlan struct {
	nickname     string
	balanceCents int64
}

var robotResetRequestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,96}$`)

const maxRobotResetBalanceCents int64 = 10_000_000_000

type BalanceRecord struct {
	ID        uint64    `json:"id"`
	UserID    uint64    `json:"user_id"`
	Reference string    `json:"reference,omitempty"`
	Amount    float64   `json:"amount"`
	Before    float64   `json:"before"`
	After     float64   `json:"after"`
	Type      string    `json:"type"`
	Remark    string    `json:"remark"`
	Operator  string    `json:"operator"`
	CreatedAt time.Time `json:"created_at"`
}

type BalanceHistoryPage struct {
	Items        []BalanceRecord `json:"items"`
	HasMore      bool            `json:"has_more"`
	NextBeforeID uint64          `json:"next_before_id"`
}

func NewUserAdminService(db *gorm.DB) *UserAdminService { return &UserAdminService{db: db} }

func (s *UserAdminService) RobotWorkspaces() ([]RobotWorkspaceOption, error) {
	var workspaces []workspacemodel.Workspace
	if err := s.db.Where("status = ? AND type IN ?", 1, robotWorkspaceTypes()).
		Order("CASE type WHEN 'platform' THEN 0 WHEN 'tenant' THEN 1 ELSE 2 END, id ASC").Find(&workspaces).Error; err != nil {
		return nil, err
	}
	workspaceIDs := make([]uint64, 0, len(workspaces))
	for _, workspace := range workspaces {
		workspaceIDs = append(workspaceIDs, workspace.ID)
	}
	type workspaceRobotCount struct {
		WorkspaceID uint64
		Count       int64
	}
	countsByWorkspace := make(map[uint64]int64, len(workspaces))
	if len(workspaceIDs) > 0 {
		var counts []workspaceRobotCount
		if err := s.db.Table("workspace_robot_profiles AS profile").
			Select("profile.workspace_id, COUNT(*) AS count").
			Joins(`JOIN "user" AS account ON account.user_id = profile.user_id`).
			Where("profile.workspace_id IN ? AND account.workspace_id = profile.workspace_id AND account.role = ? AND account.deleted_at IS NULL", workspaceIDs, "member").
			Group("profile.workspace_id").Find(&counts).Error; err != nil {
			return nil, err
		}
		for _, row := range counts {
			countsByWorkspace[row.WorkspaceID] = row.Count
		}
	}
	result := make([]RobotWorkspaceOption, 0, len(workspaces))
	for _, workspace := range workspaces {
		allocatedCount := countsByWorkspace[workspace.ID]
		if allocatedCount > int64(workspace.RobotQuota) {
			allocatedCount = int64(workspace.RobotQuota)
		}
		result = append(result, RobotWorkspaceOption{
			WorkspaceID: workspace.ID, Type: workspace.Type, Name: workspace.Name,
			RoomCode: workspace.RoomCode, Status: workspace.Status, RobotCount: allocatedCount, RobotQuota: workspace.RobotQuota,
		})
	}
	return result, nil
}

func (s *UserAdminService) List(filter UserListFilter) (*UserList, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	query := s.db.Model(&user.User{})
	if filter.WorkspaceID > 0 {
		query = query.Where(`"user".workspace_id = ?`, filter.WorkspaceID)
	}
	if keyword := strings.TrimSpace(filter.Query); keyword != "" {
		like := "%" + strings.ToLower(keyword) + "%"
		query = query.Where("LOWER(username) LIKE ? OR LOWER(nickname) LIKE ? OR LOWER(email) LIKE ? OR LOWER(phone) LIKE ? OR LOWER(remark) LIKE ?", like, like, like, like, like)
	}
	switch filter.Status {
	case "active":
		query = query.Where("status = 1")
	case "disabled":
		query = query.Where("status = 0")
	}
	if role := strings.TrimSpace(filter.Role); role != "" && role != "all" {
		query = query.Where("role = ?", role)
	}
	switch strings.TrimSpace(filter.Kind) {
	case "member":
		query = HumanMemberQuery(query)
	case "robot":
		if filter.WorkspaceID == 0 {
			return nil, apperrors.NewBusinessError("WORKSPACE_REQUIRED", "请选择要查看的机器人工作区")
		}
		var workspace workspacemodel.Workspace
		if err := s.db.Select("id", "robot_quota").First(&workspace, filter.WorkspaceID).Error; err != nil {
			return nil, err
		}
		query = query.Select(`"user".*`).Joins(`JOIN workspace_robot_profiles AS robot_profile ON robot_profile.user_id = "user".user_id`).
			Where(`"user".role = ? AND robot_profile.workspace_id = ?`, "member", filter.WorkspaceID)
		if workspace.RobotQuota == 0 {
			query = query.Where("1 = 0")
		} else {
			query = query.Where("robot_profile.id IN (?)", allocatedRobotProfileIDs(s.db, filter.WorkspaceID, workspace.RobotQuota))
		}
	case "account":
		query = query.Where("role IN ?", []string{"admin", "tenant", "agent"})
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []user.User
	if err := query.Order("created_at desc, user_id desc").Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]AdminUser, 0, len(rows))
	for _, row := range rows {
		items = append(items, adminUser(row))
	}
	populateAdminUserPresence(items)
	if err := s.enrichOwnership(items); err != nil {
		return nil, err
	}
	if err := s.enrichRobotProfiles(items); err != nil {
		return nil, err
	}
	return &UserList{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (s *UserAdminService) Stats(kind string) (*UserStats, error) {
	stats := &UserStats{}
	base := s.db.Model(&user.User{})
	applyKind := func(query *gorm.DB) *gorm.DB {
		switch strings.TrimSpace(kind) {
		case "member":
			return HumanMemberQuery(query)
		case "robot":
			return query.Joins(`JOIN workspace_robot_profiles AS robot_profile
				ON robot_profile.workspace_id = "user".workspace_id
				AND robot_profile.user_id = "user".user_id`).Where(`"user".role = ?`, "member")
		case "account":
			return query.Where("role IN ?", []string{"admin", "tenant", "agent"})
		default:
			return query
		}
	}
	base = applyKind(base)
	if err := base.Count(&stats.Total).Error; err != nil {
		return nil, err
	}
	if err := applyKind(s.db.Model(&user.User{})).Where("status = 1").Count(&stats.Active).Error; err != nil {
		return nil, err
	}
	if err := applyKind(s.db.Model(&user.User{})).Where("status = 0").Count(&stats.Disabled).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if err := applyKind(s.db.Model(&user.User{})).Where("created_at >= ?", startOfDay).Count(&stats.NewToday).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&user.User{}).Where("role = ?", "admin").Count(&stats.Administrators).Error; err != nil {
		return nil, err
	}
	var cents int64
	if err := applyKind(s.db.Model(&user.User{})).Select("COALESCE(SUM(balance_cents), 0)").Scan(&cents).Error; err != nil {
		return nil, err
	}
	stats.TotalBalance = centsToAmount(cents)
	return stats, nil
}

func (s *UserAdminService) Get(id uint64) (*AdminUser, error) {
	var row user.User
	if err := s.db.First(&row, id).Error; err != nil {
		return nil, err
	}
	result := adminUser(row)
	items := []AdminUser{result}
	populateAdminUserPresence(items)
	if err := s.enrichOwnership(items); err != nil {
		return nil, err
	}
	if err := s.enrichRobotProfiles(items); err != nil {
		return nil, err
	}
	return &items[0], nil
}

// UpdateRobot edits only room-activity accounts. It intentionally keeps the
// account as a normal member so bets use the same balance, odds and settlement
// path as any other member.
func (s *UserAdminService) UpdateRobot(id uint64, input UpdateRobotInput) (*AdminUser, error) {
	return s.updateRobot(id, nil, input)
}

func (s *UserAdminService) UpdateRobotForWorkspace(id, workspaceID uint64, input UpdateRobotInput) (*AdminUser, error) {
	if workspaceID == 0 {
		return nil, apperrors.NewBusinessError("WORKSPACE_REQUIRED", "机器人所属房间不存在")
	}
	return s.updateRobot(id, &workspaceID, input)
}

// ResetRobotsForWorkspace replaces every robot nickname and target balance in
// one locked workspace transaction. The caller must derive or authorize the
// workspace before calling this service. A durable receipt, independent from
// archivable balance history, prevents a retry from applying the reset twice.
func (s *UserAdminService) ResetRobotsForWorkspace(workspaceID uint64, input ResetWorkspaceRobotsInput, operator string) (*ResetWorkspaceRobotsResult, error) {
	if workspaceID == 0 {
		return nil, apperrors.NewBusinessError("WORKSPACE_REQUIRED", "机器人所属房间不存在")
	}
	config, err := normalizeRobotResetInput(input)
	if err != nil {
		return nil, err
	}
	duplicate := false
	resetCount := 0
	err = s.db.Transaction(func(tx *gorm.DB) error {
		workspace, err := EnabledRobotWorkspace(tx.Clauses(clause.Locking{Strength: "UPDATE"}), workspaceID)
		if err != nil {
			return err
		}
		requestIDHash := robotResetRequestIDHash(config.requestID)
		var receipt workspacemodel.RobotResetReceipt
		receiptErr := tx.Where("workspace_id = ? AND request_id_hash = ?", workspaceID, requestIDHash).First(&receipt).Error
		if receiptErr == nil {
			if receipt.PayloadHash != config.payloadHash || receipt.Mode != config.mode {
				return apperrors.NewBusinessError("REQUEST_ID_REUSED", "request_id 已被其他机器人重置参数使用")
			}
			duplicate = true
			resetCount = receipt.RobotCount
			return nil
		}
		if !errors.Is(receiptErr, gorm.ErrRecordNotFound) {
			return receiptErr
		}
		var accounts []user.User
		if workspace.RobotQuota == 0 {
			return apperrors.NewBusinessError("ROBOT_QUOTA_REQUIRED", "上级尚未分配机器人名额")
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Model(&user.User{}).
			Select(`"user".*`).
			Joins(`JOIN workspace_robot_profiles AS robot_profile ON robot_profile.user_id = "user".user_id`).
			Where(`"user".workspace_id = ? AND robot_profile.workspace_id = ? AND "user".role = ?`, workspaceID, workspaceID, "member").
			Where("robot_profile.id IN (?)", allocatedRobotProfileIDs(tx, workspaceID, workspace.RobotQuota)).
			Order(`"user".user_id ASC`).Find(&accounts).Error; err != nil {
			return err
		}
		if len(accounts) == 0 {
			return apperrors.NewBusinessError("ROBOTS_NOT_FOUND", "当前房间暂无可重置的机器人")
		}
		resetCount = len(accounts)
		plans, err := buildRobotResetPlans(config, workspaceID, len(accounts))
		if err != nil {
			return err
		}
		_, reference := robotResetReferences(workspaceID, config)

		operator = defaultString(strings.TrimSpace(operator), "后台管理员")
		for index := range accounts {
			account := &accounts[index]
			plan := plans[index]
			update := tx.Model(&user.User{}).
				Where("user_id = ? AND workspace_id = ? AND role = ?", account.UserID, workspaceID, "member").
				Updates(map[string]any{"nickname": plan.nickname, "balance_cents": plan.balanceCents})
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected != 1 {
				return apperrors.NewBusinessError("ROBOT_SCOPE_MISMATCH", "机器人账号房间归属发生变化，请刷新后重试")
			}
			if err := tx.Model(&chat.Message{}).
				Where("workspace_id = ? AND user_id = ?", workspaceID, account.UserID).
				Update("nickname", plan.nickname).Error; err != nil {
				return err
			}
			record := user.BalanceTransaction{
				WorkspaceID: workspaceID, UserID: account.UserID, Reference: reference,
				AmountCents: plan.balanceCents - account.BalanceCents,
				BeforeCents: account.BalanceCents, AfterCents: plan.balanceCents,
				Type: "robot_reset", Remark: "机器人批量重置（" + robotResetModeLabel(config.mode) + "）", Operator: operator,
			}
			if err := tx.Create(&record).Error; err != nil {
				return err
			}
		}
		return tx.Create(&workspacemodel.RobotResetReceipt{
			WorkspaceID: workspaceID, RequestIDHash: requestIDHash, PayloadHash: config.payloadHash,
			Mode: config.mode, RobotCount: resetCount,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	rows, err := s.List(UserListFilter{WorkspaceID: workspaceID, Kind: "robot", Page: 1, PageSize: 100})
	if err != nil {
		return nil, err
	}
	return &ResetWorkspaceRobotsResult{
		RequestID: config.requestID, Mode: config.mode, Count: resetCount, Duplicate: duplicate, Items: rows.Items,
	}, nil
}

func robotWorkspaceTypes() []string {
	return []string{workspacemodel.TypePlatform, workspacemodel.TypeTenant, workspacemodel.TypeAgent}
}

func EnabledRobotWorkspace(db *gorm.DB, workspaceID uint64) (workspacemodel.Workspace, error) {
	var workspace workspacemodel.Workspace
	if workspaceID == 0 {
		return workspace, apperrors.NewBusinessError("INVALID_WORKSPACE", "目标机器人工作区不存在或已停用")
	}
	err := db.Where("id = ? AND status = ? AND type IN ?", workspaceID, 1, robotWorkspaceTypes()).First(&workspace).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return workspace, apperrors.NewBusinessError("INVALID_WORKSPACE", "目标机器人工作区不存在或已停用")
	}
	return workspace, err
}

func normalizeRobotResetInput(input ResetWorkspaceRobotsInput) (normalizedRobotReset, error) {
	result := normalizedRobotReset{
		requestID: strings.TrimSpace(input.RequestID), mode: strings.ToLower(strings.TrimSpace(input.Mode)),
		nicknamePrefix: strings.TrimSpace(input.NicknamePrefix),
	}
	if !robotResetRequestIDPattern.MatchString(result.requestID) {
		return result, apperrors.NewBusinessError("INVALID_REQUEST_ID", "request_id 需要 8–96 位字母、数字或 . _ : -")
	}
	switch result.mode {
	case "random":
		minimum, err := robotResetAmountCents(input.BalanceMin)
		if err != nil {
			return result, err
		}
		maximum, err := robotResetAmountCents(input.BalanceMax)
		if err != nil {
			return result, err
		}
		if minimum > maximum {
			return result, apperrors.NewBusinessError("INVALID_BALANCE_RANGE", "随机余额下限不能大于上限")
		}
		result.balanceMin, result.balanceMax = minimum, maximum
	case "custom":
		if result.nicknamePrefix == "" || len([]rune(result.nicknamePrefix)) > 44 {
			return result, apperrors.NewBusinessError("INVALID_NICKNAME_PREFIX", "自定义昵称前缀应为 1–44 个字符")
		}
		balance, err := robotResetAmountCents(input.Balance)
		if err != nil {
			return result, err
		}
		result.balanceMin, result.balanceMax = balance, balance
	default:
		return result, apperrors.NewBusinessError("INVALID_RESET_MODE", "重置方式只能是 random 或 custom")
	}
	payload := fmt.Sprintf("%s\x00%s\x00%d\x00%d", result.mode, result.nicknamePrefix, result.balanceMin, result.balanceMax)
	digest := sha256.Sum256([]byte(payload))
	result.payloadHash = hex.EncodeToString(digest[:16])
	return result, nil
}

func robotResetAmountCents(value float64) (int64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > float64(maxRobotResetBalanceCents)/100 {
		return 0, apperrors.NewBusinessError("INVALID_BALANCE", "机器人目标余额应在 0–100000000 之间")
	}
	return int64(math.Round(value * 100)), nil
}

func buildRobotResetPlans(config normalizedRobotReset, workspaceID uint64, count int) ([]robotResetPlan, error) {
	if count < 1 {
		return nil, apperrors.NewBusinessError("ROBOTS_NOT_FOUND", "当前房间暂无可重置的机器人")
	}
	plans := make([]robotResetPlan, count)
	if config.mode == "custom" {
		width := len(strconv.Itoa(count))
		if width < 2 {
			width = 2
		}
		for index := range plans {
			plans[index] = robotResetPlan{
				nickname: config.nicknamePrefix + fmt.Sprintf("%0*d", width, index+1), balanceCents: config.balanceMin,
			}
		}
	} else {
		seedInput := fmt.Sprintf("%d\x00%s\x00%s", workspaceID, config.requestID, config.payloadHash)
		digest := sha256.Sum256([]byte(seedInput))
		seed := int64(binary.BigEndian.Uint64(digest[:8]) & uint64(^uint64(0)>>1))
		random := pseudorand.New(pseudorand.NewSource(seed))
		order := random.Perm(len(roomActivityAliases))
		for index := range plans {
			name := roomActivityAliases[order[index%len(order)]]
			if cycle := index / len(order); cycle > 0 {
				name += fmt.Sprintf("-%02d", cycle+1)
			}
			balance := config.balanceMin
			if config.balanceMax > config.balanceMin {
				balance += random.Int63n(config.balanceMax - config.balanceMin + 1)
			}
			plans[index] = robotResetPlan{nickname: name, balanceCents: balance}
		}
	}
	seen := make(map[string]struct{}, len(plans))
	for _, plan := range plans {
		if strings.TrimSpace(plan.nickname) == "" || len([]rune(plan.nickname)) > 50 {
			return nil, apperrors.NewBusinessError("INVALID_NICKNAME", "生成的机器人昵称应为 1–50 个字符")
		}
		if _, exists := seen[plan.nickname]; exists {
			return nil, apperrors.NewBusinessError("DUPLICATE_NICKNAME", "生成的机器人昵称不能重复")
		}
		seen[plan.nickname] = struct{}{}
	}
	return plans, nil
}

func robotResetReferences(workspaceID uint64, config normalizedRobotReset) (string, string) {
	requestIDHash := robotResetRequestIDHash(config.requestID)
	prefix := fmt.Sprintf("robot-reset:%d:%s:", workspaceID, requestIDHash[:32])
	return prefix, prefix + config.payloadHash
}

func robotResetRequestIDHash(requestID string) string {
	digest := sha256.Sum256([]byte(requestID))
	return hex.EncodeToString(digest[:])
}

func robotResetModeLabel(mode string) string {
	if mode == "custom" {
		return "自定义"
	}
	return "随机"
}

func (s *UserAdminService) updateRobot(id uint64, ownerWorkspaceID *uint64, input UpdateRobotInput) (*AdminUser, error) {
	nickname := strings.TrimSpace(input.Nickname)
	if nickname == "" || len([]rune(nickname)) > 50 {
		return nil, apperrors.NewBusinessError("INVALID_NICKNAME", "机器人昵称应为 1–50 个字符")
	}
	activeStart := strings.TrimSpace(input.ActiveStart)
	activeEnd := strings.TrimSpace(input.ActiveEnd)
	if (activeStart == "") != (activeEnd == "") {
		return nil, apperrors.NewBusinessError("INVALID_SCHEDULE", "运行开始和结束时间需要同时填写")
	}
	if activeStart != "" {
		if _, err := time.Parse("15:04", activeStart); err != nil {
			return nil, apperrors.NewBusinessError("INVALID_SCHEDULE", "运行开始时间格式不正确")
		}
		if _, err := time.Parse("15:04", activeEnd); err != nil {
			return nil, apperrors.NewBusinessError("INVALID_SCHEDULE", "运行结束时间格式不正确")
		}
	}
	if math.IsNaN(input.MinBet) || math.IsInf(input.MinBet, 0) || math.IsNaN(input.MaxBet) || math.IsInf(input.MaxBet, 0) || input.MinBet < 0 || input.MaxBet < 0 || input.MaxBet > 1000000 {
		return nil, apperrors.NewBusinessError("INVALID_BET_RANGE", "单注金额范围不正确")
	}
	if (input.MinBet == 0) != (input.MaxBet == 0) {
		return nil, apperrors.NewBusinessError("INVALID_BET_RANGE", "最小和最大单注需要同时填写")
	}
	if input.MinBet > 0 && input.MaxBet > 0 && input.MinBet > input.MaxBet {
		return nil, apperrors.NewBusinessError("INVALID_BET_RANGE", "最小单注不能大于最大单注")
	}
	minBetCents := int64(math.Round(input.MinBet * 100))
	maxBetCents := int64(math.Round(input.MaxBet * 100))
	cleanIDs := make([]string, 0, len(input.GameIDs))
	seen := map[string]struct{}{}
	for _, raw := range input.GameIDs {
		gameID := strings.TrimSpace(raw)
		if gameID == "" {
			continue
		}
		if _, exists := seen[gameID]; exists {
			continue
		}
		seen[gameID] = struct{}{}
		cleanIDs = append(cleanIDs, gameID)
	}
	avatar := strings.TrimSpace(input.Avatar)
	if avatar != "" && !strings.HasPrefix(avatar, "/images/avatars/") {
		return nil, apperrors.NewBusinessError("INVALID_AVATAR", "请选择头像库中的机器人头像")
	}
	encoded, err := json.Marshal(cleanIDs)
	if err != nil {
		return nil, err
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var account user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, id).Error; err != nil {
			return err
		}
		var profile workspacemodel.RobotProfile
		profileQuery := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", id)
		if ownerWorkspaceID != nil {
			profileQuery = profileQuery.Where("workspace_id = ?", *ownerWorkspaceID)
		}
		if err := profileQuery.First(&profile).Error; err != nil {
			return apperrors.NewBusinessError("NOT_ROBOT", "该账号不是独立机器人")
		}
		if account.WorkspaceID != profile.WorkspaceID {
			return apperrors.NewBusinessError("ROBOT_SCOPE_MISMATCH", "机器人账号房间归属异常")
		}
		if account.Role != "member" {
			return apperrors.NewBusinessError("NOT_ROBOT", "该账号不是房间机器人")
		}
		if ownerWorkspaceID != nil {
			var workspace workspacemodel.Workspace
			if err := tx.Select("id", "robot_quota").First(&workspace, profile.WorkspaceID).Error; err != nil {
				return err
			}
			if workspace.RobotQuota == 0 {
				return apperrors.NewBusinessError("ROBOT_QUOTA_REQUIRED", "上级尚未分配机器人名额")
			}
			var allocated int64
			if err := tx.Model(&workspacemodel.RobotProfile{}).
				Where("id = ? AND id IN (?)", profile.ID, allocatedRobotProfileIDs(tx, profile.WorkspaceID, workspace.RobotQuota)).
				Count(&allocated).Error; err != nil {
				return err
			}
			if allocated != 1 {
				return apperrors.NewBusinessError("ROBOT_QUOTA_EXCEEDED", "该机器人不在上级分配的名额内")
			}
		}
		if err := validateRobotGameIDsForWorkspace(tx, profile.WorkspaceID, cleanIDs); err != nil {
			return err
		}
		if err := tx.Model(&account).Updates(map[string]any{
			"nickname": nickname, "status": normalizeStatus(input.Status), "robot_game_ids_json": string(encoded),
			"robot_active_start": activeStart, "robot_active_end": activeEnd,
			"robot_min_bet_cents": minBetCents, "robot_max_bet_cents": maxBetCents,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&profile).Updates(map[string]any{
			"avatar": avatar, "enabled": normalizeStatus(input.Status) == 1,
			"active_start": activeStart, "active_end": activeEnd,
			"min_bet_cents": minBetCents, "max_bet_cents": maxBetCents,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("robot_id = ?", profile.ID).Delete(&workspacemodel.RobotGame{}).Error; err != nil {
			return err
		}
		for _, gameID := range cleanIDs {
			if err := tx.Create(&workspacemodel.RobotGame{RobotID: profile.ID, GameID: gameID}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&chat.Message{}).Where("workspace_id = ? AND user_id = ?", profile.WorkspaceID, id).Update("nickname", nickname).Error
	})
	if err != nil {
		return nil, err
	}
	result, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if ownerWorkspaceID != nil && result.WorkspaceID != *ownerWorkspaceID {
		return nil, apperrors.NewBusinessError("FORBIDDEN", "该机器人不属于当前房间")
	}
	return result, nil
}

func validateRobotGameIDsForWorkspace(db *gorm.DB, workspaceID uint64, gameIDs []string) error {
	if len(gameIDs) == 0 {
		return nil
	}
	views, err := NewWorkspaceGameService(db).List(workspaceID)
	if err != nil {
		return err
	}
	enabled := make(map[string]struct{}, len(views))
	for _, view := range views {
		if view.Enabled {
			enabled[view.ID] = struct{}{}
		}
	}
	for _, gameID := range gameIDs {
		if _, ok := enabled[gameID]; !ok {
			return apperrors.NewBusinessError("INVALID_GAME", "参与彩种不存在或未在当前房间开放")
		}
	}
	return nil
}

func (s *UserAdminService) enrichRobotProfiles(items []AdminUser) error {
	ids := make([]uint64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	var profiles []workspacemodel.RobotProfile
	if err := s.db.Where("user_id IN ?", ids).Find(&profiles).Error; err != nil {
		return err
	}
	byUser := make(map[uint64]workspacemodel.RobotProfile, len(profiles))
	profileIDs := make([]uint64, 0, len(profiles))
	for _, profile := range profiles {
		byUser[profile.UserID] = profile
		profileIDs = append(profileIDs, profile.ID)
	}
	gamesByRobot := map[uint64][]string{}
	if len(profileIDs) > 0 {
		var rows []workspacemodel.RobotGame
		if err := s.db.Where("robot_id IN ?", profileIDs).Order("id ASC").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			gamesByRobot[row.RobotID] = append(gamesByRobot[row.RobotID], row.GameID)
		}
	}
	for index := range items {
		profile, ok := byUser[items[index].ID]
		if !ok {
			continue
		}
		items[index].IsRobot = true
		items[index].WorkspaceID = profile.WorkspaceID
		items[index].RobotAvatar = profile.Avatar
		if strings.TrimSpace(items[index].Avatar) == "" {
			items[index].Avatar = profile.Avatar
		}
		items[index].RobotGameIDs = gamesByRobot[profile.ID]
		items[index].RobotActiveStart = profile.ActiveStart
		items[index].RobotActiveEnd = profile.ActiveEnd
		items[index].RobotMinBet = centsToAmount(profile.MinBetCents)
		items[index].RobotMaxBet = centsToAmount(profile.MaxBetCents)
	}
	return nil
}

func (s *UserAdminService) enrichOwnership(items []AdminUser) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]uint64, 0, len(items)*2)
	workspaceIDs := make([]uint64, 0, len(items))
	for _, item := range items {
		if item.WorkspaceID > 0 {
			workspaceIDs = append(workspaceIDs, item.WorkspaceID)
		}
		if item.ParentAgentID != nil {
			ids = append(ids, *item.ParentAgentID)
		}
		if item.ParentTenantID != nil {
			ids = append(ids, *item.ParentTenantID)
		}
	}
	owners := map[uint64]user.User{}
	if len(ids) > 0 {
		var rows []user.User
		if err := s.db.Select("user_id", "username", "nickname", "parent_tenant_id", "agent_room_code").Where("user_id IN ?", ids).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			owners[row.UserID] = row
		}
		for _, row := range rows {
			if row.ParentTenantID != nil {
				ids = append(ids, *row.ParentTenantID)
			}
		}
		var more []user.User
		if err := s.db.Select("user_id", "username", "nickname", "agent_room_code", "parent_tenant_id").Where("user_id IN ?", ids).Find(&more).Error; err != nil {
			return err
		}
		for _, row := range more {
			owners[row.UserID] = row
		}
	}
	workspacesByID := map[uint64]workspacemodel.Workspace{}
	workspacesByOwner := map[uint64]workspacemodel.Workspace{}
	workspaceQuery := s.db.Model(&workspacemodel.Workspace{})
	switch {
	case len(workspaceIDs) > 0 && len(ids) > 0:
		workspaceQuery = workspaceQuery.Where("id IN ? OR owner_user_id IN ?", workspaceIDs, ids)
	case len(workspaceIDs) > 0:
		workspaceQuery = workspaceQuery.Where("id IN ?", workspaceIDs)
	case len(ids) > 0:
		workspaceQuery = workspaceQuery.Where("owner_user_id IN ?", ids)
	default:
		workspaceQuery = nil
	}
	if workspaceQuery != nil {
		var rows []workspacemodel.Workspace
		if err := workspaceQuery.Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			workspacesByID[row.ID] = row
			workspacesByOwner[row.OwnerUserID] = row
		}
	}
	for index := range items {
		item := &items[index]
		prefix := "平台"
		if workspace, ok := workspacesByID[item.WorkspaceID]; ok && workspace.Type != workspacemodel.TypePlatform {
			item.RoomCode = workspace.RoomCode
		}
		if item.Role == "tenant" {
			prefix = defaultString(item.Nickname, item.Username)
			item.TenantName = prefix
			if workspace, ok := workspacesByOwner[item.ID]; ok {
				item.RoomCode = workspace.RoomCode
			}
		}
		if item.ParentTenantID != nil {
			if tenant, ok := owners[*item.ParentTenantID]; ok {
				item.TenantName = defaultString(tenant.Nickname, tenant.Username)
				prefix = item.TenantName
			}
		}
		if item.ParentAgentID != nil {
			if agent, ok := owners[*item.ParentAgentID]; ok {
				item.AgentName = defaultString(agent.Nickname, agent.Username)
				if workspace, exists := workspacesByOwner[agent.UserID]; exists {
					item.RoomCode = workspace.RoomCode
				}
				prefix = item.AgentName
				if agent.ParentTenantID != nil {
					if tenant, ok := owners[*agent.ParentTenantID]; ok {
						item.TenantName = defaultString(tenant.Nickname, tenant.Username)
					}
				}
			}
		}
		item.LoginIdentity = prefix + " / " + item.Username
	}
	return nil
}

func (s *UserAdminService) Create(input CreateAdminUserInput) (*AdminUser, error) {
	input.Username = strings.TrimSpace(input.Username)
	input.Email = strings.TrimSpace(input.Email)
	if err := validateHumanUsername(input.Username); err != nil {
		return nil, err
	}
	if err := utils.ValidatePassword(input.Password); err != nil {
		return nil, apperrors.NewBusinessError("INVALID_PASSWORD", "密码长度需为 8–72 个字符")
	}
	role, err := validRole(input.Role)
	if err != nil {
		return nil, err
	}
	// Tenant and agent accounts own workspaces and therefore require the
	// dedicated account services that create the owner, workspace, membership
	// and settings atomically. The generic user endpoint is intentionally only
	// a member-account entry point; allowing a caller to choose an owner role
	// here would create an account without its required ownership topology.
	if role != "member" {
		return nil, apperrors.NewBusinessError("DEDICATED_ACCOUNT_SERVICE_REQUIRED", "租户、代理和管理员账号必须通过对应的专用开户流程创建")
	}
	risk, err := validRisk(input.RiskLevel)
	if err != nil {
		return nil, err
	}
	hash, err := utils.HashPassword(input.Password)
	if err != nil {
		return nil, err
	}
	var row user.User
	err = s.db.Transaction(func(tx *gorm.DB) error {
		loginScope := platformLoginScope
		var parentAgentID *uint64
		var parentTenantID *uint64
		var targetRoom *workspacemodel.Workspace
		if role == "member" && input.ParentAgentID > 0 {
			var agent user.User
			if lookupErr := tx.Clauses(clause.Locking{Strength: "SHARE"}).
				Select("user_id", "parent_tenant_id", "role", "status").
				Where("user_id = ? AND role = ? AND status = ?", input.ParentAgentID, "agent", 1).
				First(&agent).Error; lookupErr != nil {
				if lookupErr == gorm.ErrRecordNotFound {
					return apperrors.NewBusinessError("ROOM_NOT_FOUND", "房间不存在或已停用")
				}
				return lookupErr
			}
			var room workspacemodel.Workspace
			if lookupErr := tx.Clauses(clause.Locking{Strength: "SHARE"}).
				Where("owner_user_id = ? AND type = ? AND status = ?", agent.UserID, workspacemodel.TypeAgent, 1).
				First(&room).Error; lookupErr != nil {
				if lookupErr == gorm.ErrRecordNotFound {
					return apperrors.NewBusinessError("ROOM_NOT_FOUND", "房间不存在或已停用")
				}
				return lookupErr
			}
			expectedScope := agentLoginScope(agent.UserID)
			if room.Scope != expectedScope {
				return apperrors.NewBusinessError("ROOM_NOT_FOUND", "房间归属异常，请先修复代理工作区")
			}
			loginScope = room.Scope
			parentAgentID = &agent.UserID
			parentTenantID = agent.ParentTenantID
			targetRoom = &room
		} else {
			// A member created without an agent still belongs to the platform
			// lobby. Never leave an interactive identity at workspace_id=0: the
			// zero value is reserved for platform-wide service filters and cannot
			// safely identify a member's current room.
			var platform workspacemodel.Workspace
			if lookupErr := tx.Clauses(clause.Locking{Strength: "SHARE"}).
				Where("type = ? AND status = ?", workspacemodel.TypePlatform, 1).
				Order("id ASC").First(&platform).Error; lookupErr != nil {
				if lookupErr == gorm.ErrRecordNotFound {
					return apperrors.NewBusinessError("WORKSPACE_NOT_FOUND", "平台工作区不存在或已停用")
				}
				return lookupErr
			}
			loginScope = platformLoginScope
			targetRoom = &platform
		}
		if uniqueErr := NewUserAdminService(tx).ensureUnique(0, loginScope, input.Username, input.Email); uniqueErr != nil {
			return uniqueErr
		}
		row = user.User{Username: input.Username, LoginScope: loginScope, Password: hash, Email: input.Email, Nickname: strings.TrimSpace(input.Nickname), Phone: strings.TrimSpace(input.Phone), Role: role, Remark: strings.TrimSpace(input.Remark), RiskLevel: risk, Status: normalizeStatus(input.Status), ParentAgentID: parentAgentID, ParentTenantID: parentTenantID}
		if createErr := tx.Create(&row).Error; createErr != nil {
			return createErr
		}
		if membershipErr := ActivateWorkspaceMembership(tx, &row, *targetRoom); membershipErr != nil {
			return membershipErr
		}
		if membershipErr := syncCurrentWorkspaceMembershipStatus(tx, row, row.Status); membershipErr != nil {
			return membershipErr
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	result := adminUser(row)
	return &result, nil
}

func (s *UserAdminService) Update(id uint64, input UpdateAdminUserInput) (*AdminUser, error) {
	var row user.User
	if err := s.db.First(&row, id).Error; err != nil {
		return nil, err
	}
	input.Email = strings.TrimSpace(input.Email)
	if err := s.ensureUnique(id, row.LoginScope, "", input.Email); err != nil {
		return nil, err
	}
	role, err := validRole(input.Role)
	if err != nil {
		return nil, err
	}
	if role != row.Role {
		return nil, apperrors.NewBusinessError("ROLE_CHANGE_NOT_ALLOWED", "账号角色不能在会员资料中修改，请使用对应的专用转换流程")
	}
	risk, err := validRisk(input.RiskLevel)
	if err != nil {
		return nil, err
	}
	nickname := strings.TrimSpace(input.Nickname)
	targetStatus := normalizeStatus(input.Status)
	updates := map[string]any{"email": input.Email, "nickname": nickname, "phone": strings.TrimSpace(input.Phone), "remark": strings.TrimSpace(input.Remark), "risk_level": risk, "status": targetStatus}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var locked user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, id).Error; err != nil {
			return err
		}
		if locked.Role != role {
			return apperrors.NewBusinessError("ROLE_CHANGE_NOT_ALLOWED", "账号角色不能在会员资料中修改，请使用对应的专用转换流程")
		}
		if err := tx.Model(&locked).Updates(updates).Error; err != nil {
			return err
		}
		if err := syncCurrentWorkspaceMembershipStatus(tx, locked, targetStatus); err != nil {
			return err
		}
		return tx.Model(&chat.Message{}).Where("user_id = ?", id).Update("nickname", nickname).Error
	}); err != nil {
		return nil, err
	}
	ws.DisconnectUser(id)
	return s.Get(id)
}

func (s *UserAdminService) SetStatus(id uint64, status int) (*AdminUser, error) {
	return s.setStatus(id, status, nil)
}

// SetStatusOwned performs the ownership check while the user row is locked.
// This prevents a former room agent from changing a member after an admin has
// concurrently reassigned that member to another room.
func (s *UserAdminService) SetStatusOwned(id, ownerAgentID uint64, status int) (*AdminUser, error) {
	return s.setStatus(id, status, &ownerAgentID)
}

func (s *UserAdminService) SetStatusInWorkspace(id, workspaceID uint64, status int) (*AdminUser, error) {
	if workspaceID == 0 {
		return nil, apperrors.NewBusinessError("FORBIDDEN", "请选择当前房间")
	}
	var result *AdminUser
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var row user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		if row.WorkspaceID != workspaceID || row.Role != "member" {
			return apperrors.NewBusinessError("FORBIDDEN", "该用户不属于当前房间")
		}
		targetStatus := normalizeStatus(status)
		if err := tx.Model(&row).Update("status", targetStatus).Error; err != nil {
			return err
		}
		if err := syncCurrentWorkspaceMembershipStatus(tx, row, targetStatus); err != nil {
			return err
		}
		// Read the response while the membership/account row is still locked;
		// a concurrent switch must not enrich it with the destination room.
		var err error
		result, err = NewUserAdminService(tx).Get(id)
		return err
	})
	if err != nil {
		return nil, err
	}
	ws.DisconnectUser(id)
	return result, nil
}

func (s *UserAdminService) setStatus(id uint64, status int, ownerAgentID *uint64) (*AdminUser, error) {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var row user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		if err := requireRoomOwnership(row, ownerAgentID); err != nil {
			return err
		}
		targetStatus := normalizeStatus(status)
		if err := tx.Model(&row).Update("status", targetStatus).Error; err != nil {
			return err
		}
		return syncCurrentWorkspaceMembershipStatus(tx, row, targetStatus)
	})
	if err != nil {
		return nil, err
	}
	ws.DisconnectUser(id)
	return s.Get(id)
}

// syncCurrentWorkspaceMembershipStatus keeps the account row and the current
// membership as one authorization fact. A missing membership is not repaired
// silently here: status changes must fail and roll back so the hierarchy audit
// can surface the broken account instead of certifying a partial update.
func syncCurrentWorkspaceMembershipStatus(tx *gorm.DB, account user.User, status int) error {
	if account.UserID == 0 || account.WorkspaceID == 0 {
		return apperrors.NewBusinessError("WORKSPACE_MEMBERSHIP_MISSING", "账号尚未绑定有效工作区")
	}
	result := tx.Model(&workspacemodel.Membership{}).
		Where("workspace_id = ? AND user_id = ?", account.WorkspaceID, account.UserID).
		Update("status", normalizeStatus(status))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return apperrors.NewBusinessError("WORKSPACE_MEMBERSHIP_MISSING", "账号当前工作区成员关系不存在")
	}
	return nil
}

func (s *UserAdminService) ResetPassword(id uint64, password string) error {
	if err := utils.ValidatePassword(password); err != nil {
		return apperrors.NewBusinessError("INVALID_PASSWORD", "密码长度需为 8–72 个字符")
	}
	hash, err := utils.HashPassword(password)
	if err != nil {
		return err
	}
	result := s.db.Model(&user.User{}).Where("user_id = ?", id).Updates(passwordSessionUpdate(hash))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	ws.DisconnectUser(id)
	return nil
}

func (s *UserAdminService) AdjustBalance(id uint64, amount float64, remark, operator string) (*AdminUser, error) {
	return s.adjustBalance(id, amount, remark, operator, nil)
}

// AdjustBalanceOwned is the room-agent variant. The ownership check, balance
// change and ledger insert are deliberately one locked transaction.
func (s *UserAdminService) AdjustBalanceOwned(id, ownerAgentID uint64, amount float64, remark, operator string) (*AdminUser, error) {
	return s.adjustBalance(id, amount, remark, operator, &ownerAgentID)
}

func (s *UserAdminService) AdjustBalanceInWorkspace(id, workspaceID uint64, amount float64, remark, operator string) (*AdminUser, error) {
	if workspaceID == 0 {
		return nil, apperrors.NewBusinessError("FORBIDDEN", "请选择当前房间")
	}
	amountCents := int64(math.Round(amount * 100))
	if amountCents == 0 || math.Abs(amount) > 100000000 {
		return nil, apperrors.NewBusinessError("INVALID_AMOUNT", "调整金额不正确")
	}
	var result *AdminUser
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var row user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		if row.WorkspaceID != workspaceID || row.Role != "member" {
			return apperrors.NewBusinessError("FORBIDDEN", "该用户不属于当前房间")
		}
		after, record := manualBalanceRecord(row, workspaceID, amountCents, remark, operator)
		if after < 0 {
			return apperrors.NewBusinessError("INSUFFICIENT_BALANCE", "扣减金额不能超过当前余额")
		}
		if err := tx.Model(&row).Update("balance_cents", after).Error; err != nil {
			return err
		}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		var err error
		result, err = NewUserAdminService(tx).Get(id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *UserAdminService) adjustBalance(id uint64, amount float64, remark, operator string, ownerAgentID *uint64) (*AdminUser, error) {
	amountCents := int64(math.Round(amount * 100))
	if amountCents == 0 {
		return nil, apperrors.NewBusinessError("INVALID_AMOUNT", "调整金额不能为 0")
	}
	if math.Abs(amount) > 100000000 {
		return nil, apperrors.NewBusinessError("INVALID_AMOUNT", "单次调整金额超出限制")
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var row user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		if err := requireRoomOwnership(row, ownerAgentID); err != nil {
			return err
		}
		after, record := manualBalanceRecord(row, row.WorkspaceID, amountCents, remark, operator)
		if after < 0 {
			return apperrors.NewBusinessError("INSUFFICIENT_BALANCE", "扣减金额不能超过当前余额")
		}
		if err := tx.Model(&row).Update("balance_cents", after).Error; err != nil {
			return err
		}
		return tx.Create(&record).Error
	})
	if err != nil {
		return nil, err
	}
	return s.Get(id)
}

// manualBalanceRecord freezes the ledger snapshot before GORM updates the
// loaded account struct in memory. Building the row after Model(&account).Update
// would make before_cents equal after_cents and violate the financial invariant.
func manualBalanceRecord(row user.User, workspaceID uint64, amountCents int64, remark, operator string) (int64, user.BalanceTransaction) {
	before := row.BalanceCents
	after := before + amountCents
	return after, user.BalanceTransaction{
		WorkspaceID: workspaceID,
		UserID:      row.UserID,
		AmountCents: amountCents,
		BeforeCents: before,
		AfterCents:  after,
		Type:        "manual",
		Remark:      strings.TrimSpace(remark),
		Operator:    strings.TrimSpace(operator),
	}
}

func requireRoomOwnership(account user.User, ownerAgentID *uint64) error {
	if ownerAgentID != nil && (account.ParentAgentID == nil || *account.ParentAgentID != *ownerAgentID) {
		return apperrors.NewBusinessError("FORBIDDEN", "该用户不属于当前房间")
	}
	return nil
}

func (s *UserAdminService) BalanceHistory(id uint64, limit int) ([]BalanceRecord, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	var rows []user.BalanceTransaction
	if err := s.db.Where("user_id = ?", id).Order("created_at desc").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]BalanceRecord, 0, len(rows))
	for _, row := range rows {
		result = append(result, BalanceRecord{ID: row.ID, UserID: row.UserID, Reference: row.Reference, Amount: centsToAmount(row.AmountCents), Before: centsToAmount(row.BeforeCents), After: centsToAmount(row.AfterCents), Type: row.Type, Remark: row.Remark, Operator: row.Operator, CreatedAt: row.CreatedAt})
	}
	return result, nil
}

// BalanceHistoryPage lets member wallets request a bounded page of ledger
// records, rather than rendering an entire account history at once.
func (s *UserAdminService) BalanceHistoryPage(id uint64, limit int, beforeID uint64) (*BalanceHistoryPage, error) {
	if limit < 1 || limit > 50 {
		limit = 20
	}
	var rows []user.BalanceTransaction
	query := s.db.Where("user_id = ?", id)
	if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}
	if err := query.Order("id desc").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	items := make([]BalanceRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, BalanceRecord{ID: row.ID, UserID: row.UserID, Reference: row.Reference, Amount: centsToAmount(row.AmountCents), Before: centsToAmount(row.BeforeCents), After: centsToAmount(row.AfterCents), Type: row.Type, Remark: row.Remark, Operator: row.Operator, CreatedAt: row.CreatedAt})
	}
	page := &BalanceHistoryPage{Items: items, HasMore: hasMore}
	if len(items) > 0 {
		page.NextBeforeID = items[len(items)-1].ID
	}
	return page, nil
}

func (s *UserAdminService) ensureUnique(excludeID uint64, scope, username, email string) error {
	if username != "" {
		if err := ensureUsernameInScope(s.db, defaultString(scope, platformLoginScope), username, excludeID); err != nil {
			return err
		}
	}
	if email != "" {
		query := s.db.Model(&user.User{}).Where("LOWER(email) = LOWER(?)", email)
		if excludeID > 0 {
			query = query.Where("user_id <> ?", excludeID)
		}
		var count int64
		if err := query.Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return apperrors.NewBusinessError("EMAIL_EXISTS", "邮箱已被使用")
		}
	}
	return nil
}

func validRole(value string) (string, error) {
	if value == "" {
		value = "member"
	}
	switch value {
	case "member", "agent", "tenant", "admin":
		return value, nil
	default:
		return "", apperrors.NewBusinessError("INVALID_ROLE", "账号角色不正确")
	}
}

func validRisk(value string) (string, error) {
	if value == "" {
		value = "normal"
	}
	switch value {
	case "normal", "watch", "restricted":
		return value, nil
	default:
		return "", apperrors.NewBusinessError("INVALID_RISK", "风控等级不正确")
	}
}

func normalizeStatus(value int) int {
	if value == 0 {
		return 0
	}
	return 1
}

func adminUser(row user.User) AdminUser {
	gameIDs := []string{}
	_ = json.Unmarshal([]byte(defaultString(row.RobotGameIDsJSON, "[]")), &gameIDs)
	return AdminUser{
		ID: row.UserID, PublicID: row.PublicID, Username: row.Username, Email: row.Email, Nickname: row.Nickname,
		Avatar: row.Avatar, PublicTitle: row.PublicTitle, PublicBadge: row.PublicBadge, Phone: row.Phone,
		Role: defaultString(row.Role, "member"), Remark: row.Remark, RiskLevel: defaultString(row.RiskLevel, "normal"),
		Balance: centsToAmount(row.BalanceCents), FlyMode: defaultString(row.FlyMode, "inherit"), FlyRate: row.FlyRate,
		AgentRoomCode: row.AgentRoomCode, ParentAgentID: row.ParentAgentID, ParentTenantID: row.ParentTenantID,
		Status: row.Status, LastLoginAt: row.LastLoginAt, LoginCount: row.LoginCount, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		IsRobot: false, RobotGameIDs: gameIDs,
		RobotActiveStart: row.RobotActiveStart, RobotActiveEnd: row.RobotActiveEnd,
		RobotMinBet: centsToAmount(row.RobotMinBetCents), RobotMaxBet: centsToAmount(row.RobotMaxBetCents),
		WorkspaceID: row.WorkspaceID,
	}
}

func populateAdminUserPresence(items []AdminUser) {
	userIDs := make([]uint64, 0, len(items))
	for _, item := range items {
		if item.ID != 0 {
			userIDs = append(userIDs, item.ID)
		}
	}
	online := ws.OnlineUsers(userIDs)
	for index := range items {
		items[index].Online = online[items[index].ID]
	}
}

func centsToAmount(cents int64) float64 { return float64(cents) / 100 }

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func ParseUserID(value string) (uint64, error) {
	var id uint64
	if _, err := fmt.Sscan(value, &id); err != nil || id == 0 {
		return 0, apperrors.NewBusinessError("INVALID_USER_ID", "用户编号不正确")
	}
	return id, nil
}
