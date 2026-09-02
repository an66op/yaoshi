package services

import (
	"backend/data/models/user"
	"reflect"
	"sort"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestTradingUserUpdateUsesOnlyTradingFieldWhitelist(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable"}), &gorm.Config{
		DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	input := UpdateUserTradingInput{FlyRate: 12, RebateRate: 3}
	for _, withExternal := range []bool{false, true} {
		var external *normalizedUserExternalFollow
		want := []string{"fly_mode", "fly_rate", "rebate_mode", "rebate_rate"}
		if withExternal {
			external = &normalizedUserExternalFollow{targetPlatform: "prepared", targetAccount: "account", singleLimitCents: 100}
			want = append(want, "fly_target_platform", "fly_target_account", "fly_endpoint_label", "fly_single_limit_cents", "fly_daily_limit_cents", "fly_connection_remark")
		}
		fields := tradingUserUpdateFields(input, "custom", "custom", external)
		got := make([]string, 0, len(fields))
		for key := range fields {
			got = append(got, key)
		}
		sort.Strings(got)
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected mutable trading columns: got=%v want=%v", got, want)
		}
		account := user.User{UserID: 42, BalanceCents: 99999, Password: "stale-password", AuthVersion: 5, Status: 1, WorkspaceID: 81}
		statement := db.Model(&account).Updates(fields).Statement
		for _, forbidden := range []string{"balance_cents", "password", "auth_version", "status", "workspace_id", "role", "parent_agent_id", "parent_tenant_id"} {
			if strings.Contains(statement.SQL.String(), `"`+forbidden+`"`) {
				t.Fatalf("trading update can overwrite %s: %s", forbidden, statement.SQL.String())
			}
		}
		where := strings.SplitN(statement.SQL.String(), " WHERE ", 2)
		if !strings.Contains(statement.SQL.String(), `UPDATE "user" SET`) || len(where) != 2 ||
			!strings.Contains(where[1], `"user_id" =`) || len(statement.Vars) == 0 || statement.Vars[len(statement.Vars)-1] != account.UserID {
			t.Fatalf("update is not scoped to the locked account: %s", statement.SQL.String())
		}
	}
}
