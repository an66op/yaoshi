package lotteryfeed

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSGSSCVerifiedJobIsSeparateAndBounded(t *testing.T) {
	count := 0
	for _, job := range DefaultJobs() {
		if job.Group != "sg-ssc-verified" {
			continue
		}
		count++
		if len(job.GameIDs) != 1 || job.GameIDs[0] != "sg-ssc" || job.Timeout != 15*time.Second || job.FastInterval != 15*time.Second || job.NormalInterval != 15*time.Second {
			t.Fatalf("SG source shares the wrong product group or unbounded cadence: %+v", job)
		}
	}
	if count != 1 {
		t.Fatalf("SG source scheduled %d times, want exactly once", count)
	}
}

func TestSchedulerEmitsOnlyFailureAndRecoveryTransitions(t *testing.T) {
	job := JobConfig{ID: "163-pc28", Name: "PC", Group: "163-pc28", GameIDs: []string{"pc-canada"}, Timeout: time.Second}
	results := [][]SyncResult{
		{{GameID: "pc-canada", Status: "error", Error: "上游开奖过期"}},
		{{GameID: "pc-canada", Status: "error", Error: "上游开奖过期"}},
		{{GameID: "pc-canada", Status: "ok", Imported: 3, LatestIssue: "3477941"}},
	}
	call := 0
	scheduler := NewScheduler([]JobConfig{job}, func(context.Context, string) []SyncResult {
		result := results[call]
		call++
		return result
	})
	var mu sync.Mutex
	events := make([]Event, 0)
	scheduler.SetEventSink(func(_ context.Context, event Event) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})
	scheduler.runOnce(context.Background(), job)
	scheduler.runOnce(context.Background(), job)
	scheduler.runOnce(context.Background(), job)

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 4 {
		t.Fatalf("repeated polling generated noisy events: %+v", events)
	}
	want := []string{"sync_error", "scheduler_error", "sync_recovered", "scheduler_recovered"}
	for index, eventType := range want {
		if events[index].Type != eventType {
			t.Fatalf("event %d type=%q want=%q; all=%+v", index, events[index].Type, eventType, events)
		}
	}
	if events[2].Imported != 3 || events[2].LatestIssue != "3477941" || events[2].GameID != "pc-canada" {
		t.Fatalf("recovery evidence missing: %+v", events[2])
	}
}

func TestSchedulerStandbyEventIsEmittedOnlyOnTransition(t *testing.T) {
	job := JobConfig{ID: "standby-job", Name: "standby", Group: "standby-group"}
	scheduler := NewScheduler([]JobConfig{job}, func(context.Context, string) []SyncResult { return nil })
	events := make([]Event, 0)
	scheduler.SetEventSink(func(_ context.Context, event Event) { events = append(events, event) })

	scheduler.markStandby(job)
	// The run loop updates the visible mode after every pass. Event deduplication
	// must not depend on that presentation field or every standby poll will log.
	scheduler.mu.Lock()
	status := scheduler.statuses[job.ID]
	status.Mode = "normal"
	scheduler.statuses[job.ID] = status
	scheduler.mu.Unlock()
	scheduler.markStandby(job)

	if len(events) != 1 || events[0].Type != "standby" {
		t.Fatalf("repeated standby polling emitted noisy events: %+v", events)
	}
}

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
