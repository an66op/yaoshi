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

type UserRebateConfig struct {
	Mode      string  `json:"mode"` // inherit / custom / off
	Rate      float64 `json:"rate"`
	Effective float64 `json:"effective"`
	Source    string  `json:"source"` // user / room / off
}

type UserOddsOverrideItem struct {
	PlayCode    string   `json:"play_code"`
	PlayName    string   `json:"play_name"`
	BaseOdds    float64  `json:"base_odds"`
	RoomOdds    float64  `json:"room_odds"`
	Override    *float64 `json:"override"`
	Effective   float64  `json:"effective"`
	HasOverride bool     `json:"has_override"`
}

type UserTradingConfig struct {
	UserID         uint64                 `json:"user_id"`
	Username       string                 `json:"username"`
	Fly            UserFlyConfig          `json:"fly"`
	Rebate         UserRebateConfig       `json:"rebate"`
	GameID         string                 `json:"game_id"`
	GameName       string                 `json:"game_name"`
	Odds           []UserOddsOverrideItem `json:"odds"`
	RoomFlyRate    float64                `json:"room_fly_rate"`
	RoomRebateRate float64                `json:"room_rebate_rate"`
}

type UpdateUserTradingInput struct {
	FlyMode    string  `json:"fly_mode"`
	FlyRate    float64 `json:"fly_rate"`
	RebateMode string  `json:"rebate_mode"`
	RebateRate float64 `json:"rebate_rate"`
	GameID     string  `json:"game_id"`
	Odds       []struct {
		PlayCode string   `json:"play_code"`
		Override *float64 `json:"override"`
	} `json:"odds"`
}

type RoomOddsOverrideItem struct {
	PlayCode    string   `json:"play_code"`
	PlayName    string   `json:"play_name"`
	BaseOdds    float64  `json:"base_odds"`
	Override    *float64 `json:"override"`
	Effective   float64  `json:"effective"`
	HasOverride bool     `json:"has_override"`
}

type RoomTradingConfig struct {
	AgentID    uint64                 `json:"agent_id"`
	RoomCode   string                 `json:"room_code"`
	RebateRate float64                `json:"rebate_rate"`
	GameID     string                 `json:"game_id"`
	GameName   string                 `json:"game_name"`
	Odds       []RoomOddsOverrideItem `json:"odds"`
}

type UpdateRoomTradingInput struct {
	RebateRate float64 `json:"rebate_rate"`
	GameID     string  `json:"game_id"`
	Odds       []struct {
		PlayCode string   `json:"play_code"`
		Override *float64 `json:"override"`
	} `json:"odds"`
}

type ResolvedTradeParams struct {
	Odds        float64
	FlyAmount   float64
	FlyRateUsed float64
	OddsSource  string // user / room / request / fallback
	FlySource   string // explicit / user / room / off
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
	roomRebateRate, _ := s.roomRebateRate(account)
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
	roomOverrides := map[string]float64{}
	if agentID := roomAgentID(account); agentID > 0 {
		var roomRows []odds.RoomPlayOdds
		_ = s.db.Where("agent_id = ? AND game_id = ?", agentID, gameID).Find(&roomRows).Error
		for _, row := range roomRows {
			roomOverrides[row.PlayCode] = row.Odds
		}
	}
	var rows []odds.UserPlayOdds
	_ = s.db.Where("user_id = ? AND game_id = ?", userID, gameID).Find(&rows).Error
	for _, row := range rows {
		overrides[row.PlayCode] = row.Odds
	}
	items := make([]UserOddsOverrideItem, 0, len(limits.Items))
	for _, item := range limits.Items {
		roomOdds := item.Odds
		if value, ok := roomOverrides[item.PlayCode]; ok {
			roomOdds = value
		}
		entry := UserOddsOverrideItem{PlayCode: item.PlayCode, PlayName: item.PlayName, BaseOdds: item.Odds, RoomOdds: roomOdds, Effective: roomOdds}
		if value, ok := overrides[item.PlayCode]; ok {
			v := value
			entry.Override = &v
			entry.Effective = value
			entry.HasOverride = true
		}
		items = append(items, entry)
	}
	mode := defaultString(account.FlyMode, "inherit")
	rebateMode := defaultString(account.RebateMode, "inherit")
	effectiveRebate, rebateSource := resolveRebate(account, roomRebateRate)
	return &UserTradingConfig{
		UserID: account.UserID, Username: account.Username,
		Fly:    UserFlyConfig{Mode: mode, Rate: account.FlyRate},
		Rebate: UserRebateConfig{Mode: rebateMode, Rate: account.RebateRate, Effective: effectiveRebate, Source: rebateSource},
		GameID: limits.GameID, GameName: limits.GameName, Odds: items, RoomFlyRate: roomRate, RoomRebateRate: roomRebateRate,
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
	rebateMode := defaultString(strings.TrimSpace(input.RebateMode), "inherit")
	if rebateMode != "inherit" && rebateMode != "custom" && rebateMode != "off" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "返水模式不正确")
	}
	if input.RebateRate < 0 || input.RebateRate > 100 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "返水比例需在 0-100 之间")
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
		account.RebateMode = rebateMode
		account.RebateRate = input.RebateRate
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

	// Odds: user override → room override → platform limit → request → fallback.
	var override odds.UserPlayOdds
	if err := s.db.Where("user_id = ? AND game_id = ? AND play_code = ?", userID, gameID, playCode).First(&override).Error; err == nil && override.Odds > 1 {
		result.Odds = override.Odds
		result.OddsSource = "user"
	} else {
		var room odds.RoomPlayOdds
		if agentID := roomAgentID(account); agentID > 0 {
			_ = s.db.Where("agent_id = ? AND game_id = ? AND play_code = ?", agentID, gameID, playCode).First(&room).Error
		}
		if room.Odds > 1 {
			result.Odds = room.Odds
			result.OddsSource = "room"
		} else {
			var platform odds.PlayLimit
			if err := s.db.Where("game_id = ? AND play_code = ?", gameID, playCode).First(&platform).Error; err == nil && platform.Odds > 1 {
				result.Odds = platform.Odds
				result.OddsSource = "platform"
			} else if requestOdds > 1 {
				result.Odds = requestOdds
				result.OddsSource = "request"
			} else {
				result.Odds = 1.993
				result.OddsSource = "fallback"
			}
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

func (s *TradingAdminService) GetRoom(agentID uint64, gameID string) (*RoomTradingConfig, error) {
	var agent user.User
	if err := s.db.Where("user_id = ? AND role = ?", agentID, "agent").First(&agent).Error; err != nil {
		return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "代理房间不存在")
	}
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		value, err := NewOddsAdminService(s.db).firstEnabledGameID()
		if err != nil {
			return nil, err
		}
		gameID = value
	}
	limits, err := NewOddsAdminService(s.db).Get(gameID)
	if err != nil {
		return nil, err
	}
	rows := map[string]float64{}
	var roomRows []odds.RoomPlayOdds
	_ = s.db.Where("agent_id = ? AND game_id = ?", agentID, gameID).Find(&roomRows).Error
	for _, row := range roomRows {
		rows[row.PlayCode] = row.Odds
	}
	items := make([]RoomOddsOverrideItem, 0, len(limits.Items))
	for _, item := range limits.Items {
		entry := RoomOddsOverrideItem{PlayCode: item.PlayCode, PlayName: item.PlayName, BaseOdds: item.Odds, Effective: item.Odds}
		if value, ok := rows[item.PlayCode]; ok {
			v := value
			entry.Override = &v
			entry.Effective = value
			entry.HasOverride = true
		}
		items = append(items, entry)
	}
	return &RoomTradingConfig{AgentID: agentID, RoomCode: agent.AgentRoomCode, RebateRate: clampPercent(agent.RoomRebateRate), GameID: limits.GameID, GameName: limits.GameName, Odds: items}, nil
}

func (s *TradingAdminService) UpdateRoom(agentID uint64, input UpdateRoomTradingInput) (*RoomTradingConfig, error) {
	if input.RebateRate < 0 || input.RebateRate > 100 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "房间返水比例需在 0-100 之间")
	}
	gameID := strings.TrimSpace(input.GameID)
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var agent user.User
		if err := tx.Where("user_id = ? AND role = ?", agentID, "agent").First(&agent).Error; err != nil {
			return apperrors.NewBusinessError("USER_NOT_FOUND", "代理房间不存在")
		}
		if err := tx.Model(&agent).Update("room_rebate_rate", input.RebateRate).Error; err != nil {
			return err
		}
		if gameID == "" {
			return nil
		}
		limits, err := NewOddsAdminService(tx).Get(gameID)
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
				if err := tx.Where("agent_id = ? AND game_id = ? AND play_code = ?", agentID, gameID, code).Delete(&odds.RoomPlayOdds{}).Error; err != nil {
					return err
				}
				continue
			}
			if *item.Override <= 1 {
				return apperrors.NewBusinessError("INVALID_REQUEST", "房间赔率必须大于 1")
			}
			row := odds.RoomPlayOdds{AgentID: agentID, GameID: gameID, PlayCode: code, Odds: *item.Override}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "agent_id"}, {Name: "game_id"}, {Name: "play_code"}}, DoUpdates: clause.AssignmentColumns([]string{"odds", "updated_at"})}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetRoom(agentID, gameID)
}

func (s *TradingAdminService) roomRebateRate(account user.User) (float64, error) {
	if account.Role == "agent" {
		return clampPercent(account.RoomRebateRate), nil
	}
	if account.ParentAgentID == nil {
		return 0, nil
	}
	var agent user.User
	if err := s.db.Select("room_rebate_rate").First(&agent, *account.ParentAgentID).Error; err != nil {
		return 0, err
	}
	return clampPercent(agent.RoomRebateRate), nil
}

func (s *TradingAdminService) ResolveRebateRate(userID uint64) (float64, string, error) {
	var account user.User
	if err := s.db.First(&account, userID).Error; err != nil {
		return 0, "", err
	}
	roomRate, _ := s.roomRebateRate(account)
	rate, source := resolveRebate(account, roomRate)
	return rate, source, nil
}

func resolveRebate(account user.User, roomRate float64) (float64, string) {
	switch defaultString(account.RebateMode, "inherit") {
	case "off":
		return 0, "off"
	case "custom":
		return clampPercent(account.RebateRate), "user"
	default:
		return clampPercent(roomRate), "room"
	}
}

func roomAgentID(account user.User) uint64 {
	if account.Role == "agent" {
		return account.UserID
	}
	if account.ParentAgentID != nil {
		return *account.ParentAgentID
	}
	return 0
}

func clampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
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
