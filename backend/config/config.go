package config

import (
	"backend/constants"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

// Config 全局配置变量
var Config *Configuration

// Configuration 系统配置结构体
type Configuration struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	JWT      JWTConfig      `mapstructure:"jwt"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port           int      `mapstructure:"port"`
	Mode           string   `mapstructure:"mode"`
	AllowedOrigins []string `mapstructure:"allowed_origins"`
	TrustedProxies []string `mapstructure:"trusted_proxies"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

// RedisConfig Redis配置
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// JWTConfig JWT配置
type JWTConfig struct {
	Secret string `mapstructure:"secret"`
	Expire int    `mapstructure:"expire"`
}

// LoadConfig 加载配置文件
func LoadConfig() {
	// 设置配置文件名称
	viper.SetConfigName("config")

	// 获取当前工作目录
	workDir, err := os.Getwd()
	if err != nil {
		log.Fatal(constants.ErrGetWorkDirFailed, ":", err)
	}

	// 添加配置文件搜索路径
	viper.AddConfigPath(filepath.Join(workDir, "configs"))
	viper.AddConfigPath(filepath.Join(workDir, "config"))
	viper.AddConfigPath(".")

	// 设置配置文件类型
	viper.SetConfigType("yaml")

	// 支持环境变量
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// 设置环境变量前缀（可选）
	viper.SetEnvPrefix("BACKEND")

	// 读取配置文件（如果存在）
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("Warning: 读取配置文件失败，将使用环境变量和默认值: %v", err)
	}

	// 将配置信息解析到结构体中
	Config = &Configuration{}
	if err := viper.Unmarshal(Config); err != nil {
		log.Fatal(constants.ErrParseConfigFailed, ":", err)
	}

	// 从环境变量覆盖配置（如果存在）
	loadFromEnv()

	// 验证配置
	if err := validateConfig(Config); err != nil {
		log.Fatalf("配置验证失败: %v", err)
	}
}

// loadFromEnv 从环境变量加载配置
func loadFromEnv() {
	// Server配置
	if port := os.Getenv("BACKEND_SERVER_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			Config.Server.Port = p
		}
	}
	if mode := os.Getenv("BACKEND_SERVER_MODE"); mode != "" {
		Config.Server.Mode = mode
	}
	if origins := os.Getenv("BACKEND_SERVER_ALLOWED_ORIGINS"); origins != "" {
		Config.Server.AllowedOrigins = splitCSV(origins)
	}
	if proxies := os.Getenv("BACKEND_SERVER_TRUSTED_PROXIES"); proxies != "" {
		Config.Server.TrustedProxies = splitCSV(proxies)
	}

	// Database配置
	if host := os.Getenv("BACKEND_DATABASE_HOST"); host != "" {
		Config.Database.Host = host
	}
	if port := os.Getenv("BACKEND_DATABASE_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			Config.Database.Port = p
		}
	}
	if user := os.Getenv("BACKEND_DATABASE_USER"); user != "" {
		Config.Database.User = user
	}
	if password := os.Getenv("BACKEND_DATABASE_PASSWORD"); password != "" {
		Config.Database.Password = password
	}
	if dbname := os.Getenv("BACKEND_DATABASE_DBNAME"); dbname != "" {
		Config.Database.DBName = dbname
	}
	if sslmode := os.Getenv("BACKEND_DATABASE_SSLMODE"); sslmode != "" {
		Config.Database.SSLMode = sslmode
	}

	// Redis配置
	if addr := os.Getenv("BACKEND_REDIS_ADDR"); addr != "" {
		Config.Redis.Addr = addr
	}
	if password := os.Getenv("BACKEND_REDIS_PASSWORD"); password != "" {
		Config.Redis.Password = password
	}
	if db := os.Getenv("BACKEND_REDIS_DB"); db != "" {
		if d, err := strconv.Atoi(db); err == nil {
			Config.Redis.DB = d
		}
	}

	// JWT配置
	if secret := os.Getenv("BACKEND_JWT_SECRET"); secret != "" {
		Config.JWT.Secret = secret
	}
	if expire := os.Getenv("BACKEND_JWT_EXPIRE"); expire != "" {
		if e, err := strconv.Atoi(expire); err == nil {
			Config.JWT.Expire = e
		}
	}
}

func splitCSV(value string) []string {
	items := strings.Split(value, ",")
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

// IsOriginAllowed validates browser origins for CORS and WebSocket upgrades.
// Requests without an Origin header are non-browser requests and are allowed.
func IsOriginAllowed(origin string) bool {
	return OriginAllowed(origin, GetConfig().Server.AllowedOrigins, GetConfig().Server.Mode)
}

func OriginAllowed(origin string, allowed []string, mode string) bool {
	if strings.TrimSpace(origin) == "" {
		return true
	}
	origin = normalizeOrigin(origin)
	if origin == "" {
		return false
	}
	for _, candidate := range allowed {
		if origin == normalizeOrigin(candidate) {
			return true
		}
	}
	// A fresh local checkout remains usable before a config file is customized.
	if len(allowed) == 0 && mode != "release" {
		u, err := url.Parse(origin)
		return err == nil && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")
	}
	return false
}

func normalizeOrigin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// validateConfig 验证配置
func validateConfig(cfg *Configuration) error {
	// 验证Server配置
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("服务器端口必须在1-65535之间，当前值: %d", cfg.Server.Port)
	}
	if cfg.Server.Mode != "debug" && cfg.Server.Mode != "release" && cfg.Server.Mode != "test" {
		log.Printf("Warning: 服务器模式应该是 debug/release/test，当前值: %s，将使用默认值", cfg.Server.Mode)
		cfg.Server.Mode = "debug"
	}

	// 根据模式设置 Gin 模式
	switch cfg.Server.Mode {
	case "release":
		// 生产模式
	case "test":
		// 测试模式
	case "debug":
		// 开发模式
	default:
		cfg.Server.Mode = "debug"
	}

	// 验证Database配置
	if cfg.Database.Host == "" {
		return fmt.Errorf("数据库主机不能为空")
	}
	if cfg.Database.Port <= 0 || cfg.Database.Port > 65535 {
		return fmt.Errorf("数据库端口必须在1-65535之间，当前值: %d", cfg.Database.Port)
	}
	if cfg.Database.User == "" {
		return fmt.Errorf("数据库用户不能为空")
	}
	if cfg.Database.DBName == "" {
		return fmt.Errorf("数据库名称不能为空")
	}

	// 验证JWT配置
	if len(cfg.JWT.Secret) < 16 {
		return fmt.Errorf("JWT密钥长度至少16个字符，当前长度: %d", len(cfg.JWT.Secret))
	}
	if cfg.JWT.Expire <= 0 {
		return fmt.Errorf("JWT过期时间必须大于0，当前值: %d", cfg.JWT.Expire)
	}

	return nil
}

// GetConfig 获取配置实例
func GetConfig() *Configuration {
	if Config == nil {
		LoadConfig()
	}
	return Config
}
