package services

import (
	"backend/data/models/bet"
	apperrors "backend/errors"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"backend/data/models/lottery"
)

func TestVerifiedBingoDrawConflictPostgresFailsClosedWithoutOverwriting(t *testing.T) {
	db := timingPostgresDatabase(t)
	drawAt := time.Date(2026, 9, 2, 1, 26, 13, 0, time.UTC)
	verified := func(issue string, numbers []int) sourceDraw {
		return sourceDraw{
			Issue: issue, Numbers: numbers, DrawAt: drawAt, BingoOrderVerified: true,
			SourceRevision: bingoOrderedSourceRevision, ConversionRevision: bingoRacingAConversionVersion,
		}
	}
	legacy := []lottery.Draw{
		{GameID: "bingo-racing-a", Issue: "settled-legacy", Numbers: "1,2,3,4,5,6,7,8,9,10", DrawAt: drawAt},
		{GameID: "bingo-racing-a", Issue: "no-bet-legacy", Numbers: "1,2,3,4,5,6,7,8,9,10", DrawAt: drawAt},
		{GameID: "bingo-racing-a", Issue: "matching-legacy", Numbers: "3,5,9,1,7,10,6,2,8,4", DrawAt: drawAt},
		{GameID: "bingo-racing-a", Issue: "wrong-time-legacy", Numbers: "3,5,9,1,7,10,6,2,8,4", DrawAt: drawAt.Add(-time.Minute)},
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	settledAt := drawAt.Add(time.Second)
	settledIssue := lottery.Issue{
		GameID: "bingo-racing-a", Issue: "settled-legacy", Status: lottery.IssueStatusSettled, SourceMode: "external",
		AcceptAt: drawAt.Add(-5 * time.Minute), SealAt: drawAt.Add(-3 * time.Second), DrawAt: &drawAt, SettledAt: &settledAt,
	}
	if err := db.Create(&settledIssue).Error; err != nil {
		t.Fatal(err)
	}
	want := []int{3, 5, 9, 1, 7, 10, 6, 2, 8, 4}
	batch := []sourceDraw{
		verified("settled-legacy", want), verified("no-bet-legacy", want),
		verified("matching-legacy", want), verified("wrong-time-legacy", want), verified("new-verified", want),
	}
	if imported, err := insertOfficialDraws(db, "bingo-racing-a", batch); err != nil || imported != 1 {
		t.Fatalf("safe legacy migration/new insert failed: imported=%d err=%v", imported, err)
	}
	var settledLegacy, corrected, matching, correctedTime, inserted lottery.Draw
	for issue, target := range map[string]*lottery.Draw{
		"settled-legacy": &settledLegacy, "no-bet-legacy": &corrected,
		"matching-legacy": &matching, "wrong-time-legacy": &correctedTime, "new-verified": &inserted,
	} {
		if err := db.First(target, "game_id = ? AND issue = ?", "bingo-racing-a", issue).Error; err != nil {
			t.Fatal(err)
		}
	}
	if settledLegacy.Numbers != legacy[0].Numbers || settledLegacy.SourceRevision != "" || settledLegacy.ConversionRevision != "" {
		t.Fatalf("settled legacy financial history was rewritten or falsely verified: %+v", settledLegacy)
	}
	for _, row := range []lottery.Draw{corrected, matching, correctedTime, inserted} {
		if row.Numbers != joinNumbers(want) || !row.DrawAt.Equal(drawAt) || row.SourceRevision != bingoOrderedSourceRevision || row.ConversionRevision != bingoRacingAConversionVersion {
			t.Fatalf("safe draw was not corrected/claimed with exact revisions: %+v", row)
		}
	}

	current := lottery.Draw{
		GameID: "bingo-racing-a", Issue: "current-conflict", Numbers: "1,2,3,4,5,6,7,8,9,10", DrawAt: drawAt,
		SourceRevision: bingoOrderedSourceRevision, ConversionRevision: bingoRacingAConversionVersion,
	}
	if err := db.Create(&current).Error; err != nil {
		t.Fatal(err)
	}
	if imported, err := insertOfficialDraws(db, current.GameID, []sourceDraw{verified(current.Issue, want)}); imported != 0 || !errors.Is(err, errVerifiedBingoDrawConflict) {
		t.Fatalf("current revision conflict was not blocked: imported=%d err=%v", imported, err)
	}
	currentTimeConflict := lottery.Draw{
		GameID: "bingo-racing-a", Issue: "current-time-conflict", Numbers: joinNumbers(want), DrawAt: drawAt.Add(-time.Minute),
		SourceRevision: bingoOrderedSourceRevision, ConversionRevision: bingoRacingAConversionVersion,
	}
	if err := db.Create(&currentTimeConflict).Error; err != nil {
		t.Fatal(err)
	}
	if imported, err := insertOfficialDraws(db, currentTimeConflict.GameID, []sourceDraw{verified(currentTimeConflict.Issue, want)}); imported != 0 || !errors.Is(err, errVerifiedBingoDrawConflict) {
		t.Fatalf("current revision draw-time conflict was not blocked: imported=%d err=%v", imported, err)
	}

	room := timingPostgresRoom(t, db, "bingo_conflict_room", "792601")
	member := timingPostgresMember(t, db, room, "bingo_conflict_member")
	pendingLegacy := lottery.Draw{GameID: "bingo-racing-a", Issue: "pending-legacy", Numbers: "1,2,3,4,5,6,7,8,9,10", DrawAt: drawAt}
	if err := db.Create(&pendingLegacy).Error; err != nil {
		t.Fatal(err)
	}
	ticket := bet.Bet{
		WorkspaceID: room.ID, GameID: pendingLegacy.GameID, Issue: pendingLegacy.Issue, RoomScope: room.Scope,
		UserID: member.UserID, Username: member.Username, PlayCode: "rank", PlayName: "名次", Position: 1, Selection: "3",
		RuleVersion: "racing-v2", RequestReference: "bingo-conflict-pending", AmountCents: 100, Odds: 2, Status: "pending",
	}
	if err := db.Create(&ticket).Error; err != nil {
		t.Fatal(err)
	}
	if imported, err := insertOfficialDraws(db, pendingLegacy.GameID, []sourceDraw{verified(pendingLegacy.Issue, want)}); imported != 0 || !errors.Is(err, errVerifiedBingoDrawConflict) {
		t.Fatalf("legacy draw with financial evidence was not blocked: imported=%d err=%v", imported, err)
	}
	var unchanged lottery.Draw
	if err := db.First(&unchanged, pendingLegacy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if unchanged.Numbers != pendingLegacy.Numbers || unchanged.SourceRevision != "" {
		t.Fatalf("blocked pending legacy history was changed: %+v", unchanged)
	}

	ordinary := verified("new-verified", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	ordinary.BingoOrderVerified = false
	if imported, err := insertOfficialDraws(db, "bingo-racing-a", []sourceDraw{ordinary}); err != nil || imported != 0 {
		t.Fatalf("ordinary source conflict behavior changed: imported=%d err=%v", imported, err)
	}
}

func TestExternalOrderedBingoRejectsManualAndRandomPublish(t *testing.T) {
	db := timingPostgresDatabase(t)
	var before lottery.Game
	if err := db.First(&before, "id = ?", "bingo-racing-a").Error; err != nil {
		t.Fatal(err)
	}
	service := NewBetAdminService(db)
	for index, numbers := range [][]int{nil, {1, 2, 3, 4, 5, 6, 7, 8, 9, 10}} {
		issue := "manual-external-" + string(rune('1'+index))
		if result, err := service.PublishDraw(before.ID, issue, numbers, "forbidden fixture"); result != nil || apperrors.GetErrorCode(err) != "EXTERNAL_DRAW_MANUAL_FORBIDDEN" {
			t.Fatalf("external draw publish was not rejected: result=%+v err=%v", result, err)
		}
		var count int64
		if err := db.Model(&lottery.Draw{}).Where("game_id = ? AND issue = ?", before.ID, issue).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("rejected external publish wrote a draw: count=%d err=%v", count, err)
		}
	}
	var after lottery.Game
	if err := db.First(&after, "id = ?", before.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.SyncStatus != before.SyncStatus || after.LastSyncError != before.LastSyncError || after.NextIssue != before.NextIssue || !after.NextDrawAt.Equal(before.NextDrawAt) {
		t.Fatalf("rejected manual publish marked source healthy or moved schedule: before=%+v after=%+v", before, after)
	}
}

func TestOrderedBingoSettlementRecoveryRequiresCurrentDrawRevision(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "bingo_recovery_room", "792602")
	member := timingPostgresMember(t, db, room, "bingo_recovery_member")
	const issue = "ordered-recovery-1"
	drawAt := time.Now().UTC().Add(-time.Minute)
	draw := lottery.Draw{GameID: "bingo-racing-a", Issue: issue, Numbers: "1,2,3,4,5,6,7,8,9,10", DrawAt: drawAt}
	if err := db.Create(&draw).Error; err != nil {
		t.Fatal(err)
	}
	ticket := bet.Bet{
		WorkspaceID: room.ID, GameID: draw.GameID, Issue: issue, RoomScope: room.Scope,
		UserID: member.UserID, Username: member.Username, PlayCode: "ball_1_5", PlayName: "指定名次号码",
		Position: 1, Selection: "3", RuleVersion: "racing-v2", RequestReference: "ordered-recovery-ticket",
		AmountCents: 100, Odds: 2, Status: "pending",
	}
	if err := db.Create(&ticket).Error; err != nil {
		t.Fatal(err)
	}
	before := timingPostgresMoney(t, db, member.UserID)
	service := NewBetAdminService(db)
	service.suppressNotifications = true
	result, err := service.RecoverSettlementBacklog(context.Background(), 20, "ordered revision recovery")
	if err != nil || result.SettledIssues != 0 || len(result.Failures) == 0 {
		t.Fatalf("legacy ordered draw was recovered financially: result=%+v err=%v", result, err)
	}
	if !strings.Contains(result.Failures[0].Error, "不是当前双源验证版本") {
		t.Fatalf("recovery failure did not explain the provenance gate: %+v", result.Failures)
	}
	if after := timingPostgresMoney(t, db, member.UserID); after != before {
		t.Fatalf("unverified recovery changed balance/ledger: before=%+v after=%+v", before, after)
	}
	if err := db.First(&ticket, ticket.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ticket.Status != "pending" || ticket.ReconciliationStatus != "abnormal" || ticket.ReconciliationNote == "" {
		t.Fatalf("unverified ticket was not kept pending for visible review: %+v", ticket)
	}
	var lifecycle lottery.Issue
	if err := db.First(&lifecycle, "game_id = ? AND issue = ?", draw.GameID, issue).Error; err != nil {
		t.Fatal(err)
	}
	if lifecycle.Status != lottery.IssueStatusError || lifecycle.LastError == "" {
		t.Fatalf("unverified draw did not create a visible issue error: %+v", lifecycle)
	}

	if err := db.Model(&lottery.Draw{}).Where("id = ?", draw.ID).Updates(map[string]any{
		"numbers": "3,5,9,1,7,10,6,2,8,4", "source_revision": bingoOrderedSourceRevision,
		"conversion_revision": bingoRacingAConversionVersion,
	}).Error; err != nil {
		t.Fatal(err)
	}
	result, err = service.RecoverSettlementBacklog(context.Background(), 20, "verified ordered revision recovery")
	if err != nil || result.SettledIssues != 1 || len(result.Failures) != 0 {
		t.Fatalf("current verified revision did not recover: result=%+v err=%v", result, err)
	}
	if err := db.First(&ticket, ticket.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ticket.Status == "pending" || ticket.SettledAt == nil {
		t.Fatalf("verified current draw was not settled: %+v", ticket)
	}
}

func TestOrderedBingoRecoverySkipsUnverifiedManualQueueWithoutStarvingVerifiedWork(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "bingo_recovery_fair_room", "792603")
	member := timingPostgresMember(t, db, room, "bingo_recovery_fair_member")
	now := time.Now().UTC().Truncate(time.Second)
	legacyDraw := lottery.Draw{
		GameID: "bingo-racing-a", Issue: "ordered-manual-old", Numbers: "1,2,3,4,5,6,7,8,9,10", DrawAt: now.Add(-10 * time.Minute),
	}
	verifiedDraw := lottery.Draw{
		GameID: "bingo-racing-a", Issue: "ordered-verified-new", Numbers: "3,5,9,1,7,10,6,2,8,4", DrawAt: now.Add(-5 * time.Minute),
		SourceRevision: bingoOrderedSourceRevision, ConversionRevision: bingoRacingAConversionVersion,
	}
	if err := db.Create(&[]lottery.Draw{legacyDraw, verifiedDraw}).Error; err != nil {
		t.Fatal(err)
	}
	tickets := []bet.Bet{
		{
			WorkspaceID: room.ID, GameID: legacyDraw.GameID, Issue: legacyDraw.Issue, RoomScope: room.Scope,
			UserID: member.UserID, Username: member.Username, PlayCode: "rank", Position: 1, Selection: "1",
			RuleVersion: "racing-v2", RequestReference: "ordered-manual-old", AmountCents: 100, Odds: 2, Status: "pending",
			ReconciliationStatus: "abnormal", ReconciliationNote: "manual review", CreatedAt: now.Add(-20 * time.Minute),
		},
		{
			WorkspaceID: room.ID, GameID: verifiedDraw.GameID, Issue: verifiedDraw.Issue, RoomScope: room.Scope,
			UserID: member.UserID, Username: member.Username, PlayCode: "rank", Position: 1, Selection: "3",
			RuleVersion: "racing-v2", RequestReference: "ordered-verified-new", AmountCents: 100, Odds: 2, Status: "pending",
			ReconciliationStatus: "abnormal", ReconciliationNote: "retry after verified import", CreatedAt: now.Add(-10 * time.Minute),
		},
	}
	if err := db.Create(&tickets).Error; err != nil {
		t.Fatal(err)
	}
	service := NewBetAdminService(db)
	candidates, err := service.pendingSettlementCandidates(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Issue != verifiedDraw.Issue {
		t.Fatalf("old unverified manual queue starved verified recovery: %+v", candidates)
	}

	legacyErrorDraw := lottery.Draw{
		GameID: "bingo-racing-a", Issue: "ordered-error-old", Numbers: legacyDraw.Numbers, DrawAt: now.Add(-30 * time.Minute),
	}
	verifiedErrorDraw := lottery.Draw{
		GameID: "bingo-racing-a", Issue: "ordered-error-verified", Numbers: verifiedDraw.Numbers, DrawAt: now.Add(-25 * time.Minute),
		SourceRevision: bingoOrderedSourceRevision, ConversionRevision: bingoRacingAConversionVersion,
	}
	if err := db.Create(&[]lottery.Draw{legacyErrorDraw, verifiedErrorDraw}).Error; err != nil {
		t.Fatal(err)
	}
	legacySeal := legacyErrorDraw.DrawAt.Add(-3 * time.Second)
	verifiedSeal := verifiedErrorDraw.DrawAt.Add(-3 * time.Second)
	errorIssues := []lottery.Issue{
		{GameID: legacyErrorDraw.GameID, Issue: legacyErrorDraw.Issue, Status: lottery.IssueStatusError, SourceMode: "external", AcceptAt: legacySeal.Add(-5 * time.Minute), SealAt: legacySeal, DrawAt: &legacyErrorDraw.DrawAt, LastError: "manual review"},
		{GameID: verifiedErrorDraw.GameID, Issue: verifiedErrorDraw.Issue, Status: lottery.IssueStatusError, SourceMode: "external", AcceptAt: verifiedSeal.Add(-5 * time.Minute), SealAt: verifiedSeal, DrawAt: &verifiedErrorDraw.DrawAt, LastError: "retry verified"},
	}
	if err := db.Create(&errorIssues).Error; err != nil {
		t.Fatal(err)
	}
	marked, failures, err := service.recoverStaleIssueRows(context.Background(), 1, "fairness fixture")
	if err != nil || marked != 0 || len(failures) != 0 {
		t.Fatalf("verified stale issue recovery failed: marked=%d failures=%+v err=%v", marked, failures, err)
	}
	var legacyIssue, verifiedIssue lottery.Issue
	if err := db.First(&legacyIssue, errorIssues[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&verifiedIssue, errorIssues[1].ID).Error; err != nil {
		t.Fatal(err)
	}
	if legacyIssue.Status != lottery.IssueStatusError || verifiedIssue.Status != lottery.IssueStatusSettled {
		t.Fatalf("unverified error consumed retry capacity or verified error stayed stuck: legacy=%+v verified=%+v", legacyIssue, verifiedIssue)
	}
}

func TestBingoOrderedSourceRevisionRunsDuringNormalCatalogBootstrap(t *testing.T) {
	db := timingPostgresDatabase(t)
	orderedIDs := []string{"bingo-ssc-1", "bingo-racing-a", "bingo-mark-six"}
	if err := db.Model(&lottery.Game{}).Where("id IN ?", orderedIDs).Updates(map[string]any{
		"source_kind": "external", "source_name": "168开奖网",
		"source_url":  "https://kj138138.com/view/api/index.html",
		"sync_status": "ok", "last_sync_error": "",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&lottery.Game{}).Where("id = ?", "bingo-racing-b").Updates(map[string]any{
		"source_kind": "external", "source_name": "operator-b-source",
		"source_url": "https://operator.invalid/b", "sync_status": "ok",
		"last_sync_error": "",
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := SeedLotteryCatalog(db, LotterySeedOptions{}); err != nil {
		t.Fatal("normal catalog bootstrap did not apply ordered source revision:", err)
	}
	for _, id := range orderedIDs {
		var game lottery.Game
		if err := db.First(&game, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		if game.SourceKind != "external" || game.SourceName != bingoVerifiedSourceName || game.SourceURL != bingoVerifiedSourceURL ||
			game.SyncStatus != "stale" || game.LastSyncError != bingoOrderPendingMessage || sourceHealthyForGame(&game) {
			t.Fatalf("%s remained healthy on its legacy source after bootstrap: %+v", id, game)
		}
	}
	var independent lottery.Game
	if err := db.First(&independent, "id = ?", "bingo-racing-b").Error; err != nil {
		t.Fatal(err)
	}
	if independent.SourceName != "operator-b-source" || independent.SourceURL != "https://operator.invalid/b" || independent.SyncStatus != "ok" || !sourceHealthyForGame(&independent) {
		t.Fatalf("normal bootstrap changed an order-independent product: %+v", independent)
	}

	// Once a complete dual-source import has written ok and cleared the error,
	// later process starts must preserve that verified state.
	if err := db.Model(&lottery.Game{}).Where("id IN ?", orderedIDs).Updates(map[string]any{
		"sync_status": "ok", "last_sync_error": "",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := SeedLotteryCatalog(db, LotterySeedOptions{}); err != nil {
		t.Fatal("repeat catalog bootstrap failed:", err)
	}
	for _, id := range orderedIDs {
		var game lottery.Game
		if err := db.First(&game, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		if game.SyncStatus != "ok" || game.LastSyncError != "" || !sourceHealthyForGame(&game) {
			t.Fatalf("%s verified source was downgraded on repeat bootstrap: %+v", id, game)
		}
	}
}
