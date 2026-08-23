package services

import (
	"backend/data/models/lottery"
	"backend/data/models/odds"
	apperrors "backend/errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OddsAdminService struct{ db *gorm.DB }

type PlayLimitItem struct {
	PlayCode       string  `json:"play_code"`
	PlayName       string  `json:"play_name"`
	Odds           float64 `json:"odds"`
	MinBet         float64 `json:"min_bet"`
	MaxBet         float64 `json:"max_bet"`
	MaxUserPeriod  float64 `json:"max_user_period"`
	MaxPeriodTotal float64 `json:"max_period_total"`
	SortOrder      int     `json:"sort_order"`
}

type GameOddsLimits struct {
	GameID   string          `json:"game_id"`
	GameName string          `json:"game_name"`
	Items    []PlayLimitItem `json:"items"`
}

type UpdateOddsLimitsInput struct {
	Items []PlayLimitItem `json:"items"`
}

type SyncOddsLimitsResult struct {
	GameCount   int      `json:"game_count"`
	SeededGames []string `json:"seeded_games"`
}

func NewOddsAdminService(db *gorm.DB) *OddsAdminService {
	return &OddsAdminService{db: db}
}

func (s *OddsAdminService) Get(gameID string) (*GameOddsLimits, error) {
	game, err := s.loadGame(gameID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureDefaults(game.ID); err != nil {
		return nil, err
	}
	var rows []odds.PlayLimit
	if err := s.db.Where("game_id = ?", game.ID).Order("sort_order asc, id asc").Find(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("ODDS_READ_FAILED", "读取赔率限额失败", err)
	}
	items := make([]PlayLimitItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, PlayLimitItem{
			PlayCode:       row.PlayCode,
			PlayName:       row.PlayName,
			Odds:           row.Odds,
			MinBet:         row.MinBet,
			MaxBet:         row.MaxBet,
			MaxUserPeriod:  row.MaxUserPeriod,
			MaxPeriodTotal: row.MaxPeriodTotal,
			SortOrder:      row.SortOrder,
		})
	}
	return &GameOddsLimits{GameID: game.ID, GameName: game.Name, Items: items}, nil
}

func (s *OddsAdminService) Update(gameID string, input UpdateOddsLimitsInput) (*GameOddsLimits, error) {
	game, err := s.loadGame(gameID)
	if err != nil {
		return nil, err
	}
	if len(input.Items) == 0 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "至少需要一条玩法配置")
	}
	seen := map[string]struct{}{}
	for i, item := range input.Items {
		code := strings.TrimSpace(item.PlayCode)
		name := strings.TrimSpace(item.PlayName)
		if code == "" || name == "" {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "玩法编号和名称不能为空")
		}
		if _, ok := seen[code]; ok {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("玩法编号重复: %s", code))
		}
		seen[code] = struct{}{}
		if item.Odds <= 1 {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("%s 赔率必须大于 1", name))
		}
		if item.MinBet < 0 || item.MaxBet < 0 || item.MaxUserPeriod < 0 || item.MaxPeriodTotal < 0 {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("%s 限额不能为负数", name))
		}
		if item.MaxBet > 0 && item.MinBet > item.MaxBet {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("%s 单注最低不能高于单注最高", name))
		}
		input.Items[i].PlayCode = code
		input.Items[i].PlayName = name
		if input.Items[i].SortOrder == 0 {
			input.Items[i].SortOrder = i
		}
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("game_id = ?", game.ID).Delete(&odds.PlayLimit{}).Error; err != nil {
			return err
		}
		rows := make([]odds.PlayLimit, 0, len(input.Items))
		for _, item := range input.Items {
			rows = append(rows, odds.PlayLimit{
				GameID:         game.ID,
				PlayCode:       item.PlayCode,
				PlayName:       item.PlayName,
				Odds:           item.Odds,
				MinBet:         item.MinBet,
				MaxBet:         item.MaxBet,
				MaxUserPeriod:  item.MaxUserPeriod,
				MaxPeriodTotal: item.MaxPeriodTotal,
				SortOrder:      item.SortOrder,
			})
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		return nil, apperrors.NewSystemError("ODDS_SAVE_FAILED", "保存赔率限额失败", err)
	}
	return s.Get(game.ID)
}

func (s *OddsAdminService) Reset(gameID string) (*GameOddsLimits, error) {
	game, err := s.loadGame(gameID)
	if err != nil {
		return nil, err
	}
	if err := s.db.Where("game_id = ?", game.ID).Delete(&odds.PlayLimit{}).Error; err != nil {
		return nil, apperrors.NewSystemError("ODDS_RESET_FAILED", "重置赔率限额失败", err)
	}
	if err := s.ensureDefaults(game.ID); err != nil {
		return nil, err
	}
	return s.Get(game.ID)
}

func (s *OddsAdminService) SyncAllGames() (*SyncOddsLimitsResult, error) {
	var games []lottery.Game
	if err := s.db.Order("sort_order asc, id asc").Find(&games).Error; err != nil {
		return nil, apperrors.NewSystemError("GAME_READ_FAILED", "读取游戏列表失败", err)
	}
	result := &SyncOddsLimitsResult{SeededGames: make([]string, 0)}
	for _, game := range games {
		var count int64
		if err := s.db.Model(&odds.PlayLimit{}).Where("game_id = ?", game.ID).Count(&count).Error; err != nil {
			return nil, err
		}
		if count > 0 {
			continue
		}
		if err := s.ensureDefaults(game.ID); err != nil {
			return nil, err
		}
		result.SeededGames = append(result.SeededGames, game.ID)
	}
	result.GameCount = len(games)
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

func (s *OddsAdminService) EnsureGameDefaults(gameID string) error {
	return s.ensureDefaults(gameID)
}

func (s *OddsAdminService) ensureDefaults(gameID string) error {
	var count int64
	if err := s.db.Model(&odds.PlayLimit{}).Where("game_id = ?", gameID).Count(&count).Error; err != nil {
		return apperrors.NewSystemError("ODDS_READ_FAILED", "读取赔率限额失败", err)
	}
	if count > 0 {
		return nil
	}
	rows := make([]odds.PlayLimit, 0, len(defaultPlays))
	for i, play := range defaultPlays {
		rows = append(rows, odds.PlayLimit{
			GameID:         gameID,
			PlayCode:       play.Code,
			PlayName:       play.Name,
			Odds:           play.Odds,
			MinBet:         1,
			MaxBet:         50000,
			MaxUserPeriod:  50000,
			MaxPeriodTotal: 100000,
			SortOrder:      i,
		})
	}
	return s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}
