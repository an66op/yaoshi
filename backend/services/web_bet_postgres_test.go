package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/data/models/odds"
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
		"next_draw_at": now.Add(10 * time.Minute), "draw_interval": 300,
	}
	if err := db.Model(&lottery.Game{}).Where("id = ?", "bingo-mark-six").Updates(updates).Error; err != nil {
		t.Fatal(err)
	}
	var game lottery.Game
	if err := db.First(&game, "id = ?", "bingo-mark-six").Error; err != nil {
		t.Fatal(err)
	}
	if err := NewOddsAdminService(db).EnsureGameDefaults(game.ID); err != nil {
		t.Fatal(err)
	}
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
	portal := NewMemberPortalService(db)
	view, err := portal.GameOdds(member.UserID, game.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range view.Items {
		if item.PlayCode == "marksix_special_zodiac" {
			t.Fatalf("unpriced atomic market leaked to member odds: %+v", item)
		}
	}

	oddsService := NewOddsAdminService(db)
	limits, err := oddsService.Get(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	for index := range limits.Items {
		if limits.Items[index].PlayCode == "marksix_special_zodiac" {
			limits.Items[index].Odds = 2.8
		}
	}
	if _, err := oddsService.Update(game.ID, UpdateOddsLimitsInput{Items: limits.Items}); err != nil {
		t.Fatal(err)
	}
	view, err = portal.GameOdds(member.UserID, game.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range view.Items {
		if item.PlayCode == "marksix_special_zodiac" {
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
	if err := NewOddsAdminService(db).EnsureGameDefaults(game.ID); err != nil {
		t.Fatal(err)
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
		if err := tx.Exec(`UPDATE "user" SET workspace_id = ? WHERE user_id = ?`, other.ID, member.UserID).Error; err != nil {
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
