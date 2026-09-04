package services

import (
	"context"
	"crypto/aes"
	"crypto/md5" // The public upstream protocol requires MD5; not used for authentication or password storage.
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"backend/data/models/lottery"
)

// Public anonymous browser protocol from the two versioned scripts cited in
// docs/开奖源核对/163接口与全彩种目录-2026-09-04.md. This is not an account
// credential. Never include generated signatures or signed URLs in diagnostics.
func source163PublicSigningValue() (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString("2mFA9KIkHOYHzoGPWnLRjE9lcGRDbLnDYomp641dGpI=")
	if err != nil {
		return "", errors.New("公开请求协议配置无效")
	}
	block, err := aes.NewCipher([]byte("1QWERdfbIU12Q2vb"))
	if err != nil || len(ciphertext)%aes.BlockSize != 0 {
		return "", errors.New("公开请求协议配置无效")
	}
	plain := make([]byte, len(ciphertext))
	for offset := 0; offset < len(ciphertext); offset += aes.BlockSize {
		block.Decrypt(plain[offset:offset+aes.BlockSize], ciphertext[offset:offset+aes.BlockSize])
	}
	padding := int(plain[len(plain)-1])
	if padding < 1 || padding > aes.BlockSize {
		return "", errors.New("公开请求协议配置无效")
	}
	for _, value := range plain[len(plain)-padding:] {
		if int(value) != padding {
			return "", errors.New("公开请求协议配置无效")
		}
	}
	return string(plain[:len(plain)-padding]), nil
}

func source163SignedURL(path string, gameID, count int, now time.Time, entropy io.Reader) (string, error) {
	if path != source163LatestPath && path != source163HistoryPath {
		return "", errors.New("未知只读接口")
	}
	key, err := source163PublicSigningValue()
	if err != nil {
		return "", err
	}
	var random [10]byte
	if _, err = io.ReadFull(entropy, random[:]); err != nil {
		return "", errors.New("生成匿名请求参数失败")
	}
	integer := (uint32(random[0])<<16 | uint32(random[1])<<8 | uint32(random[2])) % 1_000_000
	r1 := strconv.FormatUint(uint64(integer), 10)
	alphabet := "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	r2 := string([]byte{alphabet[int(random[3])%len(alphabet)], alphabet[int(random[4])%len(alphabet)], alphabet[int(random[5])%len(alphabet)], '0' + random[6]%10, 'a' + random[7]%26, 'A' + random[8]%26})
	timestamp := strconv.FormatInt(now.Unix(), 10)
	sign := func(p, r string) string {
		prefix := timestamp + "-" + r + "-0"
		hash := md5.Sum([]byte(p + "-" + prefix + "-" + key))
		return prefix + "-" + hex.EncodeToString(hash[:])
	}
	query := url.Values{"iGameId": {strconv.Itoa(gameID)}, "sign": {sign(strings.ToLower(path), r1)}, "sign2": {sign(path, r2)}}
	if path == source163HistoryPath {
		query.Set("count", strconv.Itoa(count))
	}
	return source163Base + path + "?" + query.Encode(), nil
}

type source163Envelope struct {
	Success *bool           `json:"success"`
	Result  json.RawMessage `json:"result"`
}
type source163Row struct {
	GameID  json.Number `json:"igameid"`
	Issue   any         `json:"sgameperiod"`
	Numbers string      `json:"sopennum"`
	Time    string      `json:"dopentime"`
}

func probe163Source(ctx context.Context, spec sourceDiagnosticSpec, now time.Time, entropy io.Reader, request *sourceProbeHTTP) (sourceProbeObservation, error) {
	observation := sourceProbeObservation{}
	latestURL, err := source163SignedURL(source163LatestPath, spec.UpstreamGameID, 0, now, entropy)
	if err != nil {
		return observation, err
	}
	body, err := request.request(ctx, latestURL, source163Base+"/")
	if err != nil {
		return observation, err
	}
	var payload source163Envelope
	if sourceProbeDecode(body, &payload) != nil || payload.Success == nil || !*payload.Success {
		return observation, errSourceProbeInvalid
	}
	if sourceProbeJSONEmpty(payload.Result) {
		return observation, errSourceProbeEmpty
	}
	var row source163Row
	if sourceProbeDecode(payload.Result, &row) != nil {
		return observation, errSourceProbeInvalid
	}
	if (row.Issue == nil || row.Issue == "") && strings.TrimSpace(row.Numbers) == "" && strings.TrimSpace(row.Time) == "" {
		return observation, errSourceProbeEmpty
	}
	observation.latest, err = source163Draw(row, spec)
	if err != nil {
		return sourceProbeObservation{}, err
	}
	historyURL, err := source163SignedURL(source163HistoryPath, spec.UpstreamGameID, sourceProbeHistoryLimit, now, entropy)
	if err != nil {
		return observation, err
	}
	body, err = request.request(ctx, historyURL, source163Base+"/")
	if err != nil {
		return observation, err
	}
	payload = source163Envelope{}
	if sourceProbeDecode(body, &payload) != nil || payload.Success == nil || !*payload.Success {
		return observation, errSourceProbeInvalid
	}
	if sourceProbeJSONEmpty(payload.Result) {
		return observation, errors.New("最新开奖存在，但历史样本为空")
	}
	// Upstream has returned 500 rows even for count=3. Decode only the first
	// three records; the HTTP reader independently limits the complete body.
	rows, err := sourceProbeFirstJSONRows(payload.Result, sourceProbeHistoryLimit)
	if err != nil {
		return observation, err
	}
	matched := false
	seen := map[string]bool{}
	for _, raw := range rows {
		var historyRow source163Row
		if sourceProbeDecode(raw, &historyRow) != nil {
			return observation, errSourceProbeInvalid
		}
		draw, parseErr := source163Draw(historyRow, spec)
		if parseErr != nil || seen[draw.Issue] {
			return observation, errSourceProbeInvalid
		}
		seen[draw.Issue] = true
		if draw.Issue == observation.latest.Issue {
			if !sameSourceProbeResult(draw, observation.latest) {
				return observation, errors.New("同一期最新与历史号码或时间不一致")
			}
			matched = true
		}
		observation.historyCount++
	}
	if !matched {
		return observation, errors.New("有限历史样本未能核对当前期，请稍后重试")
	}
	return observation, nil
}

func source163MirrorBindingForUpstream(upstreamGameID int) (source163MirrorBinding, bool) {
	if binding, ok := source163MarkSixBindingForUpstream(upstreamGameID); ok {
		return binding.mirrorBinding(), true
	}
	for _, binding := range source163MirrorBindings {
		if binding.UpstreamGameID == upstreamGameID {
			return binding, true
		}
	}
	for _, binding := range source163PC28Bindings {
		if binding.UpstreamGameID == upstreamGameID {
			return binding, true
		}
	}
	return source163MirrorBinding{}, false
}

func sourceProbeObservationFromDraws(draws []sourceDraw, message string) (sourceProbeObservation, error) {
	if len(draws) == 0 {
		return sourceProbeObservation{}, errSourceProbeEmpty
	}
	latest := draws[0]
	for _, draw := range draws[1:] {
		if draw.DrawAt.After(latest.DrawAt) {
			latest = draw
		}
	}
	return sourceProbeObservation{latest: latest, historyCount: len(draws), message: message}, nil
}

// Current 163 mirror sources use the exact production reader and validators.
// The HTTP transport remains the diagnostic transport, so this is still
// read-only and has tighter body/time bounds than the importer.
func probe163ProductionMirrorSource(ctx context.Context, spec sourceDiagnosticSpec, now time.Time, entropy io.Reader, request *sourceProbeHTTP) (sourceProbeObservation, error) {
	binding, ok := source163MirrorBindingForUpstream(spec.UpstreamGameID)
	if !ok {
		return sourceProbeObservation{}, errors.New("163生产母源绑定不存在")
	}
	fetch := func(ctx context.Context, endpoint string) ([]byte, error) {
		return request.request(ctx, endpoint, source163MirrorURL)
	}
	var draws []sourceDraw
	var err error
	if binding.UpstreamGameID == source163PC28UpstreamGameID {
		// Exercise the same remote-only parser and cadence contract as ID57
		// production, but deliberately do not use production's verified local
		// history fallback. The diagnostics page must expose a temporarily short
		// upstream history even when the importer can safely bridge that window.
		draws, err = fetch163PC28DrawsWithRequest(ctx, binding, func() time.Time { return now }, entropy, fetch)
	} else if markSixBinding, ok := source163MarkSixBindingForUpstream(binding.UpstreamGameID); ok {
		draws, err = fetch163MarkSixDrawsWithRequest(ctx, markSixBinding, func() time.Time { return now }, entropy, fetch)
	} else {
		draws, err = fetch163MirrorDrawsWithRequest(ctx, binding, func() time.Time { return now }, entropy, fetch)
	}
	if err != nil {
		return sourceProbeObservation{}, err
	}
	if markSixBinding, ok := source163MarkSixBindingForUpstream(binding.UpstreamGameID); ok {
		if err := validate163MarkSixDrawBatch(markSixBinding, draws); err != nil {
			return sourceProbeObservation{}, err
		}
	} else {
		game := lottery.Game{ID: binding.GameID, Name: diagnosticGameName(binding.GameID)}
		if err := validate163MirrorDrawBatch(game, binding, draws); err != nil {
			return sourceProbeObservation{}, err
		}
	}
	if binding.UpstreamGameID == source163PC28UpstreamGameID {
		if err := validate163PC28Cadence(draws); err != nil {
			return sourceProbeObservation{}, err
		}
		for _, variant := range source163PC28Bindings {
			variantGame := lottery.Game{ID: variant.GameID, Name: diagnosticGameName(variant.GameID)}
			if err := validate163MirrorDrawBatch(variantGame, variant, draws); err != nil {
				return sourceProbeObservation{}, fmt.Errorf("163加拿大28生产验收 %s 未通过: %w", variant.GameID, err)
			}
		}
	}
	return sourceProbeObservationFromDraws(draws, "已通过当前生产母源的有限历史、连续周期与新鲜度门禁；只读检测未导入数据")
}

func fetch163BingoProbeAuthority(ctx context.Context, now time.Time, entropy io.Reader, request *sourceProbeHTTP) ([]sourceDraw, error) {
	return fetch163BingoAuthorityWithRequest(ctx, func() time.Time { return now }, entropy, func(ctx context.Context, endpoint string) ([]byte, error) {
		return request.request(ctx, endpoint, bingo163SourceURL)
	})
}

func probe163BingoAuthoritySource(ctx context.Context, now time.Time, entropy io.Reader, request *sourceProbeHTTP) (sourceProbeObservation, error) {
	draws, err := fetch163BingoProbeAuthority(ctx, now, entropy, request)
	if err != nil {
		return sourceProbeObservation{}, err
	}
	if err := validate163BingoProbeDerivatives(draws, false); err != nil {
		return sourceProbeObservation{}, err
	}
	return sourceProbeObservationFromDraws(draws, "已通过当前163台湾宾果生产母源的升序集合、连续期号、开奖时点与新鲜度门禁；只读检测未导入数据")
}

// A source-level success must also satisfy every deterministic conversion and
// draw validator used by the corresponding production import. Database binding,
// existing-row conflict and settlement checks intentionally remain outside this
// read-only probe and are reported separately by runtime synchronization state.
func validate163BingoProbeDerivatives(draws []sourceDraw, ordered bool) error {
	for _, binding := range bingo163Bindings {
		if binding.RequiresOrderedSource != ordered {
			continue
		}
		converted, err := transform163BingoDraws(binding, draws)
		if err != nil {
			return fmt.Errorf("163台湾宾果生产转换 %s 未通过: %w", binding.GameID, err)
		}
		game := lottery.Game{ID: binding.GameID, Name: diagnosticGameName(binding.GameID)}
		if err := validate163BingoDerivedBatch(game, binding, converted); err != nil {
			return fmt.Errorf("163台湾宾果生产验收 %s 未通过: %w", binding.GameID, err)
		}
	}
	return nil
}

func sourceProbeJSONEmpty(raw []byte) bool {
	value := strings.TrimSpace(string(raw))
	return value == "" || value == "null" || value == `""` || value == "[]" || value == "{}"
}

func sourceProbeFirstJSONRows(raw []byte, limit int) ([]json.RawMessage, error) {
	if !json.Valid(raw) {
		return nil, errSourceProbeInvalid
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('[') {
		return nil, errSourceProbeInvalid
	}
	rows := make([]json.RawMessage, 0, limit)
	for decoder.More() && len(rows) < limit {
		var row json.RawMessage
		if decoder.Decode(&row) != nil {
			return nil, errSourceProbeInvalid
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func source163Draw(row source163Row, spec sourceDiagnosticSpec) (sourceDraw, error) {
	id, err := row.GameID.Int64()
	if err != nil || id != int64(spec.UpstreamGameID) {
		return sourceDraw{}, errSourceProbeInvalid
	}
	numbers, err := sourceProbeNumbers(row.Numbers, "|")
	if err != nil {
		return sourceDraw{}, err
	}
	drawAt, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(row.Time), sgSSCLocation)
	if err != nil {
		return sourceDraw{}, errSourceProbeInvalid
	}
	draw := sourceDraw{Issue: api168IssueText(row.Issue), Numbers: numbers, DrawAt: drawAt.UTC()}
	return draw, validateSourceProbeDraw(draw, spec)
}

// The upstream directory includes 3, 5, 7, 10 and 20-ball products. Do not
// reuse the SG-only equality helper, whose contract deliberately requires 5.
func sameSourceProbeResult(first, second sourceDraw) bool {
	if first.Issue != second.Issue || !first.DrawAt.Equal(second.DrawAt) || len(first.Numbers) == 0 || len(first.Numbers) != len(second.Numbers) {
		return false
	}
	for index, number := range first.Numbers {
		if second.Numbers[index] != number {
			return false
		}
	}
	return true
}

func sourceProbeNumbers(text, separator string) ([]int, error) {
	parts := strings.Split(strings.TrimSpace(text), separator)
	if len(parts) > 21 {
		return nil, errSourceProbeInvalid
	}
	numbers := make([]int, len(parts))
	for index, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || len(part) > 2 || strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return nil, errSourceProbeInvalid
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return nil, errSourceProbeInvalid
		}
		numbers[index] = value
	}
	return numbers, nil
}

func probe168Source(ctx context.Context, spec sourceDiagnosticSpec, now time.Time, request *sourceProbeHTTP) (sourceProbeObservation, error) {
	observation := sourceProbeObservation{}
	path, historyPath := api168Paths(spec.binding.Series)
	read := func(endpoint string) ([]sourceDraw, error) {
		body, err := request.request(ctx, endpoint, api168Referer)
		if err != nil {
			return nil, err
		}
		var payload struct {
			ErrorCode *int `json:"errorCode"`
			Result    *struct {
				BusinessCode *int            `json:"businessCode"`
				Data         json.RawMessage `json:"data"`
			} `json:"result"`
		}
		if sourceProbeDecode(body, &payload) != nil || payload.ErrorCode == nil || *payload.ErrorCode != 0 || payload.Result == nil || payload.Result.BusinessCode != nil && *payload.Result.BusinessCode != 0 {
			return nil, errSourceProbeInvalid
		}
		if sourceProbeJSONEmpty(payload.Result.Data) {
			return nil, errSourceProbeEmpty
		}
		var rawRows []json.RawMessage
		if strings.HasPrefix(strings.TrimSpace(string(payload.Result.Data)), "{") {
			rawRows = []json.RawMessage{payload.Result.Data}
		} else {
			rawRows, err = sourceProbeFirstJSONRows(payload.Result.Data, sourceProbeHistoryLimit)
			if err != nil {
				return nil, err
			}
		}
		draws := make([]sourceDraw, 0, len(rawRows))
		for _, raw := range rawRows {
			var row struct {
				api168Row
				LotCode json.Number `json:"lotCode"`
			}
			if sourceProbeDecode(raw, &row) != nil {
				return nil, errSourceProbeInvalid
			}
			if row.LotCode != "" && string(row.LotCode) != spec.binding.LotCode {
				return nil, errSourceProbeInvalid
			}
			numbers, numberErr := sourceProbeNumbers(row.Code, ",")
			draw := sourceDraw{Issue: row.IssueText(), DrawAt: parse168DrawTime(row.Time), Numbers: numbers, NextIssue: api168IssueText(row.NextIssue), NextDrawAt: parse168DrawTime(row.NextTime)}
			if is168BingoSource(spec.binding.Series, spec.binding.LotCode) {
				draw.Numbers, draw.BingoSourceTail, draw.HasBingoSourceTail, numberErr = parse168BingoNumbersWithTail(row.Code)
			}
			if numberErr != nil || validateSourceProbeDraw(draw, spec) != nil {
				return nil, errSourceProbeInvalid
			}
			draws = append(draws, draw)
		}
		return sourceProbeRecent(draws, spec)
	}
	query := url.Values{"lotCode": {spec.binding.LotCode}}
	latest, err := read(api168Base + path + "?" + query.Encode())
	if err != nil {
		return observation, err
	}
	if len(latest) == 0 {
		return observation, errSourceProbeEmpty
	}
	observation.latest = latest[0]
	if spec.binding.Series != api168LHC {
		query.Set("date", observation.latest.DrawAt.In(sgSSCLocation).Format("2006-01-02"))
	}
	history, err := read(api168Base + historyPath + "?" + query.Encode())
	if err != nil {
		return observation, err
	}
	observation.historyCount = len(history)
	matched := false
	for _, draw := range history {
		if draw.Issue == observation.latest.Issue {
			if !sameSourceProbeResult(draw, observation.latest) {
				return observation, errors.New("168最新与历史同期开奖不一致")
			}
			matched = true
		}
	}
	if !matched {
		return observation, errors.New("168有限历史样本未能核对当前期")
	}
	return observation, nil
}

func probeSGSSCSource(ctx context.Context, now time.Time, entropy io.Reader, request *sourceProbeHTTP) (sourceProbeObservation, error) {
	validation := sgSSCValidationStation()
	draws, err := fetchSGSSCVerifiedWithRequests(ctx, func() time.Time { return now }, entropy,
		func(ctx context.Context, endpoint string) ([]byte, error) {
			return request.request(ctx, endpoint, source163MirrorURL)
		},
		func(ctx context.Context, endpoint string) ([]byte, error) {
			return request.request(ctx, endpoint, validation.referer)
		},
	)
	if err != nil {
		return sourceProbeObservation{}, err
	}
	return sourceProbeObservationFromDraws(draws, "已通过当前生产写入使用的163 ID64母源、115校验源、最近24期窗口及新鲜度门禁；只读检测未导入数据")
}

func probeBingoOrderedSource(ctx context.Context, now time.Time, request *sourceProbeHTTP) (sourceProbeObservation, error) {
	spec, _ := sourceDiagnosticSpecForKey("168:10047")
	observation, err := probe168Source(ctx, spec, now, request)
	if err != nil {
		return observation, err
	}
	body, err := request.request(ctx, bingoOrderedHistoryURL+"?limit=3", bingoVerifiedSourceURL)
	if err != nil {
		return observation, err
	}
	rows, err := sourceProbeFirstJSONRows(body, sourceProbeHistoryLimit)
	if err != nil {
		return observation, err
	}
	// Reuse the strict raw-order parser on only the local sample, not on an
	// upstream-controlled unbounded history count.
	limited, err := json.Marshal(rows)
	if err != nil {
		return observation, errSourceProbeInvalid
	}
	ordered, err := parseBingoOrderedHistory(limited)
	if err != nil {
		return observation, errSourceProbeInvalid
	}
	verified, err := crossValidate168BingoOrder([]sourceDraw{observation.latest}, ordered)
	if err != nil {
		return observation, errors.New("台湾宾果两源期号、20球集合或末球不一致")
	}
	observation.latest = verified[0]
	observation.historyCount = len(ordered)
	return observation, nil
}

func probe163BingoOrderedSource(ctx context.Context, now time.Time, entropy io.Reader, request *sourceProbeHTTP) (sourceProbeObservation, error) {
	verified, err := fetch163BingoVerifiedAuthorityWithRequest(ctx, func() time.Time { return now }, entropy, func(ctx context.Context, endpoint string) ([]byte, error) {
		return request.request(ctx, endpoint, bingo163SourceURL)
	})
	if err != nil {
		return sourceProbeObservation{}, err
	}
	if err := validate163BingoProbeDerivatives(verified, true); err != nil {
		return sourceProbeObservation{}, err
	}
	return sourceProbeObservationFromDraws(verified, "已通过当前生产写入使用的163 ID185原始球序与ID135同期集合完整交叉门禁；只读检测未导入数据")
}
