package services

import (
	"context"
	"crypto/rand"
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
	source163PC28UpstreamGameID = 57
	source163PC28Revision       = "163-canada28-57-v1"
	source163PC28Interval       = 210
)

// The three local entries are the original guide's 加拿大28玩法一、玩法二、
// 玩法三. They intentionally share one ordered three-ball draw while keeping
// independent rule and odds versions.
var source163PC28Bindings = []source163MirrorBinding{
	{GameID: "pc-canada", UpstreamGameID: source163PC28UpstreamGameID, Count: 3, Min: 0, Max: 9, Revision: source163PC28Revision},
	{GameID: "canada-28", UpstreamGameID: source163PC28UpstreamGameID, Count: 3, Min: 0, Max: 9, Revision: source163PC28Revision},
	{GameID: "canada-20", UpstreamGameID: source163PC28UpstreamGameID, Count: 3, Min: 0, Max: 9, Revision: source163PC28Revision},
}

func source163PC28BindingForGame(gameID string) (source163MirrorBinding, bool) {
	gameID = strings.TrimSpace(gameID)
	for _, binding := range source163PC28Bindings {
		if binding.GameID == gameID {
			return binding, true
		}
	}
	return source163MirrorBinding{}, false
}

func source163PC28Bound(game *lottery.Game, binding source163MirrorBinding) bool {
	return source163MirrorBound(game, binding)
}

// Ensure163PC28Sources migrates only the untouched platform defaults. A
// merchant-selected custom source is never overwritten. The games remain
// fail-closed until ID 57 passes the same latest+history production validator
// used by the other direct 163 bindings.
func Ensure163PC28Sources(db *gorm.DB) error {
	if db == nil {
		return errors.New("163加拿大28母源数据库不可用")
	}
	for _, binding := range source163PC28Bindings {
		var game lottery.Game
		err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&game, "id = ?", binding.GameID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		updates, required := source163PC28BindingUpdates(game, binding)
		if !required {
			continue
		}
		if err := db.Model(&lottery.Game{}).Where("id = ?", binding.GameID).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}

func source163PC28BindingUpdates(game lottery.Game, binding source163MirrorBinding) (map[string]any, bool) {
	if source163PC28Bound(&game, binding) {
		status := strings.ToLower(strings.TrimSpace(game.SyncStatus))
		if status == "idle" || status == "syncing" || status == "stale" && strings.TrimSpace(game.LastSyncError) == "" {
			return map[string]any{"sync_status": "stale", "last_sync_error": source163MirrorPendingMessage}, true
		}
		return nil, false
	}
	legacyPlatform := game.ID == binding.GameID && strings.EqualFold(strings.TrimSpace(game.SourceKind), "platform") &&
		strings.TrimSpace(game.SourceName) == "王者开奖" && strings.TrimSpace(game.SourceURL) == ""
	if !legacyPlatform {
		return nil, false
	}
	return map[string]any{
		"source_kind": "external", "source_name": source163MirrorName, "source_url": source163MirrorURL,
		"sync_status": "stale", "last_sync_error": source163MirrorPendingMessage, "last_sync_at": nil,
		"draw_interval": source163PC28Interval,
	}, true
}

func fetch163PC28Draws(ctx context.Context, binding source163MirrorBinding) ([]sourceDraw, error) {
	return fetch163PC28DrawsWithRequest(ctx, binding, time.Now, rand.Reader, request163Mirror)
}

type source163PC28Observation struct {
	Latest  sourceDraw
	History []sourceDraw
}

// ID 57 occasionally publishes the newest draw a little before its history
// endpoint catches up. The remote-only reader stays fail-closed until that
// response independently proves a complete 210-second sequence. Production
// synchronization may complete a short observation from exact-version rows
// already verified in its own game's database history.
func fetch163PC28DrawsWithRequest(ctx context.Context, binding source163MirrorBinding, now func() time.Time, entropy io.Reader, request source163MirrorRequest) ([]sourceDraw, error) {
	observation, err := fetch163PC28ObservationWithRequest(ctx, binding, now, entropy, request)
	if err != nil {
		return nil, err
	}
	return verified163PC28LatestAndHistory(observation.Latest, observation.History)
}

// fetch163PC28ObservationWithRequest only reads and validates the two remote
// responses. Keeping database completion outside this function is important:
// diagnostics use this reader and must continue to describe the upstream as it
// actually is, including the short-history state seen around an ID 57 cutover.
func fetch163PC28ObservationWithRequest(ctx context.Context, binding source163MirrorBinding, now func() time.Time, entropy io.Reader, request source163MirrorRequest) (source163PC28Observation, error) {
	expected, bound := source163PC28BindingForGame(binding.GameID)
	if ctx == nil || now == nil || entropy == nil || request == nil || !bound || expected != binding {
		return source163PC28Observation{}, errors.New("163加拿大28读取依赖或绑定无效")
	}
	ctx, cancel := context.WithTimeout(ctx, source163MirrorTotalTimeout)
	defer cancel()

	latestURL, err := source163SignedURL(source163LatestPath, binding.UpstreamGameID, 0, now(), entropy)
	if err != nil {
		return source163PC28Observation{}, err
	}
	latestBody, err := request(ctx, latestURL)
	if err != nil {
		return source163PC28Observation{}, err
	}
	latestRows, err := decode163MirrorRows(latestBody, false, binding)
	if err != nil || len(latestRows) != 1 {
		return source163PC28Observation{}, errors.Join(errors.New("163加拿大28当前开奖无效"), err)
	}

	historyURL, err := source163SignedURL(source163HistoryPath, binding.UpstreamGameID, source163MirrorHistoryLimit, now(), entropy)
	if err != nil {
		return source163PC28Observation{}, err
	}
	historyBody, err := request(ctx, historyURL)
	if err != nil {
		return source163PC28Observation{}, err
	}
	history, err := decode163MirrorRows(historyBody, true, binding)
	if err != nil {
		return source163PC28Observation{}, fmt.Errorf("163加拿大28有限历史无效: %w", err)
	}

	current := now()
	if latestRows[0].DrawAt.After(current.Add(2 * time.Minute)) {
		return source163PC28Observation{}, errors.New("163加拿大28开奖时间在未来")
	}
	if current.Sub(latestRows[0].DrawAt) > source163MirrorMaxAge {
		return source163PC28Observation{}, errors.New("163加拿大28当前开奖已过期")
	}
	return source163PC28Observation{Latest: latestRows[0], History: history}, nil
}

func verified163PC28LatestAndHistory(latest sourceDraw, history []sourceDraw) ([]sourceDraw, error) {
	// Three history rows plus their exact next latest row prove the same three
	// consecutive intervals required by the normal four-row path.
	if len(history) < 3 {
		return nil, errors.New("163加拿大28有限历史不足，无法核对当前期和210秒周期")
	}
	return merge163PC28VerifiedWindow(latest, history, nil)
}

// merge163PC28VerifiedWindow completes a short remote observation with only
// rows previously imported under this exact ID 57/direct conversion contract.
// Remote rows win ordering/provenance, but an overlapping cached issue must be
// byte-for-byte equivalent in issue, ordered numbers and timestamp.
func merge163PC28VerifiedWindow(latest sourceDraw, remoteHistory, cachedHistory []sourceDraw) ([]sourceDraw, error) {
	if !valid163PC28VerifiedDraw(latest) {
		return nil, errors.New("163加拿大28当前期身份或版本无效")
	}
	remoteHistory, err := validate163PC28RemoteHistory(latest, remoteHistory)
	if err != nil {
		return nil, err
	}
	remoteWindow := make([]sourceDraw, 0, 1+len(remoteHistory))
	remoteByIssue := make(map[string]int, cap(remoteWindow))
	appendRemote := func(draw sourceDraw, origin string) error {
		if !valid163PC28VerifiedDraw(draw) {
			return fmt.Errorf("163加拿大28%s期号、号码、时间或版本无效", origin)
		}
		if draw.DrawAt.After(latest.DrawAt) {
			return fmt.Errorf("163加拿大28%s包含晚于当前接口的开奖记录", origin)
		}
		if index, exists := remoteByIssue[draw.Issue]; exists {
			if !sameSourceProbeResult(remoteWindow[index], draw) {
				return fmt.Errorf("163加拿大28第 %s 期远程与%s号码或时间不一致", draw.Issue, origin)
			}
			return nil
		}
		remoteByIssue[draw.Issue] = len(remoteWindow)
		remoteWindow = append(remoteWindow, draw)
		return nil
	}
	if err := appendRemote(latest, "当前"); err != nil {
		return nil, err
	}
	for _, draw := range remoteHistory {
		if err := appendRemote(draw, "远程历史"); err != nil {
			return nil, err
		}
	}
	// The latest row and the already-validated remote history are in newest to
	// oldest order. The cache may confirm overlaps, but can only extend before
	// this boundary; it must never repair a hole inside the remote observation.
	oldestRemote := remoteWindow[len(remoteWindow)-1]
	cached := append([]sourceDraw(nil), cachedHistory...)
	sort.Slice(cached, func(i, j int) bool { return cached[i].DrawAt.After(cached[j].DrawAt) })
	cachedByIssue := make(map[string]sourceDraw, len(cached))
	olderCandidates := make([]sourceDraw, 0, len(cached))
	for _, draw := range cached {
		if !valid163PC28VerifiedDraw(draw) {
			return nil, errors.New("163加拿大28本地已验证历史期号、号码、时间或版本无效")
		}
		if existing, duplicate := cachedByIssue[draw.Issue]; duplicate {
			if !sameSourceProbeResult(existing, draw) {
				return nil, fmt.Errorf("163加拿大28第 %s 期本地已验证历史重复且不一致", draw.Issue)
			}
			continue
		}
		cachedByIssue[draw.Issue] = draw
		if index, overlap := remoteByIssue[draw.Issue]; overlap {
			if !sameSourceProbeResult(remoteWindow[index], draw) {
				return nil, fmt.Errorf("163加拿大28第 %s 期远程与本地已验证历史号码或时间不一致", draw.Issue)
			}
			continue
		}
		drawIssue, _ := strconv.ParseUint(strings.TrimSpace(draw.Issue), 10, 64)
		latestIssue, _ := strconv.ParseUint(strings.TrimSpace(latest.Issue), 10, 64)
		if !draw.DrawAt.Before(latest.DrawAt) || drawIssue >= latestIssue {
			return nil, errors.New("163加拿大28本地已验证历史晚于远程当前期，疑似远程回退")
		}
		if draw.DrawAt.Before(oldestRemote.DrawAt) {
			olderCandidates = append(olderCandidates, draw)
		}
	}
	if len(remoteWindow) >= 4 {
		if err := validate163PC28Cadence(remoteWindow); err != nil {
			return nil, err
		}
		return remoteWindow, nil
	}
	sort.Slice(olderCandidates, func(i, j int) bool { return olderCandidates[i].DrawAt.After(olderCandidates[j].DrawAt) })
	merged := append([]sourceDraw(nil), remoteWindow...)
	previous := oldestRemote
	for _, draw := range olderCandidates {
		if len(merged) >= 4 {
			break
		}
		if !isImmediate163PC28Successor(draw, previous) {
			return nil, errors.New("163加拿大28本地已验证历史不能从远程最旧期向前连续补齐")
		}
		merged = append(merged, draw)
		previous = draw
	}
	if err := validate163PC28Cadence(merged); err != nil {
		return nil, err
	}
	return merged, nil
}

func validate163PC28RemoteHistory(latest sourceDraw, history []sourceDraw) ([]sourceDraw, error) {
	if len(history) == 0 {
		return nil, errors.New("163加拿大28远程历史为空，不使用本地历史单独补齐")
	}
	unique := make([]sourceDraw, 0, len(history))
	byIssue := make(map[string]int, len(history))
	for _, draw := range history {
		if !valid163PC28VerifiedDraw(draw) {
			return nil, errors.New("163加拿大28远程历史期号、号码、时间或版本无效")
		}
		if draw.DrawAt.After(latest.DrawAt) {
			return nil, errors.New("163加拿大28远程历史包含晚于当前接口的开奖记录")
		}
		if index, exists := byIssue[draw.Issue]; exists {
			if !sameSourceProbeResult(unique[index], draw) {
				return nil, fmt.Errorf("163加拿大28远程历史第 %s 期的重复记录不一致", draw.Issue)
			}
			continue
		}
		byIssue[draw.Issue] = len(unique)
		unique = append(unique, draw)
	}
	sort.Slice(unique, func(i, j int) bool { return unique[i].DrawAt.After(unique[j].DrawAt) })
	newest := unique[0]
	if newest.Issue == latest.Issue {
		if !sameSourceProbeResult(newest, latest) {
			return nil, errors.New("163加拿大28同一期当前与远程历史号码或时间不一致")
		}
	} else if !isImmediate163PC28Successor(newest, latest) {
		return nil, errors.New("163加拿大28远程历史最新期不是当前期或其紧邻前一期")
	}
	latestIncluded := newest.Issue == latest.Issue
	for index := 1; index < len(unique); index++ {
		newer, older := unique[index-1], unique[index]
		newerIssue, newerErr := strconv.ParseUint(strings.TrimSpace(newer.Issue), 10, 64)
		olderIssue, olderErr := strconv.ParseUint(strings.TrimSpace(older.Issue), 10, 64)
		if newerErr != nil || olderErr != nil || olderIssue == ^uint64(0) || newerIssue != olderIssue+1 ||
			!newer.DrawAt.Equal(older.DrawAt.Add(source163PC28Interval*time.Second)) {
			// A real provider outage creates one boundary between the current
			// run and older history. Do not keep a recovered source disabled
			// until that boundary falls out of the provider's full 25-row
			// response. The latest run must still independently prove four
			// consecutive periods before it can be trusted again.
			proven := index
			if !latestIncluded {
				proven++
			}
			if proven < 4 {
				return nil, errors.New("163加拿大28远程历史不是严格210秒，恢复后的连续历史不足4期")
			}
			return unique[:index], nil
		}
	}
	return unique, nil
}

func valid163PC28VerifiedDraw(draw sourceDraw) bool {
	if !sourceDiagnosticIssue.MatchString(strings.TrimSpace(draw.Issue)) || draw.DrawAt.IsZero() ||
		draw.SourceRevision != source163PC28Revision || draw.ConversionRevision != source163MirrorConversionVersion || len(draw.Numbers) != 3 {
		return false
	}
	for _, number := range draw.Numbers {
		if number < 0 || number > 9 {
			return false
		}
	}
	return true
}

func isImmediate163PC28Successor(previous, next sourceDraw) bool {
	previousIssue, previousErr := strconv.ParseUint(strings.TrimSpace(previous.Issue), 10, 64)
	nextIssue, nextErr := strconv.ParseUint(strings.TrimSpace(next.Issue), 10, 64)
	return previousErr == nil && nextErr == nil && previousIssue < ^uint64(0) && nextIssue == previousIssue+1 &&
		!previous.DrawAt.IsZero() && next.DrawAt.Equal(previous.DrawAt.Add(source163PC28Interval*time.Second))
}

func parse163PC28StoredNumbers(value string) ([]int, bool) {
	tokens := strings.Split(value, ",")
	if len(tokens) != 3 {
		return nil, false
	}
	numbers := make([]int, len(tokens))
	for index, raw := range tokens {
		token := strings.TrimSpace(raw)
		if token == "" || strings.IndexFunc(token, func(char rune) bool { return char < '0' || char > '9' }) >= 0 {
			return nil, false
		}
		number, err := strconv.Atoi(token)
		if err != nil || number < 0 || number > 9 {
			return nil, false
		}
		numbers[index] = number
	}
	return numbers, true
}

// load163PC28VerifiedHistory deliberately excludes legacy/default/provider
// rows. SourceRevision and ConversionRevision are part of the SQL predicate,
// then checked again while materializing the immutable sourceDraw values.
func load163PC28VerifiedHistory(ctx context.Context, db *gorm.DB, binding source163MirrorBinding) ([]sourceDraw, error) {
	expected, bound := source163PC28BindingForGame(binding.GameID)
	if ctx == nil || db == nil || !bound || expected != binding {
		return nil, errors.New("163加拿大28本地已验证历史读取依赖或绑定无效")
	}
	var rows []lottery.Draw
	if err := db.WithContext(ctx).
		Where("game_id = ? AND source_revision = ? AND conversion_revision = ?", binding.GameID, binding.Revision, source163MirrorConversionVersion).
		Order("draw_at DESC, id DESC").Limit(source163MirrorHistoryLimit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("163加拿大28本地已验证历史读取失败: %w", err)
	}
	draws := make([]sourceDraw, 0, len(rows))
	for _, row := range rows {
		numbers, validNumbers := parse163PC28StoredNumbers(row.Numbers)
		draw := sourceDraw{
			Issue: row.Issue, Numbers: numbers, DrawAt: row.DrawAt.UTC(),
			SourceRevision: row.SourceRevision, ConversionRevision: row.ConversionRevision,
		}
		if row.GameID != binding.GameID || !validNumbers || !valid163PC28VerifiedDraw(draw) {
			return nil, fmt.Errorf("163加拿大28本地已验证历史第 %s 期内容无效", row.Issue)
		}
		draws = append(draws, draw)
	}
	return draws, nil
}

// Canada28 is a 3 minute 30 second product. Every adjacent issue in the
// accepted current run must advance by one at exactly 210 seconds. After an
// upstream outage the remote reader trims older history at the first boundary,
// but only after the recovered run itself contains at least four periods.
func validate163PC28Cadence(draws []sourceDraw) error {
	if len(draws) < 4 {
		return errors.New("163加拿大28连续历史不足4期")
	}
	ordered := append([]sourceDraw(nil), draws...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].DrawAt.Before(ordered[j].DrawAt) })
	for index, draw := range ordered {
		issue, err := strconv.ParseUint(strings.TrimSpace(draw.Issue), 10, 64)
		if err != nil || draw.DrawAt.IsZero() {
			return errors.New("163加拿大28期号或开奖时间无效")
		}
		if index == 0 {
			continue
		}
		previous := ordered[index-1]
		previousIssue, err := strconv.ParseUint(strings.TrimSpace(previous.Issue), 10, 64)
		if err != nil || previousIssue == ^uint64(0) || issue != previousIssue+1 {
			return errors.New("163加拿大28历史期号不是逐期连续递增")
		}
		if !draw.DrawAt.Equal(previous.DrawAt.Add(source163PC28Interval * time.Second)) {
			return errors.New("163加拿大28相邻开奖时间不是严格210秒")
		}
	}
	return nil
}

// ID 57 is fetched once per scheduler run and then persisted independently for
// each rule variant. sync.Once also makes all three variants receive the same
// immutable observation if the upstream changes while the run is in flight.
func (s *LotteryService) sync163PC28(ctx context.Context) []SourceSyncResult {
	results := make([]SourceSyncResult, len(source163PC28Bindings))
	var fetchOnce sync.Once
	var observation source163PC28Observation
	var fetchErr error
	sharedFetch := func(ctx context.Context, binding source163MirrorBinding) ([]sourceDraw, error) {
		fetchOnce.Do(func() {
			observation, fetchErr = fetch163PC28ObservationWithRequest(ctx, binding, time.Now, rand.Reader, request163Mirror)
		})
		if fetchErr != nil {
			return nil, fetchErr
		}
		cached, err := load163PC28VerifiedHistory(ctx, s.db, binding)
		if err != nil {
			return nil, err
		}
		return merge163PC28VerifiedWindow(observation.Latest, observation.History, cached)
	}
	for index, binding := range source163PC28Bindings {
		results[index] = s.sync163MirrorGame(ctx, binding, sharedFetch, publishOfficialGameChanged)
	}
	return results
}
