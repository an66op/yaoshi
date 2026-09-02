package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"gorm.io/gorm"
)

// These tests share the strict empty, loopback-only database guard and outer
// schema rollback used by the timing integration suite. Every source response
// is an in-memory fixture; no external lottery site or business DB is queried.
func rolloverPostgresGame(t *testing.T, db *gorm.DB, issue string) (lottery.Game, sourceDraw) {
	t.Helper()
	drawAt := time.Now().UTC().Truncate(time.Second).Add(-time.Second)
	updates := map[string]any{
		"enabled": true, "source_kind": "external", "sync_status": "ok", "last_sync_error": "",
		"next_issue": issue, "next_draw_at": drawAt, "draw_interval": 75, "timing_source": "upstream",
	}
	if err := db.Model(&lottery.Game{}).Where("id = ?", "speed-racing").Updates(updates).Error; err != nil {
		t.Fatal(err)
	}
	var game lottery.Game
	if err := db.First(&game, "id = ?", "speed-racing").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := NewBetAdminService(db).EnsureCurrentIssue(&game); err != nil {
		t.Fatal(err)
	}
	return game, sourceDraw{
		Issue: issue, DrawAt: drawAt, Numbers: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		NextIssue: nextIssue(issue), NextDrawAt: drawAt.Add(75 * time.Second),
	}
}

func rolloverPostgresPendingBet(t *testing.T, db *gorm.DB, room workspacemodel.Workspace, game lottery.Game, name string) (user.User, bet.Bet) {
	t.Helper()
	member := timingPostgresMember(t, db, room, name)
	// Fixture represents an accepted two-unit stake with its debit already
	// applied. Rollover/settlement is under test, not the placement gateway.
	if err := db.Model(&member).Update("balance_cents", 99_800).Error; err != nil {
		t.Fatal(err)
	}
	ticket := bet.Bet{
		WorkspaceID: room.ID, GameID: game.ID, Issue: game.NextIssue, RoomScope: room.Scope,
		UserID: member.UserID, Username: member.Username, PlayCode: "ball_1_5", PlayName: "指定名次号码",
		RuleVersion: "racing-v2", Position: 1, Selection: "1", AmountCents: 200, Odds: 9.9, Status: "pending",
	}
	if err := db.Create(&ticket).Error; err != nil {
		t.Fatal(err)
	}
	return member, ticket
}

func rolloverPostgresIssue(t *testing.T, db *gorm.DB, gameID, issue string) lottery.Issue {
	t.Helper()
	var row lottery.Issue
	if err := db.Where("game_id = ? AND issue = ?", gameID, issue).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func TestLotteryTimingPostgresRollover(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "rollover_tenant", "76501")
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "speed-racing", true); err != nil {
		t.Fatal(err)
	}
	service := NewLotteryService(db)
	ctx := context.Background()

	t.Run("next issue ready before publication and old bet settles once", func(t *testing.T) {
		game, draw := rolloverPostgresGame(t, db, "92001")
		member, ticket := rolloverPostgresPendingBet(t, db, room, game, "rollover_winner")
		published := 0
		publish := func(event lottery.Game) {
			published++
			var stored lottery.Game
			if err := db.First(&stored, "id = ?", game.ID).Error; err != nil {
				t.Fatal(err)
			}
			if event.NextIssue != draw.NextIssue || stored.NextIssue != draw.NextIssue ||
				!stored.NextDrawAt.Equal(draw.NextDrawAt) || stored.SyncStatus != "ok" {
				t.Fatalf("publication raced its stored next schedule: event=%+v, stored=%+v", event, stored)
			}
			next := rolloverPostgresIssue(t, db, game.ID, draw.NextIssue)
			if next.Status != lottery.IssueStatusAccepting || next.ScheduledDrawAt == nil || !next.ScheduledDrawAt.Equal(draw.NextDrawAt) {
				t.Fatalf("publication preceded next lifecycle readiness: %+v", next)
			}
			var storedTicket bet.Bet
			if err := db.First(&storedTicket, ticket.ID).Error; err != nil {
				t.Fatal(err)
			}
			if storedTicket.Status != "pending" {
				t.Fatalf("old settlement ran before next schedule publication: %s", storedTicket.Status)
			}
			var imported int64
			if err := db.Model(&lottery.Draw{}).Where("game_id = ? AND issue = ?", game.ID, draw.Issue).Count(&imported).Error; err != nil || imported != 1 {
				t.Fatalf("schedule publication without its immutable draw: count=%d, err=%v", imported, err)
			}
		}
		fetch := func(context.Context) ([]sourceDraw, error) { return []sourceDraw{draw}, nil }
		result := service.syncOfficialGameWithPublisher(ctx, game.ID, fetch, publish)
		if result.Status != "ok" || result.Imported != 1 || published != 1 {
			t.Fatalf("rollover failed: result=%+v, publications=%d", result, published)
		}
		settled := rolloverPostgresIssue(t, db, game.ID, draw.Issue)
		if settled.Status != lottery.IssueStatusSettled || settled.SettledAt == nil {
			t.Fatalf("old issue did not settle: %+v", settled)
		}
		if err := db.First(&ticket, ticket.ID).Error; err != nil {
			t.Fatal(err)
		}
		if ticket.Status != "won" || ticket.PayoutCents != 1_980 {
			t.Fatalf("winning old bet did not settle correctly: %+v", ticket)
		}
		before := timingPostgresMoney(t, db, member.UserID)
		if before.BalanceCents != 101_780 || before.Pending != 0 || before.LedgerRows != 1 {
			t.Fatalf("settlement money mismatch: %+v", before)
		}
		for attempt := 0; attempt < 2; attempt++ {
			result = service.syncOfficialGameWithPublisher(ctx, game.ID, fetch, publish)
			if result.Status != "ok" || result.Imported != 0 || published != 1 {
				t.Fatalf("duplicate source response republished/reimported: result=%+v, publications=%d", result, published)
			}
			if after := timingPostgresMoney(t, db, member.UserID); after != before {
				t.Fatalf("duplicate draw moved money: before=%+v, after=%+v", before, after)
			}
			if after := rolloverPostgresIssue(t, db, game.ID, draw.Issue); !reflect.DeepEqual(after, settled) {
				t.Fatalf("duplicate draw rewrote settled lifecycle: before=%+v, after=%+v", settled, after)
			}
		}
	})

	t.Run("already imported draw recovers unfinished settlement", func(t *testing.T) {
		game, draw := rolloverPostgresGame(t, db, "92011")
		member, ticket := rolloverPostgresPendingBet(t, db, room, game, "rollover_recovery")
		storedDraw := lottery.Draw{GameID: game.ID, Issue: draw.Issue, Numbers: joinNumbers(draw.Numbers), DrawAt: draw.DrawAt}
		if err := db.Create(&storedDraw).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Model(&lottery.Issue{}).Where("game_id = ? AND issue = ?", game.ID, draw.Issue).
			Updates(map[string]any{"status": lottery.IssueStatusError, "last_error": "fixture interrupted settlement"}).Error; err != nil {
			t.Fatal(err)
		}
		result := service.syncOfficialGameWithPublisher(ctx, game.ID, func(context.Context) ([]sourceDraw, error) {
			return []sourceDraw{draw}, nil
		}, func(lottery.Game) {})
		if result.Status != "ok" || result.Imported != 0 {
			t.Fatalf("recover already imported draw: %+v", result)
		}
		if err := db.First(&ticket, ticket.ID).Error; err != nil || ticket.Status != "won" {
			t.Fatalf("previously imported pending bet was skipped: ticket=%+v, err=%v", ticket, err)
		}
		if after := timingPostgresMoney(t, db, member.UserID); after.BalanceCents != 101_780 || after.Pending != 0 || after.LedgerRows != 1 {
			t.Fatalf("recovered settlement money mismatch: %+v", after)
		}
		if row := rolloverPostgresIssue(t, db, game.ID, draw.Issue); row.Status != lottery.IssueStatusSettled || row.LastError != "" {
			t.Fatalf("recovered lifecycle remains unfinished: %+v", row)
		}
	})

	t.Run("lifecycle write failure rolls back draw and new schedule", func(t *testing.T) {
		game, draw := rolloverPostgresGame(t, db, "92021")
		if err := db.Exec("ALTER TABLE lottery_issues ADD CONSTRAINT rollover_fixture_reject_next CHECK (issue <> '92022')").Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := db.Exec("ALTER TABLE lottery_issues DROP CONSTRAINT rollover_fixture_reject_next").Error; err != nil {
				t.Error(err)
			}
		})
		published := 0
		result := service.syncOfficialGameWithPublisher(ctx, game.ID, func(context.Context) ([]sourceDraw, error) {
			return []sourceDraw{draw}, nil
		}, func(event lottery.Game) {
			published++
			// Error invalidation is legitimate; a successful next-period event is
			// not. The reader must keep the old boundary and stop accepting.
			if event.NextIssue != game.NextIssue || event.SyncStatus != "error" || !event.NextDrawAt.Equal(game.NextDrawAt) {
				t.Fatalf("published a rolled-back schedule: %+v", event)
			}
		})
		if result.Status != "error" || published != 1 {
			t.Fatalf("failed lifecycle was advertised as successful: result=%+v, publications=%d", result, published)
		}
		for _, table := range []string{"lottery_draws", "lottery_issues", "lottery_issue_windows"} {
			issue := draw.NextIssue
			if table == "lottery_draws" {
				issue = draw.Issue
			}
			var count int64
			if err := db.Table(table).Where("game_id = ? AND issue = ?", game.ID, issue).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("failed schedule leaked %s: count=%d, err=%v", table, count, err)
			}
		}
		var stored lottery.Game
		if err := db.First(&stored, "id = ?", game.ID).Error; err != nil {
			t.Fatal(err)
		}
		if stored.NextIssue != game.NextIssue || !stored.NextDrawAt.Equal(game.NextDrawAt) || stored.SyncStatus != "error" {
			t.Fatalf("failed schedule was partially committed: %+v", stored)
		}
	})

	t.Run("history backfill never rewinds live timing or clears source error", func(t *testing.T) {
		game, draw := rolloverPostgresGame(t, db, "92031")
		if err := db.Model(&lottery.Game{}).Where("id = ?", game.ID).
			Updates(map[string]any{"sync_status": "error", "last_sync_error": "fixture newest fetch failed"}).Error; err != nil {
			t.Fatal(err)
		}
		var before lottery.Game
		if err := db.First(&before, "id = ?", game.ID).Error; err != nil {
			t.Fatal(err)
		}
		draw.Issue = "91999"
		draw.DrawAt = draw.DrawAt.Add(-time.Hour)
		draw.NextIssue = "92000"
		draw.NextDrawAt = draw.DrawAt.Add(75 * time.Second)
		for attempt, expectedImports := range []int{1, 0} {
			imported, err := service.importOfficialHistory(ctx, game.ID, []sourceDraw{draw})
			if err != nil || imported != expectedImports {
				t.Fatalf("history attempt %d imported=%d, err=%v", attempt, imported, err)
			}
			var after lottery.Game
			if err := db.First(&after, "id = ?", game.ID).Error; err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("history changed live source/schedule: before=%+v, after=%+v", before, after)
			}
			if row := rolloverPostgresIssue(t, db, game.ID, draw.Issue); row.Status != lottery.IssueStatusSettled {
				t.Fatalf("history did not settle its own issue: %+v", row)
			}
		}
	})

	t.Run("fetch failure only publishes persisted error and original schedule", func(t *testing.T) {
		game, _ := rolloverPostgresGame(t, db, "92041")
		published := 0
		result := service.syncOfficialGameWithPublisher(ctx, game.ID, func(context.Context) ([]sourceDraw, error) {
			return nil, fmt.Errorf("fixture unavailable source")
		}, func(event lottery.Game) {
			published++
			if event.SyncStatus != "error" || event.NextIssue != game.NextIssue || !event.NextDrawAt.Equal(game.NextDrawAt) {
				t.Fatalf("source failure moved the period: %+v", event)
			}
			var stored lottery.Game
			if err := db.First(&stored, "id = ?", game.ID).Error; err != nil || stored.LastSyncError != event.LastSyncError {
				t.Fatalf("source failure event raced persistence: stored=%+v, err=%v", stored, err)
			}
		})
		if result.Status != "error" || published != 1 {
			t.Fatalf("source error did not invalidate readers: result=%+v, publications=%d", result, published)
		}
	})

	t.Run("stale valid response cannot rewind verified schedule or clear newer error", func(t *testing.T) {
		for index, syncStatus := range []string{"ok", "error"} {
			t.Run(syncStatus, func(t *testing.T) {
				game, draw := rolloverPostgresGame(t, db, fmt.Sprintf("%d", 92061+index*10))
				currentIssue := nextIssue(draw.NextIssue)
				currentDrawAt := draw.NextDrawAt
				lastError := ""
				if syncStatus == "error" {
					lastError = "fixture latest attempt failed after this verified schedule"
				}
				if err := db.Model(&lottery.Game{}).Where("id = ?", game.ID).Updates(map[string]any{
					"next_issue": currentIssue, "next_draw_at": currentDrawAt,
					"sync_status": syncStatus, "last_sync_error": lastError,
				}).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.First(&game, "id = ?", game.ID).Error; err != nil {
					t.Fatal(err)
				}
				if _, err := NewBetAdminService(db).EnsureCurrentIssue(&game); err != nil {
					t.Fatal(err)
				}
				before := rolloverPostgresIssue(t, db, game.ID, currentIssue)
				// The feed gives internally valid metadata, but it is one cached
				// period behind the already persisted upstream schedule.
				draw.DrawAt = draw.DrawAt.Add(-75 * time.Second)
				draw.NextDrawAt = draw.DrawAt.Add(75 * time.Second)
				published := 0
				result := service.syncOfficialGameWithPublisher(ctx, game.ID, func(context.Context) ([]sourceDraw, error) {
					return []sourceDraw{draw}, nil
				}, func(lottery.Game) { published++ })
				if result.Status != "ok" || result.Imported != 1 || published != 0 {
					t.Fatalf("stale response was treated as a current schedule: result=%+v, publications=%d", result, published)
				}
				var stored lottery.Game
				if err := db.First(&stored, "id = ?", game.ID).Error; err != nil {
					t.Fatal(err)
				}
				if stored.NextIssue != currentIssue || !stored.NextDrawAt.Equal(currentDrawAt) ||
					stored.SyncStatus != syncStatus || stored.LastSyncError != lastError || stored.TimingSource != game.TimingSource {
					t.Fatalf("cached feed rewound schedule/health: %+v", stored)
				}
				if after := rolloverPostgresIssue(t, db, game.ID, currentIssue); !reflect.DeepEqual(after, before) {
					t.Fatalf("cached feed changed current lifecycle: before=%+v, after=%+v", before, after)
				}
				if row := rolloverPostgresIssue(t, db, game.ID, draw.Issue); row.Status != lottery.IssueStatusSettled {
					t.Fatalf("cached historical draw was not backfilled/settled: %+v", row)
				}
			})
		}
	})

	t.Run("earlier cutoff for the same issue is accepted and published", func(t *testing.T) {
		game, draw := rolloverPostgresGame(t, db, "92081")
		if err := db.Model(&lottery.Game{}).Where("id = ?", game.ID).Updates(map[string]any{
			"next_issue": draw.NextIssue, "next_draw_at": draw.NextDrawAt.Add(30 * time.Second),
		}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.First(&game, "id = ?", game.ID).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := NewBetAdminService(db).EnsureCurrentIssue(&game); err != nil {
			t.Fatal(err)
		}
		published := 0
		result := service.syncOfficialGameWithPublisher(ctx, game.ID, func(context.Context) ([]sourceDraw, error) {
			return []sourceDraw{draw}, nil
		}, func(event lottery.Game) {
			published++
			if event.NextIssue != draw.NextIssue || !event.NextDrawAt.Equal(draw.NextDrawAt) {
				t.Fatalf("earlier cutoff for same period was rejected: %+v", event)
			}
		})
		if result.Status != "ok" || result.Imported != 1 || published != 1 {
			t.Fatalf("same-period correction failed: result=%+v, publications=%d", result, published)
		}
		row := rolloverPostgresIssue(t, db, game.ID, draw.NextIssue)
		if row.ScheduledDrawAt == nil || !row.ScheduledDrawAt.Equal(draw.NextDrawAt) || !row.SealAt.Equal(draw.NextDrawAt.Add(-30*time.Second)) {
			t.Fatalf("same-period earlier cutoff did not reach persisted lifecycle: %+v", row)
		}
	})

	t.Run("room operator disabling during fetch prevents import and reopen", func(t *testing.T) {
		game, draw := rolloverPostgresGame(t, db, "92051")
		result := service.syncOfficialGameWithPublisher(ctx, game.ID, func(context.Context) ([]sourceDraw, error) {
			if err := db.Model(&lottery.Game{}).Where("id = ?", game.ID).Update("enabled", false).Error; err != nil {
				t.Fatal(err)
			}
			return []sourceDraw{draw}, nil
		}, func(event lottery.Game) {
			if event.Enabled || event.NextIssue != game.NextIssue {
				t.Fatalf("fetch completion reopened disabled game: %+v", event)
			}
		})
		if result.Status != "ok" || result.Imported != 0 {
			t.Fatalf("disabled game imported source result: %+v", result)
		}
		var count int64
		if err := db.Model(&lottery.Draw{}).Where("game_id = ? AND issue = ?", game.ID, draw.Issue).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("disabled game retained a newly imported draw: count=%d, err=%v", count, err)
		}
	})
}
