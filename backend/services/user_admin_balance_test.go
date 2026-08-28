package services

import (
	"backend/data/models/user"
	"testing"
)

func TestManualBalanceRecordFreezesBeforeUpdateSnapshot(t *testing.T) {
	account := user.User{UserID: 3, WorkspaceID: 9, BalanceCents: 100000}
	after, record := manualBalanceRecord(account, account.WorkspaceID, 100000, " 房间红包备用金 ", " 管理员 ")

	// GORM's Update mutates the loaded model. The immutable ledger row must not
	// observe that later mutation.
	account.BalanceCents = after
	if record.BeforeCents != 100000 || record.AfterCents != 200000 || record.AmountCents != 100000 {
		t.Fatalf("unexpected balance snapshot: %#v", record)
	}
	if record.AfterCents != record.BeforeCents+record.AmountCents {
		t.Fatalf("ledger arithmetic broken: %#v", record)
	}
	if record.WorkspaceID != 9 || record.UserID != 3 {
		t.Fatalf("ledger ownership lost: %#v", record)
	}
	if record.Remark != "房间红包备用金" || record.Operator != "管理员" {
		t.Fatalf("ledger labels were not normalized: %#v", record)
	}
}
