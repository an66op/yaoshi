package services

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"backend/data/models/lottery"
)

// Diagnostics are deliberately separate from ListGames and source import:
// those operations may materialize issues, update cursors or settle bets.
type SourceDiagnosticGame struct {
	GameID           string     `json:"game_id"`
	Name             string     `json:"name"`
	Enabled          bool       `json:"enabled"`
	Category         string     `json:"category"`
	LobbyCategory    string     `json:"lobby_category"`
	RuleVersion      string     `json:"rule_version"`
	RulesMessage     string     `json:"rules_message"`
	SourceKind       string     `json:"source_kind"`
	SourceName       string     `json:"source_name"`
	SourceKey        *string    `json:"source_key"`
	Source163Status  string     `json:"source_163_status"`
	Source163Message string     `json:"source_163_message"`
	SyncStatus       string     `json:"sync_status"`
	LastSyncAt       *time.Time `json:"last_sync_at"`
	LastSyncError    string     `json:"last_sync_error"`
	NextIssue        string     `json:"next_issue"`
	NextDrawAt       *time.Time `json:"next_draw_at"`
}

type SourceDiagnosticSource struct {
	Key               string            `json:"key"`
	Name              string            `json:"name"`
	Provider          string            `json:"provider"`
	Groups            []string          `json:"groups"`
	Candidate         bool              `json:"candidate"`
	Relation          string            `json:"relation"`
	GameIDs           []string          `json:"game_ids"`
	GameRelations     map[string]string `json:"game_relations,omitempty"`
	Endpoint          string            `json:"endpoint"`
	UpstreamGameID    int               `json:"upstream_game_id,omitempty"`
	Warning           string            `json:"warning"`
	WarningPersistent bool              `json:"warning_persistent"`
	WarningCheckedAt  string            `json:"warning_checked_at,omitempty"`
}

const (
	sourceRelationProduction       = "production"
	sourceRelationHistorical       = "historical"
	sourceRelationSameProduct      = "same_product_candidate"
	sourceRelationDifferentProduct = "different_product"
	sourceRelationCrossCheck       = "cross_check_only"
	sourceRelationUnverified       = "unverified_candidate"
	sourceRelationUnavailable      = "unavailable"
	sourceRelationCatalogOnly      = "catalog_only"
)

const (
	source163StatusCurrent             = "current"
	source163StatusVerifiedCandidate   = "verified_candidate"
	source163StatusCandidateUnverified = "candidate_unverified"
	source163StatusUnavailable         = "unavailable"
	source163StatusNotFound            = "not_found"
	source163StatusNotAssessed         = "not_assessed"
)

type SourceDiagnostics struct {
	Games   []SourceDiagnosticGame   `json:"games"`
	Catalog []SourceDiagnosticSource `json:"catalog"`
}

type SourceProbeResult struct {
	SourceKey    string     `json:"source_key"`
	Status       string     `json:"status"`
	CheckedAt    time.Time  `json:"checked_at"`
	DurationMS   int64      `json:"duration_ms"`
	HTTPStatus   *int       `json:"http_status"`
	Issue        *string    `json:"issue"`
	DrawAt       *time.Time `json:"draw_at"`
	Numbers      []int      `json:"numbers"`
	HistoryCount int        `json:"history_count"`
	Message      string     `json:"message"`
}

const sourceProbeTimeout = 12 * time.Second
const sourceProbeBodyLimit = 1 << 20
const sourceProbeHistoryLimit = 3

var sourceProbeSlots = make(chan struct{}, 2)
var sourceDiagnosticURL = regexp.MustCompile(`https?://[^\s<>"']+`)
var sourceDiagnosticCredential = regexp.MustCompile(`(?i)(sign2?|token|password|authorization|secret|api[_-]?key)\s*[:=]\s*[^\s,;]+`)
var sourceDiagnosticIssue = regexp.MustCompile(`^[0-9]{1,64}$`)

type sourceDiagnosticSpec struct {
	SourceDiagnosticSource
	count, min, max int
	unique          bool
	staleAfter      time.Duration
	binding         *api168Binding
}

func (s *LotteryService) SourceDiagnostics(ctx context.Context) (*SourceDiagnostics, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var games []lottery.Game
	// One plain SELECT. Do not call ListGames, EnsureCurrentIssue or any sync path.
	if err := s.db.WithContext(ctx).Select("id", "name", "enabled", "category", "lobby_category", "source_kind", "source_name", "source_url", "sync_status", "last_sync_at", "last_sync_error", "next_issue", "next_draw_at").Order("sort_order, id").Find(&games).Error; err != nil {
		return nil, err
	}
	return sourceDiagnosticsForGames(games), nil
}

func sourceDiagnosticsForGames(games []lottery.Game) *SourceDiagnostics {
	result := &SourceDiagnostics{Games: make([]SourceDiagnosticGame, 0, len(games)), Catalog: []SourceDiagnosticSource{}}
	for _, spec := range sourceDiagnosticSpecs() {
		result.Catalog = append(result.Catalog, spec.SourceDiagnosticSource)
	}
	for _, game := range games {
		profile, ready := rulesForGame(&game)
		row := SourceDiagnosticGame{GameID: game.ID, Name: game.Name, Enabled: game.Enabled, Category: game.Category,
			LobbyCategory: game.LobbyCategory, RuleVersion: profile.Version, SourceKind: game.SourceKind, SourceName: game.SourceName,
			SyncStatus: game.SyncStatus, LastSyncAt: game.LastSyncAt, LastSyncError: sanitizeSourceDiagnosticMessage(game.LastSyncError), NextIssue: game.NextIssue}
		if !ready {
			row.RulesMessage = gameRulesUnavailableMessage
		} else {
			row.RulesMessage = sourceDiagnosticRulesMessage(profile)
		}
		if !game.NextDrawAt.IsZero() {
			value := game.NextDrawAt
			row.NextDrawAt = &value
		}
		if key := configuredSourceDiagnosticKey(game); key != "" {
			row.SourceKey = &key
		}
		configuredKey := ""
		if row.SourceKey != nil {
			configuredKey = *row.SourceKey
		}
		row.Source163Status, row.Source163Message = source163DirectoryAssessment(game, configuredKey)
		result.Games = append(result.Games, row)
	}
	return result
}

func sourceDiagnosticRulesMessage(profile gameRuleProfile) string {
	switch profile.Version {
	case "racing-v2":
		return "10名号码、大小单双、1–5名龙虎及冠亚和；每项独立计注"
	case "digits5-v3":
		return "五球号码、大小单双、前三/中三/后三形态及首尾龙虎和"
	case "digits3-v2":
		return "三球号码、大小单双、总和及三球形态；不代表官方彩全部原生玩法"
	case markSixRuleVersion:
		return "特码A/B、两面、头尾、正码/正码特、色波及已核验组合玩法；依赖宾果原始球序"
	case pc28RuleV1:
		return "PC28玩法一：三球和值、两面、组合及形态；13/14按本版本条款结算"
	case pc28RuleV2:
		return "PC28玩法二：三球和值、两面、组合及形态；13/14按本版本条款结算"
	case pc28RuleV3:
		return "PC28玩法三：三球和值、两面、组合及形态；13/14按本版本条款结算"
	default:
		return "按已绑定规则版本识别与结算，赔率仍由后台配置"
	}
}

func sanitizeSourceDiagnosticMessage(message string) string {
	message = sourceDiagnosticURL.ReplaceAllString(message, "[来源地址]")
	message = sourceDiagnosticCredential.ReplaceAllString(message, "$1=[已隐藏]")
	runes := []rune(message)
	if len(runes) > 300 {
		message = string(runes[:300]) + "…"
	}
	return message
}

func IsSourceDiagnosticKey(key string) bool {
	_, ok := sourceDiagnosticSpecForKey(key)
	return ok
}

func sourceDiagnosticSpecForKey(key string) (sourceDiagnosticSpec, bool) {
	for _, item := range sourceDiagnosticSpecs() {
		if item.Key == key {
			return item, true
		}
	}
	return sourceDiagnosticSpec{}, false
}

// ProbeSource does not access s.db, source locks, importers, cursors or bets.
// Only two administrator probes may run concurrently within this process.
func (s *LotteryService) ProbeSource(ctx context.Context, key string) SourceProbeResult {
	select {
	case sourceProbeSlots <- struct{}{}:
		defer func() { <-sourceProbeSlots }()
	default:
		return SourceProbeResult{SourceKey: key, Status: "error", CheckedAt: time.Now().UTC(), Numbers: []int{}, Message: "已有来源检测进行中，请稍后重试"}
	}
	return probeSourceDiagnostic(ctx, key, time.Now, &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, rand.Reader)
}

type sourceProbeObservation struct {
	latest       sourceDraw
	historyCount int
	message      string
}

type sourceProbeHTTP struct {
	client *http.Client
	mu     sync.Mutex
	status *int
}

var errSourceProbeEmpty = errors.New("来源未返回当前开奖记录")
var errSourceProbeInvalid = errors.New("上游数据格式、彩种身份或开奖号码校验失败")

func (p *sourceProbeHTTP) request(ctx context.Context, endpoint, referer string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.New("来源请求构造失败")
	}
	req.Header.Set("User-Agent", officialUserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", referer)
	response, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, errors.New("来源请求超时或已取消")
		}
		return nil, errors.New("上游连接失败") // url.Error contains signed URLs; never return it.
	}
	defer response.Body.Close()
	p.mu.Lock()
	code := response.StatusCode
	if p.status == nil || *p.status >= 200 && *p.status < 300 {
		p.status = &code
	}
	p.mu.Unlock()
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("上游 HTTP %d（不重试、不跟随跳转）", code)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, sourceProbeBodyLimit+1))
	if err != nil {
		return nil, errors.New("读取来源响应失败或超时")
	}
	if len(body) > sourceProbeBodyLimit {
		return nil, errors.New("来源响应超过 1 MiB 限制")
	}
	if len(body) == 0 {
		return nil, errSourceProbeEmpty
	}
	return body, nil
}

func probeSourceDiagnostic(ctx context.Context, key string, now func() time.Time, client *http.Client, entropy io.Reader) (result SourceProbeResult) {
	started := time.Now()
	result = SourceProbeResult{SourceKey: key, Status: "error", CheckedAt: now().UTC(), Numbers: []int{}}
	defer func() { result.DurationMS = time.Since(started).Milliseconds() }()
	spec, ok := sourceDiagnosticSpecForKey(key)
	if !ok {
		result.Message = "未知来源，仅支持固定目录中的来源键"
		return
	}
	ctx, cancel := context.WithTimeout(ctx, sourceProbeTimeout)
	defer cancel()
	// Copy the supplied client so test transports are injectable without allowing
	// redirects (including redirects to localhost) or changing global clients.
	lockedClient := *client
	lockedClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	request := &sourceProbeHTTP{client: &lockedClient}
	var observation sourceProbeObservation
	var err error
	switch {
	case key == "bingo-ordered-163" || spec.Provider == "163" && (spec.UpstreamGameID == bingo163SetUpstreamGameID || spec.UpstreamGameID == bingo163OrderedUpstreamGameID):
		observation, err = probe163BingoOrderedSource(ctx, now(), entropy, request)
	case key == "sg-ssc-verified" || spec.Provider == "163" && spec.UpstreamGameID == sgSSC163MotherBinding.UpstreamGameID:
		observation, err = probeSGSSCSource(ctx, now(), entropy, request)
	case spec.Provider == "163" && source163MirrorProductionID(spec.UpstreamGameID):
		observation, err = probe163ProductionMirrorSource(ctx, spec, now(), entropy, request)
	case spec.Provider == "163":
		observation, err = probe163Source(ctx, spec, now(), entropy, request)
	case spec.binding != nil:
		observation, err = probe168Source(ctx, spec, now(), request)
	case key == "bingo-ordered":
		observation, err = probeBingoOrderedSource(ctx, now(), request)
	case strings.HasPrefix(key, "official:"):
		observation, err = probeOfficialSource(ctx, spec, now(), request)
	default:
		err = errors.New("此来源尚无只读诊断适配器，不会触发同步导入")
	}
	result.HTTPStatus = request.status
	result.HistoryCount = observation.historyCount
	if observation.latest.Issue != "" {
		value := observation.latest.Issue
		result.Issue = &value
	}
	if !observation.latest.DrawAt.IsZero() {
		value := observation.latest.DrawAt
		result.DrawAt = &value
	}
	result.Numbers = append(result.Numbers, observation.latest.Numbers...)
	if err != nil {
		if errors.Is(err, errSourceProbeEmpty) {
			result.Status = "empty"
		}
		result.Message = sanitizeSourceDiagnosticMessage(err.Error())
		return
	}
	if err = validateSourceProbeDraw(observation.latest, spec); err != nil {
		result.Message = err.Error()
		return
	}
	if observation.latest.DrawAt.After(now().Add(2 * time.Minute)) {
		result.Message = "来源开奖时间在未来，未通过时间校验"
		return
	}
	if now().Sub(observation.latest.DrawAt) > spec.staleAfter {
		result.Status, result.Message = "stale", "接口可达，但最近开奖超过该来源诊断时效阈值；未导入任何数据"
		return
	}
	result.Status, result.Message = "success", "只读连接与有限样本校验通过；不代表来源身份已验收、持续实时或已接入结算"
	if observation.message != "" {
		result.Message += "；" + observation.message
	}
	return
}

func validateSourceProbeDraw(draw sourceDraw, spec sourceDiagnosticSpec) error {
	if !sourceDiagnosticIssue.MatchString(draw.Issue) || draw.DrawAt.IsZero() || len(draw.Numbers) != spec.count {
		return errSourceProbeInvalid
	}
	seen := make(map[int]bool, len(draw.Numbers))
	for _, number := range draw.Numbers {
		if number < spec.min || number > spec.max || spec.unique && seen[number] {
			return errSourceProbeInvalid
		}
		seen[number] = true
	}
	if spec.Key == "163:20" || spec.Key == "official:official-qxc" {
		for _, number := range draw.Numbers[:6] {
			if number > 9 {
				return errSourceProbeInvalid
			}
		}
	}
	if spec.Key == "163:136" || spec.Key == "official:official-tw-super-lotto" {
		if draw.Numbers[6] > 8 {
			return errSourceProbeInvalid
		}
		regular := map[int]bool{}
		for _, number := range draw.Numbers[:6] {
			if regular[number] {
				return errSourceProbeInvalid
			}
			regular[number] = true
		}
	}
	return nil
}

func sourceProbeRecent(draws []sourceDraw, spec sourceDiagnosticSpec) ([]sourceDraw, error) {
	sort.SliceStable(draws, func(i, j int) bool { return draws[i].DrawAt.After(draws[j].DrawAt) })
	if len(draws) > sourceProbeHistoryLimit {
		draws = draws[:sourceProbeHistoryLimit]
	}
	seen := map[string]bool{}
	for _, draw := range draws {
		if validateSourceProbeDraw(draw, spec) != nil || seen[draw.Issue] {
			return nil, errSourceProbeInvalid
		}
		seen[draw.Issue] = true
	}
	return draws, nil
}

func sourceProbeDecode(body []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if decoder.Decode(target) != nil || ensureJSONEOF(decoder) != nil {
		return errSourceProbeInvalid
	}
	return nil
}
