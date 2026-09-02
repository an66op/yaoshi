package services

import (
	"backend/data/models/lottery"
	"backend/data/models/odds"
	"backend/data/models/settings"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"encoding/json"
	"math"
	"strings"
	"unicode"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TradingAdminService struct{ db *gorm.DB }

type UserFlyConfig struct {
	Mode string  `json:"mode"` // inherit / custom / off
	Rate float64 `json:"rate"`
}

// UserExternalFollowConfig is preparation metadata for a future connector.
// Status is intentionally server-owned: without an installed connector this
// service can only report not_connected and must never suggest an order was
// sent to, accepted by, or settled on an external platform.
type UserExternalFollowConfig struct {
	Status         string  `json:"status"`
	Capability     string  `json:"capability"`
	TargetPlatform string  `json:"target_platform"`
	TargetAccount  string  `json:"target_account"`
	EndpointLabel  string  `json:"endpoint_label"`
	SingleLimit    float64 `json:"single_limit"`
	DailyLimit     float64 `json:"daily_limit"`
	Remark         string  `json:"remark"`
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
	UserID         uint64                   `json:"user_id"`
	WorkspaceID    uint64                   `json:"workspace_id"`
	Username       string                   `json:"username"`
	OddsMultiplier float64                  `json:"odds_multiplier"`
	Fly            UserFlyConfig            `json:"fly"`
	ExternalFollow UserExternalFollowConfig `json:"external_follow"`
	Rebate         UserRebateConfig         `json:"rebate"`
	GameID         string                   `json:"game_id"`
	GameName       string                   `json:"game_name"`
	Odds           []UserOddsOverrideItem   `json:"odds"`
	RoomFlyRate    float64                  `json:"room_fly_rate"`
	RoomRebateRate float64                  `json:"room_rebate_rate"`
}

type UpdateUserTradingInput struct {
	OddsMultiplier *float64                       `json:"odds_multiplier"`
	FlyMode        string                         `json:"fly_mode"`
	FlyRate        float64                        `json:"fly_rate"`
	ExternalFollow *UpdateUserExternalFollowInput `json:"external_follow"`
	RebateMode     string                         `json:"rebate_mode"`
	RebateRate     float64                        `json:"rebate_rate"`
	GameID         string                         `json:"game_id"`
	Odds           []struct {
		PlayCode string   `json:"play_code"`
		Override *float64 `json:"override"`
	} `json:"odds"`
}

type UpdateUserExternalFollowInput struct {
	TargetPlatform string  `json:"target_platform"`
	TargetAccount  string  `json:"target_account"`
	EndpointLabel  string  `json:"endpoint_label"`
	SingleLimit    float64 `json:"single_limit"`
	DailyLimit     float64 `json:"daily_limit"`
	Remark         string  `json:"remark"`
}

type normalizedUserExternalFollow struct {
	targetPlatform   string
	targetAccount    string
	endpointLabel    string
	singleLimitCents int64
	dailyLimitCents  int64
	remark           string
}

const maxExternalFollowLimitCents int64 = 10_000_000_000

type RoomOddsOverrideItem struct {
	PlayCode    string   `json:"play_code"`
	PlayName    string   `json:"play_name"`
	BaseOdds    float64  `json:"base_odds"`
	Override    *float64 `json:"override"`
	Effective   float64  `json:"effective"`
	HasOverride bool     `json:"has_override"`
}

type RoomTradingConfig struct {
	WorkspaceID uint64                 `json:"workspace_id"`
	AgentID     uint64                 `json:"agent_id"` // legacy owner identifier
	RoomCode    string                 `json:"room_code"`
	RebateRate  float64                `json:"rebate_rate"`
	GameID      string                 `json:"game_id"`
	GameName    string                 `json:"game_name"`
	Odds        []RoomOddsOverrideItem `json:"odds"`
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
	PricingCode string // audited platform/room/member price row used for this quote
	FlyAmount   float64
	FlyRateUsed float64
	OddsSource  string // user / room / request / fallback
	FlySource   string // explicit / user / room / off
}

func NewTradingAdminService(db *gorm.DB) *TradingAdminService {
	return &TradingAdminService{db: db}
}

// GetForWorkspace keeps room operators inside their own membership boundary.
// The user row remains locked during the read so a concurrent room transfer
// cannot make the response switch to another room after validation.
func (s *TradingAdminService) GetForWorkspace(workspaceID, userID uint64, gameID string) (*UserTradingConfig, error) {
	var result *UserTradingConfig
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var account user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND workspace_id = ? AND role = ?", userID, workspaceID, "member").
			First(&account).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("MEMBER_SCOPE_MISMATCH", "会员不属于当前房间")
			}
			return err
		}
		value, err := NewTradingAdminService(tx).Get(userID, gameID)
		if err != nil {
			return err
		}
		result = value
		return nil
	})
	return result, err
}

// UpdateForWorkspace validates membership and writes under the same row lock.
// A member who is being transferred cannot accidentally receive settings from
// the previous room.
func (s *TradingAdminService) UpdateForWorkspace(workspaceID, userID uint64, input UpdateUserTradingInput) (*UserTradingConfig, error) {
	var result *UserTradingConfig
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var account user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND workspace_id = ? AND role = ?", userID, workspaceID, "member").
			First(&account).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("MEMBER_SCOPE_MISMATCH", "会员不属于当前房间")
			}
			return err
		}
		value, err := NewTradingAdminService(tx).Update(userID, input)
		if err != nil {
			return err
		}
		result = value
		return nil
	})
	return result, err
}

func (s *TradingAdminService) Get(userID uint64, gameID string) (*UserTradingConfig, error) {
	var account user.User
	if err := memberTradingAccountQuery(s.db, userID).First(&account).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
		}
		return nil, err
	}
	roomRate, err := s.roomFlyRate(account)
	if err != nil {
		return nil, err
	}
	roomRebateRate, err := s.roomRebateRate(account)
	if err != nil {
		return nil, err
	}
	oddsMultiplier, err := s.membershipOddsMultiplier(account.WorkspaceID, account.UserID)
	if err != nil {
		return nil, err
	}
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
	if account.WorkspaceID > 0 {
		var roomRows []odds.RoomPlayOdds
		if err := s.db.Where("workspace_id = ? AND game_id = ?", account.WorkspaceID, gameID).Find(&roomRows).Error; err != nil {
			return nil, err
		}
		for _, row := range roomRows {
			roomOverrides[row.PlayCode] = row.Odds
		}
	}
	var rows []odds.UserPlayOdds
	if err := s.db.Where("workspace_id = ? AND user_id = ? AND game_id = ?", account.WorkspaceID, userID, gameID).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		overrides[row.PlayCode] = row.Odds
	}
	items := make([]UserOddsOverrideItem, 0, len(limits.Items))
	for _, item := range limits.Items {
		roomOdds := item.Odds
		if value, ok := roomOverrides[item.PlayCode]; ok {
			roomOdds = value
		}
		entry := UserOddsOverrideItem{PlayCode: item.PlayCode, PlayName: item.PlayName, BaseOdds: item.Odds, RoomOdds: roomOdds, Effective: applyOddsMultiplier(roomOdds, oddsMultiplier)}
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
		UserID: account.UserID, WorkspaceID: account.WorkspaceID, Username: account.Username,
		OddsMultiplier: oddsMultiplier,
		Fly:            UserFlyConfig{Mode: mode, Rate: account.FlyRate},
		ExternalFollow: externalFollowConfig(account),
		Rebate:         UserRebateConfig{Mode: rebateMode, Rate: account.RebateRate, Effective: effectiveRebate, Source: rebateSource},
		GameID:         limits.GameID, GameName: limits.GameName, Odds: items, RoomFlyRate: roomRate, RoomRebateRate: roomRebateRate,
	}, nil
}

func externalFollowConfig(account user.User) UserExternalFollowConfig {
	return UserExternalFollowConfig{
		Status:         "not_connected",
		Capability:     "configuration_only",
		TargetPlatform: account.FlyTargetPlatform,
		TargetAccount:  account.FlyTargetAccount,
		EndpointLabel:  account.FlyEndpointLabel,
		SingleLimit:    centsToAmount(account.FlySingleLimitCents),
		DailyLimit:     centsToAmount(account.FlyDailyLimitCents),
		Remark:         account.FlyConnectionRemark,
	}
}

func normalizeUserExternalFollow(input UpdateUserExternalFollowInput) (normalizedUserExternalFollow, error) {
	var result normalizedUserExternalFollow
	var err error
	if result.targetPlatform, err = normalizedFlyText(input.TargetPlatform, "目标平台", 80); err != nil {
		return result, err
	}
	if result.targetAccount, err = normalizedFlyText(input.TargetAccount, "目标账号标识", 120); err != nil {
		return result, err
	}
	if result.endpointLabel, err = normalizedFlyText(input.EndpointLabel, "端点标识", 160); err != nil {
		return result, err
	}
	if result.remark, err = normalizedFlyText(input.Remark, "连接备注", 500); err != nil {
		return result, err
	}
	if result.singleLimitCents, err = nonNegativeMoneyCents(input.SingleLimit, "单笔上限"); err != nil {
		return result, err
	}
	if result.dailyLimitCents, err = nonNegativeMoneyCents(input.DailyLimit, "每日上限"); err != nil {
		return result, err
	}
	if result.singleLimitCents > maxExternalFollowLimitCents || result.dailyLimitCents > maxExternalFollowLimitCents {
		return result, apperrors.NewBusinessError("INVALID_REQUEST", "连接预配置上限不能超过 100000000")
	}
	if result.singleLimitCents > 0 && result.dailyLimitCents > 0 && result.dailyLimitCents < result.singleLimitCents {
		return result, apperrors.NewBusinessError("INVALID_REQUEST", "每日上限不能小于单笔上限")
	}
	return result, nil
}

func normalizedFlyText(value, label string, limit int) (string, error) {
	value = strings.TrimSpace(value)
	if len([]rune(value)) > limit {
		return "", apperrors.NewBusinessError("INVALID_REQUEST", label+"过长")
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return "", apperrors.NewBusinessError("INVALID_REQUEST", label+"不能包含控制字符")
		}
	}
	return value, nil
}

func (s *TradingAdminService) Update(userID uint64, input UpdateUserTradingInput) (*UserTradingConfig, error) {
	if input.OddsMultiplier != nil {
		if err := validateOddsMultiplier(*input.OddsMultiplier); err != nil {
			return nil, err
		}
	}
	mode := strings.TrimSpace(input.FlyMode)
	if mode == "" {
		mode = "inherit"
	}
	if mode != "inherit" && mode != "custom" && mode != "off" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "飞单模式不正确")
	}
	if math.IsNaN(input.FlyRate) || math.IsInf(input.FlyRate, 0) || input.FlyRate < 0 || input.FlyRate > 100 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "飞单比例需在 0-100 之间")
	}
	rebateMode := defaultString(strings.TrimSpace(input.RebateMode), "inherit")
	if rebateMode != "inherit" && rebateMode != "custom" && rebateMode != "off" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "返水模式不正确")
	}
	if math.IsNaN(input.RebateRate) || math.IsInf(input.RebateRate, 0) || input.RebateRate < 0 || input.RebateRate > 100 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "返水比例需在 0-100 之间")
	}
	var external *normalizedUserExternalFollow
	if input.ExternalFollow != nil {
		normalized, normalizeErr := normalizeUserExternalFollow(*input.ExternalFollow)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		external = &normalized
	}
	gameID := strings.TrimSpace(input.GameID)
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var account user.User
		if err := memberTradingAccountQuery(tx, userID).First(&account).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
			}
			return err
		}
		account.FlyMode = mode
		account.FlyRate = input.FlyRate
		if external != nil {
			account.FlyTargetPlatform = external.targetPlatform
			account.FlyTargetAccount = external.targetAccount
			account.FlyEndpointLabel = external.endpointLabel
			account.FlySingleLimitCents = external.singleLimitCents
			account.FlyDailyLimitCents = external.dailyLimitCents
			account.FlyConnectionRemark = external.remark
		}
		account.RebateMode = rebateMode
		account.RebateRate = input.RebateRate
		if err := tx.Save(&account).Error; err != nil {
			return err
		}
		if input.OddsMultiplier != nil {
			result := tx.Model(&workspacemodel.Membership{}).
				Where("workspace_id = ? AND user_id = ? AND status = ?", account.WorkspaceID, account.UserID, 1).
				Update("odds_multiplier", normalizeOddsMultiplier(*input.OddsMultiplier))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return apperrors.NewBusinessError("MEMBERSHIP_NOT_FOUND", "用户当前不属于该房间")
			}
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
				return apperrors.NewBusinessError("INVALID_REQUEST", "包含当前游戏不存在的玩法赔率")
			}
			if item.Override == nil {
				if err := tx.Where("workspace_id = ? AND user_id = ? AND game_id = ? AND play_code = ?", account.WorkspaceID, userID, gameID, code).
					Delete(&odds.UserPlayOdds{}).Error; err != nil {
					return err
				}
				continue
			}
			if !isValidOddsOverride(*item.Override) {
				return apperrors.NewBusinessError("INVALID_REQUEST", "单独赔率必须大于 1")
			}
			row := odds.UserPlayOdds{WorkspaceID: account.WorkspaceID, UserID: userID, GameID: gameID, PlayCode: code, Odds: *item.Override}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "workspace_id"}, {Name: "user_id"}, {Name: "game_id"}, {Name: "play_code"}},
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

func memberTradingAccountQuery(db *gorm.DB, userID uint64) *gorm.DB {
	return db.Where("user_id = ? AND role = ?", userID, "member")
}

func (s *TradingAdminService) Resolve(userID uint64, gameID, playCode string, amount, requestOdds, requestFly float64) (*ResolvedTradeParams, error) {
	var account user.User
	if err := s.db.First(&account, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
		}
		return nil, err
	}
	return s.resolveForAccount(account, gameID, playCode, "", amount, requestOdds, requestFly)
}

// ResolveForAccount resolves all room-scoped trading terms from an account
// that the caller already loaded (and, for betting, locked) in the current
// transaction. This prevents a concurrent room transfer from mixing the old
// room's odds with a bet persisted in the new workspace.
func (s *TradingAdminService) ResolveForAccount(account user.User, gameID, playCode string, amount, requestOdds, requestFly float64) (*ResolvedTradeParams, error) {
	return s.resolveForAccount(account, gameID, playCode, "", amount, requestOdds, requestFly)
}

// ResolveForAccountSelection is the placement boundary for markets whose
// price depends on the concrete selection while their persisted bet play code
// remains stable.  Today that applies only to Bingo Racing A's crown sum.
func (s *TradingAdminService) ResolveForAccountSelection(account user.User, gameID, playCode, selection string, amount, requestOdds, requestFly float64) (*ResolvedTradeParams, error) {
	return s.resolveForAccount(account, gameID, playCode, selection, amount, requestOdds, requestFly)
}

func (s *TradingAdminService) resolveForAccount(account user.User, gameID, playCode, selection string, amount, requestOdds, requestFly float64) (*ResolvedTradeParams, error) {
	playCode = defaultString(strings.TrimSpace(playCode), "ball_1_5")
	pricingCode, pricingErr := oddsPricingCode(gameID, playCode, selection)
	if pricingErr != nil {
		return nil, pricingErr
	}
	result := &ResolvedTradeParams{PricingCode: pricingCode}
	profile, rulesReady := rulesForGame(&lottery.Game{ID: strings.TrimSpace(gameID)})

	// Odds are always server-owned. A valid platform PlayLimit is the market's
	// enablement record; orphaned room/member overrides must not resurrect a
	// play which the platform removed. Once enabled, precedence remains exact
	// user override → room override → platform price. requestOdds is retained
	// only for API compatibility and must never become authoritative.
	_ = requestOdds
	multiplier, err := s.membershipOddsMultiplier(account.WorkspaceID, account.UserID)
	if err != nil {
		return nil, err
	}
	var override odds.UserPlayOdds
	overrideErr := s.db.Where("workspace_id = ? AND user_id = ? AND game_id = ? AND play_code = ?", account.WorkspaceID, account.UserID, gameID, pricingCode).First(&override).Error
	if overrideErr != nil && overrideErr != gorm.ErrRecordNotFound {
		return nil, overrideErr
	}
	var platform odds.PlayLimit
	platformErr := s.db.Where("game_id = ? AND play_code = ?", gameID, pricingCode).First(&platform).Error
	if platformErr != nil && platformErr != gorm.ErrRecordNotFound {
		return nil, platformErr
	}
	if platformErr == gorm.ErrRecordNotFound || platform.Odds <= 1 || !rulesReady || !platform.ExplicitlyConfigured || strings.TrimSpace(platform.RuleVersion) != profile.Version {
		return nil, apperrors.NewBusinessError("ODDS_NOT_CONFIGURED", "当前玩法赔率尚未配置，请联系房间管理员")
	}
	if overrideErr == nil && override.Odds > 1 {
		result.Odds, result.OddsSource = resolveEffectiveOdds(override.Odds, 0, 0, multiplier)
	} else {
		var room odds.RoomPlayOdds
		if account.WorkspaceID > 0 {
			roomErr := s.db.Where("workspace_id = ? AND game_id = ? AND play_code = ?", account.WorkspaceID, gameID, pricingCode).First(&room).Error
			if roomErr != nil && roomErr != gorm.ErrRecordNotFound {
				return nil, roomErr
			}
		}
		if room.Odds > 1 {
			result.Odds, result.OddsSource = resolveEffectiveOdds(0, room.Odds, 0, multiplier)
		} else {
			result.Odds, result.OddsSource = resolveEffectiveOdds(0, 0, platform.Odds, multiplier)
		}
	}

	if err := s.checkFrontThreeOddsRisk(account, gameID, playCode, result.Odds); err != nil {
		return nil, err
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
		rate, err := s.roomFlyRate(account)
		if err != nil {
			return nil, err
		}
		result.FlyRateUsed = rate
		result.FlyAmount = roundMoney(amount * rate / 100)
		result.FlySource = "room"
	}
	return result, nil
}

func (s *TradingAdminService) membershipOddsMultiplier(workspaceID, userID uint64) (float64, error) {
	if workspaceID == 0 || userID == 0 {
		return 1, nil
	}
	var membership workspacemodel.Membership
	err := scopedMembershipOddsQuery(s.db, workspaceID, userID).First(&membership).Error
	if err == gorm.ErrRecordNotFound {
		// Legacy accounts created before workspace membership rows existed keep
		// the original behaviour until their membership is repaired.
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	return normalizeOddsMultiplier(membership.OddsMultiplier), nil
}

func scopedMembershipOddsQuery(db *gorm.DB, workspaceID, userID uint64) *gorm.DB {
	return db.Select("odds_multiplier").
		Where("workspace_id = ? AND user_id = ? AND status = ?", workspaceID, userID, 1)
}

func applyOddsMultiplier(value, multiplier float64) float64 {
	return roundOdds(value * normalizeOddsMultiplier(multiplier))
}

func roundOdds(value float64) float64 { return math.Round(value*10000) / 10000 }

// resolveEffectiveOdds encodes the public precedence rule in one place:
// exact user play odds > room odds × membership multiplier > platform odds ×
// membership multiplier. Exact per-play overrides are already authoritative
// and are never multiplied again.
func resolveEffectiveOdds(userOdds, roomOdds, platformOdds, multiplier float64) (float64, string) {
	if userOdds > 1 {
		return roundOdds(userOdds), "user"
	}
	multiplier = normalizeOddsMultiplier(multiplier)
	if roomOdds > 1 {
		if multiplier == 1 {
			return roundOdds(roomOdds), "room"
		}
		return applyOddsMultiplier(roomOdds, multiplier), "member_multiplier_room"
	}
	if platformOdds > 1 {
		if multiplier == 1 {
			return roundOdds(platformOdds), "platform"
		}
		return applyOddsMultiplier(platformOdds, multiplier), "member_multiplier_platform"
	}
	return 0, ""
}

func (s *TradingAdminService) GetRoom(agentID uint64, gameID string) (*RoomTradingConfig, error) {
	var agent user.User
	if err := s.db.Where("user_id = ? AND role = ?", agentID, "agent").First(&agent).Error; err != nil {
		return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "代理房间不存在")
	}
	return s.GetRoomForWorkspace(agent.WorkspaceID, gameID)
}

// GetRoomForWorkspace is the canonical room configuration lookup for both
// tenant-direct and agent rooms. Parent IDs are never accepted as scope.
func (s *TradingAdminService) GetRoomForWorkspace(workspaceID uint64, gameID string) (*RoomTradingConfig, error) {
	var workspace workspacemodel.Workspace
	if err := s.db.Where("id = ? AND type IN ?", workspaceID, []string{workspacemodel.TypeTenant, workspacemodel.TypeAgent}).First(&workspace).Error; err != nil {
		return nil, apperrors.NewBusinessError("WORKSPACE_NOT_FOUND", "房间不存在")
	}
	var owner user.User
	if err := s.db.First(&owner, workspace.OwnerUserID).Error; err != nil {
		return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "房间所有者不存在")
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
	if err := s.db.Where("workspace_id = ? AND game_id = ?", workspace.ID, gameID).Find(&roomRows).Error; err != nil {
		return nil, err
	}
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
	return &RoomTradingConfig{WorkspaceID: workspace.ID, AgentID: workspace.OwnerUserID, RoomCode: workspace.RoomCode, RebateRate: clampPercent(owner.RoomRebateRate), GameID: limits.GameID, GameName: limits.GameName, Odds: items}, nil
}

func (s *TradingAdminService) UpdateRoom(agentID uint64, input UpdateRoomTradingInput) (*RoomTradingConfig, error) {
	var agent user.User
	if err := s.db.Where("user_id = ? AND role = ?", agentID, "agent").First(&agent).Error; err != nil {
		return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "代理房间不存在")
	}
	return s.UpdateRoomForWorkspace(agent.WorkspaceID, input)
}

func (s *TradingAdminService) UpdateRoomForWorkspace(workspaceID uint64, input UpdateRoomTradingInput) (*RoomTradingConfig, error) {
	if !isFinitePercent(input.RebateRate) {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "房间返水比例需在 0-100 之间")
	}
	gameID := strings.TrimSpace(input.GameID)
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var workspace workspacemodel.Workspace
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND type IN ?", workspaceID, []string{workspacemodel.TypeTenant, workspacemodel.TypeAgent}).First(&workspace).Error; err != nil {
			return apperrors.NewBusinessError("WORKSPACE_NOT_FOUND", "房间不存在")
		}
		var owner user.User
		if err := tx.First(&owner, workspace.OwnerUserID).Error; err != nil {
			return apperrors.NewBusinessError("USER_NOT_FOUND", "房间所有者不存在")
		}
		if err := tx.Model(&owner).Update("room_rebate_rate", input.RebateRate).Error; err != nil {
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
				return apperrors.NewBusinessError("INVALID_REQUEST", "包含当前游戏不存在的玩法赔率")
			}
			if item.Override == nil {
				if err := tx.Where("workspace_id = ? AND game_id = ? AND play_code = ?", workspace.ID, gameID, code).Delete(&odds.RoomPlayOdds{}).Error; err != nil {
					return err
				}
				continue
			}
			if !isValidOddsOverride(*item.Override) {
				return apperrors.NewBusinessError("INVALID_REQUEST", "房间赔率必须大于 1")
			}
			row := odds.RoomPlayOdds{WorkspaceID: workspace.ID, AgentID: workspace.OwnerUserID, GameID: gameID, PlayCode: code, Odds: *item.Override}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "workspace_id"}, {Name: "game_id"}, {Name: "play_code"}}, DoUpdates: clause.AssignmentColumns([]string{"agent_id", "odds", "updated_at"})}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetRoomForWorkspace(workspaceID, gameID)
}

func isFinitePercent(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 100
}

func isValidOddsOverride(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 1
}

func (s *TradingAdminService) roomRebateRate(account user.User) (float64, error) {
	if account.WorkspaceID == 0 {
		return 0, nil
	}
	var workspace workspacemodel.Workspace
	if err := s.db.Select("owner_user_id").First(&workspace, account.WorkspaceID).Error; err != nil {
		return 0, err
	}
	var owner user.User
	if err := s.db.Select("room_rebate_rate").First(&owner, workspace.OwnerUserID).Error; err != nil {
		return 0, err
	}
	return clampPercent(owner.RoomRebateRate), nil
}

func (s *TradingAdminService) ResolveRebateRate(userID uint64) (float64, string, error) {
	var account user.User
	if err := s.db.First(&account, userID).Error; err != nil {
		return 0, "", err
	}
	roomRate, err := s.roomRebateRate(account)
	if err != nil {
		return 0, "", err
	}
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

func clampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func (s *TradingAdminService) roomFlyRate(account user.User) (float64, error) {
	if account.WorkspaceID == 0 {
		return 0, nil
	}
	var row settings.SystemConfig
	if err := s.db.Where("workspace_id = ?", account.WorkspaceID).First(&row).Error; err != nil {
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
