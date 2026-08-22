package services

import (
	"backend/data/models/user"
	apperrors "backend/errors"
	"backend/utils"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UserAdminService struct{ db *gorm.DB }

type AdminUser struct {
	ID            uint64     `json:"id"`
	Username      string     `json:"username"`
	Email         string     `json:"email"`
	Nickname      string     `json:"nickname"`
	Phone         string     `json:"phone"`
	Role          string     `json:"role"`
	Remark        string     `json:"remark"`
	RiskLevel     string     `json:"risk_level"`
	Balance       float64    `json:"balance"`
	FlyMode       string     `json:"fly_mode"`
	FlyRate       float64    `json:"fly_rate"`
	AgentRoomCode string     `json:"agent_room_code"`
	ParentAgentID *uint64    `json:"parent_agent_id"`
	Status        int        `json:"status"`
	LastLoginAt   *time.Time `json:"last_login_at"`
	LoginCount    int        `json:"login_count"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type UserListFilter struct {
	Query    string
	Status   string
	Role     string
	Page     int
	PageSize int
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
	Username  string
	Password  string
	Email     string
	Nickname  string
	Phone     string
	Role      string
	Remark    string
	RiskLevel string
	Status    int
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

type BalanceRecord struct {
	ID        uint64    `json:"id"`
	UserID    uint64    `json:"user_id"`
	Amount    float64   `json:"amount"`
	Before    float64   `json:"before"`
	After     float64   `json:"after"`
	Type      string    `json:"type"`
	Remark    string    `json:"remark"`
	Operator  string    `json:"operator"`
	CreatedAt time.Time `json:"created_at"`
}

func NewUserAdminService(db *gorm.DB) *UserAdminService { return &UserAdminService{db: db} }

func (s *UserAdminService) List(filter UserListFilter) (*UserList, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	query := s.db.Model(&user.User{})
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
	return &UserList{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (s *UserAdminService) Stats() (*UserStats, error) {
	stats := &UserStats{}
	base := s.db.Model(&user.User{})
	if err := base.Count(&stats.Total).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&user.User{}).Where("status = 1").Count(&stats.Active).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&user.User{}).Where("status = 0").Count(&stats.Disabled).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if err := s.db.Model(&user.User{}).Where("created_at >= ?", startOfDay).Count(&stats.NewToday).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&user.User{}).Where("role = ?", "admin").Count(&stats.Administrators).Error; err != nil {
		return nil, err
	}
	var cents int64
	if err := s.db.Model(&user.User{}).Select("COALESCE(SUM(balance_cents), 0)").Scan(&cents).Error; err != nil {
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
	return &result, nil
}

func (s *UserAdminService) Create(input CreateAdminUserInput) (*AdminUser, error) {
	input.Username = strings.TrimSpace(input.Username)
	input.Email = strings.TrimSpace(input.Email)
	if len(input.Username) < 3 || len(input.Username) > 50 {
		return nil, apperrors.NewBusinessError("INVALID_USERNAME", "用户名长度应为 3–50 个字符")
	}
	if len(input.Password) < 6 {
		return nil, apperrors.NewBusinessError("INVALID_PASSWORD", "密码至少需要 6 个字符")
	}
	if err := s.ensureUnique(0, input.Username, input.Email); err != nil {
		return nil, err
	}
	role, err := validRole(input.Role)
	if err != nil {
		return nil, err
	}
	risk, err := validRisk(input.RiskLevel)
	if err != nil {
		return nil, err
	}
	hash, err := utils.HashPassword(input.Password)
	if err != nil {
		return nil, err
	}
	row := user.User{Username: input.Username, Password: hash, Email: input.Email, Nickname: strings.TrimSpace(input.Nickname), Phone: strings.TrimSpace(input.Phone), Role: role, Remark: strings.TrimSpace(input.Remark), RiskLevel: risk, Status: normalizeStatus(input.Status)}
	if err := s.db.Create(&row).Error; err != nil {
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
	if err := s.ensureUnique(id, "", input.Email); err != nil {
		return nil, err
	}
	role, err := validRole(input.Role)
	if err != nil {
		return nil, err
	}
	risk, err := validRisk(input.RiskLevel)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{"email": input.Email, "nickname": strings.TrimSpace(input.Nickname), "phone": strings.TrimSpace(input.Phone), "role": role, "remark": strings.TrimSpace(input.Remark), "risk_level": risk, "status": normalizeStatus(input.Status)}
	if err := s.db.Model(&row).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.Get(id)
}

func (s *UserAdminService) SetStatus(id uint64, status int) (*AdminUser, error) {
	var row user.User
	if err := s.db.First(&row, id).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&row).Update("status", normalizeStatus(status)).Error; err != nil {
		return nil, err
	}
	return s.Get(id)
}

func (s *UserAdminService) ResetPassword(id uint64, password string) error {
	if len(password) < 6 {
		return apperrors.NewBusinessError("INVALID_PASSWORD", "密码至少需要 6 个字符")
	}
	hash, err := utils.HashPassword(password)
	if err != nil {
		return err
	}
	result := s.db.Model(&user.User{}).Where("user_id = ?", id).Update("password", hash)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *UserAdminService) AdjustBalance(id uint64, amount float64, remark, operator string) (*AdminUser, error) {
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
		after := row.BalanceCents + amountCents
		if after < 0 {
			return apperrors.NewBusinessError("INSUFFICIENT_BALANCE", "扣减金额不能超过当前余额")
		}
		if err := tx.Model(&row).Update("balance_cents", after).Error; err != nil {
			return err
		}
		record := user.BalanceTransaction{UserID: id, AmountCents: amountCents, BeforeCents: row.BalanceCents, AfterCents: after, Type: "manual", Remark: strings.TrimSpace(remark), Operator: strings.TrimSpace(operator)}
		return tx.Create(&record).Error
	})
	if err != nil {
		return nil, err
	}
	return s.Get(id)
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
		result = append(result, BalanceRecord{ID: row.ID, UserID: row.UserID, Amount: centsToAmount(row.AmountCents), Before: centsToAmount(row.BeforeCents), After: centsToAmount(row.AfterCents), Type: row.Type, Remark: row.Remark, Operator: row.Operator, CreatedAt: row.CreatedAt})
	}
	return result, nil
}

func (s *UserAdminService) ensureUnique(excludeID uint64, username, email string) error {
	if username != "" {
		query := s.db.Model(&user.User{}).Where("username = ?", username)
		if excludeID > 0 {
			query = query.Where("user_id <> ?", excludeID)
		}
		var count int64
		if err := query.Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return apperrors.NewBusinessError("USERNAME_EXISTS", "用户名已存在")
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
	case "member", "agent", "admin":
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
	return AdminUser{
		ID: row.UserID, Username: row.Username, Email: row.Email, Nickname: row.Nickname, Phone: row.Phone,
		Role: defaultString(row.Role, "member"), Remark: row.Remark, RiskLevel: defaultString(row.RiskLevel, "normal"),
		Balance: centsToAmount(row.BalanceCents), FlyMode: defaultString(row.FlyMode, "inherit"), FlyRate: row.FlyRate,
		AgentRoomCode: row.AgentRoomCode, ParentAgentID: row.ParentAgentID,
		Status: row.Status, LastLoginAt: row.LastLoginAt, LoginCount: row.LoginCount, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
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
