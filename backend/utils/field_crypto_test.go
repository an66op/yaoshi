package utils

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

const (
	oldFieldSecret = "old-test-data-encryption-key-with-32-bytes"
	newFieldSecret = "new-test-data-encryption-key-with-32-bytes"
)

func TestSensitiveFieldEncryptionUsesVersionedKeyIdentifier(t *testing.T) {
	if err := InitFieldEncryption(oldFieldSecret); err != nil {
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
	oldKey, err := buildSensitiveFieldKey(oldFieldSecret)
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := encryptedFieldV2Prefix + oldKey.id + ":"
	if first == second || !strings.HasPrefix(first, wantPrefix) || !IsSensitiveEncrypted(first) {
		t.Fatal("encryption must use a fresh nonce and bind the primary key identifier")
	}
	plain, err := DecryptSensitive(first)
	if err != nil || plain != "6222021234567890" {
		t.Fatalf("round trip = %q, %v", plain, err)
	}
	if preserved, err := EncryptSensitive(first); err != nil || preserved != first {
		t.Fatalf("validated envelope passthrough = %q, %v", preserved, err)
	}
	if legacy, err := DecryptSensitive("legacy-plaintext"); err != nil || legacy != "legacy-plaintext" {
		t.Fatalf("legacy plaintext passthrough = %q, %v", legacy, err)
	}
}

func TestSensitiveFieldEncryptionRotationAndLegacyV1Fallback(t *testing.T) {
	if err := InitFieldEncryption(oldFieldSecret); err != nil {
		t.Fatal(err)
	}
	oldV2, err := EncryptSensitive("provider-secret")
	if err != nil {
		t.Fatal(err)
	}
	oldV1 := encryptSensitiveV1ForTest(t, oldFieldSecret, "bank-account")

	if err := InitFieldEncryptionWithFallbacks(newFieldSecret, []string{oldFieldSecret}); err != nil {
		t.Fatal(err)
	}
	for name, test := range map[string]struct {
		value string
		want  string
		index int
	}{
		"identified v2": {value: oldV2, want: "provider-secret", index: 1},
		"legacy v1":     {value: oldV1, want: "bank-account", index: 1},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := DecryptSensitive(test.value)
			if err != nil || got != test.want {
				t.Fatalf("DecryptSensitive() = %q, %v; want %q", got, err, test.want)
			}
			inspection, err := InspectSensitiveEnvelope(test.value)
			if err != nil || inspection.PreviousKeyIndex != test.index {
				t.Fatalf("InspectSensitiveEnvelope() = %+v, %v", inspection, err)
			}
		})
	}

	newV2, err := EncryptSensitive("new-write")
	if err != nil {
		t.Fatal(err)
	}
	newKey, err := buildSensitiveFieldKey(newFieldSecret)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(newV2, encryptedFieldV2Prefix+newKey.id+":") {
		t.Fatal("new writes did not switch to the rotated primary key")
	}
	inspection, err := InspectSensitiveEnvelope(newV2)
	if err != nil || inspection.Version != "v2" || inspection.PreviousKeyIndex != 0 {
		t.Fatalf("primary envelope inspection = %+v, %v", inspection, err)
	}

	if err := InitFieldEncryption(oldFieldSecret); err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptSensitive(newV2); err == nil || err.Error() != "encrypted field key is unavailable" {
		t.Fatalf("old-only keyring accepted a new-key envelope: %v", err)
	}
	if plain, err := DecryptSensitive(oldV2); err != nil || plain != "provider-secret" {
		t.Fatalf("old key could not read its own v2 envelope: %q, %v", plain, err)
	}
}

func TestSensitiveEnvelopeInspectionNeverReturnsPlaintext(t *testing.T) {
	if err := InitFieldEncryption(newFieldSecret); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		value   string
		version string
	}{
		{value: "", version: "empty"},
		{value: "legacy-account-number", version: "plaintext"},
	} {
		inspection, err := InspectSensitiveEnvelope(test.value)
		if err != nil || inspection.Version != test.version || inspection.PreviousKeyIndex != 0 {
			t.Fatalf("inspection for %q = %+v, %v", test.version, inspection, err)
		}
	}
	if _, err := InspectSensitiveEnvelope("enc:v99:not-an-envelope"); !errors.Is(err, ErrSensitiveEnvelopeUnsupportedVersion) {
		t.Fatalf("unknown envelope error = %v", err)
	}
}

func TestReencryptSensitiveFromPreviousKeyMovesV1AndV2ToPrimary(t *testing.T) {
	if err := InitFieldEncryption(oldFieldSecret); err != nil {
		t.Fatal(err)
	}
	oldV1 := encryptSensitiveV1ForTest(t, oldFieldSecret, "enc:plaintext-may-use-marker")
	oldV2, err := EncryptSensitive("provider-secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := InitFieldEncryptionWithFallbacks(newFieldSecret, []string{oldFieldSecret}); err != nil {
		t.Fatal(err)
	}
	for _, oldEnvelope := range []string{oldV1, oldV2} {
		replacement, changed, err := ReencryptSensitiveFromPreviousKey(oldEnvelope, 1)
		if err != nil || !changed || replacement == oldEnvelope {
			t.Fatalf("rewrap=%q changed=%v err=%v", replacement, changed, err)
		}
		inspection, err := InspectSensitiveEnvelope(replacement)
		if err != nil || inspection.Version != "v2" || inspection.PreviousKeyIndex != 0 {
			t.Fatalf("replacement inspection=%+v err=%v", inspection, err)
		}
		plain, err := DecryptSensitive(replacement)
		if err != nil {
			t.Fatal(err)
		}
		if oldEnvelope == oldV1 && plain != "enc:plaintext-may-use-marker" {
			t.Fatalf("marker-prefixed plaintext changed: %q", plain)
		}
	}
	primary, err := EncryptSensitive("already primary")
	if err != nil {
		t.Fatal(err)
	}
	if replacement, changed, err := ReencryptSensitiveFromPreviousKey(primary, 1); err != nil || changed || replacement != "" {
		t.Fatalf("primary envelope was rewrapped: %q %v %v", replacement, changed, err)
	}
}

func TestSensitiveFieldEncryptionRejectsWrongKeyAndTampering(t *testing.T) {
	legacyV1 := encryptSensitiveV1ForTest(t, oldFieldSecret, "legacy-secret")
	if err := InitFieldEncryption(newFieldSecret); err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptSensitive(legacyV1); err == nil || err.Error() != "encrypted field authentication failed" {
		t.Fatalf("wrong key accepted legacy v1 ciphertext: %v", err)
	}

	value, err := EncryptSensitive("authenticated")
	if err != nil {
		t.Fatal(err)
	}
	lastColon := strings.LastIndex(value, ":")
	payload, err := base64.RawStdEncoding.DecodeString(value[lastColon+1:])
	if err != nil {
		t.Fatal(err)
	}
	payload[len(payload)-1] ^= 0x01
	tampered := value[:lastColon+1] + base64.RawStdEncoding.EncodeToString(payload)
	if _, err := DecryptSensitive(tampered); err == nil || err.Error() != "encrypted field authentication failed" {
		t.Fatalf("tampered ciphertext was accepted: %v", err)
	}
	if _, err := EncryptSensitive("enc:v99:anything"); err == nil || err.Error() != "encrypted field version is unsupported" {
		t.Fatalf("unknown envelope was accepted or double-encrypted: %v", err)
	}
}

func TestSensitiveFieldEncryptionRejectsInvalidRotationWithoutReplacingKeyring(t *testing.T) {
	if err := InitFieldEncryption(newFieldSecret); err != nil {
		t.Fatal(err)
	}
	before, err := EncryptSensitive("still-readable")
	if err != nil {
		t.Fatal(err)
	}
	if err := InitFieldEncryptionWithFallbacks(oldFieldSecret, []string{oldFieldSecret}); err == nil {
		t.Fatal("duplicate rotation key was accepted")
	}
	if got, err := DecryptSensitive(before); err != nil || got != "still-readable" {
		t.Fatalf("rejected rotation replaced the working keyring: %q, %v", got, err)
	}
	tooMany := make([]string, maxPreviousFieldKeys+1)
	for index := range tooMany {
		tooMany[index] = oldFieldSecret + string(rune('a'+index))
	}
	if err := InitFieldEncryptionWithFallbacks(oldFieldSecret, tooMany); err == nil {
		t.Fatal("oversized rotation keyring was accepted")
	}
}

func encryptSensitiveV1ForTest(t *testing.T, secret, plaintext string) string {
	t.Helper()
	key, err := buildSensitiveFieldKey(secret)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, key.aead.NonceSize())
	payload := key.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return encryptedFieldV1Prefix + base64.RawStdEncoding.EncodeToString(payload)
}
