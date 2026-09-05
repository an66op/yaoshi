package services

import (
	"backend/data/models/activity"
	"backend/data/models/lottery"
	membernotify "backend/data/models/notify"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"backend/ws"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MemberPortalService struct {
	db       *gorm.DB
	settings *SettingsAdminService
	trading  *TradingAdminService
	activity *ActivityAdminService
}

type MemberRoomSettingsView struct {
	RoomName          string             `json:"room_name"`
	RoomLogo          string             `json:"room_logo"`
	RoomNotice        string             `json:"room_notice"`
	Announcements     []AnnouncementItem `json:"announcements"`
	ShowOdds          bool               `json:"show_odds"`
	SoundEnabled      bool               `json:"sound_enabled"`
	PredictionEnabled bool               `json:"prediction_enabled"`
	MinCreditAmount   float64            `json:"min_credit_amount"`
	MinDebitAmount    float64            `json:"min_debit_amount"`
	MinChatScore      float64            `json:"min_chat_score"`
	ChatNickname      string             `json:"chat_nickname"`
	ChatAvatar        string             `json:"chat_avatar"`
	OperatorTitle     string             `json:"operator_title"`
	OperatorAvatar    string             `json:"operator_avatar"`
	LotterySourceURL  string             `json:"lottery_source_url"`
	Game              json.RawMessage    `json:"game"`
	QuickReplies      json.RawMessage    `json:"quick_replies"`
}

type MemberOddsView struct {
	RulesReady   bool             `json:"rules_ready"`
	RuleVersion  string           `json:"rule_version,omitempty"`
	RulesMessage string           `json:"rules_message,omitempty"`
	BetModes     MemberBetModes   `json:"bet_modes"`
	GameID       string           `json:"game_id"`
	GameName     string           `json:"game_name"`
	ShowOdds     bool             `json:"show_odds"`
	Items        []MemberOddsItem `json:"items"`
}

type MemberBetModes struct {
	Chat bool `json:"chat"`
	Web  bool `json:"web"`
}

type MemberOddsItem struct {
	PlayCode      string  `json:"play_code"`
	PlayName      string  `json:"play_name"`
	Category      string  `json:"category,omitempty"`
	Description   string  `json:"description,omitempty"`
	Example       string  `json:"example,omitempty"`
	Odds          float64 `json:"odds"`
	MinBet        float64 `json:"min_bet"`
	MaxBet        float64 `json:"max_bet"`
	MaxUserPeriod float64 `json:"max_user_period"`
}

type ActivityStatusView struct {
	ActivityID   uint64  `json:"activity_id"`
	Type         string  `json:"type"`
	Title        string  `json:"title"`
	CheckedIn    bool    `json:"checked_in"`
	Claimed      bool    `json:"claimed"`
	Streak       int     `json:"streak"`
	Reward       float64 `json:"reward"`
	Participants int64   `json:"participants"`
	Config       any     `json:"config"`
}

type ActivityActionResult struct {
	Reward  float64 `json:"reward"`
	Streak  int     `json:"streak"`
	Balance float64 `json:"balance"`
	Message string  `json:"message"`
}

type MemberNotificationView struct {
	ID           uint64                  `json:"id"`
	GameID       string                  `json:"game_id,omitempty"`
	RoomScope    string                  `json:"room_scope,omitempty"`
	Title        string                  `json:"title"`
	Content      string                  `json:"content"`
	Level        string                  `json:"level"`
	Category     string                  `json:"category"`
	Link         string                  `json:"link"`
	Read         bool                    `json:"read"`
	GameName     string                  `json:"game_name,omitempty"`
	Issue        string                  `json:"issue,omitempty"`
	DrawNumbers  []int                   `json:"draw_numbers,omitempty"`
	DrawAt       *time.Time              `json:"draw_at,omitempty"`
	BetCount     int                     `json:"bet_count,omitempty"`
	WonCount     int                     `json:"won_count,omitempty"`
	StakeAmount  float64                 `json:"stake_amount,omitempty"`
	PayoutAmount float64                 `json:"payout_amount,omitempty"`
	BetDetails   []NotificationBetDetail `json:"bet_details,omitempty"`
	CreatedAt    time.Time               `json:"created_at"`
}

type MemberNotificationList struct {
	Items        []MemberNotificationView `json:"items"`
	HasMore      bool                     `json:"has_more"`
	NextBeforeID uint64                   `json:"next_before_id,omitempty"`
}

func NewMemberPortalService(db *gorm.DB) *MemberPortalService {
	return &MemberPortalService{
		db:       db,
		settings: NewSettingsAdminService(db),
		trading:  NewTradingAdminService(db),
		activity: NewActivityAdminService(db),
	}
}

func (s *MemberPortalService) RoomSettings(userID uint64) (*MemberRoomSettingsView, error) {
	var account user.User
	if err := s.db.Select("workspace_id").First(&account, userID).Error; err != nil {
		return nil, err
	}
	cfg, err := s.settings.GetForWorkspaceWithInheritedAnnouncements(account.WorkspaceID)
	if err != nil {
		return nil, err
	}
	operatorAvatar := defaultString(strings.TrimSpace(cfg.ChatAvatar), cfg.RoomLogo)
	return &MemberRoomSettingsView{
		RoomName: cfg.RoomName, RoomLogo: roomLogoForDisplay(cfg.RoomLogo), RoomNotice: cfg.RoomNotice, Announcements: cfg.Announcements, ShowOdds: cfg.ShowOdds,
		SoundEnabled: cfg.SoundEnabled, PredictionEnabled: cfg.PredictionEnabled, MinCreditAmount: cfg.MinCreditAmount,
		MinDebitAmount: cfg.MinDebitAmount, MinChatScore: cfg.MinChatScore,
		ChatNickname: cfg.ChatNickname, ChatAvatar: operatorAvatar,
		OperatorTitle: cfg.ChatNickname, OperatorAvatar: operatorAvatar,
		LotterySourceURL: cfg.LotterySourceURL,
		Game:             cfg.Game, QuickReplies: cfg.QuickReplies,
	}, nil
}

func (s *MemberPortalService) GameOdds(userID uint64, gameID string) (*MemberOddsView, error) {
	var account user.User
	if err := s.db.Select("workspace_id").First(&account, userID).Error; err != nil {
		return nil, err
	}
	cfg, err := s.settings.GetForWorkspace(account.WorkspaceID)
	if err != nil {
		return nil, err
	}
	trading, err := s.trading.Get(userID, gameID)
	if err != nil {
		return nil, err
	}
	limits, err := NewOddsAdminService(s.db).Get(trading.GameID)
	if err != nil {
		return nil, err
	}
	limitMap := map[string]PlayLimitItem{}
	for _, item := range limits.Items {
		limitMap[item.PlayCode] = item
	}
	catalogMap := map[string]PlayCatalogItem{}
	for _, item := range PlayCatalogForGame(trading.GameID) {
		catalogMap[item.PlayCode] = item
	}
	items := make([]MemberOddsItem, 0, len(trading.Odds))
	for _, row := range trading.Odds {
		limit, exists := limitMap[row.PlayCode]
		if !exists || !limit.Configured || limit.Odds <= 1 {
			continue
		}
		catalog := catalogMap[row.PlayCode]
		odds := row.Effective
		if !cfg.ShowOdds {
			odds = 0
		}
		items = append(items, MemberOddsItem{
			PlayCode: row.PlayCode, PlayName: row.PlayName, Category: catalog.Category,
			Description: catalog.Description, Example: catalog.Example, Odds: odds,
			MinBet: limit.MinBet, MaxBet: limit.MaxBet, MaxUserPeriod: limit.MaxUserPeriod,
		})
	}
	ready, ruleVersion, rulesMessage, betModes := memberOddsRuleStatus(trading.GameID)
	return &MemberOddsView{
		RulesReady: ready, RuleVersion: ruleVersion, RulesMessage: rulesMessage,
		BetModes: betModes,
		GameID:   trading.GameID, GameName: trading.GameName,
		ShowOdds: cfg.ShowOdds, Items: items,
	}, nil
}

// Betting modes describe product capability, not the optional on-screen input
// keyboard. Mark Six is intentionally typed-web-only; compact chat parsing is
// retained only for the existing racing and digit contracts.
func memberOddsRuleStatus(gameID string) (bool, string, string, MemberBetModes) {
	rules, ready := rulesForGame(&lottery.Game{ID: strings.TrimSpace(gameID)})
	if !ready {
		return false, "", gameRulesUnavailableMessage, MemberBetModes{}
	}
	switch rules.Version {
	case "racing-v2", "digits5-v3", "digits3-v2", pc28RuleV1, pc28RuleV2, pc28RuleV3:
		return true, rules.Version, "", MemberBetModes{Chat: true, Web: true}
	case markSixRuleVersion, hongKongMarkSixRuleVersion, happy8MarkSixRuleVersion, newMacauMarkSixRuleVersion, oldMacauMarkSixRuleVersion:
		return true, rules.Version, "", MemberBetModes{Chat: false, Web: true}
	default:
		return false, "", gameRulesUnavailableMessage, MemberBetModes{}
	}
}

func (s *MemberPortalService) ListActivities(userID uint64, activityType string) ([]ActivityView, error) {
	var account user.User
	if err := s.db.Select("workspace_id").First(&account, userID).Error; err != nil {
		return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
	}
	items, err := s.activity.ListForWorkspace(account.WorkspaceID, "active")
	if err != nil {
		return nil, err
	}
	if typ := strings.TrimSpace(activityType); typ != "" && typ != "all" {
		filtered := make([]ActivityView, 0, len(items))
		for _, item := range items {
			if item.Type == typ {
				filtered = append(filtered, item)
			}
		}
		return filtered, nil
	}
	return items, nil
}

func (s *MemberPortalService) ActivityStatus(userID, activityID uint64) (*ActivityStatusView, error) {
	var account user.User
	if err := s.db.Select("workspace_id").First(&account, userID).Error; err != nil {
		return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
	}
	var row activity.Activity
	if err := s.db.Where("id = ? AND workspace_id = ?", activityID, account.WorkspaceID).First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("NOT_FOUND", "活动不存在")
		}
		return nil, err
	}
	if row.Status != "active" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "活动未开启")
	}
	action := row.Type
	if action == "checkin" {
		action = "checkin"
	}
	bizDate := bizDateCST(time.Now())
	view := &ActivityStatusView{
		ActivityID: row.ID, Type: row.Type, Title: row.Title,
		Reward: centsToAmount(row.RewardCents), Participants: row.Participants,
	}
	var cfg any
	_ = json.Unmarshal([]byte(defaultJSON(row.ConfigJSON, "{}")), &cfg)
	view.Config = cfg
	if row.Type == "redpacket" {
		ensureActivityPool(&row)
		if cfgMap, ok := cfg.(map[string]any); ok {
			cfgMap["pool_total"] = centsToAmount(row.PoolTotalCents)
			cfgMap["pool_remaining"] = centsToAmount(row.PoolRemainingCents)
			view.Config = cfgMap
		}
	}

	var today int64
	_ = s.db.Model(&activity.Participation{}).
		Where("workspace_id = ? AND user_id = ? AND activity_id = ? AND action = ? AND biz_date = ?",
			account.WorkspaceID, userID, activityID, action, bizDate).Count(&today).Error
	if row.Type == "checkin" {
		view.CheckedIn = today > 0
		view.Streak = s.checkinStreak(userID, activityID)
	} else if row.Type == "redpacket" {
		view.Claimed = today > 0
	}
	return view, nil
}

func (s *MemberPortalService) CheckIn(userID, activityID uint64) (*ActivityActionResult, error) {
	return s.participate(userID, activityID, "checkin", "")
}

func (s *MemberPortalService) ClaimRedPacket(userID, activityID uint64) (*ActivityActionResult, error) {
	return s.participate(userID, activityID, "redpacket", "")
}

// ClaimChatRedPacket binds a reward to one persisted chat message. The
// message reference is permanent, so refreshing the page or waiting until the
// next day can never make the same envelope claimable again.
func (s *MemberPortalService) ClaimChatRedPacket(userID, activityID, messageID uint64) (*ActivityActionResult, error) {
	return s.participate(userID, activityID, "redpacket", "chat_message:"+strconv.FormatUint(messageID, 10))
}

func (s *MemberPortalService) participate(userID, activityID uint64, action, reference string) (*ActivityActionResult, error) {
	var result ActivityActionResult
	var workspaceID uint64
	var notificationID uint64
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var account user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, userID).Error; err != nil {
			return err
		}
		workspaceID = account.WorkspaceID
		var row activity.Activity
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND workspace_id = ?", activityID, workspaceID).First(&row).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("NOT_FOUND", "活动不存在")
			}
			return err
		}
		if row.Status != "active" {
			return apperrors.NewBusinessError("INVALID_REQUEST", "活动未开启")
		}
		if action == "checkin" && row.Type != "checkin" {
			return apperrors.NewBusinessError("INVALID_REQUEST", "该活动不支持签到")
		}
		if action == "redpacket" && row.Type != "redpacket" {
			return apperrors.NewBusinessError("INVALID_REQUEST", "该活动不支持领红包")
		}
		if reference != "" {
			var claimed int64
			if err := tx.Model(&activity.Participation{}).
				Where("workspace_id = ? AND user_id = ? AND activity_id = ? AND action = ? AND reference = ?", workspaceID, userID, activityID, action, reference).
				Count(&claimed).Error; err != nil {
				return err
			}
			if claimed > 0 {
				return apperrors.NewBusinessError("REDPACKET_CLAIMED", "该红包已领取")
			}
		}

		bizDate := bizDateCST(time.Now())
		now := time.Now().UTC()
		rewardCents := row.RewardCents
		streak := 1
		if action == "checkin" {
			prev := s.checkinStreak(userID, activityID)
			streak = prev + 1
			if rewardCents <= 0 {
				rewardCents = int64(streak) * 100
			}
		}
		if action == "redpacket" {
			ensureActivityPool(&row)
			cfg := parseRedPacketConfig(row.ConfigJSON)
			var err error
			rewardCents, err = drawRedPacketReward(row.PoolRemainingCents, cfg)
			if err != nil {
				return err
			}
		}

		part := activity.Participation{
			WorkspaceID: workspaceID, UserID: userID, ActivityID: activityID, Action: action, BizDate: bizDate,
			Reference: reference, RewardCents: rewardCents, Streak: streak, ParticipatedAt: now,
		}
		if err := tx.Create(&part).Error; err != nil {
			if isDuplicateParticipation(err) {
				if reference != "" {
					return apperrors.NewBusinessError("REDPACKET_CLAIMED", "该红包已领取")
				}
				if action == "checkin" {
					return apperrors.NewBusinessError("CHECKIN_ALREADY_COMPLETED", "今日已签到，请明日再来")
				}
				return apperrors.NewBusinessError("INVALID_REQUEST", "今日已参与，请明日再来")
			}
			return err
		}

		if action == "redpacket" {
			nextRemaining := row.PoolRemainingCents - rewardCents
			if err := tx.Model(&row).Updates(map[string]any{
				"pool_total_cents":     row.PoolTotalCents,
				"pool_remaining_cents": nextRemaining,
				"participants":         gorm.Expr("participants + 1"),
			}).Error; err != nil {
				return err
			}
		} else if err := tx.Model(&row).Update("participants", gorm.Expr("participants + 1")).Error; err != nil {
			return err
		}

		before := account.BalanceCents
		after := before + rewardCents
		if err := tx.Model(&account).Update("balance_cents", after).Error; err != nil {
			return err
		}
		remark := row.Title
		if action == "checkin" {
			remark = "签到积分 · 连续" + itoa(streak) + "天"
		} else {
			remark = "红包奖励 · " + row.Title
		}
		if err := tx.Create(&user.BalanceTransaction{
			WorkspaceID: workspaceID, UserID: userID, Reference: "activity_participation:" + strconv.FormatUint(part.ID, 10),
			AmountCents: rewardCents, BeforeCents: before, AfterCents: after,
			Type: action, Remark: remark, Operator: "系统",
		}).Error; err != nil {
			return err
		}
		title := "签到成功"
		content := remark + "，获得 " + formatAmount(rewardCents) + " 积分"
		if action == "redpacket" {
			title = "红包领取成功"
			content = remark + "，到账 " + formatAmount(rewardCents) + " 元"
		}
		notice := membernotify.MemberNotification{
			WorkspaceID: workspaceID, UserID: userID, Title: title, Content: content,
			Level: "success", Category: "account",
		}
		if err := tx.Create(&notice).Error; err != nil {
			return err
		}
		notificationID = notice.ID
		result = ActivityActionResult{
			Reward: centsToAmount(rewardCents), Streak: streak,
			Balance: centsToAmount(after), Message: content,
		}
		return nil
	})
	if err != nil {
		if app, ok := err.(*apperrors.AppError); ok {
			return nil, app
		}
		return nil, apperrors.NewSystemError("ACTIVITY_ACTION_FAILED", "参与活动失败", err)
	}
	ws.NotifyUser(userID, "balance", map[string]any{"workspace_id": workspaceID, "balance": result.Balance})
	ws.NotifyUser(userID, "notification", map[string]any{"workspace_id": workspaceID, "id": notificationID, "category": "account"})
	return &result, nil
}

func isDuplicateParticipation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique") || strings.Contains(msg, "23505")
}

func (s *MemberPortalService) ListNotifications(userID uint64, limit int, beforeID uint64, category, gameID, issue string) (*MemberNotificationList, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	category = strings.TrimSpace(category)
	if category != "" && category != "all" && category != "system" && category != "account" && category != "activity" && category != "winning" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "通知分类不正确")
	}
	account, err := s.notificationAccount(userID)
	if err != nil {
		return nil, err
	}
	var rows []membernotify.MemberNotification
	query := scopedMemberNotificationQuery(s.db, userID, account.WorkspaceID).Where("title <> ?", "客服回复")
	if category != "" && category != "all" {
		query = query.Where("category = ?", category)
		if category == "system" {
			query = query.Where("title NOT IN ?", []string{"开奖结果", "恭喜中奖", "未中奖", "开奖通知"})
		}
	}
	gameID = strings.TrimSpace(gameID)
	issue = strings.TrimSpace(issue)
	if gameID != "" {
		// The client chooses a game, never a room scope.  Room ownership is
		// derived from the authenticated member to prevent cross-room replay.
		query = query.Where("game_id = ? AND room_scope = ?", gameID, betRoomScope(account))
	}
	if issue != "" {
		query = query.Where("issue = ?", issue)
	}
	if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}
	if err := query.Order("id desc").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	out := make([]MemberNotificationView, 0, len(rows))
	for _, row := range rows {
		details := make([]NotificationBetDetail, 0)
		if strings.TrimSpace(row.BetDetailsJSON) != "" {
			_ = json.Unmarshal([]byte(row.BetDetailsJSON), &details)
		}
		out = append(out, MemberNotificationView{
			ID: row.ID, GameID: row.GameID, RoomScope: row.RoomScope,
			Title: row.Title, Content: row.Content, Level: row.Level,
			Category: row.Category, Link: row.Link, Read: row.Read, CreatedAt: row.CreatedAt,
			GameName: row.GameName, Issue: row.Issue, DrawNumbers: parseNumbers(row.DrawNumbers), DrawAt: row.DrawAt,
			BetCount: row.BetCount, WonCount: row.WonCount,
			StakeAmount: centsToAmount(row.StakeCents), PayoutAmount: centsToAmount(row.PayoutCents), BetDetails: details,
		})
	}
	nextBeforeID := uint64(0)
	if len(out) > 0 {
		nextBeforeID = out[len(out)-1].ID
	}
	return &MemberNotificationList{Items: out, HasMore: hasMore, NextBeforeID: nextBeforeID}, nil
}

func (s *MemberPortalService) UnreadCount(userID uint64) (int64, error) {
	account, err := s.notificationAccount(userID)
	if err != nil {
		return 0, err
	}
	var count int64
	err = unreadMemberNotificationQuery(s.db, userID, account.WorkspaceID).
		Count(&count).Error
	return count, err
}

func (s *MemberPortalService) MarkNotificationRead(userID, id uint64) error {
	account, err := s.notificationAccount(userID)
	if err != nil {
		return err
	}
	return scopedMemberNotificationQuery(s.db, userID, account.WorkspaceID).
		Where("id = ?", id).Update("read", true).Error
}

func (s *MemberPortalService) MarkAllNotificationsRead(userID uint64) error {
	account, err := s.notificationAccount(userID)
	if err != nil {
		return err
	}
	return unreadMemberNotificationQuery(s.db, userID, account.WorkspaceID).Update("read", true).Error
}

func (s *MemberPortalService) EnsureWelcomeNotification(userID uint64) error {
	var account user.User
	if err := s.db.Select("workspace_id").First(&account, userID).Error; err != nil {
		return err
	}
	var count int64
	if err := scopedMemberNotificationQuery(s.db, userID, account.WorkspaceID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	cfg, err := s.settings.GetForWorkspace(account.WorkspaceID)
	if err != nil {
		return err
	}
	notice := strings.TrimSpace(cfg.RoomNotice)
	if notice == "" {
		notice = "欢迎来到王者，祝您游戏愉快。"
	}
	return s.db.Create(&membernotify.MemberNotification{
		WorkspaceID: account.WorkspaceID, UserID: userID, Title: "系统公告", Content: notice, Level: "info", Category: "system",
	}).Error
}

var memberUnreadCategories = []string{"system", "activity", "winning"}
var legacySystemResultTitles = []string{"开奖结果", "恭喜中奖", "未中奖", "开奖通知"}

func scopedMemberNotificationQuery(db *gorm.DB, userID, workspaceID uint64) *gorm.DB {
	return db.Model(&membernotify.MemberNotification{}).
		Where("user_id = ? AND workspace_id = ?", userID, workspaceID)
}

// unreadMemberNotificationQuery mirrors the visible member inbox. Account and
// placement/order notices are disabled there and therefore must never leave a
// badge that the user has no visible thread in which to clear.
func unreadMemberNotificationQuery(db *gorm.DB, userID, workspaceID uint64) *gorm.DB {
	return scopedMemberNotificationQuery(db, userID, workspaceID).
		Where("read = ? AND title <> ?", false, "客服回复").
		Where("category IN ?", memberUnreadCategories).
		Where("NOT (category = ? AND title IN ?)", "system", legacySystemResultTitles)
}

func (s *MemberPortalService) notificationAccount(userID uint64) (user.User, error) {
	var account user.User
	if err := s.db.Select("user_id", "workspace_id", "role", "parent_agent_id", "parent_tenant_id").First(&account, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return user.User{}, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
		}
		return user.User{}, err
	}
	if account.WorkspaceID == 0 {
		return user.User{}, apperrors.NewBusinessError("ROOM_NOT_FOUND", "用户尚未进入房间")
	}
	var workspace workspacemodel.Workspace
	if err := s.db.Select("id").Where("id = ? AND status = ?", account.WorkspaceID, 1).First(&workspace).Error; err != nil {
		return user.User{}, apperrors.NewBusinessError("ROOM_NOT_FOUND", "用户所属房间不存在或已停用")
	}
	return account, nil
}

func (s *MemberPortalService) checkinStreak(userID, activityID uint64) int {
	var last activity.Participation
	err := s.db.Where("user_id = ? AND activity_id = ? AND action = ?", userID, activityID, "checkin").
		Order("participated_at desc").First(&last).Error
	if err == gorm.ErrRecordNotFound {
		return 0
	}
	if err != nil {
		return 0
	}
	lastDay := dayStart(last.ParticipatedAt)
	today := dayStart(time.Now())
	yesterday := today.AddDate(0, 0, -1)
	if lastDay.Equal(today) {
		return last.Streak
	}
	if lastDay.Equal(yesterday) {
		return last.Streak
	}
	return 0
}

func dayRange(now time.Time) (time.Time, time.Time) {
	start := dayStart(now)
	return start, start.AddDate(0, 0, 1)
}

func dayStart(now time.Time) time.Time {
	loc := time.FixedZone("CST", 8*3600)
	local := now.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc).UTC()
}

func formatAmount(cents int64) string {
	return fmt.Sprintf("%.2f", centsToAmount(cents))
}

func itoa(v int) string {
	return strconv.Itoa(v)
}
