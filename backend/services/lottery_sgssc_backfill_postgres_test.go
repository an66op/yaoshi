package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/data/models/user"
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"gorm.io/gorm"
)

func sgSSCBackfillPostgresFixture(t *testing.T) (*gorm.DB, *LotteryService, user.User, time.Time) {
	t.Helper()
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "sg_history_room", "784201")
	member := timingPostgresMember(t, db, room, "sg_history_member")
	now := time.Now().UTC()
	next := now.Truncate(sgSSCInterval).Add(sgSSCInterval)
	if err := db.Model(&lottery.Game{}).Where("id = ?", "sg-ssc").Updates(map[string]any{
		"enabled": true, "source_kind": "external", "source_name": sgSSCVerifiedSourceName, "source_url": sgSSCVerifiedSourceURL,
		"sync_status": "error", "last_sync_error": "fixture live source still unavailable", "last_sync_at": now.Add(-time.Hour),
		"next_issue": sgSSCIssueAt(next), "next_draw_at": next, "timing_source": "upstream", "draw_interval": 300,
	}).Error; err != nil {
		t.Fatal(err)
	}
	return db, NewLotteryService(db), member, now
}

func sgSSCBackfillPostgresTicket(t *testing.T, db *gorm.DB, member user.User, at time.Time) bet.Bet {
	t.Helper()
	row := bet.Bet{WorkspaceID: member.WorkspaceID, GameID: "sg-ssc", Issue: sgSSCIssueAt(at), RoomScope: member.LoginScope,
		UserID: member.UserID, Username: member.Username, PlayCode: "two_sided", PlayName: "两面", Position: 1, Selection: "大",
		AmountCents: 100, Odds: 2, Status: "pending", RuleVersion: "digits5-v3", DrawSourceRevision: sgSSCSourceRevision,
		RequestReference: "sg-history:" + sgSSCIssueAt(at)}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func sgSSCBackfillPostgresDraw(at time.Time) sourceDraw {
	return sourceDraw{Issue: sgSSCIssueAt(at), DrawAt: at, Numbers: []int{6, 5, 8, 3, 0}, SourceRevision: sgSSCSourceRevision, ConversionRevision: sgSSCConversionRevision}
}

func sgSSCBackfillPostgresStored(t *testing.T, db *gorm.DB, at time.Time) lottery.Draw {
	t.Helper()
	draw := sgSSCBackfillPostgresDraw(at)
	stored := lottery.Draw{GameID: "sg-ssc", Issue: draw.Issue, DrawAt: at, Numbers: joinNumbers(draw.Numbers), SourceRevision: draw.SourceRevision, ConversionRevision: draw.ConversionRevision}
	if err := db.Create(&stored).Error; err != nil {
		t.Fatal(err)
	}
	return stored
}

func sgSSCBackfillPostgresItem(t *testing.T, db *gorm.DB, issue string) lottery.SGSSCBackfillItem {
	t.Helper()
	var row lottery.SGSSCBackfillItem
	if err := db.First(&row, "issue = ?", issue).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func TestSGSSCBackfillPostgresLongOutageRecoversOnceWithoutChangingLiveState(t *testing.T) {
	db, service, member, now := sgSSCBackfillPostgresFixture(t)
	at := now.Truncate(sgSSCInterval).Add(-8 * time.Hour)
	ticket := sgSSCBackfillPostgresTicket(t, db, member, at)
	period := lottery.Issue{GameID: "sg-ssc", Issue: ticket.Issue, SourceMode: "external", Status: lottery.IssueStatusError,
		AcceptAt: at.Add(-sgSSCInterval), SealAt: at.Add(-30 * time.Second), ScheduledDrawAt: &at, LastError: "对账异常：历史缺期"}
	if err := db.Create(&period).Error; err != nil {
		t.Fatal(err)
	}
	var beforeGame lottery.Game
	if err := db.First(&beforeGame, "id = ?", "sg-ssc").Error; err != nil {
		t.Fatal(err)
	}
	before := timingPostgresMoney(t, db, member.UserID)
	status, err := service.SGSSCBackfillStatus(context.Background(), 0, 20)
	if err != nil || status.Summary.UntrackedPendingIssues != 1 || len(status.Gaps) != 0 || len(status.Records) != 0 {
		t.Fatalf("read should not queue: %+v %v", status, err)
	}
	queued, err := service.QueueSGSSCBackfill(context.Background(), "history-admin", "history-request")
	if err != nil || queued.QueuedIssues != 1 {
		t.Fatalf("queue: %+v %v", queued, err)
	}
	queued, err = service.QueueSGSSCBackfill(context.Background(), "second-admin", "second-request")
	if err != nil || queued.QueuedIssues != 0 {
		t.Fatalf("duplicate queue: %+v %v", queued, err)
	}
	// The HTTP queue action uses the wall clock; advance the worker fixture
	// beyond that action rather than claiming at the earlier setup timestamp.
	now = time.Now().UTC().Add(time.Second)
	fetches := 0
	result, err := service.runSGSSCBackfill(context.Background(), func() time.Time { return now }, func(_ context.Context, issues []string) (SGSSCHistoryVerification, error) {
		fetches++
		if !reflect.DeepEqual(issues, []string{ticket.Issue}) {
			t.Fatalf("unexpected fetch targets: %v", issues)
		}
		return SGSSCHistoryVerification{Draws: []sourceDraw{sgSSCBackfillPostgresDraw(at)}}, nil
	})
	after := timingPostgresMoney(t, db, member.UserID)
	if err != nil || result.Recovered != 1 || result.Deferred != 0 || fetches != 1 || after.BalanceCents != before.BalanceCents+200 || after.LedgerRows != before.LedgerRows+1 || after.Pending != 0 {
		t.Fatalf("long outage recovery: %+v %v money %+v -> %+v fetches %d", result, err, before, after, fetches)
	}
	var afterGame lottery.Game
	if err := db.First(&afterGame, "id = ?", "sg-ssc").Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeGame, afterGame) {
		t.Fatal("historical recovery changed live health, next issue or schedule")
	}
	status, err = service.SGSSCBackfillStatus(context.Background(), 0, 20)
	if err != nil || status.Summary.CompletedIssues != 1 || len(status.Records) != 1 {
		t.Fatalf("status %+v %v", status, err)
	}
	record := status.Records[0]
	if record.Status != "recovered" || !record.Imported || record.SettledBets != 1 || record.Operator != "history-admin" || record.RequestID != "history-request" || record.Numbers != "6,5,8,3,0" {
		t.Fatalf("missing durable receipt %+v", record)
	}
	_, err = service.runSGSSCBackfill(context.Background(), func() time.Time { return now.Add(time.Minute) }, func(context.Context, []string) (SGSSCHistoryVerification, error) {
		t.Fatal("completed history fetched again")
		return SGSSCHistoryVerification{}, nil
	})
	if err != nil || !reflect.DeepEqual(timingPostgresMoney(t, db, member.UserID), after) {
		t.Fatalf("repeat changed money: %v", err)
	}
}

func TestSGSSCBackfillPostgresPartialResultsAndRetryReceipts(t *testing.T) {
	db, service, member, now := sgSSCBackfillPostgresFixture(t)
	at := now.Truncate(sgSSCInterval).Add(-4 * time.Hour)
	first := sgSSCBackfillPostgresTicket(t, db, member, at)
	second := sgSSCBackfillPostgresTicket(t, db, member, at.Add(sgSSCInterval))
	before := timingPostgresMoney(t, db, member.UserID)
	result, err := service.runSGSSCBackfill(context.Background(), func() time.Time { return now }, func(context.Context, []string) (SGSSCHistoryVerification, error) {
		return SGSSCHistoryVerification{Draws: []sourceDraw{sgSSCBackfillPostgresDraw(at)}, Failures: []SGSSCHistoryFailure{{Issue: second.Issue, Error: "115历史缺少该期"}}}, nil
	})
	if err != nil || result.Recovered != 1 || result.Deferred != 1 {
		t.Fatalf("partial %+v %v", result, err)
	}
	if sgSSCBackfillPostgresItem(t, db, first.Issue).Status != "completed" {
		t.Fatal("successful peer did not advance")
	}
	missing := sgSSCBackfillPostgresItem(t, db, second.Issue)
	if missing.Status != "retry" || missing.LastError != "115历史缺少该期" || !missing.NextRetryAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("missing-period state %+v", missing)
	}
	var count int64
	if err := db.Model(&lottery.Draw{}).Where("game_id = ? AND issue = ?", "sg-ssc", second.Issue).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("missing draw fabricated %d %v", count, err)
	}
	queued, err := service.QueueSGSSCBackfill(context.Background(), "retry-admin", "retry-manual")
	if err != nil || queued.QueuedIssues != 1 {
		t.Fatalf("retry enqueue %+v %v", queued, err)
	}
	result, err = service.runSGSSCBackfill(context.Background(), func() time.Time { return now.Add(time.Minute) }, func(_ context.Context, targets []string) (SGSSCHistoryVerification, error) {
		if !reflect.DeepEqual(targets, []string{second.Issue}) {
			t.Fatal(targets)
		}
		return SGSSCHistoryVerification{Draws: []sourceDraw{sgSSCBackfillPostgresDraw(at.Add(sgSSCInterval))}}, nil
	})
	after := timingPostgresMoney(t, db, member.UserID)
	if err != nil || result.Recovered != 1 || after.BalanceCents != before.BalanceCents+400 || after.LedgerRows != before.LedgerRows+2 {
		t.Fatalf("retry %+v %v %+v", result, err, after)
	}
	status, err := service.SGSSCBackfillStatus(context.Background(), 0, 2)
	if err != nil || !status.HasMoreRecords || len(status.Records) != 2 || status.NextBeforeID == 0 {
		t.Fatalf("cursor %+v %v", status, err)
	}
	older, err := service.SGSSCBackfillStatus(context.Background(), status.NextBeforeID, 2)
	if err != nil || len(older.Records) != 1 || older.Records[0].ID >= status.NextBeforeID || older.HasMoreRecords {
		t.Fatalf("older cursor %+v %v", older, err)
	}
	if status.Records[0].Operator != "retry-admin" || status.Records[0].Attempt != 2 || status.Records[1].Status != "source_error" {
		t.Fatalf("earlier failure lost: %+v", status.Records)
	}
}

func TestSGSSCBackfillPostgresSourceErrorCannotImportReturnedSubset(t *testing.T) {
	db, service, member, now := sgSSCBackfillPostgresFixture(t)
	at := now.Truncate(sgSSCInterval).Add(-3 * time.Hour)
	ticket := sgSSCBackfillPostgresTicket(t, db, member, at)
	before := timingPostgresMoney(t, db, member.UserID)
	result, err := service.runSGSSCBackfill(context.Background(), func() time.Time { return now }, func(context.Context, []string) (SGSSCHistoryVerification, error) {
		return SGSSCHistoryVerification{Draws: []sourceDraw{sgSSCBackfillPostgresDraw(at)}}, errors.New("source identity invalid")
	})
	if err != nil || result.Deferred != 1 || result.Recovered != 0 || !reflect.DeepEqual(before, timingPostgresMoney(t, db, member.UserID)) {
		t.Fatalf("partial error changed state: %+v %v", result, err)
	}
	if row := sgSSCBackfillPostgresItem(t, db, ticket.Issue); row.Status != "retry" || row.LastError != "source identity invalid" {
		t.Fatal(row)
	}
	var count int64
	if err := db.Model(&lottery.Draw{}).Where("game_id = ? AND issue = ?", "sg-ssc", ticket.Issue).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("error imported a draw: %d %v", count, err)
	}
}

func TestSGSSCBackfillPostgresLeaseRestartUsesDurableDrawWithoutRefetch(t *testing.T) {
	db, service, member, now := sgSSCBackfillPostgresFixture(t)
	at := now.Truncate(sgSSCInterval).Add(-3 * time.Hour)
	ticket := sgSSCBackfillPostgresTicket(t, db, member, at)
	if _, err := service.discoverSGSSCBackfill(context.Background(), now, "fixture", "crash-request", "admin", false); err != nil {
		t.Fatal(err)
	}
	claims, err := service.claimSGSSCBackfills(context.Background(), now)
	if err != nil || len(claims) != 1 {
		t.Fatalf("claim %v %v", claims, err)
	}
	if duplicate, err := service.claimSGSSCBackfills(context.Background(), now); err != nil || len(duplicate) != 0 {
		t.Fatalf("active claim stolen %v %v", duplicate, err)
	}
	draw := sgSSCBackfillPostgresDraw(at)
	if ready, err := service.prepareSGSSCBackfill(context.Background(), claims[0], &draw, now); err != nil || !ready {
		t.Fatalf("import checkpoint %v %v", ready, err)
	}
	before := timingPostgresMoney(t, db, member.UserID)
	restart := now.Add(sgSSCBackfillLease + time.Second)
	if claimed, err := service.claimSGSSCBackfills(context.Background(), restart); err != nil || len(claimed) != 0 {
		t.Fatalf("expired attempt should back off: %v %v", claimed, err)
	}
	if _, err := service.prepareSGSSCBackfill(context.Background(), claims[0], &draw, restart); !errors.Is(err, errSGSSCBackfillLeaseLost) {
		t.Fatalf("late worker imported: %v", err)
	}
	result, err := service.runSGSSCBackfill(context.Background(), func() time.Time { return restart.Add(6 * time.Minute) }, func(context.Context, []string) (SGSSCHistoryVerification, error) {
		t.Fatal("durable trusted draw should not refetch after a crash")
		return SGSSCHistoryVerification{}, nil
	})
	after := timingPostgresMoney(t, db, member.UserID)
	if err != nil || result.Recovered != 1 || after.BalanceCents != before.BalanceCents+200 || after.LedgerRows != before.LedgerRows+1 {
		t.Fatalf("restart recovery %+v %v %+v", result, err, after)
	}
	status, err := service.SGSSCBackfillStatus(context.Background(), 0, 20)
	if err != nil || len(status.Records) != 2 || status.Records[0].Status != "recovered" || status.Records[0].Imported || status.Records[1].Status != "interrupted" || !status.Records[1].Imported {
		t.Fatalf("crash evidence lost %+v %v", status, err)
	}
	if err := service.finishSGSSCBackfill(context.Background(), claims[0], restart, "completed", "recovered", "", 99); !errors.Is(err, errSGSSCBackfillLeaseLost) {
		t.Fatalf("late worker overwrote receipt %v", err)
	}
	if row := sgSSCBackfillPostgresItem(t, db, ticket.Issue); row.Attempts != 2 || row.Status != "completed" {
		t.Fatal(row)
	}
}

func TestSGSSCBackfillPostgresKeepsLegacyConflictAndExpiredMissingBlocked(t *testing.T) {
	for _, scenario := range []string{"legacy", "conflict", "expired", "paused-in-flight"} {
		t.Run(scenario, func(t *testing.T) {
			db, service, member, now := sgSSCBackfillPostgresFixture(t)
			at := now.Truncate(sgSSCInterval).Add(-4 * time.Hour)
			if scenario == "expired" {
				at = at.Add(-sgSSCBackfillMaxAge)
			}
			ticket := sgSSCBackfillPostgresTicket(t, db, member, at)
			if scenario == "legacy" {
				period := lottery.Issue{GameID: "sg-ssc", Issue: ticket.Issue, SourceMode: "platform", Status: lottery.IssueStatusAwaiting, AcceptAt: at.Add(-sgSSCInterval), SealAt: at.Add(-30 * time.Second)}
				if err := db.Create(&period).Error; err != nil {
					t.Fatal(err)
				}
			}
			before := timingPostgresMoney(t, db, member.UserID)
			result, err := service.runSGSSCBackfill(context.Background(), func() time.Time { return now }, func(context.Context, []string) (SGSSCHistoryVerification, error) {
				if scenario == "legacy" || scenario == "expired" {
					t.Fatal("blocked period contacted sources")
				}
				if scenario == "conflict" {
					existing := lottery.Draw{GameID: "sg-ssc", Issue: ticket.Issue, DrawAt: at, Numbers: "1,2,3,4,5", SourceRevision: sgSSCSourceRevision, ConversionRevision: sgSSCConversionRevision}
					if err := db.Create(&existing).Error; err != nil {
						t.Fatal(err)
					}
				} else if err := db.Model(&lottery.Game{}).Where("id = ?", "sg-ssc").Update("enabled", false).Error; err != nil {
					t.Fatal(err)
				}
				return SGSSCHistoryVerification{Draws: []sourceDraw{sgSSCBackfillPostgresDraw(at)}}, nil
			})
			wantStatus := "blocked"
			if scenario == "paused-in-flight" {
				wantStatus = "retry"
			}
			if err != nil || result.Recovered != 0 || result.Deferred != 1 || !reflect.DeepEqual(before, timingPostgresMoney(t, db, member.UserID)) {
				t.Fatalf("unsafe progress %+v %v", result, err)
			}
			if row := sgSSCBackfillPostgresItem(t, db, ticket.Issue); row.Status != wantStatus || row.LastError == "" {
				t.Fatal(row)
			}
			var draws []lottery.Draw
			if err := db.Where("game_id = ? AND issue = ?", "sg-ssc", ticket.Issue).Find(&draws).Error; err != nil {
				t.Fatal(err)
			}
			if scenario == "conflict" {
				if len(draws) != 1 || draws[0].Numbers != "1,2,3,4,5" {
					t.Fatal("trusted conflict overwritten")
				}
			} else if len(draws) != 0 {
				t.Fatal("blocked missing result fabricated")
			}
		})
	}
}

func TestSGSSCBackfillPostgresDiscoveryCutoffAndInvalidCandidates(t *testing.T) {
	db, service, member, now := sgSSCBackfillPostgresFixture(t)
	cutoff := now.Add(-sgSSCBackfillMaxAge).Truncate(sgSSCInterval)
	sgSSCBackfillPostgresStored(t, db, cutoff.Add(-sgSSCInterval))
	sgSSCBackfillPostgresStored(t, db, cutoff.Add(3*sgSSCInterval))
	validAt := now.Truncate(sgSSCInterval).Add(-2 * time.Hour)
	valid := sgSSCBackfillPostgresTicket(t, db, member, validAt)
	// Corrupted and future snapshots are deliberately not legitimate wagers.
	// They must not fill the discovery LIMIT and starve an older eligible issue.
	for index := 0; index < sgSSCDiscoveryLimit+1; index++ {
		row := valid
		row.ID, row.Issue, row.RequestReference = 0, fmt.Sprintf("20261301%03d", index+1), fmt.Sprintf("invalid-history-%d", index)
		row.CreatedAt = valid.CreatedAt.Add(-time.Hour)
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.discoverSGSSCBackfill(context.Background(), now, "fixture", "cutoff", "admin", false); err != nil {
		t.Fatal(err)
	}
	if row := sgSSCBackfillPostgresItem(t, db, valid.Issue); row.Reason != "pending_bet" {
		t.Fatal(row)
	}
	for _, at := range []time.Time{cutoff.Add(sgSSCInterval), cutoff.Add(2 * sgSSCInterval)} {
		if row := sgSSCBackfillPostgresItem(t, db, sgSSCIssueAt(at)); row.Reason != "draw_gap" {
			t.Fatal(row)
		}
	}
	var invalidCount int64
	if err := db.Model(&lottery.SGSSCBackfillItem{}).Where("issue LIKE '202613%'").Count(&invalidCount).Error; err != nil || invalidCount != 0 {
		t.Fatalf("invalid calendar enqueued %d %v", invalidCount, err)
	}
}

func TestSGSSCBackfillPostgresTrustedOlderHistoryStillSettles(t *testing.T) {
	db, service, member, now := sgSSCBackfillPostgresFixture(t)
	at := now.Truncate(sgSSCInterval).Add(-31 * 24 * time.Hour)
	sgSSCBackfillPostgresTicket(t, db, member, at)
	sgSSCBackfillPostgresStored(t, db, at)
	result, err := service.runSGSSCBackfill(context.Background(), func() time.Time { return now }, func(context.Context, []string) (SGSSCHistoryVerification, error) {
		t.Fatal("already trusted history refetched")
		return SGSSCHistoryVerification{}, nil
	})
	if err != nil || result.Recovered != 1 {
		t.Fatalf("fetch horizon blocked trusted settlement: %+v %v", result, err)
	}
}
