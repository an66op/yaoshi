package services

import (
	"backend/data/models/lottery"
	"backend/data/models/odds"
	"reflect"
	"strings"
	"testing"
	"time"
)

func productionOddsTestRows(gameID string) []odds.PlayLimit {
	profile, ready := rulesForGame(&lottery.Game{ID: gameID})
	if !ready {
		return nil
	}
	configuredAt := time.Date(2026, 9, 5, 1, 2, 3, 0, time.UTC)
	catalog := PlayCatalogForGame(gameID)
	rows := make([]odds.PlayLimit, 0, len(catalog))
	for index, item := range catalog {
		rows = append(rows, odds.PlayLimit{
			GameID: gameID, PlayCode: item.PlayCode, PlayName: item.PlayName, Odds: 2.25,
			MinBet: 1, MaxBet: 100, MaxUserPeriod: 1000, MaxPeriodTotal: 5000, SortOrder: index,
			ExplicitlyConfigured: true, RuleVersion: profile.Version,
			ConfigurationSource: oddsSourceAdminSave, ConfiguredAt: &configuredAt,
		})
	}
	return rows
}

func TestProductionOddsReadinessRequiresCompleteCurrentExplicitCatalogues(t *testing.T) {
	games := []lottery.Game{
		{ID: "speed-racing", Name: "极速赛车"},
		{ID: "hong-kong-mark-six", Name: "香港六合彩"},
	}
	rows := append(productionOddsTestRows(games[0].ID), productionOddsTestRows(games[1].ID)...)
	report := assessProductionOddsReadiness(games, rows)
	wantQuotes := len(PlayCatalogForGame(games[0].ID)) + len(PlayCatalogForGame(games[1].ID))
	if !report.Complete || report.AuditedGames != 2 || report.RequiredQuotes != wantQuotes ||
		report.ValidQuotes != wantQuotes || len(report.IncompleteGames) != 0 {
		t.Fatalf("complete current catalogues were rejected: %+v", report)
	}
}

func TestProductionOddsReadinessRejectsMissingDefaultLegacyAndDriftedRows(t *testing.T) {
	game := lottery.Game{ID: "speed-racing", Name: "极速赛车"}
	baseline := productionOddsTestRows(game.ID)
	if len(baseline) < 2 {
		t.Fatalf("test catalogue unexpectedly short: %d", len(baseline))
	}
	for _, test := range []struct {
		name string
		edit func([]odds.PlayLimit) []odds.PlayLimit
	}{
		{name: "missing", edit: func(rows []odds.PlayLimit) []odds.PlayLimit { return rows[1:] }},
		{name: "legacy numeric default", edit: func(rows []odds.PlayLimit) []odds.PlayLimit {
			rows[0].ExplicitlyConfigured, rows[0].ConfigurationSource, rows[0].RuleVersion = false, oddsSourceUnconfigured, ""
			return rows
		}},
		{name: "wrong rule version", edit: func(rows []odds.PlayLimit) []odds.PlayLimit {
			rows[0].RuleVersion = "racing-v1"
			return rows
		}},
		{name: "wrong configuration source", edit: func(rows []odds.PlayLimit) []odds.PlayLimit {
			rows[0].ConfigurationSource = "bootstrap_default"
			return rows
		}},
		{name: "invalid odds", edit: func(rows []odds.PlayLimit) []odds.PlayLimit {
			rows[0].Odds = 1
			return rows
		}},
		{name: "missing confirmation time", edit: func(rows []odds.PlayLimit) []odds.PlayLimit {
			rows[0].ConfiguredAt = nil
			return rows
		}},
		{name: "duplicate current code", edit: func(rows []odds.PlayLimit) []odds.PlayLimit {
			return append(rows, rows[0])
		}},
		{name: "whitespace play code", edit: func(rows []odds.PlayLimit) []odds.PlayLimit {
			rows[0].PlayCode = " " + rows[0].PlayCode + " "
			return rows
		}},
		{name: "whitespace game id", edit: func(rows []odds.PlayLimit) []odds.PlayLimit {
			rows[0].GameID = " " + rows[0].GameID
			return rows
		}},
		{name: "zero minimum", edit: func(rows []odds.PlayLimit) []odds.PlayLimit {
			rows[0].MinBet = 0
			return rows
		}},
		{name: "fractional-cent limit", edit: func(rows []odds.PlayLimit) []odds.PlayLimit {
			rows[0].MinBet = 1.001
			return rows
		}},
		{name: "inverted per-bet limit", edit: func(rows []odds.PlayLimit) []odds.PlayLimit {
			rows[0].MinBet, rows[0].MaxBet = 101, 100
			return rows
		}},
		{name: "per-bet exceeds member period", edit: func(rows []odds.PlayLimit) []odds.PlayLimit {
			rows[0].MaxBet, rows[0].MaxUserPeriod = 100, 99
			return rows
		}},
		{name: "member exceeds room period", edit: func(rows []odds.PlayLimit) []odds.PlayLimit {
			rows[0].MaxUserPeriod, rows[0].MaxPeriodTotal = 5001, 5000
			return rows
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := append([]odds.PlayLimit(nil), baseline...)
			rows = test.edit(rows)
			report := assessProductionOddsReadiness([]lottery.Game{game}, rows)
			if report.Complete || len(report.IncompleteGames) != 1 || report.ValidQuotes != len(baseline)-1 {
				t.Fatalf("unsafe quote set was accepted: %+v", report)
			}
			problem := report.IncompleteGames[0]
			if problem.Reason != "quotes_incomplete" || len(problem.InvalidPlayCodes) != 1 || problem.InvalidPlayCodes[0] != baseline[0].PlayCode {
				t.Fatalf("wrong incomplete evidence: %+v", problem)
			}
		})
	}
}

func TestProductionOddsReadinessRejectsRoomVisibleGameWithoutRules(t *testing.T) {
	report := assessProductionOddsReadiness([]lottery.Game{{ID: "future-game", Name: "未来彩种"}}, nil)
	if report.Complete || report.AuditedGames != 1 || len(report.IncompleteGames) != 1 ||
		report.IncompleteGames[0].Reason != "rules_not_ready" {
		t.Fatalf("unknown rules did not fail closed: %+v", report)
	}
}

func TestProductionOddsTargetQueryMatchesMemberReachableRoomSemantics(t *testing.T) {
	db := robotDryRunDB(t)
	var games []lottery.Game
	statement := productionOddsTargetGamesQuery(db).Find(&games).Statement
	query := statement.SQL.String()
	for _, fragment := range []string{
		"lottery_games.enabled =", "BTRIM(lottery_games.lobby_category) <> ''", "EXISTS (",
		"workspaces AS odds_workspace", "odds_workspace.status =", "odds_workspace.type =",
		`"user" AS odds_member`, "odds_member.workspace_id = odds_workspace.id",
		"odds_member.role =", "odds_member.status =", "odds_member.deleted_at IS NULL",
		"NOT EXISTS (", "room_game_settings AS odds_room_game", "odds_room_game.enabled =",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("production odds target query omitted %q: %s", fragment, query)
		}
	}
	want := []any{true, 1, "platform", "member", 1, false, "tenant", "agent", true}
	if !reflect.DeepEqual(statement.Vars, want) {
		t.Fatalf("query vars=%+v want=%+v", statement.Vars, want)
	}
}
