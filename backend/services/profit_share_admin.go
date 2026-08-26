package services

import (
	"backend/data/models/bet"
	"backend/data/models/profitshare"
	"backend/data/models/user"
	apperrors "backend/errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AgentProfitShareService turns the immutable share amount captured on each
// settled bet into a real, auditable credit on the owning agent account.
type AgentProfitShareService struct{ db *gorm.DB }

type ProfitShareItem struct {
	RecordID          uint64     `json:"record_id,omitempty"`
	BizDate           string     `json:"biz_date"`
	AgentID           uint64     `json:"agent_id"`
	AgentUsername     string     `json:"agent_username"`
	RoomCode          string     `json:"room_code"`
	RoomScope         string     `json:"room_scope"`
	BetCount          int64      `json:"bet_count"`
	Turnover          float64    `json:"turnover"`
	Payout            float64    `json:"payout"`
	GrossProfit       float64    `json:"gross_profit"`
	Rebate            float64    `json:"rebate"`
	AccruedShare      float64    `json:"accrued_share"`
	PaidShare         float64    `json:"paid_share"`
	PendingShare      float64    `json:"pending_share"`
	Status            string     `json:"status"`
	LastTransactionID uint64     `json:"last_transaction_id,omitempty"`
	LastPaidAt        *time.Time `json:"last_paid_at,omitempty"`
}

type ProfitShareStatement struct {
	BizDate       string            `json:"biz_date"`
	Items         []ProfitShareItem `json:"items"`
	AgentCount    int               `json:"agent_count"`
	TotalTurnover float64           `json:"total_turnover"`
	TotalGross    float64           `json:"total_gross_profit"`
	TotalAccrued  float64           `json:"total_accrued_share"`
	TotalPaid     float64           `json:"total_paid_share"`
	TotalPending  float64           `json:"total_pending_share"`
}

type ProfitShareRunResult struct {
	BizDate       string  `json:"biz_date"`
	CreditedRooms int     `json:"credited_rooms"`
	SkippedRooms  int     `json:"skipped_rooms"`
	Credited      float64 `json:"credited"`
	Pending       float64 `json:"pending"`
}

type profitShareAggregate struct {
	RoomScope     string
	BetCount      int64
	TurnoverCents int64
	PayoutCents   int64
	RebateCents   int64
	ShareCents    int64
}

func NewAgentProfitShareService(db *gorm.DB) *AgentProfitShareService {
	return &AgentProfitShareService{db: db}
}

func (s *AgentProfitShareService) Statement(date string, onlyAgentID uint64) (*ProfitShareStatement, error) {
	bizDate, start, end, err := profitShareDay(date)
	if err != nil {
		return nil, err
	}
	rows, err := s.aggregate(start, end, onlyAgentID)
	if err != nil {
		return nil, err
	}
	result := &ProfitShareStatement{BizDate: bizDate, Items: make([]ProfitShareItem, 0, len(rows))}
	for _, aggregate := range rows {
		agentID, ok := agentIDFromScope(aggregate.RoomScope)
		if !ok || (onlyAgentID > 0 && onlyAgentID != agentID) {
			continue
		}
		var agent user.User
		if err := s.db.Select("user_id", "username", "agent_room_code", "role").First(&agent, agentID).Error; err != nil || agent.Role != "agent" {
			continue
		}
		var record profitshare.DailyRecord
		recordErr := s.db.Where("biz_date = ? AND agent_id = ?", bizDate, agentID).First(&record).Error
		if recordErr != nil && recordErr != gorm.ErrRecordNotFound {
			return nil, recordErr
		}
		paid := int64(0)
		status := "pending"
		if recordErr == nil {
			paid = record.PaidShareCents
			status = record.Status
		}
		pending := outstandingProfitShare(aggregate.ShareCents, paid)
		if pending == 0 && aggregate.ShareCents == 0 {
			status = "no_share"
		} else if pending == 0 {
			status = "credited"
		} else if paid > 0 {
			status = "partial"
		}
		item := ProfitShareItem{
			RecordID: record.ID, BizDate: bizDate, AgentID: agentID, AgentUsername: agent.Username,
			RoomCode: agent.AgentRoomCode, RoomScope: aggregate.RoomScope, BetCount: aggregate.BetCount,
			Turnover: centsToAmount(aggregate.TurnoverCents), Payout: centsToAmount(aggregate.PayoutCents),
			GrossProfit: centsToAmount(aggregate.TurnoverCents - aggregate.PayoutCents), Rebate: centsToAmount(aggregate.RebateCents),
			AccruedShare: centsToAmount(aggregate.ShareCents), PaidShare: centsToAmount(paid), PendingShare: centsToAmount(pending),
			Status: status, LastTransactionID: record.LastTransactionID, LastPaidAt: record.LastPaidAt,
		}
		result.Items = append(result.Items, item)
		result.TotalTurnover += item.Turnover
		result.TotalGross += item.GrossProfit
		result.TotalAccrued += item.AccruedShare
		result.TotalPaid += item.PaidShare
		result.TotalPending += item.PendingShare
	}
	result.AgentCount = len(result.Items)
	return result, nil
}

func (s *AgentProfitShareService) Run(date, operator string) (*ProfitShareRunResult, error) {
	statement, err := s.Statement(date, 0)
	if err != nil {
		return nil, err
	}
	result := &ProfitShareRunResult{BizDate: statement.BizDate}
	for _, item := range statement.Items {
		if item.PendingShare <= 0 {
			result.SkippedRooms++
			continue
		}
		credited, err := s.creditOne(item, defaultString(operator, "系统"))
		if err != nil {
			return nil, err
		}
		if credited <= 0 {
			result.SkippedRooms++
			continue
		}
		result.CreditedRooms++
		result.Credited += centsToAmount(credited)
	}
	latest, err := s.Statement(statement.BizDate, 0)
	if err != nil {
		return nil, err
	}
	result.Pending = latest.TotalPending
	return result, nil
}

func (s *AgentProfitShareService) creditOne(item ProfitShareItem, operator string) (int64, error) {
	credited := int64(0)
	err := s.db.Transaction(func(tx *gorm.DB) error {
		placeholder := profitshare.DailyRecord{
			BizDate: item.BizDate, AgentID: item.AgentID, RoomScope: item.RoomScope,
			AgentUsername: item.AgentUsername, RoomCode: item.RoomCode, Status: "pending",
		}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "biz_date"}, {Name: "agent_id"}}, DoNothing: true}).Create(&placeholder).Error; err != nil {
			return err
		}
		var record profitshare.DailyRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("biz_date = ? AND agent_id = ?", item.BizDate, item.AgentID).First(&record).Error; err != nil {
			return err
		}
		accrued := profitShareAmountToCents(item.AccruedShare)
		pending := outstandingProfitShare(accrued, record.PaidShareCents)
		updates := map[string]any{
			"room_scope": item.RoomScope, "agent_username": item.AgentUsername, "room_code": item.RoomCode,
			"bet_count": item.BetCount, "turnover_cents": profitShareAmountToCents(item.Turnover), "payout_cents": profitShareAmountToCents(item.Payout),
			"gross_profit_cents": profitShareAmountToCents(item.GrossProfit), "rebate_cents": profitShareAmountToCents(item.Rebate), "accrued_share_cents": accrued,
		}
		if pending <= 0 {
			updates["status"] = "credited"
			return tx.Model(&record).Updates(updates).Error
		}
		var agent user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&agent, item.AgentID).Error; err != nil {
			return apperrors.NewBusinessError("AGENT_NOT_FOUND", fmt.Sprintf("代理 %d 不存在", item.AgentID))
		}
		if agent.Role != "agent" {
			return apperrors.NewBusinessError("INVALID_AGENT", fmt.Sprintf("账号 %d 已不是代理", item.AgentID))
		}
		after := agent.BalanceCents + pending
		if err := tx.Model(&agent).Update("balance_cents", after).Error; err != nil {
			return err
		}
		ledger := user.BalanceTransaction{
			UserID: agent.UserID, Reference: fmt.Sprintf("agent_share:%d:%d", record.ID, record.RunCount+1),
			AmountCents: pending, BeforeCents: agent.BalanceCents, AfterCents: after,
			Type: "agent_share", Remark: "代理利润分成 " + item.BizDate, Operator: operator,
		}
		if err := tx.Create(&ledger).Error; err != nil {
			return err
		}
		now := time.Now()
		updates["paid_share_cents"] = record.PaidShareCents + pending
		updates["last_transaction_id"] = ledger.ID
		updates["run_count"] = record.RunCount + 1
		updates["status"] = "credited"
		updates["operator"] = operator
		updates["last_paid_at"] = now
		if err := tx.Model(&record).Updates(updates).Error; err != nil {
			return err
		}
		credited = pending
		return nil
	})
	return credited, err
}

func (s *AgentProfitShareService) aggregate(start, end time.Time, onlyAgentID uint64) ([]profitShareAggregate, error) {
	query := s.db.Model(&bet.Bet{}).
		Select(`room_scope, COUNT(*) AS bet_count,
			COALESCE(SUM(amount_cents),0) AS turnover_cents,
			COALESCE(SUM(payout_cents),0) AS payout_cents,
			COALESCE(SUM(rebate_cents),0) AS rebate_cents,
			COALESCE(SUM(agent_share_cents),0) AS share_cents`).
		Where("status IN ? AND settled_at >= ? AND settled_at < ?", []string{"won", "lost"}, start, end).
		Where("room_scope LIKE ?", "agent:%")
	if onlyAgentID > 0 {
		query = query.Where("room_scope = ?", fmt.Sprintf("agent:%d", onlyAgentID))
	}
	var rows []profitShareAggregate
	if err := query.Group("room_scope").Order("room_scope ASC").Scan(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("PROFIT_SHARE_READ_FAILED", "统计代理分成失败", err)
	}
	return rows, nil
}

func profitShareDay(value string) (string, time.Time, time.Time, error) {
	zone := time.FixedZone("CST", 8*3600)
	value = strings.TrimSpace(value)
	if value == "" {
		value = time.Now().In(zone).Format("2006-01-02")
	}
	start, err := time.ParseInLocation("2006-01-02", value, zone)
	if err != nil {
		return "", time.Time{}, time.Time{}, apperrors.NewBusinessError("INVALID_DATE", "分账日期格式应为 YYYY-MM-DD")
	}
	return value, start, start.AddDate(0, 0, 1), nil
}

func agentIDFromScope(scope string) (uint64, bool) {
	if !strings.HasPrefix(scope, "agent:") {
		return 0, false
	}
	id, err := strconv.ParseUint(strings.TrimPrefix(scope, "agent:"), 10, 64)
	return id, err == nil && id > 0
}

func outstandingProfitShare(accrued, paid int64) int64 {
	if accrued <= paid {
		return 0
	}
	return accrued - paid
}

func profitShareAmountToCents(amount float64) int64 {
	return int64(math.Round(amount * 100))
}
