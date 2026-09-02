package services

import (
	"backend/data/models/bet"
	"backend/data/models/chat"
	"backend/data/models/lottery"
	membernotify "backend/data/models/notify"
	"backend/data/models/user"
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBingoMarkSixPushPostgresRefundsOnceAndIsNotTurnover(t *testing.T) {
	db := timingPostgresDatabase(t)
	game, member := markSixPostgresFixture(t, db, "986201")
	configureTestGameOdds(t, db, game.ID, map[string]float64{"marksix_special_big_small": 1.98})
	service := NewBetAssistantService(db)
	service.bets.suppressNotifications = true

	before := timingPostgresMoney(t, db, member.UserID)
	receipt, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, []WebBetItem{{
		PlayCode: "marksix_special_big_small", Position: 7, Selection: "大", Amount: 10,
	}}, member.Username, "marksix-push-refund-001")
	if err != nil || receipt.BetCount != 1 || receipt.Total != 10 {
		t.Fatalf("place push fixture: receipt=%+v err=%v", receipt, err)
	}
	afterBet := timingPostgresMoney(t, db, member.UserID)
	if afterBet.BalanceCents != before.BalanceCents-1000 || afterBet.Pending != before.Pending+1 || afterBet.LedgerRows != before.LedgerRows+1 {
		t.Fatalf("fixture debit mismatch: before=%+v after=%+v", before, afterBet)
	}

	var ticket bet.Bet
	if err := db.Where("user_id = ? AND game_id = ? AND issue = ?", member.UserID, game.ID, game.NextIssue).First(&ticket).Error; err != nil {
		t.Fatal(err)
	}
	if ticket.RuleVersion != markSixRuleVersion {
		t.Fatalf("push fixture did not freeze the current rule contract: %+v", ticket)
	}
	drawAt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	draw := lottery.Draw{
		GameID: game.ID, Issue: game.NextIssue, Numbers: "1,2,3,4,5,6,49", DrawAt: drawAt,
		SourceRevision: bingoOrderedSourceRevision, ConversionRevision: bingoMarkSixConversionVersion,
	}
	if err := db.Create(&draw).Error; err != nil {
		t.Fatal(err)
	}
	settler := NewBetAdminService(db)
	result, err := settler.SettleIssue(game.ID, game.NextIssue, "和局返本回归")
	if err != nil || result.PendingBefore != 1 || result.Won != 0 || result.Lost != 0 || result.Push != 1 || result.StakeAmount != 10 || result.PayoutAmount != 10 {
		t.Fatalf("push settlement mismatch: result=%+v err=%v", result, err)
	}
	if err := db.First(&ticket, ticket.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ticket.Status != "cancelled" || ticket.PayoutCents != 1000 || ticket.RebateCents != 0 || ticket.AgentShareCents != 0 ||
		ticket.ReconciliationStatus != "normal" || ticket.ReconciliationNote != "settlement_push" || !strings.Contains(ticket.Remark, "和局返还本金") {
		t.Fatalf("push financial row is not auditable: %+v", ticket)
	}
	afterSettlement := timingPostgresMoney(t, db, member.UserID)
	if afterSettlement.BalanceCents != before.BalanceCents || afterSettlement.Pending != before.Pending ||
		afterSettlement.Cancelled != before.Cancelled+1 || afterSettlement.LedgerRows != before.LedgerRows+2 {
		t.Fatalf("push did not restore exactly one stake: before=%+v after=%+v", before, afterSettlement)
	}
	var refundLedgers []user.BalanceTransaction
	if err := db.Where("user_id = ? AND reference = ?", member.UserID, "settlement_bet:"+fmt.Sprint(ticket.ID)).Find(&refundLedgers).Error; err != nil {
		t.Fatal(err)
	}
	if len(refundLedgers) != 1 || refundLedgers[0].AmountCents != ticket.AmountCents || refundLedgers[0].BeforeCents != afterBet.BalanceCents || refundLedgers[0].AfterCents != before.BalanceCents {
		t.Fatalf("push refund ledger mismatch: %+v", refundLedgers)
	}

	status, err := settler.SettlementStatus(game.ID, game.NextIssue)
	if err != nil || !status.HasDraw || !status.Settled || status.Pending != 0 || status.Won != 0 || status.Lost != 0 || status.Push != 1 || status.StakeAmount != 10 || status.PayoutAmount != 10 {
		t.Fatalf("push status aggregation mismatch: status=%+v err=%v", status, err)
	}
	var notice membernotify.MemberNotification
	if err := db.Where("user_id = ? AND game_id = ? AND issue = ?", member.UserID, game.ID, game.NextIssue).First(&notice).Error; err != nil {
		t.Fatal(err)
	}
	if notice.WonCount != 0 || notice.PayoutCents != 1000 || !strings.Contains(notice.Content, "和局返本 1 注") ||
		strings.Contains(notice.Content, "中奖金额") || !strings.Contains(notice.BetDetailsJSON, `"result":"push"`) {
		t.Fatalf("push notification was presented as a win: %+v", notice)
	}
	money, err := settler.GameMoneyMap()
	if err != nil {
		t.Fatal(err)
	}
	if item := money[game.ID]; item.Turnover != 0 || item.GrossProfit != 0 || item.Profit != 0 {
		t.Fatalf("push leaked into dashboard effective turnover: %+v", item)
	}
	report, err := NewReportCenterService(db).Report("summary", ReportCenterFilter{WorkspaceID: member.WorkspaceID, GameID: game.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, metric := range report.Metrics {
		if (metric.Key == "turnover" || metric.Key == "payout" || metric.Key == "member_net" || metric.Key == "gross") && metric.Value != 0 {
			t.Fatalf("push leaked into %s report metric: %+v", metric.Key, report.Metrics)
		}
	}

	retry, err := settler.SettleIssue(game.ID, game.NextIssue, "和局返本重试")
	if err != nil || retry.PendingBefore != 0 || retry.Push != 0 {
		t.Fatalf("push retry was not a no-op: result=%+v err=%v", retry, err)
	}
	if afterRetry := timingPostgresMoney(t, db, member.UserID); afterRetry != afterSettlement {
		t.Fatalf("push retry credited twice: before=%+v after=%+v", afterSettlement, afterRetry)
	}
	var refundCount int64
	if err := db.Model(&user.BalanceTransaction{}).Where("user_id = ? AND reference = ?", member.UserID, "settlement_bet:"+fmt.Sprint(ticket.ID)).Count(&refundCount).Error; err != nil || refundCount != 1 {
		t.Fatalf("push retry duplicated refund ledger: count=%d err=%v", refundCount, err)
	}
}

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
