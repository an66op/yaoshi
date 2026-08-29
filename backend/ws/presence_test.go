package ws

import (
	"backend/cluster"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func startPresenceRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	server, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	if err := cluster.Init(context.Background(), cluster.Options{Addr: server.Addr(), Prefix: "presence-test"}); err != nil {
		server.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cluster.Close()
		server.Close()
	})
	return server
}

func TestRedisPresenceIsSharedAndTracksMultipleConnections(t *testing.T) {
	server := startPresenceRedis(t)
	owner := NewHub()
	remote := NewHub()
	first := &client{identity: SessionIdentity{UserID: 501, AuthVersion: 2, WorkspaceID: 8801}, send: make(chan []byte, 1)}
	second := &client{identity: SessionIdentity{UserID: 501, AuthVersion: 2, WorkspaceID: 8801}, send: make(chan []byte, 1)}

	owner.register(first)
	owner.register(second)
	if !remote.IsUserOnline(501) {
		t.Fatal("a different backend instance did not observe Redis presence")
	}
	if ttl := server.TTL(presenceKey(501)); ttl <= 0 || ttl > presenceKeyTTL {
		t.Fatalf("presence key TTL = %s, want a bounded crash-recovery TTL", ttl)
	}

	owner.unregister(first)
	if !remote.IsUserOnline(501) {
		t.Fatal("removing one socket incorrectly removed the user's other connection")
	}
	owner.unregister(second)
	if remote.IsUserOnline(501) {
		t.Fatal("remote presence remained after the final socket unregistered")
	}
}

func TestOnlineUsersRemovesExpiredCrashTokens(t *testing.T) {
	startPresenceRedis(t)
	key := presenceKey(611)
	if err := cluster.Client().ZAdd(context.Background(), key, redis.Z{
		Score:  float64(time.Now().Add(-time.Minute).UnixMilli()),
		Member: "crashed-instance:connection",
	}).Err(); err != nil {
		t.Fatal(err)
	}

	status := NewHub().OnlineUsers([]uint64{611})
	if status[611] {
		t.Fatal("expired crash token was reported online")
	}
	if count, err := cluster.Client().ZCard(context.Background(), key).Result(); err != nil || count != 0 {
		t.Fatalf("expired presence was not cleaned up, count=%d err=%v", count, err)
	}
}

func TestPresenceTouchCannotWinAfterUnregister(t *testing.T) {
	startPresenceRedis(t)
	owner := NewHub()
	remote := NewHub()
	for attempt := 0; attempt < 40; attempt++ {
		userID := uint64(700 + attempt)
		connection := &client{identity: SessionIdentity{UserID: userID, AuthVersion: 3, WorkspaceID: 8801}, send: make(chan []byte, 1)}
		owner.register(connection)

		start := make(chan struct{})
		var workers sync.WaitGroup
		workers.Add(2)
		go func() {
			defer workers.Done()
			<-start
			for refresh := 0; refresh < 4; refresh++ {
				owner.touchPresence(connection)
			}
		}()
		go func() {
			defer workers.Done()
			<-start
			owner.unregister(connection)
		}()
		close(start)
		workers.Wait()

		if remote.IsUserOnline(userID) {
			t.Fatalf("heartbeat re-added user %d after unregister completed", userID)
		}
	}
}

func TestOnlineUsersRedisFailureIsBoundedAndKeepsLocalTruth(t *testing.T) {
	server := startPresenceRedis(t)
	hub := NewHub()
	local := &client{identity: SessionIdentity{UserID: 901, AuthVersion: 1, WorkspaceID: 8801}, send: make(chan []byte, 1)}
	hub.register(local)
	server.Close()

	userIDs := make([]uint64, 0, 21)
	userIDs = append(userIDs, 901)
	for index := 0; index < 20; index++ {
		userIDs = append(userIDs, uint64(1000+index))
	}
	started := time.Now()
	status := hub.OnlineUsers(userIDs)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("one failed batch presence lookup took %s", elapsed)
	}
	if !status[901] {
		t.Fatal("Redis failure overrode exact local presence")
	}
	for _, userID := range userIDs[1:] {
		if status[userID] {
			t.Fatalf("user %d was reported online after the Redis lookup failed", userID)
		}
	}
}
