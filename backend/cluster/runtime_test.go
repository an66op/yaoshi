package cluster

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestInitRequiresAddressInReleaseMode(t *testing.T) {
	t.Cleanup(func() { _ = Init(context.Background(), Options{}) })
	if err := Init(context.Background(), Options{Required: true}); err == nil {
		t.Fatal("required Redis accepted an empty address")
	}
	if !Required() {
		t.Fatal("required mode was not retained for fail-closed callers")
	}
}

func TestOptionalRedisUsesDisabledFallback(t *testing.T) {
	if err := Init(context.Background(), Options{Prefix: " room test "}); err != nil {
		t.Fatal(err)
	}
	if Enabled() || Required() || Configured() {
		t.Fatal("optional empty Redis configuration must use local fallback")
	}
	if got := Key("rate", "auth", "member 9"); got != "room-test:rate:auth:member_9" {
		t.Fatalf("unexpected namespaced key %q", got)
	}
	if strings.Contains(Key("ws-ticket", "abc"), " ") {
		t.Fatal("Redis keys must not contain spaces")
	}
}

func TestFailedRequiredRedisReinitializationDiscardsOldClient(t *testing.T) {
	t.Cleanup(func() { _ = Init(context.Background(), Options{}) })
	for _, empty := range []bool{false, true} {
		server := miniredis.RunT(t)
		if err := Init(context.Background(), Options{Addr: server.Addr()}); err != nil {
			t.Fatal(err)
		}
		if !Configured() || !Enabled() {
			t.Fatal("connected Redis not marked configured")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		address := "127.0.0.1:1"
		if empty {
			address = ""
		}
		if err := Init(ctx, Options{Addr: address, Required: true}); err == nil {
			t.Fatal("required failure not reported")
		}
		if Enabled() || !Required() || Configured() == empty {
			t.Fatal("failed reconfiguration retained an old client or lost required/configured state")
		}
	}
}

func TestRunWithLeaseAllowsOptionalLocalFallback(t *testing.T) {
	t.Cleanup(func() { _ = Init(context.Background(), Options{}) })
	if err := Init(context.Background(), Options{}); err != nil {
		t.Fatal(err)
	}
	runs := 0
	executed, err := RunWithLease(context.Background(), "test-local-fallback", time.Minute, func(context.Context) error {
		runs++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !executed || runs != 1 {
		t.Fatalf("expected one local execution, executed=%v runs=%d", executed, runs)
	}
}

func TestRunWithLeaseFailsClosedWhenRedisRequired(t *testing.T) {
	t.Cleanup(func() { _ = Init(context.Background(), Options{}) })
	if err := Init(context.Background(), Options{Required: true}); err == nil {
		t.Fatal("expected required Redis initialization to fail without an address")
	}
	runs := 0
	executed, err := RunWithLease(context.Background(), "test-required", time.Minute, func(context.Context) error {
		runs++
		return nil
	})
	if executed || runs != 0 {
		t.Fatalf("required runtime executed without Redis, executed=%v runs=%d", executed, runs)
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}

func TestRunWithLeaseRenewsLongRunningWork(t *testing.T) {
	t.Cleanup(func() { _ = Init(context.Background(), Options{}) })
	server := miniredis.RunT(t)
	if err := Init(context.Background(), Options{Addr: server.Addr(), Required: true}); err != nil {
		t.Fatal(err)
	}

	workStarted := make(chan struct{})
	workDone := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, err := RunWithLease(context.Background(), "test-renewal", time.Second, func(context.Context) error {
			close(workStarted)
			<-workDone
			return nil
		})
		result <- err
	}()
	<-workStarted

	// Wait beyond the original TTL. A second instance must still be unable to
	// acquire the lease because the first worker renews it in the background.
	time.Sleep(1300 * time.Millisecond)
	second, acquired, err := AcquireLease(context.Background(), "test-renewal", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if acquired {
		_ = second.Release(context.Background())
		t.Fatal("second worker acquired a lease while the first worker was active")
	}
	close(workDone)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestRunWithLeaseCancelsWorkWhenRenewalLosesOwnership(t *testing.T) {
	t.Cleanup(func() { _ = Init(context.Background(), Options{}) })
	server := miniredis.RunT(t)
	if err := Init(context.Background(), Options{Addr: server.Addr(), Required: true}); err != nil {
		t.Fatal(err)
	}

	workStarted := make(chan struct{})
	workCancelled := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, err := RunWithLease(context.Background(), "test-renewal-loss", time.Second, func(ctx context.Context) error {
			close(workStarted)
			<-ctx.Done()
			close(workCancelled)
			return ctx.Err()
		})
		result <- err
	}()
	<-workStarted
	server.Set(Key("lock", "test-renewal-loss"), "replacement-owner")

	select {
	case <-workCancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("lease loss did not cancel blocked work")
	}
	select {
	case err := <-result:
		if !errors.Is(err, ErrLeaseLost) || !errors.Is(err, context.Canceled) {
			t.Fatalf("expected joined lease-loss and cancellation error, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("lease-loss run did not return")
	}
	if value, _ := server.Get(Key("lock", "test-renewal-loss")); value != "replacement-owner" {
		t.Fatalf("stale release changed replacement owner: %q", value)
	}
}

func TestRunWithLeaseReleasesOwnershipWhenWorkPanics(t *testing.T) {
	t.Cleanup(func() { _ = Init(context.Background(), Options{}) })
	server := miniredis.RunT(t)
	if err := Init(context.Background(), Options{Addr: server.Addr(), Required: true}); err != nil {
		t.Fatal(err)
	}

	func() {
		defer func() {
			if recovered := recover(); recovered != "scheduler panic" {
				t.Fatalf("unexpected recovered panic: %v", recovered)
			}
		}()
		_, _ = RunWithLease(context.Background(), "test-panic-cleanup", time.Second, func(context.Context) error {
			panic("scheduler panic")
		})
	}()

	lease, acquired, err := AcquireLease(context.Background(), "test-panic-cleanup", time.Second)
	if err != nil || !acquired {
		t.Fatalf("panic left scheduler lease owned: acquired=%v err=%v", acquired, err)
	}
	if err := lease.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLeaseRenewDoesNotReviveReplacedOwner(t *testing.T) {
	t.Cleanup(func() { _ = Init(context.Background(), Options{}) })
	server := miniredis.RunT(t)
	if err := Init(context.Background(), Options{Addr: server.Addr(), Required: true}); err != nil {
		t.Fatal(err)
	}
	lease, acquired, err := AcquireLease(context.Background(), "test-replaced", time.Second)
	if err != nil || !acquired {
		t.Fatalf("acquire lease: acquired=%v err=%v", acquired, err)
	}
	server.Set(lease.key, "another-owner")
	if err := lease.Renew(context.Background()); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("expected ErrLeaseLost, got %v", err)
	}
	if value, _ := server.Get(lease.key); value != "another-owner" {
		t.Fatalf("stale renew changed replacement token: %q", value)
	}
}
