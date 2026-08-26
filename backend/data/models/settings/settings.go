package settings

import "time"

// SystemConfig stores room-level operational settings as a single-row table.
// Extra tabs (game / quick replies / rebate) keep flexible JSON payloads so
// the admin UI can evolve without schema churn.
type SystemConfig struct {
	ID                    uint      `gorm:"primaryKey" json:"id"`
	RoomName              string    `gorm:"size:80;not null;default:王者" json:"room_name"`
	RoomLogo              string    `gorm:"type:text" json:"room_logo"`
	RoomCode              string    `gorm:"size:40;not null;default:1231" json:"room_code"`
	ChatNickname          string    `gorm:"size:80;not null;default:群主" json:"chat_nickname"`
	NicknameDisplayLength int       `gorm:"not null;default:0" json:"nickname_display_length"`
	MinChatScore          float64   `gorm:"not null;default:0" json:"min_chat_score"`
	MinCreditAmount       float64   `gorm:"not null;default:0" json:"min_credit_amount"`
	MinDebitAmount        float64   `gorm:"not null;default:0" json:"min_debit_amount"`
	RequireJoinReview     bool      `gorm:"not null;default:true" json:"require_join_review"`
	SoundEnabled          bool      `gorm:"not null;default:true" json:"sound_enabled"`
	ShowOdds              bool      `gorm:"not null;default:true" json:"show_odds"`
	PredictionEnabled     bool      `gorm:"not null;default:true" json:"prediction_enabled"`
	AbnormalLoginAlert    bool      `gorm:"not null;default:false" json:"abnormal_login_alert"`
	SecurityPasswordCheck bool      `gorm:"not null;default:false" json:"security_password_check"`
	RoomNotice            string    `gorm:"size:2000" json:"room_notice"`
	AnnouncementsJSON     string    `gorm:"type:text" json:"-"`
	GameSettingsJSON      string    `gorm:"type:text" json:"-"`
	QuickRepliesJSON      string    `gorm:"type:text" json:"-"`
	RebateSettingsJSON    string    `gorm:"type:text" json:"-"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

func (SystemConfig) TableName() string { return "system_settings" }
