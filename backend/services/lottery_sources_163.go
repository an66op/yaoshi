package services

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"backend/data/models/lottery"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	source163MirrorName              = "163开奖网"
	source163MirrorURL               = source163Base + "/"
	source163MirrorPendingMessage    = "等待163母源首次完整校验"
	source163MirrorConversionVersion = "direct-v1"
	source163MirrorHistoryLimit      = 32
	source163MirrorBodyLimit         = 1 << 20
	source163MirrorRequestTimeout    = 6 * time.Second
	source163MirrorTotalTimeout      = 12 * time.Second
	source163MirrorMaxAge            = 15 * time.Minute
	// ID 38's public latest payload reports a 180-period daily run. A live
	// 2026-09-05 observation at sequence 141 also carried explicit sequence 142
	// metadata on the normal five-minute cadence. Restrict the no-next exception
	// to the verified final sequence instead of treating any omitted live field
	// as a scheduled stop.
	source163FlyRacingFinalSequence = "180"
	// ID 38 has a scheduled overnight break after its final daily draw. The
	// independently observed next boundary can be more than nine hours later;
	// applying the generic high-frequency threshold during that break would
	// falsely mark the verified feed stale and encourage an unsafe switch to
	// same-named ID 164, whose results are different.
	source163FlyRacingMaxAge = 18 * time.Hour
)

const (
	legacy168HighFreqName = "168开奖网"
	legacy168HighFreqURL  = "https://kj138138.com/view/api/index.html"
)

type source163MirrorBinding struct {
	GameID, Revision string
	UpstreamGameID   int
	Count, Min, Max  int
	Unique           bool
}

var source163MirrorBindings = []source163MirrorBinding{
	{GameID: "speed-racing", UpstreamGameID: 56, Count: 10, Min: 1, Max: 10, Unique: true, Revision: "163-mirror-56-v1"},
	{GameID: "speed-fly", UpstreamGameID: 61, Count: 10, Min: 1, Max: 10, Unique: true, Revision: "163-mirror-61-v1"},
	{GameID: "sg-fly", UpstreamGameID: 58, Count: 10, Min: 1, Max: 10, Unique: true, Revision: "163-mirror-58-v1"},
	{GameID: "fly-racing", UpstreamGameID: 38, Count: 10, Min: 1, Max: 10, Unique: true, Revision: "163-mirror-38-v1"},
	{GameID: "au-lucky-10", UpstreamGameID: 33, Count: 10, Min: 1, Max: 10, Unique: true, Revision: "163-mirror-33-v1"},
	{GameID: "speed-ssc", UpstreamGameID: 55, Count: 5, Min: 0, Max: 9, Revision: "163-mirror-55-v1"},
	{GameID: "au-lucky-5", UpstreamGameID: 31, Count: 5, Min: 0, Max: 9, Revision: "163-mirror-31-v1"},
}

func source163MirrorBindingForGame(gameID string) (source163MirrorBinding, bool) {
	for _, binding := range source163MirrorBindings {
		if binding.GameID == strings.TrimSpace(gameID) {
			return binding, true
		}
	}
	return source163MirrorBinding{}, false
}

func source163MirrorFreshnessLimit(binding source163MirrorBinding) time.Duration {
	if binding.GameID == "fly-racing" && binding.UpstreamGameID == 38 {
		return source163FlyRacingMaxAge
	}
	return source163MirrorMaxAge
}

func source163FlyRacingFinalIssue(issue string) bool {
	issue = strings.TrimSpace(issue)
	if len(issue) != 11 || issue[8:] != source163FlyRacingFinalSequence {
		return false
	}
	_, err := time.Parse("20060102", issue[:8])
	return err == nil
}

func source163MirrorBound(game *lottery.Game, binding source163MirrorBinding) bool {
	return game != nil && game.ID == binding.GameID && strings.EqualFold(strings.TrimSpace(game.SourceKind), "external") &&
		strings.TrimSpace(game.SourceName) == source163MirrorName && strings.TrimSpace(game.SourceURL) == source163MirrorURL
}

// Ensure163MirrorSources performs the one-way production cutover for the seven
// products whose 163 mirror was verified against the former 168 feed. It only
// replaces the exact legacy binding, so an operator's unrelated custom source
// is never silently overwritten. A changed binding starts fail-closed until a
// complete latest+history verification succeeds.
func Ensure163MirrorSources(db *gorm.DB) error {
	if db == nil {
		return errors.New("163母源数据库不可用")
	}
	for _, binding := range source163MirrorBindings {
		var game lottery.Game
		err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&game, "id = ?", binding.GameID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		updates, required := source163MirrorBindingUpdates(game, binding)
		if !required {
			continue
		}
		if err := db.Model(&lottery.Game{}).Where("id = ?", binding.GameID).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}

func source163MirrorBindingUpdates(game lottery.Game, binding source163MirrorBinding) (map[string]any, bool) {
	if source163MirrorBound(&game, binding) {
		status := strings.ToLower(strings.TrimSpace(game.SyncStatus))
		if status == "idle" || status == "syncing" || status == "stale" && strings.TrimSpace(game.LastSyncError) == "" {
			return map[string]any{"sync_status": "stale", "last_sync_error": source163MirrorPendingMessage}, true
		}
		return nil, false
	}
	legacy := strings.EqualFold(strings.TrimSpace(game.SourceKind), "external") &&
		strings.TrimSpace(game.SourceName) == legacy168HighFreqName && strings.TrimSpace(game.SourceURL) == legacy168HighFreqURL
	if !legacy {
		return nil, false
	}
	return map[string]any{
		"source_kind": "external", "source_name": source163MirrorName, "source_url": source163MirrorURL,
		"sync_status": "stale", "last_sync_error": source163MirrorPendingMessage, "last_sync_at": nil,
	}, true
}

type source163MirrorRequest func(context.Context, string) ([]byte, error)

func request163Mirror(ctx context.Context, endpoint string) ([]byte, error) {
	requestCtx, cancel := context.WithTimeout(ctx, source163MirrorRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.New("163镜像请求构造失败")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", officialUserAgent)
	req.Header.Set("Referer", source163MirrorURL)
	client := &http.Client{Timeout: source163MirrorRequestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		if requestCtx.Err() != nil {
			return nil, errors.New("163镜像请求超时或已取消")
		}
		return nil, errors.New("163镜像连接失败")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("163镜像 HTTP %d（不重试、不跟随跳转）", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, source163MirrorBodyLimit+1))
	if err != nil {
		return nil, errors.New("163镜像响应读取失败")
	}
	if len(body) > source163MirrorBodyLimit {
		return nil, errors.New("163镜像响应超过 1 MiB 限制")
	}
	if len(body) == 0 {
		return nil, errors.New("163镜像返回空响应")
	}
	return body, nil
}

func fetch163MirrorDraws(ctx context.Context, binding source163MirrorBinding) ([]sourceDraw, error) {
	return fetch163MirrorDrawsWithRequest(ctx, binding, time.Now, rand.Reader, request163Mirror)
}

func fetch163MirrorDrawsWithRequest(ctx context.Context, binding source163MirrorBinding, now func() time.Time, entropy io.Reader, request source163MirrorRequest) ([]sourceDraw, error) {
	if ctx == nil || now == nil || entropy == nil || request == nil || binding.GameID == "" || binding.UpstreamGameID <= 0 {
		return nil, errors.New("163镜像读取依赖或绑定无效")
	}
	ctx, cancel := context.WithTimeout(ctx, source163MirrorTotalTimeout)
	defer cancel()
	latestURL, err := source163SignedURL(source163LatestPath, binding.UpstreamGameID, 0, now(), entropy)
	if err != nil {
		return nil, err
	}
	latestBody, err := request(ctx, latestURL)
	if err != nil {
		return nil, err
	}
	latestRows, err := decode163MirrorRows(latestBody, false, binding)
	if err != nil || len(latestRows) != 1 {
		return nil, errors.Join(errors.New("163镜像当前开奖无效"), err)
	}
	historyURL, err := source163SignedURL(source163HistoryPath, binding.UpstreamGameID, source163MirrorHistoryLimit, now(), entropy)
	if err != nil {
		return nil, err
	}
	historyBody, err := request(ctx, historyURL)
	if err != nil {
		return nil, err
	}
	history, err := decode163MirrorRows(historyBody, true, binding)
	if err != nil {
		return nil, fmt.Errorf("163镜像有限历史无效: %w", err)
	}
	if len(history) < 4 {
		return nil, errors.New("163镜像有限历史不足，无法核对当前期和开奖周期")
	}
	matched := false
	seen := make(map[string]bool, len(history))
	for _, draw := range history {
		if seen[draw.Issue] {
			return nil, fmt.Errorf("163镜像历史期号 %s 重复", draw.Issue)
		}
		seen[draw.Issue] = true
		if draw.Issue == latestRows[0].Issue {
			if !sameSourceProbeResult(draw, latestRows[0]) {
				return nil, errors.New("163镜像同一期当前与历史号码或时间不一致")
			}
			matched = true
		}
		if draw.DrawAt.After(latestRows[0].DrawAt) {
			return nil, errors.New("163镜像历史包含晚于当前接口的开奖记录")
		}
	}
	if !matched {
		return nil, errors.New("163镜像有限历史未包含当前期")
	}
	draws := mergeSourceDraws(latestRows, history)
	if observedDrawInterval(draws) == 0 {
		return nil, errors.New("163镜像有限历史未形成稳定连续开奖周期")
	}
	current := now()
	if latestRows[0].DrawAt.After(current.Add(2 * time.Minute)) {
		return nil, errors.New("163镜像开奖时间在未来")
	}
	if current.Sub(latestRows[0].DrawAt) > source163MirrorFreshnessLimit(binding) {
		return nil, errors.New("163镜像当前开奖已过期")
	}
	return draws, nil
}

func decode163MirrorRows(body []byte, history bool, binding source163MirrorBinding) ([]sourceDraw, error) {
	var payload source163Envelope
	if sourceProbeDecode(body, &payload) != nil || payload.Success == nil || !*payload.Success || sourceProbeJSONEmpty(payload.Result) {
		return nil, errors.New("163镜像响应结构无效")
	}
	rawRows := []json.RawMessage{payload.Result}
	if history {
		var err error
		rawRows, err = sourceProbeFirstJSONRows(payload.Result, source163MirrorHistoryLimit)
		if err != nil {
			return nil, err
		}
	}
	result := make([]sourceDraw, 0, len(rawRows))
	for _, raw := range rawRows {
		var row source163Row
		if sourceProbeDecode(raw, &row) != nil {
			return nil, errors.New("163镜像开奖记录结构无效")
		}
		id, err := row.GameID.Int64()
		if err != nil || id != int64(binding.UpstreamGameID) {
			return nil, errors.New("163镜像彩种身份不一致")
		}
		numbers, err := sourceProbeNumbers(row.Numbers, "|")
		if err != nil || len(numbers) != binding.Count {
			return nil, errors.New("163镜像开奖号码数量或格式无效")
		}
		seen := make(map[int]bool, len(numbers))
		for _, number := range numbers {
			if number < binding.Min || number > binding.Max || binding.Unique && seen[number] {
				return nil, errors.New("163镜像开奖号码越界或重复")
			}
			seen[number] = true
		}
		drawAt, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(row.Time), sgSSCLocation)
		issue := api168IssueText(row.Issue)
		if err != nil || !sourceDiagnosticIssue.MatchString(issue) {
			return nil, errors.New("163镜像期号或开奖时间无效")
		}
		draw := sourceDraw{Issue: issue, Numbers: numbers, DrawAt: drawAt.UTC(), SourceRevision: binding.Revision, ConversionRevision: source163MirrorConversionVersion}
		configuredBinding, isHighFrequencyMirror := source163MirrorBindingForGame(binding.GameID)
		if !history && isHighFrequencyMirror && configuredBinding.UpstreamGameID == binding.UpstreamGameID && configuredBinding.Revision == binding.Revision {
			nextIssue, nextAt, err := decode163MirrorNextSchedule(raw, issue, draw.DrawAt)
			if err != nil {
				return nil, err
			}
			draw.NextIssue, draw.NextDrawAt = nextIssue, nextAt
		}
		result = append(result, draw)
	}
	return result, nil
}

type source163MirrorNextScheduleRow struct {
	NextGamePeriod   any         `json:"nextGamePeriod"`
	RealNextPeriod   any         `json:"realNextGamePeriod"`
	NextPeriodOpenAt json.Number `json:"nextPeriodOpenTime"`
}

func decode163MirrorNextSchedule(raw json.RawMessage, currentIssue string, drawAt time.Time) (string, time.Time, error) {
	var row source163MirrorNextScheduleRow
	if sourceProbeDecode(raw, &row) != nil {
		return "", time.Time{}, errors.New("163镜像下一期字段结构无效")
	}
	nextIssue := strings.TrimSpace(api168IssueText(row.NextGamePeriod))
	realNextIssue := strings.TrimSpace(api168IssueText(row.RealNextPeriod))
	if nextIssue == "0" {
		nextIssue = ""
	}
	if realNextIssue == "0" {
		realNextIssue = ""
	}
	if nextIssue != "" && realNextIssue != "" && nextIssue != realNextIssue {
		return "", time.Time{}, errors.New("163镜像下一期号字段不一致")
	}
	if realNextIssue != "" {
		nextIssue = realNextIssue
	}
	rawNextAt := strings.TrimSpace(row.NextPeriodOpenAt.String())
	if rawNextAt == "" || rawNextAt == "0" {
		if nextIssue != "" {
			return "", time.Time{}, errors.New("163镜像下一期开奖时间缺失")
		}
		return "", time.Time{}, nil
	}
	nextAt, err := source163UnixMillis(row.NextPeriodOpenAt)
	if err != nil {
		return "", time.Time{}, errors.New("163镜像下一期开奖时间无效")
	}
	if nextIssue == "" {
		return "", time.Time{}, errors.New("163镜像下一期号缺失")
	}
	if !validNextSourceIssue(currentIssue, nextIssue) || !nextAt.After(drawAt) {
		return "", time.Time{}, errors.New("163镜像下一期边界无效")
	}
	return nextIssue, nextAt, nil
}

var err163MirrorBindingChanged = errors.New("163镜像来源绑定已变化")
var err163MirrorDrawConflict = errors.New("163镜像开奖与既有历史冲突")

func (s *LotteryService) sync163HighFreq(ctx context.Context) []SourceSyncResult {
	// Every 163 verification performs a latest+history round trip and may use
	// most of the scheduler's bounded run window. Running the seven independent
	// products serially lets an early slow endpoint starve every later game.
	// Keep result order deterministic while fetching and importing each game in
	// parallel; transactions still lock and re-check their own source binding.
	results := make([]SourceSyncResult, len(source163MirrorBindings))
	var group sync.WaitGroup
	group.Add(len(source163MirrorBindings))
	for index, item := range source163MirrorBindings {
		index, binding := index, item
		go func() {
			defer group.Done()
			results[index] = s.sync163MirrorGame(ctx, binding, fetch163MirrorDraws, publishOfficialGameChanged)
		}()
	}
	group.Wait()
	return results
}

type source163MirrorFetch func(context.Context, source163MirrorBinding) ([]sourceDraw, error)

// source163MirrorSchedule classifies the one verified scheduled gap in the
// high-frequency mirror catalogue. ID 38 does not always expose an
// authoritative next period while its daily run is stopped. Its latest+history
// response is still useful (and the connection is healthy), but an observed
// cadence is not permission to invent another period.
//
// The returned schedule retains the observed boundary only for the regression
// guard. Callers must persist an empty issue/boundary when awaiting is true.
func source163MirrorSchedule(binding source163MirrorBinding, game lottery.Game, draws []sourceDraw) (schedule sourceSchedule, awaiting bool, err error) {
	schedule, err = scheduleFromDraws(game, draws)
	if err != nil {
		return sourceSchedule{}, false, err
	}
	var latest sourceDraw
	for _, draw := range draws {
		if latest.DrawAt.IsZero() || draw.DrawAt.After(latest.DrawAt) {
			latest = draw
		}
	}
	hasNextMetadata := strings.TrimSpace(latest.NextIssue) != "" || !latest.NextDrawAt.IsZero()
	hasConfirmedNext := validNextSourceIssue(latest.Issue, latest.NextIssue) && latest.NextDrawAt.After(latest.DrawAt)
	if hasNextMetadata && !hasConfirmedNext {
		return sourceSchedule{}, false, errors.New("163镜像下一期边界不完整或无效")
	}
	if hasConfirmedNext {
		if schedule.Issue == "" || schedule.DrawAt.IsZero() {
			return sourceSchedule{}, false, errors.New("163镜像无法确定下一期开奖边界")
		}
		return schedule, false, nil
	}

	isFlyRacing38Final := binding.GameID == "fly-racing" && binding.UpstreamGameID == 38 && source163FlyRacingFinalIssue(latest.Issue)
	if isFlyRacing38Final {
		return schedule, true, nil
	}
	if schedule.Issue == "" || schedule.DrawAt.IsZero() {
		return sourceSchedule{}, false, errors.New("163镜像无法确定下一期开奖边界")
	}
	return schedule, false, nil
}

func source163MirrorSuccessUpdates(schedule sourceSchedule, awaiting bool, syncedAt time.Time) map[string]any {
	updates := map[string]any{
		"sync_status": "ok", "last_sync_at": syncedAt.UTC(), "last_sync_error": "",
		"next_draw_at": schedule.DrawAt, "next_issue": schedule.Issue,
		"draw_interval": schedule.Interval, "timing_source": schedule.Source,
	}
	if awaiting {
		// nil is deliberate: PostgreSQL NULL scans back to time.Time's zero value,
		// which EnsureCurrentIssue treats as no advertised betting boundary.
		updates["next_draw_at"], updates["next_issue"], updates["timing_source"] = nil, "", "pending"
	}
	return updates
}

func (s *LotteryService) sync163MirrorGame(ctx context.Context, binding source163MirrorBinding, fetch source163MirrorFetch, publish func(lottery.Game)) SourceSyncResult {
	var game lottery.Game
	if err := s.db.First(&game, "id = ?", binding.GameID).Error; err != nil {
		return SourceSyncResult{GameID: binding.GameID, Status: "error", Error: err.Error()}
	}
	if !game.Enabled || !source163MirrorBound(&game, binding) {
		return SourceSyncResult{GameID: binding.GameID, SourceName: game.SourceName, Status: "ok"}
	}
	previous := game
	update := s.db.Model(&lottery.Game{}).Where("id = ? AND enabled = ? AND source_kind = ? AND source_name = ? AND source_url = ?", binding.GameID, true, "external", source163MirrorName, source163MirrorURL).Update("sync_status", "syncing")
	if update.Error != nil {
		return s.record163MirrorError(binding, update.Error, publish)
	}
	if update.RowsAffected != 1 {
		return SourceSyncResult{GameID: binding.GameID, SourceName: game.SourceName, Status: "ok"}
	}
	draws, err := fetch(ctx, binding)
	if err == nil {
		err = validate163MirrorDrawBatch(game, binding, draws)
	}
	var schedule sourceSchedule
	awaitingSchedule := false
	if err == nil {
		schedule, awaitingSchedule, err = source163MirrorSchedule(binding, game, draws)
	}
	if err != nil {
		return s.record163MirrorError(binding, err, publish)
	}
	imported := 0
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&game, "id = ?", binding.GameID).Error; err != nil {
			return err
		}
		if !game.Enabled {
			return nil
		}
		if !source163MirrorBound(&game, binding) {
			return err163MirrorBindingChanged
		}
		var insertErr error
		imported, insertErr = insert163MirrorDraws(tx, binding, draws)
		if insertErr != nil {
			return insertErr
		}
		if officialScheduleRegresses(game, schedule) {
			return tx.Model(&game).Update("sync_status", previous.SyncStatus).Error
		}
		if err := tx.Model(&game).Updates(source163MirrorSuccessUpdates(schedule, awaitingSchedule, time.Now().UTC())).Error; err != nil {
			return err
		}
		if err := tx.First(&game, "id = ?", binding.GameID).Error; err != nil {
			return err
		}
		_, err := NewBetAdminService(tx).EnsureCurrentIssue(&game)
		return err
	})
	if errors.Is(err, err163MirrorBindingChanged) {
		return SourceSyncResult{GameID: binding.GameID, SourceName: game.SourceName, Status: "ok"}
	}
	if err != nil {
		return s.record163MirrorError(binding, err, publish)
	}
	if officialGameCatalogChanged(previous, game) {
		publish(game)
	}
	if game.Enabled && source163MirrorBound(&game, binding) {
		settleImportedDrawBatch(ctx, s.db, binding.GameID, draws)
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

func validate163MirrorDrawBatch(game lottery.Game, binding source163MirrorBinding, draws []sourceDraw) error {
	if len(draws) < 4 {
		return errors.New("163镜像开奖批次不足")
	}
	if err := validateOfficialDraws(game, draws); err != nil {
		return err
	}
	seen := make(map[string]bool, len(draws))
	for _, draw := range draws {
		if !sourceDiagnosticIssue.MatchString(draw.Issue) || draw.DrawAt.IsZero() || len(draw.Numbers) != binding.Count ||
			draw.SourceRevision != binding.Revision || draw.ConversionRevision != source163MirrorConversionVersion || seen[draw.Issue] {
			return errors.New("163镜像开奖批次身份、版本或期号无效")
		}
		seen[draw.Issue] = true
	}
	return nil
}

func insert163MirrorDraws(tx *gorm.DB, binding source163MirrorBinding, draws []sourceDraw) (int, error) {
	issues := make([]string, 0, len(draws))
	byIssue := make(map[string]sourceDraw, len(draws))
	for _, draw := range draws {
		issues = append(issues, draw.Issue)
		byIssue[draw.Issue] = draw
	}
	var existing []lottery.Draw
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("game_id = ? AND issue IN ?", binding.GameID, issues).Find(&existing).Error; err != nil {
		return 0, err
	}
	existingIssues := make(map[string]bool, len(existing))
	for _, row := range existing {
		incoming, ok := byIssue[row.Issue]
		// The old 168 endpoint and the 163 mirror can publish the same product
		// with a small timestamp offset. An already persisted row keeps its actual
		// time and source provenance; only equal ordered numbers make it compatible.
		// Never relabel old history as if 163 had originally supplied it.
		if !ok || !existing163MirrorDrawCompatible(row, incoming) {
			return 0, fmt.Errorf("%w: 游戏 %s 第 %s 期", err163MirrorDrawConflict, binding.GameID, row.Issue)
		}
		existingIssues[row.Issue] = true
	}
	imported := 0
	for _, draw := range draws {
		if existingIssues[draw.Issue] {
			continue
		}
		created := tx.Create(&lottery.Draw{GameID: binding.GameID, Issue: draw.Issue, Numbers: joinNumbers(draw.Numbers), DrawAt: draw.DrawAt.UTC(), SourceRevision: binding.Revision, ConversionRevision: source163MirrorConversionVersion})
		if created.Error != nil {
			return 0, created.Error
		}
		imported += int(created.RowsAffected)
	}
	return imported, nil
}

func existing163MirrorDrawCompatible(existing lottery.Draw, incoming sourceDraw) bool {
	return strings.TrimSpace(existing.Issue) != "" && existing.Issue == strings.TrimSpace(incoming.Issue) &&
		storedDrawNumbersEqual(existing.Numbers, incoming.Numbers)
}

func (s *LotteryService) record163MirrorError(binding source163MirrorBinding, syncErr error, publish func(lottery.Game)) SourceSyncResult {
	message := limitDBText(syncErr.Error(), 480)
	var previous, game lottery.Game
	_ = s.db.First(&previous, "id = ?", binding.GameID).Error
	updated := s.db.Model(&lottery.Game{}).Where("id = ? AND source_kind = ? AND source_name = ? AND source_url = ?", binding.GameID, "external", source163MirrorName, source163MirrorURL).
		Updates(map[string]any{"sync_status": "error", "last_sync_error": message})
	if updated.Error == nil && updated.RowsAffected == 1 && s.db.First(&game, "id = ?", binding.GameID).Error == nil {
		_, _ = NewBetAdminService(s.db).EnsureCurrentIssue(&game)
		if officialGameCatalogChanged(previous, game) {
			publish(game)
		}
	}
	return SourceSyncResult{GameID: binding.GameID, SourceName: previous.SourceName, Status: "error", Error: message}
}
