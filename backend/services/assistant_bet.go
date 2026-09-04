package services

import (
	"backend/accesscontrol"
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/data/models/user"
	apperrors "backend/errors"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AssistantBetLine is the server-authoritative explanation of one parsed bet.
// The amount suffix is the amount of each expanded selection. For example,
// 1/123/100 expands to three lines of 100 and a total debit of 300.
type AssistantBetLine struct {
	Position  int     `json:"position"`
	Selection string  `json:"selection"`
	PlayCode  string  `json:"play_code"`
	PlayName  string  `json:"play_name"`
	Amount    float64 `json:"amount"`
	Odds      float64 `json:"odds"`
	Label     string  `json:"label"`
}

type AssistantBetResult struct {
	GameID      string             `json:"game_id"`
	GameName    string             `json:"game_name"`
	RuleVersion string             `json:"rule_version,omitempty"`
	Issue       string             `json:"issue"`
	Content     string             `json:"content"`
	Lines       []AssistantBetLine `json:"lines"`
	BetCount    int                `json:"bet_count"`
	Total       float64            `json:"total"`
	Balance     float64            `json:"balance"`
	AcceptedAt  time.Time          `json:"accepted_at"`
}

type AssistantDrawStatus struct {
	GameID        string         `json:"game_id"`
	GameName      string         `json:"game_name"`
	Issue         string         `json:"issue"`
	Accepting     bool           `json:"accepting"`
	NextDrawAt    time.Time      `json:"next_draw_at"`
	AcceptAt      *time.Time     `json:"accept_at,omitempty"`
	SealAt        *time.Time     `json:"seal_at,omitempty"`
	DrawInterval  int            `json:"draw_interval"`
	SealSeconds   int            `json:"seal_seconds"`
	TimingSource  string         `json:"timing_source"`
	LatestIssue   string         `json:"latest_issue,omitempty"`
	LatestNumbers []int          `json:"latest_numbers,omitempty"`
	LatestDrawAt  *time.Time     `json:"latest_draw_at,omitempty"`
	SourceName    string         `json:"source_name,omitempty"`
	IssueStatus   string         `json:"issue_status"`
	SourceHealthy bool           `json:"source_healthy"`
	SourceError   string         `json:"source_error,omitempty"`
	BettingWindow *BettingWindow `json:"betting_window,omitempty"`
	RulesReady    bool           `json:"rules_ready"`
	RuleVersion   string         `json:"rule_version,omitempty"`
	RulesMessage  string         `json:"rules_message,omitempty"`
}

// History returns all accepted compact-input requests for server-side actions
// such as “重复”. It includes both chat commands and detailed-board requests.
func (s *BetAssistantService) History(userID uint64, gameID string, limit int) ([]AssistantBetResult, error) {
	return s.history(userID, gameID, limit, false)
}

// DirectHistory returns only requests created by the detailed betting board.
// Chat-command requests already have a durable assistant reply in the chat
// table, so excluding them prevents duplicate receipts in the member timeline.
func (s *BetAssistantService) DirectHistory(userID uint64, gameID string, limit int) ([]AssistantBetResult, error) {
	return s.history(userID, gameID, limit, true)
}

func (s *BetAssistantService) history(userID uint64, gameID string, limit int, directOnly bool) ([]AssistantBetResult, error) {
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return nil, apperrors.NewBusinessError("INVALID_GAME", "彩种参数不正确")
	}
	if _, err := s.bets.loadGame(gameID); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	var owner user.User
	if err := s.db.Select("workspace_id").First(&owner, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
		}
		return nil, apperrors.NewSystemError("BET_HISTORY_READ_FAILED", "读取投注消息失败", err)
	}

	// ResultJSON contains game_id. Read a bounded recent window for this user,
	// then filter after decoding so records from another game can never leak
	// into the current room.
	var rows []bet.AssistantRequest
	query := s.db.Where("workspace_id = ? AND user_id = ? AND status = ? AND result_json <> ''", owner.WorkspaceID, userID, "completed")
	if directOnly {
		query = query.Where("request_id NOT LIKE ?", "chat-%")
	}
	if err := query.Order("id desc").Limit(500).Find(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("BET_HISTORY_READ_FAILED", "读取投注消息失败", err)
	}

	newest := make([]AssistantBetResult, 0, limit)
	for _, row := range rows {
		var result AssistantBetResult
		if err := json.Unmarshal([]byte(row.ResultJSON), &result); err != nil || result.GameID != gameID {
			continue
		}
		if result.AcceptedAt.IsZero() {
			result.AcceptedAt = row.CreatedAt.UTC()
		}
		newest = append(newest, result)
		if len(newest) == limit {
			break
		}
	}

	// Timelines render oldest -> newest so the latest accepted message remains
	// at the bottom without client-side timestamp guesses.
	for left, right := 0, len(newest)-1; left < right; left, right = left+1, right-1 {
		newest[left], newest[right] = newest[right], newest[left]
	}
	return newest, nil
}

type BetAssistantService struct {
	db   *gorm.DB
	bets *BetAdminService
}

func NewBetAssistantService(db *gorm.DB) *BetAssistantService {
	return &BetAssistantService{db: db, bets: NewBetAdminService(db)}
}

func (s *BetAssistantService) Place(userID uint64, gameID, issue, content, operator, requestID string) (*AssistantBetResult, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return s.place(userID, gameID, issue, content, operator, "")
	}
	if len(requestID) < 8 || len(requestID) > 96 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "请求标识不正确")
	}
	payloadHash, hashErr := idempotencyPayloadHash(struct {
		GameID  string `json:"game_id"`
		Issue   string `json:"issue"`
		Content string `json:"content"`
	}{
		GameID: strings.TrimSpace(gameID), Issue: strings.TrimSpace(issue),
		Content: strings.TrimSpace(content),
	})
	if hashErr != nil {
		return nil, apperrors.NewSystemError("REQUEST_SAVE_FAILED", "生成投注请求凭证失败", hashErr)
	}

	var result *AssistantBetResult
	var notify bool
	var workspaceID uint64
	var roomScope string
	var terminalErr error
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var owner user.User
		if err := tx.Select("user_id", "workspace_id").First(&owner, userID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
			}
			return err
		}

		row := bet.AssistantRequest{WorkspaceID: owner.WorkspaceID, UserID: userID, RequestID: requestID, PayloadHash: payloadHash, Status: "processing"}
		created := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "request_id"}},
			DoNothing: true,
		}).Create(&row)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 0 {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("user_id = ? AND request_id = ?", userID, requestID).First(&row).Error; err != nil {
				return err
			}
			// The request id is member supplied and remains unique across that
			// member's lifetime. Check the frozen room before returning a cached
			// receipt so moving rooms cannot replay assistant data from the prior
			// workspace.
			if row.WorkspaceID != owner.WorkspaceID {
				message := "请求所属房间已变化，请使用新的请求标识重新提交"
				if row.Status == "processing" {
					if err := tx.Model(&bet.AssistantRequest{}).Where("id = ? AND status = ?", row.ID, "processing").
						Updates(map[string]any{"status": "failed", "last_error": message}).Error; err != nil {
						return err
					}
				}
				terminalErr = apperrors.NewBusinessError("REQUEST_CONTEXT_CHANGED", message)
				return nil
			}
			if row.PayloadHash != "" && row.PayloadHash != payloadHash {
				terminalErr = apperrors.NewBusinessError("IDEMPOTENCY_CONFLICT", "该请求标识已用于其他投注")
				return nil
			}
			cached, handled, err := s.resolveExistingAssistantRequest(tx, row, time.Now().UTC())
			if err != nil {
				return err
			}
			if handled {
				result = cached
				return nil
			}
		}

		// Resolving an empty issue and PlaceBatch's preflight can both write
		// timing rows. Acquire Game before those writes in this outer request
		// transaction, not only inside PlaceBatch's nested transaction.
		if _, err := lockBettingGame(tx, gameID); err != nil {
			return err
		}
		atomic := &BetAssistantService{
			db:   tx,
			bets: &BetAdminService{db: tx, suppressNotifications: true},
		}
		placed, err := atomic.place(userID, gameID, issue, content, operator, assistantBetRequestReference(row.ID))
		if err != nil {
			return err
		}
		payload, err := json.Marshal(placed)
		if err != nil {
			return apperrors.NewSystemError("REQUEST_SAVE_FAILED", "保存投注回执失败", err)
		}
		updated := tx.Model(&bet.AssistantRequest{}).Where("id = ? AND status = ? AND result_json = ''", row.ID, "processing").
			Updates(map[string]any{"status": "completed", "result_json": string(payload), "last_error": ""})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("开奖助手请求状态发生冲突")
		}
		var committedOwner user.User
		if err := tx.Select("user_id", "workspace_id", "role", "parent_agent_id", "parent_tenant_id").First(&committedOwner, userID).Error; err != nil {
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
		s.bets.notifyPlacement(userID, workspaceID, roomScope, result.GameID, result.Issue, result.Balance)
	}
	return result, nil
}

func assistantBetRequestReference(id uint64) string {
	return "assistant_request:" + strconv.FormatUint(id, 10)
}

func (s *BetAssistantService) resolveExistingAssistantRequest(tx *gorm.DB, row bet.AssistantRequest, now time.Time) (*AssistantBetResult, bool, error) {
	if row.Status == "completed" && strings.TrimSpace(row.ResultJSON) != "" {
		var cached AssistantBetResult
		if err := json.Unmarshal([]byte(row.ResultJSON), &cached); err != nil {
			return nil, true, apperrors.NewSystemError("REQUEST_READ_FAILED", "读取投注结果失败", err)
		}
		return &cached, true, nil
	}
	if row.Status == "failed" {
		return nil, true, apperrors.NewBusinessError("REQUEST_FAILED", defaultString(row.LastError, "该次投注未成功，请重新提交"))
	}
	if !idempotencyReservationExpired(row.UpdatedAt, now) {
		return nil, true, apperrors.NewBusinessError("REQUEST_IN_PROGRESS", "投注请求处理中，请勿重复提交")
	}

	var ledger user.BalanceTransaction
	err := tx.Where("user_id = ? AND reference = ?", row.UserID, assistantBetRequestReference(row.ID)).First(&ledger).Error
	if err == gorm.ErrRecordNotFound {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	if err := validateIdempotencyRequestLedger(ledger, row.UserID, row.WorkspaceID, assistantBetRequestReference(row.ID)); err != nil {
		return nil, true, fmt.Errorf("历史开奖助手请求账务证据不一致")
	}
	recovered, payload, marshalErr := recoveredAssistantResult(ledger)
	if marshalErr != nil {
		return nil, true, marshalErr
	}
	updated := tx.Model(&bet.AssistantRequest{}).Where("id = ? AND status = ? AND result_json = ''", row.ID, "processing").
		Updates(map[string]any{"status": "completed", "result_json": string(payload), "last_error": ""})
	if updated.Error != nil {
		return nil, true, updated.Error
	}
	if updated.RowsAffected != 1 {
		return nil, true, fmt.Errorf("恢复开奖助手请求时状态发生冲突")
	}
	return recovered, true, nil
}

func recoveredAssistantResult(ledger user.BalanceTransaction) (*AssistantBetResult, []byte, error) {
	recovered := &AssistantBetResult{
		Content: "该次投注已受理，请在注单记录查看详情",
		Total:   centsToAmount(-ledger.AmountCents), Balance: centsToAmount(ledger.AfterCents), AcceptedAt: ledger.CreatedAt.UTC(),
	}
	payload, err := json.Marshal(recovered)
	return recovered, payload, err
}

func (s *BetAssistantService) place(userID uint64, gameID, issue, content, operator, ledgerReference string) (*AssistantBetResult, error) {
	game, err := s.bets.loadGame(gameID)
	if err != nil {
		return nil, err
	}
	if !game.Enabled {
		return nil, apperrors.NewBusinessError("GAME_DISABLED", "该彩种暂未开放投注")
	}
	if err := ensureGameRulesSupported(game); err != nil {
		return nil, err
	}
	rules, ok := rulesForGame(game)
	if !ok {
		return nil, apperrors.NewBusinessError("RULES_NOT_READY", "该彩种玩法规则尚未配置完整")
	}
	if rules.MarkSix {
		return nil, apperrors.NewBusinessError("BET_MODE_UNAVAILABLE", "六合彩仅支持网投，不支持聊天投注")
	}
	parseContent, allIn, err := normalizeAssistantAllIn(content)
	if err != nil {
		return nil, err
	}
	lines, err := parseAssistantBetForGame(game, parseContent)
	if err != nil {
		return nil, err
	}
	if allIn {
		var account user.User
		if err := s.db.Select("balance_cents").First(&account, userID).Error; err != nil {
			return nil, apperrors.NewSystemError("BALANCE_READ_FAILED", "读取可用积分失败", err)
		}
		if account.BalanceCents <= 0 {
			return nil, apperrors.NewBusinessError("INSUFFICIENT_BALANCE", "可用积分不足，无法梭哈")
		}
		if !applyAllInAmounts(lines, account.BalanceCents) {
			return nil, apperrors.NewBusinessError("INSUFFICIENT_BALANCE", "可用积分不足以为每个投注项分配 1 积分，无法梭哈")
		}
	}
	// Racing and flying games use 1-10. In the compact room syntax members
	// conventionally type 0 for number 10 (for example 7/67890/100).
	if maxBetPosition(game) == 10 {
		for index := range lines {
			if lines[index].PlayCode == "ball_1_5" && lines[index].Selection == "0" {
				lines[index].Selection = "10"
				lines[index].Label = assistantLineLabel(lines[index])
			}
		}
	}
	requestedIssue := strings.TrimSpace(issue)
	if requestedIssue == "" {
		requestedIssue, err = s.bets.BettingIssue(game.ID)
		if err != nil {
			return nil, err
		}
	}
	inputs := make([]PlaceBetInput, 0, len(lines))
	var totalCents int64
	for _, line := range lines {
		inputs = append(inputs, PlaceBetInput{
			GameID: game.ID, Issue: requestedIssue, UserID: userID,
			PlayCode: line.PlayCode, PlayName: line.PlayName, Position: line.Position,
			Selection: line.Selection, Amount: line.Amount, Operator: defaultString(operator, "开奖助手"),
			Remark:          "开奖助手识别投注",
			LedgerReference: ledgerReference,
		})
		totalCents += int64(math.Round(line.Amount * 100))
	}
	placedBets, err := s.bets.PlaceBatch(inputs)
	if err != nil {
		return nil, err
	}
	for index := range lines {
		if index < len(placedBets) {
			lines[index].Odds = placedBets[index].Odds
		}
	}
	var account user.User
	if err := s.db.Select("balance_cents").First(&account, userID).Error; err != nil {
		return nil, apperrors.NewSystemError("BALANCE_READ_FAILED", "读取扣分后的余额失败", err)
	}
	return &AssistantBetResult{
		GameID: game.ID, GameName: game.Name, RuleVersion: rules.Version, Issue: requestedIssue, Content: strings.TrimSpace(content),
		Lines: lines, BetCount: len(lines), Total: centsToAmount(totalCents),
		Balance: centsToAmount(account.BalanceCents), AcceptedAt: time.Now().UTC(),
	}, nil
}

// Status never publishes a draw or settles bets. It may materialize the same
// immutable-per-room acceptance window as the catalogue and betting gate.
func (s *BetAssistantService) Status(gameID string) (*AssistantDrawStatus, error) {
	return s.statusForWorkspace(gameID, 0)
}

func (s *BetAssistantService) statusForWorkspace(gameID string, workspaceID uint64) (*AssistantDrawStatus, error) {
	game, err := s.bets.loadGame(gameID)
	if err != nil {
		return nil, err
	}
	lifecycle, err := s.bets.EnsureCurrentIssue(game)
	if err != nil {
		return nil, err
	}
	rawSettings, actualWorkspaceID, err := readTimingSettings(s.db, workspaceID)
	if err != nil {
		return nil, err
	}
	status := &AssistantDrawStatus{
		GameID: game.ID, GameName: game.Name, Issue: lifecycle.Issue,
		SourceName: game.SourceName, DrawInterval: effectiveDrawInterval(game),
		SealSeconds: configuredSealSeconds(rawSettings, game.ID), TimingSource: game.TimingSource,
	}
	status.IssueStatus = lifecycle.Status
	if lifecycle.ScheduledDrawAt != nil && lifecycle.Issue != "" {
		window, err := ensureIssueWindow(s.db, actualWorkspaceID, game, lifecycle.Issue, *lifecycle.ScheduledDrawAt, rawSettings)
		if err != nil {
			return nil, err
		}
		status.NextDrawAt, status.AcceptAt, status.SealAt = window.ScheduledDrawAt, &window.AcceptAt, &window.SealAt
		status.DrawInterval, status.SealSeconds = window.DrawInterval, window.SealSeconds
		if lifecycle.Status != lottery.IssueStatusError && lifecycle.Status != lottery.IssueStatusSettling && lifecycle.Status != lottery.IssueStatusSettled {
			status.IssueStatus = windowStatus(window, time.Now().UTC())
		}
		status.Accepting = game.Enabled && sharedIssueOpen(lifecycle, time.Now().UTC()) && status.IssueStatus == lottery.IssueStatusAccepting
	}
	status.SourceHealthy = sourceHealthyForGame(game) && lifecycle.Status != lottery.IssueStatusError
	status.SourceError = lifecycle.LastError
	status.BettingWindow, err = s.bets.nextBettingWindow(game, lifecycle, actualWorkspaceID, rawSettings)
	if err != nil {
		return nil, err
	}
	status.Accepting = status.SourceHealthy && (status.Accepting || status.BettingWindow != nil)
	applyAssistantRulesStatus(game, status)
	var latest lottery.Draw
	if err := trustedDrawsForGame(s.db, game.ID).Order("draw_at desc").First(&latest).Error; err == nil {
		status.LatestIssue = latest.Issue
		status.LatestNumbers = parseNumbers(latest.Numbers)
		status.LatestDrawAt = &latest.DrawAt
	} else if err != gorm.ErrRecordNotFound {
		return nil, apperrors.NewSystemError("DRAW_READ_FAILED", "读取开奖结果失败", err)
	}
	return status, nil
}

func applyAssistantRulesStatus(game *lottery.Game, status *AssistantDrawStatus) {
	rules, ready := rulesForGame(game)
	status.RulesReady = ready
	status.RuleVersion = rules.Version
	status.RulesMessage = ""
	if !ready {
		status.Accepting = false
		status.BettingWindow = nil
		status.RulesMessage = gameRulesUnavailableMessage
	}
}

// StatusForUser narrows the platform draw state with the member's current
// room switches. A room may close itself or one game even while the shared
// platform lifecycle is healthy; the assistant must then report that it is
// not accepting rather than waiting for the final Place transaction to reject
// a command.
func (s *BetAssistantService) StatusForUser(userID uint64, gameID string) (*AssistantDrawStatus, error) {
	var account user.User
	if err := s.db.Select("user_id", "workspace_id", "role", "status", "agent_room_code", "parent_agent_id", "parent_tenant_id").
		First(&account, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
		}
		return nil, err
	}
	status, err := s.statusForWorkspace(gameID, account.WorkspaceID)
	if err != nil {
		return nil, err
	}
	roomActive, err := accesscontrol.AccountRoomActive(s.db, account)
	if err != nil {
		return nil, err
	}
	gameEnabled, err := WorkspaceGameEnabled(s.db, account.WorkspaceID, gameID)
	if err != nil {
		return nil, err
	}
	status.Accepting = status.Accepting && account.Status == 1 && roomActive && gameEnabled
	if !status.Accepting {
		status.BettingWindow = nil
	}
	return status, nil
}

var assistantPositionPlay = regexp.MustCompile(`^(10|[0-9])/?([大小单双龙虎和])$`)
var assistantRankPlay = regexp.MustCompile(`^(冠军|亚军|第三名|第四名|第五名|第六名|第七名|第八名|第九名|第十名)/?([0-9大小单双龙虎和]+)$`)
var assistantBallPlay = regexp.MustCompile(`^第([1-5])球/?([0-9大小单双龙虎和]+)$`)
var assistantAmountPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]{1,2})?$`)
var assistantCompactAmount = regexp.MustCompile(`^(.*(?:[大小单双龙虎和]|豹子|顺子|对子|半顺|杂六))([0-9]+(?:\.[0-9]{1,2})?)$`)

var assistantRanks = map[string]int{
	"冠军": 1, "亚军": 2, "第三名": 3, "第四名": 4, "第五名": 5, "第六名": 6,
	"第七名": 7, "第八名": 8, "第九名": 9, "第十名": 10,
}

// ParseAssistantBet understands the compact room syntax, for example:
// 买12345/1000#3大/2000#6/123456/100
// The final amount is the amount of EACH selection. For example
// 1/12345/100 creates five 100-point bets, while 34/大虎/236 creates four
// 236-point bets. Repeated digits intentionally accumulate after merging.
func ParseAssistantBet(content string) ([]AssistantBetLine, error) {
	// The exported pure parser retains the established racing syntax for
	// callers/tests. Production placement always supplies the actual game.
	return parseAssistantBetForGame(&lottery.Game{ID: "speed-racing"}, content)
}

func parseAssistantBetForGame(game *lottery.Game, content string) ([]AssistantBetLine, error) {
	if err := ensureGameRulesSupported(game); err != nil {
		return nil, err
	}
	rules, _ := rulesForGame(game)
	if rules.MarkSix {
		return nil, apperrors.NewBusinessError("BET_MODE_UNAVAILABLE", "六合彩仅支持网投，不支持聊天投注")
	}
	if rules.PC28 > 0 {
		return parsePC28AssistantBet(content)
	}
	raw := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(content), "买"))
	if raw == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "请输入投注内容，例如 3大/200#12345/1000")
	}
	if len([]rune(raw)) > 400 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "投注内容过长，请拆分后重试")
	}
	var lines []AssistantBetLine
	for _, segment := range strings.Split(raw, "#") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		parts := assistantSegmentParts(segment)
		if len(parts) < 2 {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("“%s”缺少金额，请使用 玩法/金额", segment))
		}
		amountCents, err := assistantMoneyCents(strings.TrimSpace(parts[len(parts)-1]))
		if err != nil {
			return nil, err
		}
		playParts := parts[:len(parts)-1]
		entries, err := assistantSegmentEntries(rules, playParts)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			line := AssistantBetLine{
				Position: entry.position, Selection: entry.selection, PlayCode: entry.playCode, PlayName: entry.playName,
				Amount: centsToAmount(amountCents),
			}
			line.Label = assistantLineLabel(line)
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "未识别到有效投注，请检查格式")
	}
	return mergeAssistantLines(lines), nil
}

// AssistantRepeatContent reconstructs explicit selections from the accepted
// receipt. Repeating a single scoped shape must not expand into all three
// windows through an unscoped shorthand; every line is validated again by the
// current placement gate.
func AssistantRepeatContent(gameID string, lines []AssistantBetLine) (string, error) {
	rules, ready := rulesForGame(&lottery.Game{ID: strings.TrimSpace(gameID)})
	if !ready || rules.MarkSix {
		return "", apperrors.NewBusinessError("INVALID_REQUEST", "上一笔投注不能在当前彩种重复")
	}
	if len(lines) == 0 {
		return "", apperrors.NewBusinessError("INVALID_REQUEST", "上一笔投注缺少可重复的明细")
	}
	if rules.PC28 > 0 {
		return pc28AssistantRepeatContent(lines)
	}
	segments := make([]string, 0, len(lines))
	for _, line := range lines {
		if line.Amount <= 0 || math.IsNaN(line.Amount) || math.IsInf(line.Amount, 0) {
			return "", apperrors.NewBusinessError("INVALID_REQUEST", "上一笔投注金额不正确，不能重复")
		}
		amount := FormatBetAmount(line.Amount)
		selection := strings.TrimSpace(line.Selection)
		var segment string
		switch strings.ToLower(strings.TrimSpace(line.PlayCode)) {
		case "leopard", "straight", "pair", "half_straight", "mixed":
			play, ok := defaultPlayByCode(line.PlayCode)
			if !ok || line.Position < 1 || line.Position > 3 {
				return "", apperrors.NewBusinessError("INVALID_REQUEST", "上一笔形态投注明细不正确，不能重复")
			}
			segment = fmt.Sprintf("%s/%s/%s", digitShapeScopeName(line.Position), play.Name, amount)
		case "dragon_tiger_tie":
			segment = fmt.Sprintf("%d/和/%s", line.Position, amount)
		case "dragon_tiger":
			segment = fmt.Sprintf("%d/%s/%s", line.Position, selection, amount)
		case "sum":
			if rules.Racing {
				segment = fmt.Sprintf("冠亚/%s/%s", selection, amount)
			} else if isSideSelection(selection) {
				segment = fmt.Sprintf("总和/%s/%s", selection, amount)
			} else {
				segment = fmt.Sprintf("总和尾/%s/%s", selection, amount)
			}
		case "ball_1_5", "two_sided":
			segment = fmt.Sprintf("%d/%s/%s", line.Position, selection, amount)
		default:
			return "", apperrors.NewBusinessError("INVALID_REQUEST", "上一笔投注包含当前不能重复的玩法")
		}
		segments = append(segments, segment)
	}
	return strings.Join(segments, "#"), nil
}

// Non-numeric plays may omit the final slash, for example 1大5, 大单20 or
// 前三豹子5. Pure-number commands deliberately still require a slash so that
// a number selection and its stake can never be guessed from the same run.
func assistantSegmentParts(segment string) []string {
	parts := strings.Split(segment, "/")
	if len(parts) > 1 {
		return parts
	}
	if match := assistantCompactAmount.FindStringSubmatch(strings.TrimSpace(segment)); len(match) == 3 {
		return []string{strings.TrimSpace(match[1]), match[2]}
	}
	return parts
}

func assistantSegmentEntries(rules gameRuleProfile, parts []string) ([]assistantEntry, error) {
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	if len(parts) == 2 {
		left, right := parts[0], parts[1]
		if isCrownSumToken(left) {
			if !rules.Racing {
				return nil, apperrors.NewBusinessError("INVALID_REQUEST", "该彩种不使用冠亚和，请使用总和/大/100或总和尾/7/100")
			}
			return assistantSumEntries(rules, right, false, isShortCrownSumToken(left))
		}
		if left == "总和" || left == "总和尾" {
			if rules.Racing {
				return nil, apperrors.NewBusinessError("INVALID_REQUEST", "赛车请使用冠亚和/玩法/金额")
			}
			return assistantSumEntries(rules, right, left == "总和尾", false)
		}
		if positions, ok := assistantNamedPositionGroup(rules, left); ok {
			return assistantSelectionsForPositions(rules, positions, right)
		}
		if scope, scoped := digitShapeScope(left); scoped {
			return assistantShapeEntries(rules, right, scope)
		}
		positions, ok := assistantPositions(rules, left)
		if !ok {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("无法识别名次“%s”", left))
		}
		return assistantSelectionsForPositions(rules, positions, right)
	}
	if len(parts) != 1 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "投注格式不正确，请使用 名次/玩法/金额")
	}
	return assistantPlayEntries(rules, parts[0])
}

func assistantPositions(rules gameRuleProfile, raw string) ([]int, bool) {
	raw = strings.TrimSpace(raw)
	if position, ok := assistantRanks[raw]; ok {
		return []int{position}, rules.Racing && position <= rules.BallCount
	}
	if !rules.Racing && strings.HasPrefix(raw, "第") && strings.HasSuffix(raw, "球") {
		position, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(raw, "第"), "球"))
		return []int{position}, err == nil && position >= 1 && position <= rules.BallCount
	}
	if raw == "10" || raw == "0" {
		return []int{10}, rules.Racing
	}
	if raw == "" || !allDigits(raw) {
		return nil, false
	}
	positions := make([]int, 0, len(raw))
	for _, char := range raw {
		position := int(char - '0')
		if position == 0 && rules.Racing {
			position = 10
		}
		if position < 1 || position > rules.BallCount {
			return nil, false
		}
		positions = append(positions, position)
	}
	return positions, true
}

func assistantSelectionsForPositions(rules gameRuleProfile, positions []int, raw string) ([]assistantEntry, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "请选择号码或大小单双龙虎")
	}
	entries := make([]assistantEntry, 0, len(positions)*len([]rune(raw)))
	for _, position := range positions {
		if position < 1 || position > rules.BallCount {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "球位不正确")
		}
		for _, char := range raw {
			selection := string(char)
			entry := assistantEntry{position: position, selection: selection}
			switch {
			case char >= '0' && char <= '9':
				entry.playCode, entry.playName = "ball_1_5", "指定名次号码"
				if !rules.Racing {
					entry.playName = fmt.Sprintf("第%d球号码", position)
				}
			case strings.ContainsRune("大小单双", char):
				entry.playCode, entry.playName = "two_sided", "第"+strconv.Itoa(position)+"名两面"
				if !rules.Racing {
					entry.playName = fmt.Sprintf("第%d球两面", position)
				}
			case char == '龙' || char == '虎':
				maxPosition := rules.BallCount / 2
				if rules.FirstLastDragonTiger {
					maxPosition = 1
				}
				if position > maxPosition {
					return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("龙虎仅支持第 1 至第 %d 位", maxPosition))
				}
				entry.playCode, entry.playName = "dragon_tiger", "龙虎"
				if !rules.Racing {
					entry.playName = fmt.Sprintf("第%d球龙虎", position)
				}
			case char == '和':
				if !rules.DragonTigerTie || position != 1 {
					return nil, apperrors.NewBusinessError("INVALID_REQUEST", "当前彩种不支持该球位的龙虎和")
				}
				entry.playCode, entry.playName = "dragon_tiger_tie", "第1球龙虎和"
			default:
				return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("无法识别玩法“%s”", selection))
			}
			if !rules.supportsPlay(entry.playCode) {
				return nil, apperrors.NewBusinessError("INVALID_REQUEST", "当前彩种不支持该玩法")
			}
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func isCrownSumToken(raw string) bool {
	raw = strings.ReplaceAll(strings.TrimSpace(raw), "军", "")
	return raw == "冠亚" || raw == "冠亚和" || raw == "和"
}

func isShortCrownSumToken(raw string) bool {
	return strings.TrimSpace(raw) == "和"
}

func assistantNamedPositionGroup(rules gameRuleProfile, raw string) ([]int, bool) {
	if !rules.Racing {
		return nil, false
	}
	switch strings.TrimSpace(raw) {
	case "前三":
		return []int{1, 2, 3}, true
	case "后三":
		return []int{8, 9, 10}, true
	case "前五":
		return []int{1, 2, 3, 4, 5}, true
	case "后五":
		return []int{6, 7, 8, 9, 10}, true
	default:
		return nil, false
	}
}

func assistantSumEntries(rules gameRuleProfile, raw string, tailOnly, splitShortNumbers bool) ([]assistantEntry, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "请选择冠亚和号码或大小单双")
	}
	if !rules.supportsPlay("sum") {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "当前彩种不支持总和玩法")
	}
	name := "冠亚和"
	if !rules.Racing {
		name = "总和"
	}
	if rules.Racing && splitShortNumbers && allDigits(raw) {
		entries := make([]assistantEntry, 0, len(raw))
		for _, char := range raw {
			total := int(char - '0')
			if total < 3 || total > 9 {
				return nil, apperrors.NewBusinessError("INVALID_REQUEST", "和玩法连写号码只能选择 3 至 9；和值 10 至 19 请使用 冠亚/和值/金额")
			}
			entries = append(entries, assistantEntry{position: 6, selection: strconv.Itoa(total), playCode: "sum", playName: name})
		}
		return entries, nil
	}
	// A numeric sum is always one complete value. Never reinterpret invalid
	// 34/99 as several smaller bets; multiple sums require separate # groups.
	if allDigits(raw) {
		total, err := strconv.Atoi(raw)
		if rules.Racing {
			if err != nil || total < 3 || total > 19 {
				return nil, apperrors.NewBusinessError("INVALID_REQUEST", "冠亚和号码只能选择完整和值 3 至 19，多项请用 # 分组")
			}
		} else {
			if err != nil || len(raw) != 1 || total < 0 || total > 9 {
				return nil, apperrors.NewBusinessError("INVALID_REQUEST", "总和尾只能选择单个 0 至 9，多项请用 # 分组")
			}
			name = "总和尾"
		}
		return []assistantEntry{{position: 6, selection: strconv.Itoa(total), playCode: "sum", playName: name}}, nil
	}
	if tailOnly {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "总和尾只能选择 0 至 9；大小单双请使用总和/大/100")
	}
	entries := make([]assistantEntry, 0, len([]rune(raw)))
	for _, char := range raw {
		selection := string(char)
		if !strings.ContainsRune("大小单双", char) {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("“%s”不是支持的%s玩法，数字与大小单双请用 # 分组", raw, name))
		}
		// Position 6 is the canonical storage slot for 冠亚和 throughout the
		// betting, odds and settlement pipeline. Keeping the assistant on the
		// same slot prevents an otherwise valid parsed ticket being rejected by
		// PlaceBatch before any balance mutation happens.
		entries = append(entries, assistantEntry{position: 6, selection: selection, playCode: "sum", playName: name})
	}
	return entries, nil
}

type assistantEntry struct {
	position  int
	selection string
	playCode  string
	playName  string
}

func assistantPlayEntries(rules gameRuleProfile, play string) ([]assistantEntry, error) {
	play = strings.TrimSpace(play)
	if rules.Racing && strings.HasPrefix(play, "和") {
		return assistantSumEntries(rules, strings.TrimPrefix(play, "和"), false, true)
	}
	// In the compact racing notation, 0 represents the tenth position. Thus
	// "10大" means positions 1 and 10, while the explicit "10/大" continues
	// to mean only the tenth position.
	if rules.Racing && strings.HasPrefix(play, "10") && len([]rune(play)) > 2 {
		selection := strings.TrimPrefix(play, "10")
		if len([]rune(selection)) == 1 && strings.ContainsRune("大小单双龙虎", []rune(selection)[0]) {
			return assistantSelectionsForPositions(rules, []int{1, 10}, selection)
		}
	}
	if match := assistantPositionPlay.FindStringSubmatch(play); len(match) == 3 {
		positions, ok := assistantPositions(rules, match[1])
		if !ok {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "球位不正确")
		}
		return assistantSelectionsForPositions(rules, positions, match[2])
	}
	if match := assistantRankPlay.FindStringSubmatch(play); len(match) == 3 {
		if !rules.Racing {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "该彩种请使用数字球位，例如 1/0/100")
		}
		return assistantSelectionsForPositions(rules, []int{assistantRanks[match[1]]}, match[2])
	}
	if match := assistantBallPlay.FindStringSubmatch(play); len(match) == 3 && !rules.Racing {
		position, _ := strconv.Atoi(match[1])
		return assistantSelectionsForPositions(rules, []int{position}, match[2])
	}
	if strings.HasPrefix(play, "冠亚和") || strings.HasPrefix(play, "冠亚") {
		if !rules.Racing {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "该彩种不使用冠亚和，请使用总和/大/100或总和尾/7/100")
		}
		selection := strings.TrimPrefix(play, "冠亚和")
		selection = strings.TrimPrefix(selection, "冠亚")
		selection = strings.TrimPrefix(selection, "/")
		return assistantSumEntries(rules, selection, false, false)
	}
	if strings.HasPrefix(play, "总和") {
		if rules.Racing {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "赛车请使用冠亚和/玩法/金额")
		}
		tailOnly := strings.HasPrefix(play, "总和尾")
		selection := strings.TrimPrefix(strings.TrimPrefix(play, "总和尾"), "总和")
		return assistantSumEntries(rules, strings.TrimPrefix(selection, "/"), tailOnly, false)
	}
	if scope, scoped := digitShapeScope(play); scoped {
		return assistantShapeEntries(rules, trimDigitShapeScope(play), scope)
	}
	if _, ok := assistantShapeCode(play); ok {
		return assistantShapeEntries(rules, play, 0)
	}
	if allAssistantSelections(play) {
		positions := []int{1}
		if !rules.Racing && !strings.ContainsAny(play, "龙虎和") {
			positions = make([]int, rules.BallCount)
			for index := range positions {
				positions[index] = index + 1
			}
		}
		return assistantSelectionsForPositions(rules, positions, play)
	}
	return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("无法识别“%s”，请使用例如 3大/200 或 12345/1000", play))
}

func allAssistantSelections(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && !strings.ContainsRune("大小单双龙虎和", char) {
			return false
		}
	}
	return true
}

func assistantShapeCode(value string) (string, bool) {
	code := normalizePlaySelection(strings.TrimSpace(value))
	switch code {
	case "leopard", "straight", "pair", "half_straight", "mixed":
		return code, true
	}
	return "", false
}

func assistantShapeEntries(rules gameRuleProfile, raw string, requestedPosition int) ([]assistantEntry, error) {
	code, ok := assistantShapeCode(raw)
	if !ok || rules.Racing || !rules.supportsPlay(code) {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "当前彩种不支持该形态玩法")
	}
	play, _ := defaultPlayByCode(code)
	positions := []int{requestedPosition}
	if requestedPosition == 0 {
		positions = []int{1}
		if rules.ThreeShapeWindows {
			positions = []int{1, 2, 3}
		}
	}
	entries := make([]assistantEntry, 0, len(positions))
	for _, position := range positions {
		if position < 1 || position > 3 || position > 1 && !rules.ThreeShapeWindows {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "当前彩种形态玩法仅支持前三球")
		}
		entries = append(entries, assistantEntry{
			position: position, selection: code, playCode: code,
			playName: digitShapeScopeName(position) + play.Name,
		})
	}
	return entries, nil
}

func assistantMoneyCents(raw string) (int64, error) {
	if !assistantAmountPattern.MatchString(raw) {
		return 0, apperrors.NewBusinessError("INVALID_REQUEST", "投注金额必须是大于 0 的常规数字，最多保留两位小数")
	}
	parts := strings.SplitN(raw, ".", 2)
	fraction := "00"
	if len(parts) == 2 {
		fraction = parts[1]
		if len(fraction) == 1 {
			fraction += "0"
		}
	}
	cents, err := strconv.ParseInt(parts[0]+fraction, 10, 64)
	if err != nil || cents <= 0 {
		return 0, apperrors.NewBusinessError("INVALID_REQUEST", "投注金额必须是大于 0 的数字")
	}
	// Parse decimal text directly into cents. The line/API still carries a
	// float, so reject values that would lose a cent at that existing boundary.
	converted, err := positiveMoneyCents(centsToAmount(cents), "投注金额")
	if err != nil || converted != cents {
		return 0, apperrors.NewBusinessError("INVALID_REQUEST", "投注金额超出可精确处理的范围")
	}
	return cents, nil
}

func normalizeAssistantAllIn(content string) (string, bool, error) {
	raw := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(content), "买"))
	if !strings.Contains(raw, "梭哈") {
		return content, false, nil
	}
	if strings.Count(raw, "梭哈") != 1 {
		return "", false, apperrors.NewBusinessError("INVALID_REQUEST", "每次只能使用一次梭哈")
	}
	if strings.Contains(raw, "#") {
		return "", false, apperrors.NewBusinessError("INVALID_REQUEST", "梭哈必须单独提交，不能与普通金额注单混合")
	}
	if !strings.HasSuffix(raw, "梭哈") {
		return "", false, apperrors.NewBusinessError("INVALID_REQUEST", "梭哈只能填写在金额位置")
	}
	if !strings.HasSuffix(raw, "/梭哈") {
		play := strings.TrimSpace(strings.TrimSuffix(raw, "梭哈"))
		if play == "" || allDigits(play) {
			return "", false, apperrors.NewBusinessError("INVALID_REQUEST", "请先输入玩法，再填写梭哈")
		}
		raw = play + "/梭哈"
	}
	return strings.TrimSuffix(raw, "梭哈") + "1", true, nil
}

func applyAllInAmounts(lines []AssistantBetLine, balanceCents int64) bool {
	if len(lines) == 0 || balanceCents <= 0 {
		return false
	}
	weights := make([]int64, len(lines))
	var totalWeight int64
	for index, line := range lines {
		weights[index] = maxInt64(1, int64(math.Round(line.Amount)))
		totalWeight += weights[index]
	}
	// The documented all-in contract uses the largest whole-number equal stake
	// for every expanded selection. Cents and an indivisible remainder remain
	// in the account instead of being assigned to the final line.
	stakeUnits := (balanceCents / 100) / totalWeight
	if stakeUnits <= 0 {
		return false
	}
	for index := range lines {
		amount := stakeUnits * weights[index] * 100
		lines[index].Amount = centsToAmount(amount)
		lines[index].Label = assistantLineLabel(lines[index])
	}
	return true
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func mergeAssistantLines(lines []AssistantBetLine) []AssistantBetLine {
	indexByKey := map[string]int{}
	merged := make([]AssistantBetLine, 0, len(lines))
	for _, line := range lines {
		key := fmt.Sprintf("%d|%s|%s", line.Position, line.Selection, line.PlayCode)
		if index, ok := indexByKey[key]; ok {
			merged[index].Amount = centsToAmount(int64(math.Round((merged[index].Amount + line.Amount) * 100)))
			merged[index].Label = assistantLineLabel(merged[index])
			continue
		}
		indexByKey[key] = len(merged)
		merged = append(merged, line)
	}
	return merged
}

func assistantLineLabel(line AssistantBetLine) string {
	_, label := assistantReceiptGroup(line)
	return fmt.Sprintf("%s[%s/%s]", label, assistantReceiptSelection(line), FormatBetAmount(line.Amount))
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
