package services

import (
	workspacemodel "backend/data/models/workspace"
	"errors"
	"testing"
	"time"
)

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
