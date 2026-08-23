package services

import (
	"backend/data/models/bet"
	"backend/data/models/rebate"
	"backend/data/models/settings"
	"backend/data/models/user"
	apperrors "backend/errors"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RebateAdminService struct{ db *gorm.DB }

type RebateConfig struct {
	Enabled     bool    `json:"enabled"`
	RatePercent float64 `json:"rate_percent"`
	MinTurnover float64 `json:"min_turnover"`
	SettleMode  string  `json:"settle_mode"`
	AutoCredit  bool    `json:"auto_credit"`
}

type RebateRunResult struct {
	BizDate       string  `json:"biz_date"`
	UserCount     int     `json:"user_count"`
	TotalRebate   float64 `json:"total_rebate"`
	TotalTurnover float64 `json:"total_turnover"`
	Skipped       int     `json:"skipped"`
}

type RebatePreview struct {
	BizDate       string  `json:"biz_date"`
	Enabled       bool    `json:"enabled"`
	RatePercent   float64 `json:"rate_percent"`
	Estimated     float64 `json:"estimated"`
	Credited      float64 `json:"credited"`
	PendingCredit float64 `json:"pending_credit"`
}

func NewRebateAdminService(db *gorm.DB) *RebateAdminService { return &RebateAdminService{db: db} }

func (s *RebateAdminService) PreviewToday() (*RebatePreview, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	bizDate := bizDateCST(time.Now())
	start := startOfDayCST(time.Now())
	type previewRow struct {
		UserID   uint64
		Turnover int64
	}
	var rows []previewRow
	if err := s.db.Model(&bet.Bet{}).Select("user_id, COALESCE(SUM(amount_cents),0) AS turnover").Where("created_at >= ? AND status IN ?", start, []string{"won", "lost"}).Group("user_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	estimated := 0.0
	if cfg.Enabled {
		for _, item := range rows {
			if centsToAmount(item.Turnover) < cfg.MinTurnover {
				continue
			}
			rate, _, err := NewTradingAdminService(s.db).ResolveRebateRate(item.UserID)
			if err != nil || rate <= 0 {
				continue
			}
			estimated += roundMoney(centsToAmount(item.Turnover) * rate / 100)
		}
	}
	var credited int64
	_ = s.db.Model(&rebate.DailyRecord{}).Where("biz_date = ?", bizDate).Select("COALESCE(SUM(amount_cents),0)").Scan(&credited).Error
	pending := estimated - centsToAmount(credited)
	if pending < 0 {
		pending = 0
	}
	return &RebatePreview{BizDate: bizDate, Enabled: cfg.Enabled, RatePercent: cfg.RatePercent, Estimated: estimated, Credited: centsToAmount(credited), PendingCredit: pending}, nil
}

func (s *RebateAdminService) TodayAmount() (float64, error) {
	preview, err := s.PreviewToday()
	if err != nil {
		return 0, err
	}
	if preview.Credited > 0 {
		return preview.Credited, nil
	}
	return preview.Estimated, nil
}

func (s *RebateAdminService) RunToday(operator string) (*RebateRunResult, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "回水功能未启用，请先在系统设置中开启")
	}
	bizDate := bizDateCST(time.Now())
	start := startOfDayCST(time.Now())
	type row struct {
		UserID   uint64
		Username string
		Turnover int64
	}
	var rows []row
	if err := s.db.Model(&bet.Bet{}).
		Select("user_id, MAX(username) as username, COALESCE(SUM(amount_cents),0) as turnover").
		Where("created_at >= ? AND status IN ?", start, []string{"won", "lost"}).
		Group("user_id").Having("COALESCE(SUM(amount_cents),0) > 0").
		Scan(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("REBATE_READ_FAILED", "统计回水流水失败", err)
	}
	result := &RebateRunResult{BizDate: bizDate}
	minCents := int64(math.Round(cfg.MinTurnover * 100))
	err = s.db.Transaction(func(tx *gorm.DB) error {
		for _, item := range rows {
			if item.Turnover < minCents {
				result.Skipped++
				continue
			}
			var exists int64
			if err := tx.Model(&rebate.DailyRecord{}).Where("biz_date = ? AND user_id = ?", bizDate, item.UserID).Count(&exists).Error; err != nil {
				return err
			}
			if exists > 0 {
				result.Skipped++
				continue
			}
			rate, _, rateErr := NewTradingAdminService(tx).ResolveRebateRate(item.UserID)
			if rateErr != nil || rate <= 0 {
				result.Skipped++
				continue
			}
			amount := int64(math.Round(float64(item.Turnover) * rate / 100))
			if amount <= 0 {
				result.Skipped++
				continue
			}
			var account user.User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, item.UserID).Error; err != nil {
				return apperrors.NewBusinessError("USER_NOT_FOUND", fmt.Sprintf("用户 %d 不存在", item.UserID))
			}
			after := account.BalanceCents + amount
			if err := tx.Model(&account).Update("balance_cents", after).Error; err != nil {
				return err
			}
			if err := tx.Create(&user.BalanceTransaction{
				UserID: account.UserID, AmountCents: amount, BeforeCents: account.BalanceCents, AfterCents: after,
				Type: "rebate", Remark: "每日回水 " + bizDate, Operator: defaultString(operator, "系统"),
			}).Error; err != nil {
				return err
			}
			if err := tx.Create(&rebate.DailyRecord{
				BizDate: bizDate, UserID: item.UserID, Username: item.Username,
				TurnoverCents: item.Turnover, RatePercent: rate, AmountCents: amount,
				Status: "credited", Operator: defaultString(operator, "系统"),
			}).Error; err != nil {
				return err
			}
			result.UserCount++
			result.TotalRebate += centsToAmount(amount)
			result.TotalTurnover += centsToAmount(item.Turnover)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *RebateAdminService) loadConfig() (*RebateConfig, error) {
	var row settings.SystemConfig
	cfg := &RebateConfig{Enabled: true, RatePercent: 0.5, MinTurnover: 0, SettleMode: "daily", AutoCredit: false}
	if err := s.db.First(&row, 1).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return cfg, nil
		}
		return nil, err
	}
	raw := defaultJSON(row.RebateSettingsJSON, "{}")
	_ = json.Unmarshal([]byte(raw), cfg)
	if cfg.SettleMode == "" {
		cfg.SettleMode = "daily"
	}
	return cfg, nil
}

func bizDateCST(now time.Time) string {
	return now.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02")
}

func roundMoney(value float64) float64 {
	return math.Round(value*100) / 100
}
