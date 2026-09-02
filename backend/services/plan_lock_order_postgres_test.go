package services

import (
	"backend/data/models/plan"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"context"
	"fmt"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The shared opt-in fixture uses only an empty, explicitly named loopback test
// database and rolls back schema/data. This traces actual statements, not a
// simulated concurrent deadlock: the graph must contain no room/config -> game
// acquisition for a queued platform writer to turn into a wait cycle.
func TestPlanVisitPostgresGameLockPrecedesRoomConfigurationAndStreamWrites(t *testing.T) {
	db, roomID := streamPostgresSetup(t)
	streamPostgresIssue(t, db, 1)
	var room workspacemodel.Workspace
	if err := db.First(&room, roomID).Error; err != nil {
		t.Fatal(err)
	}
	service := NewPlanContentService(db)
	phase := ""
	gameLocked, lowerLocked, gameLocks := false, false, 0
	lockedTables := map[string]bool{}
	start := func(name string) {
		phase, gameLocked, lowerLocked, gameLocks = name, false, false, 0
		lockedTables = map[string]bool{}
	}
	queryCallback := "test:plan_visit_game_lock_first"
	if err := db.Callback().Query().After("gorm:query").Register(queryCallback, func(tx *gorm.DB) {
		if phase == "" || tx.Error != nil {
			return
		}
		lock, ok := tx.Statement.Clauses["FOR"].Expression.(clause.Locking)
		if !ok {
			return // Ordinary catalogue reads in configuration saves do not wait on row locks.
		}
		if tx.Statement.Table == "lottery_games" {
			if lowerLocked || lock.Strength != "SHARE" {
				tx.AddError(fmt.Errorf("%s acquired game %s after lower locks=%v", phase, lock.Strength, lowerLocked))
				return
			}
			gameLocked, gameLocks = true, gameLocks+1
		} else {
			if phase == "visit" && !gameLocked {
				tx.AddError(fmt.Errorf("visit locked %s before game SHARE", tx.Statement.Table))
				return
			}
			lowerLocked = true
		}
		lockedTables[tx.Statement.Table] = true
	}); err != nil {
		t.Fatal(err)
	}
	writeCallback := "test:plan_visit_game_before_writes"
	checkWrite := func(tx *gorm.DB) {
		if phase == "" || tx.Error != nil {
			return
		}
		if phase == "visit" && !gameLocked {
			tx.AddError(fmt.Errorf("visit wrote %s before game SHARE", tx.Statement.Table))
			return
		}
		lowerLocked = true
	}
	if err := db.Callback().Create().Before("gorm:create").Register(writeCallback, checkWrite); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register(writeCallback, checkWrite); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove(queryCallback)
		_ = db.Callback().Create().Remove(writeCallback)
		_ = db.Callback().Update().Remove(writeCallback)
	})
	visit := func(position int) (PlanAutomationRun, error) {
		start("visit")
		result, err := service.touchPlan(context.Background(), roomID, "speed-racing", position, DefaultPlanKey)
		phase = ""
		if !gameLocked || gameLocks != 1 {
			t.Fatalf("visit must acquire exactly one early game lock, got locked=%v count=%d err=%v", gameLocked, gameLocks, err)
		}
		return result, err
	}
	first, err := visit(1)
	if err != nil || first.CreatedCount != 3 || first.EligibleGameCount != 1 {
		t.Fatalf("first visit = %+v, %v", first, err)
	}
	for _, table := range []string{"plan_automations", "workspaces", "user", "system_settings", "room_game_settings", "lottery_issues", "lottery_issue_windows"} {
		if !lockedTables[table] {
			t.Fatalf("full visit did not exercise %s lock: %v", table, lockedTables)
		}
	}
	var original plan.Stream
	if err := db.Where("workspace_id = ? AND position = ? AND plan_key = ?", roomID, 1, DefaultPlanKey).First(&original).Error; err != nil {
		t.Fatal(err)
	}
	if original.ActiveUntil == nil {
		t.Fatal("first visit did not create its active lease")
	}
	if repeated, err := visit(1); err != nil || repeated.CreatedCount != 0 {
		t.Fatalf("coalesced visit generated again: %+v, %v", repeated, err)
	}
	var after plan.Stream
	if err := db.First(&after, original.ID).Error; err != nil || !after.UpdatedAt.Equal(original.UpdatedAt) || after.ActiveUntil == nil || !after.ActiveUntil.Equal(*original.ActiveUntil) {
		t.Fatalf("lock reordering changed visit coalescing: before=%+v after=%+v err=%v", original, after, err)
	}
	start("save")
	on := true
	_, err = NewPlanAutomationService(db).Save(roomID, PlanAutomationInput{Enabled: &on, Positions: []int{1}})
	phase = ""
	if err != nil {
		t.Fatalf("configuration save introduced a reverse game lock: %v", err)
	}
	if _, err := visit(2); apperrors.GetErrorCode(err) != "PLAN_STREAM_NOT_ALLOWED" {
		t.Fatalf("configuration permission bypassed: %v", err)
	}
	start("room switch")
	_, err = NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "speed-racing", false)
	phase = ""
	if err != nil {
		t.Fatalf("room switch introduced a reverse game lock: %v", err)
	}
	if _, err := visit(1); apperrors.GetErrorCode(err) != "ROOM_CLOSED" {
		t.Fatalf("disabled room game generated a plan: %v", err)
	}
	start("save")
	on = false
	_, err = NewPlanAutomationService(db).Save(roomID, PlanAutomationInput{Enabled: &on})
	phase = ""
	if err != nil {
		t.Fatalf("disabling configuration failed: %v", err)
	}
	if _, err := visit(1); apperrors.GetErrorCode(err) != "AUTOMATION_DISABLED" {
		t.Fatalf("disabled automation generated a plan: %v", err)
	}
}
