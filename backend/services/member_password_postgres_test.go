package services

import (
	"backend/data/models/user"
	"backend/utils"
	"sync"
	"testing"
)

// This opt-in PostgreSQL contract exercises two stale writers carrying the
// same verified old hash. The conditional UPDATE is the serialization point:
// PostgreSQL may run them in either order, but exactly one can advance the
// credential generation and therefore exactly one revocation receipt exists.
func TestMemberPasswordPostgresCompareAndSwapAllowsOneConcurrentWriter(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "password_cas", "735213")
	member := timingPostgresMember(t, db, room, "password_cas_member")
	oldHash, err := utils.HashPassword("OldPassword#2026")
	if err != nil {
		t.Fatal(err)
	}
	if result := db.Model(&user.User{}).Where("user_id = ?", member.UserID).Updates(passwordSessionUpdate(oldHash)); result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("initialize password hash: affected=%d err=%v", result.RowsAffected, result.Error)
	}
	var before user.User
	if err := db.First(&before, member.UserID).Error; err != nil {
		t.Fatal(err)
	}
	var outboxBefore int64
	if err := db.Table("ws_session_revocation_outbox").Where("user_id = ?", member.UserID).Count(&outboxBefore).Error; err != nil {
		t.Fatal(err)
	}
	newHashes := make([]string, 2)
	for index, password := range []string{"FirstWinner#2026", "SecondWinner#2026"} {
		newHashes[index], err = utils.HashPassword(password)
		if err != nil {
			t.Fatal(err)
		}
	}

	start := make(chan struct{})
	results := make(chan *gormResult, 2)
	var wait sync.WaitGroup
	for index := range newHashes {
		wait.Add(1)
		go func(hash string) {
			defer wait.Done()
			<-start
			result := changePasswordCompareAndSwap(db, member.UserID, oldHash, hash)
			results <- &gormResult{rows: result.RowsAffected, err: result.Error}
		}(newHashes[index])
	}
	close(start)
	wait.Wait()
	close(results)
	winners := 0
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.rows == 1 {
			winners++
		} else if result.rows != 0 {
			t.Fatalf("password CAS affected %d rows", result.rows)
		}
	}
	if winners != 1 {
		t.Fatalf("concurrent stale password writers = %d winners, want 1", winners)
	}

	var after user.User
	if err := db.First(&after, member.UserID).Error; err != nil {
		t.Fatal(err)
	}
	if after.AuthVersion != before.AuthVersion+1 {
		t.Fatalf("auth_version = %d, want %d", after.AuthVersion, before.AuthVersion+1)
	}
	var outboxAfter int64
	if err := db.Table("ws_session_revocation_outbox").Where("user_id = ?", member.UserID).Count(&outboxAfter).Error; err != nil {
		t.Fatal(err)
	}
	if outboxAfter != outboxBefore+1 {
		t.Fatalf("revocation outbox rows = %d, want %d", outboxAfter, outboxBefore+1)
	}
}

type gormResult struct {
	rows int64
	err  error
}
