package services

import (
	"backend/data/models/lottery"
	"backend/data/models/odds"
	apperrors "backend/errors"
	"reflect"
	"testing"
)

func TestOddsConfigurationPostgresRejectsStaleWritersAndEmptyABA(t *testing.T) {
	db := timingPostgresDatabase(t)
	service := NewOddsAdminService(db)
	initial, err := service.Get("speed-racing")
	if err != nil || initial.ConfigRevision == "" {
		t.Fatalf("initial revision = %+v / %v", initial, err)
	}
	input := oddsUpdateInput(initial)
	input.Items = append([]PlayLimitItem(nil), initial.Items...)
	input.Items[0].Odds = 1.9876
	saved, err := service.Update(initial.GameID, input)
	if err != nil || saved.ConfigRevision == initial.ConfigRevision {
		t.Fatalf("save did not advance revision: %+v / %v", saved, err)
	}
	if _, err := service.Update(initial.GameID, input); apperrors.GetErrorCode(err) != "ODDS_CONFIGURATION_CONFLICT" {
		t.Fatalf("stale save returned %v", err)
	}
	if _, err := service.Reset(initial.GameID, oddsMutationGuard(initial)); apperrors.GetErrorCode(err) != "ODDS_CONFIGURATION_CONFLICT" {
		t.Fatalf("stale clear returned %v", err)
	}
	cleared, err := service.Reset(initial.GameID, oddsMutationGuard(saved))
	if err != nil || cleared.ConfigRevision == initial.ConfigRevision || cleared.ConfigRevision == saved.ConfigRevision {
		t.Fatalf("clear reused an earlier revision: %+v / %v", cleared, err)
	}
	if _, err := service.Update(initial.GameID, input); apperrors.GetErrorCode(err) != "ODDS_CONFIGURATION_CONFLICT" {
		t.Fatalf("ABA-stale initial form reopened cleared configuration: %v", err)
	}
	clearedAgain, err := service.Reset(initial.GameID, oddsMutationGuard(cleared))
	if err != nil || clearedAgain.ConfigRevision == cleared.ConfigRevision {
		t.Fatalf("empty clear did not advance revision: %+v / %v", clearedAgain, err)
	}
	for _, row := range clearedAgain.Items {
		if row.Configured || row.Odds != 0 {
			t.Fatalf("clear manufactured a quote: %+v", row)
		}
	}
	var game lottery.Game
	if err := db.First(&game, "id = ?", initial.GameID).Error; err != nil || game.OddsConfigRevision != 3 {
		t.Fatalf("monotonic revision = %d / %v, want 3", game.OddsConfigRevision, err)
	}
}

func TestOddsConfigurationPostgresInvalidBatchIsAtomic(t *testing.T) {
	db := timingPostgresDatabase(t)
	service := NewOddsAdminService(db)
	saved := configureTestGameOdds(t, db, "speed-racing", map[string]float64{"ball_1_5": 9.9, "two_sided": 1.99})
	for name, mutate := range map[string]func(*UpdateOddsLimitsInput){
		"missing revision": func(input *UpdateOddsLimitsInput) { input.ExpectedRevision = "" },
		"old rules":        func(input *UpdateOddsLimitsInput) { input.ExpectedRuleVersion = "racing-v1" },
		"partial":          func(input *UpdateOddsLimitsInput) { input.Items = input.Items[:1] },
		"duplicate":        func(input *UpdateOddsLimitsInput) { input.Items[1].PlayCode = input.Items[0].PlayCode },
		"invalid limit":    func(input *UpdateOddsLimitsInput) { input.Items[len(input.Items)-1].MinBet = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			input := oddsUpdateInput(saved)
			input.Items = append([]PlayLimitItem(nil), saved.Items...)
			input.Items[0].Odds = 2.1234
			mutate(&input)
			if _, err := service.Update(saved.GameID, input); err == nil {
				t.Fatal("invalid batch accepted")
			}
			after, err := service.Get(saved.GameID)
			if err != nil || !reflect.DeepEqual(saved, after) {
				t.Fatalf("invalid batch partially changed state: before=%+v after=%+v err=%v", saved, after, err)
			}
		})
	}
}

func TestOddsConfigurationPostgresPreservesExplicitZeroLimits(t *testing.T) {
	db := timingPostgresDatabase(t)
	service := NewOddsAdminService(db)
	view, err := service.Get("speed-racing")
	if err != nil {
		t.Fatal(err)
	}
	view.Items[0].Odds = 2.18
	view.Items[0].MinBet = .25
	view.Items[0].MaxBet = 0
	view.Items[0].MaxUserPeriod = 0
	view.Items[0].MaxPeriodTotal = 0
	saved, err := service.Update(view.GameID, oddsUpdateInput(view))
	if err != nil {
		t.Fatal(err)
	}
	item := saved.Items[0]
	if item.Odds != 2.18 || item.MinBet != .25 || item.MaxBet != 0 || item.MaxUserPeriod != 0 || item.MaxPeriodTotal != 0 {
		t.Fatalf("explicit zero limit was replaced by an ORM default: %+v", item)
	}
}

func TestOddsConfigurationPostgresOverridesCannotResurrectClosedOrRevisedMarkets(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "odds_closed_override", "782078")
	member := timingPostgresMember(t, db, room, "odds_closed_override_member")
	admin, trading := NewOddsAdminService(db), NewTradingAdminService(db)
	const gameID, code = "speed-racing", "ball_1_5"
	price := 8.7
	roomInput := UpdateRoomTradingInput{GameID: gameID}
	roomInput.Odds = append(roomInput.Odds, struct {
		PlayCode string   `json:"play_code"`
		Override *float64 `json:"override"`
	}{code, &price})
	memberInput := UpdateUserTradingInput{GameID: gameID}
	memberInput.Odds = append(memberInput.Odds, struct {
		PlayCode string   `json:"play_code"`
		Override *float64 `json:"override"`
	}{code, &price})
	if _, err := trading.UpdateRoomForWorkspace(room.ID, roomInput); apperrors.GetErrorCode(err) != "ODDS_NOT_CONFIGURED" {
		t.Fatalf("room created a quote for an unconfigured play: %v", err)
	}
	if _, err := trading.Update(member.UserID, memberInput); apperrors.GetErrorCode(err) != "ODDS_NOT_CONFIGURED" {
		t.Fatalf("member created a quote for an unconfigured play: %v", err)
	}
	saved := configureTestGameOdds(t, db, gameID, map[string]float64{code: 9.9})
	if _, err := trading.UpdateRoomForWorkspace(room.ID, roomInput); err != nil {
		t.Fatal(err)
	}
	if _, err := trading.Update(member.UserID, memberInput); err != nil {
		t.Fatal(err)
	}
	resolved, err := trading.Resolve(member.UserID, gameID, code, 20, 999, 0)
	if err != nil || resolved.Odds != price || resolved.OddsSource != "user" {
		t.Fatalf("configured member precedence = %+v / %v", resolved, err)
	}
	for index := range saved.Items {
		if saved.Items[index].PlayCode == code {
			saved.Items[index].Odds = 0
		}
	}
	if _, err := admin.Update(gameID, oddsUpdateInput(saved)); err != nil {
		t.Fatal(err)
	}
	assertNoOverrides := func() {
		t.Helper()
		for _, model := range []any{&odds.RoomPlayOdds{}, &odds.UserPlayOdds{}} {
			var count int64
			if err := db.Model(model).Where("game_id = ?", gameID).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("closed/revised market retained lower-level overrides: %T=%d/%v", model, count, err)
			}
		}
	}
	assertNoOverrides()
	// Even direct legacy/orphan rows stay hidden while the base quote is off.
	if err := db.Create(&odds.RoomPlayOdds{WorkspaceID: room.ID, AgentID: room.OwnerUserID, GameID: gameID, PlayCode: code, Odds: price}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&odds.UserPlayOdds{WorkspaceID: room.ID, UserID: member.UserID, GameID: gameID, PlayCode: code, Odds: price}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := trading.Resolve(member.UserID, gameID, code, 20, 999, 0); apperrors.GetErrorCode(err) != "ODDS_NOT_CONFIGURED" {
		t.Fatalf("orphaned overrides revived closed play: %v", err)
	}
	roomView, err := trading.GetRoomForWorkspace(room.ID, gameID)
	if err != nil || len(roomView.Odds) != 0 {
		t.Fatalf("closed room offers = %+v / %v", roomView, err)
	}
	memberView, err := trading.Get(member.UserID, gameID)
	if err != nil || len(memberView.Odds) != 0 {
		t.Fatalf("closed member offers = %+v / %v", memberView, err)
	}
	configureTestGameOdds(t, db, gameID, map[string]float64{code: 9.6})
	assertNoOverrides()
	resolved, err = trading.Resolve(member.UserID, gameID, code, 20, 999, 0)
	if err != nil || resolved.Odds != 9.6 || resolved.OddsSource != "platform" {
		t.Fatalf("reopening reused stale override: %+v / %v", resolved, err)
	}
	if _, err := trading.UpdateRoomForWorkspace(room.ID, roomInput); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&odds.PlayLimit{}).Where("game_id = ?", gameID).Update("rule_version", "racing-v1").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := trading.Resolve(member.UserID, gameID, code, 20, 999, 0); apperrors.GetErrorCode(err) != "ODDS_NOT_CONFIGURED" {
		t.Fatalf("room override bypassed rule binding: %v", err)
	}
	configureTestGameOdds(t, db, gameID, map[string]float64{code: 9.5})
	assertNoOverrides()
}
