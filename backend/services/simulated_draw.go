package services

import (
	"backend/cluster"
	"backend/data/models/lottery"
	"context"
	"log"
	"time"

	"gorm.io/gorm"
)

// StartSimulatedDrawLoop advances platform-operated games on their configured
// clock. External games remain exclusively controlled by upstream source sync.
func StartSimulatedDrawLoop(ctx context.Context, db *gorm.DB) {
	go func() {
		publishDue := func(workCtx context.Context) error {
			var games []lottery.Game
			now := time.Now().UTC()
			workDB := db.WithContext(workCtx)
			if err := workDB.Where("source_kind IN ? AND enabled = ? AND next_draw_at <= ?", []string{"platform", "simulated"}, true, now).
				Order("next_draw_at asc").Find(&games).Error; err != nil {
				return err
			}
			service := NewBetAdminService(workDB)
			for _, game := range games {
				if err := workCtx.Err(); err != nil {
					return err
				}
				if _, supported := rulesForGame(&game); !supported {
					// Unknown PC/Mark Six products need verified rules, not a guessed
					// five-digit result. Leave their issue and historic draws intact.
					continue
				}
				issue, err := service.CurrentIssue(game.ID)
				if err != nil {
					continue
				}
				if _, err := service.PublishDraw(game.ID, issue, nil, "王者自动开奖"); err != nil {
					log.Printf("自动开奖失败: game=%s issue=%s error=%v", game.ID, issue, err)
				}
			}
			return nil
		}
		run := func() {
			_, err := cluster.RunWithLease(ctx, "scheduler:simulated-draw", 30*time.Second, publishDue)
			if err != nil {
				log.Printf("模拟开奖调度跳过或执行失败: %v", err)
			}
		}

		run()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}
