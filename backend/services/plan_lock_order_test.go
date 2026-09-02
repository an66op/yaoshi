package services

import (
	"backend/data/models/plan"
	apperrors "backend/errors"
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// Explicit transactions still need a pool in DryRun mode. This pool permits
// only transaction boundaries: an accidental SQL execution fails without ever
// creating a network connection.
type planLockOrderDryRunPool struct{}

func (p *planLockOrderDryRunPool) BeginTx(context.Context, *sql.TxOptions) (gorm.ConnPool, error) {
	return p, nil
}
func (*planLockOrderDryRunPool) Commit() error   { return nil }
func (*planLockOrderDryRunPool) Rollback() error { return nil }
func (*planLockOrderDryRunPool) PrepareContext(context.Context, string) (*sql.Stmt, error) {
	return nil, errors.New("plan lock-order unit test must not prepare SQL")
}
func (*planLockOrderDryRunPool) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, errors.New("plan lock-order unit test must not execute SQL")
}
func (*planLockOrderDryRunPool) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, errors.New("plan lock-order unit test must not query SQL")
}
func (*planLockOrderDryRunPool) QueryRowContext(context.Context, string, ...any) *sql.Row {
	panic("plan lock-order unit test must not query a SQL row")
}

func TestPlanVisitLocksGameBeforeAutomation(t *testing.T) {
	for _, missingGame := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing game", true: "missing game keeps permission failure first"}[missingGame], func(t *testing.T) {
			testPlanVisitFirstLocks(t, missingGame)
		})
	}
}

func testPlanVisitFirstLocks(t *testing.T, missingGame bool) {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: &planLockOrderDryRunPool{}}), &gorm.Config{
		DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true,
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	var locks []string
	if err := db.Callback().Query().After("gorm:query").Register("test:plan_game_before_automation", func(tx *gorm.DB) {
		if lock, ok := tx.Statement.Clauses["FOR"].Expression.(clause.Locking); ok {
			locks = append(locks, tx.Statement.Table+":"+lock.Strength)
		}
		if tx.Statement.Table == "plan_automations" {
			*tx.Statement.Dest.(*plan.Automation) = plan.Automation{WorkspaceID: 7, Mode: "demo", GameIDsJSON: "[]"}
		}
		if tx.Statement.Table == "lottery_games" && missingGame {
			tx.AddError(gorm.ErrRecordNotFound)
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, err = NewPlanContentService(db).touchPlan(context.Background(), 7, "speed-racing", 1, DefaultPlanKey)
	if apperrors.GetErrorCode(err) != "AUTOMATION_DISABLED" {
		t.Fatalf("visit must retain its disabled-automation gate: %v", err)
	}
	if want := []string{"lottery_games:SHARE", "plan_automations:UPDATE"}; !reflect.DeepEqual(locks, want) {
		t.Fatalf("visit lock order = %v, want %v", locks, want)
	}
}
