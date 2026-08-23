package services

import (
	"backend/data/models/chat"
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
	ID       uint64 `json:"id"`
	UserID   uint64 `json:"user_id"`
	PublicID uint64 `json:"public_id,omitempty"`
	// Username is an internal login credential and must never be exposed in a
	// member-facing message payload.
	Username  string    `json:"-"`
	Nickname  string    `json:"nickname"`
	RoomType  string    `json:"room_type"`
	Content   string    `json:"content"`
	Mine      bool      `json:"mine"`
	CreatedAt time.Time `json:"created_at"`
}

type ChatMessageList struct {
	Items        []ChatMessageView `json:"items"`
	HasMore      bool              `json:"has_more"`
	NextBeforeID uint64            `json:"next_before_id,omitempty"`
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

func (s *MemberChatService) List(userID uint64, roomType string, limit int, beforeID, afterID uint64) (*ChatMessageList, error) {
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
	query := s.db.Model(&chat.Message{}).Where("room_type = ? AND scope = ? AND deleted_at IS NULL", roomType, scope)
	if afterID > 0 {
		query = query.Where("id > ?", afterID)
	} else if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}
	var rows []chat.Message
	order := "created_at desc, id desc"
	if afterID > 0 {
		order = "created_at asc, id asc"
	}
	if err := query.Order(order).Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("CHAT_READ_FAILED", "读取聊天消息失败", err)
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	items := make([]ChatMessageView, 0, len(rows))
	publicIDs, err := s.publicIDs(rows)
	if err != nil {
		return nil, apperrors.NewSystemError("CHAT_READ_FAILED", "读取聊天消息失败", err)
	}
	if afterID > 0 {
		for _, row := range rows {
			items = append(items, chatMessageView(row, userID, publicIDs[row.UserID]))
		}
	} else {
		for i := len(rows) - 1; i >= 0; i-- {
			row := rows[i]
			items = append(items, chatMessageView(row, userID, publicIDs[row.UserID]))
		}
	}
	nextBeforeID := uint64(0)
	if len(items) > 0 {
		nextBeforeID = items[0].ID
	}
	return &ChatMessageList{Items: items, HasMore: hasMore && afterID == 0, NextBeforeID: nextBeforeID}, nil
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
	if roomType == "group" && account.MutedUntil != nil && account.MutedUntil.After(time.Now()) {
		reason := strings.TrimSpace(account.MuteReason)
		if reason == "" {
			reason = "请联系在线客服"
		}
		return nil, apperrors.NewBusinessError("CHAT_MUTED", fmt.Sprintf("您已被禁言至 %s：%s", account.MutedUntil.Local().Format("2006-01-02 15:04"), reason))
	}
	nickname := defaultString(account.Nickname, account.Username)
	row := chat.Message{
		UserID: userID, Username: account.Username, Nickname: nickname,
		RoomType: roomType, Scope: chatScope(account, roomType), Content: content,
	}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, apperrors.NewSystemError("CHAT_SAVE_FAILED", "发送消息失败", err)
	}
	view := ChatMessageView{
		ID: row.ID, UserID: row.UserID, PublicID: account.PublicID, Username: row.Username, Nickname: row.Nickname,
		RoomType: row.RoomType, Content: row.Content, Mine: true, CreatedAt: row.CreatedAt,
	}
	recipients, err := s.scopeRecipients(row.Scope)
	if err == nil {
		ws.NotifyChat(recipients, roomType, row.Scope, row.ID)
	}
	return &view, nil
}

func chatMessageView(row chat.Message, userID, publicID uint64) ChatMessageView {
	return ChatMessageView{
		ID: row.ID, UserID: row.UserID, PublicID: publicID, Username: row.Username, Nickname: row.Nickname,
		RoomType: row.RoomType, Content: row.Content, Mine: row.UserID == userID, CreatedAt: row.CreatedAt,
	}
}

func (s *MemberChatService) publicIDs(rows []chat.Message) (map[uint64]uint64, error) {
	ids := make([]uint64, 0, len(rows))
	seen := make(map[uint64]struct{}, len(rows))
	for _, row := range rows {
		if row.UserID == 0 {
			continue
		}
		if _, exists := seen[row.UserID]; exists {
			continue
		}
		seen[row.UserID] = struct{}{}
		ids = append(ids, row.UserID)
	}
	if len(ids) == 0 {
		return map[uint64]uint64{}, nil
	}
	var accounts []user.User
	if err := s.db.Select("user_id", "public_id").Where("user_id IN ?", ids).Find(&accounts).Error; err != nil {
		return nil, err
	}
	result := make(map[uint64]uint64, len(accounts))
	for _, account := range accounts {
		result[account.UserID] = account.PublicID
	}
	return result, nil
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
		CanChat:      centsToAmount(account.BalanceCents) >= cfg.MinChatScore && (account.MutedUntil == nil || !account.MutedUntil.After(time.Now())),
	}
	var latest chat.Message
	query := s.db.Where("room_type = ? AND scope = ? AND deleted_at IS NULL", "group", chatScope(account, "group"))
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

// chatScope keeps group chat and customer service inside the same room
// boundary.  A room's members and its agent therefore share one service
// history, while historic user:* service conversations remain private.
func chatScope(account user.User, _ string) string {
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
