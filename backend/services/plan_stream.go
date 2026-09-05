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

var racingPlanGameIDs = []string{
	"speed-racing", "speed-fly", "sg-fly", "fly-racing", "au-lucky-10",
	"bingo-racing-a", "bingo-racing-b",
}

func racingPlanGameID(gameID string) bool {
	for _, candidate := range racingPlanGameIDs {
		if gameID == candidate {
			return true
		}
	}
	return false
}

// IsRacingPlanGame is the shared HTTP/service routing contract for every
// verified ten-position racing-v2 product.
func IsRacingPlanGame(gameID string) bool { return racingPlanGameID(gameID) }

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
	return planStreamAllowedForGame(config, "speed-racing", position, key)
}

func planStreamAllowedForGame(config PlanAutomationView, gameID string, position int, key string) bool {
	return config.Enabled && planStreamConfiguredForGame(config, gameID, position, key)
}

// Configured means the room owner still exposes this selection in the member
// catalogue. It deliberately ignores the generation switch: switching
// automation off must stop future publications without hiding already
// published, auditable history.
func planStreamConfiguredForGame(config PlanAutomationView, gameID string, position int, key string) bool {
	game, p, k := false, false, false
	for _, id := range config.GameIDs {
		game = game || id == gameID
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
	return s.StreamDetailForGame(workspaceID, "speed-racing", position, key, historyLimits...)
}

func (s *PlanContentService) StreamDetailForGame(workspaceID uint64, gameID string, position int, key string, historyLimits ...int) (PlanStreamDetail, error) {
	option, ok := planOption(key)
	if !racingPlanGameID(gameID) || !ok || position < 1 || position > 10 {
		return PlanStreamDetail{}, apperrors.NewBusinessError("INVALID_REQUEST", "推荐位置或方案不正确")
	}
	if err := s.ensureScope(workspaceID, gameID); err != nil {
		return PlanStreamDetail{}, err
	}
	result := PlanStreamDetail{PlanDetail: PlanDetail{GameID: gameID, Recommendations: []PlanRecommendationView{}, History: []PlanRecommendationView{}, LatestRecommendations: []PlanRecommendationView{}},
		Options: append([]PlanOption{}, racingPlanOptions...), Positions: planPositions(), AllowedPositions: []int{}, AllowedPlanKeys: []string{},
		Selection: PlanStreamSelection{position, key, option.Kind, option.Periods, option.NumberCount}, Stream: PlanStreamState{MaxActive: MaxActivePlanStreams}, LegacyHistory: []PlanRecommendationView{}, Notice: PlanDemoNotice}
	result.GenerationMode, result.HistoryLimit, result.RefreshSeconds = "on_visit", planHistoryLimit(historyLimits), 15
	config, err := NewPlanAutomationService(s.db).Get(workspaceID)
	if err != nil {
		return result, err
	}
	roomOpen, err := planStreamRoomAvailable(s.db, workspaceID, gameID)
	if err != nil {
		return result, err
	}
	if roomOpen {
		for _, game := range config.GameIDs {
			if game == gameID {
				result.AllowedPositions, result.AllowedPlanKeys = config.Positions, config.PlanKeys
			}
		}
	}
	result.Stream.Allowed = roomOpen && planStreamConfiguredForGame(config, gameID, position, key)
	result.AutomationEnabled = config.Enabled && result.Stream.Allowed
	// Removed selections never expose cached payload. Disabling generation alone
	// preserves the configured matrix and its already published history.
	if !result.Stream.Allowed {
		return result, nil
	}
	streams, cycles, err := readPlanStreams(s.db, workspaceID)
	if err != nil {
		return result, err
	}
	var selected plan.Stream
	for _, stream := range streams {
		if planRequestedStreamAllowed(config, stream.GameID, stream.Position, stream.PlanKey) && planStreamActive(stream, cycles[stream.CycleID], time.Now().UTC()) {
			result.Stream.ActiveCount++
		}
		if stream.GameID == gameID && stream.Position == position && stream.PlanKey == key {
			selected = stream
		}
	}
	result.Stream.ID, result.Stream.ActiveUntil = selected.ID, selected.ActiveUntil
	result.Stream.Active = selected.ID > 0 && planStreamActive(selected, cycles[selected.CycleID], time.Now().UTC())
	result.Stream.ActivationRequired = !result.Stream.Active
	result.CurrentIssue, err = s.currentOpenStreamIssueForGame(workspaceID, gameID)
	if err != nil {
		return result, err
	}
	if selected.ID > 0 {
		var periods []plan.StreamPeriod
		if err := s.db.Where("stream_id = ?", selected.ID).Order("id DESC").Limit(retainedPlanPeriods).Find(&periods).Error; err != nil {
			return result, err
		}
		// One bounded read for the selected stream's displayed periods. Results
		// are derived from immutable draws, never from publication progress or
		// an editable hit-rate field, and GET never rewrites the saved picks.
		issues := make([]string, 0, len(periods))
		for _, period := range periods {
			issues = append(issues, period.Issue)
		}
		draws := map[string]lottery.Draw{}
		if len(issues) > 0 {
			var rows []lottery.Draw
			if err := trustedDrawsForGame(s.db, gameID).Where("issue IN ?", issues).Find(&rows).Error; err != nil {
				return result, err
			}
			for _, row := range rows {
				draws[row.Issue] = row
			}
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
		allRows := make([]PlanRecommendationView, 0, len(periods)*len(planDemoMasters))
		for _, period := range periods {
			cycle := cycles[period.CycleID]
			var picks []PlanRecommendationView
			if err := json.Unmarshal([]byte(cycle.PayloadJSON), &picks); err != nil {
				return result, err
			}
			for expertIndex, pick := range picks {
				pick.ID, pick.WorkspaceID, pick.GameID, pick.Issue = period.ID*10+uint64(expertIndex+1), workspaceID, gameID, period.Issue
				pick.Position, pick.PlanKey, pick.Kind = position, key, option.Kind
				pick.CycleID, pick.CyclePeriod, pick.CyclePeriods, pick.CycleStartIssue, pick.CycleStatus = cycle.ID, period.PeriodIndex, cycle.Periods, cycle.StartIssue, cycle.Status
				pick.CreatedAt, pick.UpdatedAt = period.CreatedAt, period.CreatedAt
				pick.Result, pick.DrawNumbers, pick.DrawAt = racingPlanDrawResult(pick, period, draws[period.Issue], time.Now().UTC())
				allRows = append(allRows, pick)
			}
		}
		type score struct{ hits, settled int }
		scores := map[string]score{}
		for _, pick := range allRows {
			identity := planMasterStatisticKey(pick.GameID, pick.Source, pick.MasterName)
			value := scores[identity]
			if pick.Result == plan.ResultHit {
				value.hits++
				value.settled++
			} else if pick.Result == plan.ResultMiss {
				value.settled++
			}
			scores[identity] = value
		}
		visibleRows := result.HistoryLimit * len(planDemoMasters)
		for index, pick := range allRows {
			value := scores[planMasterStatisticKey(pick.GameID, pick.Source, pick.MasterName)]
			pick.MasterSampleCount = value.settled
			if value.settled > 0 {
				rate := float64(value.hits) * 100 / float64(value.settled)
				pick.MasterHitRate = &rate
			}
			if index < visibleRows {
				result.History = append(result.History, pick)
			}
			if index < len(planDemoMasters) {
				result.LatestRecommendations = append(result.LatestRecommendations, pick)
			}
			if pick.Issue == result.CurrentIssue {
				result.Recommendations = append(result.Recommendations, pick)
			}
		}
	}
	// Legacy content remains in its existing administration endpoint. Do not
	// reread/transfer 300 unrelated records on every stream polling request.
	return result, nil
}

func (s *PlanContentService) currentOpenStreamIssue(workspaceID uint64) (string, error) {
	return s.currentOpenStreamIssueForGame(workspaceID, "speed-racing")
}

func (s *PlanContentService) currentOpenStreamIssueForGame(workspaceID uint64, gameID string) (string, error) {
	current, err := s.currentOpenPlanIssue(workspaceID, gameID)
	if err != nil || current == "" {
		return current, err
	}
	var game lottery.Game
	if err := s.db.Select("id", "next_issue").First(&game, "id = ?", gameID).Error; err != nil {
		return "", err
	}
	if game.NextIssue != current {
		return "", nil
	}
	return current, nil
}

func (s *PlanContentService) ActivateStream(ctx context.Context, workspaceID uint64, position int, key string, historyLimits ...int) (PlanStreamDetail, error) {
	return s.activateStreamForMember(ctx, 0, workspaceID, "speed-racing", position, key, historyLimits...)
}

func (s *PlanContentService) ActivateStreamForMember(ctx context.Context, userID, workspaceID uint64, gameID string, position int, key string, historyLimits ...int) (PlanStreamDetail, error) {
	return s.activateStreamForMember(ctx, userID, workspaceID, gameID, position, key, historyLimits...)
}

func (s *PlanContentService) auditedStreamDetail(ctx context.Context, userID, workspaceID uint64, gameID string, position int, key string, historyLimits ...int) (PlanStreamDetail, error) {
	if userID == 0 {
		return s.StreamDetailForGame(workspaceID, gameID, position, key, historyLimits...)
	}
	var result PlanStreamDetail
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlanPublicationGame(tx, workspaceID, gameID); err != nil {
			return err
		}
		var err error
		result, err = NewPlanContentService(tx).StreamDetailForGame(workspaceID, gameID, position, key, historyLimits...)
		if err != nil {
			return err
		}
		return recordVisiblePlanPublicationViews(tx, userID, workspaceID, gameID, position, key, result.PlanDetail)
	})
	return result, err
}

func (s *PlanContentService) activateStreamForMember(ctx context.Context, userID, workspaceID uint64, gameID string, position int, key string, historyLimits ...int) (PlanStreamDetail, error) {
	if _, ok := planOption(key); !ok || position < 1 || position > 10 {
		return PlanStreamDetail{}, apperrors.NewBusinessError("INVALID_REQUEST", "推荐位置或方案不正确")
	}
	if !racingPlanGameID(gameID) {
		return PlanStreamDetail{}, apperrors.NewBusinessError("INVALID_REQUEST", "该彩种不支持名次计划")
	}
	if userID > 0 {
		snapshot, err := s.StreamDetailForGame(workspaceID, gameID, position, key, historyLimits...)
		if err != nil {
			return PlanStreamDetail{}, err
		}
		if !snapshot.Stream.Allowed {
			return snapshot, nil
		}
		if !snapshot.AutomationEnabled {
			var lockedSnapshot PlanStreamDetail
			becameEnabled := false
			if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if err := lockPlanPublicationGame(tx, workspaceID, gameID); err != nil {
					return err
				}
				var err error
				lockedSnapshot, err = NewPlanContentService(tx).StreamDetailForGame(workspaceID, gameID, position, key, historyLimits...)
				if err != nil {
					return err
				}
				if lockedSnapshot.AutomationEnabled {
					becameEnabled = true
					return nil
				}
				if !lockedSnapshot.Stream.Allowed {
					return nil
				}
				return recordVisiblePlanPublicationViews(tx, userID, workspaceID, gameID, position, key, lockedSnapshot.PlanDetail)
			}); err != nil {
				return PlanStreamDetail{}, err
			}
			if !becameEnabled {
				return lockedSnapshot, nil
			}
		}
	}
	if _, err := s.touchPlanForMember(ctx, userID, workspaceID, gameID, position, key); err != nil {
		return PlanStreamDetail{}, err
	}
	return s.auditedStreamDetail(ctx, userID, workspaceID, gameID, position, key, historyLimits...)
}

func planCyclePicks(workspaceID uint64, position int, option PlanOption, startIssue string) []PlanRecommendationView {
	return planCyclePicksForGame(workspaceID, "speed-racing", position, option, startIssue)
}

func planCyclePicksForGame(workspaceID uint64, gameID string, position int, option PlanOption, startIssue string) []PlanRecommendationView {
	result := make([]PlanRecommendationView, 0, 3)
	for _, master := range planDemoMasters {
		pick := PlanRecommendationView{MasterName: master.Name, MasterTitle: master.Title, MasterColor: master.Color, Numbers: []int{}, Source: "demo", Result: "pending", Note: PlanDemoNotice, Enabled: true, SortOrder: master.SortOrder}
		seed := fmt.Sprintf("plan-stream-v1\x00%d\x00%d\x00%s\x00%s\x00%s", workspaceID, position, option.Key, startIssue, master.Key)
		if gameID != "speed-racing" {
			seed = fmt.Sprintf("plan-stream-v2\x00%d\x00%s\x00%d\x00%s\x00%s\x00%s", workspaceID, gameID, position, option.Key, startIssue, master.Key)
		}
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
			payload, _ := json.Marshal(planCyclePicksForGame(workspaceID, game.ID, stream.Position, option, issue.Issue))
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
	for _, gameID := range config.GameIDs {
		available, err := planStreamRoomAvailable(s.db, workspaceID, gameID)
		if err != nil {
			return nil, err
		}
		if !available {
			continue
		}
		index := -1
		for i := range result {
			if result[i].GameID == gameID {
				index = i
				break
			}
		}
		if !racingPlanGameID(gameID) {
			// A permitted game remains discoverable before its first member visit.
			if config.Enabled && index < 0 {
				result = append(result, PlanGameSummary{GameID: gameID, HistoryOnly: true})
			}
			continue
		}
		if len(config.Positions) == 0 || len(config.PlanKeys) == 0 {
			continue
		}
		var latest plan.StreamPeriod
		query := s.db.Model(&plan.StreamPeriod{}).Select("plan_stream_periods.*").
			Joins("JOIN plan_streams AS stream ON stream.id = plan_stream_periods.stream_id").
			Where("stream.workspace_id = ? AND stream.game_id = ? AND stream.position IN ? AND stream.plan_key IN ?", workspaceID, gameID, config.Positions, config.PlanKeys)
		// Turning generation off revokes active leases, but it deliberately keeps
		// the configured matrix browseable. Removed/reclassified selections stay
		// hidden because they no longer match the saved game/position/key matrix.
		if config.Enabled {
			query = query.Where("stream.revoked = false")
		}
		query = query.Order("plan_stream_periods.id DESC").First(&latest)
		if query.Error != nil && !errors.Is(query.Error, gorm.ErrRecordNotFound) {
			return nil, query.Error
		}
		// Disabled automation exposes only a truthful history shelf. A configured
		// racing product with no persisted publication has nothing to display yet.
		if !config.Enabled && latest.ID == 0 {
			continue
		}
		current := ""
		if config.Enabled {
			current, err = s.currentOpenStreamIssueForGame(workspaceID, gameID)
			if err != nil {
				return nil, err
			}
		}
		item := PlanGameSummary{GameID: gameID, CurrentIssue: current, LatestIssue: latest.Issue, HistoryOnly: !config.Enabled || latest.Issue == "" || latest.Issue != current, UpdatedAt: latest.CreatedAt}
		if latest.ID > 0 {
			item.MasterCount = len(planDemoMasters)
		}
		if index >= 0 {
			result[index] = item
		} else {
			result = append(result, item)
		}
	}
	return result, nil
}
