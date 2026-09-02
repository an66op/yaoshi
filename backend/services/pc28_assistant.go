package services

import (
	apperrors "backend/errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var pc28CompactAmount = regexp.MustCompile(`^(.*(?:[大小单双龙虎和]|极大|极小|大单|大双|小单|小双|红波|绿波|蓝波|豹子|对子|顺子))([0-9]+(?:\.[0-9]{1,2})?)$`)

func parsePC28AssistantBet(content string) ([]AssistantBetLine, error) {
	raw := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(content), "买"))
	if raw == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "请输入投注内容，例如 1/5、1/1/5 或 大/5")
	}
	if len([]rune(raw)) > 400 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "投注内容过长，请拆分后重试")
	}
	lines := make([]AssistantBetLine, 0)
	for _, rawSegment := range strings.Split(raw, "#") {
		segment := strings.TrimSpace(rawSegment)
		if segment == "" {
			continue
		}
		parts := strings.Split(segment, "/")
		if len(parts) == 1 {
			match := pc28CompactAmount.FindStringSubmatch(segment)
			if len(match) != 3 {
				return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("“%s”缺少金额；纯数字玩法必须使用 / 分隔", segment))
			}
			parts = []string{strings.TrimSpace(match[1]), match[2]}
		}
		for index := range parts {
			parts[index] = strings.TrimSpace(parts[index])
		}
		if len(parts) < 2 {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "投注格式不正确")
		}
		amountCents, err := assistantMoneyCents(parts[len(parts)-1])
		if err != nil {
			return nil, err
		}
		entries, err := pc28AssistantEntries(parts[:len(parts)-1])
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			line := AssistantBetLine{Position: entry.position, Selection: entry.selection,
				PlayCode: entry.playCode, PlayName: entry.playName, Amount: centsToAmount(amountCents)}
			line.Label = assistantLineLabel(line)
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "未识别到有效投注，请检查格式")
	}
	return mergeAssistantLines(lines), nil
}

func pc28AssistantEntries(parts []string) ([]assistantEntry, error) {
	if len(parts) == 0 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "请选择PC28玩法")
	}
	if parts[0] == "特码" || parts[0] == "包三" || parts[0] == "特码包三" {
		values := parts[1:]
		if len(values) == 1 {
			values = strings.FieldsFunc(values[0], func(char rune) bool { return char == ',' || char == '，' || char == ' ' })
		}
		selection := strings.Join(values, ",")
		selection = pc28NormalizeSelection(pc28PackageThree, selection)
		if err := pc28ValidateChoice(pc28PackageThree, 0, selection); err != nil {
			return nil, err
		}
		return []assistantEntry{{position: 0, selection: selection, playCode: pc28PackageThree, playName: "特码包三"}}, nil
	}
	if len(parts) == 1 {
		return pc28MixedAssistantEntries(parts[0])
	}
	if len(parts) != 2 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "定位玩法请使用 球位/号码/金额，特码包三请使用 特码/号码/号码/号码/金额")
	}
	positions, ok := pc28Positions(parts[0])
	if !ok {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("无法识别球位“%s”", parts[0]))
	}
	return pc28PositionEntries(positions, parts[1])
}

func pc28Positions(raw string) ([]int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || !allDigits(raw) {
		return nil, false
	}
	positions := make([]int, 0, len(raw))
	seen := map[int]struct{}{}
	for _, char := range raw {
		position := int(char - '0')
		if position < 1 || position > 3 {
			return nil, false
		}
		if _, exists := seen[position]; exists {
			return nil, false
		}
		seen[position] = struct{}{}
		positions = append(positions, position)
	}
	return positions, len(positions) > 0
}

func pc28PositionEntries(positions []int, raw string) ([]assistantEntry, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "请选择定位号码或大小单双龙虎和")
	}
	entries := make([]assistantEntry, 0, len(positions)*len([]rune(raw)))
	for _, position := range positions {
		for _, char := range raw {
			selection := string(char)
			entry := assistantEntry{position: position, selection: selection}
			switch {
			case char >= '0' && char <= '9':
				entry.playCode, entry.playName = pc28PositionNumber, fmt.Sprintf("第%d球号码", position)
			case strings.ContainsRune("大小单双", char):
				entry.playCode, entry.playName = pc28PositionTwoSided, fmt.Sprintf("第%d球两面", position)
			case char == '龙' || char == '虎':
				if position != 1 {
					return nil, apperrors.NewBusinessError("INVALID_REQUEST", "龙虎仅比较第一球与第三球")
				}
				entry.playCode, entry.playName = pc28DragonTiger, "龙虎"
			case char == '和':
				if position != 1 {
					return nil, apperrors.NewBusinessError("INVALID_REQUEST", "和仅比较第一球与第三球")
				}
				entry.playCode, entry.playName = pc28DragonTigerTie, "和"
			default:
				return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("无法识别定位玩法“%s”", selection))
			}
			entry.selection = pc28NormalizeSelection(entry.playCode, entry.selection)
			if err := pc28ValidateChoice(entry.playCode, entry.position, entry.selection); err != nil {
				return nil, err
			}
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func pc28MixedAssistantEntries(raw string) ([]assistantEntry, error) {
	raw = strings.TrimSpace(raw)
	if total, err := strconv.Atoi(raw); err == nil {
		if total < 0 || total > 27 {
			return nil, apperrors.NewBusinessError("INVALID_REQUEST", "PC28单点和值只能选择0至27")
		}
		code := pc28ExactCode(total)
		return []assistantEntry{{position: 0, selection: strconv.Itoa(total), playCode: code, playName: fmt.Sprintf("和值%d", total)}}, nil
	}
	type mixed struct{ code, selection, name string }
	choices := map[string]mixed{
		"大": {pc28SumSize, "大", "和值大小"}, "小": {pc28SumSize, "小", "和值大小"},
		"单": {pc28SumParity, "单", "和值单双"}, "双": {pc28SumParity, "双", "和值单双"},
		"大单": {pc28ComboBigOdd, "大单", "大单"}, "大双": {pc28ComboBigEven, "大双", "大双"},
		"小单": {pc28ComboSmallOdd, "小单", "小单"}, "小双": {pc28ComboSmallEven, "小双", "小双"},
		"极大": {pc28Extreme, "极大", "极值大小"}, "极小": {pc28Extreme, "极小", "极值大小"},
		"红波": {pc28ColorRed, "红波", "红波"}, "绿波": {pc28ColorGreen, "绿波", "绿波"}, "蓝波": {pc28ColorBlue, "蓝波", "蓝波"},
		"豹子": {pc28Leopard, "豹子", "豹子"}, "对子": {pc28Pair, "对子", "对子"}, "顺子": {pc28Straight, "顺子", "顺子"},
	}
	choice, ok := choices[raw]
	if !ok {
		// Compact定位，例如1大5，经金额分离后左侧为1大。
		runes := []rune(raw)
		if len(runes) >= 2 {
			positions, positionsOK := pc28Positions(string(runes[:len(runes)-1]))
			if positionsOK && strings.ContainsRune("大小单双龙虎和", runes[len(runes)-1]) {
				return pc28PositionEntries(positions, string(runes[len(runes)-1]))
			}
		}
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", fmt.Sprintf("无法识别PC28玩法“%s”", raw))
	}
	return []assistantEntry{{position: 0, selection: choice.selection, playCode: choice.code, playName: choice.name}}, nil
}

func pc28AssistantRepeatContent(lines []AssistantBetLine) (string, error) {
	segments := make([]string, 0, len(lines))
	for _, line := range lines {
		if line.Amount <= 0 {
			return "", apperrors.NewBusinessError("INVALID_REQUEST", "上一笔PC28投注金额不正确，不能重复")
		}
		code := strings.ToLower(strings.TrimSpace(line.PlayCode))
		selection := pc28NormalizeSelection(code, line.Selection)
		if err := pc28ValidateChoice(code, line.Position, selection); err != nil {
			return "", apperrors.NewBusinessError("INVALID_REQUEST", "上一笔PC28投注明细不正确，不能重复")
		}
		amount := FormatBetAmount(line.Amount)
		var segment string
		switch code {
		case pc28PackageThree:
			segment = "特码/" + strings.ReplaceAll(selection, ",", "/") + "/" + amount
		case pc28PositionNumber, pc28PositionTwoSided, pc28DragonTiger, pc28DragonTigerTie:
			segment = fmt.Sprintf("%d/%s/%s", line.Position, selection, amount)
		default:
			if pc28IsExactPlay(code) || pc28IsTwoSidedPlay(code) || pc28IsCombinationPlay(code) ||
				code == pc28Extreme || code == pc28ColorRed || code == pc28ColorGreen || code == pc28ColorBlue ||
				code == pc28Leopard || code == pc28Pair || code == pc28Straight {
				segment = selection + "/" + amount
			} else {
				return "", apperrors.NewBusinessError("INVALID_REQUEST", "上一笔PC28投注包含当前不能重复的玩法")
			}
		}
		segments = append(segments, segment)
	}
	return strings.Join(segments, "#"), nil
}
