package main

import (
	"backend/config"
	usermodel "backend/data/models/user"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestRandomPublicIDAllocatorPostgres exercises the real PostgreSQL default.
// It rolls its transaction back so a verification run never leaves test
// accounts behind. It is opt-in because the normal unit-test suite must not
// require a developer database.
func TestRandomPublicIDAllocatorPostgres(t *testing.T) {
	if os.Getenv("BACKEND_POSTGRES_INTEGRATION") != "1" {
		t.Skip("set BACKEND_POSTGRES_INTEGRATION=1 to run PostgreSQL integration tests")
	}

	db, err := config.ConnectDB()
	if err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()

	prefix := fmt.Sprintf("public_id_check_%d_", time.Now().UnixNano())

	const accountCount = 32
	seen := make(map[uint64]struct{}, accountCount)
	for index := 0; index < accountCount; index++ {
		account := usermodel.User{
			Username:   fmt.Sprintf("%s%02d", prefix, index),
			LoginScope: "platform",
			Password:   "integration-test-only",
			Role:       "member",
			Status:     0,
		}
		if err := tx.Create(&account).Error; err != nil {
			t.Fatalf("create integration account %d: %v", index, err)
		}
		if account.PublicID < 1_000_000 || account.PublicID > 9_999_999 {
			t.Fatalf("public ID %d is not seven digits", account.PublicID)
		}
		if _, exists := seen[account.PublicID]; exists {
			t.Fatalf("duplicate public ID %d", account.PublicID)
		}
		seen[account.PublicID] = struct{}{}
	}
	if len(seen) != accountCount {
		t.Fatalf("created %d unique public IDs, want %d", len(seen), accountCount)
	}
}
