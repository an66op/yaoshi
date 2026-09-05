package services

import (
	"backend/data/models/wallet"
	uploads "backend/uploadsecurity"
	"context"
	"errors"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func dryRunMemberPaymentAccountDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestScopedMemberPaymentAccountQueryRequiresWorkspaceAndUser(t *testing.T) {
	db := dryRunMemberPaymentAccountDB(t)
	var account wallet.MemberPaymentAccount
	statement := scopedMemberPaymentAccountQuery(db, 37, 91).Where("id = ?", 15).First(&account).Statement
	sql := strings.ToLower(statement.SQL.String())
	for _, predicate := range []string{"workspace_id =", "user_id =", "id =", "deleted_at"} {
		if !strings.Contains(sql, predicate) {
			t.Fatalf("owner-scoped payment account query omitted %q: %s", predicate, sql)
		}
	}
	want := []any{uint64(37), uint64(91), 15, 1}
	if len(statement.Vars) != len(want) {
		t.Fatalf("scope vars = %#v", statement.Vars)
	}
	for index := range want {
		if statement.Vars[index] != want[index] {
			t.Fatalf("scope var %d = %#v, want %#v", index, statement.Vars[index], want[index])
		}
	}
}

func TestReservedMemberPaymentAccountCapacityIncludesActiveAndPendingQRRows(t *testing.T) {
	db := dryRunMemberPaymentAccountDB(t)
	var count int64
	statement := reservedMemberPaymentAccountCapacityQuery(db.Model(&wallet.MemberPaymentAccount{}), 37, 91).
		Count(&count).Statement
	sql := strings.ToLower(statement.SQL.String())
	for _, fragment := range []string{"workspace_id =", "user_id =", "deleted_at is null", "deleted_at is not null", "qr_code_file is not null", " or "} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("payment account capacity query omitted %q: %s", fragment, sql)
		}
	}
	// Unscoped is required so a soft-deleted QR cleanup job consumes capacity.
	if strings.Contains(sql, `and "member_payment_accounts"."deleted_at" is null`) {
		t.Fatalf("capacity query accidentally excluded deletion queue rows: %s", sql)
	}
	if maxMemberPaymentAccountsPerMember != 10 {
		t.Fatalf("member payment account hard limit = %d, want 10", maxMemberPaymentAccountsPerMember)
	}
}

func TestDeletedPaymentQRCodeCleanupQueryIsLockedAndExcludesActiveRows(t *testing.T) {
	db := dryRunMemberPaymentAccountDB(t)
	var rows []wallet.MemberPaymentAccount
	statement := deletedPaymentQRCodeCleanupQuery(db.Clauses(clause.Locking{Strength: "UPDATE"})).
		Order("id ASC").Find(&rows).Statement
	sql := strings.ToLower(statement.SQL.String())
	for _, fragment := range []string{"deleted_at is not null", "qr_code_file is not null", "for update"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("deleted QR cleanup query omitted %q: %s", fragment, sql)
		}
	}
	if strings.Contains(sql, "deleted_at\" is null") {
		t.Fatalf("deleted QR cleanup query retained GORM's active-row scope: %s", sql)
	}

	locked := lockPaymentQRCodeStorage(db)
	lockSQL := strings.ToLower(locked.Statement.SQL.String())
	if locked.Error != nil || !strings.Contains(lockSQL, "pg_advisory_xact_lock") || len(locked.Statement.Vars) != 1 || locked.Statement.Vars[0] != paymentQRCodeStorageLockID {
		t.Fatalf("payment QR cleanup is not process-serialized: sql=%s vars=%#v err=%v", lockSQL, locked.Statement.Vars, locked.Error)
	}
}

func TestAllPaymentQRCodeReferenceQueryIncludesActiveAndDeletedRows(t *testing.T) {
	db := dryRunMemberPaymentAccountDB(t)
	var rows []wallet.MemberPaymentAccount
	statement := allPaymentQRCodeReferenceQuery(db).
		Select("workspace_id", "user_id", "qr_code_file").
		Find(&rows).Statement
	sql := strings.ToLower(statement.SQL.String())
	if !strings.Contains(sql, "qr_code_file is not null") {
		t.Fatalf("QR reference scan omitted non-null scope: %s", sql)
	}
	for _, forbidden := range []string{`deleted_at" is null`, `deleted_at" is not null`} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("QR reference scan excluded active or deleted rows with %q: %s", forbidden, sql)
		}
	}
}

func TestDeletedPaymentQRCodeAcknowledgementKeepsFullOwnerAndQueueScope(t *testing.T) {
	db := dryRunMemberPaymentAccountDB(t)
	filename := "0123456789abcdef0123456789abcdef.png"
	row := wallet.MemberPaymentAccount{ID: 15, WorkspaceID: 37, UserID: 91, QRCodeFile: &filename}
	statement := acknowledgeDeletedPaymentQRCode(db, row, filename).Statement
	sql := strings.ToLower(statement.SQL.String())
	for _, fragment := range []string{"deleted_at is not null", "qr_code_file is not null", "id =", "workspace_id =", "user_id =", "qr_code_file ="} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("deleted QR acknowledgement omitted %q: %s (error=%v)", fragment, sql, statement.Error)
		}
	}
	if strings.Contains(sql, "deleted_at\" is null") {
		t.Fatalf("deleted QR acknowledgement could target an active row: %s", sql)
	}
}

func TestDeletedPaymentQRCodeReconciliationRequiresLiveContext(t *testing.T) {
	service := NewMemberPaymentAccountService(dryRunMemberPaymentAccountDB(t))
	if err := service.ReconcileDeletedPaymentQRCodes(nil); err == nil {
		t.Fatal("reconciliation accepted an unbounded nil context")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.ReconcileDeletedPaymentQRCodes(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled reconciliation error = %v, want context.Canceled", err)
	}
}

func TestOrphanedPaymentQRCodeReconciliationRequiresLiveContext(t *testing.T) {
	service := NewMemberPaymentAccountService(dryRunMemberPaymentAccountDB(t))
	if err := service.ReconcileOrphanedPaymentQRCodes(nil); err == nil {
		t.Fatal("orphan reconciliation accepted an unbounded nil context")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.ReconcileOrphanedPaymentQRCodes(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled orphan reconciliation error = %v, want context.Canceled", err)
	}
}

func TestUnreferencedPaymentQRCodeFilesKeepsActiveAndDeletionQueueReferences(t *testing.T) {
	activeName := "0123456789abcdef0123456789abcdef.png"
	queuedName := "11111111111111111111111111111111.png"
	orphanName := "22222222222222222222222222222222.png"
	wrongOwnerName := "33333333333333333333333333333333.png"
	files := []uploads.PaymentQRCodeFile{
		{WorkspaceID: 37, UserID: 91, Filename: activeName},
		{WorkspaceID: 37, UserID: 91, Filename: queuedName},
		{WorkspaceID: 37, UserID: 91, Filename: orphanName},
		{WorkspaceID: 37, UserID: 92, Filename: wrongOwnerName},
	}
	rows := []wallet.MemberPaymentAccount{
		{WorkspaceID: 37, UserID: 91, QRCodeFile: &activeName},
		// Whether DeletedAt is set is intentionally immaterial: both active
		// rows and durable deletion queue rows are references.
		{WorkspaceID: 37, UserID: 91, QRCodeFile: &queuedName, DeletedAt: gorm.DeletedAt{Valid: true}},
		{WorkspaceID: 37, UserID: 91, QRCodeFile: &wrongOwnerName},
	}
	orphans := unreferencedPaymentQRCodeFiles(files, rows)
	want := []uploads.PaymentQRCodeFile{
		{WorkspaceID: 37, UserID: 91, Filename: orphanName},
		{WorkspaceID: 37, UserID: 92, Filename: wrongOwnerName},
	}
	if len(orphans) != len(want) {
		t.Fatalf("orphans = %#v, want %#v", orphans, want)
	}
	for index := range want {
		if orphans[index] != want[index] {
			t.Fatalf("orphan %d = %#v, want %#v", index, orphans[index], want[index])
		}
	}
}

func TestPaymentQRCodeCleanupNeverAcknowledgesBeforeRemovalAndRetriesCrashGap(t *testing.T) {
	removeFailure := errors.New("filesystem unavailable")
	acknowledged := false
	err := removePaymentQRCodeBeforeAcknowledgement(func() error {
		return removeFailure
	}, func() error {
		acknowledged = true
		return nil
	})
	if !errors.Is(err, removeFailure) || acknowledged {
		t.Fatalf("failed removal lost its durable reference: acknowledged=%t err=%v", acknowledged, err)
	}

	// Equivalent to process death after unlink and before the qr_code_file=NULL
	// commit: the first acknowledgement fails, then idempotent removal and the
	// second acknowledgement complete the same durable queue item.
	ackFailure := errors.New("process interrupted before commit")
	removeCalls, acknowledgeCalls := 0, 0
	err = removePaymentQRCodeBeforeAcknowledgement(func() error {
		removeCalls++
		return nil
	}, func() error {
		acknowledgeCalls++
		return ackFailure
	})
	if !errors.Is(err, ackFailure) || removeCalls != 1 || acknowledgeCalls != 1 {
		t.Fatalf("crash-gap setup = removes:%d acknowledgements:%d err:%v", removeCalls, acknowledgeCalls, err)
	}
	err = removePaymentQRCodeBeforeAcknowledgement(func() error {
		removeCalls++
		return nil
	}, func() error {
		acknowledgeCalls++
		acknowledged = true
		return nil
	})
	if err != nil || !acknowledged || removeCalls != 2 || acknowledgeCalls != 2 {
		t.Fatalf("crash-gap retry = acknowledged:%t removes:%d acknowledgements:%d err:%v", acknowledged, removeCalls, acknowledgeCalls, err)
	}
}
