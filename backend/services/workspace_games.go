package services

import (
	"backend/data/models/chat"
	"backend/data/models/lottery"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
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
	if workspaceID == 0 {
		return nil, apperrors.NewBusinessError("ROOM_NOT_FOUND", "房间不存在")
	}
	var workspace workspacemodel.Workspace
	if err := s.db.First(&workspace, workspaceID).Error; err != nil {
		return nil, err
	}
	if !validGameWorkspaceType(workspace.Type) {
		return nil, apperrors.NewBusinessError("ROOM_NOT_FOUND", "房间不存在")
	}
	games, err := s.lottery.listGamesForWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}
	settings := make([]chat.RoomGameSetting, 0)
	if err := s.db.Where("workspace_id = ?", workspaceID).Find(&settings).Error; err != nil {
		return nil, err
	}
	roomStates := make(map[string]bool, len(settings))
	for _, setting := range settings {
		roomStates[setting.GameID] = setting.Enabled
	}
	result := make([]WorkspaceGameView, 0, len(games))
	for _, game := range games {
		result = append(result, workspaceGameView(workspace, game, roomStates))
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
		if view.Enabled {
			games = append(games, view.GameSummary)
		}
	}
	return s.lottery.EnrichForLobby(games)
}

func validGameWorkspaceType(workspaceType string) bool {
	return workspaceType == workspacemodel.TypePlatform || workspaceType == workspacemodel.TypeTenant || workspaceType == workspacemodel.TypeAgent
}

func workspaceGameView(workspace workspacemodel.Workspace, game GameSummary, roomStates map[string]bool) WorkspaceGameView {
	roomEnabled, configured := roomStates[game.ID]
	if !configured {
		// The platform owns the catalogue. Rooms inherit its structure but must
		// explicitly opt in to every game; an absent switch is never permission.
		roomEnabled = workspace.Type == workspacemodel.TypePlatform
	}
	platformEnabled := game.Enabled
	game.Enabled = workspace.ID > 0 && workspace.Status == 1 && validGameWorkspaceType(workspace.Type) &&
		platformEnabled && roomEnabled && strings.TrimSpace(game.LobbyCategory) != ""
	return WorkspaceGameView{GameSummary: game, PlatformEnabled: platformEnabled, RoomEnabled: roomEnabled}
}

// EnsureWorkspaceGameDefaults materializes a new room's closed switches without
// ever overwriting an owner's choices. Catalogue/category membership stays
// platform-owned, so games added later also appear in a room, closed by default.
func EnsureWorkspaceGameDefaults(db *gorm.DB, workspace workspacemodel.Workspace) error {
	if workspace.ID == 0 || workspace.OwnerUserID == 0 || !validGameWorkspaceType(workspace.Type) {
		return apperrors.NewBusinessError("ROOM_NOT_FOUND", "房间不存在")
	}
	if workspace.Type == workspacemodel.TypePlatform {
		return nil
	}
	return db.Exec(`INSERT INTO room_game_settings (workspace_id, agent_id, game_id, enabled, created_at, updated_at)
		SELECT ?, ?, id, FALSE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		FROM lottery_games
		ON CONFLICT DO NOTHING`, workspace.ID, workspace.OwnerUserID).Error
}

// WorkspaceGameEnabled is the authoritative availability check used by betting
// and member chat. The upgrade migration materializes historic defaults once;
// missing settings thereafter mean closed, including for newly added games.
func WorkspaceGameEnabled(db *gorm.DB, workspaceID uint64, gameID string) (bool, error) {
	if workspaceID == 0 || strings.TrimSpace(gameID) == "" {
		return false, nil
	}
	var count int64
	err := workspaceEnabledGamesQuery(db, workspaceID).Where("lottery_games.id = ?", strings.TrimSpace(gameID)).Count(&count).Error
	return count > 0, err
}

// Keep background robot execution on the same fail-closed rule as the member
// catalogue, direct bets, assistant bets and lottery-room chat.
func workspaceEnabledGamesQuery(db *gorm.DB, workspaceID uint64) *gorm.DB {
	query := db.Model(&lottery.Game{}).
		Where("lottery_games.enabled = ?", true).
		Where("BTRIM(lottery_games.lobby_category) <> ''")
	if workspaceID == 0 {
		return query.Where("FALSE")
	}
	return query.Where(`EXISTS (
		SELECT 1 FROM workspaces AS game_workspace
		WHERE game_workspace.id = ? AND game_workspace.status = ?
		  AND (
		    (game_workspace.type = ? AND NOT EXISTS (
		      SELECT 1 FROM room_game_settings AS room_game
		      WHERE room_game.workspace_id = game_workspace.id
		        AND room_game.game_id = lottery_games.id AND room_game.enabled = ?
		    ))
		    OR (game_workspace.type IN ? AND EXISTS (
		      SELECT 1 FROM room_game_settings AS room_game
		      WHERE room_game.workspace_id = game_workspace.id
		        AND room_game.game_id = lottery_games.id AND room_game.enabled = ?
		    ))
		  )
	)`, workspaceID, 1, workspacemodel.TypePlatform, false, []string{workspacemodel.TypeTenant, workspacemodel.TypeAgent}, true)
}
