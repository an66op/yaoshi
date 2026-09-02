package services

import (
	"backend/data/models/bet"
	"backend/data/models/odds"
	apperrors "backend/errors"
	"testing"
)

// This opt-in contract test uses timingPostgresDatabase's empty loopback-only
// database. It proves that all ordinary read/placement paths leave a deliberately
// missing racing price missing; only the explicit Reset operation may seed it.
func TestRacingOddsReadsAndPlacementNeverSeedMissingPrice(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "readonly_racing_odds_room", "788201")
	member := timingPostgresMember(t, db, room, "readonly_racing_odds_member")
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "speed-fly", true); err != nil {
		t.Fatal(err)
	}
	timingPostgresSettings(t, db, room.ID, `{"seal_seconds":30}`)
	game := timingPostgresSchedule(t, db, "978801")

	if err := db.Model(&odds.PlayLimit{}).
		Where("game_id = ? AND play_code = ?", game.ID, "ball_1_5").
		Update("odds", 8.7654).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("game_id = ? AND play_code = ?", game.ID, "sum").Delete(&odds.PlayLimit{}).Error; err != nil {
		t.Fatal(err)
	}
	// Orphaned lower-level prices may survive a platform market removal. They
	// must remain inert rather than making the backend accept a play hidden by
	// the member odds response.
	if err := db.Create(&odds.RoomPlayOdds{
		WorkspaceID: room.ID, AgentID: room.OwnerUserID, GameID: game.ID, PlayCode: "sum", Odds: 1.91,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&odds.UserPlayOdds{
		WorkspaceID: room.ID, UserID: member.UserID, GameID: game.ID, PlayCode: "sum", Odds: 1.92,
	}).Error; err != nil {
		t.Fatal(err)
	}
	assertStoredCount := func(want int64) {
		t.Helper()
		var count int64
		if err := db.Model(&odds.PlayLimit{}).Where("game_id = ?", game.ID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != want {
			t.Fatalf("stored %s odds rows = %d, want %d", game.ID, count, want)
		}
	}
	assertMissingSum := func(items []PlayLimitItem) {
		t.Helper()
		for _, item := range items {
			if item.PlayCode == "sum" {
				if item.Configured || item.Odds != 0 {
					t.Fatalf("missing sum price was fabricated: %+v", item)
				}
				return
			}
		}
		t.Fatal("admin catalog did not expose the unavailable sum item")
	}
	assertStoredCount(3)

	oddsService := NewOddsAdminService(db)
	limits, err := oddsService.Get(game.ID)
	if err != nil || limits.RuleVersion != "racing-v2" || len(limits.Items) != 4 {
		t.Fatalf("admin odds read failed: %+v / %v", limits, err)
	}
	assertMissingSum(limits.Items)
	assertStoredCount(3)

	memberOdds, err := NewMemberPortalService(db).GameOdds(member.UserID, game.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range memberOdds.Items {
		if item.PlayCode == "sum" {
			t.Fatalf("missing sum price was offered to a member: %+v", item)
		}
	}
	assertStoredCount(3)

	service := NewBetAdminService(db)
	service.suppressNotifications = true
	missing := PlaceBetInput{
		GameID: game.ID, Issue: game.NextIssue, UserID: member.UserID,
		PlayCode: "sum", PlayName: "冠亚和", Position: 6, Selection: "大", Amount: 10,
	}
	before := timingPostgresMoney(t, db, member.UserID)
	if _, err := service.Place(missing); apperrors.GetErrorCode(err) != "ODDS_NOT_CONFIGURED" {
		t.Fatalf("missing racing price returned %v, want ODDS_NOT_CONFIGURED", err)
	}
	if after := timingPostgresMoney(t, db, member.UserID); after != before {
		t.Fatalf("rejected missing-price bet changed money/orders: before=%+v after=%+v", before, after)
	}
	assertStoredCount(3)

	configured := PlaceBetInput{
		GameID: game.ID, Issue: game.NextIssue, UserID: member.UserID,
		PlayCode: "ball_1_5", PlayName: "指定名次号码", Position: 1, Selection: "4", Amount: 10,
	}
	placed, err := service.Place(configured)
	if err != nil || placed.Odds != 8.7654 || placed.RuleVersion != "racing-v2" {
		t.Fatalf("configured racing bet did not freeze saved terms: %+v / %v", placed, err)
	}
	var stored bet.Bet
	if err := db.First(&stored, placed.ID).Error; err != nil || stored.Odds != 8.7654 || stored.RuleVersion != "racing-v2" {
		t.Fatalf("persisted racing snapshot changed: %+v / %v", stored, err)
	}
	assertStoredCount(3)

	// Saving the admin form with an unconfigured zero placeholder must preserve
	// the omission instead of rejecting it or silently substituting a default.
	for index := range limits.Items {
		if limits.Items[index].PlayCode == "two_sided" {
			limits.Items[index].Odds = 1.8765
		}
	}
	updated, err := oddsService.Update(game.ID, UpdateOddsLimitsInput{Items: limits.Items})
	if err != nil {
		t.Fatal("save with explicit unconfigured item:", err)
	}
	assertMissingSum(updated.Items)
	assertStoredCount(3)

	reset, err := oddsService.Reset(game.ID)
	if err != nil {
		t.Fatal("explicit reset:", err)
	}
	assertStoredCount(4)
	for _, item := range reset.Items {
		if item.PlayCode == "sum" {
			if !item.Configured || item.Odds <= 1 {
				t.Fatalf("explicit reset did not restore the sum price: %+v", item)
			}
			return
		}
	}
	t.Fatal("explicit reset lost the sum catalog item")
}
