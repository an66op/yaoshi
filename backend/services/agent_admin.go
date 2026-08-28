package services

import (
	"backend/data/models/special"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"backend/utils"
	"backend/ws"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AgentAdminService struct{ db *gorm.DB }

type AgentView struct {
	ID              uint64  `json:"id"`
	PublicID        uint64  `json:"public_id"`
	Username        string  `json:"username"`
	Email           string  `json:"email"`
	Nickname        string  `json:"nickname"`
	Phone           string  `json:"phone"`
	RoomCode        string  `json:"room_code"`
	RoomName        string  `json:"room_name"`
	RoomLogo        string  `json:"room_logo"`
	WorkspaceID     uint64  `json:"workspace_id"`
	Balance         float64 `json:"balance"`
	Status          int     `json:"status"`
	MemberCount     int64   `json:"member_count"`
	RebateRate      float64 `json:"rebate_rate"`
	ProfitShareRate float64 `json:"profit_share_rate"`
	Remark          string  `json:"remark"`
	CreatedAt       string  `json:"created_at"`
	LastLoginAt     string  `json:"last_login_at"`
	LoginCount      int     `json:"login_count"`
	TenantID        *uint64 `json:"tenant_id,omitempty"`
	TenantName      string  `json:"tenant_name,omitempty"`
}

type AgentSummary struct {
	Total    int64 `json:"total"`
	Active   int64 `json:"active"`
	Disabled int64 `json:"disabled"`
	Members  int64 `json:"members"`
}

type AgentListResult struct {
	Items    []AgentView  `json:"items"`
	Total    int64        `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
	Summary  AgentSummary `json:"summary"`
}

type CreateAgentInput struct {
	Username        string
	Password        string
	Email           string
	Nickname        string
	Phone           string
	RoomCode        string
	RoomName        string
	RoomLogo        string
	Remark          string
	Status          int
	RebateRate      float64
	ProfitShareRate float64
	TenantID        *uint64
}

type UpdateAgentInput struct {
	Email           string
	Nickname        string
	Phone           string
	RoomCode        string
	RoomName        string
	RoomLogo        string
	Remark          string
	Status          int
	RebateRate      float64
	ProfitShareRate float64
	TenantID        *uint64
}

func NewAgentAdminService(db *gorm.DB) *AgentAdminService { return &AgentAdminService{db: db} }

func (s *AgentAdminService) List(query string, page, pageSize int) (*AgentListResult, error) {
	return s.list(query, page, pageSize, nil)
}

func (s *AgentAdminService) ListForTenant(tenantID uint64, query string, page, pageSize int) (*AgentListResult, error) {
	return s.list(query, page, pageSize, &tenantID)
}

func (s *AgentAdminService) list(query string, page, pageSize int, tenantID *uint64) (*AgentListResult, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	q := s.db.Model(&user.User{}).Where("role = ?", "agent")
	if tenantID != nil {
		q = q.Where("parent_tenant_id = ?", *tenantID)
	}
	if text := strings.TrimSpace(query); text != "" {
		like := "%" + text + "%"
		q = q.Where("username ILIKE ? OR nickname ILIKE ? OR agent_room_code ILIKE ? OR phone ILIKE ?", like, like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, apperrors.NewSystemError("AGENT_READ_FAILED", "读取代理列表失败", err)
	}
	var rows []user.User
	if err := q.Order("user_id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("AGENT_READ_FAILED", "读取代理列表失败", err)
	}
	items := make([]AgentView, 0, len(rows))
	for _, row := range rows {
		var members int64
		_ = s.db.Model(&user.User{}).Where("parent_agent_id = ?", row.UserID).Count(&members).Error
		roomCode, roomName, roomLogo, workspaceID := row.AgentRoomCode, agentRoomDisplayName(row), row.AgentRoomLogo, row.WorkspaceID
		if workspace, err := WorkspaceForAccount(s.db, row); err == nil {
			roomCode, roomName, roomLogo, workspaceID = workspace.RoomCode, workspace.Name, workspace.Logo, workspace.ID
		}
		view := AgentView{
			ID: row.UserID, PublicID: row.PublicID, Username: row.Username, Email: row.Email, Nickname: row.Nickname, Phone: row.Phone,
			RoomCode: roomCode, RoomName: roomName, RoomLogo: roomLogo, WorkspaceID: workspaceID, Balance: centsToAmount(row.BalanceCents), Status: row.Status,
			MemberCount: members, RebateRate: row.RoomRebateRate, ProfitShareRate: row.RoomProfitShareRate, Remark: row.Remark, CreatedAt: row.CreatedAt.Format("2006-01-02 15:04:05"), LoginCount: row.LoginCount, TenantID: row.ParentTenantID,
		}
		if row.ParentTenantID != nil {
			var tenant user.User
			if s.db.Select("username", "nickname").First(&tenant, *row.ParentTenantID).Error == nil {
				view.TenantName = firstNonEmpty(tenant.Nickname, tenant.Username)
			}
		}
		items = append(items, view)
		if row.LastLoginAt != nil {
			items[len(items)-1].LastLoginAt = row.LastLoginAt.Local().Format("2006-01-02 15:04:05")
		}
	}
	summary := AgentSummary{Total: total}
	summaryQuery := s.db.Model(&user.User{}).Where("role = ?", "agent")
	if tenantID != nil {
		summaryQuery = summaryQuery.Where("parent_tenant_id = ?", *tenantID)
	}
	_ = summaryQuery.Where("status = ?", 1).Count(&summary.Active).Error
	summary.Disabled = summary.Total - summary.Active
	memberQuery := s.db.Model(&user.User{}).Where("parent_agent_id IS NOT NULL")
	if tenantID != nil {
		memberQuery = memberQuery.Where("parent_agent_id IN (?)", s.db.Model(&user.User{}).Select("user_id").Where("role = ? AND parent_tenant_id = ?", "agent", *tenantID))
	}
	_ = memberQuery.Count(&summary.Members).Error
	return &AgentListResult{Items: items, Total: total, Page: page, PageSize: pageSize, Summary: summary}, nil
}

func (s *AgentAdminService) CreateForTenant(tenantID uint64, input CreateAgentInput) (*AgentView, error) {
	input.TenantID = &tenantID
	return s.Create(input)
}

func (s *AgentAdminService) Create(input CreateAgentInput) (*AgentView, error) {
	input.Username = strings.TrimSpace(input.Username)
	input.Email = strings.TrimSpace(input.Email)
	input.RoomCode = normalizeAgentRoomCode(input.RoomCode)
	input.RoomName = normalizeAgentRoomName(input.RoomName)
	roomLogo, logoErr := normalizeRoomLogo(input.RoomLogo)
	if logoErr != nil {
		return nil, logoErr
	}
	input.RoomLogo = roomLogo
	if err := validateAgentRoomName(input.RoomName); err != nil {
		return nil, err
	}
	if len(input.Username) < 3 || len(input.Username) > 50 {
		return nil, apperrors.NewBusinessError("INVALID_USERNAME", "登录账号长度应为 3–50 个字符")
	}
	if err := utils.ValidatePassword(input.Password); err != nil {
		return nil, apperrors.NewBusinessError("INVALID_PASSWORD", "登录密码长度需为 8–72 个字符")
	}
	if err := validateAgentRoomCode(input.RoomCode); err != nil {
		return nil, err
	}
	if input.RebateRate < 0 || input.RebateRate > 100 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "房间返水比例需在 0-100 之间")
	}
	if input.ProfitShareRate < 0 || input.ProfitShareRate > 100 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "代理分成比例需在 0-100 之间")
	}
	hash, err := utils.HashPassword(input.Password)
	if err != nil {
		return nil, err
	}
	loginScope := platformLoginScope
	if input.TenantID != nil {
		loginScope = tenantLoginScope(*input.TenantID)
	}
	row := user.User{Username: input.Username, LoginScope: loginScope, Password: hash, Email: input.Email, Nickname: strings.TrimSpace(input.Nickname), Phone: strings.TrimSpace(input.Phone), Role: "agent", AgentRoomCode: input.RoomCode, AgentRoomName: input.RoomName, AgentRoomLogo: input.RoomLogo, RoomRebateRate: input.RebateRate, RoomProfitShareRate: input.ProfitShareRate, Remark: strings.TrimSpace(input.Remark), RiskLevel: "normal", Status: normalizeAgentStatus(input.Status), ParentTenantID: input.TenantID}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := lockPublicRoomCodeRegistry(tx); err != nil {
			return err
		}
		if row.ParentTenantID != nil {
			var tenant user.User
			if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).
				Where("user_id = ? AND role = ? AND status = ?", *row.ParentTenantID, "tenant", 1).
				First(&tenant).Error; err != nil {
				if err != gorm.ErrRecordNotFound {
					return err
				}
				return apperrors.NewBusinessError("TENANT_NOT_FOUND", "租户不存在或已停用")
			}
		}
		if err := ensureUsernameInScope(tx, row.LoginScope, row.Username, 0); err != nil {
			return err
		}
		var duplicate int64
		if row.Email != "" {
			if err := tx.Model(&user.User{}).Where("LOWER(email) = LOWER(?)", row.Email).Count(&duplicate).Error; err != nil {
				return err
			}
			if duplicate > 0 {
				return apperrors.NewBusinessError("EMAIL_EXISTS", "邮箱已被使用")
			}
		}
		if err := ensureRoomCodeAvailable(tx, row.AgentRoomCode, 0); err != nil {
			return err
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return EnsureWorkspaceHierarchy(tx)
	}); err != nil {
		return nil, err
	}
	return s.view(row.UserID)
}

func (s *AgentAdminService) Update(id uint64, input UpdateAgentInput) (*AgentView, error) {
	return s.update(id, input, nil)
}

func (s *AgentAdminService) update(id uint64, input UpdateAgentInput, ownerTenantID *uint64) (*AgentView, error) {
	input.Email = strings.TrimSpace(input.Email)
	input.RoomCode = normalizeAgentRoomCode(input.RoomCode)
	input.RoomName = normalizeAgentRoomName(input.RoomName)
	roomLogo, logoErr := normalizeRoomLogo(input.RoomLogo)
	if logoErr != nil {
		return nil, logoErr
	}
	input.RoomLogo = roomLogo
	if err := validateAgentRoomName(input.RoomName); err != nil {
		return nil, err
	}
	if err := validateAgentRoomCode(input.RoomCode); err != nil {
		return nil, err
	}
	if input.RebateRate < 0 || input.RebateRate > 100 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "房间返水比例需在 0-100 之间")
	}
	if input.ProfitShareRate < 0 || input.ProfitShareRate > 100 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "代理分成比例需在 0-100 之间")
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := lockPublicRoomCodeRegistry(tx); err != nil {
			return err
		}
		var row user.User
		owned := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND role = ?", id, "agent")
		if ownerTenantID != nil {
			owned = owned.Where("parent_tenant_id = ?", *ownerTenantID)
		}
		if err := owned.First(&row).Error; err != nil {
			return apperrors.NewBusinessError("USER_NOT_FOUND", "代理不存在或不属于当前租户")
		}
		if input.Email != "" {
			var duplicate int64
			if err := tx.Model(&user.User{}).Where("LOWER(email) = LOWER(?) AND user_id <> ?", input.Email, id).Count(&duplicate).Error; err != nil {
				return err
			}
			if duplicate > 0 {
				return apperrors.NewBusinessError("EMAIL_EXISTS", "邮箱已被使用")
			}
		}
		if err := ensureRoomCodeAvailable(tx, input.RoomCode, id); err != nil {
			return err
		}
		updates := map[string]any{"email": input.Email, "nickname": strings.TrimSpace(input.Nickname), "phone": strings.TrimSpace(input.Phone), "agent_room_code": input.RoomCode, "agent_room_name": input.RoomName, "agent_room_logo": input.RoomLogo, "room_rebate_rate": input.RebateRate, "room_profit_share_rate": input.ProfitShareRate, "remark": strings.TrimSpace(input.Remark), "status": normalizeAgentStatus(input.Status)}
		if ownerTenantID == nil && input.TenantID != nil {
			var tenantCount int64
			if err := tx.Model(&user.User{}).Where("user_id = ? AND role = ? AND status = ?", *input.TenantID, "tenant", 1).Count(&tenantCount).Error; err != nil {
				return err
			}
			if tenantCount == 0 {
				return apperrors.NewBusinessError("TENANT_NOT_FOUND", "租户不存在或已停用")
			}
			updates["parent_tenant_id"] = *input.TenantID
			updates["login_scope"] = tenantLoginScope(*input.TenantID)
			if err := ensureUsernameInScope(tx, tenantLoginScope(*input.TenantID), row.Username, row.UserID); err != nil {
				return err
			}
		}
		if err := tx.Model(&row).Updates(updates).Error; err != nil {
			return err
		}
		if err := EnsureWorkspaceHierarchy(tx); err != nil {
			return err
		}
		if err := tx.Where("user_id = ? AND role = ?", id, "agent").First(&row).Error; err != nil {
			return err
		}
		workspace, err := WorkspaceForAccount(tx, row)
		if err != nil {
			return err
		}
		_, err = NewSettingsAdminService(tx).UpdateRoomIdentityForWorkspace(workspace.ID, input.RoomCode, input.RoomName, input.RoomLogo)
		return err
	})
	if err != nil {
		return nil, err
	}
	ws.DisconnectUser(id)
	return s.view(id)
}

func (s *AgentAdminService) UpdateForTenant(tenantID, id uint64, input UpdateAgentInput) (*AgentView, error) {
	// A tenant may edit only an agent already owned by it and may not transfer
	// that agent to another tenant through a crafted request body.
	input.TenantID = nil
	return s.update(id, input, &tenantID)
}

func (s *AgentAdminService) ResetPassword(id uint64, password string) error {
	return s.resetPassword(id, password, nil)
}

func (s *AgentAdminService) resetPassword(id uint64, password string, ownerTenantID *uint64) error {
	if err := utils.ValidatePassword(password); err != nil {
		return apperrors.NewBusinessError("INVALID_PASSWORD", "登录密码长度需为 8–72 个字符")
	}
	hash, err := utils.HashPassword(password)
	if err != nil {
		return err
	}
	query := s.db.Model(&user.User{}).Where("user_id = ? AND role = ?", id, "agent")
	if ownerTenantID != nil {
		query = query.Where("parent_tenant_id = ?", *ownerTenantID)
	}
	result := query.Updates(passwordSessionUpdate(hash))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return apperrors.NewBusinessError("USER_NOT_FOUND", "代理不存在或不属于当前租户")
	}
	ws.DisconnectUser(id)
	return nil
}

func (s *AgentAdminService) ResetPasswordForTenant(tenantID, id uint64, password string) error {
	return s.resetPassword(id, password, &tenantID)
}

func (s *AgentAdminService) view(id uint64) (*AgentView, error) {
	var row user.User
	if err := s.db.Where("user_id = ? AND role = ?", id, "agent").First(&row).Error; err != nil {
		return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "代理不存在")
	}
	var members int64
	if err := s.db.Model(&user.User{}).Where("parent_agent_id = ?", id).Count(&members).Error; err != nil {
		return nil, err
	}
	roomCode, roomName, roomLogo, workspaceID := row.AgentRoomCode, agentRoomDisplayName(row), row.AgentRoomLogo, row.WorkspaceID
	if workspace, err := WorkspaceForAccount(s.db, row); err == nil {
		roomCode, roomName, roomLogo, workspaceID = workspace.RoomCode, workspace.Name, workspace.Logo, workspace.ID
	}
	view := AgentView{ID: row.UserID, PublicID: row.PublicID, Username: row.Username, Email: row.Email, Nickname: row.Nickname, Phone: row.Phone, RoomCode: roomCode, RoomName: roomName, RoomLogo: roomLogo, WorkspaceID: workspaceID, Balance: centsToAmount(row.BalanceCents), Status: row.Status, MemberCount: members, RebateRate: row.RoomRebateRate, ProfitShareRate: row.RoomProfitShareRate, Remark: row.Remark, CreatedAt: row.CreatedAt.Format("2006-01-02 15:04:05"), LoginCount: row.LoginCount, TenantID: row.ParentTenantID}
	if row.ParentTenantID != nil {
		var tenant user.User
		if s.db.Select("username", "nickname").First(&tenant, *row.ParentTenantID).Error == nil {
			view.TenantName = firstNonEmpty(tenant.Nickname, tenant.Username)
		}
	}
	if row.LastLoginAt != nil {
		view.LastLoginAt = row.LastLoginAt.Local().Format("2006-01-02 15:04:05")
	}
	return &view, nil
}

func (s *AgentAdminService) Promote(userID uint64, roomCode string) (*AgentView, error) {
	roomCode = strings.TrimSpace(roomCode)
	if err := validateAgentRoomCode(roomCode); err != nil {
		return nil, err
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := lockPublicRoomCodeRegistry(tx); err != nil {
			return err
		}
		var account user.User
		if err := tx.First(&account, userID).Error; err != nil {
			return apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
		}
		if account.Role == "admin" || account.Role == "tenant" {
			return apperrors.NewBusinessError("INVALID_REQUEST", "管理员不能设置为代理")
		}
		if err := ensureRoomCodeAvailable(tx, roomCode, userID); err != nil {
			return err
		}
		if err := tx.Model(&account).Updates(map[string]any{"role": "agent", "agent_room_code": roomCode, "parent_agent_id": nil}).Error; err != nil {
			return err
		}
		return EnsureWorkspaceHierarchy(tx)
	})
	if err != nil {
		return nil, err
	}
	return s.view(userID)
}

func normalizeAgentRoomCode(value string) string { return strings.TrimSpace(value) }

func normalizeAgentRoomName(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func validateAgentRoomName(value string) error {
	if len([]rune(value)) > 30 {
		return apperrors.NewBusinessError("INVALID_REQUEST", "房间名称不能超过 30 个字符")
	}
	return nil
}

var builtInRoomLogos = map[string]struct{}{
	"/images/wangzhe-header-logo.png":       {},
	"/images/room-logos/crown-crystal.webp": {},
	"/images/room-logos/crown-shield.webp":  {},
	"/images/room-logos/crown-laurel.webp":  {},
}

func normalizeRoomLogo(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	// Built-in room marks are served by both the member and management apps.
	// Keep this as an exact allowlist: arbitrary paths and remote URLs must not
	// become stored room identities.
	if _, ok := builtInRoomLogos[value]; ok {
		return value, nil
	}
	if len(value) > 500000 {
		return "", apperrors.NewBusinessError("INVALID_REQUEST", "房间 Logo 文件过大")
	}
	allowed := []string{"data:image/png;base64,", "data:image/jpeg;base64,", "data:image/webp;base64,"}
	for _, prefix := range allowed {
		if strings.HasPrefix(value, prefix) {
			return value, nil
		}
	}
	return "", apperrors.NewBusinessError("INVALID_REQUEST", "房间 Logo 仅支持内置样式或 PNG、JPG、WebP 图片")
}

func agentRoomDisplayName(agent user.User) string {
	if name := normalizeAgentRoomName(agent.AgentRoomName); name != "" {
		return name
	}
	return defaultString(agent.Nickname, agent.Username) + "的房间"
}

func validateAgentRoomCode(value string) error {
	if len(value) < 5 || len(value) > 12 {
		return apperrors.NewBusinessError("INVALID_ROOM_CODE", "房间号须为 5–12 位数字")
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return apperrors.NewBusinessError("INVALID_ROOM_CODE", "房间号只能使用数字")
		}
	}
	return nil
}

func ensureRoomCodeAvailable(db *gorm.DB, roomCode string, excludeID uint64) error {
	return ensureRoomCodeAvailableForResource(db, roomCode, excludeID, 0)
}

func ensureRoomCodeAvailableForResource(db *gorm.DB, roomCode string, excludeID, excludeResourceID uint64) error {
	if err := ensureRoomCodeIdentityAvailable(db, roomCode, excludeID); err != nil {
		return err
	}
	resourceQuery := db.Model(&special.NumberResource{}).Where("number = ?", roomCode)
	if excludeResourceID > 0 {
		resourceQuery = resourceQuery.Where("id <> ?", excludeResourceID)
	}
	// Once a resource has been granted it is the current owner's compatibility
	// shadow and must not prevent that owner from editing the room name/logo.
	if excludeID > 0 {
		resourceQuery = resourceQuery.Where("owner_user_id IS NULL OR owner_user_id <> ?", excludeID)
	}
	var occupied int64
	if err := resourceQuery.Count(&occupied).Error; err != nil {
		return err
	}
	if occupied > 0 {
		return apperrors.NewBusinessError("ROOM_CODE_RESERVED", "房间号已在靓号库中预留")
	}
	return nil
}

func ensureRoomCodeIdentityAvailable(db *gorm.DB, roomCode string, excludeID uint64) error {
	query := db.Model(&user.User{}).Where("agent_room_code = ?", roomCode)
	if excludeID > 0 {
		query = query.Where("user_id <> ?", excludeID)
	}
	var occupied int64
	if err := query.Count(&occupied).Error; err != nil {
		return err
	}
	if occupied > 0 {
		return apperrors.NewBusinessError("ROOM_CODE_EXISTS", "房间号已被占用")
	}
	workspaceQuery := db.Model(&workspacemodel.Workspace{}).Where("room_code = ?", roomCode)
	if excludeID > 0 {
		workspaceQuery = workspaceQuery.Where("owner_user_id <> ?", excludeID)
	}
	if err := workspaceQuery.Count(&occupied).Error; err != nil {
		return err
	}
	if occupied > 0 {
		return apperrors.NewBusinessError("ROOM_CODE_EXISTS", "房间号已被占用")
	}
	return nil
}

func normalizeAgentStatus(value int) int {
	if value == 0 {
		return 0
	}
	return 1
}
