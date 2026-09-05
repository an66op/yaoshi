package ws

import (
	"backend/cluster"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const (
	revocationStreamName       = "ws-session-revocations"
	revocationOutboxLeaseName  = "ws-session-revocation-outbox"
	revocationCleanupLeaseName = "ws-session-revocation-cleanup"
	revocationOutboxBatchSize  = 64
	revocationOutboxPollPeriod = time.Second
	revocationStreamRetention  = 24 * time.Hour
	revocationReceiptRetention = 7 * 24 * time.Hour
	revocationCleanupPeriod    = time.Hour
	revocationCleanupBatchSize = 10000
)

type sessionRevocationOutbox struct {
	ID                 uint64     `gorm:"column:id;primaryKey"`
	UserID             uint64     `gorm:"column:user_id"`
	RevokedAuthVersion uint64     `gorm:"column:revoked_auth_version"`
	AttemptCount       int        `gorm:"column:attempt_count"`
	NextAttemptAt      time.Time  `gorm:"column:next_attempt_at"`
	StreamID           *string    `gorm:"column:stream_id"`
	LastError          *string    `gorm:"column:last_error"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	DeliveredAt        *time.Time `gorm:"column:delivered_at"`
}

func (sessionRevocationOutbox) TableName() string {
	return "ws_session_revocation_outbox"
}

type revocationOutboxStore interface {
	ready(context.Context, int) ([]sessionRevocationOutbox, error)
	markDelivered(context.Context, uint64, string) error
	markFailed(context.Context, uint64, time.Time, string) error
}

type postgresRevocationOutbox struct {
	db *gorm.DB
}

func (store postgresRevocationOutbox) ready(ctx context.Context, limit int) ([]sessionRevocationOutbox, error) {
	if store.db == nil {
		return nil, errors.New("revocation outbox database is nil")
	}
	if limit < 1 {
		limit = 1
	}
	var rows []sessionRevocationOutbox
	err := store.db.WithContext(ctx).
		Where("delivered_at IS NULL AND next_attempt_at <= ?", time.Now().UTC()).
		Order("id ASC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func (store postgresRevocationOutbox) markDelivered(ctx context.Context, id uint64, streamID string) error {
	if store.db == nil {
		return errors.New("revocation outbox database is nil")
	}
	return store.db.WithContext(ctx).Model(&sessionRevocationOutbox{}).
		Where("id = ? AND delivered_at IS NULL", id).
		Updates(map[string]any{
			"stream_id":     streamID,
			"delivered_at":  time.Now().UTC(),
			"last_error":    nil,
			"attempt_count": gorm.Expr("attempt_count + 1"),
		}).Error
}

func (store postgresRevocationOutbox) markFailed(ctx context.Context, id uint64, nextAttempt time.Time, message string) error {
	if store.db == nil {
		return errors.New("revocation outbox database is nil")
	}
	message = strings.TrimSpace(message)
	if len(message) > 1000 {
		message = message[:1000]
	}
	return store.db.WithContext(ctx).Model(&sessionRevocationOutbox{}).
		Where("id = ? AND delivered_at IS NULL", id).
		Updates(map[string]any{
			"attempt_count":   gorm.Expr("attempt_count + 1"),
			"next_attempt_at": nextAttempt.UTC(),
			"last_error":      message,
		}).Error
}

type revocationStreamAppender func(context.Context, clusterEnvelope) (string, error)

type revocationOutboxWorker struct {
	store  revocationOutboxStore
	append revocationStreamAppender
	now    func() time.Time
}

func newRevocationOutboxWorker(db *gorm.DB) *revocationOutboxWorker {
	return &revocationOutboxWorker{
		store:  postgresRevocationOutbox{db: db},
		append: appendRevocationStream,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

func (worker *revocationOutboxWorker) flush(ctx context.Context) (int, error) {
	if worker == nil || worker.store == nil || worker.append == nil {
		return 0, errors.New("revocation outbox worker is incomplete")
	}
	rows, err := worker.store.ready(ctx, revocationOutboxBatchSize)
	if err != nil {
		return 0, err
	}
	delivered := 0
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return delivered, err
		}
		envelope := clusterEnvelope{
			EventID:            row.ID,
			Action:             clusterActionDisconnect,
			UserIDs:            []uint64{row.UserID},
			RevokedAuthVersion: row.RevokedAuthVersion,
		}
		publishContext, cancel := context.WithTimeout(ctx, time.Second)
		streamID, publishErr := worker.append(publishContext, envelope)
		cancel()
		if publishErr != nil {
			now := time.Now().UTC()
			if worker.now != nil {
				now = worker.now().UTC()
			}
			next := now.Add(revocationRetryDelay(row.AttemptCount + 1))
			if markErr := worker.store.markFailed(ctx, row.ID, next, publishErr.Error()); markErr != nil {
				return delivered, errors.Join(publishErr, markErr)
			}
			// A Redis transport error is shared by the batch. Leave later rows
			// untouched so one outage consumes one bounded timeout per poll.
			return delivered, publishErr
		}
		if err := worker.store.markDelivered(ctx, row.ID, streamID); err != nil {
			// XADD succeeded but the receipt was not confirmed in PostgreSQL.
			// Keeping the row pending deliberately causes an at-least-once replay.
			return delivered, err
		}
		delivered++
	}
	return delivered, nil
}

func revocationRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Second
	for step := 1; step < attempt && delay < time.Minute; step++ {
		delay *= 2
		if delay > time.Minute {
			delay = time.Minute
		}
	}
	return delay
}

func appendRevocationStream(ctx context.Context, envelope clusterEnvelope) (string, error) {
	current := cluster.Client()
	if current == nil {
		return "", cluster.ErrUnavailable
	}
	if envelope.EventID == 0 || envelope.RevokedAuthVersion == 0 || len(envelope.UserIDs) == 0 {
		return "", errors.New("invalid session revocation envelope")
	}
	envelope.Origin = cluster.InstanceID()
	payload, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	return current.XAdd(ctx, &redis.XAddArgs{
		Stream: cluster.Key("stream", revocationStreamName),
		Values: map[string]any{"envelope": string(payload)},
	}).Result()
}

func startRevocationOutboxWorker(ctx context.Context, db *gorm.DB) {
	worker := newRevocationOutboxWorker(db)
	go func() {
		for {
			_, err := cluster.RunWithLease(ctx, revocationOutboxLeaseName, 30*time.Second, func(workCtx context.Context) error {
				_, flushErr := worker.flush(workCtx)
				return flushErr
			})
			if err != nil && ctx.Err() == nil {
				log.Printf("WebSocket 撤权 outbox 发布失败: %v", err)
			}

			timer := time.NewTimer(revocationOutboxPollPeriod)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return
			case <-timer.C:
			}
		}
	}()
	startRevocationRetention(ctx, db)
}

func startRevocationRetention(ctx context.Context, db *gorm.DB) {
	go func() {
		for {
			_, err := cluster.RunWithLease(ctx, revocationCleanupLeaseName, 5*time.Minute, func(workCtx context.Context) error {
				return cleanupRevocationHistory(workCtx, db, time.Now().UTC())
			})
			if err != nil && ctx.Err() == nil {
				log.Printf("WebSocket 撤权历史清理失败: %v", err)
			}

			timer := time.NewTimer(revocationCleanupPeriod)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return
			case <-timer.C:
			}
		}
	}()
}

func cleanupRevocationHistory(ctx context.Context, db *gorm.DB, now time.Time) error {
	if db == nil {
		return errors.New("revocation cleanup database is nil")
	}
	// Every open socket independently revalidates PostgreSQL identity every 30
	// seconds. Keeping Stream entries for 24 hours therefore leaves a very wide
	// recovery window for a live instance whose Redis connection was interrupted;
	// an instance farther behind has already closed invalid sockets via the DB.
	current := cluster.Client()
	if current == nil {
		return cluster.ErrUnavailable
	}
	trimContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := current.XTrimMinIDApprox(
		trimContext,
		cluster.Key("stream", revocationStreamName),
		revocationStreamMinimumID(now),
		revocationCleanupBatchSize,
	).Err(); err != nil && err != redis.Nil {
		return err
	}

	var deliveredIDs []uint64
	if err := db.WithContext(ctx).Model(&sessionRevocationOutbox{}).
		Where("delivered_at IS NOT NULL AND delivered_at < ?", now.Add(-revocationReceiptRetention)).
		Order("delivered_at ASC, id ASC").
		Limit(revocationCleanupBatchSize).
		Pluck("id", &deliveredIDs).Error; err != nil {
		return err
	}
	if len(deliveredIDs) == 0 {
		return nil
	}
	return db.WithContext(ctx).
		Where("id IN ? AND delivered_at IS NOT NULL", deliveredIDs).
		Delete(&sessionRevocationOutbox{}).Error
}

func revocationStreamMinimumID(now time.Time) string {
	return fmt.Sprintf("%d-0", now.UTC().Add(-revocationStreamRetention).UnixMilli())
}

func currentRevocationStreamCursor(ctx context.Context) (string, error) {
	current := cluster.Client()
	if current == nil {
		return "", cluster.ErrUnavailable
	}
	queryContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	messages, err := current.XRevRangeN(queryContext, cluster.Key("stream", revocationStreamName), "+", "-", 1).Result()
	if err != nil && err != redis.Nil {
		return "", fmt.Errorf("read WebSocket revocation stream cursor: %w", err)
	}
	if len(messages) == 0 {
		return "0-0", nil
	}
	return messages[0].ID, nil
}

func consumeRevocationStream(ctx context.Context, cursor string) {
	if strings.TrimSpace(cursor) == "" {
		cursor = "0-0"
	}
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		current := cluster.Client()
		if current == nil {
			if !waitRevocationReconnect(ctx) {
				return
			}
			continue
		}
		streams, err := current.XRead(ctx, &redis.XReadArgs{
			Streams: []string{cluster.Key("stream", revocationStreamName), cursor},
			Count:   128,
			Block:   2 * time.Second,
		}).Result()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("Redis WebSocket 撤权流读取失败，将从游标 %s 重试: %v", cursor, err)
			if !waitRevocationReconnect(ctx) {
				return
			}
			continue
		}
		for _, stream := range streams {
			for _, message := range stream.Messages {
				envelope, decodeErr := decodeRevocationStreamMessage(message)
				if decodeErr != nil {
					log.Printf("忽略无效的 Redis WebSocket 撤权流事件 %s: %v", message.ID, decodeErr)
					cursor = message.ID
					continue
				}
				deliverRevocationEnvelope(envelope)
				// Advance only after the in-process delivery completed. If the
				// process exits earlier, its sockets exit with it; connection errors
				// retain this cursor and replay the durable stream entry.
				cursor = message.ID
			}
		}
	}
}

func decodeRevocationStreamMessage(message redis.XMessage) (clusterEnvelope, error) {
	raw, ok := message.Values["envelope"]
	if !ok {
		return clusterEnvelope{}, errors.New("missing envelope")
	}
	var payload []byte
	switch value := raw.(type) {
	case string:
		payload = []byte(value)
	case []byte:
		payload = value
	default:
		payload = []byte(fmt.Sprint(value))
	}
	var envelope clusterEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return clusterEnvelope{}, err
	}
	if envelope.EventID == 0 || envelope.Action != clusterActionDisconnect || envelope.RevokedAuthVersion == 0 || len(envelope.UserIDs) == 0 {
		return clusterEnvelope{}, errors.New("incomplete session revocation envelope")
	}
	return envelope, nil
}

func waitRevocationReconnect(ctx context.Context) bool {
	timer := time.NewTimer(time.Second)
	select {
	case <-ctx.Done():
		if !timer.Stop() {
			<-timer.C
		}
		return false
	case <-timer.C:
		return true
	}
}
