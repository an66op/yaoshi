package services

import (
	"backend/data/models/lottery"
	"backend/data/models/user"
	apperrors "backend/errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
)

// These outcomes are mutually exclusive and exhaustive in the current digit
// contracts. This check does not assume that an upstream draw is uniform, and
// it must not be reused for overlapping or differently versioned outcomes.
var frontThreeShapeCodes = [...]string{"leopard", "straight", "pair", "half_straight", "mixed"}

type OddsRiskWarning struct {
	Code      string   `json:"code"`
	Message   string   `json:"message"`
	PlayCodes []string `json:"play_codes"`
}

func isFrontThreeShape(code string) bool {
	for _, shape := range frontThreeShapeCodes {
		if code == shape {
			return true
		}
	}
	return false
}

func hasExclusiveFrontThreeShapes(profile gameRuleProfile) bool {
	return profile.Patterns && (profile.Version == "digits3-v2" || profile.Version == "digits5-v3")
}

// A complete set of decimal (principal-inclusive) odds offers a strictly
// profitable cover when sum(1/odds) < 1. Use exact decimal rationals at the same
// four-decimal precision as resolved odds, so a fair boundary is not rejected
// because of floating-point accumulation. Missing outcomes do not prove a
// complete cover; their ordinary ODDS_NOT_CONFIGURED checks still apply.
func frontThreeOddsRisks(gameID string, offers map[string]float64) []OddsRiskWarning {
	warnings := make([]OddsRiskWarning, 0)
	profile, ready := rulesForGame(&lottery.Game{ID: gameID})
	if !ready || !hasExclusiveFrontThreeShapes(profile) {
		return warnings
	}
	codes := append([]string(nil), frontThreeShapeCodes[:]...)
	for _, code := range codes {
		value, exists := offers[code]
		if exists && (math.IsNaN(value) || math.IsInf(value, 0) || roundOdds(value) <= 1) {
			return append(warnings, OddsRiskWarning{
				Code: "INVALID_SHAPE_ODDS", PlayCodes: codes,
				Message: "前三形态赔率含无效数值，请检查配置；对应新投注暂不受理，已有注单不受影响。",
			})
		}
	}
	total := new(big.Rat)
	for _, code := range codes {
		value, exists := offers[code]
		if !exists {
			return warnings
		}
		decimal, ok := new(big.Rat).SetString(strconv.FormatFloat(roundOdds(value), 'f', 4, 64))
		if !ok || decimal.Sign() <= 0 {
			return append(warnings, OddsRiskWarning{Code: "INVALID_SHAPE_ODDS", PlayCodes: codes, Message: "前三形态赔率格式异常，请检查配置。"})
		}
		total.Add(total, new(big.Rat).Inv(decimal))
	}
	if total.Cmp(big.NewRat(1, 1)) < 0 {
		warnings = append(warnings, OddsRiskWarning{
			Code: "SHAPE_COVERAGE_RISK", PlayCodes: codes,
			Message: "前三形态赔率存在覆盖套利风险。账户最终赔率仍为该组合时，新的形态投注将被拒绝；请调整并保存，其他玩法与已有注单不受影响。",
		})
	}
	return warnings
}

func playLimitOddsRisks(gameID string, items []PlayLimitItem) []OddsRiskWarning {
	offers := make(map[string]float64, len(items))
	for _, item := range items {
		// Catalogue placeholders deliberately expose missing persisted prices as
		// zero. They are unavailable, not malformed offers, and the ordinary
		// ODDS_NOT_CONFIGURED gate handles them at placement time.
		if !item.Configured {
			continue
		}
		offers[item.PlayCode] = item.Odds
	}
	return frontThreeOddsRisks(gameID, offers)
}

// Check the account's actual prices, not just the platform defaults. This is
// read-only and runs before any debit in both Place and PlaceBatch. It is never
// used to reprice or refuse settlement of previously accepted tickets.
func (s *TradingAdminService) checkFrontThreeOddsRisk(account user.User, gameID, playCode string, selectedOdds float64) error {
	profile, ready := rulesForGame(&lottery.Game{ID: gameID})
	if !ready || !hasExclusiveFrontThreeShapes(profile) || !isFrontThreeShape(playCode) {
		return nil
	}
	// One SQL statement is important under READ COMMITTED: separate reads of
	// old platform prices and newly deleted room overrides could manufacture
	// a safe combination that was never offered at any point in time.
	var quotes []struct {
		PlayCode                     string
		PlatformOdds                 float64
		PlatformExplicitlyConfigured bool
		PlatformRuleVersion          string
		PlatformConfigurationSource  string
		RoomOdds                     float64
		MemberOdds                   float64
		OddsMultiplier               float64
	}
	err := s.db.Raw(`
SELECT codes.play_code,
       COALESCE(platform_prices.odds, 0) AS platform_odds,
       COALESCE(platform_prices.explicitly_configured, false) AS platform_explicitly_configured,
       COALESCE(platform_prices.rule_version, '') AS platform_rule_version,
       COALESCE(platform_prices.configuration_source, '') AS platform_configuration_source,
       COALESCE(room_prices.odds, 0) AS room_odds,
       COALESCE(member_prices.odds, 0) AS member_odds,
       COALESCE(room_member.odds_multiplier, 1) AS odds_multiplier
FROM (VALUES (?::text), (?::text), (?::text), (?::text), (?::text)) AS codes(play_code)
LEFT JOIN lottery_play_limits AS platform_prices
  ON platform_prices.game_id = ? AND platform_prices.play_code = codes.play_code
LEFT JOIN room_play_odds AS room_prices
  ON room_prices.workspace_id = ? AND room_prices.workspace_id <> 0
 AND room_prices.game_id = ? AND room_prices.play_code = codes.play_code
LEFT JOIN user_play_odds AS member_prices
  ON member_prices.workspace_id = ? AND member_prices.user_id = ?
 AND member_prices.game_id = ? AND member_prices.play_code = codes.play_code
LEFT JOIN workspace_memberships AS room_member
  ON room_member.workspace_id = ? AND room_member.user_id = ?
 AND room_member.workspace_id <> 0 AND room_member.user_id <> 0 AND room_member.status = 1`,
		frontThreeShapeCodes[0], frontThreeShapeCodes[1], frontThreeShapeCodes[2], frontThreeShapeCodes[3], frontThreeShapeCodes[4],
		gameID, account.WorkspaceID, gameID, account.WorkspaceID, account.UserID, gameID, account.WorkspaceID, account.UserID,
	).Scan(&quotes).Error
	if err != nil {
		return err
	}
	if len(quotes) != len(frontThreeShapeCodes) {
		return apperrors.NewSystemError("ODDS_READ_FAILED", "读取前三形态赔率失败", fmt.Errorf("expected five price rows, got %d", len(quotes)))
	}
	offers := map[string]float64{}
	for _, quote := range quotes {
		platformOdds, roomOdds, memberOdds := quote.PlatformOdds, quote.RoomOdds, quote.MemberOdds
		if !isValidOddsOverride(platformOdds) || !quote.PlatformExplicitlyConfigured || quote.PlatformRuleVersion != profile.Version || quote.PlatformConfigurationSource != oddsSourceAdminSave {
			// Orphaned room/member overrides cannot resurrect a legacy platform
			// row that has never been confirmed under the upgraded contract.
			platformOdds, roomOdds, memberOdds = 0, 0, 0
		}
		value, _ := resolveEffectiveOdds(memberOdds, roomOdds, platformOdds, quote.OddsMultiplier)
		if value != 0 {
			offers[quote.PlayCode] = value
		}
	}
	// The selected price was already resolved for this order. Never evaluate a
	// later price in its place if a configuration changed between the reads.
	offers[playCode] = selectedOdds
	if len(frontThreeOddsRisks(gameID, offers)) > 0 {
		return apperrors.NewBusinessError("ODDS_RISK_UNSAFE", "当前前三形态赔率存在风险，暂不受理此玩法，请联系管理员检查")
	}
	return nil
}
