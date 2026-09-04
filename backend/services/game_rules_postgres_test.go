package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/data/models/odds"
	"reflect"
	"testing"

	"gorm.io/gorm"
)

func TestGameRulesPostgresPlacementSnapshotsAndGuards(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "rule_snapshot_room", "782011")
	member := timingPostgresMember(t, db, room, "rule_snapshot_member")
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "speed-fly", true); err != nil {
		t.Fatal(err)
	}
	timingPostgresSettings(t, db, room.ID, `{"seal_seconds":30}`)
	game := timingPostgresSchedule(t, db, "971101")
	service := NewBetAdminService(db)
	service.suppressNotifications = true
	legacy := bet.Bet{
		WorkspaceID: room.ID, UserID: member.UserID, Username: member.Username, RoomScope: betRoomScope(member),
		GameID: game.ID, Issue: game.NextIssue, PlayCode: "ball_1_5", PlayName: "冠军", Position: 1, Selection: "2",
		AmountCents: 1000, Odds: 9.9, Status: "pending",
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	input := PlaceBetInput{GameID: game.ID, Issue: game.NextIssue, UserID: member.UserID, PlayCode: "ball_1_5", Position: 1, Selection: "2", Amount: 20}
	placed, err := service.Place(input)
	if err != nil || placed.ID == legacy.ID || placed.RuleVersion != "racing-v2" || placed.Amount != 20 {
		t.Fatalf("new stake merged with an older contract: %+v / %v", placed, err)
	}
	var untouched bet.Bet
	if err := db.First(&untouched, legacy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(untouched, legacy) {
		t.Fatalf("legacy bet changed: before=%+v after=%+v", legacy, untouched)
	}
	input.PlayCode, input.Position, input.Selection = "", 6, "大"
	sixth, err := service.PlaceIdempotent(input, "rules-rank-six")
	if err != nil || sixth.PlayCode != "two_sided" || sixth.Position != 6 || sixth.RuleVersion != "racing-v2" {
		t.Fatalf("sixth rank reinterpreted as sum: %+v / %v", sixth, err)
	}
	beforeReplay := timingPostgresMoney(t, db, member.UserID)
	replay, err := service.PlaceIdempotent(input, "rules-rank-six")
	if err != nil || replay.ID != sixth.ID || replay.RuleVersion != sixth.RuleVersion || timingPostgresMoney(t, db, member.UserID) != beforeReplay {
		t.Fatalf("idempotent request changed terms or money: %+v / %v", replay, err)
	}

	// A SQL-level guard also prevents a future administrative save from
	// silently changing the financial contract of an already accepted row.
	if err := db.Transaction(func(tx *gorm.DB) error {
		return tx.Model(&bet.Bet{}).Where("id = ?", placed.ID).Update("rule_version", "digits5-v2").Error
	}); err == nil {
		t.Fatal("database allowed a stored rule version to be rewritten")
	}
	var version string
	if err := db.Model(&bet.Bet{}).Where("id = ?", placed.ID).Pluck("rule_version", &version).Error; err != nil || version != "racing-v2" {
		t.Fatalf("version guard did not preserve snapshot: %q / %v", version, err)
	}

	for _, rejected := range []PlaceBetInput{
		{GameID: "pc-canada", Issue: "971101", UserID: member.UserID, PlayCode: "pair", Position: 1, Selection: "pair", Amount: 20},
		{GameID: "bingo-mark-six", Issue: "971101", UserID: member.UserID, PlayCode: "ball_1_5", Position: 1, Selection: "49", Amount: 20},
		{GameID: game.ID, Issue: game.NextIssue, UserID: member.UserID, PlayCode: "sum", Position: 6, Selection: "99", Amount: 20},
		{GameID: game.ID, Issue: game.NextIssue, UserID: member.UserID, PlayCode: "ball_1_5", Position: 1, Selection: "2", Amount: 1.234},
	} {
		before := timingPostgresMoney(t, db, member.UserID)
		if _, err := service.Place(rejected); err == nil {
			t.Fatalf("unsafe order accepted: %+v", rejected)
		}
		if after := timingPostgresMoney(t, db, member.UserID); after != before {
			t.Fatalf("rejected order altered funds: %+v -> %+v", before, after)
		}
	}
	beforeBatch := timingPostgresMoney(t, db, member.UserID)
	_, err = service.PlaceBatch([]PlaceBetInput{input, {
		GameID: game.ID, Issue: game.NextIssue, UserID: member.UserID, PlayCode: "sum", Position: 6, Selection: "34", Amount: 20,
	}})
	if err == nil || timingPostgresMoney(t, db, member.UserID) != beforeBatch {
		t.Fatal("invalid later group partially charged a batch")
	}
}

func TestGameRulesPostgresEffectiveOddsPreserveSavedConfiguration(t *testing.T) {
	db := timingPostgresDatabase(t)
	legacy := []odds.PlayLimit{
		{GameID: "speed-racing", PlayCode: "pair", PlayName: "旧对子", Odds: 8.8},
		{GameID: "pc-canada", PlayCode: "ball_1_5", PlayName: "旧号码配置", Odds: 9.7},
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	service := NewOddsAdminService(db)
	race, err := service.Get("speed-racing")
	if err != nil || !race.RulesReady || race.RuleVersion != "racing-v2" || len(race.Items) != 4 {
		t.Fatalf("racing still exposes unsupported shapes: %+v / %v", race, err)
	}
	pcView, err := service.Get("pc-canada")
	if err != nil || !pcView.RulesReady || pcView.RuleVersion != pc28RuleV1 || len(pcView.Items) != len(pc28PlaySpecs()) {
		t.Fatalf("PC28 atomic catalog missing: %+v / %v", pcView, err)
	}
	for _, item := range pcView.Items {
		if item.Odds != 0 || item.Configured {
			t.Fatalf("unpriced PC28 item was silently enabled: %+v", item)
		}
	}
	if err := db.Where("game_id = ?", "bingo-mark-six").Delete(&odds.PlayLimit{}).Error; err != nil {
		t.Fatal(err)
	}
	stale := odds.PlayLimit{GameID: "bingo-mark-six", PlayCode: "ball_1_5", PlayName: "旧通用号码", Odds: 9.7}
	if err := db.Create(&stale).Error; err != nil {
		t.Fatal(err)
	}
	markSix, err := service.Get("bingo-mark-six")
	if err != nil || !markSix.RulesReady || markSix.RuleVersion != markSixRuleVersion || len(markSix.Items) != len(markSixPlayCatalog()) {
		t.Fatalf("Bingo Mark Six missing read-only catalog after stale row: %+v / %v", markSix, err)
	}
	for _, item := range markSix.Items {
		spec, exists := markSixSpecByCode(markSixRuleVersion, item.PlayCode)
		if !exists || spec.HiddenFromCatalog || item.Odds != 0 || item.Configured {
			t.Fatalf("unexpected Mark Six odds item: %+v", item)
		}
	}
	var staleAfter odds.PlayLimit
	if err := db.First(&staleAfter, stale.ID).Error; err != nil || !reflect.DeepEqual(staleAfter, stale) {
		t.Fatalf("default repair overwrote stale configuration: %+v / %v", staleAfter, err)
	}
	configuredInput := oddsUpdateInput(markSix)
	configuredInput.Items = append([]PlayLimitItem(nil), markSix.Items...)
	configuredCode := "marksix_special_zodiac_horse"
	for index := range configuredInput.Items {
		if configuredInput.Items[index].PlayCode == configuredCode {
			configuredInput.Items[index].Odds = 2.8
		}
	}
	configuredView, err := service.Update("bingo-mark-six", configuredInput)
	if err != nil {
		t.Fatal("first-time atomic odds configuration failed:", err)
	}
	foundConfigured := false
	for _, item := range configuredView.Items {
		if item.PlayCode == configuredCode {
			foundConfigured = item.Configured && item.Odds == 2.8
		}
	}
	if !foundConfigured {
		t.Fatalf("newly priced atomic play was not exposed as configured: %+v", configuredView.Items)
	}
	pcBeforeReset, err := service.Get("pc-canada")
	if err != nil {
		t.Fatal(err)
	}
	for _, original := range legacy {
		var actual odds.PlayLimit
		if err := db.First(&actual, original.ID).Error; err != nil || !reflect.DeepEqual(actual, original) {
			t.Fatalf("read/filter rewrote configured odds: %+v / %v", actual, err)
		}
	}
	resetPC, err := service.Reset("pc-canada", oddsMutationGuard(pcBeforeReset))
	if err != nil || !resetPC.RulesReady || resetPC.RuleVersion != pc28RuleV1 || len(resetPC.Items) != len(pc28PlaySpecs()) {
		t.Fatalf("PC28 reset did not preserve the read-only atomic catalog: %+v / %v", resetPC, err)
	}
	for _, item := range resetPC.Items {
		if item.Odds != 0 || item.Configured {
			t.Fatalf("PC28 reset manufactured odds: %+v", item)
		}
	}
	for _, id := range []string{"speed-ssc", "sg-ssc", "bingo-ssc-1", "official-fc3d"} {
		view, err := service.Get(id)
		wantItems := 9
		if err != nil || !view.RulesReady || len(view.Items) != wantItems {
			t.Fatalf("missing modeled digit catalog for %s: %+v / %v", id, view, err)
		}
		for _, item := range view.Items {
			if item.PlayCode == "sum" && item.PlayName != "总和 / 总和尾" {
				t.Fatalf("sum still has a racing label for %s: %+v", id, item)
			}
		}
	}
	for _, id := range []string{"bingo-racing-b", "bingo-ssc-2", "bingo-ssc-3", "bingo-ssc-4"} {
		view, err := service.Get(id)
		wantVersion, wantItems := "digits5-v3", 9
		if id == "bingo-racing-b" {
			wantVersion, wantItems = "racing-v2", 4
		}
		if err != nil || !view.RulesReady || view.RuleVersion != wantVersion || len(view.Items) != wantItems {
			t.Fatalf("verified game did not expose its modeled odds catalog for %s: %+v / %v", id, view, err)
		}
		var rows int64
		if err := db.Model(&odds.PlayLimit{}).Where("game_id = ?", id).Count(&rows).Error; err != nil || rows != 0 {
			t.Fatalf("reading verified game manufactured odds for %s: %d / %v", id, rows, err)
		}
	}
	var official lottery.Game
	if err := db.First(&official, "id = ?", "official-fc3d").Error; err != nil || official.LobbyCategory != "" {
		t.Fatal("describing a profile must not classify/enable an official game", err)
	}
}
