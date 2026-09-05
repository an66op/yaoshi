package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"
)

func importSettlementFixture(issue string, drawAt time.Time) sourceDraw {
	return sourceDraw{
		Issue: issue, DrawAt: drawAt,
		Numbers: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	}
}

func storeImportSettlementDraws(t *testing.T, db *gorm.DB, gameID string, draws []sourceDraw) {
	t.Helper()
	rows := make([]lottery.Draw, 0, len(draws))
	for _, draw := range draws {
		rows = append(rows, lottery.Draw{
			GameID: gameID, Issue: draw.Issue, Numbers: joinNumbers(draw.Numbers), DrawAt: draw.DrawAt,
		})
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal("store imported draw fixtures:", err)
	}
}

func TestSettleImportedDrawBatchPostgresSkipsDrawOnlyHistory(t *testing.T) {
	db := timingPostgresDatabase(t)
	const gameID = "speed-racing"
	drawAt := time.Date(2026, 9, 5, 1, 0, 0, 0, time.UTC)
	draws := make([]sourceDraw, 0, 120)
	issues := make([]string, 0, 120)
	for index := 0; index < 120; index++ {
		issue := fmt.Sprintf("history-only-%03d", index+1)
		issues = append(issues, issue)
		draws = append(draws, importSettlementFixture(issue, drawAt.Add(time.Duration(index)*time.Minute)))
	}
	storeImportSettlementDraws(t, db, gameID, draws)

	queryCount := 0
	const callbackName = "test:count_import_settlement_batch_queries"
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(*gorm.DB) {
		queryCount++
	}); err != nil {
		t.Fatal("register query counter:", err)
	}
	settleImportedDrawBatch(context.Background(), db, gameID, draws)
	if err := db.Callback().Query().Remove(callbackName); err != nil {
		t.Fatal("remove query counter:", err)
	}
	if queryCount != 2 {
		t.Fatalf("draw-only history issued %d reads, want exactly two bulk candidate reads", queryCount)
	}

	var lifecycleCount int64
	if err := db.Model(&lottery.Issue{}).Where("game_id = ? AND issue IN ?", gameID, issues).Count(&lifecycleCount).Error; err != nil {
		t.Fatal(err)
	}
	if lifecycleCount != 0 {
		t.Fatalf("draw-only history fabricated %d lifecycle rows", lifecycleCount)
	}
}

func TestSettleImportedDrawBatchPostgresProcessesPendingAndUnfinishedOnly(t *testing.T) {
	db := timingPostgresDatabase(t)
	const gameID = "speed-racing"
	drawAt := time.Date(2026, 9, 5, 2, 0, 0, 0, time.UTC)
	draws := []sourceDraw{
		importSettlementFixture("unfinished-issue", drawAt),
		importSettlementFixture("pending-bet-issue", drawAt.Add(time.Minute)),
		importSettlementFixture("already-settled-issue", drawAt.Add(2*time.Minute)),
		importSettlementFixture("display-history-issue", drawAt.Add(3*time.Minute)),
	}
	storeImportSettlementDraws(t, db, gameID, draws)

	unfinished := lottery.Issue{
		GameID: gameID, Issue: draws[0].Issue, Status: lottery.IssueStatusAwaiting, SourceMode: "external",
		AcceptAt: draws[0].DrawAt.Add(-time.Minute), SealAt: draws[0].DrawAt.Add(-3 * time.Second),
	}
	settledAt := drawAt.Add(-time.Minute)
	alreadySettled := lottery.Issue{
		GameID: gameID, Issue: draws[2].Issue, Status: lottery.IssueStatusSettled, SourceMode: "external",
		AcceptAt: draws[2].DrawAt.Add(-time.Minute), SealAt: draws[2].DrawAt.Add(-3 * time.Second),
		DrawAt: &draws[2].DrawAt, SettledAt: &settledAt,
	}
	if err := db.Create(&unfinished).Error; err != nil {
		t.Fatal("create unfinished lifecycle:", err)
	}
	if err := db.Create(&alreadySettled).Error; err != nil {
		t.Fatal("create settled lifecycle:", err)
	}
	var settledBefore lottery.Issue
	if err := db.First(&settledBefore, alreadySettled.ID).Error; err != nil {
		t.Fatal(err)
	}

	room := timingPostgresRoom(t, db, "batch_settlement_tenant", "76931")
	member := timingPostgresMember(t, db, room, "batch_settlement_member")
	ticket := bet.Bet{
		WorkspaceID: room.ID, GameID: gameID, Issue: draws[1].Issue, RoomScope: room.Scope,
		UserID: member.UserID, Username: member.Username, PlayCode: "ball_1_5", PlayName: "指定名次号码",
		RuleVersion: "racing-v2", Position: 1, Selection: "1", AmountCents: 200, Odds: 9.9, Status: "pending",
	}
	if err := db.Create(&ticket).Error; err != nil {
		t.Fatal("create pending bet:", err)
	}

	settleImportedDrawBatch(context.Background(), db, gameID, draws)

	var unfinishedAfter, pendingAfter, settledAfter lottery.Issue
	if err := db.Where("game_id = ? AND issue = ?", gameID, draws[0].Issue).First(&unfinishedAfter).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("game_id = ? AND issue = ?", gameID, draws[1].Issue).First(&pendingAfter).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&settledAfter, alreadySettled.ID).Error; err != nil {
		t.Fatal(err)
	}
	if unfinishedAfter.Status != lottery.IssueStatusSettled || unfinishedAfter.SettledAt == nil {
		t.Fatalf("unfinished live lifecycle was not completed: %+v", unfinishedAfter)
	}
	if pendingAfter.Status != lottery.IssueStatusSettled || pendingAfter.SettledAt == nil {
		t.Fatalf("pending-bet lifecycle was not created and completed: %+v", pendingAfter)
	}
	if settledAfter.Status != settledBefore.Status || !settledAfter.UpdatedAt.Equal(settledBefore.UpdatedAt) ||
		settledAfter.SettledAt == nil || settledBefore.SettledAt == nil || !settledAfter.SettledAt.Equal(*settledBefore.SettledAt) {
		t.Fatalf("already-settled lifecycle was rewritten: before=%+v after=%+v", settledBefore, settledAfter)
	}

	var storedTicket bet.Bet
	if err := db.First(&storedTicket, ticket.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedTicket.Status != "won" {
		t.Fatalf("pending ticket was not settled from the imported draw: %+v", storedTicket)
	}
	var historyLifecycleCount int64
	if err := db.Model(&lottery.Issue{}).Where("game_id = ? AND issue = ?", gameID, draws[3].Issue).Count(&historyLifecycleCount).Error; err != nil {
		t.Fatal(err)
	}
	if historyLifecycleCount != 0 {
		t.Fatalf("display-only history fabricated %d lifecycle rows", historyLifecycleCount)
	}
}
