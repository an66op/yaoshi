package services

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"backend/data/models/lottery"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	bingo163SetUpstreamGameID     = 135
	bingo163OrderedUpstreamGameID = 185
	bingo163UpstreamGameID        = bingo163SetUpstreamGameID // compatibility for sorted-set probes
	bingo163HistoryLimit          = 64
	bingo163TotalTimeout          = 12 * time.Second

	// The old constants remain immutable historical provenance. New writes use
	// the verified ID185+ID135 revision below and never claim that old 168/jyb
	// or sorted-only rows came from the new ordered mother source.
	bingo163SetSourceName            = "163开奖网 · 台湾宾果集合"
	bingo163LegacyOrderedSourceName  = "台湾宾果双源校验（163集合＋jyb.one顺序）"
	bingo163OrderedSourceName        = "163开奖网 · 台湾宾果双源校验"
	bingo163SourceURL                = source163Base + "/"
	bingo163SetSourceRevision        = "tw-bingo-163135-sorted-v1"
	bingo163OrderSourceRevision      = "tw-bingo-163135-jyb-order-v1"
	bingo163OrderedRawSourceRevision = "tw-bingo-163185-ordered-raw-v1"
	bingo163VerifiedSourceRevision   = "tw-bingo-163185-135-order-v2"

	bingo163LegacySSC2ConversionVersion    = "bingo-ssc-2-sorted-offset1-v1"
	bingo163LegacySSC3ConversionVersion    = "bingo-ssc-3-sorted-offset2-v1"
	bingo163LegacySSC4ConversionVersion    = "bingo-ssc-4-sorted-offset3-v1"
	bingo163LegacyRacingBConversionVersion = "bingo-racing-b-sorted-offset10-v1"

	bingo163SSC2ConversionVersion    = "bingo-ssc-2-order-block2-v2"
	bingo163SSC3ConversionVersion    = "bingo-ssc-3-order-block3-v2"
	bingo163SSC4ConversionVersion    = "bingo-ssc-4-order-block4-v2"
	bingo163RacingBConversionVersion = "bingo-racing-b-rank-last10-v2"

	bingo163PublishGrace       = 2 * time.Minute
	bingo163DrawInterval       = 5 * time.Minute
	bingo163SessionStartMinute = 7*60 + 5
	bingo163SessionEndMinute   = 23*60 + 55

	bingo163PendingMessage = "等待163台湾宾果母源核验"
)

type bingo163Binding struct {
	GameID                string
	Transform             func([]int) []int
	RequiresOrderedSource bool
	SourceRevision        string
	ConversionVersion     string
}

var bingo163Bindings = []bingo163Binding{
	{GameID: "bingo-ssc-1", Transform: bingoSSCNumbers(0), RequiresOrderedSource: true, SourceRevision: bingo163VerifiedSourceRevision, ConversionVersion: bingoSSC1ConversionVersion},
	{GameID: "bingo-ssc-2", Transform: bingoSSCNumbers(5), RequiresOrderedSource: true, SourceRevision: bingo163VerifiedSourceRevision, ConversionVersion: bingo163SSC2ConversionVersion},
	{GameID: "bingo-ssc-3", Transform: bingoSSCNumbers(10), RequiresOrderedSource: true, SourceRevision: bingo163VerifiedSourceRevision, ConversionVersion: bingo163SSC3ConversionVersion},
	{GameID: "bingo-ssc-4", Transform: bingoSSCNumbers(15), RequiresOrderedSource: true, SourceRevision: bingo163VerifiedSourceRevision, ConversionVersion: bingo163SSC4ConversionVersion},
	{GameID: "bingo-racing-a", Transform: bingoRacingARankV1Numbers, RequiresOrderedSource: true, SourceRevision: bingo163VerifiedSourceRevision, ConversionVersion: bingoRacingAConversionVersion},
	{GameID: "bingo-racing-b", Transform: bingo163RacingBRankV2Numbers, RequiresOrderedSource: true, SourceRevision: bingo163VerifiedSourceRevision, ConversionVersion: bingo163RacingBConversionVersion},
	{GameID: "bingo-mark-six", Transform: bingoMarkSixNumbers, RequiresOrderedSource: true, SourceRevision: bingo163VerifiedSourceRevision, ConversionVersion: bingoMarkSixConversionVersion},
}

// bingo163RacingBRankV2Numbers ranks only balls 11-20 and returns those ranks
// in their actual draw order. It mirrors Racing A's first-ten contract without
// using the former modulo/deduplication fallback.
func bingo163RacingBRankV2Numbers(raw []int) []int {
	if validate168BingoNumbers(raw) != nil {
		return nil
	}
	window := append([]int(nil), raw[10:20]...)
	sortedWindow := append([]int(nil), window...)
	sort.Ints(sortedWindow)
	ranks := make(map[int]int, len(sortedWindow))
	for index, number := range sortedWindow {
		ranks[number] = index + 1
	}
	for index, number := range window {
		window[index] = ranks[number]
	}
	return window
}

func bingo163BindingForGame(gameID string) (bingo163Binding, bool) {
	for _, binding := range bingo163Bindings {
		if binding.GameID == strings.TrimSpace(gameID) {
			return binding, true
		}
	}
	return bingo163Binding{}, false
}

func bingo163BindingSourceDefaults(binding bingo163Binding) (name, sourceURL, syncStatus, lastSyncError string) {
	return bingo163OrderedSourceName, bingo163SourceURL, "stale", bingo163PendingMessage
}

// bingo163SourceBound is intended to be checked again while holding the game
// row lock immediately before an import. It prevents an old provider request
// which was already in flight from writing after an operator changes sources.
func bingo163SourceBound(game *lottery.Game, binding bingo163Binding) bool {
	if game == nil || game.ID != binding.GameID || !strings.EqualFold(strings.TrimSpace(game.SourceKind), "external") {
		return false
	}
	name, sourceURL, _, _ := bingo163BindingSourceDefaults(binding)
	return strings.TrimSpace(game.SourceName) == name && strings.TrimSpace(game.SourceURL) == sourceURL
}

func bingo163LegacySourceBound(game lottery.Game, binding bingo163Binding) bool {
	if game.ID != binding.GameID || !strings.EqualFold(strings.TrimSpace(game.SourceKind), "external") {
		return false
	}
	// Migrate the immediately previous 163 cutover contract exactly.
	if strings.TrimSpace(game.SourceURL) == bingo163SourceURL &&
		(strings.TrimSpace(game.SourceName) == bingo163SetSourceName || strings.TrimSpace(game.SourceName) == bingo163LegacyOrderedSourceName) {
		return true
	}
	if bingo163LegacyRequiredOrder(binding.GameID) {
		return strings.TrimSpace(game.SourceName) == bingoVerifiedSourceName && strings.TrimSpace(game.SourceURL) == bingoVerifiedSourceURL
	}
	return strings.TrimSpace(game.SourceName) == "168开奖网" && strings.TrimSpace(game.SourceURL) == "https://kj138138.com/view/api/index.html"
}

func bingo163LegacyRequiredOrder(gameID string) bool {
	switch strings.TrimSpace(gameID) {
	case "bingo-ssc-1", "bingo-racing-a", "bingo-mark-six":
		return true
	default:
		return false
	}
}

// bingo163SourceRevisionUpdates migrates only the exact previous Bingo source
// binding. An operator-selected custom source is never overwritten merely
// because its game ID belongs to the Bingo family.
func bingo163SourceRevisionUpdates(game lottery.Game, binding bingo163Binding) (map[string]any, bool) {
	if bingo163SourceBound(&game, binding) {
		status := strings.ToLower(strings.TrimSpace(game.SyncStatus))
		if status == "ok" || status == "error" || status == "paused" || status == "stale" && strings.TrimSpace(game.LastSyncError) != "" {
			return nil, false
		}
		return map[string]any{"sync_status": "stale", "last_sync_error": bingo163PendingMessage}, true
	}
	if !bingo163LegacySourceBound(game, binding) {
		return nil, false
	}
	name, sourceURL, status, message := bingo163BindingSourceDefaults(binding)
	return map[string]any{
		"source_kind": "external", "source_name": name, "source_url": sourceURL,
		"sync_status": status, "last_sync_error": message, "last_sync_at": nil,
	}, true
}

// Ensure163BingoSources is intentionally metadata-only. It never rewrites a
// draw, issue, bet or archived bet and therefore never claims old 168 history
// as if it had been supplied by 163.
func Ensure163BingoSources(db *gorm.DB) error {
	if db == nil {
		return errors.New("163台湾宾果来源版本数据库不可用")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, binding := range bingo163Bindings {
			var game lottery.Game
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&game, "id = ?", binding.GameID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			updates, required := bingo163SourceRevisionUpdates(game, binding)
			if !required {
				continue
			}
			if result := tx.Model(&lottery.Game{}).Where("id = ?", binding.GameID).Updates(updates); result.Error != nil || result.RowsAffected != 1 {
				if result.Error != nil {
					return result.Error
				}
				return fmt.Errorf("163台湾宾果游戏 %s 来源迁移未生效", binding.GameID)
			}
		}
		return nil
	})
}

type bingo163Request func(context.Context, string) ([]byte, error)

func fetch163BingoAuthority(ctx context.Context) ([]sourceDraw, error) {
	return fetch163BingoAuthorityWithRequest(ctx, time.Now, rand.Reader, func(ctx context.Context, endpoint string) ([]byte, error) {
		return request163Mirror(ctx, endpoint)
	})
}

// fetch163BingoAuthority reads the fixed sorted-set product 135. It remains a
// reusable evidence reader for diagnostics; production writes additionally
// require the ordered product 185 and exact cross-validation below.
func fetch163BingoAuthorityWithRequest(ctx context.Context, now func() time.Time, entropy io.Reader, request bingo163Request) ([]sourceDraw, error) {
	return fetch163BingoProductAuthorityWithRequest(ctx, now, entropy, request, bingo163SetUpstreamGameID, false, bingo163SetSourceRevision)
}

func fetch163BingoOrderedAuthorityWithRequest(ctx context.Context, now func() time.Time, entropy io.Reader, request bingo163Request) ([]sourceDraw, error) {
	return fetch163BingoProductAuthorityWithRequest(ctx, now, entropy, request, bingo163OrderedUpstreamGameID, true, bingo163OrderedRawSourceRevision)
}

func fetch163BingoProductAuthorityWithRequest(
	ctx context.Context,
	now func() time.Time,
	entropy io.Reader,
	request bingo163Request,
	upstreamGameID int,
	ordered bool,
	revision string,
) ([]sourceDraw, error) {
	if ctx == nil || now == nil || entropy == nil || request == nil {
		return nil, errors.New("163台湾宾果读取依赖无效")
	}
	if upstreamGameID != bingo163SetUpstreamGameID && upstreamGameID != bingo163OrderedUpstreamGameID || strings.TrimSpace(revision) == "" {
		return nil, errors.New("163台湾宾果彩种绑定无效")
	}
	productLabel := fmt.Sprintf("163台湾宾果ID%d", upstreamGameID)
	requestCtx, cancel := context.WithTimeout(ctx, bingo163TotalTimeout)
	defer cancel()
	checkedAt := now()
	if checkedAt.IsZero() {
		return nil, errors.New("163台湾宾果校验时间无效")
	}

	latestURL, err := source163SignedURL(source163LatestPath, upstreamGameID, 0, checkedAt, entropy)
	if err != nil {
		return nil, err
	}
	latestBody, err := request(requestCtx, latestURL)
	if err != nil {
		return nil, fmt.Errorf("%s当前开奖读取失败: %w", productLabel, err)
	}
	latestRows, err := decode163BingoProductRows(latestBody, false, upstreamGameID, ordered, revision)
	if err != nil || len(latestRows) != 1 {
		return nil, errors.Join(fmt.Errorf("%s当前开奖无效", productLabel), err)
	}

	historyURL, err := source163SignedURL(source163HistoryPath, upstreamGameID, bingo163HistoryLimit, checkedAt, entropy)
	if err != nil {
		return nil, err
	}
	historyBody, err := request(requestCtx, historyURL)
	if err != nil {
		return nil, fmt.Errorf("%s历史读取失败: %w", productLabel, err)
	}
	history, err := decode163BingoProductRows(historyBody, true, upstreamGameID, ordered, revision)
	if err != nil {
		return nil, fmt.Errorf("%s有限历史无效: %w", productLabel, err)
	}
	if err := validate163BingoHistoryClass(latestRows[0], history, ordered, revision); err != nil {
		return nil, err
	}
	if err := validate163BingoFreshnessClass(latestRows[0], checkedAt, ordered, revision); err != nil {
		return nil, err
	}

	nextIssueNumber, nextDrawAt, err := bingo163NextSchedule(latestRows[0].Issue, latestRows[0].DrawAt)
	if err != nil {
		return nil, err
	}
	draws := mergeSourceDraws(latestRows, history)
	for index := range draws {
		draws[index].SourceRevision = revision
		if draws[index].Issue == latestRows[0].Issue {
			draws[index].NextIssue = nextIssueNumber
			draws[index].NextDrawAt = nextDrawAt
		}
	}
	return draws, nil
}

func decode163BingoRows(body []byte, history bool) ([]sourceDraw, error) {
	return decode163BingoProductRows(body, history, bingo163SetUpstreamGameID, false, bingo163SetSourceRevision)
}

func decode163BingoProductRows(body []byte, history bool, upstreamGameID int, ordered bool, revision string) ([]sourceDraw, error) {
	var payload source163Envelope
	if sourceProbeDecode(body, &payload) != nil || payload.Success == nil || !*payload.Success || sourceProbeJSONEmpty(payload.Result) {
		return nil, errors.New("163台湾宾果响应结构无效")
	}
	rawRows := []json.RawMessage{payload.Result}
	if history {
		var err error
		rawRows, err = sourceProbeFirstJSONRows(payload.Result, bingo163HistoryLimit)
		if err != nil || len(rawRows) == 0 {
			return nil, errors.Join(errors.New("163台湾宾果历史结构无效"), err)
		}
	}

	result := make([]sourceDraw, 0, len(rawRows))
	seenIssues := make(map[string]bool, len(rawRows))
	for _, raw := range rawRows {
		var row source163Row
		if sourceProbeDecode(raw, &row) != nil {
			return nil, errors.New("163台湾宾果开奖记录结构无效")
		}
		gameID, err := row.GameID.Int64()
		if err != nil || gameID != int64(upstreamGameID) {
			return nil, errors.New("163台湾宾果彩种身份不一致")
		}
		issue := api168IssueText(row.Issue)
		if !sourceDiagnosticIssue.MatchString(issue) || seenIssues[issue] {
			return nil, fmt.Errorf("163台湾宾果期号 %q 无效或重复", issue)
		}
		seenIssues[issue] = true

		numbers, err := sourceProbeNumbers(row.Numbers, "|")
		if err != nil || validate168BingoNumbers(numbers) != nil || !ordered && !strictlyIncreasing(numbers) {
			if ordered {
				return nil, fmt.Errorf("163台湾宾果期号 %s 未返回有效的有序20球", issue)
			}
			return nil, fmt.Errorf("163台湾宾果期号 %s 未返回严格升序的20球集合", issue)
		}
		drawAt, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(row.Time), sgSSCLocation)
		if err != nil || !bingo163ScheduledDrawAt(drawAt) {
			return nil, fmt.Errorf("163台湾宾果期号 %s 开奖时间无效", issue)
		}
		result = append(result, sourceDraw{Issue: issue, Numbers: numbers, DrawAt: drawAt.UTC(), SourceRevision: revision})
	}
	return result, nil
}

func strictlyIncreasing(numbers []int) bool {
	for index := 1; index < len(numbers); index++ {
		if numbers[index] <= numbers[index-1] {
			return false
		}
	}
	return true
}

func bingo163ScheduledDrawAt(drawAt time.Time) bool {
	if drawAt.IsZero() {
		return false
	}
	local := drawAt.In(sgSSCLocation)
	minute := local.Hour()*60 + local.Minute()
	return local.Second() == 0 && local.Nanosecond() == 0 && minute >= bingo163SessionStartMinute && minute <= bingo163SessionEndMinute &&
		(minute-bingo163SessionStartMinute)%int(bingo163DrawInterval/time.Minute) == 0
}

func bingo163NextSchedule(issue string, drawAt time.Time) (string, time.Time, error) {
	issue = strings.TrimSpace(issue)
	if !sourceDiagnosticIssue.MatchString(issue) || !bingo163ScheduledDrawAt(drawAt) {
		return "", time.Time{}, errors.New("163台湾宾果期号或开奖时点无效")
	}
	current, err := strconv.ParseUint(issue, 10, 64)
	if err != nil || current == ^uint64(0) {
		return "", time.Time{}, errors.New("163台湾宾果期号无法递增")
	}
	nextIssueNumber := nextIssue(issue)
	local := drawAt.In(sgSSCLocation)
	minute := local.Hour()*60 + local.Minute()
	var next time.Time
	if minute < bingo163SessionEndMinute {
		next = local.Add(bingo163DrawInterval)
	} else {
		tomorrow := local.AddDate(0, 0, 1)
		next = time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 7, 5, 0, 0, sgSSCLocation)
	}
	return nextIssueNumber, next.UTC(), nil
}

func validate163BingoHistory(latest sourceDraw, history []sourceDraw) error {
	return validate163BingoHistoryClass(latest, history, false, bingo163SetSourceRevision)
}

func validate163BingoHistoryClass(latest sourceDraw, history []sourceDraw, orderedSource bool, revision string) error {
	if len(history) < 4 {
		return errors.New("163台湾宾果有限历史不足，无法验证开奖周期")
	}
	matched := false
	historyByTime := append([]sourceDraw(nil), history...)
	sort.Slice(historyByTime, func(i, j int) bool { return historyByTime[i].DrawAt.Before(historyByTime[j].DrawAt) })
	for index, draw := range historyByTime {
		if !bingo163DrawValidForClass(draw, orderedSource, revision) {
			return errors.New("163台湾宾果历史包含无效记录")
		}
		if draw.Issue == latest.Issue {
			if !sameBingo163AuthorityDraw(draw, latest) {
				return errors.New("163台湾宾果同一期当前与历史号码或时间不一致")
			}
			matched = true
		}
		if index == 0 {
			continue
		}
		wantIssue, wantAt, err := bingo163NextSchedule(historyByTime[index-1].Issue, historyByTime[index-1].DrawAt)
		if err != nil || draw.Issue != wantIssue || !draw.DrawAt.Equal(wantAt) {
			return fmt.Errorf("163台湾宾果历史在期号 %s 至 %s 之间不连续", historyByTime[index-1].Issue, draw.Issue)
		}
	}
	if !matched {
		return errors.New("163台湾宾果有限历史未包含当前期")
	}
	if historyByTime[len(historyByTime)-1].Issue != latest.Issue {
		return errors.New("163台湾宾果当前接口不是历史中的最新一期")
	}
	return nil
}

func bingo163AuthorityDrawValid(draw sourceDraw) bool {
	return bingo163DrawValidForClass(draw, false, bingo163SetSourceRevision)
}

func bingo163DrawValidForClass(draw sourceDraw, orderedSource bool, revision string) bool {
	return sourceDiagnosticIssue.MatchString(strings.TrimSpace(draw.Issue)) && bingo163ScheduledDrawAt(draw.DrawAt) &&
		validate168BingoNumbers(draw.Numbers) == nil && (orderedSource || strictlyIncreasing(draw.Numbers)) &&
		draw.SourceRevision == revision
}

func sameBingo163AuthorityDraw(first, second sourceDraw) bool {
	return first.Issue == second.Issue && first.DrawAt.Equal(second.DrawAt) && sameIntSequence(first.Numbers, second.Numbers)
}

func validate163BingoFreshness(latest sourceDraw, now time.Time) error {
	return validate163BingoFreshnessClass(latest, now, false, bingo163SetSourceRevision)
}

func validate163BingoFreshnessClass(latest sourceDraw, now time.Time, orderedSource bool, revision string) error {
	if !bingo163DrawValidForClass(latest, orderedSource, revision) || now.IsZero() {
		return errors.New("163台湾宾果当前开奖或校验时间无效")
	}
	_, nextDrawAt, err := bingo163NextSchedule(latest.Issue, latest.DrawAt)
	if err != nil {
		return err
	}
	now = now.UTC()
	if latest.DrawAt.After(now.Add(bingo163PublishGrace)) {
		return errors.New("163台湾宾果当前开奖时间在未来")
	}
	if now.After(nextDrawAt.Add(bingo163PublishGrace)) {
		return errors.New("163台湾宾果当前开奖已过期")
	}
	return nil
}

var err163BingoOrderMismatch = errors.New("163台湾宾果双源顺序校验失败")

// crossValidate163BingoOrder requires the complete validated ID135 and ID185
// batches to have the exact same issues, canonical timestamps and 20-ball sets.
// Only then is ID185's sequence stamped as current production provenance.
func crossValidate163BingoOrder(authoritative, ordered []sourceDraw) ([]sourceDraw, error) {
	if len(authoritative) == 0 || len(ordered) == 0 {
		return nil, fmt.Errorf("%w: 缺少可交叉校验的开奖记录", err163BingoOrderMismatch)
	}
	if len(authoritative) != len(ordered) {
		return nil, fmt.Errorf("%w: ID135与ID185历史期数不一致", err163BingoOrderMismatch)
	}
	orderedByIssue := make(map[string]sourceDraw, len(ordered))
	for _, row := range ordered {
		issue := strings.TrimSpace(row.Issue)
		if !bingo163DrawValidForClass(row, true, bingo163OrderedRawSourceRevision) {
			return nil, fmt.Errorf("%w: 有序源包含无效记录", err163BingoOrderMismatch)
		}
		if _, exists := orderedByIssue[issue]; exists {
			return nil, fmt.Errorf("%w: 有序源期号 %s 重复", err163BingoOrderMismatch, issue)
		}
		orderedByIssue[issue] = row
	}

	result := make([]sourceDraw, 0, len(authoritative))
	seenAuthority := make(map[string]bool, len(authoritative))
	for _, row := range authoritative {
		if !bingo163AuthorityDrawValid(row) || seenAuthority[row.Issue] {
			return nil, fmt.Errorf("%w: 163母源期号 %s 记录无效", err163BingoOrderMismatch, row.Issue)
		}
		seenAuthority[row.Issue] = true
		orderedRow, ok := orderedByIssue[row.Issue]
		if !ok {
			return nil, fmt.Errorf("%w: 有序源缺少163期号 %s", err163BingoOrderMismatch, row.Issue)
		}
		if !sameBingoNumberSet(row.Numbers, orderedRow.Numbers) {
			return nil, fmt.Errorf("%w: 期号 %s 的20球集合不一致", err163BingoOrderMismatch, row.Issue)
		}
		if !row.DrawAt.Equal(orderedRow.DrawAt) {
			return nil, fmt.Errorf("%w: 期号 %s 的开奖时间不一致", err163BingoOrderMismatch, row.Issue)
		}
		verified := row
		verified.Numbers = append([]int(nil), orderedRow.Numbers...)
		verified.BingoOrderVerified = true
		verified.SourceRevision = bingo163VerifiedSourceRevision
		result = append(result, verified)
	}
	return result, nil
}

func fetch163BingoVerifiedAuthority(ctx context.Context) ([]sourceDraw, error) {
	return fetch163BingoVerifiedAuthorityWithRequest(ctx, time.Now, rand.Reader, func(ctx context.Context, endpoint string) ([]byte, error) {
		return request163Mirror(ctx, endpoint)
	})
}

func fetch163BingoVerifiedAuthorityWithRequest(ctx context.Context, now func() time.Time, entropy io.Reader, request bingo163Request) ([]sourceDraw, error) {
	if ctx == nil || now == nil || entropy == nil || request == nil {
		return nil, errors.New("163台湾宾果双源读取依赖无效")
	}
	checkedAt := now()
	if checkedAt.IsZero() {
		return nil, errors.New("163台湾宾果双源校验时间无效")
	}
	requestCtx, cancel := context.WithTimeout(ctx, bingo163TotalTimeout)
	defer cancel()
	stableNow := func() time.Time { return checkedAt }
	authoritative, err := fetch163BingoAuthorityWithRequest(requestCtx, stableNow, entropy, request)
	if err != nil {
		return nil, err
	}
	ordered, err := fetch163BingoOrderedAuthorityWithRequest(requestCtx, stableNow, entropy, request)
	if err != nil {
		return nil, err
	}
	return crossValidate163BingoOrder(authoritative, ordered)
}

// Every derived product depends on true ball order. A missing or mismatched
// ID185/ID135 pair therefore fails the entire family closed.
func bingo163SourceInputForBinding(binding bingo163Binding, authoritative, ordered []sourceDraw, orderedErr error) ([]sourceDraw, error) {
	_ = authoritative // retained in the signature for compatibility with older diagnostics/tests
	if !binding.RequiresOrderedSource || binding.SourceRevision != bingo163VerifiedSourceRevision {
		return nil, errors.New("163台湾宾果当前绑定未锁定双源有序版本")
	}
	if orderedErr != nil {
		return nil, orderedErr
	}
	if len(ordered) == 0 {
		return nil, fmt.Errorf("%w: 未取得已验证的有序记录", err163BingoOrderMismatch)
	}
	return ordered, nil
}

func transform163BingoDraws(binding bingo163Binding, raw []sourceDraw) ([]sourceDraw, error) {
	if len(raw) == 0 || binding.GameID == "" || binding.Transform == nil || binding.SourceRevision == "" || binding.ConversionVersion == "" {
		return nil, errors.New("163台湾宾果转换绑定或开奖记录无效")
	}
	draws := make([]sourceDraw, 0, len(raw))
	for _, row := range raw {
		if row.SourceRevision != binding.SourceRevision {
			return nil, fmt.Errorf("163台湾宾果游戏 %s 第 %s 期来源版本不匹配", binding.GameID, row.Issue)
		}
		if binding.RequiresOrderedSource && !row.BingoOrderVerified {
			return nil, fmt.Errorf("%w: 游戏 %s 第 %s 期未通过双源顺序校验", err163BingoOrderMismatch, binding.GameID, row.Issue)
		}
		if !binding.RequiresOrderedSource && row.BingoOrderVerified {
			return nil, fmt.Errorf("163台湾宾果游戏 %s 第 %s 期错误使用有序来源", binding.GameID, row.Issue)
		}
		numbers := binding.Transform(row.Numbers)
		if len(numbers) == 0 {
			return nil, fmt.Errorf("163台湾宾果游戏 %s 第 %s 期转换结果为空", binding.GameID, row.Issue)
		}
		row.Numbers = numbers
		row.ConversionRevision = binding.ConversionVersion
		draws = append(draws, row)
	}
	return draws, nil
}

var (
	err163BingoBindingChanged = errors.New("163台湾宾果来源绑定已变化")
	err163BingoDrawConflict   = errors.New("163台湾宾果开奖与既有历史冲突")
)

// sync163Bingo fetches ID135 and ID185 once per scheduler pass. Every product
// is transformed only after the complete bounded histories match exactly, so
// no jyb.one request or sorted-set approximation is reachable from production.
func (s *LotteryService) sync163Bingo(ctx context.Context) []SourceSyncResult {
	if s == nil || s.db == nil {
		return []SourceSyncResult{{GameID: "163-bingo", Status: "error", Error: "163台湾宾果数据库不可用"}}
	}
	type bindingState struct {
		binding bingo163Binding
		game    lottery.Game
		active  bool
		err     error
	}
	states := make([]bindingState, 0, len(bingo163Bindings))
	activeCount := 0
	for _, binding := range bingo163Bindings {
		state := bindingState{binding: binding}
		state.err = s.db.First(&state.game, "id = ?", binding.GameID).Error
		state.active = state.err == nil && state.game.Enabled && bingo163SourceBound(&state.game, binding)
		if state.active {
			activeCount++
		}
		states = append(states, state)
	}
	if activeCount == 0 {
		results := make([]SourceSyncResult, 0, len(states))
		for _, state := range states {
			if state.err != nil && !errors.Is(state.err, gorm.ErrRecordNotFound) {
				results = append(results, SourceSyncResult{GameID: state.binding.GameID, Status: "error", Error: state.err.Error()})
				continue
			}
			results = append(results, SourceSyncResult{GameID: state.binding.GameID, SourceName: state.game.SourceName, Status: "ok"})
		}
		return results
	}

	verified, authorityErr := fetch163BingoVerifiedAuthority(ctx)

	results := make([]SourceSyncResult, 0, len(states))
	for _, state := range states {
		if state.err != nil {
			if !errors.Is(state.err, gorm.ErrRecordNotFound) {
				results = append(results, SourceSyncResult{GameID: state.binding.GameID, Status: "error", Error: state.err.Error()})
			} else {
				results = append(results, SourceSyncResult{GameID: state.binding.GameID, Status: "ok"})
			}
			continue
		}
		if !state.active {
			results = append(results, SourceSyncResult{GameID: state.binding.GameID, SourceName: state.game.SourceName, Status: "ok"})
			continue
		}
		if authorityErr != nil {
			results = append(results, s.record163BingoError(state.binding, authorityErr, publishOfficialGameChanged))
			continue
		}
		draws, transformErr := transform163BingoDraws(state.binding, verified)
		results = append(results, s.sync163BingoGame(ctx, state.binding, draws, transformErr, publishOfficialGameChanged))
	}
	return results
}

func (s *LotteryService) sync163BingoGame(ctx context.Context, binding bingo163Binding, draws []sourceDraw, sourceErr error, publish func(lottery.Game)) SourceSyncResult {
	var game lottery.Game
	if err := s.db.First(&game, "id = ?", binding.GameID).Error; err != nil {
		return SourceSyncResult{GameID: binding.GameID, Status: "error", Error: err.Error()}
	}
	if !game.Enabled || !bingo163SourceBound(&game, binding) {
		return SourceSyncResult{GameID: binding.GameID, SourceName: game.SourceName, Status: "ok"}
	}
	previous := game
	name, sourceURL, _, _ := bingo163BindingSourceDefaults(binding)
	update := s.db.Model(&lottery.Game{}).
		Where("id = ? AND enabled = ? AND source_kind = ? AND source_name = ? AND source_url = ?", binding.GameID, true, "external", name, sourceURL).
		Update("sync_status", "syncing")
	if update.Error != nil {
		return s.record163BingoError(binding, update.Error, publish)
	}
	if update.RowsAffected != 1 {
		return SourceSyncResult{GameID: binding.GameID, SourceName: game.SourceName, Status: "ok"}
	}
	if sourceErr == nil {
		sourceErr = validate163BingoDerivedBatch(game, binding, draws)
	}
	var schedule sourceSchedule
	if sourceErr == nil {
		schedule, sourceErr = scheduleFromDraws(game, draws)
		if sourceErr == nil && (schedule.Issue == "" || schedule.DrawAt.IsZero()) {
			sourceErr = errors.New("163台湾宾果无法确定下一期开奖边界")
		}
	}
	if sourceErr != nil {
		return s.record163BingoError(binding, sourceErr, publish)
	}

	imported := 0
	committed := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&game, "id = ?", binding.GameID).Error; err != nil {
			return err
		}
		if !game.Enabled {
			return nil
		}
		if !bingo163SourceBound(&game, binding) {
			return err163BingoBindingChanged
		}
		var insertErr error
		imported, insertErr = insert163BingoDraws(tx, binding, draws)
		if insertErr != nil {
			return insertErr
		}
		if officialScheduleRegresses(game, schedule) {
			if game.SyncStatus == "syncing" && game.LastSyncError == previous.LastSyncError {
				return tx.Model(&game).Update("sync_status", previous.SyncStatus).Error
			}
			return nil
		}
		now := time.Now().UTC()
		if err := tx.Model(&game).Updates(map[string]any{
			"sync_status": "ok", "last_sync_at": now, "last_sync_error": "",
			"next_draw_at": schedule.DrawAt, "next_issue": schedule.Issue,
			"draw_interval": schedule.Interval, "timing_source": schedule.Source,
		}).Error; err != nil {
			return err
		}
		if err := tx.First(&game, "id = ?", binding.GameID).Error; err != nil {
			return err
		}
		if _, err := NewBetAdminService(tx).EnsureCurrentIssue(&game); err != nil {
			return err
		}
		committed = true
		return nil
	})
	if errors.Is(err, err163BingoBindingChanged) {
		return SourceSyncResult{GameID: binding.GameID, SourceName: game.SourceName, Status: "ok"}
	}
	if err != nil {
		return s.record163BingoError(binding, err, publish)
	}
	if !committed {
		return SourceSyncResult{GameID: binding.GameID, SourceName: game.SourceName, Status: "ok"}
	}
	if officialGameCatalogChanged(previous, game) {
		publish(game)
	}
	if game.Enabled && bingo163SourceBound(&game, binding) {
		settleImportedDrawBatch(s.db, binding.GameID, draws)
	}
	latestIssue := ""
	var latestAt time.Time
	for _, draw := range draws {
		if draw.DrawAt.After(latestAt) {
			latestIssue, latestAt = draw.Issue, draw.DrawAt
		}
	}
	return SourceSyncResult{GameID: binding.GameID, SourceName: game.SourceName, Status: "ok", Imported: imported, LatestIssue: latestIssue}
}

func validate163BingoDerivedBatch(game lottery.Game, binding bingo163Binding, draws []sourceDraw) error {
	if len(draws) < 4 {
		return errors.New("163台湾宾果转换开奖批次不足")
	}
	if err := validateSourceDrawBatch(game, draws); err != nil {
		return err
	}
	seen := make(map[string]bool, len(draws))
	for _, draw := range draws {
		if strings.TrimSpace(draw.Issue) == "" || draw.DrawAt.IsZero() || seen[draw.Issue] ||
			draw.SourceRevision != binding.SourceRevision || draw.ConversionRevision != binding.ConversionVersion ||
			draw.BingoOrderVerified != binding.RequiresOrderedSource {
			return errors.New("163台湾宾果转换开奖身份、版本或期号无效")
		}
		seen[draw.Issue] = true
	}
	return nil
}

// insert163BingoDraws is additive and idempotent. A row from an older provider
// keeps its original timestamp and revision when its converted numbers agree.
// It is never relabelled as 163. A current-revision mismatch always fails; an
// older mismatching settled row remains audit-only, while unresolved financial
// evidence or an unsettled mismatch stops the import for manual reconciliation.
func insert163BingoDraws(tx *gorm.DB, binding bingo163Binding, draws []sourceDraw) (int, error) {
	if tx == nil || binding.GameID == "" {
		return 0, errors.New("163台湾宾果写入依赖无效")
	}
	issues := make([]string, 0, len(draws))
	byIssue := make(map[string]sourceDraw, len(draws))
	for _, draw := range draws {
		issue := strings.TrimSpace(draw.Issue)
		if issue == "" || draw.SourceRevision != binding.SourceRevision || draw.ConversionRevision != binding.ConversionVersion ||
			draw.BingoOrderVerified != binding.RequiresOrderedSource {
			return 0, fmt.Errorf("%w: 游戏 %s 的待写记录版本无效", err163BingoDrawConflict, binding.GameID)
		}
		if previous, exists := byIssue[issue]; exists {
			if !sameIntSequence(previous.Numbers, draw.Numbers) || !previous.DrawAt.Equal(draw.DrawAt) {
				return 0, fmt.Errorf("%w: 游戏 %s 第 %s 期在批次内不一致", err163BingoDrawConflict, binding.GameID, issue)
			}
			return 0, fmt.Errorf("%w: 游戏 %s 第 %s 期在批次内重复", err163BingoDrawConflict, binding.GameID, issue)
		}
		byIssue[issue] = draw
		issues = append(issues, issue)
	}
	if len(issues) == 0 {
		return 0, errors.New("163台湾宾果没有可写入的开奖记录")
	}

	var existing []lottery.Draw
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("game_id = ? AND issue IN ?", binding.GameID, issues).Find(&existing).Error; err != nil {
		return 0, err
	}
	settled, err := settledBingoIssues(tx, binding.GameID, issues)
	if err != nil {
		return 0, err
	}
	evidence, err := bingoBetEvidenceByIssue(tx, binding.GameID, issues)
	if err != nil {
		return 0, err
	}
	existingIssues := make(map[string]bool, len(existing))
	for _, row := range existing {
		incoming, ok := byIssue[row.Issue]
		if !ok {
			return 0, fmt.Errorf("%w: 游戏 %s 查询到未请求的第 %s 期", err163BingoDrawConflict, binding.GameID, row.Issue)
		}
		matchingNumbers := storedDrawNumbersEqual(row.Numbers, incoming.Numbers)
		currentRevision := row.SourceRevision == binding.SourceRevision && row.ConversionRevision == binding.ConversionVersion
		if currentRevision {
			if !matchingNumbers || row.DrawAt.IsZero() || !row.DrawAt.UTC().Equal(incoming.DrawAt.UTC()) {
				return 0, fmt.Errorf("%w: 游戏 %s 第 %s 期的当前版本号码或时间不一致", err163BingoDrawConflict, binding.GameID, row.Issue)
			}
			existingIssues[row.Issue] = true
			continue
		}
		if matchingNumbers {
			// Timestamp and provenance belong to the old source and stay untouched.
			existingIssues[row.Issue] = true
			continue
		}
		if settled[row.Issue] {
			// Settled old history remains available to administrative audit but is
			// never presented as a newly verified 163 result.
			existingIssues[row.Issue] = true
			continue
		}
		if evidence[row.Issue] > 0 {
			return 0, fmt.Errorf("%w: 游戏 %s 第 %s 期存在 %d 条未解决注单证据", err163BingoDrawConflict, binding.GameID, row.Issue, evidence[row.Issue])
		}
		return 0, fmt.Errorf("%w: 游戏 %s 第 %s 期的未结算旧开奖与163结果不同", err163BingoDrawConflict, binding.GameID, row.Issue)
	}

	imported := 0
	for _, draw := range draws {
		if existingIssues[draw.Issue] {
			continue
		}
		created := tx.Create(&lottery.Draw{
			GameID: binding.GameID, Issue: draw.Issue, Numbers: joinNumbers(draw.Numbers), DrawAt: draw.DrawAt.UTC(),
			SourceRevision: draw.SourceRevision, ConversionRevision: draw.ConversionRevision,
		})
		if created.Error != nil {
			return 0, created.Error
		}
		if created.RowsAffected != 1 {
			return 0, fmt.Errorf("%w: 游戏 %s 第 %s 期写入未生效", err163BingoDrawConflict, binding.GameID, draw.Issue)
		}
		imported++
	}
	return imported, nil
}

func (s *LotteryService) record163BingoError(binding bingo163Binding, syncErr error, publish func(lottery.Game)) SourceSyncResult {
	message := limitDBText(syncErr.Error(), 480)
	var previous, game lottery.Game
	_ = s.db.First(&previous, "id = ?", binding.GameID).Error
	name, sourceURL, _, _ := bingo163BindingSourceDefaults(binding)
	updated := s.db.Model(&lottery.Game{}).
		Where("id = ? AND source_kind = ? AND source_name = ? AND source_url = ?", binding.GameID, "external", name, sourceURL).
		Updates(map[string]any{"sync_status": "error", "last_sync_error": message})
	if updated.Error == nil && updated.RowsAffected == 1 && s.db.First(&game, "id = ?", binding.GameID).Error == nil {
		_, _ = NewBetAdminService(s.db).EnsureCurrentIssue(&game)
		if officialGameCatalogChanged(previous, game) {
			publish(game)
		}
	}
	return SourceSyncResult{GameID: binding.GameID, SourceName: previous.SourceName, Status: "error", Error: message}
}
