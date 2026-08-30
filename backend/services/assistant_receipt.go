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
		key := line.Position
		if line.PlayCode == "sum" {
			key = 11
		} else if key < 1 || key > 10 || line.Selection == "" {
			fallback = append(fallback, line.Label)
			continue
		}
		groups[key] = append(groups[key], line)
	}
	var result []string
	for rank := 1; rank <= 11; rank++ {
		items := groups[rank]
		if len(items) == 0 {
			continue
		}
		sort.SliceStable(items, func(i, j int) bool {
			return assistantReceiptChoiceOrder(items[i].Selection) < assistantReceiptChoiceOrder(items[j].Selection)
		})
		label := strings.SplitN(assistantLineLabel(items[0]), "[", 2)[0]
		choices := make([]string, 0, len(items))
		for _, item := range items {
			choices = append(choices, fmt.Sprintf("%s/%s", item.Selection, FormatBetAmount(item.Amount)))
		}
		result = append(result, label+"["+strings.Join(choices, " ")+"]")
	}
	return append(result, fallback...)
}

func assistantReceiptChoiceOrder(selection string) int {
	if number, err := strconv.Atoi(selection); err == nil {
		return number
	}
	for index, choice := range []string{"单", "双", "大", "小", "龙", "虎"} {
		if selection == choice {
			return 100 + index
		}
	}
	return 200
}
