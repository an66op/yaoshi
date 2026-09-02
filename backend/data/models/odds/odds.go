package odds

import "time"

// PlayLimit stores per-game odds and stake limits for one play type.
type PlayLimit struct {
	ID             uint64  `gorm:"primaryKey" json:"id"`
	GameID         string  `gorm:"size:40;not null;uniqueIndex:idx_odds_game_play" json:"game_id"`
	PlayCode       string  `gorm:"size:40;not null;uniqueIndex:idx_odds_game_play" json:"play_code"`
	PlayName       string  `gorm:"size:40;not null" json:"play_name"`
	Odds           float64 `gorm:"not null;default:0" json:"odds"`
	MinBet         float64 `gorm:"not null" json:"min_bet"`
	MaxBet         float64 `gorm:"not null" json:"max_bet"`
	MaxUserPeriod  float64 `gorm:"not null" json:"max_user_period"`
	MaxPeriodTotal float64 `gorm:"not null" json:"max_period_total"`
	SortOrder      int     `gorm:"not null;default:0" json:"sort_order"`
	// ExplicitlyConfigured and RuleVersion form the activation boundary for
	// every market. A number in the table is never a live quote until an
	// administrator confirms it against the current rules contract.
	ExplicitlyConfigured bool       `gorm:"not null;default:false" json:"explicitly_configured"`
	RuleVersion          string     `gorm:"size:32;not null;default:'';index" json:"rule_version"`
	ConfigurationSource  string     `gorm:"size:32;not null;default:unconfigured" json:"configuration_source"`
	ConfiguredAt         *time.Time `json:"configured_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

func (PlayLimit) TableName() string { return "lottery_play_limits" }
