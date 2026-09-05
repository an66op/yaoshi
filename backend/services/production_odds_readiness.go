package services

import (
	"backend/data/models/lottery"
	"backend/data/models/odds"
	workspacemodel "backend/data/models/workspace"
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"

	"gorm.io/gorm"
)

// ProductionOddsGameReadiness identifies one room-visible game whose complete
// current rules catalogue is not backed by explicit administrator prices.
// InvalidPlayCodes contains persisted-current catalogue codes only; no default
// or reference price is ever synthesized by this audit.
type ProductionOddsGameReadiness struct {
	GameID           string   `json:"game_id"`
	GameName         string   `json:"game_name"`
	RuleVersion      string   `json:"rule_version"`
	RequiredQuotes   int      `json:"required_quotes"`
	ValidQuotes      int      `json:"valid_quotes"`
	InvalidPlayCodes []string `json:"invalid_play_codes,omitempty"`
	Reason           string   `json:"reason,omitempty"`
}

// ProductionOddsReadinessReport is the read-only production activation gate.
// AuditedGames contains only games enabled by the platform and reachable from
// at least one active member room. Tenant/agent rooms opt in explicitly. The
// platform workspace is included only when it actually contains an active
// member; the bootstrap administrator alone must not make an otherwise empty
// first installation require prices before the operator can enter the admin
// UI and configure them.
type ProductionOddsReadinessReport struct {
	Complete        bool                          `json:"complete"`
	AuditedGames    int                           `json:"audited_games"`
	RequiredQuotes  int                           `json:"required_quotes"`
	ValidQuotes     int                           `json:"valid_quotes"`
	IncompleteGames []ProductionOddsGameReadiness `json:"incomplete_games"`
}

// productionOddsTargetGamesQuery deliberately mirrors the room/game switch
// portion of workspaceEnabledGamesQuery, but asks whether any active workspace
// can expose the game to members. Platform workspaces inherit an absent switch
// and preserve an explicit false, but count only after an active member is
// assigned there. Tenant and agent rooms must explicitly opt in, so their
// operator-owned open switch is sufficient even before the first member joins.
func productionOddsTargetGamesQuery(db *gorm.DB) *gorm.DB {
	return db.Model(&lottery.Game{}).
		Where("lottery_games.enabled = ?", true).
		Where("BTRIM(lottery_games.lobby_category) <> ''").
		Where(`EXISTS (
			SELECT 1 FROM workspaces AS odds_workspace
			WHERE odds_workspace.status = ?
			  AND (
			    (odds_workspace.type = ? AND EXISTS (
			      SELECT 1 FROM "user" AS odds_member
			      WHERE odds_member.workspace_id = odds_workspace.id
			        AND odds_member.role = ? AND odds_member.status = ?
			        AND odds_member.deleted_at IS NULL
			    ) AND NOT EXISTS (
			      SELECT 1 FROM room_game_settings AS odds_room_game
			      WHERE odds_room_game.workspace_id = odds_workspace.id
			        AND odds_room_game.game_id = lottery_games.id AND odds_room_game.enabled = ?
			    ))
			    OR (odds_workspace.type IN ? AND EXISTS (
			      SELECT 1 FROM room_game_settings AS odds_room_game
			      WHERE odds_room_game.workspace_id = odds_workspace.id
			        AND odds_room_game.game_id = lottery_games.id AND odds_room_game.enabled = ?
			    ))
			  )
		)`, 1, workspacemodel.TypePlatform, "member", 1, false, []string{workspacemodel.TypeTenant, workspacemodel.TypeAgent}, true)
}

// AuditProductionOddsReadiness takes a stable, read-only database snapshot and
// proves that every room-visible game has a complete explicit price set for
// its exact current rule version. Missing and legacy numeric rows fail closed.
func AuditProductionOddsReadiness(ctx context.Context, db *gorm.DB) (*ProductionOddsReadinessReport, error) {
	if db == nil {
		return nil, fmt.Errorf("生产赔率审计数据库不可用")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var report *ProductionOddsReadinessReport
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var games []lottery.Game
		if err := productionOddsTargetGamesQuery(tx).
			Order("lottery_games.id ASC").Find(&games).Error; err != nil {
			return err
		}
		rows := make([]odds.PlayLimit, 0)
		if len(games) > 0 {
			gameIDs := make([]string, 0, len(games))
			for _, game := range games {
				gameIDs = append(gameIDs, game.ID)
			}
			if err := tx.Where("game_id IN ?", gameIDs).
				Order("game_id ASC, play_code ASC, id ASC").Find(&rows).Error; err != nil {
				return err
			}
		}
		report = assessProductionOddsReadiness(games, rows)
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("读取生产赔率完整度失败: %w", err)
	}
	return report, nil
}

func assessProductionOddsReadiness(games []lottery.Game, rows []odds.PlayLimit) *ProductionOddsReadinessReport {
	orderedGames := append([]lottery.Game(nil), games...)
	sort.Slice(orderedGames, func(left, right int) bool { return orderedGames[left].ID < orderedGames[right].ID })
	rowsByGame := make(map[string]map[string][]odds.PlayLimit, len(orderedGames))
	for _, row := range rows {
		// Runtime lookup treats identifiers as exact persisted keys. Readiness
		// must not normalize malformed rows into a catalogue match which the
		// betting path would later reject as unconfigured.
		if _, ok := rowsByGame[row.GameID]; !ok {
			rowsByGame[row.GameID] = make(map[string][]odds.PlayLimit)
		}
		rowsByGame[row.GameID][row.PlayCode] = append(rowsByGame[row.GameID][row.PlayCode], row)
	}

	report := &ProductionOddsReadinessReport{
		AuditedGames: len(orderedGames), IncompleteGames: make([]ProductionOddsGameReadiness, 0),
	}
	for _, game := range orderedGames {
		profile, ready := rulesForGame(&game)
		gameReport := ProductionOddsGameReadiness{GameID: game.ID, GameName: game.Name}
		if !ready {
			gameReport.Reason = "rules_not_ready"
			report.IncompleteGames = append(report.IncompleteGames, gameReport)
			continue
		}
		gameReport.RuleVersion = profile.Version
		catalog := PlayCatalogForGame(game.ID)
		gameReport.RequiredQuotes = len(catalog)
		report.RequiredQuotes += len(catalog)
		for _, item := range catalog {
			matches := rowsByGame[game.ID][item.PlayCode]
			if len(matches) == 1 && productionOddsRowReady(matches[0], profile.Version) {
				gameReport.ValidQuotes++
				report.ValidQuotes++
				continue
			}
			gameReport.InvalidPlayCodes = append(gameReport.InvalidPlayCodes, item.PlayCode)
		}
		if gameReport.ValidQuotes != gameReport.RequiredQuotes {
			gameReport.Reason = "quotes_incomplete"
			report.IncompleteGames = append(report.IncompleteGames, gameReport)
		}
	}
	report.Complete = len(report.IncompleteGames) == 0
	return report
}

func productionOddsRowReady(row odds.PlayLimit, ruleVersion string) bool {
	if !isActivePlatformOdds(row, ruleVersion) || row.ConfiguredAt == nil || row.ConfiguredAt.IsZero() {
		return false
	}
	values := []float64{row.Odds, row.MinBet, row.MaxBet, row.MaxUserPeriod, row.MaxPeriodTotal}
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return false
		}
	}
	for _, limit := range []float64{row.MinBet, row.MaxBet, row.MaxUserPeriod, row.MaxPeriodTotal} {
		if math.Abs(roundMoney(limit)-limit) > 0.0000001 {
			return false
		}
	}
	return row.MinBet > 0 &&
		(row.MaxBet <= 0 || row.MinBet <= row.MaxBet) &&
		(row.MaxUserPeriod <= 0 || row.MaxBet <= row.MaxUserPeriod) &&
		(row.MaxPeriodTotal <= 0 || row.MaxUserPeriod <= row.MaxPeriodTotal)
}
