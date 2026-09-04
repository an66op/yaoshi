package services

import (
	"backend/data/models/bet"
	apperrors "backend/errors"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// markSixRuleVersion is the immutable Bingo contract. The four direct
	// Mark Six products intentionally use distinct identities even though they
	// currently delegate to the same verified catalogue and evaluator.
	markSixRuleVersion         = "mark6-v2"
	hongKongMarkSixRuleVersion = "hk-mark6-v1"
	happy8MarkSixRuleVersion   = "happy8-mark6-v1"
	newMacauMarkSixRuleVersion = "new-macau-mark6-v1"
	oldMacauMarkSixRuleVersion = "old-macau-mark6-v1"
)

func isMarkSixRuleVersion(version string) bool {
	switch strings.TrimSpace(version) {
	case markSixRuleVersion, hongKongMarkSixRuleVersion, happy8MarkSixRuleVersion, newMacauMarkSixRuleVersion, oldMacauMarkSixRuleVersion:
		return true
	default:
		return false
	}
}

type markSixPlaySpec struct {
	Play         defaultPlay
	PositionMode string // none, special, regular
	Selection    string // number, list, choice
	ListCount    int
	Choices      []string
	Kind         string
	Value        string
	// PricingOnly rows are administrator-facing prices used by one composite
	// ticket and can never be submitted as independent bets. HiddenFromCatalog
	// parent rows are bettable but obtain their one or more prices from those
	// explicit pricing rows.
	PricingOnly       bool
	HiddenFromCatalog bool
}

func markSixSpec(code, name, category, description, example string, positionMode, selection, kind string, choices ...string) markSixPlaySpec {
	return markSixPlaySpec{Play: defaultPlay{Code: code, Name: name, Category: category, Description: description, Example: example}, PositionMode: positionMode, Selection: selection, Choices: choices, Kind: kind}
}

var markSixCoreSpecs = []markSixPlaySpec{
	markSixSpec("marksix_special_a_number", "特码A", "特码", "第7球开出所选号码；赔率需后台配置", "49", "special", "number", "special-number"),
	markSixSpec("marksix_special_b_number", "特码B", "特码", "第7球开出所选号码；赔率需后台配置", "49", "special", "number", "special-number"),
	markSixSpec("marksix_regular_number", "正码", "正码", "所选号码出现在前6个正码中的任一位置；赔率需后台配置", "18", "none", "number", "regular-number"),
	markSixSpec("marksix_regular_position_number", "正码1-6", "正码", "指定正码位置开出所选号码；赔率需后台配置", "第3位/18", "regular", "number", "regular-position-number"),
	markSixSpec("marksix_regular_special_number", "正码特", "正码", "指定正码位置开出所选号码；赔率需后台配置", "正三特/18", "regular", "number", "regular-position-number"),
	{Play: defaultPlay{Code: "marksix_combo_4_all", Name: "四全中", Category: "连码", Description: "所选4个号码全部出现在前6个正码中；赔率需后台配置", Example: "1,2,3,4"}, PositionMode: "none", Selection: "list", ListCount: 4, Kind: "regular-all"},
	{Play: defaultPlay{Code: "marksix_combo_3_all", Name: "三全中", Category: "连码", Description: "所选3个号码全部出现在前6个正码中；赔率需后台配置", Example: "1,2,3"}, PositionMode: "none", Selection: "list", ListCount: 3, Kind: "regular-all"},
	{Play: defaultPlay{Code: "marksix_combo_2_all", Name: "二全中", Category: "连码", Description: "所选2个号码全部出现在前6个正码中；赔率需后台配置", Example: "1,2"}, PositionMode: "none", Selection: "list", ListCount: 2, Kind: "regular-all"},
	{Play: defaultPlay{Code: "marksix_combo_special_pair", Name: "特串", Category: "连码", Description: "所选2个号码分别命中特码和任一正码；赔率需后台配置", Example: "1,49"}, PositionMode: "none", Selection: "list", ListCount: 2, Kind: "special-pair"},
	{Play: defaultPlay{Code: "marksix_not_in", Name: "五不中", Category: "自选不中", Description: "所选5个号码均未出现在7个开奖号码中；赔率需后台配置", Example: "1,2,3,4,5"}, PositionMode: "none", Selection: "list", ListCount: 5, Kind: "not-in"},
	{Play: defaultPlay{Code: "marksix_combo_3_2", Name: "三中二", Category: "连码", Description: "一张3号码注单：恰中2个正码按中二档，中3个正码按中三档；两档赔率均在下注时冻结", Example: "1,2,3"}, PositionMode: "none", Selection: "list", ListCount: 3, Kind: "combo-3-2", HiddenFromCatalog: true},
	{Play: defaultPlay{Code: "marksix_combo_2_special", Name: "二中特", Category: "连码", Description: "一张2号码注单：两正码按中二档，一正码加特码按中特档；两档赔率均在下注时冻结", Example: "1,49"}, PositionMode: "none", Selection: "list", ListCount: 2, Kind: "combo-2-special", HiddenFromCatalog: true},
	{Play: defaultPlay{Code: "marksix_combo_3_2_exact2", Name: "三中二-中二赔率", Category: "连码", Description: "三中二复合注单的恰中2个正码定价；仅供后台配置，不能独立下注", Example: "后台定价"}, PositionMode: "none", Selection: "list", ListCount: 3, Kind: "pricing-only", PricingOnly: true},
	{Play: defaultPlay{Code: "marksix_combo_3_2_exact3", Name: "三中二-中三赔率", Category: "连码", Description: "三中二复合注单的中3个正码定价；仅供后台配置，不能独立下注", Example: "后台定价"}, PositionMode: "none", Selection: "list", ListCount: 3, Kind: "pricing-only", PricingOnly: true},
	{Play: defaultPlay{Code: "marksix_combo_2_special_regular", Name: "二中特-中二赔率", Category: "连码", Description: "二中特复合注单的两正码定价；仅供后台配置，不能独立下注", Example: "后台定价"}, PositionMode: "none", Selection: "list", ListCount: 2, Kind: "pricing-only", PricingOnly: true},
	{Play: defaultPlay{Code: "marksix_combo_2_special_mixed", Name: "二中特-中特赔率", Category: "连码", Description: "二中特复合注单的一正码加特码定价；仅供后台配置，不能独立下注", Example: "后台定价"}, PositionMode: "none", Selection: "list", ListCount: 2, Kind: "pricing-only", PricingOnly: true},
}

var markSixV2Specs = buildMarkSixV2Specs()

func buildMarkSixV2Specs() []markSixPlaySpec {
	specs := append([]markSixPlaySpec{}, markSixCoreSpecs...)
	sideMarkets := []markSixPlaySpec{
		markSixSpec("marksix_special_big_small", "特大/特小", "两面", "特码1-24为小、25-48为大，49和局返本", "大", "special", "choice", "special-big-small", "大", "小"),
		markSixSpec("marksix_special_odd_even", "特单/特双", "两面", "特码按单双结算，49和局返本", "单", "special", "choice", "special-odd-even", "单", "双"),
		markSixSpec("marksix_special_sum_big_small", "特合大/特合小", "两面", "特码十位与个位之和1-6为合小、7-12为合大，49和局返本", "合大", "special", "choice", "special-sum-big-small", "合大", "合小"),
		markSixSpec("marksix_special_sum_odd_even", "特合单/特合双", "两面", "特码十位与个位之和按单双结算，49和局返本", "合单", "special", "choice", "special-sum-odd-even", "合单", "合双"),
		markSixSpec("marksix_special_heaven_earth", "特天肖/特地肖", "两面", "牛兔龙马猴猪为天肖，鼠虎蛇羊鸡狗为地肖；49和局返本", "天肖", "special", "choice", "special-heaven-earth", "天肖", "地肖"),
		markSixSpec("marksix_special_front_back", "特前肖/特后肖", "两面", "鼠牛虎兔龙蛇为前肖，马羊猴鸡狗猪为后肖；49和局返本", "前肖", "special", "choice", "special-front-back", "前肖", "后肖"),
		markSixSpec("marksix_special_domestic_wild", "特家肖/特野肖", "两面", "牛马羊鸡狗猪为家肖，鼠虎兔龙蛇猴为野肖；49和局返本", "家肖", "special", "choice", "special-domestic-wild", "家肖", "野肖"),
		markSixSpec("marksix_special_tail_big_small", "特尾大/特尾小", "两面", "特码尾数0-4为尾小、5-9为尾大，49和局返本", "尾大", "special", "choice", "special-tail-big-small", "尾大", "尾小"),
		markSixSpec("marksix_total_odd_even", "总和单双", "两面", "7个开奖号码之和按单双结算", "总和单", "none", "choice", "total-odd-even", "总和单", "总和双"),
		markSixSpec("marksix_total_big_small", "总和大小", "两面", "7个开奖号码之和175及以上为大、174及以下为小", "总和大", "none", "choice", "total-big-small", "总和大", "总和小"),
		markSixSpec("marksix_special_half", "特码半特", "两面", "特码大小与单双组合；49不中奖", "大单", "special", "choice", "special-half", "大单", "大双", "小单", "小双"),
		markSixSpec("marksix_regular_position_big_small", "正码1-6大小", "正码1-6", "指定正码位1-24为小、25-48为大，49和局返本", "第1位/大", "regular", "choice", "regular-big-small", "大", "小"),
		markSixSpec("marksix_regular_position_odd_even", "正码1-6单双", "正码1-6", "指定正码位按单双结算，49和局返本", "第1位/单", "regular", "choice", "regular-odd-even", "单", "双"),
		markSixSpec("marksix_regular_position_sum_big_small", "正码1-6合数大小", "正码1-6", "指定正码位十位与个位之和1-6为小、7-12为大，49和局返本", "第1位/合大", "regular", "choice", "regular-sum-big-small", "合大", "合小"),
		markSixSpec("marksix_regular_position_sum_odd_even", "正码1-6合数单双", "正码1-6", "指定正码位十位与个位之和按单双结算，49和局返本", "第1位/合单", "regular", "choice", "regular-sum-odd-even", "合单", "合双"),
		markSixSpec("marksix_regular_position_tail_big_small", "正码1-6尾数大小", "正码1-6", "指定正码位尾数0-4为小、5-9为大，49和局返本", "第1位/尾大", "regular", "choice", "regular-tail-big-small", "尾大", "尾小"),
	}
	specs = append(specs, sideMarkets...)
	for _, zodiac := range markSixZodiacOptions {
		specs = append(specs, markSixSpec(
			"marksix_special_zodiac_"+zodiac.code, "特肖"+zodiac.label, "特肖头尾数",
			"按该期开奖日所属农历生肖年映射特码；49按当年生肖正常参与。每个生肖独立配置赔率",
			zodiac.label, "special", "choice", "special-zodiac", zodiac.label,
		))
	}
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
	for _, zodiac := range markSixZodiacOptions {
		specs = append(specs, markSixSpec(
			"marksix_one_zodiac_"+zodiac.code, "一肖"+zodiac.label, "一肖总肖平特尾数",
			"全部7个开奖号码中出现所选生肖即中，出现多次仍只派彩一次；49正常参与",
			zodiac.label, "none", "choice", "one-zodiac", zodiac.label,
		))
	}
	for tail := 0; tail <= 9; tail++ {
		label := strconv.Itoa(tail) + "尾"
		specs = append(specs, atomicMarkSixSpec(
			fmt.Sprintf("marksix_one_tail_%d", tail), "一尾"+label, "一肖总肖平特尾数",
			"none", "one-tail", strconv.Itoa(tail), label,
		))
	}
	for count := 2; count <= 11; count++ {
		label := fmt.Sprintf("%d合肖", count)
		specs = append(specs, markSixPlaySpec{
			Play: defaultPlay{
				Code: fmt.Sprintf("marksix_combined_zodiac_%d", count), Name: label, Category: "合肖",
				Description: fmt.Sprintf("选择%d个不同生肖，特码属于任一所选生肖即中；特码49和局返本", count), Example: markSixZodiacListExample(count),
			},
			PositionMode: "special", Selection: "zodiac-list", ListCount: count, Kind: "combined-zodiac",
		})
	}
	for _, zodiac := range markSixZodiacOptions {
		specs = append(specs, markSixSpec(
			"marksix_regular_zodiac_"+zodiac.code, "正肖"+zodiac.label, "正肖七色波",
			"前6个正码出现所选生肖即中；每多命中一个正码，按下注赔率的净赢部分再增加一份",
			zodiac.label, "none", "choice", "regular-zodiac", zodiac.label,
		))
	}
	for count := 2; count <= 5; count++ {
		specs = append(specs, markSixPlaySpec{
			Play: defaultPlay{
				Code: fmt.Sprintf("marksix_link_zodiac_%d", count), Name: fmt.Sprintf("%d连肖", count), Category: "连肖连尾",
				Description: fmt.Sprintf("选择%d个不同生肖且每个生肖均须在7个开奖号码中出现；服务端从所选生肖的有效赔率中取最低价", count), Example: markSixZodiacListExample(count),
			},
			PositionMode: "none", Selection: "zodiac-list", ListCount: count, Kind: "link-zodiac", HiddenFromCatalog: true,
		})
		specs = append(specs, markSixPlaySpec{
			Play: defaultPlay{
				Code: fmt.Sprintf("marksix_link_tail_%d", count), Name: fmt.Sprintf("%d连尾", count), Category: "连肖连尾",
				Description: fmt.Sprintf("选择%d个不同尾数且每个尾数均须在7个开奖号码中出现；服务端从所选尾数的有效赔率中取最低价", count), Example: markSixTailListExample(count),
			},
			PositionMode: "none", Selection: "tail-list", ListCount: count, Kind: "link-tail", HiddenFromCatalog: true,
		})
		for _, zodiac := range markSixZodiacOptions {
			specs = append(specs, markSixPlaySpec{
				Play: defaultPlay{
					Code: markSixLinkZodiacCode(count, zodiac.code), Name: fmt.Sprintf("%d连肖-%s", count, zodiac.label), Category: "连肖连尾",
					Description: fmt.Sprintf("选择%d个不同生肖且每个生肖均须在7个开奖号码中出现；服务端从所选生肖的有效赔率中取最低价", count), Example: markSixZodiacListExample(count),
				},
				PositionMode: "none", Selection: "zodiac-list", ListCount: count, Kind: "pricing-only", Value: zodiac.label, PricingOnly: true,
			})
		}
		for tail := 0; tail <= 9; tail++ {
			specs = append(specs, markSixPlaySpec{
				Play: defaultPlay{
					Code: markSixLinkTailCode(count, tail), Name: fmt.Sprintf("%d连尾-%d尾", count, tail), Category: "连肖连尾",
					Description: fmt.Sprintf("选择%d个不同尾数且每个尾数均须在7个开奖号码中出现；服务端从所选尾数的有效赔率中取最低价", count), Example: markSixTailListExample(count),
				},
				PositionMode: "none", Selection: "tail-list", ListCount: count, Kind: "pricing-only", Value: strconv.Itoa(tail), PricingOnly: true,
			})
		}
	}
	for count := 6; count <= 11; count++ {
		specs = append(specs, markSixPlaySpec{
			Play: defaultPlay{
				Code: fmt.Sprintf("marksix_not_in_%d", count), Name: fmt.Sprintf("%d不中", count), Category: "自选不中",
				Description: fmt.Sprintf("所选%d个号码均未出现在7个开奖号码中；赔率需后台按数量配置", count), Example: markSixNumberListExample(count),
			},
			PositionMode: "none", Selection: "list", ListCount: count, Kind: "not-in",
		})
	}
	for total := 2; total <= 7; total++ {
		choice := strconv.Itoa(total) + "肖"
		specs = append(specs, markSixSpec(
			fmt.Sprintf("marksix_total_zodiac_%d", total), "总肖"+choice, "一肖总肖平特尾数",
			"全部7个开奖号码覆盖的不同生肖数等于所选数量；49正常参与",
			choice, "none", "choice", "total-zodiac-count", choice,
		))
	}
	for _, parity := range []struct{ code, label string }{{"odd", "总肖单"}, {"even", "总肖双"}} {
		specs = append(specs, markSixSpec(
			"marksix_total_zodiac_"+parity.code, parity.label, "一肖总肖平特尾数",
			"全部7个开奖号码覆盖的不同生肖数按单双结算；49正常参与",
			parity.label, "none", "choice", "total-zodiac-parity", parity.label,
		))
	}
	for _, color := range []struct{ code, label string }{{"red", "红波"}, {"blue", "蓝波"}, {"green", "绿波"}, {"draw", "和局"}} {
		specs = append(specs, markSixSpec(
			"marksix_seven_color_"+color.code, "七色波"+color.label, "正肖七色波",
			"6个正码各1分、特码1.5分，得分最高的颜色中奖；和局时红蓝绿投注返本",
			color.label, "none", "choice", "seven-color-wave", color.label,
		))
	}
	return specs
}

func atomicMarkSixSpec(code, name, category, positionMode, kind, value, choice string) markSixPlaySpec {
	return markSixPlaySpec{Play: defaultPlay{Code: code, Name: name, Category: category, Description: name + "原子选项；赔率需后台明确配置", Example: choice}, PositionMode: positionMode, Selection: "choice", Choices: []string{choice}, Kind: kind, Value: value}
}

func markSixSpecsForVersion(version string) []markSixPlaySpec {
	if !isMarkSixRuleVersion(version) {
		return nil
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
	return spec.Play, ok && !spec.PricingOnly
}

func markSixPlayByCode(code string) (defaultPlay, bool) {
	return markSixPlayByCodeForVersion(markSixRuleVersion, code)
}

func markSixPlayCatalogForVersion(version string) []PlayCatalogItem {
	specs := markSixSpecsForVersion(version)
	items := make([]PlayCatalogItem, 0, len(specs))
	for index, spec := range specs {
		if spec.HiddenFromCatalog {
			continue
		}
		play := spec.Play
		items = append(items, PlayCatalogItem{PlayCode: play.Code, PlayName: play.Name, Category: play.Category, Description: play.Description, Example: play.Example, SortOrder: index})
	}
	return items
}

func markSixPlayCatalog() []PlayCatalogItem {
	return markSixPlayCatalogForVersion(markSixRuleVersion)
}

func markSixNormalizeSelectionForVersion(version, playCode, selection string) string {
	playCode = strings.ToLower(strings.TrimSpace(playCode))
	selection = strings.TrimSpace(strings.ReplaceAll(selection, "，", ","))
	spec, ok := markSixSpecByCode(version, playCode)
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
	case "zodiac-list":
		if values, parsed := parseMarkSixZodiacList(selection); parsed {
			return strings.Join(values, ",")
		}
	case "tail-list":
		if values, parsed := parseMarkSixTailList(selection); parsed {
			return strings.Join(values, ",")
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

// markSixNormalizeSelection retains the established Bingo helper for callers
// that do not own a game profile. Placement paths use the versioned variant.
func markSixNormalizeSelection(playCode, selection string) string {
	return markSixNormalizeSelectionForVersion(markSixRuleVersion, playCode, selection)
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
	if !ok || spec.PricingOnly {
		return apperrors.NewBusinessError("INVALID_REQUEST", "当前六合彩彩种不支持该玩法")
	}
	selection = strings.TrimSpace(selection)
	if normalized := markSixNormalizeSelectionForVersion(version, spec.Play.Code, selection); normalized != selection {
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
	case "zodiac-list":
		values, parsed := parseMarkSixZodiacList(selection)
		if !parsed || len(values) != spec.ListCount || strings.Join(values, ",") != selection || (spec.Value != "" && !containsString(values, spec.Value)) {
			return apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("该玩法必须选择%d个不同生肖，且定价生肖必须包含在组合内", spec.ListCount))
		}
		return nil
	case "tail-list":
		values, parsed := parseMarkSixTailList(selection)
		if !parsed || len(values) != spec.ListCount || strings.Join(values, ",") != selection || (spec.Value != "" && !containsString(values, spec.Value+"尾")) {
			return apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("该玩法必须选择%d个不同尾数，且定价尾数必须包含在组合内", spec.ListCount))
		}
		return nil
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

func parseMarkSixZodiacList(selection string) ([]string, bool) {
	parts := strings.Split(strings.ReplaceAll(selection, "，", ","), ",")
	if len(parts) == 0 {
		return nil, false
	}
	order := make(map[string]int, len(markSixZodiacOptions))
	for index, zodiac := range markSixZodiacOptions {
		order[zodiac.label] = index
	}
	values := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		label := strings.TrimSuffix(strings.TrimSpace(part), "肖")
		if _, exists := order[label]; !exists {
			return nil, false
		}
		if _, duplicate := seen[label]; duplicate {
			return nil, false
		}
		seen[label] = struct{}{}
		values = append(values, label)
	}
	sort.Slice(values, func(i, j int) bool { return order[values[i]] < order[values[j]] })
	return values, true
}

func parseMarkSixTailList(selection string) ([]string, bool) {
	parts := strings.Split(strings.ReplaceAll(selection, "，", ","), ",")
	if len(parts) == 0 {
		return nil, false
	}
	values := make([]int, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		raw := strings.TrimSuffix(strings.TrimSpace(part), "尾")
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || value > 9 || strconv.Itoa(value) != raw {
			return nil, false
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, false
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	sort.Ints(values)
	labels := make([]string, 0, len(values))
	for _, value := range values {
		labels = append(labels, strconv.Itoa(value)+"尾")
	}
	return labels, true
}

func markSixZodiacListExample(count int) string {
	values := make([]string, 0, count)
	for index := 0; index < count && index < len(markSixZodiacOptions); index++ {
		values = append(values, markSixZodiacOptions[index].label)
	}
	return strings.Join(values, ",")
}

func markSixTailListExample(count int) string {
	values := make([]string, 0, count)
	for tail := 0; tail < count && tail <= 9; tail++ {
		values = append(values, strconv.Itoa(tail)+"尾")
	}
	return strings.Join(values, ",")
}

func markSixNumberListExample(count int) string {
	values := make([]int, 0, count)
	for number := 1; number <= count; number++ {
		values = append(values, number)
	}
	return joinNumbers(values)
}

func markSixLinkZodiacCode(count int, zodiacCode string) string {
	return fmt.Sprintf("marksix_link_zodiac_%d_%s", count, zodiacCode)
}

func markSixLinkTailCode(count, tail int) string {
	return fmt.Sprintf("marksix_link_tail_%d_%d", count, tail)
}

// markSixLinkedPricingCandidates returns every pricing row represented by a
// linked-zodiac/tail selection. Placement resolves all of them through the
// member > room > platform precedence chain and freezes the lowest quote.
func markSixLinkedPricingCandidates(version, playCode, selection string) ([]string, bool, error) {
	spec, ok := markSixSpecByCode(version, playCode)
	if !ok || (spec.Kind != "link-zodiac" && spec.Kind != "link-tail") {
		return nil, false, nil
	}
	candidates := make([]string, 0, spec.ListCount)
	if spec.Kind == "link-zodiac" {
		values, parsed := parseMarkSixZodiacList(selection)
		if !parsed || len(values) != spec.ListCount {
			return nil, true, apperrors.NewBusinessError("INVALID_REQUEST", "连肖选择内容不正确")
		}
		codes := make(map[string]string, len(markSixZodiacOptions))
		for _, zodiac := range markSixZodiacOptions {
			codes[zodiac.label] = zodiac.code
		}
		for _, label := range values {
			candidates = append(candidates, markSixLinkZodiacCode(spec.ListCount, codes[label]))
		}
		return candidates, true, nil
	}
	values, parsed := parseMarkSixTailList(selection)
	if !parsed || len(values) != spec.ListCount {
		return nil, true, apperrors.NewBusinessError("INVALID_REQUEST", "连尾选择内容不正确")
	}
	for _, label := range values {
		tail, _ := strconv.Atoi(strings.TrimSuffix(label, "尾"))
		candidates = append(candidates, markSixLinkTailCode(spec.ListCount, tail))
	}
	return candidates, true, nil
}

func countMarkSixNumberHits(selected, drawn []int) int {
	hits := 0
	for _, number := range selected {
		if containsInt(drawn, number) {
			hits++
		}
	}
	return hits
}

func markSixRegularZodiacHits(regular []int, wanted string, drawAt time.Time) (int, error) {
	hits := 0
	for _, number := range regular {
		zodiac, err := markSixNumberZodiac(number, drawAt)
		if err != nil {
			return 0, err
		}
		if zodiac == wanted {
			hits++
		}
	}
	return hits, nil
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
	case "combo-3-2":
		selected, _ := parseMarkSixIntList(selection)
		hits := countMarkSixNumberHits(selected, regular)
		return markSixWonLost(hits >= 2), fmt.Sprintf("所选3个号码命中%d个正码", hits), nil
	case "combo-2-special":
		selected, _ := parseMarkSixIntList(selection)
		hits := countMarkSixNumberHits(selected, regular)
		won := hits == 2 || (hits == 1 && containsInt(selected, special))
		return markSixWonLost(won), fmt.Sprintf("特码%d，正码为%s", special, joinNumbers(regular)), nil
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
		return markSixOutcomeWon, fmt.Sprintf("所选%d个号码均未开出", len(selected)), nil
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
	case "one-zodiac":
		for _, number := range numbers {
			zodiac, err := markSixNumberZodiac(number, drawAt)
			if err != nil {
				return "", "", err
			}
			if zodiac == selection {
				return markSixOutcomeWon, fmt.Sprintf("7个号码中已出现%s肖", selection), nil
			}
		}
		return markSixOutcomeLost, fmt.Sprintf("7个号码中未出现%s肖", selection), nil
	case "one-tail":
		tail, _ := strconv.Atoi(spec.Value)
		for _, number := range numbers {
			if number%10 == tail {
				return markSixOutcomeWon, fmt.Sprintf("7个号码中已出现%d尾", tail), nil
			}
		}
		return markSixOutcomeLost, fmt.Sprintf("7个号码中未出现%d尾", tail), nil
	case "combined-zodiac":
		if special == 49 {
			return markSixOutcomePush, "特码49，合肖和局返还本金", nil
		}
		selected, _ := parseMarkSixZodiacList(selection)
		zodiac, err := markSixNumberZodiac(special, drawAt)
		if err != nil {
			return "", "", err
		}
		return markSixWonLost(containsString(selected, zodiac)), fmt.Sprintf("特码 %d 属%s", special, zodiac), nil
	case "link-zodiac":
		selected, _ := parseMarkSixZodiacList(selection)
		seen := make(map[string]struct{}, len(numbers))
		for _, number := range numbers {
			zodiac, err := markSixNumberZodiac(number, drawAt)
			if err != nil {
				return "", "", err
			}
			seen[zodiac] = struct{}{}
		}
		for _, zodiac := range selected {
			if _, exists := seen[zodiac]; !exists {
				return markSixOutcomeLost, fmt.Sprintf("7个号码中未出现%s肖", zodiac), nil
			}
		}
		return markSixOutcomeWon, "所选连肖均已出现", nil
	case "link-tail":
		selected, _ := parseMarkSixTailList(selection)
		seen := make(map[string]struct{}, len(numbers))
		for _, number := range numbers {
			seen[strconv.Itoa(number%10)+"尾"] = struct{}{}
		}
		for _, tail := range selected {
			if _, exists := seen[tail]; !exists {
				return markSixOutcomeLost, fmt.Sprintf("7个号码中未出现%s", tail), nil
			}
		}
		return markSixOutcomeWon, "所选连尾均已出现", nil
	case "regular-zodiac":
		hits, err := markSixRegularZodiacHits(regular, selection, drawAt)
		if err != nil {
			return "", "", err
		}
		return markSixWonLost(hits > 0), fmt.Sprintf("6个正码命中%s肖%d个", selection, hits), nil
	case "total-zodiac-count", "total-zodiac-parity":
		total, err := markSixDistinctZodiacCount(numbers, drawAt)
		if err != nil {
			return "", "", err
		}
		won := false
		if spec.Kind == "total-zodiac-count" {
			wanted, _ := strconv.Atoi(strings.TrimSuffix(selection, "肖"))
			won = total == wanted
		} else {
			won = (total%2 == 1) == (selection == "总肖单")
		}
		return markSixWonLost(won), fmt.Sprintf("本期共%d个不同生肖", total), nil
	case "seven-color-wave":
		winner, draw := markSixSevenColorOutcome(numbers)
		if draw {
			if selection == "和局" {
				return markSixOutcomeWon, "七色波开出和局", nil
			}
			return markSixOutcomePush, "七色波开出和局，红蓝绿投注返还本金", nil
		}
		return markSixWonLost(selection == winner), "七色波中奖色为" + winner, nil
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

type markSixSettlementDecision struct {
	Outcome            markSixBetOutcome
	Reason             string
	EffectiveOdds      float64
	ValidTurnoverCents int64
	Policy             string
}

type markSixOddsTerms struct {
	Version        int     `json:"version"`
	ExactThreeOdds float64 `json:"exact_three_odds,omitempty"`
	TwoRegularOdds float64 `json:"two_regular_odds,omitempty"`
	PricingCode    string  `json:"pricing_code,omitempty"`
}

const markSixOddsTermsVersion = 1

func encodeMarkSixPricingCodeTermsForVersion(version, pricingCode string) (string, error) {
	pricingCode = strings.ToLower(strings.TrimSpace(pricingCode))
	spec, ok := markSixSpecByCode(version, pricingCode)
	if !ok || !spec.PricingOnly {
		return "", apperrors.NewBusinessError("INVALID_REQUEST", "连肖连尾定价编号不正确")
	}
	payload, err := json.Marshal(markSixOddsTerms{Version: markSixOddsTermsVersion, PricingCode: pricingCode})
	if err != nil {
		return "", apperrors.NewSystemError("BET_CREATE_FAILED", "冻结组合定价编号失败", err)
	}
	return string(payload), nil
}

func encodeMarkSixPricingCodeTerms(pricingCode string) (string, error) {
	return encodeMarkSixPricingCodeTermsForVersion(markSixRuleVersion, pricingCode)
}

func markSixCompositePricingCodes(playCode string) (primary, secondary string, ok bool) {
	switch strings.ToLower(strings.TrimSpace(playCode)) {
	case "marksix_combo_3_2":
		return "marksix_combo_3_2_exact2", "marksix_combo_3_2_exact3", true
	case "marksix_combo_2_special":
		return "marksix_combo_2_special_mixed", "marksix_combo_2_special_regular", true
	default:
		return "", "", false
	}
}

func encodeMarkSixOddsTerms(playCode string, secondaryOdds float64) (string, error) {
	if secondaryOdds <= 1 || math.IsNaN(secondaryOdds) || math.IsInf(secondaryOdds, 0) {
		return "", apperrors.NewBusinessError("ODDS_NOT_CONFIGURED", "复合玩法第二档赔率尚未配置")
	}
	terms := markSixOddsTerms{Version: markSixOddsTermsVersion}
	switch strings.ToLower(strings.TrimSpace(playCode)) {
	case "marksix_combo_3_2":
		terms.ExactThreeOdds = secondaryOdds
	case "marksix_combo_2_special":
		terms.TwoRegularOdds = secondaryOdds
	default:
		return "", apperrors.NewBusinessError("INVALID_REQUEST", "该玩法不使用复合赔率")
	}
	payload, err := json.Marshal(terms)
	if err != nil {
		return "", apperrors.NewSystemError("BET_CREATE_FAILED", "冻结复合赔率失败", err)
	}
	return string(payload), nil
}

func decodeMarkSixOddsTerms(item bet.Bet) (markSixOddsTerms, error) {
	var terms markSixOddsTerms
	if strings.TrimSpace(item.OddsTerms) == "" || json.Unmarshal([]byte(item.OddsTerms), &terms) != nil || terms.Version != markSixOddsTermsVersion {
		return terms, apperrors.NewBusinessError("RULES_NOT_READY", "复合玩法缺少下注时冻结的第二档赔率")
	}
	return terms, nil
}

// decideMarkSixSettlement keeps the accepted odds snapshot authoritative.
// 正肖 is the only original market whose payout grows with the number of
// matching regular balls: each extra hit adds one more copy of the quoted net
// win, while principal is returned once. No second live odds lookup is used.
func decideMarkSixSettlement(gameID string, item bet.Bet, numbers []int, drawAt time.Time) (markSixSettlementDecision, error) {
	profile, found := rulesForVersion(item.RuleVersion)
	if !found || !profile.MarkSix || !gameSupportsRuleVersion(gameID, item.RuleVersion) {
		return markSixSettlementDecision{}, apperrors.NewBusinessError("RULES_NOT_READY", "六合彩注单规则版本未确认或与彩种不一致，暂不能结算")
	}
	if err := profile.validateDraw(numbers); err != nil {
		return markSixSettlementDecision{}, apperrors.NewBusinessError("INVALID_DRAW", "开奖号码不符合注单规则："+err.Error())
	}
	outcome, reason, err := evaluateMarkSixBetForVersion(item.RuleVersion, numbers, item.PlayCode, item.Position, item.Selection, drawAt)
	if err != nil {
		return markSixSettlementDecision{}, err
	}
	decision := markSixSettlementDecision{
		Outcome: outcome, Reason: reason, EffectiveOdds: item.Odds,
		ValidTurnoverCents: item.AmountCents, Policy: "marksix_standard",
	}
	spec, _ := markSixSpecByCode(item.RuleVersion, item.PlayCode)
	var terms markSixOddsTerms
	if spec.Kind == "combo-3-2" || spec.Kind == "combo-2-special" {
		var termsErr error
		terms, termsErr = decodeMarkSixOddsTerms(item)
		if termsErr != nil {
			return markSixSettlementDecision{}, termsErr
		}
		if spec.Kind == "combo-3-2" && (terms.ExactThreeOdds <= 1 || math.IsNaN(terms.ExactThreeOdds) || math.IsInf(terms.ExactThreeOdds, 0)) {
			return markSixSettlementDecision{}, apperrors.NewBusinessError("RULES_NOT_READY", "三中二注单缺少中三档赔率快照")
		}
		if spec.Kind == "combo-2-special" && (terms.TwoRegularOdds <= 1 || math.IsNaN(terms.TwoRegularOdds) || math.IsInf(terms.TwoRegularOdds, 0)) {
			return markSixSettlementDecision{}, apperrors.NewBusinessError("RULES_NOT_READY", "二中特注单缺少中二档赔率快照")
		}
	}
	if outcome == markSixOutcomePush {
		decision.EffectiveOdds = 1
		decision.ValidTurnoverCents = 0
		decision.Policy = "marksix_push"
		return decision, nil
	}
	if outcome == markSixOutcomeWon && spec.Kind == "combo-3-2" {
		selected, _ := parseMarkSixIntList(item.Selection)
		if countMarkSixNumberHits(selected, numbers[:6]) == 3 {
			decision.EffectiveOdds = terms.ExactThreeOdds
			decision.Policy = "marksix_combo_3_2_exact3"
			decision.Reason += "；按中三档赔率结算"
		} else {
			decision.Policy = "marksix_combo_3_2_exact2"
			decision.Reason += "；按中二档赔率结算"
		}
	}
	if outcome == markSixOutcomeWon && spec.Kind == "combo-2-special" {
		selected, _ := parseMarkSixIntList(item.Selection)
		if countMarkSixNumberHits(selected, numbers[:6]) == 2 {
			decision.EffectiveOdds = terms.TwoRegularOdds
			decision.Policy = "marksix_combo_2_special_regular"
			decision.Reason += "；按中二档赔率结算"
		} else {
			decision.Policy = "marksix_combo_2_special_mixed"
			decision.Reason += "；按中特档赔率结算"
		}
	}
	if outcome == markSixOutcomeWon && spec.Kind == "regular-zodiac" {
		hits, hitErr := markSixRegularZodiacHits(numbers[:6], item.Selection, drawAt)
		if hitErr != nil {
			return markSixSettlementDecision{}, hitErr
		}
		decision.EffectiveOdds = math.Round((1+float64(hits)*(item.Odds-1))*10000) / 10000
		decision.Policy = fmt.Sprintf("marksix_regular_zodiac_%d_hits", hits)
		decision.Reason = fmt.Sprintf("%s；按%d次命中倍增净赢", reason, hits)
	}
	return decision, nil
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

func markSixDistinctZodiacCount(numbers []int, drawAt time.Time) (int, error) {
	seen := make(map[string]struct{}, len(numbers))
	for _, number := range numbers {
		zodiac, err := markSixNumberZodiac(number, drawAt)
		if err != nil {
			return 0, err
		}
		seen[zodiac] = struct{}{}
	}
	return len(seen), nil
}

func markSixSevenColorOutcome(numbers []int) (string, bool) {
	scores := map[string]int{"red": 0, "blue": 0, "green": 0}
	for _, number := range numbers[:6] {
		scores[markSixColor(number)] += 2
	}
	scores[markSixColor(numbers[6])] += 3
	maxScore := -1
	winners := make([]string, 0, 2)
	for _, color := range []string{"red", "blue", "green"} {
		if scores[color] > maxScore {
			maxScore = scores[color]
			winners = []string{color}
		} else if scores[color] == maxScore {
			winners = append(winners, color)
		}
	}
	if len(winners) != 1 {
		return "", true
	}
	return map[string]string{"red": "红波", "blue": "蓝波", "green": "绿波"}[winners[0]], false
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
	markSixZodiacNames   = []string{"鼠", "牛", "虎", "兔", "龙", "蛇", "马", "羊", "猴", "鸡", "狗", "猪"}
	markSixZodiacOptions = []struct{ code, label string }{
		{"rat", "鼠"}, {"ox", "牛"}, {"tiger", "虎"}, {"rabbit", "兔"}, {"dragon", "龙"}, {"snake", "蛇"},
		{"horse", "马"}, {"goat", "羊"}, {"monkey", "猴"}, {"rooster", "鸡"}, {"dog", "狗"}, {"pig", "猪"},
	}
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
