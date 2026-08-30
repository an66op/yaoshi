package services

import (
	"backend/data/models/chat"
	"backend/data/models/lottery"
	membernotify "backend/data/models/notify"
	"context"
	"reflect"
	"strings"
	"testing"
)

// The shared fixture accepts only an empty, explicitly named loopback test
// database, with all schema/data changes rolled back after the test. No
// production connection, external draw source or spectator bet is needed.
func TestSettlementPushPostgresSpectatorsKeepDistinctDrawHistory(t *testing.T) {
	db := timingPostgresDatabase(t)
	roomA := timingPostgresRoom(t, db, "push_spectator_tenant_a", "76701")
	roomB := timingPostgresRoom(t, db, "push_spectator_tenant_b", "76702")
	_ = timingPostgresMember(t, db, roomA, "push_spectator_a")
	_ = timingPostgresMember(t, db, roomB, "push_spectator_b")
	baseline := make(map[string]int64)
	for _, table := range []string{"lottery_bets", "member_chat_messages", "member_notifications", "user_balance_transactions"} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		baseline[table] = count
	}
	service := NewLotteryService(db)
	settler := NewBetAdminService(db)
	wantDraws := make([]lottery.Draw, 0, 2)
	for index, issue := range []string{"94101", "94102"} {
		game, draw := rolloverPostgresGame(t, db, issue)
		if index == 1 {
			draw.Numbers = []int{8, 5, 1, 9, 7, 6, 10, 3, 2, 4}
		}
		published := 0
		fetch := func(context.Context) ([]sourceDraw, error) { return []sourceDraw{draw}, nil }
		publish := func(lottery.Game) { published++ }
		result := service.syncOfficialGameWithPublisher(context.Background(), game.ID, fetch, publish)
		if result.Status != "ok" || result.Imported != 1 || published != 1 {
			t.Fatalf("zero-bet issue was skipped: result=%+v, schedule publications=%d", result, published)
		}
		lifecycle := rolloverPostgresIssue(t, db, game.ID, issue)
		if lifecycle.Status != lottery.IssueStatusSettled || lifecycle.SettledAt == nil {
			t.Fatalf("zero-bet issue did not complete the broadcast settlement path: %+v", lifecycle)
		}
		var stored lottery.Draw
		if err := db.Where("game_id = ? AND issue = ?", game.ID, issue).First(&stored).Error; err != nil {
			t.Fatal(err)
		}
		if stored.Numbers != joinNumbers(draw.Numbers) {
			t.Fatalf("zero-bet draw was not preserved: %+v", stored)
		}
		wantDraws = append(wantDraws, stored)

		// Retries, ordinary source polling and a manual settlement retry must
		// all reuse the settled issue, not rewrite its timestamps or history.
		result = service.syncOfficialGameWithPublisher(context.Background(), game.ID, fetch, publish)
		if result.Status != "ok" || result.Imported != 0 || published != 1 {
			t.Fatalf("duplicate zero-bet draw changed its schedule: result=%+v, publications=%d", result, published)
		}
		settler.SettleImportedDraw(game.ID, issue)
		settled, err := settler.SettleIssue(game.ID, issue, "回归测试")
		if err != nil || settled.PendingBefore != 0 || settled.Won != 0 || settled.Lost != 0 {
			t.Fatalf("duplicate zero-bet settlement fabricated bets: result=%+v, error=%v", settled, err)
		}
		if after := rolloverPostgresIssue(t, db, game.ID, issue); !reflect.DeepEqual(after, lifecycle) {
			t.Fatalf("duplicate zero-bet draw rewrote lifecycle: before=%+v, after=%+v", lifecycle, after)
		}
	}
	var storedDraws []lottery.Draw
	if err := db.Where("game_id = ?", "speed-racing").Order("issue ASC").Find(&storedDraws).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(storedDraws, wantDraws) {
		t.Fatalf("a newer draw replaced a spectator's previous issue: got=%+v, want=%+v", storedDraws, wantDraws)
	}
	for table, before := range baseline {
		var after int64
		if err := db.Table(table).Count(&after).Error; err != nil || after != before {
			t.Fatalf("spectating unnecessarily generated room copies or financial records in %s: before=%d, after=%d, error=%v", table, before, after, err)
		}
	}
}

func TestSettlementPushPostgresPreservesInboxAndRoomSummary(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "push_summary_tenant", "76703")
	otherRoom := timingPostgresRoom(t, db, "push_summary_other", "76704")
	game, draw := rolloverPostgresGame(t, db, "94111")
	member, ticket := rolloverPostgresPendingBet(t, db, room, game, "push_summary_winner")
	storedDraw := lottery.Draw{GameID: game.ID, Issue: draw.Issue, Numbers: joinNumbers(draw.Numbers), DrawAt: draw.DrawAt}
	if err := db.Create(&storedDraw).Error; err != nil {
		t.Fatal(err)
	}
	settler := NewBetAdminService(db)
	result, err := settler.SettleIssue(game.ID, draw.Issue, "回归测试")
	if err != nil || result.PendingBefore != 1 || result.Won != 1 || result.PayoutAmount != 19.80 {
		t.Fatalf("fixture settlement failed: result=%+v, error=%v", result, err)
	}
	var notice membernotify.MemberNotification
	if err := db.Where("user_id = ? AND game_id = ? AND issue = ?", member.UserID, game.ID, draw.Issue).First(&notice).Error; err != nil {
		t.Fatal("the private notification centre must retain the financial result:", err)
	}
	if notice.Category != "winning" || notice.WorkspaceID != room.ID || notice.RoomScope != room.Scope ||
		notice.StakeCents != ticket.AmountCents || notice.PayoutCents != 1980 || notice.BetDetailsJSON == "" {
		t.Fatalf("private settlement evidence is incomplete: %+v", notice)
	}
	var summaries []chat.Message
	if err := db.Where("game_id = ? AND reference_id = ?", game.ID, storedDraw.ID).Order("id ASC").Find(&summaries).Error; err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 || summaries[0].MessageType != "settlement" || summaries[1].MessageType != "scoreboard" {
		t.Fatalf("expected one public settlement and scoreboard, got %+v", summaries)
	}
	for _, summary := range summaries {
		if summary.WorkspaceID != room.ID || summary.RoomScope != room.Scope || summary.WorkspaceID == otherRoom.ID {
			t.Fatalf("public summary crossed rooms: %+v", summary)
		}
	}
	if !strings.Contains(summaries[0].Content, "结算内容如下：") ||
		!strings.Contains(summaries[0].Content, "得分：+17.80") ||
		!strings.Contains(summaries[0].Content, "冠军 [1/2.00=+17.80]") {
		t.Fatalf("public summary lost the individual net results: %s", summaries[0].Content)
	}
	before := timingPostgresMoney(t, db, member.UserID)
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := settler.SettleIssue(game.ID, draw.Issue, "回归测试重试"); err != nil {
			t.Fatal(err)
		}
	}
	if after := timingPostgresMoney(t, db, member.UserID); after != before {
		t.Fatalf("settlement retry changed money: before=%+v, after=%+v", before, after)
	}
	var noticeCount, summaryCount int64
	if err := db.Model(&membernotify.MemberNotification{}).Where("event_key = ?", notice.EventKey).Count(&noticeCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&chat.Message{}).Where("game_id = ? AND reference_id = ?", game.ID, storedDraw.ID).Count(&summaryCount).Error; err != nil {
		t.Fatal(err)
	}
	if noticeCount != 1 || summaryCount != 2 {
		t.Fatalf("settlement retries duplicated inbox/room output: notices=%d, room messages=%d", noticeCount, summaryCount)
	}
}
