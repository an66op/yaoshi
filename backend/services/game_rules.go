package services

import (
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"fmt"
	"strconv"
	"strings"
)

const gameRulesUnavailableMessage = "该彩种尚未配置完整玩法，暂不受理投注"

// A profile is an explicit, versioned contract, not an inference from a game
// name, category or the length of an upstream result. Versions are stored on
// new bets; changing a profile requires a new version, never editing old terms.
type gameRuleProfile struct {
	Version         string
	BallCount       int
	MinNumber       int
	MaxNumber       int
	SumBigFrom      int
	PositionBigFrom int
	Racing          bool
	Patterns        bool
	// SumMarket is explicit: five-ball games do not include the local
	// total/total-tail extension used by three-ball games.
	SumMarket            bool
	ThreeShapeWindows    bool
	FirstLastDragonTiger bool
	DragonTigerTie       bool
	Unique               bool
	MarkSix              bool
	// PC28 is the explicitly bound Canada/PC 28 variant (1, 2 or 3). It is
	// never inferred from a three-number draw because official 3D games use a
	// different product and settlement contract.
	PC28 int
}

func rulesForVersion(version string) (gameRuleProfile, bool) {
	switch version {
	case "racing-v2":
		return gameRuleProfile{Version: version, BallCount: 10, MinNumber: 1, MaxNumber: 10,
			SumBigFrom: 12, PositionBigFrom: 6, Racing: true, SumMarket: true, Unique: true}, true
	case "digits5-v3":
		return gameRuleProfile{Version: version, BallCount: 5, MinNumber: 0, MaxNumber: 9,
			SumBigFrom: 23, PositionBigFrom: 5, Patterns: true, ThreeShapeWindows: true,
			FirstLastDragonTiger: true, DragonTigerTie: true}, true
	case "digits3-v2":
		return gameRuleProfile{Version: version, BallCount: 3, MinNumber: 0, MaxNumber: 9,
			SumBigFrom: 14, PositionBigFrom: 5, Patterns: true, SumMarket: true}, true
	case pc28RuleV1, pc28RuleV2, pc28RuleV3:
		return gameRuleProfile{Version: version, BallCount: 3, MinNumber: 0, MaxNumber: 9,
			SumBigFrom: 14, PositionBigFrom: 5, Patterns: true, PC28: pc28RuleVariant(version)}, true
	case markSixRuleVersion:
		return gameRuleProfile{Version: version, BallCount: 7, MinNumber: 1, MaxNumber: 49,
			Unique: true, MarkSix: true}, true
	default:
		return gameRuleProfile{}, false
	}
}

func rulesForGame(game *lottery.Game) (gameRuleProfile, bool) {
	if game == nil {
		return gameRuleProfile{}, false
	}
	switch game.ID {
	case "speed-racing", "speed-fly", "sg-fly", "fly-racing", "au-lucky-10", "bingo-racing-a":
		return rulesForVersion("racing-v2")
	case "speed-ssc", "sg-ssc", "au-lucky-5", "bingo-ssc-1":
		return rulesForVersion("digits5-v3")
	case "official-fc3d", "official-pl3":
		// Defining this existing digit contract does not classify or enable the
		// official games. Room/platform/category availability still applies.
		return rulesForVersion("digits3-v2")
	case "bingo-mark-six":
		return rulesForVersion(markSixRuleVersion)
	case "pc-canada":
		return rulesForVersion(pc28RuleV1)
	case "canada-28":
		return rulesForVersion(pc28RuleV2)
	case "canada-20":
		return rulesForVersion(pc28RuleV3)
	default:
		// PC, Mark Six, Keno and other native lotteries need their own approved
		// number ranges, settlement rules and odds; never silently use SSC.
		return gameRuleProfile{}, false
	}
}

// A ticket must use the exact contract bound to its game. This prelaunch
// project has no historical rule engines; unknown or retired versions fail
// closed instead of inferring terms from the number of drawn balls.
func gameSupportsRuleVersion(gameID, version string) bool {
	profile, ready := rulesForGame(&lottery.Game{ID: strings.TrimSpace(gameID)})
	return ready && profile.Version == strings.TrimSpace(version)
}

func ensureGameRulesSupported(game *lottery.Game) error {
	if _, ok := rulesForGame(game); !ok {
		return apperrors.NewBusinessError("RULES_NOT_READY", gameRulesUnavailableMessage)
	}
	return nil
}

func (profile gameRuleProfile) supportsPlay(code string) bool {
	if profile.Version == "" {
		return false
	}
	if profile.MarkSix {
		_, ok := markSixPlayByCodeForVersion(profile.Version, code)
		return ok
	}
	if profile.PC28 > 0 {
		return pc28SupportsPlay(code)
	}
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "ball_1_5", "two_sided", "dragon_tiger":
		return true
	case "sum":
		return profile.SumMarket
	case "dragon_tiger_tie":
		return profile.DragonTigerTie
	case "leopard", "straight", "pair", "half_straight", "mixed":
		return profile.Patterns
	default:
		return false
	}
}

func (profile gameRuleProfile) validateDraw(numbers []int) error {
	if profile.Version == "" || len(numbers) != profile.BallCount {
		return apperrors.NewBusinessError("INVALID_DRAW", fmt.Sprintf("开奖号码应为 %d 个", profile.BallCount))
	}
	seen := make(map[int]struct{}, len(numbers))
	for _, number := range numbers {
		if number < profile.MinNumber || number > profile.MaxNumber {
			return apperrors.NewBusinessError("INVALID_DRAW", fmt.Sprintf("开奖号码必须在 %d 至 %d 之间", profile.MinNumber, profile.MaxNumber))
		}
		if _, exists := seen[number]; profile.Unique && exists {
			return apperrors.NewBusinessError("INVALID_DRAW", "开奖号码不能重复")
		}
		seen[number] = struct{}{}
	}
	return nil
}

// validateChoice is shared by placement and versioned settlement. Every
// accepted combination must have exactly the same meaning in both paths.
func (profile gameRuleProfile) validateChoice(playCode string, position int, selection string) error {
	playCode = strings.ToLower(strings.TrimSpace(playCode))
	selection = strings.TrimSpace(selection)
	if profile.MarkSix {
		return markSixValidateChoiceForVersion(profile.Version, playCode, position, selection)
	}
	if profile.PC28 > 0 {
		return pc28ValidateChoice(playCode, position, selection)
	}
	if !profile.supportsPlay(playCode) {
		return apperrors.NewBusinessError("INVALID_REQUEST", "当前彩种不支持该玩法")
	}
	if playCode == "sum" {
		if position != 6 {
			return apperrors.NewBusinessError("INVALID_REQUEST", "总和玩法位置不正确")
		}
	} else if position < 1 || position > profile.BallCount {
		return apperrors.NewBusinessError("INVALID_REQUEST", "球位不正确")
	}
	switch playCode {
	case "ball_1_5":
		number, err := strconv.Atoi(selection)
		if err != nil || number < profile.MinNumber || number > profile.MaxNumber || (!profile.Racing && len(selection) != 1) {
			return apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("号码只能选择 %d 至 %d", profile.MinNumber, profile.MaxNumber))
		}
	case "two_sided":
		if !isSideSelection(selection) {
			return apperrors.NewBusinessError("INVALID_REQUEST", "两面玩法仅支持大、小、单、双")
		}
	case "dragon_tiger":
		maxPosition := profile.BallCount / 2
		if profile.FirstLastDragonTiger {
			maxPosition = 1
		}
		if position > maxPosition {
			return apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("龙虎仅支持第 1 至第 %d 位", maxPosition))
		}
		if normalized := normalizeSelection(selection); normalized != "dragon" && normalized != "tiger" {
			return apperrors.NewBusinessError("INVALID_REQUEST", "龙虎玩法只能选择龙或虎")
		}
	case "dragon_tiger_tie":
		if !profile.DragonTigerTie || position != 1 || normalizeSelection(selection) != "tie" {
			return apperrors.NewBusinessError("INVALID_REQUEST", "龙虎和仅支持第1球与第5球相等")
		}
	case "sum":
		if isSideSelection(selection) {
			return nil
		}
		number, err := strconv.Atoi(selection)
		if profile.Racing {
			if err != nil || number < 3 || number > 19 {
				return apperrors.NewBusinessError("INVALID_REQUEST", "冠亚和号码只能选择 3 至 19")
			}
		} else if err != nil || number < 0 || number > 9 || len(selection) != 1 {
			return apperrors.NewBusinessError("INVALID_REQUEST", "总和尾数只能选择 0 至 9")
		}
	default:
		maxPosition := 1
		if profile.ThreeShapeWindows {
			maxPosition = 3
		}
		if position < 1 || position > maxPosition {
			return apperrors.NewBusinessError("INVALID_REQUEST", "形态玩法位置不正确")
		}
		if normalized := normalizePlaySelection(selection); normalized != "yes" && normalized != "中" && normalized != playCode {
			return apperrors.NewBusinessError("INVALID_REQUEST", "形态玩法与投注内容不一致")
		}
	}
	return nil
}

// Keep existing configured odds untouched, but present the meaning of the
// supported profile rather than a generic racing label on every lottery.
func (profile gameRuleProfile) playName(code, configuredName string) string {
	if profile.MarkSix {
		if play, ok := markSixPlayByCodeForVersion(profile.Version, code); ok {
			return play.Name
		}
		return configuredName
	}
	if profile.PC28 > 0 {
		if play, ok := pc28SpecByCode(code); ok {
			return play.Name
		}
		return configuredName
	}
	switch code {
	case "ball_1_5":
		if profile.Racing {
			return "指定名次号码"
		}
		return "指定球位号码"
	case "sum":
		if profile.Racing {
			return "冠亚和"
		}
		return "总和 / 总和尾"
	default:
		if profile.Patterns {
			switch code {
			case "leopard", "straight", "pair", "half_straight", "mixed":
				if play, ok := defaultPlayByCode(code); ok {
					if profile.ThreeShapeWindows {
						return "三段" + play.Name
					}
					return "前三" + play.Name
				}
			}
		}
		if code == "dragon_tiger_tie" {
			return "龙虎和"
		}
		return configuredName
	}
}

// PlayCatalogForGame describes only plays that the chosen game's placement
// and settlement paths both implement. It never seeds or changes saved odds.
func PlayCatalogForGame(gameID string) []PlayCatalogItem {
	gameID = strings.TrimSpace(gameID)
	profile, ready := rulesForGame(&lottery.Game{ID: gameID})
	items := make([]PlayCatalogItem, 0)
	if !ready {
		return items
	}
	if profile.MarkSix {
		return markSixPlayCatalog()
	}
	if profile.PC28 > 0 {
		return pc28PlayCatalog()
	}
	for _, item := range PlayCatalog() {
		if !profile.supportsPlay(item.PlayCode) {
			continue
		}
		// Bingo Racing A's source is now order-verified, but its original
		// crown-sum market is not one flat price.  Expose internal pricing codes
		// for every mutually exclusive selection while placement and settlement
		// continue to store the stable public play code "sum".  Other racing-v2
		// games keep their existing single sum row unchanged.
		if gameID == "bingo-racing-a" && item.PlayCode == "sum" {
			items = append(items, bingoRacingASumOddsCatalog(item.SortOrder)...)
			continue
		}
		item.PlayName = profile.playName(item.PlayCode, item.PlayName)
		if !profile.Racing {
			switch item.PlayCode {
			case "ball_1_5":
				item.Description = fmt.Sprintf("第1至第%d球的单个号码0至9；每个投注项单独计费", profile.BallCount)
				item.Example = "3/07/20"
			case "two_sided":
				item.Description = "指定球位：0至4为小，5至9为大，按实际号码判断单双"
				item.Example = "1/大/20"
			case "sum":
				item.Description = fmt.Sprintf("全部%d球相加；%d及以上为大，其余为小；号码选项0至9为总和尾数", profile.BallCount, profile.SumBigFrom)
				item.Example = "总和/大/20、总和尾/7/20"
			case "dragon_tiger":
				if profile.FirstLastDragonTiger {
					item.Description = "第1球与第5球比较：大于为龙，小于为虎"
				} else {
					item.Description = fmt.Sprintf("第1至第%d球分别与对应末位号码比较；相等为和，当前仅开放龙、虎", profile.BallCount/2)
				}
				item.Example = "1/龙/20"
			case "dragon_tiger_tie":
				item.Description = "第1球与第5球相等时中奖；使用独立后台赔率"
				item.Example = "1/和/20"
			default:
				if profile.ThreeShapeWindows && isFrontThreeShape(item.PlayCode) {
					item.Description = "前三、中三、后三分别按连续三个球位判定；每个展开项独立计注"
					if play, ok := defaultPlayByCode(item.PlayCode); ok {
						item.Example = "中三/" + play.Name + "/20"
					}
				}
			}
		}
		items = append(items, item)
	}
	return items
}

func bingoRacingASumOddsCatalog(baseOrder int) []PlayCatalogItem {
	selections := []struct {
		key, name, example string
	}{
		{"big", "冠亚和大", "冠亚和大/20"},
		{"small", "冠亚和小", "冠亚和小/20"},
		{"odd", "冠亚和单", "冠亚和单/20"},
		{"even", "冠亚和双", "冠亚和双/20"},
	}
	items := make([]PlayCatalogItem, 0, 21)
	for index, selection := range selections {
		items = append(items, PlayCatalogItem{
			PlayCode: "sum_" + selection.key, PlayName: selection.name, Category: "冠亚和",
			Description: "宾果赛车(A)冠亚和按具体选项独立定价；未单独配置时该选项不可投注",
			Example:     selection.example, SortOrder: baseOrder*100 + index,
		})
	}
	for value := 3; value <= 19; value++ {
		items = append(items, PlayCatalogItem{
			PlayCode: fmt.Sprintf("sum_%d", value), PlayName: fmt.Sprintf("冠亚和%d", value), Category: "冠亚和",
			Description: "宾果赛车(A)冠亚和值号码独立定价；未单独配置时该号码不可投注",
			Example:     fmt.Sprintf("冠亚/%d/20", value),
			SortOrder:   baseOrder*100 + len(selections) + value - 3,
		})
	}
	return items
}
