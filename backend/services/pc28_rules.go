package services

import (
	"backend/data/models/bet"
	apperrors "backend/errors"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

func configuredPC28GrayPush(raw, gameID string) bool {
	settings := map[string]any{}
	if json.Unmarshal([]byte(raw), &settings) != nil {
		return false
	}
	if overrides, ok := settings["pc28_gray_push_overrides"].(map[string]any); ok {
		if value, ok := overrides[strings.TrimSpace(gameID)].(bool); ok {
			return value
		}
	}
	value, _ := settings["pc28_gray_push"].(bool)
	return value
}

const (
	pc28RuleV1 = "pc28-v1"
	pc28RuleV2 = "pc28-v2"
	pc28RuleV3 = "pc28-v3"

	pc28PackageThree     = "pc28_package_three"
	pc28PositionNumber   = "pc28_position_number"
	pc28PositionTwoSided = "pc28_position_two_sided"
	pc28DragonTiger      = "pc28_dragon_tiger"
	pc28DragonTigerTie   = "pc28_dragon_tiger_tie"
	pc28SumSize          = "pc28_sum_size"
	pc28SumParity        = "pc28_sum_parity"
	pc28ComboBigOdd      = "pc28_combo_big_odd"
	pc28ComboBigEven     = "pc28_combo_big_even"
	pc28ComboSmallOdd    = "pc28_combo_small_odd"
	pc28ComboSmallEven   = "pc28_combo_small_even"
	pc28Extreme          = "pc28_extreme"
	pc28ColorRed         = "pc28_color_red"
	pc28ColorGreen       = "pc28_color_green"
	pc28ColorBlue        = "pc28_color_blue"
	pc28Leopard          = "pc28_leopard"
	pc28Pair             = "pc28_pair"
	pc28Straight         = "pc28_straight"
	pc28ExactPrefix      = "pc28_sum_exact_"
)

type pc28PlaySpec struct {
	Code, Name, Category, Description, Example string
	SortOrder                                  int
}

func pc28RuleVariant(version string) int {
	switch strings.TrimSpace(version) {
	case pc28RuleV1:
		return 1
	case pc28RuleV2:
		return 2
	case pc28RuleV3:
		return 3
	default:
		return 0
	}
}

func pc28ExactCode(total int) string {
	if total < 0 || total > 27 {
		return ""
	}
	low, high := total, 27-total
	if low > high {
		low, high = high, low
	}
	return fmt.Sprintf("%s%d_%d", pc28ExactPrefix, low, high)
}

func pc28ExactPair(code string) (int, int, bool) {
	raw := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(code)), pc28ExactPrefix)
	if raw == strings.ToLower(strings.TrimSpace(code)) {
		return 0, 0, false
	}
	parts := strings.Split(raw, "_")
	if len(parts) != 2 {
		return 0, 0, false
	}
	low, lowErr := strconv.Atoi(parts[0])
	high, highErr := strconv.Atoi(parts[1])
	return low, high, lowErr == nil && highErr == nil && low >= 0 && high <= 27 && low <= high && low+high == 27
}

func pc28PlaySpecs() []pc28PlaySpec {
	items := make([]pc28PlaySpec, 0, 34)
	order := 0
	for low := 0; low <= 13; low++ {
		high := 27 - low
		items = append(items, pc28PlaySpec{
			Code: pc28ExactCode(low), Name: fmt.Sprintf("和值%d/%d", low, high), Category: "和值",
			Description: fmt.Sprintf("三球和值精确命中%d或%d时，分别按所选点数结算；每期最多选择10个不同点数", low, high),
			Example:     fmt.Sprintf("%d/5", low), SortOrder: order,
		})
		order++
	}
	fixed := []pc28PlaySpec{
		{Code: pc28PackageThree, Name: "特码包三", Category: "和值", Description: "从0至27选择三个互不相同的和值，任一命中即中奖", Example: "特码/1/2/3/5"},
		{Code: pc28PositionNumber, Name: "1-3球号", Category: "定位", Description: "第1至3球的号码0至9", Example: "13/89/5"},
		{Code: pc28PositionTwoSided, Name: "定位两面", Category: "定位", Description: "指定球位0至4小、5至9大，并按号码判断单双", Example: "1大5"},
		{Code: pc28DragonTiger, Name: "龙虎", Category: "龙虎和", Description: "第一球大于第三球为龙，小于为虎", Example: "1/龙/5"},
		{Code: pc28DragonTigerTie, Name: "和", Category: "龙虎和", Description: "第一球等于第三球；使用独立后台赔率", Example: "1/和/5"},
		{Code: pc28SumSize, Name: "大小", Category: "两面", Description: "和值0至13小、14至27大", Example: "大/5"},
		{Code: pc28SumParity, Name: "单双", Category: "两面", Description: "按三球和值判断单、双", Example: "单/5"},
		{Code: pc28ComboBigOdd, Name: "大单", Category: "组合", Description: "和值同时为大且为单", Example: "大单/5"},
		{Code: pc28ComboBigEven, Name: "大双", Category: "组合", Description: "和值同时为大且为双", Example: "大双/5"},
		{Code: pc28ComboSmallOdd, Name: "小单", Category: "组合", Description: "和值同时为小且为单", Example: "小单/5"},
		{Code: pc28ComboSmallEven, Name: "小双", Category: "组合", Description: "和值同时为小且为双", Example: "小双/5"},
		{Code: pc28Extreme, Name: "极值大小", Category: "极值", Description: "和值0至5为极小、22至27为极大", Example: "极小/5"},
		{Code: pc28ColorRed, Name: "红波", Category: "色波", Description: "和值命中红波；灰/黄波按下注时冻结的房间返本设置处理", Example: "红波/5"},
		{Code: pc28ColorGreen, Name: "绿波", Category: "色波", Description: "和值命中绿波；灰/黄波按下注时冻结的房间返本设置处理", Example: "绿波/5"},
		{Code: pc28ColorBlue, Name: "蓝波", Category: "色波", Description: "和值命中蓝波；灰/黄波按下注时冻结的房间返本设置处理", Example: "蓝波/5"},
		{Code: pc28Leopard, Name: "豹子", Category: "形态", Description: "三球号码全部相同", Example: "豹子/5"},
		{Code: pc28Pair, Name: "对子", Category: "形态", Description: "三球中恰有两个号码相同", Example: "对子/5"},
		{Code: pc28Straight, Name: "顺子", Category: "形态", Description: "三球互不相同且排序后连续；890、901不算顺子", Example: "顺子/5"},
	}
	for index := range fixed {
		fixed[index].SortOrder = order
		order++
		items = append(items, fixed[index])
	}
	return items
}

func pc28SpecByCode(code string) (pc28PlaySpec, bool) {
	code = strings.ToLower(strings.TrimSpace(code))
	for _, spec := range pc28PlaySpecs() {
		if spec.Code == code {
			return spec, true
		}
	}
	return pc28PlaySpec{}, false
}

func pc28PlayCatalog() []PlayCatalogItem {
	items := make([]PlayCatalogItem, 0, len(pc28PlaySpecs()))
	for _, spec := range pc28PlaySpecs() {
		// Zero is deliberate: PC28 is unusable until an administrator explicitly
		// supplies this room/game's price. Never manufacture an original-site rate.
		items = append(items, PlayCatalogItem{PlayCode: spec.Code, PlayName: spec.Name, Category: spec.Category,
			Description: spec.Description, Example: spec.Example, DefaultOdds: 0, SortOrder: spec.SortOrder})
	}
	return items
}

func pc28SupportsPlay(code string) bool {
	_, ok := pc28SpecByCode(code)
	return ok
}

func pc28DefaultValidationSelection(code string) string {
	if low, _, ok := pc28ExactPair(code); ok {
		return strconv.Itoa(low)
	}
	switch strings.ToLower(strings.TrimSpace(code)) {
	case pc28PackageThree:
		return "0,1,2"
	case pc28PositionNumber:
		return "0"
	case pc28PositionTwoSided, pc28SumSize:
		return "大"
	case pc28SumParity:
		return "单"
	case pc28DragonTiger:
		return "龙"
	case pc28DragonTigerTie:
		return "和"
	case pc28ComboBigOdd:
		return "大单"
	case pc28ComboBigEven:
		return "大双"
	case pc28ComboSmallOdd:
		return "小单"
	case pc28ComboSmallEven:
		return "小双"
	case pc28Extreme:
		return "极大"
	case pc28ColorRed:
		return "红波"
	case pc28ColorGreen:
		return "绿波"
	case pc28ColorBlue:
		return "蓝波"
	case pc28Leopard:
		return "豹子"
	case pc28Pair:
		return "对子"
	case pc28Straight:
		return "顺子"
	default:
		return ""
	}
}

func pc28NormalizeSelection(code, selection string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	selection = strings.TrimSpace(selection)
	if _, _, ok := pc28ExactPair(code); ok || code == pc28PositionNumber {
		if number, err := strconv.Atoi(selection); err == nil {
			return strconv.Itoa(number)
		}
	}
	switch code {
	case pc28PackageThree:
		if values, ok := pc28ParsePackage(selection); ok {
			parts := make([]string, len(values))
			for index, value := range values {
				parts[index] = strconv.Itoa(value)
			}
			return strings.Join(parts, ",")
		}
	case pc28PositionTwoSided, pc28SumSize, pc28SumParity, pc28DragonTiger, pc28DragonTigerTie:
		return selectionLabel(normalizeSelection(selection))
	}
	return selection
}

func pc28ParsePackage(selection string) ([]int, bool) {
	raw := strings.NewReplacer("，", ",", "/", ",", " ", ",").Replace(strings.TrimSpace(selection))
	parts := strings.Split(raw, ",")
	values := make([]int, 0, 3)
	seen := map[int]struct{}{}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || value < 0 || value > 27 {
			return nil, false
		}
		if _, exists := seen[value]; exists {
			return nil, false
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	if len(values) != 3 {
		return nil, false
	}
	sort.Ints(values)
	return values, true
}

func pc28ValidateChoice(code string, position int, selection string) error {
	code = strings.ToLower(strings.TrimSpace(code))
	selection = pc28NormalizeSelection(code, selection)
	if !pc28SupportsPlay(code) {
		return apperrors.NewBusinessError("INVALID_REQUEST", "当前PC28彩种不支持该玩法")
	}
	if low, high, exact := pc28ExactPair(code); exact {
		value, err := strconv.Atoi(selection)
		if position != 0 || err != nil || (value != low && value != high) {
			return apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("%s只能选择和值%d或%d", code, low, high))
		}
		return nil
	}
	switch code {
	case pc28PackageThree:
		if position != 0 {
			return apperrors.NewBusinessError("INVALID_REQUEST", "特码包三不使用球位")
		}
		if _, ok := pc28ParsePackage(selection); !ok {
			return apperrors.NewBusinessError("INVALID_REQUEST", "特码包三必须选择三个互不相同的0至27和值")
		}
	case pc28PositionNumber:
		value, err := strconv.Atoi(selection)
		if position < 1 || position > 3 || err != nil || value < 0 || value > 9 {
			return apperrors.NewBusinessError("INVALID_REQUEST", "定位号码须选择第1至3球及号码0至9")
		}
	case pc28PositionTwoSided:
		if position < 1 || position > 3 || !isSideSelection(selection) {
			return apperrors.NewBusinessError("INVALID_REQUEST", "定位两面须选择第1至3球及大、小、单、双")
		}
	case pc28DragonTiger:
		normalized := normalizeSelection(selection)
		if position != 1 || (normalized != "dragon" && normalized != "tiger") {
			return apperrors.NewBusinessError("INVALID_REQUEST", "龙虎只支持第一球与第三球的龙或虎")
		}
	case pc28DragonTigerTie:
		if position != 1 || normalizeSelection(selection) != "tie" {
			return apperrors.NewBusinessError("INVALID_REQUEST", "和只支持第一球与第三球相同")
		}
	case pc28SumSize:
		if position != 0 || (normalizeSelection(selection) != "big" && normalizeSelection(selection) != "small") {
			return apperrors.NewBusinessError("INVALID_REQUEST", "和值大小只能选择大或小")
		}
	case pc28SumParity:
		if position != 0 || (normalizeSelection(selection) != "odd" && normalizeSelection(selection) != "even") {
			return apperrors.NewBusinessError("INVALID_REQUEST", "和值单双只能选择单或双")
		}
	case pc28ComboBigOdd:
		return pc28ValidateFixedChoice(position, selection, "大单")
	case pc28ComboBigEven:
		return pc28ValidateFixedChoice(position, selection, "大双")
	case pc28ComboSmallOdd:
		return pc28ValidateFixedChoice(position, selection, "小单")
	case pc28ComboSmallEven:
		return pc28ValidateFixedChoice(position, selection, "小双")
	case pc28Extreme:
		if position != 0 || (selection != "极大" && selection != "极小") {
			return apperrors.NewBusinessError("INVALID_REQUEST", "极值玩法只能选择极大或极小")
		}
	case pc28ColorRed:
		return pc28ValidateFixedChoice(position, selection, "红波")
	case pc28ColorGreen:
		return pc28ValidateFixedChoice(position, selection, "绿波")
	case pc28ColorBlue:
		return pc28ValidateFixedChoice(position, selection, "蓝波")
	case pc28Leopard:
		return pc28ValidateFixedChoice(position, selection, "豹子")
	case pc28Pair:
		return pc28ValidateFixedChoice(position, selection, "对子")
	case pc28Straight:
		return pc28ValidateFixedChoice(position, selection, "顺子")
	}
	return nil
}

func pc28ValidateFixedChoice(position int, actual, expected string) error {
	if position != 0 || strings.TrimSpace(actual) != expected {
		return apperrors.NewBusinessError("INVALID_REQUEST", "玩法与投注选择不一致")
	}
	return nil
}

func pc28IsExactPlay(code string) bool {
	_, _, ok := pc28ExactPair(code)
	return ok
}

func pc28IsTwoSidedPlay(code string) bool {
	code = strings.ToLower(strings.TrimSpace(code))
	return code == pc28SumSize || code == pc28SumParity
}

func pc28IsCombinationPlay(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case pc28ComboBigOdd, pc28ComboBigEven, pc28ComboSmallOdd, pc28ComboSmallEven:
		return true
	default:
		return false
	}
}

func pc28OppositeSelection(code, selection string) string {
	selection = pc28NormalizeSelection(code, selection)
	if code == pc28SumSize {
		if selection == "大" {
			return "小"
		}
		if selection == "小" {
			return "大"
		}
	}
	if code == pc28SumParity {
		if selection == "单" {
			return "双"
		}
		if selection == "双" {
			return "单"
		}
	}
	return ""
}

func validatePC28PlacementConstraints(db *gorm.DB, profile gameRuleProfile, workspaceID uint64, roomScope, gameID, issue string, userID uint64, entries []betLimitEntry) error {
	if profile.PC28 == 0 || len(entries) == 0 {
		return nil
	}
	roomScope = defaultString(strings.TrimSpace(roomScope), "legacy")
	pointSelections := map[string]struct{}{}
	marketSelections := map[string]map[string]struct{}{}
	for _, entry := range entries {
		code := strings.ToLower(strings.TrimSpace(entry.PlayCode))
		selection := pc28NormalizeSelection(code, entry.Selection)
		if pc28IsExactPlay(code) {
			pointSelections[selection] = struct{}{}
		}
		if profile.PC28 <= 2 && pc28IsTwoSidedPlay(code) {
			if marketSelections[code] == nil {
				marketSelections[code] = map[string]struct{}{}
			}
			marketSelections[code][selection] = struct{}{}
		}
	}
	if len(pointSelections) > 0 {
		var existing []bet.Bet
		if err := db.Select("play_code", "selection").Where(
			"workspace_id = ? AND room_scope = ? AND game_id = ? AND issue = ? AND user_id = ? AND status = ? AND play_code LIKE ?",
			workspaceID, roomScope, gameID, issue, userID, "pending", pc28ExactPrefix+"%",
		).Find(&existing).Error; err != nil {
			return err
		}
		for _, row := range existing {
			if pc28IsExactPlay(row.PlayCode) {
				pointSelections[pc28NormalizeSelection(row.PlayCode, row.Selection)] = struct{}{}
			}
		}
		if len(pointSelections) > 10 {
			return apperrors.NewBusinessError("POINT_LIMIT_EXCEEDED", "同一会员每期最多投注10个不同的PC28单点和值")
		}
	}
	// Only the two aggregate sum markets are mutually exclusive in variants
	// one and two. Position-level size/parity and first-vs-third dragon/tiger
	// are distinct markets and deliberately remain combinable.
	if profile.PC28 <= 2 && len(marketSelections) > 0 {
		var existing []bet.Bet
		if err := db.Select("play_code", "selection").Where(
			"workspace_id = ? AND room_scope = ? AND game_id = ? AND issue = ? AND user_id = ? AND status = ? AND play_code IN ?",
			workspaceID, roomScope, gameID, issue, userID, "pending", []string{pc28SumSize, pc28SumParity},
		).Find(&existing).Error; err != nil {
			return err
		}
		for _, row := range existing {
			code := strings.ToLower(strings.TrimSpace(row.PlayCode))
			if marketSelections[code] == nil {
				marketSelections[code] = map[string]struct{}{}
			}
			marketSelections[code][pc28NormalizeSelection(code, row.Selection)] = struct{}{}
		}
		for code, selections := range marketSelections {
			for selection := range selections {
				if opposite := pc28OppositeSelection(code, selection); opposite != "" {
					if _, exists := selections[opposite]; exists {
						return apperrors.NewBusinessError("OPPOSITE_BET_NOT_ALLOWED", "玩法一、二禁止同一期对同一和值两面市场反向下注")
					}
				}
			}
		}
	}
	return nil
}

func pc28Color(total int) string {
	if total == 0 || total == 13 || total == 14 || total == 27 {
		return "gray"
	}
	red := map[int]struct{}{3: {}, 6: {}, 9: {}, 12: {}, 15: {}, 18: {}, 21: {}, 24: {}}
	green := map[int]struct{}{1: {}, 4: {}, 7: {}, 10: {}, 16: {}, 19: {}, 22: {}, 25: {}}
	if _, ok := red[total]; ok {
		return "red"
	}
	if _, ok := green[total]; ok {
		return "green"
	}
	return "blue"
}

func pc28Shape(numbers []int) string {
	if len(numbers) != 3 {
		return ""
	}
	if numbers[0] == numbers[1] && numbers[1] == numbers[2] {
		return pc28Leopard
	}
	if numbers[0] == numbers[1] || numbers[0] == numbers[2] || numbers[1] == numbers[2] {
		return pc28Pair
	}
	values := append([]int(nil), numbers...)
	sort.Ints(values)
	if values[1] == values[0]+1 && values[2] == values[1]+1 {
		return pc28Straight
	}
	return ""
}

func evaluatePC28Bet(profile gameRuleProfile, numbers []int, code string, position int, selection string, grayPush bool) (markSixBetOutcome, string, error) {
	if err := profile.validateDraw(numbers); err != nil {
		return "", "", err
	}
	code = strings.ToLower(strings.TrimSpace(code))
	selection = pc28NormalizeSelection(code, selection)
	if err := pc28ValidateChoice(code, position, selection); err != nil {
		return "", "", err
	}
	total := sumInts(numbers)
	won := false
	if _, _, exact := pc28ExactPair(code); exact {
		value, _ := strconv.Atoi(selection)
		won = total == value
	} else {
		switch code {
		case pc28PackageThree:
			values, _ := pc28ParsePackage(selection)
			for _, value := range values {
				won = won || value == total
			}
		case pc28PositionNumber:
			value, _ := strconv.Atoi(selection)
			won = numbers[position-1] == value
		case pc28PositionTwoSided:
			won = matchRuleSide(numbers[position-1], 5, selection)
		case pc28DragonTiger, pc28DragonTigerTie:
			outcome := "tie"
			if numbers[0] > numbers[2] {
				outcome = "dragon"
			} else if numbers[0] < numbers[2] {
				outcome = "tiger"
			}
			won = normalizeSelection(selection) == outcome
		case pc28SumSize:
			won = matchRuleSide(total, 14, selection)
		case pc28SumParity:
			won = matchRuleSide(total, 14, selection)
		case pc28ComboBigOdd:
			won = total >= 14 && total%2 == 1
		case pc28ComboBigEven:
			won = total >= 14 && total%2 == 0
		case pc28ComboSmallOdd:
			won = total < 14 && total%2 == 1
		case pc28ComboSmallEven:
			won = total < 14 && total%2 == 0
		case pc28Extreme:
			won = selection == "极小" && total <= 5 || selection == "极大" && total >= 22
		case pc28ColorRed, pc28ColorGreen, pc28ColorBlue:
			color := pc28Color(total)
			if color == "gray" && grayPush {
				return markSixOutcomePush, fmt.Sprintf("和值%d为灰波，按下注时房间设置返本", total), nil
			}
			wanted := map[string]string{pc28ColorRed: "red", pc28ColorGreen: "green", pc28ColorBlue: "blue"}[code]
			won = color == wanted
		case pc28Leopard, pc28Pair, pc28Straight:
			won = pc28Shape(numbers) == code
		}
	}
	return markSixWonLost(won), fmt.Sprintf("三球%d、%d、%d，和值%d", numbers[0], numbers[1], numbers[2], total), nil
}

type pc28SettlementDecision struct {
	Outcome             markSixBetOutcome
	Reason              string
	EffectiveOdds       float64
	ValidTurnoverCents  int64
	UserIssueStakeCents int64
	Policy              string
}

func decidePC28Settlement(gameID string, item bet.Bet, numbers []int, userIssueStakeCents int64) (pc28SettlementDecision, error) {
	profile, found := rulesForVersion(item.RuleVersion)
	if !found || profile.PC28 == 0 || !gameSupportsRuleVersion(gameID, item.RuleVersion) {
		return pc28SettlementDecision{}, apperrors.NewBusinessError("RULES_NOT_READY", "PC28注单规则版本未确认或与彩种不一致，暂不能结算")
	}
	outcome, reason, err := evaluatePC28Bet(profile, numbers, item.PlayCode, item.Position, item.Selection, item.PC28GrayPush)
	if err != nil {
		return pc28SettlementDecision{}, err
	}
	decision := pc28SettlementDecision{
		Outcome: outcome, Reason: reason, EffectiveOdds: item.Odds,
		ValidTurnoverCents: item.AmountCents, UserIssueStakeCents: userIssueStakeCents,
		Policy: "pc28_standard",
	}
	if outcome == markSixOutcomePush {
		decision.EffectiveOdds = 1
		decision.ValidTurnoverCents = 0
		decision.Policy = "pc28_gray_push"
		return decision, nil
	}
	total := sumInts(numbers)
	if total != 13 && total != 14 {
		return decision, nil
	}
	if profile.PC28 == 1 {
		// Variant one makes the entire issue invalid turnover at 13/14, even
		// when this particular ticket loses. Actual stake/GGR remain untouched.
		decision.ValidTurnoverCents = 0
	}
	gtOne := userIssueStakeCents > 100
	gt9999 := userIssueStakeCents > 999900
	if pc28IsTwoSidedPlay(item.PlayCode) && gtOne {
		switch profile.PC28 {
		case 1, 2:
			decision.EffectiveOdds = 1.5
			decision.Policy = fmt.Sprintf("pc28_v%d_13_14_two_sided_gt1", profile.PC28)
			if gt9999 {
				decision.EffectiveOdds = 1
				decision.Policy = fmt.Sprintf("pc28_v%d_13_14_two_sided_gt9999", profile.PC28)
			}
		case 3:
			decision.EffectiveOdds = 1.98
			decision.Policy = "pc28_v3_13_14_two_sided_gt1"
		}
	}
	if pc28IsCombinationPlay(item.PlayCode) {
		switch profile.PC28 {
		case 1:
			if gtOne {
				decision.EffectiveOdds = 1
				decision.Policy = "pc28_v1_13_14_combo_gt1"
			}
		case 2:
			if gtOne {
				decision.Outcome = markSixOutcomeLost
				decision.EffectiveOdds = 0
				decision.Policy = "pc28_v2_13_14_combo_dealer_take"
				decision.Reason += "；玩法二总注大于1时13/14组合庄家通吃"
			}
		case 3:
			if gtOne {
				decision.EffectiveOdds = 3.65
				decision.Policy = "pc28_v3_13_14_combo_gt1"
			}
		}
	}
	return decision, nil
}
