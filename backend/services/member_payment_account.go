package services

import (
	modeluser "backend/data/models/user"
	"backend/data/models/wallet"
	apperrors "backend/errors"
	uploads "backend/uploadsecurity"
	"backend/utils"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MemberPaymentAccountService struct{ db *gorm.DB }

const maxMemberPaymentAccountsPerMember int64 = 10

type MemberPaymentAccountView struct {
	ID          uint64 `json:"id"`
	AccountType string `json:"account_type"`
	Label       string `json:"label"`
	AccountName string `json:"account_name"`
	AccountNo   string `json:"account_no"`
	HolderName  string `json:"holder_name"`
	IsDefault   bool   `json:"is_default"`
	QRCodeURL   string `json:"qr_code_url,omitempty"`
}

type CreateMemberPaymentAccountInput struct {
	AccountType string `json:"account_type"`
	Label       string `json:"label"`
	AccountName string `json:"account_name"`
	AccountNo   string `json:"account_no"`
	HolderName  string `json:"holder_name"`
	IsDefault   bool   `json:"is_default"`
}

func NewMemberPaymentAccountService(db *gorm.DB) *MemberPaymentAccountService {
	return &MemberPaymentAccountService{db: db}
}

func (s *MemberPaymentAccountService) List(userID uint64) ([]MemberPaymentAccountView, error) {
	var owner modeluser.User
	if err := s.db.Select("workspace_id").First(&owner, userID).Error; err != nil {
		return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
	}
	var rows []wallet.MemberPaymentAccount
	if err := scopedMemberPaymentAccountQuery(s.db, owner.WorkspaceID, userID).Order("is_default desc, id desc").Find(&rows).Error; err != nil {
		return nil, apperrors.NewSystemError("PAYMENT_ACCOUNT_READ_FAILED", "读取收款方式失败", err)
	}
	items := make([]MemberPaymentAccountView, 0, len(rows))
	for _, row := range rows {
		view, err := memberPaymentAccountView(row)
		if err != nil {
			return nil, apperrors.NewSystemError("PAYMENT_ACCOUNT_DECRYPT_FAILED", "读取收款方式失败", err)
		}
		items = append(items, view)
	}
	return items, nil
}

func (s *MemberPaymentAccountService) Create(userID uint64, input CreateMemberPaymentAccountInput) (*MemberPaymentAccountView, error) {
	return s.CreateWithQRCode(userID, input, nil)
}

func (s *MemberPaymentAccountService) CreateWithQRCode(userID uint64, input CreateMemberPaymentAccountInput, qrCode *uploads.PaymentQRCode) (*MemberPaymentAccountView, error) {
	accountType, label, accountName, accountNo, holderName, err := validateMemberPaymentAccount(input)
	if err != nil {
		return nil, err
	}
	encryptedAccountNo, err := utils.EncryptSensitive(accountNo)
	if err != nil {
		return nil, apperrors.NewSystemError("PAYMENT_ACCOUNT_ENCRYPT_FAILED", "保存收款方式失败", err)
	}
	var created wallet.MemberPaymentAccount
	var storedQRCodeFilename string
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if qrCode != nil {
			// The same cross-process lock is held by startup orphan scanning.
			// Keep it through the database commit so a fully written file can
			// never be observed as unreferenced during the upload/insert window.
			if locked := lockPaymentQRCodeStorage(tx); locked.Error != nil {
				return locked.Error
			}
		}
		// Serialize all account-list mutations for one member.  Without this
		// lock two concurrent "first account" requests can both observe count=0
		// and leave two defaults behind.
		var owner modeluser.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("user_id", "workspace_id").First(&owner, userID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
			}
			return err
		}
		// The locked member row serializes JSON and multipart creates across
		// processes. Count active accounts plus soft-deleted QR cleanup jobs so
		// a failing filesystem cannot be used to accumulate unbounded files.
		var reservedCount int64
		if err := reservedMemberPaymentAccountCapacityQuery(tx.Model(&wallet.MemberPaymentAccount{}), owner.WorkspaceID, userID).
			Count(&reservedCount).Error; err != nil {
			return err
		}
		if reservedCount >= maxMemberPaymentAccountsPerMember {
			return apperrors.NewBusinessError(
				"PAYMENT_ACCOUNT_LIMIT_REACHED",
				"收款方式已达到 10 个上限，请先删除不再使用的收款方式并等待二维码清理完成",
			)
		}
		var count int64
		if err := scopedMemberPaymentAccountQuery(tx.Model(&wallet.MemberPaymentAccount{}), owner.WorkspaceID, userID).Count(&count).Error; err != nil {
			return err
		}
		isDefault := input.IsDefault || count == 0
		if isDefault {
			if err := scopedMemberPaymentAccountQuery(tx.Model(&wallet.MemberPaymentAccount{}), owner.WorkspaceID, userID).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		var qrCodeFile *string
		if qrCode != nil {
			filename, err := uploads.StorePaymentQRCode(owner.WorkspaceID, userID, qrCode)
			if err != nil {
				return err
			}
			storedQRCodeFilename = filename
			qrCodeFile = &storedQRCodeFilename
		}
		created = wallet.MemberPaymentAccount{
			WorkspaceID: owner.WorkspaceID, UserID: userID, AccountType: accountType, Label: label, AccountName: accountName,
			AccountNo: encryptedAccountNo, HolderName: holderName, QRCodeFile: qrCodeFile, IsDefault: isDefault,
		}
		return tx.Create(&created).Error
	})
	if err != nil {
		// Do not unlink here: a commit result can be ambiguous after a
		// connection failure. If the row did not commit, startup orphan
		// reconciliation removes the file; if it did commit, its reference
		// protects the file. This is safer than deleting a possibly active QR.
		if _, ok := err.(*apperrors.AppError); ok {
			return nil, err
		}
		return nil, apperrors.NewSystemError("PAYMENT_ACCOUNT_CREATE_FAILED", "新增收款方式失败", err)
	}
	view, err := memberPaymentAccountView(created)
	if err != nil {
		return nil, apperrors.NewSystemError("PAYMENT_ACCOUNT_DECRYPT_FAILED", "读取收款方式失败", err)
	}
	return &view, nil
}

func (s *MemberPaymentAccountService) Delete(userID, id uint64) error {
	var deletedQRCode struct {
		id          uint64
		workspaceID uint64
		userID      uint64
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var owner modeluser.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("user_id", "workspace_id").First(&owner, userID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
			}
			return err
		}
		var row wallet.MemberPaymentAccount
		if err := scopedMemberPaymentAccountQuery(tx.Clauses(clause.Locking{Strength: "UPDATE"}), owner.WorkspaceID, userID).Where("id = ?", id).First(&row).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				// A previous request may have committed the soft delete and then
				// failed to remove the file. Let a repeated DELETE retry that
				// durable cleanup queue item instead of discarding its reference.
				var pending wallet.MemberPaymentAccount
				pendingErr := deletedPaymentQRCodeCleanupQuery(tx).
					Where("id = ? AND workspace_id = ? AND user_id = ?", id, owner.WorkspaceID, userID).
					First(&pending).Error
				if pendingErr == nil {
					deletedQRCode.id, deletedQRCode.workspaceID, deletedQRCode.userID = pending.ID, pending.WorkspaceID, pending.UserID
					return nil
				}
				if !errors.Is(pendingErr, gorm.ErrRecordNotFound) {
					return pendingErr
				}
				return apperrors.NewBusinessError("PAYMENT_ACCOUNT_NOT_FOUND", "收款方式不存在")
			}
			return err
		}
		wasDefault := row.IsDefault
		if row.QRCodeFile != nil {
			deletedQRCode.id, deletedQRCode.workspaceID, deletedQRCode.userID = row.ID, owner.WorkspaceID, userID
		}
		// A deleted account stays available to historic application audits. Clear
		// the active-list default flag before GORM soft-deletes the row so the
		// partial unique index can immediately accept a replacement default.
		if wasDefault {
			if err := tx.Model(&row).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		if err := tx.Delete(&row).Error; err != nil {
			return err
		}
		if !wasDefault {
			return nil
		}
		// Keep the product invariant "accounts exist => one default" after a
		// member removes the current default.
		var replacement wallet.MemberPaymentAccount
		if err := scopedMemberPaymentAccountQuery(tx, owner.WorkspaceID, userID).Order("id DESC").First(&replacement).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
		return tx.Model(&replacement).Update("is_default", true).Error
	})
	if err != nil {
		if _, ok := err.(*apperrors.AppError); ok {
			return err
		}
		return apperrors.NewSystemError("PAYMENT_ACCOUNT_DELETE_FAILED", "删除收款方式失败", err)
	}
	if deletedQRCode.id != 0 {
		if err := s.reconcileDeletedPaymentQRCode(deletedQRCode.id, deletedQRCode.workspaceID, deletedQRCode.userID); err != nil {
			// The soft-deleted row deliberately keeps qr_code_file. A retry of
			// this DELETE or the next process startup will attempt removal again.
			return apperrors.NewSystemError("PAYMENT_QR_CODE_CLEANUP_PENDING", "收款方式已删除，二维码等待安全清理", err)
		}
	}
	return nil
}

const paymentQRCodeStorageLockID int64 = 0x575A5152434C4E // "WZQRCLN"

// deletedPaymentQRCodeCleanupQuery is intentionally the only global queue
// query: soft-deleted rows with a non-null server filename are durable cleanup
// jobs. Unscoped is required, while both predicates prevent active account
// files from ever entering reconciliation.
func deletedPaymentQRCodeCleanupQuery(db *gorm.DB) *gorm.DB {
	return db.Unscoped().Where("deleted_at IS NOT NULL AND qr_code_file IS NOT NULL")
}

func lockPaymentQRCodeStorage(tx *gorm.DB) *gorm.DB {
	// A transaction advisory lock serializes QR creation, immediate DELETE
	// cleanup and both startup reconciliation passes across backend processes.
	return tx.Exec("SELECT pg_advisory_xact_lock(?)", paymentQRCodeStorageLockID)
}

func acknowledgeDeletedPaymentQRCode(tx *gorm.DB, row wallet.MemberPaymentAccount, filename string) *gorm.DB {
	return deletedPaymentQRCodeCleanupQuery(tx.Model(&wallet.MemberPaymentAccount{})).
		Where("id = ? AND workspace_id = ? AND user_id = ? AND qr_code_file = ?", row.ID, row.WorkspaceID, row.UserID, filename).
		UpdateColumn("qr_code_file", nil)
}

func removePaymentQRCodeBeforeAcknowledgement(removeFile, acknowledge func() error) error {
	if err := removeFile(); err != nil {
		return err
	}
	return acknowledge()
}

func reconcileDeletedPaymentQRCodeRow(tx *gorm.DB, row wallet.MemberPaymentAccount) error {
	if row.QRCodeFile == nil || strings.TrimSpace(*row.QRCodeFile) == "" {
		return errors.New("deleted payment QR cleanup row has no filename")
	}
	filename := *row.QRCodeFile
	return removePaymentQRCodeBeforeAcknowledgement(func() error {
		if err := uploads.RemovePaymentQRCode(row.WorkspaceID, row.UserID, filename); err != nil {
			return fmt.Errorf("remove deleted payment QR workspace=%d user=%d account=%d: %w", row.WorkspaceID, row.UserID, row.ID, err)
		}
		return nil
	}, func() error {
		acknowledged := acknowledgeDeletedPaymentQRCode(tx, row, filename)
		if acknowledged.Error != nil {
			return fmt.Errorf("acknowledge deleted payment QR account=%d: %w", row.ID, acknowledged.Error)
		}
		if acknowledged.RowsAffected != 1 {
			return fmt.Errorf("deleted payment QR cleanup claim changed account=%d", row.ID)
		}
		return nil
	})
}

func (s *MemberPaymentAccountService) reconcileDeletedPaymentQRCode(id, workspaceID, userID uint64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if locked := lockPaymentQRCodeStorage(tx); locked.Error != nil {
			return locked.Error
		}
		var row wallet.MemberPaymentAccount
		err := deletedPaymentQRCodeCleanupQuery(tx.Clauses(clause.Locking{Strength: "UPDATE"})).
			Where("id = ? AND workspace_id = ? AND user_id = ?", id, workspaceID, userID).
			First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Another serialized cleanup may already have removed and
			// acknowledged this file. The operation is intentionally idempotent.
			return nil
		}
		if err != nil {
			return err
		}
		return reconcileDeletedPaymentQRCodeRow(tx, row)
	})
}

// ReconcileDeletedPaymentQRCodes retries every durable QR cleanup job. It is
// called before the HTTP server starts; any filesystem or database error is
// returned so startup fails closed and the non-null reference remains for the
// next attempt. Removing a missing file is successful, which safely recovers a
// crash between filesystem removal and the database acknowledgement.
func (s *MemberPaymentAccountService) ReconcileDeletedPaymentQRCodes(ctx context.Context) error {
	if ctx == nil {
		return errors.New("payment QR reconciliation requires a context")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if locked := lockPaymentQRCodeStorage(tx); locked.Error != nil {
			return locked.Error
		}
		var rows []wallet.MemberPaymentAccount
		if err := deletedPaymentQRCodeCleanupQuery(tx.Clauses(clause.Locking{Strength: "UPDATE"})).
			Order("id ASC").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if err := reconcileDeletedPaymentQRCodeRow(tx, row); err != nil {
				return err
			}
		}
		return nil
	})
}

type paymentQRCodeStorageKey struct {
	workspaceID uint64
	userID      uint64
	filename    string
}

// allPaymentQRCodeReferenceQuery deliberately includes active and soft-deleted
// rows. A soft-deleted row with qr_code_file still set is a durable deletion
// job, so orphan cleanup must keep its file until that job is acknowledged.
func allPaymentQRCodeReferenceQuery(db *gorm.DB) *gorm.DB {
	return db.Unscoped().Where("qr_code_file IS NOT NULL")
}

func unreferencedPaymentQRCodeFiles(files []uploads.PaymentQRCodeFile, rows []wallet.MemberPaymentAccount) []uploads.PaymentQRCodeFile {
	referenced := make(map[paymentQRCodeStorageKey]struct{}, len(rows))
	for _, row := range rows {
		if row.QRCodeFile == nil || strings.TrimSpace(*row.QRCodeFile) == "" {
			continue
		}
		referenced[paymentQRCodeStorageKey{
			workspaceID: row.WorkspaceID,
			userID:      row.UserID,
			filename:    *row.QRCodeFile,
		}] = struct{}{}
	}
	orphans := make([]uploads.PaymentQRCodeFile, 0)
	for _, file := range files {
		key := paymentQRCodeStorageKey{workspaceID: file.WorkspaceID, userID: file.UserID, filename: file.Filename}
		if _, exists := referenced[key]; !exists {
			orphans = append(orphans, file)
		}
	}
	return orphans
}

// ReconcileOrphanedPaymentQRCodes removes server-generated files that have no
// database reference. The shared transaction advisory lock prevents a scanner
// from entering while CreateWithQRCode is between its final file write and DB
// commit, including when multiple backend processes overlap during rollout.
func (s *MemberPaymentAccountService) ReconcileOrphanedPaymentQRCodes(ctx context.Context) error {
	if ctx == nil {
		return errors.New("payment QR orphan reconciliation requires a context")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if locked := lockPaymentQRCodeStorage(tx); locked.Error != nil {
			return locked.Error
		}
		var rows []wallet.MemberPaymentAccount
		if err := allPaymentQRCodeReferenceQuery(tx).
			Select("workspace_id", "user_id", "qr_code_file").
			Find(&rows).Error; err != nil {
			return err
		}
		files, err := uploads.ListPaymentQRCodeFiles(ctx)
		if err != nil {
			return err
		}
		for _, file := range unreferencedPaymentQRCodeFiles(files, rows) {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := uploads.RemovePaymentQRCode(file.WorkspaceID, file.UserID, file.Filename); err != nil {
				return fmt.Errorf("remove orphaned payment QR workspace=%d user=%d file=%s: %w", file.WorkspaceID, file.UserID, file.Filename, err)
			}
		}
		return nil
	})
}

func (s *MemberPaymentAccountService) GetOwned(userID, id uint64) (*wallet.MemberPaymentAccount, error) {
	if id == 0 {
		return nil, apperrors.NewBusinessError("PAYMENT_ACCOUNT_REQUIRED", "请先选择收款方式")
	}
	var owner modeluser.User
	if err := s.db.Select("workspace_id").First(&owner, userID).Error; err != nil {
		return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
	}
	var row wallet.MemberPaymentAccount
	if err := scopedMemberPaymentAccountQuery(s.db, owner.WorkspaceID, userID).Where("id = ?", id).First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NewBusinessError("PAYMENT_ACCOUNT_NOT_FOUND", "收款方式不存在或不属于当前账号")
		}
		return nil, apperrors.NewSystemError("PAYMENT_ACCOUNT_READ_FAILED", "读取收款方式失败", err)
	}
	plainAccountNo, err := utils.DecryptSensitive(row.AccountNo)
	if err != nil {
		return nil, apperrors.NewSystemError("PAYMENT_ACCOUNT_DECRYPT_FAILED", "读取收款方式失败", err)
	}
	row.AccountNo = plainAccountNo
	return &row, nil
}

// QRCode returns only the authenticated member's own QR code. The account ID,
// current workspace and user ID are all required; a valid ID from another
// workspace or member is deliberately indistinguishable from a missing one.
type MemberPaymentQRCode struct {
	File *os.File
	Size int64
}

func (s *MemberPaymentAccountService) QRCode(userID, id uint64) (*MemberPaymentQRCode, error) {
	if id == 0 {
		return nil, apperrors.NewBusinessError("NOT_FOUND", "收款二维码不存在")
	}
	var owner modeluser.User
	if err := s.db.Select("workspace_id").First(&owner, userID).Error; err != nil {
		return nil, apperrors.NewBusinessError("USER_NOT_FOUND", "用户不存在")
	}
	var row wallet.MemberPaymentAccount
	if err := scopedMemberPaymentAccountQuery(s.db, owner.WorkspaceID, userID).Select("qr_code_file").Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewBusinessError("NOT_FOUND", "收款二维码不存在")
		}
		return nil, apperrors.NewSystemError("PAYMENT_QR_CODE_READ_FAILED", "读取收款二维码失败", err)
	}
	if row.QRCodeFile == nil || strings.TrimSpace(*row.QRCodeFile) == "" {
		return nil, apperrors.NewBusinessError("NOT_FOUND", "收款二维码不存在")
	}
	file, size, err := uploads.OpenPaymentQRCode(owner.WorkspaceID, userID, *row.QRCodeFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, apperrors.NewBusinessError("NOT_FOUND", "收款二维码不存在")
		}
		return nil, apperrors.NewSystemError("PAYMENT_QR_CODE_READ_FAILED", "读取收款二维码失败", err)
	}
	return &MemberPaymentQRCode{File: file, Size: size}, nil
}

func scopedMemberPaymentAccountQuery(db *gorm.DB, workspaceID, userID uint64) *gorm.DB {
	return db.Where("workspace_id = ? AND user_id = ?", workspaceID, userID)
}

// reservedMemberPaymentAccountCapacityQuery includes every active account and
// only those deleted rows whose private QR deletion is still pending. Deleted
// rows acknowledged with qr_code_file=NULL are audit history, not live storage
// reservations, and therefore do not permanently consume a member slot.
func reservedMemberPaymentAccountCapacityQuery(db *gorm.DB, workspaceID, userID uint64) *gorm.DB {
	return scopedMemberPaymentAccountQuery(db.Unscoped(), workspaceID, userID).
		Where("(deleted_at IS NULL OR (deleted_at IS NOT NULL AND qr_code_file IS NOT NULL))")
}

func validateMemberPaymentAccount(input CreateMemberPaymentAccountInput) (string, string, string, string, string, error) {
	accountType := strings.ToLower(strings.TrimSpace(input.AccountType))
	if _, ok := allowedCreditTypes[accountType]; !ok || accountType == "manual" {
		return "", "", "", "", "", apperrors.NewBusinessError("INVALID_PAYMENT_ACCOUNT", "请选择微信、支付宝、银行卡或 USDT")
	}
	accountName := strings.TrimSpace(input.AccountName)
	accountNo := strings.TrimSpace(input.AccountNo)
	holderName := strings.TrimSpace(input.HolderName)
	if accountName == "" || accountNo == "" {
		return "", "", "", "", "", apperrors.NewBusinessError("INVALID_PAYMENT_ACCOUNT", "请填写收款账号和账户名称")
	}
	if utf8.RuneCountInString(accountName) > 100 || utf8.RuneCountInString(accountNo) > 180 || utf8.RuneCountInString(holderName) > 80 {
		return "", "", "", "", "", apperrors.NewBusinessError("INVALID_PAYMENT_ACCOUNT", "收款方式信息过长")
	}
	label := strings.TrimSpace(input.Label)
	if label == "" {
		label = allowedCreditTypes[accountType]
	}
	if utf8.RuneCountInString(label) > 80 {
		return "", "", "", "", "", apperrors.NewBusinessError("INVALID_PAYMENT_ACCOUNT", "收款方式名称过长")
	}
	return accountType, label, accountName, accountNo, holderName, nil
}

func memberPaymentAccountView(row wallet.MemberPaymentAccount) (MemberPaymentAccountView, error) {
	accountNo, err := utils.DecryptSensitive(row.AccountNo)
	if err != nil {
		return MemberPaymentAccountView{}, err
	}
	view := MemberPaymentAccountView{
		ID: row.ID, AccountType: row.AccountType, Label: row.Label, AccountName: row.AccountName,
		AccountNo: maskPaymentAccountNo(accountNo), HolderName: row.HolderName, IsDefault: row.IsDefault,
	}
	if row.QRCodeFile != nil && strings.TrimSpace(*row.QRCodeFile) != "" {
		view.QRCodeURL = fmt.Sprintf("/api/member/payment-accounts/%d/qr-code", row.ID)
	}
	return view, nil
}

func maskPaymentAccountNo(value string) string {
	chars := []rune(strings.TrimSpace(value))
	if len(chars) <= 4 {
		return strings.Repeat("•", len(chars))
	}
	keep := 4
	return strings.Repeat("•", len(chars)-keep) + string(chars[len(chars)-keep:])
}
