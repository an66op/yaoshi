package services

import (
	workspacemodel "backend/data/models/workspace"
	"context"
	"errors"
	"testing"
	"time"
)

func TestRoomActivityCancelledLeaseContextCannotStartOrRecordWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := &RoomActivityService{}
	setting := workspacemodel.RobotSetting{WorkspaceID: 99, Enabled: true}

	if err := service.runWorkspaceWithContext(ctx, setting); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled scheduler context returned %v", err)
	}
	if service.status.Cycles != 0 || service.status.BetsPlaced != 0 || !service.status.LastRunAt.IsZero() {
		t.Fatalf("cancelled scheduler mutated runtime status: %+v", service.status)
	}
	if err := service.finishWorkspaceRun(ctx, nil, setting, time.Now(), 1, 1, 1, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled completion returned %v", err)
	}
}

func TestClampRoomActivity(t *testing.T) {
	tests := []struct {
		name                      string
		value, min, max, fallback int
		want                      int
	}{
		{name: "keeps lower bound", value: 5, min: 5, max: 120, fallback: 10, want: 5},
		{name: "keeps upper bound", value: 120, min: 5, max: 120, fallback: 10, want: 120},
		{name: "replaces below range", value: 4, min: 5, max: 120, fallback: 10, want: 10},
		{name: "replaces above range", value: 121, min: 5, max: 120, fallback: 10, want: 10},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := clampRoomActivity(test.value, test.min, test.max, test.fallback); got != test.want {
				t.Fatalf("clampRoomActivity() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestResolveRoomActivityProcessPolicy(t *testing.T) {
	tests := []struct {
		name, mode, enabled, maximum string
		wantEnabled                  bool
		wantMaximum                  int
	}{
		{name: "release defaults off", mode: "release", enabled: "", maximum: "", wantEnabled: false},
		{name: "release explicit off", mode: "release", enabled: "0", maximum: "0", wantEnabled: false},
		{name: "release rejects truthy alias but preserves activation cap", mode: "release", enabled: "true", maximum: "2", wantEnabled: false, wantMaximum: 2},
		{name: "release requires positive cap", mode: "release", enabled: "1", maximum: "0", wantEnabled: false},
		{name: "release rejects ambiguous cap", mode: "release", enabled: "1", maximum: "01", wantEnabled: false},
		{name: "release rejects excessive cap", mode: "release", enabled: "1", maximum: "101", wantEnabled: false},
		{name: "release controlled enable", mode: "release", enabled: "1", maximum: "3", wantEnabled: true, wantMaximum: 3},
		{name: "release disabled keeps pre-activation cap", mode: "release", enabled: "0", maximum: "3", wantEnabled: false, wantMaximum: 3},
		{name: "debug keeps legacy default", mode: "debug", enabled: "", maximum: "", wantEnabled: true},
		{name: "test supports explicit off", mode: "test", enabled: "0", maximum: "4", wantEnabled: false},
		{name: "debug optional cap", mode: "debug", enabled: "1", maximum: "4", wantEnabled: true, wantMaximum: 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := resolveRoomActivityProcessPolicy(test.mode, test.enabled, test.maximum)
			if policy.enabled != test.wantEnabled || policy.maxWorkspaces != test.wantMaximum {
				t.Fatalf("policy = %#v, want enabled=%v maximum=%d", policy, test.wantEnabled, test.wantMaximum)
			}
			if !policy.enabled && policy.reason == "" {
				t.Fatal("disabled policy must explain why the scheduler is stopped")
			}
		})
	}
}

func TestValidateRoomActivityWorkspaceCountFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name       string
		enabled    int64
		maximum    int
		wantReject bool
	}{
		{name: "debug uncapped", enabled: 999, maximum: 0, wantReject: false},
		{name: "below production cap", enabled: 1, maximum: 2, wantReject: false},
		{name: "at production cap", enabled: 2, maximum: 2, wantReject: false},
		{name: "above production cap", enabled: 3, maximum: 2, wantReject: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateRoomActivityWorkspaceCount(test.enabled, test.maximum)
			if (err != nil) != test.wantReject {
				t.Fatalf("error = %v, want reject=%v", err, test.wantReject)
			}
			if test.wantReject && !errors.Is(err, ErrRoomActivityWorkspaceCap) {
				t.Fatalf("error = %v, want ErrRoomActivityWorkspaceCap", err)
			}
		})
	}
}

func TestRobotRunAllowanceProtectsDailyAndPendingBacklog(t *testing.T) {
	setting := workspacemodel.RobotSetting{DailyBetLimit: 200, MaxPendingBets: 50}
	if got, reason := robotRunAllowance(setting, 198, 10); got != 2 || reason != "" {
		t.Fatalf("daily allowance = %d, reason %q; want 2 and empty", got, reason)
	}
	if got, reason := robotRunAllowance(setting, 10, 49); got != 1 || reason != "" {
		t.Fatalf("pending allowance = %d, reason %q; want 1 and empty", got, reason)
	}
	if got, reason := robotRunAllowance(setting, 200, 0); got != 0 || reason == "" {
		t.Fatalf("daily guard = %d, reason %q; want stopped", got, reason)
	}
	if got, reason := robotRunAllowance(setting, 0, 50); got != 0 || reason == "" {
		t.Fatalf("pending guard = %d, reason %q; want stopped", got, reason)
	}
}

func TestRoomActivityStatusAccumulatesRuns(t *testing.T) {
	service := &RoomActivityService{}
	config := roomActivityConfig{
		Enabled: true, IntervalSecs: 8, BotsPerRoom: defaultWorkspaceRobotCount, BetsPerCycle: 2, ChatChancePct: 0,
	}
	firstRun := time.Date(2026, time.August, 24, 3, 10, 0, 0, time.UTC)
	service.recordActivityRun(config, firstRun, 2, 8, 12, 4, 0, nil)
	service.recordActivityRun(config, firstRun.Add(time.Second), 2, 8, 12, 3, 0, errors.New("temporary failure"))

	status := service.status
	if status.Cycles != 2 || status.BetsPlaced != 7 || status.ChatsPosted != 0 {
		t.Fatalf("unexpected totals: cycles=%d bets=%d chats=%d", status.Cycles, status.BetsPlaced, status.ChatsPosted)
	}
	if status.TargetRooms != 2 || status.EnabledGames != 8 || status.BotAccounts != 12 {
		t.Fatalf("unexpected coverage: rooms=%d games=%d bots=%d", status.TargetRooms, status.EnabledGames, status.BotAccounts)
	}
	if status.LastError != "temporary failure" {
		t.Fatalf("LastError = %q", status.LastError)
	}
	if !status.LastRunAt.Equal(firstRun.Add(time.Second)) {
		t.Fatalf("LastRunAt = %s", status.LastRunAt)
	}
}
