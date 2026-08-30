package services

import (
	"backend/data/models/lottery"
	"backend/data/models/plan"
	"backend/data/models/settings"
	apperrors "backend/errors"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"
)

const DefaultPlanKey = "four-period-five-codes"
const MaxActivePlanStreams = 20

type PlanOption struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Kind        string `json:"kind"`
	Periods     int    `json:"periods"`
	NumberCount int    `json:"number_count"`
}

var racingPlanOptions = []PlanOption{
	{"four-period-five-codes", "四期五码", "numbers", 4, 5},
	{"three-period-five-codes", "三期五码", "numbers", 3, 5},
	{"four-period-six-codes", "四期六码", "numbers", 4, 6},
	{"three-period-six-codes", "三期六码", "numbers", 3, 6},
	{"four-period-seven-codes", "四期七码", "numbers", 4, 7},
	{"three-period-seven-codes", "三期七码", "numbers", 3, 7},
	{"two-period-eight-codes", "二期八码", "numbers", 2, 8},
	{"one-period-eight-codes", "一期八码", "numbers", 1, 8},
	{"size-five-periods", "大小五期", "size", 5, 0},
	{"size-four-periods", "大小四期", "size", 4, 0},
	{"size-three-periods", "大小三期", "size", 3, 0},
	{"parity-five-periods", "单双五期", "parity", 5, 0},
	{"parity-four-periods", "单双四期", "parity", 4, 0},
	{"parity-three-periods", "单双三期", "parity", 3, 0},
	{"dragon-tiger-five-periods", "龙虎五期", "dragon_tiger", 5, 0},
	{"dragon-tiger-four-periods", "龙虎四期", "dragon_tiger", 4, 0},
	{"dragon-tiger-three-periods", "龙虎三期", "dragon_tiger", 3, 0},
}

type PlanPosition struct {
	Position         int    `json:"position"`
	Label            string `json:"label"`
	OpponentPosition int    `json:"opponent_position"`
}

func planPositions() []PlanPosition {
	names := []string{"冠军", "亚军", "第三名", "第四名", "第五名", "第六名", "第七名", "第八名", "第九名", "第十名"}
	result := make([]PlanPosition, 10)
	for i, name := range names {
		result[i] = PlanPosition{i + 1, name, 10 - i}
	}
	return result
}

func defaultPlanPositions() []int { return []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10} }
func defaultPlanKeys() []string {
	result := make([]string, 0, len(racingPlanOptions))
	for _, option := range racingPlanOptions {
		result = append(result, option.Key)
	}
	return result
}

func planOption(key string) (PlanOption, bool) {
	for _, option := range racingPlanOptions {
		if option.Key == key {
			return option, true
		}
	}
	return PlanOption{}, false
}

func normalizePlanMatrix(positions []int, keys []string) ([]int, []string, error) {
	if len(positions) > 10 || len(keys) > len(racingPlanOptions) {
		return nil, nil, apperrors.NewBusinessError("INVALID_REQUEST", "推荐位置或方案数量超出范围")
	}
	p, k := []int{}, []string{}
	seenP, seenK := map[int]bool{}, map[string]bool{}
	for _, position := range positions {
		if position < 1 || position > 10 {
			return nil, nil, apperrors.NewBusinessError("INVALID_REQUEST", "推荐位置必须为1至10")
		}
		if !seenP[position] {
			p = append(p, position)
			seenP[position] = true
		}
	}
	for _, key := range keys {
		if _, ok := planOption(key); !ok {
			return nil, nil, apperrors.NewBusinessError("INVALID_REQUEST", "不支持的推荐方案")
		}
		seenK[key] = true
	}
	for _, option := range racingPlanOptions {
		if seenK[option.Key] {
			k = append(k, option.Key)
		}
	}
	sort.Ints(p)
	return p, k, nil
}

func decodePlanMatrix(row plan.Automation) ([]int, []string, error) {
	positions, keys := defaultPlanPositions(), defaultPlanKeys()
	if row.PositionsJSON != "" {
		if err := json.Unmarshal([]byte(row.PositionsJSON), &positions); err != nil {
			return nil, nil, err
		}
	}
	if row.PlanKeysJSON != "" {
		if err := json.Unmarshal([]byte(row.PlanKeysJSON), &keys); err != nil {
			return nil, nil, err
		}
	}
	return normalizePlanMatrix(positions, keys)
}

func planStreamAllowed(config PlanAutomationView, position int, key string) bool {
	if !config.Enabled {
		return false
	}
	game, p, k := false, false, false
	for _, id := range config.GameIDs {
		game = game || id == "speed-racing"
	}
	for _, value := range config.Positions {
		p = p || value == position
	}
	for _, value := range config.PlanKeys {
		k = k || value == key
	}
	return game && p && k
}

type PlanStreamSelection struct {
	Position    int    `json:"position"`
	PlanKey     string `json:"plan_key"`
	Kind        string `json:"kind"`
	Periods     int    `json:"periods"`
	NumberCount int    `json:"number_count"`
}
type PlanStreamState struct {
	ID                 uint64     `json:"id"`
	Allowed            bool       `json:"allowed"`
	Active             bool       `json:"active"`
	ActivationRequired bool       `json:"activation_required"`
	ActiveUntil        *time.Time `json:"active_until"`
	ActiveCount        int        `json:"active_count"`
	MaxActive          int        `json:"max_active"`
}
type PlanStreamDetail struct {
	PlanDetail
	Options          []PlanOption             `json:"options"`
	Positions        []PlanPosition           `json:"positions"`
	AllowedPositions []int                    `json:"allowed_positions"`
	AllowedPlanKeys  []string                 `json:"allowed_plan_keys"`
	Selection        PlanStreamSelection      `json:"selection"`
	Stream           PlanStreamState          `json:"stream"`
	LegacyHistory    []PlanRecommendationView `json:"legacy_history"`
	Notice           string                   `json:"notice"`
}

func streamIsDefault(stream plan.Stream) bool {
	return stream.Position == 1 && stream.PlanKey == DefaultPlanKey
}
func planStreamActive(stream plan.Stream, cycle plan.StreamCycle, now time.Time) bool {
	if stream.Revoked || stream.ActiveUntil == nil {
		return false
	}
	// Clamp leases written by the old 30-minute worker as well. Neither the
	// default strategy nor an unfinished cycle keeps an unseen stream alive.
	return now.Before(*stream.ActiveUntil) && now.Before(stream.UpdatedAt.Add(time.Minute))
}

func readPlanStreams(db *gorm.DB, workspaceID uint64) ([]plan.Stream, map[uint64]plan.StreamCycle, error) {
	var streams []plan.Stream
	if err := db.Where("workspace_id = ?", workspaceID).Order("id").Find(&streams).Error; err != nil {
		return nil, nil, err
	}
	ids := []uint64{}
	for _, stream := range streams {
		if stream.CycleID > 0 {
			ids = append(ids, stream.CycleID)
		}
	}
	cycles := map[uint64]plan.StreamCycle{}
	if len(ids) > 0 {
		var rows []plan.StreamCycle
		if err := db.Select("id", "stream_id", "periods", "published_periods", "status", "start_issue", "last_issue_id", "last_scheduled_at").Where("id IN ?", ids).Find(&rows).Error; err != nil {
			return nil, nil, err
		}
		for _, row := range rows {
			cycles[row.ID] = row
		}
	}
	return streams, cycles, nil
}

func planStreamRoomAvailable(db *gorm.DB, workspaceID uint64, gameIDs ...string) (bool, error) {
	gameID := "speed-racing"
	if len(gameIDs) > 0 {
		gameID = gameIDs[0]
	}
	enabled, err := WorkspaceGameEnabled(db, workspaceID, gameID)
	if err != nil || !enabled {
		return false, err
	}
	var count int64
	err = db.Model(&settings.SystemConfig{}).
		Joins("JOIN workspaces AS room ON room.id = system_settings.workspace_id").
		Joins(`JOIN "user" AS owner ON owner.user_id = room.owner_user_id`).
		Where("system_settings.workspace_id = ? AND system_settings.room_enabled = true AND system_settings.prediction_enabled = true AND room.room_code <> '' AND room.type IN ? AND owner.status = 1 AND owner.deleted_at IS NULL", workspaceID, []string{"tenant", "agent"}).Count(&count).Error
	return count == 1, err
}

func (s *PlanContentService) StreamDetail(workspaceID uint64, position int, key string, historyLimits ...int) (PlanStreamDetail, error) {
	option, ok := planOption(key)
	if !ok || position < 1 || position > 10 {
		return PlanStreamDetail{}, apperrors.NewBusinessError("INVALID_REQUEST", "推荐位置或方案不正确")
	}
	if err := s.ensureScope(workspaceID, "speed-racing"); err != nil {
		return PlanStreamDetail{}, err
	}
	result := PlanStreamDetail{PlanDetail: PlanDetail{GameID: "speed-racing", Recommendations: []PlanRecommendationView{}, History: []PlanRecommendationView{}, LatestRecommendations: []PlanRecommendationView{}},
		Options: append([]PlanOption{}, racingPlanOptions...), Positions: planPositions(), AllowedPositions: []int{}, AllowedPlanKeys: []string{},
		Selection: PlanStreamSelection{position, key, option.Kind, option.Periods, option.NumberCount}, Stream: PlanStreamState{MaxActive: MaxActivePlanStreams}, LegacyHistory: []PlanRecommendationView{}, Notice: PlanDemoNotice}
	result.GenerationMode, result.HistoryLimit, result.RefreshSeconds = "on_visit", planHistoryLimit(historyLimits), 15
	config, err := NewPlanAutomationService(s.db).Get(workspaceID)
	if err != nil {
		return result, err
	}
	roomOpen, err := planStreamRoomAvailable(s.db, workspaceID)
	if err != nil {
		return result, err
	}
	if config.Enabled && roomOpen {
		for _, game := range config.GameIDs {
			if game == "speed-racing" {
				result.AllowedPositions, result.AllowedPlanKeys = config.Positions, config.PlanKeys
			}
		}
	}
	result.Stream.Allowed = roomOpen && planStreamAllowed(config, position, key)
	result.AutomationEnabled = result.Stream.Allowed
	// Revoked selections never expose their cached stream payload.
	if !result.Stream.Allowed {
		return result, nil
	}
	streams, cycles, err := readPlanStreams(s.db, workspaceID)
	if err != nil {
		return result, err
	}
	var selected plan.Stream
	for _, stream := range streams {
		if !planRequestedStreamAllowed(config, stream.GameID, stream.Position, stream.PlanKey) {
			continue
		}
		if planStreamActive(stream, cycles[stream.CycleID], time.Now().UTC()) {
			result.Stream.ActiveCount++
		}
		if stream.GameID == "speed-racing" && stream.Position == position && stream.PlanKey == key {
			selected = stream
		}
	}
	result.Stream.ID, result.Stream.ActiveUntil = selected.ID, selected.ActiveUntil
	result.Stream.Active = selected.ID > 0 && planStreamActive(selected, cycles[selected.CycleID], time.Now().UTC())
	result.Stream.ActivationRequired = !result.Stream.Active
	if selected.Revoked {
		return result, nil
	}
	result.CurrentIssue, err = s.currentOpenStreamIssue(workspaceID)
	if err != nil {
		return result, err
	}
	if selected.ID > 0 {
		var periods []plan.StreamPeriod
		if err := s.db.Where("stream_id = ?", selected.ID).Order("id DESC").Limit(result.HistoryLimit).Find(&periods).Error; err != nil {
			return result, err
		}
		cycleIDs := []uint64{}
		for _, period := range periods {
			cycleIDs = append(cycleIDs, period.CycleID)
		}
		if len(cycleIDs) > 0 {
			var cycleRows []plan.StreamCycle
			if err := s.db.Where("id IN ?", cycleIDs).Find(&cycleRows).Error; err != nil {
				return result, err
			}
			for _, cycle := range cycleRows {
				cycles[cycle.ID] = cycle
			}
		}
		for i, period := range periods {
			cycle := cycles[period.CycleID]
			var picks []PlanRecommendationView
			if err := json.Unmarshal([]byte(cycle.PayloadJSON), &picks); err != nil {
				return result, err
			}
			for expertIndex, pick := range picks {
				pick.ID, pick.WorkspaceID, pick.GameID, pick.Issue = period.ID*10+uint64(expertIndex+1), workspaceID, "speed-racing", period.Issue
				pick.Position, pick.PlanKey, pick.Kind = position, key, option.Kind
				pick.CycleID, pick.CyclePeriod, pick.CyclePeriods, pick.CycleStartIssue, pick.CycleStatus = cycle.ID, period.PeriodIndex, cycle.Periods, cycle.StartIssue, cycle.Status
				pick.CreatedAt, pick.UpdatedAt = period.CreatedAt, period.CreatedAt
				result.History = append(result.History, pick)
				if i == 0 {
					result.LatestRecommendations = append(result.LatestRecommendations, pick)
				}
				if period.Issue == result.CurrentIssue {
					result.Recommendations = append(result.Recommendations, pick)
				}
			}
		}
	}
	// Legacy content remains in its existing administration endpoint. Do not
	// reread/transfer 300 unrelated records on every stream polling request.
	return result, nil
}

func (s *PlanContentService) currentOpenStreamIssue(workspaceID uint64) (string, error) {
	current, err := s.currentOpenPlanIssue(workspaceID, "speed-racing")
	if err != nil || current == "" {
		return current, err
	}
	var game lottery.Game
	if err := s.db.Select("id", "next_issue").First(&game, "id = ?", "speed-racing").Error; err != nil {
		return "", err
	}
	if game.NextIssue != current {
		return "", nil
	}
	return current, nil
}

func (s *PlanContentService) ActivateStream(ctx context.Context, workspaceID uint64, position int, key string, historyLimits ...int) (PlanStreamDetail, error) {
	if _, ok := planOption(key); !ok || position < 1 || position > 10 {
		return PlanStreamDetail{}, apperrors.NewBusinessError("INVALID_REQUEST", "推荐位置或方案不正确")
	}
	if _, err := s.touchPlan(ctx, workspaceID, "speed-racing", position, key); err != nil {
		return PlanStreamDetail{}, err
	}
	return s.StreamDetail(workspaceID, position, key, historyLimits...)
}

func planCyclePicks(workspaceID uint64, position int, option PlanOption, startIssue string) []PlanRecommendationView {
	result := make([]PlanRecommendationView, 0, 3)
	for _, master := range planDemoMasters {
		pick := PlanRecommendationView{MasterName: master.Name, MasterTitle: master.Title, MasterColor: master.Color, Numbers: []int{}, Source: "demo", Result: "pending", Note: PlanDemoNotice, Enabled: true, SortOrder: master.SortOrder}
		seed := fmt.Sprintf("plan-stream-v1\x00%d\x00%d\x00%s\x00%s\x00%s", workspaceID, position, option.Key, startIssue, master.Key)
		digest := sha256.Sum256([]byte(seed))
		switch option.Kind {
		case "numbers":
			pool := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
			for i := 0; i < option.NumberCount; i++ {
				d := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d", seed, i)))
				j := i + int(binary.BigEndian.Uint64(d[:8])%uint64(10-i))
				pool[i], pool[j] = pool[j], pool[i]
			}
			pick.Numbers = append([]int{}, pool[:option.NumberCount]...)
			sort.Ints(pick.Numbers)
		case "size":
			pick.Size = []string{"大", "小"}[digest[0]%2]
		case "parity":
			pick.Parity = []string{"单", "双"}[digest[0]%2]
		case "dragon_tiger":
			pick.DragonTiger = []string{"龙", "虎"}[digest[0]%2]
		}
		result = append(result, pick)
	}
	return result
}

func advancePlanStream(tx *gorm.DB, workspaceID uint64, game lottery.Game, issue lottery.Issue, window *lottery.IssueWindow, selected plan.Stream) (int64, error) {
	var cycle plan.StreamCycle
	if selected.CycleID > 0 {
		if err := tx.First(&cycle, selected.CycleID).Error; err != nil {
			return 0, err
		}
	}
	var created int64
	// This one-element scope deliberately cannot advance any unvisited stream.
	for _, stream := range []plan.Stream{selected} {
		var exists int64
		if err := tx.Model(&plan.StreamPeriod{}).Where("stream_id = ? AND issue_id = ?", stream.ID, issue.ID).Count(&exists).Error; err != nil {
			return 0, err
		}
		if exists > 0 {
			continue
		}
		if cycle.ID > 0 && (issue.ID <= cycle.LastIssueID || !issue.ScheduledDrawAt.After(cycle.LastScheduledAt)) {
			continue
		}
		if cycle.ID > 0 && cycle.Status == "active" {
			var missed int64
			if err := tx.Model(&lottery.Issue{}).Where("game_id = ? AND scheduled_draw_at > ? AND scheduled_draw_at < ?", game.ID, cycle.LastScheduledAt, *issue.ScheduledDrawAt).Count(&missed).Error; err != nil {
				return 0, err
			}
			// A schedule gap is conservative evidence of missed periods, never
			// permission to synthesize their identifiers or backfill picks.
			if missed > 0 || issue.ScheduledDrawAt.Sub(cycle.LastScheduledAt) > time.Duration(effectiveDrawInterval(&game))*time.Second*3/2 {
				if err := tx.Model(&cycle).Update("status", "interrupted").Error; err != nil {
					return 0, err
				}
				cycle.Status = "interrupted"
			}
		}
		if cycle.ID == 0 || cycle.Status != "active" {
			if !planStreamActive(stream, cycle, time.Now().UTC()) {
				continue
			}
			option, _ := planOption(stream.PlanKey)
			payload, _ := json.Marshal(planCyclePicks(workspaceID, stream.Position, option, issue.Issue))
			cycle = plan.StreamCycle{StreamID: stream.ID, Periods: option.Periods, Status: "active", StartIssue: issue.Issue, PayloadJSON: string(payload)}
			if err := tx.Create(&cycle).Error; err != nil {
				return 0, err
			}
			if err := tx.Model(&stream).Update("cycle_id", cycle.ID).Error; err != nil {
				return 0, err
			}
		}
		// Final DB-clock gate after all lock waits. No period is created at or
		// after either room/global cutoff, even when the transaction began earlier.
		insert := tx.Exec(`INSERT INTO plan_stream_periods(stream_id,issue_id,issue,cycle_id,period_index,scheduled_draw_at,created_at)
		 SELECT ?,?,?,?,?,?,clock_timestamp() WHERE clock_timestamp() >= ? AND clock_timestamp() < ? AND clock_timestamp() < ?
		 AND NOT EXISTS(SELECT 1 FROM lottery_draws WHERE game_id = ? AND issue = ?) ON CONFLICT DO NOTHING`,
			stream.ID, issue.ID, issue.Issue, cycle.ID, cycle.PublishedPeriods+1, *issue.ScheduledDrawAt, window.AcceptAt, window.SealAt, issue.SealAt, game.ID, issue.Issue)
		if insert.Error != nil {
			return 0, insert.Error
		}
		if insert.RowsAffected == 0 {
			// Roll back an unpublishable new cycle as well, so it can never seed a
			// later issue using a closed period's identity.
			return 0, errPlanStreamCutoff
		}
		cycle.PublishedPeriods++
		if cycle.PublishedPeriods >= cycle.Periods {
			cycle.Status = "completed"
		}
		if err := tx.Model(&cycle).Updates(map[string]any{"published_periods": cycle.PublishedPeriods, "status": cycle.Status, "last_issue_id": issue.ID, "last_scheduled_at": *issue.ScheduledDrawAt}).Error; err != nil {
			return 0, err
		}
		created += 3 // Public count remains expert recommendations, not SQL rows.
	}
	return created, nil
}

var errPlanStreamCutoff = errors.New("当前期已封盘，请等待下一期")

// Revocation is durable, not only a filter: restoring a broader allow-list
// must not resurrect past subscriptions and bypass the room's active cap.
func revokeDisallowedPlanStreams(tx *gorm.DB, workspaceID uint64, config PlanAutomationView) error {
	var streams []plan.Stream
	if err := tx.Where("workspace_id = ? AND revoked = false", workspaceID).Find(&streams).Error; err != nil {
		return err
	}
	for _, stream := range streams {
		if planRequestedStreamAllowed(config, stream.GameID, stream.Position, stream.PlanKey) {
			continue
		}
		if err := tx.Model(&stream).Updates(map[string]any{"revoked": true, "active_until": nil}).Error; err != nil {
			return err
		}
		if stream.CycleID > 0 {
			if err := tx.Model(&plan.StreamCycle{}).Where("id = ? AND status = ?", stream.CycleID, "active").Update("status", "interrupted").Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *PlanContentService) appendStreamCatalog(workspaceID uint64, result []PlanGameSummary) ([]PlanGameSummary, error) {
	config, err := NewPlanAutomationService(s.db).Get(workspaceID)
	if err != nil {
		return nil, err
	}
	// A permitted game must remain discoverable before its first member visit.
	if config.Enabled {
		for _, gameID := range config.GameIDs {
			if gameID == "speed-racing" {
				continue
			}
			available, err := planStreamRoomAvailable(s.db, workspaceID, gameID)
			if err != nil {
				return nil, err
			}
			if !available {
				continue
			}
			found := false
			for _, item := range result {
				found = found || item.GameID == gameID
			}
			if !found {
				result = append(result, PlanGameSummary{GameID: gameID, HistoryOnly: true})
			}
		}
	}
	available, err := planStreamRoomAvailable(s.db, workspaceID)
	if err != nil {
		return nil, err
	}
	if !available || !config.Enabled || len(config.Positions) == 0 || len(config.PlanKeys) == 0 {
		return result, nil
	}
	gameAllowed := false
	for _, id := range config.GameIDs {
		gameAllowed = gameAllowed || id == "speed-racing"
	}
	if !gameAllowed {
		return result, nil
	}
	var latest plan.StreamPeriod
	query := s.db.Model(&plan.StreamPeriod{}).Select("plan_stream_periods.*").
		Joins("JOIN plan_streams AS stream ON stream.id = plan_stream_periods.stream_id").
		Where("stream.workspace_id = ? AND stream.game_id = ? AND stream.revoked = false AND stream.position IN ? AND stream.plan_key IN ?", workspaceID, "speed-racing", config.Positions, config.PlanKeys).
		Order("plan_stream_periods.id DESC").First(&latest)
	if query.Error != nil && !errors.Is(query.Error, gorm.ErrRecordNotFound) {
		return nil, query.Error
	}
	current, err := s.currentOpenStreamIssue(workspaceID)
	if err != nil {
		return nil, err
	}
	item := PlanGameSummary{GameID: "speed-racing", CurrentIssue: current, LatestIssue: latest.Issue, HistoryOnly: latest.Issue == "" || latest.Issue != current, UpdatedAt: latest.CreatedAt}
	if latest.ID > 0 {
		item.MasterCount = 3
	}
	for i, old := range result {
		if old.GameID == "speed-racing" {
			result[i] = item
			return result, nil
		}
	}
	return append(result, item), nil
}
