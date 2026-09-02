package captcha

import (
	"backend/cluster"
	"backend/config"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image/png"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func localTest(t *testing.T) {
	t.Helper()
	previous := config.Config
	config.Config = &config.Configuration{Server: config.ServerConfig{Mode: "test"}}
	if err := cluster.Init(context.Background(), cluster.Options{}); err != nil {
		t.Fatal(err)
	}
	local.mu.Lock()
	local.entries = make(map[string]entry)
	local.mu.Unlock()
	t.Cleanup(func() { _ = cluster.Init(context.Background(), cluster.Options{}); config.Config = previous })
}

func redisTest(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	localTest(t)
	server := miniredis.RunT(t)
	if err := cluster.Init(context.Background(), cluster.Options{Addr: server.Addr(), Prefix: "captcha-test"}); err != nil {
		t.Fatal(err)
	}
	return server
}

func TestChallengeOnlyExposesPNGAndOpaqueID(t *testing.T) {
	server := redisTest(t)
	challenge, err := Create(context.Background(), Management, "192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := hex.DecodeString(challenge.ID); err != nil || len(decoded) != 16 {
		t.Fatal("challenge ID must contain 128 random bits")
	}
	if challenge.ExpiresIn != 120 || server.TTL(cluster.Key("captcha", challenge.ID)) != Lifetime {
		t.Fatal("challenge lifetime is not two minutes")
	}
	payload, _ := json.Marshal(challenge)
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil || len(fields) != 3 || fields["id"] == nil || fields["image"] == nil || fields["expires_in"] == nil {
		t.Fatal("unexpected challenge fields expose server-only information")
	}
	if !strings.HasPrefix(challenge.Image, "data:image/png;base64,") {
		t.Fatal("captcha is not raster PNG")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(challenge.Image, "data:image/png;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(raw))
	if err != nil || decoded.Bounds().Dx() != 180 || decoded.Bounds().Dy() != 58 {
		t.Fatal("invalid captcha PNG dimensions")
	}
	stored, err := server.Get(cluster.Key("captcha", challenge.ID))
	if err != nil || len(stored) != 64 {
		t.Fatal("store must contain only a bound SHA256 digest")
	}
	if _, err := hex.DecodeString(stored); err != nil {
		t.Fatal("non-digest captcha record")
	}
	next, err := Create(context.Background(), Management, "192.0.2.1")
	if err != nil || next.ID == challenge.ID || next.Image == challenge.Image {
		t.Fatal("refresh reused the same challenge")
	}
}

func TestCaptchaConsumesEveryAttemptAndBindsPurposeAndIP(t *testing.T) {
	server := redisTest(t)
	const id, ip, code = "0123456789abcdef0123456789abcdef", "192.0.2.1", "012345"
	for _, tc := range []struct {
		name, purpose, ip, code string
		success                 bool
	}{
		{"correct", Management, ip, code, true},
		{"wrong answer", Management, ip, "999999", false},
		{"missing answer", Management, ip, "", false},
		{"malformed answer", Management, ip, "abcdef", false},
		{"different purpose", Member, ip, code, false},
		{"different IP", Management, "192.0.2.2", code, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server.Set(cluster.Key("captcha", id), boundDigest(id, Management, ip, code))
			server.SetTTL(cluster.Key("captcha", id), Lifetime)
			err := Verify(context.Background(), tc.purpose, tc.ip, id, tc.code)
			if tc.success && err != nil || !tc.success && !errors.Is(err, ErrInvalid) {
				t.Fatalf("unexpected verification result: %v", err)
			}
			if server.Exists(cluster.Key("captcha", id)) {
				t.Fatal("attempt did not consume challenge")
			}
			if err := Verify(context.Background(), Management, ip, id, code); !errors.Is(err, ErrInvalid) {
				t.Fatal("replayed consumed challenge")
			}
		})
	}
	server.Set(cluster.Key("captcha", id), boundDigest(id, Management, ip, code))
	server.SetTTL(cluster.Key("captcha", id), Lifetime)
	server.FastForward(Lifetime)
	if err := Verify(context.Background(), Management, ip, id, code); !errors.Is(err, ErrInvalid) {
		t.Fatal("expired challenge accepted")
	}
}

func TestCaptchaConcurrentRedisVerificationHasOneWinner(t *testing.T) {
	server := redisTest(t)
	const id, ip, code = "abcdef0123456789abcdef0123456789", "192.0.2.3", "654321"
	server.Set(cluster.Key("captcha", id), boundDigest(id, Member, ip, code))
	server.SetTTL(cluster.Key("captcha", id), Lifetime)
	var successes atomic.Int32
	var wait sync.WaitGroup
	for n := 0; n < 20; n++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := Verify(context.Background(), Member, ip, id, code)
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, ErrInvalid) {
				t.Errorf("unexpected verification error: %v", err)
			}
		}()
	}
	wait.Wait()
	if successes.Load() != 1 {
		t.Fatalf("verification winners=%d want1", successes.Load())
	}
}

func TestCaptchaFailsClosedForConfiguredOrRequiredRedisAndRelease(t *testing.T) {
	for _, scenario := range []string{"release without Redis", "configured address without client", "optional Redis init failure", "required without address", "connected Redis error", "closed Redis client"} {
		t.Run(scenario, func(t *testing.T) {
			localTest(t)
			switch scenario {
			case "release without Redis":
				config.Config.Server.Mode = "release"
			case "configured address without client":
				config.Config.Redis.Addr = "127.0.0.1:1"
			case "optional Redis init failure":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				_ = cluster.Init(ctx, cluster.Options{Addr: "127.0.0.1:1"})
				if !cluster.Configured() || cluster.Client() != nil {
					t.Fatal("configured-failure signal lost")
				}
			case "required without address":
				_ = cluster.Init(context.Background(), cluster.Options{Required: true})
			default:
				server := miniredis.RunT(t)
				if err := cluster.Init(context.Background(), cluster.Options{Addr: server.Addr(), Prefix: "captcha-failed"}); err != nil {
					t.Fatal(err)
				}
				if scenario == "connected Redis error" {
					server.SetError("ERR unavailable")
				} else {
					_ = cluster.Close()
				}
			}
			if LocalFallbackAllowed() {
				t.Fatal("insecure fallback allowed")
			}
			if _, err := Create(context.Background(), Member, "192.0.2.4"); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("create did not fail closed: %v", err)
			}
			if err := Verify(context.Background(), Member, "192.0.2.4", "0123456789abcdef0123456789abcdef", "123456"); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("verify did not fail closed: %v", err)
			}
			local.mu.Lock()
			count := len(local.entries)
			local.mu.Unlock()
			if count != 0 {
				t.Fatal("Redis failure created local challenges")
			}
		})
	}
}

func TestLocalCaptchaMemoryIsBoundedExpiresAndConsumes(t *testing.T) {
	localTest(t)
	store := memoryStore{entries: make(map[string]entry), capacity: 2}
	now := time.Now()
	if err := store.put("one", "digest", now); err != nil {
		t.Fatal(err)
	}
	if err := store.put("two", "digest", now); err != nil {
		t.Fatal(err)
	}
	if err := store.put("three", "digest", now); !errors.Is(err, ErrUnavailable) {
		t.Fatal("memory challenge cap bypassed")
	}
	if err := store.put("three", "digest", now.Add(Lifetime)); err != nil || len(store.entries) != 1 {
		t.Fatal("expired memory entries were not reclaimed")
	}
	if _, err := store.consume("three", now.Add(2*Lifetime)); !errors.Is(err, ErrInvalid) {
		t.Fatal("expired local captcha accepted")
	}
	const id, ip, code = "abcdef0123456789abcdef0123456789", "192.0.2.3", "654321"
	if err := local.put(id, boundDigest(id, Member, ip, code), time.Now()); err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wait sync.WaitGroup
	for n := 0; n < 20; n++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if Verify(context.Background(), Member, ip, id, code) == nil {
				successes.Add(1)
			}
		}()
	}
	wait.Wait()
	if successes.Load() != 1 {
		t.Fatal("local one-use verification is not atomic")
	}
}
