package services

import (
	"strings"
	"testing"

	"gorm.io/gorm/clause"
)

func TestPasswordSessionUpdateIsAtomic(t *testing.T) {
	updates := passwordSessionUpdate("new-hash")
	if updates["password"] != "new-hash" {
		t.Fatalf("password update = %#v", updates["password"])
	}
	expression, ok := updates["auth_version"].(clause.Expr)
	if !ok {
		t.Fatalf("auth_version update type = %T, want clause.Expr", updates["auth_version"])
	}
	if expression.SQL != "auth_version + 1" {
		t.Fatalf("auth_version expression = %q", expression.SQL)
	}
	if len(updates) != 2 {
		t.Fatalf("password update has %d fields, want exactly 2", len(updates))
	}
}

func TestPasswordChangeCompareAndSwapIncludesOldHashAndSessionGeneration(t *testing.T) {
	db := dryRunMemberPaymentAccountDB(t)
	result := changePasswordCompareAndSwap(db, 91, "$old-hash", "$new-hash")
	sql := strings.ToLower(result.Statement.SQL.String())
	for _, fragment := range []string{"update", `"user"`, "user_id =", "password =", "auth_version + 1"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("password CAS omitted %q: %s", fragment, sql)
		}
	}
	if len(result.Statement.Vars) < 4 {
		t.Fatalf("password CAS vars = %#v", result.Statement.Vars)
	}
}
