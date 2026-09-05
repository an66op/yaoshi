package config

import (
	"backend/constants"
	"backend/migrations"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const LocalDevelopmentDatabaseMarkerNamespace = "wangzhe-local-development-v1"

// ConnectDB is the only application database bootstrap. The project has not
// shipped with a pre-versioned schema, so startup always applies the checked-in
// SQL inventory and never infers schema changes from Go models.
func ConnectDB() (*gorm.DB, error) {
	db, err := OpenDatabase()
	if err != nil {
		return nil, err
	}
	if err := EnsureDatabaseInitializationComplete(db); err != nil {
		return nil, err
	}
	if err := migrations.Run(db); err != nil {
		return nil, fmt.Errorf("%s: %w", constants.ErrDatabaseMigrationFailed, err)
	}

	log.Println(constants.DatabaseConnectionSuccess)
	return db, nil
}

// EnsureDatabaseInitializationComplete runs before migrations so an ordinary
// backend cannot mutate a database while explicit local initialization is
// still incomplete. A brand-new loopback debug database must also enter
// through local-init: this closes the short createdb-to-COMMENT window even
// when a developer bypasses local-dev and starts the Go process directly.
func EnsureDatabaseInitializationComplete(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("数据库连接不可用")
	}
	var marker string
	if err := db.Raw(`
		SELECT COALESCE(shobj_description(oid, 'pg_database'), '')
		FROM pg_database WHERE datname = current_database()
	`).Scan(&marker).Error; err != nil {
		return fmt.Errorf("读取数据库初始化状态失败: %w", err)
	}
	if strings.HasPrefix(marker, LocalDevelopmentDatabaseMarkerNamespace+":initializing:") {
		return fmt.Errorf("本地数据库初始化尚未完成；请先重新运行 make dev-init")
	}
	if strings.HasPrefix(marker, LocalDevelopmentDatabaseMarkerNamespace+":") &&
		!strings.HasPrefix(marker, LocalDevelopmentDatabaseMarkerNamespace+":complete:") {
		return fmt.Errorf("本地数据库初始化状态无效；请停止服务并检查初始化凭证")
	}
	cfg := GetConfig()
	if marker == "" && requiresExplicitLocalDevelopmentInitialization(cfg) {
		var migrationsTableExists bool
		if err := db.Raw(`SELECT to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&migrationsTableExists).Error; err != nil {
			return fmt.Errorf("检查本地数据库迁移状态失败: %w", err)
		}
		if !migrationsTableExists {
			return fmt.Errorf("本地 debug 空数据库必须先执行 make dev-init；普通后端不会接管未初始化数据库")
		}
	}
	return nil
}

func requiresExplicitLocalDevelopmentInitialization(cfg *Configuration) bool {
	if cfg == nil || !strings.EqualFold(strings.TrimSpace(cfg.Server.Mode), "debug") {
		return false
	}
	host := strings.TrimSpace(cfg.Database.Host)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// OpenDatabase opens and configures PostgreSQL without changing its schema.
// Normal application code should use ConnectDB; explicit local administrative
// commands may use OpenDatabase when they must operate only after migrations
// have already been applied.
func OpenDatabase() (*gorm.DB, error) {
	config := GetConfig()
	dsn, err := BuildPostgresDSN(config.Database)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", constants.ErrDatabaseConnectionFailed, err)
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: newDatabaseLogger(log.New(os.Stdout, "\r\n", log.LstdFlags)),
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", constants.ErrDatabaseConnectionFailed, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", constants.ErrDatabaseConnectionFailed, err)
	}
	sqlDB.SetMaxIdleConns(config.Database.MaxIdleConns)
	sqlDB.SetMaxOpenConns(config.Database.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Duration(config.Database.ConnMaxLifetimeSeconds) * time.Second)

	return db, nil
}

func newDatabaseLogger(writer gormlogger.Writer) gormlogger.Interface {
	return gormlogger.New(writer, gormlogger.Config{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  gormlogger.Warn,
		IgnoreRecordNotFoundError: true,
		Colorful:                  false,
	})
}

// BuildPostgresDSN returns a pgx-compatible URL. URL encoding preserves every
// credential byte and the public search_path pins both migrations and runtime
// queries to the single application schema.
func BuildPostgresDSN(cfg DatabaseConfig) (string, error) {
	host := strings.TrimSpace(cfg.Host)
	username := cfg.User
	database := cfg.DBName
	sslMode := strings.TrimSpace(cfg.SSLMode)
	if host == "" || strings.TrimSpace(username) == "" || strings.TrimSpace(database) == "" {
		return "", fmt.Errorf("数据库主机、用户和库名不能为空")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return "", fmt.Errorf("数据库端口必须在1-65535之间")
	}

	connectionURL := &url.URL{
		Scheme: "postgresql",
		User:   url.UserPassword(username, cfg.Password),
		Path:   "/" + database,
	}
	query := url.Values{
		"sslmode":     []string{sslMode},
		"search_path": []string{"public"},
	}
	if strings.HasPrefix(host, "/") {
		query.Set("host", host)
		query.Set("port", strconv.Itoa(cfg.Port))
	} else {
		connectionURL.Host = net.JoinHostPort(host, strconv.Itoa(cfg.Port))
	}
	connectionURL.RawQuery = query.Encode()
	return connectionURL.String(), nil
}
