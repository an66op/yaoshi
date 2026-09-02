package config

import (
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestBuildPostgresDSNPreservesSpecialCharacters(t *testing.T) {
	input := DatabaseConfig{
		Host: "2001:db8::5", Port: 5432, User: " wang zhe ",
		Password: "p a'ss\\word=?&#@:%", DBName: " wang zhe ", SSLMode: "verify-full",
	}
	dsn, err := BuildPostgresDSN(input)
	if err != nil {
		t.Fatalf("BuildPostgresDSN() error = %v", err)
	}
	parsed, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("pgx.ParseConfig() error = %v (dsn redacted)", err)
	}
	if parsed.Host != input.Host || parsed.Port != uint16(input.Port) || parsed.User != input.User || parsed.Password != input.Password || parsed.Database != input.DBName {
		t.Fatalf("parsed connection fields changed: host=%q port=%d user=%q database=%q", parsed.Host, parsed.Port, parsed.User, parsed.Database)
	}
	if parsed.RuntimeParams["search_path"] != "public" {
		t.Fatal("application queries must use the versioned public schema")
	}
}

func TestBuildPostgresDSNSupportsUnixSocket(t *testing.T) {
	input := DatabaseConfig{Host: "/var/run/postgresql", Port: 5433, User: "backend", Password: "S3cure! value", DBName: "wangzhe", SSLMode: "disable"}
	dsn, err := BuildPostgresDSN(input)
	if err != nil {
		t.Fatalf("BuildPostgresDSN() error = %v", err)
	}
	parsed, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("pgx.ParseConfig() error = %v", err)
	}
	if parsed.Host != input.Host || parsed.Port != uint16(input.Port) || parsed.Password != input.Password {
		t.Fatalf("unexpected unix socket config: host=%q port=%d", parsed.Host, parsed.Port)
	}
	if parsed.RuntimeParams["search_path"] != "public" {
		t.Fatal("Unix-socket connections must use the versioned public schema")
	}
}
