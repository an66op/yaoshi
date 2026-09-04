package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// These parsers intentionally do not call fetchTaiwanBingo/parseDateAt: those
// legacy import helpers synthesize timestamps. Diagnostics must expose missing
// temporal evidence instead of treating a request time as an upstream draw.
func probeOfficialSource(ctx context.Context, spec sourceDiagnosticSpec, now time.Time, request *sourceProbeHTTP) (sourceProbeObservation, error) {
	observation := sourceProbeObservation{}
	id := strings.TrimPrefix(spec.Key, "official:")
	read := func(endpoint, referer string, target any) error {
		body, err := request.request(ctx, endpoint, referer)
		if err != nil {
			return err
		}
		return sourceProbeDecode(body, target)
	}
	var draws []sourceDraw
	switch id {
	case "official-fc3d", "official-kl8":
		name, page := "3d", "fc3d"
		if id == "official-kl8" {
			name, page = "kl8", "kl8"
		}
		request.client.Jar, _ = cookiejar.New(nil)
		indexURL := "https://www.cwl.gov.cn/ygkj/wqkjgg/" + page + "/"
		if _, err := request.request(ctx, indexURL, "https://www.cwl.gov.cn/"); err != nil {
			return observation, err
		}
		query := url.Values{"name": {name}, "pageNo": {"1"}, "pageSize": {"3"}, "systemType": {"PC"}, "issueCount": {""}, "issueStart": {""}, "issueEnd": {""}, "dayStart": {""}, "dayEnd": {""}}
		var payload struct {
			State  *int            `json:"state"`
			Result json.RawMessage `json:"result"`
		}
		if err := read(spec.Endpoint+"?"+query.Encode(), indexURL, &payload); err != nil {
			return observation, err
		}
		if payload.State != nil && *payload.State != 0 {
			return observation, errSourceProbeInvalid
		}
		if sourceProbeJSONEmpty(payload.Result) {
			return observation, errSourceProbeEmpty
		}
		rows, err := sourceProbeFirstJSONRows(payload.Result, 3)
		if err != nil {
			return observation, err
		}
		for _, raw := range rows {
			var row struct {
				Code string `json:"code"`
				Date string `json:"date"`
				Red  string `json:"red"`
			}
			if sourceProbeDecode(raw, &row) != nil {
				return observation, errSourceProbeInvalid
			}
			numbers, err := sourceProbeNumbers(row.Red, ",")
			if err != nil {
				return observation, err
			}
			day := strings.Split(row.Date, "(")[0]
			drawAt, err := time.ParseInLocation("2006-01-02", day, sgSSCLocation)
			if err != nil {
				return observation, errSourceProbeInvalid
			}
			draws = append(draws, sourceDraw{Issue: row.Code, Numbers: numbers, DrawAt: drawAt.UTC()})
		}
		observation.message = "上游仅提供开奖日期，00:00表示日期精度，不是已核验的实际开奖时刻"
	case "official-pl3", "official-qxc":
		gameNo := "35"
		if id == "official-qxc" {
			gameNo = "04"
		}
		query := url.Values{"gameNo": {gameNo}, "provinceId": {"0"}, "pageSize": {"3"}, "isVerify": {"1"}, "pageNo": {"1"}}
		var payload struct {
			Success bool `json:"success"`
			Value   struct {
				List json.RawMessage `json:"list"`
			} `json:"value"`
		}
		if err := read(spec.Endpoint+"?"+query.Encode(), "https://www.lottery.gov.cn/", &payload); err != nil {
			return observation, err
		}
		if !payload.Success {
			return observation, errSourceProbeInvalid
		}
		if sourceProbeJSONEmpty(payload.Value.List) {
			return observation, errSourceProbeEmpty
		}
		rows, err := sourceProbeFirstJSONRows(payload.Value.List, 3)
		if err != nil {
			return observation, err
		}
		for _, raw := range rows {
			var row struct {
				Issue   string `json:"lotteryDrawNum"`
				Date    string `json:"lotteryDrawTime"`
				Numbers string `json:"lotteryDrawResult"`
			}
			if sourceProbeDecode(raw, &row) != nil {
				return observation, errSourceProbeInvalid
			}
			numbers, err := sourceProbeNumbers(strings.Join(strings.Fields(row.Numbers), ","), ",")
			if err != nil {
				return observation, err
			}
			drawAt := parse168DrawTime(row.Date)
			if drawAt.IsZero() {
				return observation, errSourceProbeInvalid
			}
			if len(row.Date) == 10 {
				observation.message = "上游仅提供开奖日期，00:00表示日期精度，不是实际开奖时刻"
			}
			draws = append(draws, sourceDraw{Issue: row.Issue, Numbers: numbers, DrawAt: drawAt})
		}
	case "official-tw-bingo":
		query := url.Values{"openDate": {now.In(sgSSCLocation).Format("2006-01-02")}, "pageNum": {"1"}, "pageSize": {"3"}}
		var payload struct {
			RTCode  *int `json:"rtCode"`
			Content struct {
				Items []struct {
					Issue   int64    `json:"drawTerm"`
					Numbers []string `json:"openShowOrder"`
				} `json:"bingoQueryResult"`
			} `json:"content"`
		}
		if err := read(spec.Endpoint+"?"+query.Encode(), "https://www.taiwanlottery.com/", &payload); err != nil {
			return observation, err
		}
		if payload.RTCode == nil || *payload.RTCode != 0 {
			return observation, errSourceProbeInvalid
		}
		if len(payload.Content.Items) == 0 {
			return observation, errSourceProbeEmpty
		}
		row := payload.Content.Items[0]
		numbers, err := sourceProbeNumbers(strings.Join(row.Numbers, ","), ",")
		if err != nil {
			return observation, err
		}
		if validate168BingoNumbers(numbers) != nil {
			return observation, errSourceProbeInvalid
		}
		observation.latest = sourceDraw{Issue: strconv.FormatInt(row.Issue, 10), Numbers: numbers}
		observation.historyCount = min(len(payload.Content.Items), sourceProbeHistoryLimit)
		return observation, errors.New("已读取原始号码，但接口未提供可核验的逐期开奖时刻；不会用当前时间替代")
	default:
		var payload struct {
			RTCode  *int                       `json:"rtCode"`
			Content map[string]json.RawMessage `json:"content"`
		}
		if err := read(spec.Endpoint, "https://www.taiwanlottery.com/", &payload); err != nil {
			return observation, err
		}
		if payload.RTCode == nil || *payload.RTCode != 0 {
			return observation, errSourceProbeInvalid
		}
		field := map[string]string{"official-tw-super-lotto": "superLotto638Result", "official-tw-daily539": "daily539Result", "official-tw-lotto649": "lotto649Result"}[id]
		raw := payload.Content[field]
		if sourceProbeJSONEmpty(raw) {
			return observation, errSourceProbeEmpty
		}
		var row taiwanDraw
		if sourceProbeDecode(raw, &row) != nil {
			return observation, errSourceProbeInvalid
		}
		drawAt := parse168DrawTime(row.LotteryDate)
		if drawAt.IsZero() {
			parsed, err := time.ParseInLocation("2006-01-02T15:04:05", row.LotteryDate, sgSSCLocation)
			if err != nil {
				return observation, errSourceProbeInvalid
			}
			drawAt = parsed.UTC()
		}
		if len(row.LotteryDate) == 10 {
			observation.message = "上游仅提供开奖日期，00:00表示日期精度，不是实际开奖时刻"
		}
		draws = []sourceDraw{{Issue: strconv.FormatInt(row.Period, 10), Numbers: row.Numbers, DrawAt: drawAt}}
	}
	if len(draws) == 0 {
		return observation, errSourceProbeEmpty
	}
	recent, err := sourceProbeRecent(draws, spec)
	if err != nil {
		return observation, err
	}
	observation.latest = recent[0]
	observation.historyCount = len(recent)
	return observation, nil
}
