package services

import (
	"backend/data/models/lifecycle"
	"encoding/json"
	"strings"
	"testing"
)

func TestGameChatRetentionDefaultsAndBoundaries(t *testing.T) {
	spec := lifecycleSpecs[lifecycle.ClassGameChatMessages]
	if spec.DefaultDays != 7 || spec.MinimumDays != 1 || spec.Action != lifecycle.ActionSoftDelete {
		t.Fatalf("unexpected game chat defaults: %#v", spec)
	}
	for _, fragment := range []string{"message.room_type = 'group'", "lottery_games", "message.request_id = ''", "message.user_id = 0", "message.username = 'draw_assistant'", "'settlement', 'scoreboard'", "issue.status = 'settled'", "issue.last_error = ''", "issue.settled_at IS NOT NULL", "unsettled.status = 'pending'", "unsettled.reconciliation_status <> 'normal'", "unsettled.workspace_id = message.workspace_id", "unsettled.room_scope = message.room_scope", "latest.draw_at DESC", "newer.draw_at, newer.issue"} {
		if !strings.Contains(gameChatLifecyclePredicate, fragment) {
			t.Fatalf("game-room retention boundary missing %q", fragment)
		}
	}
	for _, predicate := range []string{genericChatLifecyclePredicate, robotChatLifecyclePredicate} {
		if !strings.Contains(predicate, "AND NOT ("+gameChatRoomPredicate+")") {
			t.Fatal("older chat categories overlap the game-room policy")
		}
	}
}

func TestGameChatPurgeDaysRequireExplicitScopedChoice(t *testing.T) {
	for _, test := range []struct {
		class   string
		value   int
		invalid bool
	}{
		{lifecycle.ClassGameChatMessages, 0, false}, {lifecycle.ClassGameChatMessages, 1, false},
		{lifecycle.ClassGameChatMessages, 3650, false}, {lifecycle.ClassGameChatMessages, -1, true},
		{lifecycle.ClassGameChatMessages, 3651, true}, {lifecycle.ClassChatMessages, 1, true},
		{lifecycle.ClassRobotChatMessages, 1, true}, {lifecycle.ClassNotifications, 1, true},
	} {
		if err := validateLifecyclePurgeDays(test.class, &test.value); (err != nil) != test.invalid {
			t.Fatalf("purge %s/%d: %v", test.class, test.value, err)
		}
	}
	if err := validateLifecyclePurgeDays(lifecycle.ClassGameChatMessages, nil); err != nil {
		t.Fatal("omitted old-client purge setting was rejected:", err)
	}
}

func TestGameChatRestoreReportsOriginalClasses(t *testing.T) {
	original := []CleanupResultItem{{DataClass: lifecycle.ClassGameChatMessages, AffectedCount: 3}, {DataClass: lifecycle.ClassRobotChatMessages, AffectedCount: 2}}
	raw, _ := json.Marshal(original)
	items, err := classifyRestoredChatResults([]CleanupResultItem{{DataClass: lifecycle.ClassChatMessages, AffectedCount: 5}}, string(raw))
	if err != nil || len(items) != 2 || items[0].DataClass != lifecycle.ClassGameChatMessages || items[0].AffectedCount != 3 || items[1].AffectedCount != 2 {
		t.Fatalf("restore classes: %#v %v", items, err)
	}
	if _, err := classifyRestoredChatResults([]CleanupResultItem{{DataClass: lifecycle.ClassChatMessages, AffectedCount: 4}}, string(raw)); err == nil {
		t.Fatal("incomplete restore was accepted")
	}
}
