package services

import (
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// The business-data reset takes this lock before it clears any rows and
	// appends its immutable receipt. Funding uses the same lock so it cannot
	// validate one reset generation and commit into another one.
	devAcceptanceResetLock = int64(729421120)
	devAcceptanceType      = "system_topup"
	// Match the existing administrative credit ceiling: 100,000,000.00.
	maxDevAcceptanceAmountCents = int64(10_000_000_000)
)

var devAcceptanceRequestIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{7,95}$`)

// DevAcceptanceFundingSafety is supplied from the already-loaded application
// configuration. Keeping it explicit makes direct callers subject to the same
// debug/loopback gate as the command instead of relying on a CLI-only check.
type DevAcceptanceFundingSafety struct {
	Mode         string
	DatabaseHost string
	DatabaseName string
}

// DevAcceptanceFundingInput identifies one deliberate post-reset credit. The
// amount is integer cents so no floating-point conversion can alter the grant.
type DevAcceptanceFundingInput struct {
	ResetRequestID string
	LoginScope     string
	Username       string
	AmountCents    int64
}

type DevAcceptanceFundingResult struct {
	ResetRequestID string `json:"reset_request_id"`
	LoginScope     string `json:"login_scope"`
	Username       string `json:"username"`
	UserID         uint64 `json:"user_id"`
	WorkspaceID    uint64 `json:"workspace_id"`
	AmountCents    int64  `json:"amount_cents"`
	BeforeCents    int64  `json:"before_cents"`
	AfterCents     int64  `json:"after_cents"`
	Reference      string `json:"reference"`
	Duplicate      bool   `json:"duplicate"`
}

type devAcceptanceDatabaseIdentity struct {
	DatabaseName  string `gorm:"column:database_name"`
	ServerAddress string `gorm:"column:server_address"`
}

type devAcceptanceResetReceipt struct {
	RequestID    string `gorm:"column:request_id"`
	DatabaseName string `gorm:"column:database_name"`
	ResetScope   string `gorm:"column:reset_scope"`
}

// ValidateDevAcceptanceFundingSafety rejects release/test environments and
// remote database targets before a connection is opened.
func ValidateDevAcceptanceFundingSafety(safety DevAcceptanceFundingSafety) error {
	if safety.Mode != "debug" {
		return fmt.Errorf("验收账号注资仅允许 debug 模式")
	}
	if !devAcceptanceLocalHost(safety.DatabaseHost) {
		return fmt.Errorf("验收账号注资仅允许本机 PostgreSQL")
	}
	if safety.DatabaseName == "" || safety.DatabaseName != strings.TrimSpace(safety.DatabaseName) {
		return fmt.Errorf("必须提供明确的目标数据库名")
	}
	return nil
}

func devAcceptanceLocalHost(raw string) bool {
	host := strings.TrimSpace(strings.ToLower(raw))
	if host == "localhost" || strings.HasPrefix(host, "/") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func normalizeDevAcceptanceFundingInput(input DevAcceptanceFundingInput) (DevAcceptanceFundingInput, error) {
	if input.ResetRequestID != strings.TrimSpace(input.ResetRequestID) || !devAcceptanceRequestIDPattern.MatchString(input.ResetRequestID) {
		return input, fmt.Errorf("reset request_id 需要 8–96 位字母、数字或 . _ : -")
	}
	if input.Username == "" || input.Username != strings.TrimSpace(input.Username) || utf8.RuneCountInString(input.Username) > 50 || strings.ContainsAny(input.Username, "\x00\r\n\t") {
		return input, fmt.Errorf("username 必须是 1–50 位且不能包含首尾空白或控制字符")
	}
	if input.LoginScope == "" || input.LoginScope != strings.TrimSpace(input.LoginScope) || utf8.RuneCountInString(input.LoginScope) > 80 || strings.ContainsAny(input.LoginScope, "\x00\r\n\t") {
		return input, fmt.Errorf("login-scope 必须是 1–80 位且不能包含首尾空白或控制字符")
	}
	if input.AmountCents <= 0 || input.AmountCents > maxDevAcceptanceAmountCents {
		return input, fmt.Errorf("amount-cents 必须在 1–%d 之间", maxDevAcceptanceAmountCents)
	}
	return input, nil
}

// ValidateDevAcceptanceFundingInput lets the command reject malformed or
// dangerously large requests before it opens a database connection. The
// service repeats the validation for non-CLI callers.
func ValidateDevAcceptanceFundingInput(input DevAcceptanceFundingInput) error {
	_, err := normalizeDevAcceptanceFundingInput(input)
	return err
}

func devAcceptanceReference(requestID string, userID uint64) string {
	return "dev_acceptance:" + requestID + ":" + strconv.FormatUint(userID, 10)
}

func validateExistingDevAcceptanceFunding(
	row user.BalanceTransaction,
	account user.User,
	input DevAcceptanceFundingInput,
	reference string,
) error {
	if row.UserID != account.UserID || row.WorkspaceID != account.WorkspaceID ||
		row.Reference != reference || row.Type != devAcceptanceType ||
		row.AmountCents != input.AmountCents || row.BeforeCents != 0 || row.AfterCents != input.AmountCents {
		return fmt.Errorf("已有验收注资流水与本次参数不一致，拒绝复用 request_id")
	}
	return nil
}

func devAcceptanceResult(
	input DevAcceptanceFundingInput,
	account user.User,
	row user.BalanceTransaction,
	duplicate bool,
) *DevAcceptanceFundingResult {
	return &DevAcceptanceFundingResult{
		ResetRequestID: input.ResetRequestID,
		LoginScope:     account.LoginScope,
		Username:       account.Username,
		UserID:         account.UserID,
		WorkspaceID:    account.WorkspaceID,
		AmountCents:    row.AmountCents,
		BeforeCents:    row.BeforeCents,
		AfterCents:     row.AfterCents,
		Reference:      row.Reference,
		Duplicate:      duplicate,
	}
}

// FundDevAcceptanceAccount credits one existing member after an operator-run
// business-data reset. It changes only user.balance_cents and appends one
// immutable ledger row; account identity, workspace, lottery and rule records
// are never updated.
func FundDevAcceptanceAccount(
	db *gorm.DB,
	safety DevAcceptanceFundingSafety,
	input DevAcceptanceFundingInput,
) (*DevAcceptanceFundingResult, error) {
	if err := ValidateDevAcceptanceFundingSafety(safety); err != nil {
		return nil, err
	}
	input, err := normalizeDevAcceptanceFundingInput(input)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, fmt.Errorf("数据库连接不可用")
	}

	var result *DevAcceptanceFundingResult
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(?)`, devAcceptanceResetLock).Error; err != nil {
			return err
		}
		// The reset utility also owns the advisory lock. This table lock closes
		// the smaller race against an out-of-band manual receipt insert, keeping
		// the meaning of "latest" stable until the funding commit.
		if err := tx.Exec(`LOCK TABLE public.development_reset_receipts IN SHARE MODE`).Error; err != nil {
			return err
		}

		var identity devAcceptanceDatabaseIdentity
		identityQuery := tx.Raw(`
			SELECT current_database() AS database_name,
			       COALESCE(host(inet_server_addr()), 'local-socket') AS server_address
		`).Scan(&identity)
		if identityQuery.Error != nil {
			return identityQuery.Error
		}
		if identity.DatabaseName != safety.DatabaseName {
			return fmt.Errorf("当前数据库 %q 与配置目标 %q 不匹配", identity.DatabaseName, safety.DatabaseName)
		}
		if identity.ServerAddress != "local-socket" && !devAcceptanceLocalHost(identity.ServerAddress) {
			return fmt.Errorf("数据库服务端地址 %q 不是本机回环地址", identity.ServerAddress)
		}

		var receipt devAcceptanceResetReceipt
		receiptQuery := tx.Raw(`
			SELECT request_id, database_name, reset_scope
			FROM public.development_reset_receipts
			ORDER BY created_at DESC, request_id DESC
			LIMIT 1
			FOR SHARE
		`).Scan(&receipt)
		if receiptQuery.Error != nil {
			return receiptQuery.Error
		}
		if receiptQuery.RowsAffected != 1 {
			return fmt.Errorf("未找到业务数据重置凭证，拒绝注资")
		}
		if receipt.RequestID != input.ResetRequestID || receipt.DatabaseName != identity.DatabaseName || receipt.ResetScope != "business_data" {
			return fmt.Errorf("最新重置凭证与 request_id、数据库或 business_data 范围不匹配")
		}

		var accounts []user.User
		accountErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("login_scope = ? AND LOWER(username) = LOWER(?)", input.LoginScope, input.Username).
			Limit(2).Find(&accounts).Error
		if accountErr != nil {
			return accountErr
		}
		if len(accounts) == 0 {
			return fmt.Errorf("验收账号不存在")
		}
		if len(accounts) != 1 {
			return fmt.Errorf("login_scope 内存在多个同名验收账号，拒绝选择任意账号")
		}
		account := accounts[0]
		if account.Role != "member" || account.Status != 1 || account.WorkspaceID == 0 {
			return fmt.Errorf("验收账号必须是已启用且已进入工作区的 member")
		}

		var workspace workspacemodel.Workspace
		workspaceErr := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&workspace, account.WorkspaceID).Error
		if errors.Is(workspaceErr, gorm.ErrRecordNotFound) || workspaceErr == nil && workspace.Status != 1 {
			return fmt.Errorf("验收账号所属工作区不存在或未启用")
		}
		if workspaceErr != nil {
			return workspaceErr
		}

		var membership workspacemodel.Membership
		membershipErr := tx.Clauses(clause.Locking{Strength: "SHARE"}).
			Where("workspace_id = ? AND user_id = ?", workspace.ID, account.UserID).
			Take(&membership).Error
		if errors.Is(membershipErr, gorm.ErrRecordNotFound) || membershipErr == nil && (membership.Status != 1 || membership.Role != "member") {
			return fmt.Errorf("验收账号没有当前工作区的 active member 关系")
		}
		if membershipErr != nil {
			return membershipErr
		}

		reference := devAcceptanceReference(input.ResetRequestID, account.UserID)
		var existing user.BalanceTransaction
		ledgerErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND reference = ?", account.UserID, reference).
			Take(&existing).Error
		if ledgerErr == nil {
			if err := validateExistingDevAcceptanceFunding(existing, account, input, reference); err != nil {
				return err
			}
			result = devAcceptanceResult(input, account, existing, true)
			return nil
		}
		if !errors.Is(ledgerErr, gorm.ErrRecordNotFound) {
			return ledgerErr
		}
		if account.BalanceCents != 0 {
			return fmt.Errorf("验收账号首次注资前余额必须为 0，当前为 %d cents", account.BalanceCents)
		}

		updated := tx.Model(&user.User{}).
			Where("user_id = ? AND workspace_id = ? AND role = ? AND status = ? AND balance_cents = 0", account.UserID, account.WorkspaceID, "member", 1).
			UpdateColumn("balance_cents", input.AmountCents)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("验收账号状态或余额在注资前发生变化")
		}

		row := user.BalanceTransaction{
			WorkspaceID: account.WorkspaceID,
			UserID:      account.UserID,
			Reference:   reference,
			AmountCents: input.AmountCents,
			BeforeCents: 0,
			AfterCents:  input.AmountCents,
			Type:        devAcceptanceType,
			Remark:      "业务数据重置后验收账号注资",
			Operator:    "开发验收初始化工具",
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		result = devAcceptanceResult(input, account, row, false)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
