package services

import (
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
)

// AssistantBetLine is the server-authoritative explanation of one parsed bet.
// Amounts are returned in yuan and always add up to the amount typed by the member.
type AssistantBetLine struct {
	Position  int     `json:"position"`
	Selection string  `json:"selection"`
	PlayCode  string  `json:"play_code"`
	PlayName  string  `json:"play_name"`
	Amount    float64 `json:"amount"`
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
	LatestIssue   string     `json:"latest_issue,omitempty"`
	LatestNumbers []int      `json:"latest_numbers,omitempty"`
	LatestDrawAt  *time.Time `json:"latest_draw_at,omitempty"`
	SourceName    string     `json:"source_name,omitempty"`
}

// History returns the member's accepted compact-input messages for one game.
// AssistantRequest already stores the authoritative result for idempotency, so
// it is also the correct source for rebuilding the room timeline after refresh.
func (s *BetAssistantService) History(userID uint64, gameID string, limit int) ([]AssistantBetResult, error) {
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

	// ResultJSON contains game_id. Read a bounded recent window for this user,
	// then filter after decoding so records from another game can never leak
	// into the current room.
	var rows []bet.AssistantRequest
	if err := s.db.Where("user_id = ? AND result_json <> ''", userID).
		Order("id desc").Limit(500).Find(&rows).Error; err != nil {
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
	if requestID != "" {
		if len(requestID) < 8 || len(requestID) > 96 {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "请求标识不正确")
		}
		row := bet.AssistantRequest{UserID: userID, RequestID: requestID}
		created := s.db.Create(&row)
		if created.Error != nil {
			if !strings.Contains(strings.ToLower(created.Error.Error()), "duplicate") {
				return nil, apperrors.NewSystemError("REQUEST_SAVE_FAILED", "保存投注请求失败", created.Error)
			}
			var previous bet.AssistantRequest
			if err := s.db.Where("user_id = ? AND request_id = ?", userID, requestID).First(&previous).Error; err != nil {
				return nil, apperrors.NewSystemError("REQUEST_READ_FAILED", "读取投注请求失败", err)
			}
			if strings.TrimSpace(previous.ResultJSON) == "" {
				return nil, apperrors.NewBusinessError("REQUEST_IN_PROGRESS", "投注请求处理中，请勿重复提交")
			}
			var cached AssistantBetResult
			if err := json.Unmarshal([]byte(previous.ResultJSON), &cached); err != nil {
				return nil, apperrors.NewSystemError("REQUEST_READ_FAILED", "读取投注结果失败", err)
			}
			return &cached, nil
		}
		result, err := s.place(userID, gameID, issue, content, operator)
		if err != nil {
			_ = s.db.Delete(&row).Error // validation failures may be corrected and retried with the same key.
			return nil, err
		}
		payload, err := json.Marshal(result)
		if err != nil {
			return nil, apperrors.NewSystemError("REQUEST_SAVE_FAILED", "保存投注结果失败", err)
		}
		if err := s.db.Model(&row).Update("result_json", string(payload)).Error; err != nil {
			return nil, apperrors.NewSystemError("REQUEST_SAVE_FAILED", "保存投注结果失败", err)
		}
		return result, nil
	}
	return s.place(userID, gameID, issue, content, operator)
}

func (s *BetAssistantService) place(userID uint64, gameID, issue, content, operator string) (*AssistantBetResult, error) {
	game, err := s.bets.loadGame(gameID)
	if err != nil {
		return nil, err
	}
	if !game.Enabled {
		return nil, apperrors.NewBusinessError("GAME_DISABLED", "该彩种暂未开放投注")
	}
	lines, err := ParseAssistantBet(content)
	if err != nil {
		return nil, err
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
			Remark: "开奖助手识别投注",
		})
		totalCents += int64(math.Round(line.Amount * 100))
	}
	if _, err := s.bets.PlaceBatch(inputs); err != nil {
		return nil, err
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

// Status is read-only. Publishing results remains restricted to the official
// synchronizer and authenticated administrator endpoints.
func (s *BetAssistantService) Status(gameID string) (*AssistantDrawStatus, error) {
	game, err := s.bets.loadGame(gameID)
	if err != nil {
		return nil, err
	}
	issue, err := s.bets.CurrentIssue(game.ID)
	if err != nil {
		return nil, err
	}
	status := &AssistantDrawStatus{
		GameID: game.ID, GameName: game.Name, Issue: issue, NextDrawAt: game.NextDrawAt,
		Accepting:  game.Enabled && game.NextDrawAt.After(time.Now().UTC().Add(3*time.Second)),
		SourceName: game.SourceName,
	}
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

var assistantPositionPlay = regexp.MustCompile(`^([1-6])([大小单双龙虎])$`)
var assistantPositionNumbers = regexp.MustCompile(`^([1-6])/([0-9]+)$`)
var assistantRankPlay = regexp.MustCompile(`^(冠军|亚军|第三名|第四名|第五名|第六名)([0-9大小单双龙虎])$`)

var assistantRanks = map[string]int{
	"冠军": 1, "亚军": 2, "第三名": 3, "第四名": 4, "第五名": 5, "第六名": 6,
}

// ParseAssistantBet understands the compact room syntax, for example:
// 买12345/1000#3大/2000#6/123456/100
// The final amount of a segment is distributed exactly to the cent among its
// individual selections, which prevents invisible rounding differences.
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
		cut := strings.LastIndex(segment, "/")
		if cut <= 0 || cut == len(segment)-1 {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("“%s”缺少金额，请使用 玩法/金额", segment))
		}
		amountCents, err := assistantMoneyCents(strings.TrimSpace(segment[cut+1:]))
		if err != nil {
			return nil, err
		}
		play := strings.TrimSpace(segment[:cut])
		entries, err := assistantPlayEntries(play)
		if err != nil {
			return nil, err
		}
		parts := splitAssistantCents(amountCents, len(entries))
		for index, entry := range entries {
			playCode, playName := InferPlay(entry.playCode, entry.playName, entry.position, entry.selection)
			line := AssistantBetLine{
				Position: entry.position, Selection: entry.selection, PlayCode: playCode, PlayName: playName,
				Amount: centsToAmount(parts[index]),
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
		return assistantNumberEntries(position, match[2], ""), nil
	}
	if match := assistantPositionPlay.FindStringSubmatch(play); len(match) == 3 {
		position, _ := strconv.Atoi(match[1])
		return []assistantEntry{{position: position, selection: match[2]}}, nil
	}
	if match := assistantRankPlay.FindStringSubmatch(play); len(match) == 3 {
		return []assistantEntry{{position: assistantRanks[match[1]], selection: match[2], playName: play}}, nil
	}
	if strings.HasPrefix(play, "冠亚和") {
		selection := strings.TrimPrefix(play, "冠亚和")
		if selection != "大" && selection != "小" && selection != "单" && selection != "双" {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("“%s”不是支持的冠亚和玩法", play))
		}
		return []assistantEntry{{position: 6, selection: selection, playCode: "sum", playName: play}}, nil
	}
	if len(play) == 1 && play[0] >= '0' && play[0] <= '9' {
		return []assistantEntry{{position: 1, selection: play}}, nil
	}
	if allDigits(play) {
		if len(play) > 6 {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "号码组合最多支持 6 位")
		}
		entries := make([]assistantEntry, 0, len(play))
		for index, digit := range play {
			entries = append(entries, assistantEntry{position: index + 1, selection: string(digit)})
		}
		return entries, nil
	}
	return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("无法识别“%s”，请使用例如 3大/200 或 12345/1000", play))
}

func assistantNumberEntries(position int, digits, name string) []assistantEntry {
	entries := make([]assistantEntry, 0, len(digits))
	for _, digit := range digits {
		entries = append(entries, assistantEntry{position: position, selection: string(digit), playName: name})
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

func splitAssistantCents(total int64, count int) []int64 {
	parts := make([]int64, count)
	if count == 0 {
		return parts
	}
	base, remainder := total/int64(count), total%int64(count)
	for index := range parts {
		parts[index] = base
		if int64(index) < remainder {
			parts[index]++
		}
	}
	return parts
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
	names := []string{"一", "二", "三", "四", "五", "六"}
	position := strconv.Itoa(line.Position)
	if line.Position >= 1 && line.Position <= len(names) {
		position = names[line.Position-1]
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
