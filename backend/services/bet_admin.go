package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/data/models/user"
	apperrors "backend/errors"
	"backend/ws"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BetAdminService struct{ db *gorm.DB }

type BetView struct {
	ID        uint64    `json:"id"`
	GameID    string    `json:"game_id"`
	Issue     string    `json:"issue"`
	UserID    uint64    `json:"user_id"`
	Username  string    `json:"username"`
	PlayCode  string    `json:"play_code"`
	PlayName  string    `json:"play_name"`
	Position  int       `json:"position"`
	Selection string    `json:"selection"`
	Amount    float64   `json:"amount"`
	Odds      float64   `json:"odds"`
	Status    string    `json:"status"`
	Payout    float64   `json:"payout"`
	FlyAmount float64   `json:"fly_amount"`
	Remark    string    `json:"remark"`
	Operator  string    `json:"operator"`
	CreatedAt time.Time `json:"created_at"`
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
	Matrix      [][]float64       `json:"matrix"` // 10 rows (0-9) x 6 cols (ball1-5 + sum)
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
	UserBalance       float64 `json:"user_balance"`
	TodayTurnover     float64 `json:"today_turnover"`
	TodayGrossProfit  float64 `json:"today_gross_profit"`
	TodayNetProfit    float64 `json:"today_net_profit"`
	TodayRebate       float64 `json:"today_rebate"`
	TodayWelfare      float64 `json:"today_welfare"`
	TotalGrossProfit  float64 `json:"total_gross_profit"`
	TotalNetProfit    float64 `json:"total_net_profit"`
	PendingSettlement float64 `json:"pending_settlement"`
	TodayProfit       float64 `json:"today_profit"` // 兼容旧字段 = 毛利
	TotalProfit       float64 `json:"total_profit"` // 兼容旧字段 = 毛利
}

type gameMoney struct {
	GameID      string
	Turnover    float64
	GrossProfit float64
	Profit      float64 // 兼容旧字段 = 毛利
}

func NewBetAdminService(db *gorm.DB) *BetAdminService { return &BetAdminService{db: db} }

func (s *BetAdminService) Place(input PlaceBetInput) (*BetView, error) {
	game, err := s.loadGame(input.GameID)
	if err != nil {
		return nil, err
	}
	if !game.Enabled {
		return nil, apperrors.NewBusinessError("GAME_DISABLED", "该彩种暂未开放投注")
	}
	issue := strings.TrimSpace(input.Issue)
	if issue == "" {
		issue, err = s.CurrentIssue(game.ID)
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
	if input.Position < 1 || input.Position > 6 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "球位不正确")
	}
	amountCents := int64(math.Round(input.Amount * 100))
	if amountCents <= 0 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "下注金额必须大于 0")
	}
	playCode, playName := InferPlay(input.PlayCode, input.PlayName, input.Position, input.Selection)
	if err := s.validateBetLimits(game.ID, issue, input.UserID, playCode, input.Position, selection, amountCents); err != nil {
		return nil, err
	}

	requestFly := -1.0
	if input.FlyAmount != nil {
		requestFly = *input.FlyAmount
	}
	resolved, err := NewTradingAdminService(s.db).Resolve(input.UserID, game.ID, playCode, input.Amount, input.Odds, requestFly)
	if err != nil {
		return nil, err
	}
	odds := resolved.Odds
	flyCents := clampFlyCents(amountCents, int64(math.Round(resolved.FlyAmount*100)))

	var view *BetView
	var afterBalance int64
	var roomScope string
	err = s.db.Transaction(func(tx *gorm.DB) error {
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
		if account.BalanceCents < amountCents {
			return apperrors.NewBusinessError("INSUFFICIENT_BALANCE", "用户余额不足")
		}
		roomScope = betRoomScope(account)
		before := account.BalanceCents
		after := before - amountCents
		afterBalance = after
		if err := tx.Model(&account).Update("balance_cents", after).Error; err != nil {
			return err
		}
		ledger := user.BalanceTransaction{
			UserID: account.UserID, AmountCents: -amountCents, BeforeCents: before, AfterCents: after,
			Type: "bet", Remark: fmt.Sprintf("下注 %s/%s", game.Name, issue), Operator: defaultString(input.Operator, "后台管理员"),
		}
		if err := tx.Create(&ledger).Error; err != nil {
			return err
		}
		row := bet.Bet{
			GameID: game.ID, Issue: issue, RoomScope: roomScope, UserID: account.UserID, Username: account.Username,
			PlayCode: playCode, PlayName: playName, Position: input.Position, Selection: selection,
			AmountCents: amountCents, Odds: odds, Status: "pending", FlyCents: flyCents,
			Remark: strings.TrimSpace(input.Remark), Operator: defaultString(input.Operator, "后台管理员"),
		}
		// Upsert: same user/play/position/selection on same issue accumulates amount.
		existing := bet.Bet{}
		findErr := tx.Where("room_scope = ? AND game_id = ? AND issue = ? AND user_id = ? AND play_code = ? AND position = ? AND selection = ?",
			row.RoomScope, row.GameID, row.Issue, row.UserID, row.PlayCode, row.Position, row.Selection).First(&existing).Error
		if findErr == nil {
			existing.AmountCents += amountCents
			existing.FlyCents += flyCents
			existing.Odds = odds
			existing.Remark = row.Remark
			existing.Operator = row.Operator
			if err := tx.Save(&existing).Error; err != nil {
				return err
			}
			v := toBetView(existing)
			view = &v
			return nil
		}
		if findErr != gorm.ErrRecordNotFound {
			return findErr
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
	if recipients, recipientsErr := betScopeRecipients(s.db, roomScope); recipientsErr == nil {
		ws.NotifyBetFeed(recipients, game.ID, issue, roomScope)
	}
	ws.NotifyUser(input.UserID, "balance", map[string]any{"balance": centsToAmount(afterBalance)})
	return view, nil
}

// PlaceBatch accepts an already-validated ticket as one financial operation.
// All rows and the balance deduction are committed together, so a later line
// can never leave the member with a partially accepted ticket.
func (s *BetAdminService) PlaceBatch(inputs []PlaceBetInput) ([]BetView, error) {
	if len(inputs) == 0 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "请至少提供一注投注内容")
	}
	game, err := s.loadGame(inputs[0].GameID)
	if err != nil {
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
		issue, err = s.CurrentIssue(game.ID)
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
		odds        float64
		flyCents    int64
	}
	prepared := make([]preparedBet, 0, len(inputs))
	var totalCents int64
	for _, input := range inputs {
		if strings.TrimSpace(input.GameID) != game.ID || input.UserID != userID {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "同一张投注单只能包含同一彩种和用户")
		}
		if requestedIssue := strings.TrimSpace(input.Issue); requestedIssue != "" && requestedIssue != issue {
			return nil, apperrors.NewBusinessError("ISSUE_MISMATCH", "同一张投注单的期号必须一致")
		}
		selection := strings.TrimSpace(input.Selection)
		if selection == "" {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "投注内容不能为空")
		}
		if input.Position < 1 || input.Position > 6 {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "球位不正确")
		}
		amountCents := int64(math.Round(input.Amount * 100))
		if amountCents <= 0 {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "下注金额必须大于 0")
		}
		playCode, playName := InferPlay(input.PlayCode, input.PlayName, input.Position, selection)
		if err := s.validateBetLimits(game.ID, issue, userID, playCode, input.Position, selection, amountCents); err != nil {
			return nil, err
		}
		requestFly := -1.0
		if input.FlyAmount != nil {
			requestFly = *input.FlyAmount
		}
		resolved, err := NewTradingAdminService(s.db).Resolve(userID, game.ID, playCode, centsToAmount(amountCents), input.Odds, requestFly)
		if err != nil {
			return nil, err
		}
		prepared = append(prepared, preparedBet{
			input: input, playCode: playCode, playName: playName, selection: selection,
			amountCents: amountCents, odds: resolved.Odds,
			flyCents: clampFlyCents(amountCents, int64(math.Round(resolved.FlyAmount*100))),
		})
		totalCents += amountCents
	}

	views := make([]BetView, 0, len(prepared))
	var afterBalance int64
	var roomScope string
	err = s.db.Transaction(func(tx *gorm.DB) error {
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
		if account.BalanceCents < totalCents {
			return apperrors.NewBusinessError("INSUFFICIENT_BALANCE", "用户余额不足")
		}
		roomScope = betRoomScope(account)
		before := account.BalanceCents
		afterBalance = before - totalCents
		if err := tx.Model(&account).Update("balance_cents", afterBalance).Error; err != nil {
			return err
		}
		operator := defaultString(inputs[0].Operator, "开奖助手")
		if err := tx.Create(&user.BalanceTransaction{
			UserID: account.UserID, AmountCents: -totalCents, BeforeCents: before, AfterCents: afterBalance,
			Type: "bet", Remark: fmt.Sprintf("助手下注 %s/%s（%d 注）", game.Name, issue, len(prepared)), Operator: operator,
		}).Error; err != nil {
			return err
		}
		for _, item := range prepared {
			row := bet.Bet{
				GameID: game.ID, Issue: issue, RoomScope: roomScope, UserID: account.UserID, Username: account.Username,
				PlayCode: item.playCode, PlayName: item.playName, Position: item.input.Position, Selection: item.selection,
				AmountCents: item.amountCents, Odds: item.odds, Status: "pending", FlyCents: item.flyCents,
				Remark: strings.TrimSpace(item.input.Remark), Operator: defaultString(item.input.Operator, operator),
			}
			existing := bet.Bet{}
			findErr := tx.Where("room_scope = ? AND game_id = ? AND issue = ? AND user_id = ? AND play_code = ? AND position = ? AND selection = ?",
				row.RoomScope, row.GameID, row.Issue, row.UserID, row.PlayCode, row.Position, row.Selection).First(&existing).Error
			if findErr == nil {
				existing.AmountCents += item.amountCents
				existing.FlyCents += item.flyCents
				existing.Odds = item.odds
				existing.Remark = row.Remark
				existing.Operator = row.Operator
				if err := tx.Save(&existing).Error; err != nil {
					return err
				}
				views = append(views, toBetView(existing))
				continue
			}
			if findErr != gorm.ErrRecordNotFound {
				return findErr
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			views = append(views, toBetView(row))
		}
		return nil
	})
	if err != nil {
		if app, ok := err.(*apperrors.AppError); ok {
			return nil, app
		}
		return nil, apperrors.NewSystemError("BET_CREATE_FAILED", "创建注单失败", err)
	}
	if recipients, recipientsErr := betScopeRecipients(s.db, roomScope); recipientsErr == nil {
		ws.NotifyBetFeed(recipients, game.ID, issue, roomScope)
	}
	ws.NotifyUser(userID, "balance", map[string]any{"balance": centsToAmount(afterBalance)})
	return views, nil
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
	return "lobby"
}

func betScopeRecipients(db *gorm.DB, scope string) ([]uint64, error) {
	query := db.Model(&user.User{}).Where("status = ?", 1)
	if strings.HasPrefix(scope, "agent:") {
		id, err := strconv.ParseUint(strings.TrimPrefix(scope, "agent:"), 10, 64)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("invalid bet room scope")
		}
		query = query.Where("user_id = ? OR parent_agent_id = ?", id, id)
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
	matrix := make([][]float64, 10)
	for i := range matrix {
		matrix[i] = make([]float64, 6)
	}
	var total int64
	users := map[uint64]struct{}{}
	for _, row := range rows {
		total += row.AmountCents
		users[row.UserID] = struct{}{}
		digit, ok := parseDigit(row.Selection)
		if !ok || row.Position < 1 || row.Position > 6 {
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

func (s *BetAdminService) SeedDemo(gameID string) (*MonitorSnapshot, error) {
	game, err := s.loadGame(gameID)
	if err != nil {
		return nil, err
	}
	issue, err := s.CurrentIssue(game.ID)
	if err != nil {
		return nil, err
	}
	var count int64
	if err := s.db.Model(&bet.Bet{}).Where("game_id = ? AND issue = ?", game.ID, issue).Count(&count).Error; err != nil {
		return nil, apperrors.NewSystemError("MONITOR_READ_FAILED", "读取注单失败", err)
	}
	if count > 0 {
		return s.Monitor(game.ID, issue)
	}
	var members []user.User
	if err := s.db.Where("role = ? AND status = 1", "member").Order("user_id asc").Limit(5).Find(&members).Error; err != nil {
		return nil, apperrors.NewSystemError("USER_READ_FAILED", "读取用户失败", err)
	}
	if len(members) == 0 {
		// Fall back to any non-admin active user, else admin itself for local demo.
		if err := s.db.Where("status = 1").Order("user_id asc").Limit(3).Find(&members).Error; err != nil {
			return nil, err
		}
	}
	if len(members) == 0 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "没有可用于演示下注的用户，请先在用户管理中创建会员")
	}
	demos := []struct {
		Position  int
		Selection string
		Amount    float64
		PlayCode  string
		PlayName  string
		Odds      float64
	}{
		{1, "3", 120, "ball_1_5", "1-5球号", 9.9},
		{1, "7", 80, "ball_1_5", "1-5球号", 9.9},
		{2, "0", 50, "ball_1_5", "1-5球号", 9.9},
		{2, "5", 200, "ball_1_5", "1-5球号", 9.9},
		{3, "8", 90, "ball_1_5", "1-5球号", 9.9},
		{4, "2", 60, "ball_1_5", "1-5球号", 9.9},
		{4, "9", 150, "ball_1_5", "1-5球号", 9.9},
		{5, "1", 70, "ball_1_5", "1-5球号", 9.9},
		{6, "4", 110, "ball_1_5", "1-5球号", 9.9},
		{6, "6", 40, "ball_1_5", "1-5球号", 9.9},
		{1, "big", 100, "two_sided", "两面", 1.993},
		{6, "odd", 80, "two_sided", "两面", 1.993},
		{1, "dragon", 60, "dragon_tiger", "龙虎", 2.0},
		{1, "豹子", 40, "leopard", "豹子", 60},
	}
	for i, demo := range demos {
		member := members[i%len(members)]
		_ = s.db.Model(&user.User{}).Where("user_id = ? AND balance_cents < ?", member.UserID, int64(demo.Amount*100)+10000).
			Update("balance_cents", gorm.Expr("balance_cents + ?", 500000)).Error
		fly := demo.Amount * 0.3
		_, placeErr := s.Place(PlaceBetInput{
			GameID: game.ID, Issue: issue, UserID: member.UserID,
			PlayCode: demo.PlayCode, PlayName: demo.PlayName, Position: demo.Position, Selection: demo.Selection,
			Amount: demo.Amount, Odds: demo.Odds, FlyAmount: &fly, Remark: "演示注单", Operator: "系统演示",
		})
		if placeErr != nil {
			return nil, placeErr
		}
	}
	return s.Monitor(game.ID, issue)
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
	pairQuery := s.db.Model(&bet.Bet{}).Where(`user_id NOT IN (SELECT user_id FROM "user" WHERE remark = ?)`, roomActivityRemark).Select("game_id, issue")
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
	query := s.db.Model(&bet.Bet{}).Where(`user_id NOT IN (SELECT user_id FROM "user" WHERE remark = ?)`, roomActivityRemark).
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
	Query    string
	GameID   string
	Issue    string
	UserID   uint64
	Status   string
	Page     int
	PageSize int
}

type BetListResult struct {
	Items    []BetView `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}

func (s *BetAdminService) List(filter BetListFilter) (*BetListResult, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	query := s.db.Model(&bet.Bet{})
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
			query = query.Where("status IN ?", []string{"won", "lost", "cancelled"})
		} else {
			query = query.Where("status = ?", st)
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, apperrors.NewSystemError("BET_READ_FAILED", "读取注单失败", err)
	}
	var rows []bet.Bet
	if err := query.Order("id desc").Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("BET_READ_FAILED", "读取注单失败", err)
	}
	items := make([]BetView, 0, len(rows))
	for _, row := range rows {
		items = append(items, toBetView(row))
	}
	return &BetListResult{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (s *BetAdminService) Cancel(id uint64, operator string) (*BetView, error) {
	var view BetView
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var row bet.Bet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("NOT_FOUND", "注单不存在")
			}
			return err
		}
		if row.Status != "pending" {
			return apperrors.NewBusinessError("INVALID_REQUEST", "仅待结算注单可撤单")
		}
		var account user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, row.UserID).Error; err != nil {
			return apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
		}
		after := account.BalanceCents + row.AmountCents
		if err := tx.Model(&account).Update("balance_cents", after).Error; err != nil {
			return err
		}
		if err := tx.Create(&user.BalanceTransaction{
			UserID: account.UserID, AmountCents: row.AmountCents, BeforeCents: account.BalanceCents, AfterCents: after,
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

func (s *BetAdminService) DashboardStats() (*DashboardStats, error) {
	var totalBalance int64
	if err := s.db.Model(&user.User{}).Where("remark IS NULL OR remark <> ?", roomActivityRemark).Select("COALESCE(SUM(balance_cents),0)").Scan(&totalBalance).Error; err != nil {
		return nil, err
	}
	start := startOfDayCST(time.Now())
	var todayStake, todaySettledStake, todayPayout, pending, allSettledStake, allPayout int64
	realBets := func() *gorm.DB {
		return s.db.Model(&bet.Bet{}).Where(`user_id NOT IN (SELECT user_id FROM "user" WHERE remark = ?)`, roomActivityRemark)
	}
	if err := realBets().Where("created_at >= ? AND status <> ?", start, "cancelled").
		Select("COALESCE(SUM(amount_cents),0)").Scan(&todayStake).Error; err != nil {
		return nil, err
	}
	if err := realBets().Where("created_at >= ? AND status IN ?", start, []string{"won", "lost"}).
		Select("COALESCE(SUM(amount_cents),0)").Scan(&todaySettledStake).Error; err != nil {
		return nil, err
	}
	if err := realBets().Where("created_at >= ? AND status IN ?", start, []string{"won", "lost"}).
		Select("COALESCE(SUM(payout_cents),0)").Scan(&todayPayout).Error; err != nil {
		return nil, err
	}
	if err := realBets().Where("status = ?", "pending").Select("COALESCE(SUM(amount_cents),0)").Scan(&pending).Error; err != nil {
		return nil, err
	}
	if err := realBets().Where("status IN ?", []string{"won", "lost"}).
		Select("COALESCE(SUM(amount_cents),0)").Scan(&allSettledStake).Error; err != nil {
		return nil, err
	}
	if err := realBets().Where("status IN ?", []string{"won", "lost"}).
		Select("COALESCE(SUM(payout_cents),0)").Scan(&allPayout).Error; err != nil {
		return nil, err
	}
	todayRebate, err := NewRebateAdminService(s.db).TodayAmount()
	if err != nil {
		todayRebate = 0
	}
	todayWelfare, err := s.welfareCostSince(start)
	if err != nil {
		todayWelfare = 0
	}
	allWelfare, err := s.welfareCostSince(time.Time{})
	if err != nil {
		allWelfare = 0
	}
	todayGross := centsToAmount(todaySettledStake - todayPayout)
	totalGross := centsToAmount(allSettledStake - allPayout)
	return &DashboardStats{
		UserBalance:       centsToAmount(totalBalance),
		TodayTurnover:     centsToAmount(todayStake),
		TodayGrossProfit:  todayGross,
		TodayNetProfit:    todayGross - todayWelfare,
		TodayRebate:       todayRebate,
		TodayWelfare:      todayWelfare,
		TotalGrossProfit:  totalGross,
		TotalNetProfit:    totalGross - allWelfare,
		PendingSettlement: centsToAmount(pending),
		TodayProfit:       todayGross,
		TotalProfit:       totalGross,
	}, nil
}

func (s *BetAdminService) welfareCostSince(start time.Time) (float64, error) {
	query := s.db.Model(&user.BalanceTransaction{}).Where("type IN ? AND amount_cents > 0",
		[]string{"rebate", "checkin", "redpacket", "invite"})
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
	if err := s.db.Model(&bet.Bet{}).Where(`user_id NOT IN (SELECT user_id FROM "user" WHERE remark = ?)`, roomActivityRemark).
		Select("game_id, COALESCE(SUM(CASE WHEN status <> 'cancelled' THEN amount_cents ELSE 0 END),0) as stake_cents, COALESCE(SUM(CASE WHEN status IN ('won','lost') THEN amount_cents ELSE 0 END),0) as settled_stake_cents, COALESCE(SUM(CASE WHEN status IN ('won','lost') THEN payout_cents ELSE 0 END),0) as payout_cents").
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
	var draw lottery.Draw
	err := s.db.Where("game_id = ?", gameID).Order("draw_at desc").First(&draw).Error
	if err == nil && strings.TrimSpace(draw.Issue) != "" {
		return nextIssue(draw.Issue), nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return "", apperrors.NewSystemError("DRAW_READ_FAILED", "读取开奖期号失败", err)
	}
	now := time.Now().In(time.FixedZone("CST", 8*3600))
	return now.Format("200601021504"), nil
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

func toBetView(row bet.Bet) BetView {
	return BetView{
		ID: row.ID, GameID: row.GameID, Issue: row.Issue, UserID: row.UserID, Username: row.Username,
		PlayCode: row.PlayCode, PlayName: row.PlayName, Position: row.Position, Selection: row.Selection,
		Amount: centsToAmount(row.AmountCents), Odds: row.Odds, Status: row.Status,
		Payout: centsToAmount(row.PayoutCents), FlyAmount: centsToAmount(row.FlyCents),
		Remark: row.Remark, Operator: row.Operator, CreatedAt: row.CreatedAt,
	}
}

func parseDigit(value string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 0 || n > 9 {
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

func (s *BetAdminService) validateBetLimits(gameID, issue string, userID uint64, playCode string, position int, selection string, amountCents int64) error {
	limits, err := NewOddsAdminService(s.db).Get(gameID)
	if err != nil {
		return err
	}
	var limit *PlayLimitItem
	for i := range limits.Items {
		if limits.Items[i].PlayCode == playCode {
			limit = &limits.Items[i]
			break
		}
	}
	if limit == nil {
		return nil
	}
	amount := centsToAmount(amountCents)
	if limit.MinBet > 0 && amount < limit.MinBet {
		return apperrors.NewBusinessError("BET_TOO_SMALL", fmt.Sprintf("单注最低 %.2f 元", limit.MinBet))
	}
	var existing bet.Bet
	findErr := s.db.Where(
		"game_id = ? AND issue = ? AND user_id = ? AND play_code = ? AND position = ? AND selection = ?",
		gameID, issue, userID, playCode, position, selection,
	).First(&existing).Error
	lineTotal := amountCents
	if findErr == nil {
		lineTotal = existing.AmountCents + amountCents
	} else if findErr != gorm.ErrRecordNotFound {
		return findErr
	}
	if limit.MaxBet > 0 && centsToAmount(lineTotal) > limit.MaxBet {
		return apperrors.NewBusinessError("BET_TOO_LARGE", fmt.Sprintf("单注最高 %.2f 元", limit.MaxBet))
	}
	var periodCents int64
	if err := s.db.Model(&bet.Bet{}).Where(
		"game_id = ? AND issue = ? AND user_id = ? AND play_code = ? AND status != ?",
		gameID, issue, userID, playCode, "cancelled",
	).Select("COALESCE(SUM(amount_cents),0)").Scan(&periodCents).Error; err != nil {
		return err
	}
	if limit.MaxUserPeriod > 0 && centsToAmount(periodCents+amountCents) > limit.MaxUserPeriod {
		return apperrors.NewBusinessError("PERIOD_LIMIT", fmt.Sprintf("本期该玩法限额 %.2f 元", limit.MaxUserPeriod))
	}
	return nil
}

func (s *BetAdminService) ensureIssueOpen(game *lottery.Game, issue string) error {
	current, err := s.CurrentIssue(game.ID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(issue) != current {
		return apperrors.NewBusinessError("ISSUE_MISMATCH", "期号已变更，请刷新页面后重试")
	}
	now := time.Now().UTC()
	if !game.NextDrawAt.After(now.Add(3 * time.Second)) {
		return apperrors.NewBusinessError("ISSUE_CLOSED", "当前期已封盘，请等待下一期")
	}
	return nil
}

func startOfDayCST(now time.Time) time.Time {
	loc := time.FixedZone("CST", 8*3600)
	t := now.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).UTC()
}
