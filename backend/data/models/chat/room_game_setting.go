package chat

import "time"

// RoomGameSetting controls whether one lottery is available inside one room.
// The same switch covers lobby visibility, betting and its lottery chat.
// New rooms and missing rows are closed; historic defaults are materialized by
// a one-time migration. An explicit false must never become GORM's true default.
type RoomGameSetting struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	WorkspaceID uint64    `gorm:"not null;default:0;index;uniqueIndex:idx_workspace_game_setting,priority:1" json:"workspace_id"`
	AgentID     uint64    `gorm:"not null;uniqueIndex:idx_room_game_setting" json:"agent_id"`
	GameID      string    `gorm:"size:40;not null;uniqueIndex:idx_room_game_setting;uniqueIndex:idx_workspace_game_setting,priority:2" json:"game_id"`
	Enabled     bool      `gorm:"not null;default:false" json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
