package migrations

import (
	"strings"
	"testing"
)

func TestCheckInPointsCopyOnlyTargetsCheckInRecordsAndCopy(t *testing.T) {
	raw, err := migrationFiles.ReadFile("202609050005_checkin_points_copy.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{
		"WHERE type = 'checkin'",
		"WHERE title = '签到成功'",
		"'签到积分'",
		"'，获得 '",
		"' 积分'",
		"title = '连续签到七天送积分'",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("check-in points migration missing %q", fragment)
		}
	}
	for _, forbidden := range []string{"TRUNCATE", "DELETE FROM", "balance_cents =", "amount_cents ="} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("check-in copy migration must not alter financial data: %q", forbidden)
		}
	}
}
