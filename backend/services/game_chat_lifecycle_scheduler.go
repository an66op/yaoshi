package services

import (
	"backend/cluster"
	"backend/data/models/lifecycle"
	workspacemodel "backend/data/models/workspace"
	"context"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
)

const (
	gameChatCleanupBatchSize  = 1000
	gameChatCleanupMaxBatches = 5
	gameChatCleanupRunBudget  = 2 * time.Minute
)

// Game-room traffic is higher volume than private conversations. Its opt-in
// policy is serviced hourly, in small bounded transactions, rather than being
// limited to one daily batch. Financial records and command evidence are not
// eligible for this content policy.
func startGameChatLifecycleLoop(ctx context.Context, db *gorm.DB) {
	go func() {
		for {
			next := nextGameChatLifecycleRun(time.Now())
			timer := time.NewTimer(time.Until(next))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				runCtx, cancel := context.WithTimeout(ctx, gameChatCleanupRunBudget)
				_, err := cluster.RunWithLease(runCtx, "scheduler:game-chat-lifecycle", 5*time.Minute, func(workCtx context.Context) error {
					return runScheduledGameChatLifecycle(workCtx, db, next)
				})
				cancel()
				if err != nil && ctx.Err() == nil {
					log.Printf("游戏聊天自动维护跳过或失败: %v", err)
				}
			}
		}
	}()
}

func nextGameChatLifecycleRun(now time.Time) time.Time {
	local := now.In(beijingLifecycleLocation())
	next := time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), 10, 0, 0, local.Location())
	if !next.After(local) {
		next = next.Add(time.Hour)
	}
	return next
}

func gameChatCleanupModes(policy lifecycle.RetentionPolicy) []string {
	if !policy.Enabled || policy.DataClass != lifecycle.ClassGameChatMessages {
		return nil
	}
	modes := []string{DeleteModeSoft}
	// Permanent deletion is a second, explicit opt-in. Enabling ordinary
	// retention alone must never authorize an irreversible purge.
	if policy.PurgeAfterDays > 0 {
		modes = append(modes, DeleteModeHard)
	}
	return modes
}

func runScheduledGameChatLifecycle(ctx context.Context, db *gorm.DB, scheduledAt time.Time) error {
	db = db.WithContext(ctx)
	var platform workspacemodel.Workspace
	if err := db.Where("type = ? AND status = 1", workspacemodel.TypePlatform).Order("id ASC").First(&platform).Error; err != nil {
		return err
	}
	var workspaces []workspacemodel.Workspace
	if err := db.Where("status = 1").Order("id ASC").Find(&workspaces).Error; err != nil {
		return err
	}
	service := NewDataLifecycleService(db)
	actor := LifecycleActor{UserID: platform.OwnerUserID, Username: "系统聊天维护", WorkspaceID: platform.ID}
	// Rotate the first room every hour so a busy room cannot indefinitely
	// starve rooms at the end of the list when the overall time budget expires.
	if len(workspaces) > 0 {
		offset := int(scheduledAt.Unix()/3600) % len(workspaces)
		workspaces = append(workspaces[offset:], workspaces[:offset]...)
	}
	for _, workspace := range workspaces {
		if err := ctx.Err(); err != nil {
			return err
		}
		policy, _, err := service.policyForWorkspace(workspace.ID, lifecycle.ClassGameChatMessages)
		if err != nil {
			log.Printf("游戏聊天自动维护读取策略失败：workspace=%d error=%v", workspace.ID, err)
			continue
		}
		for _, mode := range gameChatCleanupModes(policy) {
			err = runGameChatCleanupBatches(ctx, scheduledAt, workspace.ID, mode, func(requestID string, limit int) (int64, error) {
				return executeGameChatCleanupBatch(db, workspace.ID, mode, requestID, limit, actor)
			})
			if err != nil {
				log.Printf("游戏聊天自动维护失败：workspace=%d mode=%s error=%v", workspace.ID, mode, err)
			}
		}
	}
	return ctx.Err()
}

func executeGameChatCleanupBatch(db *gorm.DB, workspaceID uint64, mode, requestID string, limit int, actor LifecycleActor) (int64, error) {
	var affected int64
	err := db.Transaction(func(tx *gorm.DB) error {
		// Policy updates and cleanup execution share this transaction lock. A
		// scheduled batch must re-read the current opt-ins under the lock, not
		// continue purging from a snapshot taken before an operator disabled it.
		if tx.Dialector.Name() == "postgres" {
			if err := tx.Exec(`SELECT pg_advisory_xact_lock(?)`, int64(729421118)).Error; err != nil {
				return err
			}
		}
		service := NewDataLifecycleService(tx)
		policy, _, err := service.policyForWorkspace(workspaceID, lifecycle.ClassGameChatMessages)
		if err != nil {
			return err
		}
		allowed := false
		for _, currentMode := range gameChatCleanupModes(policy) {
			allowed = allowed || mode == currentMode
		}
		if !allowed {
			return nil
		}
		days := policy.RetentionDays
		if mode == DeleteModeHard {
			days = policy.PurgeAfterDays
		}
		cutoff := service.now().AddDate(0, 0, -days)
		// Do not append empty hourly receipts forever in an idle room. An
		// existence probe stops at the first candidate instead of repeating
		// Preview's complete count of an accumulated backlog on every batch.
		hasCandidates, err := hasGameChatCleanupCandidates(tx, workspaceID, mode, cutoff)
		if err != nil || !hasCandidates {
			return err
		}
		preview, err := service.Preview(CleanupPreviewInput{
			RequestID: requestID, WorkspaceID: &workspaceID,
			DataClasses: []string{lifecycle.ClassGameChatMessages}, BatchLimit: limit, DeleteMode: mode,
		}, actor)
		if err != nil {
			return err
		}
		result, err := service.Execute(CleanupExecuteInput{RequestID: preview.RequestID}, actor)
		if err != nil {
			return err
		}
		for _, item := range result.Items {
			affected += item.AffectedCount
		}
		return nil
	})
	return affected, err
}

func hasGameChatCleanupCandidates(db *gorm.DB, workspaceID uint64, mode string, cutoff time.Time) (bool, error) {
	deletedPredicate := "message.deleted_at IS NULL AND message.created_at < ?"
	if mode == DeleteModeHard {
		deletedPredicate = "message.deleted_at IS NOT NULL AND message.deleted_at < ?"
	}
	var found bool
	err := db.Raw(`SELECT EXISTS (
		SELECT 1 FROM member_chat_messages message
		WHERE message.workspace_id = ? AND `+deletedPredicate+`
		  AND `+gameChatLifecyclePredicate+`
	)`, workspaceID, cutoff).Scan(&found).Error
	return found, err
}

func runGameChatCleanupBatches(ctx context.Context, scheduledAt time.Time, workspaceID uint64, mode string, run func(string, int) (int64, error)) error {
	stamp := scheduledAt.In(beijingLifecycleLocation()).Format("2006010215")
	for batch := 1; batch <= gameChatCleanupMaxBatches; batch++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		requestID := fmt.Sprintf("gamechat:%s:ws:%d:%s:%02d", stamp, workspaceID, mode, batch)
		affected, err := run(requestID, gameChatCleanupBatchSize)
		if err != nil {
			return err
		}
		if affected == 0 {
			break
		}
	}
	return nil
}
