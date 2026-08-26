package services

import (
	"backend/data/models/user"
	apperrors "backend/errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// OperatingReport is the business P&L view.  Unlike FinancialReport (cash
// ledger), it is driven by settled bet snapshots and can be reconciled down to
// one ticket without recalculating historic room contracts.
type OperatingReportService struct{ db *gorm.DB }

type OperatingReportFilter struct {
	Query, Start, End, RoomScope, GameID, Dimension string
	UserID                                          uint64
	Page, PageSize                                  int
}

type OperatingSummary struct {
	PeriodStart       string  `json:"period_start"`
	PeriodEnd         string  `json:"period_end"`
	SettledTurnover   float64 `json:"settled_turnover"`
	Payout            float64 `json:"payout"`
	MemberNet         float64 `json:"member_net"`
	GrossProfit       float64 `json:"gross_profit"`
	GrossMargin       float64 `json:"gross_margin"`
	RebateAccrued     float64 `json:"rebate_accrued"`
	WelfareCost       float64 `json:"welfare_cost"`
	AgentShare        float64 `json:"agent_share"`
	PlatformNetProfit float64 `json:"platform_net_profit"`
	NetMargin         float64 `json:"net_margin"`
	PendingTurnover   float64 `json:"pending_turnover"`
	FlyAmount         float64 `json:"fly_amount"`
	SettledTickets    int64   `json:"settled_tickets"`
	PendingTickets    int64   `json:"pending_tickets"`
	Bettors           int64   `json:"bettors"`
}

type OperatingPoint struct {
	Date           string  `json:"date"`
	Turnover       float64 `json:"turnover"`
	Payout         float64 `json:"payout"`
	GrossProfit    float64 `json:"gross_profit"`
	Rebate         float64 `json:"rebate"`
	Welfare        float64 `json:"welfare"`
	AgentShare     float64 `json:"agent_share"`
	PlatformProfit float64 `json:"platform_profit"`
}

type OperatingBreakdown struct {
	Key            string  `json:"key"`
	Label          string  `json:"label"`
	Turnover       float64 `json:"turnover"`
	Payout         float64 `json:"payout"`
	GrossProfit    float64 `json:"gross_profit"`
	Rebate         float64 `json:"rebate"`
	Welfare        float64 `json:"welfare"`
	AgentShare     float64 `json:"agent_share"`
	PlatformProfit float64 `json:"platform_profit"`
	Tickets        int64   `json:"tickets"`
}

type OperatingBetRecord struct {
	ID             uint64     `json:"id"`
	RoomScope      string     `json:"room_scope"`
	GameID         string     `json:"game_id"`
	GameName       string     `json:"game_name"`
	Issue          string     `json:"issue"`
	UserID         uint64     `json:"user_id"`
	Username       string     `json:"username"`
	PlayName       string     `json:"play_name"`
	Selection      string     `json:"selection"`
	Status         string     `json:"status"`
	Stake          float64    `json:"stake"`
	Payout         float64    `json:"payout"`
	MemberNet      float64    `json:"member_net"`
	GrossProfit    float64    `json:"gross_profit"`
	RebateRate     float64    `json:"rebate_rate"`
	Rebate         float64    `json:"rebate"`
	AgentShareRate float64    `json:"agent_share_rate"`
	AgentShare     float64    `json:"agent_share"`
	PlatformProfit float64    `json:"platform_profit"`
	FlyAmount      float64    `json:"fly_amount"`
	SettledAt      *time.Time `json:"settled_at,omitempty"`
}

type OperatingReport struct {
	Summary   OperatingSummary     `json:"summary"`
	Trend     []OperatingPoint     `json:"trend"`
	Breakdown []OperatingBreakdown `json:"breakdown"`
	Items     []OperatingBetRecord `json:"items"`
	Total     int64                `json:"total"`
	Page      int                  `json:"page"`
	PageSize  int                  `json:"page_size"`
}

type operatingAggregate struct {
	TurnoverCents   int64 `gorm:"column:turnover_cents"`
	PayoutCents     int64 `gorm:"column:payout_cents"`
	RebateCents     int64 `gorm:"column:rebate_cents"`
	AgentShareCents int64 `gorm:"column:agent_share_cents"`
	FlyCents        int64 `gorm:"column:fly_cents"`
	Tickets         int64 `gorm:"column:tickets"`
	Bettors         int64 `gorm:"column:bettors"`
}

type operatingRow struct {
	ID                                                               uint64
	RoomScope, GameID, GameName, Issue                               string
	UserID                                                           uint64
	Username, PlayName, Selection, Status                            string
	AmountCents, PayoutCents, RebateCents, AgentShareCents, FlyCents int64
	RebateRateSnapshot, AgentShareRateSnapshot                       float64
	SettledAt                                                        *time.Time
}

func NewOperatingReportService(db *gorm.DB) *OperatingReportService {
	return &OperatingReportService{db: db}
}

func (s *OperatingReportService) Report(filter OperatingReportFilter) (*OperatingReport, error) {
	period, err := parseReportPeriod(filter.Start, filter.End)
	if err != nil {
		return nil, err
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	if filter.Dimension == "" {
		filter.Dimension = "room"
	}
	if filter.Dimension != "room" && filter.Dimension != "game" && filter.Dimension != "user" {
		return nil, apperrors.NewBusinessError("INVALID_REPORT_DIMENSION", "不支持的汇总维度")
	}
	if scope := strings.TrimSpace(filter.RoomScope); scope != "" && scope != "lobby" && !strings.HasPrefix(scope, "agent:") {
		return nil, apperrors.NewBusinessError("INVALID_ROOM_SCOPE", "房间范围不正确")
	}

	settled := s.filteredBets(filter, period, true)
	var agg operatingAggregate
	if err := settled.Select(`COALESCE(SUM(b.amount_cents),0) turnover_cents,
		COALESCE(SUM(b.payout_cents),0) payout_cents, COALESCE(SUM(b.rebate_cents),0) rebate_cents,
		COALESCE(SUM(b.agent_share_cents),0) agent_share_cents, COALESCE(SUM(b.fly_cents),0) fly_cents,
		COUNT(*) tickets, COUNT(DISTINCT b.user_id) bettors`).Scan(&agg).Error; err != nil {
		return nil, err
	}

	pending := s.filteredBets(filter, period, false).Where("b.status = ?", "pending")
	var pendingAgg struct {
		AmountCents int64
		Tickets     int64
	}
	if err := pending.Select("COALESCE(SUM(b.amount_cents),0) amount_cents, COUNT(*) tickets").Scan(&pendingAgg).Error; err != nil {
		return nil, err
	}
	welfareCents, err := s.welfareCost(filter, period)
	if err != nil {
		return nil, err
	}

	gross, platform := operatingProfitCents(agg.TurnoverCents, agg.PayoutCents, agg.RebateCents, welfareCents, agg.AgentShareCents)
	summary := OperatingSummary{
		PeriodStart: period.StartDate, PeriodEnd: period.EndDate,
		SettledTurnover: centsToAmount(agg.TurnoverCents), Payout: centsToAmount(agg.PayoutCents),
		MemberNet: centsToAmount(agg.PayoutCents - agg.TurnoverCents), GrossProfit: centsToAmount(gross),
		RebateAccrued: centsToAmount(agg.RebateCents), WelfareCost: centsToAmount(welfareCents),
		AgentShare: centsToAmount(agg.AgentShareCents), PlatformNetProfit: centsToAmount(platform),
		PendingTurnover: centsToAmount(pendingAgg.AmountCents), FlyAmount: centsToAmount(agg.FlyCents),
		SettledTickets: agg.Tickets, PendingTickets: pendingAgg.Tickets, Bettors: agg.Bettors,
	}
	if agg.TurnoverCents > 0 {
		summary.GrossMargin = roundMoney(float64(gross) / float64(agg.TurnoverCents) * 100)
		summary.NetMargin = roundMoney(float64(platform) / float64(agg.TurnoverCents) * 100)
	}
	trend, err := s.trend(filter, period)
	if err != nil {
		return nil, err
	}
	breakdown, err := s.breakdown(filter, period)
	if err != nil {
		return nil, err
	}

	var total int64
	if err := settled.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []operatingRow
	if err := settled.Select(`b.id, b.room_scope, b.game_id, COALESCE(g.name,b.game_id) game_name, b.issue,
		b.user_id, b.username, b.play_name, b.selection, b.status, b.amount_cents, b.payout_cents,
		b.rebate_rate_snapshot, b.rebate_cents, b.agent_share_rate_snapshot, b.agent_share_cents,
		b.fly_cents, b.settled_at`).
		Joins("LEFT JOIN lottery_games g ON g.id = b.game_id").
		Order("COALESCE(b.settled_at,b.updated_at,b.created_at) DESC, b.id DESC").
		Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]OperatingBetRecord, 0, len(rows))
	for _, row := range rows {
		grossCents, platformCents := operatingProfitCents(row.AmountCents, row.PayoutCents, row.RebateCents, 0, row.AgentShareCents)
		items = append(items, OperatingBetRecord{ID: row.ID, RoomScope: row.RoomScope, GameID: row.GameID, GameName: row.GameName,
			Issue: row.Issue, UserID: row.UserID, Username: row.Username, PlayName: row.PlayName, Selection: row.Selection,
			Status: row.Status, Stake: centsToAmount(row.AmountCents), Payout: centsToAmount(row.PayoutCents),
			MemberNet: centsToAmount(row.PayoutCents - row.AmountCents), GrossProfit: centsToAmount(grossCents),
			RebateRate: row.RebateRateSnapshot, Rebate: centsToAmount(row.RebateCents), AgentShareRate: row.AgentShareRateSnapshot,
			AgentShare: centsToAmount(row.AgentShareCents), PlatformProfit: centsToAmount(platformCents),
			FlyAmount: centsToAmount(row.FlyCents), SettledAt: row.SettledAt})
	}
	return &OperatingReport{Summary: summary, Trend: trend, Breakdown: breakdown, Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (s *OperatingReportService) filteredBets(filter OperatingReportFilter, period reportPeriod, settled bool) *gorm.DB {
	q := s.db.Table("lottery_bets b").Where(`NOT EXISTS (SELECT 1 FROM "user" activity_account WHERE activity_account.user_id=b.user_id AND activity_account.remark=?)`, roomActivityRemark)
	if settled {
		q = q.Where("b.status IN ?", []string{"won", "lost"}).Where("COALESCE(b.settled_at,b.updated_at,b.created_at) >= ? AND COALESCE(b.settled_at,b.updated_at,b.created_at) < ?", period.Start, period.End)
	} else {
		q = q.Where("b.created_at >= ? AND b.created_at < ?", period.Start, period.End)
	}
	if filter.RoomScope != "" {
		q = q.Where("b.room_scope = ?", filter.RoomScope)
	}
	if filter.GameID != "" {
		q = q.Where("b.game_id = ?", filter.GameID)
	}
	if filter.UserID > 0 {
		q = q.Where("b.user_id = ?", filter.UserID)
	}
	if keyword := strings.TrimSpace(filter.Query); keyword != "" {
		like := "%" + strings.ToLower(keyword) + "%"
		q = q.Where("(LOWER(b.username) LIKE ? OR LOWER(b.issue) LIKE ? OR LOWER(b.play_name) LIKE ? OR LOWER(b.selection) LIKE ?)", like, like, like, like)
	}
	return q
}

func (s *OperatingReportService) welfareCost(filter OperatingReportFilter, period reportPeriod) (int64, error) {
	// Welfare has no game attribution.  Hide it for a single-game drilldown so
	// the game P&L is not charged with an arbitrary platform promotion.
	if filter.GameID != "" || strings.TrimSpace(filter.Query) != "" {
		return 0, nil
	}
	q := s.db.Table("user_balance_transactions t").Joins(`JOIN "user" u ON u.user_id=t.user_id`).
		Where("t.created_at >= ? AND t.created_at < ?", period.Start, period.End).
		Where("t.type IN ? AND t.amount_cents > 0", []string{"checkin", "redpacket", "invite"}).Where("u.remark IS NULL OR u.remark <> ?", roomActivityRemark)
	if filter.RoomScope == "lobby" {
		q = q.Where("u.parent_agent_id IS NULL AND u.role <> ?", "agent")
	}
	if strings.HasPrefix(filter.RoomScope, "agent:") {
		q = q.Where("u.parent_agent_id = ?", strings.TrimPrefix(filter.RoomScope, "agent:"))
	}
	if filter.UserID > 0 {
		q = q.Where("t.user_id = ?", filter.UserID)
	}
	var cents int64
	if err := q.Select("COALESCE(SUM(t.amount_cents),0)").Scan(&cents).Error; err != nil {
		return 0, err
	}
	return cents, nil
}

func (s *OperatingReportService) welfareQuery(filter OperatingReportFilter, period reportPeriod) *gorm.DB {
	q := s.db.Table("user_balance_transactions t").Joins(`JOIN "user" u ON u.user_id=t.user_id`).
		Where("t.created_at >= ? AND t.created_at < ?", period.Start, period.End).
		Where("t.type IN ? AND t.amount_cents > 0", []string{"checkin", "redpacket", "invite"}).
		Where("u.remark IS NULL OR u.remark <> ?", roomActivityRemark)
	if filter.RoomScope == "lobby" {
		q = q.Where("u.parent_agent_id IS NULL AND u.role <> ?", "agent")
	}
	if strings.HasPrefix(filter.RoomScope, "agent:") {
		q = q.Where("u.parent_agent_id = ?", strings.TrimPrefix(filter.RoomScope, "agent:"))
	}
	if filter.UserID > 0 {
		q = q.Where("t.user_id = ?", filter.UserID)
	}
	return q
}

func (s *OperatingReportService) welfareByDimension(filter OperatingReportFilter, period reportPeriod) (map[string]int64, error) {
	result := map[string]int64{}
	// 福利没有彩种归属；关键字可能匹配期号或玩法，因此这两种钻取不强行摊销。
	if filter.Dimension == "game" || filter.GameID != "" || strings.TrimSpace(filter.Query) != "" {
		return result, nil
	}
	keyExpr := "CAST(t.user_id AS TEXT)"
	if filter.Dimension == "room" {
		keyExpr = "CASE WHEN u.parent_agent_id IS NULL OR u.role = 'agent' THEN 'lobby' ELSE 'agent:' || CAST(u.parent_agent_id AS TEXT) END"
	}
	var rows []struct {
		Key         string
		AmountCents int64
	}
	if err := s.welfareQuery(filter, period).Select(fmt.Sprintf("%s key, COALESCE(SUM(t.amount_cents),0) amount_cents", keyExpr)).Group(keyExpr).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.Key] = row.AmountCents
	}
	return result, nil
}

func (s *OperatingReportService) welfareByDate(filter OperatingReportFilter, period reportPeriod) (map[string]int64, error) {
	result := map[string]int64{}
	if filter.GameID != "" || strings.TrimSpace(filter.Query) != "" {
		return result, nil
	}
	dateExpr := "TO_CHAR(t.created_at AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD')"
	if s.db.Dialector.Name() == "sqlite" {
		dateExpr = "strftime('%Y-%m-%d',t.created_at)"
	}
	var rows []struct {
		Date        string
		AmountCents int64
	}
	if err := s.welfareQuery(filter, period).Select(fmt.Sprintf("%s date, COALESCE(SUM(t.amount_cents),0) amount_cents", dateExpr)).Group(dateExpr).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.Date] = row.AmountCents
	}
	return result, nil
}

type groupedOperatingRow struct {
	Key, Label                                               string
	TurnoverCents, PayoutCents, RebateCents, AgentShareCents int64
	Tickets                                                  int64
}

// operatingProfitCents is the accounting identity used by the summary,
// trend, breakdown and ticket drill-down. Keeping it in one place prevents
// the dashboard totals from drifting away from the underlying ticket ledger.
func operatingProfitCents(turnoverCents, payoutCents, rebateCents, welfareCents, agentShareCents int64) (grossCents, platformCents int64) {
	grossCents = turnoverCents - payoutCents
	platformCents = grossCents - rebateCents - welfareCents - agentShareCents
	return grossCents, platformCents
}

func (s *OperatingReportService) breakdown(filter OperatingReportFilter, period reportPeriod) ([]OperatingBreakdown, error) {
	q := s.filteredBets(filter, period, true)
	keyExpr, labelExpr, join := "b.room_scope", "b.room_scope", ""
	switch filter.Dimension {
	case "game":
		keyExpr, labelExpr, join = "b.game_id", "COALESCE(g.name,b.game_id)", "LEFT JOIN lottery_games g ON g.id=b.game_id"
	case "user":
		keyExpr, labelExpr = "CAST(b.user_id AS TEXT)", "MAX(b.username)"
	}
	if join != "" {
		q = q.Joins(join)
	}
	groupExpr := keyExpr
	// PostgreSQL does not allow the joined display label to be selected while
	// grouping by only b.game_id.  Group the exact label expression as well so
	// the game breakdown behaves the same in production and in lightweight
	// local tests.
	if filter.Dimension == "game" {
		groupExpr += ", " + labelExpr
	}
	var rows []groupedOperatingRow
	selectSQL := fmt.Sprintf(`%s key, %s label, COALESCE(SUM(b.amount_cents),0) turnover_cents,
		COALESCE(SUM(b.payout_cents),0) payout_cents, COALESCE(SUM(b.rebate_cents),0) rebate_cents,
		COALESCE(SUM(b.agent_share_cents),0) agent_share_cents, COUNT(*) tickets`, keyExpr, labelExpr)
	if err := q.Select(selectSQL).Group(groupExpr).Order("turnover_cents DESC").Limit(100).Scan(&rows).Error; err != nil {
		return nil, err
	}
	welfareByKey, err := s.welfareByDimension(filter, period)
	if err != nil {
		return nil, err
	}
	roomLabels := map[string]string{"lobby": "平台大厅"}
	if filter.Dimension == "room" {
		ids := make([]uint64, 0, len(rows))
		for _, row := range rows {
			if strings.HasPrefix(row.Key, "agent:") {
				if id, parseErr := strconv.ParseUint(strings.TrimPrefix(row.Key, "agent:"), 10, 64); parseErr == nil {
					ids = append(ids, id)
				}
			}
		}
		if len(ids) > 0 {
			var agents []user.User
			if err := s.db.Where("user_id IN ?", ids).Find(&agents).Error; err != nil {
				return nil, err
			}
			for _, agent := range agents {
				label := "房间 " + defaultString(agent.AgentRoomCode, strconv.FormatUint(agent.UserID, 10))
				if name := strings.TrimSpace(agent.Nickname); name != "" {
					label += " · " + name
				}
				roomLabels[fmt.Sprintf("agent:%d", agent.UserID)] = label
			}
		}
	}
	result := make([]OperatingBreakdown, 0, len(rows))
	for _, row := range rows {
		welfare := welfareByKey[row.Key]
		gross, platform := operatingProfitCents(row.TurnoverCents, row.PayoutCents, row.RebateCents, welfare, row.AgentShareCents)
		if label := roomLabels[row.Key]; label != "" {
			row.Label = label
		}
		result = append(result, OperatingBreakdown{Key: row.Key, Label: row.Label,
			Turnover: centsToAmount(row.TurnoverCents), Payout: centsToAmount(row.PayoutCents), GrossProfit: centsToAmount(gross),
			Rebate: centsToAmount(row.RebateCents), Welfare: centsToAmount(welfare), AgentShare: centsToAmount(row.AgentShareCents),
			PlatformProfit: centsToAmount(platform), Tickets: row.Tickets})
	}
	return result, nil
}

func (s *OperatingReportService) trend(filter OperatingReportFilter, period reportPeriod) ([]OperatingPoint, error) {
	dateExpr := "TO_CHAR(COALESCE(b.settled_at,b.updated_at,b.created_at) AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD')"
	if s.db.Dialector.Name() == "sqlite" {
		dateExpr = "strftime('%Y-%m-%d',COALESCE(b.settled_at,b.updated_at,b.created_at))"
	}
	var rows []struct {
		Date                                                     string
		TurnoverCents, PayoutCents, RebateCents, AgentShareCents int64
	}
	q := s.filteredBets(filter, period, true)
	if err := q.Select(fmt.Sprintf(`%s date, COALESCE(SUM(b.amount_cents),0) turnover_cents, COALESCE(SUM(b.payout_cents),0) payout_cents,
		COALESCE(SUM(b.rebate_cents),0) rebate_cents, COALESCE(SUM(b.agent_share_cents),0) agent_share_cents`, dateExpr)).Group(dateExpr).Order("date").Scan(&rows).Error; err != nil {
		return nil, err
	}
	byDate := map[string]OperatingPoint{}
	welfareByDate, err := s.welfareByDate(filter, period)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		welfare := welfareByDate[row.Date]
		gross, platform := operatingProfitCents(row.TurnoverCents, row.PayoutCents, row.RebateCents, welfare, row.AgentShareCents)
		byDate[row.Date] = OperatingPoint{Date: row.Date, Turnover: centsToAmount(row.TurnoverCents), Payout: centsToAmount(row.PayoutCents), GrossProfit: centsToAmount(gross), Rebate: centsToAmount(row.RebateCents), Welfare: centsToAmount(welfare), AgentShare: centsToAmount(row.AgentShareCents), PlatformProfit: centsToAmount(platform)}
	}
	result := make([]OperatingPoint, 0, 93)
	for day := period.Start; day.Before(period.End); day = day.AddDate(0, 0, 1) {
		date := day.In(period.Location).Format("2006-01-02")
		point := byDate[date]
		point.Date = date
		if _, exists := byDate[date]; !exists {
			point.Welfare = centsToAmount(welfareByDate[date])
			point.PlatformProfit = -point.Welfare
		}
		result = append(result, point)
	}
	return result, nil
}
