package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type LotteryService struct{ db *gorm.DB }

type GameSummary struct {
	ID            string     `json:"id"`
	Code          string     `json:"code"`
	Name          string     `json:"name"`
	Category      string     `json:"category"`
	Badge         string     `json:"badge"`
	BadgeColor    string     `json:"badge_color"`
	Enabled       bool       `json:"enabled"`
	Issue         string     `json:"issue"`
	CurrentIssue  string     `json:"current_issue,omitempty"`
	BettorCount   int64      `json:"bettor_count,omitempty"`
	LatestNumbers []int      `json:"latest_numbers,omitempty"`
	NextDrawAt    time.Time  `json:"next_draw_at"`
	Turnover      float64    `json:"turnover"`
	Profit        float64    `json:"profit"`
	SourceKind    string     `json:"source_kind"`
	SourceName    string     `json:"source_name"`
	SourceURL     string     `json:"source_url"`
	SyncStatus    string     `json:"sync_status"`
	LastSyncAt    *time.Time `json:"last_sync_at"`
	LastSyncError string     `json:"last_sync_error"`
	ScheduleMode  string     `json:"schedule_mode"`
}

type DrawResult struct {
	ID      uint64    `json:"id"`
	GameID  string    `json:"game_id"`
	Issue   string    `json:"issue"`
	Numbers []int     `json:"numbers"`
	DrawAt  time.Time `json:"draw_at"`
}

func NewLotteryService(db *gorm.DB) *LotteryService { return &LotteryService{db: db} }

func (s *LotteryService) SyncTargetGames() (*SyncTargetResult, error) {
	return SyncTargetGames(s.db)
}

func (s *LotteryService) ListGames() ([]GameSummary, error) {
	var games []lottery.Game
	if err := s.db.Order("sort_order asc").Find(&games).Error; err != nil {
		return nil, err
	}
	result := make([]GameSummary, 0, len(games))
	for _, game := range games {
		var draw lottery.Draw
		err := s.db.Where("game_id = ?", game.ID).Order("draw_at desc").First(&draw).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return nil, err
		}
		result = append(result, GameSummary{
			ID: game.ID, Code: game.Code, Name: game.Name, Category: game.Category,
			Badge: game.Badge, BadgeColor: game.BadgeColor, Enabled: game.Enabled,
			Issue: draw.Issue, LatestNumbers: parseNumbers(draw.Numbers), NextDrawAt: game.NextDrawAt,
			SourceKind: game.SourceKind, SourceName: game.SourceName, SourceURL: game.SourceURL,
			SyncStatus: game.SyncStatus, LastSyncAt: game.LastSyncAt, LastSyncError: game.LastSyncError,
			ScheduleMode: func() string {
				if game.SourceKind == "official" {
					return "official-feed"
				}
				if game.SourceKind == "external" {
					return "external-feed"
				}
				return "interval"
			}(),
		})
	}
	return result, nil
}

// EnrichForLobby fills the open issue and distinct bettor count for lobby cards.
func (s *LotteryService) EnrichForLobby(games []GameSummary) ([]GameSummary, error) {
	if len(games) == 0 {
		return games, nil
	}
	betSvc := NewBetAdminService(s.db)
	for i := range games {
		issue, err := betSvc.CurrentIssue(games[i].ID)
		if err != nil {
			continue
		}
		games[i].CurrentIssue = issue
		var count int64
		if err := s.db.Model(&bet.Bet{}).Where(
			"game_id = ? AND issue = ? AND status IN ?",
			games[i].ID, issue, []string{"pending", "won", "lost"},
		).Select("COUNT(DISTINCT user_id)").Scan(&count).Error; err != nil {
			return nil, err
		}
		games[i].BettorCount = count
	}
	return games, nil
}

func (s *LotteryService) ListDraws(gameID string, limit int) ([]DrawResult, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var draws []lottery.Draw
	if err := s.db.Where("game_id = ?", gameID).Order("draw_at desc").Limit(limit).Find(&draws).Error; err != nil {
		return nil, err
	}
	result := make([]DrawResult, 0, len(draws))
	for _, draw := range draws {
		result = append(result, DrawResult{ID: draw.ID, GameID: draw.GameID, Issue: draw.Issue, Numbers: parseNumbers(draw.Numbers), DrawAt: draw.DrawAt})
	}
	return result, nil
}

func (s *LotteryService) SetEnabled(id string, enabled bool) (*GameSummary, error) {
	var game lottery.Game
	if err := s.db.First(&game, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&game).Update("enabled", enabled).Error; err != nil {
		return nil, err
	}
	games, err := s.ListGames()
	if err != nil {
		return nil, err
	}
	for _, item := range games {
		if item.ID == id {
			return &item, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func parseNumbers(value string) []int {
	parts := strings.Split(value, ",")
	result := make([]int, 0, len(parts))
	for _, item := range parts {
		if n, err := strconv.Atoi(strings.TrimSpace(item)); err == nil {
			result = append(result, n)
		}
	}
	return result
}

type seedGame struct {
	ID, Code, Name, Category, Badge, Color string
	Interval                               int
	SortOrder                              int
}

var defaultGames = []seedGame{
	// 宾果系列
	{"bingo-ssc-1", "BINGO_SSC_1", "宾果时时彩(一)", "时时彩", "时时彩", "orange", 300, 1},
	{"bingo-ssc-2", "BINGO_SSC_2", "宾果时时彩(二)", "时时彩", "时时彩", "orange", 300, 2},
	{"bingo-ssc-3", "BINGO_SSC_3", "宾果时时彩(三)", "时时彩", "时时彩", "orange", 300, 3},
	{"bingo-ssc-4", "BINGO_SSC_4", "宾果时时彩(四)", "时时彩", "时时彩", "orange", 300, 4},
	{"bingo-racing-a", "BINGO_RACING_A", "宾果赛车(A)", "赛车", "赛车", "red", 300, 5},
	{"bingo-racing-b", "BINGO_RACING_B", "宾果赛车(B)", "赛车", "赛车", "red", 300, 6},
	{"bingo-mark-six", "BINGO_MARK_SIX", "宾果六合彩", "六合彩", "六合彩", "blue", 600, 7},
	// 六合彩系列
	{"hong-kong-mark-six", "HK_MARK_SIX", "香港六合彩", "六合彩", "六合彩", "white", 600, 8},
	{"happy8-mark-six", "HAPPY8_MARK_SIX", "快乐8六合彩", "六合彩", "六合彩", "green", 600, 9},
	{"new-macau-mark-six", "NEW_MACAU_MARK_SIX", "新澳门六合彩", "六合彩", "六合彩", "gold", 600, 10},
	{"old-macau-mark-six", "OLD_MACAU_MARK_SIX", "老澳门六合彩", "六合彩", "六合彩", "brown", 600, 11},
	// 其他演示盘
	{"speed-racing", "SPEED_RACING", "极速赛车", "赛车", "赛车", "red", 180, 21},
	{"au-lucky-10", "AU_LUCKY_10", "澳洲幸运10", "幸运10", "幸运10", "purple", 300, 22},
	{"au-lucky-5", "AU_LUCKY_5", "澳洲幸运5", "幸运5", "幸运5", "purple", 300, 23},
	{"fly-racing", "FLY_RACING", "幸运飞艇", "飞艇", "飞艇", "blue", 300, 24},
	{"speed-fly", "SPEED_FLY", "极速飞艇", "飞艇", "飞艇", "blue", 180, 25},
	{"speed-ssc", "SPEED_SSC", "极速时时彩", "时时彩", "时时彩", "orange", 180, 26},
	{"sg-fly", "SG_FLY", "SG飞艇", "飞艇", "飞艇", "blue", 300, 27},
	{"sg-ssc", "SG_SSC", "SG时时彩", "时时彩", "时时彩", "orange", 300, 28},
	// PC 系列暂由平台自动开奖；接入稳定数据源后可在后台切换。
	{"pc-canada", "PC_CANADA", "PC加拿大", "PC", "PC", "teal", 210, 31},
	{"canada-28", "CANADA_28", "加拿大28", "PC", "28", "purple", 210, 32},
	{"canada-20", "CANADA_20", "加拿大2.0", "PC", "2.0", "blue", 120, 33},
}

var officialGames = []lottery.Game{
	{ID: "official-fc3d", Code: "OFFICIAL_FC3D", Name: "福彩3D", Category: "全国彩", Badge: "福彩3D", BadgeColor: "#e15b64", Enabled: true, SortOrder: 101, DrawInterval: 86400, SourceKind: "official", SourceName: "中国福彩网", SourceURL: "https://www.cwl.gov.cn/", SyncStatus: "idle"},
	{ID: "official-kl8", Code: "OFFICIAL_KL8", Name: "福彩快乐8", Category: "全国彩", Badge: "快乐8", BadgeColor: "#ef8a3c", Enabled: true, SortOrder: 102, DrawInterval: 86400, SourceKind: "official", SourceName: "中国福彩网", SourceURL: "https://www.cwl.gov.cn/", SyncStatus: "idle"},
	{ID: "official-pl3", Code: "OFFICIAL_PL3", Name: "排列3D", Category: "全国彩", Badge: "排列3", BadgeColor: "#4f7edc", Enabled: true, SortOrder: 103, DrawInterval: 86400, SourceKind: "official", SourceName: "中国体彩网", SourceURL: "https://www.sporttery.cn/", SyncStatus: "idle"},
	{ID: "official-qxc", Code: "OFFICIAL_QXC", Name: "七星彩", Category: "全国彩", Badge: "七星彩", BadgeColor: "#8066ca", Enabled: true, SortOrder: 104, DrawInterval: 259200, SourceKind: "official", SourceName: "中国体彩网", SourceURL: "https://www.sporttery.cn/", SyncStatus: "idle"},
	{ID: "official-tw-bingo", Code: "OFFICIAL_TW_BINGO", Name: "台湾宾果", Category: "高频彩", Badge: "宾果", BadgeColor: "#2eaa8c", Enabled: true, SortOrder: 105, DrawInterval: 300, SourceKind: "official", SourceName: "台湾彩券", SourceURL: "https://www.taiwanlottery.com/", SyncStatus: "idle"},
	{ID: "official-tw-super-lotto", Code: "OFFICIAL_TW_SUPER_LOTTO", Name: "台湾威力彩", Category: "境外彩", Badge: "威力彩", BadgeColor: "#e25e78", Enabled: true, SortOrder: 106, DrawInterval: 259200, SourceKind: "official", SourceName: "台湾彩券", SourceURL: "https://www.taiwanlottery.com/", SyncStatus: "idle"},
	{ID: "official-tw-daily539", Code: "OFFICIAL_TW_DAILY539", Name: "台湾今彩539", Category: "境外彩", Badge: "今彩539", BadgeColor: "#d69a32", Enabled: true, SortOrder: 107, DrawInterval: 86400, SourceKind: "official", SourceName: "台湾彩券", SourceURL: "https://www.taiwanlottery.com/", SyncStatus: "idle"},
	{ID: "official-tw-lotto649", Code: "OFFICIAL_TW_LOTTO649", Name: "台湾大乐透", Category: "境外彩", Badge: "大乐透", BadgeColor: "#3f91c9", Enabled: true, SortOrder: 108, DrawInterval: 259200, SourceKind: "official", SourceName: "台湾彩券", SourceURL: "https://www.taiwanlottery.com/", SyncStatus: "idle"},
}

// SeedLotteryData makes an empty development database useful immediately. It
// never overwrites live configuration or published results.
func SeedLotteryData(db *gorm.DB) error {
	now := time.Now().UTC().Truncate(time.Minute)
	for index, item := range defaultGames {
		next := now.Add(time.Duration(item.Interval) * time.Second)
		sortOrder := item.SortOrder
		if sortOrder <= 0 {
			sortOrder = index + 1
		}
		game := lottery.Game{
			ID: item.ID, Code: item.Code, Name: item.Name, Category: item.Category, Badge: item.Badge, BadgeColor: item.Color,
			Enabled: true, SortOrder: sortOrder, DrawInterval: item.Interval, NextDrawAt: next,
			SourceKind: "simulated", SourceName: "本地演示", SyncStatus: "idle",
		}
		created := db.Where("id = ?", game.ID).FirstOrCreate(&game)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 0 {
			continue
		}
		for offset := 0; offset < 12; offset++ {
			drawAt := now.Add(-time.Duration(offset+1) * time.Duration(item.Interval) * time.Second)
			issue := fmt.Sprintf("%s-%s-%03d", drawAt.Format("20060102"), strings.ToUpper(strings.ReplaceAll(item.Code, "_", "")), 999-offset)
			numbers := deterministicNumbers(index, offset)
			if err := db.Create(&lottery.Draw{GameID: item.ID, Issue: issue, Numbers: numbers, DrawAt: drawAt}).Error; err != nil {
				return err
			}
		}
	}
	for _, template := range officialGames {
		game := template
		game.NextDrawAt = now.Add(time.Duration(game.DrawInterval) * time.Second)
		if err := db.Where("id = ?", game.ID).FirstOrCreate(&game).Error; err != nil {
			return err
		}
	}
	return Ensure168SourceGames(db)
}

type SyncTargetResult struct {
	Created []string `json:"created"`
	Total   int      `json:"total"`
	Missing []string `json:"missing"`
}

// TargetGameIDs lists the 11 production bingo/mark-six games expected in admin.
var TargetGameIDs = []string{
	"bingo-ssc-1", "bingo-ssc-2", "bingo-ssc-3", "bingo-ssc-4",
	"bingo-racing-a", "bingo-racing-b", "bingo-mark-six",
	"hong-kong-mark-six", "happy8-mark-six", "new-macau-mark-six", "old-macau-mark-six",
}

// SyncTargetGames inserts missing target lottery games and default odds.
func SyncTargetGames(db *gorm.DB) (*SyncTargetResult, error) {
	targetSet := make(map[string]struct{}, len(TargetGameIDs))
	for _, id := range TargetGameIDs {
		targetSet[id] = struct{}{}
	}
	now := time.Now().UTC().Truncate(time.Minute)
	oddsSvc := NewOddsAdminService(db)
	result := &SyncTargetResult{Total: len(TargetGameIDs)}
	for index, item := range defaultGames {
		if item.SortOrder > 11 {
			continue
		}
		delete(targetSet, item.ID)
		next := now.Add(time.Duration(item.Interval) * time.Second)
		sortOrder := item.SortOrder
		if sortOrder <= 0 {
			sortOrder = index + 1
		}
		game := lottery.Game{
			ID: item.ID, Code: item.Code, Name: item.Name, Category: item.Category, Badge: item.Badge, BadgeColor: item.Color,
			Enabled: true, SortOrder: sortOrder, DrawInterval: item.Interval, NextDrawAt: next,
			SourceKind: "simulated", SourceName: "本地演示", SyncStatus: "idle",
		}
		created := db.Where("id = ?", game.ID).FirstOrCreate(&game)
		if created.Error != nil {
			return nil, created.Error
		}
		if created.RowsAffected > 0 {
			result.Created = append(result.Created, item.ID)
			for offset := 0; offset < 12; offset++ {
				drawAt := now.Add(-time.Duration(offset+1) * time.Duration(item.Interval) * time.Second)
				issue := fmt.Sprintf("%s-%s-%03d", drawAt.Format("20060102"), strings.ToUpper(strings.ReplaceAll(item.Code, "_", "")), 999-offset)
				numbers := deterministicNumbers(index, offset)
				if err := db.Create(&lottery.Draw{GameID: item.ID, Issue: issue, Numbers: numbers, DrawAt: drawAt}).Error; err != nil {
					return nil, err
				}
			}
		}
		if err := oddsSvc.EnsureGameDefaults(item.ID); err != nil {
			return nil, err
		}
	}
	for id := range targetSet {
		result.Missing = append(result.Missing, id)
	}
	if err := Ensure168SourceGames(db); err != nil {
		return nil, err
	}
	return result, nil
}

func deterministicNumbers(seed, offset int) string {
	values := []int{(seed + offset + 2) % 10, (seed*3 + offset + 8) % 10, (seed + offset*2 + 4) % 10, (seed*2 + offset + 1) % 10, (seed*4 + offset + 6) % 10}
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = strconv.Itoa(value)
	}
	return strings.Join(parts, ",")
}
