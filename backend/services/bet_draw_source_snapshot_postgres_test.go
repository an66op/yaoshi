package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/data/models/user"
	apperrors "backend/errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The shared opt-in fixture accepts only an empty loopback timing-test database
// and rolls back its schema and rows. No application DB or source HTTP is used.
func sgSSCPlacementPostgresFixture(t *testing.T) (*gorm.DB, *BetAdminService, user.User, *lottery.Game) {
	t.Helper()
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "sg_placement_room", "783031")
	member := timingPostgresMember(t, db, room, "sg_placement_member")
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "sg-ssc", true); err != nil {
		t.Fatal(err)
	}
	timingPostgresSettings(t, db, room.ID, `{"seal_seconds":30}`)
	configureTestGameOdds(t, db, "sg-ssc", map[string]float64{"ball_1_5": 9.9})

	now := time.Now().UTC()
	nextAt := now.Truncate(sgSSCInterval).Add(sgSSCInterval)
	if nextAt.Sub(now) < 45*time.Second {
		t.Skip("SG fixture is near its real five-minute seal boundary; rerun next period")
	}
	if err := db.Model(&lottery.Game{}).Where("id = ?", "sg-ssc").Updates(map[string]any{
		"enabled": true, "source_kind": "external", "source_name": sgSSCVerifiedSourceName, "source_url": sgSSCVerifiedSourceURL,
		"sync_status": "ok", "last_sync_error": "", "last_sync_at": now,
		"next_issue": sgSSCIssueAt(nextAt), "next_draw_at": nextAt, "draw_interval": 300, "timing_source": "upstream",
	}).Error; err != nil {
		t.Fatal(err)
	}
	var game lottery.Game
	if err := db.First(&game, "id = ?", "sg-ssc").Error; err != nil {
		t.Fatal(err)
	}
	if !sourceHealthyForGame(&game) {
		t.Fatalf("fixture did not bind a fresh, verified upstream SG schedule: %+v", game)
	}
	service := NewBetAdminService(db)
	service.suppressNotifications = true
	return db, service, member, &game
}

func sgSSCPlacementPostgresPlace(service *BetAdminService, member user.User, game *lottery.Game, batch bool) ([]BetView, error) {
	input := placementPostgresInput(member, game, "2", 20)
	if batch {
		return service.PlaceBatch([]PlaceBetInput{input, placementPostgresInput(member, game, "3", 30)})
	}
	row, err := service.Place(input)
	if err != nil {
		return nil, err
	}
	return []BetView{*row}, nil
}

func sgSSCPlacementLegacyTicket(member user.User, game *lottery.Game) bet.Bet {
	return bet.Bet{
		WorkspaceID: member.WorkspaceID, UserID: member.UserID, Username: member.Username, RoomScope: betRoomScope(member),
		GameID: game.ID, Issue: game.NextIssue, PlayCode: "ball_1_5", PlayName: "第一球", Position: 1, Selection: "2",
		RuleVersion: "digits5-v3", Status: "pending", AmountCents: 1000, Odds: 9.9, RequestReference: "sg-legacy-orphan",
		// Deliberately empty: a pre-cutover ticket cannot acquire today's source.
		DrawSourceRevision: "",
	}
}

func TestSGSSCPlacementPostgresFreezesDrawSourceRevision(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(fmt.Sprintf("batch=%t", batch), func(t *testing.T) {
			db, service, member, game := sgSSCPlacementPostgresFixture(t)
			before := timingPostgresMoney(t, db, member.UserID)
			views, err := sgSSCPlacementPostgresPlace(service, member, game, batch)
			wantRows, wantDebit := 1, int64(2000)
			if batch {
				wantRows, wantDebit = 2, 5000
			}
			if err != nil || len(views) != wantRows {
				t.Fatalf("verified SG placement failed: %+v / %v", views, err)
			}
			for _, view := range views {
				row := placementPostgresRow(t, db, view.ID)
				if row.DrawSourceRevision != sgSSCSourceRevision || view.DrawSourceRevision != sgSSCSourceRevision ||
					row.RuleVersion != "digits5-v3" || row.Issue != game.NextIssue || row.Status != "pending" {
					t.Fatalf("placement lost the immutable source/rule contract: row=%+v view=%+v", row, view)
				}
				placementPostgresAssertDebitEvidence(t, db, row, wantDebit)
			}
			after := timingPostgresMoney(t, db, member.UserID)
			if after.BalanceCents != before.BalanceCents-wantDebit || after.Bets != before.Bets+int64(wantRows) ||
				after.Pending != before.Pending+int64(wantRows) || after.LedgerRows != before.LedgerRows+1 {
				t.Fatalf("source snapshot and debit did not commit together: before=%+v after=%+v", before, after)
			}
			var issue lottery.Issue
			if err := db.Where("game_id = ? AND issue = ?", game.ID, game.NextIssue).First(&issue).Error; err != nil || issue.SourceMode != "external" {
				t.Fatalf("new verified tickets did not bind an external lifecycle: %+v / %v", issue, err)
			}
		})
	}
}

func TestSGSSCPlacementPostgresFinalLockRejectsUnverifiedSource(t *testing.T) {
	for _, scenario := range []struct {
		name, table, strength, code string
	}{
		{"failed sync", "lottery_games", "SHARE", "SOURCE_UNAVAILABLE"},
		{"stale sync", "lottery_games", "SHARE", "SOURCE_UNAVAILABLE"},
		{"old platform issue", "lottery_issues", "UPDATE", "DRAW_SOURCE_UNVERIFIED"},
		{"legacy ticket with external lifecycle", "lottery_issues", "UPDATE", "DRAW_SOURCE_UNVERIFIED"},
	} {
		for _, batch := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/batch=%t", scenario.name, batch), func(t *testing.T) {
				db, service, member, game := sgSSCPlacementPostgresFixture(t)
				before := timingPostgresMoney(t, db, member.UserID)
				var injected, gameLocked bool
				var injectionErr error
				var walletLocks int
				const beforeCallback = "test:sg_source_before_final_lock"
				const afterCallback = "test:sg_source_after_game_lock"
				if err := db.Callback().Query().After("gorm:query").Register(afterCallback, func(tx *gorm.DB) {
					lock, ok := tx.Statement.Clauses["FOR"].Expression.(clause.Locking)
					if tx.Error == nil && ok && tx.Statement.Table == "lottery_games" && lock.Strength == "SHARE" {
						gameLocked = true
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = db.Callback().Query().Remove(afterCallback) })
				if err := db.Callback().Query().Before("gorm:query").Register(beforeCallback, func(tx *gorm.DB) {
					lock, ok := tx.Statement.Clauses["FOR"].Expression.(clause.Locking)
					if !ok {
						return
					}
					if tx.Statement.Table == (user.User{}).TableName() && lock.Strength == "UPDATE" {
						walletLocks++
					}
					if injected || tx.Statement.Table != scenario.table || lock.Strength != scenario.strength {
						return
					}
					injected = true
					if scenario.table == "lottery_issues" && !gameLocked {
						injectionErr = fmt.Errorf("Issue UPDATE preceded the successful Game SHARE lock")
						tx.AddError(injectionErr)
						return
					}
					// Use the same real transaction with a clean statement. The
					// injection is rolled back with the rejected placement savepoint.
					fixture := tx.Session(&gorm.Session{NewDB: true})
					var issue lottery.Issue
					injectionErr = fixture.Where("game_id = ? AND issue = ?", game.ID, game.NextIssue).First(&issue).Error
					if injectionErr == nil && (issue.SourceMode != "external" || issue.Status != lottery.IssueStatusAccepting) {
						injectionErr = fmt.Errorf("healthy preflight did not finish before injection: %+v", issue)
					}
					if injectionErr == nil {
						switch scenario.name {
						case "failed sync":
							injectionErr = fixture.Model(&lottery.Game{}).Where("id = ?", game.ID).
								Updates(map[string]any{"sync_status": "error", "last_sync_error": "fixture station disagreement"}).Error
						case "stale sync":
							injectionErr = fixture.Model(&lottery.Game{}).Where("id = ?", game.ID).
								Update("last_sync_at", time.Now().UTC().Add(-2*time.Minute)).Error
						case "old platform issue":
							injectionErr = fixture.Model(&lottery.Issue{}).Where("id = ?", issue.ID).Update("source_mode", "platform").Error
						case "legacy ticket with external lifecycle":
							legacy := sgSSCPlacementLegacyTicket(member, game)
							injectionErr = fixture.Create(&legacy).Error
						}
					}
					if injectionErr != nil {
						tx.AddError(injectionErr)
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = db.Callback().Query().Remove(beforeCallback) })

				views, err := sgSSCPlacementPostgresPlace(service, member, game, batch)
				if !injected || injectionErr != nil || !gameLocked {
					t.Fatalf("test did not reach its real final-lock injection: injected=%t gameLocked=%t fixture=%v placement=%v", injected, gameLocked, injectionErr, err)
				}
				if apperrors.GetErrorCode(err) != scenario.code || len(views) != 0 || walletLocks != 0 {
					t.Fatalf("unverified SG reached wallet/acceptance: views=%+v err=%v walletLocks=%d", views, err, walletLocks)
				}
				if after := timingPostgresMoney(t, db, member.UserID); after != before {
					t.Fatalf("failed final source gate changed money or tickets: before=%+v after=%+v", before, after)
				}
			})
		}
	}
}

func TestSGSSCPlacementPostgresPreservesOrphanLegacyTicket(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(fmt.Sprintf("batch=%t", batch), func(t *testing.T) {
			db, service, member, game := sgSSCPlacementPostgresFixture(t)
			legacy := sgSSCPlacementLegacyTicket(member, game)
			if err := db.Create(&legacy).Error; err != nil {
				t.Fatal(err)
			}
			legacy = placementPostgresRow(t, db, legacy.ID)
			before := timingPostgresMoney(t, db, member.UserID)
			views, err := sgSSCPlacementPostgresPlace(service, member, game, batch)
			if apperrors.GetErrorCode(err) != "SOURCE_UNAVAILABLE" || len(views) != 0 || timingPostgresMoney(t, db, member.UserID) != before {
				t.Fatalf("legacy orphan joined a new upstream period: views=%+v err=%v", views, err)
			}
			if after := placementPostgresRow(t, db, legacy.ID); !reflect.DeepEqual(after, legacy) || after.DrawSourceRevision != "" {
				t.Fatalf("legacy ticket was relabeled or otherwise rewritten: before=%+v after=%+v", legacy, after)
			}
			var issues int64
			if err := db.Model(&lottery.Issue{}).Where("game_id = ? AND issue = ?", game.ID, game.NextIssue).Count(&issues).Error; err != nil || issues != 0 {
				t.Fatalf("placement manufactured an external lifecycle for an orphan: count=%d err=%v", issues, err)
			}
		})
	}
}
