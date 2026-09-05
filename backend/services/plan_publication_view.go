package services

import (
	"backend/data/models/plan"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"strings"

	"gorm.io/gorm"
)

func lockPlanPublicationGame(tx *gorm.DB, workspaceID uint64, gameID string) error {
	return tx.Exec("SELECT lock_plan_publication_game(?, ?)", workspaceID, gameID).Error
}

func ensurePlanPublicationMember(tx *gorm.DB, userID, workspaceID uint64) error {
	if userID == 0 {
		return nil
	}
	var membershipCount int64
	if err := tx.Model(&workspacemodel.Membership{}).
		Joins(`JOIN "user" AS account ON account.user_id = workspace_memberships.user_id`).
		Where("workspace_memberships.workspace_id = ? AND workspace_memberships.user_id = ? AND workspace_memberships.status = 1 AND account.status = 1 AND account.deleted_at IS NULL", workspaceID, userID).
		Count(&membershipCount).Error; err != nil {
		return err
	}
	if membershipCount != 1 {
		return apperrors.NewBusinessError("ROOM_ACCESS_DENIED", "当前账号不属于该房间")
	}
	return nil
}

func visiblePlanIssues(detail PlanDetail) []string {
	seen := make(map[string]bool)
	issues := make([]string, 0)
	for _, rows := range [][]PlanRecommendationView{detail.Recommendations, detail.LatestRecommendations, detail.History} {
		for _, row := range rows {
			issue := strings.TrimSpace(row.Issue)
			if issue != "" && !seen[issue] {
				seen[issue] = true
				issues = append(issues, issue)
			}
		}
	}
	return issues
}

// recordVisiblePlanPublicationViews binds every unique publication actually
// present in one response to an immutable member-view receipt. Refreshes use
// ON CONFLICT DO NOTHING, preserving the original viewed_at timestamp.
func recordVisiblePlanPublicationViews(tx *gorm.DB, userID, workspaceID uint64, gameID string, position int, key string, detail PlanDetail) error {
	if err := ensurePlanPublicationMember(tx, userID, workspaceID); err != nil || userID == 0 {
		return err
	}
	if err := lockPlanPublicationGame(tx, workspaceID, gameID); err != nil {
		return err
	}
	auditPosition := position
	if !racingPlanGameID(gameID) {
		auditPosition = 0
	}
	for _, issue := range visiblePlanIssues(detail) {
		if err := tx.Exec(`INSERT INTO plan_publication_views
			(workspace_id,user_id,game_id,issue,position,plan_key,viewed_at)
			VALUES (?,?,?,?,?,?,clock_timestamp()) ON CONFLICT DO NOTHING`,
			workspaceID, userID, gameID, issue, auditPosition, key).Error; err != nil {
			return err
		}
	}
	return nil
}

func recommendationPublicationViewed(db *gorm.DB, row plan.Recommendation) (bool, error) {
	var count int64
	err := db.Model(&plan.PublicationView{}).
		Where("workspace_id = ? AND game_id = ? AND issue = ? AND position = 0", row.WorkspaceID, row.GameID, row.Issue).
		Count(&count).Error
	return count > 0, err
}
