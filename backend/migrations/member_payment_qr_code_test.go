package migrations

import (
	"strings"
	"testing"
)

func TestMemberPaymentQRCodeMigrationIsNullableAndConstrainsServerFilename(t *testing.T) {
	raw, err := migrationFiles.ReadFile("202609050003_member_payment_qr_code.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{
		"ADD COLUMN IF NOT EXISTS qr_code_file character varying(64)",
		"qr_code_file IS NULL",
		"^[0-9a-f]{32}\\.png$",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("member payment QR migration missing %q", fragment)
		}
	}
}
