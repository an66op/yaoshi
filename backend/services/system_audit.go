package services

import (
	"backend/data/models/audit"
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/data/models/user"
	"context"
	"time"

	"gorm.io/gorm"
)

type SystemAuditService struct{ db *gorm.DB }

func NewSystemAuditService(db *gorm.DB) *SystemAuditService { return &SystemAuditService{db: db} }

func (s *SystemAuditService) RecoverSettlement(ctx context.Context, limit int, operator string) (SettlementRecoveryResult, error) {
	return NewBetAdminService(s.db).RecoverSettlementBacklog(ctx, limit, operator)
}

type AuditLogPage struct {
	Items      []audit.Log `json:"items"`
	NextBefore uint64      `json:"next_before_id,omitempty"`
	HasMore    bool        `json:"has_more"`
}

func (s *SystemAuditService) Logs(beforeID uint64, limit int) (AuditLogPage, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	query := s.db.Order("id desc").Limit(limit + 1)
	if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}
	var rows []audit.Log
	if err := query.Find(&rows).Error; err != nil {
		return AuditLogPage{}, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	next := uint64(0)
	if hasMore && len(rows) > 0 {
		next = rows[len(rows)-1].ID
	}
	return AuditLogPage{Items: rows, NextBefore: next, HasMore: hasMore}, nil
}

type ReconciliationSummary struct {
	GeneratedAt time.Time       `json:"generated_at"`
	IssueErrors []lottery.Issue `json:"issue_errors"`
	// AbnormalBets contains only rows that can actually use the manual refund
	// action: an unresolved pending bet explicitly marked abnormal. Historical
	// settled abnormalities remain visible through HistoricalAbnormalBetCount,
	// but are never presented as refundable work.
	AbnormalBets                  []ReconciliationBetView `json:"abnormal_bets"`
	IssueErrorCount               int64                   `json:"issue_error_count"`
	AbnormalBetCount              int64                   `json:"abnormal_bet_count"`
	HistoricalAbnormalBetCount    int64                   `json:"historical_abnormal_bet_count"`
	PendingOnClosedCount          int64                   `json:"pending_on_closed_count"`
	UnresolvedBetCount            int64                   `json:"unresolved_bet_count"`
	RecoverableBetCount           int64                   `json:"recoverable_bet_count"`
	UnrecoverableBetCount         int64                   `json:"unrecoverable_bet_count"`
	MissingIssueBetCount          int64                   `json:"missing_issue_bet_count"`
	DisabledGamePendingCount      int64                   `json:"disabled_game_pending_count"`
	StaleIssueCount               int64                   `json:"stale_issue_count"`
	StalePendingIssueCount        int64                   `json:"stale_pending_issue_count"`
	StaleAwaitingIssueCount       int64                   `json:"stale_awaiting_issue_count"`
	StaleSettlingIssueCount       int64                   `json:"stale_settling_issue_count"`
	SourceErrorGameCount          int64                   `json:"source_error_game_count"`
	NegativeBalanceCount          int64                   `json:"negative_balance_count"`
	OrphanLedgerCount             int64                   `json:"orphan_ledger_count"`
	DuplicateLedgerReferenceCount int64                   `json:"duplicate_ledger_reference_count"`
	LedgerChainGapCount           int                     `json:"ledger_chain_gap_count"`
	LedgerArithmeticCount         int64                   `json:"ledger_arithmetic_error_count"`
	LatestBalanceGapCount         int                     `json:"latest_balance_gap_count"`
	UntrackedBalanceUsers         int                     `json:"untracked_balance_user_count"`
	PaymentAccountErrorCount      int64                   `json:"payment_account_error_count"`
	PaymentChannelErrorCount      int64                   `json:"payment_channel_error_count"`
	NotificationFinancialErrors   int64                   `json:"notification_financial_error_count"`
	RebateFinancialErrors         int64                   `json:"rebate_financial_error_count"`
	ProfitShareFinancialErrors    int64                   `json:"profit_share_financial_error_count"`
	AccountHierarchyErrorCount    int64                   `json:"account_hierarchy_error_count"`
	WorkspaceHierarchyErrorCount  int64                   `json:"workspace_hierarchy_error_count"`
	MembershipHierarchyErrorCount int64                   `json:"membership_hierarchy_error_count"`
}

func (s *SystemAuditService) Reconciliation() (ReconciliationSummary, error) {
	result := ReconciliationSummary{GeneratedAt: time.Now().UTC()}
	realUserIDs := excludeRobotProfileUsers(s.db.Model(&user.User{})).Select("user_id")
	health, err := NewBetAdminService(s.db).SettlementHealth(result.GeneratedAt)
	if err != nil {
		return result, err
	}
	result.UnresolvedBetCount = health.UnresolvedBetCount
	result.RecoverableBetCount = health.RecoverableBetCount
	result.UnrecoverableBetCount = health.UnrecoverableBetCount
	result.MissingIssueBetCount = health.MissingIssueBetCount
	result.DisabledGamePendingCount = health.DisabledGamePendingCount
	result.StaleIssueCount = health.StaleIssueCount
	result.StalePendingIssueCount = health.StalePendingIssueCount
	result.StaleAwaitingIssueCount = health.StaleAwaitingIssueCount
	result.StaleSettlingIssueCount = health.StaleSettlingIssueCount
	result.SourceErrorGameCount = health.SourceErrorGameCount
	if err := s.db.Model(&lottery.Issue{}).Where("status = ?", lottery.IssueStatusError).Count(&result.IssueErrorCount).Error; err != nil {
		return result, err
	}
	if err := s.db.Where("status = ?", lottery.IssueStatusError).Order("updated_at desc").Limit(50).Find(&result.IssueErrors).Error; err != nil {
		return result, err
	}
	if err := s.db.Model(&bet.Bet{}).Where("reconciliation_status = ?", "abnormal").Count(&result.HistoricalAbnormalBetCount).Error; err != nil {
		return result, err
	}
	abnormalQuery := s.db.Model(&bet.Bet{}).Where("status = ? AND reconciliation_status = ?", "pending", "abnormal")
	if err := abnormalQuery.Session(&gorm.Session{}).Count(&result.AbnormalBetCount).Error; err != nil {
		return result, err
	}
	var refundableRows []bet.Bet
	if err := abnormalQuery.Session(&gorm.Session{}).Order("id desc").Limit(50).Find(&refundableRows).Error; err != nil {
		return result, err
	}
	result.AbnormalBets = make([]ReconciliationBetView, 0, len(refundableRows))
	for _, row := range refundableRows {
		result.AbnormalBets = append(result.AbnormalBets, toReconciliationBetView(row))
	}
	if err := s.db.Model(&bet.Bet{}).Joins("JOIN lottery_issues ON lottery_issues.game_id = lottery_bets.game_id AND lottery_issues.issue = lottery_bets.issue").
		Where("lottery_bets.status = ? AND lottery_issues.status IN ?", "pending", []string{lottery.IssueStatusSettled, lottery.IssueStatusError}).
		Count(&result.PendingOnClosedCount).Error; err != nil {
		return result, err
	}
	if err := s.db.Model(&user.BalanceTransaction{}).
		Where("user_id IN (?)", realUserIDs).
		Where("after_cents <> before_cents + amount_cents").
		Count(&result.LedgerArithmeticCount).Error; err != nil {
		return result, err
	}
	if err := s.db.Model(&user.User{}).Where("balance_cents < 0").Count(&result.NegativeBalanceCount).Error; err != nil {
		return result, err
	}
	if err := s.db.Raw(`
		SELECT COUNT(*) FROM user_balance_transactions AS ledger
		LEFT JOIN "user" AS account ON account.user_id = ledger.user_id
		WHERE account.user_id IS NULL
	`).Scan(&result.OrphanLedgerCount).Error; err != nil {
		return result, err
	}
	if err := s.db.Raw(`
		SELECT COUNT(*) FROM (
			SELECT user_id, reference
			FROM user_balance_transactions
			WHERE reference <> ''
			GROUP BY user_id, reference
			HAVING COUNT(*) > 1
		) AS duplicates
	`).Scan(&result.DuplicateLedgerReferenceCount).Error; err != nil {
		return result, err
	}
	secondaryChecks := []struct {
		query  string
		target *int64
	}{
		{`SELECT COUNT(*) FROM member_payment_accounts WHERE account_type NOT IN ('bank', 'alipay', 'wechat', 'usdt')`, &result.PaymentAccountErrorCount},
		{`SELECT COUNT(*) FROM wallet_payment_channels WHERE fee_rate NOT BETWEEN 0 AND 100 OR min_amount < 0 OR max_amount < 0 OR (max_amount <> 0 AND max_amount < min_amount) OR credit_type NOT IN ('manual', 'bank', 'alipay', 'wechat', 'usdt') OR status NOT IN ('enabled', 'disabled')`, &result.PaymentChannelErrorCount},
		{`SELECT COUNT(*) FROM member_notifications WHERE bet_count < 0 OR won_count < 0 OR won_count > bet_count OR stake_cents < 0 OR payout_cents < 0 OR category NOT IN ('system', 'account', 'activity', 'winning') OR level NOT IN ('info', 'success', 'warning', 'error')`, &result.NotificationFinancialErrors},
		{`SELECT COUNT(*) FROM rebate_daily_records WHERE turnover_cents < 0 OR rate_percent NOT BETWEEN 0 AND 100 OR amount_cents < 0 OR status <> 'credited'`, &result.RebateFinancialErrors},
		{`SELECT COUNT(*) FROM agent_profit_share_records WHERE bet_count < 0 OR turnover_cents < 0 OR payout_cents < 0 OR rebate_cents < 0 OR accrued_share_cents < 0 OR paid_share_cents < 0 OR paid_share_cents > accrued_share_cents OR run_count < 0 OR BTRIM(room_scope) = '' OR status NOT IN ('pending', 'credited')`, &result.ProfitShareFinancialErrors},
		{accountHierarchyAuditSQL, &result.AccountHierarchyErrorCount},
		{workspaceHierarchyAuditSQL, &result.WorkspaceHierarchyErrorCount},
		{membershipHierarchyAuditSQL, &result.MembershipHierarchyErrorCount},
	}
	for _, check := range secondaryChecks {
		if err := s.db.Raw(check.query).Scan(check.target).Error; err != nil {
			return result, err
		}
	}
	rows, err := s.db.Model(&user.BalanceTransaction{}).Where("user_id IN (?)", realUserIDs).Order("user_id asc, id asc").Rows()
	if err != nil {
		return result, err
	}
	defer rows.Close()
	latest := make(map[uint64]int64)
	previous := make(map[uint64]int64)
	seen := make(map[uint64]bool)
	for rows.Next() {
		var row user.BalanceTransaction
		if err := s.db.ScanRows(rows, &row); err != nil {
			return result, err
		}
		if seen[row.UserID] && previous[row.UserID] != row.BeforeCents {
			result.LedgerChainGapCount++
		}
		seen[row.UserID] = true
		previous[row.UserID] = row.AfterCents
		latest[row.UserID] = row.AfterCents
	}
	var accounts []user.User
	if err := excludeRobotProfileUsers(s.db.Model(&user.User{})).Select("user_id", "balance_cents").Find(&accounts).Error; err != nil {
		return result, err
	}
	for _, account := range accounts {
		after, ok := latest[account.UserID]
		if !ok {
			if account.BalanceCents != 0 {
				result.UntrackedBalanceUsers++
			}
			continue
		}
		if account.BalanceCents != after {
			result.LatestBalanceGapCount++
		}
	}
	return result, nil
}

const accountHierarchyAuditSQL = `
	SELECT COUNT(*)
	FROM "user" AS account
	LEFT JOIN workspaces AS current_workspace ON current_workspace.id = account.workspace_id
	LEFT JOIN "user" AS parent_agent ON parent_agent.user_id = account.parent_agent_id AND parent_agent.deleted_at IS NULL
	LEFT JOIN "user" AS parent_tenant ON parent_tenant.user_id = account.parent_tenant_id AND parent_tenant.deleted_at IS NULL
	WHERE account.deleted_at IS NULL AND (
		account.role NOT IN ('admin', 'tenant', 'agent', 'member')
		OR account.status IS NULL OR account.status NOT IN (0, 1)
		OR account.workspace_id = 0 OR current_workspace.id IS NULL
		OR CASE account.role
			WHEN 'admin' THEN current_workspace.type <> 'platform'
				OR current_workspace.owner_user_id <> account.user_id
				OR account.parent_agent_id IS NOT NULL OR account.parent_tenant_id IS NOT NULL
				OR account.login_scope <> 'platform'
			WHEN 'tenant' THEN current_workspace.type <> 'tenant'
				OR current_workspace.owner_user_id <> account.user_id
				OR account.parent_agent_id IS NOT NULL OR account.parent_tenant_id IS NOT NULL
				OR account.login_scope <> 'platform'
			WHEN 'agent' THEN current_workspace.type <> 'agent'
				OR current_workspace.owner_user_id <> account.user_id
				OR account.parent_agent_id IS NOT NULL
				OR (account.parent_tenant_id IS NULL AND account.login_scope <> 'platform')
				OR (account.parent_tenant_id IS NOT NULL AND (
					parent_tenant.user_id IS NULL OR parent_tenant.role <> 'tenant'
					OR account.login_scope <> 'tenant:' || account.parent_tenant_id::text
				))
			WHEN 'member' THEN
				(current_workspace.type = 'platform' AND account.login_scope <> 'platform')
				OR (current_workspace.type <> 'platform' AND account.login_scope <> current_workspace.scope)
				OR CASE current_workspace.type
					WHEN 'platform' THEN account.parent_agent_id IS NOT NULL OR account.parent_tenant_id IS NOT NULL
					WHEN 'tenant' THEN account.parent_agent_id IS NOT NULL
						OR account.parent_tenant_id IS DISTINCT FROM current_workspace.owner_user_id
					WHEN 'agent' THEN account.parent_agent_id IS DISTINCT FROM current_workspace.owner_user_id
						OR parent_agent.user_id IS NULL OR parent_agent.role <> 'agent'
						OR account.parent_tenant_id IS DISTINCT FROM parent_agent.parent_tenant_id
					ELSE TRUE
				END
			ELSE TRUE
		END
	)`

const workspaceHierarchyAuditSQL = `
	SELECT COUNT(*)
	FROM workspaces AS workspace
	LEFT JOIN "user" AS owner ON owner.user_id = workspace.owner_user_id AND owner.deleted_at IS NULL
	LEFT JOIN workspaces AS parent ON parent.id = workspace.parent_id
	WHERE owner.user_id IS NULL
		OR workspace.type NOT IN ('platform', 'tenant', 'agent')
		OR workspace.status NOT IN (0, 1)
		OR workspace.status IS DISTINCT FROM owner.status
		OR (workspace.type = 'platform' AND owner.role <> 'admin')
		OR (workspace.type = 'tenant' AND owner.role <> 'tenant')
		OR (workspace.type = 'agent' AND owner.role <> 'agent')
		OR owner.workspace_id <> workspace.id
		OR CASE workspace.type
			WHEN 'platform' THEN workspace.parent_id IS NOT NULL
				OR workspace.code <> '00000' OR workspace.scope <> 'lobby'
			WHEN 'tenant' THEN parent.id IS NULL OR parent.type <> 'platform'
				OR workspace.scope <> 'tenant:' || workspace.owner_user_id::text
			WHEN 'agent' THEN workspace.scope <> 'agent:' || workspace.owner_user_id::text
				OR parent.id IS NULL
				OR (owner.parent_tenant_id IS NULL AND parent.type <> 'platform')
				OR (owner.parent_tenant_id IS NOT NULL AND (
					parent.type <> 'tenant' OR parent.owner_user_id IS DISTINCT FROM owner.parent_tenant_id
				))
			ELSE TRUE
		END`

const membershipHierarchyAuditSQL = `
	SELECT COUNT(*)
	FROM "user" AS account
	LEFT JOIN workspace_memberships AS current_membership
		ON current_membership.user_id = account.user_id
		AND current_membership.workspace_id = account.workspace_id
	WHERE account.deleted_at IS NULL AND (
		account.workspace_id = 0 OR current_membership.id IS NULL
		OR current_membership.role <> account.role
		OR current_membership.status <> account.status
		OR EXISTS (
			SELECT 1 FROM workspace_memberships AS active_membership
			WHERE active_membership.user_id = account.user_id
				AND active_membership.status = 1
				AND (account.status <> 1 OR active_membership.workspace_id <> account.workspace_id)
		)
	)`
