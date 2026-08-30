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
	GameID     string             `json:"game_id"`
	GameName   string             `json:"game_name"`
	Issue      string             `json:"issue"`
	Content    string             `json:"content"`
	Lines      []AssistantBetLine `json:"lines"`
	BetCount   int                `json:"bet_count"`
	Total      float64            `json:"total"`
	Balance    float64            `json:"balance"`
	AcceptedAt time.Time          `json:"accepted_at"`
}

type AssistantDrawStatus struct {
	GameID        string     `json:"game_id"`
	GameName      string     `json:"game_name"`
	Issue         string     `json:"issue"`
	Accepting     bool       `json:"accepting"`
	NextDrawAt    time.Time  `json:"next_draw_at"`
	AcceptAt      *time.Time `json:"accept_at,omitempty"`
	SealAt        *time.Time `json:"seal_at,omitempty"`
	DrawInterval  int        `json:"draw_interval"`
	SealSeconds   int        `json:"seal_seconds"`
	TimingSource  string     `json:"timing_source"`
	LatestIssue   string     `json:"latest_issue,omitempty"`
	LatestNumbers []int      `json:"latest_numbers,omitempty"`
	LatestDrawAt  *time.Time `json:"latest_draw_at,omitempty"`
	SourceName    string     `json:"source_name,omitempty"`
	IssueStatus   string     `json:"issue_status"`
	SourceHealthy bool       `json:"source_healthy"`
	SourceError   string     `json:"source_error,omitempty"`
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
	parseContent, allIn, err := normalizeAssistantAllIn(content)
	if err != nil {
		return nil, err
	}
	lines, err := ParseAssistantBet(parseContent)
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
		applyAllInAmounts(lines, account.BalanceCents)
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
		requestedIssue, err = s.bets.CurrentIssue(game.ID)
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
		GameID: game.ID, GameName: game.Name, Issue: requestedIssue, Content: strings.TrimSpace(content),
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
	status.SourceHealthy = lifecycle.Status != lottery.IssueStatusError
	status.SourceError = lifecycle.LastError
	var latest lottery.Draw
	if err := s.db.Where("game_id = ?", game.ID).Order("draw_at desc").First(&latest).Error; err == nil {
		status.LatestIssue = latest.Issue
		status.LatestNumbers = parseNumbers(latest.Numbers)
		status.LatestDrawAt = &latest.DrawAt
	} else if err != gorm.ErrRecordNotFound {
		return nil, apperrors.NewSystemError("DRAW_READ_FAILED", "读取开奖结果失败", err)
	}
	return status, nil
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
	status.Accepting = status.Accepting && roomActive && gameEnabled
	return status, nil
}

var assistantPositionPlay = regexp.MustCompile(`^(10|[1-9])/?([大小单双龙虎])$`)
var assistantPositionNumbers = regexp.MustCompile(`^(10|[1-9])/([0-9]+)$`)
var assistantRankPlay = regexp.MustCompile(`^(冠军|亚军|第三名|第四名|第五名|第六名|第七名|第八名|第九名|第十名)/?([0-9大小单双龙虎])$`)

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
		parts := strings.Split(segment, "/")
		if len(parts) < 2 {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("“%s”缺少金额，请使用 玩法/金额", segment))
		}
		amountCents, err := assistantMoneyCents(strings.TrimSpace(parts[len(parts)-1]))
		if err != nil {
			return nil, err
		}
		playParts := parts[:len(parts)-1]
		entries, err := assistantSegmentEntries(playParts)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			playCode, playName := InferPlay(entry.playCode, entry.playName, entry.position, entry.selection)
			line := AssistantBetLine{
				Position: entry.position, Selection: entry.selection, PlayCode: playCode, PlayName: playName,
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

func assistantSegmentEntries(parts []string) ([]assistantEntry, error) {
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	if len(parts) == 2 {
		left, right := parts[0], parts[1]
		if isCrownSumToken(left) {
			return assistantSumEntries(right)
		}
		positions, ok := assistantPositions(left)
		if !ok {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("无法识别名次“%s”", left))
		}
		return assistantSelectionsForPositions(positions, right)
	}
	if len(parts) != 1 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "投注格式不正确，请使用 名次/玩法/金额")
	}
	return assistantPlayEntries(parts[0])
}

func assistantPositions(raw string) ([]int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "10" || raw == "0" {
		return []int{10}, true
	}
	if raw == "" || !allDigits(raw) {
		return nil, false
	}
	positions := make([]int, 0, len(raw))
	for _, char := range raw {
		position := int(char - '0')
		if position == 0 {
			position = 10
		}
		positions = append(positions, position)
	}
	return positions, true
}

func assistantSelectionsForPositions(positions []int, raw string) ([]assistantEntry, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "请选择号码或大小单双龙虎")
	}
	entries := make([]assistantEntry, 0, len(positions)*len([]rune(raw)))
	for _, position := range positions {
		for _, char := range raw {
			selection := string(char)
			entry := assistantEntry{position: position, selection: selection}
			switch {
			case char >= '0' && char <= '9':
				entry.playCode, entry.playName = "ball_1_5", "指定名次号码"
			case strings.ContainsRune("大小单双", char):
				entry.playCode, entry.playName = "two_sided", "第"+strconv.Itoa(position)+"名两面"
			case char == '龙' || char == '虎':
				if position > 5 {
					return nil, apperrors.NewBusinessError("INVALID_REQUEST", "龙虎仅支持第 1 至第 5 名")
				}
				entry.playCode, entry.playName = "dragon_tiger", "龙虎"
			default:
				return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("无法识别玩法“%s”", selection))
			}
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func isCrownSumToken(raw string) bool {
	raw = strings.ReplaceAll(strings.TrimSpace(raw), "军", "")
	return raw == "冠亚" || raw == "冠亚和"
}

func assistantSumEntries(raw string) ([]assistantEntry, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "请选择冠亚和号码或大小单双")
	}
	// 冠亚和 10-19 必须作为一个完整和值，不能按两个字符拆成两注。
	// 单个 3-9 仍是一个和值；多个单字符和值可以继续用 # 分段下单。
	if total, err := strconv.Atoi(raw); err == nil && total >= 3 && total <= 19 {
		return []assistantEntry{{position: 6, selection: strconv.Itoa(total), playCode: "sum", playName: "冠亚和"}}, nil
	}
	entries := make([]assistantEntry, 0, len([]rune(raw)))
	for _, char := range raw {
		selection := string(char)
		if !(char >= '0' && char <= '9') && !strings.ContainsRune("大小单双", char) {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("“%s”不是支持的冠亚和玩法", raw))
		}
		// Position 6 is the canonical storage slot for 冠亚和 throughout the
		// betting, odds and settlement pipeline. Keeping the assistant on the
		// same slot prevents an otherwise valid parsed ticket being rejected by
		// PlaceBatch before any balance mutation happens.
		entries = append(entries, assistantEntry{position: 6, selection: selection, playCode: "sum", playName: "冠亚和"})
	}
	return entries, nil
}

type assistantEntry struct {
	position  int
	selection string
	playCode  string
	playName  string
}

func assistantPlayEntries(play string) ([]assistantEntry, error) {
	play = strings.TrimSpace(play)
	if match := assistantPositionNumbers.FindStringSubmatch(play); len(match) == 3 {
		position, _ := strconv.Atoi(match[1])
		return assistantNumberEntries(position, match[2], "指定名次号码"), nil
	}
	if match := assistantPositionPlay.FindStringSubmatch(play); len(match) == 3 {
		position, _ := strconv.Atoi(match[1])
		selection := match[2]
		playCode := "two_sided"
		playName := "第" + strconv.Itoa(position) + "名两面"
		if selection == "龙" || selection == "虎" {
			playCode, playName = "dragon_tiger", "龙虎"
		}
		return []assistantEntry{{position: position, selection: selection, playCode: playCode, playName: playName}}, nil
	}
	if match := assistantRankPlay.FindStringSubmatch(play); len(match) == 3 {
		selection := match[2]
		playCode := "ball_1_5"
		if selection == "大" || selection == "小" || selection == "单" || selection == "双" {
			playCode = "two_sided"
		} else if selection == "龙" || selection == "虎" {
			playCode = "dragon_tiger"
		}
		return []assistantEntry{{position: assistantRanks[match[1]], selection: selection, playCode: playCode, playName: play}}, nil
	}
	if strings.HasPrefix(play, "冠亚和") || strings.HasPrefix(play, "冠亚") {
		selection := strings.TrimPrefix(play, "冠亚和")
		selection = strings.TrimPrefix(selection, "冠亚")
		selection = strings.TrimPrefix(selection, "/")
		return assistantSumEntries(selection)
	}
	playRunes := []rune(play)
	if len(playRunes) == 1 && strings.ContainsRune("大小单双龙虎", playRunes[0]) {
		return assistantSelectionsForPositions([]int{1}, play)
	}
	if allDigits(play) {
		return assistantSelectionsForPositions([]int{1}, play)
	}
	return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("无法识别“%s”，请使用例如 3大/200 或 12345/1000", play))
}

func assistantNumberEntries(position int, digits, name string) []assistantEntry {
	entries := make([]assistantEntry, 0, len(digits))
	for _, digit := range digits {
		entries = append(entries, assistantEntry{position: position, selection: string(digit), playCode: "ball_1_5", playName: name})
	}
	return entries
}

func assistantMoneyCents(raw string) (int64, error) {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return 0, apperrors.NewBusinessError("INVALID_REQUEST", "投注金额必须是大于 0 的数字")
	}
	cents := int64(math.Round(value * 100))
	if cents <= 0 {
		return 0, apperrors.NewBusinessError("INVALID_REQUEST", "投注金额必须大于 0")
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
	if !strings.HasSuffix(raw, "梭哈") {
		return "", false, apperrors.NewBusinessError("INVALID_REQUEST", "梭哈只能填写在金额位置")
	}
	if !strings.HasSuffix(raw, "/梭哈") {
		play := strings.TrimSpace(strings.TrimSuffix(raw, "梭哈"))
		if len([]rune(play)) != 1 || !strings.ContainsRune("大小单双龙虎", []rune(play)[0]) {
			return "", false, apperrors.NewBusinessError("INVALID_REQUEST", "请先输入玩法，再填写梭哈")
		}
		raw = play + "/梭哈"
	}
	return strings.TrimSuffix(raw, "梭哈") + "1", true, nil
}

func applyAllInAmounts(lines []AssistantBetLine, balanceCents int64) {
	if len(lines) == 0 || balanceCents <= 0 {
		return
	}
	weights := make([]int64, len(lines))
	var totalWeight int64
	for index, line := range lines {
		weights[index] = maxInt64(1, int64(math.Round(line.Amount*100)))
		totalWeight += weights[index]
	}
	remaining := balanceCents
	for index := range lines {
		amount := remaining
		if index < len(lines)-1 {
			amount = balanceCents * weights[index] / totalWeight
			remaining -= amount
		}
		lines[index].Amount = centsToAmount(amount)
		lines[index].Label = assistantLineLabel(lines[index])
	}
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
	if line.PlayCode == "sum" {
		return fmt.Sprintf("冠亚和[%s/%.2f]", line.Selection, line.Amount)
	}
	names := []string{"冠军", "亚军", "第三名", "第四名", "第五名", "第六名", "第七名", "第八名", "第九名", "第十名"}
	position := strconv.Itoa(line.Position)
	if line.Position >= 1 && line.Position <= len(names) {
		position = names[line.Position-1]
		return fmt.Sprintf("%s[%s/%.2f]", position, line.Selection, line.Amount)
	}
	return fmt.Sprintf("第%s名[%s/%.2f]", position, line.Selection, line.Amount)
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
