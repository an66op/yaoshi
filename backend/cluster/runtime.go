// Package cluster provides the small set of Redis primitives that must be
// shared by every backend instance: atomic rate windows, one-shot tickets,
// WebSocket fan-out and scheduler leases. Business data remains in PostgreSQL.
package cluster

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type Options struct {
	Addr     string
	Username string
	Password string
	DB       int
	TLS      bool
	Prefix   string
	Required bool
}

var (
	mu         sync.RWMutex
	client     *redis.Client
	prefix     = "wangzhe"
	instance   = randomToken()
	required   bool
	configured bool
)

func Init(ctx context.Context, options Options) error {
	mu.Lock()
	required = options.Required
	configured = strings.TrimSpace(options.Addr) != ""
	if value := normalizePrefix(options.Prefix); value != "" {
		prefix = value
	}
	mu.Unlock()
	addr := strings.TrimSpace(options.Addr)
	if addr == "" {
		if options.Required {
			disableClient()
			return errors.New("Redis is required but redis.addr is empty")
		}
		disableClient()
		return nil
	}
	settings := &redis.Options{
		Addr: addr, Username: options.Username, Password: options.Password, DB: options.DB,
		Protocol: 2, DisableIdentity: true,
		DialTimeout: 3 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second,
	}
	if options.TLS {
		settings.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	next := redis.NewClient(settings)
	pingCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	if err := next.Ping(pingCtx).Err(); err != nil {
		_ = next.Close()
		if options.Required {
			disableClient()
			return fmt.Errorf("Redis unavailable at %s: %w", addr, err)
		}
		disableClient()
		return nil
	}
	mu.Lock()
	old := client
	client = next
	mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return nil
}

func disableClient() {
	mu.Lock()
	old := client
	client = nil
	mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
}

func Close() error {
	mu.Lock()
	current := client
	client = nil
	mu.Unlock()
	if current != nil {
		return current.Close()
	}
	return nil
}

func Client() *redis.Client {
	mu.RLock()
	defer mu.RUnlock()
	return client
}

func Enabled() bool { return Client() != nil }

// Configured remains true after an optional connection failure or Close, so
// security controls cannot mistake an unavailable shared store for local mode.
func Configured() bool {
	mu.RLock()
	defer mu.RUnlock()
	return configured
}
func Required() bool {
	mu.RLock()
	defer mu.RUnlock()
	return required
}
func InstanceID() string { return instance }

func Key(parts ...string) string {
	mu.RLock()
	base := prefix
	mu.RUnlock()
	clean := make([]string, 0, len(parts)+1)
	clean = append(clean, base)
	for _, part := range parts {
		part = strings.Trim(strings.TrimSpace(part), ":")
		part = strings.ReplaceAll(part, " ", "_")
		if part != "" {
			clean = append(clean, part)
		}
	}
	return strings.Join(clean, ":")
}

func normalizePrefix(value string) string {
	value = strings.Trim(strings.TrimSpace(value), ":")
	value = strings.ReplaceAll(value, " ", "-")
	return value
}

var fixedWindowScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
local ttl = redis.call('PTTL', KEYS[1])
return {count, ttl}
`)

// AllowFixedWindow is atomic across processes. When Redis is not configured it
// returns (false, 0, ErrUnavailable), allowing callers to use their local
// development fallback without silently doing so in release mode.
func AllowFixedWindow(ctx context.Context, name, subject string, limit int, period time.Duration) (bool, time.Duration, error) {
	current := Client()
	if current == nil {
		return false, 0, ErrUnavailable
	}
	if limit < 1 || period <= 0 {
		return false, 0, errors.New("invalid rate-limit window")
	}
	result, err := fixedWindowScript.Run(ctx, current, []string{Key("rate", name, subject)}, period.Milliseconds()).Slice()
	if err != nil {
		return false, 0, err
	}
	if len(result) != 2 {
		return false, 0, errors.New("invalid Redis rate-limit response")
	}
	count, ok := result[0].(int64)
	if !ok {
		return false, 0, errors.New("invalid Redis rate-limit count")
	}
	ttlMS, _ := result[1].(int64)
	return count <= int64(limit), time.Duration(ttlMS) * time.Millisecond, nil
}

type Lease struct {
	key   string
	token string
	ttl   time.Duration
}

var releaseLeaseScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) end
return 0
`)

func AcquireLease(ctx context.Context, name string, ttl time.Duration) (*Lease, bool, error) {
	current := Client()
	if current == nil {
		return nil, false, ErrUnavailable
	}
	if ttl < time.Second {
		ttl = time.Second
	}
	lease := &Lease{key: Key("lock", name), token: instance + ":" + randomToken(), ttl: ttl}
	ok, err := current.SetNX(ctx, lease.key, lease.token, ttl).Result()
	return lease, ok, err
}

func (lease *Lease) Release(ctx context.Context) error {
	if lease == nil {
		return nil
	}
	current := Client()
	if current == nil {
		return ErrUnavailable
	}
	return releaseLeaseScript.Run(ctx, current, []string{lease.key}, lease.token).Err()
}

func Publish(ctx context.Context, channel string, payload []byte) error {
	current := Client()
	if current == nil {
		return ErrUnavailable
	}
	return current.Publish(ctx, Key("pubsub", channel), payload).Err()
}

func Subscribe(ctx context.Context, channel string) (*redis.PubSub, error) {
	current := Client()
	if current == nil {
		return nil, ErrUnavailable
	}
	subscription := current.Subscribe(ctx, Key("pubsub", channel))
	if _, err := subscription.Receive(ctx); err != nil {
		_ = subscription.Close()
		return nil, err
	}
	return subscription, nil
}

var ErrUnavailable = errors.New("Redis shared runtime unavailable")

func randomToken() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(bytes[:])
}
