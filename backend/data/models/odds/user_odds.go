package odds

import "time"

// UserPlayOdds stores a per-user odds override for one game play.
// Missing rows mean the user inherits the room PlayLimit odds.
type UserPlayOdds struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	WorkspaceID uint64    `gorm:"not null;default:0;index;uniqueIndex:idx_user_odds_game_play,priority:1" json:"workspace_id"`
	UserID      uint64    `gorm:"not null;uniqueIndex:idx_user_odds_game_play,priority:2" json:"user_id"`
	GameID      string    `gorm:"size:40;not null;uniqueIndex:idx_user_odds_game_play,priority:3" json:"game_id"`
	PlayCode    string    `gorm:"size:40;not null;uniqueIndex:idx_user_odds_game_play,priority:4" json:"play_code"`
	Odds        float64   `gorm:"not null" json:"odds"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (UserPlayOdds) TableName() string { return "user_play_odds" }

// RoomPlayOdds stores the odds inherited by every member of one workspace.
// A missing row means the room inherits the platform PlayLimit odds.
type RoomPlayOdds struct {
	ID          uint64 `gorm:"primaryKey" json:"id"`
	WorkspaceID uint64 `gorm:"not null;default:0;index;uniqueIndex:idx_room_odds_game_play,priority:1" json:"workspace_id"`
	// AgentID is retained as an owner snapshot for legacy reports. Tenant-owned
	// direct rooms store their tenant owner ID here as well; authorization and
	// runtime resolution always use WorkspaceID.
	AgentID   uint64    `gorm:"not null;index" json:"agent_id"`
	GameID    string    `gorm:"size:40;not null;uniqueIndex:idx_room_odds_game_play,priority:2" json:"game_id"`
	PlayCode  string    `gorm:"size:40;not null;uniqueIndex:idx_room_odds_game_play,priority:3" json:"play_code"`
	Odds      float64   `gorm:"not null" json:"odds"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (RoomPlayOdds) TableName() string { return "room_play_odds" }
