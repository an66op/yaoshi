package services

import (
	"backend/data/models/activity"
	"backend/data/models/bet"
	membernotify "backend/data/models/notify"
	"backend/data/models/rebate"
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

type MemberWalletSummary struct {
	TodayTurnover float64 `json:"today_turnover"`
	TodayProfit   float64 `json:"today_profit"`
	TodayRebate   float64 `json:"today_rebate"`
	PendingAmount float64 `json:"pending_amount"`
	PendingCount  int64   `json:"pending_count"`
	TotalBetCount int64   `json:"total_bet_count"`
}

type GameFeedItem struct {
	Nickname  string    `json:"nickname"`
	Detail    string    `json:"detail"`
	Amount    float64   `json:"amount"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *MemberPortalService) WalletSummary(userID uint64) (*MemberWalletSummary, error) {
	start := startOfDayCST(time.Now())
	summary := &MemberWalletSummary{}

	var todayStake int64
	if err := s.db.Model(&bet.Bet{}).Where("user_id = ? AND created_at >= ? AND status <> ?", userID, start, "cancelled").
		Select(`COALESCE(SUM(CASE
			WHEN status IN ('won','lost') THEN COALESCE(valid_turnover_cents,amount_cents)
			WHEN status IN ('pending','settling') THEN amount_cents
			ELSE 0 END),0)`).Scan(&todayStake).Error; err != nil {
		return nil, err
	}
	summary.TodayTurnover = centsToAmount(todayStake)

	var todaySettledStake, todayPayout int64
	if err := s.db.Model(&bet.Bet{}).Where("user_id = ? AND created_at >= ? AND status IN ?", userID, start, []string{"won", "lost"}).
		Select("COALESCE(SUM(amount_cents),0)").Scan(&todaySettledStake).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&bet.Bet{}).Where("user_id = ? AND created_at >= ? AND status IN ?", userID, start, []string{"won", "lost"}).
		Select("COALESCE(SUM(payout_cents),0)").Scan(&todayPayout).Error; err != nil {
		return nil, err
	}
	summary.TodayProfit = centsToAmount(todayPayout - todaySettledStake)

	var rebate int64
	if err := s.db.Model(&user.BalanceTransaction{}).Where("user_id = ? AND created_at >= ? AND type LIKE ?", userID, start, "%rebate%").
		Select("COALESCE(SUM(amount_cents),0)").Scan(&rebate).Error; err != nil {
		return nil, err
	}
	summary.TodayRebate = centsToAmount(rebate)

	var pending int64
	if err := s.db.Model(&bet.Bet{}).Where("user_id = ? AND status = ?", userID, "pending").
		Select("COALESCE(SUM(amount_cents),0)").Scan(&pending).Error; err != nil {
		return nil, err
	}
	summary.PendingAmount = centsToAmount(pending)

	if err := s.db.Model(&bet.Bet{}).Where("user_id = ? AND status = ?", userID, "pending").Count(&summary.PendingCount).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&bet.Bet{}).Where("user_id = ?", userID).Count(&summary.TotalBetCount).Error; err != nil {
		return nil, err
	}
	return summary, nil
}

type MemberRebatePreview struct {
	BizDate       string  `json:"biz_date"`
	Enabled       bool    `json:"enabled"`
	RatePercent   float64 `json:"rate_percent"`
	TodayTurnover float64 `json:"today_turnover"`
	Estimated     float64 `json:"estimated"`
	Credited      float64 `json:"credited"`
	PendingCredit float64 `json:"pending_credit"`
}

func (s *MemberPortalService) RebatePreview(userID uint64) (*MemberRebatePreview, error) {
	var account user.User
	if err := s.db.Select("workspace_id").First(&account, userID).Error; err != nil {
		return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
	}
	cfg, err := s.loadRebateConfig(account.WorkspaceID)
	if err != nil {
		return nil, err
	}
	start := startOfDayCST(time.Now())
	bizDate := bizDateCST(time.Now())
	var turnover int64
	if err := s.db.Model(&bet.Bet{}).Where("user_id = ? AND created_at >= ? AND status IN ?", userID, start, []string{"won", "lost"}).
		Select("COALESCE(SUM(COALESCE(valid_turnover_cents,amount_cents)),0)").Scan(&turnover).Error; err != nil {
		return nil, err
	}
	turnoverAmount := centsToAmount(turnover)
	rate, _, rateErr := NewTradingAdminService(s.db).ResolveRebateRate(userID)
	if rateErr != nil {
		return nil, rateErr
	}
	estimated := 0.0
	if cfg.Enabled && rate > 0 && turnoverAmount >= cfg.MinTurnover {
		estimated = roundMoney(turnoverAmount * rate / 100)
	}
	var credited int64
	_ = s.db.Model(&rebate.DailyRecord{}).Where("workspace_id = ? AND biz_date = ? AND user_id = ?", account.WorkspaceID, bizDate, userID).
		Select("COALESCE(SUM(amount_cents),0)").Scan(&credited).Error
	pending := estimated - centsToAmount(credited)
	if pending < 0 {
		pending = 0
	}
	return &MemberRebatePreview{
		BizDate: bizDate, Enabled: cfg.Enabled, RatePercent: rate,
		TodayTurnover: turnoverAmount, Estimated: estimated,
		Credited: centsToAmount(credited), PendingCredit: pending,
	}, nil
}

func (s *MemberPortalService) loadRebateConfig(workspaceID uint64) (RebateConfig, error) {
	var row settings.SystemConfig
	if err := s.db.Where("workspace_id = ?", workspaceID).First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return *disabledRebateConfig(), nil
		}
		return RebateConfig{}, err
	}
	cfg, err := decodeStoredRebateConfig(row.RebateSettingsJSON)
	if err != nil {
		return RebateConfig{}, err
	}
	return *cfg, nil
}

func (s *MemberPortalService) GameFeed(userID uint64, gameID, issue string, limit int) ([]GameFeedItem, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var account user.User
	if err := s.db.Select("user_id", "workspace_id", "role", "parent_agent_id").First(&account, userID).Error; err != nil {
		return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
	}
	gameID = strings.TrimSpace(gameID)
	// The member's own ticket has a private assistant receipt. The public room
	// feed contains other members' accepted wagers only, preventing duplicate
	// self cards and keeping another member's parsing details private.
	query := s.db.Model(&bet.Bet{}).
		Where("workspace_id = ? AND room_scope = ? AND game_id = ?", account.WorkspaceID, betRoomScope(account), gameID).
		Where("user_id <> ?", userID)
	if issue = strings.TrimSpace(issue); issue != "" {
		query = query.Where("issue = ?", issue)
	}
	var rows []bet.Bet
	if err := query.Order("created_at desc").Limit(limit).Find(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("FEED_READ_FAILED", "读取投注动态失败", err)
	}
	userIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		userIDs = append(userIDs, row.UserID)
	}
	nicknames := map[uint64]string{}
	if len(userIDs) > 0 {
		var accounts []user.User
		if err := s.db.Select("user_id", "nickname").Where("user_id IN ?", userIDs).Find(&accounts).Error; err != nil {
			return nil, apperrors.NewSystemError("FEED_READ_FAILED", "读取投注昵称失败", err)
		}
		for _, account := range accounts {
			nicknames[account.UserID] = account.Nickname
		}
	}
	items := make([]GameFeedItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, GameFeedItem{
			Nickname:  feedNickname(row, nicknames[row.UserID]),
			Detail:    fmt.Sprintf("%s · 第%d球 · %s", row.PlayName, row.Position, row.Selection),
			Amount:    centsToAmount(row.AmountCents),
			CreatedAt: row.CreatedAt,
		})
	}
	return items, nil
}

var feedAliases = []string{
	"幸运流星", "追风旅人", "晴空玩家", "银翼探索者", "星河漫游者",
	"热心云朵", "稳稳向前", "清爽海盐", "逐光伙伴", "自在松果",
}

// The live feed intentionally does not expose an account name. Generated,
// neutral aliases make the room feel alive without displaying a user's real
// identity or awkward masked strings such as d**o.
func feedNickname(row bet.Bet, nickname string) string {
	if strings.HasPrefix(row.Username, "testbot_") && strings.TrimSpace(nickname) != "" {
		return nickname
	}
	return feedAliases[int(row.UserID%uint64(len(feedAliases)))]
}

func notifyMemberApplication(db *gorm.DB, item *applicationReviewNotify) {
	if item == nil || item.UserID == 0 {
		return
	}
	title := "申请审核结果"
	content := item.Remark
	level := "info"
	if item.Decision == "approved" {
		level = "success"
		title = "申请已通过"
	} else if item.Decision == "rejected" {
		level = "warning"
		title = "申请未通过"
	}
	notice := membernotify.MemberNotification{
		WorkspaceID: item.WorkspaceID, UserID: item.UserID, Title: title, Content: content,
		Level: level, Category: "account",
	}
	if err := db.Create(&notice).Error; err != nil {
		return
	}
	ws.NotifyUser(item.UserID, "notification", map[string]any{
		"id": notice.ID, "title": title, "content": content,
		"workspace_id": item.WorkspaceID, "level": level, "category": "account", "created_at": notice.CreatedAt,
	})

	if strings.TrimSpace(item.RoomScope) == "" || strings.TrimSpace(item.GameID) == "" {
		return
	}
	var account user.User
	nickname := "会员"
	if err := db.Select("nickname", "username").First(&account, item.UserID).Error; err == nil {
		nickname = defaultString(account.Nickname, account.Username)
	}
	label := map[string]string{"credit": "上分", "debit": "下分"}[item.RequestType]
	if label == "" {
		return
	}
	amount := item.RequestedAmount
	if item.Decision == "approved" && item.RequestType == "credit" && item.ReceivedAmount > 0 {
		amount = item.ReceivedAmount
	}
	resultText := "申请未通过审核"
	if item.Decision == "approved" {
		resultText = "申请已通过审核！"
	}
	message := fmt.Sprintf("@%s\n[%s %s]%s", nickname, label, strconv.FormatFloat(amount, 'f', -1, 64), resultText)
	_, _ = NewMemberChatService(db).PostAssistant(item.RoomScope, item.GameID, message, item.ApplicationID)
}

type applicationReviewNotify struct {
	WorkspaceID     uint64
	UserID          uint64
	Decision        string
	Remark          string
	RequestType     string
	RequestedAmount float64
	ReceivedAmount  float64
	RoomScope       string
	GameID          string
	ApplicationID   uint64
}

type MemberInviteInfo struct {
	InviteCode   string  `json:"invite_code"`
	Username     string  `json:"username"`
	RoomCode     string  `json:"room_code"`
	Title        string  `json:"title"`
	Reward       float64 `json:"reward"`
	InvitedCount int64   `json:"invited_count"`
	TotalReward  float64 `json:"total_reward"`
	ShareText    string  `json:"share_text"`
}

type memberInviteRewardSummary struct {
	InvitedCount     int64
	TotalRewardCents int64
}

func memberInviteRewardSummaryQuery(db *gorm.DB, userID uint64) *gorm.DB {
	return db.Model(&activity.Participation{}).
		Where("user_id = ? AND action = ?", userID, "invite_referral")
}

// Keep four numeric characters after the U prefix so generated invite codes
// are easy to read and never shorter than the registration requirement.
func formatInviteCode(userID uint64) string {
	return fmt.Sprintf("U%04d", userID)
}

func (s *MemberPortalService) InviteInfo(userID uint64) (*MemberInviteInfo, error) {
	profile, err := NewMemberService(s.db).Profile(userID)
	if err != nil {
		return nil, err
	}
	info := &MemberInviteInfo{
		InviteCode: formatInviteCode(userID),
		Username:   profile.Username,
		RoomCode:   profile.RoomCode,
		ShareText:  fmt.Sprintf("我在王者娱乐，邀请码 %s", formatInviteCode(userID)),
	}
	var summary memberInviteRewardSummary
	if err := memberInviteRewardSummaryQuery(s.db, userID).
		Select("COUNT(*) AS invited_count, COALESCE(SUM(reward_cents), 0) AS total_reward_cents").
		Scan(&summary).Error; err != nil {
		return nil, err
	}
	info.InvitedCount = summary.InvitedCount
	info.TotalReward = centsToAmount(summary.TotalRewardCents)
	activities, err := s.ListActivities(userID, "invite")
	if err == nil {
		for _, item := range activities {
			if item.Status == "active" {
				info.Title = item.Title
				info.Reward = item.Reward
				info.ShareText = fmt.Sprintf("邀请好友加入，双方各得 %.2f 元奖励。我的邀请码：%s", item.Reward, info.InviteCode)
				if profile.RoomCode != "" {
					info.ShareText += fmt.Sprintf("，房间号 %s", profile.RoomCode)
				}
				break
			}
		}
	}
	if info.Title == "" {
		info.Title = "邀请好友"
	}
	return info, nil
}
