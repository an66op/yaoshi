package services

import (
	"backend/utils"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
)

const (
	sensitiveOldTestKey = "old-sensitive-readiness-key-with-32-bytes"
	sensitiveNewTestKey = "new-sensitive-readiness-key-with-32-bytes"
)

func TestSensitiveFieldInventoryCoversEveryEncryptedDatabaseColumn(t *testing.T) {
	want := []sensitiveFieldColumn{
		{
			name:           "member_payment_account_number",
			inventoryQuery: `SELECT "account_no" FROM "member_payment_accounts"`,
			batchQuery:     `SELECT "id", "account_no" FROM "member_payment_accounts" WHERE "id" > ? ORDER BY "id" ASC LIMIT ?`,
			casUpdateQuery: `UPDATE "member_payment_accounts" SET "account_no" = ? WHERE "id" = ? AND "account_no" = ?`,
		},
		{
			name:           "wallet_payment_channel_secret",
			inventoryQuery: `SELECT "secret_key" FROM "wallet_payment_channels"`,
			batchQuery:     `SELECT "id", "secret_key" FROM "wallet_payment_channels" WHERE "id" > ? ORDER BY "id" ASC LIMIT ?`,
			casUpdateQuery: `UPDATE "wallet_payment_channels" SET "secret_key" = ? WHERE "id" = ? AND "secret_key" = ?`,
		},
		{
			name:           "entertainment_platform_secret",
			inventoryQuery: `SELECT "secret_key" FROM "entertainment_platforms"`,
			batchQuery:     `SELECT "id", "secret_key" FROM "entertainment_platforms" WHERE "id" > ? ORDER BY "id" ASC LIMIT ?`,
			casUpdateQuery: `UPDATE "entertainment_platforms" SET "secret_key" = ? WHERE "id" = ? AND "secret_key" = ?`,
		},
	}
	if !reflect.DeepEqual(sensitiveFieldColumns, want) {
		t.Fatalf("encrypted-column inventory drifted: %+v", sensitiveFieldColumns)
	}
}

func TestSensitiveFieldReadinessCountsOnlyAuthenticatedAggregateMetadata(t *testing.T) {
	if err := utils.InitFieldEncryption(sensitiveOldTestKey); err != nil {
		t.Fatal(err)
	}
	oldV2, err := utils.EncryptSensitive("old provider secret")
	if err != nil {
		t.Fatal(err)
	}
	oldV1 := sensitiveV1ForServiceTest(t, sensitiveOldTestKey, "old account number")
	if err := utils.InitFieldEncryptionWithFallbacks(sensitiveNewTestKey, []string{sensitiveOldTestKey}); err != nil {
		t.Fatal(err)
	}
	newV2, err := utils.EncryptSensitive("new provider secret")
	if err != nil {
		t.Fatal(err)
	}

	report := newSensitiveFieldReadinessReport()
	inspectSensitiveFieldValue(report, 0, oldV1)
	inspectSensitiveFieldValue(report, 0, "")
	inspectSensitiveFieldValue(report, 1, oldV2)
	inspectSensitiveFieldValue(report, 2, newV2)
	finalizeSensitiveFieldReadiness(report)

	if !report.Complete || report.AuditedColumns != 3 || report.Counts.Total != 4 ||
		report.Counts.Empty != 1 || report.Counts.V1 != 1 || report.Counts.V2 != 2 ||
		report.Counts.PrimaryKey != 1 || report.Counts.PreviousKey != 2 || report.Counts.Invalid != 0 {
		t.Fatalf("unexpected aggregate report: %+v", report)
	}
	wantDependency := []SensitivePreviousKeyDependency{{PreviousKeyIndex: 1, Total: 2, V1: 1, V2: 1}}
	if !reflect.DeepEqual(report.PreviousKeyDependencies, wantDependency) {
		t.Fatalf("previous-key dependencies=%+v want=%+v", report.PreviousKeyDependencies, wantDependency)
	}
	if report.Columns[0].Counts.V1 != 1 || report.Columns[1].Counts.V2 != 1 || report.Columns[2].Counts.PrimaryKey != 1 {
		t.Fatalf("column counts were not isolated: %+v", report.Columns)
	}
}

func TestSensitiveFieldReadinessFailsClosedForPlaintextUnknownKeyAndTampering(t *testing.T) {
	if err := utils.InitFieldEncryptionWithFallbacks(sensitiveNewTestKey, []string{sensitiveOldTestKey}); err != nil {
		t.Fatal(err)
	}

	report := newSensitiveFieldReadinessReport()
	for _, stored := range []string{
		"historic plaintext",
		"enc:v99:future",
		"enc:v2:not-a-key-id:payload",
		"enc:v1:AQ",
		"enc:v2:00000000000000000000000000000000:AQ",
	} {
		inspectSensitiveFieldValue(report, 0, stored)
	}
	valid, err := utils.EncryptSensitive("authenticated")
	if err != nil {
		t.Fatal(err)
	}
	lastColon := strings.LastIndex(valid, ":")
	payload, err := base64.RawStdEncoding.DecodeString(valid[lastColon+1:])
	if err != nil {
		t.Fatal(err)
	}
	payload[len(payload)-1] ^= 0x01
	inspectSensitiveFieldValue(report, 0, valid[:lastColon+1]+base64.RawStdEncoding.EncodeToString(payload))
	finalizeSensitiveFieldReadiness(report)

	counts := report.Counts
	if report.Complete || counts.Total != 6 || counts.Plaintext != 1 || counts.Invalid != 5 ||
		counts.UnsupportedVersion != 1 || counts.Malformed != 1 || counts.Truncated != 1 ||
		counts.KeyUnavailable != 1 || counts.AuthenticationFailed != 1 || counts.OtherFailure != 0 {
		t.Fatalf("unsafe values were not classified fail-closed: %+v", report)
	}
	if compatibility := AssessSensitiveFieldCompatibility(report, SensitiveEnvelopeReadCapabilities{ReadVersions: []int{1, 2}, SupportsPreviousKeyring: true}); compatibility.Compatible {
		t.Fatalf("incomplete inventory was accepted: %+v", compatibility)
	}
}

func TestSensitiveFieldCompatibilityCoversOldKeyV1AndNewWriteV2(t *testing.T) {
	report := &SensitiveFieldReadinessReport{
		Complete:                true,
		Counts:                  SensitiveEnvelopeCounts{Total: 2, V1: 1, V2: 1, PrimaryKey: 1, PreviousKey: 1},
		PreviousKeyDependencies: []SensitivePreviousKeyDependency{{PreviousKeyIndex: 1, Total: 1, V1: 1}},
	}
	for _, test := range []struct {
		name string
		caps SensitiveEnvelopeReadCapabilities
		want []string
	}{
		{name: "fully compatible", caps: SensitiveEnvelopeReadCapabilities{ReadVersions: []int{1, 2}, SupportsPreviousKeyring: true}},
		{name: "old binary cannot read new writes", caps: SensitiveEnvelopeReadCapabilities{ReadVersions: []int{1}}, want: []string{"v2_not_readable", "previous_key_not_readable"}},
		{name: "no v1 reader", caps: SensitiveEnvelopeReadCapabilities{ReadVersions: []int{2}, SupportsPreviousKeyring: true}, want: []string{"v1_not_readable"}},
		{name: "no rotation fallback", caps: SensitiveEnvelopeReadCapabilities{ReadVersions: []int{1, 2}}, want: []string{"previous_key_not_readable"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := AssessSensitiveFieldCompatibility(report, test.caps)
			if got.Compatible != (len(test.want) == 0) || !reflect.DeepEqual(got.Reasons, test.want) {
				t.Fatalf("compatibility=%+v want reasons=%v", got, test.want)
			}
		})
	}
	if SensitivePreviousKeySlotUnused(report, 1) || !SensitivePreviousKeySlotUnused(report, 2) || SensitivePreviousKeySlotUnused(nil, 1) {
		t.Fatal("previous-key removal gate returned the wrong result")
	}
}

func sensitiveV1ForServiceTest(t *testing.T, secret, plaintext string) string {
	t.Helper()
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, aead.NonceSize())
	payload := aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return "enc:v1:" + base64.RawStdEncoding.EncodeToString(payload)
}
