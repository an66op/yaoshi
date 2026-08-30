package services

import (
	workspacemodel "backend/data/models/workspace"
	"encoding/json"
	"strings"

	"gorm.io/gorm"
)

// initialRoomGameSettings copies only the platform's timing template into a
// new room. The returned JSON is a snapshot owned by that room: later platform
// edits must not rewrite it or become a runtime fallback. All unrelated room
// defaults (including closed games/robots and max_open_games=0) stay unchanged.
func initialRoomGameSettings(platformRaw string) string {
	var room map[string]any
	_ = json.Unmarshal(normalizeGameSettings(`{"max_open_games":0}`), &room)
	var platform map[string]json.RawMessage
	if json.Unmarshal([]byte(platformRaw), &platform) == nil {
		if seconds, ok := timingSeconds(platform["seal_seconds"]); ok {
			room["seal_seconds"] = seconds
		}
		var platformOverrides map[string]map[string]json.RawMessage
		if json.Unmarshal(platform["game_timing_overrides"], &platformOverrides) == nil {
			overrides := make(map[string]map[string]int)
			for gameID, timing := range platformOverrides {
				if strings.TrimSpace(gameID) == "" || len(gameID) > 40 {
					continue
				}
				if seconds, ok := timingSeconds(timing["seal_seconds"]); ok {
					overrides[gameID] = map[string]int{"seal_seconds": seconds}
				}
			}
			if len(overrides) > 0 {
				room["game_timing_overrides"] = overrides
			}
		}
	}
	encoded, _ := json.Marshal(room)
	return string(encoded)
}

// Both creation paths read the actual platform row, not whichever settings row
// happens to have the smallest ID. Missing platform settings use the existing
// 30-second bootstrap default; no platform row is created or mutated here.
func initialRoomGameSettingsFromPlatform(db *gorm.DB) (string, error) {
	platformRaw, _, err := readTimingSettings(db, 0)
	if err != nil {
		return "", err
	}
	return initialRoomGameSettings(platformRaw), nil
}

func roomTimingSettingsChanged(previous, next string) bool {
	// Canonical snapshots omit unrelated settings and normalize the default,
	// so formatting or absent-vs-explicit default values do not cause traffic.
	return initialRoomGameSettings(previous) != initialRoomGameSettings(next)
}

func notifyRoomTimingSettingsChanged(workspace workspacemodel.Workspace, previous, next string, publish func(uint64, string, string, string, bool)) {
	if workspace.ID == 0 || !roomTimingSettingsChanged(previous, next) {
		return
	}
	// An empty game ID invalidates this room's whole timing catalogue. Even a
	// platform template edit is room-scoped: already-created rooms own snapshots
	// and must not receive a global template-change event.
	publish(workspace.ID, workspace.Scope, workspace.RoomCode, "", workspace.Status == 1)
}
