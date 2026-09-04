package config

import (
	"backend/constants"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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
	Bind                            string   `mapstructure:"bind"`
	Port                            int      `mapstructure:"port"`
	Mode                            string   `mapstructure:"mode"`
	SeedExperienceAccounts          bool     `mapstructure:"seed_experience_accounts"`
	SeedDeterministicLotteryHistory bool     `mapstructure:"seed_deterministic_lottery_history"`
	AllowedOrigins                  []string `mapstructure:"allowed_origins"`
	TrustedProxies                  []string `mapstructure:"trusted_proxies"`
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
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	TLS      bool   `mapstructure:"tls"`
	Prefix   string `mapstructure:"prefix"`
}

// JWTConfig JWT配置
type JWTConfig struct {
	Secret string `mapstructure:"secret"`
	// Expire is the token lifetime in seconds.
	Expire int `mapstructure:"expire"`
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

	// 从环境变量覆盖配置（如果存在）。无效的显式值必须让进程停止，
	// 不能悄悄回退到配置文件中的开发端口或过期时间。
	if err := loadFromEnv(); err != nil {
		log.Fatalf("环境变量配置无效: %v", err)
	}

	// 验证配置
	if err := validateConfig(Config); err != nil {
		log.Fatalf("配置验证失败: %v", err)
	}
}

// loadFromEnv 从环境变量加载配置
func loadFromEnv() error {
	// Server配置
	if bind := os.Getenv("BACKEND_SERVER_BIND"); bind != "" {
		Config.Server.Bind = strings.TrimSpace(bind)
	}
	if port := os.Getenv("BACKEND_SERVER_PORT"); port != "" {
		p, err := strconv.Atoi(port)
		if err != nil {
			return fmt.Errorf("BACKEND_SERVER_PORT 必须是整数")
		}
		Config.Server.Port = p
	}
	if mode := os.Getenv("BACKEND_SERVER_MODE"); mode != "" {
		Config.Server.Mode = mode
	}
	if seedValue, exists := os.LookupEnv("BACKEND_SEED_EXPERIENCE_ACCOUNTS"); exists {
		enabled, err := strconv.ParseBool(strings.TrimSpace(seedValue))
		if err != nil {
			return fmt.Errorf("BACKEND_SEED_EXPERIENCE_ACCOUNTS 必须是 true 或 false")
		}
		Config.Server.SeedExperienceAccounts = enabled
	}
	if seedValue, exists := os.LookupEnv("BACKEND_SEED_DETERMINISTIC_LOTTERY_HISTORY"); exists {
		enabled, err := strconv.ParseBool(strings.TrimSpace(seedValue))
		if err != nil {
			return fmt.Errorf("BACKEND_SEED_DETERMINISTIC_LOTTERY_HISTORY 必须是 true 或 false")
		}
		Config.Server.SeedDeterministicLotteryHistory = enabled
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
		p, err := strconv.Atoi(port)
		if err != nil {
			return fmt.Errorf("BACKEND_DATABASE_PORT 必须是整数")
		}
		Config.Database.Port = p
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
	if username := os.Getenv("BACKEND_REDIS_USERNAME"); username != "" {
		Config.Redis.Username = strings.TrimSpace(username)
	}
	if password := os.Getenv("BACKEND_REDIS_PASSWORD"); password != "" {
		Config.Redis.Password = password
	}
	if db := os.Getenv("BACKEND_REDIS_DB"); db != "" {
		d, err := strconv.Atoi(db)
		if err != nil {
			return fmt.Errorf("BACKEND_REDIS_DB 必须是整数")
		}
		Config.Redis.DB = d
	}
	if tlsValue := os.Getenv("BACKEND_REDIS_TLS"); tlsValue != "" {
		enabled, err := strconv.ParseBool(tlsValue)
		if err != nil {
			return fmt.Errorf("BACKEND_REDIS_TLS 必须是 true 或 false")
		}
		Config.Redis.TLS = enabled
	}
	if prefix := os.Getenv("BACKEND_REDIS_PREFIX"); prefix != "" {
		Config.Redis.Prefix = strings.TrimSpace(prefix)
	}

	// JWT配置
	if secret := os.Getenv("BACKEND_JWT_SECRET"); secret != "" {
		Config.JWT.Secret = secret
	}
	if expire := os.Getenv("BACKEND_JWT_EXPIRE"); expire != "" {
		e, err := strconv.Atoi(expire)
		if err != nil {
			return fmt.Errorf("BACKEND_JWT_EXPIRE 必须是整数秒")
		}
		Config.JWT.Expire = e
	}
	if key := os.Getenv("BACKEND_SECURITY_DATA_ENCRYPTION_KEY"); key != "" {
		Config.Security.DataEncryptionKey = key
	}
	return nil
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
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// validateConfig 验证配置
func validateConfig(cfg *Configuration) error {
	// 验证Server配置
	if net.ParseIP(strings.TrimSpace(cfg.Server.Bind)) == nil {
		return fmt.Errorf("服务器监听地址必须是明确的 IP，当前值: %q", cfg.Server.Bind)
	}
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("服务器端口必须在1-65535之间，当前值: %d", cfg.Server.Port)
	}
	if cfg.Server.Mode != "debug" && cfg.Server.Mode != "release" && cfg.Server.Mode != "test" {
		return fmt.Errorf("服务器模式必须是 debug/release/test，当前值: %s", cfg.Server.Mode)
	}
	if cfg.Server.SeedExperienceAccounts && cfg.Server.Mode != "debug" {
		return fmt.Errorf("BACKEND_SEED_EXPERIENCE_ACCOUNTS 仅允许在 debug 模式启用")
	}
	if cfg.Server.SeedDeterministicLotteryHistory && cfg.Server.Mode != "debug" {
		return fmt.Errorf("BACKEND_SEED_DETERMINISTIC_LOTTERY_HISTORY 仅允许在 debug 模式启用")
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
	validSSLModes := map[string]bool{"disable": true, "allow": true, "prefer": true, "require": true, "verify-ca": true, "verify-full": true}
	if !validSSLModes[cfg.Database.SSLMode] {
		return fmt.Errorf("数据库 sslmode 无效: %q", cfg.Database.SSLMode)
	}
	for _, origin := range cfg.Server.AllowedOrigins {
		if normalizeOrigin(origin) == "" {
			return fmt.Errorf("无效的 CORS 来源: %q", origin)
		}
	}
	for _, proxy := range cfg.Server.TrustedProxies {
		if !validTrustedProxy(proxy) {
			return fmt.Errorf("无效或过宽的受信任代理: %q", proxy)
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
		if strings.TrimSpace(cfg.Redis.Addr) == "" {
			return fmt.Errorf("release 模式必须配置 Redis，用于多实例票据、限流、推送与调度锁")
		}
		if strings.TrimSpace(cfg.Redis.Username) == "" {
			return fmt.Errorf("release 模式必须配置独立 Redis ACL 用户")
		}
		if cfg.Redis.Username == "default" || !regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`).MatchString(cfg.Redis.Username) {
			return fmt.Errorf("release 模式 Redis ACL 用户名无效或仍在使用 default")
		}
		if len(cfg.Redis.Password) < 24 {
			return fmt.Errorf("release 模式 Redis ACL 密码至少需要 24 位")
		}
		if len(cfg.Server.AllowedOrigins) == 0 {
			return fmt.Errorf("release 模式必须显式配置 allowed_origins")
		}
		if len(cfg.JWT.Secret) < 32 || isPlaceholderSecret(cfg.JWT.Secret) || !hasSufficientSecretVariety(cfg.JWT.Secret) {
			return fmt.Errorf("release 模式必须配置至少32位的随机 JWT 密钥，不能使用示例或默认值")
		}
		if len(cfg.Database.Password) < 16 || isPlaceholderSecret(cfg.Database.Password) || !hasSufficientSecretVariety(cfg.Database.Password) {
			return fmt.Errorf("release 模式必须配置至少16位的随机数据库密码")
		}
		if len(cfg.Security.DataEncryptionKey) < 32 || isPlaceholderSecret(cfg.Security.DataEncryptionKey) || !hasSufficientSecretVariety(cfg.Security.DataEncryptionKey) {
			return fmt.Errorf("release 模式必须配置至少32位的随机数据加密密钥")
		}
		if cfg.JWT.Secret == cfg.Security.DataEncryptionKey {
			return fmt.Errorf("JWT 密钥与数据加密密钥必须独立生成")
		}
		if cfg.Database.Password == cfg.JWT.Secret || cfg.Database.Password == cfg.Security.DataEncryptionKey {
			return fmt.Errorf("数据库密码不得复用 JWT 或数据加密密钥")
		}
		if len(cfg.Server.TrustedProxies) == 0 {
			return fmt.Errorf("release 模式必须显式配置 trusted_proxies")
		}
		for _, origin := range cfg.Server.AllowedOrigins {
			if !strings.HasPrefix(normalizeOrigin(origin), "https://") {
				return fmt.Errorf("release 模式只允许 HTTPS CORS 来源: %q", origin)
			}
		}
		if !isLocalDatabaseHost(cfg.Database.Host) && cfg.Database.SSLMode != "verify-ca" && cfg.Database.SSLMode != "verify-full" {
			return fmt.Errorf("远程生产数据库必须使用 sslmode=verify-ca 或 verify-full")
		}
	}

	return nil
}

func validTrustedProxy(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "0.0.0.0/0" || value == "::/0" {
		return false
	}
	if net.ParseIP(value) != nil {
		return true
	}
	_, network, err := net.ParseCIDR(value)
	if err != nil {
		return false
	}
	ones, bits := network.Mask.Size()
	// Inspect the parsed mask rather than the raw string so values such as
	// 203.0.113.7/0 cannot disguise an all-address proxy range. Extremely broad
	// networks are unsafe too: trusting them lets arbitrary clients forge the
	// forwarded address used by audit logs and rate limits. /8 and /32 still
	// permit conventional private IPv4 and IPv6 load-balancer networks.
	if bits == net.IPv4len*8 {
		return ones >= 8
	}
	return bits == net.IPv6len*8 && ones >= 32
}

func isLocalDatabaseHost(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "localhost" || strings.HasPrefix(value, "/") {
		return true
	}
	ip := net.ParseIP(value)
	return ip != nil && ip.IsLoopback()
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

// hasSufficientSecretVariety is a deployment guardrail, not an entropy
// estimator. It rejects obviously repeated/pattern-like values that satisfy a
// length check while providing almost no effective key space.
func hasSufficientSecretVariety(value string) bool {
	distinct := make(map[rune]struct{}, 8)
	for _, char := range value {
		distinct[char] = struct{}{}
		if len(distinct) >= 8 {
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
