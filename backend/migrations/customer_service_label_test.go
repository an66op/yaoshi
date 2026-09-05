package migrations

import (
	"strings"
	"testing"
)

func TestCustomerServiceLabelMigration(t *testing.T) {
	raw, err := migrationFiles.ReadFile("202609050002_customer_service_label.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{
		"ALTER COLUMN chat_nickname SET DEFAULT '客服'",
		"SET chat_nickname = '客服'",
		"IN ('', '群主')",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("customer-service label migration missing %q", fragment)
		}
	}
}
