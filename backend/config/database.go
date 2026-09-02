package config

import (
	"backend/constants"
	"backend/migrations"
	"fmt"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ConnectDB is the only application database bootstrap. The project has not
// shipped with a pre-versioned schema, so startup always applies the checked-in
// SQL inventory and never infers schema changes from Go models.
func ConnectDB() (*gorm.DB, error) {
	db, err := OpenDatabase()
	if err != nil {
		return nil, err
	}
	if err := migrations.Run(db); err != nil {
		return nil, fmt.Errorf("%s: %w", constants.ErrDatabaseMigrationFailed, err)
	}

	log.Println(constants.DatabaseConnectionSuccess)
	return db, nil
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

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", constants.ErrDatabaseConnectionFailed, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", constants.ErrDatabaseConnectionFailed, err)
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return db, nil
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
