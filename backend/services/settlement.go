package services

import (
	"backend/data/models/bet"
	"backend/data/models/chat"
	"backend/data/models/lottery"
	membernotify "backend/data/models/notify"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"backend/ws"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
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

type robotBetIdentity struct {
	workspaceID uint64
	userID      uint64
}

func isRobotSettlementRecipient(robotBets map[robotBetIdentity]struct{}, recipient settlementRecipient) bool {
	_, exists := robotBets[robotBetIdentity{workspaceID: recipient.WorkspaceID, userID: recipient.UserID}]
	return exists
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

// settleIssueOnce performs one idempotent settlement attempt. SettleIssue in
// settlement_recovery.go adds bounded retries for PostgreSQL deadlocks and
// serialization failures around this operation.
func (s *BetAdminService) settleIssueOnce(gameID, issue, operator string) (*SettlementResult, error) {
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
	mode := "platform"
	if game.SourceKind == "external" || game.SourceKind == "official" {
		mode = "external"
	}
	interval := time.Duration(maxInt(game.DrawInterval, 60)) * time.Second
	issueRow := lottery.Issue{
		GameID: game.ID, Issue: issue, Status: lottery.IssueStatusSettling, SourceMode: mode,
		AcceptAt: draw.DrawAt.UTC().Add(-interval), SealAt: draw.DrawAt.UTC().Add(-3 * time.Second),
	}
	if err := s.db.Where("game_id = ? AND issue = ?", game.ID, issue).FirstOrCreate(&issueRow).Error; err != nil {
		return nil, apperrors.NewSystemError("ISSUE_SAVE_FAILED", "保存期号状态失败", err)
	}
	drawAt := draw.DrawAt.UTC()
	alreadySettled := issueRow.Status == lottery.IssueStatusSettled
	if !alreadySettled {
		if err := s.setIssueStatus(game.ID, issue, lottery.IssueStatusSettling, "", &drawAt, nil); err != nil {
			return nil, apperrors.NewSystemError("ISSUE_SAVE_FAILED", "更新期号结算状态失败", err)
		}
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
		// Re-reading an already settled draw is a no-op. A newly imported draw is
		// still announced once even when nobody placed a bet.
		if !alreadySettled {
			settledAt := time.Now().UTC()
			if err := s.setIssueStatus(game.ID, issue, lottery.IssueStatusSettled, "", &drawAt, &settledAt); err != nil {
				return nil, apperrors.NewSystemError("ISSUE_SAVE_FAILED", "完成期号结算状态失败", err)
			}
			ws.NotifyDraw(gameID, issue, numbers)
		}
		return result, nil
	}

	operator = defaultString(strings.TrimSpace(operator), "系统结算")
	summaries := map[settlementRecipient]*settleUserSummary{}
	deliveries := make([]settlementNotificationDelivery, 0)
	roomMessages := make([]chat.Message, 0)
	err = s.db.Transaction(func(tx *gorm.DB) error {
		pendingUserIDs := make([]uint64, 0, len(pending))
		for _, item := range pending {
			pendingUserIDs = append(pendingUserIDs, item.UserID)
		}
		var robotProfiles []workspacemodel.RobotProfile
		if err := tx.Select("workspace_id", "user_id").Where("user_id IN ?", pendingUserIDs).Find(&robotProfiles).Error; err != nil {
			return err
		}
		robotBets := make(map[robotBetIdentity]struct{}, len(robotProfiles))
		for _, profile := range robotProfiles {
			robotBets[robotBetIdentity{workspaceID: profile.WorkspaceID, userID: profile.UserID}] = struct{}{}
		}
		for _, item := range pending {
			won, reason := evaluateBet(numbers, item.PlayCode, item.Position, item.Selection)
			payout := int64(0)
			status := "lost"
			if won {
				status = "won"
				payout = int64(math.Round(float64(item.AmountCents) * item.Odds))
			}
			settledAt := time.Now().UTC()
			_, isRobotBet := robotBets[robotBetIdentity{workspaceID: item.WorkspaceID, userID: item.UserID}]
			rebateCents, agentShareCents := settledBetFinancialAmounts(item, payout, isRobotBet)
			updates := map[string]any{
				"status":                status,
				"payout_cents":          payout,
				"rebate_cents":          rebateCents,
				"agent_share_cents":     agentShareCents,
				"settled_at":            settledAt,
				"remark":                trimRemark(item.Remark, reason),
				"operator":              operator,
				"updated_at":            settledAt,
				"reconciliation_status": "normal",
				"reconciliation_note":   "",
			}
			if isRobotBet {
				updates["fly_cents"] = 0
				updates["rebate_rate_snapshot"] = 0
				updates["agent_share_rate_snapshot"] = 0
			}
			updated := tx.Model(&bet.Bet{}).Where("id = ? AND status = ?", item.ID, "pending").Updates(updates)
			if updated.Error != nil {
				return updated.Error
			}
			// Another settlement worker may have completed this row after the
			// initial snapshot.  Never credit or notify unless this transaction
			// actually won the pending -> settled transition.
			if updated.RowsAffected == 0 {
				result.Skipped++
				continue
			}
			recipient := settlementRecipient{
				WorkspaceID: item.WorkspaceID,
				UserID:      item.UserID,
				RoomScope:   defaultString(strings.TrimSpace(item.RoomScope), "legacy"),
			}
			summary := summaries[recipient]
			if summary == nil {
				summary = &settleUserSummary{roomScope: recipient.RoomScope}
				summaries[recipient] = summary
			}
			summary.stakeCents += item.AmountCents
			summary.details = append(summary.details, NotificationBetDetail{
				PlayCode: item.PlayCode, PlayName: settlementPositionLabel(item.Position, item.PlayCode, item.PlayName), Position: item.Position, Selection: item.Selection,
				Amount: centsToAmount(item.AmountCents), Odds: item.Odds, Result: status,
				Payout: centsToAmount(payout),
			})
			if won {
				summary.wonCount++
				summary.payoutCents += payout
			} else {
				summary.lostCount++
			}
			result.StakeAmount += centsToAmount(item.AmountCents)
			if won {
				result.Won++
				result.PayoutAmount += centsToAmount(payout)
				if payout > 0 {
					if err := creditSettlement(tx, item.WorkspaceID, item.ID, item.UserID, payout, game.Name, issue, operator); err != nil {
						return err
					}
				}
			} else {
				result.Lost++
			}
		}
		var persistErr error
		deliveries, persistErr = persistSettlementResults(tx, game.ID, game.Name, issue, numbers, draw.DrawAt, summaries)
		if persistErr != nil {
			return persistErr
		}
		roomMessages, persistErr = persistRoomSettlementMessages(tx, draw.ID, game.ID, game.Name, issue, summaries, robotBets)
		if persistErr != nil {
			return persistErr
		}
		return tx.Model(&lottery.Issue{}).
			Where("game_id = ? AND issue = ?", game.ID, issue).
			Updates(map[string]any{
				"status":     lottery.IssueStatusSettled,
				"last_error": "",
				"draw_at":    drawAt,
				"settled_at": result.SettledAt,
			}).Error
	})
	if err != nil {
		_ = s.setIssueStatus(game.ID, issue, lottery.IssueStatusError, err.Error(), &drawAt, nil)
		if app, ok := err.(*apperrors.AppError); ok {
			return nil, app
		}
		return nil, apperrors.NewSystemError("SETTLEMENT_FAILED", "开奖结算失败", err)
	}
	deliverSettlementNotifications(deliveries)
	deliverRoomSettlementMessages(s.db, roomMessages)
	ws.NotifyDraw(gameID, issue, numbers)
	return result, nil
}

type roomSettlementPlayer struct {
	userID       uint64
	nickname     string
	balanceCents int64
	stakeCents   int64
	payoutCents  int64
	details      []NotificationBetDetail
}

type roomSettlementKey struct {
	workspaceID uint64
	roomScope   string
}

// persistRoomSettlementMessages writes two durable room events for every room
// that participated in the issue: the per-player settlement detail and the
// post-settlement score board. drawID + message type is the idempotency key.
func persistRoomSettlementMessages(db *gorm.DB, drawID uint64, gameID, gameName, issue string, summaries map[settlementRecipient]*settleUserSummary, robotBets map[robotBetIdentity]struct{}) ([]chat.Message, error) {
	byRoom := make(map[roomSettlementKey][]roomSettlementPlayer)
	for recipient, summary := range summaries {
		if summary == nil {
			continue
		}
		// Robot identity is the immutable workspace/profile pair loaded in this
		// settlement transaction. Operator edits to a user's mutable remark must
		// never make synthetic results visible in member settlement messages.
		if isRobotSettlementRecipient(robotBets, recipient) {
			continue
		}
		var account user.User
		if err := db.Select("user_id", "username", "nickname", "balance_cents").First(&account, recipient.UserID).Error; err != nil {
			return nil, err
		}
		roomScope := defaultString(strings.TrimSpace(summary.roomScope), recipient.RoomScope)
		key := roomSettlementKey{workspaceID: recipient.WorkspaceID, roomScope: roomScope}
		byRoom[key] = append(byRoom[key], roomSettlementPlayer{
			userID: recipient.UserID, nickname: defaultString(account.Nickname, account.Username), balanceCents: account.BalanceCents,
			stakeCents: summary.stakeCents, payoutCents: summary.payoutCents, details: summary.details,
		})
	}
	rooms := make([]roomSettlementKey, 0, len(byRoom))
	for key := range byRoom {
		rooms = append(rooms, key)
	}
	sort.Slice(rooms, func(i, j int) bool {
		if rooms[i].workspaceID != rooms[j].workspaceID {
			return rooms[i].workspaceID < rooms[j].workspaceID
		}
		return rooms[i].roomScope < rooms[j].roomScope
	})
	createdMessages := make([]chat.Message, 0, len(rooms)*2)
	for _, room := range rooms {
		players := byRoom[room]
		sort.SliceStable(players, func(i, j int) bool { return players[i].userID < players[j].userID })
		resultContent := formatRoomSettlement(gameName, issue, players)
		resultRow, created, err := createRoomSettlementMessage(db, drawID, room.workspaceID, room.roomScope, gameID, "settlement", resultContent)
		if err != nil {
			return nil, err
		}
		if created {
			createdMessages = append(createdMessages, resultRow)
		}

		scores, err := roomScorePlayers(db, room.workspaceID)
		if err != nil {
			return nil, err
		}
		scoreContent := formatRoomScores(gameName, issue, scores)
		scoreRow, created, err := createRoomSettlementMessage(db, drawID, room.workspaceID, room.roomScope, gameID, "scoreboard", scoreContent)
		if err != nil {
			return nil, err
		}
		if created {
			createdMessages = append(createdMessages, scoreRow)
		}
	}
	return createdMessages, nil
}

func createRoomSettlementMessage(db *gorm.DB, drawID, workspaceID uint64, roomScope, gameID, messageType, content string) (chat.Message, bool, error) {
	row := chat.Message{
		WorkspaceID: workspaceID, UserID: 0, Username: "draw_assistant", Nickname: "开奖助手", RoomType: "group",
		Scope: roomScope, RoomScope: roomScope, GameID: gameID, Content: content,
		MessageType: messageType, ReferenceID: drawID,
	}
	result := db.Where(
		"workspace_id = ? AND room_type = ? AND room_scope = ? AND game_id = ? AND message_type = ? AND reference_id = ?",
		workspaceID, "group", roomScope, gameID, messageType, drawID,
	).FirstOrCreate(&row)
	return row, result.RowsAffected > 0, result.Error
}

func formatRoomSettlement(gameName, issue string, players []roomSettlementPlayer) string {
	var body strings.Builder
	fmt.Fprintf(&body, "【%s - %s】\n结算内容如下：", gameName, issue)
	for _, player := range players {
		net := player.payoutCents - player.stakeCents
		fmt.Fprintf(&body, "\n\n[%s]\n得分：%+.2f", player.nickname, centsToAmount(net))
		for _, detail := range player.details {
			label := settlementPositionLabel(detail.Position, detail.PlayCode, detail.PlayName)
			lineNet := int64(math.Round((detail.Payout - detail.Amount) * 100))
			fmt.Fprintf(&body, "\n%s [%s/%.2f=%+.2f]", label, detail.Selection, detail.Amount, centsToAmount(lineNet))
		}
	}
	return body.String()
}

func settlementPositionLabel(position int, playCode, playName string) string {
	if playCode == "sum" {
		return "冠亚和"
	}
	names := []string{"冠军", "亚军", "第三名", "第四名", "第五名", "第六名", "第七名", "第八名", "第九名", "第十名"}
	if position >= 1 && position <= len(names) {
		return names[position-1]
	}
	if label := strings.TrimSpace(playName); label != "" && label != "指定名次号码" {
		return label
	}
	return fmt.Sprintf("第%d名", position)
}

func roomScorePlayers(db *gorm.DB, workspaceID uint64) ([]roomSettlementPlayer, error) {
	if workspaceID == 0 {
		return nil, fmt.Errorf("invalid workspace")
	}
	query := roomScorePlayersQuery(db, workspaceID)
	var accounts []user.User
	if err := query.Select("user_id", "username", "nickname", "balance_cents").Order("balance_cents DESC, user_id ASC").Limit(100).Find(&accounts).Error; err != nil {
		return nil, err
	}
	players := make([]roomSettlementPlayer, 0, len(accounts))
	for _, account := range accounts {
		players = append(players, roomSettlementPlayer{userID: account.UserID, nickname: defaultString(account.Nickname, account.Username), balanceCents: account.BalanceCents})
	}
	return players, nil
}

func roomScorePlayersQuery(db *gorm.DB, workspaceID uint64) *gorm.DB {
	return excludeRobotProfileUsers(db.Model(&user.User{})).
		Where("workspace_id = ? AND status = ? AND role NOT IN ?", workspaceID, 1, []string{"admin", "tenant", "agent"})
}

func formatRoomScores(gameName, issue string, players []roomSettlementPlayer) string {
	var body strings.Builder
	fmt.Fprintf(&body, "【%s - %s】\n玩家积分如下：", gameName, issue)
	for _, player := range players {
		fmt.Fprintf(&body, "\n[%s  积分：%.2f]", player.nickname, centsToAmount(player.balanceCents))
	}
	return body.String()
}

func deliverRoomSettlementMessages(db *gorm.DB, messages []chat.Message) {
	for _, message := range messages {
		if recipients, err := betScopeRecipients(db, message.RoomScope); err == nil {
			notifyChatEvent(db, recipients, message, "created")
		}
	}
}

type settleUserSummary struct {
	roomScope   string
	wonCount    int
	lostCount   int
	stakeCents  int64
	payoutCents int64
	details     []NotificationBetDetail
}

type settlementRecipient struct {
	WorkspaceID uint64
	UserID      uint64
	RoomScope   string
}

type NotificationBetDetail struct {
	PlayCode  string  `json:"play_code,omitempty"`
	PlayName  string  `json:"play_name"`
	Position  int     `json:"position,omitempty"`
	Selection string  `json:"selection"`
	Amount    float64 `json:"amount"`
	Odds      float64 `json:"odds"`
	Result    string  `json:"result"`
	Payout    float64 `json:"payout"`
}

type settlementNotificationDelivery struct {
	userID  uint64
	payload map[string]any
}

func persistSettlementResults(db *gorm.DB, gameID, gameName, issue string, numbers []int, drawAt time.Time, summaries map[settlementRecipient]*settleUserSummary) ([]settlementNotificationDelivery, error) {
	numText := formatDrawNumbers(numbers)
	deliveries := make([]settlementNotificationDelivery, 0, len(summaries))
	for recipient, summary := range summaries {
		if summary == nil || (summary.wonCount == 0 && summary.lostCount == 0) {
			continue
		}
		userID := recipient.UserID
		roomScope := defaultString(strings.TrimSpace(summary.roomScope), recipient.RoomScope)
		title := "开奖通知"
		level := "info"
		betCount := summary.wonCount + summary.lostCount
		content := fmt.Sprintf("%s 第 %s 期开奖，开奖号码 %s。共投注 %d 注，投注金额 %.2f 元", gameName, issue, numText, betCount, centsToAmount(summary.stakeCents))
		if summary.wonCount > 0 {
			level = "success"
			content += fmt.Sprintf("；中奖 %d 注，中奖金额 %.2f 元。", summary.wonCount, centsToAmount(summary.payoutCents))
		} else {
			content += "；本期未中奖。"
		}
		detailsJSON, _ := json.Marshal(summary.details)
		eventKey := settlementEventKey(gameID, issue, userID, roomScope)
		notice := membernotify.MemberNotification{
			WorkspaceID: recipient.WorkspaceID, UserID: userID, Title: title, Content: content,
			GameID: gameID, RoomScope: roomScope, EventKey: eventKey,
			Level: level, Category: "winning", GameName: gameName, Issue: issue,
			DrawNumbers: joinNumbers(numbers), DrawAt: &drawAt,
			BetCount: betCount, WonCount: summary.wonCount,
			StakeCents: summary.stakeCents, PayoutCents: summary.payoutCents,
			BetDetailsJSON: string(detailsJSON),
		}
		created := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&notice)
		if created.Error != nil {
			return nil, created.Error
		}
		if created.RowsAffected == 0 {
			continue
		}
		deliveries = append(deliveries, settlementNotificationDelivery{userID: userID, payload: map[string]any{
			"id": notice.ID, "workspace_id": recipient.WorkspaceID, "game_id": gameID, "room_scope": roomScope,
			"title": title, "content": content, "level": level, "category": "winning",
			"game_name": gameName, "issue": issue, "draw_numbers": numbers,
			"draw_at": drawAt, "bet_count": betCount, "won_count": summary.wonCount,
			"stake_amount": centsToAmount(summary.stakeCents), "payout_amount": centsToAmount(summary.payoutCents),
			"bet_details": summary.details, "created_at": notice.CreatedAt,
		}})
	}
	return deliveries, nil
}

func deliverSettlementNotifications(deliveries []settlementNotificationDelivery) {
	for _, delivery := range deliveries {
		ws.NotifyUser(delivery.userID, "notification", delivery.payload)
	}
}

func settlementEventKey(gameID, issue string, userID uint64, roomScope string) string {
	return fmt.Sprintf("settlement:%s:%s:%d:%s", gameID, issue, userID, roomScope)
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
	// Persist the accepting/sealed lifecycle before the immutable draw advances
	// CurrentIssue to the following period.
	_, _ = s.EnsureCurrentIssue(game)
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
	// A provider may return an already stored draw after an earlier settlement
	// attempt failed. Re-run only when pending bets or an unfinished lifecycle
	// remain; a fully settled period is a strict no-op.
	var pending int64
	if err := s.db.Model(&bet.Bet{}).Where("game_id = ? AND issue = ? AND status = ?", gameID, issue, "pending").Count(&pending).Error; err != nil {
		return
	}
	var lifecycle lottery.Issue
	issueErr := s.db.Where("game_id = ? AND issue = ?", gameID, issue).First(&lifecycle).Error
	if pending == 0 && issueErr == nil && lifecycle.Status == lottery.IssueStatusSettled {
		return
	}
	_, _ = s.SettleIssue(gameID, issue, "官方开奖自动结算")
}

func creditSettlement(tx *gorm.DB, workspaceID, betID, userID uint64, payoutCents int64, gameName, issue, operator string) error {
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
		WorkspaceID: workspaceID, UserID: userID, Reference: fmt.Sprintf("settlement_bet:%d", betID),
		AmountCents: payoutCents, BeforeCents: before, AfterCents: after,
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
		// PK10 room syntax uses 0 as number 10. Keep zero unchanged for
		// five-ball games where it is an actual draw number.
		if len(balls) >= 10 && digit == 0 {
			digit = 10
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
		if position < 1 || position > len(balls) {
			if matchSumSize(value, len(balls), selection) {
				return true, label + "命中" + selectionLabel(selection)
			}
			return false, fmt.Sprintf("%s为%d", label, value)
		}
		if matchPositionSide(value, balls, selection) {
			return true, label + "命中" + selectionLabel(selection)
		}
		return false, label + "为" + describeSide(value)

	case "dragon_tiger":
		if len(balls) < 2 {
			return false, "号码不足"
		}
		// 赛车龙虎按对应名次比较：冠军对第十、亚军对第九，依次到
		// 第五对第六，不能把所有龙虎都误算成冠军对末位。
		if position < 1 || position > len(balls)/2 {
			return false, "龙虎名次无效"
		}
		left, right := balls[position-1], balls[len(balls)-position]
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
		// 赛车“冠亚和”只取冠军与亚军，不是十个号码总和。
		total := sumInts(balls)
		if len(balls) >= 2 {
			total = balls[0] + balls[1]
		}
		if selectedTotal, err := strconv.Atoi(selection); err == nil {
			// Ten-position racing games bet the exact 冠亚和值 (3-19).
			// Other legacy number games retain their historic sum-tail rule.
			if len(balls) >= 10 {
				if total == selectedTotal {
					return true, fmt.Sprintf("冠亚和 %d", total)
				}
				return false, fmt.Sprintf("冠亚和 %d", total)
			}
			if total%10 == selectedTotal {
				return true, fmt.Sprintf("总和尾 %d", selectedTotal)
			}
			return false, fmt.Sprintf("总和尾 %d", total%10)
		}
		if matchSumSize(total, len(balls), selection) {
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

func matchPositionSide(value int, balls []int, selection string) bool {
	racing := len(balls) >= 10
	if racing {
		switch selection {
		case "big", "大":
			return value >= 6
		case "small", "小":
			return value <= 5
		case "odd", "单":
			return value%2 == 1
		case "even", "双":
			return value%2 == 0
		}
	}
	return matchSide(value, selection)
}

func matchSumSize(total, ballCount int, selection string) bool {
	// PK10 冠亚和 is 3-19: 3-11 small and 12-19 big. Other games retain
	// their established midpoint rules.
	threshold := 23
	if ballCount >= 10 {
		threshold = 12
	} else if ballCount <= 3 {
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

// agentProfitShareCents applies an agent contract only to actual positive
// room GGR. When players win more than they staked, the platform absorbs that
// loss; it must never turn into a negative commission credit in the report.
func agentProfitShareCents(grossProfitCents int64, rate float64) int64 {
	if grossProfitCents <= 0 {
		return 0
	}
	return int64(math.Round(float64(grossProfitCents) * clampPercent(rate) / 100))
}

func settledBetFinancialAmounts(item bet.Bet, payoutCents int64, isRobot bool) (rebateCents, agentShareCents int64) {
	if isRobot {
		return 0, 0
	}
	rebateCents = int64(math.Round(float64(item.AmountCents) * clampPercent(item.RebateRateSnapshot) / 100))
	agentShareCents = agentProfitShareCents(item.AmountCents-payoutCents, item.AgentShareRateSnapshot)
	return rebateCents, agentShareCents
}
