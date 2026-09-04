// Command dev-bootstrap performs the explicit, non-production initialization
// used by a fresh local clone. Normal backend startup deliberately does not
// install numeric odds or open a room.
package main

import (
	"backend/config"
	"backend/migrations"
	"backend/services"
	"backend/utils"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const developmentDatabaseMarkerNamespace = services.DevelopmentDatabaseMarkerNamespace

type developmentDatabaseMarker struct {
	Raw       string
	Phase     string
	ClusterID string
	Nonce     string
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("dev-bootstrap", flag.ContinueOnError)
	confirmed := flags.Bool("confirm-local-development", false, "confirm this is a disposable local development database")
	auditOnly := flags.Bool("audit-only", false, "verify the initialized local profile without changing schema or data")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !*confirmed || flags.NArg() != 0 {
		return fmt.Errorf("必须显式提供 --confirm-local-development")
	}
	config.LoadConfig()
	cfg := config.GetConfig()
	if err := validateDevelopmentTarget(cfg, !*auditOnly); err != nil {
		return err
	}
	if err := utils.InitFieldEncryption(cfg.Security.DataEncryptionKey); err != nil {
		return fmt.Errorf("初始化敏感字段加密失败: %w", err)
	}
	db, err := config.OpenDatabase()
	if err != nil {
		return err
	}
	db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	if *auditOnly {
		report, err := auditDevelopmentDatabase(db, cfg.Server.Mode)
		if err != nil {
			return fmt.Errorf("本地验收审计失败: %w", err)
		}
		return json.NewEncoder(output).Encode(report)
	}
	databaseMarker, existingReport, err := prepareDevelopmentDatabase(db, cfg.Server.Mode)
	if err != nil {
		return err
	}
	if existingReport != nil {
		return json.NewEncoder(output).Encode(existingReport)
	}
	if err := migrations.Run(db); err != nil {
		return fmt.Errorf("执行本地数据库迁移失败: %w", err)
	}
	if err := validateDevelopmentDatabaseState(db, cfg.Server.Mode); err != nil {
		return err
	}
	completeMarker, err := developmentDatabaseCompleteMarker(databaseMarker.ClusterID, databaseMarker.Nonce)
	if err != nil {
		return err
	}
	report, err := services.InitializeDevelopmentAcceptance(db, services.BootstrapOptions{
		Mode: cfg.Server.Mode, SeedExperienceAccounts: true,
	}, completeMarker)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(report)
}

// prepareDevelopmentDatabase is deliberately called before migrations. Only
// a database marked "initializing" by local-init may receive schema or fixture
// writes. A completed database is always audited read-only and is never
// repaired or reseeded by repeating dev-init.
func prepareDevelopmentDatabase(db *gorm.DB, mode string) (developmentDatabaseMarker, *services.DevelopmentBootstrapReport, error) {
	if db == nil {
		return developmentDatabaseMarker{}, nil, fmt.Errorf("数据库连接不可用")
	}
	rawMarker, err := readDevelopmentDatabaseMarker(db)
	if err != nil {
		return developmentDatabaseMarker{}, nil, err
	}
	marker, err := parseDevelopmentDatabaseMarker(rawMarker)
	if err != nil {
		return developmentDatabaseMarker{}, nil, fmt.Errorf("目标数据库没有有效的 local-init 初始化凭证，拒绝迁移或写入体验数据；请使用新的空数据库: %w", err)
	}
	if marker.Phase == "initializing" {
		if os.Getenv("BACKEND_LOCAL_INIT_CLUSTER_ID") != marker.ClusterID || os.Getenv("BACKEND_LOCAL_INIT_NONCE") != marker.Nonce {
			return developmentDatabaseMarker{}, nil, fmt.Errorf("初始化中的数据库必须通过持有本机恢复凭证的 scripts/local-init.sh 继续")
		}
		return marker, nil, nil
	}
	report, err := auditDevelopmentDatabase(db, mode)
	if err != nil {
		return developmentDatabaseMarker{}, nil, fmt.Errorf("已完成的本地验收库与当前配置不一致，dev-init 不会自动覆盖: %w", err)
	}
	return marker, report, nil
}

// auditDevelopmentDatabase makes read-only behavior a database-enforced
// property of the complete command, including migration inventory checks.
func auditDevelopmentDatabase(db *gorm.DB, mode string) (*services.DevelopmentBootstrapReport, error) {
	var report *services.DevelopmentBootstrapReport
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := migrations.VerifyApplied(tx); err != nil {
			return fmt.Errorf("本地验收审计要求数据库已完成当前迁移: %w", err)
		}
		var err error
		report, err = services.AuditDevelopmentAcceptanceProfile(tx, mode)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return report, err
}

// validateDevelopmentDatabaseState prevents the explicit fixture command from
// turning an unrelated local database into a partially seeded development
// database. A migration-only database is safe. An existing database is allowed
// only when its complete, already-priced acceptance profile passes the same
// read-only account, hierarchy, ledger, odds and room audit used after setup.
func validateDevelopmentDatabaseState(db *gorm.DB, mode string) error {
	if db == nil {
		return fmt.Errorf("数据库连接不可用")
	}
	criticalTables := []string{
		`"user"`,
		"workspaces",
		"workspace_memberships",
		"workspace_robot_profiles",
		"lottery_games",
		"lottery_play_limits",
		"room_game_settings",
		"system_settings",
		"user_balance_transactions",
		"lottery_bets",
		"lottery_issues",
		"lottery_draws",
	}
	nonEmpty := make([]string, 0, len(criticalTables))
	var configuredOdds int64
	for _, table := range criticalTables {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil {
			return fmt.Errorf("检查本地初始化目标表 %s 失败: %w", table, err)
		}
		if count > 0 {
			nonEmpty = append(nonEmpty, table)
		}
		if table == "lottery_play_limits" {
			configuredOdds = count
		}
	}
	if len(nonEmpty) == 0 {
		return nil
	}
	if configuredOdds == 0 {
		return fmt.Errorf("目标数据库已有业务数据但没有完整验收赔率，拒绝写入体验账号；请改用新的空数据库（已有表: %s）", strings.Join(nonEmpty, ", "))
	}
	if _, err := services.AuditDevelopmentAcceptanceProfile(db, mode); err != nil {
		return fmt.Errorf("目标数据库不是已完成的本地验收库，拒绝写入或修补体验数据: %w", err)
	}
	return nil
}

func readDevelopmentDatabaseMarker(db *gorm.DB) (string, error) {
	var marker string
	if err := db.Raw(`
		SELECT COALESCE(shobj_description(oid, 'pg_database'), '')
		FROM pg_database
		WHERE datname = current_database()
	`).Scan(&marker).Error; err != nil {
		return "", fmt.Errorf("读取本地数据库初始化凭证失败: %w", err)
	}
	return marker, nil
}

func developmentDatabaseCompleteMarker(clusterID, nonce string) (string, error) {
	if _, err := strconv.ParseUint(clusterID, 10, 64); err != nil {
		return "", fmt.Errorf("本地 PostgreSQL 集群身份无效")
	}
	decodedNonce, err := hex.DecodeString(nonce)
	if err != nil || len(decodedNonce) != 16 {
		return "", fmt.Errorf("本地初始化恢复凭证无效")
	}
	profileIdentity, err := services.DevelopmentAcceptanceProfileIdentity()
	if err != nil {
		return "", err
	}
	requirements, err := migrations.Requirements()
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	for _, requirement := range requirements {
		_, _ = fmt.Fprintf(hash, "%s\x00%s\x00", requirement.Version, requirement.Checksum)
	}
	return fmt.Sprintf("%s:complete:%s:%s:%s:%x", developmentDatabaseMarkerNamespace, clusterID, nonce, profileIdentity, hash.Sum(nil)), nil
}

func validCompletedDevelopmentDatabaseMarker(marker string) bool {
	parsed, err := parseDevelopmentDatabaseMarker(marker)
	return err == nil && parsed.Phase == "complete"
}

func parseDevelopmentDatabaseMarker(raw string) (developmentDatabaseMarker, error) {
	parts := strings.Split(raw, ":")
	if len(parts) < 4 || parts[0] != developmentDatabaseMarkerNamespace {
		return developmentDatabaseMarker{}, fmt.Errorf("凭证命名空间不匹配")
	}
	marker := developmentDatabaseMarker{Raw: raw, Phase: parts[1], ClusterID: parts[2], Nonce: parts[3]}
	if _, err := strconv.ParseUint(marker.ClusterID, 10, 64); err != nil {
		return developmentDatabaseMarker{}, fmt.Errorf("凭证集群身份无效")
	}
	nonce, err := hex.DecodeString(marker.Nonce)
	if err != nil || len(nonce) != 16 {
		return developmentDatabaseMarker{}, fmt.Errorf("凭证随机值无效")
	}
	switch marker.Phase {
	case "initializing":
		if len(parts) != 4 {
			return developmentDatabaseMarker{}, fmt.Errorf("初始化凭证格式无效")
		}
	case "complete":
		if len(parts) != 7 || strings.TrimSpace(parts[4]) == "" {
			return developmentDatabaseMarker{}, fmt.Errorf("完成凭证格式无效")
		}
		for _, digest := range []string{parts[5], parts[6]} {
			decoded, err := hex.DecodeString(digest)
			if err != nil || len(decoded) != sha256.Size {
				return developmentDatabaseMarker{}, fmt.Errorf("完成凭证摘要无效")
			}
		}
	default:
		return developmentDatabaseMarker{}, fmt.Errorf("凭证阶段无效")
	}
	return marker, nil
}

func validateDevelopmentTarget(cfg *config.Configuration, requireFixtureWriteOptIn bool) error {
	if cfg == nil {
		return fmt.Errorf("配置不可用")
	}
	if strings.ToLower(strings.TrimSpace(cfg.Server.Mode)) != "debug" {
		return fmt.Errorf("dev-bootstrap 仅允许 debug 模式")
	}
	if requireFixtureWriteOptIn && !cfg.Server.SeedExperienceAccounts {
		return fmt.Errorf("必须显式设置 BACKEND_SEED_EXPERIENCE_ACCOUNTS=true")
	}
	if cfg.Server.SeedDeterministicLotteryHistory {
		return fmt.Errorf("本地首次初始化不会生成模拟开奖历史")
	}
	host := strings.TrimSpace(cfg.Database.Host)
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("dev-bootstrap 只允许连接本机 PostgreSQL")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Database.DBName)) {
	case "", "postgres", "template0", "template1":
		return fmt.Errorf("dev-bootstrap 必须使用独立的应用数据库")
	}
	return nil
}
