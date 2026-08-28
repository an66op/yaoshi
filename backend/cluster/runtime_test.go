package cluster

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
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
	if Enabled() || Required() {
		t.Fatal("optional empty Redis configuration must use local fallback")
	}
	if got := Key("rate", "auth", "member 9"); got != "room-test:rate:auth:member_9" {
		t.Fatalf("unexpected namespaced key %q", got)
	}
	if strings.Contains(Key("ws-ticket", "abc"), " ") {
		t.Fatal("Redis keys must not contain spaces")
	}
}

func TestRunWithLeaseAllowsOptionalLocalFallback(t *testing.T) {
	t.Cleanup(func() { _ = Init(context.Background(), Options{}) })
	if err := Init(context.Background(), Options{}); err != nil {
		t.Fatal(err)
	}
	runs := 0
	executed, err := RunWithLease(context.Background(), "test-local-fallback", time.Minute, func() error {
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
	executed, err := RunWithLease(context.Background(), "test-required", time.Minute, func() error {
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
