package services

import (
	"backend/data/models/user"
	apperrors "backend/errors"
	"strings"

	"gorm.io/gorm"
)

type AgentAdminService struct{ db *gorm.DB }

type AgentView struct {
	ID            uint64  `json:"id"`
	Username      string  `json:"username"`
	Nickname      string  `json:"nickname"`
	Phone         string  `json:"phone"`
	RoomCode      string  `json:"room_code"`
	Balance       float64 `json:"balance"`
	Status        int     `json:"status"`
	MemberCount   int64   `json:"member_count"`
	Remark        string  `json:"remark"`
	CreatedAt     string  `json:"created_at"`
}

type AgentListResult struct {
	Items    []AgentView `json:"items"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
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
			ID: row.UserID, Username: row.Username, Nickname: row.Nickname, Phone: row.Phone,
			RoomCode: row.AgentRoomCode, Balance: centsToAmount(row.BalanceCents), Status: row.Status,
			MemberCount: members, Remark: row.Remark, CreatedAt: row.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return &AgentListResult{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *AgentAdminService) Promote(userID uint64, roomCode string) (*AgentView, error) {
	roomCode = strings.TrimSpace(roomCode)
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
	list, err := s.List("", 1, 100)
	if err != nil {
		return nil, err
	}
	for _, item := range list.Items {
		if item.ID == userID {
			return &item, nil
		}
	}
	return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "代理不存在")
}
