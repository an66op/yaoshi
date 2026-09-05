package services

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSyncOfficialGroupDoesNotWaitForBusyGroupAfterCancellation(t *testing.T) {
	gate := officialGroupLocks["china-welfare"]
	gate <- struct{}{}
	defer func() { <-gate }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan []SourceSyncResult, 1)
	go func() {
		done <- (&LotteryService{}).SyncOfficialGroup(ctx, "china-welfare")
	}()

	select {
	case results := <-done:
		if len(results) != 1 || results[0].Status != "error" || !strings.Contains(results[0].Error, "已取消") {
			t.Fatalf("unexpected cancelled group result: %+v", results)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled source sync remained blocked on the group gate")
	}
}
