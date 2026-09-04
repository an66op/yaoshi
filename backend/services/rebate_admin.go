package services

import (
	"backend/data/models/bet"
	"backend/data/models/rebate"
	"backend/data/models/settings"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"encoding/json"
	"fmt"
	"math"
	"strings"
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
	return s.PreviewTodayForWorkspace(0)
}

func (s *RebateAdminService) PreviewTodayForWorkspace(workspaceID uint64) (*RebatePreview, error) {
	cfg, err := s.loadConfig(workspaceID)
	if err != nil {
		return nil, err
	}
	bizDate := bizDateCST(time.Now())
	start := startOfDayCST(time.Now())
	type previewRow struct {
		UserID      uint64
		Turnover    int64
		RebateCents int64
	}
	var rows []previewRow
	betQuery := s.db.Model(&bet.Bet{}).
		Select("user_id, COALESCE(SUM(COALESCE(valid_turnover_cents,amount_cents)),0) AS turnover, COALESCE(SUM(rebate_cents),0) AS rebate_cents").
		Where("COALESCE(settled_at,updated_at,created_at) >= ? AND status IN ?", start, []string{"won", "lost"})
	betQuery = excludeRobotProfileBets(betQuery)
	if workspaceID > 0 {
		betQuery = betQuery.Where("workspace_id = ?", workspaceID)
	} else {
		betQuery = betQuery.Where("workspace_id > 0")
	}
	if err := betQuery.Group("user_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	estimated := 0.0
	if cfg.Enabled {
		for _, item := range rows {
			if centsToAmount(item.Turnover) < cfg.MinTurnover {
				continue
			}
			estimated += centsToAmount(item.RebateCents)
		}
	}
	var credited int64
	recordQuery := s.db.Model(&rebate.DailyRecord{}).Where("biz_date = ?", bizDate)
	if workspaceID > 0 {
		recordQuery = recordQuery.Where("workspace_id = ?", workspaceID)
	} else {
		recordQuery = recordQuery.Where("workspace_id > 0")
	}
	_ = recordQuery.Select("COALESCE(SUM(amount_cents),0)").Scan(&credited).Error
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
	return s.RunTodayForWorkspace(0, operator)
}

func (s *RebateAdminService) RunTodayForWorkspace(workspaceID uint64, operator string) (*RebateRunResult, error) {
	if workspaceID == 0 {
		return nil, apperrors.NewBusinessError("WORKSPACE_REQUIRED", "请选择需要结算回水的房间")
	}
	cfg, err := s.loadConfig(workspaceID)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "回水功能未启用，请先在系统设置中开启")
	}
	bizDate := bizDateCST(time.Now())
	start := startOfDayCST(time.Now())
	type row struct {
		UserID      uint64
		Username    string
		Turnover    int64
		RebateCents int64
	}
	var rows []row
	rebateQuery := s.db.Model(&bet.Bet{}).
		Select("user_id, MAX(username) as username, COALESCE(SUM(COALESCE(valid_turnover_cents,amount_cents)),0) as turnover, COALESCE(SUM(rebate_cents),0) as rebate_cents").
		Where("workspace_id = ? AND COALESCE(settled_at,updated_at,created_at) >= ? AND status IN ?", workspaceID, start, []string{"won", "lost"}).
		Group("user_id").Having("COALESCE(SUM(COALESCE(valid_turnover_cents,amount_cents)),0) > 0")
	rebateQuery = excludeRobotProfileBets(rebateQuery)
	if err := rebateQuery.Scan(&rows).Error; err != nil {
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
			if err := tx.Model(&rebate.DailyRecord{}).Where("workspace_id = ? AND biz_date = ? AND user_id = ?", workspaceID, bizDate, item.UserID).Count(&exists).Error; err != nil {
				return err
			}
			if exists > 0 {
				result.Skipped++
				continue
			}
			amount := item.RebateCents
			if amount <= 0 {
				result.Skipped++
				continue
			}
			var account user.User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND workspace_id = ?", item.UserID, workspaceID).First(&account).Error; err != nil {
				return apperrors.NewBusinessError("USER_NOT_FOUND", fmt.Sprintf("用户 %d 不存在", item.UserID))
			}
			before := account.BalanceCents
			after := before + amount
			if err := tx.Model(&account).Update("balance_cents", after).Error; err != nil {
				return err
			}
			if err := tx.Create(&user.BalanceTransaction{
				WorkspaceID: workspaceID, UserID: account.UserID, Reference: fmt.Sprintf("rebate:%d:%s", workspaceID, bizDate),
				AmountCents: amount, BeforeCents: before, AfterCents: after,
				Type: "rebate", Remark: "每日回水 " + bizDate, Operator: defaultString(operator, "系统"),
			}).Error; err != nil {
				return err
			}
			rate := roundMoney(float64(amount) / float64(item.Turnover) * 100)
			if err := tx.Create(&rebate.DailyRecord{
				WorkspaceID: workspaceID, BizDate: bizDate, UserID: item.UserID, Username: item.Username,
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

func (s *RebateAdminService) loadConfig(workspaceID uint64) (*RebateConfig, error) {
	var row settings.SystemConfig
	query := s.db.Model(&settings.SystemConfig{})
	if workspaceID > 0 {
		query = query.Where("workspace_id = ?", workspaceID)
	} else {
		var platform workspacemodel.Workspace
		if err := s.db.Select("id").Where("type = ?", workspacemodel.TypePlatform).First(&platform).Error; err == nil {
			query = query.Where("workspace_id = ?", platform.ID)
		} else {
			query = query.Where("workspace_id = 0").Order("id ASC")
		}
	}
	if err := query.First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return disabledRebateConfig(), nil
		}
		return nil, err
	}
	return decodeStoredRebateConfig(row.RebateSettingsJSON)
}

func disabledRebateConfig() *RebateConfig {
	return &RebateConfig{Enabled: false, RatePercent: 0, MinTurnover: 0, SettleMode: "daily", AutoCredit: false}
}

// decodeStoredRebateConfig is fail-closed. A missing or malformed room setting
// must never silently turn on a financial credit at the historic 0.5% value.
func decodeStoredRebateConfig(raw string) (*RebateConfig, error) {
	cfg := disabledRebateConfig()
	if strings.TrimSpace(raw) == "" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return nil, apperrors.NewSystemError("REBATE_CONFIG_INVALID", "回水配置格式错误，请先在系统设置中修复", err)
	}
	if math.IsNaN(cfg.RatePercent) || math.IsInf(cfg.RatePercent, 0) || cfg.RatePercent < 0 || cfg.RatePercent > 100 ||
		math.IsNaN(cfg.MinTurnover) || math.IsInf(cfg.MinTurnover, 0) || cfg.MinTurnover < 0 || cfg.SettleMode != "daily" {
		return nil, apperrors.NewBusinessError("REBATE_CONFIG_INVALID", "回水配置数值或结算模式不正确")
	}
	return cfg, nil
}

func bizDateCST(now time.Time) string {
	return now.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02")
}

func roundMoney(value float64) float64 {
	return math.Round(value*100) / 100
}
