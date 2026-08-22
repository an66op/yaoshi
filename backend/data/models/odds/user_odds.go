package odds

import "time"

// UserPlayOdds stores a per-user odds override for one game play.
// Missing rows mean the user inherits the room PlayLimit odds.
type UserPlayOdds struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	UserID    uint64    `gorm:"not null;uniqueIndex:idx_user_odds_game_play" json:"user_id"`
	GameID    string    `gorm:"size:40;not null;uniqueIndex:idx_user_odds_game_play" json:"game_id"`
	PlayCode  string    `gorm:"size:40;not null;uniqueIndex:idx_user_odds_game_play" json:"play_code"`
	Odds      float64   `gorm:"not null" json:"odds"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (UserPlayOdds) TableName() string { return "user_play_odds" }
