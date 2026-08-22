package services

import (
	"backend/data/models/odds"
	"backend/data/models/settings"
	"backend/data/models/user"
	apperrors "backend/errors"
	"encoding/json"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TradingAdminService struct{ db *gorm.DB }

type UserFlyConfig struct {
	Mode string  `json:"mode"` // inherit / custom / off
	Rate float64 `json:"rate"`
}

type UserOddsOverrideItem struct {
	PlayCode   string  `json:"play_code"`
	PlayName   string  `json:"play_name"`
	RoomOdds   float64 `json:"room_odds"`
	Override   *float64 `json:"override"`
	Effective  float64 `json:"effective"`
	HasOverride bool   `json:"has_override"`
}

type UserTradingConfig struct {
	UserID   uint64                 `json:"user_id"`
	Username string                 `json:"username"`
	Fly      UserFlyConfig          `json:"fly"`
	GameID   string                 `json:"game_id"`
	GameName string                 `json:"game_name"`
	Odds     []UserOddsOverrideItem `json:"odds"`
	RoomFlyRate float64             `json:"room_fly_rate"`
}

type UpdateUserTradingInput struct {
	FlyMode string `json:"fly_mode"`
	FlyRate float64 `json:"fly_rate"`
	GameID  string `json:"game_id"`
	Odds    []struct {
		PlayCode string   `json:"play_code"`
		Override *float64 `json:"override"`
	} `json:"odds"`
}

type ResolvedTradeParams struct {
	Odds         float64
	FlyAmount    float64
	FlyRateUsed  float64
	OddsSource   string // user / room / request / fallback
	FlySource    string // explicit / user / room / off
}

func NewTradingAdminService(db *gorm.DB) *TradingAdminService {
	return &TradingAdminService{db: db}
}

func (s *TradingAdminService) Get(userID uint64, gameID string) (*UserTradingConfig, error) {
	var account user.User
	if err := s.db.First(&account, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
		}
		return nil, err
	}
	roomRate, _ := s.roomFlyRate()
	gameID = strings.TrimSpace(gameID)
	oddsSvc := NewOddsAdminService(s.db)
	if gameID == "" {
		game, err := oddsSvc.firstEnabledGameID()
		if err != nil {
			return nil, err
		}
		gameID = game
	}
	limits, err := oddsSvc.Get(gameID)
	if err != nil {
		return nil, err
	}
	overrides := map[string]float64{}
	var rows []odds.UserPlayOdds
	_ = s.db.Where("user_id = ? AND game_id = ?", userID, gameID).Find(&rows).Error
	for _, row := range rows {
		overrides[row.PlayCode] = row.Odds
	}
	items := make([]UserOddsOverrideItem, 0, len(limits.Items))
	for _, item := range limits.Items {
		entry := UserOddsOverrideItem{
			PlayCode: item.PlayCode, PlayName: item.PlayName, RoomOdds: item.Odds, Effective: item.Odds,
		}
		if value, ok := overrides[item.PlayCode]; ok {
			v := value
			entry.Override = &v
			entry.Effective = value
			entry.HasOverride = true
		}
		items = append(items, entry)
	}
	mode := defaultString(account.FlyMode, "inherit")
	return &UserTradingConfig{
		UserID: account.UserID, Username: account.Username,
		Fly: UserFlyConfig{Mode: mode, Rate: account.FlyRate},
		GameID: limits.GameID, GameName: limits.GameName, Odds: items, RoomFlyRate: roomRate,
	}, nil
}

func (s *TradingAdminService) Update(userID uint64, input UpdateUserTradingInput) (*UserTradingConfig, error) {
	mode := strings.TrimSpace(input.FlyMode)
	if mode == "" {
		mode = "inherit"
	}
	if mode != "inherit" && mode != "custom" && mode != "off" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "飞单模式不正确")
	}
	if input.FlyRate < 0 || input.FlyRate > 100 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "飞单比例需在 0-100 之间")
	}
	gameID := strings.TrimSpace(input.GameID)
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var account user.User
		if err := tx.First(&account, userID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
			}
			return err
		}
		account.FlyMode = mode
		account.FlyRate = input.FlyRate
		if err := tx.Save(&account).Error; err != nil {
			return err
		}
		if gameID == "" {
			return nil
		}
		oddsSvc := NewOddsAdminService(tx)
		limits, err := oddsSvc.Get(gameID)
		if err != nil {
			return err
		}
		valid := map[string]bool{}
		for _, item := range limits.Items {
			valid[item.PlayCode] = true
		}
		for _, item := range input.Odds {
			code := strings.TrimSpace(item.PlayCode)
			if !valid[code] {
				continue
			}
			if item.Override == nil {
				if err := tx.Where("user_id = ? AND game_id = ? AND play_code = ?", userID, gameID, code).
					Delete(&odds.UserPlayOdds{}).Error; err != nil {
					return err
				}
				continue
			}
			if *item.Override <= 1 {
				return apperrors.NewBusinessError("INVALID_REQUEST", "单独赔率必须大于 1")
			}
			row := odds.UserPlayOdds{UserID: userID, GameID: gameID, PlayCode: code, Odds: *item.Override}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "user_id"}, {Name: "game_id"}, {Name: "play_code"}},
				DoUpdates: clause.AssignmentColumns([]string{"odds", "updated_at"}),
			}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.Get(userID, gameID)
}

func (s *TradingAdminService) Resolve(userID uint64, gameID, playCode string, amount, requestOdds, requestFly float64) (*ResolvedTradeParams, error) {
	var account user.User
	if err := s.db.First(&account, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
		}
		return nil, err
	}
	playCode = defaultString(strings.TrimSpace(playCode), "ball_1_5")
	result := &ResolvedTradeParams{}

	// Odds: user override → room play limit → request → fallback
	var override odds.UserPlayOdds
	if err := s.db.Where("user_id = ? AND game_id = ? AND play_code = ?", userID, gameID, playCode).First(&override).Error; err == nil && override.Odds > 1 {
		result.Odds = override.Odds
		result.OddsSource = "user"
	} else {
		var room odds.PlayLimit
		if err := s.db.Where("game_id = ? AND play_code = ?", gameID, playCode).First(&room).Error; err == nil && room.Odds > 1 {
			result.Odds = room.Odds
			result.OddsSource = "room"
		} else if requestOdds > 1 {
			result.Odds = requestOdds
			result.OddsSource = "request"
		} else {
			result.Odds = 1.993
			result.OddsSource = "fallback"
		}
	}

	// Fly: explicit amount ≥ 0 with request flag — if requestFly > 0 use it; if caller passes negative sentinel means auto.
	// Convention: requestFly < 0 → auto resolve; requestFly >= 0 → explicit (including 0).
	if requestFly >= 0 {
		result.FlyAmount = requestFly
		result.FlySource = "explicit"
		result.FlyRateUsed = 0
		if amount > 0 && requestFly > 0 {
			result.FlyRateUsed = requestFly / amount * 100
		}
		return result, nil
	}

	mode := defaultString(account.FlyMode, "inherit")
	switch mode {
	case "off":
		result.FlyAmount = 0
		result.FlySource = "off"
	case "custom":
		result.FlyRateUsed = account.FlyRate
		result.FlyAmount = roundMoney(amount * account.FlyRate / 100)
		result.FlySource = "user"
	default:
		rate, _ := s.roomFlyRate()
		result.FlyRateUsed = rate
		result.FlyAmount = roundMoney(amount * rate / 100)
		result.FlySource = "room"
	}
	return result, nil
}

func (s *TradingAdminService) roomFlyRate() (float64, error) {
	var row settings.SystemConfig
	if err := s.db.First(&row, 1).Error; err != nil {
		return 0, err
	}
	raw := defaultJSON(row.GameSettingsJSON, "{}")
	var game struct {
		DefaultFlyRate float64 `json:"default_fly_rate"`
	}
	_ = json.Unmarshal([]byte(raw), &game)
	if game.DefaultFlyRate < 0 {
		return 0, nil
	}
	if game.DefaultFlyRate > 100 {
		return 100, nil
	}
	return game.DefaultFlyRate, nil
}

func (s *OddsAdminService) firstEnabledGameID() (string, error) {
	var game struct{ ID string }
	err := s.db.Table("lottery_games").Select("id").Where("enabled = ?", true).Order("id asc").Limit(1).Scan(&game).Error
	if err != nil || game.ID == "" {
		err = s.db.Table("lottery_games").Select("id").Order("id asc").Limit(1).Scan(&game).Error
	}
	if err != nil || game.ID == "" {
		return "", apperrors.NewBusinessError("NOT_FOUND", "暂无可用彩种")
	}
	return game.ID, nil
}

func clampFlyCents(amountCents, flyCents int64) int64 {
	if flyCents < 0 {
		return 0
	}
	if flyCents > amountCents {
		return amountCents
	}
	return flyCents
}
