package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"backend/data/models/lottery"

	"gorm.io/gorm"
)

const (
	api168Base    = "https://api.api16868.com"
	api168Referer = "https://kj138138.com/"
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

var api168BingoBindings = []struct {
	GameID    string
	Transform func([]int) []int
}{
	{GameID: "bingo-ssc-1", Transform: bingoSSCNumbers(0)},
	{GameID: "bingo-ssc-2", Transform: bingoSSCNumbers(1)},
	{GameID: "bingo-ssc-3", Transform: bingoSSCNumbers(2)},
	{GameID: "bingo-ssc-4", Transform: bingoSSCNumbers(3)},
	{GameID: "bingo-racing-a", Transform: bingoRacingNumbers(0)},
	{GameID: "bingo-racing-b", Transform: bingoRacingNumbers(10)},
	{GameID: "bingo-mark-six", Transform: bingoMarkSixNumbers},
}

func (s *LotteryService) sync168HighFreq(ctx context.Context) []SourceSyncResult {
	gameIDs := make([]string, 0, len(api168HighFreqBindings))
	for _, binding := range api168HighFreqBindings {
		gameIDs = append(gameIDs, binding.GameID)
	}
	enabled, err := s.enabledOfficialGames(gameIDs)
	if err != nil {
		return []SourceSyncResult{{GameID: "168-highfreq", Status: "error", Error: err.Error()}}
	}
	results := make([]SourceSyncResult, 0, len(api168HighFreqBindings))
	for _, item := range api168HighFreqBindings {
		binding := item
		if !enabled[binding.GameID] {
			results = append(results, SourceSyncResult{GameID: binding.GameID, Status: "ok"})
			continue
		}
		historyIncluded := false
		result := sync168LatestThenHistory(ctx,
			func(ctx context.Context) SourceSyncResult {
				return s.syncOfficialGame(ctx, binding.GameID, func(ctx context.Context) ([]sourceDraw, error) {
					draws, included, err := fetch168LiveDraws(ctx, binding, request168JSON)
					historyIncluded = included
					return draws, err
				})
			},
			func(ctx context.Context) ([]sourceDraw, error) {
				if historyIncluded {
					return nil, nil
				}
				return fetch168History(ctx, binding.Series, binding.LotCode, nil, time.Now(), request168JSON)
			},
			func(ctx context.Context, draws []sourceDraw) (int, error) {
				return s.importOfficialHistory(ctx, binding.GameID, draws)
			})
		results = append(results, result)
	}
	return results
}

func fetch168LiveDraws(ctx context.Context, binding api168Binding, request api168Request) ([]sourceDraw, bool, error) {
	draws, err := fetch168Latest(ctx, binding.Series, binding.LotCode, nil, request)
	if err != nil {
		return nil, false, err
	}
	var latest sourceDraw
	for _, draw := range draws {
		if latest.DrawAt.IsZero() || draw.DrawAt.After(latest.DrawAt) {
			latest = draw
		}
	}
	if !latest.DrawAt.IsZero() && validNextSourceIssue(latest.Issue, latest.NextIssue) && latest.NextDrawAt.After(latest.DrawAt) {
		return draws, false, nil
	}
	// If the provider omits its upcoming boundary, enough historic evidence is
	// still required to infer cadence. A lone latest row must not resurrect an
	// arbitrary seed interval (for example 180 seconds instead of 75 seconds).
	history, historyErr := fetch168History(ctx, binding.Series, binding.LotCode, nil, time.Now(), request)
	draws = mergeSourceDraws(draws, history)
	if observedDrawInterval(draws) == 0 {
		return nil, true, errors.Join(fmt.Errorf("尚未取得有效的下一期开奖时间或足够的历史周期"), historyErr)
	}
	return draws, true, nil
}

// Keep the existing bounded two-day recovery pass, but never make a fresh
// result wait for those slower history requests before publishing its next
// period. No extra requests, polling loops or detached goroutines are added.
func sync168LatestThenHistory(ctx context.Context, syncLatest func(context.Context) SourceSyncResult, fetchHistory func(context.Context) ([]sourceDraw, error), importHistory func(context.Context, []sourceDraw) (int, error)) SourceSyncResult {
	result := syncLatest(ctx)
	if result.Status != "ok" {
		return result
	}
	draws, fetchErr := fetchHistory(ctx)
	if len(draws) > 0 {
		imported, err := importHistory(ctx, draws)
		if err != nil {
			fetchErr = errors.Join(fetchErr, err)
		} else {
			result.Imported += imported
		}
	}
	if fetchErr != nil {
		// A backfill failure cannot erase a valid live result or clear an error
		// written by a newer live sync. The same dates retry on the next pass.
		log.Printf("历史开奖补采未完成，将在下轮重试: game=%s error=%v", result.GameID, fetchErr)
	}
	return result
}

func (s *LotteryService) sync168MarkSix(ctx context.Context) []SourceSyncResult {
	results := make([]SourceSyncResult, 0, len(api168MarkSixBindings))
	for _, item := range api168MarkSixBindings {
		binding := item
		results = append(results, s.syncOfficialGame(ctx, binding.GameID, func(ctx context.Context) ([]sourceDraw, error) {
			return fetch168Recent(ctx, binding.Series, binding.LotCode, nil)
		}))
	}
	return results
}

func (s *LotteryService) sync168Bingo(ctx context.Context) []SourceSyncResult {
	gameIDs := make([]string, 0, len(api168BingoBindings))
	for _, item := range api168BingoBindings {
		gameIDs = append(gameIDs, item.GameID)
	}
	enabled, enabledErr := s.enabledOfficialGames(gameIDs)
	if enabledErr != nil {
		return []SourceSyncResult{{GameID: "168-bingo", Status: "error", Error: enabledErr.Error()}}
	}
	if len(enabled) == 0 {
		return []SourceSyncResult{{GameID: "168-bingo", Status: "ok"}}
	}

	raw, err := fetch168Recent(ctx, api168KL8, "10047", nil)
	results := make([]SourceSyncResult, 0, len(enabled))
	if err != nil {
		for _, item := range api168BingoBindings {
			if !enabled[item.GameID] {
				continue
			}
			results = append(results, s.recordSyncError(item.GameID, err))
		}
		return results
	}
	for _, item := range api168BingoBindings {
		if !enabled[item.GameID] {
			continue
		}
		binding := item
		draws := make([]sourceDraw, 0, len(raw))
		for _, row := range raw {
			numbers := binding.Transform(row.Numbers)
			if len(numbers) == 0 {
				continue
			}
			draws = append(draws, sourceDraw{Issue: row.Issue, Numbers: numbers, DrawAt: row.DrawAt, NextIssue: row.NextIssue, NextDrawAt: row.NextDrawAt})
		}
		results = append(results, s.syncOfficialGame(ctx, binding.GameID, func(context.Context) ([]sourceDraw, error) {
			if len(draws) == 0 {
				return nil, fmt.Errorf("168开奖网未返回可映射的台湾宾果记录")
			}
			return draws, nil
		}))
	}
	return results
}

func fetch168Recent(ctx context.Context, series api168Series, lotCode string, transform func([]int) []int) ([]sourceDraw, error) {
	latest, err := fetch168Latest(ctx, series, lotCode, transform, request168JSON)
	if err != nil {
		return nil, err
	}
	history, _ := fetch168History(ctx, series, lotCode, transform, time.Now(), request168JSON)
	return mergeSourceDraws(latest, history), nil
}

type api168Request func(context.Context, string, *api168Envelope) error

func request168JSON(ctx context.Context, endpoint string, payload *api168Envelope) error {
	_, err := doJSONRequest(ctx, &http.Client{Timeout: 15 * time.Second}, endpoint, api168Referer, payload)
	return err
}

func fetch168Latest(ctx context.Context, series api168Series, lotCode string, transform func([]int) []int, request api168Request) ([]sourceDraw, error) {
	latestPath, _ := api168Paths(series)
	if latestPath == "" {
		return nil, fmt.Errorf("未知 168 彩种系列")
	}
	var latestPayload api168Envelope
	latestURL := api168Base + latestPath + "?lotCode=" + url.QueryEscape(lotCode)
	if err := request(ctx, latestURL, &latestPayload); err != nil {
		return nil, fmt.Errorf("168开奖网读取失败: %w", err)
	}
	if latestPayload.ErrorCode != 0 {
		return nil, fmt.Errorf("168开奖网返回错误: %s", latestPayload.Message)
	}
	result := sourceDrawsFrom168Rows(latestPayload.Rows(), transform)
	if len(result) == 0 {
		return nil, fmt.Errorf("168开奖网未返回开奖记录")
	}

	return result, nil
}

func fetch168History(ctx context.Context, series api168Series, lotCode string, transform func([]int) []int, now time.Time, request api168Request) ([]sourceDraw, error) {
	_, historyPath := api168Paths(series)
	if historyPath == "" {
		return nil, fmt.Errorf("未知 168 彩种系列")
	}
	urls := make([]string, 0, 2)
	historyURL := api168Base + historyPath + "?lotCode=" + url.QueryEscape(lotCode)
	location, _ := time.LoadLocation("Asia/Shanghai")
	if series == api168LHC {
		urls = append(urls, historyURL)
	} else {
		for dayOffset := 0; dayOffset < 2; dayOffset++ {
			date := now.In(location).AddDate(0, 0, -dayOffset).Format("2006-01-02")
			urls = append(urls, historyURL+"&date="+url.QueryEscape(date))
		}
	}
	var result []sourceDraw
	var fetchErrors []error
	for _, endpoint := range urls {
		var historyPayload api168Envelope
		if err := request(ctx, endpoint, &historyPayload); err != nil {
			fetchErrors = append(fetchErrors, err)
			continue
		}
		if historyPayload.ErrorCode != 0 {
			fetchErrors = append(fetchErrors, fmt.Errorf("168历史开奖返回错误: %s", historyPayload.Message))
			continue
		}
		result = mergeSourceDraws(result, sourceDrawsFrom168Rows(historyPayload.Rows(), transform))
	}
	return result, errors.Join(fetchErrors...)
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
	if len(raw) == 0 || string(raw) == "null" || string(raw) == `""` {
		return nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "{") {
		var row api168Row
		if err := json.Unmarshal(raw, &row); err != nil {
			return nil
		}
		return []api168Row{row}
	}
	var rows []api168Row
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil
	}
	return rows
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
		if len(raw) < offset+5 {
			return nil
		}
		out := make([]int, 5)
		for i := 0; i < 5; i++ {
			out[i] = raw[offset+i] % 10
		}
		return out
	}
}

func bingoRacingNumbers(offset int) func([]int) []int {
	return func(raw []int) []int {
		if len(raw) < offset+10 {
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
	if len(raw) < 7 {
		return nil
	}
	used := map[int]bool{}
	out := make([]int, 0, 7)
	for i := 0; i < len(raw) && len(out) < 7; i++ {
		n := raw[i]%49 + 1
		if used[n] {
			continue
		}
		used[n] = true
		out = append(out, n)
	}
	if len(out) < 7 {
		return nil
	}
	return out
}

func Ensure168SourceGames(db *gorm.DB) error {
	ids := make([]string, 0, 32)
	for _, item := range api168HighFreqBindings {
		ids = append(ids, item.GameID)
	}
	for _, item := range api168MarkSixBindings {
		ids = append(ids, item.GameID)
	}
	for _, item := range api168BingoBindings {
		ids = append(ids, item.GameID)
	}
	for _, id := range ids {
		if err := db.Model(&lottery.Game{}).Where("id = ?", id).Updates(map[string]any{
			// 168 is a public third-party aggregation source, not an issuing
			// authority. Keep that distinction explicit for the API/UI and audits.
			"source_kind": "external",
			"source_name": "168开奖网",
			"source_url":  "https://kj138138.com/view/api/index.html",
			"sync_status": "idle",
		}).Error; err != nil {
			return err
		}
	}
	return nil
}
