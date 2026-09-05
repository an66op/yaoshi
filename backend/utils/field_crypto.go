package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
)

const (
	encryptedFieldMarker   = "enc:"
	encryptedFieldV1Prefix = "enc:v1:"
	encryptedFieldV2Prefix = "enc:v2:"
	fieldKeyIDBytes        = 16
	maxPreviousFieldKeys   = 8
)

var fieldKeyIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

var (
	ErrSensitiveEnvelopeUnsupportedVersion = errors.New("encrypted field version is unsupported")
	ErrSensitiveEnvelopeMalformed          = errors.New("encrypted field is malformed")
	ErrSensitiveEnvelopeTruncated          = errors.New("encrypted field is truncated")
	ErrSensitiveEnvelopeAuthentication     = errors.New("encrypted field authentication failed")
	ErrSensitiveEnvelopeKeyUnavailable     = errors.New("encrypted field key is unavailable")
)

type sensitiveFieldKey struct {
	id               string
	aead             cipher.AEAD
	previousKeyIndex int
}

var fieldCipher struct {
	sync.RWMutex
	primary sensitiveFieldKey
	byID    map[string]sensitiveFieldKey
	legacy  []sensitiveFieldKey
}

// SensitiveEnvelopeInspection contains only non-secret classification data.
// It never returns the plaintext, ciphertext, key identifier or key material.
// PreviousKeyIndex is one-based and zero means that the primary key was used.
type SensitiveEnvelopeInspection struct {
	Version          string
	PreviousKeyIndex int
}

// InitFieldEncryption configures a single-key keyring. It remains as the
// compatibility entrypoint for tests and callers that do not need a rotation
// window; production startup should pass its retained previous keys to
// InitFieldEncryptionWithFallbacks.
func InitFieldEncryption(secret string) error {
	return InitFieldEncryptionWithFallbacks(secret, nil)
}

// InitFieldEncryptionWithFallbacks configures authenticated encryption for
// database fields. New values carry a non-secret identifier derived from the
// primary key and use the v2 envelope. Retained previous keys decrypt both v2
// values carrying their identifier and identifier-less v1 values. It does not
// rewrite any database row.
//
// The whole keyring is built before it is swapped into use, so a rejected
// rotation cannot partially replace a working process keyring.
func InitFieldEncryptionWithFallbacks(primarySecret string, previousSecrets []string) error {
	if len(previousSecrets) > maxPreviousFieldKeys {
		return fmt.Errorf("data encryption keyring has more than %d previous keys", maxPreviousFieldKeys)
	}
	secrets := make([]string, 0, 1+len(previousSecrets))
	secrets = append(secrets, primarySecret)
	secrets = append(secrets, previousSecrets...)

	keys := make([]sensitiveFieldKey, 0, len(secrets))
	byID := make(map[string]sensitiveFieldKey, len(secrets))
	for index, secret := range secrets {
		if strings.TrimSpace(secret) == "" {
			if index == 0 {
				return errors.New("data encryption key is empty")
			}
			return fmt.Errorf("previous data encryption key %d is empty", index)
		}
		key, err := buildSensitiveFieldKey(secret)
		if err != nil {
			return err
		}
		if _, exists := byID[key.id]; exists {
			return fmt.Errorf("data encryption keyring contains a duplicate key at position %d", index+1)
		}
		key.previousKeyIndex = index
		keys = append(keys, key)
		byID[key.id] = key
	}

	legacy := make([]sensitiveFieldKey, 0, len(keys))
	for _, key := range keys {
		legacy = append(legacy, key)
	}
	fieldCipher.Lock()
	fieldCipher.primary = keys[0]
	fieldCipher.byID = byID
	fieldCipher.legacy = legacy
	fieldCipher.Unlock()
	return nil
}

func buildSensitiveFieldKey(secret string) (sensitiveFieldKey, error) {
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return sensitiveFieldKey{}, fmt.Errorf("initialize field cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return sensitiveFieldKey{}, fmt.Errorf("initialize field GCM: %w", err)
	}
	return sensitiveFieldKey{
		id:   hex.EncodeToString(key[:fieldKeyIDBytes]),
		aead: aead,
	}, nil
}

func IsSensitiveEncrypted(value string) bool {
	return strings.HasPrefix(value, encryptedFieldMarker)
}

func EncryptSensitive(value string) (string, error) {
	if value == "" {
		return value, nil
	}
	if IsSensitiveEncrypted(value) {
		// Never double-encrypt an envelope, but also never accept an unknown,
		// malformed or no-longer-decryptable envelope as plaintext.
		if _, err := DecryptSensitive(value); err != nil {
			return "", err
		}
		return value, nil
	}
	return encryptSensitivePlaintext(value)
}

// ReencryptSensitiveFromPreviousKey authenticates and decrypts an existing
// envelope only when it depends on the requested one-based previous-key slot,
// then emits a fresh v2 envelope under the primary key. Neither plaintext nor
// either envelope is logged or returned to operators; the returned ciphertext
// is intended only for a compare-and-swap database update.
func ReencryptSensitiveFromPreviousKey(value string, previousKeyIndex int) (string, bool, error) {
	if previousKeyIndex <= 0 {
		return "", false, errors.New("previous data encryption key position must be positive")
	}
	plaintext, inspection, err := inspectAndDecryptSensitive(value)
	if err != nil {
		return "", false, err
	}
	if (inspection.Version != "v1" && inspection.Version != "v2") || inspection.PreviousKeyIndex != previousKeyIndex {
		return "", false, nil
	}
	replacement, err := encryptSensitivePlaintext(plaintext)
	if err != nil {
		return "", false, err
	}
	return replacement, true, nil
}

func encryptSensitivePlaintext(value string) (string, error) {
	fieldCipher.RLock()
	primary := fieldCipher.primary
	fieldCipher.RUnlock()
	if primary.aead == nil {
		return "", errors.New("field encryption is not initialized")
	}
	header := encryptedFieldV2Prefix + primary.id + ":"
	nonce := make([]byte, primary.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate encryption nonce: %w", err)
	}
	payload := primary.aead.Seal(nonce, nonce, []byte(value), []byte(header))
	return header + base64.RawStdEncoding.EncodeToString(payload), nil
}

func DecryptSensitive(value string) (string, error) {
	plaintext, _, err := inspectAndDecryptSensitive(value)
	return plaintext, err
}

// InspectSensitiveEnvelope authenticates one stored value without returning
// its plaintext. Production inventory and rollback gates use this to prove
// that every encrypted database field is readable by the configured keyring.
func InspectSensitiveEnvelope(value string) (SensitiveEnvelopeInspection, error) {
	_, inspection, err := inspectAndDecryptSensitive(value)
	return inspection, err
}

func inspectAndDecryptSensitive(value string) (string, SensitiveEnvelopeInspection, error) {
	if value == "" {
		return value, SensitiveEnvelopeInspection{Version: "empty"}, nil
	}
	if !IsSensitiveEncrypted(value) {
		// Runtime compatibility still permits historic plaintext. The production
		// inventory gate classifies it separately and refuses to admit traffic.
		return value, SensitiveEnvelopeInspection{Version: "plaintext"}, nil
	}
	if strings.HasPrefix(value, encryptedFieldV1Prefix) {
		return decryptSensitiveV1(strings.TrimPrefix(value, encryptedFieldV1Prefix))
	}
	if strings.HasPrefix(value, encryptedFieldV2Prefix) {
		return decryptSensitiveV2(value)
	}
	return "", SensitiveEnvelopeInspection{}, ErrSensitiveEnvelopeUnsupportedVersion
}

func decryptSensitiveV1(encoded string) (string, SensitiveEnvelopeInspection, error) {
	fieldCipher.RLock()
	legacy := append([]sensitiveFieldKey(nil), fieldCipher.legacy...)
	fieldCipher.RUnlock()
	if len(legacy) == 0 {
		return "", SensitiveEnvelopeInspection{}, errors.New("field encryption is not initialized")
	}
	payload, err := decodeSensitivePayload(encoded)
	if err != nil {
		return "", SensitiveEnvelopeInspection{}, err
	}
	for _, key := range legacy {
		if len(payload) < key.aead.NonceSize() {
			return "", SensitiveEnvelopeInspection{}, ErrSensitiveEnvelopeTruncated
		}
		nonce, ciphertext := payload[:key.aead.NonceSize()], payload[key.aead.NonceSize():]
		plaintext, openErr := key.aead.Open(nil, nonce, ciphertext, nil)
		if openErr == nil {
			return string(plaintext), SensitiveEnvelopeInspection{
				Version: "v1", PreviousKeyIndex: key.previousKeyIndex,
			}, nil
		}
	}
	return "", SensitiveEnvelopeInspection{}, ErrSensitiveEnvelopeAuthentication
}

func decryptSensitiveV2(value string) (string, SensitiveEnvelopeInspection, error) {
	remainder := strings.TrimPrefix(value, encryptedFieldV2Prefix)
	keyID, encoded, found := strings.Cut(remainder, ":")
	if !found || !fieldKeyIDPattern.MatchString(keyID) {
		return "", SensitiveEnvelopeInspection{}, ErrSensitiveEnvelopeMalformed
	}
	header := encryptedFieldV2Prefix + keyID + ":"
	fieldCipher.RLock()
	key, exists := fieldCipher.byID[keyID]
	fieldCipher.RUnlock()
	if !exists || key.aead == nil {
		return "", SensitiveEnvelopeInspection{}, ErrSensitiveEnvelopeKeyUnavailable
	}
	payload, err := decodeSensitivePayload(encoded)
	if err != nil {
		return "", SensitiveEnvelopeInspection{}, err
	}
	if len(payload) < key.aead.NonceSize() {
		return "", SensitiveEnvelopeInspection{}, ErrSensitiveEnvelopeTruncated
	}
	nonce, ciphertext := payload[:key.aead.NonceSize()], payload[key.aead.NonceSize():]
	plaintext, err := key.aead.Open(nil, nonce, ciphertext, []byte(header))
	if err != nil {
		return "", SensitiveEnvelopeInspection{}, ErrSensitiveEnvelopeAuthentication
	}
	return string(plaintext), SensitiveEnvelopeInspection{
		Version: "v2", PreviousKeyIndex: key.previousKeyIndex,
	}, nil
}

func decodeSensitivePayload(encoded string) ([]byte, error) {
	payload, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, ErrSensitiveEnvelopeMalformed
	}
	return payload, nil
}
