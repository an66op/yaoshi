package services

import (
	"backend/data/models/chat"
	"backend/data/models/user"
	"strings"

	"gorm.io/gorm"
)

// WorkspaceGameView combines the platform switch with one room's own switch.
// A room can only narrow the platform catalogue; it can never reopen a game
// disabled by the platform administrator.
type WorkspaceGameView struct {
	GameSummary
	PlatformEnabled bool `json:"platform_enabled"`
	RoomEnabled     bool `json:"room_enabled"`
}

type WorkspaceGameService struct {
	db      *gorm.DB
	lottery *LotteryService
}

func NewWorkspaceGameService(db *gorm.DB) *WorkspaceGameService {
	return &WorkspaceGameService{db: db, lottery: NewLotteryService(db)}
}

func (s *WorkspaceGameService) List(workspaceID uint64) ([]WorkspaceGameView, error) {
	games, err := s.lottery.ListGames()
	if err != nil {
		return nil, err
	}
	settings := make([]chat.RoomGameSetting, 0)
	if workspaceID > 0 {
		if err := s.db.Where("workspace_id = ?", workspaceID).Find(&settings).Error; err != nil {
			return nil, err
		}
	}
	roomStates := make(map[string]bool, len(settings))
	for _, setting := range settings {
		roomStates[setting.GameID] = setting.Enabled
	}
	result := make([]WorkspaceGameView, 0, len(games))
	for _, game := range games {
		roomEnabled, configured := roomStates[game.ID]
		if !configured {
			roomEnabled = true
		}
		platformEnabled := game.Enabled
		game.Enabled = platformEnabled && roomEnabled
		result = append(result, WorkspaceGameView{GameSummary: game, PlatformEnabled: platformEnabled, RoomEnabled: roomEnabled})
	}
	return result, nil
}

func (s *WorkspaceGameService) ListEnabledForMember(userID uint64) ([]GameSummary, error) {
	var account user.User
	if err := s.db.Select("user_id", "workspace_id").First(&account, userID).Error; err != nil {
		return nil, err
	}
	views, err := s.List(account.WorkspaceID)
	if err != nil {
		return nil, err
	}
	games := make([]GameSummary, 0, len(views))
	for _, view := range views {
		if view.Enabled && strings.TrimSpace(view.LobbyCategory) != "" {
			games = append(games, view.GameSummary)
		}
	}
	return s.lottery.EnrichForLobby(games)
}

// WorkspaceGameEnabled intentionally treats a missing row as enabled so the
// migration remains backwards compatible for every existing room.
func WorkspaceGameEnabled(db *gorm.DB, workspaceID uint64, gameID string) (bool, error) {
	if workspaceID == 0 || strings.TrimSpace(gameID) == "" {
		return true, nil
	}
	var setting chat.RoomGameSetting
	result := db.Select("enabled").Where("workspace_id = ? AND game_id = ?", workspaceID, strings.TrimSpace(gameID)).Limit(1).Find(&setting)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return true, nil
	}
	return setting.Enabled, nil
}
