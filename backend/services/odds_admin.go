package services

import (
	"backend/data/models/lottery"
	"backend/data/models/odds"
	apperrors "backend/errors"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OddsAdminService struct{ db *gorm.DB }

type PlayLimitItem struct {
	PlayCode            string     `json:"play_code"`
	PlayName            string     `json:"play_name"`
	Odds                float64    `json:"odds"`
	MinBet              float64    `json:"min_bet"`
	MaxBet              float64    `json:"max_bet"`
	MaxUserPeriod       float64    `json:"max_user_period"`
	MaxPeriodTotal      float64    `json:"max_period_total"`
	SortOrder           int        `json:"sort_order"`
	Configured          bool       `json:"configured"`
	RuleVersion         string     `json:"rule_version"`
	ConfigurationSource string     `json:"configuration_source"`
	ConfiguredAt        *time.Time `json:"configured_at,omitempty"`
}

const (
	oddsSourceAdminSave           = "admin_save"
	oddsSourceUnconfigured        = "unconfigured"
	oddsSourceRuleVersionMismatch = "rule_version_mismatch"
)

type GameOddsLimits struct {
	GameID         string            `json:"game_id"`
	GameName       string            `json:"game_name"`
	Items          []PlayLimitItem   `json:"items"`
	RulesReady     bool              `json:"rules_ready"`
	RuleVersion    string            `json:"rule_version"`
	ConfigRevision string            `json:"config_revision"`
	RulesMessage   string            `json:"rules_message"`
	RiskWarnings   []OddsRiskWarning `json:"risk_warnings"`
}

type UpdateOddsLimitsInput struct {
	ExpectedRuleVersion string          `json:"expected_rule_version"`
	ExpectedRevision    string          `json:"expected_revision"`
	Items               []PlayLimitItem `json:"items"`
}

type OddsMutationGuard struct {
	ExpectedRuleVersion string `json:"expected_rule_version"`
	ExpectedRevision    string `json:"expected_revision"`
}

func NewOddsAdminService(db *gorm.DB) *OddsAdminService {
	return &OddsAdminService{db: db}
}

func (s *OddsAdminService) Get(gameID string) (*GameOddsLimits, error) {
	var result *GameOddsLimits
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Share the same per-game lock used by writes. Both catalogue rows and
		// their revision now come from one configuration, and callers already
		// inside an override transaction retain this lock until their commit.
		game, err := NewOddsAdminService(tx.Clauses(clause.Locking{Strength: "SHARE"})).loadGame(gameID)
		if err != nil {
			return err
		}
		result, err = NewOddsAdminService(tx).readGameLimits(game)
		return err
	})
	return result, err
}

// getReadOnly returns the same catalogue without row locks or a nested
// read-write transaction. It is reserved for callers already holding a stable
// read-only snapshot, such as the local acceptance auditor.
func (s *OddsAdminService) getReadOnly(gameID string) (*GameOddsLimits, error) {
	game, err := s.loadGame(gameID)
	if err != nil {
		return nil, err
	}
	return s.readGameLimits(game)
}

func (s *OddsAdminService) readGameLimits(game *lottery.Game) (*GameOddsLimits, error) {
	profile, ready := rulesForGame(game)
	if !ready {
		return &GameOddsLimits{GameID: game.ID, GameName: game.Name, Items: []PlayLimitItem{}, RulesMessage: gameRulesUnavailableMessage}, nil
	}
	var rows []odds.PlayLimit
	if err := s.db.Where("game_id = ?", game.ID).Order("sort_order asc, id asc").Find(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("ODDS_READ_FAILED", "读取赔率限额失败", err)
	}
	// Get is deliberately read-only. Missing, unconfirmed, or older-version
	// prices remain unavailable until an administrator explicitly saves the
	// complete current catalogue.
	items := playLimitItemsForProfile(game.ID, profile, PlayCatalogForGame(game.ID), rows)
	return &GameOddsLimits{
		GameID: game.ID, GameName: game.Name, Items: items, RulesReady: true,
		RuleVersion: profile.Version, ConfigRevision: oddsConfigRevision(profile.Version, game.OddsConfigRevision, items),
		RiskWarnings: playLimitOddsRisks(game.ID, items),
	}, nil
}

func playLimitItemsForProfile(gameID string, profile gameRuleProfile, catalog []PlayCatalogItem, rows []odds.PlayLimit) []PlayLimitItem {
	allowed := make(map[string]struct{}, len(catalog))
	for _, item := range catalog {
		allowed[item.PlayCode] = struct{}{}
	}
	configured := make(map[string]odds.PlayLimit, len(rows))
	for _, row := range rows {
		if _, ok := allowed[row.PlayCode]; ok {
			configured[row.PlayCode] = row
		}
	}
	items := make([]PlayLimitItem, 0, len(catalog))
	for _, item := range catalog {
		row, ok := configured[item.PlayCode]
		if ok {
			rowConfigured := isActivePlatformOdds(row, profile.Version)
			source := strings.TrimSpace(row.ConfigurationSource)
			if strings.TrimSpace(row.RuleVersion) != "" && strings.TrimSpace(row.RuleVersion) != profile.Version {
				source = oddsSourceRuleVersionMismatch
			} else if source == "" || !rowConfigured {
				source = oddsSourceUnconfigured
			}
			value := row.Odds
			configuredAt := row.ConfiguredAt
			if !rowConfigured {
				value, configuredAt = 0, nil
			}
			items = append(items, PlayLimitItem{
				PlayCode: row.PlayCode, PlayName: profile.playName(row.PlayCode, row.PlayName), Odds: value,
				MinBet: row.MinBet, MaxBet: row.MaxBet, MaxUserPeriod: row.MaxUserPeriod,
				MaxPeriodTotal: row.MaxPeriodTotal, SortOrder: item.SortOrder, Configured: rowConfigured,
				RuleVersion: row.RuleVersion, ConfigurationSource: source, ConfiguredAt: configuredAt,
			})
			continue
		}
		items = append(items, PlayLimitItem{
			PlayCode: item.PlayCode, PlayName: item.PlayName, Odds: 0,
			MinBet: 1, MaxBet: 50000, MaxUserPeriod: 50000, MaxPeriodTotal: 100000,
			SortOrder: item.SortOrder, Configured: false, ConfigurationSource: oddsSourceUnconfigured,
		})
	}
	return items
}

func isActivePlatformOdds(row odds.PlayLimit, ruleVersion string) bool {
	return isValidOddsOverride(row.Odds) && row.ExplicitlyConfigured &&
		strings.TrimSpace(row.RuleVersion) == ruleVersion &&
		strings.TrimSpace(row.ConfigurationSource) == oddsSourceAdminSave
}

func oddsConfigRevision(ruleVersion string, revision uint64, items []PlayLimitItem) string {
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "rule=%s\nrevision=%d\n", strings.TrimSpace(ruleVersion), revision)
	for _, item := range items {
		configuredAt := ""
		if item.ConfiguredAt != nil {
			configuredAt = item.ConfiguredAt.UTC().Format(time.RFC3339Nano)
		}
		_, _ = fmt.Fprintf(hash, "%s\x00%.4f\x00%.2f\x00%.2f\x00%.2f\x00%.2f\x00%t\x00%s\x00%s\x00%s\n",
			item.PlayCode, item.Odds, item.MinBet, item.MaxBet, item.MaxUserPeriod, item.MaxPeriodTotal,
			item.Configured, item.RuleVersion, item.ConfigurationSource, configuredAt)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (s *OddsAdminService) Update(gameID string, input UpdateOddsLimitsInput) (*GameOddsLimits, error) {
	game, err := s.loadGame(gameID)
	if err != nil {
		return nil, err
	}
	if err := ensureGameRulesSupported(game); err != nil {
		return nil, err
	}
	profile, _ := rulesForGame(game)
	if err := validateOddsMutationGuard(profile.Version, input.ExpectedRuleVersion, input.ExpectedRevision); err != nil {
		return nil, err
	}
	catalog := PlayCatalogForGame(game.ID)
	normalized, err := normalizeOddsLimitItems(catalog, input.Items)
	if err != nil {
		return nil, err
	}

	var result *GameOddsLimits
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var locked lottery.Game
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", game.ID).Error; err != nil {
			return err
		}
		current, err := NewOddsAdminService(tx).readGameLimits(&locked)
		if err != nil {
			return err
		}
		if current.RuleVersion != input.ExpectedRuleVersion || current.ConfigRevision != input.ExpectedRevision {
			return apperrors.NewBusinessError("ODDS_CONFIGURATION_CONFLICT", "赔率配置已被其他操作更新，请刷新后重新编辑")
		}
		if err := tx.Where("game_id = ?", game.ID).Delete(&odds.PlayLimit{}).Error; err != nil {
			return err
		}
		rows := make([]odds.PlayLimit, 0, len(normalized))
		previouslyActive := make(map[string]bool, len(current.Items))
		for _, item := range current.Items {
			previouslyActive[item.PlayCode] = item.Configured
		}
		preservedOverrideCodes := make([]string, 0, len(normalized))
		configuredAt := time.Now().UTC()
		for _, item := range normalized {
			if item.Odds == 0 {
				continue
			}
			if previouslyActive[item.PlayCode] {
				preservedOverrideCodes = append(preservedOverrideCodes, item.PlayCode)
			}
			rows = append(rows, odds.PlayLimit{
				GameID:               game.ID,
				PlayCode:             item.PlayCode,
				PlayName:             item.PlayName,
				Odds:                 roundOdds(item.Odds),
				MinBet:               item.MinBet,
				MaxBet:               item.MaxBet,
				MaxUserPeriod:        item.MaxUserPeriod,
				MaxPeriodTotal:       item.MaxPeriodTotal,
				SortOrder:            item.SortOrder,
				ExplicitlyConfigured: true,
				RuleVersion:          profile.Version,
				ConfigurationSource:  oddsSourceAdminSave,
				ConfiguredAt:         &configuredAt,
			})
		}
		if len(rows) > 0 {
			if err := tx.Create(&rows).Error; err != nil {
				return err
			}
		}
		// Preserve overrides only across an uninterrupted current-rule market.
		// Retired, unconfirmed, and newly reopened plays cannot inherit stale
		// room/member prices from a previous activation or rules contract.
		if err := deleteLowerLevelOddsOverridesExcept(tx, game.ID, preservedOverrideCodes); err != nil {
			return err
		}
		if err := tx.Model(&lottery.Game{}).Where("id = ?", game.ID).
			UpdateColumn("odds_config_revision", gorm.Expr("odds_config_revision + 1")).Error; err != nil {
			return err
		}
		locked.OddsConfigRevision++
		result, err = NewOddsAdminService(tx).readGameLimits(&locked)
		return err
	})
	if err != nil {
		if apperrors.IsBusinessError(err) {
			return nil, err
		}
		return nil, apperrors.NewSystemError("ODDS_SAVE_FAILED", "保存赔率限额失败", err)
	}
	return result, nil
}

func validateOddsMutationGuard(currentRuleVersion, expectedRuleVersion, expectedRevision string) error {
	if strings.TrimSpace(expectedRuleVersion) == "" || strings.TrimSpace(expectedRevision) == "" {
		return apperrors.NewBusinessError("INVALID_REQUEST", "缺少赔率配置版本，请刷新页面后重试")
	}
	if strings.TrimSpace(expectedRuleVersion) != strings.TrimSpace(currentRuleVersion) {
		return apperrors.NewBusinessError("RULE_VERSION_CONFLICT", "玩法规则版本已更新，请刷新赔率页面后重新配置")
	}
	return nil
}

func normalizeOddsLimitItems(catalog []PlayCatalogItem, input []PlayLimitItem) ([]PlayLimitItem, error) {
	if len(input) != len(catalog) {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "必须提交当前彩种的完整玩法目录")
	}
	provided := make(map[string]PlayLimitItem, len(input))
	for _, item := range input {
		code := strings.TrimSpace(item.PlayCode)
		if code == "" {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "玩法编号不能为空")
		}
		if _, exists := provided[code]; exists {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("玩法编号重复: %s", code))
		}
		provided[code] = item
	}
	normalized := make([]PlayLimitItem, 0, len(catalog))
	for index, spec := range catalog {
		item, exists := provided[spec.PlayCode]
		if !exists {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "缺少玩法配置: "+spec.PlayCode)
		}
		item.PlayCode, item.PlayName, item.SortOrder = spec.PlayCode, spec.PlayName, index
		values := []float64{item.Odds, item.MinBet, item.MaxBet, item.MaxUserPeriod, item.MaxPeriodTotal}
		for _, value := range values {
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
				return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("%s 赔率与限额必须是非负有限数值", item.PlayName))
			}
		}
		if item.Odds != 0 && !isValidOddsOverride(item.Odds) {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("%s 赔率必须大于 1，填 0 表示关闭", item.PlayName))
		}
		item.Odds = roundOdds(item.Odds)
		for _, limit := range []float64{item.MinBet, item.MaxBet, item.MaxUserPeriod, item.MaxPeriodTotal} {
			if math.Abs(roundMoney(limit)-limit) > 0.0000001 {
				return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("%s 限额最多保留两位小数", item.PlayName))
			}
		}
		if item.Odds > 1 && item.MinBet <= 0 {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("%s 开启时单注最低必须大于 0", item.PlayName))
		}
		if item.MaxBet > 0 && item.MinBet > item.MaxBet {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("%s 单注最低不能高于单注最高", item.PlayName))
		}
		if item.MaxUserPeriod > 0 && item.MaxBet > item.MaxUserPeriod {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("%s 单注最高不能高于会员单期限额", item.PlayName))
		}
		if item.MaxPeriodTotal > 0 && item.MaxUserPeriod > item.MaxPeriodTotal {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("%s 会员单期限额不能高于全房单期限额", item.PlayName))
		}
		normalized = append(normalized, item)
	}
	return normalized, nil
}

func deleteLowerLevelOddsOverridesExcept(tx *gorm.DB, gameID string, playCodes []string) error {
	query := tx.Where("game_id = ?", gameID)
	if len(playCodes) > 0 {
		query = query.Where("play_code NOT IN ?", playCodes)
	}
	if err := query.Session(&gorm.Session{}).Delete(&odds.RoomPlayOdds{}).Error; err != nil {
		return err
	}
	return query.Session(&gorm.Session{}).Delete(&odds.UserPlayOdds{}).Error
}

func (s *OddsAdminService) Reset(gameID string, guard OddsMutationGuard) (*GameOddsLimits, error) {
	game, err := s.loadGame(gameID)
	if err != nil {
		return nil, err
	}
	if err := ensureGameRulesSupported(game); err != nil {
		return nil, err
	}
	profile, _ := rulesForGame(game)
	if err := validateOddsMutationGuard(profile.Version, guard.ExpectedRuleVersion, guard.ExpectedRevision); err != nil {
		return nil, err
	}
	var result *GameOddsLimits
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var locked lottery.Game
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", game.ID).Error; err != nil {
			return err
		}
		current, err := NewOddsAdminService(tx).readGameLimits(&locked)
		if err != nil {
			return err
		}
		if current.RuleVersion != guard.ExpectedRuleVersion || current.ConfigRevision != guard.ExpectedRevision {
			return apperrors.NewBusinessError("ODDS_CONFIGURATION_CONFLICT", "赔率配置已被其他操作更新，请刷新后重试")
		}
		if err := tx.Where("game_id = ?", game.ID).Delete(&odds.PlayLimit{}).Error; err != nil {
			return err
		}
		if err := tx.Where("game_id = ?", game.ID).Delete(&odds.RoomPlayOdds{}).Error; err != nil {
			return err
		}
		if err := tx.Where("game_id = ?", game.ID).Delete(&odds.UserPlayOdds{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&lottery.Game{}).Where("id = ?", game.ID).
			UpdateColumn("odds_config_revision", gorm.Expr("odds_config_revision + 1")).Error; err != nil {
			return err
		}
		locked.OddsConfigRevision++
		result, err = NewOddsAdminService(tx).readGameLimits(&locked)
		return err
	})
	if err != nil {
		if apperrors.IsBusinessError(err) {
			return nil, err
		}
		return nil, apperrors.NewSystemError("ODDS_RESET_FAILED", "清空赔率限额失败", err)
	}
	return result, nil
}

func (s *OddsAdminService) loadGame(gameID string) (*lottery.Game, error) {
	id := strings.TrimSpace(gameID)
	if id == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "游戏编号不能为空")
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

// odds writes are intentionally never initialized from code. The catalogue
// supplies structure only; administrators own every numeric quote.
