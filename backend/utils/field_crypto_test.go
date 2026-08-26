package utils

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestSensitiveFieldEncryption(t *testing.T) {
	if err := InitFieldEncryption("test-data-encryption-key-with-32-bytes"); err != nil {
		t.Fatal(err)
	}
	first, err := EncryptSensitive("6222021234567890")
	if err != nil {
		t.Fatal(err)
	}
	second, err := EncryptSensitive("6222021234567890")
	if err != nil {
		t.Fatal(err)
	}
	if first == second || !IsSensitiveEncrypted(first) {
		t.Fatal("encryption must use a fresh nonce and include the version prefix")
	}
	plain, err := DecryptSensitive(first)
	if err != nil || plain != "6222021234567890" {
		t.Fatalf("round trip = %q, %v", plain, err)
	}
	if legacy, err := DecryptSensitive("legacy-plaintext"); err != nil || legacy != "legacy-plaintext" {
		t.Fatalf("legacy passthrough = %q, %v", legacy, err)
	}

	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(first, encryptedFieldPrefix))
	if err != nil {
		t.Fatal(err)
	}
	payload[len(payload)-1] ^= 0x01
	tampered := encryptedFieldPrefix + base64.RawStdEncoding.EncodeToString(payload)
	if _, err := DecryptSensitive(tampered); err == nil {
		t.Fatal("tampered ciphertext was accepted")
	}
}
