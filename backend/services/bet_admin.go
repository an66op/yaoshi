package services

import (
	"backend/accesscontrol"
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"backend/ws"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BetAdminService struct {
	db                    *gorm.DB
	suppressNotifications bool
}

type BetView struct {
	ID               uint64    `json:"id"`
	GameID           string    `json:"game_id"`
	Issue            string    `json:"issue"`
	UserID           uint64    `json:"user_id"`
	Username         string    `json:"username"`
	PlayCode         string    `json:"play_code"`
	PlayName         string    `json:"play_name"`
	RuleVersion      string    `json:"rule_version,omitempty"`
	Position         int       `json:"position"`
	Selection        string    `json:"selection"`
	Amount           float64   `json:"amount"`
	Odds             float64   `json:"odds"`
	SettlementOdds   *float64  `json:"settlement_odds,omitempty"`
	ValidTurnover    *float64  `json:"valid_turnover,omitempty"`
	UserIssueStake   *float64  `json:"user_issue_stake,omitempty"`
	SettlementPolicy string    `json:"settlement_policy,omitempty"`
	Status           string    `json:"status"`
	Payout           float64   `json:"payout"`
	FlyAmount        float64   `json:"fly_amount"`
	Remark           string    `json:"remark"`
	Operator         string    `json:"operator"`
	CreatedAt        time.Time `json:"created_at"`
	Deducted         float64   `json:"deducted,omitempty"`
	Balance          float64   `json:"balance,omitempty"`
}

type CancelIssueResult struct {
	GameID  string  `json:"game_id"`
	Issue   string  `json:"issue"`
	Count   int     `json:"count"`
	Refund  float64 `json:"refund"`
	Balance float64 `json:"balance"`
}

type PlaceBetInput struct {
	GameID    string
	Issue     string
	UserID    uint64
	PlayCode  string
	PlayName  string
	Position  int
	Selection string
	Amount    float64
	Odds      float64
	FlyAmount *float64 // nil = 按用户/房间飞单策略自动计算
	Remark    string
	Operator  string
	// BetMode is server-owned. Mark Six is intentionally available only to
	// the typed member web ticket and never to compact/chat parsers.
	BetMode string
	// LedgerReference links an automatic balance deduction to its idempotent
	// request. Internal/manual callers may leave it empty; placement then gives
	// the financial operation its own reference rather than altering an old bet.
	LedgerReference string
}

type betLimitEntry struct {
	PlayCode    string
	Position    int
	Selection   string
	AmountCents int64
}

type MonitorSnapshot struct {
	GameID      string            `json:"game_id"`
	GameName    string            `json:"game_name"`
	Issue       string            `json:"issue"`
	TotalAmount float64           `json:"total_amount"`
	BettorCount int64             `json:"bettor_count"`
	BetCount    int64             `json:"bet_count"`
	NextDrawAt  time.Time         `json:"next_draw_at"`
	DrawAtLabel string            `json:"draw_at_label"`
	Matrix      [][]float64       `json:"matrix"` // SSC: 0-9 x 6; racing/flying: 0-10 x 10 positions
	UpdatedAt   time.Time         `json:"updated_at"`
	Settlement  *SettlementStatus `json:"settlement,omitempty"`
}

type BoardReportRow struct {
	GameID      string     `json:"game_id"`
	GameName    string     `json:"game_name"`
	Issue       string     `json:"issue"`
	BetCount    int64      `json:"bet_count"`
	TotalAmount float64    `json:"total_amount"`
	FlyAmount   float64    `json:"fly_amount"`
	Status      string     `json:"status"`
	DrawAt      *time.Time `json:"draw_at"`
	DrawResult  string     `json:"draw_result"`
}

type BoardReport struct {
	Items    []BoardReportRow `json:"items"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}

type DashboardStats struct {
	UserBalance          float64 `json:"user_balance"`
	TodayTurnover        float64 `json:"today_turnover"`
	TodaySettledTurnover float64 `json:"today_settled_turnover"`
	TodayGrossProfit     float64 `json:"today_gross_profit"`
	TodayNetProfit       float64 `json:"today_net_profit"`
	TodayRebate          float64 `json:"today_rebate"`
	TodayWelfare         float64 `json:"today_welfare"`
	TodayAgentShare      float64 `json:"today_agent_share"`
	TotalGrossProfit     float64 `json:"total_gross_profit"`
	TotalNetProfit       float64 `json:"total_net_profit"`
	TotalRebate          float64 `json:"total_rebate"`
	TotalWelfare         float64 `json:"total_welfare"`
	TotalAgentShare      float64 `json:"total_agent_share"`
	PendingSettlement    float64 `json:"pending_settlement"`
	TodayProfit          float64 `json:"today_profit"` // 兼容旧字段 = 毛利
	TotalProfit          float64 `json:"total_profit"` // 兼容旧字段 = 毛利
}

type gameMoney struct {
	GameID      string
	Turnover    float64
	GrossProfit float64
	Profit      float64 // 兼容旧字段 = 毛利
}

func NewBetAdminService(db *gorm.DB) *BetAdminService { return &BetAdminService{db: db} }

const idempotencyReservationTimeout = 2 * time.Minute

const (
	maxSignedInt64 = int64(9223372036854775807)
	minSignedInt64 = -maxSignedInt64 - 1
)

// positiveMoneyCents is the only supported float-to-ledger conversion for a
// stake. Go's direct float64 -> int64 conversion is implementation-dependent
// outside the integer range (and NaN/Inf can otherwise turn into a negative
// debit), so every public amount must be proven finite and in range first.
func positiveMoneyCents(value float64, label string) (int64, error) {
	cents, err := moneyCents(value, false, label)
	if err != nil {
		return 0, err
	}
	return cents, nil
}

// A caller's stake must not be silently rounded into a different order.
// Internal proportional fly/rebate calculations may still round to cents.
func validatedStakeCents(value float64) (int64, error) {
	cents, err := positiveMoneyCents(value, "下注金额")
	if err != nil {
		return 0, err
	}
	canonical := strconv.FormatFloat(value, 'f', -1, 64)
	if point := strings.IndexByte(canonical, '.'); point >= 0 && len(canonical)-point-1 > 2 {
		return 0, apperrors.NewBusinessError("INVALID_REQUEST", "下注金额最多支持两位小数")
	}
	return cents, nil
}

func nonNegativeMoneyCents(value float64, label string) (int64, error) {
	return moneyCents(value, true, label)
}

func moneyCents(value float64, allowZero bool, label string) (int64, error) {
	label = defaultString(strings.TrimSpace(label), "金额")
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, apperrors.NewBusinessError("INVALID_REQUEST", label+"不正确")
	}
	scaled := value * 100
	// 2^63 is exactly representable as float64 whereas MaxInt64 is not. The
	// strict comparison therefore also rejects the first unrepresentable int64.
	int64ExclusiveUpperBound := math.Ldexp(1, 63)
	if math.IsNaN(scaled) || math.IsInf(scaled, 0) || scaled >= int64ExclusiveUpperBound {
		return 0, apperrors.NewBusinessError("INVALID_REQUEST", label+"超出系统范围")
	}
	rounded := math.Round(scaled)
	if rounded >= int64ExclusiveUpperBound {
		return 0, apperrors.NewBusinessError("INVALID_REQUEST", label+"超出系统范围")
	}
	cents := int64(rounded)
	if cents < 0 || (!allowZero && cents == 0) {
		if allowZero {
			return 0, apperrors.NewBusinessError("INVALID_REQUEST", label+"不能小于 0")
		}
		return 0, apperrors.NewBusinessError("INVALID_REQUEST", label+"必须大于 0")
	}
	return cents, nil
}

func requestedFlyAmount(value *float64) (float64, error) {
	if value == nil {
		return -1, nil
	}
	cents, err := nonNegativeMoneyCents(*value, "飞单金额")
	if err != nil {
		return 0, err
	}
	return centsToAmount(cents), nil
}

func safeAddInt64(left, right int64) (int64, bool) {
	if right > 0 && left > maxSignedInt64-right {
		return 0, false
	}
	if right < 0 && left < minSignedInt64-right {
		return 0, false
	}
	return left + right, true
}

func idempotencyPayloadHash(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

// Every financial operation owns its bet rows, including administrative and
// robot calls without a client request ID. Both the live and archive dedupe
// keys already include this reference. Giving new calls a fresh reference
// preserves prior odds/rates/status without rewriting historical contracts or
// weakening either unique constraint.
func placementRequestReference(inputs []PlaceBetInput) (string, error) {
	if len(inputs) == 0 {
		return "", apperrors.NewBusinessError("INVALID_REQUEST", "请至少提供一注投注内容")
	}
	reference := strings.TrimSpace(inputs[0].LedgerReference)
	for _, input := range inputs[1:] {
		if strings.TrimSpace(input.LedgerReference) != reference {
			return "", apperrors.NewBusinessError("INVALID_REQUEST", "同一张投注单的请求标识必须一致")
		}
	}
	if reference != "" {
		return reference, nil
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", apperrors.NewSystemError("REQUEST_SAVE_FAILED", "生成投注请求凭证失败", err)
	}
	return "internal_bet:" + hex.EncodeToString(nonce[:]), nil
}

type placementBetKey struct {
	workspaceID                              uint64
	userID                                   uint64
	roomScope, gameID, issue, playCode       string
	selection, requestReference, ruleVersion string
	position                                 int
}

// Consolidate duplicate selections only inside this new financial operation.
// The source-index mapping keeps one result per caller line (the assistant
// associates odds with those line indexes). Never read or update an older row.
func aggregatePlacementRows(rows []bet.Bet) ([]bet.Bet, []int, error) {
	aggregated := make([]bet.Bet, 0, len(rows))
	indexes := make([]int, 0, len(rows))
	byKey := make(map[placementBetKey]int, len(rows))
	for _, row := range rows {
		if row.ID != 0 || row.Status != "pending" || row.SettledAt != nil || row.PayoutCents != 0 ||
			row.RebateCents != 0 || row.AgentShareCents != 0 || strings.TrimSpace(row.RequestReference) == "" {
			return nil, nil, apperrors.NewBusinessError("INVALID_REQUEST", "只能合并本次新建的待结算注单")
		}
		if row.AmountCents <= 0 || row.FlyCents < 0 || row.FlyCents > row.AmountCents {
			return nil, nil, apperrors.NewBusinessError("INVALID_REQUEST", "注单金额异常")
		}
		key := placementBetKey{
			workspaceID: row.WorkspaceID, userID: row.UserID, roomScope: row.RoomScope,
			gameID: row.GameID, issue: row.Issue, playCode: row.PlayCode, position: row.Position,
			selection: row.Selection, requestReference: row.RequestReference, ruleVersion: row.RuleVersion,
		}
		index, exists := byKey[key]
		if !exists {
			byKey[key] = len(aggregated)
			indexes = append(indexes, len(aggregated))
			aggregated = append(aggregated, row)
			continue
		}
		previous := &aggregated[index]
		if previous.Odds != row.Odds || previous.RebateRateSnapshot != row.RebateRateSnapshot ||
			previous.AgentShareRateSnapshot != row.AgentShareRateSnapshot || previous.PC28GrayPush != row.PC28GrayPush {
			return nil, nil, apperrors.NewBusinessError("INVALID_REQUEST", "同一投注项的财务条件已变化，请重新提交")
		}
		amount, ok := safeAddInt64(previous.AmountCents, row.AmountCents)
		if !ok || amount <= 0 {
			return nil, nil, apperrors.NewBusinessError("INVALID_REQUEST", "注单累计金额过大")
		}
		fly, ok := safeAddInt64(previous.FlyCents, row.FlyCents)
		if !ok || fly < 0 || fly > amount {
			return nil, nil, apperrors.NewBusinessError("INVALID_REQUEST", "注单飞单金额异常")
		}
		previous.AmountCents = amount
		if previous.ValidTurnoverCents != nil && row.ValidTurnoverCents != nil {
			valid, validOK := safeAddInt64(*previous.ValidTurnoverCents, *row.ValidTurnoverCents)
			if !validOK || valid < 0 || valid > amount {
				return nil, nil, apperrors.NewBusinessError("INVALID_REQUEST", "注单有效流水异常")
			}
			previous.ValidTurnoverCents = int64Pointer(valid)
		} else {
			previous.ValidTurnoverCents = nil
		}
		previous.FlyCents = fly
		indexes = append(indexes, index)
	}
	return aggregated, indexes, nil
}

func (s *BetAdminService) Place(input PlaceBetInput) (*BetView, error) {
	game, err := s.loadGame(input.GameID)
	if err != nil {
		return nil, err
	}
	if err := ensureGameRulesSupported(game); err != nil {
		return nil, err
	}
	profile, _ := rulesForGame(game)
	if err := ensurePlacementBetMode(profile, input.BetMode); err != nil {
		return nil, err
	}
	if !game.Enabled {
		return nil, apperrors.NewBusinessError("GAME_DISABLED", "该彩种暂未开放投注")
	}
	issue := strings.TrimSpace(input.Issue)
	if issue == "" {
		issue, err = s.BettingIssue(game.ID)
		if err != nil {
			return nil, err
		}
	}
	if err := s.ensureIssueOpen(game, issue); err != nil {
		return nil, err
	}
	if input.UserID == 0 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "请选择下注用户")
	}
	selection := strings.TrimSpace(input.Selection)
	if selection == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "投注内容不能为空")
	}
	playCode, playName, inferErr := InferPlayForGame(game, input.PlayCode, input.PlayName, input.Position, input.Selection)
	if inferErr != nil {
		return nil, inferErr
	}
	selection = normalizeBetSelection(game, playCode, selection)
	if err := validateBetChoice(game, playCode, input.Position, selection); err != nil {
		return nil, err
	}
	amountCents, amountErr := validatedStakeCents(input.Amount)
	if amountErr != nil {
		return nil, amountErr
	}
	requestFly, flyErr := requestedFlyAmount(input.FlyAmount)
	if flyErr != nil {
		return nil, flyErr
	}
	requestReference, referenceErr := placementRequestReference([]PlaceBetInput{input})
	if referenceErr != nil {
		return nil, referenceErr
	}

	var view *BetView
	var afterBalance int64
	var roomScope string
	var workspaceID uint64
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := lockBettingIssue(tx, game.ID, issue); err != nil {
			return err
		}
		var account user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, input.UserID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
			}
			return err
		}
		if account.Status != 1 {
			return apperrors.NewBusinessError("USER_DISABLED", "用户已被禁用")
		}
		roomActive, roomErr := accesscontrol.AccountRoomActive(tx, account)
		if roomErr != nil {
			return roomErr
		}
		if !roomActive {
			return apperrors.NewBusinessError("ROOM_UNAVAILABLE", "当前房间已停用，请切换房间后再下注")
		}
		roomGameEnabled, roomGameErr := WorkspaceGameEnabled(tx, account.WorkspaceID, game.ID)
		if roomGameErr != nil {
			return roomGameErr
		}
		if !roomGameEnabled {
			return apperrors.NewBusinessError("GAME_DISABLED", "当前房间暂未开放该游戏")
		}
		financialTerms, termsErr := resolveBetFinancialTerms(tx, account)
		if termsErr != nil {
			return termsErr
		}
		pc28GrayPush := false
		if profile.PC28 > 0 {
			rawSettings, _, settingsErr := readTimingSettings(tx, account.WorkspaceID)
			if settingsErr != nil {
				return settingsErr
			}
			pc28GrayPush = configuredPC28GrayPush(rawSettings, game.ID)
		}
		resolved, resolveErr := NewTradingAdminService(tx).ResolveForAccountSelection(
			account, game.ID, playCode, selection, centsToAmount(amountCents), input.Odds, financialTerms.flyRequest(requestFly),
		)
		if resolveErr != nil {
			return resolveErr
		}
		odds := resolved.Odds
		resolvedFlyCents, flyErr := nonNegativeMoneyCents(resolved.FlyAmount, "飞单金额")
		if flyErr != nil {
			return flyErr
		}
		flyCents := clampFlyCents(amountCents, resolvedFlyCents)
		if financialTerms.isRobot {
			flyCents = 0
		}
		if err := validateBetLimitEntries(tx, game.ID, issue, input.UserID, []betLimitEntry{{
			PlayCode: playCode, Position: input.Position, Selection: selection, AmountCents: amountCents,
		}}); err != nil {
			return err
		}
		if err := validatePC28PlacementConstraints(tx, profile, account.WorkspaceID, betRoomScope(account), game.ID, issue, input.UserID, []betLimitEntry{{
			PlayCode: playCode, Position: input.Position, Selection: selection, AmountCents: amountCents,
		}}); err != nil {
			return err
		}
		if account.BalanceCents < amountCents {
			return apperrors.NewBusinessError("INSUFFICIENT_BALANCE", "用户余额不足")
		}
		if err := checkWorkspaceIssueWindow(tx, account.WorkspaceID, game, issue); err != nil {
			return err
		}
		roomScope = betRoomScope(account)
		workspaceID = account.WorkspaceID
		before := account.BalanceCents
		after := before - amountCents
		afterBalance = after
		if err := tx.Model(&account).Update("balance_cents", after).Error; err != nil {
			return err
		}
		ledger := user.BalanceTransaction{
			WorkspaceID: account.WorkspaceID, UserID: account.UserID, Reference: requestReference,
			AmountCents: -amountCents, BeforeCents: before, AfterCents: after,
			Type: "bet", Remark: fmt.Sprintf("下注 %s/%s", game.Name, issue), Operator: defaultString(input.Operator, "后台管理员"),
		}
		if err := tx.Create(&ledger).Error; err != nil {
			return err
		}
		row := bet.Bet{
			WorkspaceID: account.WorkspaceID, GameID: game.ID, Issue: issue, RoomScope: roomScope, UserID: account.UserID, Username: account.Username,
			PlayCode: playCode, PlayName: playName, Position: input.Position, Selection: selection, RequestReference: requestReference,
			RuleVersion: profile.Version,
			AmountCents: amountCents, Odds: odds, ValidTurnoverCents: int64Pointer(amountCents), PC28GrayPush: pc28GrayPush,
			Status: "pending", FlyCents: flyCents,
			RebateRateSnapshot: financialTerms.rebateRate, AgentShareRateSnapshot: financialTerms.shareRate,
			Remark: strings.TrimSpace(input.Remark), Operator: defaultString(input.Operator, "后台管理员"),
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		v := toBetView(row)
		view = &v
		return nil
	})
	if err != nil {
		if app, ok := err.(*apperrors.AppError); ok {
			return nil, app
		}
		return nil, apperrors.NewSystemError("BET_CREATE_FAILED", "创建注单失败", err)
	}
	if !s.suppressNotifications {
		s.notifyPlacement(input.UserID, workspaceID, roomScope, game.ID, issue, centsToAmount(afterBalance))
	}
	if view != nil {
		view.Deducted = centsToAmount(amountCents)
		view.Balance = centsToAmount(afterBalance)
	}
	return view, nil
}

// PlaceIdempotent protects the public direct-bet endpoint from double clicks
// and retry-after-timeout duplicates.  Administrative internal calls can keep
// using Place because they do not accept an untrusted client request id.
func (s *BetAdminService) PlaceIdempotent(input PlaceBetInput, requestID string) (*BetView, error) {
	requestID = strings.TrimSpace(requestID)
	if len(requestID) < 8 || len(requestID) > 96 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "请求标识不正确")
	}
	hashInput := input
	hashInput.Operator = ""
	hashInput.LedgerReference = ""
	payloadHash, hashErr := idempotencyPayloadHash(hashInput)
	if hashErr != nil {
		return nil, apperrors.NewSystemError("REQUEST_SAVE_FAILED", "生成投注请求凭证失败", hashErr)
	}
	var result *BetView
	var notify bool
	var workspaceID uint64
	var roomScope string
	var terminalErr error
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var owner user.User
		if err := tx.Select("user_id", "workspace_id").First(&owner, input.UserID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
			}
			return err
		}

		row := bet.BetRequest{WorkspaceID: owner.WorkspaceID, UserID: input.UserID, RequestID: requestID, PayloadHash: payloadHash, Status: "processing"}
		created := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "request_id"}},
			DoNothing: true,
		}).Create(&row)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 0 {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("user_id = ? AND request_id = ?", input.UserID, requestID).First(&row).Error; err != nil {
				return err
			}
			if row.WorkspaceID != owner.WorkspaceID {
				message := "请求所属房间已变化，请使用新的请求标识重新提交"
				if row.Status == "processing" {
					if err := tx.Model(&bet.BetRequest{}).Where("id = ? AND status = ?", row.ID, "processing").
						Updates(map[string]any{"status": "failed", "last_error": message}).Error; err != nil {
						return err
					}
				}
				terminalErr = apperrors.NewBusinessError("REQUEST_FAILED", message)
				return nil
			}
			if row.PayloadHash != "" && row.PayloadHash != payloadHash {
				terminalErr = apperrors.NewBusinessError("IDEMPOTENCY_CONFLICT", "该请求标识已用于其他投注")
				return nil
			}
			cached, handled, err := s.resolveExistingBetRequest(tx, row, time.Now().UTC())
			if err != nil {
				return err
			}
			if handled {
				result = cached
				return nil
			}
		}

		// Place performs lifecycle/window materialization before its nested
		// financial transaction. Hold the game first in this outer transaction
		// too, so a source sync cannot hold Game while waiting for our Issue.
		// Completed idempotent receipts above deliberately bypass these locks.
		if _, err := lockBettingGame(tx, input.GameID); err != nil {
			return err
		}
		input.LedgerReference = directBetRequestReference(row.ID)
		placed, err := (&BetAdminService{db: tx, suppressNotifications: true}).Place(input)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(placed)
		if err != nil {
			return apperrors.NewSystemError("REQUEST_SAVE_FAILED", "保存投注回执失败", err)
		}
		updated := tx.Model(&bet.BetRequest{}).Where("id = ? AND status = ?", row.ID, "processing").
			Updates(map[string]any{"status": "completed", "result_json": string(payload), "last_error": ""})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("投注请求状态发生冲突")
		}
		var committedOwner user.User
		if err := tx.Select("user_id", "workspace_id", "role", "parent_agent_id", "parent_tenant_id").First(&committedOwner, input.UserID).Error; err != nil {
			return err
		}
		result = placed
		notify = true
		workspaceID = committedOwner.WorkspaceID
		roomScope = betRoomScope(committedOwner)
		return nil
	})
	if err != nil {
		if app, ok := err.(*apperrors.AppError); ok {
			return nil, app
		}
		return nil, apperrors.NewSystemError("BET_CREATE_FAILED", "创建注单失败", err)
	}
	if terminalErr != nil {
		return nil, terminalErr
	}
	if notify && result != nil {
		s.notifyPlacement(input.UserID, workspaceID, roomScope, result.GameID, result.Issue, result.Balance)
	}
	return result, nil
}

func directBetRequestReference(id uint64) string {
	return "bet_request:" + strconv.FormatUint(id, 10)
}

// resolveExistingBetRequest handles both ordinary retries and reservations
// left by the pre-atomic implementation.  A durable ledger row proves that a
// legacy request charged the member; in that case we build a conservative
// final receipt and never execute the bet again.  An old reservation without
// a ledger is safe to reuse because the bet and its ledger commit together.
func (s *BetAdminService) resolveExistingBetRequest(tx *gorm.DB, row bet.BetRequest, now time.Time) (*BetView, bool, error) {
	if row.Status == "completed" && strings.TrimSpace(row.ResultJSON) != "" {
		var cached BetView
		if err := json.Unmarshal([]byte(row.ResultJSON), &cached); err != nil {
			return nil, true, apperrors.NewSystemError("REQUEST_READ_FAILED", "读取投注结果失败", err)
		}
		return &cached, true, nil
	}
	if row.Status == "failed" {
		return nil, true, apperrors.NewBusinessError("REQUEST_FAILED", defaultString(row.LastError, "该次投注未成功，请重新提交"))
	}
	if !idempotencyReservationExpired(row.UpdatedAt, now) {
		return nil, true, apperrors.NewBusinessError("REQUEST_PROCESSING", "投注正在处理中，请稍候查看注单")
	}

	var ledger user.BalanceTransaction
	err := tx.Where("user_id = ? AND reference = ?", row.UserID, directBetRequestReference(row.ID)).First(&ledger).Error
	if err == gorm.ErrRecordNotFound {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	if err := validateIdempotencyRequestLedger(ledger, row.UserID, row.WorkspaceID, directBetRequestReference(row.ID)); err != nil {
		return nil, true, fmt.Errorf("历史投注请求账务证据不一致")
	}
	recovered, payload, marshalErr := recoveredDirectBetView(ledger)
	if marshalErr != nil {
		return nil, true, marshalErr
	}
	updated := tx.Model(&bet.BetRequest{}).Where("id = ? AND status = ?", row.ID, "processing").
		Updates(map[string]any{"status": "completed", "result_json": string(payload), "last_error": ""})
	if updated.Error != nil {
		return nil, true, updated.Error
	}
	if updated.RowsAffected != 1 {
		return nil, true, fmt.Errorf("恢复投注请求时状态发生冲突")
	}
	return recovered, true, nil
}

func recoveredDirectBetView(ledger user.BalanceTransaction) (*BetView, []byte, error) {
	recovered := &BetView{
		Deducted: centsToAmount(-ledger.AmountCents), Balance: centsToAmount(ledger.AfterCents),
		Remark: "该次投注已受理，请在注单记录查看详情", CreatedAt: ledger.CreatedAt.UTC(),
	}
	payload, err := json.Marshal(recovered)
	return recovered, payload, err
}

func idempotencyReservationExpired(updatedAt, now time.Time) bool {
	if updatedAt.IsZero() {
		return true
	}
	return !now.Before(updatedAt.Add(idempotencyReservationTimeout))
}

func validateIdempotencyDebitLedger(ledger user.BalanceTransaction) error {
	if ledger.UserID == 0 || strings.TrimSpace(ledger.Reference) == "" || ledger.AmountCents >= 0 ||
		ledger.BeforeCents < 0 || ledger.AfterCents < 0 || ledger.AfterCents != ledger.BeforeCents+ledger.AmountCents {
		return fmt.Errorf("idempotency ledger is not a valid debit")
	}
	return nil
}

func validateIdempotencyRequestLedger(ledger user.BalanceTransaction, userID, workspaceID uint64, reference string) error {
	if err := validateIdempotencyDebitLedger(ledger); err != nil {
		return err
	}
	if strings.TrimSpace(ledger.Type) != "bet" {
		return fmt.Errorf("idempotency ledger is not a bet debit")
	}
	if ledger.UserID != userID || ledger.WorkspaceID != workspaceID || strings.TrimSpace(ledger.Reference) != strings.TrimSpace(reference) {
		return fmt.Errorf("idempotency ledger scope does not match the request")
	}
	return nil
}

// PlaceBatch accepts an already-validated ticket as one financial operation.
// All rows and the balance deduction are committed together, so a later line
// can never leave the member with a partially accepted ticket.
func (s *BetAdminService) PlaceBatch(inputs []PlaceBetInput) ([]BetView, error) {
	if len(inputs) == 0 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "请至少提供一注投注内容")
	}
	requestReference, referenceErr := placementRequestReference(inputs)
	if referenceErr != nil {
		return nil, referenceErr
	}
	game, err := s.loadGame(inputs[0].GameID)
	if err != nil {
		return nil, err
	}
	if err := ensureGameRulesSupported(game); err != nil {
		return nil, err
	}
	profile, _ := rulesForGame(game)
	if err := ensurePlacementBetMode(profile, inputs[0].BetMode); err != nil {
		return nil, err
	}
	if !game.Enabled {
		return nil, apperrors.NewBusinessError("GAME_DISABLED", "该彩种暂未开放投注")
	}
	userID := inputs[0].UserID
	if userID == 0 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "请选择下注用户")
	}
	issue := strings.TrimSpace(inputs[0].Issue)
	if issue == "" {
		issue, err = s.BettingIssue(game.ID)
		if err != nil {
			return nil, err
		}
	}
	if err := s.ensureIssueOpen(game, issue); err != nil {
		return nil, err
	}

	type preparedBet struct {
		input       PlaceBetInput
		playCode    string
		playName    string
		selection   string
		amountCents int64
		requestFly  float64
		odds        float64
		flyCents    int64
	}
	prepared := make([]preparedBet, 0, len(inputs))
	var totalCents int64
	for _, input := range inputs {
		if strings.TrimSpace(input.GameID) != game.ID || input.UserID != userID {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "同一张投注单只能包含同一彩种和用户")
		}
		if err := ensurePlacementBetMode(profile, input.BetMode); err != nil {
			return nil, err
		}
		if requestedIssue := strings.TrimSpace(input.Issue); requestedIssue != "" && requestedIssue != issue {
			return nil, apperrors.NewBusinessError("ISSUE_MISMATCH", "同一张投注单的期号必须一致")
		}
		selection := strings.TrimSpace(input.Selection)
		if selection == "" {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "投注内容不能为空")
		}
		playCode, playName, inferErr := InferPlayForGame(game, input.PlayCode, input.PlayName, input.Position, selection)
		if inferErr != nil {
			return nil, inferErr
		}
		selection = normalizeBetSelection(game, playCode, selection)
		if err := validateBetChoice(game, playCode, input.Position, selection); err != nil {
			return nil, err
		}
		amountCents, amountErr := validatedStakeCents(input.Amount)
		if amountErr != nil {
			return nil, amountErr
		}
		requestFly, flyErr := requestedFlyAmount(input.FlyAmount)
		if flyErr != nil {
			return nil, flyErr
		}
		prepared = append(prepared, preparedBet{
			input: input, playCode: playCode, playName: playName, selection: selection,
			amountCents: amountCents, requestFly: requestFly,
		})
		var ok bool
		totalCents, ok = safeAddInt64(totalCents, amountCents)
		if !ok || totalCents <= 0 {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "下注总金额过大")
		}
	}

	views := make([]BetView, 0, len(prepared))
	var afterBalance int64
	var roomScope string
	var workspaceID uint64
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := lockBettingIssue(tx, game.ID, issue); err != nil {
			return err
		}
		var account user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, userID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
			}
			return err
		}
		if account.Status != 1 {
			return apperrors.NewBusinessError("USER_DISABLED", "用户已被禁用")
		}
		roomActive, roomErr := accesscontrol.AccountRoomActive(tx, account)
		if roomErr != nil {
			return roomErr
		}
		if !roomActive {
			return apperrors.NewBusinessError("ROOM_UNAVAILABLE", "当前房间已停用，请切换房间后再下注")
		}
		roomGameEnabled, roomGameErr := WorkspaceGameEnabled(tx, account.WorkspaceID, game.ID)
		if roomGameErr != nil {
			return roomGameErr
		}
		if !roomGameEnabled {
			return apperrors.NewBusinessError("GAME_DISABLED", "当前房间暂未开放该游戏")
		}
		financialTerms, termsErr := resolveBetFinancialTerms(tx, account)
		if termsErr != nil {
			return termsErr
		}
		trading := NewTradingAdminService(tx)
		pc28GrayPush := false
		if profile.PC28 > 0 {
			rawSettings, _, settingsErr := readTimingSettings(tx, account.WorkspaceID)
			if settingsErr != nil {
				return settingsErr
			}
			pc28GrayPush = configuredPC28GrayPush(rawSettings, game.ID)
		}
		quotedShapes := make(map[string]float64)
		for index := range prepared {
			item := &prepared[index]
			resolved, resolveErr := trading.ResolveForAccountSelection(
				account, game.ID, item.playCode, item.selection, centsToAmount(item.amountCents), item.input.Odds, financialTerms.flyRequest(item.requestFly),
			)
			if resolveErr != nil {
				return resolveErr
			}
			item.odds = resolved.Odds
			if isFrontThreeShape(item.playCode) && item.odds > quotedShapes[item.playCode] {
				quotedShapes[item.playCode] = item.odds
			}
			resolvedFlyCents, flyErr := nonNegativeMoneyCents(resolved.FlyAmount, "飞单金额")
			if flyErr != nil {
				return flyErr
			}
			item.flyCents = clampFlyCents(item.amountCents, resolvedFlyCents)
			if financialTerms.isRobot {
				item.flyCents = 0
			}
		}
		// Each line checks the current full market. A concurrent admin update
		// can nevertheless leave this batch holding prices from two individually
		// safe markets. Check the actual accepted quotes too, before any debit.
		if len(frontThreeOddsRisks(game.ID, quotedShapes)) > 0 {
			return apperrors.NewBusinessError("ODDS_RISK_UNSAFE", "本次前三形态赔率组合存在风险，请重新选择后提交")
		}
		limitEntries := make([]betLimitEntry, 0, len(prepared))
		for _, item := range prepared {
			limitEntries = append(limitEntries, betLimitEntry{
				PlayCode: item.playCode, Position: item.input.Position, Selection: item.selection, AmountCents: item.amountCents,
			})
		}
		if err := validateBetLimitEntries(tx, game.ID, issue, userID, limitEntries); err != nil {
			return err
		}
		if err := validatePC28PlacementConstraints(tx, profile, account.WorkspaceID, betRoomScope(account), game.ID, issue, userID, limitEntries); err != nil {
			return err
		}
		if account.BalanceCents < totalCents {
			return apperrors.NewBusinessError("INSUFFICIENT_BALANCE", "用户余额不足")
		}
		if err := checkWorkspaceIssueWindow(tx, account.WorkspaceID, game, issue); err != nil {
			return err
		}
		roomScope = betRoomScope(account)
		workspaceID = account.WorkspaceID
		before := account.BalanceCents
		afterBalance = before - totalCents
		if err := tx.Model(&account).Update("balance_cents", afterBalance).Error; err != nil {
			return err
		}
		operator, action := defaultString(inputs[0].Operator, "开奖助手"), "助手下注"
		if strings.TrimSpace(inputs[0].BetMode) == "web" {
			operator, action = defaultString(inputs[0].Operator, "网投"), "网投下注"
		}
		if err := tx.Create(&user.BalanceTransaction{
			WorkspaceID: account.WorkspaceID, UserID: account.UserID, Reference: requestReference,
			AmountCents: -totalCents, BeforeCents: before, AfterCents: afterBalance,
			Type: "bet", Remark: fmt.Sprintf("%s %s/%s（%d 注）", action, game.Name, issue, len(prepared)), Operator: operator,
		}).Error; err != nil {
			return err
		}
		rows := make([]bet.Bet, 0, len(prepared))
		for _, item := range prepared {
			rows = append(rows, bet.Bet{
				WorkspaceID: account.WorkspaceID, GameID: game.ID, Issue: issue, RoomScope: roomScope, UserID: account.UserID, Username: account.Username,
				PlayCode: item.playCode, PlayName: item.playName, Position: item.input.Position, Selection: item.selection, RequestReference: requestReference,
				RuleVersion: profile.Version,
				AmountCents: item.amountCents, Odds: item.odds, ValidTurnoverCents: int64Pointer(item.amountCents), PC28GrayPush: pc28GrayPush,
				Status: "pending", FlyCents: item.flyCents,
				RebateRateSnapshot: financialTerms.rebateRate, AgentShareRateSnapshot: financialTerms.shareRate,
				Remark: strings.TrimSpace(item.input.Remark), Operator: defaultString(item.input.Operator, operator),
			})
		}
		aggregated, indexes, aggregateErr := aggregatePlacementRows(rows)
		if aggregateErr != nil {
			return aggregateErr
		}
		for index := range aggregated {
			if err := tx.Create(&aggregated[index]).Error; err != nil {
				return err
			}
		}
		for _, index := range indexes {
			views = append(views, toBetView(aggregated[index]))
		}
		return nil
	})
	if err != nil {
		if app, ok := err.(*apperrors.AppError); ok {
			return nil, app
		}
		return nil, apperrors.NewSystemError("BET_CREATE_FAILED", "创建注单失败", err)
	}
	if !s.suppressNotifications {
		s.notifyPlacement(userID, workspaceID, roomScope, game.ID, issue, centsToAmount(afterBalance))
	}
	return views, nil
}

func (s *BetAdminService) notifyPlacement(userID, workspaceID uint64, roomScope, gameID, issue string, balance float64) {
	if recipients, recipientsErr := betScopeRecipients(s.db, roomScope); recipientsErr == nil {
		ws.NotifyBetFeed(recipients, workspaceID, gameID, issue, roomScope)
	}
	ws.NotifyUser(userID, "balance", map[string]any{"workspace_id": workspaceID, "balance": balance})
}

// betRoomScope is persisted on the bet, rather than inferred during reads.
// A member may later join another room, but historic betting dynamics must
// remain visible only to the room in which they were placed.
func betRoomScope(account user.User) string {
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

type betFinancialTerms struct {
	isRobot    bool
	rebateRate float64
	shareRate  float64
}

func (terms betFinancialTerms) flyRequest(requested float64) float64 {
	if terms.isRobot {
		return 0
	}
	return requested
}

func resolveBetFinancialTerms(db *gorm.DB, account user.User) (betFinancialTerms, error) {
	var robotCount int64
	if err := robotProfileFinancialQuery(db, account).Count(&robotCount).Error; err != nil {
		return betFinancialTerms{}, err
	}
	if robotCount > 0 {
		// Robot activity is synthetic room traffic. It must never create an
		// external fly order, member rebate, or real agent profit share.
		return betFinancialTerms{isRobot: true}, nil
	}
	var roomOwner user.User
	if account.WorkspaceID > 0 {
		var workspace workspacemodel.Workspace
		if loadErr := db.Select("owner_user_id").First(&workspace, account.WorkspaceID).Error; loadErr != nil {
			return betFinancialTerms{}, loadErr
		}
		if loadErr := db.Select("user_id", "room_rebate_rate", "room_profit_share_rate").First(&roomOwner, workspace.OwnerUserID).Error; loadErr != nil {
			return betFinancialTerms{}, loadErr
		}
	}
	rebateRate, _ := resolveRebate(account, roomOwner.RoomRebateRate)
	return betFinancialTerms{
		rebateRate: clampPercent(rebateRate),
		shareRate:  clampPercent(roomOwner.RoomProfitShareRate),
	}, nil
}

func robotProfileFinancialQuery(db *gorm.DB, account user.User) *gorm.DB {
	return db.Model(&workspacemodel.RobotProfile{}).
		Where("workspace_id = ? AND user_id = ?", account.WorkspaceID, account.UserID)
}

// excludeRobotProfileRows removes synthetic robot activity by the immutable
// workspace/user pair stored in workspace_robot_profiles.  workspaceColumn and
// userColumn are internal, compile-time SQL identifiers supplied by the small
// wrappers below; values still flow through GORM parameters as usual.
func excludeRobotProfileRows(query *gorm.DB, workspaceColumn, userColumn string) *gorm.DB {
	return query.Where(fmt.Sprintf(`NOT EXISTS (
		SELECT 1 FROM workspace_robot_profiles AS robot_profile
		WHERE robot_profile.workspace_id = %s
		  AND robot_profile.user_id = %s
	)`, workspaceColumn, userColumn))
}

func excludeRobotProfileBets(query *gorm.DB) *gorm.DB {
	return excludeRobotProfileRows(query, "lottery_bets.workspace_id", "lottery_bets.user_id")
}

func excludeRobotProfileUsers(query *gorm.DB) *gorm.DB {
	return excludeRobotProfileRows(query, `"user".workspace_id`, `"user".user_id`)
}

func excludeRobotProfileLedgers(query *gorm.DB) *gorm.DB {
	return excludeRobotProfileRows(query, "user_balance_transactions.workspace_id", "user_balance_transactions.user_id")
}

func weightedRate(oldRate float64, oldAmount int64, newRate float64, newAmount int64) float64 {
	total := float64(oldAmount) + float64(newAmount)
	if total <= 0 || math.IsNaN(total) || math.IsInf(total, 0) {
		return clampPercent(newRate)
	}
	return clampPercent((oldRate*float64(oldAmount) + newRate*float64(newAmount)) / total)
}

func betScopeRecipients(db *gorm.DB, scope string) ([]uint64, error) {
	query := db.Model(&user.User{}).Where("status = ?", 1)
	if strings.HasPrefix(scope, "agent:") {
		id, err := strconv.ParseUint(strings.TrimPrefix(scope, "agent:"), 10, 64)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("invalid bet room scope")
		}
		query = query.Where("user_id = ? OR parent_agent_id = ?", id, id)
	} else if strings.HasPrefix(scope, "tenant:") {
		id, err := strconv.ParseUint(strings.TrimPrefix(scope, "tenant:"), 10, 64)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("invalid bet room scope")
		}
		query = query.Where("user_id = ? OR (parent_tenant_id = ? AND parent_agent_id IS NULL)", id, id)
	} else if scope == "lobby" {
		query = query.Where("parent_agent_id IS NULL AND role <> ?", "agent")
	} else {
		return nil, fmt.Errorf("invalid bet room scope")
	}
	var ids []uint64
	return ids, query.Pluck("user_id", &ids).Error
}

func (s *BetAdminService) Monitor(gameID, issue string) (*MonitorSnapshot, error) {
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		var game lottery.Game
		if err := s.db.Where("enabled = ?", true).Order("sort_order asc").First(&game).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				if err := s.db.Order("sort_order asc").First(&game).Error; err != nil {
					return nil, apperrors.NewBusinessError("GAME_NOT_FOUND", "暂无可用游戏")
				}
			} else {
				return nil, apperrors.NewSystemError("GAME_READ_FAILED", "读取游戏失败", err)
			}
		}
		gameID = game.ID
	}
	game, err := s.loadGame(gameID)
	if err != nil {
		return nil, err
	}
	issue = strings.TrimSpace(issue)
	if issue == "" {
		issue, err = s.CurrentIssue(game.ID)
		if err != nil {
			return nil, err
		}
	}
	var rows []bet.Bet
	if err := s.db.Where("game_id = ? AND issue = ? AND status IN ?", game.ID, issue, []string{"pending", "won", "lost"}).Find(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("MONITOR_READ_FAILED", "读取现场监控失败", err)
	}
	numberRows := 10
	positionColumns := 6
	if maxBetPosition(game) == 10 {
		numberRows = 11
		positionColumns = 10
	}
	matrix := make([][]float64, numberRows)
	for i := range matrix {
		matrix[i] = make([]float64, positionColumns)
	}
	var total int64
	users := map[uint64]struct{}{}
	for _, row := range rows {
		total += row.AmountCents
		users[row.UserID] = struct{}{}
		digit, ok := parseDigit(row.Selection)
		if !ok || digit >= len(matrix) || row.Position < 1 || row.Position > positionColumns {
			continue
		}
		matrix[digit][row.Position-1] += centsToAmount(row.AmountCents)
	}
	drawLabel := game.NextDrawAt.In(time.FixedZone("CST", 8*3600)).Format("15:04:05")
	snapshot := &MonitorSnapshot{
		GameID: game.ID, GameName: game.Name, Issue: issue,
		TotalAmount: centsToAmount(total), BettorCount: int64(len(users)), BetCount: int64(len(rows)),
		NextDrawAt: game.NextDrawAt, DrawAtLabel: drawLabel, Matrix: matrix, UpdatedAt: time.Now().UTC(),
	}
	if status, statusErr := s.SettlementStatus(game.ID, issue); statusErr == nil {
		snapshot.Settlement = status
	}
	return snapshot, nil
}

type BoardReportFilter struct {
	GameID   string
	Query    string
	Page     int
	PageSize int
}

func (s *BetAdminService) BoardReport(filter BoardReportFilter) (*BoardReport, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	type aggRow struct {
		GameID     string
		Issue      string
		BetCount   int64
		TotalCents int64
		FlyCents   int64
		PendingCnt int64
	}
	var aggs []aggRow
	var total int64
	var pairs []struct {
		GameID string
		Issue  string
	}
	pairQuery := excludeRobotProfileBets(s.db.Model(&bet.Bet{})).Select("game_id, issue")
	if gid := strings.TrimSpace(filter.GameID); gid != "" && gid != "all" {
		pairQuery = pairQuery.Where("game_id = ?", gid)
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		pairQuery = pairQuery.Where("issue ILIKE ?", "%"+q+"%")
	}
	if err := pairQuery.Group("game_id, issue").Scan(&pairs).Error; err != nil {
		return nil, apperrors.NewSystemError("BOARD_READ_FAILED", "读取打盘报表失败", err)
	}
	total = int64(len(pairs))
	query := excludeRobotProfileBets(s.db.Model(&bet.Bet{})).
		Select("game_id, issue, COUNT(*) as bet_count, COALESCE(SUM(amount_cents),0) as total_cents, COALESCE(SUM(fly_cents),0) as fly_cents, SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) as pending_cnt").
		Group("game_id, issue")
	if gid := strings.TrimSpace(filter.GameID); gid != "" && gid != "all" {
		query = query.Where("game_id = ?", gid)
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		query = query.Where("issue ILIKE ?", "%"+q+"%")
	}
	if err := query.Order("issue desc").Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Scan(&aggs).Error; err != nil {
		return nil, apperrors.NewSystemError("BOARD_READ_FAILED", "读取打盘报表失败", err)
	}
	names := map[string]string{}
	var games []lottery.Game
	_ = s.db.Find(&games).Error
	for _, g := range games {
		names[g.ID] = g.Name
	}
	items := make([]BoardReportRow, 0, len(aggs))
	for _, row := range aggs {
		status := "已完成"
		if row.PendingCnt > 0 {
			status = "待结算"
		} else if row.BetCount > 0 {
			status = "已结算"
		}
		var draw lottery.Draw
		drawErr := s.db.Where("game_id = ? AND issue = ?", row.GameID, row.Issue).First(&draw).Error
		item := BoardReportRow{
			GameID: row.GameID, GameName: defaultString(names[row.GameID], row.GameID), Issue: row.Issue,
			BetCount: row.BetCount, TotalAmount: centsToAmount(row.TotalCents), FlyAmount: centsToAmount(row.FlyCents),
			Status: status, DrawResult: "",
		}
		if drawErr == nil {
			item.DrawAt = &draw.DrawAt
			item.DrawResult = draw.Numbers
			if row.PendingCnt > 0 {
				item.Status = "已开奖待结算"
			} else {
				item.Status = "已开奖已结算"
			}
		}
		items = append(items, item)
	}
	return &BoardReport{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

type BetListFilter struct {
	Query       string
	GameID      string
	Issue       string
	UserID      uint64
	WorkspaceID uint64
	Status      string
	BeforeID    uint64
	Page        int
	PageSize    int
}

type BetListResult struct {
	Items        []BetView `json:"items"`
	Total        int64     `json:"total"`
	Page         int       `json:"page"`
	PageSize     int       `json:"page_size"`
	HasMore      bool      `json:"has_more"`
	NextBeforeID uint64    `json:"next_before_id,omitempty"`
}

func (s *BetAdminService) List(filter BetListFilter) (*BetListResult, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	query := s.db.Model(&bet.Bet{})
	if filter.WorkspaceID > 0 {
		query = query.Where("workspace_id = ?", filter.WorkspaceID)
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		like := "%" + q + "%"
		query = query.Where("username ILIKE ? OR issue ILIKE ? OR selection ILIKE ? OR play_name ILIKE ?", like, like, like, like)
	}
	if gameID := strings.TrimSpace(filter.GameID); gameID != "" && gameID != "all" {
		query = query.Where("game_id = ?", gameID)
	}
	if issue := strings.TrimSpace(filter.Issue); issue != "" {
		query = query.Where("issue = ?", issue)
	}
	if filter.UserID > 0 {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if st := strings.TrimSpace(filter.Status); st != "" && st != "all" {
		if st == "settled" {
			query = query.Where("status IN ?", []string{"won", "lost", "push", "cancelled"})
		} else {
			query = query.Where("status = ?", st)
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, apperrors.NewSystemError("BET_READ_FAILED", "读取注单失败", err)
	}
	if filter.BeforeID > 0 {
		query = query.Where("id < ?", filter.BeforeID)
	}
	var rows []bet.Bet
	limit := filter.PageSize
	offset := (filter.Page - 1) * filter.PageSize
	if filter.BeforeID > 0 {
		limit++
		offset = 0
	}
	if err := query.Order("id desc").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("BET_READ_FAILED", "读取注单失败", err)
	}
	hasMore := filter.Page*filter.PageSize < int(total)
	if filter.BeforeID > 0 {
		hasMore = len(rows) > filter.PageSize
		if hasMore {
			rows = rows[:filter.PageSize]
		}
	}
	items := make([]BetView, 0, len(rows))
	for _, row := range rows {
		items = append(items, toBetView(row))
	}
	nextBeforeID := uint64(0)
	if len(items) > 0 {
		nextBeforeID = items[len(items)-1].ID
	}
	return &BetListResult{
		Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize,
		HasMore: hasMore, NextBeforeID: nextBeforeID,
	}, nil
}

func (s *BetAdminService) Cancel(id uint64, operator string) (*BetView, error) {
	return s.cancel(id, nil, operator)
}

// CancelOwned is the member boundary. Ownership is part of the same locked
// query as the refund, so a separate pre-check cannot drift from the mutation.
func (s *BetAdminService) CancelOwned(id, userID uint64, operator string) (*BetView, error) {
	return s.cancel(id, &userID, operator)
}

func (s *BetAdminService) cancel(id uint64, ownerUserID *uint64, operator string) (*BetView, error) {
	var view BetView
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var row bet.Bet
		if ownerUserID != nil {
			// Read only the member-owned target first so we can take the issue lock
			// before the mutable bet/account locks. This matches placement and bulk
			// cancellation lock order and makes a post-seal member refund impossible.
			if err := tx.Where("id = ? AND user_id = ?", id, *ownerUserID).First(&row).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					return apperrors.NewBusinessError("NOT_FOUND", "注单不存在")
				}
				return err
			}
			if err := lockAcceptingIssue(tx, row.GameID, row.Issue); err != nil {
				return err
			}
			row = bet.Bet{}
		}
		owned := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id)
		if ownerUserID != nil {
			owned = owned.Where("user_id = ?", *ownerUserID)
		}
		if err := owned.First(&row).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("NOT_FOUND", "注单不存在")
			}
			return err
		}
		if row.Status != "pending" {
			return apperrors.NewBusinessError("INVALID_REQUEST", "仅待结算注单可撤单")
		}
		if row.AmountCents <= 0 {
			return apperrors.NewBusinessError("INVALID_REQUEST", "注单金额异常，无法撤单")
		}
		var account user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, row.UserID).Error; err != nil {
			return apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
		}
		if ownerUserID != nil {
			game, err := NewBetAdminService(tx).loadGame(row.GameID)
			if err != nil {
				return err
			}
			// Use the bet's frozen workspace, not a member's later room switch.
			if err := checkWorkspaceIssueWindow(tx, row.WorkspaceID, game, row.Issue); err != nil {
				return err
			}
		}
		before := account.BalanceCents
		after, ok := safeAddInt64(before, row.AmountCents)
		if !ok || after < 0 {
			return apperrors.NewBusinessError("INVALID_REQUEST", "退款后余额超出系统范围")
		}
		if err := tx.Model(&account).Update("balance_cents", after).Error; err != nil {
			return err
		}
		if err := tx.Create(&user.BalanceTransaction{
			WorkspaceID: row.WorkspaceID, UserID: account.UserID, Reference: fmt.Sprintf("bet_cancel:%d", row.ID),
			AmountCents: row.AmountCents, BeforeCents: before, AfterCents: after,
			Type: "bet_cancel", Remark: fmt.Sprintf("撤单 #%d %s/%s", row.ID, row.GameID, row.Issue),
			Operator: defaultString(operator, "后台管理员"),
		}).Error; err != nil {
			return err
		}
		row.Status = "cancelled"
		row.Operator = defaultString(operator, row.Operator)
		row.Remark = strings.TrimSpace(row.Remark + " | 已撤单")
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		view = toBetView(row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &view, nil
}

// CancelCurrentIssue atomically cancels all of one member's pending bets in
// the current accepting issue and refunds their combined stake exactly once.
// New clients freeze the issue they confirmed; legacy callers may omit it.
func (s *BetAdminService) CancelCurrentIssue(userID uint64, gameID, operator string, expectedIssues ...string) (*CancelIssueResult, error) {
	expectedIssue := ""
	if len(expectedIssues) > 0 {
		expectedIssue = strings.TrimSpace(expectedIssues[0])
	}
	result := &CancelIssueResult{}
	roomScope := ""
	workspaceID := uint64(0)
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Resolve and validate one target while retaining the same Game lock as
		// placement/source synchronization. Never re-resolve after comparison.
		game, err := lockBettingGame(tx, gameID)
		if err != nil {
			return err
		}
		target, err := NewBetAdminService(tx).bettingIssueForGame(game, nil)
		if err != nil {
			return err
		}
		issue := target.Issue
		if expectedIssue != "" && expectedIssue != issue {
			return apperrors.NewBusinessError("ISSUE_MISMATCH", "期号已变更，请核对最新一期后再撤单")
		}
		result.GameID, result.Issue = game.ID, issue
		// Serializes cancellation with placement and rejects a click that arrives
		// after sealing. Settlement can therefore never race a member refund.
		if err := lockAcceptingIssue(tx, game.ID, issue); err != nil {
			return err
		}
		var rows []bet.Bet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND game_id = ? AND issue = ? AND status = ?", userID, game.ID, issue, "pending").
			Order("id asc").Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return apperrors.NewBusinessError("NO_PENDING_BETS", "本期没有可撤回的注单")
		}

		var account user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, userID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
			}
			return err
		}
		var refundCents int64
		ids := make([]uint64, 0, len(rows))
		checkedWorkspaces := make(map[uint64]bool)
		for _, row := range rows {
			if !checkedWorkspaces[row.WorkspaceID] {
				if err := checkWorkspaceIssueWindow(tx, row.WorkspaceID, game, issue); err != nil {
					return err
				}
				checkedWorkspaces[row.WorkspaceID] = true
			}
			if row.AmountCents <= 0 {
				return apperrors.NewBusinessError("INVALID_REQUEST", "注单金额异常，无法撤回")
			}
			var ok bool
			refundCents, ok = safeAddInt64(refundCents, row.AmountCents)
			if !ok || refundCents <= 0 {
				return apperrors.NewBusinessError("INVALID_REQUEST", "撤回金额超出系统范围")
			}
			ids = append(ids, row.ID)
			if roomScope == "" {
				roomScope = row.RoomScope
				workspaceID = row.WorkspaceID
			}
		}
		before := account.BalanceCents
		after, ok := safeAddInt64(before, refundCents)
		if !ok || after < 0 {
			return apperrors.NewBusinessError("INVALID_REQUEST", "退款后余额超出系统范围")
		}
		if err := tx.Model(&account).Update("balance_cents", after).Error; err != nil {
			return err
		}
		if err := tx.Create(&user.BalanceTransaction{
			WorkspaceID: workspaceID, UserID: account.UserID, Reference: fmt.Sprintf("bet_cancel_issue:%d:%s:%s:%d", userID, game.ID, issue, time.Now().UTC().UnixNano()),
			AmountCents: refundCents, BeforeCents: before, AfterCents: after,
			Type: "bet_cancel", Remark: fmt.Sprintf("撤回本期 %s/%s（%d 注）", game.Name, issue, len(rows)),
			Operator: defaultString(operator, account.Username),
		}).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		updated := tx.Model(&bet.Bet{}).Where("id IN ? AND status = ?", ids, "pending").Updates(map[string]any{
			"status": "cancelled", "operator": defaultString(operator, account.Username),
			"remark": "会员撤回本期注单", "updated_at": now,
		})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != int64(len(rows)) {
			return apperrors.NewBusinessError("BET_STATUS_CHANGED", "注单状态已变化，请刷新后重试")
		}
		result.Count = len(rows)
		result.Refund = centsToAmount(refundCents)
		result.Balance = centsToAmount(after)
		return nil
	})
	if err != nil {
		if app, ok := err.(*apperrors.AppError); ok {
			return nil, app
		}
		return nil, apperrors.NewSystemError("BET_CANCEL_FAILED", "撤回本期注单失败", err)
	}
	if recipients, recipientsErr := betScopeRecipients(s.db, roomScope); recipientsErr == nil {
		ws.NotifyBetFeed(recipients, workspaceID, result.GameID, result.Issue, roomScope)
	}
	ws.NotifyUser(userID, "balance", map[string]any{"workspace_id": workspaceID, "balance": result.Balance})
	return result, nil
}

func (s *BetAdminService) DashboardStats() (*DashboardStats, error) {
	var totalBalance int64
	if err := excludeRobotProfileUsers(s.db.Model(&user.User{})).Select("COALESCE(SUM(balance_cents),0)").Scan(&totalBalance).Error; err != nil {
		return nil, err
	}
	start := startOfDayCST(time.Now())
	var todayStake, pending int64
	type profitAggregate struct {
		TurnoverCents   int64 `gorm:"column:turnover_cents"`
		PayoutCents     int64 `gorm:"column:payout_cents"`
		RebateCents     int64 `gorm:"column:rebate_cents"`
		AgentShareCents int64 `gorm:"column:agent_share_cents"`
	}
	var today, all profitAggregate
	realBets := func() *gorm.DB {
		return excludeRobotProfileBets(s.db.Model(&bet.Bet{}))
	}
	if err := realBets().Where("created_at >= ? AND status <> ?", start, "cancelled").
		Select("COALESCE(SUM(amount_cents),0)").Scan(&todayStake).Error; err != nil {
		return nil, err
	}
	if err := realBets().Where("status IN ?", []string{"won", "lost"}).
		Where("COALESCE(settled_at,updated_at,created_at) >= ?", start).
		Select(`COALESCE(SUM(amount_cents),0) turnover_cents,
			COALESCE(SUM(payout_cents),0) payout_cents,
			COALESCE(SUM(rebate_cents),0) rebate_cents,
			COALESCE(SUM(agent_share_cents),0) agent_share_cents`).Scan(&today).Error; err != nil {
		return nil, err
	}
	if err := realBets().Where("status = ?", "pending").Select("COALESCE(SUM(amount_cents),0)").Scan(&pending).Error; err != nil {
		return nil, err
	}
	if err := realBets().Where("status IN ?", []string{"won", "lost"}).
		Select(`COALESCE(SUM(amount_cents),0) turnover_cents,
			COALESCE(SUM(payout_cents),0) payout_cents,
			COALESCE(SUM(rebate_cents),0) rebate_cents,
			COALESCE(SUM(agent_share_cents),0) agent_share_cents`).Scan(&all).Error; err != nil {
		return nil, err
	}
	todayWelfare, err := s.welfareCostSince(start)
	if err != nil {
		todayWelfare = 0
	}
	allWelfare, err := s.welfareCostSince(time.Time{})
	if err != nil {
		allWelfare = 0
	}
	todayGrossCents := today.TurnoverCents - today.PayoutCents
	totalGrossCents := all.TurnoverCents - all.PayoutCents
	todayGross := centsToAmount(todayGrossCents)
	totalGross := centsToAmount(totalGrossCents)
	todayRebate := centsToAmount(today.RebateCents)
	totalRebate := centsToAmount(all.RebateCents)
	todayAgentShare := centsToAmount(today.AgentShareCents)
	totalAgentShare := centsToAmount(all.AgentShareCents)
	return &DashboardStats{
		UserBalance:          centsToAmount(totalBalance),
		TodayTurnover:        centsToAmount(todayStake),
		TodaySettledTurnover: centsToAmount(today.TurnoverCents),
		TodayGrossProfit:     todayGross,
		TodayNetProfit:       todayGross - todayRebate - todayWelfare - todayAgentShare,
		TodayRebate:          todayRebate,
		TodayWelfare:         todayWelfare,
		TodayAgentShare:      todayAgentShare,
		TotalGrossProfit:     totalGross,
		TotalNetProfit:       totalGross - totalRebate - allWelfare - totalAgentShare,
		TotalRebate:          totalRebate,
		TotalWelfare:         allWelfare,
		TotalAgentShare:      totalAgentShare,
		PendingSettlement:    centsToAmount(pending),
		TodayProfit:          todayGross,
		TotalProfit:          totalGross,
	}, nil
}

func (s *BetAdminService) welfareCostSince(start time.Time) (float64, error) {
	query := excludeRobotProfileLedgers(s.db.Model(&user.BalanceTransaction{})).
		Where("type IN ? AND amount_cents > 0", []string{"checkin", "redpacket", "invite"})
	if !start.IsZero() {
		query = query.Where("created_at >= ?", start)
	}
	var cents int64
	if err := query.Select("COALESCE(SUM(amount_cents),0)").Scan(&cents).Error; err != nil {
		return 0, err
	}
	return centsToAmount(cents), nil
}

func (s *BetAdminService) GameMoneyMap() (map[string]gameMoney, error) {
	start := startOfDayCST(time.Now())
	type row struct {
		GameID            string
		StakeCents        int64
		SettledStakeCents int64
		PayoutCents       int64
	}
	var today []row
	if err := excludeRobotProfileBets(s.db.Model(&bet.Bet{})).
		Select("game_id, COALESCE(SUM(CASE WHEN status NOT IN ('cancelled','push') THEN amount_cents ELSE 0 END),0) as stake_cents, COALESCE(SUM(CASE WHEN status IN ('won','lost') THEN amount_cents ELSE 0 END),0) as settled_stake_cents, COALESCE(SUM(CASE WHEN status IN ('won','lost') THEN payout_cents ELSE 0 END),0) as payout_cents").
		Where("created_at >= ?", start).
		Group("game_id").Scan(&today).Error; err != nil {
		return nil, err
	}
	result := map[string]gameMoney{}
	for _, item := range today {
		gross := centsToAmount(item.SettledStakeCents - item.PayoutCents)
		result[item.GameID] = gameMoney{
			GameID:      item.GameID,
			Turnover:    centsToAmount(item.StakeCents),
			GrossProfit: gross,
			Profit:      gross,
		}
	}
	return result, nil
}

func (s *BetAdminService) CurrentIssue(gameID string) (string, error) {
	game, err := s.loadGame(gameID)
	if err != nil {
		return "", err
	}
	return s.currentIssueForGame(game)
}

// Keep a source-provided issue paired with the schedule on this exact game
// snapshot. Re-reading just the issue can pair the next sync's issue with the
// previous sync's draw timestamp in a concurrent request.
func (s *BetAdminService) currentIssueForGame(game *lottery.Game) (string, error) {
	if game == nil {
		return "", apperrors.NewBusinessError("GAME_NOT_FOUND", "游戏不存在")
	}
	if issue := strings.TrimSpace(game.NextIssue); issue != "" {
		return issue, nil
	}
	var draw lottery.Draw
	err := trustedDrawsForGame(s.db, game.ID).Order("draw_at desc").First(&draw).Error
	if err == nil && strings.TrimSpace(draw.Issue) != "" {
		if game.SourceKind == "external" || game.SourceKind == "official" {
			return inferredNextSourceIssue(draw.Issue, game.NextDrawAt), nil
		}
		return nextIssue(draw.Issue), nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return "", apperrors.NewSystemError("DRAW_READ_FAILED", "读取开奖期号失败", err)
	}
	if game.SourceKind == "external" || game.SourceKind == "official" {
		return "", nil // No fixture/time-derived issue is valid for an external feed.
	}
	return initialPlatformIssue(game.NextDrawAt), nil
}

func (s *BetAdminService) loadGame(gameID string) (*lottery.Game, error) {
	id := strings.TrimSpace(gameID)
	if id == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "请选择游戏")
	}
	var game lottery.Game
	if err := s.db.First(&game, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("GAME_NOT_FOUND", "游戏不存在")
		}
		return nil, apperrors.NewSystemError("GAME_READ_FAILED", "读取游戏失败", err)
	}
	return &game, nil
}

func maxBetPosition(game *lottery.Game) int {
	if profile, ok := rulesForGame(game); ok {
		return profile.BallCount
	}
	return 0
}

func validBetPosition(game *lottery.Game, playCode string, position int) bool {
	profile, ok := rulesForGame(game)
	if !ok || !profile.supportsPlay(playCode) {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(playCode), "sum") {
		return position == 6
	}
	if profile.PC28 > 0 {
		return profile.validateChoice(playCode, position, pc28DefaultValidationSelection(playCode)) == nil
	}
	return position >= 1 && position <= profile.BallCount
}

func settlementPositionCount(game *lottery.Game) int {
	return maxBetPosition(game)
}

// validateBetChoice is the server-side allowlist shared by direct and assistant
// betting. Settlement must never reinterpret an arbitrary code/selection: in
// particular a shape bet may not name a different (higher probability) shape
// while retaining the submitted play's odds.
func validateBetChoice(game *lottery.Game, playCode string, position int, selection string) error {
	profile, ok := rulesForGame(game)
	if !ok {
		return ensureGameRulesSupported(game)
	}
	return profile.validateChoice(playCode, position, selection)
}

func ensurePlacementBetMode(profile gameRuleProfile, mode string) error {
	if profile.MarkSix && strings.TrimSpace(mode) != "web" {
		return apperrors.NewBusinessError("BET_MODE_UNAVAILABLE", "宾果六合彩仅支持网投，不支持聊天或通用单注入口")
	}
	return nil
}

func normalizeBetSelection(game *lottery.Game, playCode, selection string) string {
	selection = strings.TrimSpace(selection)
	if profile, ok := rulesForGame(game); ok {
		if profile.MarkSix {
			return markSixNormalizeSelection(playCode, selection)
		}
		if profile.PC28 > 0 {
			return pc28NormalizeSelection(playCode, selection)
		}
	}
	switch strings.ToLower(strings.TrimSpace(playCode)) {
	case "ball_1_5":
		if number, err := strconv.Atoi(selection); err == nil {
			if maxBetPosition(game) == 10 && number == 0 {
				return "10"
			}
			return strconv.Itoa(number)
		}
	case "two_sided", "dragon_tiger", "dragon_tiger_tie":
		return selectionLabel(normalizeSelection(selection))
	case "sum":
		if isSideSelection(selection) {
			return selectionLabel(normalizeSelection(selection))
		}
		if total, err := strconv.Atoi(selection); err == nil {
			return strconv.Itoa(total)
		}
	case "leopard", "straight", "pair", "half_straight", "mixed":
		code := strings.ToLower(strings.TrimSpace(playCode))
		normalized := normalizePlaySelection(selection)
		if normalized == "yes" || normalized == "中" || normalized == code {
			return code
		}
		return normalized
	}
	return selection
}

func toBetView(row bet.Bet) BetView {
	view := BetView{
		ID: row.ID, GameID: row.GameID, Issue: row.Issue, UserID: row.UserID, Username: row.Username,
		PlayCode: row.PlayCode, PlayName: row.PlayName, Position: row.Position, Selection: row.Selection,
		RuleVersion: row.RuleVersion,
		Amount:      centsToAmount(row.AmountCents), Odds: row.Odds, SettlementOdds: row.SettlementOdds,
		SettlementPolicy: row.SettlementPolicy, Status: row.Status,
		Payout: centsToAmount(row.PayoutCents), FlyAmount: centsToAmount(row.FlyCents),
		Remark: row.Remark, Operator: row.Operator, CreatedAt: row.CreatedAt,
	}
	if row.ValidTurnoverCents != nil {
		value := centsToAmount(*row.ValidTurnoverCents)
		view.ValidTurnover = &value
	}
	if row.UserIssueStakeCentsSnapshot != nil {
		value := centsToAmount(*row.UserIssueStakeCentsSnapshot)
		view.UserIssueStake = &value
	}
	return view
}

func int64Pointer(value int64) *int64 { return &value }

func parseDigit(value string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 0 || n > 10 {
		return 0, false
	}
	return n, true
}

func nextIssue(issue string) string {
	trimmed := strings.TrimSpace(issue)
	if trimmed == "" {
		return time.Now().Format("200601021504")
	}
	// Numeric suffix +1 when possible; otherwise append "-next".
	i := len(trimmed) - 1
	for i >= 0 && trimmed[i] >= '0' && trimmed[i] <= '9' {
		i--
	}
	prefix, digits := trimmed[:i+1], trimmed[i+1:]
	if digits == "" {
		return trimmed + "-next"
	}
	n, err := strconv.ParseUint(digits, 10, 64)
	if err != nil {
		return trimmed + "-next"
	}
	next := strconv.FormatUint(n+1, 10)
	if len(next) < len(digits) {
		next = strings.Repeat("0", len(digits)-len(next)) + next
	}
	return prefix + next
}

func lockAcceptingIssue(db *gorm.DB, gameID, issue string) error {
	var row lottery.Issue
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("game_id = ? AND issue = ?", gameID, issue).First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NewBusinessError("ISSUE_MISMATCH", "期号已变更，请刷新页面后重试")
		}
		return err
	}
	if row.Status == lottery.IssueStatusError {
		return apperrors.NewBusinessError("SOURCE_UNAVAILABLE", "开奖数据暂时异常，本期已暂停投注")
	}
	if !sharedIssueOpen(&row, time.Now().UTC()) {
		return apperrors.NewBusinessError("ISSUE_CLOSED", "当前期已封盘，请等待下一期")
	}
	return nil
}

// validateBetLimitEntries checks the complete ticket against one locked member
// snapshot.  The caller must hold the member row lock.  This prevents two
// concurrent requests, and repeated selections inside one assistant ticket,
// from each passing a stale per-period limit check.
func validateBetLimitEntries(db *gorm.DB, gameID, issue string, userID uint64, entries []betLimitEntry) error {
	limits, err := NewOddsAdminService(db).Get(gameID)
	if err != nil {
		return err
	}
	limitByPlay := make(map[string]PlayLimitItem, len(limits.Items))
	for i := range limits.Items {
		limitByPlay[limits.Items[i].PlayCode] = limits.Items[i]
	}
	type lineKey struct {
		PricingCode string
		PlayCode    string
		Position    int
		Selection   string
	}
	type marketKey struct {
		PricingCode string
		PlayCode    string
		// Selection is populated only when one public play has independently
		// priced outcomes (currently Bingo Racing A crown sum).
		Selection string
	}
	lineDeltas := make(map[lineKey]int64)
	playDeltas := make(map[marketKey]int64)
	for _, entry := range entries {
		pricingCode, pricingErr := oddsPricingCode(gameID, entry.PlayCode, entry.Selection)
		if pricingErr != nil {
			return pricingErr
		}
		limit, ok := limitByPlay[pricingCode]
		if !ok {
			continue
		}
		if limit.MinBet > 0 && centsToAmount(entry.AmountCents) < limit.MinBet {
			return apperrors.NewBusinessError("BET_TOO_SMALL", fmt.Sprintf("单注最低 %.2f 元", limit.MinBet))
		}
		key := lineKey{PricingCode: pricingCode, PlayCode: entry.PlayCode, Position: entry.Position, Selection: entry.Selection}
		lineTotal, ok := safeAddInt64(lineDeltas[key], entry.AmountCents)
		if !ok || lineTotal < 0 {
			return apperrors.NewBusinessError("INVALID_REQUEST", "单注累计金额过大")
		}
		market := marketKey{PricingCode: pricingCode, PlayCode: entry.PlayCode}
		if pricingCode != entry.PlayCode {
			market.Selection = entry.Selection
		}
		playTotal, ok := safeAddInt64(playDeltas[market], entry.AmountCents)
		if !ok || playTotal < 0 {
			return apperrors.NewBusinessError("INVALID_REQUEST", "玩法累计金额过大")
		}
		lineDeltas[key] = lineTotal
		playDeltas[market] = playTotal
	}
	for key, delta := range lineDeltas {
		limit := limitByPlay[key.PricingCode]
		if limit.MaxBet <= 0 {
			continue
		}
		var existingCents int64
		if err := db.Model(&bet.Bet{}).Where(
			"game_id = ? AND issue = ? AND user_id = ? AND play_code = ? AND position = ? AND selection = ? AND status != ?",
			gameID, issue, userID, key.PlayCode, key.Position, key.Selection, "cancelled",
		).Select("COALESCE(SUM(amount_cents),0)").Scan(&existingCents).Error; err != nil {
			return err
		}
		combined, ok := safeAddInt64(existingCents, delta)
		if !ok || combined < 0 {
			return apperrors.NewBusinessError("INVALID_REQUEST", "单注累计金额超出系统范围")
		}
		if centsToAmount(combined) > limit.MaxBet {
			return apperrors.NewBusinessError("BET_TOO_LARGE", fmt.Sprintf("单注最高 %.2f 元", limit.MaxBet))
		}
	}
	for market, delta := range playDeltas {
		limit := limitByPlay[market.PricingCode]
		if limit.MaxUserPeriod > 0 {
			var userPeriodCents int64
			query := db.Model(&bet.Bet{}).Where(
				"game_id = ? AND issue = ? AND user_id = ? AND play_code = ? AND status != ?",
				gameID, issue, userID, market.PlayCode, "cancelled",
			)
			if market.Selection != "" {
				query = query.Where("selection = ?", market.Selection)
			}
			if err := query.Select("COALESCE(SUM(amount_cents),0)").Scan(&userPeriodCents).Error; err != nil {
				return err
			}
			combined, ok := safeAddInt64(userPeriodCents, delta)
			if !ok || combined < 0 {
				return apperrors.NewBusinessError("INVALID_REQUEST", "本期用户累计金额超出系统范围")
			}
			if centsToAmount(combined) > limit.MaxUserPeriod {
				return apperrors.NewBusinessError("PERIOD_LIMIT", fmt.Sprintf("本期该玩法限额 %.2f 元", limit.MaxUserPeriod))
			}
		}
		if limit.MaxPeriodTotal > 0 {
			var totalPeriodCents int64
			query := db.Model(&bet.Bet{}).Where(
				"game_id = ? AND issue = ? AND play_code = ? AND status != ?",
				gameID, issue, market.PlayCode, "cancelled",
			)
			if market.Selection != "" {
				query = query.Where("selection = ?", market.Selection)
			}
			if err := query.Select("COALESCE(SUM(amount_cents),0)").Scan(&totalPeriodCents).Error; err != nil {
				return err
			}
			combined, ok := safeAddInt64(totalPeriodCents, delta)
			if !ok || combined < 0 {
				return apperrors.NewBusinessError("INVALID_REQUEST", "本期玩法累计金额超出系统范围")
			}
			if centsToAmount(combined) > limit.MaxPeriodTotal {
				return apperrors.NewBusinessError("GAME_PERIOD_LIMIT", fmt.Sprintf("本期该玩法总限额 %.2f 元", limit.MaxPeriodTotal))
			}
		}
	}
	return nil
}

func (s *BetAdminService) ensureIssueOpen(game *lottery.Game, issue string) error {
	if !sourceHealthyForGame(game) {
		return apperrors.NewBusinessError("SOURCE_UNAVAILABLE", "开奖数据暂时异常，本期已暂停投注")
	}
	lifecycle, err := s.bettingIssueForGame(game, nil)
	if err != nil {
		return err
	}
	if strings.TrimSpace(issue) != lifecycle.Issue {
		return apperrors.NewBusinessError("ISSUE_MISMATCH", "期号已变更，请刷新页面后重试")
	}
	if !sharedIssueOpen(lifecycle, time.Now().UTC()) {
		if lifecycle.Status == lottery.IssueStatusError {
			return apperrors.NewBusinessError("SOURCE_UNAVAILABLE", "开奖数据暂时异常，本期已暂停投注")
		}
		return apperrors.NewBusinessError("ISSUE_CLOSED", "当前期已封盘，请等待下一期")
	}
	return nil
}

func startOfDayCST(now time.Time) time.Time {
	loc := time.FixedZone("CST", 8*3600)
	t := now.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).UTC()
}
