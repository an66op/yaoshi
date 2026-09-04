package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestSGSSCIntegrationPostgresCutoverPreservesData(t *testing.T) {
	db := timingPostgresDatabase(t)
	var game lottery.Game
	if err := db.First(&game, "id = ?", "sg-ssc").Error; err != nil {
		t.Fatal(err)
	}
	if !sgSSCSourceBound(&game) || game.TimingSource != "pending" || !game.NextDrawAt.IsZero() || game.NextIssue != "" || sourceHealthyForGame(&game) {
		t.Fatalf("fresh SG was not fail-closed: %+v", game)
	}
	if err := seedDeterministicHistory(db, game, seedGame{}, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	var fixtureCount int64
	if err := db.Model(&lottery.Draw{}).Where("game_id = ?", game.ID).Count(&fixtureCount).Error; err != nil || fixtureCount != 0 {
		t.Fatalf("SG debug initialization invented results: %d %v", fixtureCount, err)
	}
	old := lottery.Draw{GameID: game.ID, Issue: "old-platform", Numbers: "1,2,3,4,5", DrawAt: time.Now().Add(-time.Hour)}
	if err := db.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&game).Updates(map[string]any{"source_kind": "platform", "source_name": "王者开奖", "source_url": "", "sync_status": "ok",
		"next_issue": "old-platform-next", "next_draw_at": time.Now().Add(time.Minute), "timing_source": "configured", "enabled": false, "odds_config_revision": 31}).Error; err != nil {
		t.Fatal(err)
	}
	if err := EnsureSGSSCVerifiedSource(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&game, "id = ?", game.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !sgSSCSourceBound(&game) || game.Enabled || game.OddsConfigRevision != 31 || game.NextIssue != "" || !game.NextDrawAt.IsZero() {
		t.Fatalf("cutover changed unrelated data or retained old schedule: %+v", game)
	}
	var unchanged lottery.Draw
	if err := db.First(&unchanged, old.ID).Error; err != nil || unchanged.Numbers != old.Numbers || unchanged.SourceRevision != "" {
		t.Fatalf("cutover relabeled historical draw: %+v %v", unchanged, err)
	}
	var visible int64
	if err := trustedDrawsForGame(db.Model(&lottery.Draw{}), game.ID).Count(&visible).Error; err != nil || visible != 0 {
		t.Fatalf("platform result exposed as verified: %d %v", visible, err)
	}
}

func TestSGSSCIntegrationPostgresLegacyIsolationAndConflicts(t *testing.T) {
	db := timingPostgresDatabase(t)
	batch := sgSSCIntegrationBatch(time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC))
	old := []lottery.Draw{
		{GameID: "sg-ssc", Issue: batch[0].Issue, Numbers: joinNumbers(batch[0].Numbers), DrawAt: batch[0].DrawAt},
		{GameID: "sg-ssc", Issue: batch[1].Issue, Numbers: "0,0,0,0,0", DrawAt: batch[1].DrawAt},
	}
	if err := db.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	oldIssue := lottery.Issue{GameID: "sg-ssc", Issue: batch[2].Issue, SourceMode: "platform", Status: lottery.IssueStatusSettled,
		AcceptAt: batch[2].DrawAt.Add(-sgSSCInterval), SealAt: batch[2].DrawAt.Add(-3 * time.Second), DrawAt: &batch[2].DrawAt, SettledAt: &batch[2].DrawAt}
	if err := db.Create(&oldIssue).Error; err != nil {
		t.Fatal(err)
	}
	room := timingPostgresRoom(t, db, "sg_isolation", "783003")
	member := timingPostgresMember(t, db, room, "sg_old_member")
	ticket := bet.Bet{WorkspaceID: room.ID, GameID: "sg-ssc", Issue: batch[3].Issue, RoomScope: room.Scope, UserID: member.UserID,
		Username: member.Username, PlayCode: "two_sided", PlayName: "两面", Position: 1, Selection: "大", AmountCents: 100,
		Odds: 2, Status: "pending", RuleVersion: "digits5-v3", RequestReference: "sg-old-orphan"}
	if err := db.Create(&ticket).Error; err != nil {
		t.Fatal(err)
	}
	// An archive-only period must be just as strong evidence as a live ticket.
	if err := db.Exec(`INSERT INTO lottery_bet_archives SELECT (jsonb_populate_record(NULL::lottery_bet_archives,
		to_jsonb(b) || jsonb_build_object('id', 9800001, 'issue', ?::text, 'source_json', to_jsonb(b),
		'row_hash', md5(to_jsonb(b)::text), 'archived_at', now(), 'cleanup_request_id', 'sg-archive-fixture'))).*
		FROM lottery_bets b WHERE b.id = ?`, batch[4].Issue, ticket.ID).Error; err != nil {
		t.Fatal(err)
	}
	if imported, err := insertOfficialDraws(db, "sg-ssc", batch); err != nil || imported != len(batch)-5 {
		t.Fatalf("fresh periods not isolated from legacy: imported=%d err=%v", imported, err)
	}
	for _, row := range old {
		var after lottery.Draw
		if err := db.First(&after, row.ID).Error; err != nil || after.SourceRevision != "" || after.Numbers != row.Numbers {
			t.Fatalf("old draw claimed/overwritten: %+v %v", after, err)
		}
	}
	for index := 2; index <= 4; index++ {
		if err := sgSSCIssueEvidenceError(db, batch[index].Issue, nil); apperrors.GetErrorCode(err) != "DRAW_SOURCE_UNVERIFIED" {
			t.Fatalf("old lifecycle/live/archive accepted index=%d: %v", index, err)
		}
	}
	var game lottery.Game
	if err := db.First(&game, "id = ?", "sg-ssc").Error; err != nil {
		t.Fatal(err)
	}
	game.NextIssue, game.NextDrawAt = oldIssue.Issue, batch[2].DrawAt
	view, err := NewBetAdminService(db).EnsureCurrentIssue(&game)
	if err != nil || view.Status != lottery.IssueStatusError || view.LastError == "" {
		t.Fatalf("legacy collision should pause only SG, not fail catalog: %+v %v", view, err)
	}
	stored := rolloverPostgresIssue(t, db, game.ID, oldIssue.Issue)
	if stored.Status != oldIssue.Status || stored.SourceMode != oldIssue.SourceMode || stored.DrawAt == nil || stored.SettledAt == nil {
		t.Fatalf("view mutated legacy lifecycle: %+v", stored)
	}
	if err := db.Model(&lottery.Game{}).Where("id = ?", game.ID).Updates(map[string]any{"next_issue": game.NextIssue, "next_draw_at": game.NextDrawAt}).Error; err != nil {
		t.Fatal(err)
	}
	catalog, err := NewLotteryService(db).ListGames()
	if err != nil || len(catalog) < 2 {
		t.Fatalf("SG collision broke the entire catalog: games=%d err=%v", len(catalog), err)
	}
	for _, summary := range catalog {
		if summary.ID == game.ID && summary.IssueStatus != lottery.IssueStatusError {
			t.Fatalf("SG collision not paused in catalog: %+v", summary)
		}
	}
	conflict := append([]sourceDraw(nil), batch...)
	conflict[10].Numbers = []int{9, 9, 9, 9, 9}
	if imported, err := insertOfficialDraws(db, "sg-ssc", conflict); err == nil || imported != 0 {
		t.Fatalf("verified history silently overwritten: %d %v", imported, err)
	}
	// A collision at the latest period cannot be advertised as a successful poll.
	latestCollision := sgSSCIntegrationBatch(batch[0].DrawAt)
	if imported, err := insertOfficialDraws(db, "sg-ssc", latestCollision); apperrors.GetErrorCode(err) != "DRAW_SOURCE_UNVERIFIED" || imported != 0 {
		t.Fatalf("latest platform period promoted: %d %v", imported, err)
	}
}

func TestSGSSCIntegrationPostgresSyncAndSettlement(t *testing.T) {
	db := timingPostgresDatabase(t)
	batch := sgSSCIntegrationBatch(time.Now())
	latest := batch[len(batch)-1]
	if time.Until(latest.NextDrawAt) < 15*time.Second {
		t.Skip("fixture crossed the five-minute boundary; rerun in the next period")
	}
	svc := NewLotteryService(db)
	result := svc.syncOfficialGameWithPublisher(context.Background(), "sg-ssc", func(context.Context) ([]sourceDraw, error) { return batch, nil }, func(lottery.Game) {})
	if result.Status != "ok" || result.Imported != sgSSCWindowSize || result.LatestIssue != latest.Issue {
		t.Fatalf("verified batch did not update live schedule: %+v", result)
	}
	var game lottery.Game
	if err := db.First(&game, "id = ?", "sg-ssc").Error; err != nil {
		t.Fatal(err)
	}
	if !sourceHealthyForGame(&game) || game.NextIssue != latest.NextIssue || !game.NextDrawAt.Equal(latest.NextDrawAt) {
		t.Fatalf("upstream schedule/health not materialized: %+v", game)
	}
	failed := svc.syncOfficialGameWithPublisher(context.Background(), game.ID, func(context.Context) ([]sourceDraw, error) { return nil, errors.New("fixture station mismatch") }, func(lottery.Game) {})
	if err := db.First(&game, "id = ?", game.ID).Error; err != nil {
		t.Fatal(err)
	}
	if failed.Status != "error" || sourceHealthyForGame(&game) || game.NextIssue != latest.NextIssue {
		t.Fatalf("source failure fell back to generated schedule: %+v %+v", failed, game)
	}
	room := timingPostgresRoom(t, db, "sg_settlement", "783004")
	member := timingPostgresMember(t, db, room, "sg_settle_member")
	service := NewBetAdminService(db)
	service.suppressNotifications = true
	for index, revision := range []string{sgSSCSourceRevision, ""} {
		at := latest.DrawAt.Add(-time.Duration(48+index) * sgSSCInterval)
		issue := sgSSCIssueAt(at)
		draw := lottery.Draw{GameID: game.ID, Issue: issue, Numbers: "6,5,8,3,0", DrawAt: at, SourceRevision: sgSSCSourceRevision, ConversionRevision: sgSSCConversionRevision}
		if err := db.Create(&draw).Error; err != nil {
			t.Fatal(err)
		}
		ticket := bet.Bet{WorkspaceID: room.ID, GameID: game.ID, Issue: issue, RoomScope: room.Scope, UserID: member.UserID,
			Username: member.Username, PlayCode: "two_sided", PlayName: "两面", Position: 1, Selection: "大", AmountCents: 100,
			Odds: 2, Status: "pending", RuleVersion: "digits5-v3", DrawSourceRevision: revision, RequestReference: fmt.Sprintf("sg-settle-%d", index)}
		if err := db.Create(&ticket).Error; err != nil {
			t.Fatal(err)
		}
		before := timingPostgresMoney(t, db, member.UserID)
		settled, err := service.SettleIssue(game.ID, issue, "sg-fixture")
		after := timingPostgresMoney(t, db, member.UserID)
		if revision == "" {
			if settled != nil || apperrors.GetErrorCode(err) != "DRAW_SOURCE_UNVERIFIED" || before != after {
				t.Fatalf("old ticket joined new draw: %+v %v before=%+v after=%+v", settled, err, before, after)
			}
			candidates, err := service.pendingSettlementCandidates(context.Background(), 100)
			if err != nil {
				t.Fatal(err)
			}
			for _, candidate := range candidates {
				if candidate.GameID == game.ID && candidate.Issue == issue {
					t.Fatal("quarantined legacy ticket repeatedly re-entered recovery")
				}
			}
			continue
		}
		if err != nil || settled.Won != 1 || after.BalanceCents != before.BalanceCents+200 || after.LedgerRows != before.LedgerRows+1 {
			t.Fatalf("verified source snapshot not settled once: %+v %v before=%+v after=%+v", settled, err, before, after)
		}
		if _, err := service.SettleIssue(game.ID, issue, "sg-fixture-retry"); err != nil || !reflect.DeepEqual(timingPostgresMoney(t, db, member.UserID), after) {
			t.Fatalf("verified settlement retry paid twice: %v", err)
		}
	}
}
