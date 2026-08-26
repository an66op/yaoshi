package chat

import "time"

// RoomGameSetting controls whether one lottery chat is open inside one agent
// room. Missing rows intentionally mean enabled for backwards compatibility.
type RoomGameSetting struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	AgentID   uint64    `gorm:"not null;uniqueIndex:idx_room_game_setting" json:"agent_id"`
	GameID    string    `gorm:"size:40;not null;uniqueIndex:idx_room_game_setting" json:"game_id"`
	Enabled   bool      `gorm:"not null" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
