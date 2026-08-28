package services

import (
	"backend/data/models/user"
	"backend/data/models/workspace"
	"math"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestResolveEffectiveOddsPrecedenceAndMemberMultiplier(t *testing.T) {
	tests := []struct {
		name                             string
		user, room, platform, multiplier float64
		want                             float64
		wantSource                       string
	}{
		{name: "exact user odds stay authoritative", user: 2.25, room: 2, platform: 1.99, multiplier: 1.2, want: 2.25, wantSource: "user"},
		{name: "room odds use member multiplier", room: 2, platform: 1.99, multiplier: .8, want: 1.6, wantSource: "member_multiplier_room"},
		{name: "platform odds use member multiplier", platform: 1.99, multiplier: 1.2, want: 2.388, wantSource: "member_multiplier_platform"},
		{name: "one keeps room inheritance", room: 1.99, platform: 1.98, multiplier: 1, want: 1.99, wantSource: "room"},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			got, source := resolveEffectiveOdds(item.user, item.room, item.platform, item.multiplier)
			if math.Abs(got-item.want) > .00001 || source != item.wantSource {
				t.Fatalf("resolveEffectiveOdds() = %.4f/%q, want %.4f/%q", got, source, item.want, item.wantSource)
			}
		})
	}
}

func TestMembershipMultiplierChangesWithWorkspace(t *testing.T) {
	roomOdds := 2.0
	roomAMultiplier := .8
	roomBMultiplier := 1.2
	roomA, _ := resolveEffectiveOdds(0, roomOdds, 0, roomAMultiplier)
	roomB, _ := resolveEffectiveOdds(0, roomOdds, 0, roomBMultiplier)
	if roomA != 1.6 || roomB != 2.4 {
		t.Fatalf("workspace-specific results = %.2f and %.2f, want 1.60 and 2.40", roomA, roomB)
	}
}

func TestScopedMembershipOddsQueryCannotReadAnotherRoom(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable"}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	var membership workspace.Membership
	statement := scopedMembershipOddsQuery(db.Model(&workspace.Membership{}), 81, 92).First(&membership).Statement
	if sql := statement.SQL.String(); !strings.Contains(sql, "workspace_id =") || !strings.Contains(sql, "user_id =") || !strings.Contains(sql, "status =") {
		t.Fatalf("membership multiplier query is not fully scoped: %q", sql)
	}
	want := []any{uint64(81), uint64(92), 1}
	if len(statement.Vars) < len(want) {
		t.Fatalf("membership query vars = %#v, want workspace/user/status", statement.Vars)
	}
	for index := range want {
		if statement.Vars[index] != want[index] {
			t.Fatalf("membership query var %d = %#v, want %#v", index, statement.Vars[index], want[index])
		}
	}
}

func TestResolveForAccountUsesProvidedLockedWorkspace(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable"}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	type observedQuery struct {
		table string
		vars  []any
	}
	queries := make([]observedQuery, 0, 4)
	if err := db.Callback().Query().After("gorm:query").Register("test:observe_locked_account_scope", func(tx *gorm.DB) {
		queries = append(queries, observedQuery{table: tx.Statement.Table, vars: append([]any(nil), tx.Statement.Vars...)})
	}); err != nil {
		t.Fatal(err)
	}

	account := user.User{UserID: 92, WorkspaceID: 81}
	_, _ = NewTradingAdminService(db).ResolveForAccount(account, "speed-racing", "ball_1_5", 10, 0, 0)

	var scopedOddsQuery *observedQuery
	for index := range queries {
		query := &queries[index]
		if query.table == "users" {
			t.Fatal("ResolveForAccount reloaded the user instead of using the transaction-locked account")
		}
		if query.table == "user_play_odds" {
			scopedOddsQuery = query
		}
	}
	if scopedOddsQuery == nil {
		t.Fatalf("user odds query was not executed: %#v", queries)
	}
	wantPrefix := []any{uint64(81), uint64(92), "speed-racing", "ball_1_5"}
	if len(scopedOddsQuery.vars) < len(wantPrefix) {
		t.Fatalf("user odds query vars = %#v, want locked workspace/user scope", scopedOddsQuery.vars)
	}
	for index := range wantPrefix {
		if scopedOddsQuery.vars[index] != wantPrefix[index] {
			t.Fatalf("user odds query var %d = %#v, want %#v", index, scopedOddsQuery.vars[index], wantPrefix[index])
		}
	}
}

func TestValidateOddsMultiplierBounds(t *testing.T) {
	for _, value := range []float64{.5, .8, 1, 1.2, 1.5} {
		if err := validateOddsMultiplier(value); err != nil {
			t.Fatalf("validateOddsMultiplier(%v): %v", value, err)
		}
	}
	for _, value := range []float64{.4999, 1.5001, math.NaN(), math.Inf(1)} {
		if err := validateOddsMultiplier(value); err == nil {
			t.Fatalf("validateOddsMultiplier(%v) unexpectedly succeeded", value)
		}
	}
}
