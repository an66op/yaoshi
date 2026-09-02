package services

import (
	"backend/data/models/user"
	"fmt"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestTradingAdminPostgresProtectsAccountStateAndUsesBettingLockOrder(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "trading_locked_update", "782079")
	member := timingPostgresMember(t, db, room, "trading_locked_update_member")
	configureTestGameOdds(t, db, "speed-racing", map[string]float64{"ball_1_5": 9.9})
	service := NewTradingAdminService(db)
	var gameLocked, userLocked, inject, injecting bool
	var expected user.User
	if err := db.Callback().Query().After("gorm:query").Register("test:trading_lock_order_and_stale_account", func(tx *gorm.DB) {
		if tx.Error != nil || injecting {
			return
		}
		locking, ok := tx.Statement.Clauses["FOR"].Expression.(clause.Locking)
		if !ok {
			return
		}
		if tx.Statement.Table == "lottery_games" && locking.Strength == "SHARE" {
			gameLocked = true
		}
		if locking.Strength == "UPDATE" && (tx.Statement.Table == "user" || tx.Statement.Table == "workspaces") && !gameLocked {
			tx.AddError(fmt.Errorf("trading account/workspace lock preceded the game SHARE lock"))
			return
		}
		if tx.Statement.Table != "user" || locking.Strength != "UPDATE" {
			return
		}
		userLocked = true
		if !inject {
			return
		}
		inject, injecting = false, true
		// A deterministic same-transaction interleave changes protected state
		// after the service has loaded its account snapshot. An accidental
		// Save(&account) would overwrite this; a whitelist update cannot.
		err := tx.Session(&gorm.Session{NewDB: true}).Model(&user.User{}).Where("user_id = ?", member.UserID).Updates(map[string]any{
			"balance_cents": expected.BalanceCents, "password": expected.Password,
			"auth_version": expected.AuthVersion, "status": expected.Status,
		}).Error
		injecting = false
		if err != nil {
			tx.AddError(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("test:trading_white_list", func(tx *gorm.DB) {
		if tx.Statement.Table != "user" || injecting {
			return
		}
		if !gameLocked {
			tx.AddError(fmt.Errorf("account trading write preceded game lock"))
			return
		}
		fields, ok := tx.Statement.Dest.(map[string]any)
		if !ok {
			tx.AddError(fmt.Errorf("trading write used a complete account instead of a field whitelist"))
			return
		}
		for _, forbidden := range []string{"balance_cents", "password", "auth_version", "status", "workspace_id", "role"} {
			if _, found := fields[forbidden]; found {
				tx.AddError(fmt.Errorf("trading update included protected %s", forbidden))
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove("test:trading_lock_order_and_stale_account")
		_ = db.Callback().Update().Remove("test:trading_white_list")
	})
	for _, scoped := range []bool{false, true} {
		var before user.User
		if err := db.First(&before, member.UserID).Error; err != nil {
			t.Fatal(err)
		}
		expected = before
		expected.BalanceCents += 1234
		expected.AuthVersion++
		expected.Password = before.Password + "-changed"
		expected.Status = 0
		gameLocked, userLocked, inject = false, false, true
		input := UpdateUserTradingInput{GameID: "speed-racing", FlyMode: "custom", FlyRate: 12, RebateMode: "custom", RebateRate: 3}
		var err error
		if scoped {
			_, err = service.UpdateForWorkspace(room.ID, member.UserID, input)
		} else {
			_, err = service.Update(member.UserID, input)
		}
		if err != nil || !gameLocked || !userLocked || inject {
			t.Fatalf("scoped=%v did not take correct locks and exercise the interleave: game=%v user=%v pending=%v err=%v", scoped, gameLocked, userLocked, inject, err)
		}
		var after user.User
		if err := db.First(&after, member.UserID).Error; err != nil {
			t.Fatal(err)
		}
		if after.BalanceCents != expected.BalanceCents || after.Password != expected.Password || after.AuthVersion != expected.AuthVersion || after.Status != expected.Status || after.WorkspaceID != before.WorkspaceID {
			t.Fatalf("scoped=%v trading write overwrote protected account state", scoped)
		}
		if after.FlyMode != "custom" || after.FlyRate != 12 || after.RebateMode != "custom" || after.RebateRate != 3 {
			t.Fatalf("scoped=%v trading fields did not update", scoped)
		}
	}
	for _, gameID := range []string{"speed-racing", ""} {
		gameLocked, userLocked, inject = false, false, false
		if _, err := service.GetForWorkspace(room.ID, member.UserID, gameID); err != nil || !gameLocked || !userLocked {
			t.Fatalf("scoped read lock order with game=%q: game=%v user=%v err=%v", gameID, gameLocked, userLocked, err)
		}
		gameLocked, userLocked = false, false
		if _, err := service.UpdateRoomForWorkspace(room.ID, UpdateRoomTradingInput{GameID: gameID, RebateRate: 1}); err != nil || !gameLocked {
			t.Fatalf("room update lock order with game=%q: game=%v err=%v", gameID, gameLocked, err)
		}
	}
}
