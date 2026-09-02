package services

import (
	"backend/data/models/lottery"
	"backend/data/models/plan"
	"backend/data/models/settings"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const singlePeriodPlanKey = "single-period-three-codes"
const retainedPlanPeriods = 20

func planHistoryLimit(limits []int) int {
	if len(limits) == 0 || limits[0] < 1 {
		return 6
	}
	if limits[0] > 10 {
		return 10
	}
	return limits[0]
}

func planRequestedStreamAllowed(config PlanAutomationView, gameID string, position int, key string) bool {
	if gameID == "speed-racing" {
		return planStreamAllowed(config, position, key)
	}
	if !config.Enabled || position != 1 || key != singlePeriodPlanKey {
		return false
	}
	for _, id := range config.GameIDs {
		if id == gameID {
			return true
		}
	}
	return false
}

func (s *PlanContentService) ActivateGame(ctx context.Context, workspaceID uint64, gameID string, historyLimits ...int) (PlanDetail, error) {
	if gameID == "speed-racing" {
		return PlanDetail{}, apperrors.NewBusinessError("INVALID_REQUEST", "请选择极速赛车推荐方案")
	}
	if _, err := s.touchPlan(ctx, workspaceID, gameID, 1, singlePeriodPlanKey); err != nil {
		return PlanDetail{}, err
	}
	return s.Detail(workspaceID, gameID, historyLimits...)
}

// A member visit serializes with configuration changes and other instances.
// Only the requested identity is touched. The persisted update timestamp
// coalesces simultaneous visitors for five seconds without extending a lease
// or running generation again; GET never enters this path.
func (s *PlanContentService) touchPlan(ctx context.Context, workspaceID uint64, gameID string, position int, key string) (PlanAutomationRun, error) {
	result := PlanAutomationRun{WorkspaceID: workspaceID, RanAt: time.Now().UTC(), SkippedGameIDs: []string{}, Notice: PlanDemoNotice}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Match betting and room trading: game before automation, room, owner,
		// settings or stream locks. Taking SHARE later can deadlock behind a
		// queued platform UPDATE while room trading waits for our room lock.
		var game lottery.Game
		gameErr := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&game, "id = ?", gameID).Error
		if gameErr != nil && !errors.Is(gameErr, gorm.ErrRecordNotFound) {
			return gameErr
		}
		var row plan.Automation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "workspace_id = ?", workspaceID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.NewBusinessError("AUTOMATION_DISABLED", "本房间尚未开启按访问推荐")
			}
			return err
		}
		config, err := planAutomationView(row)
		if err != nil {
			return err
		}
		if !config.Enabled || row.Mode != "demo" {
			return apperrors.NewBusinessError("AUTOMATION_DISABLED", "本房间尚未开启按访问推荐")
		}
		if !planRequestedStreamAllowed(config, gameID, position, key) {
			return apperrors.NewBusinessError("PLAN_STREAM_NOT_ALLOWED", "管理员未开放此彩种、位置或推荐方案")
		}
		var room workspacemodel.Workspace
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&room, workspaceID).Error; err != nil {
			return err
		}
		if room.Status != 1 || room.RoomCode == "" || (room.Type != workspacemodel.TypeTenant && room.Type != workspacemodel.TypeAgent) {
			return apperrors.NewBusinessError("ROOM_NOT_FOUND", "房间不存在或已停用")
		}
		var owner user.User
		if err := tx.Select("user_id", "status").Clauses(clause.Locking{Strength: "SHARE"}).First(&owner, room.OwnerUserID).Error; err != nil {
			return err
		}
		var roomSettings settings.SystemConfig
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&roomSettings, "workspace_id = ?", workspaceID).Error; err != nil {
			return err
		}
		if owner.Status != 1 || !roomSettings.RoomEnabled || !roomSettings.PredictionEnabled {
			return apperrors.NewBusinessError("ROOM_CLOSED", "房间或计划展示已关闭")
		}
		available, err := planStreamRoomAvailable(tx, workspaceID, gameID)
		if err != nil {
			return err
		}
		if !available {
			return apperrors.NewBusinessError("ROOM_CLOSED", "房间、推荐展示或彩种已关闭")
		}
		// Keep permission/availability failures ahead of a missing catalogue
		// row, as before; no later query may acquire a different game lock.
		if gameErr != nil {
			return gameErr
		}
		if _, _, supported := planDemoNumberRange(game); !supported {
			return apperrors.NewBusinessError("INVALID_REQUEST", "该彩种暂不支持按访问推荐")
		}
		streams, cycles, err := readPlanStreams(tx, workspaceID)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		var selected plan.Stream
		active := 0
		for _, stream := range streams {
			if planRequestedStreamAllowed(config, stream.GameID, stream.Position, stream.PlanKey) && planStreamActive(stream, cycles[stream.CycleID], now) {
				active++
			}
			if stream.GameID == gameID && stream.Position == position && stream.PlanKey == key {
				selected = stream
			}
		}
		if selected.ID > 0 && planStreamActive(selected, cycles[selected.CycleID], now) && now.Sub(selected.UpdatedAt) < 5*time.Second {
			return nil
		}
		if !planStreamActive(selected, cycles[selected.CycleID], now) && active >= MaxActivePlanStreams {
			return apperrors.NewBusinessError("PLAN_STREAM_LIMIT", "本房间已有20个访问中的推荐方案，请稍后重试")
		}
		until := now.Add(time.Minute)
		if selected.ID == 0 {
			selected = plan.Stream{WorkspaceID: workspaceID, GameID: gameID, Position: position, PlanKey: key, ActiveUntil: &until}
			if err := tx.Create(&selected).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&selected).Updates(map[string]any{"active_until": until, "revoked": false}).Error; err != nil {
				return err
			}
			selected.ActiveUntil, selected.Revoked, selected.UpdatedAt = &until, false, now
		}
		created, eligible, err := generatePlanDemoGame(tx, workspaceID, game, roomSettings.GameSettingsJSON, selected)
		if err != nil {
			return err
		}
		if gameID == "speed-racing" {
			if err := prunePlanStreamHistory(tx, selected.ID); err != nil {
				return err
			}
		}
		result.CreatedCount = created
		if eligible {
			result.EligibleGameCount = 1
		} else {
			result.SkippedGameIDs = append(result.SkippedGameIDs, gameID)
		}
		return tx.Model(&row).Updates(map[string]any{"last_run_at": now, "last_created_count": created, "last_error": ""}).Error
	})
	if err != nil {
		result.CreatedCount, result.EligibleGameCount = 0, 0
	}
	return result, err
}
