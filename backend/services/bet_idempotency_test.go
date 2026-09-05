package services

import (
	"backend/data/models/bet"
	"backend/data/models/user"
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestIdempotencyReservationTimeoutBoundary(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	updatedAt := now.Add(-idempotencyReservationTimeout)
	if !idempotencyReservationExpired(updatedAt, now) {
		t.Fatal("a reservation at the exact recovery boundary must be recoverable")
	}
	if idempotencyReservationExpired(updatedAt.Add(time.Nanosecond), now) {
		t.Fatal("an in-flight reservation must not be stolen before the timeout")
	}
}

func TestIdempotencyRecoveryStopsBeforeDatabaseWorkWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&BetAdminService{}).RecoverStaleIdempotencyRequests(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation before database access, got %v", err)
	}
}

func TestIdempotencyRecoveryRequiresMatchingBetEvidence(t *testing.T) {
	ledger := user.BalanceTransaction{
		WorkspaceID: 88, UserID: 9, Reference: "assistant_request:77", Type: "bet",
		AmountCents: -2500, BeforeCents: 10000, AfterCents: 7500,
	}
	rows := []bet.Bet{
		{WorkspaceID: 88, UserID: 9, RequestReference: ledger.Reference, GameID: "pk10", Issue: "20260828-001", AmountCents: 1000},
		{WorkspaceID: 88, UserID: 9, RequestReference: ledger.Reference, GameID: "pk10", Issue: "20260828-001", AmountCents: 1500},
	}
	if err := validateIdempotencyBetEvidence(ledger, rows, 9, 88, ledger.Reference, false); err != nil {
		t.Fatalf("matching assistant bet evidence rejected: %v", err)
	}

	cases := map[string]struct {
		ledger user.BalanceTransaction
		rows   []bet.Bet
	}{
		"missing bets":      {ledger: ledger},
		"wrong ledger type": {ledger: func() user.BalanceTransaction { value := ledger; value.Type = "manual"; return value }(), rows: rows},
		"wrong reference": {ledger: ledger, rows: func() []bet.Bet {
			value := append([]bet.Bet(nil), rows...)
			value[0].RequestReference = "assistant_request:78"
			return value
		}()},
		"mixed issue": {ledger: ledger, rows: func() []bet.Bet {
			value := append([]bet.Bet(nil), rows...)
			value[1].Issue = "20260828-002"
			return value
		}()},
		"wrong total": {ledger: ledger, rows: func() []bet.Bet { value := append([]bet.Bet(nil), rows...); value[1].AmountCents = 1499; return value }()},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validateIdempotencyBetEvidence(test.ledger, test.rows, 9, 88, ledger.Reference, false); err == nil {
				t.Fatal("unsafe recovery evidence was accepted")
			}
		})
	}
}

func TestDirectIdempotencyRecoveryRequiresOneBet(t *testing.T) {
	ledger := user.BalanceTransaction{
		WorkspaceID: 88, UserID: 9, Reference: "bet_request:77", Type: "bet",
		AmountCents: -2500, BeforeCents: 10000, AfterCents: 7500,
	}
	row := bet.Bet{WorkspaceID: 88, UserID: 9, RequestReference: ledger.Reference, GameID: "pk10", Issue: "20260828-001", AmountCents: 2500}
	if err := validateIdempotencyBetEvidence(ledger, []bet.Bet{row}, 9, 88, ledger.Reference, true); err != nil {
		t.Fatalf("matching direct bet evidence rejected: %v", err)
	}
	if err := validateIdempotencyBetEvidence(ledger, []bet.Bet{row, row}, 9, 88, ledger.Reference, true); err == nil {
		t.Fatal("direct recovery accepted multiple matching bet rows")
	}
}

func TestIdempotencyRecoveryPoisonRowDoesNotAbortBatch(t *testing.T) {
	result := IdempotencyRecoveryResult{}
	accumulateIdempotencyRecovery(&result, 1, "direct", false, false, false, errors.New("poison row"))
	accumulateIdempotencyRecovery(&result, 2, "direct", true, false, false, nil)
	accumulateIdempotencyRecovery(&result, 3, "assistant", false, true, true, nil)

	if result.Scanned != 3 || result.Completed != 1 || result.Failed != 1 || result.Errors != 2 {
		t.Fatalf("poison row aborted or corrupted batch counters: %#v", result)
	}
}

func TestIdempotencyRecoveryQuarantinesOnlyNonContextErrors(t *testing.T) {
	if !shouldQuarantineIdempotencyRecovery(context.Background(), errors.New("poison row")) {
		t.Fatal("repeatable row error was not selected for quarantine")
	}
	if shouldQuarantineIdempotencyRecovery(context.Background(), context.Canceled) {
		t.Fatal("canceled recovery must not mutate the reservation")
	}
	if shouldQuarantineIdempotencyRecovery(context.Background(), context.DeadlineExceeded) {
		t.Fatal("timed out recovery must not mutate the reservation")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if shouldQuarantineIdempotencyRecovery(canceled, errors.New("database error")) {
		t.Fatal("recovery must not quarantine after its context is canceled")
	}
}

func TestIdempotencyReservationDecisionIsConcurrencySafe(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	updatedAt := now.Add(-idempotencyReservationTimeout - time.Second)
	const workers = 128
	results := make(chan bool, workers)
	var group sync.WaitGroup
	for i := 0; i < workers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			results <- idempotencyReservationExpired(updatedAt, now)
		}()
	}
	group.Wait()
	close(results)
	for recoverable := range results {
		if !recoverable {
			t.Fatal("concurrent callers disagreed about a frozen reservation boundary")
		}
	}
}

func TestLegacyIdempotencyRecoveryFailsClosedOnInvalidLedger(t *testing.T) {
	valid := user.BalanceTransaction{
		UserID: 9, Reference: "bet_request:77", AmountCents: -2500,
		BeforeCents: 10000, AfterCents: 7500, Type: "bet",
	}
	if err := validateIdempotencyDebitLedger(valid); err != nil {
		t.Fatalf("valid debit evidence rejected: %v", err)
	}

	cases := map[string]user.BalanceTransaction{
		"credit instead of debit": valid,
		"broken arithmetic":       valid,
		"negative result":         valid,
		"missing reference":       valid,
		"missing user":            valid,
	}
	row := cases["credit instead of debit"]
	row.AmountCents = 2500
	cases["credit instead of debit"] = row
	row = cases["broken arithmetic"]
	row.AfterCents++
	cases["broken arithmetic"] = row
	row = cases["negative result"]
	row.BeforeCents = 100
	row.AfterCents = -2400
	cases["negative result"] = row
	row = cases["missing reference"]
	row.Reference = ""
	cases["missing reference"] = row
	row = cases["missing user"]
	row.UserID = 0
	cases["missing user"] = row

	for name, ledger := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validateIdempotencyDebitLedger(ledger); err == nil {
				t.Fatal("unsafe ledger evidence was accepted")
			}
		})
	}
}

func TestIdempotencyReferencesAreStableAndSeparated(t *testing.T) {
	if directBetRequestReference(41) != "bet_request:41" {
		t.Fatalf("unexpected direct reference: %q", directBetRequestReference(41))
	}
	if assistantBetRequestReference(41) != "assistant_request:41" {
		t.Fatalf("unexpected assistant reference: %q", assistantBetRequestReference(41))
	}
	if directBetRequestReference(41) == assistantBetRequestReference(41) {
		t.Fatal("direct and assistant requests must never share a ledger namespace")
	}
}

func TestIdempotencyLedgerMustMatchFrozenRequestScope(t *testing.T) {
	ledger := user.BalanceTransaction{
		WorkspaceID: 88, UserID: 9, Reference: "bet_request:77", AmountCents: -2500,
		BeforeCents: 10000, AfterCents: 7500, Type: "bet",
	}
	if err := validateIdempotencyRequestLedger(ledger, 9, 88, "bet_request:77"); err != nil {
		t.Fatalf("matching frozen request evidence was rejected: %v", err)
	}
	cases := []struct {
		name        string
		userID      uint64
		workspaceID uint64
		reference   string
	}{
		{name: "other user", userID: 10, workspaceID: 88, reference: "bet_request:77"},
		{name: "other room", userID: 9, workspaceID: 89, reference: "bet_request:77"},
		{name: "other operation", userID: 9, workspaceID: 88, reference: "assistant_request:77"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := validateIdempotencyRequestLedger(ledger, test.userID, test.workspaceID, test.reference); err == nil {
				t.Fatal("cross-scope financial evidence was accepted")
			}
		})
	}
}
