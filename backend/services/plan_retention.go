package services

import (
	"backend/data/models/plan"
	"fmt"

	"gorm.io/gorm"
)

// Only request-generated plan payload is pruned, inside the visit's locked
// transaction. The stream and its current cycle are durable high-water marks:
// old issue IDs/times cannot be republished after an old period is removed.
// Legacy/manual recommendations, issue/draw, betting and account data are never
// targets. The two concrete deletes are audited in deletion_policy_test.go.
func prunePlanStreamHistory(tx *gorm.DB, streamID uint64) error {
	if streamID == 0 {
		return fmt.Errorf("计划历史清理缺少明确方案")
	}
	oldPeriods := tx.Model(&plan.StreamPeriod{}).Select("id").Where("stream_id = ?", streamID).Order("id DESC").Offset(retainedPlanPeriods)
	if err := tx.Where("stream_id = ? AND id IN (?)", streamID, oldPeriods).Delete(&plan.StreamPeriod{}).Error; err != nil {
		return err
	}
	return tx.Where("stream_id = ?", streamID).
		Where("NOT EXISTS (SELECT 1 FROM plan_stream_periods AS period WHERE period.cycle_id = plan_stream_cycles.id)").
		Where("NOT EXISTS (SELECT 1 FROM plan_streams AS stream WHERE stream.cycle_id = plan_stream_cycles.id)").
		Delete(&plan.StreamCycle{}).Error
}
