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
	"sync"
	"time"

	"backend/data/models/lottery"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	source163MarkSixHistoryMinimum = 7
	source163MarkSixMaxFuture      = 8 * 24 * time.Hour
	source163MarkSixDailyInterval  = 24 * time.Hour
	source163MarkSixPendingMessage = "等待163六合彩七球母源首次完整校验"
)

// The source and conversion identities are separate contracts. All four
// products are direct seven-ball feeds, but a later product-specific transform
// must never silently acquire another product's settlement identity.
type source163MarkSixBinding struct {
	GameID, SourceRevision, ConversionRevision string
	UpstreamGameID                             int
	Daily                                      bool
}

var source163MarkSixBindings = []source163MarkSixBinding{
	{GameID: "hong-kong-mark-six", UpstreamGameID: 18, SourceRevision: "163-hong-kong-mark-six-18-v1", ConversionRevision: "hong-kong-mark-six-direct7-v1"},
	{GameID: "happy8-mark-six", UpstreamGameID: 141, SourceRevision: "163-happy8-mark-six-141-v1", ConversionRevision: "happy8-mark-six-direct7-v1", Daily: true},
	{GameID: "new-macau-mark-six", UpstreamGameID: 140, SourceRevision: "163-new-macau-140-v1", ConversionRevision: "new-macau-mark-six-direct7-v1", Daily: true},
	{GameID: "old-macau-mark-six", UpstreamGameID: 70, SourceRevision: "163-old-macau-mark-six-70-v1", ConversionRevision: "old-macau-mark-six-direct7-v1", Daily: true},
}

func source163MarkSixBindingForGame(gameID string) (source163MarkSixBinding, bool) {
	for _, binding := range source163MarkSixBindings {
		if binding.GameID == strings.TrimSpace(gameID) {
			return binding, true
		}
	}
	return source163MarkSixBinding{}, false
}

func source163MarkSixBindingForUpstream(upstreamGameID int) (source163MarkSixBinding, bool) {
	for _, binding := range source163MarkSixBindings {
		if binding.UpstreamGameID == upstreamGameID {
			return binding, true
		}
	}
	return source163MarkSixBinding{}, false
}

func (binding source163MarkSixBinding) mirrorBinding() source163MirrorBinding {
	return source163MirrorBinding{GameID: binding.GameID, UpstreamGameID: binding.UpstreamGameID, Count: 7, Min: 1, Max: 49, Unique: true, Revision: binding.SourceRevision}
}

func source163MarkSixBound(game *lottery.Game, binding source163MarkSixBinding) bool {
	return source163MirrorBound(game, binding.mirrorBinding())
}

// Ensure163MarkSixSources cuts over only the exact former defaults. It leaves
// operator-selected custom sources and all historic draw/financial rows alone.
func Ensure163MarkSixSources(db *gorm.DB) error {
	if db == nil {
		return errors.New("163六合彩数据库不可用")
	}
	for _, binding := range source163MarkSixBindings {
		var game lottery.Game
		err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&game, "id = ?", binding.GameID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		updates, required := source163MarkSixBindingUpdates(game, binding)
		if required {
			if err := db.Model(&lottery.Game{}).Where("id = ?", binding.GameID).Updates(updates).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func source163MarkSixBindingUpdates(game lottery.Game, binding source163MarkSixBinding) (map[string]any, bool) {
	if source163MarkSixBound(&game, binding) {
		status := strings.ToLower(strings.TrimSpace(game.SyncStatus))
		if status == "idle" || status == "syncing" || status == "stale" && strings.TrimSpace(game.LastSyncError) == "" {
			return map[string]any{"sync_status": "stale", "last_sync_error": source163MarkSixPendingMessage}, true
		}
		return nil, false
	}
	legacy168 := strings.EqualFold(strings.TrimSpace(game.SourceKind), "external") &&
		strings.TrimSpace(game.SourceName) == legacy168HighFreqName && strings.TrimSpace(game.SourceURL) == legacy168HighFreqURL
	legacyPlatform := binding.GameID == "happy8-mark-six" &&
		(strings.EqualFold(strings.TrimSpace(game.SourceKind), "platform") || strings.EqualFold(strings.TrimSpace(game.SourceKind), "simulated")) &&
		strings.TrimSpace(game.SourceName) == "王者开奖" && strings.TrimSpace(game.SourceURL) == ""
	if !legacy168 && !legacyPlatform {
		return nil, false
	}
	return map[string]any{
		"source_kind": "external", "source_name": source163MirrorName, "source_url": source163MirrorURL,
		"sync_status": "stale", "last_sync_error": source163MarkSixPendingMessage, "last_sync_at": nil,
	}, true
}

type source163MarkSixLatestRow struct {
	source163Row
	NextGamePeriod   any         `json:"nextGamePeriod"`
	RealNextPeriod   any         `json:"realNextGamePeriod"`
	NextPeriodOpenAt json.Number `json:"nextPeriodOpenTime"`
}

func fetch163MarkSixDraws(ctx context.Context, binding source163MarkSixBinding) ([]sourceDraw, error) {
	return fetch163MarkSixDrawsWithRequest(ctx, binding, time.Now, rand.Reader, request163Mirror)
}

func fetch163MarkSixDrawsWithRequest(ctx context.Context, binding source163MarkSixBinding, now func() time.Time, entropy io.Reader, request source163MirrorRequest) ([]sourceDraw, error) {
	if ctx == nil || now == nil || entropy == nil || request == nil || binding.GameID == "" || binding.UpstreamGameID <= 0 {
		return nil, errors.New("163六合彩读取依赖或绑定无效")
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
	latest, err := decode163MarkSixLatest(latestBody, binding)
	if err != nil {
		return nil, fmt.Errorf("163六合彩当前开奖无效: %w", err)
	}

	historyURL, err := source163SignedURL(source163HistoryPath, binding.UpstreamGameID, source163MirrorHistoryLimit, now(), entropy)
	if err != nil {
		return nil, err
	}
	historyBody, err := request(ctx, historyURL)
	if err != nil {
		return nil, err
	}
	history, err := decode163MirrorRows(historyBody, true, binding.mirrorBinding())
	if err != nil {
		return nil, fmt.Errorf("163六合彩有限历史无效: %w", err)
	}
	for index := range history {
		history[index].ConversionRevision = binding.ConversionRevision
	}
	if len(history) < source163MarkSixHistoryMinimum {
		return nil, errors.New("163六合彩有限历史不足")
	}

	matched := false
	seen := make(map[string]bool, len(history))
	for _, draw := range history {
		if seen[draw.Issue] {
			return nil, fmt.Errorf("163六合彩历史期号 %s 重复", draw.Issue)
		}
		seen[draw.Issue] = true
		if draw.Issue == latest.Issue {
			if !sameSourceProbeResult(draw, latest) {
				return nil, errors.New("163六合彩同一期当前与历史号码或时间不一致")
			}
			matched = true
		}
		if draw.DrawAt.After(latest.DrawAt) {
			return nil, errors.New("163六合彩历史包含晚于当前接口的开奖记录")
		}
	}
	if !matched {
		return nil, errors.New("163六合彩有限历史未包含当前期")
	}
	draws := mergeSourceDraws([]sourceDraw{latest}, history)
	if err := validate163MarkSixDrawBatch(binding, draws); err != nil {
		return nil, err
	}
	current := now().UTC()
	if latest.DrawAt.After(current.Add(2 * time.Minute)) {
		return nil, errors.New("163六合彩开奖时间在未来")
	}
	if latest.NextDrawAt.Before(current.Add(-2 * time.Minute)) {
		return nil, errors.New("163六合彩下一期开奖时间已过期")
	}
	if latest.NextDrawAt.After(current.Add(source163MarkSixMaxFuture)) {
		return nil, errors.New("163六合彩下一期开奖时间超出安全窗口")
	}
	return draws, nil
}

func decode163MarkSixLatest(body []byte, binding source163MarkSixBinding) (sourceDraw, error) {
	rows, err := decode163MirrorRows(body, false, binding.mirrorBinding())
	if err != nil || len(rows) != 1 {
		return sourceDraw{}, errors.Join(errors.New("七球结构无效"), err)
	}
	var payload source163Envelope
	if sourceProbeDecode(body, &payload) != nil {
		return sourceDraw{}, errors.New("响应结构无效")
	}
	var row source163MarkSixLatestRow
	if sourceProbeDecode(payload.Result, &row) != nil {
		return sourceDraw{}, errors.New("下一期字段结构无效")
	}
	nextIssue := api168IssueText(row.RealNextPeriod)
	if nextIssue == "" {
		nextIssue = api168IssueText(row.NextGamePeriod)
	}
	if !consecutive163DailyIssue(rows[0].Issue, nextIssue) {
		return sourceDraw{}, errors.New("下一期号不是当前期的连续期")
	}
	nextAt, err := source163UnixMillis(row.NextPeriodOpenAt)
	if err != nil || !nextAt.After(rows[0].DrawAt) {
		return sourceDraw{}, errors.New("下一期开奖时间无效")
	}
	draw := rows[0]
	draw.NextIssue = nextIssue
	draw.NextDrawAt = nextAt
	draw.ConversionRevision = binding.ConversionRevision
	return draw, nil
}

func source163UnixMillis(value json.Number) (time.Time, error) {
	milliseconds, err := strconv.ParseInt(strings.TrimSpace(value.String()), 10, 64)
	if err != nil || milliseconds <= 0 {
		return time.Time{}, errors.New("毫秒时间戳无效")
	}
	return time.UnixMilli(milliseconds).UTC(), nil
}

func validate163MarkSixDrawBatch(binding source163MarkSixBinding, draws []sourceDraw) error {
	if len(draws) < source163MarkSixHistoryMinimum {
		return errors.New("163六合彩开奖批次不足")
	}
	ordered := append([]sourceDraw(nil), draws...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].DrawAt.Before(ordered[j].DrawAt) })
	seenIssues := make(map[string]bool, len(ordered))
	for index, draw := range ordered {
		if !sourceDiagnosticIssue.MatchString(draw.Issue) || draw.DrawAt.IsZero() || len(draw.Numbers) != 7 ||
			draw.SourceRevision != binding.SourceRevision || draw.ConversionRevision != binding.ConversionRevision || seenIssues[draw.Issue] {
			return errors.New("163六合彩开奖批次身份、版本、时间或七球结构无效")
		}
		seenIssues[draw.Issue] = true
		seenNumbers := make(map[int]bool, 7)
		for _, number := range draw.Numbers {
			if number < 1 || number > 49 || seenNumbers[number] {
				return errors.New("163六合彩开奖号码越界或重复")
			}
			seenNumbers[number] = true
		}
		if index == 0 {
			continue
		}
		previous := ordered[index-1]
		if !consecutive163DailyIssue(previous.Issue, draw.Issue) || !draw.DrawAt.After(previous.DrawAt) {
			return errors.New("163六合彩有限历史期号或开奖时间不连续")
		}
		if binding.Daily && draw.DrawAt.Sub(previous.DrawAt) != source163MarkSixDailyInterval {
			return errors.New("163六合彩日更产品开奖间隔不连续")
		}
	}
	latest := ordered[len(ordered)-1]
	if latest.NextIssue == "" || latest.NextDrawAt.IsZero() || !consecutive163DailyIssue(latest.Issue, latest.NextIssue) || !latest.NextDrawAt.After(latest.DrawAt) {
		return errors.New("163六合彩缺少可信的下一期号或开奖时间")
	}
	if binding.Daily && latest.NextDrawAt.Sub(latest.DrawAt) != source163MarkSixDailyInterval {
		return errors.New("163六合彩日更产品下一期开奖间隔无效")
	}
	return nil
}

func consecutive163DailyIssue(previous, current string) bool {
	previousNumber, previousErr := strconv.ParseUint(strings.TrimSpace(previous), 10, 64)
	currentNumber, currentErr := strconv.ParseUint(strings.TrimSpace(current), 10, 64)
	if previousErr != nil || currentErr != nil || previousNumber == 0 || currentNumber == 0 {
		return false
	}
	if currentNumber == previousNumber+1 {
		return true
	}
	previousText, currentText := strings.TrimSpace(previous), strings.TrimSpace(current)
	return len(previousText) == 7 && len(currentText) == 7 && currentText[4:] == "001" && currentText[:4] == strconv.FormatUint(previousNumber/1000+1, 10)
}

func schedule163MarkSix(binding source163MarkSixBinding, draws []sourceDraw) (sourceSchedule, error) {
	if err := validate163MarkSixDrawBatch(binding, draws); err != nil {
		return sourceSchedule{}, err
	}
	latest := draws[0]
	for _, draw := range draws[1:] {
		if draw.DrawAt.After(latest.DrawAt) {
			latest = draw
		}
	}
	interval := int(latest.NextDrawAt.Sub(latest.DrawAt) / time.Second)
	if interval <= 0 {
		return sourceSchedule{}, errors.New("163六合彩开奖周期无效")
	}
	return sourceSchedule{Issue: latest.NextIssue, DrawAt: latest.NextDrawAt.UTC(), Interval: interval, Source: "upstream"}, nil
}

var err163MarkSixBindingChanged = errors.New("163六合彩来源绑定已变化")

func (s *LotteryService) sync163MarkSix(ctx context.Context) []SourceSyncResult {
	results := make([]SourceSyncResult, len(source163MarkSixBindings))
	var group sync.WaitGroup
	group.Add(len(source163MarkSixBindings))
	for index, item := range source163MarkSixBindings {
		index, binding := index, item
		go func() {
			defer group.Done()
			results[index] = s.sync163MarkSixGame(ctx, binding, fetch163MarkSixDraws, publishOfficialGameChanged)
		}()
	}
	group.Wait()
	return results
}

type source163MarkSixFetch func(context.Context, source163MarkSixBinding) ([]sourceDraw, error)

func (s *LotteryService) sync163MarkSixGame(ctx context.Context, binding source163MarkSixBinding, fetch source163MarkSixFetch, publish func(lottery.Game)) SourceSyncResult {
	var game lottery.Game
	if err := s.db.First(&game, "id = ?", binding.GameID).Error; err != nil {
		return SourceSyncResult{GameID: binding.GameID, Status: "error", Error: err.Error()}
	}
	if !game.Enabled || !source163MarkSixBound(&game, binding) {
		return SourceSyncResult{GameID: binding.GameID, SourceName: game.SourceName, Status: "ok"}
	}
	previous := game
	updated := s.db.Model(&lottery.Game{}).Where("id = ? AND enabled = ? AND source_kind = ? AND source_name = ? AND source_url = ?", binding.GameID, true, "external", source163MirrorName, source163MirrorURL).Update("sync_status", "syncing")
	if updated.Error != nil {
		return s.record163MarkSixError(binding, updated.Error, publish)
	}
	if updated.RowsAffected != 1 {
		return SourceSyncResult{GameID: binding.GameID, SourceName: game.SourceName, Status: "ok"}
	}
	draws, err := fetch(ctx, binding)
	if err == nil {
		err = validate163MarkSixDrawBatch(binding, draws)
	}
	var schedule sourceSchedule
	if err == nil {
		schedule, err = schedule163MarkSix(binding, draws)
	}
	if err != nil {
		return s.record163MarkSixError(binding, err, publish)
	}

	imported := 0
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&game, "id = ?", binding.GameID).Error; err != nil {
			return err
		}
		if !game.Enabled {
			return nil
		}
		if !source163MarkSixBound(&game, binding) {
			return err163MarkSixBindingChanged
		}
		var insertErr error
		imported, insertErr = insert163MarkSixDraws(tx, binding, draws)
		if insertErr != nil {
			return insertErr
		}
		if officialScheduleRegresses(game, schedule) {
			return tx.Model(&game).Update("sync_status", previous.SyncStatus).Error
		}
		now := time.Now().UTC()
		if err := tx.Model(&game).Updates(map[string]any{
			"sync_status": "ok", "last_sync_at": now, "last_sync_error": "", "next_draw_at": schedule.DrawAt,
			"next_issue": schedule.Issue, "draw_interval": schedule.Interval, "timing_source": schedule.Source,
		}).Error; err != nil {
			return err
		}
		if err := tx.First(&game, "id = ?", binding.GameID).Error; err != nil {
			return err
		}
		if _, err := repair163HongKongIssueLifecycle(tx, &game, binding, schedule, now); err != nil {
			return err
		}
		_, err := NewBetAdminService(tx).EnsureCurrentIssue(&game)
		return err
	})
	if errors.Is(err, err163MarkSixBindingChanged) {
		return SourceSyncResult{GameID: binding.GameID, SourceName: game.SourceName, Status: "ok"}
	}
	if err != nil {
		return s.record163MarkSixError(binding, err, publish)
	}
	if officialGameCatalogChanged(previous, game) {
		publish(game)
	}
	if game.Enabled && source163MarkSixBound(&game, binding) {
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

func insert163MarkSixDraws(tx *gorm.DB, binding source163MarkSixBinding, draws []sourceDraw) (int, error) {
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
	imported := 0
	existingIssues := make(map[string]bool, len(existing))
	for _, row := range existing {
		incoming, ok := byIssue[row.Issue]
		if !ok || !existing163MirrorDrawCompatible(row, incoming) {
			return 0, fmt.Errorf("%w: 游戏 %s 第 %s 期", err163MirrorDrawConflict, binding.GameID, row.Issue)
		}
		if row.SourceRevision == binding.SourceRevision && row.ConversionRevision == binding.ConversionRevision && !row.DrawAt.UTC().Equal(incoming.DrawAt.UTC()) {
			return 0, fmt.Errorf("%w: 游戏 %s 第 %s 期的已晋升开奖时间与163不一致", err163MirrorDrawConflict, binding.GameID, row.Issue)
		}
		if promotable163MarkSixDraw(row, incoming, binding) {
			// The row was locked above. Repeat every safety predicate in the UPDATE
			// so even a future caller without that lock cannot relabel a changed row.
			promoted := tx.Model(&lottery.Draw{}).
				Where("id = ? AND game_id = ? AND issue = ? AND numbers = ? AND draw_at = ? AND source_revision = ? AND conversion_revision = ?",
					row.ID, binding.GameID, row.Issue, row.Numbers, row.DrawAt, row.SourceRevision, row.ConversionRevision).
				Updates(map[string]any{"source_revision": binding.SourceRevision, "conversion_revision": binding.ConversionRevision})
			if promoted.Error != nil {
				return 0, promoted.Error
			}
			if promoted.RowsAffected != 1 {
				return 0, fmt.Errorf("%w: 游戏 %s 第 %s 期在来源晋升前发生变化", err163MirrorDrawConflict, binding.GameID, row.Issue)
			}
			imported++
		}
		// A different non-empty source and a blank row whose timestamp differs
		// remain immutable and untrusted. Matching numbers alone are never enough
		// to claim that 163 originally supplied the result.
		existingIssues[row.Issue] = true
	}
	for _, draw := range draws {
		if existingIssues[draw.Issue] {
			continue
		}
		created := tx.Create(&lottery.Draw{GameID: binding.GameID, Issue: draw.Issue, Numbers: joinNumbers(draw.Numbers), DrawAt: draw.DrawAt.UTC(), SourceRevision: binding.SourceRevision, ConversionRevision: binding.ConversionRevision})
		if created.Error != nil {
			return 0, created.Error
		}
		imported += int(created.RowsAffected)
	}
	return imported, nil
}

func promotable163MarkSixDraw(existing lottery.Draw, incoming sourceDraw, binding source163MarkSixBinding) bool {
	if existing.GameID != binding.GameID || incoming.SourceRevision != binding.SourceRevision || incoming.ConversionRevision != binding.ConversionRevision ||
		strings.TrimSpace(existing.Issue) == "" || existing.Issue != strings.TrimSpace(incoming.Issue) ||
		!storedDrawNumbersEqual(existing.Numbers, incoming.Numbers) || !existing.DrawAt.UTC().Equal(incoming.DrawAt.UTC()) {
		return false
	}
	sourceRevision, conversionRevision := strings.TrimSpace(existing.SourceRevision), strings.TrimSpace(existing.ConversionRevision)
	if sourceRevision == "" && conversionRevision == "" {
		return true
	}
	return binding.GameID == "new-macau-mark-six" && binding.UpstreamGameID == 140 &&
		sourceRevision == binding.SourceRevision && conversionRevision == source163MirrorConversionVersion
}

func (s *LotteryService) record163MarkSixError(binding source163MarkSixBinding, syncErr error, publish func(lottery.Game)) SourceSyncResult {
	message := limitDBText(syncErr.Error(), 480)
	var previous, game lottery.Game
	_ = s.db.First(&previous, "id = ?", binding.GameID).Error
	updated := s.db.Model(&lottery.Game{}).Where("id = ? AND source_kind = ? AND source_name = ? AND source_url = ?", binding.GameID, "external", source163MirrorName, source163MirrorURL).Updates(map[string]any{"sync_status": "error", "last_sync_error": message})
	if updated.Error == nil && updated.RowsAffected == 1 && s.db.First(&game, "id = ?", binding.GameID).Error == nil {
		_, _ = NewBetAdminService(s.db).EnsureCurrentIssue(&game)
		if officialGameCatalogChanged(previous, game) {
			publish(game)
		}
	}
	return SourceSyncResult{GameID: binding.GameID, SourceName: previous.SourceName, Status: "error", Error: message}
}
