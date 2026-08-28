package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"backend/data/models/lottery"

	"gorm.io/gorm/clause"
)

const officialUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/126.0 Safari/537.36"

var officialGroupLocks = map[string]*sync.Mutex{
	"china-welfare":  {},
	"china-sport":    {},
	"taiwan-bingo":   {},
	"taiwan-lottery": {},
	"168-highfreq":   {},
	"168-marksix":    {},
	"168-bingo":      {},
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
	Issue   string
	Numbers []int
	DrawAt  time.Time
}

// SyncOfficialSources imports public draw results from the official lottery
// websites. Calls are deliberately sequential and capped to a small history
// window to avoid putting pressure on upstream services.
func (s *LotteryService) SyncOfficialSources(ctx context.Context) []SourceSyncResult {
	results := make([]SourceSyncResult, 0, 8)
	for _, group := range []string{"china-welfare", "china-sport", "taiwan-bingo", "taiwan-lottery", "168-highfreq", "168-marksix", "168-bingo"} {
		results = append(results, s.SyncOfficialGroup(ctx, group)...)
	}
	return results
}

// SyncOfficialGroup refreshes one upstream provider group. Keeping provider
// groups independent allows the high-frequency Bingo feed to run without
// waiting for the slower daily lottery websites.
func (s *LotteryService) SyncOfficialGroup(ctx context.Context, group string) []SourceSyncResult {
	group = strings.TrimSpace(group)
	lock, ok := officialGroupLocks[group]
	if !ok {
		return []SourceSyncResult{{Status: "error", Error: "未知官方数据源分组: " + group}}
	}
	lock.Lock()
	defer lock.Unlock()

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
	case "168-highfreq":
		return s.sync168HighFreq(ctx)
	case "168-marksix":
		return s.sync168MarkSix(ctx)
	case "168-bingo":
		return s.sync168Bingo(ctx)
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
	var game lottery.Game
	if err := s.db.First(&game, "id = ?", gameID).Error; err != nil {
		return SourceSyncResult{GameID: gameID, Status: "error", Error: err.Error()}
	}
	// Disabled games remain available in the admin console with their complete
	// history, but must not consume upstream requests or settle new rounds.
	if !game.Enabled {
		return SourceSyncResult{GameID: gameID, SourceName: game.SourceName, Status: "ok"}
	}
	// Keep the previous error visible while retrying.  A failed source must not
	// reopen betting until a complete successful response clears the error.
	_ = s.db.Model(&game).Update("sync_status", "syncing").Error
	draws, err := fetch(ctx)
	if err != nil {
		return s.recordSyncError(gameID, err)
	}
	// Validate the complete upstream batch before writing any row. Racing,
	// flying and Lucky 10 results are permutations of 1..10; accepting a
	// partial, duplicated or out-of-range result would make the immutable draw
	// impossible to settle correctly. Doing this before the insert loop also
	// prevents a response whose later row is malformed from being half-imported.
	if err := validateOfficialDraws(game, draws); err != nil {
		return s.recordSyncError(gameID, err)
	}
	imported := 0
	latestDraw := sourceDraw{}
	for _, item := range draws {
		if item.Issue == "" || len(item.Numbers) == 0 {
			continue
		}
		draw := lottery.Draw{GameID: gameID, Issue: item.Issue, Numbers: joinNumbers(item.Numbers), DrawAt: item.DrawAt.UTC()}
		result := s.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "game_id"}, {Name: "issue"}}, DoNothing: true}).Create(&draw)
		if result.Error != nil {
			return s.recordSyncError(gameID, result.Error)
		}
		if result.RowsAffected > 0 {
			imported += int(result.RowsAffected)
			NewBetAdminService(s.db).SettleImportedDraw(gameID, item.Issue)
		}
		if latestDraw.DrawAt.IsZero() || item.DrawAt.After(latestDraw.DrawAt) {
			latestDraw = item
		}
	}
	now := time.Now().UTC()
	updates := map[string]any{"sync_status": "ok", "last_sync_at": now, "last_sync_error": ""}
	// A source can return a perfectly valid draw history while the old next
	// draw timestamp has already passed.  Advancing it from the freshest draw
	// prevents the member UI from being stuck at 00:00 / 封盘中 after a restart.
	if !latestDraw.DrawAt.IsZero() && game.DrawInterval > 0 {
		next := latestDraw.DrawAt.UTC().Add(time.Duration(game.DrawInterval) * time.Second)
		if !next.After(now) {
			missed := now.Sub(latestDraw.DrawAt.UTC()) / (time.Duration(game.DrawInterval) * time.Second)
			next = latestDraw.DrawAt.UTC().Add((missed + 1) * time.Duration(game.DrawInterval) * time.Second)
		}
		updates["next_draw_at"] = next
	}
	if err := s.db.Model(&game).Updates(updates).Error; err != nil {
		return SourceSyncResult{GameID: gameID, SourceName: game.SourceName, Status: "error", Error: err.Error()}
	}
	if err := s.db.First(&game, "id = ?", gameID).Error; err == nil {
		_, _ = NewBetAdminService(s.db).EnsureCurrentIssue(&game)
	}
	latestIssue := ""
	if len(draws) > 0 {
		latestIssue = draws[0].Issue
	}
	return SourceSyncResult{GameID: gameID, SourceName: game.SourceName, Status: "ok", Imported: imported, LatestIssue: latestIssue}
}

func validateOfficialDraws(game lottery.Game, draws []sourceDraw) error {
	if !requiresTenUniqueDrawNumbers(game) {
		return nil
	}
	for _, draw := range draws {
		if len(draw.Numbers) != 10 {
			return fmt.Errorf("%s 第 %s 期开奖数据无效：赛车类必须包含恰好 10 个号码，实际为 %d 个", game.Name, sourceIssueLabel(draw.Issue), len(draw.Numbers))
		}
		seen := make(map[int]struct{}, 10)
		for _, number := range draw.Numbers {
			if number < 1 || number > 10 {
				return fmt.Errorf("%s 第 %s 期开奖数据无效：号码 %d 超出 1~10", game.Name, sourceIssueLabel(draw.Issue), number)
			}
			if _, exists := seen[number]; exists {
				return fmt.Errorf("%s 第 %s 期开奖数据无效：号码 %d 重复", game.Name, sourceIssueLabel(draw.Issue), number)
			}
			seen[number] = struct{}{}
		}
	}
	return nil
}

func requiresTenUniqueDrawNumbers(game lottery.Game) bool {
	identity := game.Name + " " + game.Category
	return strings.Contains(identity, "赛车") || strings.Contains(identity, "飞艇") || strings.Contains(identity, "幸运10")
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
	message := limitDBText(syncErr.Error(), 480)
	var game lottery.Game
	_ = s.db.First(&game, "id = ?", gameID).Error
	_ = s.db.Model(&lottery.Game{}).Where("id = ?", gameID).Updates(map[string]any{"sync_status": "error", "last_sync_error": message}).Error
	if err := s.db.First(&game, "id = ?", gameID).Error; err == nil {
		_, _ = NewBetAdminService(s.db).EnsureCurrentIssue(&game)
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
