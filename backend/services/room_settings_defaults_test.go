package services

import (
	workspacemodel "backend/data/models/workspace"
	"encoding/json"
	"reflect"
	"testing"
)

func decodeInitialRoomGameSettings(t *testing.T, raw string) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("decode initial room settings: %v", err)
	}
	return result
}

func TestInitialRoomGameSettingsCopiesCurrentPlatformSeal(t *testing.T) {
	for _, test := range []struct {
		name, raw string
		seconds   float64
	}{
		{"configured", `{"seal_seconds":45}`, 45},
		{"zero is configured", `{"seal_seconds":0}`, 0},
		{"missing", `{}`, 30},
		{"malformed legacy", `not-json`, 30},
		{"invalid legacy seal", `{"seal_seconds":-1}`, 30},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := decodeInitialRoomGameSettings(t, initialRoomGameSettings(test.raw))
			if got["seal_seconds"] != test.seconds {
				t.Fatalf("seal_seconds = %#v, want %v", got["seal_seconds"], test.seconds)
			}
			if got["max_open_games"] != float64(0) {
				t.Fatalf("new room no longer uses max_open_games=0: %#v", got)
			}
		})
	}
}

func TestInitialRoomGameSettingsCopiesOnlyTimingTemplate(t *testing.T) {
	platform := `{"seal_seconds":55,"game_timing_overrides":{"speed-racing":{"seal_seconds":15,"other_setting":true},"speed-fly":{"seal_seconds":0}},"allow_cancel":false,"max_open_games":99,"room_activity_enabled":true,"show_member_profit":false,"default_fly_rate":99,"agent_menu":["custom"]}`
	got := decodeInitialRoomGameSettings(t, initialRoomGameSettings(platform))
	wantDefaults := decodeInitialRoomGameSettings(t, string(normalizeGameSettings(`{"max_open_games":0}`)))
	for key, want := range wantDefaults {
		if key == "seal_seconds" {
			continue
		}
		if !reflect.DeepEqual(got[key], want) {
			t.Fatalf("unrelated default %s changed: %#v, want %#v", key, got[key], want)
		}
	}
	if got["seal_seconds"] != float64(55) {
		t.Fatalf("platform seal not copied: %#v", got)
	}
	wantOverrides := map[string]any{
		"speed-racing": map[string]any{"seal_seconds": float64(15)},
		"speed-fly":    map[string]any{"seal_seconds": float64(0)},
	}
	if !reflect.DeepEqual(got["game_timing_overrides"], wantOverrides) {
		t.Fatalf("per-game timing snapshot = %#v, want %#v", got["game_timing_overrides"], wantOverrides)
	}
	if _, copied := got["agent_menu"]; copied {
		t.Fatal("unrelated platform configuration was copied into a new room")
	}
}

func TestInitialRoomGameSettingsIgnoresInvalidLegacyOverrides(t *testing.T) {
	for _, raw := range []string{
		`{"game_timing_overrides":[]}`,
		`{"game_timing_overrides":{"speed-racing":{"seal_seconds":-1}}}`,
		`{"game_timing_overrides":{"speed-racing":{"seal_seconds":null}}}`,
		`{"game_timing_overrides":{"speed-racing":{"seal_seconds":"15"}}}`,
		`{"game_timing_overrides":{"":{"seal_seconds":15}}}`,
	} {
		got := decodeInitialRoomGameSettings(t, initialRoomGameSettings(raw))
		if _, exists := got["game_timing_overrides"]; exists {
			t.Fatalf("invalid override copied: %s -> %#v", raw, got)
		}
		if got["seal_seconds"] != float64(30) {
			t.Fatalf("default seal changed: %#v", got)
		}
	}
}

func TestInitialRoomGameSettingsReturnsIndependentSnapshots(t *testing.T) {
	first := initialRoomGameSettings(`{"seal_seconds":45,"game_timing_overrides":{"speed-racing":{"seal_seconds":10}}}`)
	second := initialRoomGameSettings(`{"seal_seconds":60,"game_timing_overrides":{"speed-racing":{"seal_seconds":20}}}`)
	if configuredSealSeconds(first, "speed-fly") != 45 || configuredSealSeconds(first, "speed-racing") != 10 {
		t.Fatal("creating another room changed the first room's timing snapshot")
	}
	if configuredSealSeconds(second, "speed-fly") != 60 || configuredSealSeconds(second, "speed-racing") != 20 {
		t.Fatal("later room did not copy the latest platform template")
	}
}

func TestRoomTimingSettingsChangedIgnoresUnrelatedAndFormattingChanges(t *testing.T) {
	for _, test := range []struct{ previous, next string }{
		{`{}`, `{"seal_seconds":30}`},
		{`{"seal_seconds":45}`, `{"allow_cancel":false,"seal_seconds":45,"max_open_games":100}`},
		{`{"game_timing_overrides":{"speed-racing":{"seal_seconds":0}},"seal_seconds":45}`, `{"seal_seconds":45,"game_timing_overrides":{"speed-racing":{"seal_seconds":0}}}`},
	} {
		if roomTimingSettingsChanged(test.previous, test.next) {
			t.Fatalf("non-timing change triggered invalidation: %s -> %s", test.previous, test.next)
		}
	}
	for _, test := range []struct{ previous, next string }{
		{`{"seal_seconds":45}`, `{"seal_seconds":30}`},
		{`{}`, `{"game_timing_overrides":{"speed-racing":{"seal_seconds":0}}}`},
		{`{"game_timing_overrides":{"speed-racing":{"seal_seconds":0}}}`, `{}`},
	} {
		if !roomTimingSettingsChanged(test.previous, test.next) {
			t.Fatalf("timing change did not invalidate: %s -> %s", test.previous, test.next)
		}
	}
}

func TestTimingSettingsNotificationStaysInsideSavedWorkspace(t *testing.T) {
	for _, workspace := range []workspacemodel.Workspace{
		{ID: 9, Type: workspacemodel.TypeAgent, Scope: "agent:7", RoomCode: "88001", Status: 1},
		{ID: 1, Type: workspacemodel.TypePlatform, Scope: "platform", Status: 1},
	} {
		calls := 0
		notifyRoomTimingSettingsChanged(workspace, `{"seal_seconds":30}`, `{"seal_seconds":45}`, func(workspaceID uint64, scope, code, gameID string, enabled bool) {
			calls++
			if workspaceID == 0 || workspaceID != workspace.ID || scope != workspace.Scope || code != workspace.RoomCode || gameID != "" || !enabled {
				t.Fatalf("timing invalidation escaped saved room: id=%d scope=%q code=%q game=%q", workspaceID, scope, code, gameID)
			}
		})
		if calls != 1 {
			t.Fatalf("workspace %d notifications = %d", workspace.ID, calls)
		}
	}
	for _, workspace := range []workspacemodel.Workspace{{ID: 0}, {ID: 9}} {
		previous, next := `{"seal_seconds":30}`, `{"seal_seconds":45}`
		if workspace.ID > 0 {
			next = previous
		}
		notifyRoomTimingSettingsChanged(workspace, previous, next, func(uint64, string, string, string, bool) {
			t.Fatalf("unchanged/unknown workspace must not broadcast: %+v", workspace)
		})
	}
}
