package config

import "testing"

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
			Port:           8080,
			Mode:           mode,
			AllowedOrigins: []string{"https://app.example.test"},
		},
		Database: DatabaseConfig{
			Host: "127.0.0.1", Port: 5432, User: "lottery",
			Password: "random-database-password-for-tests", DBName: "lottery",
		},
		JWT:      JWTConfig{Secret: "random-jwt-secret-that-is-longer-than-32-bytes", Expire: 86400},
		Security: SecurityConfig{DataEncryptionKey: "random-data-key-that-is-longer-than-32-bytes"},
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
}

func TestValidateConfigRejectsInvalidOrigins(t *testing.T) {
	cfg := validTestConfig("debug")
	cfg.Server.AllowedOrigins = []string{"https://app.example.test/path"}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("config accepted a CORS origin with a path")
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
