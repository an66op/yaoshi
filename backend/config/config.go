package config

import (
	"backend/constants"
	"fmt"
	"log"
	"net"
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
	Security SecurityConfig `mapstructure:"security"`
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

type SecurityConfig struct {
	DataEncryptionKey string `mapstructure:"data_encryption_key"`
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
	if key := os.Getenv("BACKEND_SECURITY_DATA_ENCRYPTION_KEY"); key != "" {
		Config.Security.DataEncryptionKey = key
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
	// 本地调试时允许同一局域网内的手机访问两个固定前端端口。
	// release 模式仍只接受配置文件中明确列出的来源。
	if mode != "release" {
		u, err := url.Parse(origin)
		if err == nil && (u.Port() == "5173" || u.Port() == "5174") {
			host := u.Hostname()
			ip := net.ParseIP(host)
			if host == "localhost" || ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
				return true
			}
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
	for _, origin := range cfg.Server.AllowedOrigins {
		if normalizeOrigin(origin) == "" {
			return fmt.Errorf("无效的 CORS 来源: %q", origin)
		}
	}

	// 验证JWT配置
	if len(cfg.JWT.Secret) < 16 {
		return fmt.Errorf("JWT密钥长度至少16个字符，当前长度: %d", len(cfg.JWT.Secret))
	}
	if cfg.JWT.Expire <= 0 {
		return fmt.Errorf("JWT过期时间必须大于0，当前值: %d", cfg.JWT.Expire)
	}
	if len(cfg.Security.DataEncryptionKey) < 16 {
		return fmt.Errorf("数据加密密钥长度至少16个字符，当前长度: %d", len(cfg.Security.DataEncryptionKey))
	}
	if cfg.Server.Mode == "release" {
		if len(cfg.Server.AllowedOrigins) == 0 {
			return fmt.Errorf("release 模式必须显式配置 allowed_origins")
		}
		if len(cfg.JWT.Secret) < 32 || isPlaceholderSecret(cfg.JWT.Secret) {
			return fmt.Errorf("release 模式必须配置至少32位的随机 JWT 密钥，不能使用示例或默认值")
		}
		if strings.TrimSpace(cfg.Database.Password) == "" || isPlaceholderSecret(cfg.Database.Password) {
			return fmt.Errorf("release 模式必须配置非默认数据库密码")
		}
		if len(cfg.Security.DataEncryptionKey) < 32 || isPlaceholderSecret(cfg.Security.DataEncryptionKey) {
			return fmt.Errorf("release 模式必须配置至少32位的随机数据加密密钥")
		}
	}

	return nil
}

func isPlaceholderSecret(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return true
	}
	for _, fragment := range []string{"change_me", "changeme", "replace_with", "example", "backend_jwt_secret_key_2024"} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	for _, exact := range []string{"123456", "password", "postgres", "secret"} {
		if normalized == exact {
			return true
		}
	}
	return false
}

// GetConfig 获取配置实例
func GetConfig() *Configuration {
	if Config == nil {
		LoadConfig()
	}
	return Config
}
