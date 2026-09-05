package services

import (
	"backend/cluster"
	"backend/data/models/lottery"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// RoomActivityService supplies rooms with persisted, room-scoped
// activity.  The accounts are ordinary member accounts from the perspective
// of the feed: no client-facing label identifies them as automation.
//
// Release mode is fail-closed: BACKEND_ROOM_ACTIVITY must be exactly 1 and a
// positive BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES cap must be present before the
// scheduler starts. Debug/test keep the legacy opt-out behaviour so local
// fixtures continue to work unless BACKEND_ROOM_ACTIVITY=0 is set.
const roomActivityRemark = "房间活跃账号"

const maxProductionRoomActivityWorkspaces = 100

var ErrRoomActivityWorkspaceCap = errors.New("机器人生产启用工作区超过上限")

// This PostgreSQL advisory transaction lock serializes production robot
// setting changes with an actual robot run. It closes the small window where a
// room could be disabled (or the global cap exceeded) between validation and
// placing the first bet.
const roomActivityPolicyLockID int64 = 0x57414e475a484552

type RoomActivityService struct {
	db       *gorm.DB
	mu       sync.Mutex
	cycleMu  sync.Mutex
	statusMu sync.RWMutex
	random   *rand.Rand
	// Zero means uncapped and is only used by debug/test compatibility mode.
	maxEnabledWorkspaces int
	status               RoomActivityStatus
}

type RoomActivityStatus struct {
	Running       bool      `json:"running"`
	Enabled       bool      `json:"enabled"`
	IntervalSecs  int       `json:"interval_secs"`
	BotsPerRoom   int       `json:"bots_per_room"`
	BetsPerCycle  int       `json:"bets_per_cycle"`
	ChatChancePct int       `json:"chat_chance_percent"`
	TargetRooms   int       `json:"target_rooms"`
	EnabledGames  int       `json:"enabled_games"`
	BotAccounts   int       `json:"bot_accounts"`
	Cycles        int64     `json:"cycles"`
	BetsPlaced    int64     `json:"bets_placed"`
	ChatsPosted   int64     `json:"chats_posted"`
	PausedReason  string    `json:"paused_reason,omitempty"`
	LastRunAt     time.Time `json:"last_run_at,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
}

var roomActivityRegistry struct {
	sync.RWMutex
	service              *RoomActivityService
	maxEnabledWorkspaces int
}

type roomActivityConfig struct {
	Enabled       bool `json:"room_activity_enabled"`
	IntervalSecs  int  `json:"room_activity_interval_secs"`
	BotsPerRoom   int  `json:"room_activity_bots_per_room"`
	BetsPerCycle  int  `json:"room_activity_bets_per_cycle"`
	ChatChancePct int  `json:"room_activity_chat_chance_percent"`
}

type roomActivityTarget struct {
	workspaceID uint64
	scope       string
}

var roomActivityAliases = []string{
	"轻快的云朵", "银翼探索者", "星河旅人", "好运收藏家", "清风与海", "幸运信号", "远山来信", "夏日微光",
	"月光捕手", "暖风经过", "青柠汽水", "漫游小队", "蓝鲸跃迁", "晴天预报", "星轨玩家", "橙色闪电",
}

// StartRoomActivityForMode receives the already validated server mode from the
// application instead of inferring it from a second ambient configuration path.
func StartRoomActivityForMode(ctx context.Context, db *gorm.DB, serverMode string) {
	if ctx == nil {
		ctx = context.Background()
	}
	policy := resolveRoomActivityProcessPolicy(
		serverMode,
		os.Getenv("BACKEND_ROOM_ACTIVITY"),
		os.Getenv("BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES"),
	)
	roomActivityRegistry.Lock()
	roomActivityRegistry.service = nil
	roomActivityRegistry.maxEnabledWorkspaces = policy.maxWorkspaces
	roomActivityRegistry.Unlock()
	if !policy.enabled {
		log.Printf("房间活跃调度未启用：%s", policy.reason)
		return
	}
	if serverMode == "release" {
		// This line is retained by journald and records the process-level side of
		// activation. Workspace setting changes are separately covered by the
		// privileged API audit middleware.
		log.Printf("房间活跃调度已通过生产双门禁启用：工作区上限=%d", policy.maxWorkspaces)
	}
	// Missing demo rows are expected during warmup. Keep those normal probes out
	// of the server log, while the primary application DB retains its logger.
	activityDB := db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	service := &RoomActivityService{
		db: activityDB, random: rand.New(rand.NewSource(time.Now().UnixNano())),
		maxEnabledWorkspaces: policy.maxWorkspaces,
	}
	roomActivityRegistry.Lock()
	roomActivityRegistry.service = service
	roomActivityRegistry.maxEnabledWorkspaces = policy.maxWorkspaces
	roomActivityRegistry.Unlock()
	service.statusMu.Lock()
	service.status.Running = true
	service.statusMu.Unlock()
	go service.run(ctx)
}

type roomActivityProcessPolicy struct {
	enabled       bool
	maxWorkspaces int
	reason        string
}

func resolveRoomActivityProcessPolicy(mode, enabledValue, maxWorkspacesValue string) roomActivityProcessPolicy {
	enabledValue = strings.TrimSpace(enabledValue)
	maxWorkspacesValue = strings.TrimSpace(maxWorkspacesValue)
	if mode != "release" {
		if enabledValue == "0" {
			return roomActivityProcessPolicy{reason: "BACKEND_ROOM_ACTIVITY=0"}
		}
		maximum, _ := strconv.Atoi(maxWorkspacesValue)
		if maximum < 1 || maximum > maxProductionRoomActivityWorkspaces {
			maximum = 0
		}
		return roomActivityProcessPolicy{enabled: true, maxWorkspaces: maximum}
	}

	maximum, maximumErr := strconv.Atoi(maxWorkspacesValue)
	validMaximum := maximumErr == nil && strconv.Itoa(maximum) == maxWorkspacesValue && maximum >= 0 && maximum <= maxProductionRoomActivityWorkspaces
	if !validMaximum {
		maximum = 0
	}
	if enabledValue != "1" {
		return roomActivityProcessPolicy{maxWorkspaces: maximum, reason: "release 模式要求显式设置 BACKEND_ROOM_ACTIVITY=1"}
	}
	if !validMaximum || maximum < 1 {
		return roomActivityProcessPolicy{reason: fmt.Sprintf(
			"BACKEND_ROOM_ACTIVITY_MAX_WORKSPACES 必须在 1-%d 之间",
			maxProductionRoomActivityWorkspaces,
		)}
	}
	return roomActivityProcessPolicy{enabled: true, maxWorkspaces: maximum}
}

func (s *RoomActivityService) run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_, err := cluster.RunWithLease(ctx, "scheduler:room-activity", 5*time.Minute, s.runDueWorkspaces)
			if err != nil {
				log.Printf("机器人调度失败: %v", err)
			}
		case <-ctx.Done():
			s.statusMu.Lock()
			s.status.Running = false
			s.statusMu.Unlock()
			return
		}
	}
}

func (s *RoomActivityService) runDueWorkspaces(ctx context.Context) error {
	workDB := s.db.WithContext(ctx)
	if err := s.validateEnabledWorkspaceCapWithDB(workDB); err != nil {
		return err
	}
	var settings []workspacemodel.RobotSetting
	if err := workDB.Where("enabled = ?", true).Order("workspace_id ASC").Find(&settings).Error; err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, setting := range settings {
		if err := ctx.Err(); err != nil {
			return err
		}
		interval := clampRoomActivity(setting.IntervalSecs, 5, 3600, 30)
		if setting.LastRunAt != nil && now.Sub(*setting.LastRunAt) < time.Duration(interval)*time.Second {
			continue
		}
		if err := s.runWorkspaceWithContext(ctx, setting); err != nil {
			log.Printf("工作区 %d 机器人执行失败: %v", setting.WorkspaceID, err)
		}
	}
	return nil
}

func (s *RoomActivityService) validateEnabledWorkspaceCapWithDB(db *gorm.DB) error {
	if s.maxEnabledWorkspaces <= 0 {
		return nil
	}
	var enabledCount int64
	if err := db.Model(&workspacemodel.RobotSetting{}).Where("enabled = ?", true).Count(&enabledCount).Error; err != nil {
		return err
	}
	return validateRoomActivityWorkspaceCount(enabledCount, s.maxEnabledWorkspaces)
}

func validateRoomActivityWorkspaceCount(enabledCount int64, maximum int) error {
	if maximum > 0 && enabledCount > int64(maximum) {
		return fmt.Errorf(
			"%w：启用数量 %d，安全上限 %d，调度已暂停",
			ErrRoomActivityWorkspaceCap,
			enabledCount,
			maximum,
		)
	}
	return nil
}

func IsRoomActivityWorkspaceCapError(err error) bool {
	return errors.Is(err, ErrRoomActivityWorkspaceCap)
}

func (s *RoomActivityService) runWorkspace(setting workspacemodel.RobotSetting) error {
	return s.runWorkspaceWithContext(context.Background(), setting)
}

func (s *RoomActivityService) runWorkspaceWithContext(ctx context.Context, setting workspacemodel.RobotSetting) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.cycleMu.Lock()
	defer s.cycleMu.Unlock()
	workDB := s.db.WithContext(ctx)
	return withRoomActivityPolicyLock(workDB, func(tx *gorm.DB) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		freshSetting, err := RobotSettingForWorkspace(tx, setting.WorkspaceID)
		if err != nil {
			return err
		}
		return s.runWorkspaceWithPolicyLock(ctx, tx, NewBetAdminService(tx), freshSetting)
	})
}

func (s *RoomActivityService) runWorkspaceWithPolicyLock(ctx context.Context, db *gorm.DB, bets *BetAdminService, setting workspacemodel.RobotSetting) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.validateEnabledWorkspaceCapWithDB(db); err != nil {
		return err
	}
	if s.maxEnabledWorkspaces > 0 && !setting.Enabled {
		return fmt.Errorf("工作区 %d 未通过生产机器人启用门禁", setting.WorkspaceID)
	}
	started := time.Now().UTC()
	var workspace workspacemodel.Workspace
	if err := db.Where("id = ? AND status = ?", setting.WorkspaceID, 1).First(&workspace).Error; err != nil {
		return s.finishWorkspaceRun(ctx, db, setting, started, 0, 0, 0, err)
	}
	target := roomActivityTarget{workspaceID: workspace.ID, scope: workspace.Scope}
	bots, err := s.ensureAccountsWithDB(db, target, 0)
	if err != nil {
		return s.finishWorkspaceRun(ctx, db, setting, started, 0, 0, 0, err)
	}
	bots = activeRobotAccounts(bots, time.Now())
	if len(bots) == 0 {
		return s.finishWorkspaceRun(ctx, db, setting, started, 0, 0, 0, fmt.Errorf("没有已启用的机器人"))
	}
	todayBets, pendingBets, err := s.robotBetCountsWithDB(db, setting.WorkspaceID, started)
	if err != nil {
		return s.finishWorkspaceRun(ctx, db, setting, started, len(bots), 0, 0, err)
	}
	allowance, pauseReason := robotRunAllowance(setting, todayBets, pendingBets)
	if pauseReason != "" {
		return s.finishWorkspacePause(ctx, db, setting, started, len(bots), pauseReason)
	}
	games, err := s.enabledGamesWithDB(db, setting.WorkspaceID)
	if err != nil || len(games) == 0 {
		if err == nil {
			err = fmt.Errorf("没有已启用的彩种")
		}
		return s.finishWorkspaceRun(ctx, db, setting, started, len(bots), len(games), 0, err)
	}
	betsPlaced := 0
	count := clampRoomActivity(setting.BetsPerCycle, 1, 20, 1)
	if count > allowance {
		count = allowance
	}
	for index := 0; index < count; index++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		bot := bots[s.randomIndex(len(bots))]
		available := allowedRobotGames(bot, games)
		if len(available) == 0 {
			continue
		}
		game := available[s.randomIndex(len(available))]
		issue, issueErr := bets.CurrentIssue(game.ID)
		if issueErr != nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if s.placeWithService(bets, bot, game, issue, int(time.Now().UnixNano())+index) == nil {
			betsPlaced++
		}
	}
	return s.finishWorkspaceRun(ctx, db, setting, started, len(bots), len(games), betsPlaced, nil)
}

func (s *RoomActivityService) finishWorkspaceRun(ctx context.Context, db *gorm.DB, setting workspacemodel.RobotSetting, at time.Time, bots, games, bets int, runErr error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	updates := map[string]any{"last_run_at": at, "last_error": "", "pause_reason": ""}
	if runErr != nil {
		updates["last_error"] = truncateRunMessage(runErr.Error(), 500)
	}
	_ = db.Model(&workspacemodel.RobotSetting{}).Where("workspace_id = ?", setting.WorkspaceID).Updates(updates).Error
	config := roomActivityConfig{Enabled: setting.Enabled, IntervalSecs: setting.IntervalSecs, BetsPerCycle: setting.BetsPerCycle}
	s.recordActivityRun(config, at, 1, games, bots, bets, 0, runErr)
	return runErr
}

func (s *RoomActivityService) finishWorkspacePause(ctx context.Context, db *gorm.DB, setting workspacemodel.RobotSetting, at time.Time, bots int, reason string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	reason = truncateRunMessage(reason, 240)
	_ = db.Model(&workspacemodel.RobotSetting{}).Where("workspace_id = ?", setting.WorkspaceID).Updates(map[string]any{
		"last_run_at": at, "last_error": "", "pause_reason": reason,
	}).Error
	config := roomActivityConfig{Enabled: setting.Enabled, IntervalSecs: setting.IntervalSecs, BetsPerCycle: setting.BetsPerCycle}
	s.recordActivityRun(config, at, 1, 0, bots, 0, 0, nil)
	s.statusMu.Lock()
	s.status.PausedReason = reason
	s.statusMu.Unlock()
	return nil
}

func truncateRunMessage(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}

func (s *RoomActivityService) recordActivityRun(config roomActivityConfig, at time.Time, rooms, games, bots, bets, chats int, runErr error) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	s.status.Running = true
	s.status.Enabled = config.Enabled
	s.status.IntervalSecs = config.IntervalSecs
	s.status.BotsPerRoom = config.BotsPerRoom
	s.status.BetsPerCycle = config.BetsPerCycle
	s.status.ChatChancePct = config.ChatChancePct
	s.status.TargetRooms = rooms
	s.status.EnabledGames = games
	s.status.BotAccounts = bots
	s.status.Cycles++
	s.status.BetsPlaced += int64(bets)
	s.status.ChatsPosted += int64(chats)
	s.status.LastRunAt = at
	if runErr != nil {
		s.status.LastError = runErr.Error()
	} else {
		s.status.LastError = ""
	}
	if runErr != nil || bets > 0 {
		s.status.PausedReason = ""
	}
}

func (s *RoomActivityService) Status() RoomActivityStatus {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	return s.status
}

type UpdateRobotSettingInput struct {
	Enabled        *bool `json:"enabled"`
	IntervalSecs   *int  `json:"interval_secs"`
	BetsPerCycle   *int  `json:"bets_per_cycle"`
	DailyBetLimit  *int  `json:"daily_bet_limit"`
	MaxPendingBets *int  `json:"max_pending_bets"`
}

func RobotSettingForWorkspace(db *gorm.DB, workspaceID uint64) (workspacemodel.RobotSetting, error) {
	var result workspacemodel.RobotSetting
	if workspaceID == 0 {
		return result, fmt.Errorf("工作区不存在")
	}
	err := db.First(&result, "workspace_id = ?", workspaceID).Error
	if err != nil {
		return result, err
	}
	if result.DailyBetLimit <= 0 {
		result.DailyBetLimit = 200
	}
	if result.MaxPendingBets <= 0 {
		result.MaxPendingBets = 50
	}
	var workspace workspacemodel.Workspace
	if err := db.Select("id", "robot_quota").First(&workspace, workspaceID).Error; err != nil {
		return result, err
	}
	result.RobotQuota = workspace.RobotQuota
	service := RoomActivityService{db: db}
	result.TodayBets, result.PendingBets, err = service.robotBetCounts(workspaceID, time.Now().UTC())
	return result, err
}

func UpdateRobotSettingForWorkspace(db *gorm.DB, workspaceID uint64, input UpdateRobotSettingInput) (workspacemodel.RobotSetting, error) {
	var result workspacemodel.RobotSetting
	err := withRoomActivityPolicyLock(db, func(tx *gorm.DB) error {
		current, err := RobotSettingForWorkspace(tx, workspaceID)
		if err != nil {
			return err
		}
		updates := map[string]any{}
		if input.Enabled != nil {
			if *input.Enabled && current.RobotQuota == 0 {
				return apperrors.NewBusinessError("ROBOT_QUOTA_REQUIRED", "上级尚未分配机器人名额")
			}
			updates["enabled"] = *input.Enabled
			if *input.Enabled {
				updates["last_run_at"] = nil
				updates["last_error"] = ""
				updates["pause_reason"] = ""
			}
		}
		if input.IntervalSecs != nil {
			updates["interval_secs"] = clampRoomActivity(*input.IntervalSecs, 30, 3600, 60)
		}
		if input.BetsPerCycle != nil {
			updates["bets_per_cycle"] = clampRoomActivity(*input.BetsPerCycle, 1, 20, 1)
		}
		if input.DailyBetLimit != nil {
			updates["daily_bet_limit"] = clampRoomActivity(*input.DailyBetLimit, 1, 10000, 200)
		}
		if input.MaxPendingBets != nil {
			updates["max_pending_bets"] = clampRoomActivity(*input.MaxPendingBets, 1, 5000, 50)
		}
		if len(updates) > 0 {
			if err := tx.Model(&current).Updates(updates).Error; err != nil {
				return err
			}
		}
		if maximum := activeRoomActivityWorkspaceCap(); maximum > 0 {
			var enabledCount int64
			if err := tx.Model(&workspacemodel.RobotSetting{}).Where("enabled = ?", true).Count(&enabledCount).Error; err != nil {
				return err
			}
			if err := validateRoomActivityWorkspaceCount(enabledCount, maximum); err != nil {
				return err
			}
		}
		result, err = RobotSettingForWorkspace(tx, workspaceID)
		return err
	})
	return result, err
}

func activeRoomActivityWorkspaceCap() int {
	roomActivityRegistry.RLock()
	defer roomActivityRegistry.RUnlock()
	return roomActivityRegistry.maxEnabledWorkspaces
}

func withRoomActivityPolicyLock(db *gorm.DB, operation func(*gorm.DB) error) error {
	if db.Dialector.Name() != "postgres" {
		return operation(db)
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", roomActivityPolicyLockID).Error; err != nil {
			return fmt.Errorf("获取机器人生产策略锁失败: %w", err)
		}
		return operation(tx)
	})
}

func (s *RoomActivityService) robotBetCounts(workspaceID uint64, now time.Time) (int64, int64, error) {
	return s.robotBetCountsWithDB(s.db, workspaceID, now)
}

func (s *RoomActivityService) robotBetCountsWithDB(db *gorm.DB, workspaceID uint64, now time.Time) (int64, int64, error) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	localNow := now.In(location)
	dayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location).UTC()
	robotUsers := db.Table("workspace_robot_profiles").Select("user_id").Where("workspace_id = ? AND enabled = ?", workspaceID, true)
	base := db.Table("lottery_bets").Where("workspace_id = ? AND user_id IN (?)", workspaceID, robotUsers)
	var today, pending int64
	if err := base.Session(&gorm.Session{}).Where("created_at >= ?", dayStart).Count(&today).Error; err != nil {
		return 0, 0, err
	}
	if err := base.Session(&gorm.Session{}).Where("status = ?", "pending").Count(&pending).Error; err != nil {
		return 0, 0, err
	}
	return today, pending, nil
}

func robotRunAllowance(setting workspacemodel.RobotSetting, todayBets, pendingBets int64) (int, string) {
	dailyLimit := setting.DailyBetLimit
	if dailyLimit <= 0 {
		dailyLimit = 200
	}
	pendingLimit := setting.MaxPendingBets
	if pendingLimit <= 0 {
		pendingLimit = 50
	}
	if todayBets >= int64(dailyLimit) {
		return 0, fmt.Sprintf("已达到今日自动下注上限（%d 笔）", dailyLimit)
	}
	if pendingBets >= int64(pendingLimit) {
		return 0, fmt.Sprintf("待结算机器人注单已达到保护上限（%d 笔）", pendingLimit)
	}
	dailyRemaining := dailyLimit - int(todayBets)
	pendingRemaining := pendingLimit - int(pendingBets)
	if dailyRemaining < pendingRemaining {
		return dailyRemaining, ""
	}
	return pendingRemaining, ""
}

func RoomActivityStatusSnapshot() RoomActivityStatus {
	roomActivityRegistry.RLock()
	service := roomActivityRegistry.service
	roomActivityRegistry.RUnlock()
	if service == nil {
		return RoomActivityStatus{LastError: "房间自动活跃服务尚未启动"}
	}
	return service.Status()
}

func RunRoomActivityOnce() (RoomActivityStatus, error) {
	roomActivityRegistry.RLock()
	service := roomActivityRegistry.service
	roomActivityRegistry.RUnlock()
	if service == nil {
		return RoomActivityStatus{}, fmt.Errorf("房间自动活跃服务尚未启动")
	}
	var settings []workspacemodel.RobotSetting
	if err := service.db.Where("enabled = ?", true).Order("workspace_id ASC").Find(&settings).Error; err != nil {
		return service.Status(), err
	}
	for _, setting := range settings {
		if err := service.runWorkspace(setting); err != nil {
			return service.Status(), err
		}
	}
	return service.Status(), nil
}

// RunRoomActivityOnceForAgent executes only the authenticated room. It is used
// by the agent workbench and never accepts a browser supplied room scope.
func RunRoomActivityOnceForAgent(agentID uint64) (RoomActivityStatus, error) {
	roomActivityRegistry.RLock()
	service := roomActivityRegistry.service
	roomActivityRegistry.RUnlock()
	if service == nil {
		return RoomActivityStatus{}, fmt.Errorf("房间自动活跃服务尚未启动")
	}
	if agentID == 0 {
		return RoomActivityStatus{}, fmt.Errorf("房间代理编号不正确")
	}
	var workspace workspacemodel.Workspace
	if err := service.db.Where("owner_user_id = ? AND type = ?", agentID, workspacemodel.TypeAgent).First(&workspace).Error; err != nil {
		return service.Status(), fmt.Errorf("当前代理工作区不存在")
	}
	return RunRoomActivityOnceForWorkspace(workspace.ID)
}

func RunRoomActivityOnceForWorkspace(workspaceID uint64) (RoomActivityStatus, error) {
	roomActivityRegistry.RLock()
	service := roomActivityRegistry.service
	roomActivityRegistry.RUnlock()
	if service == nil {
		return RoomActivityStatus{}, fmt.Errorf("房间自动活跃服务尚未启动")
	}
	if workspaceID == 0 {
		return service.Status(), fmt.Errorf("房间工作区不正确")
	}
	setting, err := RobotSettingForWorkspace(service.db, workspaceID)
	if err != nil {
		return service.Status(), err
	}
	if err := service.runWorkspace(setting); err != nil {
		return service.Status(), err
	}
	return service.Status(), nil
}

func clampRoomActivity(value, minimum, maximum, fallback int) int {
	if value < minimum || value > maximum {
		return fallback
	}
	return value
}

func (s *RoomActivityService) enabledGamesWithDB(db *gorm.DB, workspaceID uint64) ([]lottery.Game, error) {
	var games []lottery.Game
	err := workspaceEnabledGamesQuery(db, workspaceID).Order("sort_order asc, id asc").Find(&games).Error
	return games, err
}

func allowedRobotGames(account user.User, games []lottery.Game) []lottery.Game {
	configured := []string{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(account.RobotGameIDsJSON)), &configured); err != nil || len(configured) == 0 {
		return games
	}
	allowed := make(map[string]struct{}, len(configured))
	for _, gameID := range configured {
		allowed[gameID] = struct{}{}
	}
	result := make([]lottery.Game, 0, len(configured))
	for _, game := range games {
		if _, ok := allowed[game.ID]; ok {
			result = append(result, game)
		}
	}
	return result
}

func activeRobotAccounts(accounts []user.User, now time.Time) []user.User {
	result := make([]user.User, 0, len(accounts))
	for _, account := range accounts {
		if robotAccountActiveAt(account, now) {
			result = append(result, account)
		}
	}
	return result
}

func robotAccountActiveAt(account user.User, now time.Time) bool {
	startText := strings.TrimSpace(account.RobotActiveStart)
	endText := strings.TrimSpace(account.RobotActiveEnd)
	if startText == "" || endText == "" {
		return true
	}
	start, startErr := time.Parse("15:04", startText)
	end, endErr := time.Parse("15:04", endText)
	if startErr != nil || endErr != nil {
		return true
	}
	startMinute := start.Hour()*60 + start.Minute()
	endMinute := end.Hour()*60 + end.Minute()
	nowMinute := now.Hour()*60 + now.Minute()
	if startMinute <= endMinute {
		return nowMinute >= startMinute && nowMinute <= endMinute
	}
	return nowMinute >= startMinute || nowMinute <= endMinute
}

func (s *RoomActivityService) ensureAccountsWithDB(db *gorm.DB, target roomActivityTarget, count int) ([]user.User, error) {
	// Robot identities are provisioned once and remain independent. Runtime
	// execution only consumes profiles already owned by this workspace; it never
	// clones accounts merely because another room exists.
	var bots []user.User
	var workspace workspacemodel.Workspace
	if err := db.Select("id", "robot_quota").First(&workspace, target.workspaceID).Error; err != nil {
		return nil, err
	}
	if workspace.RobotQuota == 0 {
		return bots, nil
	}
	query := db.Model(&user.User{}).
		Select(`"user".*`).
		Joins("JOIN workspace_robot_profiles AS profile ON profile.user_id = \"user\".user_id").
		Where(`"user".workspace_id = ? AND "user".status = ? AND profile.workspace_id = ? AND profile.enabled = ?`, target.workspaceID, 1, target.workspaceID, true).
		Where("profile.id IN (?)", allocatedRobotProfileIDs(db, target.workspaceID, workspace.RobotQuota)).
		Order(`"user".user_id ASC`)
	if count > 0 {
		query = query.Limit(count)
	}
	if err := query.Find(&bots).Error; err != nil {
		return nil, err
	}
	return bots, nil
}

func (s *RoomActivityService) placeWithService(bets *BetAdminService, account user.User, game lottery.Game, issue string, salt int) error {
	position := s.randomIndex(5) + 1
	selection := strconv.Itoa((s.randomIndex(10) + salt) % 10)
	amounts := []float64{12.5, 18.8, 24.75, 33.33, 50}
	amount := amounts[s.randomIndex(len(amounts))]
	if account.RobotMinBetCents > 0 && account.RobotMaxBetCents >= account.RobotMinBetCents {
		amount = float64(account.RobotMinBetCents+s.randomInt64(account.RobotMaxBetCents-account.RobotMinBetCents+1)) / 100
	}
	noFly := 0.0
	_, err := bets.Place(PlaceBetInput{
		GameID: game.ID, Issue: issue, UserID: account.UserID,
		PlayCode: "ball_1_5", PlayName: "1-5球号", Position: position,
		Selection: selection, Amount: amount, Odds: 9.9, FlyAmount: &noFly,
		Remark: "房间实时动态", Operator: "房间活动",
	})
	return err
}

func (s *RoomActivityService) randomIndex(limit int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.random.Intn(limit)
}

func (s *RoomActivityService) randomInt64(limit int64) int64 {
	if limit <= 1 {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.random.Int63n(limit)
}
