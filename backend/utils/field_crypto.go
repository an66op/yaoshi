package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

const encryptedFieldPrefix = "enc:v1:"

var fieldCipher struct {
	sync.RWMutex
	aead cipher.AEAD
}

// InitFieldEncryption configures authenticated encryption for database fields
// that must not be useful to somebody who obtains a raw database dump. A
// SHA-256 KDF provides a fixed AES-256 key while deployment validation still
// requires a high-entropy source secret in release mode.
func InitFieldEncryption(secret string) error {
	if strings.TrimSpace(secret) == "" {
		return errors.New("data encryption key is empty")
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return fmt.Errorf("initialize field cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("initialize field GCM: %w", err)
	}
	fieldCipher.Lock()
	fieldCipher.aead = aead
	fieldCipher.Unlock()
	return nil
}

func IsSensitiveEncrypted(value string) bool {
	return strings.HasPrefix(value, encryptedFieldPrefix)
}

func EncryptSensitive(value string) (string, error) {
	if value == "" || IsSensitiveEncrypted(value) {
		return value, nil
	}
	fieldCipher.RLock()
	aead := fieldCipher.aead
	fieldCipher.RUnlock()
	if aead == nil {
		return "", errors.New("field encryption is not initialized")
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate encryption nonce: %w", err)
	}
	payload := aead.Seal(nonce, nonce, []byte(value), nil)
	return encryptedFieldPrefix + base64.RawStdEncoding.EncodeToString(payload), nil
}

func DecryptSensitive(value string) (string, error) {
	if value == "" || !IsSensitiveEncrypted(value) {
		// Legacy plaintext is accepted only long enough for the startup migration
		// to encrypt it. This also makes rolling upgrades non-destructive.
		return value, nil
	}
	fieldCipher.RLock()
	aead := fieldCipher.aead
	fieldCipher.RUnlock()
	if aead == nil {
		return "", errors.New("field encryption is not initialized")
	}
	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, encryptedFieldPrefix))
	if err != nil {
		return "", errors.New("encrypted field is malformed")
	}
	if len(payload) < aead.NonceSize() {
		return "", errors.New("encrypted field is truncated")
	}
	nonce, ciphertext := payload[:aead.NonceSize()], payload[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", errors.New("encrypted field authentication failed")
	}
	return string(plaintext), nil
}
