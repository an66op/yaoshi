package services

import (
	"backend/data/models/bet"
	"backend/data/models/user"
	apperrors "backend/errors"
	"encoding/json"
	"strings"
	"testing"
)

func TestReconciliationRefundReferenceIsStable(t *testing.T) {
	if first, second := reconciliationRefundReference(91), reconciliationRefundReference(91); first != "reconciliation_refund:91" || first != second {
		t.Fatalf("refund reference is not stable: %q / %q", first, second)
	}
	if reconciliationRefundReference(92) == reconciliationRefundReference(91) {
		t.Fatal("different bets must never share a refund reference")
	}
}

func TestRefundableAbnormalBetRequiresPendingAbnormalState(t *testing.T) {
	valid := bet.Bet{ID: 7, Status: "pending", ReconciliationStatus: "abnormal", AmountCents: 2500}
	if err := validateRefundableAbnormalBet(valid); err != nil {
		t.Fatalf("valid abnormal pending bet was rejected: %v", err)
	}
	for name, row := range map[string]bet.Bet{
		"normal pending":   {Status: "pending", ReconciliationStatus: "normal", AmountCents: 2500},
		"settled abnormal": {Status: "lost", ReconciliationStatus: "abnormal", AmountCents: 2500},
		"empty stake":      {Status: "pending", ReconciliationStatus: "abnormal", AmountCents: 0},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateRefundableAbnormalBet(row)
			if err == nil || !apperrors.IsBusinessError(err) {
				t.Fatalf("unsafe refund state should be a business rejection, got %v", err)
			}
		})
	}
}

func TestExistingReconciliationRefundIsIdempotentOnlyWhenEvidenceMatches(t *testing.T) {
	row := bet.Bet{
		ID: 44, WorkspaceID: 8, UserID: 12, AmountCents: 3600,
		Status: "cancelled", ReconciliationStatus: reconciliationRefundedStatus,
	}
	ledger := user.BalanceTransaction{
		WorkspaceID: 8, UserID: 12, Reference: reconciliationRefundReference(44),
		AmountCents: 3600, BeforeCents: 1000, AfterCents: 4600,
		Type: reconciliationRefundLedgerType,
	}
	if err := validateExistingReconciliationRefund(row, ledger); err != nil {
		t.Fatalf("matching retry evidence must be idempotent: %v", err)
	}

	conflicts := map[string]user.BalanceTransaction{
		"wrong amount":      ledger,
		"wrong user":        ledger,
		"broken arithmetic": ledger,
		"wrong type":        ledger,
	}
	value := conflicts["wrong amount"]
	value.AmountCents++
	conflicts["wrong amount"] = value
	value = conflicts["wrong user"]
	value.UserID++
	conflicts["wrong user"] = value
	value = conflicts["broken arithmetic"]
	value.AfterCents++
	conflicts["broken arithmetic"] = value
	value = conflicts["wrong type"]
	value.Type = "bet_cancel"
	conflicts["wrong type"] = value
	for name, conflict := range conflicts {
		t.Run(name, func(t *testing.T) {
			if err := validateExistingReconciliationRefund(row, conflict); err == nil {
				t.Fatal("conflicting financial evidence must fail closed")
			}
		})
	}
}

func TestReconciliationBetViewExposesExactStakeAndRefundability(t *testing.T) {
	view := toReconciliationBetView(bet.Bet{
		ID: 5, WorkspaceID: 77, AmountCents: 12345,
		Status: "pending", ReconciliationStatus: "abnormal",
	})
	if view.WorkspaceID != 77 || view.AmountCents != 12345 || !view.Refundable {
		t.Fatalf("unexpected reconciliation view: %#v", view)
	}
	payload, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal reconciliation view: %v", err)
	}
	if text := string(payload); !strings.Contains(text, `"workspace_id":77`) || !strings.Contains(text, `"amount_cents":12345`) || !strings.Contains(text, `"refundable":true`) {
		t.Fatalf("reconciliation JSON is missing refund fields: %s", text)
	}
}

func TestIdempotentRefundDoesNotBroadcastHistoricalBalance(t *testing.T) {
	if shouldBroadcastReconciliationRefund(nil) {
		t.Fatal("nil result must not publish a balance event")
	}
	if shouldBroadcastReconciliationRefund(&ReconciliationRefundResult{AlreadyRefunded: true, AfterCents: 4600}) {
		t.Fatal("retry must not overwrite the UI with the original historical balance")
	}
	if !shouldBroadcastReconciliationRefund(&ReconciliationRefundResult{AfterCents: 4600}) {
		t.Fatal("a newly committed refund must publish its new balance")
	}
}
