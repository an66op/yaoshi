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
	lifecycleRunHour   = 3
	lifecycleRunMinute = 30
)

// StartDataLifecycleLoop runs enabled retention policies once per Beijing
// calendar day. Policies are disabled by default, so installing the feature
// never deletes data. Every automatic run still uses the same frozen preview,
// request id, transaction and recovery receipt as a manual operation.
func StartDataLifecycleLoop(ctx context.Context, db *gorm.DB) {
	startGameChatLifecycleLoop(ctx, db)
	go func() {
		for {
			now := time.Now()
			next := nextLifecycleRun(now)
			timer := time.NewTimer(time.Until(next))
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return
			case <-timer.C:
				_, err := cluster.RunWithLease(ctx, "scheduler:data-lifecycle", 6*time.Hour, func(workCtx context.Context) error {
					runScheduledLifecycle(workCtx, db, next)
					return nil
				})
				if err != nil {
					log.Printf("数据生命周期自动维护跳过或执行失败: %v", err)
				}
			}
		}
	}()
}

func runScheduledLifecycle(ctx context.Context, db *gorm.DB, scheduledAt time.Time) {
	db = db.WithContext(ctx)
	var platform workspacemodel.Workspace
	if err := db.Where("type = ? AND status = 1", workspacemodel.TypePlatform).Order("id ASC").First(&platform).Error; err != nil {
		log.Printf("数据生命周期自动维护跳过：平台工作区不可用: %v", err)
		return
	}

	var workspaces []workspacemodel.Workspace
	if err := db.Where("status = 1").Order("id ASC").Find(&workspaces).Error; err != nil {
		log.Printf("数据生命周期自动维护读取工作区失败: %v", err)
		return
	}

	service := NewDataLifecycleService(db)
	actor := LifecycleActor{UserID: platform.OwnerUserID, Username: "系统维护", WorkspaceID: platform.ID}
	date := scheduledAt.In(beijingLifecycleLocation()).Format("20060102")
	for _, workspace := range workspaces {
		if ctx.Err() != nil {
			return
		}
		policies, err := service.Policies(workspace.ID)
		if err != nil {
			log.Printf("数据生命周期自动维护读取工作区 %d 策略失败: %v", workspace.ID, err)
			continue
		}
		classes := make([]string, 0, len(policies))
		for _, policy := range policies {
			if policy.Enabled && policy.DataClass != lifecycle.ClassGameChatMessages {
				classes = append(classes, policy.DataClass)
			}
		}
		if len(classes) == 0 {
			continue
		}

		workspaceID := workspace.ID
		requestID := fmt.Sprintf("auto:%s:ws:%d", date, workspace.ID)
		preview, err := service.Preview(CleanupPreviewInput{
			RequestID: requestID, WorkspaceID: &workspaceID,
			DataClasses: classes, BatchLimit: defaultCleanupBatch,
		}, actor)
		if err != nil {
			log.Printf("数据生命周期自动维护预览工作区 %d 失败: %v", workspace.ID, err)
			continue
		}
		if _, err := service.Execute(CleanupExecuteInput{RequestID: preview.RequestID}, actor); err != nil {
			log.Printf("数据生命周期自动维护执行工作区 %d 失败: %v", workspace.ID, err)
			continue
		}
		log.Printf("数据生命周期自动维护完成：workspace=%d request_id=%s", workspace.ID, requestID)
	}
}

func nextLifecycleRun(now time.Time) time.Time {
	location := beijingLifecycleLocation()
	local := now.In(location)
	next := time.Date(local.Year(), local.Month(), local.Day(), lifecycleRunHour, lifecycleRunMinute, 0, 0, location)
	if !next.After(local) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func beijingLifecycleLocation() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err == nil {
		return location
	}
	return time.FixedZone("CST", 8*60*60)
}
