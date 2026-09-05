package services

import (
	"backend/utils"
	"context"
	"testing"
	"time"
)

// This opt-in integration test uses the dedicated disposable database enforced
// by timingPostgresDatabase. It proves that raw inventory includes soft-deleted
// rows and that every rewrap UPDATE is a real PostgreSQL compare-and-swap.
func TestSensitiveFieldRewrapPostgresCoversAllColumnsSoftDeletesAndCAS(t *testing.T) {
	db := timingPostgresDatabase(t)
	if err := utils.InitFieldEncryption(sensitiveOldTestKey); err != nil {
		t.Fatal(err)
	}
	memberEnvelope := sensitiveV1ForServiceTest(t, sensitiveOldTestKey, "member account")
	if err := utils.InitFieldEncryption(sensitiveOldTestKey); err != nil {
		t.Fatal(err)
	}
	walletEnvelope, err := utils.EncryptSensitive("wallet secret")
	if err != nil {
		t.Fatal(err)
	}
	entertainmentEnvelope := sensitiveV1ForServiceTest(t, sensitiveOldTestKey, "entertainment secret")

	var workspaceID, userID uint64
	if err := db.Raw(`SELECT "id" FROM "workspaces" WHERE "type" = 'platform' ORDER BY "id" LIMIT 1`).Scan(&workspaceID).Error; err != nil || workspaceID == 0 {
		t.Fatalf("platform workspace: id=%d err=%v", workspaceID, err)
	}
	if err := db.Raw(`SELECT "id" FROM "user" WHERE "workspace_id" = ? ORDER BY "id" LIMIT 1`, workspaceID).Scan(&userID).Error; err != nil || userID == 0 {
		t.Fatalf("platform user: id=%d err=%v", userID, err)
	}
	deletedAt := time.Now().UTC()
	var memberID uint64
	if err := db.Raw(`
		INSERT INTO "member_payment_accounts"
		  ("workspace_id", "user_id", "account_type", "label", "account_name", "account_no", "is_default", "created_at", "updated_at", "deleted_at")
		VALUES (?, ?, 'bank', 'rewrap', 'rewrap', ?, false, ?, ?, ?)
		RETURNING "id"
	`, workspaceID, userID, memberEnvelope, deletedAt, deletedAt, deletedAt).Scan(&memberID).Error; err != nil {
		t.Fatal("insert soft-deleted member payment account:", err)
	}
	var walletID uint64
	if err := db.Raw(`
		INSERT INTO "wallet_payment_channels"
		  ("workspace_id", "provider", "name", "credit_type", "status", "mode", "secret_key", "created_at", "updated_at", "deleted_at")
		VALUES (?, 'rewrap', 'rewrap', 'bank', 'disabled', 'api', ?, ?, ?, ?)
		RETURNING "id"
	`, workspaceID, walletEnvelope, deletedAt, deletedAt, deletedAt).Scan(&walletID).Error; err != nil {
		t.Fatal("insert soft-deleted wallet channel:", err)
	}
	var entertainmentID uint64
	if err := db.Raw(`
		INSERT INTO "entertainment_platforms"
		  ("code", "name", "category", "secret_key", "status", "created_at", "updated_at")
		VALUES ('rewrap-test', 'rewrap', 'test', ?, 'disabled', ?, ?)
		RETURNING "id"
	`, entertainmentEnvelope, deletedAt, deletedAt).Scan(&entertainmentID).Error; err != nil {
		t.Fatal("insert entertainment platform:", err)
	}
	if memberID == 0 || walletID == 0 || entertainmentID == 0 {
		t.Fatal("rewrap fixtures did not receive primary keys")
	}

	if err := utils.InitFieldEncryptionWithFallbacks(sensitiveNewTestKey, []string{sensitiveOldTestKey}); err != nil {
		t.Fatal(err)
	}
	before, err := AuditSensitiveFieldReadiness(context.Background(), db)
	if err != nil || !before.Complete || before.Counts.PreviousKey != 3 ||
		before.Columns[0].Counts.PreviousKey != 1 || before.Columns[1].Counts.PreviousKey != 1 || before.Columns[2].Counts.PreviousKey != 1 {
		t.Fatalf("pre-rewrap inventory did not include all/soft-deleted rows: report=%+v err=%v", before, err)
	}
	dryRun, err := RewrapSensitiveFieldsFromPreviousKey(context.Background(), db, SensitiveFieldRewrapOptions{
		PreviousKeyIndex: 1, BatchSize: 1,
	})
	if err != nil || !dryRun.DryRun || dryRun.CandidateEnvelopes != 3 || dryRun.UpdatedEnvelopes != 0 || dryRun.ReadyForKeyRemoval {
		t.Fatalf("dry-run report=%+v err=%v", dryRun, err)
	}

	concurrentPrimary, err := utils.EncryptSensitive("concurrent replacement")
	if err != nil {
		t.Fatal(err)
	}
	concurrentChangeDone := false
	concurrentChangeFailed := false
	maintenanceChecks := 0
	rewrapped, err := RewrapSensitiveFieldsFromPreviousKey(context.Background(), db, SensitiveFieldRewrapOptions{
		PreviousKeyIndex: 1,
		BatchSize:        1,
		Execute:          true,
		MaintenanceCheck: func() error { maintenanceChecks++; return nil },
		beforeCompareAndSwap: func(field string) {
			if concurrentChangeDone || field != "member_payment_account_number" {
				return
			}
			concurrentChangeDone = true
			result := db.Exec(`UPDATE "member_payment_accounts" SET "account_no" = ? WHERE "id" = ?`, concurrentPrimary, memberID)
			if result.Error != nil || result.RowsAffected != 1 {
				concurrentChangeFailed = true
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !concurrentChangeDone || concurrentChangeFailed || maintenanceChecks < 4 || rewrapped.DryRun || rewrapped.CandidateEnvelopes != 3 ||
		rewrapped.UpdatedEnvelopes != 2 || rewrapped.CompareAndSwapMisses != 1 ||
		rewrapped.RemainingDependencies != 0 || !rewrapped.ReadyForKeyRemoval ||
		rewrapped.Inventory == nil || !rewrapped.Inventory.Complete || rewrapped.Inventory.Counts.PreviousKey != 0 {
		t.Fatalf("execute report did not prove CAS and final zero-dependency inventory: %+v", rewrapped)
	}
	for _, target := range []struct {
		query string
		id    uint64
	}{
		{query: `SELECT "account_no" FROM "member_payment_accounts" WHERE "id" = ?`, id: memberID},
		{query: `SELECT "secret_key" FROM "wallet_payment_channels" WHERE "id" = ?`, id: walletID},
		{query: `SELECT "secret_key" FROM "entertainment_platforms" WHERE "id" = ?`, id: entertainmentID},
	} {
		var stored string
		if err := db.Raw(target.query, target.id).Scan(&stored).Error; err != nil {
			t.Fatal(err)
		}
		inspection, err := utils.InspectSensitiveEnvelope(stored)
		if err != nil || inspection.Version != "v2" || inspection.PreviousKeyIndex != 0 {
			t.Fatalf("rewrapped value is not primary v2: inspection=%+v err=%v", inspection, err)
		}
	}
}
