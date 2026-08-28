package lotteryfeed

import (
	"context"
	"testing"
	"time"
)

func TestJobIntervalUsesDrawWindow(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	job := JobConfig{Timezone: "Asia/Shanghai", FastStart: "21:00", FastEnd: "22:30", FastInterval: 20 * time.Second, NormalInterval: 15 * time.Minute}
	interval, mode := jobInterval(job, time.Date(2026, 8, 21, 21, 25, 0, 0, location))
	if interval != 20*time.Second || mode != "draw-window" {
		t.Fatalf("expected draw window, got %s %s", interval, mode)
	}
	interval, mode = jobInterval(job, time.Date(2026, 8, 21, 12, 0, 0, 0, location))
	if interval != 15*time.Minute || mode != "normal" {
		t.Fatalf("expected normal mode, got %s %s", interval, mode)
	}
}

func TestInWindowSupportsMidnight(t *testing.T) {
	if !inWindow(30, 23*60, 60) || !inWindow(23*60+30, 23*60, 60) || inWindow(12*60, 23*60, 60) {
		t.Fatal("midnight-spanning window calculation is incorrect")
	}
}

func TestRetryIntervalBacksOffAndCaps(t *testing.T) {
	if retryInterval(1) != 30*time.Second || retryInterval(3) != 2*time.Minute || retryInterval(99) != 8*time.Minute {
		t.Fatal("retry backoff is incorrect")
	}
}

func TestTruncatePreservesUnicode(t *testing.T) {
	if got := truncate("开奖源读取失败", 4); got != "开奖源读" {
		t.Fatalf("truncate returned %q", got)
	}
}

func TestSchedulerRunsImmediately(t *testing.T) {
	called := make(chan struct{}, 1)
	job := JobConfig{ID: "test", Name: "test", Group: "test", Timezone: "Asia/Shanghai", FastStart: "00:00", FastEnd: "23:59", FastInterval: time.Hour, NormalInterval: time.Hour, Timeout: time.Second}
	scheduler := NewScheduler([]JobConfig{job}, func(context.Context, string) []SyncResult {
		called <- struct{}{}
		return []SyncResult{{GameID: "game", Status: "ok", Imported: 1, LatestIssue: "100"}}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scheduler.Start(ctx)
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not run immediately")
	}
	deadline := time.Now().Add(time.Second)
	for {
		status := scheduler.Status()
		if len(status.Jobs) == 1 && !status.Jobs[0].LastSuccessAt.IsZero() {
			if status.Jobs[0].LatestIssue != "100" || status.Jobs[0].Imported != 1 {
				t.Fatalf("unexpected job status: %+v", status.Jobs[0])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("scheduler status was not updated")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
