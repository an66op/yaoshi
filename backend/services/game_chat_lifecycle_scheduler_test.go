package services

import (
	"backend/data/models/lifecycle"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestNextGameChatLifecycleRun(t *testing.T) {
	location := beijingLifecycleLocation()
	for _, test := range []struct{ now, want time.Time }{
		{time.Date(2026, 8, 31, 3, 9, 59, 0, location), time.Date(2026, 8, 31, 3, 10, 0, 0, location)},
		{time.Date(2026, 8, 31, 3, 10, 0, 0, location), time.Date(2026, 8, 31, 4, 10, 0, 0, location)},
		{time.Date(2026, 8, 31, 23, 59, 0, 0, location), time.Date(2026, 9, 1, 0, 10, 0, 0, location)},
	} {
		if got := nextGameChatLifecycleRun(test.now.UTC()); !got.Equal(test.want) {
			t.Fatalf("next=%v want=%v", got, test.want)
		}
	}
}

func TestGameChatCleanupRequiresSeparatePurgeOptIn(t *testing.T) {
	policy := lifecycle.RetentionPolicy{DataClass: lifecycle.ClassGameChatMessages, PurgeAfterDays: 30}
	if got := gameChatCleanupModes(policy); len(got) != 0 {
		t.Fatal("disabled policy ran", got)
	}
	policy.Enabled, policy.PurgeAfterDays = true, 0
	if got := gameChatCleanupModes(policy); !reflect.DeepEqual(got, []string{DeleteModeSoft}) {
		t.Fatal("soft retention authorized a permanent purge", got)
	}
	policy.PurgeAfterDays = 30
	if got := gameChatCleanupModes(policy); !reflect.DeepEqual(got, []string{DeleteModeSoft, DeleteModeHard}) {
		t.Fatal(got)
	}
	policy.DataClass = lifecycle.ClassRobotTestData
	if got := gameChatCleanupModes(policy); len(got) != 0 {
		t.Fatal("financial class entered content scheduler", got)
	}
}

func TestGameChatCleanupBatchesAreBoundedAndIdempotentlyNamed(t *testing.T) {
	at := time.Date(2026, 8, 31, 14, 10, 0, 0, beijingLifecycleLocation())
	var ids []string
	err := runGameChatCleanupBatches(context.Background(), at, 9, DeleteModeSoft, func(id string, limit int) (int64, error) {
		if limit != 1000 || !cleanupRequestIDPattern.MatchString(id) {
			t.Fatalf("unexpected batch %q/%d", id, limit)
		}
		ids = append(ids, id)
		return int64(limit), nil
	})
	if err != nil || len(ids) != 5 || ids[0] != "gamechat:2026083114:ws:9:soft:01" {
		t.Fatalf("ids=%v error=%v", ids, err)
	}
	var retried []string
	_ = runGameChatCleanupBatches(context.Background(), at.UTC(), 9, DeleteModeSoft, func(id string, _ int) (int64, error) {
		retried = append(retried, id)
		return 1, nil
	})
	if !reflect.DeepEqual(ids, retried) {
		t.Fatal("same scheduled run generated different request ids", ids, retried)
	}
}

func TestGameChatCleanupStopsAtEmptyErrorOrCancellation(t *testing.T) {
	at := time.Now()
	calls := 0
	if err := runGameChatCleanupBatches(context.Background(), at, 1, DeleteModeSoft, func(string, int) (int64, error) {
		calls++
		return 0, nil
	}); err != nil || calls != 1 {
		t.Fatalf("empty calls=%d error=%v", calls, err)
	}
	wantErr := errors.New("batch failed")
	if err := runGameChatCleanupBatches(context.Background(), at, 1, DeleteModeSoft, func(string, int) (int64, error) {
		return 0, wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls = 0
	err := runGameChatCleanupBatches(ctx, at, 1, DeleteModeSoft, func(string, int) (int64, error) {
		calls++
		cancel()
		return 1000, nil
	})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("cancellation calls=%d error=%v", calls, err)
	}
}
