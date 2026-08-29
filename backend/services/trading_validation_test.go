package services

import (
	"math"
	"strings"
	"testing"

	"backend/data/models/user"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestTradingNumericValidationRejectsNonFiniteValues(t *testing.T) {
	for _, value := range []float64{0, 25.5, 100} {
		if !isFinitePercent(value) {
			t.Fatalf("isFinitePercent(%v) = false, want true", value)
		}
	}
	for _, value := range []float64{-0.01, 100.01, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if isFinitePercent(value) {
			t.Fatalf("isFinitePercent(%v) = true, want false", value)
		}
	}
	for _, value := range []float64{1, 0, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if isValidOddsOverride(value) {
			t.Fatalf("isValidOddsOverride(%v) = true, want false", value)
		}
	}
	if !isValidOddsOverride(1.001) {
		t.Fatal("a finite odds override above one was rejected")
	}
}

func TestMemberTradingAccountQueryRequiresMemberRole(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable"}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	var account user.User
	statement := memberTradingAccountQuery(db.Model(&user.User{}), 42).First(&account).Statement
	if sql := statement.SQL.String(); !strings.Contains(sql, "user_id =") || !strings.Contains(sql, "role =") {
		t.Fatalf("member trading account query is not role scoped: %q", sql)
	}
	want := []any{uint64(42), "member"}
	if len(statement.Vars) < len(want) || statement.Vars[0] != want[0] || statement.Vars[1] != want[1] {
		t.Fatalf("member trading account query vars = %#v, want %#v", statement.Vars, want)
	}
}
