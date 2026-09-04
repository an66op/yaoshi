package main

import (
	"backend/config"
	"strings"
	"testing"
)

func validDevelopmentConfig() *config.Configuration {
	return &config.Configuration{
		Server:   config.ServerConfig{Mode: "debug", SeedExperienceAccounts: true},
		Database: config.DatabaseConfig{Host: "127.0.0.1", Port: 5432, User: "postgres", DBName: "wangzhe_dev", SSLMode: "disable"},
	}
}

func TestCompletedDevelopmentDatabaseMarkerContract(t *testing.T) {
	clusterID := "7679044470357611185"
	nonce := strings.Repeat("a", 32)
	marker, err := developmentDatabaseCompleteMarker(clusterID, nonce)
	if err != nil {
		t.Fatal(err)
	}
	if !validCompletedDevelopmentDatabaseMarker(marker) {
		t.Fatalf("generated marker is invalid: %q", marker)
	}
	if !strings.HasPrefix(marker, developmentDatabaseMarkerNamespace+":complete:"+clusterID+":"+nonce+":development-acceptance-odds-v1:") {
		t.Fatalf("generated marker has unexpected identity: %q", marker)
	}
	initializing := developmentDatabaseMarkerNamespace + ":initializing:" + clusterID + ":" + nonce
	parsed, err := parseDevelopmentDatabaseMarker(initializing)
	if err != nil || parsed.Phase != "initializing" || parsed.ClusterID != clusterID || parsed.Nonce != nonce {
		t.Fatalf("initializing marker did not round-trip: parsed=%+v err=%v", parsed, err)
	}
	for _, invalid := range []string{
		"", developmentDatabaseMarkerNamespace + ":initializing",
		developmentDatabaseMarkerNamespace + ":complete:" + clusterID + ":" + nonce + ":profile:not-a-hash:not-a-hash",
		"another-project:complete:" + clusterID + ":" + nonce + ":profile:" + strings.Repeat("0", 64) + ":" + strings.Repeat("0", 64),
	} {
		if validCompletedDevelopmentDatabaseMarker(invalid) {
			t.Fatalf("invalid marker accepted: %q", invalid)
		}
	}
}

func TestValidateDevelopmentTarget(t *testing.T) {
	if err := validateDevelopmentTarget(validDevelopmentConfig(), true); err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*config.Configuration){
		"release":       func(cfg *config.Configuration) { cfg.Server.Mode = "release" },
		"implicit seed": func(cfg *config.Configuration) { cfg.Server.SeedExperienceAccounts = false },
		"fake draws":    func(cfg *config.Configuration) { cfg.Server.SeedDeterministicLotteryHistory = true },
		"remote host":   func(cfg *config.Configuration) { cfg.Database.Host = "db.example.com" },
		"system db":     func(cfg *config.Configuration) { cfg.Database.DBName = "postgres" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := validDevelopmentConfig()
			mutate(cfg)
			if err := validateDevelopmentTarget(cfg, true); err == nil {
				t.Fatal("unsafe development target accepted")
			}
		})
	}
	cfg := validDevelopmentConfig()
	cfg.Server.SeedExperienceAccounts = false
	if err := validateDevelopmentTarget(cfg, false); err != nil {
		t.Fatalf("read-only audit incorrectly requires fixture write opt-in: %v", err)
	}
}
