package services

import (
	"backend/data/models/lottery"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestLimitDBTextPreservesUnicodeAndColumnLimit(t *testing.T) {
	got := limitDBText("  开奖结算失败：数据库暂时繁忙  ", 8)
	if got != "开奖结算失败：数" {
		t.Fatalf("unexpected truncated value %q", got)
	}
	if !strings.HasPrefix(limitDBText("正常", 500), "正常") {
		t.Fatal("short text must remain unchanged")
	}
}

func TestReconciliationIssueErrorIsStable(t *testing.T) {
	first := reconciliationIssueError("等待开奖超时")
	second := reconciliationIssueError(first)
	if first != "对账异常：等待开奖超时" || second != first {
		t.Fatalf("reconciliation marker must be idempotent, got %q then %q", first, second)
	}
}

func TestRetryableTransactionErrorRecognizesOnlyConcurrencyFailures(t *testing.T) {
	for _, code := range []string{"40P01", "40001", "55P03"} {
		if !isRetryableTransactionError(&pgconn.PgError{Code: code}) {
			t.Fatalf("PostgreSQL code %s must be retried", code)
		}
	}
	if !isRetryableTransactionError(errors.New("deadlock detected while updating account")) {
		t.Fatal("wrapped driver deadlock text must be retried")
	}
	if isRetryableTransactionError(errors.New("invalid draw numbers")) {
		t.Fatal("business/data errors must never be retried")
	}
}

func TestRecoveryActionSettlesOnlyVerifiedDraws(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	action, _ := recoveryActionForCandidate(settlementCandidate{GameExists: true, GameEnabled: false, DrawID: 9}, now)
	if action != recoverySettle {
		t.Fatal("an immutable draw is safe to settle even after the game is disabled")
	}
	action, _ = recoveryActionForCandidate(settlementCandidate{GameExists: true, GameEnabled: false}, now)
	if action != recoveryMarkAbnormal {
		t.Fatal("a disabled game without a draw must be reconciled, not generated")
	}
	action, _ = recoveryActionForCandidate(settlementCandidate{GameExists: false, DrawID: 9}, now)
	if action != recoveryMarkAbnormal {
		t.Fatal("a draw cannot be evaluated without its game rules")
	}
}

func TestRecoveryActionDefersFreshMissingIssueAndMarksStaleOne(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	fresh := settlementCandidate{GameExists: true, GameEnabled: true, OldestBetAt: now.Add(-time.Minute)}
	action, _ := recoveryActionForCandidate(fresh, now)
	if action != recoveryDefer {
		t.Fatal("a fresh placement must be allowed time to materialize its lifecycle")
	}
	stale := fresh
	stale.OldestBetAt = now.Add(-missingIssueStaleAfter - time.Second)
	action, _ = recoveryActionForCandidate(stale, now)
	if action != recoveryMarkAbnormal {
		t.Fatal("a stale bet with no issue and no draw must be queued for review")
	}
}

func TestRecoveryActionHandlesStaleLifecycle(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	freshAt := now.Add(-time.Minute)
	staleAt := now.Add(-settlementStaleAfter - time.Second)
	action, _ := recoveryActionForCandidate(settlementCandidate{
		GameExists: true, GameEnabled: true, IssueID: 1,
		IssueStatus: lottery.IssueStatusSettling, IssueSealAt: &freshAt,
	}, now)
	if action != recoveryDefer {
		t.Fatal("an active settlement must not be interrupted")
	}
	action, _ = recoveryActionForCandidate(settlementCandidate{
		GameExists: true, GameEnabled: true, IssueID: 1,
		IssueStatus: lottery.IssueStatusSettling, IssueSealAt: &staleAt,
	}, now)
	if action != recoveryMarkAbnormal {
		t.Fatal("a stale settlement without a draw must be marked abnormal")
	}
}
