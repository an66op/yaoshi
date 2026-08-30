package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/ws"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type LotteryService struct{ db *gorm.DB }

type GameSummary struct {
	ID             string     `json:"id"`
	Code           string     `json:"code"`
	Name           string     `json:"name"`
	Category       string     `json:"category"`
	LobbyCategory  string     `json:"lobby_category"`
	LobbySortOrder int        `json:"lobby_sort_order"`
	Badge          string     `json:"badge"`
	BadgeColor     string     `json:"badge_color"`
	Enabled        bool       `json:"enabled"`
	Issue          string     `json:"issue"`
	CurrentIssue   string     `json:"current_issue,omitempty"`
	BettorCount    int64      `json:"bettor_count,omitempty"`
	LatestNumbers  []int      `json:"latest_numbers,omitempty"`
	NextDrawAt     time.Time  `json:"next_draw_at"`
	DrawInterval   int        `json:"draw_interval"`
	SealSeconds    int        `json:"seal_seconds"`
	TimingSource   string     `json:"timing_source"`
	Turnover       float64    `json:"turnover"`
	Profit         float64    `json:"profit"`
	SourceKind     string     `json:"source_kind"`
	SourceName     string     `json:"source_name"`
	SourceURL      string     `json:"source_url"`
	SyncStatus     string     `json:"sync_status"`
	LastSyncAt     *time.Time `json:"last_sync_at"`
	LastSyncError  string     `json:"last_sync_error"`
	ScheduleMode   string     `json:"schedule_mode"`
	IssueStatus    string     `json:"issue_status"`
	AcceptAt       *time.Time `json:"accept_at,omitempty"`
	SealAt         *time.Time `json:"seal_at,omitempty"`
	SourceHealthy  bool       `json:"source_healthy"`
}

type LobbyCategorySummary struct {
	ID               uint64 `json:"id"`
	Name             string `json:"name"`
	SortOrder        int    `json:"sort_order"`
	GameCount        int64  `json:"game_count"`
	EnabledGameCount int64  `json:"enabled_game_count"`
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
	return s.listGamesForWorkspace(0)
}

// The platform and each room share published draws, not their betting cutoff.
// Read one room's settings once so every item in this response uses the same
// configuration snapshot; existing per-issue windows may only become shorter.
func (s *LotteryService) listGamesForWorkspace(workspaceID uint64) ([]GameSummary, error) {
	var games []lottery.Game
	if err := s.db.Order("sort_order asc").Find(&games).Error; err != nil {
		return nil, err
	}
	rawSettings, actualWorkspaceID, err := readTimingSettings(s.db, workspaceID)
	if err != nil {
		return nil, err
	}
	result := make([]GameSummary, 0, len(games))
	for _, game := range games {
		var draw lottery.Draw
		err := s.db.Where("game_id = ?", game.ID).Order("draw_at desc").First(&draw).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return nil, err
		}
		lifecycle, lifecycleErr := NewBetAdminService(s.db).EnsureCurrentIssue(&game)
		if lifecycleErr != nil {
			return nil, lifecycleErr
		}
		timingSource := game.TimingSource
		if timingSource == "" {
			timingSource = "configured"
		}
		summary := GameSummary{
			ID: game.ID, Code: game.Code, Name: game.Name, Category: game.Category,
			LobbyCategory: game.LobbyCategory, LobbySortOrder: game.LobbySortOrder,
			Badge: game.Badge, BadgeColor: game.BadgeColor, Enabled: game.Enabled,
			Issue: draw.Issue, LatestNumbers: parseNumbers(draw.Numbers),
			DrawInterval: effectiveDrawInterval(&game), SealSeconds: configuredSealSeconds(rawSettings, game.ID),
			TimingSource: timingSource,
			SourceKind:   game.SourceKind, SourceName: game.SourceName, SourceURL: game.SourceURL,
			SyncStatus: game.SyncStatus, LastSyncAt: game.LastSyncAt, LastSyncError: game.LastSyncError,
			// Source health is deliberately derived only from this game's live
			// synchronizer state.  Settlement/reconciliation health is a separate
			// administrative signal and can contain old (including robot) debt; it
			// must never make every member-facing game look like its source failed.
			// A current-period lifecycle error is still exposed through IssueStatus
			// and therefore continues to close that specific period.
			SourceHealthy: sourceHealthyForGame(&game),
			ScheduleMode: func() string {
				if game.SourceKind == "official" {
					return "official-feed"
				}
				if game.SourceKind == "external" {
					return "external-feed"
				}
				return "interval"
			}(),
		}
		var window *lottery.IssueWindow
		if lifecycle != nil && lifecycle.Issue != "" && lifecycle.ScheduledDrawAt != nil && !lifecycle.ScheduledDrawAt.IsZero() {
			window, err = ensureIssueWindow(s.db, actualWorkspaceID, &game, lifecycle.Issue, *lifecycle.ScheduledDrawAt, rawSettings)
			if err != nil {
				return nil, err
			}
		}
		applyGameTimingSummary(&summary, lifecycle, window, time.Now().UTC())
		result = append(result, summary)
	}
	var categories []lottery.LobbyCategory
	if err := s.db.Order("sort_order asc, id asc").Find(&categories).Error; err != nil {
		return nil, err
	}
	categoryRanks := make(map[string]int, len(categories))
	for index, category := range categories {
		categoryRanks[category.Name] = index
	}
	sort.SliceStable(result, func(i, j int) bool {
		leftRank, leftKnown := categoryRanks[result[i].LobbyCategory]
		rightRank, rightKnown := categoryRanks[result[j].LobbyCategory]
		if !leftKnown {
			leftRank = len(categories) + 1
		}
		if !rightKnown {
			rightRank = len(categories) + 1
		}
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if result[i].LobbySortOrder != result[j].LobbySortOrder {
			return result[i].LobbySortOrder < result[j].LobbySortOrder
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// Keep the issue number, countdown boundaries and status in the same snapshot.
// A platform's earlier seal must not close a room with a later valid cutoff;
// actual results and error/settlement states always remain authoritative.
func applyGameTimingSummary(summary *GameSummary, lifecycle *lottery.Issue, window *lottery.IssueWindow, now time.Time) {
	summary.CurrentIssue, summary.IssueStatus = "", lottery.IssueStatusAwaiting
	summary.AcceptAt, summary.SealAt, summary.NextDrawAt = nil, nil, time.Time{}
	if lifecycle == nil {
		return
	}
	summary.CurrentIssue, summary.IssueStatus = lifecycle.Issue, lifecycle.Status
	if window == nil || window.Issue != lifecycle.Issue || window.GameID != summary.ID ||
		window.ScheduledDrawAt.IsZero() || window.SealAt.IsZero() || window.SealAt.After(window.ScheduledDrawAt) {
		switch lifecycle.Status {
		case lottery.IssueStatusPending, lottery.IssueStatusAccepting, lottery.IssueStatusSealed:
			summary.IssueStatus = lottery.IssueStatusAwaiting
		}
		return
	}
	acceptAt, sealAt := window.AcceptAt, window.SealAt
	summary.AcceptAt, summary.SealAt, summary.NextDrawAt = &acceptAt, &sealAt, window.ScheduledDrawAt
	summary.DrawInterval = window.DrawInterval
	summary.SealSeconds = int(window.ScheduledDrawAt.Sub(window.SealAt) / time.Second)
	switch lifecycle.Status {
	case lottery.IssueStatusError, lottery.IssueStatusSettling, lottery.IssueStatusSettled:
		return
	case lottery.IssueStatusPending, lottery.IssueStatusAccepting, lottery.IssueStatusSealed, lottery.IssueStatusAwaiting:
		if lifecycle.DrawAt != nil {
			summary.IssueStatus = lottery.IssueStatusSettling
			return
		}
		summary.IssueStatus = windowStatus(window, now)
	}
}

// sourceHealthyForGame reports only the live draw-source state for one game.
// Do not add settlement-health, historic issue or bet aggregates here: those
// belong to the reconciliation dashboard, not the member lobby availability
// decision.  Current-period availability remains independently represented by
// GameSummary.IssueStatus.
func sourceHealthyForGame(game *lottery.Game) bool {
	if game == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(game.SourceKind)) {
	case "external", "official":
		status := strings.ToLower(strings.TrimSpace(game.SyncStatus))
		switch status {
		case "error", "stale", "paused":
			return false
		case "syncing":
			// During a retry, keep the previous failure visible until a complete
			// successful response clears LastSyncError.
			return strings.TrimSpace(game.LastSyncError) == ""
		default:
			return true
		}
	default:
		// Platform draws do not depend on an upstream source.  Their current
		// period may still be closed through IssueStatus without being falsely
		// labelled as an upstream-source outage.
		return true
	}
}

// defaultLobbyPlacement is the initial shared shelf configuration, not the
// source/provider classification. Only the explicitly listed games get a
// default shelf; other catalog games remain unclassified until configured.
func defaultLobbyPlacement(gameID string) (string, int) {
	order := map[string]int{
		"speed-racing": 1, "speed-fly": 2, "speed-ssc": 3, "sg-fly": 4, "sg-ssc": 5,
		"fly-racing": 6, "au-lucky-5": 7, "au-lucky-10": 8,
		"bingo-mark-six": 1, "bingo-racing-a": 2, "bingo-racing-b": 3,
		"bingo-ssc-1": 4, "bingo-ssc-2": 5, "bingo-ssc-3": 6, "bingo-ssc-4": 7,
		"pc-canada": 1, "canada-28": 2, "canada-20": 3,
		"hong-kong-mark-six": 1, "happy8-mark-six": 2, "new-macau-mark-six": 3, "old-macau-mark-six": 4,
	}
	value, ok := order[gameID]
	if !ok {
		return "", 0
	}
	if strings.HasPrefix(gameID, "bingo-") {
		return "宾果", value
	}
	if strings.HasPrefix(gameID, "pc-") || strings.HasPrefix(gameID, "canada-") {
		return "PC", value
	}
	switch gameID {
	case "hong-kong-mark-six", "happy8-mark-six", "new-macau-mark-six", "old-macau-mark-six":
		return "六合彩", value
	default:
		return "彩票", value
	}
}

func (s *LotteryService) ListLobbyCategories() ([]LobbyCategorySummary, error) {
	var categories []lottery.LobbyCategory
	if err := s.db.Order("sort_order asc, id asc").Find(&categories).Error; err != nil {
		return nil, err
	}
	result := make([]LobbyCategorySummary, 0, len(categories))
	for _, category := range categories {
		var gameCount, enabledCount int64
		if err := s.db.Model(&lottery.Game{}).Where("lobby_category = ?", category.Name).Count(&gameCount).Error; err != nil {
			return nil, err
		}
		if err := s.db.Model(&lottery.Game{}).Where("lobby_category = ? AND enabled = ?", category.Name, true).Count(&enabledCount).Error; err != nil {
			return nil, err
		}
		result = append(result, LobbyCategorySummary{ID: category.ID, Name: category.Name, SortOrder: category.SortOrder, GameCount: gameCount, EnabledGameCount: enabledCount})
	}
	return result, nil
}

func (s *LotteryService) SaveLobbyCategory(id uint64, name string, sortOrder int) (*LobbyCategorySummary, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "未分类" {
		return nil, fmt.Errorf("分类名称不能为空或使用“未分类”")
	}
	if sortOrder < 0 {
		sortOrder = 0
	}
	var saved lottery.LobbyCategory
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if id == 0 {
			// Name is globally unique so recreating a previously retired shelf
			// restores that exact row instead of losing its audit identity or
			// failing on the historic unique key.
			var retired lottery.LobbyCategory
			findErr := tx.Unscoped().Where("name = ? AND deleted_at IS NOT NULL", name).First(&retired).Error
			if findErr == nil {
				if err := tx.Unscoped().Model(&retired).Updates(map[string]any{
					"deleted_at": nil, "sort_order": sortOrder,
				}).Error; err != nil {
					return err
				}
				saved = retired
				return nil
			}
			if findErr != gorm.ErrRecordNotFound {
				return findErr
			}
			saved = lottery.LobbyCategory{Name: name, SortOrder: sortOrder}
			return tx.Create(&saved).Error
		}
		if err := tx.First(&saved, id).Error; err != nil {
			return err
		}
		oldName := saved.Name
		if err := tx.Model(&saved).Updates(map[string]any{"name": name, "sort_order": sortOrder}).Error; err != nil {
			return err
		}
		if oldName != name {
			if err := tx.Model(&lottery.Game{}).Where("lobby_category = ?", oldName).Update("lobby_category", name).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	categories, err := s.ListLobbyCategories()
	if err != nil {
		return nil, err
	}
	for _, category := range categories {
		if category.ID == saved.ID {
			ws.NotifyGameCatalogChanged(0, "*", "", "", true)
			return &category, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (s *LotteryService) DeleteLobbyCategory(id uint64) error {
	var count int64
	if err := s.db.Model(&lottery.LobbyCategory{}).Count(&count).Error; err != nil {
		return err
	}
	if count <= 1 {
		return fmt.Errorf("至少保留一个前台分类")
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var category lottery.LobbyCategory
		if err := tx.First(&category, id).Error; err != nil {
			return err
		}
		if err := tx.Model(&lottery.Game{}).Where("lobby_category = ?", category.Name).Updates(map[string]any{"lobby_category": "", "lobby_sort_order": 0, "enabled": false}).Error; err != nil {
			return err
		}
		return tx.Delete(&category).Error
	})
	if err == nil {
		ws.NotifyGameCatalogChanged(0, "*", "", "", false)
	}
	return err
}

func (s *LotteryService) AssignLobbyCategory(gameID, categoryName string, sortOrder int) (*GameSummary, error) {
	categoryName = strings.TrimSpace(categoryName)
	if sortOrder < 0 {
		sortOrder = 0
	}
	if categoryName != "" {
		var count int64
		if err := s.db.Model(&lottery.LobbyCategory{}).Where("name = ?", categoryName).Count(&count).Error; err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, fmt.Errorf("所选分类不存在")
		}
	}
	updates := map[string]any{"lobby_category": categoryName, "lobby_sort_order": sortOrder}
	if categoryName == "" {
		updates["enabled"] = false
	}
	result := s.db.Model(&lottery.Game{}).Where("id = ?", gameID).Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	game, err := s.gameSummary(gameID)
	if err == nil {
		ws.NotifyGameCatalogChanged(0, "*", "", gameID, game.Enabled)
	}
	return game, err
}

func (s *LotteryService) gameSummary(gameID string) (*GameSummary, error) {
	games, err := s.ListGames()
	if err != nil {
		return nil, err
	}
	for _, item := range games {
		if item.ID == gameID {
			return &item, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

// EnrichForLobby fills the open issue and distinct bettor count for lobby cards.
func (s *LotteryService) EnrichForLobby(games []GameSummary) ([]GameSummary, error) {
	if len(games) == 0 {
		return games, nil
	}
	for i := range games {
		// ListGames already froze this issue together with its countdown. A
		// second CurrentIssue query can cross a draw and mismatch that snapshot.
		issue := games[i].CurrentIssue
		if strings.TrimSpace(issue) == "" {
			continue
		}
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
	if enabled && strings.TrimSpace(game.LobbyCategory) == "" {
		return nil, fmt.Errorf("请先为彩种选择前台分类")
	}
	if err := s.db.Model(&game).Update("enabled", enabled).Error; err != nil {
		return nil, err
	}
	summary, err := s.gameSummary(id)
	if err == nil {
		ws.NotifyGameCatalogChanged(0, "*", "", id, summary.Enabled)
	}
	return summary, err
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
	// 平台自开彩
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
	{ID: "official-fc3d", Code: "OFFICIAL_FC3D", Name: "福彩3D", Category: "全国彩", Badge: "福彩3D", BadgeColor: "#e15b64", Enabled: false, SortOrder: 101, DrawInterval: 86400, SourceKind: "official", SourceName: "中国福彩网", SourceURL: "https://www.cwl.gov.cn/", SyncStatus: "idle"},
	{ID: "official-kl8", Code: "OFFICIAL_KL8", Name: "福彩快乐8", Category: "全国彩", Badge: "快乐8", BadgeColor: "#ef8a3c", Enabled: false, SortOrder: 102, DrawInterval: 86400, SourceKind: "official", SourceName: "中国福彩网", SourceURL: "https://www.cwl.gov.cn/", SyncStatus: "idle"},
	{ID: "official-pl3", Code: "OFFICIAL_PL3", Name: "排列3D", Category: "全国彩", Badge: "排列3", BadgeColor: "#4f7edc", Enabled: false, SortOrder: 103, DrawInterval: 86400, SourceKind: "official", SourceName: "中国体彩网", SourceURL: "https://www.sporttery.cn/", SyncStatus: "idle"},
	{ID: "official-qxc", Code: "OFFICIAL_QXC", Name: "七星彩", Category: "全国彩", Badge: "七星彩", BadgeColor: "#8066ca", Enabled: false, SortOrder: 104, DrawInterval: 259200, SourceKind: "official", SourceName: "中国体彩网", SourceURL: "https://www.sporttery.cn/", SyncStatus: "idle"},
	{ID: "official-tw-bingo", Code: "OFFICIAL_TW_BINGO", Name: "台湾宾果", Category: "高频彩", Badge: "宾果", BadgeColor: "#2eaa8c", Enabled: false, SortOrder: 105, DrawInterval: 300, SourceKind: "official", SourceName: "台湾彩券", SourceURL: "https://www.taiwanlottery.com/", SyncStatus: "idle"},
	{ID: "official-tw-super-lotto", Code: "OFFICIAL_TW_SUPER_LOTTO", Name: "台湾威力彩", Category: "境外彩", Badge: "威力彩", BadgeColor: "#e25e78", Enabled: false, SortOrder: 106, DrawInterval: 259200, SourceKind: "official", SourceName: "台湾彩券", SourceURL: "https://www.taiwanlottery.com/", SyncStatus: "idle"},
	{ID: "official-tw-daily539", Code: "OFFICIAL_TW_DAILY539", Name: "台湾今彩539", Category: "境外彩", Badge: "今彩539", BadgeColor: "#d69a32", Enabled: false, SortOrder: 107, DrawInterval: 86400, SourceKind: "official", SourceName: "台湾彩券", SourceURL: "https://www.taiwanlottery.com/", SyncStatus: "idle"},
	{ID: "official-tw-lotto649", Code: "OFFICIAL_TW_LOTTO649", Name: "台湾大乐透", Category: "境外彩", Badge: "大乐透", BadgeColor: "#3f91c9", Enabled: false, SortOrder: 108, DrawInterval: 259200, SourceKind: "official", SourceName: "台湾彩券", SourceURL: "https://www.taiwanlottery.com/", SyncStatus: "idle"},
}

// LotterySeedOptions separates the shared lottery catalog from local-only
// fixture history. Production needs the catalog, but must never publish
// deterministic numbers which could be mistaken for an official result.
type LotterySeedOptions struct {
	IncludeDeterministicHistory bool
}

type lobbyCategorySeed struct {
	Name      string
	SortOrder int
}

var defaultLobbyCategories = []lobbyCategorySeed{
	{Name: "彩票", SortOrder: 10},
	{Name: "宾果", SortOrder: 20},
	{Name: "PC", SortOrder: 30},
	{Name: "六合彩", SortOrder: 40},
}

const deterministicFixtureDrawCount = 12

// SeedLotteryData is the safe catalog-only compatibility entry point. New
// callers should use SeedLotteryCatalog and opt into fixture history only from
// an explicitly debug-scoped bootstrap.
func SeedLotteryData(db *gorm.DB) error {
	return SeedLotteryCatalog(db, LotterySeedOptions{})
}

// SeedLotteryCatalog makes an empty database operational without overwriting
// live configuration or published results.
func SeedLotteryCatalog(db *gorm.DB, options LotterySeedOptions) error {
	if db == nil {
		return fmt.Errorf("lottery catalog database is required")
	}
	now := time.Now().UTC().Truncate(time.Minute)
	return db.Transaction(func(tx *gorm.DB) error {
		var existingCategories []lottery.LobbyCategory
		// A retired default shelf is still an operator decision. Include its
		// tombstone in the inventory so restarting does not try to recreate it.
		if err := tx.Unscoped().Order("sort_order asc, id asc").Find(&existingCategories).Error; err != nil {
			return err
		}
		for _, category := range missingDefaultLobbyCategories(existingCategories) {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&category).Error; err != nil {
				return err
			}
		}

		for index, item := range defaultGames {
			next := now.Add(time.Duration(item.Interval) * time.Second)
			sortOrder := item.SortOrder
			if sortOrder <= 0 {
				sortOrder = index + 1
			}
			lobbyCategory, lobbySortOrder := defaultLobbyPlacement(item.ID)
			sourceKind, sourceName, sourceURL, syncStatus := defaultLotterySource(item.ID)
			game := lottery.Game{
				ID: item.ID, Code: item.Code, Name: item.Name, Category: item.Category, Badge: item.Badge, BadgeColor: item.Color,
				Enabled: true, SortOrder: sortOrder, DrawInterval: item.Interval, NextDrawAt: next,
				LobbyCategory: lobbyCategory, LobbySortOrder: lobbySortOrder,
				SourceKind: sourceKind, SourceName: sourceName, SourceURL: sourceURL, SyncStatus: syncStatus,
				CreatedAt: now,
			}
			if err := tx.Where("id = ?", game.ID).FirstOrCreate(&game).Error; err != nil {
				return err
			}
			if !options.IncludeDeterministicHistory {
				continue
			}
			if err := seedDeterministicHistory(tx, game, item, index, now); err != nil {
				return err
			}
		}

		for _, template := range officialGames {
			game := template
			game.NextDrawAt = now.Add(time.Duration(game.DrawInterval) * time.Second)
			game.CreatedAt = now
			game.UpdatedAt = now
			// Game.Enabled has a database default of true. GORM intentionally
			// substitutes that default when a struct contains the zero bool value,
			// which would accidentally expose every catalog-only official game on a
			// fresh install. A map keeps the explicit false value while the conflict
			// clause preserves any operator configuration already stored for the ID.
			if err := tx.Model(&lottery.Game{}).Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "id"}},
				DoNothing: true,
			}).Create(officialGameSeedValues(game)).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func officialGameSeedValues(game lottery.Game) map[string]any {
	// This helper is insert-only. Existing rows are protected by ON CONFLICT
	// DO NOTHING above, so operator assignments (including a deliberate blank)
	// are never reset on restart. Games outside the default shelf list stay
	// unclassified without changing their explicit disabled state.
	if strings.TrimSpace(game.LobbyCategory) == "" {
		category, sortOrder := defaultLobbyPlacement(game.ID)
		game.LobbyCategory = category
		if game.LobbySortOrder == 0 {
			game.LobbySortOrder = sortOrder
		}
	}
	return map[string]any{
		"id": game.ID, "code": game.Code, "name": game.Name, "category": game.Category,
		"lobby_category": game.LobbyCategory, "lobby_sort_order": game.LobbySortOrder,
		"badge": game.Badge, "badge_color": game.BadgeColor, "enabled": game.Enabled,
		"sort_order": game.SortOrder, "draw_interval": game.DrawInterval, "next_draw_at": game.NextDrawAt,
		"source_kind": game.SourceKind, "source_name": game.SourceName, "source_url": game.SourceURL,
		"sync_status": game.SyncStatus, "last_sync_at": game.LastSyncAt, "last_sync_error": game.LastSyncError,
		"created_at": game.CreatedAt, "updated_at": game.UpdatedAt,
	}
}

func missingDefaultLobbyCategories(existing []lottery.LobbyCategory) []lottery.LobbyCategory {
	present := make(map[string]struct{}, len(existing))
	for _, category := range existing {
		present[category.Name] = struct{}{}
	}
	missing := make([]lottery.LobbyCategory, 0, len(defaultLobbyCategories))
	for _, template := range defaultLobbyCategories {
		if _, ok := present[template.Name]; ok {
			continue
		}
		missing = append(missing, lottery.LobbyCategory{Name: template.Name, SortOrder: template.SortOrder})
	}
	return missing
}

func defaultLotterySource(gameID string) (kind, name, sourceURL, status string) {
	for _, item := range api168HighFreqBindings {
		if item.GameID == gameID {
			return "external", "168开奖网", "https://kj138138.com/view/api/index.html", "idle"
		}
	}
	for _, item := range api168MarkSixBindings {
		if item.GameID == gameID {
			return "external", "168开奖网", "https://kj138138.com/view/api/index.html", "idle"
		}
	}
	for _, item := range api168BingoBindings {
		if item.GameID == gameID {
			return "external", "168开奖网", "https://kj138138.com/view/api/index.html", "idle"
		}
	}
	return "platform", "王者开奖", "", "ok"
}

func seedDeterministicHistory(tx *gorm.DB, game lottery.Game, item seedGame, seed int, fallback time.Time) error {
	var candidates []lottery.Draw
	codeToken := deterministicFixtureCode(item.Code)
	if err := tx.Where("game_id = ? AND issue LIKE ?", game.ID, "%"+codeToken+"%").
		Order("created_at asc, id asc").Limit(128).Find(&candidates).Error; err != nil {
		return err
	}
	anchor, hasFixture := deterministicFixtureAnchor(item.Code, item.Interval, game.CreatedAt, fallback, candidates)
	if !hasFixture {
		var drawCount int64
		if err := tx.Model(&lottery.Draw{}).Where("game_id = ?", game.ID).Count(&drawCount).Error; err != nil {
			return err
		}
		// A pre-existing game with genuine/operator-managed history must not gain
		// local fixtures merely because debug mode is enabled later.
		if drawCount > 0 {
			return nil
		}
	}
	draws := deterministicFixtureRows(game.ID, item.Code, item.Interval, seed, anchor)
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "game_id"}, {Name: "issue"}},
		DoNothing: true,
	}).Create(&draws).Error
}

func deterministicFixtureCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "_", ""))
}

func deterministicFixtureOffset(issue, code string) (int, bool) {
	codeToken := deterministicFixtureCode(code)
	for offset := 0; offset < deterministicFixtureDrawCount; offset++ {
		if strings.HasSuffix(issue, fmt.Sprintf("-%s-%03d", codeToken, 999-offset)) {
			return offset, true
		}
	}
	return 0, false
}

func deterministicFixtureAnchor(code string, interval int, createdAt, fallback time.Time, existing []lottery.Draw) (time.Time, bool) {
	if interval <= 0 {
		interval = 300
	}
	for _, draw := range existing {
		offset, ok := deterministicFixtureOffset(draw.Issue, code)
		if !ok {
			continue
		}
		anchor := draw.DrawAt.UTC().Add(time.Duration(offset+1) * time.Duration(interval) * time.Second)
		return anchor.Truncate(time.Minute), true
	}
	anchor := createdAt
	if anchor.IsZero() {
		anchor = fallback
	}
	return anchor.UTC().Truncate(time.Minute), false
}

func deterministicFixtureRows(gameID, code string, interval, seed int, anchor time.Time) []lottery.Draw {
	if interval <= 0 {
		interval = 300
	}
	anchor = anchor.UTC().Truncate(time.Minute)
	draws := make([]lottery.Draw, 0, deterministicFixtureDrawCount)
	for offset := 0; offset < deterministicFixtureDrawCount; offset++ {
		drawAt := anchor.Add(-time.Duration(offset+1) * time.Duration(interval) * time.Second)
		issue := fmt.Sprintf("%s-%s-%03d", drawAt.Format("20060102"), deterministicFixtureCode(code), 999-offset)
		draws = append(draws, lottery.Draw{
			GameID:  gameID,
			Issue:   issue,
			Numbers: deterministicNumbers(seed, offset),
			DrawAt:  drawAt,
		})
	}
	return draws
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
		lobbyCategory, lobbySortOrder := defaultLobbyPlacement(item.ID)
		game := lottery.Game{
			ID: item.ID, Code: item.Code, Name: item.Name, Category: item.Category, Badge: item.Badge, BadgeColor: item.Color,
			Enabled: true, SortOrder: sortOrder, DrawInterval: item.Interval, NextDrawAt: next,
			LobbyCategory: lobbyCategory, LobbySortOrder: lobbySortOrder,
			SourceKind: "platform", SourceName: "王者开奖", SyncStatus: "ok",
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
