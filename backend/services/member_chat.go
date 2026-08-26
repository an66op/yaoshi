package services

import (
	"backend/accesscontrol"
	"backend/data/models/activity"
	"backend/data/models/bet"
	"backend/data/models/chat"
	"backend/data/models/lottery"
	membernotify "backend/data/models/notify"
	"backend/data/models/user"
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
	Username             string    `json:"-"`
	Nickname             string    `json:"nickname"`
	RoomType             string    `json:"room_type"`
	RoomScope            string    `json:"room_scope"`
	GameID               string    `json:"game_id"`
	Content              string    `json:"content"`
	MessageType          string    `json:"message_type"`
	ReferenceID          uint64    `json:"reference_id,omitempty"`
	RedPacketCount       int       `json:"red_packet_count,omitempty"`
	RedPacketTotal       float64   `json:"red_packet_total,omitempty"`
	RedPacketMinTurnover float64   `json:"red_packet_min_turnover,omitempty"`
	RedPacketCover       string    `json:"red_packet_cover,omitempty"`
	Claimed              bool      `json:"claimed,omitempty"`
	RedPacketReward      float64   `json:"red_packet_reward,omitempty"`
	Mine                 bool      `json:"mine"`
	CreatedAt            time.Time `json:"created_at"`
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

func (s *MemberChatService) List(userID uint64, roomType, gameID string, limit int, beforeID, afterID uint64) (*ChatMessageList, error) {
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
	query := s.db.Model(&chat.Message{}).Where(
		"room_type = ? AND scope = ? AND room_scope = ? AND game_id = ? AND deleted_at IS NULL",
		roomType, scope, roomScope, gameID,
	)
	// Game rooms should contain genuine member conversation and bet activity.
	// Older releases also wrote synthetic filler text through activity accounts;
	// hide those rows without deleting real room history or lobby messages.
	if roomType == "group" && gameID != "lobby" {
		activityAccounts := s.db.Model(&user.User{}).
			Select("user_id").
			Where("remark = ?", roomActivityRemark)
		query = query.Where("user_id NOT IN (?)", activityAccounts)
	}
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
	claimedPackets, err := s.claimedRedPackets(userID, rows)
	if err != nil {
		return nil, apperrors.NewSystemError("CHAT_READ_FAILED", "读取红包状态失败", err)
	}
	if afterID > 0 {
		for _, row := range rows {
			items = append(items, chatMessageView(row, userID, publicIDs[row.UserID], claimedPackets[row.ID]))
		}
	} else {
		for i := len(rows) - 1; i >= 0; i-- {
			row := rows[i]
			items = append(items, chatMessageView(row, userID, publicIDs[row.UserID], claimedPackets[row.ID]))
		}
	}
	nextBeforeID := uint64(0)
	if len(items) > 0 {
		nextBeforeID = items[0].ID
	}
	return &ChatMessageList{Items: items, HasMore: hasMore && afterID == 0, NextBeforeID: nextBeforeID}, nil
}

func (s *MemberChatService) Post(userID uint64, roomType, gameID, content string) (*ChatMessageView, error) {
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
	scope, roomScope, gameID, err := s.chatContext(account, roomType, gameID)
	if err != nil {
		return nil, err
	}
	if roomType == "group" && gameID != "lobby" && account.ParentAgentID != nil && !NewChatAdminService(s.db).LotteryRoomEnabled(*account.ParentAgentID, gameID) {
		return nil, apperrors.NewBusinessError("LOTTERY_ROOM_CLOSED", "该彩票室已关闭")
	}
	row := chat.Message{
		UserID: userID, Username: account.Username, Nickname: nickname,
		RoomType: roomType, Scope: scope, RoomScope: roomScope, GameID: gameID, Content: content, MessageType: "text",
	}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, apperrors.NewSystemError("CHAT_SAVE_FAILED", "发送消息失败", err)
	}
	view := ChatMessageView{
		ID: row.ID, UserID: row.UserID, PublicID: account.PublicID, Username: row.Username, Nickname: row.Nickname,
		RoomType: row.RoomType, RoomScope: row.RoomScope, GameID: row.GameID,
		Content: row.Content, MessageType: row.MessageType, ReferenceID: row.ReferenceID, Mine: true, CreatedAt: row.CreatedAt,
	}
	recipients, err := s.scopeRecipients(row.Scope)
	if err == nil {
		notifyChatEvent(s.db, recipients, roomType, row.RoomScope, row.GameID, row.Scope, row.ID)
	}
	return &view, nil
}

// PostAssistant persists a room-scoped assistant reply. It is used for
// application acknowledgements and review results so they survive refreshes
// and cannot leak into another room or lottery conversation.
func (s *MemberChatService) PostAssistant(roomScope, gameID, content string, referenceID uint64) (*ChatMessageView, error) {
	roomScope = strings.TrimSpace(roomScope)
	gameID = strings.TrimSpace(gameID)
	content = strings.TrimSpace(content)
	if content == "" || gameID == "" || (roomScope != "lobby" && !strings.HasPrefix(roomScope, "agent:")) {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "群聊回复范围不正确")
	}
	row := chat.Message{
		UserID: 0, Username: "draw_assistant", Nickname: "开奖助手",
		RoomType: "group", Scope: roomScope, RoomScope: roomScope, GameID: gameID,
		Content: content, MessageType: "application", ReferenceID: referenceID,
	}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, apperrors.NewSystemError("CHAT_SAVE_FAILED", "保存申请回复失败", err)
	}
	view := ChatMessageView{
		ID: row.ID, UserID: 0, Nickname: row.Nickname, RoomType: row.RoomType,
		RoomScope: row.RoomScope, GameID: row.GameID, Content: row.Content,
		MessageType: row.MessageType, ReferenceID: row.ReferenceID, Mine: false, CreatedAt: row.CreatedAt,
	}
	if recipients, err := s.scopeRecipients(row.Scope); err == nil {
		notifyChatEvent(s.db, recipients, row.RoomType, row.RoomScope, row.GameID, row.Scope, row.ID)
	}
	return &view, nil
}

func chatMessageView(row chat.Message, userID, publicID uint64, claimedRewardCents int64) ChatMessageView {
	return ChatMessageView{
		ID: row.ID, UserID: row.UserID, PublicID: publicID, Username: row.Username, Nickname: row.Nickname,
		RoomType: row.RoomType, RoomScope: row.RoomScope, GameID: row.GameID,
		Content: row.Content, MessageType: defaultString(row.MessageType, "text"), ReferenceID: row.ReferenceID,
		RedPacketCount: row.RedPacketCount, RedPacketTotal: centsToAmount(row.RedPacketTotalCents),
		RedPacketMinTurnover: centsToAmount(row.RedPacketMinTurnoverCents), RedPacketCover: row.RedPacketCover,
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
	if err := s.db.Where("id = ? AND room_type = ? AND message_type = ? AND deleted_at IS NULL", messageID, "group", "redpacket").First(&row).Error; err != nil {
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
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var packet chat.RedPacket
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&packet, packetID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("NOT_FOUND", "红包不存在或已失效")
			}
			return err
		}
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
		if packet.MinDailyTurnoverCents > 0 {
			var turnoverCents int64
			if err := tx.Model(&bet.Bet{}).
				Where("user_id = ? AND created_at >= ? AND status IN ?", userID, startOfDayCST(time.Now()), []string{"won", "lost"}).
				Select("COALESCE(SUM(amount_cents),0)").Scan(&turnoverCents).Error; err != nil {
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
		claim := chat.RedPacketClaim{PacketID: packet.ID, UserID: userID, AmountCents: rewardCents}
		if err := tx.Create(&claim).Error; err != nil {
			if isDuplicateParticipation(err) {
				return apperrors.NewBusinessError("REDPACKET_CLAIMED", "该红包已领取")
			}
			return err
		}

		var account user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, userID).Error; err != nil {
			return err
		}
		before := account.BalanceCents
		after := before + rewardCents
		if err := tx.Model(&account).Update("balance_cents", after).Error; err != nil {
			return err
		}
		remark := "红包奖励 · " + defaultString(strings.TrimSpace(packet.Greeting), "恭喜发财")
		if err := tx.Create(&user.BalanceTransaction{
			UserID: userID, Reference: "chat_red_packet_claim:" + strconv.FormatUint(claim.ID, 10),
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
		if err := tx.Model(&packet).Updates(map[string]any{
			"remaining_cents": remaining,
			"claimed_count":   claimedCount,
			"status":          status,
		}).Error; err != nil {
			return err
		}

		content := remark + "，到账 " + formatAmount(rewardCents) + " 元"
		notice := membernotify.MemberNotification{
			UserID: userID, GameID: packet.GameID, RoomScope: packet.RoomScope,
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
	ws.NotifyUser(userID, "balance", map[string]any{"balance": result.Balance})
	ws.NotifyUser(userID, "notification", map[string]any{"id": notificationID, "category": "account"})
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
	roomScope := chatScope(account, "group")
	query := s.db.Where(
		"room_type = ? AND scope = ? AND room_scope = ? AND game_id = ? AND deleted_at IS NULL",
		"group", roomScope, roomScope, "lobby",
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
	return roomScope, roomScope, game.ID, nil
}

func (s *MemberChatService) account(userID uint64) (user.User, error) {
	var account user.User
	if err := s.db.First(&account, userID).Error; err != nil {
		return user.User{}, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
	}
	return account, nil
}

// Group chat belongs to the current agent room. Customer service is always a
// private user conversation; the user's ParentAgentID still identifies which
// room's support team owns the conversation in the admin console.
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
