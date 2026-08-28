package ws

import (
	"backend/cluster"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const (
	clusterChannel          = "ws-events"
	clusterActionBroadcast  = "broadcast"
	clusterActionUsers      = "users"
	clusterActionDisconnect = "disconnect"
)

type clusterEnvelope struct {
	Origin             string          `json:"origin"`
	EventID            uint64          `json:"event_id,omitempty"`
	Action             string          `json:"action"`
	WorkspaceID        uint64          `json:"workspace_id,omitempty"`
	UserIDs            []uint64        `json:"user_ids,omitempty"`
	RevokedAuthVersion uint64          `json:"revoked_auth_version,omitempty"`
	Payload            json.RawMessage `json:"payload,omitempty"`
}

func publishClusterEnvelope(ctx context.Context, envelope clusterEnvelope) error {
	envelope.Origin = cluster.InstanceID()
	payload, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	return cluster.Publish(ctx, clusterChannel, payload)
}

func publishClusterEvent(envelope clusterEnvelope) {
	if !cluster.Enabled() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := publishClusterEnvelope(ctx, envelope); err != nil {
		log.Printf("Redis WebSocket 事件发布失败: %v", err)
	}
}

// StartClusterBridge subscribes before returning, so release startup fails
// rather than pretending cross-instance delivery is healthy. The background
// loops reconnect after transient Redis disconnects. Ordinary realtime events
// use Pub/Sub; security revocations use a PostgreSQL outbox and Redis Stream.
func StartClusterBridge(ctx context.Context, db *gorm.DB) error {
	if !cluster.Enabled() {
		return nil
	}
	if db == nil {
		return fmt.Errorf("WebSocket cluster bridge database is nil")
	}
	subscription, err := cluster.Subscribe(ctx, clusterChannel)
	if err != nil {
		return err
	}
	revocationCursor, err := currentRevocationStreamCursor(ctx)
	if err != nil {
		_ = subscription.Close()
		return err
	}
	go consumeClusterSubscription(ctx, subscription)
	go consumeRevocationStream(ctx, revocationCursor)
	startRevocationOutboxWorker(ctx, db)
	return nil
}

func consumeClusterSubscription(ctx context.Context, subscription *redis.PubSub) {
	for {
		for message := range subscription.Channel() {
			var envelope clusterEnvelope
			if err := json.Unmarshal([]byte(message.Payload), &envelope); err != nil {
				log.Printf("忽略无效的 Redis WebSocket 事件: %v", err)
				continue
			}
			deliverClusterEnvelope(envelope)
		}
		_ = subscription.Close()
		if ctx.Err() != nil {
			return
		}
		for {
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return
			case <-timer.C:
			}
			next, err := cluster.Subscribe(ctx, clusterChannel)
			if err != nil {
				log.Printf("Redis WebSocket 订阅重连失败: %v", err)
				continue
			}
			subscription = next
			break
		}
	}
}

func deliverClusterEnvelope(envelope clusterEnvelope) {
	if envelope.Origin == "" || envelope.Origin == cluster.InstanceID() {
		return
	}
	switch envelope.Action {
	case clusterActionBroadcast:
		defaultHub.broadcast(envelope.Payload, 0, envelope.WorkspaceID)
	case clusterActionUsers:
		defaultHub.broadcastUsers(envelope.Payload, envelope.UserIDs, envelope.WorkspaceID)
	case clusterActionDisconnect:
		deliverRevocationEnvelope(envelope)
	}
}

func deliverRevocationEnvelope(envelope clusterEnvelope) {
	if envelope.EventID == 0 || envelope.RevokedAuthVersion == 0 {
		return
	}
	for _, userID := range envelope.UserIDs {
		defaultHub.disconnectUserGeneration(userID, envelope.RevokedAuthVersion)
	}
}
