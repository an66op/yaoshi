package services

import (
	"backend/data/models/chat"
	"backend/data/models/settings"
	"backend/data/models/user"
	apperrors "backend/errors"
	"backend/ws"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ChatAdminService owns all operational chat actions. Group chat and current
// customer service both use a room scope, so an agent can support every member
// of their room from one conversation.
type ChatAdminService struct{ db *gorm.DB }

type AdminConversation struct {
	Scope        string     `json:"scope"`
	RoomType     string     `json:"room_type"`
	Title        string     `json:"title"`
	Subtitle     string     `json:"subtitle"`
	UserID       uint64     `json:"user_id,omitempty"`
	Username     string     `json:"username,omitempty"`
	Nickname     string     `json:"nickname,omitempty"`
	LatestText   string     `json:"latest_text"`
	LatestAt     time.Time  `json:"latest_at"`
	MessageCount int64      `json:"message_count"`
	MutedUntil   *time.Time `json:"muted_until,omitempty"`
}

type AdminConversationList struct {
	Items    []AdminConversation `json:"items"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
}

type AdminChatMessage struct {
	ID        uint64    `json:"id"`
	UserID    uint64    `json:"user_id"`
	Username  string    `json:"username"`
	Nickname  string    `json:"nickname"`
	RoomType  string    `json:"room_type"`
	Scope     string    `json:"scope"`
	Content   string    `json:"content"`
	IsStaff   bool      `json:"is_staff"`
	CreatedAt time.Time `json:"created_at"`
}

type AdminChatMessageList struct {
	Items        []AdminChatMessage `json:"items"`
	HasMore      bool               `json:"has_more"`
	NextBeforeID uint64             `json:"next_before_id,omitempty"`
}

func NewChatAdminService(db *gorm.DB) *ChatAdminService { return &ChatAdminService{db: db} }

func (s *ChatAdminService) Conversations(roomType, query string, page, pageSize int) (*AdminConversationList, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 30
	}
	roomType = strings.TrimSpace(roomType)
	if roomType != "" && roomType != "group" && roomType != "service" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "聊天室类型不正确")
	}
	type key struct {
		Scope        string
		RoomType     string
		LatestID     uint64
		MessageCount int64
	}
	q := s.db.Model(&chat.Message{}).Where("deleted_at IS NULL")
	if roomType != "" {
		q = q.Where("room_type = ?", roomType)
	}
	if value := strings.TrimSpace(query); value != "" {
		like := "%" + strings.ToLower(value) + "%"
		q = q.Where("LOWER(username) LIKE ? OR LOWER(nickname) LIKE ? OR LOWER(content) LIKE ?", like, like, like)
	}
	var total int64
	if err := q.Select("COUNT(*)").Group("scope, room_type").Count(&total).Error; err != nil {
		return nil, err
	}
	var keys []key
	if err := q.Select("scope, room_type, MAX(id) AS latest_id, COUNT(*) AS message_count").Group("scope, room_type").Order("MAX(id) DESC").Offset((page - 1) * pageSize).Limit(pageSize).Scan(&keys).Error; err != nil {
		return nil, err
	}
	items := make([]AdminConversation, 0, len(keys))
	for _, item := range keys {
		var latest chat.Message
		if err := s.db.First(&latest, item.LatestID).Error; err != nil {
			continue
		}
		view := AdminConversation{Scope: item.Scope, RoomType: item.RoomType, LatestText: latest.Content, LatestAt: latest.CreatedAt, MessageCount: item.MessageCount}
		s.fillConversationIdentity(&view)
		items = append(items, view)
	}
	return &AdminConversationList{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *ChatAdminService) Messages(scope, roomType string, limit int, beforeID uint64) (*AdminChatMessageList, error) {
	if err := validAdminScope(scope, roomType); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	q := s.db.Model(&chat.Message{}).Where("scope = ? AND room_type = ? AND deleted_at IS NULL", scope, roomType)
	if beforeID > 0 {
		q = q.Where("id < ?", beforeID)
	}
	var rows []chat.Message
	if err := q.Order("id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	items := make([]AdminChatMessage, 0, len(rows))
	for index := len(rows) - 1; index >= 0; index-- {
		items = append(items, adminChatMessage(rows[index]))
	}
	next := uint64(0)
	if len(items) > 0 {
		next = items[0].ID
	}
	return &AdminChatMessageList{Items: items, HasMore: hasMore, NextBeforeID: next}, nil
}

func (s *ChatAdminService) Reply(scope, roomType, content, operator string) (*AdminChatMessage, error) {
	if err := validAdminScope(scope, roomType); err != nil {
		return nil, err
	}
	content = strings.TrimSpace(content)
	if content == "" || len([]rune(content)) > 500 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "消息长度应为 1–500 个字符")
	}
	nickname := defaultString(strings.TrimSpace(operator), "在线客服")
	row := chat.Message{Username: "support", Nickname: nickname, RoomType: roomType, Scope: scope, Content: content}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, err
	}
	if recipients, err := chatScopeRecipients(s.db, scope); err == nil {
		ws.NotifyChat(recipients, roomType, scope, row.ID)
	}
	view := adminChatMessage(row)
	return &view, nil
}

func (s *ChatAdminService) DeleteMessage(id uint64, operator string) error {
	now := time.Now().UTC()
	var row chat.Message
	if err := s.db.Where("deleted_at IS NULL").First(&row, id).Error; err != nil {
		return apperrors.NewBusinessError("NOT_FOUND", "消息不存在或已撤回")
	}
	if err := s.db.Model(&row).Updates(map[string]any{"deleted_at": now, "deleted_by": defaultString(strings.TrimSpace(operator), "后台管理员")}).Error; err != nil {
		return err
	}
	if recipients, err := chatScopeRecipients(s.db, row.Scope); err == nil {
		ws.NotifyChat(recipients, row.RoomType, row.Scope, row.ID)
	}
	return nil
}

func (s *ChatAdminService) SetMute(userID uint64, minutes int, reason string) (*user.User, error) {
	var account user.User
	if err := s.db.First(&account, userID).Error; err != nil {
		return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
	}
	if account.Role == "admin" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "不能禁言管理员")
	}
	updates := map[string]any{"mute_reason": strings.TrimSpace(reason)}
	if minutes <= 0 {
		updates["muted_until"] = nil
		updates["mute_reason"] = ""
	} else {
		if minutes > 60*24*30 {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "禁言时长不能超过 30 天")
		}
		updates["muted_until"] = time.Now().UTC().Add(time.Duration(minutes) * time.Minute)
	}
	if err := s.db.Model(&account).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := s.db.First(&account, userID).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func (s *ChatAdminService) SetAnnouncement(content string) (string, error) {
	content = strings.TrimSpace(content)
	if len([]rune(content)) > 500 {
		return "", apperrors.NewBusinessError("INVALID_REQUEST", "公告不能超过 500 个字符")
	}
	if _, err := NewSettingsAdminService(s.db).Get(); err != nil {
		return "", err
	}
	if err := s.db.Model(&settings.SystemConfig{}).Where("id = ?", 1).Update("room_notice", content).Error; err != nil {
		return "", err
	}
	return content, nil
}

func (s *ChatAdminService) fillConversationIdentity(view *AdminConversation) {
	if view.RoomType == "service" {
		view.Title = "房间客服"
		view.Subtitle = "房间成员与客服"
	} else {
		view.Title = "大厅聊天室"
		view.Subtitle = "大厅群消息"
	}
	prefix, value, found := strings.Cut(view.Scope, ":")
	if !found {
		if view.RoomType == "service" && view.Scope == "lobby" {
			view.Title = "大厅客服"
			view.Subtitle = "大厅成员与客服"
		}
		return
	}
	if prefix != "user" && prefix != "agent" {
		return
	}
	id, err := strconv.ParseUint(value, 10, 64)
	if err != nil || id == 0 {
		return
	}
	var account user.User
	if err := s.db.First(&account, id).Error; err != nil {
		return
	}
	view.UserID, view.Username, view.Nickname, view.MutedUntil = account.UserID, account.Username, account.Nickname, account.MutedUntil
	if view.RoomType == "service" {
		if prefix == "agent" {
			view.Title = defaultString(account.Nickname, account.Username) + "的房间客服"
			view.Subtitle = "房间号 " + defaultString(account.AgentRoomCode, fmt.Sprintf("代理 #%d", account.UserID))
		} else {
			// Historical service conversations were once personal. Keep them
			// visible to administrators without merging them into room support.
			view.Title = defaultString(account.Nickname, account.Username)
			view.Subtitle = "历史专属客服"
		}
		return
	}
	if prefix == "agent" {
		view.Title = defaultString(account.Nickname, account.Username) + " 的房间"
		view.Subtitle = "房间号 " + defaultString(account.AgentRoomCode, fmt.Sprintf("代理 #%d", account.UserID))
	}
}

func adminChatMessage(row chat.Message) AdminChatMessage {
	return AdminChatMessage{ID: row.ID, UserID: row.UserID, Username: row.Username, Nickname: row.Nickname, RoomType: row.RoomType, Scope: row.Scope, Content: row.Content, IsStaff: row.UserID == 0 || row.Username == "support", CreatedAt: row.CreatedAt}
}

func validAdminScope(scope, roomType string) error {
	scope, roomType = strings.TrimSpace(scope), strings.TrimSpace(roomType)
	if roomType != "group" && roomType != "service" {
		return apperrors.NewBusinessError("INVALID_REQUEST", "聊天室类型不正确")
	}
	if scope == "" || len(scope) > 64 {
		return apperrors.NewBusinessError("INVALID_REQUEST", "会话标识不正确")
	}
	if roomType == "service" && scope != "lobby" && !strings.HasPrefix(scope, "agent:") && !strings.HasPrefix(scope, "user:") {
		return apperrors.NewBusinessError("INVALID_REQUEST", "客服会话标识不正确")
	}
	if roomType == "group" && scope != "lobby" && !strings.HasPrefix(scope, "agent:") {
		return apperrors.NewBusinessError("INVALID_REQUEST", "群聊会话标识不正确")
	}
	return nil
}

func chatScopeRecipients(db *gorm.DB, scope string) ([]uint64, error) {
	if strings.HasPrefix(scope, "user:") {
		id, err := strconv.ParseUint(strings.TrimPrefix(scope, "user:"), 10, 64)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("invalid chat scope")
		}
		return []uint64{id}, nil
	}
	q := db.Model(&user.User{}).Where("status = ?", 1)
	if strings.HasPrefix(scope, "agent:") {
		id, err := strconv.ParseUint(strings.TrimPrefix(scope, "agent:"), 10, 64)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("invalid chat scope")
		}
		q = q.Where("user_id = ? OR parent_agent_id = ?", id, id)
	} else if scope == "lobby" {
		q = q.Where("parent_agent_id IS NULL AND role <> ?", "agent")
	} else {
		return nil, fmt.Errorf("invalid chat scope")
	}
	var ids []uint64
	return ids, q.Pluck("user_id", &ids).Error
}
