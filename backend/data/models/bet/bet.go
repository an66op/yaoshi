package bet

import "time"

// Bet is a single stake placed on a game issue. Amounts use integer cents.
// Position 1-5 = ball slots, 6 = sum column used by the live monitor matrix.
type Bet struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	GameID      string    `gorm:"size:40;not null;index;uniqueIndex:idx_bet_dedupe" json:"game_id"`
	Issue       string    `gorm:"size:64;not null;index;uniqueIndex:idx_bet_dedupe" json:"issue"`
	UserID      uint64    `gorm:"not null;index;uniqueIndex:idx_bet_dedupe" json:"user_id"`
	Username    string    `gorm:"size:50;not null;index" json:"username"`
	PlayCode    string    `gorm:"size:40;not null;uniqueIndex:idx_bet_dedupe" json:"play_code"`
	PlayName    string    `gorm:"size:40;not null" json:"play_name"`
	Position    int       `gorm:"not null;default:0;uniqueIndex:idx_bet_dedupe" json:"position"`
	Selection   string    `gorm:"size:40;not null;uniqueIndex:idx_bet_dedupe" json:"selection"`
	AmountCents int64     `gorm:"not null" json:"-"`
	Odds        float64   `gorm:"not null;default:1.993" json:"odds"`
	Status      string    `gorm:"size:20;not null;default:pending;index" json:"status"`
	PayoutCents int64     `gorm:"not null;default:0" json:"-"`
	FlyCents    int64     `gorm:"not null;default:0" json:"-"`
	Remark      string    `gorm:"size:300" json:"remark"`
	Operator    string    `gorm:"size:80" json:"operator"`
	CreatedAt   time.Time `gorm:"index" json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Bet) TableName() string { return "lottery_bets" }
