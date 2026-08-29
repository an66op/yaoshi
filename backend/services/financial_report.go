package services

import (
	"backend/data/models/application"
	"backend/data/models/user"
	apperrors "backend/errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const reportTimezone = "Asia/Shanghai"

// FinancialReportService reads immutable balance ledger entries and turns them
// into the operational financial report shown in the admin console.
type FinancialReportService struct{ db *gorm.DB }

type FinancialReportFilter struct {
	Query, Type, Start, End string
	WorkspaceID             uint64
	Page, PageSize          int
}

type FinancialReportSummary struct {
	PeriodStart         string  `json:"period_start"`
	PeriodEnd           string  `json:"period_end"`
	TotalBalance        float64 `json:"total_balance"`
	CreditAmount        float64 `json:"credit_amount"`
	DebitAmount         float64 `json:"debit_amount"`
	NetChange           float64 `json:"net_change"`
	FinanceCredit       float64 `json:"finance_credit"`
	FinanceDebit        float64 `json:"finance_debit"`
	BettingCredit       float64 `json:"betting_credit"`
	BettingDebit        float64 `json:"betting_debit"`
	WelfareCredit       float64 `json:"welfare_credit"`
	AgentShareCredit    float64 `json:"agent_share_credit"`
	RecordCount         int64   `json:"record_count"`
	ActiveUsers         int64   `json:"active_users"`
	PendingApplications int64   `json:"pending_applications"`
}

type FinancialReportPoint struct {
	Date        string  `json:"date"`
	Credit      float64 `json:"credit"`
	Debit       float64 `json:"debit"`
	Net         float64 `json:"net"`
	RecordCount int64   `json:"record_count"`
}

type FinancialRecord struct {
	ID        uint64    `json:"id"`
	UserID    uint64    `json:"user_id"`
	Username  string    `json:"username"`
	Nickname  string    `json:"nickname"`
	Amount    float64   `json:"amount"`
	Before    float64   `json:"before"`
	After     float64   `json:"after"`
	Type      string    `json:"type"`
	Category  string    `json:"category"`
	Remark    string    `json:"remark"`
	Operator  string    `json:"operator"`
	CreatedAt time.Time `json:"created_at"`
}

type FinancialReport struct {
	Summary  FinancialReportSummary `json:"summary"`
	Trend    []FinancialReportPoint `json:"trend"`
	Items    []FinancialRecord      `json:"items"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
}

type reportPeriod struct {
	Start, End time.Time
	StartDate  string
	EndDate    string
	Location   *time.Location
}

type financialAggregate struct {
	CreditCents int64 `gorm:"column:credit_cents"`
	DebitCents  int64 `gorm:"column:debit_cents"`
	RecordCount int64 `gorm:"column:record_count"`
	ActiveUsers int64 `gorm:"column:active_users"`
}

type financialCategoryRow struct {
	Category    string `gorm:"column:category"`
	CreditCents int64  `gorm:"column:credit_cents"`
	DebitCents  int64  `gorm:"column:debit_cents"`
}

func categoryCaseSQL() string {
	return `CASE
		WHEN t.type IN ('manual','application_credit','application_debit','redpacket_reserve','redpacket_refund') THEN 'finance'
		WHEN t.type IN ('bet','bet_cancel','settlement','reconciliation_refund') THEN 'betting'
		WHEN t.type IN ('rebate','checkin','redpacket','invite') THEN 'welfare'
		WHEN t.type = 'agent_share' THEN 'share'
		ELSE 'other' END`
}

type financialTrendRow struct {
	Date        string `gorm:"column:date"`
	CreditCents int64  `gorm:"column:credit_cents"`
	DebitCents  int64  `gorm:"column:debit_cents"`
	RecordCount int64  `gorm:"column:record_count"`
}

type financialRecordRow struct {
	ID          uint64    `gorm:"column:id"`
	UserID      uint64    `gorm:"column:user_id"`
	Username    string    `gorm:"column:username"`
	Nickname    string    `gorm:"column:nickname"`
	AmountCents int64     `gorm:"column:amount_cents"`
	BeforeCents int64     `gorm:"column:before_cents"`
	AfterCents  int64     `gorm:"column:after_cents"`
	Type        string    `gorm:"column:type"`
	Remark      string    `gorm:"column:remark"`
	Operator    string    `gorm:"column:operator"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

func NewFinancialReportService(db *gorm.DB) *FinancialReportService {
	return &FinancialReportService{db: db}
}

func (s *FinancialReportService) Financial(filter FinancialReportFilter) (*FinancialReport, error) {
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
	if err := validateLedgerType(filter.Type); err != nil {
		return nil, err
	}

	ledger := s.filteredLedger(filter, period)
	var aggregate financialAggregate
	if err := ledger.Select(`
		COALESCE(SUM(CASE WHEN amount_cents > 0 THEN amount_cents ELSE 0 END), 0) AS credit_cents,
		COALESCE(SUM(CASE WHEN amount_cents < 0 THEN -amount_cents ELSE 0 END), 0) AS debit_cents,
		COUNT(*) AS record_count,
		COUNT(DISTINCT user_id) AS active_users`).Scan(&aggregate).Error; err != nil {
		return nil, err
	}

	var totalBalanceCents int64
	balanceQuery := excludeRobotProfileUsers(s.db.Model(&user.User{}))
	if filter.WorkspaceID > 0 {
		balanceQuery = balanceQuery.Where("workspace_id = ?", filter.WorkspaceID)
	}
	if err := balanceQuery.Select("COALESCE(SUM(balance_cents), 0)").Scan(&totalBalanceCents).Error; err != nil {
		return nil, err
	}
	var pendingApplications int64
	applicationQuery := s.db.Model(&application.Application{}).Where("status = ?", "pending")
	if filter.WorkspaceID > 0 {
		applicationQuery = applicationQuery.Where("workspace_id = ?", filter.WorkspaceID)
	}
	if err := applicationQuery.Count(&pendingApplications).Error; err != nil {
		return nil, err
	}

	trend, err := s.trend(filter, period)
	if err != nil {
		return nil, err
	}
	categoryTotals, err := s.categoryTotals(filter, period)
	if err != nil {
		return nil, err
	}
	var total int64
	if err := ledger.Count(&total).Error; err != nil {
		return nil, err
	}

	var rows []financialRecordRow
	if err := ledger.
		Select(`t.id, t.user_id, COALESCE(u.username, '已删除用户') AS username, COALESCE(u.nickname, '') AS nickname,
			t.amount_cents, t.before_cents, t.after_cents, t.type, t.remark, t.operator, t.created_at`).
		Joins(`LEFT JOIN "user" AS u ON u.user_id = t.user_id`).
		Order("t.created_at DESC, t.id DESC").
		Offset((filter.Page - 1) * filter.PageSize).
		Limit(filter.PageSize).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	items := make([]FinancialRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, FinancialRecord{
			ID: row.ID, UserID: row.UserID, Username: row.Username, Nickname: row.Nickname,
			Amount: centsToAmount(row.AmountCents), Before: centsToAmount(row.BeforeCents), After: centsToAmount(row.AfterCents),
			Type: row.Type, Category: ledgerCategory(row.Type), Remark: row.Remark, Operator: row.Operator, CreatedAt: row.CreatedAt,
		})
	}

	summary := FinancialReportSummary{
		PeriodStart: period.StartDate, PeriodEnd: period.EndDate, TotalBalance: centsToAmount(totalBalanceCents),
		CreditAmount: centsToAmount(aggregate.CreditCents), DebitAmount: centsToAmount(aggregate.DebitCents),
		NetChange: centsToAmount(aggregate.CreditCents - aggregate.DebitCents), RecordCount: aggregate.RecordCount,
		ActiveUsers: aggregate.ActiveUsers, PendingApplications: pendingApplications,
	}
	for _, row := range categoryTotals {
		switch row.Category {
		case "finance":
			summary.FinanceCredit = centsToAmount(row.CreditCents)
			summary.FinanceDebit = centsToAmount(row.DebitCents)
		case "betting":
			summary.BettingCredit = centsToAmount(row.CreditCents)
			summary.BettingDebit = centsToAmount(row.DebitCents)
		case "welfare":
			summary.WelfareCredit = centsToAmount(row.CreditCents)
		case "share":
			summary.AgentShareCredit = centsToAmount(row.CreditCents)
		}
	}

	return &FinancialReport{
		Summary: summary,
		Trend:   trend, Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize,
	}, nil
}

func (s *FinancialReportService) filteredLedger(filter FinancialReportFilter, period reportPeriod) *gorm.DB {
	query := excludeRobotProfileRows(
		s.db.Table("user_balance_transactions AS t").Where("t.created_at >= ? AND t.created_at < ?", period.Start, period.End),
		"t.workspace_id", "t.user_id",
	)
	if filter.WorkspaceID > 0 {
		query = query.Where("t.workspace_id = ?", filter.WorkspaceID)
	}
	switch strings.TrimSpace(filter.Type) {
	case "credit":
		query = query.Where("t.amount_cents > 0")
	case "debit":
		query = query.Where("t.amount_cents < 0")
	case "finance", "betting", "welfare", "share":
		types := ledgerTypesByCategory(filter.Type)
		query = query.Where("t.type IN ?", types)
	case "manual", "application_credit", "application_debit", "bet", "bet_cancel", "settlement", "reconciliation_refund", "rebate", "checkin", "redpacket", "invite", "agent_share":
		query = query.Where("t.type = ?", filter.Type)
	}
	if keyword := strings.TrimSpace(filter.Query); keyword != "" {
		like := "%" + strings.ToLower(keyword) + "%"
		query = query.Where(`(
			LOWER(COALESCE((SELECT username FROM "user" WHERE user_id = t.user_id), '')) LIKE ?
			OR LOWER(t.remark) LIKE ? OR LOWER(t.operator) LIKE ?
		)`, like, like, like)
	}
	return query
}

func ledgerTypesByCategory(category string) []string {
	switch category {
	case "finance":
		return []string{"manual", "application_credit", "application_debit"}
	case "betting":
		return []string{"bet", "bet_cancel", "settlement", "reconciliation_refund"}
	case "welfare":
		return []string{"rebate", "checkin", "redpacket", "invite"}
	case "share":
		return []string{"agent_share"}
	default:
		return []string{}
	}
}

func (s *FinancialReportService) categoryTotals(filter FinancialReportFilter, period reportPeriod) ([]financialCategoryRow, error) {
	query := s.filteredLedger(FinancialReportFilter{Query: filter.Query, Type: filter.Type, WorkspaceID: filter.WorkspaceID}, period)
	var rows []financialCategoryRow
	err := query.Select(fmt.Sprintf(`%s AS category,
		COALESCE(SUM(CASE WHEN t.amount_cents > 0 THEN t.amount_cents ELSE 0 END), 0) AS credit_cents,
		COALESCE(SUM(CASE WHEN t.amount_cents < 0 THEN -t.amount_cents ELSE 0 END), 0) AS debit_cents`, categoryCaseSQL())).
		Group("category").Scan(&rows).Error
	return rows, err
}

func (s *FinancialReportService) trend(filter FinancialReportFilter, period reportPeriod) ([]FinancialReportPoint, error) {
	query := s.filteredLedger(FinancialReportFilter{Query: filter.Query, Type: filter.Type, WorkspaceID: filter.WorkspaceID}, period)
	var rows []financialTrendRow
	if err := query.Select(`
		TO_CHAR(t.created_at AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD') AS date,
		COALESCE(SUM(CASE WHEN t.amount_cents > 0 THEN t.amount_cents ELSE 0 END), 0) AS credit_cents,
		COALESCE(SUM(CASE WHEN t.amount_cents < 0 THEN -t.amount_cents ELSE 0 END), 0) AS debit_cents,
		COUNT(*) AS record_count`).
		Group("TO_CHAR(t.created_at AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD')").
		Order("date ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	byDate := make(map[string]financialTrendRow, len(rows))
	for _, row := range rows {
		byDate[row.Date] = row
	}
	points := make([]FinancialReportPoint, 0, int(period.End.Sub(period.Start).Hours()/24))
	for day := period.Start; day.Before(period.End); day = day.AddDate(0, 0, 1) {
		date := day.In(period.Location).Format("2006-01-02")
		row := byDate[date]
		credit, debit := centsToAmount(row.CreditCents), centsToAmount(row.DebitCents)
		points = append(points, FinancialReportPoint{Date: date, Credit: credit, Debit: debit, Net: credit - debit, RecordCount: row.RecordCount})
	}
	return points, nil
}

func parseReportPeriod(startText, endText string) (reportPeriod, error) {
	location, err := time.LoadLocation(reportTimezone)
	if err != nil {
		location = time.Local
	}
	now := time.Now().In(location)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	start := today.AddDate(0, 0, -6)
	endExclusive := today.AddDate(0, 0, 1)

	if strings.TrimSpace(startText) != "" {
		start, err = time.ParseInLocation("2006-01-02", startText, location)
		if err != nil {
			return reportPeriod{}, apperrors.NewBusinessError("INVALID_REPORT_DATE", "开始日期格式不正确")
		}
	}
	if strings.TrimSpace(endText) != "" {
		end, parseErr := time.ParseInLocation("2006-01-02", endText, location)
		if parseErr != nil {
			return reportPeriod{}, apperrors.NewBusinessError("INVALID_REPORT_DATE", "结束日期格式不正确")
		}
		endExclusive = end.AddDate(0, 0, 1)
	}
	if !endExclusive.After(start) {
		return reportPeriod{}, apperrors.NewBusinessError("INVALID_REPORT_DATE", "结束日期不能早于开始日期")
	}
	if endExclusive.Sub(start) > 92*24*time.Hour {
		return reportPeriod{}, apperrors.NewBusinessError("REPORT_RANGE_TOO_LARGE", "单次最多查询 92 天数据")
	}
	return reportPeriod{Start: start, End: endExclusive, StartDate: start.Format("2006-01-02"), EndDate: endExclusive.AddDate(0, 0, -1).Format("2006-01-02"), Location: location}, nil
}

func validateLedgerType(value string) error {
	switch strings.TrimSpace(value) {
	case "", "all", "credit", "debit", "finance", "betting", "welfare", "share",
		"manual", "application_credit", "application_debit",
		"bet", "bet_cancel", "settlement", "reconciliation_refund",
		"rebate", "checkin", "redpacket", "invite", "agent_share":
		return nil
	default:
		return apperrors.NewBusinessError("INVALID_REPORT_TYPE", fmt.Sprintf("不支持的流水类型：%s", value))
	}
}
