package services

import (
	"backend/data/models/user"
	apperrors "backend/errors"
	"backend/utils"
	"strings"

	"gorm.io/gorm"
)

type TenantAdminService struct{ db *gorm.DB }

type TenantView struct {
	ID          uint64  `json:"id"`
	PublicID    uint64  `json:"public_id"`
	Username    string  `json:"username"`
	Email       string  `json:"email"`
	Nickname    string  `json:"nickname"`
	Phone       string  `json:"phone"`
	Balance     float64 `json:"balance"`
	Status      int     `json:"status"`
	AgentCount  int64   `json:"agent_count"`
	MemberCount int64   `json:"member_count"`
	Remark      string  `json:"remark"`
	CreatedAt   string  `json:"created_at"`
	LastLoginAt string  `json:"last_login_at"`
	LoginCount  int     `json:"login_count"`
}

type TenantListResult struct {
	Items    []TenantView `json:"items"`
	Total    int64        `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
	Active   int64        `json:"active"`
	Agents   int64        `json:"agents"`
	Members  int64        `json:"members"`
}

type TenantPayload struct {
	Username string
	Password string
	Email    string
	Nickname string
	Phone    string
	Remark   string
	Status   int
}

func NewTenantAdminService(db *gorm.DB) *TenantAdminService { return &TenantAdminService{db: db} }

func (s *TenantAdminService) List(query string, page, pageSize int) (*TenantListResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := s.db.Model(&user.User{}).Where("role = ?", "tenant")
	if value := strings.TrimSpace(query); value != "" {
		like := "%" + value + "%"
		q = q.Where("username ILIKE ? OR nickname ILIKE ? OR email ILIKE ? OR phone ILIKE ?", like, like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, apperrors.NewSystemError("TENANT_READ_FAILED", "读取租户列表失败", err)
	}
	var rows []user.User
	if err := q.Order("user_id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("TENANT_READ_FAILED", "读取租户列表失败", err)
	}
	items := make([]TenantView, 0, len(rows))
	for _, row := range rows {
		view, err := s.toView(row)
		if err != nil {
			return nil, err
		}
		items = append(items, view)
	}
	result := &TenantListResult{Items: items, Total: total, Page: page, PageSize: pageSize}
	_ = s.db.Model(&user.User{}).Where("role = ? AND status = ?", "tenant", 1).Count(&result.Active).Error
	_ = s.db.Model(&user.User{}).Where("role = ? AND parent_tenant_id IS NOT NULL", "agent").Count(&result.Agents).Error
	_ = s.db.Model(&user.User{}).Where("role = ? AND parent_agent_id IN (?)", "member", s.db.Model(&user.User{}).Select("user_id").Where("role = ? AND parent_tenant_id IS NOT NULL", "agent")).Count(&result.Members).Error
	return result, nil
}

func (s *TenantAdminService) Create(input TenantPayload) (*TenantView, error) {
	input.Username = strings.TrimSpace(input.Username)
	input.Email = strings.TrimSpace(input.Email)
	if len(input.Username) < 3 || len(input.Username) > 50 {
		return nil, apperrors.NewBusinessError("INVALID_USERNAME", "登录账号长度应为 3–50 个字符")
	}
	if err := utils.ValidatePassword(input.Password); err != nil {
		return nil, apperrors.NewBusinessError("INVALID_PASSWORD", "登录密码长度需为 8–72 个字符")
	}
	hash, err := utils.HashPassword(input.Password)
	if err != nil {
		return nil, err
	}
	row := user.User{Username: input.Username, LoginScope: platformLoginScope, Password: hash, Email: input.Email, Nickname: strings.TrimSpace(input.Nickname), Phone: strings.TrimSpace(input.Phone), Role: "tenant", Remark: strings.TrimSpace(input.Remark), RiskLevel: "normal", Status: normalizeAgentStatus(input.Status)}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := ensureAccountAvailable(tx, row.Username, row.Email, 0); err != nil {
			return err
		}
		return tx.Create(&row).Error
	}); err != nil {
		return nil, err
	}
	view, err := s.toView(row)
	return &view, err
}

func (s *TenantAdminService) Update(id uint64, input TenantPayload) (*TenantView, error) {
	input.Email = strings.TrimSpace(input.Email)
	var row user.User
	if err := s.db.Where("user_id = ? AND role = ?", id, "tenant").First(&row).Error; err != nil {
		return nil, apperrors.NewBusinessError("TENANT_NOT_FOUND", "租户不存在")
	}
	if err := ensureAccountAvailable(s.db, row.Username, input.Email, id); err != nil {
		return nil, err
	}
	if err := s.db.Model(&row).Updates(map[string]any{"email": input.Email, "nickname": strings.TrimSpace(input.Nickname), "phone": strings.TrimSpace(input.Phone), "remark": strings.TrimSpace(input.Remark), "status": normalizeAgentStatus(input.Status)}).Error; err != nil {
		return nil, apperrors.NewSystemError("TENANT_UPDATE_FAILED", "保存租户失败", err)
	}
	if err := s.db.First(&row, id).Error; err != nil {
		return nil, err
	}
	view, err := s.toView(row)
	return &view, err
}

func (s *TenantAdminService) ResetPassword(id uint64, password string) error {
	if err := utils.ValidatePassword(password); err != nil {
		return apperrors.NewBusinessError("INVALID_PASSWORD", "登录密码长度需为 8–72 个字符")
	}
	hash, err := utils.HashPassword(password)
	if err != nil {
		return err
	}
	result := s.db.Model(&user.User{}).Where("user_id = ? AND role = ?", id, "tenant").Update("password", hash)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return apperrors.NewBusinessError("TENANT_NOT_FOUND", "租户不存在")
	}
	return nil
}

func (s *TenantAdminService) Dashboard(tenantID uint64) (map[string]any, error) {
	var tenant user.User
	if err := s.db.Where("user_id = ? AND role = ?", tenantID, "tenant").First(&tenant).Error; err != nil {
		return nil, apperrors.NewBusinessError("TENANT_NOT_FOUND", "租户不存在")
	}
	var agents, activeAgents, members int64
	agentIDs := s.db.Model(&user.User{}).Select("user_id").Where("role = ? AND parent_tenant_id = ?", "agent", tenantID)
	if err := s.db.Model(&user.User{}).Where("role = ? AND parent_tenant_id = ?", "agent", tenantID).Count(&agents).Error; err != nil {
		return nil, err
	}
	_ = s.db.Model(&user.User{}).Where("role = ? AND parent_tenant_id = ? AND status = ?", "agent", tenantID, 1).Count(&activeAgents).Error
	_ = s.db.Model(&user.User{}).Where("role = ? AND parent_agent_id IN (?)", "member", agentIDs).Count(&members).Error
	return map[string]any{"tenant_id": tenant.UserID, "tenant_name": firstNonEmpty(tenant.Nickname, tenant.Username), "agent_count": agents, "active_agent_count": activeAgents, "member_count": members}, nil
}

func (s *TenantAdminService) toView(row user.User) (TenantView, error) {
	var agents, members int64
	agentIDs := s.db.Model(&user.User{}).Select("user_id").Where("role = ? AND parent_tenant_id = ?", "agent", row.UserID)
	if err := s.db.Model(&user.User{}).Where("role = ? AND parent_tenant_id = ?", "agent", row.UserID).Count(&agents).Error; err != nil {
		return TenantView{}, err
	}
	if err := s.db.Model(&user.User{}).Where("role = ? AND parent_agent_id IN (?)", "member", agentIDs).Count(&members).Error; err != nil {
		return TenantView{}, err
	}
	view := TenantView{ID: row.UserID, PublicID: row.PublicID, Username: row.Username, Email: row.Email, Nickname: row.Nickname, Phone: row.Phone, Balance: centsToAmount(row.BalanceCents), Status: row.Status, AgentCount: agents, MemberCount: members, Remark: row.Remark, CreatedAt: row.CreatedAt.Format("2006-01-02 15:04:05"), LoginCount: row.LoginCount}
	if row.LastLoginAt != nil {
		view.LastLoginAt = row.LastLoginAt.Local().Format("2006-01-02 15:04:05")
	}
	return view, nil
}

func ensureAccountAvailable(db *gorm.DB, username, email string, excludeID uint64) error {
	if err := ensureUsernameInScope(db, platformLoginScope, username, excludeID); err != nil {
		return err
	}
	var count int64
	if email == "" {
		return nil
	}
	q := db.Model(&user.User{}).Where("LOWER(email) = LOWER(?)", email)
	if excludeID > 0 {
		q = q.Where("user_id <> ?", excludeID)
	}
	if err := q.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return apperrors.NewBusinessError("EMAIL_EXISTS", "邮箱已被使用")
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "租户"
}
