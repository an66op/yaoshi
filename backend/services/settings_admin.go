package services

import (
	"backend/data/models/settings"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SettingsAdminService struct{ db *gorm.DB }

var defaultGameSettings = map[string]any{
	"seal_seconds":                      30,
	"allow_cancel":                      true,
	"default_fly_rate":                  0,
	"max_open_games":                    8,
	"room_activity_enabled":             false,
	"room_activity_interval_secs":       60,
	"room_activity_bots_per_room":       defaultWorkspaceRobotCount,
	"room_activity_bets_per_cycle":      1,
	"room_activity_chat_chance_percent": 0,
	"show_member_turnover":              true,
	"show_member_profit":                true,
	"show_member_rebate":                true,
	"web_keyboard_enabled":              true,
	"show_mipai_tool":                   true,
	"show_orders_tool":                  true,
	"show_streak_tool":                  true,
	"show_prediction_tool":              true,
}

// normalizeGameSettings keeps legacy room JSON compatible with settings added
// later. Stored values, including an explicit false, always win over defaults.
func normalizeGameSettings(value string) json.RawMessage {
	merged := make(map[string]any, len(defaultGameSettings))
	for key, item := range defaultGameSettings {
		merged[key] = item
	}
	var stored map[string]any
	if json.Unmarshal([]byte(value), &stored) == nil {
		for key, item := range stored {
			merged[key] = item
		}
	}
	encoded, _ := json.Marshal(merged)
	return json.RawMessage(encoded)
}

type AnnouncementItem struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Content      string `json:"content"`
	Enabled      bool   `json:"enabled"`
	PopupOnLogin bool   `json:"popup_on_login"`
	SortOrder    int    `json:"sort_order"`
}

type SystemSettingsView struct {
	RoomName              string             `json:"room_name"`
	RoomLogo              string             `json:"room_logo"`
	ChatNickname          string             `json:"chat_nickname"`
	ChatAvatar            string             `json:"chat_avatar"`
	NicknameDisplayLength int                `json:"nickname_display_length"`
	MinChatScore          float64            `json:"min_chat_score"`
	MinCreditAmount       float64            `json:"min_credit_amount"`
	MinDebitAmount        float64            `json:"min_debit_amount"`
	RoomEnabled           bool               `json:"room_enabled"`
	RequireJoinReview     bool               `json:"require_join_review"`
	SoundEnabled          bool               `json:"sound_enabled"`
	ShowOdds              bool               `json:"show_odds"`
	PredictionEnabled     bool               `json:"prediction_enabled"`
	AbnormalLoginAlert    bool               `json:"abnormal_login_alert"`
	SecurityPasswordCheck bool               `json:"security_password_check"`
	RoomNotice            string             `json:"room_notice"`
	Announcements         []AnnouncementItem `json:"announcements"`
	Game                  json.RawMessage    `json:"game"`
	QuickReplies          json.RawMessage    `json:"quick_replies"`
	Rebate                json.RawMessage    `json:"rebate"`
}

type UpdateSystemSettingsInput struct {
	RoomName              string             `json:"room_name"`
	RoomLogo              string             `json:"room_logo"`
	ChatNickname          string             `json:"chat_nickname"`
	ChatAvatar            string             `json:"chat_avatar"`
	NicknameDisplayLength int                `json:"nickname_display_length"`
	MinChatScore          float64            `json:"min_chat_score"`
	MinCreditAmount       float64            `json:"min_credit_amount"`
	MinDebitAmount        float64            `json:"min_debit_amount"`
	RoomEnabled           bool               `json:"room_enabled"`
	RequireJoinReview     bool               `json:"require_join_review"`
	SoundEnabled          bool               `json:"sound_enabled"`
	ShowOdds              bool               `json:"show_odds"`
	PredictionEnabled     bool               `json:"prediction_enabled"`
	AbnormalLoginAlert    bool               `json:"abnormal_login_alert"`
	SecurityPasswordCheck bool               `json:"security_password_check"`
	RoomNotice            string             `json:"room_notice"`
	Announcements         []AnnouncementItem `json:"announcements"`
	Game                  json.RawMessage    `json:"game"`
	QuickReplies          json.RawMessage    `json:"quick_replies"`
	Rebate                json.RawMessage    `json:"rebate"`
}

func NewSettingsAdminService(db *gorm.DB) *SettingsAdminService {
	return &SettingsAdminService{db: db}
}

func (s *SettingsAdminService) Get() (*SystemSettingsView, error) {
	return s.GetForWorkspace(0)
}

func (s *SettingsAdminService) GetForWorkspace(workspaceID uint64) (*SystemSettingsView, error) {
	row, err := s.ensure(workspaceID)
	if err != nil {
		return nil, err
	}
	if err := s.applyAuthoritativeWorkspaceIdentity(row); err != nil {
		return nil, err
	}
	return toSettingsView(row), nil
}

// MenuTemplate returns the platform-owned visual menu template for a
// privileged room role. It never carries authorization rules: tenant and
// agent API middleware remains the only source of access control.
func (s *SettingsAdminService) MenuTemplate(role string) (json.RawMessage, error) {
	if role != "tenant" && role != "agent" {
		return nil, apperrors.NewBusinessError("INVALID_ROLE", "菜单角色不正确")
	}
	view, err := s.Get()
	if err != nil {
		return nil, err
	}
	values := map[string]json.RawMessage{}
	if err := json.Unmarshal(view.Game, &values); err != nil {
		return json.RawMessage("[]"), nil
	}
	result := values[role+"_menu"]
	var items []any
	if len(result) == 0 || json.Unmarshal(result, &items) != nil {
		return json.RawMessage("[]"), nil
	}
	return result, nil
}

func (s *SettingsAdminService) Update(input UpdateSystemSettingsInput) (*SystemSettingsView, error) {
	return s.UpdateForWorkspace(0, input)
}

func (s *SettingsAdminService) UpdateRoomProfileForWorkspace(workspaceID uint64, name, logo string) (*SystemSettingsView, error) {
	var workspace workspacemodel.Workspace
	if err := s.db.First(&workspace, workspaceID).Error; err != nil {
		return nil, apperrors.NewBusinessError("WORKSPACE_NOT_FOUND", "房间不存在")
	}
	return s.UpdateRoomIdentityForWorkspace(workspaceID, workspace.RoomCode, name, logo)
}

// UpdateRoomIdentityForWorkspace is the single write path for a room's
// public identity. Workspace owns the value; SystemConfig and the legacy agent
// columns are updated atomically as compatibility shadows.
func (s *SettingsAdminService) UpdateRoomIdentityForWorkspace(workspaceID uint64, code, name, logo string) (*SystemSettingsView, error) {
	row, err := s.ensure(workspaceID)
	if err != nil {
		return nil, err
	}
	roomCode := normalizeAgentRoomCode(code)
	if err := validateAgentRoomCode(roomCode); err != nil {
		return nil, err
	}
	roomName := strings.Join(strings.Fields(name), " ")
	if len([]rune(roomName)) < 2 || len([]rune(roomName)) > 30 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "房间名称长度需为 2–30 个字符")
	}
	roomLogo, err := normalizeRoomLogo(logo)
	if err != nil {
		return nil, err
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := lockPublicRoomCodeRegistry(tx); err != nil {
			return err
		}
		var workspace workspacemodel.Workspace
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&workspace, workspaceID).Error; err != nil {
			return err
		}
		if err := ensureRoomCodeAvailable(tx, roomCode, workspace.OwnerUserID); err != nil {
			return err
		}
		if err := tx.Model(&workspace).Updates(map[string]any{"room_code": roomCode, "name": roomName, "logo": roomLogo}).Error; err != nil {
			return err
		}
		if err := tx.Model(row).Updates(map[string]any{"room_code": roomCode, "room_name": roomName, "room_logo": roomLogo}).Error; err != nil {
			return err
		}
		workspace.RoomCode, workspace.Name, workspace.Logo = roomCode, roomName, roomLogo
		return syncLegacyAgentRoomIdentity(tx, workspace)
	}); err != nil {
		return nil, apperrors.NewSystemError("SETTINGS_SAVE_FAILED", "保存房间资料失败", err)
	}
	row.RoomCode, row.RoomName, row.RoomLogo = roomCode, roomName, roomLogo
	return toSettingsView(row), nil
}

func (s *SettingsAdminService) UpdateForWorkspace(workspaceID uint64, input UpdateSystemSettingsInput) (*SystemSettingsView, error) {
	row, err := s.ensure(workspaceID)
	if err != nil {
		return nil, err
	}
	roomName := strings.Join(strings.Fields(input.RoomName), " ")
	if len([]rune(roomName)) < 2 || len([]rune(roomName)) > 30 {
		return nil, apperrors.NewBusinessError("INVALID_REQUEST", "房间名称长度需为 2–30 个字符")
	}
	roomLogo, logoErr := normalizeRoomLogo(input.RoomLogo)
	if logoErr != nil {
		return nil, logoErr
	}
	row.RoomName = roomName
	row.RoomLogo = roomLogo
	row.ChatNickname = defaultString(strings.TrimSpace(input.ChatNickname), "群主")
	row.ChatAvatar, err = normalizeMemberAvatar(input.ChatAvatar)
	if err != nil {
		return nil, err
	}
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
	row.RoomEnabled = input.RoomEnabled
	row.RequireJoinReview = input.RequireJoinReview
	row.SoundEnabled = input.SoundEnabled
	row.ShowOdds = input.ShowOdds
	row.PredictionEnabled = input.PredictionEnabled
	row.AbnormalLoginAlert = input.AbnormalLoginAlert
	row.SecurityPasswordCheck = input.SecurityPasswordCheck
	if input.Announcements != nil {
		announcements, encoded, normalizeErr := normalizeAnnouncements(input.Announcements)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		row.AnnouncementsJSON = encoded
		row.RoomNotice = firstEnabledAnnouncementContent(announcements)
	} else {
		row.RoomNotice = strings.TrimSpace(input.RoomNotice)
	}
	row.GameSettingsJSON = string(normalizeGameSettings(string(input.Game)))
	row.QuickRepliesJSON = rawOrEmptyArray(input.QuickReplies)
	row.RebateSettingsJSON = rawOrEmptyObject(input.Rebate)
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(row).Error; err != nil {
			return err
		}
		if row.WorkspaceID > 0 {
			var workspace workspacemodel.Workspace
			if err := tx.First(&workspace, row.WorkspaceID).Error; err != nil {
				return err
			}
			if err := tx.Model(&workspace).Updates(map[string]any{"name": row.RoomName, "logo": row.RoomLogo}).Error; err != nil {
				return err
			}
			workspace.Name, workspace.Logo = row.RoomName, row.RoomLogo
			if err := syncLegacyAgentRoomIdentity(tx, workspace); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, apperrors.NewSystemError("SETTINGS_SAVE_FAILED", "保存系统设置失败", err)
	}
	return toSettingsView(row), nil
}

func (s *SettingsAdminService) ensure(workspaceID uint64) (*settings.SystemConfig, error) {
	var row settings.SystemConfig
	query := s.db.Model(&settings.SystemConfig{})
	if workspaceID > 0 {
		query = query.Where("workspace_id = ?", workspaceID)
	} else {
		var platform workspacemodel.Workspace
		if err := s.db.Select("id").Where("type = ?", workspacemodel.TypePlatform).First(&platform).Error; err == nil {
			query = query.Where("workspace_id = ?", platform.ID)
		} else {
			// Startup creates the original settings singleton before workspace
			// migration. Only that bootstrap path may fall back to the first row.
			query = query.Where("workspace_id = 0").Order("id ASC")
		}
	}
	err := query.First(&row).Error
	if err == nil {
		platformRow := workspaceID == 0
		if row.WorkspaceID > 0 {
			var workspace workspacemodel.Workspace
			if loadErr := s.db.Select("type").First(&workspace, row.WorkspaceID).Error; loadErr == nil {
				platformRow = workspace.Type == workspacemodel.TypePlatform
			}
		}
		if platformRow && strings.TrimSpace(row.AnnouncementsJSON) == "" {
			notice := strings.TrimSpace(row.RoomNotice)
			if notice == "" {
				notice = "欢迎来到王者，祝您游戏愉快。"
				row.RoomNotice = notice
			}
			_, encoded, _ := normalizeAnnouncements([]AnnouncementItem{{
				ID: "welcome", Title: "欢迎公告", Content: notice,
				Enabled: true, PopupOnLogin: true, SortOrder: 10,
			}})
			row.AnnouncementsJSON = encoded
			if saveErr := s.db.Model(&row).Updates(map[string]any{
				"announcements_json": row.AnnouncementsJSON,
				"room_notice":        row.RoomNotice,
			}).Error; saveErr != nil {
				return nil, apperrors.NewSystemError("SETTINGS_SAVE_FAILED", "初始化公告设置失败", saveErr)
			}
		}
		return &row, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, apperrors.NewSystemError("SETTINGS_READ_FAILED", "读取系统设置失败", err)
	}
	if workspaceID > 0 {
		var workspace workspacemodel.Workspace
		if err := s.db.First(&workspace, workspaceID).Error; err != nil {
			return nil, apperrors.NewBusinessError("WORKSPACE_NOT_FOUND", "房间不存在")
		}
		row = settings.SystemConfig{
			WorkspaceID: workspace.ID, RoomName: workspace.Name, RoomLogo: workspace.Logo, RoomCode: workspace.RoomCode,
			ChatNickname: "客服", ChatAvatar: workspace.Logo, RoomEnabled: true, RequireJoinReview: true, SoundEnabled: true, ShowOdds: true, PredictionEnabled: true,
			AnnouncementsJSON: "[]", GameSettingsJSON: string(normalizeGameSettings(`{"max_open_games":0}`)),
			QuickRepliesJSON: "[]", RebateSettingsJSON: `{"enabled":false,"rate_percent":0,"min_turnover":0,"settle_mode":"daily","auto_credit":false}`,
		}
		if err := s.db.Create(&row).Error; err != nil {
			return nil, apperrors.NewSystemError("SETTINGS_INIT_FAILED", "初始化房间设置失败", err)
		}
		return &row, nil
	}
	row = settings.SystemConfig{
		ID:                 1,
		RoomName:           "王者",
		RoomCode:           "",
		ChatNickname:       "群主",
		ChatAvatar:         "",
		RoomEnabled:        true,
		RequireJoinReview:  true,
		SoundEnabled:       true,
		ShowOdds:           true,
		PredictionEnabled:  true,
		RoomNotice:         "欢迎来到王者，祝您游戏愉快。",
		AnnouncementsJSON:  `[{"id":"welcome","title":"欢迎公告","content":"欢迎来到王者，祝您游戏愉快。","enabled":true,"popup_on_login":true,"sort_order":10}]`,
		GameSettingsJSON:   string(normalizeGameSettings(`{"max_open_games":8}`)),
		QuickRepliesJSON:   `[{"title":"欢迎光临","content":"欢迎进入王者房间，祝您游戏愉快。"},{"title":"封盘提醒","content":"本期即将封盘，请尽快完成下注。"},{"title":"开奖公告","content":"本期已开奖，请留意中奖结果。"}]`,
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
	announcements := decodeAnnouncements(row.AnnouncementsJSON, row.RoomNotice)
	return &SystemSettingsView{
		RoomName:              defaultString(strings.TrimSpace(row.RoomName), "王者大厅"),
		RoomLogo:              row.RoomLogo,
		ChatNickname:          row.ChatNickname,
		ChatAvatar:            row.ChatAvatar,
		NicknameDisplayLength: row.NicknameDisplayLength,
		MinChatScore:          row.MinChatScore,
		MinCreditAmount:       row.MinCreditAmount,
		MinDebitAmount:        row.MinDebitAmount,
		RoomEnabled:           row.RoomEnabled,
		RequireJoinReview:     row.RequireJoinReview,
		SoundEnabled:          row.SoundEnabled,
		ShowOdds:              row.ShowOdds,
		PredictionEnabled:     row.PredictionEnabled,
		AbnormalLoginAlert:    row.AbnormalLoginAlert,
		SecurityPasswordCheck: row.SecurityPasswordCheck,
		RoomNotice:            row.RoomNotice,
		Announcements:         announcements,
		Game:                  normalizeGameSettings(row.GameSettingsJSON),
		QuickReplies:          json.RawMessage(defaultJSON(row.QuickRepliesJSON, `[]`)),
		Rebate:                json.RawMessage(defaultJSON(row.RebateSettingsJSON, `{"enabled":true,"rate_percent":0.5,"min_turnover":0,"settle_mode":"daily","auto_credit":false}`)),
	}
}

// applyAuthoritativeWorkspaceIdentity keeps the compatibility columns in the
// same direction as the ownership model: Workspace is authoritative,
// SystemConfig and the legacy agent columns are read-through shadows. This is
// deliberately idempotent so old installations self-heal on the first room
// settings read without allowing a stale `agent_room_name` to overwrite the
// configured workspace again.
func (s *SettingsAdminService) applyAuthoritativeWorkspaceIdentity(row *settings.SystemConfig) error {
	if row == nil || row.WorkspaceID == 0 {
		return nil
	}
	var workspace workspacemodel.Workspace
	if err := s.db.First(&workspace, row.WorkspaceID).Error; err != nil {
		return apperrors.NewBusinessError("WORKSPACE_NOT_FOUND", "房间不存在")
	}
	settingsChanged := row.RoomCode != workspace.RoomCode || row.RoomName != workspace.Name || row.RoomLogo != workspace.Logo
	legacyChanged := false
	if workspace.Type == workspacemodel.TypeAgent && workspace.OwnerUserID > 0 {
		var owner user.User
		if err := s.db.Select("user_id", "agent_room_code", "agent_room_name", "agent_room_logo").First(&owner, workspace.OwnerUserID).Error; err != nil {
			return err
		}
		legacyChanged = owner.AgentRoomCode != workspace.RoomCode || owner.AgentRoomName != workspace.Name || owner.AgentRoomLogo != workspace.Logo
	}
	if settingsChanged || legacyChanged {
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			if settingsChanged {
				if err := tx.Model(&settings.SystemConfig{}).Where("id = ? AND workspace_id = ?", row.ID, workspace.ID).Updates(map[string]any{
					"room_code": workspace.RoomCode, "room_name": workspace.Name, "room_logo": workspace.Logo,
				}).Error; err != nil {
					return err
				}
			}
			return syncLegacyAgentRoomIdentity(tx, workspace)
		}); err != nil {
			return apperrors.NewSystemError("SETTINGS_SYNC_FAILED", "同步房间资料失败", err)
		}
	}
	row.RoomCode, row.RoomName, row.RoomLogo = workspace.RoomCode, workspace.Name, workspace.Logo
	return nil
}

func syncLegacyAgentRoomIdentity(tx *gorm.DB, workspace workspacemodel.Workspace) error {
	if workspace.Type != workspacemodel.TypeAgent || workspace.OwnerUserID == 0 {
		return nil
	}
	return tx.Model(&user.User{}).Where("user_id = ? AND role = ?", workspace.OwnerUserID, "agent").Updates(map[string]any{
		"agent_room_code": workspace.RoomCode,
		"agent_room_name": workspace.Name,
		"agent_room_logo": workspace.Logo,
	}).Error
}

func decodeAnnouncements(value, fallback string) []AnnouncementItem {
	var items []AnnouncementItem
	if json.Unmarshal([]byte(value), &items) == nil {
		normalized, _, err := normalizeAnnouncements(items)
		if err == nil {
			return normalized
		}
	}
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return []AnnouncementItem{}
	}
	return []AnnouncementItem{{ID: "welcome", Title: "欢迎公告", Content: fallback, Enabled: true, PopupOnLogin: true, SortOrder: 10}}
}

func normalizeAnnouncements(input []AnnouncementItem) ([]AnnouncementItem, string, error) {
	if len(input) > 50 {
		return nil, "", apperrors.NewBusinessError("INVALID_REQUEST", "公告最多保留 50 条")
	}
	items := make([]AnnouncementItem, 0, len(input))
	usedIDs := map[string]struct{}{}
	for index, item := range input {
		item.Title = strings.TrimSpace(item.Title)
		item.Content = strings.TrimSpace(item.Content)
		if item.Title == "" || item.Content == "" {
			return nil, "", apperrors.NewBusinessError("INVALID_REQUEST", "公告标题和内容不能为空")
		}
		if len([]rune(item.Title)) > 80 || len([]rune(item.Content)) > 2000 {
			return nil, "", apperrors.NewBusinessError("INVALID_REQUEST", "公告标题或内容过长")
		}
		item.ID = strings.TrimSpace(item.ID)
		if item.ID == "" {
			item.ID = fmt.Sprintf("announcement-%d", index+1)
		}
		baseID := item.ID
		for suffix := 2; ; suffix++ {
			if _, exists := usedIDs[item.ID]; !exists {
				break
			}
			item.ID = fmt.Sprintf("%s-%d", baseID, suffix)
		}
		usedIDs[item.ID] = struct{}{}
		items = append(items, item)
	}
	sort.SliceStable(items, func(left, right int) bool {
		if items[left].SortOrder == items[right].SortOrder {
			return false
		}
		return items[left].SortOrder < items[right].SortOrder
	})
	encoded, err := json.Marshal(items)
	if err != nil {
		return nil, "", apperrors.NewSystemError("SETTINGS_ENCODE_FAILED", "编码公告设置失败", err)
	}
	return items, string(encoded), nil
}

func firstEnabledAnnouncementContent(items []AnnouncementItem) string {
	for _, item := range items {
		if item.Enabled {
			return item.Content
		}
	}
	return ""
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
