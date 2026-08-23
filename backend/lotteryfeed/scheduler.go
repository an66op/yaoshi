// Package lotteryfeed contains the provider-neutral runtime used by the
// official draw feed. It deliberately has no database or HTTP framework
// dependency, so it can later move into a standalone draw-results service.
package lotteryfeed

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type SyncResult struct {
	GameID      string `json:"game_id"`
	Status      string `json:"status"`
	Imported    int    `json:"imported"`
	LatestIssue string `json:"latest_issue"`
	Error       string `json:"error,omitempty"`
}

type SyncFunc func(context.Context, string) []SyncResult

type JobConfig struct {
	ID             string
	Name           string
	Group          string
	GameIDs        []string
	Timezone       string
	FastStart      string
	FastEnd        string
	FastInterval   time.Duration
	NormalInterval time.Duration
	Timeout        time.Duration
}

type JobStatus struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Group          string    `json:"group"`
	GameIDs        []string  `json:"game_ids"`
	Timezone       string    `json:"timezone"`
	Mode           string    `json:"mode"`
	Running        bool      `json:"running"`
	LastStartedAt  time.Time `json:"last_started_at,omitempty"`
	LastFinishedAt time.Time `json:"last_finished_at,omitempty"`
	LastSuccessAt  time.Time `json:"last_success_at,omitempty"`
	NextRunAt      time.Time `json:"next_run_at,omitempty"`
	Imported       int       `json:"imported"`
	LatestIssue    string    `json:"latest_issue"`
	ConsecutiveErr int       `json:"consecutive_errors"`
	LastError      string    `json:"last_error,omitempty"`
}

type Status struct {
	Running      bool        `json:"running"`
	StartedAt    time.Time   `json:"started_at,omitempty"`
	ServerTime   time.Time   `json:"server_time"`
	ServerTimeMS int64       `json:"server_time_ms"`
	Timezone     string      `json:"timezone"`
	Jobs         []JobStatus `json:"jobs"`
}

type Scheduler struct {
	mu        sync.RWMutex
	jobs      []JobConfig
	statuses  map[string]JobStatus
	sync      SyncFunc
	running   bool
	startedAt time.Time
	startOnce sync.Once
}

func DefaultJobs() []JobConfig {
	return []JobConfig{
		{ID: "taiwan-bingo", Name: "台湾宾果即时开奖", Group: "taiwan-bingo", GameIDs: []string{"official-tw-bingo"}, Timezone: "Asia/Taipei", FastStart: "00:00", FastEnd: "23:59", FastInterval: 15 * time.Second, NormalInterval: 15 * time.Second, Timeout: 12 * time.Second},
		{ID: "china-welfare", Name: "中国福利彩票开奖", Group: "china-welfare", GameIDs: []string{"official-fc3d", "official-kl8"}, Timezone: "Asia/Shanghai", FastStart: "20:45", FastEnd: "22:30", FastInterval: 20 * time.Second, NormalInterval: 15 * time.Minute, Timeout: 40 * time.Second},
		{ID: "china-sport", Name: "中国体育彩票开奖", Group: "china-sport", GameIDs: []string{"official-pl3", "official-qxc"}, Timezone: "Asia/Shanghai", FastStart: "21:00", FastEnd: "22:30", FastInterval: 20 * time.Second, NormalInterval: 15 * time.Minute, Timeout: 40 * time.Second},
		{ID: "taiwan-lottery", Name: "台湾彩券晚间开奖", Group: "taiwan-lottery", GameIDs: []string{"official-tw-super-lotto", "official-tw-daily539", "official-tw-lotto649"}, Timezone: "Asia/Taipei", FastStart: "20:15", FastEnd: "22:15", FastInterval: 20 * time.Second, NormalInterval: 15 * time.Minute, Timeout: 20 * time.Second},
		{ID: "168-highfreq", Name: "168高频彩开奖", Group: "168-highfreq", GameIDs: []string{"speed-racing", "au-lucky-10", "au-lucky-5", "fly-racing", "speed-fly", "sg-fly", "speed-ssc"}, Timezone: "Asia/Shanghai", FastStart: "00:00", FastEnd: "23:59", FastInterval: 15 * time.Second, NormalInterval: 15 * time.Second, Timeout: 15 * time.Second},
		{ID: "168-bingo", Name: "168宾果映射开奖", Group: "168-bingo", GameIDs: []string{"bingo-ssc-1", "bingo-ssc-2", "bingo-ssc-3", "bingo-ssc-4", "bingo-racing-a", "bingo-racing-b", "bingo-mark-six"}, Timezone: "Asia/Taipei", FastStart: "00:00", FastEnd: "23:59", FastInterval: 20 * time.Second, NormalInterval: 20 * time.Second, Timeout: 15 * time.Second},
		{ID: "168-marksix", Name: "168六合彩开奖", Group: "168-marksix", GameIDs: []string{"hong-kong-mark-six", "new-macau-mark-six", "old-macau-mark-six"}, Timezone: "Asia/Shanghai", FastStart: "21:00", FastEnd: "22:30", FastInterval: 30 * time.Second, NormalInterval: 10 * time.Minute, Timeout: 20 * time.Second},
	}
}

func NewScheduler(jobs []JobConfig, syncFn SyncFunc) *Scheduler {
	statuses := make(map[string]JobStatus, len(jobs))
	for _, job := range jobs {
		statuses[job.ID] = JobStatus{ID: job.ID, Name: job.Name, Group: job.Group, GameIDs: append([]string(nil), job.GameIDs...), Timezone: job.Timezone, Mode: "waiting"}
	}
	return &Scheduler{jobs: append([]JobConfig(nil), jobs...), statuses: statuses, sync: syncFn}
}

func (s *Scheduler) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		s.mu.Lock()
		s.running = true
		s.startedAt = time.Now().UTC()
		s.mu.Unlock()
		for _, job := range s.jobs {
			go s.run(ctx, job)
		}
		go func() {
			<-ctx.Done()
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
		}()
	})
}

func (s *Scheduler) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	jobs := make([]JobStatus, 0, len(s.statuses))
	for _, item := range s.statuses {
		item.GameIDs = append([]string(nil), item.GameIDs...)
		jobs = append(jobs, item)
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].ID < jobs[j].ID })
	return Status{Running: s.running, StartedAt: s.startedAt, ServerTime: now, ServerTimeMS: now.UnixMilli(), Timezone: "Asia/Shanghai", Jobs: jobs}
}

func (s *Scheduler) run(ctx context.Context, job JobConfig) {
	// Each job runs immediately on process start to backfill anything missed
	// while the service was offline.
	for {
		if ctx.Err() != nil {
			return
		}
		s.runOnce(ctx, job)
		interval, mode := jobInterval(job, time.Now())
		s.mu.RLock()
		failures := s.statuses[job.ID].ConsecutiveErr
		s.mu.RUnlock()
		if failures > 0 {
			retry := retryInterval(failures)
			if retry < interval {
				interval = retry
			}
			mode = "retry"
		}
		next := alignedNext(time.Now(), interval)
		s.mu.Lock()
		status := s.statuses[job.ID]
		status.NextRunAt = next.UTC()
		status.Mode = mode
		s.statuses[job.ID] = status
		s.mu.Unlock()

		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}

func (s *Scheduler) runOnce(parent context.Context, job JobConfig) {
	started := time.Now().UTC()
	s.mu.Lock()
	status := s.statuses[job.ID]
	status.Running = true
	status.LastStartedAt = started
	status.LastError = ""
	s.statuses[job.ID] = status
	s.mu.Unlock()

	timeout := job.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	results := s.sync(ctx, job.Group)
	cancel()

	imported := 0
	latest := ""
	errors := make([]string, 0)
	for _, result := range results {
		imported += result.Imported
		if result.LatestIssue != "" {
			latest = result.LatestIssue
		}
		if result.Status != "ok" {
			message := result.Error
			if message == "" {
				message = result.GameID + " 同步失败"
			}
			errors = append(errors, message)
		}
	}
	if len(results) == 0 {
		errors = append(errors, "同步任务未返回结果")
	}

	finished := time.Now().UTC()
	s.mu.Lock()
	status = s.statuses[job.ID]
	status.Running = false
	status.LastFinishedAt = finished
	status.Imported = imported
	status.LatestIssue = latest
	if len(errors) == 0 {
		status.LastSuccessAt = finished
		status.ConsecutiveErr = 0
		status.LastError = ""
	} else {
		status.ConsecutiveErr++
		status.LastError = truncate(strings.Join(errors, "; "), 500)
	}
	s.statuses[job.ID] = status
	s.mu.Unlock()
}

func jobInterval(job JobConfig, now time.Time) (time.Duration, string) {
	location, err := time.LoadLocation(job.Timezone)
	if err != nil {
		location = time.UTC
	}
	local := now.In(location)
	minute := local.Hour()*60 + local.Minute()
	start, startErr := clockMinute(job.FastStart)
	end, endErr := clockMinute(job.FastEnd)
	if startErr == nil && endErr == nil && inWindow(minute, start, end) {
		return positiveDuration(job.FastInterval, time.Minute), "draw-window"
	}
	return positiveDuration(job.NormalInterval, 10*time.Minute), "normal"
}

func clockMinute(value string) (int, error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, fmt.Errorf("invalid clock %q: %w", value, err)
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}

func inWindow(value, start, end int) bool {
	if start <= end {
		return value >= start && value <= end
	}
	return value >= start || value <= end
}

func alignedNext(now time.Time, interval time.Duration) time.Time {
	interval = positiveDuration(interval, time.Minute)
	return now.Truncate(interval).Add(interval)
}

func positiveDuration(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

func retryInterval(failures int) time.Duration {
	if failures < 1 {
		return 0
	}
	if failures > 5 {
		failures = 5
	}
	return time.Duration(1<<(failures-1)) * 30 * time.Second
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
