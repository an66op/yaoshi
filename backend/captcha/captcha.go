// Package captcha issues short-lived, one-use image challenges for both login
// surfaces. Answers never leave this package; only their bound digest is kept.
package captcha

import (
	"backend/cluster"
	"backend/config"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	Management    = "management"
	Member        = "member"
	Lifetime      = 120 * time.Second
	localCapacity = 4096
)

var (
	ErrInvalid     = errors.New("验证码错误或已过期，请刷新后重试")
	ErrUnavailable = errors.New("验证码服务暂不可用，请稍后重试")
	local          = memoryStore{entries: make(map[string]entry), capacity: localCapacity}
)

type Challenge struct {
	ID        string `json:"id"`
	Image     string `json:"image"`
	ExpiresIn int    `json:"expires_in"`
}

type entry struct {
	digest  string
	expires time.Time
}
type memoryStore struct {
	mu       sync.Mutex
	entries  map[string]entry
	capacity int
}

func (s *memoryStore) put(id, digest string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, item := range s.entries {
		if !now.Before(item.expires) {
			delete(s.entries, key)
		}
	}
	if len(s.entries) >= s.capacity {
		return ErrUnavailable
	}
	s.entries[id] = entry{digest: digest, expires: now.Add(Lifetime)}
	return nil
}

func (s *memoryStore) consume(id string, now time.Time) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.entries[id]
	delete(s.entries, id)
	if !ok || !now.Before(item.expires) {
		return "", ErrInvalid
	}
	return item.digest, nil
}

// LocalFallbackAllowed is intentionally stricter than general optional Redis
// operations: only wholly unconfigured debug/test instances may use memory.
func LocalFallbackAllowed() bool {
	if cluster.Configured() || cluster.Required() || cluster.Client() != nil {
		return false
	}
	mode := gin.Mode()
	if cfg := config.Config; cfg != nil {
		if strings.TrimSpace(cfg.Redis.Addr) != "" {
			return false
		}
		if cfg.Server.Mode != "" {
			mode = cfg.Server.Mode
		}
	}
	return mode == gin.DebugMode || mode == gin.TestMode
}

func boundDigest(id, purpose, ip, code string) string {
	sum := sha256.Sum256([]byte(id + "\x00" + purpose + "\x00" + ip + "\x00" + code))
	return hex.EncodeToString(sum[:])
}

func validPurpose(purpose string) bool { return purpose == Management || purpose == Member }

func Create(ctx context.Context, purpose, ip string) (*Challenge, error) {
	if !validPurpose(purpose) || ip == "" {
		return nil, ErrInvalid
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return nil, ErrUnavailable
	}
	id := hex.EncodeToString(idBytes)
	digits := make([]byte, 6)
	for i := range digits {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return nil, ErrUnavailable
		}
		digits[i] = byte(n.Int64()) + '0'
	}
	image, err := renderPNG(string(digits))
	if err != nil {
		return nil, ErrUnavailable
	}
	digest := boundDigest(id, purpose, ip, string(digits))
	if current := cluster.Client(); current != nil {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		stored, err := current.SetNX(ctx, cluster.Key("captcha", id), digest, Lifetime).Result()
		if err != nil || !stored {
			return nil, ErrUnavailable
		}
	} else {
		if !LocalFallbackAllowed() {
			return nil, ErrUnavailable
		}
		if err := local.put(id, digest, time.Now()); err != nil {
			return nil, err
		}
	}
	return &Challenge{ID: id, Image: image, ExpiresIn: int(Lifetime / time.Second)}, nil
}

// Verify consumes before comparison, including wrong answers, wrong purpose
// and wrong IP. Redis GETDEL/memory mutex guarantee exactly one winning caller.
func Verify(ctx context.Context, purpose, ip, id, code string) error {
	if !validPurpose(purpose) || len(id) != 32 {
		return ErrInvalid
	}
	if _, err := hex.DecodeString(id); err != nil {
		return ErrInvalid
	}
	var expected string
	if current := cluster.Client(); current != nil {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		value, err := current.GetDel(ctx, cluster.Key("captcha", id)).Result()
		if errors.Is(err, redis.Nil) {
			return ErrInvalid
		}
		if err != nil {
			return ErrUnavailable
		}
		expected = value
	} else {
		if !LocalFallbackAllowed() {
			return ErrUnavailable
		}
		var err error
		expected, err = local.consume(id, time.Now())
		if err != nil {
			return err
		}
	}
	code = strings.TrimSpace(code)
	if ip == "" || len(code) != 6 {
		return ErrInvalid
	}
	for _, digit := range code {
		if digit < '0' || digit > '9' {
			return ErrInvalid
		}
	}
	actual := boundDigest(id, purpose, ip, code)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) != 1 {
		return ErrInvalid
	}
	return nil
}
