package services

import (
	"backend/cluster"
	"context"
	"log"
	"time"

	"gorm.io/gorm"
)

// StartRedPacketExpiry closes abandoned envelopes without requiring somebody
// to open the chat page. Closing is idempotent and returns the unused room
// reserve in the same transaction.
func StartRedPacketExpiry(ctx context.Context, db *gorm.DB) {
	if db == nil {
		return
	}
	service := NewChatAdminService(db)
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, err := cluster.RunWithLease(ctx, "scheduler:red-packet-expiry", 2*time.Minute, func() error {
					return service.CloseExpiredRedPackets(200)
				})
				if err != nil {
					log.Printf("关闭过期红包失败: %v", err)
				}
			}
		}
	}()
}
