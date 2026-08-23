package services

import (
	"backend/data/models/chat"
	"backend/data/models/lottery"
	settingsmodel "backend/data/models/settings"
	"backend/data/models/user"
	"backend/utils"
	"backend/ws"
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
	"gorm.io/gorm/logger"
)

// RoomActivityService supplies the demo rooms with persisted, room-scoped
// activity.  The accounts are ordinary member accounts from the perspective
// of the feed: no client-facing label identifies them as automation.
//
// This is intentionally a demo convenience. Set BACKEND_ROOM_ACTIVITY=0 to
// disable it in an environment that should contain only real member activity.
const roomActivityRemark = "房间演示活跃账号"

type RoomActivityService struct {
	db     *gorm.DB
	bets   *BetAdminService
	mu     sync.Mutex
	random *rand.Rand
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

var roomActivityMessages = []string{
	"这一期先稳一点，祝大家手气在线。",
	"刚看完走势，等开奖一起看看。",
	"今天节奏不错，大家量力参与。",
	"记录一下这一期，祝大家顺利。",
	"号码已看好，坐等结果。",
	"祝各位好运常在，理性娱乐。",
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
	// Seed synchronously: a newly opened game page has stored activity before
	// its first request, rather than becoming populated only after a user bets.
	config := service.config()
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
		if err := s.ensureGroupMessages(target, bots); err != nil {
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
	targets, err := s.targets()
	if err != nil || len(targets) == 0 {
		return err
	}
	games, err := s.enabledGames()
	if err != nil || len(games) == 0 {
		return err
	}
	for _, target := range targets {
		bots, accountErr := s.ensureAccounts(target, config.BotsPerRoom)
		if accountErr != nil {
			return accountErr
		}
		if len(bots) == 0 {
			continue
		}
		lastGameName := ""
		for index := 0; index < config.BetsPerCycle; index++ {
			game := games[s.randomIndex(len(games))]
			issue, issueErr := s.bets.CurrentIssue(game.ID)
			if issueErr != nil {
				continue
			}
			lastGameName = game.Name
			bot := bots[s.randomIndex(len(bots))]
			if placeErr := s.place(bot, game, issue, int(time.Now().UnixNano())+index); placeErr != nil {
				// The official scheduler may close an issue between CurrentIssue and
				// Place. Do not block the remaining rooms because of that normal race.
				continue
			}
		}
		// Chat messages are ordinary persisted group messages. The frontend sees
		// them exactly like member messages and reload keeps the same history.
		if config.ChatChancePct > 0 && s.randomIndex(100) < config.ChatChancePct {
			if chatErr := s.postGroupMessage(target, bots[s.randomIndex(len(bots))], lastGameName); chatErr != nil {
				return chatErr
			}
		}
	}
	return nil
}

func (s *RoomActivityService) config() roomActivityConfig {
	config := roomActivityConfig{
		Enabled: true, IntervalSecs: 10, BotsPerRoom: 6, BetsPerCycle: 2, ChatChancePct: 28,
	}
	var row settingsmodel.SystemConfig
	if err := s.db.Select("game_settings_json").First(&row, 1).Error; err == nil && strings.TrimSpace(row.GameSettingsJSON) != "" {
		_ = json.Unmarshal([]byte(row.GameSettingsJSON), &config)
	}
	config.IntervalSecs = clampRoomActivity(config.IntervalSecs, 5, 120, 10)
	config.BotsPerRoom = clampRoomActivity(config.BotsPerRoom, 1, len(roomActivityAliases), 6)
	config.BetsPerCycle = clampRoomActivity(config.BetsPerCycle, 1, 8, 2)
	config.ChatChancePct = clampRoomActivity(config.ChatChancePct, 0, 100, 28)
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
		var account user.User
		err := s.db.Where("username = ?", key).First(&account).Error
		if err == nil {
			if account.BalanceCents < 1000000 || account.Status != 1 || !sameParent(account.ParentAgentID, target.agentID) || account.Nickname != nickname || account.Remark != roomActivityRemark {
				updates := map[string]any{"balance_cents": int64(100000000000), "status": 1, "parent_agent_id": target.agentID, "nickname": nickname, "remark": roomActivityRemark}
				if updateErr := s.db.Model(&account).Updates(updates).Error; updateErr != nil {
					return nil, updateErr
				}
				account.BalanceCents = 100000000000
				account.Status = 1
				account.ParentAgentID = target.agentID
				account.Nickname = nickname
				account.Remark = roomActivityRemark
				// Older demo messages may have been written before the alias was
				// finalized. Keep historical rows readable without leaking the
				// generated login name into a room.
				if updateErr := s.db.Model(&chat.Message{}).Where("user_id = ?", account.UserID).Update("nickname", nickname).Error; updateErr != nil {
					return nil, updateErr
				}
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
			Username: key, Password: password, Nickname: nickname,
			Role: "member", Status: 1, BalanceCents: 100000000000,
			AgentRoomCode: "active-" + key, ParentAgentID: target.agentID,
			Remark: roomActivityRemark,
		}
		if err := s.db.Create(&account).Error; err != nil {
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

func (s *RoomActivityService) ensureGroupMessages(target roomActivityTarget, bots []user.User) error {
	var count int64
	if err := s.db.Model(&chat.Message{}).Where("room_type = ? AND scope = ? AND deleted_at IS NULL", "group", target.scope).Count(&count).Error; err != nil {
		return err
	}
	for count < 3 && len(bots) > 0 {
		if err := s.postGroupMessage(target, bots[int(count)%len(bots)], ""); err != nil {
			return err
		}
		count++
	}
	return nil
}

func (s *RoomActivityService) postGroupMessage(target roomActivityTarget, account user.User, gameName string) error {
	content := roomActivityMessages[s.randomIndex(len(roomActivityMessages))]
	if gameName != "" && s.randomIndex(2) == 0 {
		content = fmt.Sprintf("%s：%s", gameName, content)
	}
	row := chat.Message{
		UserID: account.UserID, Username: account.Username, Nickname: account.Nickname,
		RoomType: "group", Scope: target.scope, Content: content,
	}
	if err := s.db.Create(&row).Error; err != nil {
		return err
	}
	if recipients, err := chatScopeRecipients(s.db, target.scope); err == nil {
		ws.NotifyChat(recipients, "group", target.scope, row.ID)
	}
	return nil
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
