package services

import (
	"backend/data/models/lottery"
	"backend/data/models/user"
	"backend/utils"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

// TestBotService creates a clearly-labelled, local test-only betting stream.
// It never uses a member's balance: every wager belongs to a dedicated bot
// account created by this service. The worker is opt-in at startup through
// BACKEND_TEST_BOTS=1, or may be started by an authenticated administrator.
type TestBotService struct {
	db   *gorm.DB
	bets *BetAdminService

	mu       sync.Mutex
	cancel   chan struct{}
	running  bool
	interval time.Duration
	cycles   int64
	lastRun  time.Time
	lastErr  string
	random   *rand.Rand
}

type TestBotStatus struct {
	Enabled      bool      `json:"enabled"`
	IntervalSecs int       `json:"interval_secs"`
	Cycles       int64     `json:"cycles"`
	LastRunAt    time.Time `json:"last_run_at,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
	Bots         []string  `json:"bots"`
	TestOnly     bool      `json:"test_only"`
}

var testBotAccounts = []struct {
	Username string
	Nickname string
}{
	{Username: "testbot_drift", Nickname: "疾风测试员"},
	{Username: "testbot_starlight", Nickname: "星轨测试员"},
	{Username: "testbot_vortex", Nickname: "旋涡测试员"},
}

func NewTestBotService(db *gorm.DB) *TestBotService {
	return &TestBotService{
		db: db, bets: NewBetAdminService(db), interval: 12 * time.Second,
		random: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (s *TestBotService) Status() TestBotStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

func (s *TestBotService) Start(interval time.Duration) (TestBotStatus, error) {
	if interval <= 0 {
		interval = 12 * time.Second
	}
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	if interval > 2*time.Minute {
		interval = 2 * time.Minute
	}
	if err := s.ensureAccounts(); err != nil {
		return s.Status(), err
	}

	s.mu.Lock()
	if s.running {
		status := s.statusLocked()
		s.mu.Unlock()
		return status, nil
	}
	stop := make(chan struct{})
	s.cancel = stop
	s.running = true
	s.interval = interval
	s.lastErr = ""
	s.mu.Unlock()

	// The first round happens immediately so a tester can see the live stream.
	go s.runCycle()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.runCycle()
			case <-stop:
				return
			}
		}
	}()
	return s.Status(), nil
}

func (s *TestBotService) Stop() TestBotStatus {
	s.mu.Lock()
	stop := s.cancel
	s.cancel = nil
	s.running = false
	s.mu.Unlock()
	if stop != nil {
		close(stop)
	}
	return s.Status()
}

// RunOnce is useful while verifying a page. It works when the repeating worker
// is stopped and still uses only generated test accounts.
func (s *TestBotService) RunOnce() (TestBotStatus, error) {
	if err := s.runCycle(); err != nil {
		return s.Status(), err
	}
	return s.Status(), nil
}

func (s *TestBotService) runCycle() error {
	if err := s.ensureAccounts(); err != nil {
		s.recordRun(err)
		return err
	}
	var games []lottery.Game
	if err := s.db.Where("enabled = ?", true).Order("sort_order asc").Find(&games).Error; err != nil {
		s.recordRun(err)
		return err
	}
	if len(games) == 0 {
		err := fmt.Errorf("没有已启用的彩种，测试机器人未下注")
		s.recordRun(err)
		return err
	}

	var bots []user.User
	if err := s.db.Where("username IN ?", testBotUsernames()).Order("username asc").Find(&bots).Error; err != nil {
		s.recordRun(err)
		return err
	}
	if len(bots) == 0 {
		err := fmt.Errorf("测试机器人账号未就绪")
		s.recordRun(err)
		return err
	}

	s.mu.Lock()
	game := games[int(s.cycles)%len(games)]
	s.mu.Unlock()
	issue, err := s.bets.CurrentIssue(game.ID)
	if err != nil {
		s.recordRun(err)
		return err
	}

	for index, bot := range bots {
		position, selection, amount := s.randomBet(index)
		_, placeErr := s.bets.Place(PlaceBetInput{
			GameID: game.ID, Issue: issue, UserID: bot.UserID,
			PlayCode: "ball_1_5", PlayName: "测试号码", Position: position,
			Selection: selection, Amount: amount, Odds: 9.9,
			Remark: "测试机器人自动投注", Operator: "测试机器人",
		})
		if placeErr != nil {
			s.recordRun(placeErr)
			return placeErr
		}
	}
	s.recordRun(nil)
	return nil
}

func (s *TestBotService) randomBet(index int) (int, string, float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	position := s.random.Intn(5) + 1
	// Offset each bot so their bets do not repeatedly merge into one record.
	selection := fmt.Sprintf("%d", (s.random.Intn(10)+index)%10)
	amounts := []float64{10, 20, 30, 50}
	return position, selection, amounts[s.random.Intn(len(amounts))]
}

func (s *TestBotService) recordRun(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastRun = time.Now().UTC()
	if err == nil {
		s.cycles++
		s.lastErr = ""
		return
	}
	s.lastErr = err.Error()
}

func (s *TestBotService) ensureAccounts() error {
	for _, bot := range testBotAccounts {
		var count int64
		if err := s.db.Model(&user.User{}).Where("username = ?", bot.Username).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		hash, err := utils.HashPassword(fmt.Sprintf("local-test-bot-%s", bot.Username))
		if err != nil {
			return err
		}
		if err := s.db.Create(&user.User{
			Username: bot.Username, Password: hash, Nickname: bot.Nickname,
			Role: "member", Status: 1, BalanceCents: 100000000,
			// The existing schema keeps this column unique even for members;
			// do not leave all generated bot rows with the same empty value.
			AgentRoomCode: "test-" + bot.Username,
			Remark:        "测试机器人专用账号；仅用于本地测试自动投注",
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *TestBotService) statusLocked() TestBotStatus {
	return TestBotStatus{
		Enabled: s.running, IntervalSecs: int(s.interval.Seconds()), Cycles: s.cycles,
		LastRunAt: s.lastRun, LastError: s.lastErr, Bots: testBotUsernames(), TestOnly: true,
	}
}

func testBotUsernames() []string {
	users := make([]string, 0, len(testBotAccounts))
	for _, bot := range testBotAccounts {
		users = append(users, bot.Username)
	}
	return users
}

// StartTestBotsFromEnvironment keeps automatic execution opt-in.
func StartTestBotsFromEnvironment(service *TestBotService) {
	if service == nil || !strings.EqualFold(strings.TrimSpace(os.Getenv("BACKEND_TEST_BOTS")), "1") {
		return
	}
	_, _ = service.Start(12 * time.Second)
}
