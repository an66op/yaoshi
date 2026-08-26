package services

import (
	"backend/data/models/chat"
	"backend/data/models/lottery"
	settingsmodel "backend/data/models/settings"
	"backend/data/models/user"
	"backend/utils"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// RoomActivityService supplies rooms with persisted, room-scoped
// activity.  The accounts are ordinary member accounts from the perspective
// of the feed: no client-facing label identifies them as automation.
//
// Set BACKEND_ROOM_ACTIVITY=0 to
// disable it in an environment that should contain only real member activity.
const roomActivityRemark = "房间活跃账号"

type RoomActivityService struct {
	db       *gorm.DB
	bets     *BetAdminService
	mu       sync.Mutex
	cycleMu  sync.Mutex
	statusMu sync.RWMutex
	random   *rand.Rand
	status   RoomActivityStatus
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
	LastRunAt     time.Time `json:"last_run_at,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
}

var roomActivityRegistry struct {
	sync.RWMutex
	service *RoomActivityService
}

type roomActivityConfig struct {
	Enabled       bool `json:"room_activity_enabled"`
	IntervalSecs  int  `json:"room_activity_interval_secs"`
	BotsPerRoom   int  `json:"room_activity_bots_per_room"`
	BetsPerCycle  int  `json:"room_activity_bets_per_cycle"`
	ChatChancePct int  `json:"room_activity_chat_chance_percent"`
}

type roomActivityTarget struct {
	scope   string
	agentID *uint64
}

var roomActivityAliases = []string{
	"轻快的云朵", "银翼探索者", "星河旅人", "好运收藏家", "清风与海", "幸运信号", "远山来信", "夏日微光",
	"月光捕手", "暖风经过", "青柠汽水", "漫游小队", "蓝鲸跃迁", "晴天预报", "星轨玩家", "橙色闪电",
}

func StartRoomActivity(ctxDone <-chan struct{}, db *gorm.DB) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("BACKEND_ROOM_ACTIVITY")), "0") {
		log.Printf("房间活跃数据已通过 BACKEND_ROOM_ACTIVITY=0 关闭")
		return
	}
	// Missing demo rows are expected during warmup. Keep those normal probes out
	// of the server log, while the primary application DB retains its logger.
	activityDB := db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	service := &RoomActivityService{
		db: activityDB, bets: NewBetAdminService(activityDB), random: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	roomActivityRegistry.Lock()
	roomActivityRegistry.service = service
	roomActivityRegistry.Unlock()
	// Seed synchronously: a newly opened game page has stored activity before
	// its first request, rather than becoming populated only after a user bets.
	config := service.config()
	service.setRuntimeConfig(config, true)
	if config.Enabled {
		if err := service.warmup(config); err != nil {
			log.Printf("预置房间活跃数据失败: %v", err)
		}
	} else {
		log.Printf("房间自动活跃当前已在后台设置中关闭")
	}
	go service.run(ctxDone)
}

func (s *RoomActivityService) run(ctxDone <-chan struct{}) {
	for {
		config := s.config()
		s.setRuntimeConfig(config, true)
		wait := time.Duration(config.IntervalSecs) * time.Second
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
			if config.Enabled {
				if err := s.runCycle(config); err != nil {
					log.Printf("房间活跃数据写入失败: %v", err)
				}
			}
		case <-ctxDone:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			s.statusMu.Lock()
			s.status.Running = false
			s.statusMu.Unlock()
			return
		}
	}
}

func (s *RoomActivityService) warmup(config roomActivityConfig) error {
	targets, err := s.targets()
	if err != nil {
		return err
	}
	games, err := s.enabledGames()
	if err != nil {
		return err
	}
	for _, target := range targets {
		bots, err := s.ensureAccounts(target, config.BotsPerRoom)
		if err != nil {
			return err
		}
		for gameIndex, game := range games {
			issue, issueErr := s.bets.CurrentIssue(game.ID)
			if issueErr != nil {
				continue
			}
			// Three independently stored entries make the initial feed feel alive,
			// including when no real member has placed a bet yet.
			for slot := 0; slot < len(bots); slot++ {
				if err := s.place(bots[slot], game, issue, gameIndex+slot); err != nil {
					// A just-closed issue is normal while the simulated draw loop rolls.
					continue
				}
			}
		}
	}
	return nil
}

func (s *RoomActivityService) runCycle(config roomActivityConfig) error {
	s.cycleMu.Lock()
	defer s.cycleMu.Unlock()
	started := time.Now().UTC()
	betsPlaced, chatsPosted, botAccounts := 0, 0, 0
	targets, err := s.targets()
	if err != nil || len(targets) == 0 {
		if err == nil {
			err = fmt.Errorf("没有可用房间")
		}
		s.recordActivityRun(config, started, len(targets), 0, botAccounts, betsPlaced, chatsPosted, err)
		return err
	}
	games, err := s.enabledGames()
	if err != nil || len(games) == 0 {
		if err == nil {
			err = fmt.Errorf("没有已启用的彩种")
		}
		s.recordActivityRun(config, started, len(targets), len(games), botAccounts, betsPlaced, chatsPosted, err)
		return err
	}
	for _, target := range targets {
		bots, accountErr := s.ensureAccounts(target, config.BotsPerRoom)
		if accountErr != nil {
			s.recordActivityRun(config, started, len(targets), len(games), botAccounts, betsPlaced, chatsPosted, accountErr)
			return accountErr
		}
		botAccounts += len(bots)
		if len(bots) == 0 {
			continue
		}
		for index := 0; index < config.BetsPerCycle; index++ {
			game := games[s.randomIndex(len(games))]
			issue, issueErr := s.bets.CurrentIssue(game.ID)
			if issueErr != nil {
				continue
			}
			bot := bots[s.randomIndex(len(bots))]
			if placeErr := s.place(bot, game, issue, int(time.Now().UnixNano())+index); placeErr != nil {
				// The official scheduler may close an issue between CurrentIssue and
				// Place. Do not block the remaining rooms because of that normal race.
				continue
			}
			betsPlaced++
		}
	}
	s.recordActivityRun(config, started, len(targets), len(games), botAccounts, betsPlaced, chatsPosted, nil)
	return nil
}

func (s *RoomActivityService) setRuntimeConfig(config roomActivityConfig, running bool) {
	s.statusMu.Lock()
	s.status.Running = running
	s.status.Enabled = config.Enabled
	s.status.IntervalSecs = config.IntervalSecs
	s.status.BotsPerRoom = config.BotsPerRoom
	s.status.BetsPerCycle = config.BetsPerCycle
	s.status.ChatChancePct = config.ChatChancePct
	s.statusMu.Unlock()
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
}

func (s *RoomActivityService) Status() RoomActivityStatus {
	config := s.config()
	s.setRuntimeConfig(config, true)
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	return s.status
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
	config := service.config()
	err := service.runCycle(config)
	return service.Status(), err
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
	config := service.config()
	target := roomActivityTarget{scope: "agent:" + strconv.FormatUint(agentID, 10), agentID: &agentID}
	service.cycleMu.Lock()
	defer service.cycleMu.Unlock()
	bots, err := service.ensureAccounts(target, config.BotsPerRoom)
	if err != nil {
		return service.Status(), err
	}
	games, err := service.enabledGames()
	if err != nil || len(games) == 0 {
		if err == nil {
			err = fmt.Errorf("没有已启用的彩种")
		}
		return service.Status(), err
	}
	betsPlaced, chatsPosted := 0, 0
	for index := 0; index < config.BetsPerCycle; index++ {
		game := games[service.randomIndex(len(games))]
		issue, issueErr := service.bets.CurrentIssue(game.ID)
		if issueErr != nil {
			continue
		}
		if err := service.place(bots[service.randomIndex(len(bots))], game, issue, int(time.Now().UnixNano())+index); err == nil {
			betsPlaced++
		}
	}
	service.recordActivityRun(config, time.Now().UTC(), 1, len(games), len(bots), betsPlaced, chatsPosted, nil)
	return service.Status(), nil
}

func (s *RoomActivityService) config() roomActivityConfig {
	config := roomActivityConfig{
		Enabled: true, IntervalSecs: 10, BotsPerRoom: 6, BetsPerCycle: 2, ChatChancePct: 0,
	}
	var row settingsmodel.SystemConfig
	if err := s.db.Select("game_settings_json").First(&row, 1).Error; err == nil && strings.TrimSpace(row.GameSettingsJSON) != "" {
		_ = json.Unmarshal([]byte(row.GameSettingsJSON), &config)
	}
	config.IntervalSecs = clampRoomActivity(config.IntervalSecs, 5, 120, 10)
	config.BotsPerRoom = clampRoomActivity(config.BotsPerRoom, 1, len(roomActivityAliases), 6)
	config.BetsPerCycle = clampRoomActivity(config.BetsPerCycle, 1, 8, 2)
	// Room activity is represented by persisted bets only. Ignore older saved
	// chat probability values so synthetic chatter cannot return after restart.
	config.ChatChancePct = 0
	return config
}

func clampRoomActivity(value, minimum, maximum, fallback int) int {
	if value < minimum || value > maximum {
		return fallback
	}
	return value
}

func (s *RoomActivityService) targets() ([]roomActivityTarget, error) {
	targets := []roomActivityTarget{{scope: "lobby"}}
	var agents []user.User
	if err := s.db.Select("user_id").Where("role = ? AND status = ?", "agent", 1).Find(&agents).Error; err != nil {
		return nil, err
	}
	for _, agent := range agents {
		id := agent.UserID
		targets = append(targets, roomActivityTarget{scope: "agent:" + strconv.FormatUint(id, 10), agentID: &id})
	}
	return targets, nil
}

func (s *RoomActivityService) enabledGames() ([]lottery.Game, error) {
	var games []lottery.Game
	err := s.db.Where("enabled = ?", true).Order("sort_order asc, id asc").Find(&games).Error
	return games, err
}

func (s *RoomActivityService) ensureAccounts(target roomActivityTarget, count int) ([]user.User, error) {
	bots := make([]user.User, 0, count)
	for slot := 0; slot < count; slot++ {
		key := fmt.Sprintf("room_activity_%08x_%d", crc32.ChecksumIEEE([]byte(target.scope)), slot+1)
		nickname := roomActivityAliases[(int(crc32.ChecksumIEEE([]byte(target.scope)))+slot)%len(roomActivityAliases)]
		loginScope := platformLoginScope
		if target.agentID != nil {
			loginScope = agentLoginScope(*target.agentID)
		}
		var account user.User
		err := s.db.Where("login_scope = ? AND username = ?", loginScope, key).First(&account).Error
		if err == nil {
			if txErr := s.db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, account.UserID).Error; err != nil {
					return err
				}
				updates := map[string]any{"status": 1, "login_scope": loginScope, "parent_agent_id": target.agentID, "nickname": nickname, "remark": roomActivityRemark}
				if err := tx.Model(&account).Updates(updates).Error; err != nil {
					return err
				}
				account.Status = 1
				account.ParentAgentID = target.agentID
				account.Nickname = nickname
				account.Remark = roomActivityRemark
				if err := ensureSeededBalance(tx, &account, 1000000, 100000000000, "房间活跃账户"); err != nil {
					return err
				}
				// Older messages may have been written before the alias was
				// finalized. Keep historical rows readable without leaking the
				// generated login name into a room.
				return tx.Model(&chat.Message{}).Where("user_id = ?", account.UserID).Update("nickname", nickname).Error
			}); txErr != nil {
				return nil, txErr
			}
			bots = append(bots, account)
			continue
		}
		if err != gorm.ErrRecordNotFound {
			return nil, err
		}
		password, hashErr := utils.HashPassword("room-activity-" + key)
		if hashErr != nil {
			return nil, hashErr
		}
		account = user.User{
			Username: key, LoginScope: loginScope, Password: password, Nickname: nickname,
			Role: "member", Status: 1, BalanceCents: 100000000000,
			AgentRoomCode: "active-" + key, ParentAgentID: target.agentID,
			Remark: roomActivityRemark,
		}
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&account).Error; err != nil {
				return err
			}
			return ensureSeededBalance(tx, &account, 1000000, 100000000000, "房间活跃账户")
		}); err != nil {
			return nil, err
		}
		bots = append(bots, account)
	}
	return bots, nil
}

func (s *RoomActivityService) place(account user.User, game lottery.Game, issue string, salt int) error {
	position := s.randomIndex(5) + 1
	selection := strconv.Itoa((s.randomIndex(10) + salt) % 10)
	amounts := []float64{12.5, 18.8, 24.75, 33.33, 50}
	_, err := s.bets.Place(PlaceBetInput{
		GameID: game.ID, Issue: issue, UserID: account.UserID,
		PlayCode: "ball_1_5", PlayName: "1-5球号", Position: position,
		Selection: selection, Amount: amounts[s.randomIndex(len(amounts))], Odds: 9.9,
		Remark: "房间实时动态", Operator: "房间活动",
	})
	return err
}

func (s *RoomActivityService) randomIndex(limit int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.random.Intn(limit)
}

func sameParent(left, right *uint64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
