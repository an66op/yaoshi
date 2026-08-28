package services

import (
	"backend/data/models/lottery"
	"testing"
)

func TestSourceHealthyForGameUsesOnlyLiveSourceState(t *testing.T) {
	tests := []struct {
		name string
		game *lottery.Game
		want bool
	}{
		{name: "nil game", game: nil, want: false},
		{name: "platform source", game: &lottery.Game{SourceKind: "platform", SyncStatus: "error", LastSyncError: "historic reconciliation debt"}, want: true},
		{name: "external ok", game: &lottery.Game{SourceKind: "external", SyncStatus: "ok"}, want: true},
		{name: "official idle", game: &lottery.Game{SourceKind: "official", SyncStatus: "idle"}, want: true},
		{name: "external error", game: &lottery.Game{SourceKind: "external", SyncStatus: "error"}, want: false},
		{name: "external stale", game: &lottery.Game{SourceKind: "external", SyncStatus: "stale"}, want: false},
		{name: "official paused", game: &lottery.Game{SourceKind: "official", SyncStatus: "paused"}, want: false},
		{name: "retry after recovered error", game: &lottery.Game{SourceKind: "external", SyncStatus: "syncing", LastSyncError: "upstream timeout"}, want: false},
		{name: "first sync in progress", game: &lottery.Game{SourceKind: "external", SyncStatus: "syncing"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sourceHealthyForGame(tt.game); got != tt.want {
				t.Fatalf("sourceHealthyForGame() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCurrentIssueErrorRemainsSeparateFromSourceHealth(t *testing.T) {
	game := &lottery.Game{SourceKind: "external", SyncStatus: "ok"}
	issue := lottery.Issue{Status: lottery.IssueStatusError, LastError: "对账异常：历史机器人注单缺少可验证开奖"}

	if !sourceHealthyForGame(game) {
		t.Fatal("historic/current-period reconciliation state must not be reported as an upstream source outage")
	}
	if issue.Status != lottery.IssueStatusError {
		t.Fatal("the current issue error must remain available to the caller as an independent period-closing signal")
	}
}
