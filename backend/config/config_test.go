package config

import (
	"net/url"
	"testing"
)

func TestBuildPostgresDSNPinsPublicSearchPath(t *testing.T) {
	dsn, err := BuildPostgresDSN(DatabaseConfig{
		Host: "127.0.0.1", Port: 5432, User: "lottery user",
		Password: "password with spaces&symbols", DBName: "lottery_dev", SSLMode: "disable",
	})
	if err != nil {
		t.Fatalf("BuildPostgresDSN() error = %v", err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	if got := parsed.Query().Get("search_path"); got != "public" {
		t.Fatalf("search_path = %q, want public", got)
	}
	if got := parsed.Query().Get("sslmode"); got != "disable" {
		t.Fatalf("sslmode = %q, want disable", got)
	}
	if parsed.User.Username() != "lottery user" {
		t.Fatalf("username was not URL-safe: %q", parsed.User.Username())
	}
	password, ok := parsed.User.Password()
	if !ok || password != "password with spaces&symbols" {
		t.Fatal("password was not preserved by URL encoding")
	}
}

func TestExplicitLocalDevelopmentInitializationBoundary(t *testing.T) {
	for _, test := range []struct {
		name string
		mode string
		host string
		want bool
	}{
		{name: "local debug hostname", mode: "debug", host: "localhost", want: true},
		{name: "local debug ipv4", mode: "debug", host: "127.0.0.1", want: true},
		{name: "local debug ipv6", mode: "debug", host: "::1", want: true},
		{name: "remote debug", mode: "debug", host: "db.example.test", want: false},
		{name: "local test", mode: "test", host: "127.0.0.1", want: false},
		{name: "local release", mode: "release", host: "127.0.0.1", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := &Configuration{Server: ServerConfig{Mode: test.mode}, Database: DatabaseConfig{Host: test.host}}
			if got := requiresExplicitLocalDevelopmentInitialization(cfg); got != test.want {
				t.Fatalf("requiresExplicitLocalDevelopmentInitialization() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestOriginAllowed(t *testing.T) {
	allowed := []string{"http://localhost:5173", "https://example.com"}
	for _, origin := range []string{"http://localhost:5173", "https://example.com", ""} {
		if !OriginAllowed(origin, allowed, "release") {
			t.Fatalf("expected %q to be allowed", origin)
		}
	}
	for _, origin := range []string{"https://evil.example", "http://localhost:5173/path", "ftp://localhost:5173"} {
		if OriginAllowed(origin, allowed, "release") {
			t.Fatalf("expected %q to be denied", origin)
		}
	}
}

func validTestConfig(mode string) *Configuration {
	return &Configuration{
		Server: ServerConfig{
			Bind:           "127.0.0.1",
			Port:           8080,
			Mode:           mode,
			AllowedOrigins: []string{"https://app.example.test"},
			TrustedProxies: []string{"127.0.0.1"},
		},
		Database: DatabaseConfig{
			Host: "127.0.0.1", Port: 5432, User: "lottery",
			Password: "random-database-password-for-tests", DBName: "lottery", SSLMode: "disable",
		},
		Redis: RedisConfig{
			Addr: "127.0.0.1:6379", Username: "wangzhe-app", Password: "redis-password-longer-than-24", Prefix: "wangzhe-test",
		},
		JWT:      JWTConfig{Secret: "random-jwt-secret-that-is-longer-than-32-bytes", Expire: 86400},
		Security: SecurityConfig{DataEncryptionKey: "random-data-key-that-is-longer-than-32-bytes"},
	}
}

func TestValidateConfigRequiresRedisInRelease(t *testing.T) {
	cfg := validTestConfig("release")
	cfg.Redis.Addr = ""
	if err := validateConfig(cfg); err == nil {
		t.Fatal("release config accepted an empty Redis address")
	}

	cfg = validTestConfig("release")
	cfg.Redis.Username = ""
	if err := validateConfig(cfg); err == nil {
		t.Fatal("release config accepted an empty Redis ACL username")
	}

	cfg = validTestConfig("release")
	cfg.Redis.Username = "default"
	if err := validateConfig(cfg); err == nil {
		t.Fatal("release config accepted Redis default ACL user")
	}

	cfg = validTestConfig("release")
	cfg.Redis.Password = "too-short"
	if err := validateConfig(cfg); err == nil {
		t.Fatal("release config accepted a short Redis ACL password")
	}
}

func TestValidateConfigRejectsReleasePlaceholders(t *testing.T) {
	cfg := validTestConfig("release")
	cfg.JWT.Secret = "REPLACE_WITH_A_RANDOM_SECRET_AT_LEAST_16_CHARACTERS"
	if err := validateConfig(cfg); err == nil {
		t.Fatal("release config accepted the sample JWT secret")
	}

	cfg = validTestConfig("release")
	cfg.Database.Password = "CHANGE_ME"
	if err := validateConfig(cfg); err == nil {
		t.Fatal("release config accepted the sample database password")
	}

	cfg = validTestConfig("release")
	cfg.Security.DataEncryptionKey = "CHANGE_ME"
	if err := validateConfig(cfg); err == nil {
		t.Fatal("release config accepted the sample data encryption key")
	}

	cfg = validTestConfig("release")
	cfg.JWT.Secret = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := validateConfig(cfg); err == nil {
		t.Fatal("release config accepted a low-variety JWT secret")
	}

	cfg = validTestConfig("release")
	cfg.Security.DataEncryptionKey = "abababababababababababababababab"
	if err := validateConfig(cfg); err == nil {
		t.Fatal("release config accepted a low-variety data encryption key")
	}

	cfg = validTestConfig("release")
	cfg.Database.Password = cfg.JWT.Secret
	if err := validateConfig(cfg); err == nil {
		t.Fatal("release config accepted a database password reused as the JWT secret")
	}

	cfg = validTestConfig("release")
	cfg.Database.Password = "weak-password"
	if err := validateConfig(cfg); err == nil {
		t.Fatal("release config accepted a short database password")
	}
}

func TestValidateConfigRejectsInvalidOrigins(t *testing.T) {
	cfg := validTestConfig("debug")
	cfg.Server.AllowedOrigins = []string{"https://app.example.test/path"}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("config accepted a CORS origin with a path")
	}
}

func TestValidateConfigRejectsInvalidModeInsteadOfFallingBack(t *testing.T) {
	cfg := validTestConfig("production")
	if err := validateConfig(cfg); err == nil {
		t.Fatal("config silently accepted an invalid server mode")
	}
}

func TestExperienceAccountSeedConfigurationIsTypedAndDebugOnly(t *testing.T) {
	previous := Config
	t.Cleanup(func() { Config = previous })

	Config = validTestConfig("debug")
	if Config.Server.SeedExperienceAccounts {
		t.Fatal("experience account seed must default to false")
	}
	t.Setenv("BACKEND_SEED_EXPERIENCE_ACCOUNTS", "true")
	if err := loadFromEnv(); err != nil {
		t.Fatalf("load typed experience seed setting: %v", err)
	}
	if !Config.Server.SeedExperienceAccounts {
		t.Fatal("explicit true did not enable the experience account seed")
	}
	if err := validateConfig(Config); err != nil {
		t.Fatalf("debug configuration rejected explicit experience seed: %v", err)
	}

	for _, mode := range []string{"test", "release"} {
		cfg := validTestConfig(mode)
		cfg.Server.SeedExperienceAccounts = true
		if err := validateConfig(cfg); err == nil {
			t.Fatalf("%s configuration accepted the debug-only experience seed", mode)
		}
	}

	Config = validTestConfig("debug")
	t.Setenv("BACKEND_SEED_EXPERIENCE_ACCOUNTS", "sometimes")
	if err := loadFromEnv(); err == nil {
		t.Fatal("invalid experience seed boolean was silently accepted")
	}
}

func TestDatabasePasswordEnvironmentCanExplicitlyBeEmpty(t *testing.T) {
	previous := Config
	t.Cleanup(func() { Config = previous })
	Config = validTestConfig("debug")
	t.Setenv("BACKEND_DATABASE_PASSWORD", "")
	if err := loadFromEnv(); err != nil {
		t.Fatal(err)
	}
	if Config.Database.Password != "" {
		t.Fatal("explicit empty local database password was ignored")
	}
}

func TestDeterministicLotteryHistorySeedConfigurationIsTypedAndDebugOnly(t *testing.T) {
	previous := Config
	t.Cleanup(func() { Config = previous })

	Config = validTestConfig("debug")
	if Config.Server.SeedDeterministicLotteryHistory {
		t.Fatal("deterministic lottery history seed must default to false")
	}
	t.Setenv("BACKEND_SEED_DETERMINISTIC_LOTTERY_HISTORY", "true")
	if err := loadFromEnv(); err != nil {
		t.Fatalf("load typed deterministic lottery history setting: %v", err)
	}
	if !Config.Server.SeedDeterministicLotteryHistory {
		t.Fatal("explicit true did not enable deterministic lottery history")
	}
	if err := validateConfig(Config); err != nil {
		t.Fatalf("debug configuration rejected explicit deterministic lottery history: %v", err)
	}

	for _, mode := range []string{"test", "release"} {
		cfg := validTestConfig(mode)
		cfg.Server.SeedDeterministicLotteryHistory = true
		if err := validateConfig(cfg); err == nil {
			t.Fatalf("%s configuration accepted debug-only deterministic lottery history", mode)
		}
	}

	Config = validTestConfig("debug")
	t.Setenv("BACKEND_SEED_DETERMINISTIC_LOTTERY_HISTORY", "sometimes")
	if err := loadFromEnv(); err == nil {
		t.Fatal("invalid deterministic lottery history boolean was silently accepted")
	}
}

func TestValidateConfigReleaseTransportBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Configuration)
	}{
		{name: "http origin", mutate: func(cfg *Configuration) { cfg.Server.AllowedOrigins = []string{"http://app.example.test"} }},
		{name: "missing proxy", mutate: func(cfg *Configuration) { cfg.Server.TrustedProxies = nil }},
		{name: "trust all proxy", mutate: func(cfg *Configuration) { cfg.Server.TrustedProxies = []string{"0.0.0.0/0"} }},
		{name: "disguised trust all proxy", mutate: func(cfg *Configuration) { cfg.Server.TrustedProxies = []string{"203.0.113.7/0"} }},
		{name: "overbroad ipv4 proxy", mutate: func(cfg *Configuration) { cfg.Server.TrustedProxies = []string{"128.0.0.0/1"} }},
		{name: "overbroad ipv6 proxy", mutate: func(cfg *Configuration) { cfg.Server.TrustedProxies = []string{"8000::/1"} }},
		{name: "remote database without verified TLS", mutate: func(cfg *Configuration) { cfg.Database.Host = "db.example.test"; cfg.Database.SSLMode = "require" }},
		{name: "reused application keys", mutate: func(cfg *Configuration) { cfg.Security.DataEncryptionKey = cfg.JWT.Secret }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validTestConfig("release")
			cfg.Server.TrustedProxies = []string{"127.0.0.1"}
			cfg.Database.SSLMode = "disable"
			tt.mutate(cfg)
			if err := validateConfig(cfg); err == nil {
				t.Fatal("release config accepted an unsafe transport boundary")
			}
		})
	}
}

func TestValidateConfigAllowsLocalDatabaseWithoutTLS(t *testing.T) {
	cfg := validTestConfig("release")
	cfg.Server.TrustedProxies = []string{"127.0.0.1", "::1"}
	cfg.Database.SSLMode = "disable"
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("local release database should be allowed without TLS: %v", err)
	}
}

func TestOriginAllowedDevelopmentFallback(t *testing.T) {
	if !OriginAllowed("http://127.0.0.1:5173", nil, "debug") {
		t.Fatal("local development origin should be allowed without explicit config")
	}
	if OriginAllowed("https://evil.example", nil, "debug") {
		t.Fatal("non-local origin must not be allowed by the development fallback")
	}
	if !OriginAllowed("http://192.168.31.99:5173", []string{"http://localhost:5173"}, "debug") {
		t.Fatal("private LAN member origin should be allowed in debug mode")
	}
	if !OriginAllowed("http://10.0.0.8:5174", []string{"http://localhost:5174"}, "debug") {
		t.Fatal("private LAN admin origin should be allowed in debug mode")
	}
	if OriginAllowed("http://192.168.31.99:5173", []string{"http://localhost:5173"}, "release") {
		t.Fatal("private LAN fallback must remain disabled in release mode")
	}
}

func TestOriginAllowedRejectsUserInfo(t *testing.T) {
	if OriginAllowed("https://operator:secret@app.example.test", []string{"https://app.example.test"}, "release") {
		t.Fatal("origin containing URL user info must not normalize to an allowed origin")
	}
}
