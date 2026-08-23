package services

import "strings"

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
		Description: "第1-5球或冠亚和的大小、单双",
		Example:     "1大/100、冠亚和小/50",
		Odds:        1.993,
	},
	{
		Code: "ball_1_5", Name: "1-5球号", Category: "号码",
		Description: "指定名次开出指定号码；第6位为冠亚和尾数",
		Example:     "3/7/100、12345/200",
		Odds:        9.9,
	},
	{
		Code: "dragon_tiger", Name: "龙虎", Category: "龙虎",
		Description: "冠军与末位号码比大小",
		Example:     "冠军龙/100",
		Odds:        1.993,
	},
	{
		Code: "sum", Name: "冠亚和", Category: "总和",
		Description: "冠亚和值的大小、单双或尾数",
		Example:     "冠亚和大/100",
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
		Description: "前三位号码连续递增",
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

// InferPlay resolves play_code and play_name from bet payload when omitted.
func InferPlay(playCode, playName string, position int, selection string) (string, string) {
	code := strings.ToLower(strings.TrimSpace(playCode))
	name := strings.TrimSpace(playName)
	if code != "" {
		if name == "" {
			if play, ok := defaultPlayByCode(code); ok {
				name = play.Name
			}
		}
		return code, defaultString(name, code)
	}
	selection = strings.TrimSpace(selection)
	if position == 6 {
		if selection == "龙" || selection == "虎" {
			return "dragon_tiger", defaultString(name, "龙虎")
		}
		return "sum", defaultString(name, "冠亚和")
	}
	if selection == "龙" || selection == "虎" {
		return "dragon_tiger", defaultString(name, "龙虎")
	}
	if isDigit(selection) {
		return "ball_1_5", defaultString(name, "1-5球号")
	}
	if isSideSelection(selection) {
		return "two_sided", defaultString(name, "两面")
	}
	for _, play := range defaultPlays {
		if strings.EqualFold(selection, play.Name) || strings.EqualFold(selection, play.Code) {
			return play.Code, play.Name
		}
	}
	return "ball_1_5", defaultString(name, "1-5球号")
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
