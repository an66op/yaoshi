package services

import (
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"fmt"

	"gorm.io/gorm"
)

const MaxWorkspaceRobotQuota = 10

func validateWorkspaceRobotQuota(quota int) error {
	if quota < 0 || quota > MaxWorkspaceRobotQuota {
		return apperrors.NewBusinessError("INVALID_ROBOT_QUOTA", "机器人名额需在 0-10 之间")
	}
	return nil
}

func allocatedRobotProfileIDs(db *gorm.DB, workspaceID uint64, quota int) *gorm.DB {
	return db.Model(&workspacemodel.RobotProfile{}).
		Select("id").Where("workspace_id = ?", workspaceID).
		Order("id ASC").Limit(quota)
}

func applyWorkspaceRobotQuota(db *gorm.DB, workspaceID uint64, quota int) error {
	if err := validateWorkspaceRobotQuota(quota); err != nil {
		return err
	}
	var workspace workspacemodel.Workspace
	if err := db.Where("id = ? AND type = ?", workspaceID, workspacemodel.TypeAgent).First(&workspace).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NewBusinessError("INVALID_WORKSPACE", "代理房间不存在")
		}
		return err
	}
	if err := db.Model(&workspace).Update("robot_quota", quota).Error; err != nil {
		return err
	}

	var profiles []workspacemodel.RobotProfile
	if err := db.Where("workspace_id = ?", workspaceID).Order("id ASC").Find(&profiles).Error; err != nil {
		return err
	}
	if quota >= len(profiles) {
		return nil
	}
	excessProfileIDs := make([]uint64, 0, len(profiles)-quota)
	excessUserIDs := make([]uint64, 0, len(profiles)-quota)
	for _, profile := range profiles[quota:] {
		excessProfileIDs = append(excessProfileIDs, profile.ID)
		excessUserIDs = append(excessUserIDs, profile.UserID)
	}
	if err := db.Model(&workspacemodel.RobotProfile{}).Where("id IN ?", excessProfileIDs).Update("enabled", false).Error; err != nil {
		return err
	}
	if err := db.Model(&user.User{}).Where("workspace_id = ? AND user_id IN ?", workspaceID, excessUserIDs).Update("status", 0).Error; err != nil {
		return err
	}
	if err := db.Model(&workspacemodel.Membership{}).Where("workspace_id = ? AND user_id IN ?", workspaceID, excessUserIDs).Update("status", 0).Error; err != nil {
		return err
	}
	if quota == 0 {
		if err := db.Model(&workspacemodel.RobotSetting{}).Where("workspace_id = ?", workspaceID).Updates(map[string]any{
			"enabled": false, "pause_reason": "上级未分配机器人名额",
		}).Error; err != nil {
			return fmt.Errorf("停用无名额房间机器人: %w", err)
		}
	}
	return nil
}
