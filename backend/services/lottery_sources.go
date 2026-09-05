package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/ws"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const officialUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/126.0 Safari/537.36"

var officialGroupLocks = map[string]chan struct{}{
	"china-welfare":   make(chan struct{}, 1),
	"china-sport":     make(chan struct{}, 1),
	"taiwan-bingo":    make(chan struct{}, 1),
	"taiwan-lottery":  make(chan struct{}, 1),
	"163-highfreq":    make(chan struct{}, 1),
	"163-pc28":        make(chan struct{}, 1),
	"163-bingo":       make(chan struct{}, 1),
	"163-marksix":     make(chan struct{}, 1),
	"sg-ssc-verified": make(chan struct{}, 1),
}

// IsOfficialSourceGroup keeps the administrator test endpoint constrained to
// the same fixed provider allowlist used by the scheduler. It intentionally
// does not accept a URL from the request, so the endpoint cannot become an
// arbitrary server-side request primitive.
func IsOfficialSourceGroup(group string) bool {
	_, ok := officialGroupLocks[strings.TrimSpace(group)]
	return ok
}

type SourceSyncResult struct {
	GameID      string `json:"game_id"`
	SourceName  string `json:"source_name"`
	Status      string `json:"status"`
	Imported    int    `json:"imported"`
	LatestIssue string `json:"latest_issue"`
	Error       string `json:"error,omitempty"`
}

type sourceDraw struct {
	Issue      string
	Numbers    []int
	DrawAt     time.Time
	NextIssue  string
	NextDrawAt time.Time
	// BingoSourceTail is the optional 21st value exposed by the 168 Taiwan
	// Bingo endpoint. It duplicates the actual final/super ball and is retained
	// only so order-dependent derived games can cross-check a second feed.
	BingoSourceTail    int
	HasBingoSourceTail bool
	BingoOrderVerified bool
	SourceRevision     string
	ConversionRevision string
}

// SyncOfficialSources imports public draw results from the official lottery
// websites. Calls are deliberately sequential and capped to a small history
// window to avoid putting pressure on upstream services.
func (s *LotteryService) SyncOfficialSources(ctx context.Context) []SourceSyncResult {
	results := make([]SourceSyncResult, 0, 8)
	for _, group := range []string{"china-welfare", "china-sport", "taiwan-bingo", "taiwan-lottery", "163-highfreq", "163-pc28", "163-bingo", "163-marksix", "sg-ssc-verified"} {
		results = append(results, s.SyncOfficialGroup(ctx, group)...)
	}
	return results
}

// SyncOfficialGroup refreshes one upstream provider group. Keeping provider
// groups independent allows the high-frequency Bingo feed to run without
// waiting for the slower daily lottery websites.
func (s *LotteryService) SyncOfficialGroup(ctx context.Context, group string) []SourceSyncResult {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return []SourceSyncResult{{Status: "error", Error: "数据源同步已取消: " + err.Error()}}
	}
	group = strings.TrimSpace(group)
	lock, ok := officialGroupLocks[group]
	if !ok {
		return []SourceSyncResult{{Status: "error", Error: "未知官方数据源分组: " + group}}
	}
	select {
	case lock <- struct{}{}:
		defer func() { <-lock }()
	case <-ctx.Done():
		return []SourceSyncResult{{Status: "error", Error: "数据源同步已取消: " + ctx.Err().Error()}}
	}
	if s == nil || s.db == nil {
		return []SourceSyncResult{{Status: "error", Error: "开奖数据库不可用"}}
	}
	// Bind every query, transaction and imported-draw settlement to the lease
	// context. Losing leadership can then cancel the entire write chain instead
	// of allowing a stale process to keep advancing schedules or balances.
	worker := *s
	worker.db = s.db.WithContext(ctx)
	s = &worker

	switch group {
	case "china-welfare":
		return []SourceSyncResult{
			s.syncOfficialGame(ctx, "official-fc3d", func(ctx context.Context) ([]sourceDraw, error) { return fetchCWL(ctx, "3d", "fc3d") }),
			s.syncOfficialGame(ctx, "official-kl8", func(ctx context.Context) ([]sourceDraw, error) { return fetchCWL(ctx, "kl8", "kl8") }),
		}
	case "china-sport":
		return []SourceSyncResult{
			s.syncOfficialGame(ctx, "official-pl3", func(ctx context.Context) ([]sourceDraw, error) { return fetchSporttery(ctx, "35") }),
			s.syncOfficialGame(ctx, "official-qxc", func(ctx context.Context) ([]sourceDraw, error) { return fetchSporttery(ctx, "04") }),
		}
	case "taiwan-bingo":
		return []SourceSyncResult{s.syncOfficialGame(ctx, "official-tw-bingo", fetchTaiwanBingo)}
	case "taiwan-lottery":
		return s.syncTaiwanLatest(ctx)
	case "163-highfreq":
		return s.sync163HighFreq(ctx)
	case "163-pc28":
		return s.sync163PC28(ctx)
	case "163-bingo":
		return s.sync163Bingo(ctx)
	case "163-marksix":
		return s.sync163MarkSix(ctx)
	case "sg-ssc-verified":
		return []SourceSyncResult{s.syncOfficialGame(ctx, "sg-ssc", fetchSGSSCVerified)}
	default:
		return nil
	}
}

func (s *LotteryService) syncTaiwanLatest(ctx context.Context) []SourceSyncResult {
	gameIDs := []string{"official-tw-super-lotto", "official-tw-daily539", "official-tw-lotto649"}
	enabled, enabledErr := s.enabledOfficialGames(gameIDs)
	if enabledErr != nil {
		return []SourceSyncResult{{GameID: "taiwan-lottery", Status: "error", Error: enabledErr.Error()}}
	}
	if len(enabled) == 0 {
		return []SourceSyncResult{{GameID: "taiwan-lottery", Status: "ok"}}
	}

	latest, err := fetchTaiwanLatest(ctx)
	results := make([]SourceSyncResult, 0, len(enabled))
	for _, gameID := range gameIDs {
		if !enabled[gameID] {
			continue
		}
		if err != nil {
			results = append(results, s.recordSyncError(gameID, err))
			continue
		}
		draws := latest[gameID]
		results = append(results, s.syncOfficialGame(ctx, gameID, func(context.Context) ([]sourceDraw, error) {
			if len(draws) == 0 {
				return nil, fmt.Errorf("官方接口未返回开奖记录")
			}
			return draws, nil
		}))
	}
	return results
}

func (s *LotteryService) syncOfficialGame(ctx context.Context, gameID string, fetch func(context.Context) ([]sourceDraw, error)) SourceSyncResult {
	return s.syncOfficialGameWithPublisher(ctx, gameID, fetch, publishOfficialGameChanged)
}

// The publisher is passed per call, rather than through a mutable global hook.
// All observers (including draw notifications emitted by settlement) must see
// a committed next-period schedule and lifecycle when they refresh the game.
func (s *LotteryService) syncOfficialGameWithPublisher(ctx context.Context, gameID string, fetch func(context.Context) ([]sourceDraw, error), publish func(lottery.Game)) SourceSyncResult {
	var game lottery.Game
	if err := s.db.First(&game, "id = ?", gameID).Error; err != nil {
		return SourceSyncResult{GameID: gameID, Status: "error", Error: err.Error()}
	}
	// Disabled games remain available in the admin console with their complete
	// history, but must not consume upstream requests or settle new rounds.
	if !game.Enabled {
		return SourceSyncResult{GameID: gameID, SourceName: game.SourceName, Status: "ok"}
	}
	if gameID == "sg-ssc" && !sgSSCSourceBound(&game) {
		return SourceSyncResult{GameID: gameID, SourceName: game.SourceName, Status: "ok"}
	}
	markSix168Binding, is168MarkSix := api168MarkSixBindingForGame(gameID)
	if is168MarkSix && !api168MarkSixSourceBound(&game, markSix168Binding) {
		return SourceSyncResult{GameID: gameID, SourceName: game.SourceName, Status: "ok"}
	}
	previous := game
	// Keep the previous error visible while retrying.  A failed source must not
	// reopen betting until a complete successful response clears the error.
	syncing := s.db.Model(&lottery.Game{}).Where("id = ?", gameID)
	if gameID == "sg-ssc" {
		syncing = syncing.Where("enabled = ? AND source_kind = ? AND source_name = ? AND source_url = ?", true, "external", sgSSCVerifiedSourceName, sgSSCVerifiedSourceURL)
	} else if is168MarkSix {
		syncing = syncing.Where("enabled = ? AND source_kind = ? AND source_name = ? AND source_url = ?", true, "external", legacy168HighFreqName, legacy168HighFreqURL)
	}
	updated := syncing.Update("sync_status", "syncing")
	if updated.Error != nil {
		return s.recordSyncErrorWithPublisher(gameID, updated.Error, publish)
	}
	if updated.RowsAffected != 1 {
		return SourceSyncResult{GameID: gameID, SourceName: game.SourceName, Status: "ok"}
	}
	draws, err := fetch(ctx)
	if err != nil {
		return s.recordSyncErrorWithPublisher(gameID, err, publish)
	}
	// Validate the complete transformed game batch before writing any row.
	// Profiles are keyed by game ID, not the source's raw ball count (a Bingo
	// feed can contain 20 balls before its racing/SSC transform is applied).
	// Checking first prevents a malformed later row from being half-imported.
	if err := validateSourceDrawBatch(game, draws); err != nil {
		return s.recordSyncErrorWithPublisher(gameID, err, publish)
	}
	schedule, err := scheduleFromDraws(game, draws)
	if err != nil {
		return s.recordSyncErrorWithPublisher(gameID, err, publish)
	}
	imported := 0
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A room operator may disable a game while its HTTP request is in flight.
		// Re-read under the same lock used by this schedule write before opening
		// anything, and publish the resulting state only after commit.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&game, "id = ?", gameID).Error; err != nil {
			return err
		}
		if !game.Enabled {
			return nil
		}
		if gameID == "sg-ssc" {
			if !sgSSCSourceBound(&game) {
				return fmt.Errorf("SG时时彩来源绑定已变化，等待重新核验")
			}
			if err := validateSGSSCFreshness(draws[len(draws)-1], time.Now()); err != nil {
				return err
			}
			if err := sgSSCIssueEvidenceError(tx, schedule.Issue, nil); err != nil {
				return err
			}
		}
		if is168MarkSix && !api168MarkSixSourceBound(&game, markSix168Binding) {
			return err168MarkSixBindingChanged
		}
		var importErr error
		imported, importErr = insertOfficialDraws(tx, gameID, draws)
		if importErr != nil {
			return importErr
		}
		if officialScheduleRegresses(game, schedule) {
			// A cached latest response is still useful for missing historical
			// draws, but cannot rewind a verified newer issue or declare a failed
			// live feed healthy. Undo only our own transient syncing marker.
			if game.SyncStatus == "syncing" && game.LastSyncError == previous.LastSyncError {
				if err := tx.Model(&game).Update("sync_status", previous.SyncStatus).Error; err != nil {
					return err
				}
			}
			return nil
		}
		now := time.Now().UTC()
		// The issue and boundary are one schedule, never independent guesses.
		// A stale response must not move the same expired issue into the future.
		updates := map[string]any{
			"sync_status": "ok", "last_sync_at": now, "last_sync_error": "",
			"next_draw_at": schedule.DrawAt, "next_issue": schedule.Issue,
			"draw_interval": schedule.Interval, "timing_source": schedule.Source,
		}
		if err := tx.Model(&game).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.First(&game, "id = ?", gameID).Error; err != nil {
			return err
		}
		_, err := NewBetAdminService(tx).EnsureCurrentIssue(&game)
		return err
	})
	if errors.Is(err, err168MarkSixBindingChanged) {
		return SourceSyncResult{GameID: gameID, SourceName: game.SourceName, Status: "ok"}
	}
	if err != nil {
		return s.recordSyncErrorWithPublisher(gameID, err, publish)
	}
	if officialGameCatalogChanged(previous, game) {
		publish(game)
	}
	shouldSettle := game.Enabled
	if shouldSettle && is168MarkSix {
		var current lottery.Game
		shouldSettle = s.db.First(&current, "id = ?", gameID).Error == nil && api168MarkSixSourceBound(&current, markSix168Binding)
	}
	if shouldSettle {
		// Retrying an existing draw is deliberate: a process can stop after the
		// draw/schedule commit but before settlement. SettleImportedDraw already
		// makes completely settled periods a no-op, including notifications.
		settleImportedDrawBatch(ctx, s.db, gameID, draws)
	}
	latestIssue := ""
	var latestAt time.Time
	for _, draw := range draws {
		if draw.DrawAt.After(latestAt) {
			latestIssue, latestAt = draw.Issue, draw.DrawAt
		}
	}
	return SourceSyncResult{GameID: gameID, SourceName: game.SourceName, Status: "ok", Imported: imported, LatestIssue: latestIssue}
}

func publishOfficialGameChanged(game lottery.Game) {
	ws.NotifyGameCatalogChanged(0, "*", "", game.ID, game.Enabled)
}

func officialGameCatalogChanged(previous, current lottery.Game) bool {
	return previous.Enabled != current.Enabled || previous.NextIssue != current.NextIssue ||
		!previous.NextDrawAt.Equal(current.NextDrawAt) || previous.DrawInterval != current.DrawInterval ||
		previous.TimingSource != current.TimingSource ||
		sourceHealthyForGame(&previous) != sourceHealthyForGame(&current) ||
		previous.LastSyncError != current.LastSyncError
}

func officialScheduleRegresses(game lottery.Game, candidate sourceSchedule) bool {
	if game.NextIssue == "" || game.NextIssue == candidate.Issue || game.NextDrawAt.IsZero() {
		return false
	}
	// An arbitrary configured seed is not a verified boundary: the first real
	// upstream/history observation must still be allowed to correct that seed.
	if game.TimingSource != "upstream" && game.TimingSource != "observed" {
		return false
	}
	return candidate.DrawAt.Before(game.NextDrawAt)
}

func validateSourceDrawBatch(game lottery.Game, draws []sourceDraw) error {
	if err := validateOfficialDraws(game, draws); err != nil {
		return err
	}
	if game.ID == "sg-ssc" {
		if err := validateSGSSCImportRevision(draws); err != nil {
			return err
		}
	}
	for _, draw := range draws {
		if strings.TrimSpace(draw.Issue) == "" || len(draw.Numbers) == 0 {
			return fmt.Errorf("缺少有效开奖期号或号码")
		}
		if draw.DrawAt.IsZero() {
			return fmt.Errorf("第 %s 期缺少有效开奖时间", sourceIssueLabel(draw.Issue))
		}
	}
	return nil
}

func insertOfficialDraws(db *gorm.DB, gameID string, draws []sourceDraw) (int, error) {
	if gameID == "sg-ssc" {
		return insertVerifiedSGSSCDraws(db, draws)
	}
	verifiedExisting, err := verifiedBingoExistingDraws(db, gameID, draws)
	if err != nil {
		return 0, err
	}
	imported := 0
	for _, item := range draws {
		draw := lottery.Draw{
			GameID: gameID, Issue: item.Issue, Numbers: joinNumbers(item.Numbers), DrawAt: item.DrawAt.UTC(),
			SourceRevision: item.SourceRevision, ConversionRevision: item.ConversionRevision,
		}
		if item.BingoOrderVerified {
			if verifiedExisting[item.Issue] {
				continue
			}
			// A verified missing issue is inserted without ON CONFLICT. If another
			// writer races this transaction, the uniqueness error keeps the source
			// unhealthy rather than silently accepting a row we did not compare.
			result := db.Create(&draw)
			if result.Error != nil {
				return 0, result.Error
			}
			imported += int(result.RowsAffected)
			continue
		}
		result := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "game_id"}, {Name: "issue"}}, DoNothing: true}).Create(&draw)
		if result.Error != nil {
			return 0, result.Error
		}
		imported += int(result.RowsAffected)
	}
	return imported, nil
}

var errVerifiedBingoDrawConflict = errors.New("双源验证开奖与既有历史冲突")

type verifiedBingoIncoming struct {
	Numbers            []int
	DrawAt             time.Time
	SourceRevision     string
	ConversionRevision string
}

type verifiedBingoLegacyUpdate struct {
	ID     uint64
	Values map[string]any
}

// verifiedBingoExistingDraws protects both the legacy transition and all
// later verified revisions. Exact legacy results can be claimed by the new
// revision. A mismatching legacy result may be corrected only when no live or
// archived bet exists and its issue is not settled. Settled legacy history is
// deliberately isolated (kept blank/non-current) so new periods can proceed;
// unresolved financial evidence and current-revision mismatches fail closed.
func verifiedBingoExistingDraws(db *gorm.DB, gameID string, draws []sourceDraw) (map[string]bool, error) {
	incoming := make(map[string]verifiedBingoIncoming)
	issues := make([]string, 0, len(draws))
	for _, draw := range draws {
		if !draw.BingoOrderVerified {
			continue
		}
		issue := strings.TrimSpace(draw.Issue)
		if issue == "" || strings.TrimSpace(draw.SourceRevision) == "" || strings.TrimSpace(draw.ConversionRevision) == "" {
			return nil, fmt.Errorf("%w: 游戏 %s 的双源记录缺少期号或持久化版本", errVerifiedBingoDrawConflict, gameID)
		}
		if previous, exists := incoming[issue]; exists {
			if !sameIntSequence(previous.Numbers, draw.Numbers) || !previous.DrawAt.Equal(draw.DrawAt.UTC()) ||
				previous.SourceRevision != draw.SourceRevision || previous.ConversionRevision != draw.ConversionRevision {
				return nil, verifiedBingoDrawConflictError(gameID, issue, joinNumbers(previous.Numbers), draw.Numbers)
			}
			return nil, fmt.Errorf("%w: 游戏 %s 第 %s 期在同一批次重复", errVerifiedBingoDrawConflict, gameID, issue)
		}
		incoming[issue] = verifiedBingoIncoming{
			Numbers: append([]int(nil), draw.Numbers...), DrawAt: draw.DrawAt.UTC(),
			SourceRevision: draw.SourceRevision, ConversionRevision: draw.ConversionRevision,
		}
		issues = append(issues, issue)
	}
	existingByIssue := make(map[string]bool, len(issues))
	if len(issues) == 0 {
		return existingByIssue, nil
	}
	var existing []lottery.Draw
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("game_id = ? AND issue IN ?", gameID, issues).Find(&existing).Error; err != nil {
		return nil, err
	}
	settled, err := settledBingoIssues(db, gameID, issues)
	if err != nil {
		return nil, err
	}
	evidence, err := bingoBetEvidenceByIssue(db, gameID, issues)
	if err != nil {
		return nil, err
	}
	updates := make([]verifiedBingoLegacyUpdate, 0, len(existing))
	for _, row := range existing {
		verified, ok := incoming[row.Issue]
		if !ok {
			return nil, fmt.Errorf("%w: 游戏 %s 查询到未请求的第 %s 期", errVerifiedBingoDrawConflict, gameID, row.Issue)
		}
		matchingNumbers := storedDrawNumbersEqual(row.Numbers, verified.Numbers)
		matchingDraw := matchingNumbers && !row.DrawAt.IsZero() && row.DrawAt.UTC().Equal(verified.DrawAt.UTC())
		currentRevision := row.SourceRevision == verified.SourceRevision && row.ConversionRevision == verified.ConversionRevision
		if currentRevision {
			if !matchingDraw {
				return nil, fmt.Errorf("%w: 游戏 %s 第 %s 期的当前版本已有号码/时间 %q @ %s，与双源验证值 %q @ %s 不一致；同步已停止且不会自动覆盖，请转人工对账",
					errVerifiedBingoDrawConflict, gameID, row.Issue, row.Numbers, row.DrawAt.UTC().Format(time.RFC3339Nano),
					joinNumbers(verified.Numbers), verified.DrawAt.UTC().Format(time.RFC3339Nano))
			}
			existingByIssue[row.Issue] = true
			continue
		}
		if matchingDraw {
			updates = append(updates, verifiedBingoLegacyUpdate{ID: row.ID, Values: map[string]any{
				"source_revision": verified.SourceRevision, "conversion_revision": verified.ConversionRevision,
			}})
			existingByIssue[row.Issue] = true
			continue
		}
		if settled[row.Issue] {
			// This published financial history remains intentionally legacy. It is
			// never passed off as verified and settleImportedDrawBatch skips it.
			existingByIssue[row.Issue] = true
			continue
		}
		if evidence[row.Issue] > 0 {
			return nil, verifiedBingoFinancialConflictError(gameID, row.Issue, evidence[row.Issue])
		}
		updates = append(updates, verifiedBingoLegacyUpdate{ID: row.ID, Values: map[string]any{
			"numbers": joinNumbers(verified.Numbers), "draw_at": verified.DrawAt,
			"source_revision": verified.SourceRevision, "conversion_revision": verified.ConversionRevision,
		}})
		existingByIssue[row.Issue] = true
	}
	// All rows are classified before the first mutation. In production this is
	// inside the same import transaction and the earlier FOR UPDATE lock makes
	// the financial-evidence decision and correction atomic.
	for _, update := range updates {
		result := db.Model(&lottery.Draw{}).Where("id = ?", update.ID).Updates(update.Values)
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected != 1 {
			return nil, fmt.Errorf("%w: 旧开奖 %d 的原子升级未生效", errVerifiedBingoDrawConflict, update.ID)
		}
	}
	return existingByIssue, nil
}

func settledBingoIssues(db *gorm.DB, gameID string, issues []string) (map[string]bool, error) {
	var rows []string
	if err := db.Model(&lottery.Issue{}).
		Where("game_id = ? AND issue IN ? AND status = ?", gameID, issues, lottery.IssueStatusSettled).
		Pluck("issue", &rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string]bool, len(rows))
	for _, issue := range rows {
		result[issue] = true
	}
	return result, nil
}

func bingoBetEvidenceByIssue(db *gorm.DB, gameID string, issues []string) (map[string]int64, error) {
	type evidenceRow struct {
		Issue string
		Count int64
	}
	var rows []evidenceRow
	if err := db.Raw(`
		SELECT evidence.issue, COUNT(*) AS count
		FROM (
			SELECT issue FROM lottery_bets WHERE game_id = ? AND issue IN ?
			UNION ALL
			SELECT issue FROM lottery_bet_archives WHERE game_id = ? AND issue IN ?
		) AS evidence
		GROUP BY evidence.issue`, gameID, issues, gameID, issues).Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string]int64, len(rows))
	for _, row := range rows {
		result[row.Issue] = row.Count
	}
	return result, nil
}

func verifiedBingoDrawConflictError(gameID, issue, stored string, verified []int) error {
	return fmt.Errorf("%w: 游戏 %s 第 %s 期的当前版本已有号码 %q，与双源验证号码 %q 不一致；同步已停止且不会自动覆盖，请转人工对账",
		errVerifiedBingoDrawConflict, gameID, issue, stored, joinNumbers(verified))
}

func verifiedBingoFinancialConflictError(gameID, issue string, evidence int64) error {
	return fmt.Errorf("%w: 游戏 %s 第 %s 期旧开奖与双源结果不一致，且存在 %d 条未隔离的投注证据；已保持原开奖与资金状态并停止同步，请转人工对账",
		errVerifiedBingoDrawConflict, gameID, issue, evidence)
}

func storedDrawNumbersEqual(stored string, verified []int) bool {
	tokens := strings.Split(stored, ",")
	if len(tokens) != len(verified) {
		return false
	}
	for index, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" || strings.IndexFunc(token, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return false
		}
		number, err := strconv.Atoi(token)
		if err != nil || number != verified[index] {
			return false
		}
	}
	return true
}

func sameIntSequence(first, second []int) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}

func settleImportedDrawBatch(ctx context.Context, db *gorm.DB, gameID string, draws []sourceDraw) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil || db == nil || len(draws) == 0 {
		return
	}
	db = db.WithContext(ctx)
	issues := make([]string, 0, len(draws))
	for _, draw := range draws {
		issues = append(issues, draw.Issue)
	}
	if len(trustedDrawRevisionContracts(gameID)) > 0 {
		var trustedIssues []string
		if err := trustedDrawsForGame(db.Model(&lottery.Draw{}), gameID).
			Where("issue IN ?", issues).Pluck("issue", &trustedIssues).Error; err != nil {
			log.Printf("可信开奖结算候选读取失败: game=%s error=%v", gameID, err)
			return
		}
		trusted := make(map[string]bool, len(trustedIssues))
		for _, issue := range trustedIssues {
			trusted[issue] = true
		}
		filtered := make([]sourceDraw, 0, len(draws))
		for _, draw := range draws {
			if trusted[draw.Issue] {
				filtered = append(filtered, draw)
			}
		}
		draws = filtered
		if len(draws) == 0 {
			return
		}
		issues = issues[:0]
		for _, draw := range draws {
			issues = append(issues, draw.Issue)
		}
	}
	// A history backfill can contain thousands of draw-only rows. Build the
	// settlement worklist with two bulk queries and do not create lifecycle rows
	// for periods that never accepted a bet. Live periods remain candidates
	// because their lifecycle already exists in a non-settled state.
	var pendingIssues, unfinishedIssues []string
	if err := db.Model(&bet.Bet{}).Distinct("issue").Where("game_id = ? AND issue IN ? AND status = ?", gameID, issues, "pending").Pluck("issue", &pendingIssues).Error; err != nil {
		log.Printf("待结算期号读取失败: game=%s error=%v", gameID, err)
		return
	}
	if err := db.Model(&lottery.Issue{}).
		Where("game_id = ? AND issue IN ? AND status <> ?", gameID, issues, lottery.IssueStatusSettled).
		Pluck("issue", &unfinishedIssues).Error; err != nil {
		log.Printf("未完成开奖生命周期读取失败: game=%s error=%v", gameID, err)
		return
	}
	service := NewBetAdminService(db)
	settleImportedDrawCandidates(ctx, draws, pendingIssues, unfinishedIssues, func(issue string) {
		service.SettleImportedDraw(gameID, issue)
	})
}

func settleImportedDrawCandidates(ctx context.Context, draws []sourceDraw, pendingIssues, unfinishedIssues []string, settle func(string)) {
	if settle == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	candidates := make(map[string]struct{}, len(pendingIssues)+len(unfinishedIssues))
	for _, issue := range pendingIssues {
		candidates[issue] = struct{}{}
	}
	for _, issue := range unfinishedIssues {
		candidates[issue] = struct{}{}
	}
	for _, draw := range draws {
		if ctx.Err() != nil {
			return
		}
		if _, candidate := candidates[draw.Issue]; candidate {
			settle(draw.Issue)
			delete(candidates, draw.Issue)
		}
	}
}

// History is a backfill, not another live schedule. It must never rewind the
// next issue or clear a more recent source error while recovering missed draws.
func (s *LotteryService) importOfficialHistory(ctx context.Context, gameID string, draws []sourceDraw) (int, error) {
	if len(draws) == 0 {
		return 0, nil
	}
	var game lottery.Game
	if err := s.db.First(&game, "id = ?", gameID).Error; err != nil {
		return 0, err
	}
	if !game.Enabled {
		return 0, nil
	}
	if err := validateSourceDrawBatch(game, draws); err != nil {
		return 0, err
	}
	imported := 0
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&game, "id = ?", gameID).Error; err != nil {
			return err
		}
		if !game.Enabled {
			return nil
		}
		var err error
		imported, err = insertOfficialDraws(tx, gameID, draws)
		return err
	})
	if err != nil {
		return 0, err
	}
	if game.Enabled {
		settleImportedDrawBatch(ctx, s.db, gameID, draws)
	}
	return imported, nil
}

func validateOfficialDraws(game lottery.Game, draws []sourceDraw) error {
	profile, supported := rulesForGame(&game)
	if !supported {
		// Unmodelled products may retain/display raw external history. They
		// cannot take new bets or perform financial settlement, and must never
		// be inferred to be racing simply because they have 10 or 20 balls.
		return nil
	}
	for _, draw := range draws {
		if err := profile.validateDraw(draw.Numbers); err != nil {
			return fmt.Errorf("%s 第 %s 期开奖数据无效：%w", game.Name, sourceIssueLabel(draw.Issue), err)
		}
	}
	return nil
}

func sourceIssueLabel(issue string) string {
	if issue = strings.TrimSpace(issue); issue != "" {
		return issue
	}
	return "未知期号"
}

func (s *LotteryService) enabledOfficialGames(gameIDs []string) (map[string]bool, error) {
	type gameRow struct {
		ID string
	}
	rows := make([]gameRow, 0, len(gameIDs))
	if err := s.db.Model(&lottery.Game{}).
		Select("id").
		Where("id IN ? AND enabled = ?", gameIDs, true).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	enabled := make(map[string]bool, len(rows))
	for _, row := range rows {
		enabled[row.ID] = true
	}
	return enabled, nil
}

func (s *LotteryService) recordSyncError(gameID string, syncErr error) SourceSyncResult {
	return s.recordSyncErrorWithPublisher(gameID, syncErr, publishOfficialGameChanged)
}

func (s *LotteryService) recordSyncErrorWithPublisher(gameID string, syncErr error, publish func(lottery.Game)) SourceSyncResult {
	message := limitDBText(syncErr.Error(), 480)
	var game lottery.Game
	_ = s.db.First(&game, "id = ?", gameID).Error
	previous := game
	update := s.db.Model(&lottery.Game{}).Where("id = ?", gameID)
	if gameID == "sg-ssc" {
		update = update.Where("enabled = ? AND source_kind = ? AND source_name = ? AND source_url = ?", true, "external", sgSSCVerifiedSourceName, sgSSCVerifiedSourceURL)
	} else if _, ok := api168MarkSixBindingForGame(gameID); ok {
		update = update.Where("enabled = ? AND source_kind = ? AND source_name = ? AND source_url = ?", true, "external", legacy168HighFreqName, legacy168HighFreqURL)
	}
	result := update.Updates(map[string]any{"sync_status": "error", "last_sync_error": message})
	if result.Error == nil && result.RowsAffected == 1 && s.db.First(&game, "id = ?", gameID).Error == nil {
		_, _ = NewBetAdminService(s.db).EnsureCurrentIssue(&game)
		if officialGameCatalogChanged(previous, game) {
			publish(game)
		}
	}
	return SourceSyncResult{GameID: gameID, SourceName: game.SourceName, Status: "error", Error: message}
}

func fetchCWL(ctx context.Context, gameName, pageName string) ([]sourceDraw, error) {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Timeout: 15 * time.Second, Jar: jar}
	indexURL := fmt.Sprintf("https://www.cwl.gov.cn/ygkj/wqkjgg/%s/", pageName)
	if _, err := doJSONRequest(ctx, client, indexURL, "https://www.cwl.gov.cn/", nil); err != nil {
		return nil, fmt.Errorf("初始化中国福彩网会话失败: %w", err)
	}
	query := url.Values{"name": {gameName}, "issueCount": {""}, "issueStart": {""}, "issueEnd": {""}, "dayStart": {""}, "dayEnd": {""}, "pageNo": {"1"}, "pageSize": {"30"}, "systemType": {"PC"}}
	endpoint := "https://www.cwl.gov.cn/cwl_admin/front/cwlkj/search/kjxx/findDrawNotice?" + query.Encode()
	var payload struct {
		Result []struct {
			Code string `json:"code"`
			Date string `json:"date"`
			Red  string `json:"red"`
		} `json:"result"`
	}
	if _, err := doJSONRequest(ctx, client, endpoint, indexURL, &payload); err != nil {
		return nil, fmt.Errorf("中国福彩网读取失败: %w", err)
	}
	if len(payload.Result) == 0 {
		return nil, fmt.Errorf("中国福彩网未返回开奖记录")
	}
	result := make([]sourceDraw, 0, len(payload.Result))
	for _, item := range payload.Result {
		result = append(result, sourceDraw{Issue: item.Code, Numbers: parseNumberList(item.Red), DrawAt: parseChinaDrawDate(item.Date)})
	}
	return result, nil
}

func fetchSporttery(ctx context.Context, gameNo string) ([]sourceDraw, error) {
	query := url.Values{"gameNo": {gameNo}, "provinceId": {"0"}, "pageSize": {"30"}, "isVerify": {"1"}, "pageNo": {"1"}}
	endpoint := "https://webapi.sporttery.cn/gateway/lottery/getHistoryPageListV1.qry?" + query.Encode()
	var payload struct {
		Success bool `json:"success"`
		Value   struct {
			List []struct {
				Issue   string `json:"lotteryDrawNum"`
				Date    string `json:"lotteryDrawTime"`
				Numbers string `json:"lotteryDrawResult"`
			} `json:"list"`
		} `json:"value"`
	}
	if _, err := doJSONRequest(ctx, &http.Client{Timeout: 15 * time.Second}, endpoint, "https://www.lottery.gov.cn/", &payload); err != nil {
		return nil, fmt.Errorf("中国体彩网读取失败: %w", err)
	}
	if !payload.Success || len(payload.Value.List) == 0 {
		return nil, fmt.Errorf("中国体彩网未返回开奖记录")
	}
	result := make([]sourceDraw, 0, len(payload.Value.List))
	for _, item := range payload.Value.List {
		result = append(result, sourceDraw{Issue: item.Issue, Numbers: parseNumberList(item.Numbers), DrawAt: parseDateAt(item.Date, 20, 30, "Asia/Shanghai")})
	}
	return result, nil
}

func fetchTaiwanBingo(ctx context.Context) ([]sourceDraw, error) {
	location, _ := time.LoadLocation("Asia/Taipei")
	for dayOffset := 0; dayOffset < 2; dayOffset++ {
		date := time.Now().In(location).AddDate(0, 0, -dayOffset).Format("2006-01-02")
		query := url.Values{"openDate": {date}, "pageNum": {"1"}, "pageSize": {"30"}}
		var payload struct {
			RTCode  int `json:"rtCode"`
			Content struct {
				Items []struct {
					Issue   int64    `json:"drawTerm"`
					Numbers []string `json:"openShowOrder"`
				} `json:"bingoQueryResult"`
			} `json:"content"`
		}
		endpoint := "https://api.taiwanlottery.com/TLCAPIWeB/Lottery/BingoResult?" + query.Encode()
		if _, err := doJSONRequest(ctx, &http.Client{Timeout: 15 * time.Second}, endpoint, "https://www.taiwanlottery.com/", &payload); err != nil {
			return nil, fmt.Errorf("台湾彩券宾果读取失败: %w", err)
		}
		if len(payload.Content.Items) == 0 {
			continue
		}
		now := time.Now().In(location)
		result := make([]sourceDraw, 0, len(payload.Content.Items))
		for index, item := range payload.Content.Items {
			result = append(result, sourceDraw{Issue: strconv.FormatInt(item.Issue, 10), Numbers: parseStringNumbers(item.Numbers), DrawAt: now.Add(-time.Duration(index) * 5 * time.Minute)})
		}
		return result, nil
	}
	return nil, fmt.Errorf("台湾彩券宾果未返回开奖记录")
}

func fetchTaiwanLatest(ctx context.Context) (map[string][]sourceDraw, error) {
	var payload struct {
		RTCode  int `json:"rtCode"`
		Content struct {
			SuperLotto *taiwanDraw `json:"superLotto638Result"`
			Daily539   *taiwanDraw `json:"daily539Result"`
			Lotto649   *taiwanDraw `json:"lotto649Result"`
		} `json:"content"`
	}
	endpoint := "https://api.taiwanlottery.com/TLCAPIWeB/Lottery/LatestResult"
	if _, err := doJSONRequest(ctx, &http.Client{Timeout: 15 * time.Second}, endpoint, "https://www.taiwanlottery.com/", &payload); err != nil {
		return nil, fmt.Errorf("台湾彩券读取失败: %w", err)
	}
	result := make(map[string][]sourceDraw)
	for gameID, item := range map[string]*taiwanDraw{"official-tw-super-lotto": payload.Content.SuperLotto, "official-tw-daily539": payload.Content.Daily539, "official-tw-lotto649": payload.Content.Lotto649} {
		if item == nil || item.Period == 0 || len(item.Numbers) == 0 {
			continue
		}
		result[gameID] = []sourceDraw{{Issue: strconv.FormatInt(item.Period, 10), Numbers: item.Numbers, DrawAt: parseDateAt(strings.Split(item.LotteryDate, "T")[0], 20, 30, "Asia/Taipei")}}
	}
	return result, nil
}

type taiwanDraw struct {
	Period      int64  `json:"period"`
	LotteryDate string `json:"lotteryDate"`
	Numbers     []int  `json:"drawNumberSize"`
}

func doJSONRequest(ctx context.Context, client *http.Client, endpoint, referer string, target any) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", officialUserAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	if target != nil {
		if err := json.Unmarshal(body, target); err != nil {
			return nil, fmt.Errorf("响应不是有效 JSON: %w", err)
		}
	}
	return body, nil
}

func parseNumberList(value string) []int {
	value = strings.NewReplacer(",", " ", "+", " ").Replace(value)
	return parseStringNumbers(strings.Fields(value))
}

func parseStringNumbers(items []string) []int {
	result := make([]int, 0, len(items))
	for _, item := range items {
		if number, err := strconv.Atoi(strings.TrimSpace(item)); err == nil {
			result = append(result, number)
		}
	}
	return result
}

func joinNumbers(numbers []int) string {
	parts := make([]string, len(numbers))
	for index, number := range numbers {
		parts[index] = strconv.Itoa(number)
	}
	return strings.Join(parts, ",")
}

func parseChinaDrawDate(value string) time.Time {
	return parseDateAt(strings.Split(value, "(")[0], 21, 15, "Asia/Shanghai")
}

func parseDateAt(value string, hour, minute int, timezone string) time.Time {
	location, _ := time.LoadLocation(timezone)
	parsed, err := time.ParseInLocation("2006-01-02", value, location)
	if err != nil {
		return time.Now().In(location)
	}
	return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), hour, minute, 0, 0, location)
}
