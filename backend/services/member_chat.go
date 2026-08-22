package services

import (
	"backend/data/models/chat"
	membernotify "backend/data/models/notify"
	"backend/data/models/user"
	apperrors "backend/errors"
	"backend/ws"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type MemberChatService struct {
	db       *gorm.DB
	settings *SettingsAdminService
}

type ChatMessageView struct {
	ID        uint64    `json:"id"`
	UserID    uint64    `json:"user_id"`
	Username  string    `json:"username"`
	Nickname  string    `json:"nickname"`
	RoomType  string    `json:"room_type"`
	Content   string    `json:"content"`
	Mine      bool      `json:"mine"`
	CreatedAt time.Time `json:"created_at"`
}

type ChatPreview struct {
	LatestMessage string     `json:"latest_message"`
	LatestAt      *time.Time `json:"latest_at,omitempty"`
	CanChat       bool       `json:"can_chat"`
	MinChatScore  float64    `json:"min_chat_score"`
	ChatNickname  string     `json:"chat_nickname"`
	Balance       float64    `json:"balance"`
}

func NewMemberChatService(db *gorm.DB) *MemberChatService {
	return &MemberChatService{db: db, settings: NewSettingsAdminService(db)}
}

func (s *MemberChatService) List(userID uint64, roomType string, limit int) ([]ChatMessageView, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	roomType = defaultString(strings.TrimSpace(roomType), "group")
	if roomType != "group" && roomType != "service" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "聊天室类型不正确")
	}
	account, err := s.account(userID)
	if err != nil {
		return nil, err
	}
	scope := chatScope(account, roomType)
	query := s.db.Model(&chat.Message{}).Where("room_type = ? AND scope = ?", roomType, scope)
	var rows []chat.Message
	if err := query.Order("created_at desc").Limit(limit).Find(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("CHAT_READ_FAILED", "读取聊天消息失败", err)
	}
	items := make([]ChatMessageView, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		row := rows[i]
		items = append(items, ChatMessageView{
			ID: row.ID, UserID: row.UserID, Username: row.Username, Nickname: row.Nickname,
			RoomType: row.RoomType, Content: row.Content, Mine: row.UserID == userID, CreatedAt: row.CreatedAt,
		})
	}
	return items, nil
}

func (s *MemberChatService) Post(userID uint64, roomType, content string) (*ChatMessageView, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "消息不能为空")
	}
	if len([]rune(content)) > 200 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "消息过长")
	}
	roomType = defaultString(strings.TrimSpace(roomType), "group")
	if roomType != "group" && roomType != "service" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "聊天室类型不正确")
	}
	cfg, err := s.settings.Get()
	if err != nil {
		return nil, err
	}
	account, err := s.account(userID)
	if err != nil {
		return nil, err
	}
	if roomType == "group" && account.BalanceCents < int64(cfg.MinChatScore*100) {
		return nil, apperrors.NewBusinessError("CHAT_FORBIDDEN", fmt.Sprintf("余额需达到 %.2f 元才可发言", cfg.MinChatScore))
	}
	nickname := defaultString(account.Nickname, account.Username)
	row := chat.Message{
		UserID: userID, Username: account.Username, Nickname: nickname,
		RoomType: roomType, Scope: chatScope(account, roomType), Content: content,
	}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, apperrors.NewSystemError("CHAT_SAVE_FAILED", "发送消息失败", err)
	}
	if roomType == "service" {
		reply := fmt.Sprintf("已收到您的消息：「%s」。客服将在工作时间内尽快回复。", content)
		_ = s.db.Create(&membernotify.MemberNotification{
			UserID: userID, Title: "客服回复", Content: reply,
			Level: "info", Category: "system",
		}).Error
		ws.NotifyUser(userID, "notification", map[string]any{
			"title": "客服回复", "content": reply, "level": "info", "category": "system",
		})
	}
	view := ChatMessageView{
		ID: row.ID, UserID: row.UserID, Username: row.Username, Nickname: row.Nickname,
		RoomType: row.RoomType, Content: row.Content, Mine: true, CreatedAt: row.CreatedAt,
	}
	recipients, err := s.scopeRecipients(row.Scope)
	if err == nil {
		ws.NotifyChat(recipients, roomType, row.Scope, row.ID)
	}
	return &view, nil
}

func (s *MemberChatService) Preview(userID uint64) (*ChatPreview, error) {
	cfg, err := s.settings.Get()
	if err != nil {
		return nil, err
	}
	account, err := s.account(userID)
	if err != nil {
		return nil, err
	}
	preview := &ChatPreview{
		MinChatScore: cfg.MinChatScore,
		ChatNickname: cfg.ChatNickname,
		Balance:      centsToAmount(account.BalanceCents),
		CanChat:      centsToAmount(account.BalanceCents) >= cfg.MinChatScore,
	}
	var latest chat.Message
	query := s.db.Where("room_type = ? AND scope = ?", "group", chatScope(account, "group"))
	if err := query.Order("created_at desc").First(&latest).Error; err == nil {
		preview.LatestMessage = latest.Nickname + "：" + latest.Content
		preview.LatestAt = &latest.CreatedAt
	}
	return preview, nil
}

func (s *MemberChatService) account(userID uint64) (user.User, error) {
	var account user.User
	if err := s.db.First(&account, userID).Error; err != nil {
		return user.User{}, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
	}
	return account, nil
}

func chatScope(account user.User, roomType string) string {
	if roomType == "service" {
		return "user:" + strconv.FormatUint(account.UserID, 10)
	}
	if account.Role == "agent" {
		return "agent:" + strconv.FormatUint(account.UserID, 10)
	}
	if account.ParentAgentID != nil {
		return "agent:" + strconv.FormatUint(*account.ParentAgentID, 10)
	}
	return "lobby"
}

func (s *MemberChatService) scopeRecipients(scope string) ([]uint64, error) {
	if strings.HasPrefix(scope, "user:") {
		userID, err := strconv.ParseUint(strings.TrimPrefix(scope, "user:"), 10, 64)
		if err != nil || userID == 0 {
			return nil, fmt.Errorf("invalid chat scope")
		}
		return []uint64{userID}, nil
	}
	query := s.db.Model(&user.User{}).Where("status = ?", 1)
	if strings.HasPrefix(scope, "agent:") {
		agentID, err := strconv.ParseUint(strings.TrimPrefix(scope, "agent:"), 10, 64)
		if err != nil || agentID == 0 {
			return nil, fmt.Errorf("invalid chat scope")
		}
		query = query.Where("user_id = ? OR parent_agent_id = ?", agentID, agentID)
	} else if scope == "lobby" {
		query = query.Where("parent_agent_id IS NULL AND role <> ?", "agent")
	} else {
		return nil, fmt.Errorf("invalid chat scope")
	}
	var recipients []uint64
	if err := query.Pluck("user_id", &recipients).Error; err != nil {
		return nil, err
	}
	return recipients, nil
}
