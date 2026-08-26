package services

import (
	"backend/data/models/lottery"
	"context"
	"time"

	"gorm.io/gorm"
)

// StartSimulatedDrawLoop advances platform-operated games on their configured
// clock. External games remain exclusively controlled by upstream source sync.
func StartSimulatedDrawLoop(ctx context.Context, db *gorm.DB) {
	go func() {
		publishDue := func() {
			var games []lottery.Game
			now := time.Now().UTC()
			if err := db.Where("source_kind IN ? AND enabled = ? AND next_draw_at <= ?", []string{"platform", "simulated"}, true, now).
				Order("next_draw_at asc").Find(&games).Error; err != nil {
				return
			}
			service := NewBetAdminService(db)
			for _, game := range games {
				issue, err := service.CurrentIssue(game.ID)
				if err != nil {
					continue
				}
				_, _ = service.PublishDraw(game.ID, issue, nil, "王者自动开奖")
			}
		}

		publishDue()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				publishDue()
			}
		}
	}()
}
