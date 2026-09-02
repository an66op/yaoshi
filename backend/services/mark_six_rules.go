package services

import (
	apperrors "backend/errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	markSixLegacyRuleVersion = "mark6-v1"
	markSixRuleVersion       = "mark6-v2"
)

type markSixPlaySpec struct {
	Play         defaultPlay
	PositionMode string // none, special, regular
	Selection    string // number, list, choice
	ListCount    int
	Choices      []string
	Kind         string
	Value        string
}

func markSixSpec(code, name, category, description, example string, odds float64, positionMode, selection, kind string, choices ...string) markSixPlaySpec {
	return markSixPlaySpec{Play: defaultPlay{Code: code, Name: name, Category: category, Description: description, Example: example, Odds: odds}, PositionMode: positionMode, Selection: selection, Choices: choices, Kind: kind}
}

var markSixV1Specs = []markSixPlaySpec{
	markSixSpec("marksix_special_a_number", "特码A", "特码", "第7球开出所选号码；48为首次后台默认赔率，可由后台调整", "49", 48, "special", "number", "special-number"),
	markSixSpec("marksix_special_b_number", "特码B", "特码", "第7球开出所选号码；48为首次后台默认赔率，可由后台调整", "49", 48, "special", "number", "special-number"),
	markSixSpec("marksix_regular_number", "正码", "正码", "所选号码出现在前6个正码中的任一位置；7为首次后台默认赔率，可由后台调整", "18", 7, "none", "number", "regular-number"),
	markSixSpec("marksix_regular_position_number", "正码1-6", "正码", "指定正码位置开出所选号码；48为首次后台默认赔率，可由后台调整", "第3位/18", 48, "regular", "number", "regular-position-number"),
	markSixSpec("marksix_regular_special_number", "正码特", "正码", "指定正码位置开出所选号码；48为首次后台默认赔率，可由后台调整", "正三特/18", 48, "regular", "number", "regular-position-number"),
	{Play: defaultPlay{Code: "marksix_combo_4_all", Name: "四全中", Category: "连码", Description: "所选4个号码全部出现在前6个正码中；700为首次后台默认赔率，可由后台调整", Example: "1,2,3,4", Odds: 700}, PositionMode: "none", Selection: "list", ListCount: 4, Kind: "regular-all"},
	{Play: defaultPlay{Code: "marksix_combo_3_all", Name: "三全中", Category: "连码", Description: "所选3个号码全部出现在前6个正码中；580为首次后台默认赔率，可由后台调整", Example: "1,2,3", Odds: 580}, PositionMode: "none", Selection: "list", ListCount: 3, Kind: "regular-all"},
	{Play: defaultPlay{Code: "marksix_combo_2_all", Name: "二全中", Category: "连码", Description: "所选2个号码全部出现在前6个正码中；60为首次后台默认赔率，可由后台调整", Example: "1,2", Odds: 60}, PositionMode: "none", Selection: "list", ListCount: 2, Kind: "regular-all"},
	{Play: defaultPlay{Code: "marksix_combo_special_pair", Name: "特串", Category: "连码", Description: "所选2个号码分别命中特码和任一正码；150为首次后台默认赔率，可由后台调整", Example: "1,49", Odds: 150}, PositionMode: "none", Selection: "list", ListCount: 2, Kind: "special-pair"},
	{Play: defaultPlay{Code: "marksix_not_in", Name: "五不中", Category: "自选不中", Description: "所选5个号码均未出现在7个开奖号码中；2为首次后台默认赔率，可由后台调整", Example: "1,2,3,4,5", Odds: 2}, PositionMode: "none", Selection: "list", ListCount: 5, Kind: "not-in"},
}

var markSixV2Specs = buildMarkSixV2Specs()
var markSixLegacyDefaultPlays = markSixPricedPlays(markSixV1Specs)
var markSixDefaultPlays = markSixPricedPlays(markSixV2Specs)

func buildMarkSixV2Specs() []markSixPlaySpec {
	specs := append([]markSixPlaySpec{}, markSixV1Specs...)
	priced := []markSixPlaySpec{
		markSixSpec("marksix_special_big_small", "特大/特小", "两面", "特码1-24为小、25-48为大，49和局返本", "大", 1.98, "special", "choice", "special-big-small", "大", "小"),
		markSixSpec("marksix_special_odd_even", "特单/特双", "两面", "特码按单双结算，49和局返本", "单", 1.98, "special", "choice", "special-odd-even", "单", "双"),
		markSixSpec("marksix_special_sum_big_small", "特合大/特合小", "两面", "特码十位与个位之和1-6为合小、7-12为合大，49和局返本", "合大", 1.98, "special", "choice", "special-sum-big-small", "合大", "合小"),
		markSixSpec("marksix_special_sum_odd_even", "特合单/特合双", "两面", "特码十位与个位之和按单双结算，49和局返本", "合单", 1.98, "special", "choice", "special-sum-odd-even", "合单", "合双"),
		markSixSpec("marksix_special_heaven_earth", "特天肖/特地肖", "两面", "牛兔龙马猴猪为天肖，鼠虎蛇羊鸡狗为地肖；49和局返本", "天肖", 1.98, "special", "choice", "special-heaven-earth", "天肖", "地肖"),
		markSixSpec("marksix_special_front_back", "特前肖/特后肖", "两面", "鼠牛虎兔龙蛇为前肖，马羊猴鸡狗猪为后肖；49和局返本", "前肖", 1.98, "special", "choice", "special-front-back", "前肖", "后肖"),
		markSixSpec("marksix_special_domestic_wild", "特家肖/特野肖", "两面", "牛马羊鸡狗猪为家肖，鼠虎兔龙蛇猴为野肖；49和局返本", "家肖", 1.98, "special", "choice", "special-domestic-wild", "家肖", "野肖"),
		markSixSpec("marksix_special_tail_big_small", "特尾大/特尾小", "两面", "特码尾数0-4为尾小、5-9为尾大，49和局返本", "尾大", 1.98, "special", "choice", "special-tail-big-small", "尾大", "尾小"),
		markSixSpec("marksix_total_odd_even", "总和单双", "两面", "7个开奖号码之和按单双结算", "总和单", 1.98, "none", "choice", "total-odd-even", "总和单", "总和双"),
		markSixSpec("marksix_total_big_small", "总和大小", "两面", "7个开奖号码之和175及以上为大、174及以下为小", "总和大", 1.98, "none", "choice", "total-big-small", "总和大", "总和小"),
		markSixSpec("marksix_special_half", "特码半特", "两面", "特码大小与单双组合；49不中奖", "大单", 3.72, "special", "choice", "special-half", "大单", "大双", "小单", "小双"),
		markSixSpec("marksix_regular_position_big_small", "正码1-6大小", "正码1-6", "指定正码位1-24为小、25-48为大，49和局返本", "第1位/大", 1.98, "regular", "choice", "regular-big-small", "大", "小"),
		markSixSpec("marksix_regular_position_odd_even", "正码1-6单双", "正码1-6", "指定正码位按单双结算，49和局返本", "第1位/单", 1.98, "regular", "choice", "regular-odd-even", "单", "双"),
		markSixSpec("marksix_regular_position_sum_big_small", "正码1-6合数大小", "正码1-6", "指定正码位十位与个位之和1-6为小、7-12为大，49和局返本", "第1位/合大", 1.98, "regular", "choice", "regular-sum-big-small", "合大", "合小"),
		markSixSpec("marksix_regular_position_sum_odd_even", "正码1-6合数单双", "正码1-6", "指定正码位十位与个位之和按单双结算，49和局返本", "第1位/合单", 1.98, "regular", "choice", "regular-sum-odd-even", "合单", "合双"),
		markSixSpec("marksix_regular_position_tail_big_small", "正码1-6尾数大小", "正码1-6", "指定正码位尾数0-4为小、5-9为大，49和局返本", "第1位/尾大", 1.98, "regular", "choice", "regular-tail-big-small", "尾大", "尾小"),
	}
	specs = append(specs, priced...)
	// Zodiac is symmetric, but no reference price was supplied. It is exposed
	// to the odds editor with zero until an administrator explicitly prices it.
	specs = append(specs, markSixSpec("marksix_special_zodiac", "特码生肖", "特肖头尾数", "按该期开奖日所属农历生肖年映射特码；49按当年生肖正常参与。赔率需后台配置", "猴", 0, "special", "choice", "special-zodiac", markSixZodiacNames...))
	colors := []struct{ code, label string }{{"red", "红"}, {"blue", "蓝"}, {"green", "绿"}}
	for _, color := range colors {
		specs = append(specs, atomicMarkSixSpec("marksix_color_wave_"+color.code, color.label+"波", "色波", "special", "color", color.code, color.label+"波"))
		for _, side := range []struct{ code, label string }{{"big", "大"}, {"small", "小"}, {"odd", "单"}, {"even", "双"}} {
			specs = append(specs, atomicMarkSixSpec("marksix_half_wave_"+color.code+"_"+side.code, color.label+side.label, "色波半波", "special", "half-wave", color.code+":"+side.code, color.label+side.label))
		}
		for _, size := range []struct{ code, label string }{{"big", "大"}, {"small", "小"}} {
			for _, parity := range []struct{ code, label string }{{"odd", "单"}, {"even", "双"}} {
				label := color.label + size.label + parity.label
				specs = append(specs, atomicMarkSixSpec("marksix_halfhalf_"+color.code+"_"+size.code+"_"+parity.code, label, "色波半半波", "special", "half-half-wave", color.code+":"+size.code+":"+parity.code, label))
			}
		}
		specs = append(specs, atomicMarkSixSpec("marksix_regular_color_"+color.code, "正码1-6"+color.label+"波", "正码1-6", "regular", "regular-color", color.code, color.label+"波"))
	}
	for head := 0; head <= 4; head++ {
		label := strconv.Itoa(head) + "头"
		specs = append(specs, atomicMarkSixSpec(fmt.Sprintf("marksix_special_head_%d", head), "特码"+label, "特肖头尾数", "special", "head", strconv.Itoa(head), label))
	}
	for tail := 0; tail <= 9; tail++ {
		label := strconv.Itoa(tail) + "尾"
		specs = append(specs, atomicMarkSixSpec(fmt.Sprintf("marksix_special_tail_%d", tail), "特码"+label, "特肖头尾数", "special", "tail", strconv.Itoa(tail), label))
	}
	for _, element := range []struct{ code, label string }{{"metal", "金"}, {"wood", "木"}, {"water", "水"}, {"fire", "火"}, {"earth", "土"}} {
		specs = append(specs, atomicMarkSixSpec("marksix_five_element_"+element.code, "五行"+element.label, "五行", "special", "element", element.code, element.label))
	}
	return specs
}

func atomicMarkSixSpec(code, name, category, positionMode, kind, value, choice string) markSixPlaySpec {
	return markSixPlaySpec{Play: defaultPlay{Code: code, Name: name, Category: category, Description: name + "原子选项；赔率需后台明确配置", Example: choice}, PositionMode: positionMode, Selection: "choice", Choices: []string{choice}, Kind: kind, Value: value}
}

func markSixPricedPlays(specs []markSixPlaySpec) []defaultPlay {
	plays := make([]defaultPlay, 0, len(specs))
	for _, spec := range specs {
		if spec.Play.Odds > 1 {
			plays = append(plays, spec.Play)
		}
	}
	return plays
}

func markSixSpecsForVersion(version string) []markSixPlaySpec {
	if version == markSixLegacyRuleVersion {
		return markSixV1Specs
	}
	return markSixV2Specs
}

func markSixSpecByCode(version, code string) (markSixPlaySpec, bool) {
	code = strings.ToLower(strings.TrimSpace(code))
	for _, spec := range markSixSpecsForVersion(version) {
		if spec.Play.Code == code {
			return spec, true
		}
	}
	return markSixPlaySpec{}, false
}

func markSixPlayByCodeForVersion(version, code string) (defaultPlay, bool) {
	spec, ok := markSixSpecByCode(version, code)
	return spec.Play, ok
}

func markSixPlayByCode(code string) (defaultPlay, bool) {
	return markSixPlayByCodeForVersion(markSixRuleVersion, code)
}

func markSixPlayCatalog() []PlayCatalogItem {
	items := make([]PlayCatalogItem, 0, len(markSixV2Specs))
	for index, spec := range markSixV2Specs {
		play := spec.Play
		items = append(items, PlayCatalogItem{PlayCode: play.Code, PlayName: play.Name, Category: play.Category, Description: play.Description, Example: play.Example, DefaultOdds: play.Odds, SortOrder: index})
	}
	return items
}

func markSixNormalizeSelection(playCode, selection string) string {
	playCode = strings.ToLower(strings.TrimSpace(playCode))
	selection = strings.TrimSpace(strings.ReplaceAll(selection, "，", ","))
	spec, ok := markSixSpecByCode(markSixRuleVersion, playCode)
	if !ok {
		return selection
	}
	switch spec.Selection {
	case "number":
		if value, err := strconv.Atoi(selection); err == nil {
			return strconv.Itoa(value)
		}
	case "list":
		if values, parsed := parseMarkSixIntList(selection); parsed {
			sort.Ints(values)
			return joinNumbers(values)
		}
	case "choice":
		selection = canonicalMarkSixSelection(selection)
		for _, choice := range spec.Choices {
			if selection == choice || strings.TrimSuffix(selection, "波") == strings.TrimSuffix(choice, "波") {
				return choice
			}
		}
	}
	return selection
}

func canonicalMarkSixSelection(selection string) string {
	switch strings.ToLower(strings.TrimSpace(selection)) {
	case "big", "特大":
		return "大"
	case "small", "特小":
		return "小"
	case "odd", "特单":
		return "单"
	case "even", "特双":
		return "双"
	case "domestic", "家禽", "特家肖":
		return "家肖"
	case "wild", "野兽", "特野肖":
		return "野肖"
	default:
		return strings.TrimSpace(selection)
	}
}

func markSixValidateChoiceForVersion(version, playCode string, position int, selection string) error {
	spec, ok := markSixSpecByCode(version, playCode)
	if !ok {
		return apperrors.NewBusinessError("INVALID_REQUEST", "宾果六合彩当前不支持该玩法")
	}
	selection = strings.TrimSpace(selection)
	if normalized := markSixNormalizeSelection(spec.Play.Code, selection); normalized != selection {
		return apperrors.NewBusinessError("INVALID_REQUEST", "投注内容格式不正确")
	}
	switch spec.PositionMode {
	case "special":
		if position != 7 {
			return apperrors.NewBusinessError("INVALID_REQUEST", "特码玩法位置必须为7")
		}
	case "regular":
		if position < 1 || position > 6 {
			return apperrors.NewBusinessError("INVALID_REQUEST", "正码位置只能选择1至6")
		}
	default:
		if position != 0 {
			return apperrors.NewBusinessError("INVALID_REQUEST", "该玩法位置必须为0")
		}
	}
	switch spec.Selection {
	case "number":
		return requireIntegerRange(selection, 1, 49, "号码只能选择1至49")
	case "list":
		return requireMarkSixIntList(selection, spec.ListCount, fmt.Sprintf("该玩法必须选择%d个不同号码", spec.ListCount))
	case "choice":
		for _, choice := range spec.Choices {
			if selection == choice {
				return nil
			}
		}
	}
	return apperrors.NewBusinessError("INVALID_REQUEST", "投注选项不正确")
}

func markSixValidateChoice(playCode string, position int, selection string) error {
	return markSixValidateChoiceForVersion(markSixRuleVersion, playCode, position, selection)
}

func requireIntegerRange(selection string, min, max int, message string) error {
	value, err := strconv.Atoi(selection)
	if err != nil || value < min || value > max || strconv.Itoa(value) != selection {
		return apperrors.NewBusinessError("INVALID_REQUEST", message)
	}
	return nil
}

func parseMarkSixIntList(selection string) ([]int, bool) {
	parts := strings.Split(selection, ",")
	if len(parts) == 0 {
		return nil, false
	}
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		value, err := strconv.Atoi(part)
		if err != nil || strconv.Itoa(value) != part {
			return nil, false
		}
		values = append(values, value)
	}
	return values, true
}

func requireMarkSixIntList(selection string, count int, message string) error {
	values, ok := parseMarkSixIntList(selection)
	if !ok || len(values) != count {
		return apperrors.NewBusinessError("INVALID_REQUEST", message)
	}
	for index, value := range values {
		if value < 1 || value > 49 || (index > 0 && values[index-1] >= value) {
			return apperrors.NewBusinessError("INVALID_REQUEST", message)
		}
	}
	return nil
}

type markSixBetOutcome string

const (
	markSixOutcomeWon  markSixBetOutcome = "won"
	markSixOutcomeLost markSixBetOutcome = "lost"
	markSixOutcomePush markSixBetOutcome = "push"
)

func evaluateMarkSixBetForVersion(version string, numbers []int, playCode string, position int, selection string, drawAt time.Time) (markSixBetOutcome, string, error) {
	if err := markSixValidateChoiceForVersion(version, playCode, position, selection); err != nil {
		return "", "", err
	}
	spec, _ := markSixSpecByCode(version, playCode)
	special, regular := numbers[6], numbers[:6]
	switch spec.Kind {
	case "special-number":
		selected, _ := strconv.Atoi(selection)
		return markSixWonLost(selected == special), fmt.Sprintf("特码开出 %d", special), nil
	case "regular-number":
		selected, _ := strconv.Atoi(selection)
		return markSixWonLost(containsInt(regular, selected)), fmt.Sprintf("正码为 %s", joinNumbers(regular)), nil
	case "regular-position-number":
		selected, _ := strconv.Atoi(selection)
		actual := regular[position-1]
		return markSixWonLost(selected == actual), fmt.Sprintf("正码%d开出 %d", position, actual), nil
	case "regular-all":
		selected, _ := parseMarkSixIntList(selection)
		for _, number := range selected {
			if !containsInt(regular, number) {
				return markSixOutcomeLost, "所选连码未全部出现在正码", nil
			}
		}
		return markSixOutcomeWon, "所选连码全部命中正码", nil
	case "special-pair":
		selected, _ := parseMarkSixIntList(selection)
		if !containsInt(selected, special) {
			return markSixOutcomeLost, "所选号码未命中特码", nil
		}
		for _, number := range selected {
			if number != special && containsInt(regular, number) {
				return markSixOutcomeWon, "所选号码分别命中特码和正码", nil
			}
		}
		return markSixOutcomeLost, "所选号码未同时命中特码和正码", nil
	case "not-in":
		selected, _ := parseMarkSixIntList(selection)
		for _, number := range selected {
			if containsInt(numbers, number) {
				return markSixOutcomeLost, fmt.Sprintf("所选不中号码%d已开出", number), nil
			}
		}
		return markSixOutcomeWon, "所选五个号码均未开出", nil
	case "total-odd-even":
		total := sumInts(numbers)
		return markSixWonLost((total%2 == 1) == (selection == "总和单")), fmt.Sprintf("总和 %d", total), nil
	case "total-big-small":
		total := sumInts(numbers)
		return markSixWonLost((total >= 175) == (selection == "总和大")), fmt.Sprintf("总和 %d", total), nil
	case "special-zodiac", "special-heaven-earth", "special-front-back", "special-domestic-wild":
		if spec.Kind != "special-zodiac" && special == 49 {
			return markSixOutcomePush, "特码49，和局返还本金", nil
		}
		zodiac, err := markSixNumberZodiac(special, drawAt)
		if err != nil {
			return "", "", err
		}
		won := zodiac == selection
		if spec.Kind == "special-heaven-earth" {
			won = containsString([]string{"牛", "兔", "龙", "马", "猴", "猪"}, zodiac) == (selection == "天肖")
		} else if spec.Kind == "special-front-back" {
			won = containsString([]string{"鼠", "牛", "虎", "兔", "龙", "蛇"}, zodiac) == (selection == "前肖")
		} else if spec.Kind == "special-domestic-wild" {
			won = containsString([]string{"牛", "马", "羊", "鸡", "狗", "猪"}, zodiac) == (selection == "家肖")
		}
		return markSixWonLost(won), fmt.Sprintf("特码 %d 属%s", special, zodiac), nil
	case "special-big-small", "special-odd-even", "special-sum-big-small", "special-sum-odd-even", "special-tail-big-small":
		return evaluateMarkSixSide(special, spec.Kind, selection, "特码", true)
	case "special-half":
		return evaluateMarkSixSide(special, spec.Kind, selection, "特码", false)
	case "regular-big-small", "regular-odd-even", "regular-sum-big-small", "regular-sum-odd-even", "regular-tail-big-small":
		return evaluateMarkSixSide(regular[position-1], spec.Kind, selection, fmt.Sprintf("正码%d", position), true)
	case "color", "half-wave", "half-half-wave", "head", "tail", "element":
		if (spec.Kind == "half-wave" || spec.Kind == "half-half-wave") && special == 49 {
			return markSixOutcomePush, "特码49，和局返还本金", nil
		}
		return markSixWonLost(evaluateMarkSixAtomic(special, spec)), fmt.Sprintf("特码开出 %d", special), nil
	case "regular-color":
		actual := regular[position-1]
		return markSixWonLost(markSixColor(actual) == spec.Value), fmt.Sprintf("正码%d开出 %d", position, actual), nil
	default:
		return "", "", apperrors.NewBusinessError("RULES_NOT_READY", "注单玩法尚未建模，暂不能结算")
	}
}

func evaluateMarkSixSide(number int, kind, selection, label string, push49 bool) (markSixBetOutcome, string, error) {
	if push49 && number == 49 {
		return markSixOutcomePush, label + "开出49，和局返还本金", nil
	}
	digitSum := number/10 + number%10
	won := false
	switch kind {
	case "special-big-small", "regular-big-small":
		won = (number >= 25) == (selection == "大")
	case "special-odd-even", "regular-odd-even":
		won = (number%2 == 1) == (selection == "单")
	case "special-sum-big-small", "regular-sum-big-small":
		won = (digitSum >= 7) == (selection == "合大")
	case "special-sum-odd-even", "regular-sum-odd-even":
		won = (digitSum%2 == 1) == (selection == "合单")
	case "special-tail-big-small", "regular-tail-big-small":
		won = (number%10 >= 5) == (selection == "尾大")
	case "special-half":
		wantBig, wantOdd := strings.HasPrefix(selection, "大"), strings.HasSuffix(selection, "单")
		won = number != 49 && (number >= 25) == wantBig && (number%2 == 1) == wantOdd
	default:
		return "", "", apperrors.NewBusinessError("RULES_NOT_READY", "两面玩法尚未建模")
	}
	return markSixWonLost(won), fmt.Sprintf("%s开出 %d", label, number), nil
}

func evaluateMarkSixAtomic(number int, spec markSixPlaySpec) bool {
	parts := strings.Split(spec.Value, ":")
	switch spec.Kind {
	case "color":
		return markSixColor(number) == spec.Value
	case "half-wave":
		return len(parts) == 2 && markSixColor(number) == parts[0] && markSixSideMatch(number, parts[1])
	case "half-half-wave":
		return len(parts) == 3 && markSixColor(number) == parts[0] && markSixSideMatch(number, parts[1]) && markSixSideMatch(number, parts[2])
	case "head":
		value, _ := strconv.Atoi(spec.Value)
		return number/10 == value
	case "tail":
		value, _ := strconv.Atoi(spec.Value)
		return number%10 == value
	case "element":
		return containsInt(markSixFiveElements[spec.Value], number)
	default:
		return false
	}
}

func markSixSideMatch(number int, side string) bool {
	switch side {
	case "big":
		return number >= 25
	case "small":
		return number <= 24
	case "odd":
		return number%2 == 1
	case "even":
		return number%2 == 0
	default:
		return false
	}
}

func markSixWonLost(won bool) markSixBetOutcome {
	if won {
		return markSixOutcomeWon
	}
	return markSixOutcomeLost
}

func markSixColor(number int) string {
	if containsInt(markSixRedNumbers, number) {
		return "red"
	}
	if containsInt(markSixBlueNumbers, number) {
		return "blue"
	}
	if containsInt(markSixGreenNumbers, number) {
		return "green"
	}
	return ""
}

var (
	markSixRedNumbers   = []int{1, 2, 7, 8, 12, 13, 18, 19, 23, 24, 29, 30, 34, 35, 40, 45, 46}
	markSixBlueNumbers  = []int{3, 4, 9, 10, 14, 15, 20, 25, 26, 31, 36, 37, 41, 42, 47, 48}
	markSixGreenNumbers = []int{5, 6, 11, 16, 17, 21, 22, 27, 28, 32, 33, 38, 39, 43, 44, 49}
	markSixFiveElements = map[string][]int{
		"metal": {6, 7, 20, 21, 28, 29, 36, 37},
		"wood":  {2, 3, 10, 11, 18, 19, 32, 33, 40, 41, 48, 49},
		"water": {8, 9, 16, 17, 24, 25, 38, 39, 46, 47},
		"fire":  {4, 5, 12, 13, 26, 27, 34, 35, 42, 43},
		"earth": {1, 14, 15, 22, 23, 30, 31, 44, 45},
	}
	markSixZodiacNames = []string{"鼠", "牛", "虎", "兔", "龙", "蛇", "马", "羊", "猴", "鸡", "狗", "猪"}
)

var markSixLunarNewYear = map[int]time.Time{
	2019: shanghaiDate(2019, 2, 5), 2020: shanghaiDate(2020, 1, 25), 2021: shanghaiDate(2021, 2, 12),
	2022: shanghaiDate(2022, 2, 1), 2023: shanghaiDate(2023, 1, 22), 2024: shanghaiDate(2024, 2, 10),
	2025: shanghaiDate(2025, 1, 29), 2026: shanghaiDate(2026, 2, 17), 2027: shanghaiDate(2027, 2, 6),
	2028: shanghaiDate(2028, 1, 26), 2029: shanghaiDate(2029, 2, 13), 2030: shanghaiDate(2030, 2, 3),
	2031: shanghaiDate(2031, 1, 23), 2032: shanghaiDate(2032, 2, 11), 2033: shanghaiDate(2033, 1, 31),
	2034: shanghaiDate(2034, 2, 19), 2035: shanghaiDate(2035, 2, 8), 2036: shanghaiDate(2036, 1, 28),
	2037: shanghaiDate(2037, 2, 15), 2038: shanghaiDate(2038, 2, 4), 2039: shanghaiDate(2039, 1, 24),
	2040: shanghaiDate(2040, 2, 12),
}

func shanghaiDate(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
}

func markSixNumberZodiac(number int, drawAt time.Time) (string, error) {
	if number < 1 || number > 49 || drawAt.IsZero() {
		return "", apperrors.NewBusinessError("RULES_NOT_READY", "生肖结算需要有效的开奖时间")
	}
	local := drawAt.In(time.FixedZone("Asia/Shanghai", 8*60*60))
	year := local.Year()
	boundary, ok := markSixLunarNewYear[year]
	if !ok {
		return "", apperrors.NewBusinessError("RULES_NOT_READY", "该开奖年份的生肖边界尚未配置")
	}
	if local.Before(boundary) {
		year--
	}
	if _, ok := markSixLunarNewYear[year]; !ok {
		return "", apperrors.NewBusinessError("RULES_NOT_READY", "该开奖年份的生肖边界尚未配置")
	}
	yearIndex := positiveMod(year-2020, 12)
	return markSixZodiacNames[positiveMod(yearIndex-(number-1), 12)], nil
}

func positiveMod(value, modulus int) int {
	value %= modulus
	if value < 0 {
		value += modulus
	}
	return value
}

func containsInt(values []int, wanted int) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
