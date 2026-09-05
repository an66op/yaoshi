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
	cryptorand "crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
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
	Push          int64     `json:"push"`
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
	Push         int64      `json:"push"`
	StakeAmount  float64    `json:"stake_amount"`
	PayoutAmount float64    `json:"payout_amount"`
	Settled      bool       `json:"settled"`
}

// settleIssueOnce performs one idempotent settlement attempt. SettleIssue in
// settlement_recovery.go adds bounded retries for PostgreSQL deadlocks and
// serialization failures around this operation.
func (s *BetAdminService) settleIssueOnce(gameID, issue, operator string, gate func(*gorm.DB) error) (*SettlementResult, error) {
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
	if revisionErr := orderedBingoSettlementRevisionError(game.ID, draw); revisionErr != nil {
		if err := s.markUnverifiedOrderedBingoIssue(game, draw, revisionErr.Error()); err != nil {
			return nil, apperrors.NewSystemError("ISSUE_SAVE_FAILED", "记录未验证开奖对账状态失败", err)
		}
		return nil, revisionErr
	}
	if game.ID == "sg-ssc" {
		var existing lottery.Issue
		if err := s.db.Where("game_id = ? AND issue = ?", game.ID, issue).Limit(1).Find(&existing).Error; err != nil {
			return nil, err
		}
		if err := sgSSCIssueEvidenceError(s.db, issue, &existing); err != nil {
			if apperrors.GetErrorCode(err) == "DRAW_SOURCE_UNVERIFIED" {
				if markErr := s.markUnverifiedOrderedBingoIssue(game, draw, err.Error()); markErr != nil {
					return nil, markErr
				}
			}
			return nil, err
		}
	}
	numbers := parseNumbers(draw.Numbers)
	if len(numbers) == 0 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "开奖号码无效")
	}
	drawAt := draw.DrawAt.UTC()
	var issueRow lottery.Issue
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if gate != nil {
			if err := gate(tx); err != nil {
				return err
			}
			// Re-read under the gate's Game lock; a snapshot from before a
			// source change must not create a platform history lifecycle.
			if err := tx.First(game, "id = ?", game.ID).Error; err != nil {
				return err
			}
		}
		mode := "platform"
		if game.SourceKind == "external" || game.SourceKind == "official" {
			mode = "external"
		}
		interval := time.Duration(maxInt(game.DrawInterval, 60)) * time.Second
		issueRow = lottery.Issue{GameID: game.ID, Issue: issue, Status: lottery.IssueStatusSettling, SourceMode: mode,
			AcceptAt: drawAt.Add(-interval), SealAt: drawAt.Add(-3 * time.Second)}
		if err := tx.Where("game_id = ? AND issue = ?", game.ID, issue).FirstOrCreate(&issueRow).Error; err != nil {
			return err
		}
		if issueRow.Status == lottery.IssueStatusSettled {
			return nil
		}
		return NewBetAdminService(tx).setIssueStatus(game.ID, issue, lottery.IssueStatusSettling, "", &drawAt, nil)
	}); err != nil {
		return nil, apperrors.NewSystemError("ISSUE_SAVE_FAILED", "保存期号结算状态失败", err)
	}
	alreadySettled := issueRow.Status == lottery.IssueStatusSettled

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
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			if gate != nil {
				if err := gate(tx); err != nil {
					return err
				}
			}
			if alreadySettled {
				return nil
			}
			settledAt := time.Now().UTC()
			return NewBetAdminService(tx).setIssueStatus(game.ID, issue, lottery.IssueStatusSettled, "", &drawAt, &settledAt)
		}); err != nil {
			return nil, apperrors.NewSystemError("ISSUE_SAVE_FAILED", "完成期号结算状态失败", err)
		}
		if !alreadySettled {
			ws.NotifyDraw(gameID, issue, numbers)
		}
		return result, nil
	}

	operator = defaultString(strings.TrimSpace(operator), "系统结算")
	summaries := map[settlementRecipient]*settleUserSummary{}
	deliveries := make([]settlementNotificationDelivery, 0)
	roomMessages := make([]chat.Message, 0)
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if gate != nil {
			if err := gate(tx); err != nil {
				return err
			}
		}
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
		type pc28StakeKey struct {
			workspaceID uint64
			roomScope   string
			userID      uint64
		}
		pc28UserIssueStake := make(map[pc28StakeKey]int64)
		gameProfile, _ := rulesForGame(game)
		if gameProfile.PC28 > 0 {
			for _, item := range pending {
				key := pc28StakeKey{workspaceID: item.WorkspaceID, roomScope: defaultString(strings.TrimSpace(item.RoomScope), "legacy"), userID: item.UserID}
				total, ok := safeAddInt64(pc28UserIssueStake[key], item.AmountCents)
				if !ok || total < item.AmountCents {
					return apperrors.NewBusinessError("INVALID_REQUEST", "PC28会员本期总注超出系统范围")
				}
				pc28UserIssueStake[key] = total
			}
		}
		for _, item := range pending {
			if revisionErr := betDrawRevisionError(game.ID, issue, item.DrawSourceRevision, draw.SourceRevision); revisionErr != nil {
				return apperrors.NewBusinessError("DRAW_SOURCE_UNVERIFIED", revisionErr.Error())
			}
			outcome, reason := markSixOutcomeLost, ""
			effectiveOdds := item.Odds
			validTurnoverCents := item.AmountCents
			settlementPolicy := "standard"
			userIssueStakeCents := int64(0)
			var evaluationErr error
			if gameProfile.PC28 > 0 {
				key := pc28StakeKey{workspaceID: item.WorkspaceID, roomScope: defaultString(strings.TrimSpace(item.RoomScope), "legacy"), userID: item.UserID}
				decision, decisionErr := decidePC28Settlement(game.ID, item, numbers, pc28UserIssueStake[key])
				if decisionErr != nil {
					evaluationErr = decisionErr
				} else {
					outcome, reason, effectiveOdds = decision.Outcome, decision.Reason, decision.EffectiveOdds
					validTurnoverCents, userIssueStakeCents, settlementPolicy = decision.ValidTurnoverCents, decision.UserIssueStakeCents, decision.Policy
				}
			} else if gameProfile.MarkSix {
				decision, decisionErr := decideMarkSixSettlement(game.ID, item, numbers, draw.DrawAt)
				if decisionErr != nil {
					evaluationErr = decisionErr
				} else {
					outcome, reason, effectiveOdds = decision.Outcome, decision.Reason, decision.EffectiveOdds
					validTurnoverCents, settlementPolicy = decision.ValidTurnoverCents, decision.Policy
				}
			} else {
				outcome, reason, evaluationErr = evaluateBetOutcomeForRuleVersionAt(game, item.RuleVersion, numbers, item.PlayCode, item.Position, item.Selection, draw.DrawAt)
				if outcome == markSixOutcomePush {
					effectiveOdds, validTurnoverCents, settlementPolicy = 1, 0, "settlement_push"
				}
			}
			if evaluationErr != nil {
				// The transaction must roll back every ticket in this issue. An
				// unknown rule or malformed draw is not a losing result.
				return evaluationErr
			}
			payout := int64(0)
			status := string(outcome)
			storedStatus := status
			if outcome == markSixOutcomeWon {
				payout = int64(math.Round(float64(item.AmountCents) * effectiveOdds))
			} else if outcome == markSixOutcomePush {
				payout = item.AmountCents
				// The persisted status check predates explicit push outcomes. Keep
				// the existing cancelled financial state while the immutable reason
				// and notification detail retain the more precise "push" result.
				storedStatus = "cancelled"
			}
			settledAt := time.Now().UTC()
			item.ValidTurnoverCents = int64Pointer(validTurnoverCents)
			item.SettlementOdds = &effectiveOdds
			item.SettlementPolicy = settlementPolicy
			if gameProfile.PC28 > 0 {
				item.UserIssueStakeCentsSnapshot = int64Pointer(userIssueStakeCents)
			}
			_, isRobotBet := robotBets[robotBetIdentity{workspaceID: item.WorkspaceID, userID: item.UserID}]
			rebateCents, agentShareCents := int64(0), int64(0)
			if outcome != markSixOutcomePush {
				rebateCents, agentShareCents = settledBetFinancialAmounts(item, payout, isRobotBet)
			}
			updates := map[string]any{
				"status":                storedStatus,
				"payout_cents":          payout,
				"rebate_cents":          rebateCents,
				"agent_share_cents":     agentShareCents,
				"valid_turnover_cents":  validTurnoverCents,
				"settlement_odds":       effectiveOdds,
				"settlement_policy":     settlementPolicy,
				"settled_at":            settledAt,
				"remark":                trimRemark(item.Remark, reason),
				"operator":              operator,
				"updated_at":            settledAt,
				"reconciliation_status": "normal",
				"reconciliation_note":   "",
			}
			if gameProfile.PC28 > 0 {
				updates["user_issue_stake_cents_snapshot"] = userIssueStakeCents
			}
			if outcome == markSixOutcomePush {
				// PostgreSQL's historical bet-status constraint has no dedicated
				// push value. Keep the financial state recoverable as cancelled and
				// attach an immutable machine marker so status aggregation can tell
				// a settled push apart from an operator cancellation.
				updates["reconciliation_note"] = "settlement_push"
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
				PlayCode: item.PlayCode, PlayName: settlementBetLabel(item), Position: item.Position, Selection: item.Selection,
				Amount: centsToAmount(item.AmountCents), Odds: effectiveOdds, QuotedOdds: item.Odds,
				EffectiveOdds: effectiveOdds, SettlementPolicy: settlementPolicy, Result: status,
				Payout: centsToAmount(payout),
			})
			switch outcome {
			case markSixOutcomeWon:
				summary.wonCount++
				summary.payoutCents += payout
				summary.wonPayoutCents += payout
			case markSixOutcomePush:
				summary.pushCount++
				summary.payoutCents += payout
				summary.refundCents += payout
			default:
				summary.lostCount++
			}
			result.StakeAmount += centsToAmount(item.AmountCents)
			switch outcome {
			case markSixOutcomeWon:
				result.Won++
				result.PayoutAmount += centsToAmount(payout)
			case markSixOutcomePush:
				result.Push++
				result.PayoutAmount += centsToAmount(payout)
			default:
				result.Lost++
			}
			if payout > 0 {
				if err := creditSettlement(tx, item.WorkspaceID, item.ID, item.UserID, payout, game.Name, issue, operator); err != nil {
					return err
				}
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
		if gate == nil {
			_ = s.setIssueStatus(game.ID, issue, lottery.IssueStatusError, err.Error(), &drawAt, nil)
		} else {
			// A paused or replaced recovery worker must not overwrite a newer
			// worker's completed lifecycle while reporting its own failure.
			_ = s.db.Transaction(func(tx *gorm.DB) error {
				if gateErr := gate(tx); gateErr != nil {
					return gateErr
				}
				return tx.Model(&lottery.Issue{}).Where("game_id = ? AND issue = ? AND status <> ?", game.ID, issue, lottery.IssueStatusSettled).
					Updates(map[string]any{"status": lottery.IssueStatusError, "last_error": limitDBText(err.Error(), 500), "draw_at": drawAt}).Error
			})
		}
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

func orderedBingoSettlementRevisionError(gameID string, draw lottery.Draw) error {
	if orderedBingoDrawRevisionCurrent(gameID, draw.SourceRevision, draw.ConversionRevision) {
		return nil
	}
	return apperrors.NewBusinessError("DRAW_SOURCE_UNVERIFIED",
		fmt.Sprintf("第 %s 期不是受信任的开奖来源版本，已保留注单并转人工对账", draw.Issue))
}

func orderedBingoDrawRevisionCurrent(gameID, sourceRevision, conversionRevision string) bool {
	return trustedDrawRevisionMatches(gameID, sourceRevision, conversionRevision)
}

func (s *BetAdminService) markUnverifiedOrderedBingoIssue(game *lottery.Game, draw lottery.Draw, reason string) error {
	if game == nil {
		return fmt.Errorf("游戏配置不存在")
	}
	reason = reconciliationIssueError(reason)
	return s.db.Transaction(func(tx *gorm.DB) error {
		interval := time.Duration(maxInt(game.DrawInterval, 60)) * time.Second
		row := lottery.Issue{
			GameID: game.ID, Issue: draw.Issue, Status: lottery.IssueStatusError, SourceMode: sourceMode(game.SourceKind),
			AcceptAt: draw.DrawAt.UTC().Add(-interval), SealAt: draw.DrawAt.UTC().Add(-3 * time.Second),
			DrawAt: &draw.DrawAt, LastError: reason,
		}
		if game.ID == "sg-ssc" {
			row.SourceMode = "legacy"
		}
		if err := tx.Where("game_id = ? AND issue = ?", game.ID, draw.Issue).FirstOrCreate(&row).Error; err != nil {
			return err
		}
		if row.Status != lottery.IssueStatusSettled {
			if err := tx.Model(&lottery.Issue{}).Where("id = ?", row.ID).Updates(map[string]any{
				"status": lottery.IssueStatusError, "last_error": reason, "draw_at": draw.DrawAt.UTC(),
			}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&bet.Bet{}).
			Where("game_id = ? AND issue = ? AND status = ?", game.ID, draw.Issue, "pending").
			Updates(map[string]any{"reconciliation_status": "abnormal", "reconciliation_note": reason}).Error
	})
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
	result := db.Unscoped().Where(
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
			// The financial pass already froze this label from the bet's rule
			// version. Reclassifying it here would turn digit sums into racing
			// 冠亚和 and overwrite numbered-ball/front-three labels.
			label := strings.TrimSpace(detail.PlayName)
			if label == "" {
				label = settlementPositionLabel(detail.Position, detail.PlayCode, "")
			}
			lineNet := int64(math.Round((detail.Payout - detail.Amount) * 100))
			fmt.Fprintf(&body, "\n%s [%s/%.2f=%+.2f]", label, detail.Selection, detail.Amount, centsToAmount(lineNet))
		}
	}
	return body.String()
}

func settlementBetLabel(item bet.Bet) string {
	profile, versioned := rulesForVersion(item.RuleVersion)
	if !versioned || !gameSupportsRuleVersion(item.GameID, item.RuleVersion) {
		return defaultString(strings.TrimSpace(item.PlayName), "未识别玩法")
	}
	if profile.Racing {
		return settlementPositionLabel(item.Position, item.PlayCode, item.PlayName)
	}
	if profile.MarkSix {
		if spec, ok := markSixSpecByCode(item.RuleVersion, item.PlayCode); ok {
			if spec.PositionMode == "regular" && item.Position >= 1 && item.Position <= 6 {
				return fmt.Sprintf("%s 第%d位", spec.Play.Name, item.Position)
			}
			return spec.Play.Name
		}
		return defaultString(strings.TrimSpace(item.PlayName), "六合彩")
	}
	if profile.PC28 > 0 {
		if spec, ok := pc28SpecByCode(item.PlayCode); ok {
			if item.Position >= 1 && item.Position <= 3 {
				return fmt.Sprintf("%s 第%d球", spec.Name, item.Position)
			}
			return spec.Name
		}
		return defaultString(strings.TrimSpace(item.PlayName), "PC28")
	}
	code := strings.ToLower(strings.TrimSpace(item.PlayCode))
	switch code {
	case "sum":
		if isSideSelection(item.Selection) {
			return "总和"
		}
		return "总和尾"
	case "leopard", "straight", "pair", "half_straight", "mixed":
		return digitShapeScopeName(item.Position) + playNameOf(code)
	case "dragon_tiger":
		if profile.Version == "digits5-v3" {
			return "第1球龙虎"
		}
		return fmt.Sprintf("第%d球", item.Position)
	case "dragon_tiger_tie":
		return "第1球龙虎和"
	default:
		return fmt.Sprintf("第%d球", item.Position)
	}
}

// BetDisplayLabel exposes the same immutable rule-version label used by
// settlement notifications to read-only bet views such as the room “查”
// command. Display code must not infer a racing rank from a v3 digit shape.
func BetDisplayLabel(item BetView) string {
	return settlementBetLabel(bet.Bet{
		GameID:      item.GameID,
		RuleVersion: item.RuleVersion,
		PlayCode:    item.PlayCode,
		PlayName:    item.PlayName,
		Position:    item.Position,
		Selection:   item.Selection,
	})
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
	pushCount   int
	stakeCents  int64
	payoutCents int64
	// payoutCents is the actual balance credit used by room net calculations;
	// these two components keep member copy from calling a push refund a win.
	wonPayoutCents int64
	refundCents    int64
	details        []NotificationBetDetail
}

type settlementRecipient struct {
	WorkspaceID uint64
	UserID      uint64
	RoomScope   string
}

type NotificationBetDetail struct {
	PlayCode         string  `json:"play_code,omitempty"`
	PlayName         string  `json:"play_name"`
	Position         int     `json:"position,omitempty"`
	Selection        string  `json:"selection"`
	Amount           float64 `json:"amount"`
	Odds             float64 `json:"odds"`
	QuotedOdds       float64 `json:"quoted_odds,omitempty"`
	EffectiveOdds    float64 `json:"effective_odds,omitempty"`
	SettlementPolicy string  `json:"settlement_policy,omitempty"`
	Result           string  `json:"result"`
	Payout           float64 `json:"payout"`
}

type settlementNotificationDelivery struct {
	userID  uint64
	payload map[string]any
}

func persistSettlementResults(db *gorm.DB, gameID, gameName, issue string, numbers []int, drawAt time.Time, summaries map[settlementRecipient]*settleUserSummary) ([]settlementNotificationDelivery, error) {
	numText := formatDrawNumbers(numbers)
	deliveries := make([]settlementNotificationDelivery, 0, len(summaries))
	for recipient, summary := range summaries {
		if summary == nil || (summary.wonCount == 0 && summary.lostCount == 0 && summary.pushCount == 0) {
			continue
		}
		userID := recipient.UserID
		roomScope := defaultString(strings.TrimSpace(summary.roomScope), recipient.RoomScope)
		title := "开奖通知"
		level := "info"
		betCount := summary.wonCount + summary.lostCount + summary.pushCount
		content := fmt.Sprintf("%s 第 %s 期开奖，开奖号码 %s。共投注 %d 注，投注金额 %.2f 元", gameName, issue, numText, betCount, centsToAmount(summary.stakeCents))
		if summary.wonCount > 0 {
			level = "success"
			content += fmt.Sprintf("；中奖 %d 注，中奖金额 %.2f 元。", summary.wonCount, centsToAmount(summary.wonPayoutCents))
		} else {
			content += "；本期未中奖。"
		}
		if summary.pushCount > 0 {
			content += fmt.Sprintf("；和局返本 %d 注，返还 %.2f 元。", summary.pushCount, centsToAmount(summary.refundCents))
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
			"refund_amount": centsToAmount(summary.refundCents),
			"bet_details":   summary.details, "created_at": notice.CreatedAt,
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
	return s.publishDrawWithEntropy(gameID, issue, numbers, operator, cryptorand.Reader)
}

// Keep entropy injection local to an invocation: regression tests can verify
// that a failed generator makes no writes without replacing a global reader.
func (s *BetAdminService) publishDrawWithEntropy(gameID, issue string, numbers []int, operator string, entropy io.Reader) (*SettlementResult, error) {
	game, err := s.loadGame(gameID)
	if err != nil {
		return nil, err
	}
	if _, _, versionedSource := trustedDrawRevision(game.ID); versionedSource {
		return nil, apperrors.NewBusinessError("EXTERNAL_DRAW_MANUAL_FORBIDDEN", "该彩种只能写入已核验并带来源版本的开奖结果，请使用来源同步或人工对账流程")
	}
	issue = strings.TrimSpace(issue)
	if len(numbers) == 0 {
		numbers, err = generateDrawNumbers(game, entropy)
		if err != nil {
			return nil, err
		}
	}
	if profile, supported := rulesForGame(game); supported {
		if err := profile.validateDraw(numbers); err != nil {
			return nil, apperrors.NewBusinessError("INVALID_DRAW", "开奖号码不符合彩种规则："+err.Error())
		}
	} else if len(numbers) < 3 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "开奖号码至少需要 3 个")
	}
	// Persist the accepting/sealed lifecycle before the immutable draw advances
	// CurrentIssue to the following period.
	_, _ = s.EnsureCurrentIssue(game)
	if issue == "" {
		issue, err = s.CurrentIssue(game.ID)
		if err != nil {
			return nil, err
		}
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
		"next_draw_at":  now.Add(time.Duration(effectiveDrawInterval(game)) * time.Second),
		"next_issue":    nextIssue(issue),
		"timing_source": "configured",
		"sync_status":   "ok",
		"last_sync_at":  now,
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
	if err := trustedDrawsForGame(s.db, game.ID).Where("issue = ?", issue).First(&draw).Error; err == nil {
		status.HasDraw = true
		status.Numbers = parseNumbers(draw.Numbers)
		status.DrawAt = &draw.DrawAt
	} else if err != gorm.ErrRecordNotFound {
		return nil, apperrors.NewSystemError("DRAW_READ_FAILED", "读取开奖结果失败", err)
	}
	type agg struct {
		Status             string
		ReconciliationNote string
		Cnt                int64
		StakeCents         int64
		PayoutCents        int64
	}
	var rows []agg
	if err := s.db.Model(&bet.Bet{}).
		Select("status, COALESCE(reconciliation_note,'') as reconciliation_note, COUNT(*) as cnt, COALESCE(SUM(amount_cents),0) as stake_cents, COALESCE(SUM(payout_cents),0) as payout_cents").
		Where("game_id = ? AND issue = ?", game.ID, issue).
		Group("status, reconciliation_note").Scan(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("BET_READ_FAILED", "读取结算状态失败", err)
	}
	for _, row := range rows {
		status.StakeAmount += centsToAmount(row.StakeCents)
		status.PayoutAmount += centsToAmount(row.PayoutCents)
		switch row.Status {
		case "pending":
			status.Pending += row.Cnt
		case "won":
			status.Won += row.Cnt
		case "lost":
			status.Lost += row.Cnt
		case "push":
			// Kept for compatibility with archival stores that predate the
			// PostgreSQL status constraint used by the current write path.
			status.Push += row.Cnt
		case "cancelled":
			if row.ReconciliationNote == "settlement_push" {
				status.Push += row.Cnt
			}
		}
	}
	status.Settled = status.HasDraw && status.Pending == 0 && (status.Won+status.Lost+status.Push) > 0
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
	if pending == 0 {
		var lifecycle lottery.Issue
		result := s.db.Select("status").Where("game_id = ? AND issue = ?", gameID, issue).Limit(1).Find(&lifecycle)
		if result.Error != nil || result.RowsAffected == 0 || lifecycle.Status == lottery.IssueStatusSettled {
			return
		}
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

// evaluateBetOutcomeForRuleVersionAt is the financial settlement entry point.
// Mark Six zodiac rules receive the immutable official draw timestamp; using
// the server clock would change a historical ticket around lunar new year.
func evaluateBetOutcomeForRuleVersionAt(game *lottery.Game, version string, numbers []int, playCode string, position int, selection string, drawAt time.Time) (markSixBetOutcome, string, error) {
	gameProfile, supported := rulesForGame(game)
	if supported && gameProfile.MarkSix {
		profile, err := settlementRuleProfile(game, version)
		if err != nil {
			return "", "", err
		}
		if err := profile.validateDraw(numbers); err != nil {
			return "", "", apperrors.NewBusinessError("INVALID_DRAW", "开奖号码不符合注单规则："+err.Error())
		}
		return evaluateMarkSixBetForVersion(version, numbers, strings.ToLower(strings.TrimSpace(playCode)), position, strings.TrimSpace(selection), drawAt)
	}
	won, reason, err := evaluateBetForRuleVersion(game, version, numbers, playCode, position, selection)
	if err != nil {
		return "", "", err
	}
	return markSixWonLost(won), reason, nil
}

// settlementRuleProfile is the single rule identity gate for settlement.
// Missing snapshots never fall back to an unversioned payout algorithm.
func settlementRuleProfile(game *lottery.Game, version string) (gameRuleProfile, error) {
	if _, supported := rulesForGame(game); !supported {
		return gameRuleProfile{}, apperrors.NewBusinessError("RULES_NOT_READY", "该彩种规则尚待核对，暂不能结算")
	}
	profile, found := rulesForVersion(version)
	if !found || !gameSupportsRuleVersion(game.ID, version) {
		return gameRuleProfile{}, apperrors.NewBusinessError("RULES_NOT_READY", "注单规则版本未确认或与彩种不一致，暂不能结算")
	}
	return profile, nil
}

// Push outcomes use evaluateBetOutcomeForRuleVersionAt. This bool evaluator
// serves the other rule families without inferring a family from draw shape.
func evaluateBetForRuleVersion(game *lottery.Game, version string, numbers []int, playCode string, position int, selection string) (bool, string, error) {
	profile, err := settlementRuleProfile(game, version)
	if err != nil {
		return false, "", err
	}
	if err := profile.validateDraw(numbers); err != nil {
		return false, "", apperrors.NewBusinessError("INVALID_DRAW", "开奖号码不符合注单规则："+err.Error())
	}
	if profile.MarkSix {
		outcome, reason, err := evaluateMarkSixBetForVersion(version, numbers, strings.ToLower(strings.TrimSpace(playCode)), position, strings.TrimSpace(selection), time.Time{})
		return outcome == markSixOutcomeWon, reason, err
	}
	if profile.PC28 > 0 {
		outcome, reason, err := evaluatePC28Bet(profile, numbers, strings.ToLower(strings.TrimSpace(playCode)), position, strings.TrimSpace(selection), false)
		return outcome == markSixOutcomeWon, reason, err
	}
	playCode = strings.ToLower(strings.TrimSpace(playCode))
	if err := profile.validateChoice(playCode, position, selection); err != nil {
		return false, "", err
	}
	selection = normalizeSelection(selection)
	switch playCode {
	case "ball_1_5":
		digit, _ := strconv.Atoi(selection) // validateChoice has checked the domain.
		value := numbers[position-1]
		return digit == value, fmt.Sprintf("第%d球开出 %d", position, value), nil
	case "two_sided":
		value := numbers[position-1]
		label := fmt.Sprintf("第%d球", position)
		if matchRuleSide(value, profile.PositionBigFrom, selection) {
			return true, label + "命中" + selectionLabel(selection), nil
		}
		return false, label + "为" + describeSideAtThreshold(value, profile.PositionBigFrom), nil
	case "dragon_tiger", "dragon_tiger_tie":
		left, right := numbers[position-1], numbers[profile.BallCount-position]
		outcome := "tie"
		if left > right {
			outcome = "dragon"
		} else if left < right {
			outcome = "tiger"
		}
		return selection == outcome, fmt.Sprintf("龙虎 %d:%d", left, right), nil
	case "sum":
		total := sumInts(numbers)
		label := "总和"
		if profile.Racing {
			total, label = numbers[0]+numbers[1], "冠亚和"
		}
		if selected, err := strconv.Atoi(selection); err == nil {
			if profile.Racing {
				return total == selected, fmt.Sprintf("冠亚和 %d", total), nil
			}
			return total%10 == selected, fmt.Sprintf("总和尾 %d", total%10), nil
		}
		if matchRuleSide(total, profile.SumBigFrom, selection) {
			return true, fmt.Sprintf("%s %d 命中%s", label, total, selectionLabel(selection)), nil
		}
		return false, label + "为" + describeSideAtThreshold(total, profile.SumBigFrom), nil
	case "leopard", "straight", "pair", "half_straight", "mixed":
		start := position - 1
		pattern := frontPattern(numbers[start : start+3])
		return pattern == playCode, digitShapeScopeName(position) + "形态为" + playNameOf(pattern), nil
	default:
		return false, "", apperrors.NewBusinessError("RULES_NOT_READY", "注单玩法尚未建模，暂不能结算")
	}
}

func matchRuleSide(value, bigFrom int, selection string) bool {
	switch normalizeSelection(selection) {
	case "big":
		return value >= bigFrom
	case "small":
		return value < bigFrom
	case "odd":
		return value%2 != 0
	case "even":
		return value%2 == 0
	default:
		return false
	}
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
	return describeSideAtThreshold(value, 5)
}

func describeSideAtThreshold(value, bigFrom int) string {
	size := "小"
	if value >= bigFrom {
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

// generateDrawNumbers samples only an explicitly modelled game. Entropy is
// supplied per call so failure tests never replace the process-wide reader.
func generateDrawNumbers(game *lottery.Game, entropy io.Reader) ([]int, error) {
	if game != nil && game.ID == "sg-ssc" {
		return nil, apperrors.NewBusinessError("DRAW_NOT_FOUND", "SG时时彩仅接受双站核对结果，不能自动生成")
	}
	profile, supported := rulesForGame(game)
	if !supported {
		return nil, apperrors.NewBusinessError("RULES_NOT_READY", "该彩种规则尚待核对，不能自动生成开奖号码")
	}
	if profile.MarkSix {
		return nil, apperrors.NewBusinessError("DRAW_NOT_FOUND", "宾果六合彩必须提供实际来源开奖号码，不能自动生成")
	}
	switch strings.ToLower(strings.TrimSpace(game.SourceKind)) {
	case "platform", "simulated":
		// Only an explicit platform source may generate a result.
	default:
		return nil, apperrors.NewBusinessError("DRAW_NOT_FOUND", "外部或官方彩种必须提供实际开奖号码，不能自动生成")
	}
	if entropy == nil {
		return nil, apperrors.NewBusinessError("DRAW_RANDOM_FAILED", "开奖随机源不可用")
	}
	rangeSize := profile.MaxNumber - profile.MinNumber + 1
	if profile.BallCount <= 0 || rangeSize <= 0 || profile.Unique && profile.BallCount > rangeSize {
		return nil, apperrors.NewBusinessError("RULES_NOT_READY", "彩种开奖规则不完整")
	}
	result := make([]int, profile.BallCount)
	if profile.Unique {
		pool := make([]int, rangeSize)
		for i := range pool {
			pool[i] = profile.MinNumber + i
		}
		// Partial Fisher-Yates with rejection-sampled uniform indices: no
		// modulo bias or deterministic positional offsets.
		for i := range result {
			index, err := cryptorand.Int(entropy, big.NewInt(int64(len(pool)-i)))
			if err != nil {
				return nil, apperrors.NewSystemError("DRAW_RANDOM_FAILED", "开奖随机源读取失败", err)
			}
			j := i + int(index.Int64())
			pool[i], pool[j] = pool[j], pool[i]
			result[i] = pool[i]
		}
	} else {
		for i := range result {
			number, err := cryptorand.Int(entropy, big.NewInt(int64(rangeSize)))
			if err != nil {
				return nil, apperrors.NewSystemError("DRAW_RANDOM_FAILED", "开奖随机源读取失败", err)
			}
			result[i] = profile.MinNumber + int(number.Int64())
		}
	}
	if err := profile.validateDraw(result); err != nil {
		return nil, apperrors.NewSystemError("DRAW_RANDOM_FAILED", "生成开奖号码校验失败", err)
	}
	return result, nil
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
	validTurnoverCents := item.AmountCents
	if item.ValidTurnoverCents != nil {
		validTurnoverCents = *item.ValidTurnoverCents
	}
	rebateCents = int64(math.Round(float64(validTurnoverCents) * clampPercent(item.RebateRateSnapshot) / 100))
	// Agent share remains based on actual positive GGR. PC28-v1 can make valid
	// turnover zero without erasing the member's actual stake or platform P&L.
	agentShareCents = agentProfitShareCents(item.AmountCents-payoutCents, item.AgentShareRateSnapshot)
	return rebateCents, agentShareCents
}
