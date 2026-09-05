package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"backend/data/models/lottery"
)

const (
	api168Base    = "https://api.api16868.com"
	api168Referer = "https://kj138138.com/"

	// 168 publishes the Taiwan Bingo result as a sorted set. Products whose
	// contract depends on the actual ball order must therefore be joined with
	// an ordered feed and cross-checked by period, complete 20-ball set and the
	// provider's duplicated 21st/super ball before they may be imported.
	bingoOrderedHistoryURL     = "https://jyb.one/api/history"
	bingoOrderedHistoryLimit   = 500 // two complete 203-period operating days
	bingoVerifiedSourceName    = "台湾宾果双源校验（168集合＋jyb.one顺序）"
	bingoVerifiedSourceURL     = "https://jyb.one/"
	bingoOrderPendingMessage   = "等待台湾宾果双源顺序校验"
	bingoOrderedSourceRevision = "tw-bingo-168-jyb-order-v1"

	bingoRacingAConversionVersion = "bingo-racing-a-rank-v1"
	bingoSSC1ConversionVersion    = "bingo-ssc-1-taildigit-v1"
	bingoMarkSixConversionVersion = "bingo-mark-six-filter49-v1"
)

type api168Series string

const (
	api168PK10 api168Series = "pk10"
	api168SSC  api168Series = "ssc"
	api168LHC  api168Series = "lhc"
	api168KL8  api168Series = "kl8"
)

type api168Binding struct {
	GameID  string
	LotCode string
	Series  api168Series
}

var api168HighFreqBindings = []api168Binding{
	{GameID: "speed-racing", LotCode: "10037", Series: api168PK10},
	{GameID: "au-lucky-10", LotCode: "10012", Series: api168PK10},
	{GameID: "au-lucky-5", LotCode: "10010", Series: api168SSC},
	{GameID: "fly-racing", LotCode: "10057", Series: api168PK10},
	{GameID: "speed-fly", LotCode: "10035", Series: api168PK10},
	{GameID: "sg-fly", LotCode: "10058", Series: api168PK10},
	{GameID: "speed-ssc", LotCode: "10036", Series: api168SSC},
}

var api168MarkSixBindings = []api168Binding{
	{GameID: "hong-kong-mark-six", LotCode: "10091", Series: api168LHC},
	{GameID: "new-macau-mark-six", LotCode: "10092", Series: api168LHC},
	{GameID: "old-macau-mark-six", LotCode: "10093", Series: api168LHC},
}

func api168MarkSixBindingForGame(gameID string) (api168Binding, bool) {
	for _, binding := range api168MarkSixBindings {
		if binding.GameID == strings.TrimSpace(gameID) {
			return binding, true
		}
	}
	return api168Binding{}, false
}

func api168MarkSixSourceBound(game *lottery.Game, binding api168Binding) bool {
	return game != nil && game.ID == binding.GameID && strings.EqualFold(strings.TrimSpace(game.SourceKind), "external") &&
		strings.TrimSpace(game.SourceName) == legacy168HighFreqName && strings.TrimSpace(game.SourceURL) == legacy168HighFreqURL
}

var err168MarkSixBindingChanged = errors.New("168六合彩来源绑定已变化")

type api168BingoBinding struct {
	GameID                string
	Transform             func([]int) []int
	RequiresOrderedSource bool
	ConversionVersion     string
}

var api168BingoBindings = []api168BingoBinding{
	{GameID: "bingo-ssc-1", Transform: bingoSSCNumbers(0), RequiresOrderedSource: true, ConversionVersion: bingoSSC1ConversionVersion},
	{GameID: "bingo-ssc-2", Transform: bingoSSCNumbers(1)},
	{GameID: "bingo-ssc-3", Transform: bingoSSCNumbers(2)},
	{GameID: "bingo-ssc-4", Transform: bingoSSCNumbers(3)},
	{GameID: "bingo-racing-a", Transform: bingoRacingARankV1Numbers, RequiresOrderedSource: true, ConversionVersion: bingoRacingAConversionVersion},
	{GameID: "bingo-racing-b", Transform: bingoRacingNumbers(10)},
	{GameID: "bingo-mark-six", Transform: bingoMarkSixNumbers, RequiresOrderedSource: true, ConversionVersion: bingoMarkSixConversionVersion},
}

func api168BingoBindingForGame(gameID string) (api168BingoBinding, bool) {
	for _, binding := range api168BingoBindings {
		if binding.GameID == strings.TrimSpace(gameID) {
			return binding, true
		}
	}
	return api168BingoBinding{}, false
}

func bingoBindingSourceDefaults(binding api168BingoBinding) (name, sourceURL, syncStatus, lastSyncError string) {
	if binding.RequiresOrderedSource {
		return bingoVerifiedSourceName, bingoVerifiedSourceURL, "stale", bingoOrderPendingMessage
	}
	return "168开奖网", "https://kj138138.com/view/api/index.html", "idle", ""
}

// transform168BingoDraws converts one already validated source batch for one
// derived game. A single unconvertible issue rejects the whole game batch so
// the sync cursor can never advance past a missing result.
func transform168BingoDraws(gameID string, raw []sourceDraw, transform func([]int) []int) ([]sourceDraw, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("168开奖网未返回可映射的台湾宾果记录")
	}
	if transform == nil {
		return nil, fmt.Errorf("%w: 游戏 %s 缺少开奖转换规则", err168BingoRawInvalid, gameID)
	}
	draws := make([]sourceDraw, 0, len(raw))
	for _, row := range raw {
		if bingoGameRequiresOrderedSource(gameID) && !row.BingoOrderVerified {
			return nil, fmt.Errorf("%w: 游戏 %s 期号 %s 未通过双源顺序校验", err168BingoOrderMismatch, gameID, row.Issue)
		}
		numbers := transform(row.Numbers)
		if len(numbers) == 0 {
			return nil, fmt.Errorf("%w: 游戏 %s 期号 %s 转换结果为空", err168BingoRawInvalid, gameID, row.Issue)
		}
		row.Numbers = numbers
		if row.BingoOrderVerified {
			binding, ok := api168BingoBindingForGame(gameID)
			if !ok || strings.TrimSpace(binding.ConversionVersion) == "" || strings.TrimSpace(row.SourceRevision) == "" {
				return nil, fmt.Errorf("%w: 游戏 %s 缺少已验证来源或转换版本", err168BingoOrderMismatch, gameID)
			}
			row.ConversionRevision = binding.ConversionVersion
		}
		draws = append(draws, row)
	}
	return draws, nil
}

func bingoGameRequiresOrderedSource(gameID string) bool {
	binding, ok := api168BingoBindingForGame(gameID)
	return ok && binding.RequiresOrderedSource
}

type bingoOrderedHistoryRow struct {
	Period       string `json:"period"`
	DrawTime     string `json:"drawTime"`
	Numbers      []int  `json:"numbers"`
	SuperNumber  int    `json:"superNumber"`
	SumVal       int    `json:"sumVal"`
	SumSize      string `json:"sumSize"`
	SumParity    string `json:"sumParity"`
	SuperSize    string `json:"superSize"`
	SuperParity  string `json:"superParity"`
	PlateUpDown  string `json:"plateUpDown"`
	PlateOddEven string `json:"plateOddEven"`
}

// parseBingoOrderedHistory is deliberately strict. This feed contributes the
// one fact the sorted 168 response cannot prove (ball order), so a schema,
// issue, range, uniqueness or super-ball anomaly closes order-dependent games
// instead of being repaired or guessed.
func parseBingoOrderedHistory(body []byte) ([]sourceDraw, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var rows []bingoOrderedHistoryRow
	if err := decoder.Decode(&rows); err != nil {
		return nil, fmt.Errorf("台湾宾果有序开奖响应格式错误: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, fmt.Errorf("台湾宾果有序开奖响应格式错误: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("台湾宾果有序开奖源未返回记录")
	}
	draws := make([]sourceDraw, 0, len(rows))
	seenIssues := make(map[string]bool, len(rows))
	for index, row := range rows {
		issue := strings.TrimSpace(row.Period)
		if issue == "" || strings.IndexFunc(issue, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return nil, fmt.Errorf("台湾宾果有序开奖第 %d 条期号无效", index+1)
		}
		if seenIssues[issue] {
			return nil, fmt.Errorf("台湾宾果有序开奖期号 %s 重复", issue)
		}
		seenIssues[issue] = true
		if err := validate168BingoNumbers(row.Numbers); err != nil {
			return nil, fmt.Errorf("台湾宾果有序开奖期号 %s: %w", issue, err)
		}
		if row.SuperNumber != row.Numbers[len(row.Numbers)-1] {
			return nil, fmt.Errorf("台湾宾果有序开奖期号 %s 末球 %d 与超级号 %d 不一致", issue, row.Numbers[len(row.Numbers)-1], row.SuperNumber)
		}
		drawAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(row.DrawTime))
		if err != nil || drawAt.IsZero() {
			return nil, fmt.Errorf("台湾宾果有序开奖期号 %s 开奖时间无效", issue)
		}
		draws = append(draws, sourceDraw{
			Issue: issue, Numbers: append([]int(nil), row.Numbers...), DrawAt: drawAt.UTC(),
		})
	}
	return draws, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return fmt.Errorf("响应包含多个 JSON 值")
	}
	return err
}

var err168BingoOrderMismatch = errors.New("台湾宾果双源顺序校验失败")

// crossValidate168BingoOrder keeps 168 authoritative for period membership,
// the complete 20-ball set, the provider's final/super-ball marker and all
// next-issue metadata. The second source contributes only the ball order.
// Every 168 row must match; otherwise the entire derived-game batch is closed.
func crossValidate168BingoOrder(authoritative, ordered []sourceDraw) ([]sourceDraw, error) {
	if len(authoritative) == 0 || len(ordered) == 0 {
		return nil, fmt.Errorf("%w: 缺少可交叉校验的开奖记录", err168BingoOrderMismatch)
	}
	orderedByIssue := make(map[string]sourceDraw, len(ordered))
	for _, row := range ordered {
		if strings.TrimSpace(row.Issue) == "" || validate168BingoNumbers(row.Numbers) != nil {
			return nil, fmt.Errorf("%w: 有序源包含无效记录", err168BingoOrderMismatch)
		}
		if _, exists := orderedByIssue[row.Issue]; exists {
			return nil, fmt.Errorf("%w: 有序源期号 %s 重复", err168BingoOrderMismatch, row.Issue)
		}
		orderedByIssue[row.Issue] = row
	}
	result := make([]sourceDraw, 0, len(authoritative))
	for _, row := range authoritative {
		if validate168BingoNumbers(row.Numbers) != nil {
			return nil, fmt.Errorf("%w: 168期号 %s 原始集合无效", err168BingoOrderMismatch, row.Issue)
		}
		orderedRow, ok := orderedByIssue[row.Issue]
		if !ok {
			return nil, fmt.Errorf("%w: 有序源缺少168期号 %s", err168BingoOrderMismatch, row.Issue)
		}
		if !sameBingoNumberSet(row.Numbers, orderedRow.Numbers) {
			return nil, fmt.Errorf("%w: 期号 %s 的20球集合不一致", err168BingoOrderMismatch, row.Issue)
		}
		if !row.HasBingoSourceTail {
			return nil, fmt.Errorf("%w: 168期号 %s 缺少第21个末球校验值", err168BingoOrderMismatch, row.Issue)
		}
		if row.BingoSourceTail != orderedRow.Numbers[len(orderedRow.Numbers)-1] {
			return nil, fmt.Errorf("%w: 期号 %s 的末球不一致", err168BingoOrderMismatch, row.Issue)
		}
		verified := row
		verified.Numbers = append([]int(nil), orderedRow.Numbers...)
		verified.BingoOrderVerified = true
		verified.SourceRevision = bingoOrderedSourceRevision
		result = append(result, verified)
	}
	return result, nil
}

func sameBingoNumberSet(first, second []int) bool {
	if len(first) != 20 || len(second) != 20 {
		return false
	}
	var seen [81]bool
	for _, number := range first {
		if number < 1 || number > 80 || seen[number] {
			return false
		}
		seen[number] = true
	}
	for _, number := range second {
		if number < 1 || number > 80 || !seen[number] {
			return false
		}
		seen[number] = false
	}
	for number := 1; number <= 80; number++ {
		if seen[number] {
			return false
		}
	}
	return true
}

var err168BingoRawInvalid = errors.New("168台湾宾果原始开奖数据无效")

func is168BingoSource(series api168Series, lotCode string) bool {
	return series == api168KL8 && lotCode == "10047"
}

func sourceDrawsFrom168Payload(payload api168Envelope, series api168Series, lotCode string, transform func([]int) []int) ([]sourceDraw, error) {
	if !is168BingoSource(series, lotCode) {
		return sourceDrawsFrom168Rows(payload.Rows(), transform), nil
	}
	rows, err := parseAPI168RowsStrict(payload.Result.Data)
	if err != nil {
		return nil, fmt.Errorf("%w: 响应记录格式错误: %v", err168BingoRawInvalid, err)
	}
	result := make([]sourceDraw, 0, len(rows))
	seen := make(map[string]int, len(rows))
	for index, row := range rows {
		issue := strings.TrimSpace(row.IssueText())
		if issue == "" {
			return nil, fmt.Errorf("%w: 第 %d 条记录缺少期号", err168BingoRawInvalid, index+1)
		}
		// Do not use the permissive generic parser here: it drops broken tokens
		// and empty entries, which could make a damaged source look valid after
		// conversion. Validate even duplicate issues before deduplicating them.
		numbers, sourceTail, hasSourceTail, err := parse168BingoNumbersWithTail(row.Code)
		if err != nil {
			return nil, fmt.Errorf("期号 %s: %w", issue, err)
		}
		candidate := sourceDraw{
			Issue: issue, Numbers: numbers, DrawAt: parse168DrawTime(row.Time),
			NextIssue: api168IssueText(row.NextIssue), NextDrawAt: parse168DrawTime(row.NextTime),
			BingoSourceTail: sourceTail, HasBingoSourceTail: hasSourceTail,
		}
		if candidate.DrawAt.IsZero() {
			return nil, fmt.Errorf("%w: 期号 %s 开奖时间无效", err168BingoRawInvalid, issue)
		}
		if existingIndex, exists := seen[issue]; exists {
			merged, mergeErr := mergeEquivalent168BingoDraw(result[existingIndex], candidate)
			if mergeErr != nil {
				return nil, mergeErr
			}
			result[existingIndex] = merged
			continue
		}
		seen[issue] = len(result)
		result = append(result, candidate)
	}
	// No transformation is run until every raw row in this response is valid.
	if transform != nil {
		for index := range result {
			result[index].Numbers = transform(result[index].Numbers)
			if len(result[index].Numbers) == 0 {
				return nil, fmt.Errorf("%w: 期号 %s 转换结果为空", err168BingoRawInvalid, result[index].Issue)
			}
		}
	}
	return result, nil
}

func parse168BingoNumbers(code string) ([]int, error) {
	numbers, _, _, err := parse168BingoNumbersWithTail(code)
	return numbers, err
}

func parse168BingoNumbersWithTail(code string) ([]int, int, bool, error) {
	tokens := strings.Split(code, ",")
	if len(tokens) != 20 && len(tokens) != 21 {
		return nil, 0, false, fmt.Errorf("%w: 应有 20 个原始号码，实际 %d 个", err168BingoRawInvalid, len(tokens))
	}
	numbers := make([]int, len(tokens))
	for index, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" || strings.IndexFunc(token, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return nil, 0, false, fmt.Errorf("%w: 第 %d 个号码不是有效整数", err168BingoRawInvalid, index+1)
		}
		number, err := strconv.Atoi(token)
		if err != nil {
			return nil, 0, false, fmt.Errorf("%w: 第 %d 个号码不是有效整数", err168BingoRawInvalid, index+1)
		}
		numbers[index] = number
	}
	if len(numbers) == 21 {
		// The 168 KL8 endpoint occasionally appends one already-listed ball to
		// an otherwise complete 20-ball draw. Accept only that exact provider
		// defect: the authoritative first 20 balls must still be valid and the
		// extra token must duplicate one of them. Never discard an arbitrary
		// 21st value, because that could silently publish a damaged draw.
		firstTwenty := numbers[:20]
		if err := validate168BingoNumbers(firstTwenty); err != nil {
			return nil, 0, false, err
		}
		extra := numbers[20]
		duplicate := false
		for _, number := range firstTwenty {
			if number == extra {
				duplicate = true
				break
			}
		}
		if !duplicate {
			return nil, 0, false, fmt.Errorf("%w: 第 21 个附加号码 %d 未在前 20 个号码中重复", err168BingoRawInvalid, extra)
		}
		return append([]int(nil), firstTwenty...), extra, true, nil
	}
	if err := validate168BingoNumbers(numbers); err != nil {
		return nil, 0, false, err
	}
	return numbers, 0, false, nil
}

func validate168BingoNumbers(numbers []int) error {
	if len(numbers) != 20 {
		return fmt.Errorf("%w: 应有 20 个原始号码，实际 %d 个", err168BingoRawInvalid, len(numbers))
	}
	var seen [81]bool
	for index, number := range numbers {
		if number < 1 || number > 80 {
			return fmt.Errorf("%w: 第 %d 个号码超出 1–80", err168BingoRawInvalid, index+1)
		}
		if seen[number] {
			return fmt.Errorf("%w: 原始号码 %d 重复", err168BingoRawInvalid, number)
		}
		seen[number] = true
	}
	return nil
}

func sourceDrawsFrom168Rows(rows []api168Row, transform func([]int) []int) []sourceDraw {
	result := make([]sourceDraw, 0, len(rows))
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		issue := strings.TrimSpace(row.IssueText())
		if issue == "" || strings.TrimSpace(row.Code) == "" || seen[issue] {
			continue
		}
		numbers := parseNumberList(row.Code)
		if transform != nil {
			numbers = transform(numbers)
		}
		if len(numbers) == 0 {
			continue
		}
		seen[issue] = true
		result = append(result, sourceDraw{
			Issue: issue, Numbers: numbers, DrawAt: parse168DrawTime(row.Time),
			NextIssue: api168IssueText(row.NextIssue), NextDrawAt: parse168DrawTime(row.NextTime),
		})
	}
	return result
}

func mergeSourceDraws(first, additional []sourceDraw) []sourceDraw {
	result := make([]sourceDraw, 0, len(first)+len(additional))
	seen := make(map[string]bool, len(first)+len(additional))
	for _, group := range [][]sourceDraw{first, additional} {
		for _, draw := range group {
			if !seen[draw.Issue] {
				seen[draw.Issue] = true
				result = append(result, draw)
			}
		}
	}
	return result
}

func mergeEquivalent168BingoDraw(first, second sourceDraw) (sourceDraw, error) {
	issue := strings.TrimSpace(first.Issue)
	if issue == "" || issue != strings.TrimSpace(second.Issue) ||
		!sameIntSequence(first.Numbers, second.Numbers) ||
		first.HasBingoSourceTail != second.HasBingoSourceTail ||
		(first.HasBingoSourceTail && first.BingoSourceTail != second.BingoSourceTail) ||
		first.DrawAt.IsZero() || second.DrawAt.IsZero() || !first.DrawAt.Equal(second.DrawAt) {
		return sourceDraw{}, fmt.Errorf("%w: 168期号 %s 的重复记录号码、末球或开奖时间不一致", err168BingoRawInvalid, issue)
	}
	if first.NextIssue != "" && second.NextIssue != "" && first.NextIssue != second.NextIssue {
		return sourceDraw{}, fmt.Errorf("%w: 168期号 %s 的重复记录下一期号不一致", err168BingoRawInvalid, issue)
	}
	if !first.NextDrawAt.IsZero() && !second.NextDrawAt.IsZero() && !first.NextDrawAt.Equal(second.NextDrawAt) {
		return sourceDraw{}, fmt.Errorf("%w: 168期号 %s 的重复记录下一期开奖时间不一致", err168BingoRawInvalid, issue)
	}
	if first.NextIssue == "" {
		first.NextIssue = second.NextIssue
	}
	if first.NextDrawAt.IsZero() {
		first.NextDrawAt = second.NextDrawAt
	}
	return first, nil
}

func api168Paths(series api168Series) (latest, history string) {
	switch series {
	case api168PK10:
		return "/pks/getLotteryPksInfo.do", "/pks/getPksHistoryList.do"
	case api168SSC:
		return "/CQShiCai/getBaseCQShiCai.do", "/CQShiCai/getBaseCQShiCaiList.do"
	case api168LHC:
		return "/6hc/getLotteryInfo.do", "/6hc/getHistoryLotteryInfo.do"
	case api168KL8:
		return "/LuckTwenty/getBaseLuckTewnty.do", "/LuckTwenty/getBaseLuckTwentyList.do"
	default:
		return "", ""
	}
}

type api168Envelope struct {
	ErrorCode int    `json:"errorCode"`
	Message   string `json:"message"`
	Result    struct {
		Data json.RawMessage `json:"data"`
	} `json:"result"`
}

type api168Row struct {
	Issue     any    `json:"preDrawIssue"`
	Time      string `json:"preDrawTime"`
	Code      string `json:"preDrawCode"`
	NextIssue any    `json:"drawIssue"`
	NextTime  string `json:"drawTime"`
}

func (payload api168Envelope) Rows() []api168Row {
	return parseAPI168Rows(payload.Result.Data)
}

func parseAPI168Rows(raw json.RawMessage) []api168Row {
	rows, _ := parseAPI168RowsStrict(raw)
	return rows
}

func parseAPI168RowsStrict(raw json.RawMessage) ([]api168Row, error) {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == `""` {
		return nil, nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "{") {
		var row api168Row
		if err := json.Unmarshal(raw, &row); err != nil {
			return nil, err
		}
		return []api168Row{row}, nil
	}
	var rows []api168Row
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (row api168Row) IssueText() string {
	return api168IssueText(row.Issue)
}

func api168IssueText(issue any) string {
	switch value := issue.(type) {
	case string:
		return strings.TrimSpace(value)
	case float64:
		return strconv.FormatInt(int64(value), 10)
	case json.Number:
		return value.String()
	default:
		return ""
	}
}

func parse168DrawTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	location, _ := time.LoadLocation("Asia/Shanghai")
	if unix, err := strconv.ParseInt(value, 10, 64); err == nil && unix > 1_000_000_000 {
		if unix >= 1_000_000_000_000 {
			return time.UnixMilli(unix).UTC()
		}
		return time.Unix(unix, 0).In(location).UTC()
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC()
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func bingoSSCNumbers(offset int) func([]int) []int {
	return func(raw []int) []int {
		if validate168BingoNumbers(raw) != nil || offset < 0 || len(raw) < offset+5 {
			return nil
		}
		out := make([]int, 5)
		for i := 0; i < 5; i++ {
			out[i] = raw[offset+i] % 10
		}
		return out
	}
}

// bingoRacingARankV1Numbers implements the published Bingo Racing (A)
// conversion contract. Only the first ten balls participate: the smallest is
// car 1, the next-smallest car 2, through car 10; the result remains in actual
// draw order. Callers must supply cross-validated ordered raw balls—feeding the
// sorted 168 set directly would always fabricate 1..10 and is forbidden.
func bingoRacingARankV1Numbers(raw []int) []int {
	if validate168BingoNumbers(raw) != nil {
		return nil
	}
	window := append([]int(nil), raw[:10]...)
	sortedWindow := append([]int(nil), window...)
	sort.Ints(sortedWindow)
	rank := make(map[int]int, len(sortedWindow))
	for index, number := range sortedWindow {
		rank[number] = index + 1
	}
	result := make([]int, len(window))
	for index, number := range window {
		result[index] = rank[number]
	}
	return result
}

func bingoRacingNumbers(offset int) func([]int) []int {
	return func(raw []int) []int {
		if validate168BingoNumbers(raw) != nil || offset < 0 || len(raw) < offset+10 {
			return nil
		}
		used := map[int]bool{}
		out := make([]int, 0, 10)
		for i := 0; i < len(raw) && len(out) < 10; i++ {
			idx := (offset + i) % len(raw)
			candidate := raw[idx]%10 + 1
			if candidate == 0 {
				candidate = 10
			}
			if used[candidate] {
				continue
			}
			used[candidate] = true
			out = append(out, candidate)
		}
		if len(out) < 10 {
			for n := 1; n <= 10 && len(out) < 10; n++ {
				if !used[n] {
					out = append(out, n)
				}
			}
		}
		if len(out) != 10 {
			return nil
		}
		return out
	}
}

func bingoMarkSixNumbers(raw []int) []int {
	if validate168BingoNumbers(raw) != nil {
		return nil
	}
	out := make([]int, 0, 7)
	for _, number := range raw {
		if number < 1 || number > 49 {
			continue
		}
		out = append(out, number)
		if len(out) == 7 {
			break
		}
	}
	if len(out) < 7 {
		return nil
	}
	return out
}
