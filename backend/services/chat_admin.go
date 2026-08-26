package services

import (
	"backend/data/models/chat"
	"backend/data/models/lottery"
	"backend/data/models/settings"
	"backend/data/models/user"
	apperrors "backend/errors"
	"backend/ws"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChatAdminService owns all operational chat actions. Group chat is room
// scoped, while customer-service conversations are private per user and show
// that user's owning room in the console.
type ChatAdminService struct{ db *gorm.DB }

type AdminConversation struct {
	Scope         string     `json:"scope"`
	RoomScope     string     `json:"room_scope"`
	GameID        string     `json:"game_id"`
	RoomType      string     `json:"room_type"`
	Title         string     `json:"title"`
	Subtitle      string     `json:"subtitle"`
	UserID        uint64     `json:"user_id,omitempty"`
	Username      string     `json:"username,omitempty"`
	Nickname      string     `json:"nickname,omitempty"`
	LatestText    string     `json:"latest_text"`
	LatestIsStaff bool       `json:"latest_is_staff"`
	LatestType    string     `json:"latest_message_type,omitempty"`
	LatestAt      *time.Time `json:"latest_at,omitempty"`
	MessageCount  int64      `json:"message_count"`
	Pinned        bool       `json:"pinned,omitempty"`
	MutedUntil    *time.Time `json:"muted_until,omitempty"`
	Enabled       bool       `json:"enabled"`
}

type LotteryRoomStatus struct {
	AgentID uint64 `json:"agent_id"`
	GameID  string `json:"game_id"`
	Enabled bool   `json:"enabled"`
}

type AdminConversationList struct {
	Items    []AdminConversation `json:"items"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
}

type AdminChatMessage struct {
	ID                   uint64    `json:"id"`
	UserID               uint64    `json:"user_id"`
	Username             string    `json:"username"`
	Nickname             string    `json:"nickname"`
	RoomType             string    `json:"room_type"`
	Scope                string    `json:"scope"`
	RoomScope            string    `json:"room_scope"`
	GameID               string    `json:"game_id"`
	Content              string    `json:"content"`
	MessageType          string    `json:"message_type"`
	ReferenceID          uint64    `json:"reference_id,omitempty"`
	RedPacketCount       int       `json:"red_packet_count,omitempty"`
	RedPacketTotal       float64   `json:"red_packet_total,omitempty"`
	RedPacketMinTurnover float64   `json:"red_packet_min_turnover,omitempty"`
	RedPacketCover       string    `json:"red_packet_cover,omitempty"`
	IsStaff              bool      `json:"is_staff"`
	CreatedAt            time.Time `json:"created_at"`
}

type ChatRedPacketInput struct {
	Count            int
	TotalAmount      float64
	MinDailyTurnover float64
	Greeting         string
	Cover            string
}

type AdminChatMessageList struct {
	Items        []AdminChatMessage `json:"items"`
	HasMore      bool               `json:"has_more"`
	NextBeforeID uint64             `json:"next_before_id,omitempty"`
}

func NewChatAdminService(db *gorm.DB) *ChatAdminService { return &ChatAdminService{db: db} }

func (s *ChatAdminService) Conversations(roomType, query, channel string, page, pageSize int) (*AdminConversationList, error) {
	return s.conversations(roomType, query, channel, "", page, pageSize)
}

func (s *ChatAdminService) ConversationsForRoom(roomType, query, channel, roomScope string, page, pageSize int) (*AdminConversationList, error) {
	roomScope = strings.TrimSpace(roomScope)
	if !strings.HasPrefix(roomScope, "agent:") {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "房间范围不正确")
	}
	return s.conversations(roomType, query, channel, roomScope, page, pageSize)
}

func (s *ChatAdminService) conversations(roomType, query, channel, roomScope string, page, pageSize int) (*AdminConversationList, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 30
	}
	roomType = strings.TrimSpace(roomType)
	channel = strings.TrimSpace(channel)
	switch channel {
	case "":
	case "service":
		roomType = "service"
	case "room", "lottery":
		roomType = "group"
	default:
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "会话频道不正确")
	}
	if roomType != "" && roomType != "group" && roomType != "service" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "聊天室类型不正确")
	}
	type key struct {
		Scope        string
		RoomScope    string
		GameID       string
		RoomType     string
		LatestID     uint64
		MessageCount int64
	}
	q := s.db.Model(&chat.Message{}).Where("deleted_at IS NULL")
	if roomScope != "" {
		q = q.Where("room_scope = ?", roomScope)
	}
	if roomType != "" {
		q = q.Where("room_type = ?", roomType)
	}
	switch channel {
	case "room":
		q = q.Where("game_id = ?", "lobby")
	case "lottery":
		q = q.Where("game_id NOT IN ?", []string{"lobby", "legacy", "service"})
	}
	if value := strings.TrimSpace(query); value != "" {
		like := "%" + strings.ToLower(value) + "%"
		q = q.Where("LOWER(username) LIKE ? OR LOWER(nickname) LIKE ? OR LOWER(content) LIKE ?", like, like, like)
	}
	var total int64
	if err := q.Select("COUNT(*)").Group("scope, room_scope, game_id, room_type").Count(&total).Error; err != nil {
		return nil, err
	}
	var keys []key
	if err := q.Select("scope, room_scope, game_id, room_type, MAX(id) AS latest_id, COUNT(*) AS message_count").Group("scope, room_scope, game_id, room_type").Order("MAX(id) DESC").Offset((page - 1) * pageSize).Limit(pageSize).Scan(&keys).Error; err != nil {
		return nil, err
	}
	items := make([]AdminConversation, 0, len(keys))
	for _, item := range keys {
		var latest chat.Message
		if err := s.db.First(&latest, item.LatestID).Error; err != nil {
			continue
		}
		latestAt := latest.CreatedAt
		view := AdminConversation{
			Scope: item.Scope, RoomScope: item.RoomScope, GameID: item.GameID, RoomType: item.RoomType,
			LatestText: latest.Content, LatestIsStaff: latest.UserID == 0 || latest.Username == "support", LatestType: defaultString(latest.MessageType, "text"),
			LatestAt: &latestAt, MessageCount: item.MessageCount, Enabled: true,
		}
		if channel == "lottery" {
			view.Enabled = s.lotteryRoomEnabled(view.RoomScope, view.GameID)
		}
		s.fillConversationIdentity(&view)
		s.fillGameIdentity(&view)
		items = append(items, view)
	}
	// A chat channel is a room capability, not a side effect of already having
	// messages. Keep the lobby and every active agent room selectable so an
	// operator can send the first message or red packet after history is cleared.
	if page == 1 && strings.TrimSpace(query) == "" && (roomType == "" || roomType == "group") {
		var bases []AdminConversation
		var err error
		if channel == "lottery" {
			bases, err = s.baseLotteryConversations(roomScope)
		} else {
			bases, err = s.baseGroupConversations(roomScope)
		}
		if err != nil {
			return nil, err
		}
		baseKeys := make(map[string]struct{}, len(bases))
		existing := make(map[string]AdminConversation, len(items))
		for _, item := range items {
			existing[conversationKey(item.Scope, item.RoomScope, item.GameID, item.RoomType)] = item
		}
		pinned := make([]AdminConversation, 0, len(bases))
		missing := int64(0)
		for _, base := range bases {
			key := conversationKey(base.Scope, base.RoomScope, base.GameID, base.RoomType)
			baseKeys[key] = struct{}{}
			if current, exists := existing[key]; exists {
				current.Pinned = base.Pinned
				current.Enabled = base.Enabled
				pinned = append(pinned, current)
				continue
			}
			missing++
			pinned = append(pinned, base)
		}
		remaining := make([]AdminConversation, 0, len(items))
		for _, item := range items {
			if _, isBase := baseKeys[conversationKey(item.Scope, item.RoomScope, item.GameID, item.RoomType)]; !isBase {
				remaining = append(remaining, item)
			}
		}
		total += missing
		items = append(pinned, remaining...)
		if len(items) > pageSize {
			items = items[:pageSize]
		}
	}
	return &AdminConversationList{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *ChatAdminService) baseLotteryConversations(roomScope string) ([]AdminConversation, error) {
	agentQuery := s.db.Model(&user.User{}).Where("role = ? AND status = ?", "agent", 1)
	if roomScope != "" {
		id, err := strconv.ParseUint(strings.TrimPrefix(roomScope, "agent:"), 10, 64)
		if err != nil || id == 0 || roomScope != "agent:"+strconv.FormatUint(id, 10) {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "房间范围不正确")
		}
		agentQuery = agentQuery.Where("user_id = ?", id)
	}
	var agents []user.User
	if err := agentQuery.Order("user_id ASC").Find(&agents).Error; err != nil {
		return nil, err
	}
	var games []lottery.Game
	if err := s.db.Where("enabled = ?", true).Order("sort_order ASC, id ASC").Find(&games).Error; err != nil {
		return nil, err
	}
	items := make([]AdminConversation, 0, len(agents)*len(games))
	for _, agent := range agents {
		scope := "agent:" + strconv.FormatUint(agent.UserID, 10)
		for _, game := range games {
			item := AdminConversation{
				Scope: scope, RoomScope: scope, GameID: game.ID, RoomType: "group",
				LatestText: "暂无聊天记录", Enabled: s.lotteryRoomEnabled(scope, game.ID),
			}
			s.fillConversationIdentity(&item)
			s.fillGameIdentity(&item)
			items = append(items, item)
		}
	}
	return items, nil
}

func conversationKey(scope, roomScope, gameID, roomType string) string {
	return roomType + "\x00" + scope + "\x00" + roomScope + "\x00" + gameID
}

func (s *ChatAdminService) baseGroupConversations(roomScope string) ([]AdminConversation, error) {
	items := make([]AdminConversation, 0)
	query := s.db.Model(&user.User{}).Where("role = ? AND status = ?", "agent", 1)
	if roomScope != "" {
		id, err := strconv.ParseUint(strings.TrimPrefix(roomScope, "agent:"), 10, 64)
		if err != nil || id == 0 || roomScope != "agent:"+strconv.FormatUint(id, 10) {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "房间范围不正确")
		}
		query = query.Where("user_id = ?", id)
	}
	var agents []user.User
	if err := query.Order("user_id ASC").Find(&agents).Error; err != nil {
		return nil, err
	}
	for _, agent := range agents {
		scope := "agent:" + strconv.FormatUint(agent.UserID, 10)
		item := AdminConversation{Scope: scope, RoomScope: scope, GameID: "lobby", RoomType: "group", LatestText: "暂无聊天记录", Pinned: true}
		s.fillConversationIdentity(&item)
		items = append(items, item)
	}
	if roomScope == "" {
		items = append(items, AdminConversation{
			Scope: "lobby", RoomScope: "lobby", GameID: "lobby", RoomType: "group",
			Title: "大厅聊天室", Subtitle: "大厅群消息", LatestText: "暂无聊天记录", Pinned: true,
		})
	}
	return items, nil
}

func (s *ChatAdminService) fillGameIdentity(view *AdminConversation) {
	if view.RoomType != "group" || view.GameID == "lobby" || view.GameID == "legacy" {
		return
	}
	var game lottery.Game
	if err := s.db.Select("id", "name").First(&game, "id = ?", view.GameID).Error; err != nil {
		return
	}
	view.Title = game.Name
	if view.Subtitle == "" {
		view.Subtitle = view.RoomScope
	}
}

func (s *ChatAdminService) lotteryRoomEnabled(roomScope, gameID string) bool {
	agentID, err := strconv.ParseUint(strings.TrimPrefix(roomScope, "agent:"), 10, 64)
	if err != nil || agentID == 0 || roomScope != "agent:"+strconv.FormatUint(agentID, 10) {
		return true
	}
	var setting chat.RoomGameSetting
	if err := s.db.Where("agent_id = ? AND game_id = ?", agentID, gameID).First(&setting).Error; err != nil {
		return true
	}
	return setting.Enabled
}

func (s *ChatAdminService) SetLotteryRoomEnabled(agentID uint64, gameID string, enabled bool) (*LotteryRoomStatus, error) {
	gameID = strings.TrimSpace(gameID)
	if agentID == 0 || gameID == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "房间和彩种不能为空")
	}
	var agent user.User
	if err := s.db.Where("user_id = ? AND role = ?", agentID, "agent").First(&agent).Error; err != nil {
		return nil, apperrors.NewBusinessError("ROOM_NOT_FOUND", "房间不存在")
	}
	var game lottery.Game
	if err := s.db.Where("id = ?", gameID).First(&game).Error; err != nil {
		return nil, apperrors.NewBusinessError("GAME_NOT_FOUND", "彩种不存在")
	}
	row := chat.RoomGameSetting{AgentID: agentID, GameID: gameID, Enabled: enabled}
	if err := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "agent_id"}, {Name: "game_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return nil, apperrors.NewSystemError("LOTTERY_ROOM_UPDATE_FAILED", "保存彩票室状态失败", err)
	}
	return &LotteryRoomStatus{AgentID: agentID, GameID: gameID, Enabled: enabled}, nil
}

func (s *ChatAdminService) LotteryRoomEnabled(agentID uint64, gameID string) bool {
	return s.lotteryRoomEnabled("agent:"+strconv.FormatUint(agentID, 10), strings.TrimSpace(gameID))
}

func (s *ChatAdminService) Messages(scope, roomType, roomScope, gameID string, limit int, beforeID uint64) (*AdminChatMessageList, error) {
	if err := validAdminScope(scope, roomType); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	roomScope, gameID, err := s.normalizeAdminContext(scope, roomType, roomScope, gameID)
	if err != nil {
		return nil, err
	}
	q := s.db.Model(&chat.Message{}).Where(
		"scope = ? AND room_type = ? AND room_scope = ? AND game_id = ? AND deleted_at IS NULL",
		scope, roomType, roomScope, gameID,
	)
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

func (s *ChatAdminService) Reply(scope, roomType, roomScope, gameID, content, operator string) (*AdminChatMessage, error) {
	if err := validAdminScope(scope, roomType); err != nil {
		return nil, err
	}
	content = strings.TrimSpace(content)
	if content == "" || len([]rune(content)) > 500 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "消息长度应为 1–500 个字符")
	}
	// 登录帐号只用于后台审计，会员端统一显示“客服”，避免把 admin
	// 之类的内部帐号名称暴露到会话预览和聊天气泡中。
	nickname := "客服"
	roomScope, gameID, err := s.normalizeAdminContext(scope, roomType, roomScope, gameID)
	if err != nil {
		return nil, err
	}
	row := chat.Message{Username: "support", Nickname: nickname, RoomType: roomType, Scope: scope, RoomScope: roomScope, GameID: gameID, Content: content, MessageType: "text"}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, err
	}
	if recipients, err := chatScopeRecipients(s.db, scope); err == nil {
		notifyChatEvent(s.db, recipients, roomType, row.RoomScope, row.GameID, scope, row.ID)
	}
	view := adminChatMessage(row)
	return &view, nil
}

// SendRedPacket persists one independently funded room envelope. Every send
// owns its count, total, greeting and cover; it is not coupled to a reusable
// campaign pool.
func (s *ChatAdminService) SendRedPacket(scope, roomScope, gameID string, input ChatRedPacketInput, operator string) (*AdminChatMessage, error) {
	if err := validAdminScope(scope, "group"); err != nil {
		return nil, err
	}
	if input.Count < 1 || input.Count > 500 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "红包个数应为 1–500 个")
	}
	totalCents := int64(math.Round(input.TotalAmount * 100))
	if totalCents < int64(input.Count) {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "总金额至少需要保证每个红包 0.01 元")
	}
	if totalCents > 100_000_000 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "单个红包总金额不能超过 100 万元")
	}
	minDailyTurnoverCents := int64(math.Round(input.MinDailyTurnover * 100))
	if minDailyTurnoverCents < 0 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "领取流水要求不能小于 0")
	}
	if minDailyTurnoverCents > 100_000_000_000 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "领取流水要求不能超过 10 亿元")
	}
	greeting := strings.TrimSpace(input.Greeting)
	if greeting == "" {
		greeting = "恭喜发财"
	}
	if len([]rune(greeting)) > 30 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "红包标语不能超过 30 个字符")
	}
	cover := strings.TrimSpace(input.Cover)
	if cover != "classic" && cover != "celebration" && cover != "lucky" {
		cover = "classic"
	}
	roomScope, gameID, err := s.normalizeAdminContext(scope, "group", roomScope, gameID)
	if err != nil {
		return nil, err
	}
	row := chat.Message{
		// The operator account belongs in audit data, never in the member-facing
		// room identity.  A red packet is presented as a room benefit on every
		// client, regardless of which administrator clicked Send.
		Username: "support", Nickname: "房间福利",
		RoomType: "group", Scope: scope, RoomScope: roomScope, GameID: gameID,
		Content: greeting, MessageType: "redpacket", RedPacketCount: input.Count,
		RedPacketTotalCents: totalCents, RedPacketMinTurnoverCents: minDailyTurnoverCents, RedPacketCover: cover,
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		packet := chat.RedPacket{
			MessageID: row.ID, Scope: scope, RoomScope: roomScope, GameID: gameID,
			TotalCents: totalCents, RemainingCents: totalCents, PacketCount: input.Count,
			MinDailyTurnoverCents: minDailyTurnoverCents,
			Greeting:              greeting, Cover: cover, Status: "active",
		}
		if err := tx.Create(&packet).Error; err != nil {
			return err
		}
		row.ReferenceID = packet.ID
		return tx.Model(&row).Update("reference_id", packet.ID).Error
	})
	if err != nil {
		return nil, err
	}
	if recipients, err := chatScopeRecipients(s.db, scope); err == nil {
		notifyChatEvent(s.db, recipients, row.RoomType, row.RoomScope, row.GameID, row.Scope, row.ID)
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
		notifyChatEvent(s.db, recipients, row.RoomType, row.RoomScope, row.GameID, row.Scope, row.ID)
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
	if len([]rune(content)) > 2000 {
		return "", apperrors.NewBusinessError("INVALID_REQUEST", "公告不能超过 2000 个字符")
	}
	current, err := NewSettingsAdminService(s.db).Get()
	if err != nil {
		return "", err
	}
	announcements := current.Announcements
	if len(announcements) == 0 && content != "" {
		announcements = []AnnouncementItem{{ID: "welcome", Title: "大厅公告", Content: content, Enabled: true, PopupOnLogin: true, SortOrder: 10}}
	} else if len(announcements) > 0 && content == "" {
		announcements[0].Enabled = false
	} else if len(announcements) > 0 {
		announcements[0].Content = content
		announcements[0].Enabled = true
	}
	_, encoded, normalizeErr := normalizeAnnouncements(announcements)
	if normalizeErr != nil {
		return "", normalizeErr
	}
	if err := s.db.Model(&settings.SystemConfig{}).Where("id = ?", 1).Updates(map[string]any{"room_notice": content, "announcements_json": encoded}).Error; err != nil {
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
			view.Title = defaultString(account.Nickname, account.Username)
			view.Subtitle = "大厅 · 专属客服"
			if account.ParentAgentID != nil {
				var agent user.User
				if err := s.db.Select("agent_room_code").First(&agent, *account.ParentAgentID).Error; err == nil {
					view.Subtitle = "房间号 " + defaultString(agent.AgentRoomCode, fmt.Sprintf("代理 #%d", *account.ParentAgentID)) + " · 专属客服"
				}
			}
		}
		return
	}
	if prefix == "agent" {
		view.Title = "聊天室"
		view.Subtitle = "房间号 " + defaultString(account.AgentRoomCode, fmt.Sprintf("代理 #%d", account.UserID))
	}
}

func adminChatMessage(row chat.Message) AdminChatMessage {
	return AdminChatMessage{ID: row.ID, UserID: row.UserID, Username: row.Username, Nickname: row.Nickname, RoomType: row.RoomType, Scope: row.Scope, RoomScope: row.RoomScope, GameID: row.GameID, Content: row.Content, MessageType: defaultString(row.MessageType, "text"), ReferenceID: row.ReferenceID, RedPacketCount: row.RedPacketCount, RedPacketTotal: centsToAmount(row.RedPacketTotalCents), RedPacketMinTurnover: centsToAmount(row.RedPacketMinTurnoverCents), RedPacketCover: row.RedPacketCover, IsStaff: row.UserID == 0 || row.Username == "support", CreatedAt: row.CreatedAt}
}

func (s *ChatAdminService) normalizeAdminContext(scope, roomType, roomScope, gameID string) (string, string, error) {
	roomScope = strings.TrimSpace(roomScope)
	gameID = strings.TrimSpace(gameID)
	if roomType == "group" {
		roomScope = defaultString(roomScope, scope)
		if roomScope != scope || (scope != "lobby" && !strings.HasPrefix(scope, "agent:")) {
			return "", "", apperrors.NewBusinessError("INVALID_REQUEST", "群聊与房间号不匹配")
		}
		gameID = defaultString(gameID, "lobby")
		return roomScope, gameID, nil
	}
	gameID = "service"
	if !strings.HasPrefix(scope, "user:") {
		return "", "", apperrors.NewBusinessError("INVALID_REQUEST", "客服会话缺少所属房间")
	}
	userID, err := strconv.ParseUint(strings.TrimPrefix(scope, "user:"), 10, 64)
	if err != nil || userID == 0 {
		return "", "", apperrors.NewBusinessError("INVALID_REQUEST", "客服会话标识不正确")
	}
	var account user.User
	if err := s.db.Select("user_id", "parent_agent_id").First(&account, userID).Error; err != nil {
		return "", "", apperrors.NewBusinessError("USER_NOT_FOUND", "客服用户不存在")
	}
	expectedRoom := chatScope(account, "group")
	if roomScope != "" && roomScope != expectedRoom {
		return "", "", apperrors.NewBusinessError("INVALID_REQUEST", "客服用户与房间号不匹配")
	}
	return expectedRoom, gameID, nil
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

// notifyChatEvent adds only authorized operators to the ordinary room
// recipients. The frame contains message ids and scope metadata, never the
// message body; each console still has to pass its normal API authorization
// before it can load the conversation.
func notifyChatEvent(db *gorm.DB, recipients []uint64, roomType, roomScope, gameID, scope string, messageID uint64) {
	seen := make(map[uint64]struct{}, len(recipients)+4)
	result := make([]uint64, 0, len(recipients)+4)
	appendID := func(id uint64) {
		if id == 0 {
			return
		}
		if _, exists := seen[id]; exists {
			return
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	for _, id := range recipients {
		appendID(id)
	}
	if strings.HasPrefix(roomScope, "agent:") {
		if id, err := strconv.ParseUint(strings.TrimPrefix(roomScope, "agent:"), 10, 64); err == nil {
			appendID(id)
		}
	}
	var adminIDs []uint64
	if err := db.Model(&user.User{}).Where("role = ? AND status = ?", "admin", 1).Pluck("user_id", &adminIDs).Error; err == nil {
		for _, id := range adminIDs {
			appendID(id)
		}
	}
	ws.NotifyChat(result, roomType, roomScope, gameID, scope, messageID)
}
