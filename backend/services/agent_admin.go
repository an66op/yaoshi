package services

import (
	"backend/data/models/user"
	apperrors "backend/errors"
	"backend/utils"
	"strings"
	"unicode"

	"gorm.io/gorm"
)

type AgentAdminService struct{ db *gorm.DB }

type AgentView struct {
	ID          uint64  `json:"id"`
	PublicID    uint64  `json:"public_id"`
	Username    string  `json:"username"`
	Email       string  `json:"email"`
	Nickname    string  `json:"nickname"`
	Phone       string  `json:"phone"`
	RoomCode    string  `json:"room_code"`
	Balance     float64 `json:"balance"`
	Status      int     `json:"status"`
	MemberCount int64   `json:"member_count"`
	Remark      string  `json:"remark"`
	CreatedAt   string  `json:"created_at"`
	LastLoginAt string  `json:"last_login_at"`
	LoginCount  int     `json:"login_count"`
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
	Username string
	Password string
	Email    string
	Nickname string
	Phone    string
	RoomCode string
	Remark   string
	Status   int
}

type UpdateAgentInput struct {
	Email    string
	Nickname string
	Phone    string
	RoomCode string
	Remark   string
	Status   int
}

func NewAgentAdminService(db *gorm.DB) *AgentAdminService { return &AgentAdminService{db: db} }

func (s *AgentAdminService) List(query string, page, pageSize int) (*AgentListResult, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	q := s.db.Model(&user.User{}).Where("role = ?", "agent")
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
		items = append(items, AgentView{
			ID: row.UserID, PublicID: row.PublicID, Username: row.Username, Email: row.Email, Nickname: row.Nickname, Phone: row.Phone,
			RoomCode: row.AgentRoomCode, Balance: centsToAmount(row.BalanceCents), Status: row.Status,
			MemberCount: members, Remark: row.Remark, CreatedAt: row.CreatedAt.Format("2006-01-02 15:04:05"), LoginCount: row.LoginCount,
		})
		if row.LastLoginAt != nil {
			items[len(items)-1].LastLoginAt = row.LastLoginAt.Local().Format("2006-01-02 15:04:05")
		}
	}
	summary := AgentSummary{Total: total}
	_ = s.db.Model(&user.User{}).Where("role = ? AND status = ?", "agent", 1).Count(&summary.Active).Error
	summary.Disabled = summary.Total - summary.Active
	_ = s.db.Model(&user.User{}).Where("parent_agent_id IS NOT NULL").Count(&summary.Members).Error
	return &AgentListResult{Items: items, Total: total, Page: page, PageSize: pageSize, Summary: summary}, nil
}

func (s *AgentAdminService) Create(input CreateAgentInput) (*AgentView, error) {
	input.Username = strings.TrimSpace(input.Username)
	input.Email = strings.TrimSpace(input.Email)
	input.RoomCode = normalizeAgentRoomCode(input.RoomCode)
	if len(input.Username) < 3 || len(input.Username) > 50 {
		return nil, apperrors.NewBusinessError("INVALID_USERNAME", "登录账号长度应为 3–50 个字符")
	}
	if len(input.Password) < 6 {
		return nil, apperrors.NewBusinessError("INVALID_PASSWORD", "登录密码至少需要 6 个字符")
	}
	if err := validateAgentRoomCode(input.RoomCode); err != nil {
		return nil, err
	}
	hash, err := utils.HashPassword(input.Password)
	if err != nil {
		return nil, err
	}
	row := user.User{Username: input.Username, Password: hash, Email: input.Email, Nickname: strings.TrimSpace(input.Nickname), Phone: strings.TrimSpace(input.Phone), Role: "agent", AgentRoomCode: input.RoomCode, Remark: strings.TrimSpace(input.Remark), RiskLevel: "normal", Status: normalizeAgentStatus(input.Status)}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var duplicate int64
		if err := tx.Model(&user.User{}).Where("username = ?", row.Username).Count(&duplicate).Error; err != nil {
			return err
		}
		if duplicate > 0 {
			return apperrors.NewBusinessError("USERNAME_EXISTS", "登录账号已存在")
		}
		if row.Email != "" {
			if err := tx.Model(&user.User{}).Where("LOWER(email) = LOWER(?)", row.Email).Count(&duplicate).Error; err != nil {
				return err
			}
			if duplicate > 0 {
				return apperrors.NewBusinessError("EMAIL_EXISTS", "邮箱已被使用")
			}
		}
		return ensureRoomCodeAvailable(tx, row.AgentRoomCode, 0)
	}); err != nil {
		return nil, err
	}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, err
	}
	return s.view(row.UserID)
}

func (s *AgentAdminService) Update(id uint64, input UpdateAgentInput) (*AgentView, error) {
	input.Email = strings.TrimSpace(input.Email)
	input.RoomCode = normalizeAgentRoomCode(input.RoomCode)
	if err := validateAgentRoomCode(input.RoomCode); err != nil {
		return nil, err
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var row user.User
		if err := tx.First(&row, id).Error; err != nil {
			return apperrors.NewBusinessError("USER_NOT_FOUND", "代理不存在")
		}
		if row.Role != "agent" {
			return apperrors.NewBusinessError("INVALID_REQUEST", "该账号不是代理")
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
		return tx.Model(&row).Updates(map[string]any{"email": input.Email, "nickname": strings.TrimSpace(input.Nickname), "phone": strings.TrimSpace(input.Phone), "agent_room_code": input.RoomCode, "remark": strings.TrimSpace(input.Remark), "status": normalizeAgentStatus(input.Status)}).Error
	})
	if err != nil {
		return nil, err
	}
	return s.view(id)
}

func (s *AgentAdminService) ResetPassword(id uint64, password string) error {
	if len(password) < 6 {
		return apperrors.NewBusinessError("INVALID_PASSWORD", "登录密码至少需要 6 个字符")
	}
	hash, err := utils.HashPassword(password)
	if err != nil {
		return err
	}
	result := s.db.Model(&user.User{}).Where("user_id = ? AND role = ?", id, "agent").Update("password", hash)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return apperrors.NewBusinessError("USER_NOT_FOUND", "代理不存在")
	}
	return nil
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
	view := AgentView{ID: row.UserID, PublicID: row.PublicID, Username: row.Username, Email: row.Email, Nickname: row.Nickname, Phone: row.Phone, RoomCode: row.AgentRoomCode, Balance: centsToAmount(row.BalanceCents), Status: row.Status, MemberCount: members, Remark: row.Remark, CreatedAt: row.CreatedAt.Format("2006-01-02 15:04:05"), LoginCount: row.LoginCount}
	if row.LastLoginAt != nil {
		view.LastLoginAt = row.LastLoginAt.Local().Format("2006-01-02 15:04:05")
	}
	return &view, nil
}

func (s *AgentAdminService) Promote(userID uint64, roomCode string) (*AgentView, error) {
	roomCode = strings.TrimSpace(roomCode)
	if roomCode != "" {
		if err := validateAgentRoomCode(roomCode); err != nil {
			return nil, err
		}
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var account user.User
		if err := tx.First(&account, userID).Error; err != nil {
			return apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
		}
		if account.Role == "admin" {
			return apperrors.NewBusinessError("INVALID_REQUEST", "管理员不能设置为代理")
		}
		updates := map[string]any{"role": "agent"}
		if roomCode != "" {
			var occupied int64
			if err := tx.Model(&user.User{}).Where("agent_room_code = ? AND user_id <> ?", roomCode, userID).Count(&occupied).Error; err != nil {
				return err
			}
			if occupied > 0 {
				return apperrors.NewBusinessError("INVALID_REQUEST", "房间号已被占用")
			}
			updates["agent_room_code"] = roomCode
		}
		return tx.Model(&account).Updates(updates).Error
	})
	if err != nil {
		return nil, err
	}
	return s.view(userID)
}

func normalizeAgentRoomCode(value string) string { return strings.TrimSpace(value) }

func validateAgentRoomCode(value string) error {
	if len(value) < 4 || len(value) > 12 {
		return apperrors.NewBusinessError("INVALID_ROOM_CODE", "房间号须为 4–12 位数字")
	}
	for _, char := range value {
		if !unicode.IsDigit(char) {
			return apperrors.NewBusinessError("INVALID_ROOM_CODE", "房间号只能使用数字")
		}
	}
	return nil
}

func ensureRoomCodeAvailable(db *gorm.DB, roomCode string, excludeID uint64) error {
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
	return nil
}

func normalizeAgentStatus(value int) int {
	if value == 0 {
		return 0
	}
	return 1
}
