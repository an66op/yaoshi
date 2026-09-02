package services

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// FormatBetAmount only removes redundant fractional zeros from a stake's
// presentation. Real cents and persisted accounting amounts remain intact.
func FormatBetAmount(amount float64) string {
	return strings.TrimSuffix(strconv.FormatFloat(amount, 'f', 2, 64), ".00")
}

// AssistantReceiptLines groups presentation only. The authoritative line items,
// odds snapshots, stakes and persisted order rows are never changed or combined.
func AssistantReceiptLines(lines []AssistantBetLine) []string {
	groups := make(map[int][]AssistantBetLine)
	var fallback []string
	for _, line := range lines {
		key, _ := assistantReceiptGroup(line)
		if key == 0 || line.Selection == "" {
			fallback = append(fallback, line.Label)
			continue
		}
		groups[key] = append(groups[key], line)
	}
	var result []string
	ranks := make([]int, 0, len(groups))
	for rank := range groups {
		ranks = append(ranks, rank)
	}
	sort.Ints(ranks)
	for _, rank := range ranks {
		items := groups[rank]
		if len(items) == 0 {
			continue
		}
		sort.SliceStable(items, func(i, j int) bool {
			return assistantReceiptChoiceOrder(items[i].Selection) < assistantReceiptChoiceOrder(items[j].Selection)
		})
		_, label := assistantReceiptGroup(items[0])
		choices := make([]string, 0, len(items))
		for _, item := range items {
			choices = append(choices, fmt.Sprintf("%s/%s", assistantReceiptSelection(item), FormatBetAmount(item.Amount)))
		}
		result = append(result, label+"["+strings.Join(choices, " ")+"]")
	}
	return append(result, fallback...)
}

// A front-three shape is not a first-ball bet, and a digit total/tail is not
// a racing crown sum. Group on that meaning, without modifying order rows.
func assistantReceiptGroup(line AssistantBetLine) (int, string) {
	if pc28IsExactPlay(line.PlayCode) {
		return 40, "PC28和值"
	}
	switch line.PlayCode {
	case pc28PackageThree:
		return 41, "特码包三"
	case pc28SumSize:
		return 42, "和值大小"
	case pc28SumParity:
		return 43, "和值单双"
	case pc28ComboBigOdd, pc28ComboBigEven, pc28ComboSmallOdd, pc28ComboSmallEven:
		return 44, "大小单双"
	case pc28Extreme:
		return 45, "极值大小"
	case pc28ColorRed, pc28ColorGreen, pc28ColorBlue:
		return 46, "色波"
	case pc28Leopard, pc28Pair, pc28Straight:
		return 47, "三球形态"
	case pc28PositionNumber, pc28PositionTwoSided:
		return 50 + line.Position, fmt.Sprintf("第%d球", line.Position)
	case pc28DragonTiger, pc28DragonTigerTie:
		return 54, "第一球龙虎和"
	}
	if _, ok := assistantShapeCode(line.PlayCode); ok {
		position := line.Position
		if position < 1 || position > 3 {
			position = 1
		}
		return 19 + position, digitShapeScopeName(position)
	}
	if line.PlayCode == "sum" {
		if line.PlayName == "总和尾" {
			return 31, "总和尾"
		}
		if strings.HasPrefix(line.PlayName, "总和") {
			return 30, "总和"
		}
		return 11, "冠亚和"
	}
	if line.PlayCode == "dragon_tiger_tie" || line.PlayCode == "dragon_tiger" && strings.Contains(line.PlayName, "球龙虎") {
		return 10 + line.Position, fmt.Sprintf("第%d球龙虎", line.Position)
	}
	if line.Position < 1 || line.Position > 10 {
		return 0, fmt.Sprintf("第%d名", line.Position)
	}
	if strings.HasPrefix(line.PlayName, fmt.Sprintf("第%d球", line.Position)) {
		return line.Position, fmt.Sprintf("第%d球", line.Position)
	}
	names := []string{"冠军", "亚军", "第三名", "第四名", "第五名", "第六名", "第七名", "第八名", "第九名", "第十名"}
	return line.Position, names[line.Position-1]
}

func assistantReceiptSelection(line AssistantBetLine) string {
	if code, ok := assistantShapeCode(line.PlayCode); ok {
		if play, found := defaultPlayByCode(code); found {
			return play.Name
		}
	}
	return line.Selection
}

func assistantReceiptChoiceOrder(selection string) int {
	if number, err := strconv.Atoi(selection); err == nil {
		return number
	}
	for index, choice := range []string{"单", "双", "大", "小", "龙", "虎", "和"} {
		if selection == choice {
			return 100 + index
		}
	}
	return 200
}
