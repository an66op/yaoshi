package services

import (
	"backend/data/models/application"
	"backend/data/models/audit"
	"backend/data/models/bet"
	"backend/data/models/chat"
	"backend/data/models/lottery"
	"backend/data/models/rebate"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"
)

type ReportDefinition struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Group string `json:"group"`
}

var reportDefinitions = []ReportDefinition{
	{Key: "summary", Title: "总报表", Group: "经营分析"},
	{Key: "users", Title: "用户报表", Group: "经营分析"},
	{Key: "entertainment", Title: "娱乐报表", Group: "经营分析"},
	{Key: "28", Title: "28报表", Group: "经营分析"},
	{Key: "categories", Title: "分类报表", Group: "经营分析"},
	{Key: "unsettled", Title: "未结报表", Group: "经营分析"},
	{Key: "financial", Title: "财务报表", Group: "财务结算"},
	{Key: "commission", Title: "返佣报表", Group: "财务结算"},
	{Key: "redpackets", Title: "红包报表", Group: "财务结算"},
	{Key: "rebates", Title: "回水报表", Group: "财务结算"},
	{Key: "entertainment-rebates", Title: "娱乐回水", Group: "财务结算"},
	{Key: "28-rebates", Title: "28回水", Group: "财务结算"},
	{Key: "alerts", Title: "告警报表", Group: "风控会员"},
	{Key: "new-members", Title: "新会员统计", Group: "风控会员"},
	{Key: "daily-members", Title: "当日会员概要", Group: "风控会员"},
	{Key: "logs", Title: "日志报表", Group: "系统审计"},
}

type ReportCenterFilter struct {
	WorkspaceID uint64
	Query       string
	Start       string
	End         string
	GameID      string
	Category    string
	Issue       string
	Status      string
	Page        int
	PageSize    int
}

type ReportMetric struct {
	Key   string  `json:"key"`
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

type ReportColumn struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type ReportCenterResult struct {
	Key         string           `json:"key"`
	Title       string           `json:"title"`
	PeriodStart string           `json:"period_start"`
	PeriodEnd   string           `json:"period_end"`
	Metrics     []ReportMetric   `json:"metrics"`
	Columns     []ReportColumn   `json:"columns"`
	Items       []map[string]any `json:"items"`
	Total       int64            `json:"total"`
	Page        int              `json:"page"`
	PageSize    int              `json:"page_size"`
}

type ReportCenterService struct{ db *gorm.DB }

func NewReportCenterService(db *gorm.DB) *ReportCenterService { return &ReportCenterService{db: db} }

func ReportCatalog() []ReportDefinition {
	return append([]ReportDefinition(nil), reportDefinitions...)
}

func reportDefinition(key string) (ReportDefinition, bool) {
	for _, item := range reportDefinitions {
		if item.Key == key {
			return item, true
		}
	}
	return ReportDefinition{}, false
}

// newReportCenterResult is the single constructor for every report response.
// It deliberately allocates all collection fields: encoding a nil slice as
// JSON null breaks clients which render an otherwise valid empty report.
func newReportCenterResult(definition ReportDefinition, period reportPeriod, filter ReportCenterFilter) *ReportCenterResult {
	return &ReportCenterResult{
		Key: definition.Key, Title: definition.Title,
		PeriodStart: period.StartDate, PeriodEnd: period.EndDate,
		Metrics: []ReportMetric{}, Columns: []ReportColumn{}, Items: []map[string]any{},
		Total: 0, Page: filter.Page, PageSize: filter.PageSize,
	}
}

func (s *ReportCenterService) Report(key string, filter ReportCenterFilter) (*ReportCenterResult, error) {
	definition, ok := reportDefinition(strings.TrimSpace(key))
	if !ok {
		return nil, apperrors.NewBusinessError("REPORT_NOT_FOUND", "报表类型不存在")
	}
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
	result := newReportCenterResult(definition, period, filter)
	switch definition.Key {
	case "summary", "entertainment", "28":
		err = s.betOverview(result, filter, period, definition.Key)
	case "users":
		err = s.dailyMemberReport(result, filter, period)
	case "daily-members":
		err = s.dailyMemberReport(result, filter, period)
	case "categories":
		err = s.categoryReport(result, filter, period)
	case "unsettled":
		err = s.unsettledReport(result, filter, period)
	case "financial", "commission":
		err = s.ledgerReport(result, filter, period, definition.Key == "commission")
	case "redpackets":
		err = s.redPacketReport(result, filter, period)
	case "rebates":
		err = s.rebateReport(result, filter, period)
	case "entertainment-rebates", "28-rebates":
		err = s.betRebateReport(result, filter, period, definition.Key == "28-rebates")
	case "alerts":
		err = s.alertReport(result, filter, period)
	case "new-members":
		err = s.newMemberReport(result, filter, period)
	case "logs":
		err = s.logReport(result, filter, period)
	}
	return result, err
}

var pc28GameIDs = []string{"pc-canada", "canada-28", "canada-20"}

func (s *ReportCenterService) betBase(filter ReportCenterFilter, period reportPeriod) *gorm.DB {
	query := s.db.Model(&bet.Bet{}).
		Where("created_at >= ? AND created_at < ?", period.Start, period.End).
		Where(`NOT EXISTS (SELECT 1 FROM workspace_robot_profiles AS robot WHERE robot.user_id = lottery_bets.user_id AND robot.workspace_id = lottery_bets.workspace_id)`)
	if filter.WorkspaceID > 0 {
		query = query.Where("workspace_id = ?", filter.WorkspaceID)
	} else {
		query = query.Where("workspace_id > 0")
	}
	if filter.GameID != "" && filter.GameID != "all" {
		query = query.Where("game_id = ?", filter.GameID)
	}
	if category := strings.TrimSpace(filter.Category); category != "" && category != "all" {
		query = query.Where("game_id IN (?)", s.db.Model(&lottery.Game{}).Select("id").Where("lobby_category = ?", category))
	}
	if filter.Issue != "" {
		query = query.Where("issue = ?", filter.Issue)
	}
	if filter.Status != "" && filter.Status != "all" {
		query = query.Where("status = ?", filter.Status)
	}
	if text := strings.TrimSpace(filter.Query); text != "" {
		like := "%" + strings.ToLower(text) + "%"
		query = query.Where("LOWER(username) LIKE ? OR LOWER(issue) LIKE ? OR LOWER(play_name) LIKE ? OR LOWER(selection) LIKE ?", like, like, like, like)
	}
	return query
}

type reportBetAggregate struct {
	Turnover int64
	Payout   int64
	Rebate   int64
	Share    int64
	Tickets  int64
	Members  int64
}

func (s *ReportCenterService) betOverview(out *ReportCenterResult, filter ReportCenterFilter, period reportPeriod, reportKey string) error {
	query := s.betBase(filter, period)
	if reportKey == "28" {
		query = query.Where("game_id IN ?", pc28GameIDs)
	}
	var aggregate reportBetAggregate
	if err := query.Session(&gorm.Session{}).Select(`COALESCE(SUM(CASE WHEN status <> 'cancelled' THEN amount_cents ELSE 0 END),0) turnover,
		COALESCE(SUM(CASE WHEN status IN ('won','lost') THEN payout_cents ELSE 0 END),0) payout,
		COALESCE(SUM(rebate_cents),0) rebate, COALESCE(SUM(agent_share_cents),0) share,
		COUNT(*) tickets, COUNT(DISTINCT user_id) members`).Scan(&aggregate).Error; err != nil {
		return err
	}
	gross := aggregate.Turnover - aggregate.Payout
	out.Metrics = []ReportMetric{
		{Key: "turnover", Label: "有效流水", Value: centsToAmount(aggregate.Turnover)},
		{Key: "payout", Label: "派彩", Value: centsToAmount(aggregate.Payout)},
		{Key: "member_net", Label: "会员输赢", Value: centsToAmount(aggregate.Payout - aggregate.Turnover)},
		{Key: "gross", Label: "毛利", Value: centsToAmount(gross)},
		{Key: "rebate", Label: "回水", Value: centsToAmount(aggregate.Rebate)},
		{Key: "net", Label: "净利润", Value: centsToAmount(gross - aggregate.Rebate - aggregate.Share)},
	}
	if reportKey == "summary" {
		workspaceLedger := s.db.Model(&user.BalanceTransaction{}).Where("created_at >= ? AND created_at < ?", period.Start, period.End)
		workspaceLedger = workspaceLedger.Where(`NOT EXISTS (SELECT 1 FROM workspace_robot_profiles AS robot WHERE robot.user_id = user_balance_transactions.user_id AND robot.workspace_id = user_balance_transactions.workspace_id)`)
		workspacePackets := s.db.Model(&chat.RedPacket{}).Where("created_at >= ? AND created_at < ?", period.Start, period.End)
		if filter.WorkspaceID > 0 {
			workspaceLedger = workspaceLedger.Where("workspace_id = ?", filter.WorkspaceID)
			workspacePackets = workspacePackets.Where("workspace_id = ?", filter.WorkspaceID)
		} else {
			workspaceLedger = workspaceLedger.Where("workspace_id > 0")
			workspacePackets = workspacePackets.Where("workspace_id > 0")
		}
		var costs struct{ Activity, Commission int64 }
		if err := workspaceLedger.Select(`COALESCE(SUM(CASE WHEN type IN ('activity','checkin','promotion') AND amount_cents > 0 THEN amount_cents ELSE 0 END),0) activity,
			COALESCE(SUM(CASE WHEN type = 'invite' AND amount_cents > 0 THEN amount_cents ELSE 0 END),0) commission`).Scan(&costs).Error; err != nil {
			return err
		}
		var packets struct{ Total, Remaining, Refunded int64 }
		if err := workspacePackets.Select("COALESCE(SUM(total_cents),0) total, COALESCE(SUM(remaining_cents),0) remaining, COALESCE(SUM(refunded_cents),0) refunded").Scan(&packets).Error; err != nil {
			return err
		}
		redPacketCost := packets.Total - packets.Remaining - packets.Refunded
		net := gross - aggregate.Rebate - aggregate.Share - redPacketCost - costs.Activity - costs.Commission
		var unsettled int64
		if err := query.Session(&gorm.Session{}).Where("status IN ?", []string{"pending", "settling"}).Select("COALESCE(SUM(amount_cents),0)").Scan(&unsettled).Error; err != nil {
			return err
		}
		out.Metrics = []ReportMetric{
			{Key: "turnover", Label: "有效流水", Value: centsToAmount(aggregate.Turnover)}, {Key: "payout", Label: "派彩", Value: centsToAmount(aggregate.Payout)},
			{Key: "member_net", Label: "会员输赢", Value: centsToAmount(aggregate.Payout - aggregate.Turnover)}, {Key: "gross", Label: "毛利", Value: centsToAmount(gross)},
			{Key: "rebate", Label: "回水", Value: centsToAmount(aggregate.Rebate)}, {Key: "redpacket", Label: "红包", Value: centsToAmount(redPacketCost)},
			{Key: "activity", Label: "活动成本", Value: centsToAmount(costs.Activity)}, {Key: "commission", Label: "会员返佣", Value: centsToAmount(costs.Commission)},
			{Key: "share", Label: "渠道分账", Value: centsToAmount(aggregate.Share)}, {Key: "net", Label: "净利润", Value: centsToAmount(net)},
			{Key: "unsettled", Label: "未结金额", Value: centsToAmount(unsettled)},
		}
	}
	return s.betRows(out, query)
}

func (s *ReportCenterService) betRows(out *ReportCenterResult, query *gorm.DB) error {
	if err := query.Session(&gorm.Session{}).Count(&out.Total).Error; err != nil {
		return err
	}
	var rows []bet.Bet
	if err := query.Session(&gorm.Session{}).Order("created_at DESC, id DESC").Offset((out.Page - 1) * out.PageSize).Limit(out.PageSize).Find(&rows).Error; err != nil {
		return err
	}
	out.Columns = []ReportColumn{{Key: "id", Label: "注单"}, {Key: "username", Label: "会员"}, {Key: "game_id", Label: "彩种"}, {Key: "issue", Label: "期号"}, {Key: "stake", Label: "投注"}, {Key: "payout", Label: "派彩"}, {Key: "status", Label: "状态"}, {Key: "created_at", Label: "时间"}}
	for _, row := range rows {
		out.Items = append(out.Items, map[string]any{"id": row.ID, "username": row.Username, "game_id": row.GameID, "issue": row.Issue, "stake": centsToAmount(row.AmountCents), "payout": centsToAmount(row.PayoutCents), "status": row.Status, "created_at": row.CreatedAt})
	}
	return nil
}

func (s *ReportCenterService) userReport(out *ReportCenterResult, filter ReportCenterFilter, period reportPeriod, daily bool) error {
	query := s.betBase(filter, period)
	type row struct {
		UserID   uint64
		Username string
		Stake    int64
		Payout   int64
		Rebate   int64
		Tickets  int64
	}
	grouped := query.Session(&gorm.Session{}).Select("user_id, username, COALESCE(SUM(amount_cents),0) stake, COALESCE(SUM(payout_cents),0) payout, COALESCE(SUM(rebate_cents),0) rebate, COUNT(*) tickets").Group("user_id, username")
	if err := s.db.Table("(?) AS grouped", grouped).Count(&out.Total).Error; err != nil {
		return err
	}
	var rows []row
	if err := grouped.Session(&gorm.Session{}).Order("stake DESC").Offset((out.Page - 1) * out.PageSize).Limit(out.PageSize).Scan(&rows).Error; err != nil {
		return err
	}
	out.Columns = []ReportColumn{{Key: "user_id", Label: "会员ID"}, {Key: "username", Label: "会员"}, {Key: "stake", Label: "投注"}, {Key: "payout", Label: "派彩"}, {Key: "net", Label: "输赢"}, {Key: "rebate", Label: "回水"}, {Key: "tickets", Label: "注数"}}
	for _, row := range rows {
		out.Items = append(out.Items, map[string]any{"user_id": row.UserID, "username": row.Username, "stake": centsToAmount(row.Stake), "payout": centsToAmount(row.Payout), "net": centsToAmount(row.Payout - row.Stake), "rebate": centsToAmount(row.Rebate), "tickets": row.Tickets})
	}
	label := "活跃会员"
	if daily {
		label = "当日会员"
	}
	out.Metrics = []ReportMetric{{Key: "members", Label: label, Value: float64(out.Total)}}
	return nil
}

type dailyMemberAggregate struct {
	UserID     uint64
	Username   string
	Opening    int64
	Credit     int64
	Debit      int64
	Stake      int64
	Payout     int64
	Rebate     int64
	RedPacket  int64
	Commission int64
	Closing    int64
	Tickets    int64
	hasOpening bool
	hasClosing bool
}

// dailyMemberReport joins the immutable room snapshots from bets, ledgers and
// packet claims. A member who later moves rooms therefore remains visible in
// the original room's historical report.
func (s *ReportCenterService) dailyMemberReport(out *ReportCenterResult, filter ReportCenterFilter, period reportPeriod) error {
	items := map[uint64]*dailyMemberAggregate{}
	member := func(id uint64, username string) *dailyMemberAggregate {
		row, ok := items[id]
		if !ok {
			row = &dailyMemberAggregate{UserID: id, Username: username}
			items[id] = row
		} else if row.Username == "" && username != "" {
			row.Username = username
		}
		return row
	}

	type betRow struct {
		UserID   uint64
		Username string
		Stake    int64
		Payout   int64
		Rebate   int64
		Tickets  int64
	}
	var bets []betRow
	if err := s.betBase(filter, period).
		Select("user_id, username, COALESCE(SUM(CASE WHEN status <> 'cancelled' THEN amount_cents ELSE 0 END),0) stake, COALESCE(SUM(CASE WHEN status IN ('won','lost') THEN payout_cents ELSE 0 END),0) payout, COALESCE(SUM(rebate_cents),0) rebate, COUNT(*) tickets").
		Group("user_id, username").Scan(&bets).Error; err != nil {
		return err
	}
	for _, source := range bets {
		row := member(source.UserID, source.Username)
		row.Stake += source.Stake
		row.Payout += source.Payout
		row.Rebate += source.Rebate
		row.Tickets += source.Tickets
	}

	ledgerQuery := s.db.Model(&user.BalanceTransaction{}).Where("created_at < ?", period.End)
	ledgerQuery = ledgerQuery.Where(`NOT EXISTS (SELECT 1 FROM workspace_robot_profiles AS robot WHERE robot.user_id = user_balance_transactions.user_id AND robot.workspace_id = user_balance_transactions.workspace_id)`)
	if filter.WorkspaceID > 0 {
		ledgerQuery = ledgerQuery.Where("workspace_id = ?", filter.WorkspaceID)
	} else {
		ledgerQuery = ledgerQuery.Where("workspace_id > 0")
	}
	var ledgers []user.BalanceTransaction
	if err := ledgerQuery.Order("user_id ASC, created_at ASC, id ASC").Find(&ledgers).Error; err != nil {
		return err
	}
	for _, source := range ledgers {
		row := member(source.UserID, "")
		if source.CreatedAt.Before(period.Start) {
			row.Opening = source.AfterCents
			row.Closing = source.AfterCents
			row.hasOpening, row.hasClosing = true, true
			continue
		}
		if !row.hasOpening {
			row.Opening = source.BeforeCents
			row.hasOpening = true
		}
		row.Closing, row.hasClosing = source.AfterCents, true
		switch source.Type {
		case "application_credit":
			if source.AmountCents > 0 {
				row.Credit += source.AmountCents
			}
		case "application_debit":
			if source.AmountCents < 0 {
				row.Debit -= source.AmountCents
			}
		case "invite":
			if source.AmountCents > 0 {
				row.Commission += source.AmountCents
			}
		}
	}

	type packetRow struct {
		UserID uint64
		Amount int64
	}
	packetQuery := s.db.Model(&chat.RedPacketClaim{}).Where("created_at >= ? AND created_at < ?", period.Start, period.End)
	if filter.WorkspaceID > 0 {
		packetQuery = packetQuery.Where("workspace_id = ?", filter.WorkspaceID)
	} else {
		packetQuery = packetQuery.Where("workspace_id > 0")
	}
	var packetRows []packetRow
	if err := packetQuery.Select("user_id, COALESCE(SUM(amount_cents),0) amount").Group("user_id").Scan(&packetRows).Error; err != nil {
		return err
	}
	for _, source := range packetRows {
		member(source.UserID, "").RedPacket += source.Amount
	}

	ids := make([]uint64, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	if len(ids) > 0 {
		var accounts []user.User
		if err := s.db.Select("user_id", "username").Where("user_id IN ?", ids).Find(&accounts).Error; err != nil {
			return err
		}
		for _, account := range accounts {
			member(account.UserID, account.Username)
		}
	}

	rows := make([]*dailyMemberAggregate, 0, len(items))
	queryText := strings.ToLower(strings.TrimSpace(filter.Query))
	for _, row := range items {
		if queryText != "" && !strings.Contains(strings.ToLower(row.Username), queryText) && !strings.Contains(strconv.FormatUint(row.UserID, 10), queryText) {
			continue
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Stake == rows[j].Stake {
			return rows[i].UserID < rows[j].UserID
		}
		return rows[i].Stake > rows[j].Stake
	})
	out.Total = int64(len(rows))
	start := (out.Page - 1) * out.PageSize
	if start > len(rows) {
		start = len(rows)
	}
	end := start + out.PageSize
	if end > len(rows) {
		end = len(rows)
	}
	out.Columns = []ReportColumn{{Key: "user_id", Label: "会员ID"}, {Key: "username", Label: "会员"}, {Key: "opening", Label: "期初余额"}, {Key: "credit", Label: "上分"}, {Key: "debit", Label: "下分"}, {Key: "stake", Label: "投注"}, {Key: "payout", Label: "派彩"}, {Key: "net", Label: "输赢"}, {Key: "rebate", Label: "回水"}, {Key: "redpacket", Label: "红包"}, {Key: "commission", Label: "返佣"}, {Key: "closing", Label: "期末余额"}}
	for _, row := range rows[start:end] {
		out.Items = append(out.Items, map[string]any{"user_id": row.UserID, "username": row.Username, "opening": centsToAmount(row.Opening), "credit": centsToAmount(row.Credit), "debit": centsToAmount(row.Debit), "stake": centsToAmount(row.Stake), "payout": centsToAmount(row.Payout), "net": centsToAmount(row.Payout - row.Stake), "rebate": centsToAmount(row.Rebate), "redpacket": centsToAmount(row.RedPacket), "commission": centsToAmount(row.Commission), "closing": centsToAmount(row.Closing)})
	}
	out.Metrics = []ReportMetric{{Key: "members", Label: "当日会员", Value: float64(out.Total)}}
	return nil
}

func (s *ReportCenterService) categoryReport(out *ReportCenterResult, filter ReportCenterFilter, period reportPeriod) error {
	base := s.db.Table("lottery_bets AS b").
		Joins("LEFT JOIN lottery_games AS g ON g.id = b.game_id").
		Where("b.created_at >= ? AND b.created_at < ?", period.Start, period.End).
		Where(`NOT EXISTS (SELECT 1 FROM workspace_robot_profiles AS robot WHERE robot.user_id = b.user_id AND robot.workspace_id = b.workspace_id)`)
	if filter.WorkspaceID > 0 {
		base = base.Where("b.workspace_id = ?", filter.WorkspaceID)
	} else {
		base = base.Where("b.workspace_id > 0")
	}
	if filter.GameID != "" && filter.GameID != "all" {
		base = base.Where("b.game_id = ?", filter.GameID)
	}
	if filter.Issue != "" {
		base = base.Where("b.issue = ?", filter.Issue)
	}
	if filter.Status != "" && filter.Status != "all" {
		base = base.Where("b.status = ?", filter.Status)
	}
	if text := strings.TrimSpace(filter.Query); text != "" {
		like := "%" + strings.ToLower(text) + "%"
		base = base.Where("LOWER(b.username) LIKE ? OR LOWER(b.issue) LIKE ? OR LOWER(b.play_name) LIKE ? OR LOWER(b.selection) LIKE ?", like, like, like, like)
	}
	if filter.Category != "" && filter.Category != "all" {
		base = base.Where("g.lobby_category = ?", filter.Category)
	}
	type row struct {
		Category string
		Stake    int64
		Payout   int64
		Tickets  int64
	}
	const categoryExpression = "COALESCE(NULLIF(g.lobby_category,''),'未分类')"
	grouped := base.Session(&gorm.Session{}).Select(categoryExpression + " category, COALESCE(SUM(b.amount_cents),0) stake, COALESCE(SUM(b.payout_cents),0) payout, COUNT(*) tickets").Group(categoryExpression)
	if err := s.db.Table("(?) AS grouped", grouped).Count(&out.Total).Error; err != nil {
		return err
	}
	var rows []row
	if err := grouped.Session(&gorm.Session{}).Order("stake DESC").Offset((out.Page - 1) * out.PageSize).Limit(out.PageSize).Scan(&rows).Error; err != nil {
		return err
	}
	out.Columns = []ReportColumn{{Key: "category", Label: "分类"}, {Key: "stake", Label: "流水"}, {Key: "payout", Label: "派彩"}, {Key: "gross", Label: "毛利"}, {Key: "tickets", Label: "注数"}}
	for _, row := range rows {
		out.Items = append(out.Items, map[string]any{"category": row.Category, "stake": centsToAmount(row.Stake), "payout": centsToAmount(row.Payout), "gross": centsToAmount(row.Stake - row.Payout), "tickets": row.Tickets})
	}
	return nil
}

func (s *ReportCenterService) unsettledReport(out *ReportCenterResult, filter ReportCenterFilter, period reportPeriod) error {
	query := s.betBase(filter, period).Where("status IN ? OR reconciliation_status <> ?", []string{"pending", "settling"}, "normal")
	var cents int64
	_ = query.Session(&gorm.Session{}).Select("COALESCE(SUM(amount_cents),0)").Scan(&cents).Error
	out.Metrics = []ReportMetric{{Key: "unsettled", Label: "未结金额", Value: centsToAmount(cents)}}
	return s.betRows(out, query)
}

func (s *ReportCenterService) ledgerReport(out *ReportCenterResult, filter ReportCenterFilter, period reportPeriod, commissionOnly bool) error {
	query := s.db.Model(&user.BalanceTransaction{}).Where("created_at >= ? AND created_at < ?", period.Start, period.End)
	query = query.Where(`NOT EXISTS (SELECT 1 FROM workspace_robot_profiles AS robot WHERE robot.user_id = user_balance_transactions.user_id AND robot.workspace_id = user_balance_transactions.workspace_id)`)
	if filter.WorkspaceID > 0 {
		query = query.Where("workspace_id = ?", filter.WorkspaceID)
	} else {
		query = query.Where("workspace_id > 0")
	}
	if commissionOnly {
		query = query.Where("type = ?", "invite")
	}
	if filter.Status != "" && filter.Status != "all" && !commissionOnly {
		query = query.Where("type = ?", filter.Status)
	}
	if text := strings.TrimSpace(filter.Query); text != "" {
		like := "%" + strings.ToLower(text) + "%"
		query = query.Where("LOWER(remark) LIKE ? OR LOWER(operator) LIKE ?", like, like)
	}
	var totals struct{ Credit, Debit int64 }
	if err := query.Session(&gorm.Session{}).Select("COALESCE(SUM(CASE WHEN amount_cents > 0 THEN amount_cents ELSE 0 END),0) credit, COALESCE(SUM(CASE WHEN amount_cents < 0 THEN -amount_cents ELSE 0 END),0) debit").Scan(&totals).Error; err != nil {
		return err
	}
	out.Metrics = []ReportMetric{{Key: "credit", Label: "收入", Value: centsToAmount(totals.Credit)}, {Key: "debit", Label: "支出", Value: centsToAmount(totals.Debit)}, {Key: "net", Label: "净变动", Value: centsToAmount(totals.Credit - totals.Debit)}}
	if err := query.Session(&gorm.Session{}).Count(&out.Total).Error; err != nil {
		return err
	}
	var rows []user.BalanceTransaction
	if err := query.Session(&gorm.Session{}).Order("created_at DESC, id DESC").Offset((out.Page - 1) * out.PageSize).Limit(out.PageSize).Find(&rows).Error; err != nil {
		return err
	}
	out.Columns = []ReportColumn{{Key: "id", Label: "流水号"}, {Key: "user_id", Label: "会员ID"}, {Key: "type", Label: "类型"}, {Key: "amount", Label: "金额"}, {Key: "remark", Label: "备注"}, {Key: "operator", Label: "操作人"}, {Key: "created_at", Label: "时间"}}
	for _, row := range rows {
		out.Items = append(out.Items, map[string]any{"id": row.ID, "user_id": row.UserID, "type": row.Type, "amount": centsToAmount(row.AmountCents), "remark": row.Remark, "operator": row.Operator, "created_at": row.CreatedAt})
	}
	return nil
}

func (s *ReportCenterService) redPacketReport(out *ReportCenterResult, filter ReportCenterFilter, period reportPeriod) error {
	query := s.db.Model(&chat.RedPacket{}).Where("created_at >= ? AND created_at < ?", period.Start, period.End)
	if filter.WorkspaceID > 0 {
		query = query.Where("workspace_id = ?", filter.WorkspaceID)
	} else {
		query = query.Where("workspace_id > 0")
	}
	var totals struct{ Total, Remaining, Refunded int64 }
	if err := query.Session(&gorm.Session{}).Select("COALESCE(SUM(total_cents),0) total, COALESCE(SUM(remaining_cents),0) remaining, COALESCE(SUM(refunded_cents),0) refunded").Scan(&totals).Error; err != nil {
		return err
	}
	out.Metrics = []ReportMetric{{Key: "sent", Label: "发送金额", Value: centsToAmount(totals.Total)}, {Key: "claimed", Label: "领取金额", Value: centsToAmount(totals.Total - totals.Remaining - totals.Refunded)}, {Key: "refunded", Label: "退回金额", Value: centsToAmount(totals.Refunded)}, {Key: "remaining", Label: "剩余金额", Value: centsToAmount(totals.Remaining)}}
	if err := query.Session(&gorm.Session{}).Count(&out.Total).Error; err != nil {
		return err
	}
	var rows []chat.RedPacket
	if err := query.Session(&gorm.Session{}).Order("created_at DESC, id DESC").Offset((out.Page - 1) * out.PageSize).Limit(out.PageSize).Find(&rows).Error; err != nil {
		return err
	}
	out.Columns = []ReportColumn{{Key: "id", Label: "红包"}, {Key: "game_id", Label: "会话"}, {Key: "packet_count", Label: "红包个数"}, {Key: "claimed_count", Label: "领取人数"}, {Key: "total", Label: "总金额"}, {Key: "claimed", Label: "已领取"}, {Key: "refunded", Label: "已退回"}, {Key: "remaining", Label: "剩余"}, {Key: "threshold", Label: "流水门槛"}, {Key: "status", Label: "状态"}, {Key: "funding_status", Label: "资金状态"}, {Key: "created_at", Label: "时间"}}
	for _, row := range rows {
		out.Items = append(out.Items, map[string]any{"id": row.ID, "game_id": row.GameID, "packet_count": row.PacketCount, "claimed_count": row.ClaimedCount, "total": centsToAmount(row.TotalCents), "claimed": centsToAmount(row.TotalCents - row.RemainingCents - row.RefundedCents), "refunded": centsToAmount(row.RefundedCents), "remaining": centsToAmount(row.RemainingCents), "threshold": centsToAmount(row.MinDailyTurnoverCents), "status": row.Status, "funding_status": row.FundingStatus, "created_at": row.CreatedAt})
	}
	return nil
}

func (s *ReportCenterService) rebateReport(out *ReportCenterResult, filter ReportCenterFilter, period reportPeriod) error {
	query := s.db.Model(&rebate.DailyRecord{}).Where("created_at >= ? AND created_at < ?", period.Start, period.End)
	if filter.WorkspaceID > 0 {
		query = query.Where("workspace_id = ?", filter.WorkspaceID)
	} else {
		query = query.Where("workspace_id > 0")
	}
	var totals struct{ Turnover, Amount int64 }
	_ = query.Session(&gorm.Session{}).Select("COALESCE(SUM(turnover_cents),0) turnover, COALESCE(SUM(amount_cents),0) amount").Scan(&totals).Error
	out.Metrics = []ReportMetric{{Key: "turnover", Label: "计回水流水", Value: centsToAmount(totals.Turnover)}, {Key: "rebate", Label: "回水金额", Value: centsToAmount(totals.Amount)}}
	if err := query.Session(&gorm.Session{}).Count(&out.Total).Error; err != nil {
		return err
	}
	var rows []rebate.DailyRecord
	if err := query.Session(&gorm.Session{}).Order("created_at DESC, id DESC").Offset((out.Page - 1) * out.PageSize).Limit(out.PageSize).Find(&rows).Error; err != nil {
		return err
	}
	out.Columns = []ReportColumn{{Key: "biz_date", Label: "日期"}, {Key: "username", Label: "会员"}, {Key: "turnover", Label: "流水"}, {Key: "rate", Label: "比例"}, {Key: "amount", Label: "回水"}, {Key: "status", Label: "状态"}}
	for _, row := range rows {
		out.Items = append(out.Items, map[string]any{"biz_date": row.BizDate, "username": row.Username, "turnover": centsToAmount(row.TurnoverCents), "rate": row.RatePercent, "amount": centsToAmount(row.AmountCents), "status": row.Status})
	}
	return nil
}

func (s *ReportCenterService) betRebateReport(out *ReportCenterResult, filter ReportCenterFilter, period reportPeriod, only28 bool) error {
	query := s.betBase(filter, period).Where("rebate_cents > 0")
	if only28 {
		query = query.Where("game_id IN ?", pc28GameIDs)
	}
	var amount int64
	_ = query.Session(&gorm.Session{}).Select("COALESCE(SUM(rebate_cents),0)").Scan(&amount).Error
	out.Metrics = []ReportMetric{{Key: "rebate", Label: "回水金额", Value: centsToAmount(amount)}}
	return s.betRows(out, query)
}

func (s *ReportCenterService) alertReport(out *ReportCenterResult, filter ReportCenterFilter, period reportPeriod) error {
	var games []lottery.Game
	query := s.db.Model(&lottery.Game{}).Where("sync_status IN ? OR last_sync_error <> ''", []string{"error", "stale", "paused"})
	if filter.GameID != "" {
		query = query.Where("id = ?", filter.GameID)
	}
	if err := query.Order("updated_at DESC").Find(&games).Error; err != nil {
		return err
	}
	var issues []lottery.Issue
	issueQuery := s.db.Model(&lottery.Issue{}).Where("status = ? AND updated_at >= ? AND updated_at < ?", lottery.IssueStatusError, period.Start, period.End)
	if filter.GameID != "" {
		issueQuery = issueQuery.Where("game_id = ?", filter.GameID)
	}
	if err := issueQuery.Order("updated_at DESC").Find(&issues).Error; err != nil {
		return err
	}
	var robotSettings []workspacemodel.RobotSetting
	robotQuery := s.db.Where("last_error <> '' AND updated_at >= ? AND updated_at < ?", period.Start, period.End)
	if filter.WorkspaceID > 0 {
		robotQuery = robotQuery.Where("workspace_id = ?", filter.WorkspaceID)
	} else {
		robotQuery = robotQuery.Where("workspace_id > 0")
	}
	if err := robotQuery.Order("updated_at DESC").Find(&robotSettings).Error; err != nil {
		return err
	}
	var abnormalBets []bet.Bet
	if err := s.betBase(filter, period).Where("reconciliation_status <> ?", "normal").Order("updated_at DESC").Limit(100).Find(&abnormalBets).Error; err != nil {
		return err
	}
	out.Columns = []ReportColumn{{Key: "source", Label: "来源"}, {Key: "workspace_id", Label: "房间"}, {Key: "game_id", Label: "彩种"}, {Key: "issue", Label: "期号"}, {Key: "status", Label: "状态"}, {Key: "message", Label: "原因"}, {Key: "updated_at", Label: "时间"}}
	for _, row := range games {
		out.Items = append(out.Items, map[string]any{"source": "开奖源", "workspace_id": "共享", "game_id": row.ID, "issue": "", "status": row.SyncStatus, "message": row.LastSyncError, "updated_at": row.UpdatedAt})
	}
	for _, row := range issues {
		out.Items = append(out.Items, map[string]any{"source": "期号", "workspace_id": "共享", "game_id": row.GameID, "issue": row.Issue, "status": row.Status, "message": row.LastError, "updated_at": row.UpdatedAt})
	}
	for _, row := range robotSettings {
		out.Items = append(out.Items, map[string]any{"source": "机器人", "workspace_id": row.WorkspaceID, "game_id": "", "issue": "", "status": "error", "message": row.LastError, "updated_at": row.UpdatedAt})
	}
	for _, row := range abnormalBets {
		out.Items = append(out.Items, map[string]any{"source": "注单对账", "workspace_id": row.WorkspaceID, "game_id": row.GameID, "issue": row.Issue, "status": row.ReconciliationStatus, "message": defaultString(row.ReconciliationNote, "注单需要人工核对"), "updated_at": row.UpdatedAt})
	}
	out.Total = int64(len(out.Items))
	out.Metrics = []ReportMetric{{Key: "alerts", Label: "异常数量", Value: float64(out.Total)}}
	return nil
}

func (s *ReportCenterService) newMemberReport(out *ReportCenterResult, filter ReportCenterFilter, period reportPeriod) error {
	query := s.db.Model(&user.User{}).Where("role = ? AND created_at >= ? AND created_at < ?", "member", period.Start, period.End)
	query = query.Where(`NOT EXISTS (SELECT 1 FROM workspace_robot_profiles AS robot WHERE robot.user_id = "user".user_id)`)
	if filter.WorkspaceID > 0 {
		query = query.Where("workspace_id = ?", filter.WorkspaceID)
	} else {
		query = query.Where("workspace_id > 0")
	}
	if text := strings.TrimSpace(filter.Query); text != "" {
		like := "%" + strings.ToLower(text) + "%"
		query = query.Where("LOWER(username) LIKE ? OR LOWER(nickname) LIKE ?", like, like)
	}
	if err := query.Session(&gorm.Session{}).Count(&out.Total).Error; err != nil {
		return err
	}
	var rows []user.User
	if err := query.Session(&gorm.Session{}).Order("created_at DESC, user_id DESC").Offset((out.Page - 1) * out.PageSize).Limit(out.PageSize).Find(&rows).Error; err != nil {
		return err
	}
	applicationQuery := s.db.Model(&application.Application{}).Where("created_at >= ? AND created_at < ?", period.Start, period.End)
	if filter.WorkspaceID > 0 {
		applicationQuery = applicationQuery.Where("workspace_id = ?", filter.WorkspaceID)
	} else {
		applicationQuery = applicationQuery.Where("workspace_id > 0")
	}
	var joinSubmitted, joinApproved int64
	if err := applicationQuery.Session(&gorm.Session{}).Where("request_type = ?", "join").Distinct("user_id").Count(&joinSubmitted).Error; err != nil {
		return err
	}
	if err := applicationQuery.Session(&gorm.Session{}).Where("request_type = ? AND status = ?", "join", "approved").Distinct("user_id").Count(&joinApproved).Error; err != nil {
		return err
	}
	firstCreditQuery := s.db.Model(&user.BalanceTransaction{}).Select("user_id, MIN(created_at) first_at").Where("type = ?", "application_credit")
	firstBetQuery := s.db.Model(&bet.Bet{}).Select("user_id, MIN(created_at) first_at").
		Where("status <> ?", "cancelled").
		Where("NOT EXISTS (SELECT 1 FROM workspace_robot_profiles AS robot WHERE robot.user_id = lottery_bets.user_id)")
	if filter.WorkspaceID > 0 {
		firstCreditQuery = firstCreditQuery.Where("workspace_id = ?", filter.WorkspaceID)
		firstBetQuery = firstBetQuery.Where("workspace_id = ?", filter.WorkspaceID)
	} else {
		firstCreditQuery = firstCreditQuery.Where("workspace_id > 0")
		firstBetQuery = firstBetQuery.Where("workspace_id > 0")
	}
	var firstCredits, firstBets int64
	if err := s.db.Table("(?) AS first_credit", firstCreditQuery.Group("user_id")).Where("first_at >= ? AND first_at < ?", period.Start, period.End).Count(&firstCredits).Error; err != nil {
		return err
	}
	if err := s.db.Table("(?) AS first_bet", firstBetQuery.Group("user_id")).Where("first_at >= ? AND first_at < ?", period.Start, period.End).Count(&firstBets).Error; err != nil {
		return err
	}
	cohortQuery := s.db.Model(&user.User{}).
		Select("user_id, workspace_id, created_at").
		Where("role = ? AND created_at >= ? AND created_at < ?", "member", period.Start, period.End).
		Where(`NOT EXISTS (SELECT 1 FROM workspace_robot_profiles AS robot WHERE robot.user_id = "user".user_id)`)
	if filter.WorkspaceID > 0 {
		cohortQuery = cohortQuery.Where("workspace_id = ?", filter.WorkspaceID)
	} else {
		cohortQuery = cohortQuery.Where("workspace_id > 0")
	}
	var retained int64
	if err := s.db.Table("(?) AS cohort", cohortQuery).
		Where(`EXISTS (
			SELECT 1 FROM lottery_bets AS retained_bet
			WHERE retained_bet.user_id = cohort.user_id
			  AND retained_bet.workspace_id = cohort.workspace_id
			  AND retained_bet.status <> 'cancelled'
			  AND timezone('Asia/Shanghai', retained_bet.created_at)::date > timezone('Asia/Shanghai', cohort.created_at)::date
		)`).Count(&retained).Error; err != nil {
		return err
	}
	out.Metrics = []ReportMetric{
		{Key: "registered", Label: "注册会员", Value: float64(out.Total)},
		{Key: "join_submitted", Label: "申请入房", Value: float64(joinSubmitted)},
		{Key: "join_approved", Label: "审核通过", Value: float64(joinApproved)},
		{Key: "first_credit", Label: "首次上分", Value: float64(firstCredits)},
		{Key: "first_bet", Label: "首次投注", Value: float64(firstBets)},
		{Key: "retained", Label: "次日后留存", Value: float64(retained)},
	}
	out.Columns = []ReportColumn{{Key: "user_id", Label: "会员ID"}, {Key: "username", Label: "账号"}, {Key: "nickname", Label: "昵称"}, {Key: "balance", Label: "余额"}, {Key: "status", Label: "状态"}, {Key: "created_at", Label: "注册时间"}}
	for _, row := range rows {
		out.Items = append(out.Items, map[string]any{"user_id": row.UserID, "username": row.Username, "nickname": row.Nickname, "balance": centsToAmount(row.BalanceCents), "status": row.Status, "created_at": row.CreatedAt})
	}
	return nil
}

func (s *ReportCenterService) logReport(out *ReportCenterResult, filter ReportCenterFilter, period reportPeriod) error {
	query := s.db.Model(&audit.Log{}).Where("created_at >= ? AND created_at < ?", period.Start, period.End)
	if filter.WorkspaceID > 0 {
		query = query.Where("workspace_id = ?", filter.WorkspaceID)
	} else {
		query = query.Where("workspace_id > 0")
	}
	if text := strings.TrimSpace(filter.Query); text != "" {
		like := "%" + strings.ToLower(text) + "%"
		query = query.Where("LOWER(actor_name) LIKE ? OR LOWER(path) LIKE ? OR LOWER(request_id) LIKE ?", like, like, like)
	}
	if err := query.Session(&gorm.Session{}).Count(&out.Total).Error; err != nil {
		return err
	}
	var rows []audit.Log
	if err := query.Session(&gorm.Session{}).Order("created_at DESC, id DESC").Offset((out.Page - 1) * out.PageSize).Limit(out.PageSize).Find(&rows).Error; err != nil {
		return err
	}
	out.Columns = []ReportColumn{{Key: "id", Label: "日志"}, {Key: "actor", Label: "操作人"}, {Key: "role", Label: "角色"}, {Key: "method", Label: "方法"}, {Key: "path", Label: "路径"}, {Key: "status", Label: "结果"}, {Key: "request_id", Label: "请求ID"}, {Key: "created_at", Label: "时间"}}
	for _, row := range rows {
		out.Items = append(out.Items, map[string]any{"id": row.ID, "actor": row.ActorName, "role": row.ActorRole, "method": row.Method, "path": row.Path, "status": row.StatusCode, "request_id": row.RequestID, "created_at": row.CreatedAt})
	}
	return nil
}

func WriteReportCSV(writer io.Writer, result *ReportCenterResult) error {
	csvWriter := csv.NewWriter(writer)
	headers := make([]string, 0, len(result.Columns))
	for _, column := range result.Columns {
		headers = append(headers, reportCSVValue(column.Label))
	}
	if err := csvWriter.Write(headers); err != nil {
		return err
	}
	for _, item := range result.Items {
		values := make([]string, 0, len(result.Columns))
		for _, column := range result.Columns {
			values = append(values, reportCSVValue(item[column.Key]))
		}
		if err := csvWriter.Write(values); err != nil {
			return err
		}
	}
	csvWriter.Flush()
	return csvWriter.Error()
}

// ExportReportCSV streams the complete filtered report in bounded pages. The
// ordinary report endpoint remains capped at 100 rows per request, while CSV
// export no longer silently truncates a room's ledger at the first page.
func (s *ReportCenterService) ExportReportCSV(writer io.Writer, key string, filter ReportCenterFilter) error {
	filter.Page = 1
	filter.PageSize = 100
	first, err := s.Report(key, filter)
	if err != nil {
		return err
	}
	csvWriter := csv.NewWriter(writer)
	headers := make([]string, 0, len(first.Columns))
	for _, column := range first.Columns {
		headers = append(headers, reportCSVValue(column.Label))
	}
	if err := csvWriter.Write(headers); err != nil {
		return err
	}
	writeRows := func(result *ReportCenterResult) error {
		for _, item := range result.Items {
			values := make([]string, 0, len(result.Columns))
			for _, column := range result.Columns {
				values = append(values, reportCSVValue(item[column.Key]))
			}
			if err := csvWriter.Write(values); err != nil {
				return err
			}
		}
		return nil
	}
	if err := writeRows(first); err != nil {
		return err
	}
	for page := 2; int64((page-1)*filter.PageSize) < first.Total; page++ {
		filter.Page = page
		result, err := s.Report(key, filter)
		if err != nil {
			return err
		}
		if err := writeRows(result); err != nil {
			return err
		}
	}
	csvWriter.Flush()
	return csvWriter.Error()
}

func reportCSVValue(value any) string {
	switch item := value.(type) {
	case nil:
		return ""
	case time.Time:
		return item.In(time.FixedZone("CST", 8*60*60)).Format("2006-01-02 15:04:05")
	case string:
		return neutralizeCSVFormula(item)
	case []byte:
		return neutralizeCSVFormula(string(item))
	case float64:
		return strconv.FormatFloat(item, 'f', 2, 64)
	case float32:
		return strconv.FormatFloat(float64(item), 'f', 2, 32)
	case int:
		return strconv.Itoa(item)
	case int8:
		return strconv.FormatInt(int64(item), 10)
	case int16:
		return strconv.FormatInt(int64(item), 10)
	case int32:
		return strconv.FormatInt(int64(item), 10)
	case int64:
		return strconv.FormatInt(item, 10)
	case uint:
		return strconv.FormatUint(uint64(item), 10)
	case uint8:
		return strconv.FormatUint(uint64(item), 10)
	case uint16:
		return strconv.FormatUint(uint64(item), 10)
	case uint32:
		return strconv.FormatUint(uint64(item), 10)
	case uint64:
		return strconv.FormatUint(item, 10)
	default:
		return neutralizeCSVFormula(fmt.Sprint(item))
	}
}

// neutralizeCSVFormula prevents spreadsheet applications from interpreting
// user-controlled text as a formula. Excel may ignore leading spaces, line
// breaks, tabs, control bytes and Unicode format characters before looking
// for a formula marker, so inspect the first effective rune rather than only
// value[0]. Prefixing an apostrophe at the very start keeps the original text
// intact while forcing spreadsheet text semantics.
func neutralizeCSVFormula(value string) string {
	for _, char := range value {
		if unicode.IsSpace(char) || unicode.IsControl(char) || unicode.In(char, unicode.Cf) {
			continue
		}
		switch char {
		case '=', '+', '-', '@':
			return "'" + value
		default:
			return value
		}
	}
	return value
}

func SortedReportGroups() []string {
	seen := map[string]struct{}{}
	groups := make([]string, 0, 4)
	for _, item := range reportDefinitions {
		if _, ok := seen[item.Group]; !ok {
			seen[item.Group] = struct{}{}
			groups = append(groups, item.Group)
		}
	}
	sort.Strings(groups)
	return groups
}
