package services

import (
	"backend/data/models/chat"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func TestWorkspaceGameDefaultsInheritCatalogueWithoutOpeningRooms(t *testing.T) {
	game := GameSummary{ID: "speed-racing", Name: "极速赛车", LobbyCategory: "赛车", Enabled: true}
	for _, workspaceType := range []string{workspacemodel.TypeTenant, workspacemodel.TypeAgent} {
		t.Run(workspaceType, func(t *testing.T) {
			workspace := workspacemodel.Workspace{ID: 37, OwnerUserID: 91, Type: workspaceType, Status: 1}
			view := workspaceGameView(workspace, game, nil)
			if view.Enabled || view.RoomEnabled || !view.PlatformEnabled {
				t.Fatalf("new room must retain platform switch but default closed: %#v", view)
			}
			if view.ID != game.ID || view.Name != game.Name || view.LobbyCategory != game.LobbyCategory {
				t.Fatalf("closed room lost the inherited catalogue/category: %#v", view)
			}
			// Opening one game must not implicitly open a future catalogue entry.
			future := game
			future.ID = "future-racing"
			if view = workspaceGameView(workspace, future, map[string]bool{game.ID: true}); view.Enabled || view.RoomEnabled {
				t.Fatalf("new catalogue entry inherited another game's open switch: %#v", view)
			}
		})
	}
}

func TestWorkspaceGameViewUsesEffectiveAvailabilityAndPreservesRawSwitches(t *testing.T) {
	cases := []struct {
		name            string
		workspaceType   string
		workspaceStatus int
		platformEnabled bool
		category        string
		roomStates      map[string]bool
		wantRoom        bool
		wantEnabled     bool
	}{
		{"room explicitly enabled", workspacemodel.TypeAgent, 1, true, "赛车", map[string]bool{"speed-racing": true}, true, true},
		{"tenant explicitly enabled", workspacemodel.TypeTenant, 1, true, "赛车", map[string]bool{"speed-racing": true}, true, true},
		{"explicit off remains off", workspacemodel.TypeAgent, 1, true, "赛车", map[string]bool{"speed-racing": false}, false, false},
		{"platform closed", workspacemodel.TypeTenant, 1, false, "赛车", map[string]bool{"speed-racing": true}, true, false},
		{"uncategorized", workspacemodel.TypeAgent, 1, true, " ", map[string]bool{"speed-racing": true}, true, false},
		{"workspace disabled", workspacemodel.TypeTenant, 0, true, "赛车", map[string]bool{"speed-racing": true}, true, false},
		{"invalid workspace kind", "unknown", 1, true, "赛车", map[string]bool{"speed-racing": true}, true, false},
		{"platform catalogue unaffected", workspacemodel.TypePlatform, 1, true, "赛车", nil, true, true},
		{"platform explicit off preserved", workspacemodel.TypePlatform, 1, true, "赛车", map[string]bool{"speed-racing": false}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workspace := workspacemodel.Workspace{ID: 37, Type: tc.workspaceType, Status: tc.workspaceStatus}
			game := GameSummary{ID: "speed-racing", Enabled: tc.platformEnabled, LobbyCategory: tc.category}
			view := workspaceGameView(workspace, game, tc.roomStates)
			if view.Enabled != tc.wantEnabled || view.RoomEnabled != tc.wantRoom || view.PlatformEnabled != tc.platformEnabled {
				t.Fatalf("view = %#v, want effective=%v room=%v platform=%v", view, tc.wantEnabled, tc.wantRoom, tc.platformEnabled)
			}
		})
	}
}

func TestWorkspaceGameLookupsRejectMissingIdentity(t *testing.T) {
	for _, input := range []struct {
		workspaceID uint64
		gameID      string
	}{{0, "speed-racing"}, {37, ""}, {37, " \t\n"}} {
		enabled, err := WorkspaceGameEnabled(nil, input.workspaceID, input.gameID)
		if err != nil || enabled {
			t.Fatalf("invalid game identity was allowed: input=%#v enabled=%v err=%v", input, enabled, err)
		}
	}
	if _, err := NewWorkspaceGameService(nil).List(0); apperrors.GetErrorCode(err) != "ROOM_NOT_FOUND" {
		t.Fatalf("unscoped catalogue lookup error = %v", err)
	}
	for _, workspace := range []workspacemodel.Workspace{
		{}, {ID: 37, Type: workspacemodel.TypeAgent}, {ID: 37, OwnerUserID: 91, Type: "unknown"},
	} {
		if err := EnsureWorkspaceGameDefaults(nil, workspace); apperrors.GetErrorCode(err) != "ROOM_NOT_FOUND" {
			t.Fatalf("invalid new-room identity error = %v", err)
		}
	}
}

func TestEnsureWorkspaceGameDefaultsInsertsFalseWithoutOverwritingChoices(t *testing.T) {
	for _, workspaceType := range []string{workspacemodel.TypeTenant, workspacemodel.TypeAgent} {
		t.Run(workspaceType, func(t *testing.T) {
			db := robotDryRunDB(t)
			var statement *gorm.Statement
			if err := db.Callback().Raw().After("gorm:raw").Register("test:room_defaults", func(tx *gorm.DB) {
				statement = tx.Statement
			}); err != nil {
				t.Fatal(err)
			}
			workspace := workspacemodel.Workspace{ID: 37, OwnerUserID: 91, Type: workspaceType, Status: 1}
			if err := EnsureWorkspaceGameDefaults(db, workspace); err != nil {
				t.Fatal(err)
			}
			if statement == nil {
				t.Fatal("room defaults were not materialized")
			}
			for _, fragment := range []string{"INSERT INTO room_game_settings", "agent_id", "FALSE", "FROM lottery_games", "ON CONFLICT DO NOTHING"} {
				if !strings.Contains(statement.SQL.String(), fragment) {
					t.Fatalf("room default insert omitted %q: %s", fragment, statement.SQL.String())
				}
			}
			if len(statement.Vars) != 2 || statement.Vars[0] != workspace.ID || statement.Vars[1] != workspace.OwnerUserID {
				t.Fatalf("room defaults are not scoped to their owner: %#v", statement.Vars)
			}
		})
	}
	if err := EnsureWorkspaceGameDefaults(nil, workspacemodel.Workspace{ID: 1, OwnerUserID: 1, Type: workspacemodel.TypePlatform}); err != nil {
		t.Fatalf("platform catalogue must not receive default-off room rows: %v", err)
	}
}

func TestRoomGameSettingFalseSurvivesGORMCreate(t *testing.T) {
	db := robotDryRunDB(t).Session(&gorm.Session{SkipDefaultTransaction: true})
	setting := chat.RoomGameSetting{WorkspaceID: 37, AgentID: 91, GameID: "speed-racing", Enabled: false}
	statement := db.Create(&setting).Statement
	if statement.Error != nil {
		t.Fatal(statement.Error)
	}
	if setting.Enabled {
		t.Fatal("GORM changed an explicit closed room switch into enabled")
	}
	field := statement.Schema.LookUpField("Enabled")
	if field == nil || field.DefaultValue != "false" {
		t.Fatalf("room game schema is not default-closed: %#v", field)
	}
	if len(statement.Vars) < 4 || statement.Vars[3] != false {
		t.Fatalf("room game insert did not persist false: %#v", statement.Vars)
	}
}
