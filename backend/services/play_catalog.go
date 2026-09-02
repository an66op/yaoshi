package services

import (
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"strings"
)

// PlayCatalogItem documents a supported play type for admin UI and settlement.
type PlayCatalogItem struct {
	PlayCode    string  `json:"play_code"`
	PlayName    string  `json:"play_name"`
	Category    string  `json:"category"`
	Description string  `json:"description"`
	Example     string  `json:"example"`
	DefaultOdds float64 `json:"default_odds"`
	SortOrder   int     `json:"sort_order"`
}

type defaultPlay struct {
	Code        string
	Name        string
	Category    string
	Description string
	Example     string
	Odds        float64
}

var defaultPlays = []defaultPlay{
	{
		Code: "two_sided", Name: "两面", Category: "两面盘",
		Description: "指定名次的大小、单双；赛车冠亚和使用专用玩法",
		Example:     "1大/100、冠亚和小/50",
		Odds:        1.993,
	},
	{
		Code: "ball_1_5", Name: "指定名次号码", Category: "号码",
		Description: "指定名次开出指定号码；赛车、飞艇支持第1-10名",
		Example:     "3/7/100、12345/200",
		Odds:        9.9,
	},
	{
		Code: "dragon_tiger", Name: "龙虎", Category: "龙虎",
		Description: "赛车第1至5名分别与第10至6名比较；数字彩按已确认球位规则比较",
		Example:     "冠军龙/100",
		Odds:        1.993,
	},
	{
		Code: "dragon_tiger_tie", Name: "龙虎和", Category: "龙虎",
		Description: "已确认的数字彩第1球与第5球相等；赔率与龙、虎分开配置",
		Example:     "1/和/20",
		Odds:        8.7,
	},
	{
		Code: "sum", Name: "冠亚和", Category: "总和",
		Description: "赛车冠亚和值3-19，3-11小、12-19大；数字彩使用明确的总和或总和尾玩法",
		Example:     "冠亚和大/100、冠亚/14/50",
		Odds:        1.993,
	},
	{
		Code: "leopard", Name: "豹子", Category: "形态",
		Description: "前三位号码相同",
		Example:     "豹子/50",
		Odds:        50,
	},
	{
		Code: "straight", Name: "顺子", Category: "形态",
		Description: "数字彩前三位号码构成顺子，按已有前三形态规则结算",
		Example:     "顺子/30",
		Odds:        15,
	},
	{
		Code: "pair", Name: "对子", Category: "形态",
		Description: "前三位有且仅有一对相同",
		Example:     "对子/20",
		Odds:        8,
	},
	{
		Code: "half_straight", Name: "半顺", Category: "形态",
		Description: "前三位有两号相邻但不构成顺子",
		Example:     "半顺/15",
		Odds:        6,
	},
	{
		Code: "mixed", Name: "杂六", Category: "形态",
		Description: "前三位互不相同且不构成半顺",
		Example:     "杂六/10",
		Odds:        4,
	},
}

func PlayCatalog() []PlayCatalogItem {
	items := make([]PlayCatalogItem, 0, len(defaultPlays))
	for i, play := range defaultPlays {
		items = append(items, PlayCatalogItem{
			PlayCode: play.Code, PlayName: play.Name, Category: play.Category,
			Description: play.Description, Example: play.Example,
			DefaultOdds: play.Odds, SortOrder: i,
		})
	}
	return items
}

func defaultPlayByCode(code string) (defaultPlay, bool) {
	code = strings.ToLower(strings.TrimSpace(code))
	for _, play := range defaultPlays {
		if play.Code == code {
			return play, true
		}
	}
	return defaultPlay{}, false
}

// InferPlayForGame is the only bet inference boundary. The game profile owns
// the meaning of every position, so the sixth racing rank cannot be confused
// with a sum market and an unsupported lottery cannot inherit SSC semantics.
func InferPlayForGame(game *lottery.Game, playCode, playName string, position int, selection string) (string, string, error) {
	if err := ensureGameRulesSupported(game); err != nil {
		return "", "", err
	}
	rules, _ := rulesForGame(game)
	code := strings.ToLower(strings.TrimSpace(playCode))
	name := strings.TrimSpace(playName)
	selection = strings.TrimSpace(selection)
	if rules.MarkSix {
		if code == "" {
			return "", "", apperrors.NewBusinessError("INVALID_REQUEST", "宾果六合彩网投必须明确玩法编号")
		}
		play, ok := markSixPlayByCode(code)
		if !ok {
			return "", "", apperrors.NewBusinessError("INVALID_REQUEST", "宾果六合彩当前不支持该玩法")
		}
		return code, play.Name, nil
	}
	if rules.PC28 > 0 {
		if code == "" {
			return "", "", apperrors.NewBusinessError("INVALID_REQUEST", "PC28投注必须明确玩法编号")
		}
		play, ok := pc28SpecByCode(code)
		if !ok {
			return "", "", apperrors.NewBusinessError("INVALID_REQUEST", "当前PC28彩种不支持该玩法")
		}
		return code, play.Name, nil
	}
	for _, value := range []string{name, selection} {
		if scope, scoped := digitShapeScope(value); scoped {
			if !rules.ThreeShapeWindows && scope != 1 {
				return "", "", apperrors.NewBusinessError("INVALID_REQUEST", "当前彩种形态玩法仅支持前三球")
			}
			if position >= 1 && position <= 3 && position != scope {
				return "", "", apperrors.NewBusinessError("INVALID_REQUEST", "形态玩法名称与球位不一致")
			}
		}
	}
	if !rules.Racing && isCrownSumToken(name) {
		return "", "", apperrors.NewBusinessError("INVALID_REQUEST", "该彩种不使用冠亚和，请明确填写总和或总和尾")
	}
	if rules.Racing && (name == "总和" || name == "总和尾") {
		return "", "", apperrors.NewBusinessError("INVALID_REQUEST", "赛车请明确填写冠亚和玩法")
	}
	if !rules.Racing && name == "总和尾" && !allDigits(selection) {
		return "", "", apperrors.NewBusinessError("INVALID_REQUEST", "总和尾只能选择 0 至 9；大小单双请明确填写总和")
	}
	if code == "" {
		switch {
		case strings.EqualFold(name, "sum"):
			code, name = "sum", ""
		case rules.Racing && isCrownSumToken(name):
			code = "sum"
		case !rules.Racing && (name == "总和" || name == "总和尾"):
			code = "sum"
		default:
			if _, scoped := digitShapeScope(name); scoped {
				if shape, ok := assistantShapeCode(trimDigitShapeScope(name)); ok {
					code = shape
				}
			}
			for _, play := range defaultPlays {
				if code == "" && play.Code != "sum" && (strings.EqualFold(name, play.Name) || strings.EqualFold(name, play.Code)) {
					code = play.Code
					break
				}
			}
		}
	}
	if code == "" {
		switch {
		case normalizeSelection(selection) == "dragon" || normalizeSelection(selection) == "tiger":
			code = "dragon_tiger"
		case normalizeSelection(selection) == "tie":
			code = "dragon_tiger_tie"
		case isSideSelection(selection):
			code = "two_sided"
		case allDigits(selection):
			code = "ball_1_5"
		default:
			for _, play := range defaultPlays {
				if play.Code != "sum" && (strings.EqualFold(selection, play.Name) || strings.EqualFold(selection, play.Code)) {
					code = play.Code
					break
				}
			}
		}
	}
	if !rules.supportsPlay(code) {
		return "", "", apperrors.NewBusinessError("INVALID_REQUEST", "当前彩种不支持该玩法")
	}
	if name == "" {
		if code == "sum" {
			name = "冠亚和"
			if !rules.Racing {
				name = "总和"
				if allDigits(selection) {
					name = "总和尾"
				}
			}
		} else if play, ok := defaultPlayByCode(code); ok {
			name = rules.playName(code, play.Name)
			if isFrontThreeShape(code) {
				name = digitShapeScopeName(position) + play.Name
			}
		}
	}
	return code, defaultString(name, code), nil
}

func digitShapeScope(value string) (int, bool) {
	value = strings.TrimSpace(value)
	for position, prefix := range []string{"前三", "中三", "后三"} {
		if strings.HasPrefix(value, prefix) {
			return position + 1, true
		}
	}
	return 0, false
}

func trimDigitShapeScope(value string) string {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"前三", "中三", "后三"} {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimPrefix(value, prefix)
		}
	}
	return value
}

func digitShapeScopeName(position int) string {
	switch position {
	case 2:
		return "中三"
	case 3:
		return "后三"
	default:
		return "前三"
	}
}

func isDigit(value string) bool {
	if len(value) != 1 {
		return false
	}
	return value[0] >= '0' && value[0] <= '9'
}

func isSideSelection(value string) bool {
	switch strings.ToLower(value) {
	case "大", "小", "单", "双", "big", "small", "odd", "even":
		return true
	default:
		return false
	}
}
