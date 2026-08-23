package services

import (
	"backend/data/models/bet"
	membernotify "backend/data/models/notify"
	"backend/data/models/rebate"
	"backend/data/models/settings"
	"backend/data/models/user"
	apperrors "backend/errors"
	"encoding/json"
	"fmt"
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
		Select("COALESCE(SUM(amount_cents),0)").Scan(&todayStake).Error; err != nil {
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
	cfg, err := s.loadRebateConfig()
	if err != nil {
		return nil, err
	}
	start := startOfDayCST(time.Now())
	bizDate := bizDateCST(time.Now())
	var turnover int64
	if err := s.db.Model(&bet.Bet{}).Where("user_id = ? AND created_at >= ? AND status IN ?", userID, start, []string{"won", "lost"}).
		Select("COALESCE(SUM(amount_cents),0)").Scan(&turnover).Error; err != nil {
		return nil, err
	}
	turnoverAmount := centsToAmount(turnover)
	estimated := 0.0
	if cfg.Enabled && cfg.RatePercent > 0 && turnoverAmount >= cfg.MinTurnover {
		estimated = roundMoney(turnoverAmount * cfg.RatePercent / 100)
	}
	var credited int64
	_ = s.db.Model(&rebate.DailyRecord{}).Where("biz_date = ? AND user_id = ?", bizDate, userID).
		Select("COALESCE(SUM(amount_cents),0)").Scan(&credited).Error
	pending := estimated - centsToAmount(credited)
	if pending < 0 {
		pending = 0
	}
	return &MemberRebatePreview{
		BizDate: bizDate, Enabled: cfg.Enabled, RatePercent: cfg.RatePercent,
		TodayTurnover: turnoverAmount, Estimated: estimated,
		Credited: centsToAmount(credited), PendingCredit: pending,
	}, nil
}

func (s *MemberPortalService) loadRebateConfig() (RebateConfig, error) {
	var row settings.SystemConfig
	cfg := RebateConfig{Enabled: true, RatePercent: 0.5, MinTurnover: 0, SettleMode: "daily"}
	if err := s.db.First(&row, 1).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return cfg, nil
		}
		return RebateConfig{}, err
	}
	raw := defaultJSON(row.RebateSettingsJSON, "{}")
	_ = json.Unmarshal([]byte(raw), &cfg)
	if cfg.SettleMode == "" {
		cfg.SettleMode = "daily"
	}
	return cfg, nil
}

func (s *MemberPortalService) GameFeed(userID uint64, gameID, issue string, limit int) ([]GameFeedItem, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var account user.User
	if err := s.db.Select("user_id", "role", "parent_agent_id").First(&account, userID).Error; err != nil {
		return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
	}
	gameID = strings.TrimSpace(gameID)
	query := s.db.Model(&bet.Bet{}).Where("room_scope = ? AND game_id = ?", betRoomScope(account), gameID)
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
	_ = db.Create(&membernotify.MemberNotification{
		UserID: item.UserID, Title: title, Content: content,
		Level: level, Category: "system",
	}).Error
}

type applicationReviewNotify struct {
	UserID   uint64
	Decision string
	Remark   string
}

type MemberInviteInfo struct {
	InviteCode string  `json:"invite_code"`
	Username   string  `json:"username"`
	RoomCode   string  `json:"room_code"`
	Title      string  `json:"title"`
	Reward     float64 `json:"reward"`
	ShareText  string  `json:"share_text"`
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
	activities, err := s.ListActivities("invite")
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
