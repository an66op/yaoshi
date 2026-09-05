package services

import (
	"backend/data/models/chat"
	"backend/data/models/lottery"
	"backend/data/models/settings"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"backend/ws"
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChatAdminService owns all operational chat actions. Group chat is room
// scoped, while customer-service conversations are private per user and stay
// attached to the workspace/room snapshot written with each message.
type ChatAdminService struct{ db *gorm.DB }

type AdminConversation struct {
	// WorkspaceID is the immutable owner copied from the durable message. It is
	// kept server-side so platform listings cannot accidentally merge two
	// conversations merely because a malformed legacy row reused a room scope.
	WorkspaceID      uint64     `json:"-"`
	Scope            string     `json:"scope"`
	RoomScope        string     `json:"room_scope"`
	GameID           string     `json:"game_id"`
	RoomType         string     `json:"room_type"`
	Title            string     `json:"title"`
	Subtitle         string     `json:"subtitle"`
	UserID           uint64     `json:"user_id,omitempty"`
	Username         string     `json:"username,omitempty"`
	Nickname         string     `json:"nickname,omitempty"`
	LatestText       string     `json:"latest_text"`
	LatestIsStaff    bool       `json:"latest_is_staff"`
	LatestType       string     `json:"latest_message_type,omitempty"`
	LatestAt         *time.Time `json:"latest_at,omitempty"`
	LatestMessageID  uint64     `json:"latest_message_id,omitempty"`
	MessageCount     int64      `json:"message_count"`
	UnreadCount      int64      `json:"unread_count,omitempty"`
	Pinned           bool       `json:"pinned,omitempty"`
	MutedUntil       *time.Time `json:"muted_until,omitempty"`
	GroupChatEnabled bool       `json:"group_chat_enabled"`
	LobbyCategory    string     `json:"lobby_category,omitempty"`
	Enabled          bool       `json:"enabled"`
	RoomCode         string     `json:"room_code,omitempty"`
	RoomName         string     `json:"room_name,omitempty"`
	RoomLogo         string     `json:"room_logo,omitempty"`
	OperatorTitle    string     `json:"operator_title,omitempty"`
	OperatorAvatar   string     `json:"operator_avatar,omitempty"`
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
	ID                   uint64     `json:"id"`
	UserID               uint64     `json:"user_id"`
	PublicID             uint64     `json:"public_id,omitempty"`
	Username             string     `json:"username"`
	Nickname             string     `json:"nickname"`
	Avatar               string     `json:"avatar,omitempty"`
	Title                string     `json:"title,omitempty"`
	Badge                string     `json:"badge,omitempty"`
	RoomType             string     `json:"room_type"`
	Scope                string     `json:"scope"`
	RoomScope            string     `json:"room_scope"`
	GameID               string     `json:"game_id"`
	Content              string     `json:"content"`
	MessageType          string     `json:"message_type"`
	ReferenceID          uint64     `json:"reference_id,omitempty"`
	RedPacketCount       int        `json:"red_packet_count,omitempty"`
	RedPacketTotal       float64    `json:"red_packet_total,omitempty"`
	RedPacketMinTurnover float64    `json:"red_packet_min_turnover,omitempty"`
	RedPacketCover       string     `json:"red_packet_cover,omitempty"`
	RedPacketStatus      string     `json:"red_packet_status,omitempty"`
	RedPacketFunding     string     `json:"red_packet_funding_status,omitempty"`
	RedPacketClaimed     int        `json:"red_packet_claimed_count,omitempty"`
	RedPacketRemaining   float64    `json:"red_packet_remaining,omitempty"`
	RedPacketRefunded    float64    `json:"red_packet_refunded,omitempty"`
	RedPacketExpiresAt   *time.Time `json:"red_packet_expires_at,omitempty"`
	RedPacketClosedAt    *time.Time `json:"red_packet_closed_at,omitempty"`
	RedPacketCloseReason string     `json:"red_packet_close_reason,omitempty"`
	IsStaff              bool       `json:"is_staff"`
	CreatedAt            time.Time  `json:"created_at"`
}

type ChatRedPacketInput struct {
	RequestID        string
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

type AdminChatUnreadSummary struct {
	Items       []AdminConversation `json:"items"`
	TotalUnread int64               `json:"total_unread"`
}

type AdminChatReadResult struct {
	Scope             string `json:"scope"`
	RoomScope         string `json:"room_scope"`
	GameID            string `json:"game_id"`
	RoomType          string `json:"room_type"`
	LastReadMessageID uint64 `json:"last_read_message_id"`
}

func NewChatAdminService(db *gorm.DB) *ChatAdminService { return &ChatAdminService{db: db} }

func (s *ChatAdminService) Conversations(roomType, query, channel string, page, pageSize int) (*AdminConversationList, error) {
	return s.conversations(roomType, query, channel, "", page, pageSize)
}

func (s *ChatAdminService) ConversationsForRoom(roomType, query, channel, roomScope string, page, pageSize int) (*AdminConversationList, error) {
	roomScope = strings.TrimSpace(roomScope)
	if !strings.HasPrefix(roomScope, "agent:") && !strings.HasPrefix(roomScope, "tenant:") {
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
		WorkspaceID  uint64
		Scope        string
		RoomScope    string
		GameID       string
		RoomType     string
		LatestID     uint64
		MessageCount int64
	}
	q := s.db.Model(&chat.Message{}).Where("deleted_at IS NULL")
	if roomScope != "" {
		workspace, err := WorkspaceByScope(s.db, roomScope)
		if err != nil || workspace.ID == 0 {
			return nil, apperrors.NewBusinessError("ROOM_NOT_FOUND", "消息所属房间不存在")
		}
		q = q.Where("workspace_id = ? AND room_scope = ?", workspace.ID, roomScope)
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
	if err := q.Select("COUNT(*)").Group("workspace_id, scope, room_scope, game_id, room_type").Count(&total).Error; err != nil {
		return nil, err
	}
	var keys []key
	if err := q.Select("workspace_id, scope, room_scope, game_id, room_type, MAX(id) AS latest_id, COUNT(*) AS message_count").Group("workspace_id, scope, room_scope, game_id, room_type").Order("MAX(id) DESC").Offset((page - 1) * pageSize).Limit(pageSize).Scan(&keys).Error; err != nil {
		return nil, err
	}
	items := make([]AdminConversation, 0, len(keys))
	for _, item := range keys {
		var latest chat.Message
		if err := s.db.Where("id = ? AND workspace_id = ?", item.LatestID, item.WorkspaceID).First(&latest).Error; err != nil {
			continue
		}
		latestAt := latest.CreatedAt
		view := AdminConversation{
			WorkspaceID: item.WorkspaceID,
			Scope:       item.Scope, RoomScope: item.RoomScope, GameID: item.GameID, RoomType: item.RoomType,
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
			existing[conversationKey(item.WorkspaceID, item.Scope, item.RoomScope, item.GameID, item.RoomType)] = item
		}
		pinned := make([]AdminConversation, 0, len(bases))
		missing := int64(0)
		for _, base := range bases {
			key := conversationKey(base.WorkspaceID, base.Scope, base.RoomScope, base.GameID, base.RoomType)
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
			if _, isBase := baseKeys[conversationKey(item.WorkspaceID, item.Scope, item.RoomScope, item.GameID, item.RoomType)]; !isBase {
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
	workspaceQuery := s.db.Model(&workspacemodel.Workspace{}).Where("type IN ? AND status = ?", []string{workspacemodel.TypeTenant, workspacemodel.TypeAgent}, 1)
	if roomScope != "" {
		workspace, err := WorkspaceByScope(s.db, roomScope)
		if err != nil || workspace.ID == 0 {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "房间范围不正确")
		}
		workspaceQuery = workspaceQuery.Where("id = ?", workspace.ID)
	}
	var workspaces []workspacemodel.Workspace
	if err := workspaceQuery.Order("id ASC").Find(&workspaces).Error; err != nil {
		return nil, err
	}
	var games []lottery.Game
	if err := s.db.Table("lottery_games AS games").Select("games.*").
		Joins("LEFT JOIN lottery_lobby_categories AS categories ON categories.name = games.lobby_category").
		Where("games.enabled = ?", true).
		Order("COALESCE(categories.sort_order, 2147483647) ASC, games.lobby_sort_order ASC, games.sort_order ASC, games.id ASC").
		Scan(&games).Error; err != nil {
		return nil, err
	}
	items := make([]AdminConversation, 0, len(workspaces)*len(games))
	for _, workspace := range workspaces {
		scope := workspace.Scope
		for _, game := range games {
			item := AdminConversation{
				WorkspaceID: workspace.ID,
				Scope:       scope, RoomScope: scope, GameID: game.ID, RoomType: "group",
				LatestText: "暂无聊天记录", Enabled: s.lotteryRoomEnabled(scope, game.ID),
			}
			s.fillConversationIdentity(&item)
			s.fillGameIdentity(&item)
			items = append(items, item)
		}
	}
	return items, nil
}

func conversationKey(workspaceID uint64, scope, roomScope, gameID, roomType string) string {
	return strconv.FormatUint(workspaceID, 10) + "\x00" + roomType + "\x00" + scope + "\x00" + roomScope + "\x00" + gameID
}

func (s *ChatAdminService) baseGroupConversations(roomScope string) ([]AdminConversation, error) {
	items := make([]AdminConversation, 0)
	query := s.db.Model(&workspacemodel.Workspace{}).Where("type IN ? AND status = ?", []string{workspacemodel.TypeTenant, workspacemodel.TypeAgent}, 1)
	if roomScope != "" {
		workspace, err := WorkspaceByScope(s.db, roomScope)
		if err != nil || workspace.ID == 0 {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "房间范围不正确")
		}
		query = query.Where("id = ?", workspace.ID)
	}
	var workspaces []workspacemodel.Workspace
	if err := query.Order("id ASC").Find(&workspaces).Error; err != nil {
		return nil, err
	}
	for _, workspace := range workspaces {
		scope := workspace.Scope
		item := AdminConversation{WorkspaceID: workspace.ID, Scope: scope, RoomScope: scope, GameID: "lobby", RoomType: "group", LatestText: "暂无聊天记录", Pinned: true}
		s.fillConversationIdentity(&item)
		items = append(items, item)
	}
	if roomScope == "" {
		platform, err := WorkspaceByScope(s.db, "lobby")
		if err != nil || platform.ID == 0 {
			return nil, apperrors.NewBusinessError("ROOM_NOT_FOUND", "平台大厅不存在")
		}
		items = append(items, AdminConversation{
			WorkspaceID: platform.ID,
			Scope:       "lobby", RoomScope: "lobby", GameID: "lobby", RoomType: "group",
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
	if err := s.db.Select("id", "name", "lobby_category").First(&game, "id = ?", view.GameID).Error; err != nil {
		return
	}
	view.Title = game.Name
	view.LobbyCategory = game.LobbyCategory
	if view.Subtitle == "" {
		view.Subtitle = view.RoomScope
	}
}

func (s *ChatAdminService) lotteryRoomEnabled(roomScope, gameID string) bool {
	workspace, err := WorkspaceByScope(s.db, roomScope)
	if err != nil || workspace.ID == 0 {
		return false
	}
	enabled, err := WorkspaceGameEnabled(s.db, workspace.ID, gameID)
	return err == nil && enabled
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
	workspace, err := WorkspaceForAccount(s.db, agent)
	if err != nil {
		return nil, apperrors.NewBusinessError("ROOM_NOT_FOUND", "房间不存在")
	}
	return s.SetLotteryRoomEnabledForWorkspace(workspace, gameID, enabled)
}

func (s *ChatAdminService) SetLotteryRoomEnabledForWorkspace(workspace workspacemodel.Workspace, gameID string, enabled bool) (*LotteryRoomStatus, error) {
	gameID = strings.TrimSpace(gameID)
	if workspace.ID == 0 || gameID == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "房间和彩种不能为空")
	}
	// Use persisted ownership, never a caller-supplied owner/scope or an orphan
	// workspace ID, when writing the room switch and publishing its update.
	if err := s.db.First(&workspace, workspace.ID).Error; err != nil || !validGameWorkspaceType(workspace.Type) {
		return nil, apperrors.NewBusinessError("ROOM_NOT_FOUND", "房间不存在")
	}
	var game lottery.Game
	if err := s.db.Where("id = ?", gameID).First(&game).Error; err != nil {
		return nil, apperrors.NewBusinessError("GAME_NOT_FOUND", "彩种不存在")
	}
	row := chat.RoomGameSetting{WorkspaceID: workspace.ID, AgentID: workspace.OwnerUserID, GameID: gameID, Enabled: enabled}
	if err := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "workspace_id"}, {Name: "game_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return nil, apperrors.NewSystemError("LOTTERY_ROOM_UPDATE_FAILED", "保存房间游戏状态失败", err)
	}
	// Room game switches are runtime configuration. Notify only this room after
	// the database write succeeds; members in every other workspace must neither
	// refresh nor observe which game was changed here.
	ws.NotifyGameCatalogChanged(workspace.ID, workspace.Scope, workspace.RoomCode, gameID,
		enabled && game.Enabled && strings.TrimSpace(game.LobbyCategory) != "" && workspace.Status == 1)
	return &LotteryRoomStatus{AgentID: workspace.OwnerUserID, GameID: gameID, Enabled: enabled}, nil
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
	workspace, err := WorkspaceByScope(s.db, roomScope)
	if err != nil || workspace.ID == 0 {
		return nil, apperrors.NewBusinessError("ROOM_NOT_FOUND", "消息所属房间不存在")
	}
	if roomType == "service" {
		if err := s.authorizeServiceConversation(workspace.ID, scope, roomScope, gameID); err != nil {
			return nil, err
		}
	}
	q := s.db.Model(&chat.Message{}).Where(
		"workspace_id = ? AND scope = ? AND room_type = ? AND room_scope = ? AND game_id = ? AND deleted_at IS NULL",
		workspace.ID, scope, roomType, roomScope, gameID,
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
	cfg, err := NewSettingsAdminService(s.db).GetForWorkspace(workspace.ID)
	if err != nil {
		return nil, err
	}
	identities, err := loadChatMemberIdentities(s.db, workspace.ID, rows, cfg)
	if err != nil {
		return nil, err
	}
	items := make([]AdminChatMessage, 0, len(rows))
	packetStates, err := loadRedPacketViewStates(s.db, rows)
	if err != nil {
		return nil, err
	}
	for index := len(rows) - 1; index >= 0; index-- {
		item := adminChatMessage(rows[index])
		applyAdminRedPacketState(&item, packetStates[rows[index].ID])
		applyAdminChatIdentity(&item, identities[item.UserID])
		items = append(items, item)
	}
	next := uint64(0)
	if len(items) > 0 {
		next = items[0].ID
	}
	return &AdminChatMessageList{Items: items, HasMore: hasMore, NextBeforeID: next}, nil
}

// UnreadServiceMessages returns the authoritative unread state for member
// messages in private customer-service conversations. roomScope is empty only
// for platform admins; room operators always pass their authenticated scope.
func (s *ChatAdminService) UnreadServiceMessages(operatorUserID uint64, roomScope string, limit int) (*AdminChatUnreadSummary, error) {
	if operatorUserID == 0 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "操作员身份不正确")
	}
	roomScope = strings.TrimSpace(roomScope)
	if roomScope != "" && !strings.HasPrefix(roomScope, "agent:") && !strings.HasPrefix(roomScope, "tenant:") {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "房间范围不正确")
	}
	if limit < 1 || limit > 100 {
		limit = 30
	}

	base := unreadServiceMessageQuery(s.db, operatorUserID, roomScope)

	var totalUnread int64
	if err := base.Count(&totalUnread).Error; err != nil {
		return nil, err
	}
	type unreadConversation struct {
		WorkspaceID     uint64
		Scope           string
		RoomScope       string
		GameID          string
		RoomType        string
		LatestMessageID uint64
		UnreadCount     int64
	}
	var groups []unreadConversation
	if err := base.Select(`message.workspace_id, message.scope, message.room_scope, message.game_id, message.room_type,
		MAX(message.id) AS latest_message_id, COUNT(*) AS unread_count`).
		Group("message.workspace_id, message.scope, message.room_scope, message.game_id, message.room_type").
		Order("MAX(message.id) DESC").Limit(limit).Scan(&groups).Error; err != nil {
		return nil, err
	}
	items := make([]AdminConversation, 0, len(groups))
	for _, group := range groups {
		var latest chat.Message
		if err := s.db.Where("id = ? AND workspace_id = ? AND deleted_at IS NULL", group.LatestMessageID, group.WorkspaceID).First(&latest).Error; err != nil {
			continue
		}
		latestAt := latest.CreatedAt
		view := AdminConversation{
			WorkspaceID: group.WorkspaceID,
			Scope:       group.Scope, RoomScope: group.RoomScope, GameID: group.GameID, RoomType: group.RoomType,
			LatestText: latest.Content, LatestIsStaff: false, LatestType: defaultString(latest.MessageType, "text"),
			LatestAt: &latestAt, LatestMessageID: group.LatestMessageID, UnreadCount: group.UnreadCount, Enabled: true,
		}
		s.fillConversationIdentity(&view)
		items = append(items, view)
	}
	return &AdminChatUnreadSummary{Items: items, TotalUnread: totalUnread}, nil
}

func unreadServiceMessageQuery(db *gorm.DB, operatorUserID uint64, roomScope string) *gorm.DB {
	query := db.Table("member_chat_messages AS message").
		Joins(`LEFT JOIN member_chat_read_cursors AS cursor
			ON cursor.operator_user_id = ?
			AND cursor.workspace_id = message.workspace_id
			AND cursor.scope = message.scope
			AND cursor.room_scope = message.room_scope
			AND cursor.game_id = message.game_id
			AND cursor.room_type = message.room_type`, operatorUserID).
		Where(`message.deleted_at IS NULL
			AND message.room_type = ?
			AND message.user_id <> 0
			AND message.username <> ?
			AND message.id > COALESCE(cursor.last_read_message_id, 0)
			AND message.scope = CONCAT('user:', message.user_id)
			AND EXISTS (
				SELECT 1 FROM "user" AS account
				WHERE account.user_id = message.user_id
					AND account.role = 'member'
					AND account.deleted_at IS NULL
			)
			AND EXISTS (
				SELECT 1 FROM workspaces AS owning_workspace
				WHERE owning_workspace.id = message.workspace_id
					AND owning_workspace.scope = message.room_scope
			)
			AND NOT EXISTS (
				SELECT 1 FROM workspace_robot_profiles AS robot
				WHERE robot.workspace_id = message.workspace_id
					AND robot.user_id = message.user_id
			)`, "service", "support")
	if value := strings.TrimSpace(roomScope); value != "" {
		query = query.Where("message.room_scope = ?", value)
	}
	return query
}

func serviceConversationMessageQuery(db *gorm.DB, workspaceID uint64, scope, roomScope, gameID string) *gorm.DB {
	return db.Model(&chat.Message{}).
		Where("workspace_id = ? AND scope = ? AND room_type = ? AND room_scope = ? AND game_id = ? AND deleted_at IS NULL",
			workspaceID, scope, "service", roomScope, gameID)
}

func serviceConversationAccessQuery(db *gorm.DB, workspaceID, userID uint64, scope, roomScope, gameID string) *gorm.DB {
	return db.Raw(`
		SELECT (
			EXISTS (
				SELECT 1 FROM member_chat_messages AS message
				WHERE message.workspace_id = ?
					AND message.user_id IN (0, ?)
					AND message.scope = ?
					AND message.room_type = 'service'
					AND message.room_scope = ?
					AND message.game_id = ?
					AND message.deleted_at IS NULL
			) OR EXISTS (
				SELECT 1
				FROM workspace_memberships AS membership
				JOIN "user" AS account ON account.user_id = membership.user_id
				WHERE membership.workspace_id = ?
					AND membership.user_id = ?
					AND membership.status = 1
					AND account.workspace_id = membership.workspace_id
					AND account.role = 'member'
					AND account.status = 1
					AND account.deleted_at IS NULL
			)
		) AS allowed
	`, workspaceID, userID, scope, roomScope, gameID, workspaceID, userID)
}

// authorizeServiceConversation preserves old-room history without allowing a
// room operator to enumerate or message arbitrary members. A room may access a
// service context only when it already owns a durable message in that exact
// context, or when the member currently has the room's active membership (the
// latter permits a legitimate first staff message in a new room).
func (s *ChatAdminService) authorizeServiceConversation(workspaceID uint64, scope, roomScope, gameID string) error {
	userID, err := strconv.ParseUint(strings.TrimPrefix(scope, "user:"), 10, 64)
	if err != nil || userID == 0 || workspaceID == 0 {
		return apperrors.NewBusinessError("INVALID_REQUEST", "客服会话标识不正确")
	}
	var allowed bool
	if err := serviceConversationAccessQuery(s.db, workspaceID, userID, scope, roomScope, gameID).Scan(&allowed).Error; err != nil {
		return err
	}
	return requireServiceConversationAccess(allowed)
}

func requireServiceConversationAccess(allowed bool) error {
	if allowed {
		return nil
	}
	return apperrors.NewBusinessError("FORBIDDEN", "该房间无权访问此客服会话")
}

// MarkServiceConversationRead advances, but never rewinds, one operator's
// cursor. allowedRoomScope is supplied by agent/tenant controllers and keeps
// their existing workspace boundary authoritative; platform admins pass "".
func (s *ChatAdminService) MarkServiceConversationRead(operatorUserID uint64, allowedRoomScope, scope, roomScope, gameID string, throughMessageID uint64) (*AdminChatReadResult, error) {
	if operatorUserID == 0 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "操作员身份不正确")
	}
	if err := validAdminScope(scope, "service"); err != nil {
		return nil, err
	}
	normalizedRoom, normalizedGame, err := s.normalizeAdminContext(scope, "service", roomScope, gameID)
	if err != nil {
		return nil, err
	}
	if allowed := strings.TrimSpace(allowedRoomScope); allowed != "" && allowed != normalizedRoom {
		return nil, apperrors.NewBusinessError("FORBIDDEN", "不能更新其他房间客服的已读状态")
	}
	workspace, err := WorkspaceByScope(s.db, normalizedRoom)
	if err != nil || workspace.ID == 0 {
		return nil, apperrors.NewBusinessError("ROOM_NOT_FOUND", "消息所属房间不存在")
	}
	if err := s.authorizeServiceConversation(workspace.ID, scope, normalizedRoom, normalizedGame); err != nil {
		return nil, err
	}
	var latestMessageID uint64
	conversationMessages := serviceConversationMessageQuery(s.db, workspace.ID, scope, normalizedRoom, normalizedGame)
	if err := conversationMessages.
		Select("COALESCE(MAX(id), 0)").Scan(&latestMessageID).Error; err != nil {
		return nil, err
	}
	if throughMessageID > 0 {
		var matching int64
		if err := conversationMessages.Where("id = ?", throughMessageID).
			Count(&matching).Error; err != nil {
			return nil, err
		}
		if matching == 0 {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "已读消息不属于当前客服会话")
		}
	}
	if throughMessageID == 0 {
		throughMessageID = latestMessageID
	}
	if throughMessageID == 0 {
		return &AdminChatReadResult{Scope: scope, RoomScope: normalizedRoom, GameID: normalizedGame, RoomType: "service"}, nil
	}
	cursor := chat.ReadCursor{
		OperatorUserID: operatorUserID, WorkspaceID: workspace.ID, Scope: scope,
		RoomScope: normalizedRoom, GameID: normalizedGame, RoomType: "service", LastReadMessageID: throughMessageID,
	}
	if err := s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "operator_user_id"}, {Name: "workspace_id"}, {Name: "scope"}, {Name: "room_scope"}, {Name: "game_id"}, {Name: "room_type"}},
		DoUpdates: clause.Assignments(map[string]any{
			"last_read_message_id": gorm.Expr("GREATEST(member_chat_read_cursors.last_read_message_id, EXCLUDED.last_read_message_id)"),
			"updated_at":           gorm.Expr("CURRENT_TIMESTAMP"),
		}),
	}).Create(&cursor).Error; err != nil {
		return nil, err
	}
	var stored chat.ReadCursor
	if err := s.db.Where("operator_user_id = ? AND workspace_id = ? AND scope = ? AND room_scope = ? AND game_id = ? AND room_type = ?",
		operatorUserID, workspace.ID, scope, normalizedRoom, normalizedGame, "service").First(&stored).Error; err != nil {
		return nil, err
	}
	return &AdminChatReadResult{Scope: scope, RoomScope: normalizedRoom, GameID: normalizedGame, RoomType: "service", LastReadMessageID: stored.LastReadMessageID}, nil
}

func (s *ChatAdminService) Reply(scope, roomType, roomScope, gameID, content, operator string) (*AdminChatMessage, error) {
	if err := validAdminScope(scope, roomType); err != nil {
		return nil, err
	}
	content = strings.TrimSpace(content)
	if content == "" || len([]rune(content)) > 500 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "消息长度应为 1–500 个字符")
	}
	roomScope, gameID, err := s.normalizeAdminContext(scope, roomType, roomScope, gameID)
	if err != nil {
		return nil, err
	}
	workspace, err := WorkspaceByScope(s.db, roomScope)
	if err != nil || workspace.ID == 0 {
		return nil, apperrors.NewBusinessError("ROOM_NOT_FOUND", "消息所属房间不存在")
	}
	if roomType == "service" {
		if err := s.authorizeServiceConversation(workspace.ID, scope, roomScope, gameID); err != nil {
			return nil, err
		}
	}
	cfg, err := NewSettingsAdminService(s.db).GetForWorkspace(workspace.ID)
	if err != nil {
		return nil, err
	}
	// Login credentials remain audit-only. The durable room message uses the
	// operator identity configured by that exact workspace.
	nickname := defaultString(strings.TrimSpace(cfg.ChatNickname), "客服")
	row := chat.Message{WorkspaceID: workspace.ID, Username: "support", Nickname: nickname, RoomType: roomType, Scope: scope, RoomScope: roomScope, GameID: gameID, Content: content, MessageType: "text"}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, err
	}
	if recipients, err := chatScopeRecipients(s.db, scope); err == nil {
		notifyChatEvent(s.db, recipients, row, "created")
	}
	view := adminChatMessage(row)
	if identities, identityErr := loadChatMemberIdentities(s.db, workspace.ID, []chat.Message{row}, cfg); identityErr == nil {
		applyAdminChatIdentity(&view, identities[0])
	}
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
	if gameID != "lobby" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "房间红包只能发送到房间大厅")
	}
	workspace, err := WorkspaceByScope(s.db, roomScope)
	if err != nil || workspace.ID == 0 {
		return nil, apperrors.NewBusinessError("ROOM_NOT_FOUND", "红包所属房间不存在")
	}
	requestID := strings.TrimSpace(input.RequestID)
	if requestID != "" && (len(requestID) < 8 || len(requestID) > 96) {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "请求标识不正确")
	}
	row := chat.Message{
		// The operator account belongs in audit data, never in the member-facing
		// room identity.  A red packet is presented as a room benefit on every
		// client, regardless of which administrator clicked Send.
		WorkspaceID: workspace.ID, Username: "support", Nickname: "房间福利",
		RoomType: "group", Scope: scope, RoomScope: roomScope, GameID: gameID,
		Content: greeting, MessageType: "redpacket", RedPacketCount: input.Count,
		RedPacketTotalCents: totalCents, RedPacketMinTurnoverCents: minDailyTurnoverCents, RedPacketCover: cover,
	}
	createdNew := false
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if requestID != "" {
			var existing chat.RedPacket
			lookup := tx.Where("workspace_id = ? AND request_id = ?", workspace.ID, requestID).First(&existing)
			if lookup.Error == nil {
				if err := tx.First(&row, existing.MessageID).Error; err != nil {
					return err
				}
				return nil
			}
			if lookup.Error != gorm.ErrRecordNotFound {
				return lookup.Error
			}
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		expiresAt := time.Now().UTC().Add(24 * time.Hour)
		packet := chat.RedPacket{
			WorkspaceID: workspace.ID, RequestID: requestID, MessageID: row.ID, Scope: scope, RoomScope: roomScope, GameID: gameID,
			FundingUserID: workspace.OwnerUserID,
			TotalCents:    totalCents, RemainingCents: totalCents, PacketCount: input.Count,
			MinDailyTurnoverCents: minDailyTurnoverCents,
			Greeting:              greeting, Cover: cover, Status: "active", FundingStatus: "reserved", ExpiresAt: &expiresAt,
		}
		if err := tx.Create(&packet).Error; err != nil {
			return err
		}
		if workspace.OwnerUserID == 0 {
			return apperrors.NewBusinessError("ROOM_FUND_NOT_FOUND", "房间福利账户不存在")
		}
		var funder user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND workspace_id = ? AND status = ?", workspace.OwnerUserID, workspace.ID, 1).
			First(&funder).Error; err != nil {
			return apperrors.NewBusinessError("ROOM_FUND_NOT_FOUND", "房间福利账户不存在或已停用")
		}
		if funder.BalanceCents < totalCents {
			return apperrors.NewBusinessError("ROOM_FUND_INSUFFICIENT", "房间福利余额不足，请先为房间账户上分")
		}
		before := funder.BalanceCents
		after := before - totalCents
		if err := tx.Model(&funder).Update("balance_cents", after).Error; err != nil {
			return err
		}
		if err := tx.Create(&user.BalanceTransaction{
			WorkspaceID: workspace.ID, UserID: funder.UserID,
			Reference:   "redpacket_reserve:" + strconv.FormatUint(packet.ID, 10),
			AmountCents: -totalCents, BeforeCents: before, AfterCents: after,
			Type: "redpacket_reserve", Remark: "房间红包预留 · " + greeting,
			Operator: defaultString(strings.TrimSpace(operator), "后台管理员"),
		}).Error; err != nil {
			return err
		}
		row.ReferenceID = packet.ID
		if err := tx.Model(&row).Update("reference_id", packet.ID).Error; err != nil {
			return err
		}
		createdNew = true
		return nil
	})
	if err != nil {
		// A concurrent backend may have committed the same request id while this
		// transaction was waiting on the unique index. Return that durable result
		// instead of charging the room a second time.
		if requestID != "" {
			var existing chat.RedPacket
			if lookupErr := s.db.Where("workspace_id = ? AND request_id = ?", workspace.ID, requestID).First(&existing).Error; lookupErr == nil {
				if messageErr := s.db.First(&row, existing.MessageID).Error; messageErr == nil {
					createdNew = false
					err = nil
				}
			}
		}
		if err != nil {
			return nil, err
		}
	}
	if createdNew {
		if recipients, err := chatScopeRecipients(s.db, scope); err == nil {
			notifyChatEvent(s.db, recipients, row, "created")
		}
	}
	view := adminChatMessage(row)
	var persistedPacket chat.RedPacket
	if row.ReferenceID > 0 && s.db.First(&persistedPacket, row.ReferenceID).Error == nil {
		applyAdminRedPacketState(&view, redPacketViewState{Status: persistedPacket.Status, FundingStatus: persistedPacket.FundingStatus, ClaimedCount: persistedPacket.ClaimedCount, RemainingCents: persistedPacket.RemainingCents, RefundedCents: persistedPacket.RefundedCents, ExpiresAt: persistedPacket.ExpiresAt, ClosedAt: persistedPacket.ClosedAt, CloseReason: persistedPacket.CloseReason})
	}
	if cfg, cfgErr := NewSettingsAdminService(s.db).GetForWorkspace(workspace.ID); cfgErr == nil {
		if identities, identityErr := loadChatMemberIdentities(s.db, workspace.ID, []chat.Message{row}, cfg); identityErr == nil {
			applyAdminChatIdentity(&view, identities[0])
		}
	}
	return &view, nil
}

func (s *ChatAdminService) DeleteMessage(id uint64, operator string) error {
	now := time.Now().UTC()
	var row chat.Message
	redPacketChanged := false
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("deleted_at IS NULL").First(&row, id).Error; err != nil {
			return apperrors.NewBusinessError("NOT_FOUND", "消息不存在或已撤回")
		}
		if row.MessageType == "redpacket" && row.ReferenceID > 0 {
			changed, err := closeRedPacketTx(tx, row.ReferenceID, defaultString(strings.TrimSpace(operator), "后台管理员"), "消息已撤回", "closed")
			if err != nil {
				return err
			}
			redPacketChanged = changed
		}
		return tx.Model(&row).Updates(map[string]any{"deleted_at": now, "deleted_by": defaultString(strings.TrimSpace(operator), "后台管理员")}).Error
	})
	if err != nil {
		return err
	}
	if redPacketChanged {
		notifyRedPacketMessageUpdated(s.db, row)
	}
	if recipients, err := chatScopeRecipients(s.db, row.Scope); err == nil {
		notifyChatEvent(s.db, recipients, row, "deleted")
	}
	return nil
}

// CloseExpiredRedPackets returns every unclaimed reserve to its owning room.
// It is safe to run concurrently: each packet is locked and only active rows
// can transition to expired.
func (s *ChatAdminService) CloseExpiredRedPackets(limit int) error {
	return s.CloseExpiredRedPacketsContext(context.Background(), limit)
}

func (s *ChatAdminService) CloseExpiredRedPacketsContext(ctx context.Context, limit int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	db := s.db.WithContext(ctx)
	var ids []uint64
	if err := db.Model(&chat.RedPacket{}).
		Where("status = ? AND expires_at IS NOT NULL AND expires_at <= ?", "active", time.Now().UTC()).
		Order("id ASC").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return err
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		changed := false
		if err := db.Transaction(func(tx *gorm.DB) error {
			var err error
			changed, err = closeRedPacketTx(tx, id, "系统", "红包已过期", "expired")
			return err
		}); err != nil {
			return err
		}
		if changed {
			notifyRedPacketMessageUpdatedByPacketID(db, id)
		}
	}
	return nil
}

func closeRedPacketTx(tx *gorm.DB, packetID uint64, operator, reason, closeStatus string) (bool, error) {
	var packet chat.RedPacket
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&packet, packetID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, err
	}
	if packet.Status != "active" {
		return false, nil
	}
	plan, err := planRedPacketClose(packet)
	if err != nil {
		return false, err
	}
	if plan.RefundCents > 0 {
		var funder user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND workspace_id = ?", packet.FundingUserID, packet.WorkspaceID).
			First(&funder).Error; err != nil {
			return false, err
		}
		before := funder.BalanceCents
		after := before + plan.RefundCents
		if err := tx.Model(&funder).Update("balance_cents", after).Error; err != nil {
			return false, err
		}
		if err := tx.Create(&user.BalanceTransaction{
			WorkspaceID: packet.WorkspaceID, UserID: funder.UserID,
			Reference:   "redpacket_refund:" + strconv.FormatUint(packet.ID, 10),
			AmountCents: plan.RefundCents, BeforeCents: before, AfterCents: after,
			Type: "redpacket_refund", Remark: "房间红包退回 · " + defaultString(strings.TrimSpace(reason), "红包已关闭"),
			Operator: defaultString(strings.TrimSpace(operator), "系统"),
		}).Error; err != nil {
			return false, err
		}
	}
	now := time.Now().UTC()
	if closeStatus != "expired" && closeStatus != "closed" {
		closeStatus = "closed"
	}
	result := tx.Model(&packet).Where("status = ?", "active").Updates(map[string]any{
		"remaining_cents": 0,
		// Only money actually returned to a room account is a refund. Historical
		// unfunded packets are closed without minting points and without
		// pretending their discarded remainder was refunded.
		"refunded_cents": plan.FinalRefundedCents,
		"status":         closeStatus,
		"funding_status": plan.FinalFundingStatus,
		"closed_at":      now,
		"closed_by":      defaultString(strings.TrimSpace(operator), "系统"),
		"close_reason":   defaultString(strings.TrimSpace(reason), "红包已关闭"),
	})
	return result.RowsAffected == 1, result.Error
}

type redPacketClosePlan struct {
	RefundCents        int64
	FinalRefundedCents int64
	FinalFundingStatus string
}

func redPacketFundingMayRelease(packet chat.RedPacket) bool {
	if packet.FundingUserID == 0 || packet.RefundedCents != 0 {
		return false
	}
	return packet.FundingStatus == "reserved" || packet.FundingStatus == "partially_released"
}

// planRedPacketClose is the fail-closed funding state machine shared by
// expiry and manual message withdrawal. It is intentionally pure so every
// allowed transition can be tested without touching account data.
func planRedPacketClose(packet chat.RedPacket) (redPacketClosePlan, error) {
	if packet.RemainingCents < 0 || packet.RefundedCents < 0 ||
		packet.RemainingCents+packet.RefundedCents > packet.TotalCents {
		return redPacketClosePlan{}, fmt.Errorf("红包金额状态不一致")
	}
	switch packet.FundingStatus {
	case "reserved", "partially_released":
		if !redPacketFundingMayRelease(packet) {
			return redPacketClosePlan{}, fmt.Errorf("红包预留资金状态不一致")
		}
		if packet.RemainingCents == 0 {
			return redPacketClosePlan{FinalRefundedCents: packet.RefundedCents, FinalFundingStatus: "released"}, nil
		}
		return redPacketClosePlan{
			RefundCents: packet.RemainingCents, FinalRefundedCents: packet.RefundedCents + packet.RemainingCents,
			FinalFundingStatus: "refunded",
		}, nil
	case "legacy_unfunded":
		return redPacketClosePlan{FinalRefundedCents: packet.RefundedCents, FinalFundingStatus: "legacy_unfunded"}, nil
	case "released", "refunded":
		if packet.RemainingCents != 0 {
			return redPacketClosePlan{}, fmt.Errorf("已释放红包仍有未处理余额")
		}
		return redPacketClosePlan{FinalRefundedCents: packet.RefundedCents, FinalFundingStatus: packet.FundingStatus}, nil
	default:
		return redPacketClosePlan{}, fmt.Errorf("未知红包资金状态")
	}
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

func (s *ChatAdminService) SetRoomGroupChatEnabled(agentID uint64, enabled bool) (*user.User, error) {
	if agentID == 0 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "房间不能为空")
	}
	var account user.User
	if err := s.db.Where("user_id = ? AND role = ?", agentID, "agent").First(&account).Error; err != nil {
		return nil, apperrors.NewBusinessError("ROOM_NOT_FOUND", "房间不存在")
	}
	if err := s.db.Model(&account).Update("group_chat_enabled", enabled).Error; err != nil {
		return nil, err
	}
	account.GroupChatEnabled = enabled
	return &account, nil
}

func (s *ChatAdminService) SetAnnouncement(content string) (string, error) {
	return s.SetAnnouncementForWorkspace(0, content)
}

func (s *ChatAdminService) SetAnnouncementForWorkspace(workspaceID uint64, content string) (string, error) {
	content = strings.TrimSpace(content)
	if len([]rune(content)) > 2000 {
		return "", apperrors.NewBusinessError("INVALID_REQUEST", "公告不能超过 2000 个字符")
	}
	current, err := NewSettingsAdminService(s.db).GetForWorkspace(workspaceID)
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
	query := s.db.Model(&settings.SystemConfig{})
	if workspaceID > 0 {
		query = query.Where("workspace_id = ?", workspaceID)
	} else {
		var platform workspacemodel.Workspace
		if err := s.db.Select("id").Where("type = ?", workspacemodel.TypePlatform).First(&platform).Error; err != nil {
			return "", err
		}
		query = query.Where("workspace_id = ?", platform.ID)
	}
	if err := query.Updates(map[string]any{"room_notice": content, "announcements_json": encoded}).Error; err != nil {
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
	if workspace, err := WorkspaceByScope(s.db, view.RoomScope); err == nil {
		view.WorkspaceID = workspace.ID
		view.RoomCode, view.RoomName, view.RoomLogo = workspace.RoomCode, workspace.Name, workspace.Logo
		if cfg, cfgErr := NewSettingsAdminService(s.db).GetForWorkspace(workspace.ID); cfgErr == nil {
			view.RoomName, view.RoomLogo = cfg.RoomName, cfg.RoomLogo
			view.OperatorTitle = defaultString(strings.TrimSpace(cfg.ChatNickname), "房间运营")
			view.OperatorAvatar = defaultString(strings.TrimSpace(cfg.ChatAvatar), cfg.RoomLogo)
		}
	}
	prefix, value, found := strings.Cut(view.Scope, ":")
	if !found {
		if view.RoomType == "service" && view.Scope == "lobby" {
			view.Title = "大厅客服"
			view.Subtitle = "大厅成员与客服"
		}
		return
	}
	if prefix != "user" && prefix != "agent" && prefix != "tenant" {
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
	if prefix == "agent" || prefix == "tenant" {
		view.GroupChatEnabled = account.GroupChatEnabled
	}
	if view.RoomType == "service" {
		if prefix == "agent" || prefix == "tenant" {
			view.Title = defaultString(account.Nickname, account.Username) + "的房间客服"
			view.Subtitle = "房间号 " + defaultString(view.RoomCode, fmt.Sprintf("房间 #%d", account.UserID))
		} else {
			view.Title = defaultString(account.Nickname, account.Username)
			view.Subtitle = "大厅 · 专属客服"
			if view.RoomCode != "" {
				view.Subtitle = "房间号 " + view.RoomCode + " · 专属客服"
			}
		}
		return
	}
	if prefix == "agent" || prefix == "tenant" {
		view.Title = "聊天室"
		view.Subtitle = "房间号 " + defaultString(view.RoomCode, fmt.Sprintf("房间 #%d", account.UserID))
	}
}

func adminChatMessage(row chat.Message) AdminChatMessage {
	return AdminChatMessage{ID: row.ID, UserID: row.UserID, Username: row.Username, Nickname: row.Nickname, RoomType: row.RoomType, Scope: row.Scope, RoomScope: row.RoomScope, GameID: row.GameID, Content: row.Content, MessageType: defaultString(row.MessageType, "text"), ReferenceID: row.ReferenceID, RedPacketCount: row.RedPacketCount, RedPacketTotal: centsToAmount(row.RedPacketTotalCents), RedPacketMinTurnover: centsToAmount(row.RedPacketMinTurnoverCents), RedPacketCover: row.RedPacketCover, IsStaff: row.UserID == 0 || row.Username == "support", CreatedAt: row.CreatedAt}
}

func applyAdminChatIdentity(view *AdminChatMessage, identity chatMemberIdentity) {
	if view == nil {
		return
	}
	view.PublicID = identity.PublicID
	view.Avatar = identity.Avatar
	view.Title = identity.Title
	view.Badge = identity.Badge
}

func applyAdminRedPacketState(view *AdminChatMessage, packet redPacketViewState) {
	view.RedPacketStatus = packet.Status
	view.RedPacketFunding = packet.FundingStatus
	view.RedPacketClaimed = packet.ClaimedCount
	view.RedPacketRemaining = centsToAmount(packet.RemainingCents)
	view.RedPacketRefunded = centsToAmount(packet.RefundedCents)
	view.RedPacketExpiresAt = packet.ExpiresAt
	view.RedPacketClosedAt = packet.ClosedAt
	view.RedPacketCloseReason = packet.CloseReason
}

func (s *ChatAdminService) normalizeAdminContext(scope, roomType, roomScope, gameID string) (string, string, error) {
	roomScope = strings.TrimSpace(roomScope)
	gameID = strings.TrimSpace(gameID)
	if roomType == "group" {
		roomScope = defaultString(roomScope, scope)
		if roomScope != scope || (scope != "lobby" && !strings.HasPrefix(scope, "agent:") && !strings.HasPrefix(scope, "tenant:")) {
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
	// The room is part of the immutable conversation key. Never derive it from
	// the member's current workspace: members can move rooms while the original
	// support team must retain its historical conversation.
	if roomScope != "lobby" && !strings.HasPrefix(roomScope, "agent:") && !strings.HasPrefix(roomScope, "tenant:") {
		return "", "", apperrors.NewBusinessError("INVALID_REQUEST", "客服会话缺少所属房间")
	}
	return roomScope, gameID, nil
}

func validAdminScope(scope, roomType string) error {
	scope, roomType = strings.TrimSpace(scope), strings.TrimSpace(roomType)
	if roomType != "group" && roomType != "service" {
		return apperrors.NewBusinessError("INVALID_REQUEST", "聊天室类型不正确")
	}
	if scope == "" || len(scope) > 64 {
		return apperrors.NewBusinessError("INVALID_REQUEST", "会话标识不正确")
	}
	if roomType == "service" && scope != "lobby" && !strings.HasPrefix(scope, "agent:") && !strings.HasPrefix(scope, "tenant:") && !strings.HasPrefix(scope, "user:") {
		return apperrors.NewBusinessError("INVALID_REQUEST", "客服会话标识不正确")
	}
	if roomType == "group" && scope != "lobby" && !strings.HasPrefix(scope, "agent:") && !strings.HasPrefix(scope, "tenant:") {
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
	} else if strings.HasPrefix(scope, "tenant:") {
		id, err := strconv.ParseUint(strings.TrimPrefix(scope, "tenant:"), 10, 64)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("invalid chat scope")
		}
		q = q.Where("user_id = ? OR (parent_tenant_id = ? AND parent_agent_id IS NULL)", id, id)
	} else if scope == "lobby" {
		q = q.Where("parent_agent_id IS NULL AND role <> ?", "agent")
	} else {
		return nil, fmt.Errorf("invalid chat scope")
	}
	var ids []uint64
	return ids, q.Pluck("user_id", &ids).Error
}

func chatScopeRecipientsForWorkspace(db *gorm.DB, workspaceID uint64, scope string) ([]uint64, error) {
	q, err := chatScopeRecipientsForWorkspaceQuery(db, workspaceID, scope)
	if err != nil {
		return nil, err
	}
	var ids []uint64
	return ids, q.Pluck("user_id", &ids).Error
}

func chatScopeRecipientsForWorkspaceQuery(db *gorm.DB, workspaceID uint64, scope string) (*gorm.DB, error) {
	if workspaceID == 0 {
		return nil, fmt.Errorf("invalid chat workspace")
	}
	q := db.Model(&user.User{}).Where("status = ? AND workspace_id = ?", 1, workspaceID)
	if strings.HasPrefix(scope, "user:") {
		id, err := strconv.ParseUint(strings.TrimPrefix(scope, "user:"), 10, 64)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("invalid chat scope")
		}
		q = q.Where("user_id = ?", id)
	} else if strings.HasPrefix(scope, "agent:") {
		id, err := strconv.ParseUint(strings.TrimPrefix(scope, "agent:"), 10, 64)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("invalid chat scope")
		}
		q = q.Where("user_id = ? OR parent_agent_id = ?", id, id)
	} else if strings.HasPrefix(scope, "tenant:") {
		id, err := strconv.ParseUint(strings.TrimPrefix(scope, "tenant:"), 10, 64)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("invalid chat scope")
		}
		q = q.Where("user_id = ? OR (parent_tenant_id = ? AND parent_agent_id IS NULL)", id, id)
	} else if scope == "lobby" {
		q = q.Where("parent_agent_id IS NULL AND role <> ?", "agent")
	} else {
		return nil, fmt.Errorf("invalid chat scope")
	}
	return q, nil
}

func redPacketMessageByPacketQuery(db *gorm.DB, packetID uint64) *gorm.DB {
	return db.Table("member_chat_messages AS message").
		Select("message.*").
		Joins(`JOIN chat_red_packets AS packet
			ON packet.message_id = message.id
			AND packet.workspace_id = message.workspace_id
			AND packet.scope = message.scope
			AND packet.room_scope = message.room_scope
			AND packet.game_id = message.game_id`).
		Where("packet.id = ? AND message.room_type = ? AND message.message_type = ?", packetID, "group", "redpacket")
}

func notifyRedPacketMessageUpdatedByPacketID(db *gorm.DB, packetID uint64) {
	var row chat.Message
	if err := redPacketMessageByPacketQuery(db, packetID).Take(&row).Error; err != nil {
		return
	}
	notifyRedPacketMessageUpdated(db, row)
}

func notifyRedPacketMessageUpdated(db *gorm.DB, row chat.Message) {
	if row.ID == 0 || row.WorkspaceID == 0 || row.RoomType != "group" || row.MessageType != "redpacket" {
		return
	}
	recipients, err := chatScopeRecipientsForWorkspace(db, row.WorkspaceID, row.Scope)
	if err != nil {
		return
	}
	notifyChatEvent(db, recipients, row, "updated")
}

// notifyChatEvent adds only authorized operators to the ordinary room
// recipients. The frame contains message ids and scope metadata, never the
// message body; each console still has to pass its normal API authorization
// before it can load the conversation.
func notifyChatEvent(db *gorm.DB, recipients []uint64, message chat.Message, operation string) {
	type adminRecipient struct {
		UserID      uint64
		WorkspaceID uint64
	}
	var admins []adminRecipient
	_ = db.Model(&user.User{}).
		Select("user_id, workspace_id").
		Where("role = ? AND status = ?", "admin", 1).
		Find(&admins).Error
	adminIDs := make(map[uint64]struct{}, len(admins))
	for _, admin := range admins {
		adminIDs[admin.UserID] = struct{}{}
	}

	seen := make(map[uint64]struct{}, len(recipients)+4)
	result := make([]uint64, 0, len(recipients)+4)
	appendID := func(id uint64) {
		if id == 0 {
			return
		}
		// Admin sockets are bound to the admin's authenticated workspace, which
		// may differ from the room workspace. Publish them only in the grouped
		// delivery below so a lobby recipient cannot receive the same event twice.
		if _, isAdmin := adminIDs[id]; isAdmin {
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
	if strings.HasPrefix(message.RoomScope, "agent:") {
		if id, err := strconv.ParseUint(strings.TrimPrefix(message.RoomScope, "agent:"), 10, 64); err == nil {
			appendID(id)
		}
	} else if strings.HasPrefix(message.RoomScope, "tenant:") {
		if id, err := strconv.ParseUint(strings.TrimPrefix(message.RoomScope, "tenant:"), 10, 64); err == nil {
			appendID(id)
		}
	}
	sourceWorkspaceID := message.WorkspaceID
	if sourceWorkspaceID == 0 {
		if workspace, err := WorkspaceByScope(db, message.RoomScope); err == nil {
			sourceWorkspaceID = workspace.ID
		}
	}
	senderKind := "member"
	if message.UserID == 0 || message.Username == "support" || message.Username == "draw_assistant" {
		senderKind = "staff"
	}
	messageType := defaultString(message.MessageType, "text")
	if len(result) > 0 {
		ws.NotifyChat(result, sourceWorkspaceID, sourceWorkspaceID, message.RoomType, message.RoomScope, message.GameID, message.Scope, message.ID, operation, senderKind, messageType)
	}

	// Platform admins have a platform-bound socket, so a room-bound delivery
	// frame is correctly filtered by the hub. Send the same source metadata in
	// a frame grouped by each admin's authenticated delivery workspace.
	if len(admins) > 0 {
		byWorkspace := make(map[uint64][]uint64)
		for _, admin := range admins {
			byWorkspace[admin.WorkspaceID] = append(byWorkspace[admin.WorkspaceID], admin.UserID)
		}
		for deliveryWorkspaceID, adminIDs := range byWorkspace {
			ws.NotifyChat(adminIDs, deliveryWorkspaceID, sourceWorkspaceID, message.RoomType, message.RoomScope, message.GameID, message.Scope, message.ID, operation, senderKind, messageType)
		}
	}
}
