package services

import (
	"backend/data/models/wallet"
	apperrors "backend/errors"
	"backend/uploadsecurity"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func postgresPaymentQRCode(t *testing.T) *uploadsecurity.PaymentQRCode {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			canvas.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 255})
		}
	}
	var raw bytes.Buffer
	if err := png.Encode(&raw, canvas); err != nil {
		t.Fatal(err)
	}
	cleaned, err := uploadsecurity.SanitizePaymentQRCode(bytes.NewReader(raw.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return cleaned
}

func assertPaymentQRCodeOrphanScanWaitsForUploadCommit(t *testing.T, activeTarget string) {
	t.Helper()
	otherDB, err := gorm.Open(postgres.Open(os.Getenv("BACKEND_TIMING_TEST_DSN")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := otherDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	// timingPostgresDatabase wraps the fixture in an outer transaction. The QR
	// create's xact advisory lock therefore remains held here, modeling the
	// exact file-written / row-not-yet-committed window on a second process.
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	err = NewMemberPaymentAccountService(otherDB).ReconcileOrphanedPaymentQRCodes(ctx)
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("concurrent orphan scan did not wait for upload transaction lock: %v", err)
	}
	if _, statErr := os.Stat(activeTarget); statErr != nil {
		t.Fatalf("blocked orphan scan touched in-flight upload: %v", statErr)
	}
}

// This opt-in PostgreSQL contract test exercises the crash-safe boundary that
// a DryRun query cannot: the soft-delete commits first, a failed filesystem
// cleanup retains qr_code_file, and a later reconciler removes and acknowledges
// exactly that owner-scoped file.
func TestMemberPaymentQRCodePostgresDeletionRemainsRetryable(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "payment_qr_cleanup", "735211")
	member := timingPostgresMember(t, db, room, "payment_qr_cleanup_member")
	uploadRoot := t.TempDir()
	t.Setenv("BACKEND_UPLOAD_DIR", uploadRoot)
	service := NewMemberPaymentAccountService(db)

	create := func(accountNo string) (MemberPaymentAccountView, wallet.MemberPaymentAccount, string) {
		t.Helper()
		view, err := service.CreateWithQRCode(member.UserID, CreateMemberPaymentAccountInput{
			AccountType: "wechat", AccountName: "测试收款人", AccountNo: accountNo,
		}, postgresPaymentQRCode(t))
		if err != nil {
			t.Fatal(err)
		}
		var row wallet.MemberPaymentAccount
		if err := db.First(&row, view.ID).Error; err != nil || row.QRCodeFile == nil {
			t.Fatalf("created payment QR row = %+v, error=%v", row, err)
		}
		target := filepath.Join(uploadRoot, ".private", "member-payment-qr",
			strconv.FormatUint(row.WorkspaceID, 10), strconv.FormatUint(row.UserID, 10), *row.QRCodeFile)
		return *view, row, target
	}

	_, first, firstTarget := create("wx-cleanup-first")
	assertPaymentQRCodeOrphanScanWaitsForUploadCommit(t, firstTarget)
	if err := service.Delete(member.UserID, first.ID); err != nil {
		t.Fatal("immediate QR cleanup:", err)
	}
	var firstDeleted wallet.MemberPaymentAccount
	if err := db.Unscoped().First(&firstDeleted, first.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !firstDeleted.DeletedAt.Valid || firstDeleted.QRCodeFile != nil {
		t.Fatalf("successful deletion was not acknowledged: %+v", firstDeleted)
	}
	if _, err := os.Stat(firstTarget); !os.IsNotExist(err) {
		t.Fatalf("successfully deleted QR still exists: %v", err)
	}

	_, pending, pendingTarget := create("wx-cleanup-pending")
	ownerDirectory := filepath.Dir(pendingTarget)
	realOwnerDirectory := ownerDirectory + ".held-for-test"
	if err := os.Rename(ownerDirectory, realOwnerDirectory); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realOwnerDirectory, ownerDirectory); err != nil {
		t.Fatal(err)
	}
	cleanupErr := service.Delete(member.UserID, pending.ID)
	if apperrors.GetErrorCode(cleanupErr) != "PAYMENT_QR_CODE_CLEANUP_PENDING" {
		t.Fatalf("unsafe path cleanup error = %v", cleanupErr)
	}
	var queued wallet.MemberPaymentAccount
	if err := db.Unscoped().First(&queued, pending.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !queued.DeletedAt.Valid || queued.QRCodeFile == nil || *queued.QRCodeFile != *pending.QRCodeFile {
		t.Fatalf("failed cleanup discarded its durable filename: %+v", queued)
	}
	if _, err := os.Stat(filepath.Join(realOwnerDirectory, *pending.QRCodeFile)); err != nil {
		t.Fatal("failed cleanup removed a file through an unsafe path:", err)
	}

	if err := os.Remove(ownerDirectory); err != nil { // removes only the symlink
		t.Fatal(err)
	}
	if err := os.Rename(realOwnerDirectory, ownerDirectory); err != nil {
		t.Fatal(err)
	}
	if err := service.ReconcileDeletedPaymentQRCodes(context.Background()); err != nil {
		t.Fatal("startup-style retry:", err)
	}
	if err := db.Unscoped().First(&queued, pending.ID).Error; err != nil {
		t.Fatal(err)
	}
	if queued.QRCodeFile != nil {
		t.Fatalf("successful retry did not acknowledge cleanup: %+v", queued)
	}
	if _, err := os.Stat(pendingTarget); !os.IsNotExist(err) {
		t.Fatalf("retry left deleted QR on disk: %v", err)
	}

	// Simulate a crash after unlink but before acknowledgement. A missing file
	// is idempotent success, while an active account remains out of scope.
	_, interrupted, interruptedTarget := create("wx-cleanup-interrupted")
	if err := db.Delete(&interrupted).Error; err != nil {
		t.Fatal(err)
	}
	if err := uploadsecurity.RemovePaymentQRCode(interrupted.WorkspaceID, interrupted.UserID, *interrupted.QRCodeFile); err != nil {
		t.Fatal(err)
	}
	_, active, activeTarget := create("wx-cleanup-active")
	if err := service.ReconcileDeletedPaymentQRCodes(context.Background()); err != nil {
		t.Fatal(err)
	}
	var interruptedAfter, activeAfter wallet.MemberPaymentAccount
	if err := db.Unscoped().First(&interruptedAfter, interrupted.ID).Error; err != nil || interruptedAfter.QRCodeFile != nil {
		t.Fatalf("interrupted cleanup was not acknowledged: %+v, %v", interruptedAfter, err)
	}
	if err := db.First(&activeAfter, active.ID).Error; err != nil || activeAfter.QRCodeFile == nil {
		t.Fatalf("reconciler touched an active payment account: %+v, %v", activeAfter, err)
	}
	if _, err := os.Stat(interruptedTarget); !os.IsNotExist(err) {
		t.Fatalf("interrupted QR unexpectedly reappeared: %v", err)
	}
	if _, err := os.Stat(activeTarget); err != nil {
		t.Fatalf("active QR was removed: %v", err)
	}

	// The authenticated read contract returns an open bounded file for
	// streaming, and an account id from another member remains indistinguishable
	// from a missing account.
	otherMember := timingPostgresMember(t, db, room, "payment_qr_other_member")
	if _, err := service.QRCode(otherMember.UserID, active.ID); apperrors.GetErrorCode(err) != "NOT_FOUND" {
		t.Fatalf("cross-member QR read error = %v, want NOT_FOUND", err)
	}
	streamed, err := service.QRCode(member.UserID, active.ID)
	if err != nil {
		t.Fatal("owner QR stream:", err)
	}
	if streamed.Size <= 8 {
		_ = streamed.File.Close()
		t.Fatalf("owner QR stream size = %d", streamed.Size)
	}
	var signature [8]byte
	if _, err := streamed.File.Read(signature[:]); err != nil {
		_ = streamed.File.Close()
		t.Fatal("read streamed PNG signature:", err)
	}
	if err := streamed.File.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(signature[:], []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("streamed QR signature = %x", signature)
	}

	// A soft-deleted row whose cleanup has not run is still a reference and
	// must not be mistaken for an orphan by the other startup pass.
	_, queuedForDelete, queuedForDeleteTarget := create("wx-orphan-scan-delete-queue")
	if err := db.Delete(&queuedForDelete).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.ReconcileOrphanedPaymentQRCodes(context.Background()); err != nil {
		t.Fatal("orphan scan with durable delete queue:", err)
	}
	var queuedForDeleteAfter wallet.MemberPaymentAccount
	if err := db.Unscoped().First(&queuedForDeleteAfter, queuedForDelete.ID).Error; err != nil || queuedForDeleteAfter.QRCodeFile == nil {
		t.Fatalf("orphan scan discarded deletion queue reference: %+v, %v", queuedForDeleteAfter, err)
	}
	if _, err := os.Stat(queuedForDeleteTarget); err != nil {
		t.Fatalf("orphan scan removed deletion queue file: %v", err)
	}
	if err := service.ReconcileDeletedPaymentQRCodes(context.Background()); err != nil {
		t.Fatal("delete queue cleanup after orphan scan:", err)
	}

	// Equivalent to a process dying after StorePaymentQRCode completes but
	// before the account insert/commit: no database row ever references this
	// canonical server-generated file, so startup safely reclaims it.
	orphanFilename, err := uploadsecurity.StorePaymentQRCode(member.WorkspaceID, member.UserID, postgresPaymentQRCode(t))
	if err != nil {
		t.Fatal(err)
	}
	orphanTarget := filepath.Join(uploadRoot, ".private", "member-payment-qr",
		strconv.FormatUint(member.WorkspaceID, 10), strconv.FormatUint(member.UserID, 10), orphanFilename)
	if err := service.ReconcileOrphanedPaymentQRCodes(context.Background()); err != nil {
		t.Fatal("startup orphan cleanup:", err)
	}
	if _, err := os.Stat(orphanTarget); !os.IsNotExist(err) {
		t.Fatalf("crash-orphan QR still exists: %v", err)
	}
	if _, err := os.Stat(activeTarget); err != nil {
		t.Fatalf("orphan cleanup removed active QR: %v", err)
	}
}

func TestMemberPaymentAccountPostgresHardLimitIncludesPendingQRCode(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "payment_account_limit", "735212")
	member := timingPostgresMember(t, db, room, "payment_account_limit_member")
	uploadRoot := t.TempDir()
	t.Setenv("BACKEND_UPLOAD_DIR", uploadRoot)
	service := NewMemberPaymentAccountService(db)

	pendingView, err := service.CreateWithQRCode(member.UserID, CreateMemberPaymentAccountInput{
		AccountType: "wechat", AccountName: "待清理二维码", AccountNo: "pending-qr",
	}, postgresPaymentQRCode(t))
	if err != nil {
		t.Fatal(err)
	}
	var pending wallet.MemberPaymentAccount
	if err := db.First(&pending, pendingView.ID).Error; err != nil || pending.QRCodeFile == nil {
		t.Fatalf("pending QR fixture = %+v, %v", pending, err)
	}
	pendingTarget := filepath.Join(uploadRoot, ".private", "member-payment-qr",
		strconv.FormatUint(pending.WorkspaceID, 10), strconv.FormatUint(pending.UserID, 10), *pending.QRCodeFile)
	// Bypass the immediate service cleanup to model a committed soft-delete
	// queue job. It must reserve one of the ten live storage slots.
	if err := db.Delete(&pending).Error; err != nil {
		t.Fatal(err)
	}

	for index := 0; index < 9; index++ {
		if _, err := service.Create(member.UserID, CreateMemberPaymentAccountInput{
			AccountType: "bank", AccountName: fmt.Sprintf("纯 JSON 账户 %d", index+1), AccountNo: fmt.Sprintf("json-%d", index+1),
		}); err != nil {
			t.Fatalf("create JSON account %d: %v", index+1, err)
		}
	}
	if _, err := service.Create(member.UserID, CreateMemberPaymentAccountInput{
		AccountType: "bank", AccountName: "超限账户", AccountNo: "over-limit",
	}); apperrors.GetErrorCode(err) != "PAYMENT_ACCOUNT_LIMIT_REACHED" {
		t.Fatalf("11th reserved account error = %v", err)
	}
	if _, err := service.CreateWithQRCode(member.UserID, CreateMemberPaymentAccountInput{
		AccountType: "wechat", AccountName: "超限二维码账户", AccountNo: "over-limit-qr",
	}, postgresPaymentQRCode(t)); apperrors.GetErrorCode(err) != "PAYMENT_ACCOUNT_LIMIT_REACHED" {
		t.Fatalf("over-limit QR account error = %v", err)
	}
	storedFiles, err := uploadsecurity.ListPaymentQRCodeFiles(context.Background())
	if err != nil || len(storedFiles) != 1 {
		t.Fatalf("over-limit create wrote another QR: files=%#v error=%v", storedFiles, err)
	}
	if _, err := os.Stat(pendingTarget); err != nil {
		t.Fatalf("capacity check touched pending QR: %v", err)
	}

	if err := service.ReconcileDeletedPaymentQRCodes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(member.UserID, CreateMemberPaymentAccountInput{
		AccountType: "bank", AccountName: "清理后账户", AccountNo: "after-cleanup",
	}); err != nil {
		t.Fatalf("acknowledged QR cleanup did not release capacity: %v", err)
	}
}
