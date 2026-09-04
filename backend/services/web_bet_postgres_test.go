package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/data/models/odds"
	"backend/data/models/settings"
	"backend/data/models/user"
	workspacemodel "backend/data/models/workspace"
	apperrors "backend/errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func markSixPostgresFixture(t *testing.T, db *gorm.DB, issue string) (*lottery.Game, user.User) {
	t.Helper()
	room := timingPostgresRoom(t, db, "mark_six_web_room", "786611")
	member := timingPostgresMember(t, db, room, "mark_six_web_member")
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "bingo-mark-six", true); err != nil {
		t.Fatal(err)
	}
	timingPostgresSettings(t, db, room.ID, `{"seal_seconds":30}`)
	now := time.Now().UTC().Truncate(time.Second)
	updates := map[string]any{
		"enabled": true, "source_kind": "external", "timing_source": "upstream", "sync_status": "ok",
		"last_sync_error": "", "last_sync_at": now, "next_issue": issue,
		"next_draw_at": now.Add(3 * time.Minute), "draw_interval": 300,
	}
	if err := db.Model(&lottery.Game{}).Where("id = ?", "bingo-mark-six").Updates(updates).Error; err != nil {
		t.Fatal(err)
	}
	var game lottery.Game
	if err := db.First(&game, "id = ?", "bingo-mark-six").Error; err != nil {
		t.Fatal(err)
	}
	configureTestGameOdds(t, db, game.ID, map[string]float64{
		"marksix_special_a_number": 48, "marksix_special_b_number": 48,
		"marksix_regular_number": 7, "marksix_regular_position_number": 48, "marksix_regular_special_number": 48,
		"marksix_combo_4_all": 700, "marksix_combo_3_all": 580, "marksix_combo_2_all": 60,
		"marksix_combo_special_pair": 150, "marksix_not_in": 2,
	})
	return &game, member
}

func markSixWorkspace(t *testing.T, db *gorm.DB, id uint64) workspacemodel.Workspace {
	t.Helper()
	var room workspacemodel.Workspace
	if err := db.First(&room, id).Error; err != nil {
		t.Fatal(err)
	}
	return room
}

func TestBingoMarkSixMemberOddsOnlyExposeConfiguredAtomicMarkets(t *testing.T) {
	db := timingPostgresDatabase(t)
	game, member := markSixPostgresFixture(t, db, "985901")
	if err := db.Model(&settings.SystemConfig{}).Where("workspace_id = ?", member.WorkspaceID).Update("show_odds", false).Error; err != nil {
		t.Fatal(err)
	}
	portal := NewMemberPortalService(db)
	view, err := portal.GameOdds(member.UserID, game.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range view.Items {
		if item.PlayCode == "marksix_special_zodiac_horse" {
			t.Fatalf("unpriced atomic market leaked to member odds: %+v", item)
		}
	}

	oddsService := NewOddsAdminService(db)
	limits, err := oddsService.Get(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	for index := range limits.Items {
		if limits.Items[index].PlayCode == "marksix_special_zodiac_horse" {
			limits.Items[index].Odds = 2.8
		}
	}
	if _, err := oddsService.Update(game.ID, oddsUpdateInput(limits)); err != nil {
		t.Fatal(err)
	}
	view, err = portal.GameOdds(member.UserID, game.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range view.Items {
		if item.PlayCode == "marksix_special_zodiac_horse" {
			found = true
			// This room hides displayed odds, but the configured play remains
			// available. The separate Configured gate prevents an unpriced zero
			// item from becoming indistinguishable from this case.
			if view.ShowOdds || item.Odds != 0 {
				t.Fatalf("hidden configured odds were exposed: view=%+v item=%+v", view, item)
			}
		}
	}
	if !found {
		t.Fatal("configured atomic market was not exposed to the member")
	}
}

func TestBingoMarkSixWebBatchPostgresIsAtomicIdempotentAndServerPriced(t *testing.T) {
	db := timingPostgresDatabase(t)
	game, member := markSixPostgresFixture(t, db, "986001")
	service := NewBetAssistantService(db)
	service.bets.suppressNotifications = true
	items := []WebBetItem{
		{PlayCode: "marksix_special_a_number", PlayName: "客户端伪造名称", Position: 7, Selection: "49", Amount: 10},
		{PlayCode: "marksix_combo_2_all", PlayName: "客户端伪造名称", Position: 0, Selection: "2,1", Amount: 20},
	}
	before := timingPostgresMoney(t, db, member.UserID)
	accepted, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, items, member.Username, "marksix-web-idempotent-001")
	if err != nil {
		t.Fatal(err)
	}
	if accepted.GameID != game.ID || accepted.RuleVersion != markSixRuleVersion || accepted.Issue != game.NextIssue || accepted.Content != "网投 2 注" || accepted.BetCount != 2 || accepted.Total != 30 || accepted.Balance != 970 || len(accepted.Lines) != 2 {
		t.Fatalf("unexpected receipt: %+v", accepted)
	}
	if accepted.Lines[0].PlayName != "特码A" || accepted.Lines[0].Odds != 48 || accepted.Lines[0].Amount != 10 ||
		accepted.Lines[1].PlayName != "二全中" || accepted.Lines[1].Selection != "1,2" || accepted.Lines[1].Odds != 60 || accepted.Lines[1].Amount != 20 || accepted.Lines[1].Label != "二全中 1,2" {
		t.Fatalf("client fields affected canonical receipt or server odds: %+v", accepted.Lines)
	}
	after := timingPostgresMoney(t, db, member.UserID)
	if after.BalanceCents != before.BalanceCents-3000 || after.Bets != before.Bets+2 || after.Pending != before.Pending+2 || after.LedgerRows != before.LedgerRows+1 {
		t.Fatalf("ticket was not one financial operation: before=%+v after=%+v", before, after)
	}
	var rows []bet.Bet
	if err := db.Where("user_id = ? AND game_id = ? AND issue = ?", member.UserID, game.ID, game.NextIssue).Order("id asc").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].RuleVersion != markSixRuleVersion || rows[1].RuleVersion != markSixRuleVersion || rows[0].PlayName != "特码A" || rows[1].Selection != "1,2" {
		t.Fatalf("stored contract mismatch: %+v", rows)
	}
	var ledger user.BalanceTransaction
	if err := db.Where("user_id = ? AND reference = ?", member.UserID, rows[0].RequestReference).First(&ledger).Error; err != nil || !strings.Contains(ledger.Remark, "网投下注") {
		t.Fatalf("web ledger provenance missing: %+v / %v", ledger, err)
	}

	replay, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, items, member.Username, "marksix-web-idempotent-001")
	if err != nil || !reflect.DeepEqual(replay, accepted) || timingPostgresMoney(t, db, member.UserID) != after {
		t.Fatalf("retry changed receipt or money: %+v / %v", replay, err)
	}
	changed := append([]WebBetItem(nil), items...)
	changed[0].Selection = "48"
	if _, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, changed, member.Username, "marksix-web-idempotent-001"); apperrors.GetErrorCode(err) != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("same id accepted a different payload: %v", err)
	}
	if timingPostgresMoney(t, db, member.UserID) != after {
		t.Fatal("idempotency conflict changed money")
	}

	invalid := []WebBetItem{items[0], {PlayCode: "marksix_color_wave", Position: 7, Selection: "绿波", Amount: 10}}
	if _, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, invalid, member.Username, "marksix-web-atomic-invalid-01"); apperrors.GetErrorCode(err) != "INVALID_REQUEST" {
		t.Fatalf("unsafe later line returned %v", err)
	}
	if timingPostgresMoney(t, db, member.UserID) != after {
		t.Fatal("invalid later line partially charged ticket")
	}

	if err := db.Where("game_id = ? AND play_code = ?", game.ID, "marksix_regular_number").Delete(&odds.PlayLimit{}).Error; err != nil {
		t.Fatal(err)
	}
	missingOdds := []WebBetItem{{PlayCode: "marksix_regular_number", Position: 0, Selection: "20", Amount: 10}}
	if _, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, missingOdds, member.Username, "marksix-web-no-odds-001"); apperrors.GetErrorCode(err) != "ODDS_NOT_CONFIGURED" {
		t.Fatalf("missing server odds returned %v", err)
	}
	if timingPostgresMoney(t, db, member.UserID) != after {
		t.Fatal("missing odds changed money")
	}

	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(markSixWorkspace(t, db, member.WorkspaceID), game.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, missingOdds, member.Username, "marksix-web-room-off-001"); apperrors.GetErrorCode(err) != "GAME_DISABLED" {
		t.Fatalf("disabled room returned %v", err)
	}
	if timingPostgresMoney(t, db, member.UserID) != after {
		t.Fatal("disabled room changed money")
	}
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(markSixWorkspace(t, db, member.WorkspaceID), game.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&workspacemodel.Workspace{}).Where("id = ?", member.WorkspaceID).Update("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, missingOdds, member.Username, "marksix-web-room-stopped-01"); apperrors.GetErrorCode(err) != "ROOM_UNAVAILABLE" {
		t.Fatalf("stopped room returned %v", err)
	}
	if timingPostgresMoney(t, db, member.UserID) != after {
		t.Fatal("stopped room changed money")
	}
	if err := db.Model(&workspacemodel.Workspace{}).Where("id = ?", member.WorkspaceID).Update("status", 1).Error; err != nil {
		t.Fatal(err)
	}

	direct := PlaceBetInput{GameID: game.ID, Issue: game.NextIssue, UserID: member.UserID, PlayCode: "marksix_special_a_number", Position: 7, Selection: "49", Amount: 10}
	if _, err := NewBetAdminService(db).Place(direct); apperrors.GetErrorCode(err) != "BET_MODE_UNAVAILABLE" {
		t.Fatalf("generic direct endpoint path accepted Mark Six: %v", err)
	}
	if _, err := service.Place(member.UserID, game.ID, game.NextIssue, "7/49/10", member.Username, "marksix-chat-rejected-001"); apperrors.GetErrorCode(err) != "BET_MODE_UNAVAILABLE" {
		t.Fatalf("assistant/chat path accepted Mark Six: %v", err)
	}
	if _, err := service.PlaceWeb(member.UserID, "speed-racing", game.NextIssue, items, member.Username, "marksix-wrong-path-game-01"); apperrors.GetErrorCode(err) != "BET_MODE_UNAVAILABLE" {
		t.Fatalf("typed route accepted another path game: %v", err)
	}
	if timingPostgresMoney(t, db, member.UserID) != after {
		t.Fatal("rejected alternate paths changed money")
	}

	draw := lottery.Draw{
		GameID: game.ID, Issue: game.NextIssue, Numbers: "1,2,3,4,5,6,49", DrawAt: time.Now().UTC(),
		SourceRevision: bingoOrderedSourceRevision, ConversionRevision: bingoMarkSixConversionVersion,
	}
	if err := db.Create(&draw).Error; err != nil {
		t.Fatal(err)
	}
	settled, err := NewBetAdminService(db).SettleIssue(game.ID, game.NextIssue, "mark6-v1 test")
	if err != nil || settled.Won != 2 || settled.Lost != 0 || settled.PayoutAmount != 1680 {
		t.Fatalf("end-to-end settlement mismatch: %+v / %v", settled, err)
	}
	final := timingPostgresMoney(t, db, member.UserID)
	if final.BalanceCents != 265000 || final.Pending != 0 || final.Bets != after.Bets {
		t.Fatalf("settlement funds mismatch: %+v", final)
	}
}

func TestBingoMarkSixCompositeAndLinkedWebTicketsFreezeServerTerms(t *testing.T) {
	db := timingPostgresDatabase(t)
	game, member := markSixPostgresFixture(t, db, "986051")
	configureTestGameOdds(t, db, game.ID, map[string]float64{
		"marksix_combo_3_2_exact2":        20.1,
		"marksix_combo_3_2_exact3":        125,
		"marksix_combo_2_special_mixed":   22,
		"marksix_combo_2_special_regular": 55,
		"marksix_link_zodiac_2_rat":       4.2,
		"marksix_link_zodiac_2_ox":        3.55,
		"marksix_link_tail_2_0":           7.5,
		"marksix_link_tail_2_1":           3,
	})
	service := NewBetAssistantService(db)
	service.bets.suppressNotifications = true
	items := []WebBetItem{
		{PlayCode: "marksix_combo_3_2", Position: 0, Selection: "1,2,3", Amount: 10},
		{PlayCode: "marksix_combo_2_special", Position: 0, Selection: "4,5", Amount: 10},
		{PlayCode: "marksix_link_zodiac_2", Position: 0, Selection: "鼠,牛", Amount: 10},
		{PlayCode: "marksix_link_tail_2", Position: 0, Selection: "0尾,1尾", Amount: 10},
	}

	before := timingPostgresMoney(t, db, member.UserID)
	receipt, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, items, member.Username, "marksix-composite-linked-001")
	if err != nil {
		t.Fatal(err)
	}
	after := timingPostgresMoney(t, db, member.UserID)
	if receipt.BetCount != 4 || receipt.Total != 40 || after.BalanceCents != before.BalanceCents-4000 || after.Bets != before.Bets+4 || after.LedgerRows != before.LedgerRows+1 {
		t.Fatalf("composite batch was not one four-line financial operation: receipt=%+v before=%+v after=%+v", receipt, before, after)
	}

	var rows []bet.Bet
	if err := db.Where("user_id = ? AND game_id = ? AND issue = ?", member.UserID, game.ID, game.NextIssue).Order("id asc").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("stored %d Mark Six rows, want 4: %+v", len(rows), rows)
	}
	want := []struct {
		code        string
		odds        float64
		pricingCode string
		exactThree  float64
		twoRegular  float64
	}{
		{code: "marksix_combo_3_2", odds: 20.1, exactThree: 125},
		{code: "marksix_combo_2_special", odds: 22, twoRegular: 55},
		{code: "marksix_link_zodiac_2", odds: 3.55, pricingCode: "marksix_link_zodiac_2_ox"},
		{code: "marksix_link_tail_2", odds: 3, pricingCode: "marksix_link_tail_2_1"},
	}
	for index, expected := range want {
		row := rows[index]
		terms, termsErr := decodeMarkSixOddsTerms(row)
		if termsErr != nil {
			t.Fatalf("decode frozen terms for %s: %v (%q)", row.PlayCode, termsErr, row.OddsTerms)
		}
		if row.PlayCode != expected.code || row.Odds != expected.odds || terms.Version != markSixOddsTermsVersion || terms.PricingCode != expected.pricingCode || terms.ExactThreeOdds != expected.exactThree || terms.TwoRegularOdds != expected.twoRegular {
			t.Fatalf("stored public code or frozen price mismatch: row=%+v terms=%+v want=%+v", row, terms, expected)
		}
	}

	// The period limit belongs to the public 三中二 market, not to each
	// three-number selection. Changing the combination must not reset it.
	limitService := NewOddsAdminService(db)
	limits, err := limitService.Get(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	for index := range limits.Items {
		if limits.Items[index].PlayCode == "marksix_combo_3_2_exact2" {
			limits.Items[index].MaxUserPeriod = 15
		}
		if limits.Items[index].PlayCode == "marksix_link_zodiac_2_rat" {
			limits.Items[index].MaxUserPeriod = 15
		}
	}
	if _, err := limitService.Update(game.ID, oddsUpdateInput(limits)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, []WebBetItem{{
		PlayCode: "marksix_combo_3_2", Position: 0, Selection: "7,8,9", Amount: 10,
	}}, member.Username, "marksix-composite-period-limit-001"); apperrors.GetErrorCode(err) != "PERIOD_LIMIT" {
		t.Fatalf("a different 三中二 selection bypassed the public period limit: %v", err)
	}
	if money := timingPostgresMoney(t, db, member.UserID); money != after {
		t.Fatalf("period-limit rejection changed money: before=%+v after=%+v", after, money)
	}
	if _, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, []WebBetItem{{
		PlayCode: "marksix_link_zodiac_2", Position: 0, Selection: "鼠,牛", Amount: 10,
	}}, member.Username, "marksix-linked-all-price-limits-001"); apperrors.GetErrorCode(err) != "PERIOD_LIMIT" {
		t.Fatalf("linked ticket ignored the stricter non-winning pricing row limit: %v", err)
	}
	if money := timingPostgresMoney(t, db, member.UserID); money != after {
		t.Fatalf("linked period-limit rejection changed money: before=%+v after=%+v", after, money)
	}

	// Internal rows exist solely to configure a public composite ticket. They
	// must never become independently chargeable bets.
	if _, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, []WebBetItem{{
		PlayCode: "marksix_combo_3_2_exact3", Position: 0, Selection: "1,2,3", Amount: 10,
	}}, member.Username, "marksix-internal-price-rejected-001"); apperrors.GetErrorCode(err) != "INVALID_REQUEST" {
		t.Fatalf("internal pricing row was accepted as a ticket: %v", err)
	}
	if money := timingPostgresMoney(t, db, member.UserID); money != after {
		t.Fatalf("rejected internal pricing row changed money: before=%+v after=%+v", after, money)
	}

	// A linked ticket needs every selected atom priced. Removing one price must
	// fail the whole request before debit; the server must not silently fall
	// back to the remaining (and therefore higher) quote.
	if err := db.Where("game_id = ? AND play_code = ?", game.ID, "marksix_link_tail_2_1").Delete(&odds.PlayLimit{}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.PlaceWeb(member.UserID, game.ID, game.NextIssue, []WebBetItem{{
		PlayCode: "marksix_link_tail_2", Position: 0, Selection: "0尾,1尾", Amount: 10,
	}}, member.Username, "marksix-linked-missing-price-001"); apperrors.GetErrorCode(err) != "ODDS_NOT_CONFIGURED" {
		t.Fatalf("linked ticket with one missing atom returned %v", err)
	}
	if money := timingPostgresMoney(t, db, member.UserID); money != after {
		t.Fatalf("linked missing-price rejection changed money: before=%+v after=%+v", after, money)
	}

	draw := lottery.Draw{
		GameID: game.ID, Issue: game.NextIssue, Numbers: "1,2,3,4,5,6,49", DrawAt: time.Now().UTC(),
		SourceRevision: bingoOrderedSourceRevision, ConversionRevision: bingoMarkSixConversionVersion,
	}
	if err := db.Create(&draw).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := NewBetAdminService(db).SettleIssue(game.ID, game.NextIssue, "Mark Six composite snapshot test"); err != nil {
		t.Fatal(err)
	}
	var settled []bet.Bet
	if err := db.Where("id IN ?", []uint64{rows[0].ID, rows[1].ID}).Order("id asc").Find(&settled).Error; err != nil {
		t.Fatal(err)
	}
	if len(settled) != 2 || settled[0].Status != "won" || settled[0].SettlementOdds == nil || *settled[0].SettlementOdds != 125 || settled[0].PayoutCents != 125000 ||
		settled[1].Status != "won" || settled[1].SettlementOdds == nil || *settled[1].SettlementOdds != 55 || settled[1].PayoutCents != 55000 {
		t.Fatalf("settlement did not use the frozen alternate tiers: %+v", settled)
	}
}

func TestBingoMarkSixWebBatchPostgresRollsBackRoomTransferRace(t *testing.T) {
	db := timingPostgresDatabase(t)
	game, member := markSixPostgresFixture(t, db, "986101")
	other := timingPostgresRoom(t, db, "mark_six_race_other", "786612")
	callbackName := "test:mark-six-room-transfer-race"
	inject := false
	if err := db.Callback().Create().After("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if !inject || tx.Statement == nil || tx.Statement.Table != "lottery_bets" {
			return
		}
		inject = false
		if err := tx.Session(&gorm.Session{NewDB: true}).Exec(`UPDATE "user" SET workspace_id = ? WHERE user_id = ?`, other.ID, member.UserID).Error; err != nil {
			tx.AddError(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callbackName) })
	before := timingPostgresMoney(t, db, member.UserID)
	inject = true
	_, err := NewBetAssistantService(db).PlaceWeb(member.UserID, game.ID, game.NextIssue, []WebBetItem{{
		PlayCode: "marksix_special_a_number", Position: 7, Selection: "49", Amount: 10,
	}}, member.Username, "marksix-room-race-request-01")
	if apperrors.GetErrorCode(err) != "REQUEST_CONTEXT_CHANGED" {
		t.Fatalf("room-transfer race returned %v", err)
	}
	if after := timingPostgresMoney(t, db, member.UserID); after != before {
		t.Fatalf("room-transfer race did not roll back money and bets: before=%+v after=%+v", before, after)
	}
	var actual user.User
	if err := db.First(&actual, member.UserID).Error; err != nil || actual.WorkspaceID != member.WorkspaceID {
		t.Fatalf("injected room move escaped rollback: %+v / %v", actual, err)
	}
	var requests int64
	if err := db.Model(&bet.AssistantRequest{}).Where("user_id = ? AND request_id = ?", member.UserID, "marksix-room-race-request-01").Count(&requests).Error; err != nil || requests != 0 {
		t.Fatalf("race left a completed/stale receipt: count=%d err=%v", requests, err)
	}
}
