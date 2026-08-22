package services

import (
	"backend/data/models/settings"
	apperrors "backend/errors"
	"encoding/json"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SettingsAdminService struct{ db *gorm.DB }

type SystemSettingsView struct {
	RoomName              string          `json:"room_name"`
	RoomCode              string          `json:"room_code"`
	ChatNickname          string          `json:"chat_nickname"`
	NicknameDisplayLength int             `json:"nickname_display_length"`
	MinChatScore          float64         `json:"min_chat_score"`
	MinCreditAmount       float64         `json:"min_credit_amount"`
	MinDebitAmount        float64         `json:"min_debit_amount"`
	RequireJoinReview     bool            `json:"require_join_review"`
	SoundEnabled          bool            `json:"sound_enabled"`
	ShowOdds              bool            `json:"show_odds"`
	PredictionEnabled     bool            `json:"prediction_enabled"`
	AbnormalLoginAlert    bool            `json:"abnormal_login_alert"`
	SecurityPasswordCheck bool            `json:"security_password_check"`
	RoomNotice            string          `json:"room_notice"`
	Game                  json.RawMessage `json:"game"`
	QuickReplies          json.RawMessage `json:"quick_replies"`
	Rebate                json.RawMessage `json:"rebate"`
}

type UpdateSystemSettingsInput struct {
	RoomName              string          `json:"room_name"`
	RoomCode              string          `json:"room_code"`
	ChatNickname          string          `json:"chat_nickname"`
	NicknameDisplayLength int             `json:"nickname_display_length"`
	MinChatScore          float64         `json:"min_chat_score"`
	MinCreditAmount       float64         `json:"min_credit_amount"`
	MinDebitAmount        float64         `json:"min_debit_amount"`
	RequireJoinReview     bool            `json:"require_join_review"`
	SoundEnabled          bool            `json:"sound_enabled"`
	ShowOdds              bool            `json:"show_odds"`
	PredictionEnabled     bool            `json:"prediction_enabled"`
	AbnormalLoginAlert    bool            `json:"abnormal_login_alert"`
	SecurityPasswordCheck bool            `json:"security_password_check"`
	RoomNotice            string          `json:"room_notice"`
	Game                  json.RawMessage `json:"game"`
	QuickReplies          json.RawMessage `json:"quick_replies"`
	Rebate                json.RawMessage `json:"rebate"`
}

func NewSettingsAdminService(db *gorm.DB) *SettingsAdminService {
	return &SettingsAdminService{db: db}
}

func (s *SettingsAdminService) Get() (*SystemSettingsView, error) {
	row, err := s.ensure()
	if err != nil {
		return nil, err
	}
	return toSettingsView(row), nil
}

func (s *SettingsAdminService) Update(input UpdateSystemSettingsInput) (*SystemSettingsView, error) {
	row, err := s.ensure()
	if err != nil {
		return nil, err
	}
	row.RoomName = defaultString(strings.TrimSpace(input.RoomName), "曜图")
	row.RoomCode = defaultString(strings.TrimSpace(input.RoomCode), "1231")
	row.ChatNickname = defaultString(strings.TrimSpace(input.ChatNickname), "群主")
	if input.NicknameDisplayLength < 0 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "昵称显示长度不能为负数")
	}
	row.NicknameDisplayLength = input.NicknameDisplayLength
	if input.MinChatScore < 0 || input.MinCreditAmount < 0 || input.MinDebitAmount < 0 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "金额门槛不能为负数")
	}
	row.MinChatScore = input.MinChatScore
	row.MinCreditAmount = input.MinCreditAmount
	row.MinDebitAmount = input.MinDebitAmount
	row.RequireJoinReview = input.RequireJoinReview
	row.SoundEnabled = input.SoundEnabled
	row.ShowOdds = input.ShowOdds
	row.PredictionEnabled = input.PredictionEnabled
	row.AbnormalLoginAlert = input.AbnormalLoginAlert
	row.SecurityPasswordCheck = input.SecurityPasswordCheck
	row.RoomNotice = strings.TrimSpace(input.RoomNotice)
	row.GameSettingsJSON = rawOrEmptyObject(input.Game)
	row.QuickRepliesJSON = rawOrEmptyArray(input.QuickReplies)
	row.RebateSettingsJSON = rawOrEmptyObject(input.Rebate)
	if err := s.db.Save(row).Error; err != nil {
		return nil, apperrors.NewSystemError("SETTINGS_SAVE_FAILED", "保存系统设置失败", err)
	}
	return toSettingsView(row), nil
}

func (s *SettingsAdminService) ensure() (*settings.SystemConfig, error) {
	var row settings.SystemConfig
	err := s.db.First(&row, 1).Error
	if err == nil {
		return &row, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, apperrors.NewSystemError("SETTINGS_READ_FAILED", "读取系统设置失败", err)
	}
	row = settings.SystemConfig{
		ID:                1,
		RoomName:          "曜图",
		RoomCode:          "1231",
		ChatNickname:      "群主",
		RequireJoinReview: true,
		SoundEnabled:      true,
		ShowOdds:          true,
		PredictionEnabled: true,
		GameSettingsJSON:  `{"seal_seconds":30,"allow_cancel":true,"default_fly_rate":0,"max_open_games":8}`,
		QuickRepliesJSON:  `[{"title":"欢迎光临","content":"欢迎进入曜图房间，祝您游戏愉快。"},{"title":"封盘提醒","content":"本期即将封盘，请尽快完成下注。"},{"title":"开奖公告","content":"本期已开奖，请留意中奖结果。"}]`,
		RebateSettingsJSON: `{"enabled":true,"rate_percent":0.5,"min_turnover":0,"settle_mode":"daily","auto_credit":false}`,
	}
	if err := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return nil, apperrors.NewSystemError("SETTINGS_INIT_FAILED", "初始化系统设置失败", err)
	}
	if err := s.db.First(&row, 1).Error; err != nil {
		return nil, apperrors.NewSystemError("SETTINGS_READ_FAILED", "读取系统设置失败", err)
	}
	return &row, nil
}

func toSettingsView(row *settings.SystemConfig) *SystemSettingsView {
	return &SystemSettingsView{
		RoomName:              row.RoomName,
		RoomCode:              defaultString(row.RoomCode, "1231"),
		ChatNickname:          row.ChatNickname,
		NicknameDisplayLength: row.NicknameDisplayLength,
		MinChatScore:          row.MinChatScore,
		MinCreditAmount:       row.MinCreditAmount,
		MinDebitAmount:        row.MinDebitAmount,
		RequireJoinReview:     row.RequireJoinReview,
		SoundEnabled:          row.SoundEnabled,
		ShowOdds:              row.ShowOdds,
		PredictionEnabled:     row.PredictionEnabled,
		AbnormalLoginAlert:    row.AbnormalLoginAlert,
		SecurityPasswordCheck: row.SecurityPasswordCheck,
		RoomNotice:            row.RoomNotice,
		Game:                  json.RawMessage(defaultJSON(row.GameSettingsJSON, `{"seal_seconds":30,"allow_cancel":true,"default_fly_rate":0,"max_open_games":8}`)),
		QuickReplies:          json.RawMessage(defaultJSON(row.QuickRepliesJSON, `[]`)),
		Rebate:                json.RawMessage(defaultJSON(row.RebateSettingsJSON, `{"enabled":true,"rate_percent":0.5,"min_turnover":0,"settle_mode":"daily","auto_credit":false}`)),
	}
}

func rawOrEmptyObject(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	if !json.Valid(raw) {
		return "{}"
	}
	return string(raw)
}

func rawOrEmptyArray(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "[]"
	}
	if !json.Valid(raw) {
		return "[]"
	}
	return string(raw)
}

func defaultJSON(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	if !json.Valid([]byte(value)) {
		return fallback
	}
	return value
}
