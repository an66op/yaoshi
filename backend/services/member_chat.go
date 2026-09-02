package services

import (
	"backend/accesscontrol"
	"backend/data/models/activity"
	"backend/data/models/bet"
	"backend/data/models/chat"
	"backend/data/models/lottery"
	membernotify "backend/data/models/notify"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"backend/ws"
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	Username             string     `json:"-"`
	Nickname             string     `json:"nickname"`
	Avatar               string     `json:"avatar,omitempty"`
	Title                string     `json:"title,omitempty"`
	Badge                string     `json:"badge,omitempty"`
	RoomType             string     `json:"room_type"`
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
	Claimed              bool       `json:"claimed,omitempty"`
	RedPacketReward      float64    `json:"red_packet_reward,omitempty"`
	Mine                 bool       `json:"mine"`
	CreatedAt            time.Time  `json:"created_at"`
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

const serviceWelcomeContent = "您好，我是客服小七，很高兴为您服务。请问有什么可以帮您？"

func NewMemberChatService(db *gorm.DB) *MemberChatService {
	return &MemberChatService{db: db, settings: NewSettingsAdminService(db)}
}

func (s *MemberChatService) List(userID uint64, roomType, gameID string, limit int, beforeID, afterID uint64, since ...time.Time) (*ChatMessageList, error) {
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
	scope, roomScope, gameID, err := s.chatContext(account, roomType, gameID)
	if err != nil {
		return nil, err
	}
	if roomType == "service" {
		if err := s.ensureServiceWelcome(account, scope, roomScope, gameID); err != nil {
			return nil, err
		}
	}
	query := scopedChatMessageQuery(s.db, account.WorkspaceID, roomType, scope, roomScope, gameID)
	fromVisit := len(since) > 0 && !since[0].IsZero()
	if fromVisit {
		query = query.Where("created_at >= ?", since[0])
	}
	// Game rooms should contain genuine member conversation and bet activity.
	// Older releases also wrote synthetic filler text through activity accounts;
	// hide those rows without deleting real room history or lobby messages.
	if roomType == "group" && gameID != "lobby" {
		query = excludeRobotProfileRows(query, "member_chat_messages.workspace_id", "member_chat_messages.user_id")
	}
	if afterID > 0 {
		query = query.Where("id > ?", afterID)
	} else if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}
	var rows []chat.Message
	order := "created_at desc, id desc"
	if afterID > 0 || (fromVisit && beforeID == 0) {
		// Incremental cursors are IDs. Ordering by the same key avoids skipping
		// messages whose timestamps tie or arrive slightly out of order.
		order = "id asc"
	}
	if err := query.Order(order).Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("CHAT_READ_FAILED", "读取聊天消息失败", err)
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	items := make([]ChatMessageView, 0, len(rows))
	cfg, err := s.settings.GetForWorkspace(account.WorkspaceID)
	if err != nil {
		return nil, apperrors.NewSystemError("CHAT_READ_FAILED", "读取聊天消息失败", err)
	}
	identities, err := loadChatMemberIdentities(s.db, account.WorkspaceID, rows, cfg)
	if err != nil {
		return nil, apperrors.NewSystemError("CHAT_READ_FAILED", "读取聊天成员资料失败", err)
	}
	claimedPackets, err := s.claimedRedPackets(userID, rows)
	if err != nil {
		return nil, apperrors.NewSystemError("CHAT_READ_FAILED", "读取红包状态失败", err)
	}
	packetStates, err := loadRedPacketViewStates(s.db, rows)
	if err != nil {
		return nil, apperrors.NewSystemError("CHAT_READ_FAILED", "读取红包资金状态失败", err)
	}
	if afterID > 0 || (fromVisit && beforeID == 0) {
		for _, row := range rows {
			items = append(items, chatMessageView(row, userID, identities[row.UserID], claimedPackets[row.ID], packetStates[row.ID]))
		}
	} else {
		for i := len(rows) - 1; i >= 0; i-- {
			row := rows[i]
			items = append(items, chatMessageView(row, userID, identities[row.UserID], claimedPackets[row.ID], packetStates[row.ID]))
		}
	}
	nextBeforeID := uint64(0)
	if len(items) > 0 {
		nextBeforeID = items[0].ID
	}
	return &ChatMessageList{Items: items, HasMore: hasMore && (afterID == 0 || fromVisit), NextBeforeID: nextBeforeID}, nil
}

// LatestClaimableRedPacket reads the newest still-open envelope from the
// member's current room lobby. It deliberately does not depend on the chat
// history page size, so ordinary messages cannot push an unclaimed envelope
// out of the game-room shortcut.
func (s *MemberChatService) LatestClaimableRedPacket(userID uint64) (*ChatMessageView, error) {
	account, err := s.account(userID)
	if err != nil {
		return nil, err
	}
	scope, roomScope, _, err := s.chatContext(account, "group", "lobby")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	var row chat.Message
	if err := claimableLobbyRedPacketQuery(s.db, account.WorkspaceID, userID, scope, roomScope, now).
		Order("message.created_at DESC, message.id DESC").Take(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, apperrors.NewSystemError("REDPACKET_READ_FAILED", "读取房间红包失败", err)
	}

	claimedPackets, err := s.claimedRedPackets(userID, []chat.Message{row})
	if err != nil {
		return nil, apperrors.NewSystemError("REDPACKET_READ_FAILED", "读取红包领取状态失败", err)
	}
	if claimedPackets[row.ID] > 0 {
		return nil, nil
	}
	packetStates, err := loadRedPacketViewStates(s.db, []chat.Message{row})
	if err != nil {
		return nil, apperrors.NewSystemError("REDPACKET_READ_FAILED", "读取红包资金状态失败", err)
	}
	packet, exists := packetStates[row.ID]
	if !exists || !redPacketStateClaimable(row, packet, now) {
		return nil, nil
	}
	cfg, err := s.settings.GetForWorkspace(account.WorkspaceID)
	if err != nil {
		return nil, apperrors.NewSystemError("REDPACKET_READ_FAILED", "读取房间红包失败", err)
	}
	identities, err := loadChatMemberIdentities(s.db, account.WorkspaceID, []chat.Message{row}, cfg)
	if err != nil {
		return nil, apperrors.NewSystemError("REDPACKET_READ_FAILED", "读取红包发送者资料失败", err)
	}
	view := chatMessageView(row, userID, identities[row.UserID], 0, packet)
	return &view, nil
}

func (s *MemberChatService) Post(userID uint64, roomType, gameID, content string) (*ChatMessageView, error) {
	return s.post(userID, roomType, gameID, content, false, "")
}

// PostCommand persists a financial/game-room command without applying the
// ordinary conversation balance and mute gates. Commands are exposed through
// a separately rate-limited controller route and still keep every room/game
// isolation and room-availability check performed by post(). This keeps text
// betting behavior aligned with the structured betting panel: muting normal
// conversation must never silently change whether the same account can place,
// query, repeat or cancel a bet.
func (s *MemberChatService) PostCommand(userID uint64, roomType, gameID, content, requestID string) (*ChatMessageView, error) {
	return s.post(userID, roomType, gameID, content, true, requestID)
}

func (s *MemberChatService) post(userID uint64, roomType, gameID, content string, command bool, requestID string) (*ChatMessageView, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "消息不能为空")
	}
	if len([]rune(content)) > 200 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "消息过长")
	}
	requestID = strings.TrimSpace(requestID)
	if command && (len(requestID) < 8 || len(requestID) > 96) {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "请求标识不正确")
	}
	roomType = defaultString(strings.TrimSpace(roomType), "group")
	if roomType != "group" && roomType != "service" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "聊天室类型不正确")
	}
	account, err := s.account(userID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.settings.GetForWorkspace(account.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if roomType == "group" && !command && account.BalanceCents < int64(cfg.MinChatScore*100) {
		return nil, apperrors.NewBusinessError("CHAT_FORBIDDEN", fmt.Sprintf("余额需达到 %.2f 元才可发言", cfg.MinChatScore))
	}
	scope, roomScope, gameID, err := s.chatContext(account, roomType, gameID)
	if err != nil {
		return nil, err
	}
	if roomType == "service" {
		if err := s.ensureServiceWelcome(account, scope, roomScope, gameID); err != nil {
			return nil, err
		}
	}
	if roomType == "group" && gameID == "lobby" && !s.roomGroupChatEnabled(account) {
		return nil, apperrors.NewBusinessError("CHAT_MUTED", "群聊暂未开放，请联系在线客服")
	}
	if roomType == "group" && !command && account.MutedUntil != nil && account.MutedUntil.After(time.Now()) {
		reason := strings.TrimSpace(account.MuteReason)
		if reason == "" {
			reason = "请联系在线客服"
		}
		return nil, apperrors.NewBusinessError("CHAT_MUTED", fmt.Sprintf("您已被禁言至 %s：%s", account.MutedUntil.Local().Format("2006-01-02 15:04"), reason))
	}
	nickname := defaultString(account.Nickname, account.Username)
	row := chat.Message{
		WorkspaceID: account.WorkspaceID, UserID: userID, Username: account.Username, Nickname: nickname,
		RoomType: roomType, Scope: scope, RoomScope: roomScope, GameID: gameID, Content: content, MessageType: "text",
		RequestID: requestID,
	}
	if command {
		var previous chat.Message
		lookup := s.db.Unscoped().Where("user_id = ? AND request_id = ?", userID, requestID).First(&previous)
		if lookup.Error == nil {
			if previous.WorkspaceID != row.WorkspaceID || previous.RoomType != row.RoomType || previous.RoomScope != row.RoomScope || previous.GameID != row.GameID || previous.Content != row.Content {
				return nil, apperrors.NewBusinessError("IDEMPOTENCY_CONFLICT", "该请求标识已用于其他指令")
			}
			return memberPostedMessageView(previous, account), nil
		}
		if lookup.Error != gorm.ErrRecordNotFound {
			return nil, apperrors.NewSystemError("CHAT_READ_FAILED", "读取指令状态失败", lookup.Error)
		}
	}
	if err := s.db.Create(&row).Error; err != nil {
		if command {
			var previous chat.Message
			if lookupErr := s.db.Unscoped().Where("user_id = ? AND request_id = ?", userID, requestID).First(&previous).Error; lookupErr == nil {
				if previous.WorkspaceID != row.WorkspaceID || previous.RoomType != row.RoomType || previous.RoomScope != row.RoomScope || previous.GameID != row.GameID || previous.Content != row.Content {
					return nil, apperrors.NewBusinessError("IDEMPOTENCY_CONFLICT", "该请求标识已用于其他指令")
				}
				return memberPostedMessageView(previous, account), nil
			}
		}
		return nil, apperrors.NewSystemError("CHAT_SAVE_FAILED", "发送消息失败", err)
	}
	view := memberPostedMessageView(row, account)
	recipients, err := s.scopeRecipients(row.Scope)
	if err == nil {
		notifyChatEvent(s.db, recipients, row, "created")
	}
	return view, nil
}

func memberPostedMessageView(row chat.Message, account user.User) *ChatMessageView {
	return &ChatMessageView{
		ID: row.ID, UserID: row.UserID, PublicID: account.PublicID, Username: row.Username, Nickname: row.Nickname,
		Avatar: account.Avatar, Title: account.PublicTitle, Badge: account.PublicBadge,
		RoomType: row.RoomType, RoomScope: row.RoomScope, GameID: row.GameID,
		Content: row.Content, MessageType: row.MessageType, ReferenceID: row.ReferenceID, Mine: true, CreatedAt: row.CreatedAt,
	}
}

// ensureServiceWelcome materializes the customer-service greeting as the
// first durable message in a new private conversation. It intentionally does
// not append a greeting to an existing conversation: older service timelines
// already have a real first message and must keep their original order. Soft-
// deleted history also counts as existing so a later lifecycle restore cannot
// collide with a newly generated greeting. If an older lifecycle build soft-
// deleted the greeting itself, restore that exact row instead of creating a
// second greeting or leaving the conversation permanently without one.
//
// The partial unique index installed by the matching SQL migration makes this
// safe when a list request and the member's first send arrive concurrently.
func (s *MemberChatService) ensureServiceWelcome(account user.User, scope, roomScope, gameID string) error {
	row := newServiceWelcomeMessage(account, scope, roomScope, gameID)
	result := s.db.Exec(`
		WITH existing_welcome AS (
			SELECT id
			FROM member_chat_messages
			WHERE workspace_id = ? AND room_type = 'service' AND scope = ? AND room_scope = ? AND game_id = ?
			  AND message_type = 'welcome'
			ORDER BY id ASC
			LIMIT 1
		), restored_welcome AS (
			UPDATE member_chat_messages
			SET deleted_at = NULL, deleted_by = '', cleanup_request_id = ''
			WHERE id = (SELECT id FROM existing_welcome) AND deleted_at IS NOT NULL
			RETURNING id
		)
		INSERT INTO member_chat_messages
			(workspace_id, user_id, username, nickname, room_type, scope, room_scope, game_id, content, message_type, reference_id,
			 red_packet_count, red_packet_total_cents, red_packet_min_turnover_cents, red_packet_cover, created_at, deleted_by, cleanup_request_id)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 0, 0, '', NOW(), '', ''
		WHERE NOT EXISTS (
			SELECT 1
			FROM member_chat_messages
			WHERE workspace_id = ? AND room_type = 'service' AND scope = ? AND room_scope = ? AND game_id = ?
		)
		ON CONFLICT DO NOTHING
	`, row.WorkspaceID, row.Scope, row.RoomScope, row.GameID,
		row.WorkspaceID, row.UserID, row.Username, row.Nickname, row.RoomType, row.Scope, row.RoomScope, row.GameID, row.Content, row.MessageType,
		row.WorkspaceID, row.Scope, row.RoomScope, row.GameID)
	if result.Error != nil {
		return apperrors.NewSystemError("CHAT_SAVE_FAILED", "创建客服会话失败", result.Error)
	}
	return nil
}

func newServiceWelcomeMessage(account user.User, scope, roomScope, gameID string) chat.Message {
	return chat.Message{
		WorkspaceID: account.WorkspaceID,
		UserID:      0,
		Username:    "support",
		Nickname:    "客服小七",
		RoomType:    "service",
		Scope:       scope,
		RoomScope:   roomScope,
		GameID:      gameID,
		Content:     serviceWelcomeContent,
		MessageType: "welcome",
	}
}

// PostAssistant persists a room-scoped assistant reply. It is used for
// application acknowledgements and review results so they survive refreshes
// and cannot leak into another room or lottery conversation.
func (s *MemberChatService) PostAssistant(roomScope, gameID, content string, referenceID uint64) (*ChatMessageView, error) {
	roomScope = strings.TrimSpace(roomScope)
	gameID = strings.TrimSpace(gameID)
	content = strings.TrimSpace(content)
	if content == "" || gameID == "" || (roomScope != "lobby" && !strings.HasPrefix(roomScope, "agent:") && !strings.HasPrefix(roomScope, "tenant:")) {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "群聊回复范围不正确")
	}
	workspace, err := WorkspaceByScope(s.db, roomScope)
	if err != nil || workspace.ID == 0 {
		return nil, apperrors.NewBusinessError("ROOM_NOT_FOUND", "消息所属房间不存在")
	}
	row := chat.Message{
		WorkspaceID: workspace.ID, UserID: 0, Username: "draw_assistant", Nickname: "开奖助手",
		RoomType: "group", Scope: roomScope, RoomScope: roomScope, GameID: gameID,
		Content: content, MessageType: "application", ReferenceID: referenceID,
	}
	if referenceID > 0 {
		var previous chat.Message
		lookup := s.db.Where("workspace_id = ? AND room_scope = ? AND game_id = ? AND user_id = 0 AND reference_id = ? AND message_type = ?", workspace.ID, roomScope, gameID, referenceID, "application").First(&previous)
		if lookup.Error == nil {
			return s.assistantMessageView(workspace.ID, previous), nil
		}
		if lookup.Error != gorm.ErrRecordNotFound {
			return nil, apperrors.NewSystemError("CHAT_READ_FAILED", "读取申请回复失败", lookup.Error)
		}
	}
	created := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if created.Error != nil {
		return nil, apperrors.NewSystemError("CHAT_SAVE_FAILED", "保存申请回复失败", created.Error)
	}
	if created.RowsAffected == 0 && referenceID > 0 {
		var previous chat.Message
		if err := s.db.Where("workspace_id = ? AND room_scope = ? AND game_id = ? AND user_id = 0 AND reference_id = ? AND message_type = ?", workspace.ID, roomScope, gameID, referenceID, "application").First(&previous).Error; err != nil {
			return nil, apperrors.NewSystemError("CHAT_READ_FAILED", "读取申请回复失败", err)
		}
		return s.assistantMessageView(workspace.ID, previous), nil
	}
	view := s.assistantMessageView(workspace.ID, row)
	if recipients, err := s.scopeRecipients(row.Scope); err == nil {
		notifyChatEvent(s.db, recipients, row, "created")
	}
	return view, nil
}

func (s *MemberChatService) assistantMessageView(workspaceID uint64, row chat.Message) *ChatMessageView {
	view := &ChatMessageView{
		ID: row.ID, UserID: 0, Nickname: row.Nickname, RoomType: row.RoomType,
		RoomScope: row.RoomScope, GameID: row.GameID, Content: row.Content,
		MessageType: row.MessageType, ReferenceID: row.ReferenceID, Mine: false, CreatedAt: row.CreatedAt,
	}
	if cfg, cfgErr := s.settings.GetForWorkspace(workspaceID); cfgErr == nil {
		if identities, identityErr := loadChatMemberIdentities(s.db, workspaceID, []chat.Message{row}, cfg); identityErr == nil {
			identity := identities[0]
			view.Avatar, view.Title, view.Badge = identity.Avatar, identity.Title, identity.Badge
		}
	}
	return view
}

func chatMessageView(row chat.Message, userID uint64, identity chatMemberIdentity, claimedRewardCents int64, packet redPacketViewState) ChatMessageView {
	return ChatMessageView{
		ID: row.ID, UserID: row.UserID, PublicID: identity.PublicID, Username: row.Username, Nickname: row.Nickname,
		Avatar: identity.Avatar, Title: identity.Title, Badge: identity.Badge,
		RoomType: row.RoomType, RoomScope: row.RoomScope, GameID: row.GameID,
		Content: row.Content, MessageType: defaultString(row.MessageType, "text"), ReferenceID: row.ReferenceID,
		RedPacketCount: row.RedPacketCount, RedPacketTotal: centsToAmount(row.RedPacketTotalCents),
		RedPacketMinTurnover: centsToAmount(row.RedPacketMinTurnoverCents), RedPacketCover: row.RedPacketCover,
		RedPacketStatus: packet.Status, RedPacketFunding: packet.FundingStatus, RedPacketClaimed: packet.ClaimedCount,
		RedPacketRemaining: centsToAmount(packet.RemainingCents), RedPacketRefunded: centsToAmount(packet.RefundedCents),
		RedPacketExpiresAt: packet.ExpiresAt, RedPacketClosedAt: packet.ClosedAt, RedPacketCloseReason: packet.CloseReason,
		Claimed: claimedRewardCents > 0, RedPacketReward: centsToAmount(claimedRewardCents),
		Mine: row.UserID == userID, CreatedAt: row.CreatedAt,
	}
}

func (s *MemberChatService) claimedRedPackets(userID uint64, rows []chat.Message) (map[uint64]int64, error) {
	messageIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		if row.MessageType == "redpacket" {
			messageIDs = append(messageIDs, row.ID)
		}
	}
	claimed := make(map[uint64]int64, len(messageIDs))
	if len(messageIDs) == 0 {
		return claimed, nil
	}

	// New room envelopes are independent persisted records. Resolve claims by
	// packet id first, then retain the activity lookup below only for envelopes
	// sent by older releases.
	var packets []chat.RedPacket
	if err := s.db.Select("id", "message_id").Where("message_id IN ?", messageIDs).Find(&packets).Error; err != nil {
		return nil, err
	}
	packetIDs := make([]uint64, 0, len(packets))
	messageByPacket := make(map[uint64]uint64, len(packets))
	newPacketMessages := make(map[uint64]struct{}, len(packets))
	for _, packet := range packets {
		packetIDs = append(packetIDs, packet.ID)
		messageByPacket[packet.ID] = packet.MessageID
		newPacketMessages[packet.MessageID] = struct{}{}
	}
	if len(packetIDs) > 0 {
		var claims []chat.RedPacketClaim
		if err := s.db.Select("packet_id", "amount_cents").Where("user_id = ? AND packet_id IN ?", userID, packetIDs).Find(&claims).Error; err != nil {
			return nil, err
		}
		for _, claim := range claims {
			claimed[messageByPacket[claim.PacketID]] = claim.AmountCents
		}
	}

	references := make([]string, 0, len(messageIDs))
	messageByReference := make(map[string]uint64, len(messageIDs))
	for _, messageID := range messageIDs {
		if _, exists := newPacketMessages[messageID]; exists {
			continue
		}
		reference := "chat_message:" + strconv.FormatUint(messageID, 10)
		references = append(references, reference)
		messageByReference[reference] = messageID
	}
	if len(references) == 0 {
		return claimed, nil
	}
	var participations []activity.Participation
	if err := s.db.Select("reference", "reward_cents").
		Where("user_id = ? AND action = ? AND reference IN ?", userID, "redpacket", references).
		Find(&participations).Error; err != nil {
		return nil, err
	}
	for _, participation := range participations {
		claimed[messageByReference[participation.Reference]] = participation.RewardCents
	}
	return claimed, nil
}

// ClaimRedPacket verifies the message belongs to the member's current room
// before paying it. Activity ids sent by the browser are never trusted.
func (s *MemberChatService) ClaimRedPacket(userID, messageID uint64) (*ActivityActionResult, error) {
	account, err := s.account(userID)
	if err != nil {
		return nil, err
	}
	var row chat.Message
	if err := s.db.Where("id = ? AND workspace_id = ? AND room_type = ? AND message_type = ? AND deleted_at IS NULL", messageID, account.WorkspaceID, "group", "redpacket").First(&row).Error; err != nil {
		return nil, apperrors.NewBusinessError("NOT_FOUND", "红包不存在或已失效")
	}
	scope, roomScope, gameID, err := s.chatContext(account, "group", row.GameID)
	if err != nil {
		return nil, err
	}
	if row.Scope != scope || row.RoomScope != roomScope || row.GameID != gameID {
		return nil, apperrors.NewBusinessError("FORBIDDEN", "该红包不属于当前房间")
	}
	var packet chat.RedPacket
	if readErr := s.db.Where("message_id = ?", row.ID).First(&packet).Error; readErr == nil {
		return s.claimPersistedRedPacket(userID, packet.ID)
	} else if readErr != gorm.ErrRecordNotFound {
		return nil, apperrors.NewSystemError("REDPACKET_READ_FAILED", "读取红包失败", readErr)
	}
	if row.ReferenceID == 0 {
		return nil, apperrors.NewBusinessError("NOT_FOUND", "红包不存在或已失效")
	}
	return NewMemberPortalService(s.db).ClaimChatRedPacket(userID, row.ReferenceID, row.ID)
}

func (s *MemberChatService) claimPersistedRedPacket(userID, packetID uint64) (*ActivityActionResult, error) {
	var result ActivityActionResult
	var notificationID uint64
	var workspaceID uint64
	var changedPacketID uint64
	var expired bool
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var packet chat.RedPacket
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&packet, packetID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("NOT_FOUND", "红包不存在或已失效")
			}
			return err
		}
		var account user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, userID).Error; err != nil {
			return err
		}
		if packet.WorkspaceID == 0 || account.WorkspaceID != packet.WorkspaceID {
			return apperrors.NewBusinessError("FORBIDDEN", "该红包不属于当前房间")
		}
		if packet.ExpiresAt != nil && !time.Now().UTC().Before(packet.ExpiresAt.UTC()) {
			changed, err := closeRedPacketTx(tx, packet.ID, "系统", "红包已过期", "expired")
			if err != nil {
				return err
			}
			if changed {
				changedPacketID = packet.ID
			}
			expired = true
			return nil
		}
		workspaceID = packet.WorkspaceID
		var existing int64
		if err := tx.Model(&chat.RedPacketClaim{}).Where("packet_id = ? AND user_id = ?", packet.ID, userID).Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return apperrors.NewBusinessError("REDPACKET_CLAIMED", "该红包已领取")
		}
		slots := packet.PacketCount - packet.ClaimedCount
		if packet.Status != "active" || packet.RemainingCents <= 0 || slots <= 0 {
			return apperrors.NewBusinessError("RED_PACKET_EMPTY", "红包已经领完了")
		}
		if !redPacketFundingMayRelease(packet) {
			return apperrors.NewBusinessError("RED_PACKET_FUNDING_INVALID", "红包资金状态异常，暂不可领取")
		}
		if packet.MinDailyTurnoverCents > 0 {
			var turnoverCents int64
			now := time.Now()
			dayStart := startOfDayCST(now)
			dayEnd := dayStart.Add(24 * time.Hour)
			if err := tx.Model(&bet.Bet{}).
				Where("workspace_id = ? AND user_id = ? AND created_at >= ? AND created_at < ? AND status IN ?", packet.WorkspaceID, userID, dayStart, dayEnd, []string{"won", "lost"}).
				Select("COALESCE(SUM(COALESCE(valid_turnover_cents,amount_cents)),0)").Scan(&turnoverCents).Error; err != nil {
				return err
			}
			if turnoverCents < packet.MinDailyTurnoverCents {
				shortfall := packet.MinDailyTurnoverCents - turnoverCents
				return apperrors.NewBusinessError("REDPACKET_TURNOVER_REQUIRED", fmt.Sprintf(
					"今日有效流水 %.2f，达到 %.2f 后可领取，还差 %.2f",
					centsToAmount(turnoverCents), centsToAmount(packet.MinDailyTurnoverCents), centsToAmount(shortfall),
				))
			}
		}
		rewardCents, err := drawChatRedPacketReward(packet.RemainingCents, slots)
		if err != nil {
			return err
		}
		claim := chat.RedPacketClaim{WorkspaceID: packet.WorkspaceID, PacketID: packet.ID, UserID: userID, AmountCents: rewardCents}
		if err := tx.Create(&claim).Error; err != nil {
			if isDuplicateParticipation(err) {
				return apperrors.NewBusinessError("REDPACKET_CLAIMED", "该红包已领取")
			}
			return err
		}

		before := account.BalanceCents
		after := before + rewardCents
		if err := tx.Model(&account).Update("balance_cents", after).Error; err != nil {
			return err
		}
		remark := "红包奖励 · " + defaultString(strings.TrimSpace(packet.Greeting), "恭喜发财")
		if err := tx.Create(&user.BalanceTransaction{
			WorkspaceID: packet.WorkspaceID, UserID: userID, Reference: "chat_red_packet_claim:" + strconv.FormatUint(claim.ID, 10),
			AmountCents: rewardCents, BeforeCents: before, AfterCents: after,
			Type: "redpacket", Remark: remark, Operator: "系统",
		}).Error; err != nil {
			return err
		}

		remaining := packet.RemainingCents - rewardCents
		claimedCount := packet.ClaimedCount + 1
		status := "active"
		if remaining == 0 || claimedCount >= packet.PacketCount {
			status = "empty"
		}
		packetUpdates := map[string]any{
			"remaining_cents": remaining,
			"claimed_count":   claimedCount,
			"status":          status,
			"funding_status":  "partially_released",
		}
		if status == "empty" {
			packetUpdates["funding_status"] = "released"
			closedAt := time.Now().UTC()
			packetUpdates["closed_at"] = closedAt
			packetUpdates["closed_by"] = "系统"
			packetUpdates["close_reason"] = "红包已领完"
		}
		if err := tx.Model(&packet).Updates(packetUpdates).Error; err != nil {
			return err
		}
		changedPacketID = packet.ID

		content := remark + "，到账 " + formatAmount(rewardCents) + " 元"
		notice := membernotify.MemberNotification{
			WorkspaceID: packet.WorkspaceID, UserID: userID, GameID: packet.GameID, RoomScope: packet.RoomScope,
			Title: "红包领取成功", Content: content, Level: "success", Category: "account",
		}
		if err := tx.Create(&notice).Error; err != nil {
			return err
		}
		notificationID = notice.ID
		result = ActivityActionResult{Reward: centsToAmount(rewardCents), Balance: centsToAmount(after), Message: content}
		return nil
	})
	if err != nil {
		if app, ok := err.(*apperrors.AppError); ok {
			return nil, app
		}
		return nil, apperrors.NewSystemError("REDPACKET_CLAIM_FAILED", "领取红包失败", err)
	}
	if changedPacketID > 0 {
		notifyRedPacketMessageUpdatedByPacketID(s.db, changedPacketID)
	}
	if expired {
		return nil, apperrors.NewBusinessError("RED_PACKET_EXPIRED", "红包已过期")
	}
	ws.NotifyUser(userID, "balance", map[string]any{"workspace_id": workspaceID, "balance": result.Balance})
	ws.NotifyUser(userID, "notification", map[string]any{"workspace_id": workspaceID, "id": notificationID, "category": "account"})
	return &result, nil
}

func drawChatRedPacketReward(remaining int64, slots int) (int64, error) {
	if remaining <= 0 || slots <= 0 || remaining < int64(slots) {
		return 0, apperrors.NewBusinessError("RED_PACKET_EMPTY", "红包已经领完了")
	}
	if slots == 1 {
		return remaining, nil
	}
	// Reserve one cent for every unopened envelope. The random upper bound is
	// twice the current average, capped by the amount that can safely be drawn.
	maxAvailable := remaining - int64(slots-1)
	upper := (remaining / int64(slots)) * 2
	if upper < 1 {
		upper = 1
	}
	if upper > maxAvailable {
		upper = maxAvailable
	}
	n, err := rand.Int(rand.Reader, big.NewInt(upper))
	if err != nil {
		return 0, err
	}
	return n.Int64() + 1, nil
}

type chatMemberIdentity struct {
	PublicID uint64
	Avatar   string
	Title    string
	Badge    string
}

func scopedChatMessageQuery(db *gorm.DB, workspaceID uint64, roomType, scope, roomScope, gameID string) *gorm.DB {
	return db.Model(&chat.Message{}).Where(
		"workspace_id = ? AND room_type = ? AND scope = ? AND room_scope = ? AND game_id = ? AND deleted_at IS NULL",
		workspaceID, roomType, scope, roomScope, gameID,
	)
}

func claimableLobbyRedPacketQuery(db *gorm.DB, workspaceID, userID uint64, scope, roomScope string, now time.Time) *gorm.DB {
	return db.Table("member_chat_messages AS message").
		Select("message.*").
		Joins(`JOIN chat_red_packets AS packet
			ON packet.message_id = message.id
			AND packet.workspace_id = message.workspace_id
			AND packet.scope = message.scope
			AND packet.room_scope = message.room_scope
			AND packet.game_id = message.game_id`).
		Where(`message.workspace_id = ? AND message.room_type = ?
			AND message.scope = ? AND message.room_scope = ? AND message.game_id = ?
			AND message.message_type = ? AND message.deleted_at IS NULL`,
			workspaceID, "group", scope, roomScope, "lobby", "redpacket").
		Where(`packet.status = ? AND packet.remaining_cents > 0
			AND packet.claimed_count < packet.packet_count
			AND (packet.expires_at IS NULL OR packet.expires_at > ?)
			AND packet.funding_user_id > 0 AND packet.refunded_cents = 0
			AND packet.funding_status IN ?`, "active", now, []string{"reserved", "partially_released"}).
		Where(`NOT EXISTS (
			SELECT 1 FROM chat_red_packet_claims AS claim
			WHERE claim.workspace_id = ? AND claim.packet_id = packet.id AND claim.user_id = ?
		)`, workspaceID, userID)
}

func redPacketStateClaimable(row chat.Message, packet redPacketViewState, now time.Time) bool {
	if packet.Status != "active" || packet.RemainingCents <= 0 || packet.ClaimedCount >= row.RedPacketCount || packet.RefundedCents != 0 {
		return false
	}
	if packet.FundingStatus != "reserved" && packet.FundingStatus != "partially_released" {
		return false
	}
	return packet.ExpiresAt == nil || now.Before(packet.ExpiresAt.UTC())
}

func loadChatMemberIdentities(db *gorm.DB, workspaceID uint64, rows []chat.Message, cfg *SystemSettingsView) (map[uint64]chatMemberIdentity, error) {
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
	result := make(map[uint64]chatMemberIdentity, len(ids)+1)
	if cfg != nil {
		result[0] = chatMemberIdentity{
			Avatar: defaultString(strings.TrimSpace(cfg.ChatAvatar), cfg.RoomLogo),
			Title:  defaultString(strings.TrimSpace(cfg.ChatNickname), "房间运营"),
			Badge:  "官方",
		}
	}
	if len(ids) == 0 {
		return result, nil
	}
	var accounts []user.User
	// A membership is the durable proof that this member belonged to the
	// message workspace. Do not require account.workspace_id here: that column
	// points at the member's current room and changes after an approved room
	// switch, while historical messages must retain their public identity in the
	// original room. Inactive memberships are intentionally included.
	if err := historicalChatMemberIdentityQuery(db, workspaceID, ids).Scan(&accounts).Error; err != nil {
		return nil, err
	}
	for _, account := range accounts {
		result[account.UserID] = chatMemberIdentity{
			PublicID: account.PublicID, Avatar: account.Avatar,
			Title: account.PublicTitle, Badge: account.PublicBadge,
		}
	}
	var robotProfiles []workspacemodel.RobotProfile
	if err := db.Select("user_id", "avatar").Where("workspace_id = ? AND user_id IN ?", workspaceID, ids).Find(&robotProfiles).Error; err != nil {
		return nil, err
	}
	for _, profile := range robotProfiles {
		identity := result[profile.UserID]
		if strings.TrimSpace(identity.Avatar) == "" {
			identity.Avatar = profile.Avatar
			result[profile.UserID] = identity
		}
	}
	return result, nil
}

func historicalChatMemberIdentityQuery(db *gorm.DB, workspaceID uint64, userIDs []uint64) *gorm.DB {
	return db.Table(`"user" AS account`).
		Select("account.user_id, account.public_id, account.workspace_id, account.avatar, account.public_title, account.public_badge").
		Joins("JOIN workspace_memberships AS membership ON membership.user_id = account.user_id AND membership.workspace_id = ?", workspaceID).
		Where("account.deleted_at IS NULL AND account.user_id IN ?", userIDs)
}

func (s *MemberChatService) Preview(userID uint64) (*ChatPreview, error) {
	account, err := s.account(userID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.settings.GetForWorkspace(account.WorkspaceID)
	if err != nil {
		return nil, err
	}
	preview := &ChatPreview{
		MinChatScore: cfg.MinChatScore,
		ChatNickname: cfg.ChatNickname,
		Balance:      centsToAmount(account.BalanceCents),
		CanChat:      centsToAmount(account.BalanceCents) >= cfg.MinChatScore && s.roomGroupChatEnabled(account) && (account.MutedUntil == nil || !account.MutedUntil.After(time.Now())),
	}
	var latest chat.Message
	roomScope := chatScope(account, "group")
	query := s.db.Where(
		"workspace_id = ? AND room_type = ? AND scope = ? AND room_scope = ? AND game_id = ? AND deleted_at IS NULL",
		account.WorkspaceID, "group", roomScope, roomScope, "lobby",
	)
	if err := query.Order("created_at desc").First(&latest).Error; err == nil {
		if latest.MessageType == "redpacket" {
			preview.LatestMessage = "收到一个红包 · " + defaultString(strings.TrimSpace(latest.Content), "恭喜发财")
		} else {
			preview.LatestMessage = defaultString(strings.TrimSpace(latest.Nickname), "会员") + "：" + latest.Content
		}
		preview.LatestAt = &latest.CreatedAt
	}
	return preview, nil
}

func (s *MemberChatService) roomGroupChatEnabled(account user.User) bool {
	if account.Role == "agent" {
		return account.GroupChatEnabled
	}
	if account.ParentAgentID == nil || *account.ParentAgentID == 0 {
		if account.ParentTenantID == nil || *account.ParentTenantID == 0 {
			return false
		}
		var tenant user.User
		if err := s.db.Select("group_chat_enabled").First(&tenant, *account.ParentTenantID).Error; err != nil {
			return false
		}
		return tenant.GroupChatEnabled
	}
	var agent user.User
	if err := s.db.Select("group_chat_enabled").First(&agent, *account.ParentAgentID).Error; err != nil {
		return false
	}
	return agent.GroupChatEnabled
}

func (s *MemberChatService) chatContext(account user.User, roomType, gameID string) (scope, roomScope, resolvedGameID string, err error) {
	roomScope = chatScope(account, "group")
	if roomType == "service" {
		return chatScope(account, "service"), roomScope, "service", nil
	}
	roomActive, roomErr := accesscontrol.AccountRoomActive(s.db, account)
	if roomErr != nil {
		return "", "", "", roomErr
	}
	if !roomActive {
		return "", "", "", apperrors.NewBusinessError("ROOM_UNAVAILABLE", "当前房间已停用，请先切换房间")
	}
	resolvedGameID = defaultString(strings.TrimSpace(gameID), "lobby")
	if resolvedGameID == "lobby" {
		return roomScope, roomScope, resolvedGameID, nil
	}
	var game lottery.Game
	if readErr := s.db.Select("id", "enabled").First(&game, "id = ?", resolvedGameID).Error; readErr != nil {
		return "", "", "", apperrors.NewBusinessError("GAME_NOT_FOUND", "彩种不存在")
	}
	if !game.Enabled {
		return "", "", "", apperrors.NewBusinessError("GAME_DISABLED", "该彩种暂未开放")
	}
	roomGameEnabled, roomGameErr := WorkspaceGameEnabled(s.db, account.WorkspaceID, game.ID)
	if roomGameErr != nil {
		return "", "", "", roomGameErr
	}
	if !roomGameEnabled {
		return "", "", "", apperrors.NewBusinessError("LOTTERY_ROOM_CLOSED", "该彩票室已关闭")
	}
	return roomScope, roomScope, game.ID, nil
}

func (s *MemberChatService) account(userID uint64) (user.User, error) {
	var account user.User
	if err := s.db.First(&account, userID).Error; err != nil {
		return user.User{}, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
	}
	return account, nil
}

// Group chat belongs to the current room. Customer service is always a private
// user conversation; chatContext freezes the current workspace and room scope
// on each new message so a later room switch cannot rewrite old history.
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
	if account.ParentTenantID != nil {
		return "tenant:" + strconv.FormatUint(*account.ParentTenantID, 10)
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
	} else if strings.HasPrefix(scope, "tenant:") {
		tenantID, err := strconv.ParseUint(strings.TrimPrefix(scope, "tenant:"), 10, 64)
		if err != nil || tenantID == 0 {
			return nil, fmt.Errorf("invalid chat scope")
		}
		query = query.Where("user_id = ? OR (parent_tenant_id = ? AND parent_agent_id IS NULL)", tenantID, tenantID)
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
