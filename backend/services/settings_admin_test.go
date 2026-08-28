package services

import (
	"encoding/json"
	"testing"
)

func TestNormalizeGameSettingsBackfillsLegacyDisplayFields(t *testing.T) {
	var result map[string]any
	if err := json.Unmarshal(normalizeGameSettings(`{"seal_seconds":45,"show_member_profit":false}`), &result); err != nil {
		t.Fatalf("decode normalized settings: %v", err)
	}
	if result["seal_seconds"] != float64(45) {
		t.Fatalf("stored value was not preserved: %#v", result["seal_seconds"])
	}
	if result["show_member_profit"] != false {
		t.Fatalf("explicit false was not preserved: %#v", result["show_member_profit"])
	}
	if result["room_activity_bots_per_room"] != float64(defaultWorkspaceRobotCount) {
		t.Fatalf("robot default = %#v, want %d", result["room_activity_bots_per_room"], defaultWorkspaceRobotCount)
	}
	for _, key := range []string{"show_member_turnover", "show_member_rebate", "show_orders_tool"} {
		if result[key] != true {
			t.Fatalf("legacy field %s was not enabled: %#v", key, result[key])
		}
	}
}
