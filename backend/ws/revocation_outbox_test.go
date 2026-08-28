package ws

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type fakeRevocationOutboxStore struct {
	rows          []sessionRevocationOutbox
	deliveredID   uint64
	deliveredTo   string
	failedID      uint64
	failedUntil   time.Time
	failedMessage string
	markError     error
}

func (store *fakeRevocationOutboxStore) ready(context.Context, int) ([]sessionRevocationOutbox, error) {
	return append([]sessionRevocationOutbox(nil), store.rows...), nil
}

func (store *fakeRevocationOutboxStore) markDelivered(_ context.Context, id uint64, streamID string) error {
	if store.markError != nil {
		return store.markError
	}
	store.deliveredID = id
	store.deliveredTo = streamID
	return nil
}

func (store *fakeRevocationOutboxStore) markFailed(_ context.Context, id uint64, next time.Time, message string) error {
	if store.markError != nil {
		return store.markError
	}
	store.failedID = id
	store.failedUntil = next
	store.failedMessage = message
	return nil
}

func TestRevocationOutboxFlushPublishesVersionedEventAndConfirms(t *testing.T) {
	store := &fakeRevocationOutboxStore{rows: []sessionRevocationOutbox{{
		ID: 41, UserID: 29, RevokedAuthVersion: 7,
	}}}
	var published clusterEnvelope
	worker := &revocationOutboxWorker{
		store: store,
		append: func(_ context.Context, envelope clusterEnvelope) (string, error) {
			published = envelope
			return "1720000000000-0", nil
		},
	}

	delivered, err := worker.flush(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if delivered != 1 || store.deliveredID != 41 || store.deliveredTo != "1720000000000-0" {
		t.Fatalf("unexpected confirmation: delivered=%d store=%+v", delivered, store)
	}
	if published.EventID != 41 || published.Action != clusterActionDisconnect || published.RevokedAuthVersion != 7 || len(published.UserIDs) != 1 || published.UserIDs[0] != 29 {
		t.Fatalf("unexpected durable envelope: %+v", published)
	}
}

func TestRevocationOutboxFailureRemainsPendingWithBoundedBackoff(t *testing.T) {
	now := time.Date(2026, 8, 28, 8, 0, 0, 0, time.UTC)
	store := &fakeRevocationOutboxStore{rows: []sessionRevocationOutbox{{
		ID: 42, UserID: 30, RevokedAuthVersion: 8, AttemptCount: 2,
	}}}
	worker := &revocationOutboxWorker{
		store: store,
		append: func(context.Context, clusterEnvelope) (string, error) {
			return "", errors.New("redis unavailable")
		},
		now: func() time.Time { return now },
	}

	delivered, err := worker.flush(context.Background())
	if err == nil || delivered != 0 {
		t.Fatalf("flush = (%d, %v), want a retained failure", delivered, err)
	}
	if store.failedID != 42 || store.deliveredID != 0 || store.failedMessage != "redis unavailable" {
		t.Fatalf("failure receipt was not retained: %+v", store)
	}
	if want := now.Add(4 * time.Second); !store.failedUntil.Equal(want) {
		t.Fatalf("next attempt = %s, want %s", store.failedUntil, want)
	}
	if got := revocationRetryDelay(100); got != time.Minute {
		t.Fatalf("retry delay was not capped: %s", got)
	}
}

func TestRevocationOutboxReplaysWhenDatabaseConfirmationFails(t *testing.T) {
	store := &fakeRevocationOutboxStore{
		rows:      []sessionRevocationOutbox{{ID: 43, UserID: 31, RevokedAuthVersion: 9}},
		markError: errors.New("database confirmation failed"),
	}
	attempts := 0
	worker := &revocationOutboxWorker{
		store: store,
		append: func(context.Context, clusterEnvelope) (string, error) {
			attempts++
			return "1720000000001-0", nil
		},
	}
	if _, err := worker.flush(context.Background()); err == nil {
		t.Fatal("missing PostgreSQL confirmation was treated as delivered")
	}
	store.markError = nil
	if delivered, err := worker.flush(context.Background()); err != nil || delivered != 1 {
		t.Fatalf("at-least-once replay = (%d, %v)", delivered, err)
	}
	if attempts != 2 {
		t.Fatalf("XADD attempts = %d, want 2", attempts)
	}
}

func TestDecodeRevocationStreamMessageRequiresDurableIdentity(t *testing.T) {
	message := redis.XMessage{ID: "1720000000000-0", Values: map[string]any{
		"envelope": `{"origin":"node-a","event_id":51,"action":"disconnect","user_ids":[39],"revoked_auth_version":11}`,
	}}
	envelope, err := decodeRevocationStreamMessage(message)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.EventID != 51 || envelope.RevokedAuthVersion != 11 || envelope.UserIDs[0] != 39 {
		t.Fatalf("decoded envelope = %+v", envelope)
	}

	message.Values["envelope"] = `{"action":"disconnect","user_ids":[39]}`
	if _, err := decodeRevocationStreamMessage(message); err == nil {
		t.Fatal("unversioned stream event was accepted")
	}
}

func TestRevocationRetentionUsesTimeWindowBeyondDatabaseRevalidation(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 123000000, time.UTC)
	want := time.Date(2026, 8, 27, 12, 0, 0, 123000000, time.UTC)
	if got, expected := revocationStreamMinimumID(now), strconv.FormatInt(want.UnixMilli(), 10)+"-0"; got != expected {
		t.Fatalf("stream retention minimum = %q, want %q", got, expected)
	}
}
