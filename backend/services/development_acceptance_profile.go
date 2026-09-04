package services

import (
	"backend/config"
	"backend/constants"
	"backend/data/models/chat"
	"backend/data/models/lottery"
	"backend/data/models/odds"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	"backend/utils"
	"bytes"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"gorm.io/gorm"
)

const (
	DevelopmentDatabaseMarkerNamespace  = config.LocalDevelopmentDatabaseMarkerNamespace
	developmentAcceptanceProfileVersion = "development-acceptance-odds-v1"
	developmentAcceptanceProfileLock    = int64(0x575A44455650524F)
)

// developmentOddsProfileJSON is a reviewed snapshot of the local acceptance
// prices. It is never used by normal server bootstrap: applying numeric odds
// remains an explicit, debug-only operator action.
//
//go:embed development_odds_profile.json
var developmentOddsProfileJSON []byte

type developmentOddsProfile struct {
	Version   string                           `json:"version"`
	Limits    developmentOddsProfileLimits     `json:"limits"`
	Templates []developmentOddsProfileTemplate `json:"templates"`
}

type developmentOddsProfileLimits struct {
	MinBet         float64 `json:"min_bet"`
	MaxBet         float64 `json:"max_bet"`
	MaxUserPeriod  float64 `json:"max_user_period"`
	MaxPeriodTotal float64 `json:"max_period_total"`
}

type developmentOddsProfileTemplate struct {
	Name  string             `json:"name"`
	Games []string           `json:"games"`
	Odds  map[string]float64 `json:"odds"`
}

// DevelopmentBootstrapReport is safe to print: it contains no passwords,
// tokens, hashes or database connection values.
type DevelopmentBootstrapReport struct {
	ProfileVersion       string `json:"profile_version"`
	HumanAccounts        int64  `json:"human_accounts"`
	RobotAccounts        int64  `json:"robot_accounts"`
	Workspaces           int64  `json:"workspaces"`
	ActiveAccounts       int64  `json:"active_accounts"`
	ActiveMemberships    int64  `json:"active_memberships"`
	ConfiguredGames      int    `json:"configured_games"`
	ConfiguredPlayQuotes int    `json:"configured_play_quotes"`
	AgentRoomCode        string `json:"agent_room_code"`
	AgentRoomOpenGames   int64  `json:"agent_room_open_games"`
	LedgerRows           int64  `json:"ledger_rows"`
	LedgerBalanceCents   int64  `json:"ledger_balance_cents"`
}

// DevelopmentAcceptanceProfileIdentity binds a completed local database to
// both the reviewed profile version and the exact embedded JSON bytes. It is
// intentionally non-secret and is used only as a durable initialization
// receipt; authorization to create that receipt remains in local-init.
func DevelopmentAcceptanceProfileIdentity() (string, error) {
	profile, err := loadDevelopmentOddsProfile()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(developmentOddsProfileJSON)
	return fmt.Sprintf("%s:%x", profile.Version, digest), nil
}

// ApplyDevelopmentAcceptanceProfile installs the reviewed odds and opens the
// seeded agent room only on an otherwise unpriced debug database. A repeated
// call is read-only and succeeds only when the existing configuration still
// matches the profile exactly; it never overwrites administrator changes.
func ApplyDevelopmentAcceptanceProfile(db *gorm.DB, mode string) (*DevelopmentBootstrapReport, error) {
	if db == nil {
		return nil, fmt.Errorf("数据库连接不可用")
	}
	if strings.ToLower(strings.TrimSpace(mode)) != "debug" {
		return nil, fmt.Errorf("本地验收配置仅允许在 debug 模式显式初始化")
	}
	profile, err := loadDevelopmentOddsProfile()
	if err != nil {
		return nil, err
	}
	var report *DevelopmentBootstrapReport
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", developmentAcceptanceProfileLock).Error; err != nil {
			return err
		}
		var existingRows int64
		if err := tx.Model(&odds.PlayLimit{}).Count(&existingRows).Error; err != nil {
			return err
		}
		fresh := existingRows == 0
		if fresh {
			if err := writeDevelopmentOddsProfile(tx, profile); err != nil {
				return err
			}
		}
		if err := verifyDevelopmentOddsProfile(tx, profile, true); err != nil {
			return err
		}
		if err := configureDevelopmentAgentRoom(tx, profile, fresh); err != nil {
			return err
		}
		report, err = verifyDevelopmentBootstrap(tx, profile)
		return err
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

// AuditDevelopmentAcceptanceProfile performs the same complete acceptance
// check without migrations, bootstrap writes, odds writes or room-switch
// changes. It is the read-only implementation behind `make dev-audit`.
func AuditDevelopmentAcceptanceProfile(db *gorm.DB, mode string) (*DevelopmentBootstrapReport, error) {
	if db == nil {
		return nil, fmt.Errorf("数据库连接不可用")
	}
	if strings.ToLower(strings.TrimSpace(mode)) != "debug" {
		return nil, fmt.Errorf("本地验收审计仅允许在 debug 模式运行")
	}
	profile, err := loadDevelopmentOddsProfile()
	if err != nil {
		return nil, err
	}
	var report *DevelopmentBootstrapReport
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := verifyDevelopmentOddsProfile(tx, profile, false); err != nil {
			return err
		}
		if err := configureDevelopmentAgentRoom(tx, profile, false); err != nil {
			return err
		}
		report, err = verifyDevelopmentBootstrap(tx, profile)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	return report, nil
}

func loadDevelopmentOddsProfile() (developmentOddsProfile, error) {
	var profile developmentOddsProfile
	decoder := json.NewDecoder(bytes.NewReader(developmentOddsProfileJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&profile); err != nil {
		return profile, fmt.Errorf("读取本地验收赔率配置失败: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return profile, fmt.Errorf("本地验收赔率配置只能包含一个 JSON 对象")
	}
	if err := validateDevelopmentOddsProfile(profile); err != nil {
		return profile, err
	}
	return profile, nil
}

func validateDevelopmentOddsProfile(profile developmentOddsProfile) error {
	if profile.Version != developmentAcceptanceProfileVersion {
		return fmt.Errorf("本地验收赔率配置版本不受支持: %s", profile.Version)
	}
	limits := profile.Limits
	if limits.MinBet <= 0 || limits.MaxBet < limits.MinBet || limits.MaxUserPeriod < limits.MaxBet || limits.MaxPeriodTotal < limits.MaxUserPeriod {
		return fmt.Errorf("本地验收限额关系不正确")
	}
	expectedGames := make(map[string]struct{}, len(defaultGames))
	for _, game := range defaultGames {
		expectedGames[game.ID] = struct{}{}
	}
	seenGames := make(map[string]struct{}, len(expectedGames))
	seenTemplates := make(map[string]struct{}, len(profile.Templates))
	for _, template := range profile.Templates {
		name := strings.TrimSpace(template.Name)
		if name == "" {
			return fmt.Errorf("本地验收赔率模板名称不能为空")
		}
		if _, exists := seenTemplates[name]; exists {
			return fmt.Errorf("本地验收赔率模板重复: %s", name)
		}
		seenTemplates[name] = struct{}{}
		if len(template.Games) == 0 || len(template.Odds) == 0 {
			return fmt.Errorf("本地验收赔率模板 %s 不能为空", name)
		}
		for code, value := range template.Odds {
			if strings.TrimSpace(code) != code || code == "" || math.IsNaN(value) || math.IsInf(value, 0) || !isValidOddsOverride(value) {
				return fmt.Errorf("本地验收赔率模板 %s 包含无效玩法或赔率: %s", name, code)
			}
		}
		for _, gameID := range template.Games {
			if _, ok := expectedGames[gameID]; !ok {
				return fmt.Errorf("本地验收赔率引用未知彩种: %s", gameID)
			}
			if _, exists := seenGames[gameID]; exists {
				return fmt.Errorf("本地验收赔率重复配置彩种: %s", gameID)
			}
			seenGames[gameID] = struct{}{}
			catalog := PlayCatalogForGame(gameID)
			if len(catalog) != len(template.Odds) {
				return fmt.Errorf("本地验收赔率与 %s 当前玩法数量不一致: %d/%d", gameID, len(template.Odds), len(catalog))
			}
			for _, item := range catalog {
				if _, ok := template.Odds[item.PlayCode]; !ok {
					return fmt.Errorf("本地验收赔率缺少 %s/%s", gameID, item.PlayCode)
				}
			}
		}
	}
	if len(seenGames) != len(expectedGames) {
		missing := make([]string, 0)
		for gameID := range expectedGames {
			if _, ok := seenGames[gameID]; !ok {
				missing = append(missing, gameID)
			}
		}
		sort.Strings(missing)
		return fmt.Errorf("本地验收赔率缺少彩种: %s", strings.Join(missing, ","))
	}
	return nil
}

func writeDevelopmentOddsProfile(tx *gorm.DB, profile developmentOddsProfile) error {
	service := NewOddsAdminService(tx)
	for _, template := range profile.Templates {
		for _, gameID := range template.Games {
			current, err := service.Get(gameID)
			if err != nil {
				return fmt.Errorf("读取 %s 赔率目录: %w", gameID, err)
			}
			if !current.RulesReady || current.RuleVersion == "" {
				return fmt.Errorf("彩种 %s 规则尚未就绪", gameID)
			}
			items := make([]PlayLimitItem, 0, len(current.Items))
			for _, item := range current.Items {
				item.Odds = template.Odds[item.PlayCode]
				item.MinBet = profile.Limits.MinBet
				item.MaxBet = profile.Limits.MaxBet
				item.MaxUserPeriod = profile.Limits.MaxUserPeriod
				item.MaxPeriodTotal = profile.Limits.MaxPeriodTotal
				items = append(items, item)
			}
			if _, err := service.Update(gameID, UpdateOddsLimitsInput{
				ExpectedRuleVersion: current.RuleVersion,
				ExpectedRevision:    current.ConfigRevision,
				Items:               items,
			}); err != nil {
				return fmt.Errorf("写入 %s 本地验收赔率: %w", gameID, err)
			}
		}
	}
	return nil
}

func verifyDevelopmentOddsProfile(tx *gorm.DB, profile developmentOddsProfile, lock bool) error {
	service := NewOddsAdminService(tx)
	profileGames := make(map[string]struct{})
	expectedRows := int64(0)
	for _, template := range profile.Templates {
		for _, gameID := range template.Games {
			profileGames[gameID] = struct{}{}
			expectedRows += int64(len(template.Odds))
			var current *GameOddsLimits
			var err error
			if lock {
				current, err = service.Get(gameID)
			} else {
				current, err = service.getReadOnly(gameID)
			}
			if err != nil {
				return err
			}
			if len(current.Items) != len(template.Odds) {
				return fmt.Errorf("%s 已有赔率目录与本地验收配置不一致", gameID)
			}
			for _, item := range current.Items {
				want, ok := template.Odds[item.PlayCode]
				if !ok || !item.Configured || math.Abs(item.Odds-want) > 0.00001 ||
					item.MinBet != profile.Limits.MinBet || item.MaxBet != profile.Limits.MaxBet ||
					item.MaxUserPeriod != profile.Limits.MaxUserPeriod || item.MaxPeriodTotal != profile.Limits.MaxPeriodTotal {
					return fmt.Errorf("%s/%s 已有赔率或限额与本地验收配置不一致；不会自动覆盖", gameID, item.PlayCode)
				}
			}
		}
	}
	var foreignConfigured int64
	if err := tx.Model(&odds.PlayLimit{}).
		Where("explicitly_configured = ? AND game_id NOT IN ?", true, mapKeys(profileGames)).
		Count(&foreignConfigured).Error; err != nil {
		return err
	}
	if foreignConfigured != 0 {
		return fmt.Errorf("已有 %d 条验收范围外的已确认赔率；不会自动覆盖", foreignConfigured)
	}
	var actualRows int64
	if err := tx.Model(&odds.PlayLimit{}).Count(&actualRows).Error; err != nil {
		return err
	}
	if actualRows != expectedRows {
		return fmt.Errorf("赔率表存在缺失、重复或目录外记录: %d/%d", actualRows, expectedRows)
	}
	return nil
}

func configureDevelopmentAgentRoom(tx *gorm.DB, profile developmentOddsProfile, write bool) error {
	var agent user.User
	if err := tx.Where("LOWER(username) = LOWER(?) AND role = ? AND deleted_at IS NULL", demoAgentUsername, "agent").First(&agent).Error; err != nil {
		return fmt.Errorf("本地体验代理不存在: %w", err)
	}
	var room workspacemodel.Workspace
	if err := tx.Where("owner_user_id = ? AND type = ?", agent.UserID, workspacemodel.TypeAgent).First(&room).Error; err != nil {
		return fmt.Errorf("本地体验代理房间不存在: %w", err)
	}
	if room.RoomCode != demoRoomCode {
		return fmt.Errorf("本地体验代理房间号异常: %s", room.RoomCode)
	}
	gameIDs := developmentProfileGameIDs(profile)
	if write {
		result := tx.Model(&chat.RoomGameSetting{}).
			Where("workspace_id = ? AND game_id IN ?", room.ID, gameIDs).
			Update("enabled", true)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(gameIDs)) {
			return fmt.Errorf("本地体验房间彩种目录不完整: %d/%d", result.RowsAffected, len(gameIDs))
		}
	}
	var enabled int64
	if err := tx.Model(&chat.RoomGameSetting{}).
		Where("workspace_id = ? AND enabled = ?", room.ID, true).
		Count(&enabled).Error; err != nil {
		return err
	}
	if enabled != int64(len(gameIDs)) {
		return fmt.Errorf("本地体验房间已有开关与验收配置不一致；不会自动覆盖: %d/%d", enabled, len(gameIDs))
	}
	return nil
}

func verifyDevelopmentBootstrap(tx *gorm.DB, profile developmentOddsProfile) (*DevelopmentBootstrapReport, error) {
	expected := []struct {
		username string
		role     string
		password string
	}{
		{constants.DefaultAdminUsername, "admin", constants.DefaultAdminPassword},
		{demoTenantUsername, "tenant", demoTenantPassword},
		{demoAgentUsername, "agent", demoAgentPassword},
		{demoUsername, "member", demoPassword},
	}
	accounts := make(map[string]user.User, len(expected))
	for _, item := range expected {
		var account user.User
		if err := tx.Where("LOWER(username) = LOWER(?) AND deleted_at IS NULL", item.username).First(&account).Error; err != nil {
			return nil, fmt.Errorf("本地验收账号 %s 不存在: %w", item.username, err)
		}
		if account.Role != item.role || account.Status != 1 || !utils.CheckPasswordHash(item.password, account.Password) {
			return nil, fmt.Errorf("本地验收账号 %s 的角色、状态或凭据不一致", item.username)
		}
		accounts[item.username] = account
	}
	tenant, agent, member := accounts[demoTenantUsername], accounts[demoAgentUsername], accounts[demoUsername]
	if agent.ParentTenantID == nil || *agent.ParentTenantID != tenant.UserID || member.ParentTenantID == nil || *member.ParentTenantID != tenant.UserID || member.ParentAgentID == nil || *member.ParentAgentID != agent.UserID {
		return nil, fmt.Errorf("本地验收账号上下级关系不正确")
	}
	var agentRoom workspacemodel.Workspace
	if err := tx.Where("owner_user_id = ? AND type = ?", agent.UserID, workspacemodel.TypeAgent).First(&agentRoom).Error; err != nil {
		return nil, err
	}
	if member.WorkspaceID != agentRoom.ID || member.LoginScope != agentRoom.Scope || agentRoom.RoomCode != demoRoomCode {
		return nil, fmt.Errorf("本地验收会员未正确归属代理房间")
	}
	for label, query := range map[string]string{
		"账号": accountHierarchyAuditSQL, "工作区": workspaceHierarchyAuditSQL, "成员关系": membershipHierarchyAuditSQL,
	} {
		var count int64
		if err := tx.Raw(query).Scan(&count).Error; err != nil {
			return nil, err
		}
		if count != 0 {
			return nil, fmt.Errorf("检测到 %d 个%s层级异常", count, label)
		}
	}
	var ledgerErrors int64
	if err := tx.Raw(`
		WITH all_rows AS (
			SELECT id,user_id,amount_cents,before_cents,after_cents FROM user_balance_transaction_archives
			UNION ALL
			SELECT id,user_id,amount_cents,before_cents,after_cents FROM user_balance_transactions
		), ordered AS (
			SELECT *,LAG(after_cents) OVER (PARTITION BY user_id ORDER BY id) AS previous_after FROM all_rows
		)
		SELECT COUNT(*) FROM ordered WHERE after_cents <> before_cents + amount_cents OR before_cents < 0 OR after_cents < 0 OR (previous_after IS NOT NULL AND before_cents <> previous_after)
	`).Scan(&ledgerErrors).Error; err != nil {
		return nil, err
	}
	if ledgerErrors != 0 {
		return nil, fmt.Errorf("检测到 %d 条账务算术或链路错误", ledgerErrors)
	}
	var latestBalanceErrors int64
	if err := tx.Raw(`
		WITH candidates AS (
			(SELECT DISTINCT ON (user_id) id,user_id,after_cents FROM user_balance_transactions ORDER BY user_id,id DESC)
			UNION ALL
			(SELECT DISTINCT ON (user_id) id,user_id,after_cents FROM user_balance_transaction_archives ORDER BY user_id,id DESC)
		), latest AS (SELECT DISTINCT ON (user_id) user_id,after_cents FROM candidates ORDER BY user_id,id DESC)
		SELECT COUNT(*) FROM "user" account LEFT JOIN latest ON latest.user_id=account.user_id
		WHERE account.deleted_at IS NULL AND COALESCE(latest.after_cents,0) <> account.balance_cents
	`).Scan(&latestBalanceErrors).Error; err != nil {
		return nil, err
	}
	if latestBalanceErrors != 0 {
		return nil, fmt.Errorf("检测到 %d 个账号余额与最新账务流水不一致", latestBalanceErrors)
	}
	var abnormalBets int64
	if err := tx.Table("lottery_bets").Where("reconciliation_status = ?", "abnormal").Count(&abnormalBets).Error; err != nil {
		return nil, err
	}
	if abnormalBets != 0 {
		return nil, fmt.Errorf("检测到 %d 条异常对账注单", abnormalBets)
	}
	var roomOddsOverrides int64
	if err := tx.Model(&odds.RoomPlayOdds{}).Where("workspace_id = ?", agentRoom.ID).Count(&roomOddsOverrides).Error; err != nil {
		return nil, err
	}
	if roomOddsOverrides != 0 {
		return nil, fmt.Errorf("88001 验收房间存在 %d 条房间赔率覆盖，实际赔率已偏离验收配置", roomOddsOverrides)
	}
	var memberOddsOverrides int64
	if err := tx.Model(&odds.UserPlayOdds{}).Where("workspace_id = ? AND user_id = ?", agentRoom.ID, member.UserID).Count(&memberOddsOverrides).Error; err != nil {
		return nil, err
	}
	if memberOddsOverrides != 0 {
		return nil, fmt.Errorf("体验会员存在 %d 条单独赔率覆盖，实际赔率已偏离验收配置", memberOddsOverrides)
	}
	var membershipOddsMultiplier float64
	if err := tx.Model(&workspacemodel.Membership{}).
		Select("odds_multiplier").
		Where("workspace_id = ? AND user_id = ? AND status = ?", agentRoom.ID, member.UserID, 1).
		Scan(&membershipOddsMultiplier).Error; err != nil {
		return nil, err
	}
	if math.Abs(membershipOddsMultiplier-1) > 0.00001 {
		return nil, fmt.Errorf("体验会员房间赔率系数为 %.4f，实际赔率已偏离验收配置", membershipOddsMultiplier)
	}
	gameIDs := developmentProfileGameIDs(profile)
	var platformGameIDs []string
	if err := tx.Model(&lottery.Game{}).
		Where("enabled = ? AND BTRIM(lobby_category) <> ''", true).
		Order("id ASC").
		Pluck("id", &platformGameIDs).Error; err != nil {
		return nil, err
	}
	if strings.Join(platformGameIDs, ",") != strings.Join(gameIDs, ",") {
		return nil, fmt.Errorf("平台实际开放并分类的彩种与验收配置不一致: %d/%d", len(platformGameIDs), len(gameIDs))
	}
	var visibleGameIDs []string
	if err := workspaceEnabledGamesQuery(tx, agentRoom.ID).
		Order("lottery_games.id ASC").
		Pluck("lottery_games.id", &visibleGameIDs).Error; err != nil {
		return nil, err
	}
	if strings.Join(visibleGameIDs, ",") != strings.Join(gameIDs, ",") {
		return nil, fmt.Errorf("88001 验收房间实际可下注彩种与配置不一致: %d/%d", len(visibleGameIDs), len(gameIDs))
	}
	report := &DevelopmentBootstrapReport{ProfileVersion: profile.Version, AgentRoomCode: agentRoom.RoomCode, ConfiguredGames: len(developmentProfileGameIDs(profile))}
	if err := tx.Model(&user.User{}).Where("deleted_at IS NULL AND NOT EXISTS (SELECT 1 FROM workspace_robot_profiles robot WHERE robot.user_id = \"user\".user_id)").Count(&report.HumanAccounts).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&workspacemodel.RobotProfile{}).Count(&report.RobotAccounts).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&workspacemodel.Workspace{}).Count(&report.Workspaces).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&user.User{}).Where("deleted_at IS NULL AND status = ?", 1).Count(&report.ActiveAccounts).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&workspacemodel.Membership{}).Where("status = ?", 1).Count(&report.ActiveMemberships).Error; err != nil {
		return nil, err
	}
	var configuredPlayQuotes int64
	if err := tx.Model(&odds.PlayLimit{}).Where("explicitly_configured = ?", true).Count(&configuredPlayQuotes).Error; err != nil {
		return nil, err
	}
	report.ConfiguredPlayQuotes = int(configuredPlayQuotes)
	if err := tx.Model(&chat.RoomGameSetting{}).Where("workspace_id = ? AND game_id IN ? AND enabled = ?", agentRoom.ID, gameIDs, true).Count(&report.AgentRoomOpenGames).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&user.BalanceTransaction{}).Count(&report.LedgerRows).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&user.User{}).Select("COALESCE(SUM(balance_cents),0)").Scan(&report.LedgerBalanceCents).Error; err != nil {
		return nil, err
	}
	if report.HumanAccounts < 4 || report.RobotAccounts < 30 || report.Workspaces < 3 ||
		report.ActiveAccounts != report.ActiveMemberships || report.AgentRoomOpenGames != int64(len(gameIDs)) {
		return nil, fmt.Errorf("本地验收账号、工作区、成员关系或房间开关数量不正确")
	}
	return report, nil
}

func developmentProfileGameIDs(profile developmentOddsProfile) []string {
	ids := make([]string, 0, len(defaultGames))
	for _, template := range profile.Templates {
		ids = append(ids, template.Games...)
	}
	sort.Strings(ids)
	return ids
}

func mapKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
