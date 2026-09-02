package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/data/models/user"
	apperrors "backend/errors"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WebBetItem is a typed, atomic ticket line. Game, issue, member and odds are
// deliberately absent: the route/service bind them and the trading resolver
// supplies the price.
type WebBetItem struct {
	PlayCode  string  `json:"play_code"`
	PlayName  string  `json:"play_name,omitempty"`
	Position  int     `json:"position"`
	Selection string  `json:"selection"`
	Amount    float64 `json:"amount"`
}

// PlaceWeb accepts server-declared typed web contracts. Mark Six remains
// web-only; PC28 deliberately supports both this atomic board and chat input.
// It reuses the durable assistant request receipt so detailed-board history,
// request retries and the financial debit remain one atomic operation.
func (s *BetAssistantService) PlaceWeb(userID uint64, gameID, issue string, items []WebBetItem, operator, requestID string) (*AssistantBetResult, error) {
	requestID = strings.TrimSpace(requestID)
	if len(requestID) < 8 || len(requestID) > 96 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "请求标识不正确")
	}
	if len(items) == 0 || len(items) > 200 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "每张网投单必须包含1至200个投注项")
	}
	gameID, issue = strings.TrimSpace(gameID), strings.TrimSpace(issue)
	profile, webReady := rulesForGame(&lottery.Game{ID: gameID})
	if !webReady || (!profile.MarkSix && profile.PC28 == 0) {
		return nil, apperrors.NewBusinessError("BET_MODE_UNAVAILABLE", "该彩种暂不支持详细网投")
	}
	if len([]rune(issue)) > 64 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "期号过长")
	}
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		code := strings.ToLower(strings.TrimSpace(item.PlayCode))
		selection := strings.TrimSpace(item.Selection)
		if profile.MarkSix {
			selection = markSixNormalizeSelection(code, selection)
		} else {
			selection = pc28NormalizeSelection(code, selection)
		}
		if code == "" || selection == "" || len([]rune(code)) > 40 || len([]rune(item.PlayName)) > 40 || len([]rune(selection)) > 40 {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "网投项玩法或选择内容不正确")
		}
		if err := profile.validateChoice(code, item.Position, selection); err != nil {
			return nil, err
		}
		key := fmt.Sprintf("%s\x00%d\x00%s", code, item.Position, selection)
		if _, exists := seen[key]; exists {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "同一张网投单不能包含重复投注项")
		}
		seen[key] = struct{}{}
	}
	payloadHash, hashErr := idempotencyPayloadHash(struct {
		GameID string       `json:"game_id"`
		Issue  string       `json:"issue"`
		Items  []WebBetItem `json:"items"`
	}{GameID: gameID, Issue: issue, Items: items})
	if hashErr != nil {
		return nil, apperrors.NewSystemError("REQUEST_SAVE_FAILED", "生成网投请求凭证失败", hashErr)
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
		row := bet.AssistantRequest{
			WorkspaceID: owner.WorkspaceID, UserID: userID, RequestID: requestID,
			PayloadHash: payloadHash, Status: "processing",
		}
		created := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}, {Name: "request_id"}}, DoNothing: true,
		}).Create(&row)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 0 {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("user_id = ? AND request_id = ?", userID, requestID).First(&row).Error; err != nil {
				return err
			}
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

		if _, err := lockBettingGame(tx, gameID); err != nil {
			return err
		}
		atomic := &BetAssistantService{db: tx, bets: &BetAdminService{db: tx, suppressNotifications: true}}
		placed, err := atomic.placeWeb(userID, gameID, issue, items, operator, assistantBetRequestReference(row.ID))
		if err != nil {
			return err
		}
		var committedOwner user.User
		if err := tx.Select("user_id", "workspace_id", "role", "parent_agent_id", "parent_tenant_id").First(&committedOwner, userID).Error; err != nil {
			return err
		}
		if committedOwner.WorkspaceID != row.WorkspaceID {
			// The member changed rooms after this request reserved its receipt but
			// before PlaceBatch acquired the member lock. Returning an error here
			// rolls back the debit, bet rows and still-processing receipt together.
			return apperrors.NewBusinessError("REQUEST_CONTEXT_CHANGED", "下注期间所属房间已变化，请刷新后重新提交")
		}
		payload, err := json.Marshal(placed)
		if err != nil {
			return apperrors.NewSystemError("REQUEST_SAVE_FAILED", "保存网投回执失败", err)
		}
		updated := tx.Model(&bet.AssistantRequest{}).Where("id = ? AND status = ? AND result_json = ''", row.ID, "processing").
			Updates(map[string]any{"status": "completed", "result_json": string(payload), "last_error": ""})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("网投请求状态发生冲突")
		}
		result, notify = placed, true
		workspaceID, roomScope = committedOwner.WorkspaceID, betRoomScope(committedOwner)
		return nil
	})
	if err != nil {
		if app, ok := err.(*apperrors.AppError); ok {
			return nil, app
		}
		return nil, apperrors.NewSystemError("BET_CREATE_FAILED", "创建网投单失败", err)
	}
	if terminalErr != nil {
		return nil, terminalErr
	}
	if notify && result != nil {
		s.bets.notifyPlacement(userID, workspaceID, roomScope, result.GameID, result.Issue, result.Balance)
	}
	return result, nil
}

func (s *BetAssistantService) placeWeb(userID uint64, gameID, issue string, items []WebBetItem, operator, ledgerReference string) (*AssistantBetResult, error) {
	game, err := s.bets.loadGame(gameID)
	if err != nil {
		return nil, err
	}
	profile, ready := rulesForGame(game)
	if !ready || (!profile.MarkSix && profile.PC28 == 0) {
		return nil, apperrors.NewBusinessError("BET_MODE_UNAVAILABLE", "该彩种暂不支持详细网投")
	}
	if !game.Enabled {
		return nil, apperrors.NewBusinessError("GAME_DISABLED", "该彩种暂未开放投注")
	}
	if ok, version, _, modes := memberOddsRuleStatus(game.ID); !ok || version != profile.Version || !modes.Web || (profile.MarkSix && modes.Chat) {
		return nil, apperrors.NewBusinessError("RULES_NOT_READY", "当前彩种网投规则尚未就绪")
	}
	requestedIssue := strings.TrimSpace(issue)
	if requestedIssue == "" {
		requestedIssue, err = s.bets.BettingIssue(game.ID)
		if err != nil {
			return nil, err
		}
	}
	inputs := make([]PlaceBetInput, 0, len(items))
	lines := make([]AssistantBetLine, 0, len(items))
	var totalCents int64
	for _, item := range items {
		amountCents, err := validatedStakeCents(item.Amount)
		if err != nil {
			return nil, err
		}
		var ok bool
		totalCents, ok = safeAddInt64(totalCents, amountCents)
		if !ok || totalCents <= 0 {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "下注总金额过大")
		}
		remark := "PC28详细网投"
		if profile.MarkSix {
			remark = "宾果六合彩网投"
		}
		inputs = append(inputs, PlaceBetInput{
			GameID: game.ID, Issue: requestedIssue, UserID: userID,
			PlayCode: item.PlayCode, PlayName: item.PlayName, Position: item.Position,
			Selection: item.Selection, Amount: item.Amount, Operator: defaultString(operator, "网投"),
			Remark: remark, LedgerReference: ledgerReference, BetMode: "web",
		})
	}
	placed, err := s.bets.PlaceBatch(inputs)
	if err != nil {
		return nil, err
	}
	for index := range items {
		if index >= len(placed) {
			return nil, apperrors.NewSystemError("BET_CREATE_FAILED", "网投回执数量不一致", nil)
		}
		view := placed[index]
		lines = append(lines, AssistantBetLine{
			Position: view.Position, Selection: view.Selection, PlayCode: view.PlayCode, PlayName: view.PlayName,
			Amount: view.Amount, Odds: view.Odds, Label: typedWebLineLabel(view),
		})
	}
	var account user.User
	if err := s.db.Select("balance_cents").First(&account, userID).Error; err != nil {
		return nil, apperrors.NewSystemError("BALANCE_READ_FAILED", "读取扣分后的余额失败", err)
	}
	return &AssistantBetResult{
		GameID: game.ID, GameName: game.Name, RuleVersion: profile.Version, Issue: requestedIssue,
		Content: fmt.Sprintf("网投 %d 注", len(lines)), Lines: lines, BetCount: len(lines),
		Total: centsToAmount(totalCents), Balance: centsToAmount(account.BalanceCents), AcceptedAt: time.Now().UTC(),
	}, nil
}

func markSixWebLineLabel(view BetView) string {
	if view.Position > 0 {
		return fmt.Sprintf("%s 第%d位 %s", view.PlayName, view.Position, view.Selection)
	}
	return strings.TrimSpace(view.PlayName + " " + view.Selection)
}

func typedWebLineLabel(view BetView) string {
	if strings.HasPrefix(view.RuleVersion, "pc28-") {
		if view.Position > 0 {
			return fmt.Sprintf("%s 第%d球 %s", view.PlayName, view.Position, view.Selection)
		}
		return strings.TrimSpace(view.PlayName + " " + view.Selection)
	}
	return markSixWebLineLabel(view)
}
