package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	membernotify "backend/data/models/notify"
	"backend/data/models/user"
	apperrors "backend/errors"
	"backend/ws"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SettlementResult struct {
	GameID        string    `json:"game_id"`
	GameName      string    `json:"game_name"`
	Issue         string    `json:"issue"`
	Numbers       []int     `json:"numbers"`
	PendingBefore int64     `json:"pending_before"`
	Won           int64     `json:"won"`
	Lost          int64     `json:"lost"`
	Skipped       int64     `json:"skipped"`
	StakeAmount   float64   `json:"stake_amount"`
	PayoutAmount  float64   `json:"payout_amount"`
	SettledAt     time.Time `json:"settled_at"`
}

type SettlementStatus struct {
	GameID       string     `json:"game_id"`
	Issue        string     `json:"issue"`
	HasDraw      bool       `json:"has_draw"`
	Numbers      []int      `json:"numbers"`
	DrawAt       *time.Time `json:"draw_at"`
	Pending      int64      `json:"pending"`
	Won          int64      `json:"won"`
	Lost         int64      `json:"lost"`
	StakeAmount  float64    `json:"stake_amount"`
	PayoutAmount float64    `json:"payout_amount"`
	Settled      bool       `json:"settled"`
}

// SettleIssue settles every pending bet for a published draw.
func (s *BetAdminService) SettleIssue(gameID, issue, operator string) (*SettlementResult, error) {
	game, err := s.loadGame(gameID)
	if err != nil {
		return nil, err
	}
	issue = strings.TrimSpace(issue)
	if issue == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "期号不能为空")
	}
	var draw lottery.Draw
	if err := s.db.Where("game_id = ? AND issue = ?", game.ID, issue).First(&draw).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("DRAW_NOT_FOUND", "该期尚未开奖，无法结算")
		}
		return nil, apperrors.NewSystemError("DRAW_READ_FAILED", "读取开奖结果失败", err)
	}
	numbers := parseNumbers(draw.Numbers)
	if len(numbers) == 0 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "开奖号码无效")
	}

	var pending []bet.Bet
	if err := s.db.Where("game_id = ? AND issue = ? AND status = ?", game.ID, issue, "pending").Order("id asc").Find(&pending).Error; err != nil {
		return nil, apperrors.NewSystemError("BET_READ_FAILED", "读取待结算注单失败", err)
	}
	result := &SettlementResult{
		GameID: game.ID, GameName: game.Name, Issue: issue, Numbers: numbers,
		PendingBefore: int64(len(pending)), SettledAt: time.Now().UTC(),
	}
	if len(pending) == 0 {
		// A draw is still a room event even when nobody placed a bet. Clients use
		// it to refresh the clock and let the draw assistant announce the result.
		ws.NotifyDraw(gameID, issue, numbers)
		return result, nil
	}

	operator = defaultString(strings.TrimSpace(operator), "系统结算")
	summaries := map[uint64]*settleUserSummary{}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		for _, item := range pending {
			won, reason := evaluateBet(numbers, item.PlayCode, item.Position, item.Selection)
			payout := int64(0)
			status := "lost"
			if won {
				status = "won"
				payout = int64(math.Round(float64(item.AmountCents) * item.Odds))
			}
			summary := summaries[item.UserID]
			if summary == nil {
				summary = &settleUserSummary{}
				summaries[item.UserID] = summary
			}
			summary.stakeCents += item.AmountCents
			if won {
				summary.wonCount++
				summary.payoutCents += payout
			} else {
				summary.lostCount++
			}
			updates := map[string]any{
				"status":       status,
				"payout_cents": payout,
				"remark":       trimRemark(item.Remark, reason),
				"operator":     operator,
				"updated_at":   time.Now().UTC(),
			}
			if err := tx.Model(&bet.Bet{}).Where("id = ? AND status = ?", item.ID, "pending").Updates(updates).Error; err != nil {
				return err
			}
			result.StakeAmount += centsToAmount(item.AmountCents)
			if won {
				result.Won++
				result.PayoutAmount += centsToAmount(payout)
				if payout > 0 {
					if err := creditSettlement(tx, item.UserID, payout, game.Name, issue, operator); err != nil {
						return err
					}
				}
			} else {
				result.Lost++
			}
		}
		return nil
	})
	if err != nil {
		if app, ok := err.(*apperrors.AppError); ok {
			return nil, app
		}
		return nil, apperrors.NewSystemError("SETTLEMENT_FAILED", "开奖结算失败", err)
	}
	notifySettlementResults(s.db, game.Name, issue, numbers, summaries)
	ws.NotifyDraw(gameID, issue, numbers)
	return result, nil
}

type settleUserSummary struct {
	wonCount    int
	lostCount   int
	stakeCents  int64
	payoutCents int64
}

func notifySettlementResults(db *gorm.DB, gameName, issue string, numbers []int, summaries map[uint64]*settleUserSummary) {
	numText := formatDrawNumbers(numbers)
	for userID, summary := range summaries {
		if summary == nil || (summary.wonCount == 0 && summary.lostCount == 0) {
			continue
		}
		title := "开奖结果"
		level := "info"
		content := fmt.Sprintf("【%s · %s】开奖 %s。", gameName, issue, numText)
		if summary.wonCount > 0 {
			level = "success"
			title = "恭喜中奖"
			content += fmt.Sprintf(" 您有 %d 注中奖，派彩 %.2f 元。", summary.wonCount, centsToAmount(summary.payoutCents))
		} else {
			level = "warning"
			title = "未中奖"
			content += fmt.Sprintf(" 本期 %d 注未中奖，投注 %.2f 元。", summary.lostCount, centsToAmount(summary.stakeCents))
		}
		_ = db.Create(&membernotify.MemberNotification{
			UserID: userID, Title: title, Content: content,
			Level: level, Category: "winning",
		}).Error
		ws.NotifyUser(userID, "notification", map[string]any{
			"title": title, "content": content, "level": level, "category": "winning",
		})
	}
}

func formatDrawNumbers(numbers []int) string {
	parts := make([]string, len(numbers))
	for i, n := range numbers {
		parts[i] = fmt.Sprintf("%d", n)
	}
	return strings.Join(parts, "、")
}

// PublishDraw creates (or reuses) a draw for simulated games and settles bets.
func (s *BetAdminService) PublishDraw(gameID, issue string, numbers []int, operator string) (*SettlementResult, error) {
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
	if len(numbers) == 0 {
		numbers = defaultDrawNumbers(game.Category, issue)
	}
	if len(numbers) < 3 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "开奖号码至少需要 3 个")
	}
	now := time.Now().UTC()
	draw := lottery.Draw{GameID: game.ID, Issue: issue, Numbers: joinNumbers(numbers), DrawAt: now}
	create := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "game_id"}, {Name: "issue"}},
		DoNothing: true,
	}).Create(&draw)
	if create.Error != nil {
		return nil, apperrors.NewSystemError("DRAW_CREATE_FAILED", "写入开奖结果失败", create.Error)
	}
	_ = s.db.Model(&lottery.Game{}).Where("id = ?", game.ID).Updates(map[string]any{
		"next_draw_at": now.Add(time.Duration(maxInt(game.DrawInterval, 60)) * time.Second),
		"sync_status":  "ok",
		"last_sync_at": now,
	}).Error
	return s.SettleIssue(game.ID, issue, defaultString(operator, "手动开奖结算"))
}

func (s *BetAdminService) SettlementStatus(gameID, issue string) (*SettlementStatus, error) {
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
	status := &SettlementStatus{GameID: game.ID, Issue: issue}
	var draw lottery.Draw
	if err := s.db.Where("game_id = ? AND issue = ?", game.ID, issue).First(&draw).Error; err == nil {
		status.HasDraw = true
		status.Numbers = parseNumbers(draw.Numbers)
		status.DrawAt = &draw.DrawAt
	} else if err != gorm.ErrRecordNotFound {
		return nil, apperrors.NewSystemError("DRAW_READ_FAILED", "读取开奖结果失败", err)
	}
	type agg struct {
		Status      string
		Cnt         int64
		StakeCents  int64
		PayoutCents int64
	}
	var rows []agg
	if err := s.db.Model(&bet.Bet{}).
		Select("status, COUNT(*) as cnt, COALESCE(SUM(amount_cents),0) as stake_cents, COALESCE(SUM(payout_cents),0) as payout_cents").
		Where("game_id = ? AND issue = ?", game.ID, issue).
		Group("status").Scan(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("BET_READ_FAILED", "读取结算状态失败", err)
	}
	for _, row := range rows {
		status.StakeAmount += centsToAmount(row.StakeCents)
		status.PayoutAmount += centsToAmount(row.PayoutCents)
		switch row.Status {
		case "pending":
			status.Pending = row.Cnt
		case "won":
			status.Won = row.Cnt
		case "lost":
			status.Lost = row.Cnt
		}
	}
	status.Settled = status.HasDraw && status.Pending == 0 && (status.Won+status.Lost) > 0
	return status, nil
}

// SettleImportedDraw is called after a new official draw row is inserted.
func (s *BetAdminService) SettleImportedDraw(gameID, issue string) {
	if strings.TrimSpace(gameID) == "" || strings.TrimSpace(issue) == "" {
		return
	}
	// Always pass a newly imported official draw through SettleIssue. Besides
	// settling bets, it publishes draw_update so rooms without bets also receive
	// the automatic draw announcement.
	_, _ = s.SettleIssue(gameID, issue, "官方开奖自动结算")
}

func creditSettlement(tx *gorm.DB, userID uint64, payoutCents int64, gameName, issue, operator string) error {
	var account user.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, userID).Error; err != nil {
		return err
	}
	before := account.BalanceCents
	after := before + payoutCents
	if err := tx.Model(&account).Update("balance_cents", after).Error; err != nil {
		return err
	}
	return tx.Create(&user.BalanceTransaction{
		UserID: userID, AmountCents: payoutCents, BeforeCents: before, AfterCents: after,
		Type: "settlement", Remark: fmt.Sprintf("开奖派彩 %s/%s", gameName, issue), Operator: operator,
	}).Error
}

func evaluateBet(numbers []int, playCode string, position int, selection string) (bool, string) {
	playCode = strings.ToLower(strings.TrimSpace(playCode))
	selection = normalizeSelection(selection)
	balls := usableBalls(numbers)

	switch playCode {
	case "ball_1_5":
		digit, ok := parseDigit(selection)
		if !ok {
			return false, "号码无效"
		}
		if position >= 1 && position <= len(balls) {
			if balls[position-1] == digit {
				return true, fmt.Sprintf("第%d球开出 %d", position, digit)
			}
			return false, fmt.Sprintf("第%d球开出 %d", position, balls[position-1])
		}
		if position == 6 {
			tail := sumInts(balls) % 10
			if tail == digit {
				return true, fmt.Sprintf("总和尾数 %d", digit)
			}
			return false, fmt.Sprintf("总和尾数 %d", tail)
		}
		return false, "球位无效"

	case "two_sided":
		value, label := sideValue(balls, position)
		if position == 6 || position < 1 || position > len(balls) {
			if matchSumSize(value, len(balls), selection) {
				return true, label + "命中" + selectionLabel(selection)
			}
			return false, fmt.Sprintf("%s为%d", label, value)
		}
		if matchSide(value, selection) {
			return true, label + "命中" + selectionLabel(selection)
		}
		return false, label + "为" + describeSide(value)

	case "dragon_tiger":
		if len(balls) < 2 {
			return false, "号码不足"
		}
		left, right := balls[0], balls[len(balls)-1]
		outcome := "tie"
		if left > right {
			outcome = "dragon"
		} else if left < right {
			outcome = "tiger"
		}
		if selection == outcome || selection == "和" && outcome == "tie" || selection == "龙" && outcome == "dragon" || selection == "虎" && outcome == "tiger" {
			return true, fmt.Sprintf("龙虎 %d:%d", left, right)
		}
		return false, fmt.Sprintf("龙虎 %d:%d", left, right)

	case "sum":
		total := sumInts(balls)
		if digit, ok := parseDigit(selection); ok {
			if total%10 == digit {
				return true, fmt.Sprintf("总和尾 %d", digit)
			}
			return false, fmt.Sprintf("总和尾 %d", total%10)
		}
		if matchSide(total, selection) || matchSumSize(total, len(balls), selection) {
			return true, fmt.Sprintf("总和 %d 命中", total)
		}
		return false, fmt.Sprintf("总和 %d", total)

	case "leopard", "straight", "pair", "half_straight", "mixed":
		pattern := frontPattern(balls)
		want := playCode
		if selection != "" && selection != "yes" && selection != "中" {
			want = normalizePlaySelection(selection)
		}
		if pattern == want {
			return true, "形态命中" + playNameOf(pattern)
		}
		return false, "形态为" + playNameOf(pattern)

	default:
		// Unknown play: treat like ball number on given position.
		digit, ok := parseDigit(selection)
		if ok && position >= 1 && position <= len(balls) {
			if balls[position-1] == digit {
				return true, "号码命中"
			}
			return false, "号码未中"
		}
		return false, "未知玩法按未中处理"
	}
}

func usableBalls(numbers []int) []int {
	if len(numbers) == 0 {
		return numbers
	}
	// Prefer first 5 balls for SSC-style matrices; keep all when shorter.
	if len(numbers) > 5 {
		return numbers[:5]
	}
	return numbers
}

func sideValue(balls []int, position int) (int, string) {
	if position >= 1 && position <= len(balls) {
		return balls[position-1], fmt.Sprintf("第%d球", position)
	}
	return sumInts(balls), "总和"
}

func matchSide(value int, selection string) bool {
	switch selection {
	case "big", "大":
		return value >= 5
	case "small", "小":
		return value <= 4
	case "odd", "单":
		return value%2 == 1
	case "even", "双":
		return value%2 == 0
	}
	return false
}

func matchSumSize(total, ballCount int, selection string) bool {
	// SSC 5 balls: 0-45, mid 23. 3 balls: 0-27, mid 14.
	threshold := 23
	if ballCount <= 3 {
		threshold = 14
	}
	switch selection {
	case "big", "大":
		return total >= threshold
	case "small", "小":
		return total < threshold
	case "odd", "单":
		return total%2 == 1
	case "even", "双":
		return total%2 == 0
	}
	return false
}

func frontPattern(balls []int) string {
	if len(balls) < 3 {
		return "mixed"
	}
	a, b, c := balls[0], balls[1], balls[2]
	if a == b && b == c {
		return "leopard"
	}
	vals := []int{a, b, c}
	sort.Ints(vals)
	// Treat 0,1,9 / 0,8,9 style wrap as straight for SSC.
	if isStraight(vals) {
		return "straight"
	}
	if a == b || b == c || a == c {
		return "pair"
	}
	if math.Abs(float64(vals[0]-vals[1])) == 1 || math.Abs(float64(vals[1]-vals[2])) == 1 || wrapAdjacent(vals[0], vals[2]) {
		return "half_straight"
	}
	return "mixed"
}

func isStraight(sorted []int) bool {
	if len(sorted) != 3 {
		return false
	}
	if sorted[0]+1 == sorted[1] && sorted[1]+1 == sorted[2] {
		return true
	}
	// Circular straights commonly used in SSC: 890 / 901 / 019
	a, b, c := sorted[0], sorted[1], sorted[2]
	return (a == 0 && b == 8 && c == 9) || (a == 0 && b == 1 && c == 9)
}

func wrapAdjacent(a, b int) bool {
	return (a == 0 && b == 9) || (a == 9 && b == 0)
}

func normalizeSelection(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "龙":
		return "dragon"
	case "虎":
		return "tiger"
	case "和", "和局":
		return "tie"
	case "大":
		return "big"
	case "小":
		return "small"
	case "单":
		return "odd"
	case "双":
		return "even"
	}
	return value
}

func normalizePlaySelection(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "豹子", "leopard":
		return "leopard"
	case "顺子", "straight":
		return "straight"
	case "对子", "pair":
		return "pair"
	case "半顺", "half_straight":
		return "half_straight"
	case "杂六", "mixed":
		return "mixed"
	}
	return strings.ToLower(value)
}

func selectionLabel(selection string) string {
	switch selection {
	case "big":
		return "大"
	case "small":
		return "小"
	case "odd":
		return "单"
	case "even":
		return "双"
	case "dragon":
		return "龙"
	case "tiger":
		return "虎"
	case "tie":
		return "和"
	}
	return selection
}

func describeSide(value int) string {
	size := "小"
	if value >= 5 {
		size = "大"
	}
	parity := "双"
	if value%2 == 1 {
		parity = "单"
	}
	return fmt.Sprintf("%d(%s/%s)", value, size, parity)
}

func playNameOf(code string) string {
	switch code {
	case "leopard":
		return "豹子"
	case "straight":
		return "顺子"
	case "pair":
		return "对子"
	case "half_straight":
		return "半顺"
	default:
		return "杂六"
	}
}

func sumInts(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func trimRemark(existing, reason string) string {
	reason = strings.TrimSpace(reason)
	if existing == "" {
		return reason
	}
	if reason == "" {
		return existing
	}
	if strings.Contains(existing, reason) {
		return existing
	}
	merged := existing + " | " + reason
	if len(merged) > 280 {
		return merged[:280]
	}
	return merged
}

func defaultDrawNumbers(category, issue string) []int {
	seed := int(time.Now().UnixNano())
	for _, ch := range issue {
		seed += int(ch)
	}
	count := 5
	switch {
	case strings.Contains(category, "3"):
		count = 3
	case strings.Contains(category, "赛车"), strings.Contains(category, "飞艇"), strings.Contains(category, "幸运10"):
		count = 10
	}
	result := make([]int, count)
	if count == 10 {
		// 1-10 permutation style
		pool := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
		for i := range result {
			idx := (seed + i*7) % len(pool)
			result[i] = pool[idx]
			pool = append(pool[:idx], pool[idx+1:]...)
		}
		return result
	}
	for i := range result {
		result[i] = (seed + i*3 + i*i) % 10
	}
	return result
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
