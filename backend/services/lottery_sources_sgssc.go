package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	sgSSCVerifiedSourceName   = "SG时时彩母源（163:64＋115校验）"
	sgSSCVerifiedSourceURL    = source163MirrorURL
	sgSSCLegacySourceName     = "SG时时彩双站核对（168＋115）"
	sgSSCLegacySourceURL      = "https://pkk168.com/webapp/html/shishicai_sg/index.html"
	sgSSCLegacySourceRevision = "sgssc-168-115-v1"
	sgSSCSourceRevision       = "sgssc-163-64-115-v2"
	sgSSCConversionRevision   = "sgssc-direct-v1"
	sgSSCPendingMessage       = "等待SG时时彩163母源与115校验源完成期号、时间及五球核对"

	sgSSCWindowSize      = 24 // Two-hour automatic recovery only; not an unlimited backfill.
	sgSSCPeriodsPerDay   = 288
	sgSSCInterval        = 5 * time.Minute
	sgSSCTotalTimeout    = 12 * time.Second
	sgSSCRequestTimeout  = 5 * time.Second
	sgSSCMaxResponseSize = 512 * 1024
	sgSSCLatestPath      = "/CQShiCai/getBaseCQShiCai.do"
	sgSSCHistoryPath     = "/CQShiCai/getBaseCQShiCaiList.do"
)

var sgSSC163MotherBinding = source163MirrorBinding{
	GameID: "sg-ssc", UpstreamGameID: 64, Count: 5, Min: 0, Max: 9,
	Revision: sgSSCSourceRevision,
}

var sgSSCLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

type sgSSCStation struct {
	name, base, lotCode, referer string
}

func sgSSCStations() [2]sgSSCStation {
	// These are two public aggregation sites, not proven independent upstreams.
	// In particular, do not replace the other games' api.api16868.com base.
	return [2]sgSSCStation{
		{"168", "https://api.api168168.com", "10075", "https://pkk168.com/"},
		{"115", "https://www.115kai.com", "sgssc", "https://www.115kai.com/"},
	}
}

func sgSSCValidationStation() sgSSCStation { return sgSSCStations()[1] }

type sgSSCRequest func(context.Context, string) ([]byte, error)

type sgSSCEnvelope struct {
	ErrorCode *int `json:"errorCode"`
	Result    *struct {
		BusinessCode *int            `json:"businessCode"`
		Data         json.RawMessage `json:"data"`
	} `json:"result"`
}

// Only raw identity, period, time and ordered balls are authoritative. Derived
// sums, dragon/tiger flags, serverTime and provider countdowns are not consumed.
type sgSSCRow struct {
	LotCode      json.RawMessage `json:"lotCode"`
	LotName      *string         `json:"lotName"`
	TotalCount   *int            `json:"totalCount"`
	Issue        json.RawMessage `json:"preDrawIssue"`
	DrawTime     string          `json:"preDrawTime"`
	Code         string          `json:"preDrawCode"`
	NextIssue    json.RawMessage `json:"drawIssue"`
	NextDrawTime string          `json:"drawTime"`
}

func fetchSGSSCVerified(ctx context.Context) ([]sourceDraw, error) {
	return fetchSGSSCVerifiedWithRequests(ctx, time.Now, rand.Reader, request163Mirror, requestSGSSCJSON)
}

// Each attempt performs four requests, or five when the 24-period window spans
// two issue dates. The 163 mother and 115 verifier must supply every period in
// that bounded window.
// Old gaps outside it are not imported or silently manufactured. Longer outages
// require a separate audited backfill; this poll never walks unlimited dates.
func fetchSGSSCVerifiedWithRequest(ctx context.Context, now func() time.Time, request sgSSCRequest) ([]sourceDraw, error) {
	return fetchSGSSCVerifiedWithRequests(ctx, now, rand.Reader, source163MirrorRequest(request), request)
}

// 163 ID 64 is the only authoritative writer input. 115 is a read-only
// verifier: it may veto a period but can never supply a missing mother-source
// row. Keeping the requests injectable lets tests prove that a single-source
// response, a disagreement, or a stale frame fails closed.
func fetchSGSSCVerifiedWithRequests(ctx context.Context, now func() time.Time, entropy io.Reader, request163 source163MirrorRequest, request115 sgSSCRequest) ([]sourceDraw, error) {
	if ctx == nil || now == nil || entropy == nil || request163 == nil || request115 == nil {
		return nil, fmt.Errorf("SG时时彩核对依赖不可用")
	}
	startedAt := now()
	if startedAt.IsZero() {
		return nil, fmt.Errorf("SG时时彩核对本机时间无效")
	}
	ctx, cancel := context.WithTimeout(ctx, sgSSCTotalTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	validation := sgSSCValidationStation()
	var mother []sourceDraw
	var motherLatestBody []byte
	var verifierLatest sourceDraw
	group, groupContext := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		capture := func(requestContext context.Context, endpoint string) ([]byte, error) {
			body, requestErr := request163(requestContext, endpoint)
			if requestErr == nil {
				if strictErr := validateSGSSCJSONDocument(body); strictErr != nil {
					return nil, fmt.Errorf("SG时时彩163:64母源JSON无效: %w", strictErr)
				}
				parsed, parseErr := url.Parse(endpoint)
				if parseErr == nil && parsed.Path == source163LatestPath {
					motherLatestBody = append([]byte(nil), body...)
				}
			}
			return body, requestErr
		}
		mother, err = fetch163MirrorDrawsWithRequest(groupContext, sgSSC163MotherBinding, func() time.Time { return startedAt }, entropy, capture)
		if err != nil {
			return fmt.Errorf("SG时时彩163:64母源: %w", err)
		}
		return nil
	})
	group.Go(func() error {
		body, err := request115(groupContext, sgSSCEndpoint(validation, ""))
		if err != nil {
			return fmt.Errorf("SG时时彩115校验源最新记录: %w", err)
		}
		if len(body) == 0 || len(body) > sgSSCMaxResponseSize {
			return fmt.Errorf("SG时时彩115校验源响应为空或超过大小限制")
		}
		verifierLatest, err = parseSGSSCLatest(body, validation)
		if err != nil {
			return fmt.Errorf("SG时时彩115校验源最新记录: %w", err)
		}
		return nil
	})
	if err := group.Wait(); err != nil {
		return nil, err
	}
	if len(mother) == 0 {
		return nil, fmt.Errorf("SG时时彩163:64母源没有开奖记录")
	}
	motherLatest := mother[0]
	for _, draw := range mother[1:] {
		if draw.DrawAt.After(motherLatest.DrawAt) {
			motherLatest = draw
		}
	}
	motherNextIssue, motherNextAt, err := parseSGSSC163Schedule(motherLatestBody, motherLatest)
	if err != nil {
		return nil, err
	}
	if !sameSGSSCResult(motherLatest, verifierLatest) {
		return nil, fmt.Errorf("SG时时彩163:64母源与115校验源最新期号、五球或时间不一致")
	}
	if motherNextIssue != verifierLatest.NextIssue || !motherNextAt.Equal(verifierLatest.NextDrawAt) {
		return nil, fmt.Errorf("SG时时彩163:64母源与115校验源下一期期号或时间不一致")
	}
	day, ordinal, _, err := parseSGSSCIssue(motherLatest.Issue)
	if err != nil {
		return nil, err
	}
	dates := []string{day.Format("2006-01-02")}
	if ordinal < sgSSCWindowSize {
		dates = append(dates, day.AddDate(0, 0, -1).Format("2006-01-02"))
	}
	historyURLs := make([]string, 0, len(dates))
	for _, date := range dates {
		historyURLs = append(historyURLs, sgSSCEndpoint(validation, date))
	}
	bodies, err := requestSGSSCBatch(ctx, historyURLs, request115)
	if err != nil {
		return nil, err
	}
	motherHistory := make(map[string]sourceDraw, len(mother))
	for _, row := range mother {
		if _, duplicate := motherHistory[row.Issue]; duplicate {
			return nil, fmt.Errorf("SG时时彩163:64母源历史期号%s重复", row.Issue)
		}
		motherHistory[row.Issue] = row
	}
	verifierHistory := make(map[string]sourceDraw)
	for dateIndex, date := range dates {
		rows, parseErr := parseSGSSCHistory(bodies[dateIndex], validation, date, verifierLatest)
		if parseErr != nil {
			return nil, fmt.Errorf("SG时时彩115校验源历史%s: %w", date, parseErr)
		}
		for _, row := range rows {
			if _, duplicate := verifierHistory[row.Issue]; duplicate {
				return nil, fmt.Errorf("SG时时彩115校验源历史期号%s重复", row.Issue)
			}
			verifierHistory[row.Issue] = row
		}
	}
	row, found := verifierHistory[verifierLatest.Issue]
	if !found || !sameSGSSCResult(row, verifierLatest) {
		return nil, fmt.Errorf("SG时时彩115校验源最新与历史不一致")
	}
	result := make([]sourceDraw, 0, sgSSCWindowSize)
	for offset := sgSSCWindowSize - 1; offset >= 0; offset-- {
		drawAt := motherLatest.DrawAt.Add(-time.Duration(offset) * sgSSCInterval)
		issue := sgSSCIssueAt(drawAt)
		first, firstFound := motherHistory[issue]
		second, secondFound := verifierHistory[issue]
		if !firstFound || !secondFound {
			return nil, fmt.Errorf("SG时时彩最近%d期窗口缺少163:64母源或115校验源期号%s", sgSSCWindowSize, issue)
		}
		if !sameSGSSCResult(first, second) || !first.DrawAt.Equal(drawAt) {
			return nil, fmt.Errorf("SG时时彩期号%s的163:64母源与115校验源五球或时间不一致", issue)
		}
		first.SourceRevision, first.ConversionRevision = sgSSCSourceRevision, sgSSCConversionRevision
		first.Numbers = append([]int(nil), first.Numbers...)
		result = append(result, first)
	}
	for index := range result {
		if index+1 < len(result) {
			result[index].NextIssue, result[index].NextDrawAt = result[index+1].Issue, result[index+1].DrawAt
		} else {
			result[index].NextIssue, result[index].NextDrawAt = motherNextIssue, motherNextAt
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Network work may cross a draw boundary. A stale next period must never be
	// opened using a cached serverTime or by extrapolating another future issue.
	finishedAt := now()
	if finishedAt.IsZero() || finishedAt.Before(startedAt) {
		return nil, fmt.Errorf("SG时时彩核对期间本机时钟无效或回退")
	}
	if err := validateSGSSCFreshness(result[len(result)-1], finishedAt); err != nil {
		return nil, err
	}
	if err := validateSGSSCVerifiedBatch(result); err != nil {
		return nil, err
	}
	return result, nil
}

// The 163 latest endpoint has several similarly named clocks. Only
// nextPeriodOpenTime is an absolute millisecond timestamp; nextOpenTime is a
// countdown and drealopen is a publication time, so neither is accepted here.
func parseSGSSC163Schedule(body []byte, latest sourceDraw) (string, time.Time, error) {
	var payload source163Envelope
	if sourceProbeDecode(body, &payload) != nil || payload.Success == nil || !*payload.Success || sourceProbeJSONEmpty(payload.Result) {
		return "", time.Time{}, fmt.Errorf("SG时时彩163:64母源下一期响应结构无效")
	}
	var row struct {
		GameID       json.Number `json:"igameid"`
		NextIssue    any         `json:"nextGamePeriod"`
		NextOpenTime any         `json:"nextPeriodOpenTime"`
	}
	if sourceProbeDecode(payload.Result, &row) != nil {
		return "", time.Time{}, fmt.Errorf("SG时时彩163:64母源下一期结构无效")
	}
	id, err := row.GameID.Int64()
	issue := api168IssueText(row.NextIssue)
	if err != nil || id != int64(sgSSC163MotherBinding.UpstreamGameID) || issue == "" {
		return "", time.Time{}, fmt.Errorf("SG时时彩163:64母源下一期身份或期号无效")
	}
	expectedIssue := sgSSCIssueAt(latest.DrawAt.Add(sgSSCInterval))
	_, _, expectedAt, issueErr := parseSGSSCIssue(issue)
	if issueErr != nil || issue != expectedIssue {
		return "", time.Time{}, fmt.Errorf("SG时时彩163:64母源下一期期号不连续")
	}
	providedAt := parse168DrawTime(api168IssueText(row.NextOpenTime))
	if providedAt.IsZero() || !providedAt.Equal(expectedAt) {
		return "", time.Time{}, fmt.Errorf("SG时时彩163:64母源下一期时间与期号不一致")
	}
	return issue, expectedAt, nil
}

func sgSSCEndpoint(station sgSSCStation, date string) string {
	query := url.Values{"lotCode": {station.lotCode}}
	path := sgSSCLatestPath
	if date != "" {
		path = sgSSCHistoryPath
		query.Set("date", date)
	}
	return station.base + path + "?" + query.Encode()
}

func requestSGSSCBatch(ctx context.Context, endpoints []string, request sgSSCRequest) ([][]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	group, groupContext := errgroup.WithContext(ctx)
	bodies := make([][]byte, len(endpoints))
	for index, endpoint := range endpoints {
		group.Go(func() error {
			body, err := request(groupContext, endpoint)
			if err != nil {
				return fmt.Errorf("SG时时彩读取%s失败: %w", endpoint, err)
			}
			if len(body) == 0 || len(body) > sgSSCMaxResponseSize {
				return fmt.Errorf("SG时时彩响应为空或超过大小限制")
			}
			bodies[index] = body
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return bodies, ctx.Err()
}

func requestSGSSCJSON(ctx context.Context, endpoint string) ([]byte, error) {
	client := &http.Client{
		Timeout:       sgSSCRequestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return requestSGSSCJSONWithClient(ctx, endpoint, client)
}

func requestSGSSCJSONWithClient(ctx context.Context, endpoint string, client *http.Client) ([]byte, error) {
	if ctx == nil || client == nil {
		return nil, fmt.Errorf("SG时时彩请求依赖不可用")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Fragment != "" || parsed.RawPath != "" {
		return nil, fmt.Errorf("SG时时彩来源地址不合法")
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return nil, fmt.Errorf("SG时时彩来源参数不合法")
	}
	var referer string
	for _, station := range sgSSCStations() {
		if parsed.Scheme+"://"+parsed.Host == station.base && query.Get("lotCode") == station.lotCode {
			referer = station.referer
		}
	}
	if referer == "" || (parsed.Path != sgSSCLatestPath && parsed.Path != sgSSCHistoryPath) {
		return nil, fmt.Errorf("SG时时彩来源不在固定白名单")
	}
	for key, values := range query {
		if len(values) != 1 || (key != "lotCode" && key != "date") {
			return nil, fmt.Errorf("SG时时彩来源参数不合法")
		}
	}
	if parsed.Path == sgSSCHistoryPath {
		if _, err := time.Parse("2006-01-02", query.Get("date")); err != nil {
			return nil, fmt.Errorf("SG时时彩历史日期不合法")
		}
	} else if query.Has("date") {
		return nil, fmt.Errorf("SG时时彩最新接口不接受历史日期")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", officialUserAgent)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Referer", referer)
	boundedClient := *client
	if boundedClient.Timeout <= 0 || boundedClient.Timeout > sgSSCRequestTimeout {
		boundedClient.Timeout = sgSSCRequestTimeout
	}
	boundedClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := boundedClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SG时时彩来源HTTP状态%d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, sgSSCMaxResponseSize+1))
	if err != nil {
		return nil, err
	}
	if len(body) == 0 || len(body) > sgSSCMaxResponseSize {
		return nil, fmt.Errorf("SG时时彩响应为空或超过大小限制")
	}
	return body, nil
}

func decodeSGSSCData(body []byte) (json.RawMessage, error) {
	if len(body) == 0 || len(body) > sgSSCMaxResponseSize {
		return nil, fmt.Errorf("响应为空或超过大小限制")
	}
	// encoding/json otherwise accepts duplicate keys with last-value-wins
	// semantics. Conflicting success codes, identities or raw draws are not a
	// valid source contract, even when the last value happens to look correct.
	if err := validateSGSSCJSONDocument(body); err != nil {
		return nil, err
	}
	var envelope sgSSCEnvelope
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("响应不是有效JSON: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	if envelope.ErrorCode == nil || *envelope.ErrorCode != 0 || envelope.Result == nil || envelope.Result.BusinessCode == nil || *envelope.Result.BusinessCode != 0 {
		return nil, fmt.Errorf("响应缺少明确成功的双层业务码")
	}
	return envelope.Result.Data, nil
}

func validateSGSSCJSONDocument(body []byte) error {
	keyDecoder := json.NewDecoder(bytes.NewReader(body))
	keyDecoder.UseNumber()
	if err := scanSGSSCJSONValue(keyDecoder, 0); err != nil {
		return err
	}
	return ensureJSONEOF(keyDecoder)
}

func scanSGSSCJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return fmt.Errorf("SG时时彩JSON嵌套过深")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	if delimiter != '{' && delimiter != '[' {
		return fmt.Errorf("SG时时彩JSON结构无效")
	}
	keys := map[string]bool{}
	for decoder.More() {
		if delimiter == '{' {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, valid := keyToken.(string)
			// Struct decoding is case-insensitive, so aliases are duplicates too.
			key = strings.ToLower(key)
			if !valid || keys[key] {
				return fmt.Errorf("SG时时彩JSON字段重复或无效")
			}
			keys[key] = true
		}
		if err := scanSGSSCJSONValue(decoder, depth+1); err != nil {
			return err
		}
	}
	_, err = decoder.Token() // Decoder validates the corresponding closing token.
	return err
}

func parseSGSSCLatest(body []byte, station sgSSCStation) (sourceDraw, error) {
	row, draw, err := parseSGSSCIdentifiedLatest(body, station)
	if err != nil {
		return sourceDraw{}, err
	}
	draw.NextIssue, err = sgSSCScalarText(row.NextIssue)
	if err != nil {
		return sourceDraw{}, fmt.Errorf("下一期期号无效: %w", err)
	}
	_, _, nextAt, err := parseSGSSCIssue(draw.NextIssue)
	if err != nil || !nextAt.Equal(draw.DrawAt.Add(sgSSCInterval)) {
		return sourceDraw{}, fmt.Errorf("下一期期号不连续")
	}
	providedNextAt, err := time.ParseInLocation("2006-01-02 15:04:05", row.NextDrawTime, sgSSCLocation)
	if err != nil || providedNextAt.Format("2006-01-02 15:04:05") != row.NextDrawTime || !providedNextAt.Equal(nextAt) {
		return sourceDraw{}, fmt.Errorf("下一期时间与期号不一致")
	}
	draw.NextDrawAt = nextAt.UTC()
	return draw, nil
}

// History responses omit their own product identity. Both live polls and
// explicitly scoped backfills first verify the identity-bearing latest record.
// Only the live caller additionally consumes and validates next-period data.
func parseSGSSCIdentifiedLatest(body []byte, station sgSSCStation) (sgSSCRow, sourceDraw, error) {
	data, err := decodeSGSSCData(body)
	if err != nil {
		return sgSSCRow{}, sourceDraw{}, err
	}
	var row sgSSCRow
	if err := json.Unmarshal(data, &row); err != nil {
		return sgSSCRow{}, sourceDraw{}, fmt.Errorf("最新记录结构无效: %w", err)
	}
	if err := validateSGSSCIdentity(row, station, true); err != nil {
		return sgSSCRow{}, sourceDraw{}, err
	}
	draw, err := parseSGSSCRow(row)
	if err != nil {
		return sgSSCRow{}, sourceDraw{}, err
	}
	return row, draw, nil
}

func parseSGSSCHistory(body []byte, station sgSSCStation, date string, latest sourceDraw) ([]sourceDraw, error) {
	return parseSGSSCDatedHistory(body, station, date, latest.DrawAt, false)
}

// Live polling requires a nonempty history no later than that site's latest
// frame. Explicit historical targets may instead use a local-clock upper bound
// and treat a valid empty array as missing periods, never as verified draws.
func parseSGSSCDatedHistory(body []byte, station sgSSCStation, date string, upperBound time.Time, allowEmpty bool) ([]sourceDraw, error) {
	data, err := decodeSGSSCData(body)
	if err != nil {
		return nil, err
	}
	var rows []sgSSCRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("历史记录结构无效: %w", err)
	}
	if allowEmpty && len(rows) == 0 && bytes.HasPrefix(bytes.TrimSpace(data), []byte("[")) {
		return []sourceDraw{}, nil
	}
	if len(rows) == 0 || len(rows) > sgSSCPeriodsPerDay {
		return nil, fmt.Errorf("历史记录为空或超过一日期数")
	}
	seen := make(map[string]bool, len(rows))
	result := make([]sourceDraw, 0, len(rows))
	direction := 0
	for _, row := range rows {
		if err := validateSGSSCIdentity(row, station, false); err != nil {
			return nil, err
		}
		draw, err := parseSGSSCRow(row)
		if err != nil {
			return nil, err
		}
		if strings.ReplaceAll(date, "-", "") != draw.Issue[:8] || draw.DrawAt.After(upperBound) {
			return nil, fmt.Errorf("历史期号%s不在请求日期或超出允许时间上界", draw.Issue)
		}
		if seen[draw.Issue] {
			return nil, fmt.Errorf("历史期号%s重复", draw.Issue)
		}
		seen[draw.Issue] = true
		if len(result) > 0 {
			step := 1
			if draw.DrawAt.Before(result[len(result)-1].DrawAt) {
				step = -1
			}
			if direction != 0 && step != direction {
				return nil, fmt.Errorf("历史记录排序不一致")
			}
			direction = step
		}
		result = append(result, draw)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].DrawAt.Before(result[j].DrawAt) })
	return result, nil
}

func validateSGSSCIdentity(row sgSSCRow, station sgSSCStation, required bool) error {
	if required || len(row.LotCode) != 0 {
		code, err := sgSSCScalarText(row.LotCode)
		if err != nil || code != station.lotCode {
			return fmt.Errorf("彩种编号不是%s的SG时时彩", station.name)
		}
	}
	if (required && row.LotName == nil) || (row.LotName != nil && *row.LotName != "SG时时彩") {
		return fmt.Errorf("彩种名称不是SG时时彩")
	}
	if (required && row.TotalCount == nil) || (row.TotalCount != nil && *row.TotalCount != sgSSCPeriodsPerDay) {
		return fmt.Errorf("SG时时彩每日期数不是288")
	}
	return nil
}

func sgSSCScalarText(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", fmt.Errorf("缺少标识")
	}
	if raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", err
		}
		return value, nil
	}
	return string(raw), nil
}

// The issue date owns periods 001 at 00:05 through 288 at the next midnight.
// Consequently 20260902288 is drawn at 2026-09-03 00:00, and belongs to the
// history request date=2026-09-02 at both verified public endpoints.
func parseSGSSCIssue(issue string) (time.Time, int, time.Time, error) {
	if len(issue) != 11 {
		return time.Time{}, 0, time.Time{}, fmt.Errorf("SG时时彩期号必须是日期加三位序号")
	}
	for _, value := range issue {
		if value < '0' || value > '9' {
			return time.Time{}, 0, time.Time{}, fmt.Errorf("SG时时彩期号包含非数字")
		}
	}
	day, err := time.ParseInLocation("20060102", issue[:8], sgSSCLocation)
	ordinal, numberErr := strconv.Atoi(issue[8:])
	if err != nil || numberErr != nil || ordinal < 1 || ordinal > sgSSCPeriodsPerDay {
		return time.Time{}, 0, time.Time{}, fmt.Errorf("SG时时彩期号日期或序号无效")
	}
	return day, ordinal, day.Add(time.Duration(ordinal) * sgSSCInterval).UTC(), nil
}

func sgSSCIssueAt(drawAt time.Time) string {
	local := drawAt.In(sgSSCLocation)
	day, _ := time.ParseInLocation("2006-01-02", local.Add(-time.Nanosecond).Format("2006-01-02"), sgSSCLocation)
	return fmt.Sprintf("%s%03d", day.Format("20060102"), int(local.Sub(day)/sgSSCInterval))
}

func parseSGSSCRow(row sgSSCRow) (sourceDraw, error) {
	issue, err := sgSSCScalarText(row.Issue)
	if err != nil {
		return sourceDraw{}, err
	}
	_, _, expectedAt, err := parseSGSSCIssue(issue)
	if err != nil {
		return sourceDraw{}, err
	}
	drawAt, err := time.ParseInLocation("2006-01-02 15:04:05", row.DrawTime, sgSSCLocation)
	if err != nil || drawAt.Format("2006-01-02 15:04:05") != row.DrawTime || !drawAt.Equal(expectedAt) {
		return sourceDraw{}, fmt.Errorf("期号%s的开奖时间不一致", issue)
	}
	parts := strings.Split(row.Code, ",")
	if len(parts) != 5 {
		return sourceDraw{}, fmt.Errorf("期号%s必须提供五球", issue)
	}
	numbers := make([]int, 5)
	for index, part := range parts {
		if len(part) != 1 || part[0] < '0' || part[0] > '9' {
			return sourceDraw{}, fmt.Errorf("期号%s的第%d球不是0至9数字", issue, index+1)
		}
		numbers[index] = int(part[0] - '0')
	}
	return sourceDraw{Issue: issue, DrawAt: expectedAt, Numbers: numbers}, nil
}

func sameSGSSCResult(first, second sourceDraw) bool {
	if first.Issue != second.Issue || !first.DrawAt.Equal(second.DrawAt) || len(first.Numbers) != 5 || len(second.Numbers) != 5 {
		return false
	}
	for index := range first.Numbers {
		if first.Numbers[index] != second.Numbers[index] {
			return false
		}
	}
	return true
}

func validateSGSSCFreshness(latest sourceDraw, now time.Time) error {
	if now.IsZero() || latest.DrawAt.After(now) || !now.Before(latest.NextDrawAt) || now.Sub(latest.DrawAt) >= sgSSCInterval {
		return fmt.Errorf("SG时时彩最新记录落后或超前于本机当前期，暂停受理")
	}
	return nil
}

// A pure import-boundary guard. Freshness must additionally be checked against
// the local clock by the fetcher/importer; this validates the complete window's
// shape, revisions, ordered balls and actual next-period metadata only.
func validateSGSSCVerifiedBatch(draws []sourceDraw) error {
	if len(draws) != sgSSCWindowSize {
		return fmt.Errorf("SG时时彩核对批次必须包含连续%d期", sgSSCWindowSize)
	}
	for index, draw := range draws {
		_, _, expectedAt, err := parseSGSSCIssue(draw.Issue)
		if err != nil || !draw.DrawAt.Equal(expectedAt) || len(draw.Numbers) != 5 || draw.SourceRevision != sgSSCSourceRevision || draw.ConversionRevision != sgSSCConversionRevision {
			return fmt.Errorf("SG时时彩核对批次第%d条缺少有效原始记录或版本", index+1)
		}
		for _, number := range draw.Numbers {
			if number < 0 || number > 9 {
				return fmt.Errorf("SG时时彩核对批次第%d条号码越界", index+1)
			}
		}
		if draw.NextIssue != sgSSCIssueAt(draw.DrawAt.Add(sgSSCInterval)) || !draw.NextDrawAt.Equal(draw.DrawAt.Add(sgSSCInterval)) {
			return fmt.Errorf("SG时时彩核对批次第%d条下一期不连续", index+1)
		}
		if index > 0 && (draws[index-1].NextIssue != draw.Issue || !draws[index-1].NextDrawAt.Equal(draw.DrawAt)) {
			return fmt.Errorf("SG时时彩核对批次排序不连续")
		}
	}
	return nil
}
