package odds

import "time"

// PlayLimit stores per-game odds and stake limits for one play type.
type PlayLimit struct {
	ID             uint64    `gorm:"primaryKey" json:"id"`
	GameID         string    `gorm:"size:40;not null;uniqueIndex:idx_odds_game_play" json:"game_id"`
	PlayCode       string    `gorm:"size:40;not null;uniqueIndex:idx_odds_game_play" json:"play_code"`
	PlayName       string    `gorm:"size:40;not null" json:"play_name"`
	Odds           float64   `gorm:"not null;default:1.993" json:"odds"`
	MinBet         float64   `gorm:"not null;default:1" json:"min_bet"`
	MaxBet         float64   `gorm:"not null;default:50000" json:"max_bet"`
	MaxUserPeriod  float64   `gorm:"not null;default:50000" json:"max_user_period"`
	MaxPeriodTotal float64   `gorm:"not null;default:100000" json:"max_period_total"`
	SortOrder      int       `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (PlayLimit) TableName() string { return "lottery_play_limits" }
