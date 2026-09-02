package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/data/models/odds"
	apperrors "backend/errors"
	"testing"
	"time"
)

func findPlayLimitItem(t *testing.T, items []PlayLimitItem, code string) PlayLimitItem {
	t.Helper()
	for _, item := range items {
		if item.PlayCode == code {
			return item
		}
	}
	t.Fatalf("missing odds item %s", code)
	return PlayLimitItem{}
}

func TestExplicitOddsConfirmationPostgresLegacySaveResetLifecycle(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "explicit_odds_lifecycle", "782071")
	member := timingPostgresMember(t, db, room, "explicit_odds_lifecycle_member")
	service := NewOddsAdminService(db)
	trade := NewTradingAdminService(db)

	for _, test := range []struct {
		gameID, playCode string
		price            float64
	}{
		{"pc-canada", pc28SumSize, 1.91},
		{"speed-ssc", "ball_1_5", 9.81},
		{bingoRacingAGameID, "ball_1_5", 9.95},
	} {
		t.Run(test.gameID, func(t *testing.T) {
			legacy := odds.PlayLimit{
				GameID: test.gameID, PlayCode: test.playCode, PlayName: test.playCode, Odds: test.price,
				MinBet: 1, MaxBet: 50000, MaxUserPeriod: 50000, MaxPeriodTotal: 100000,
			}
			if err := db.Create(&legacy).Error; err != nil {
				t.Fatal(err)
			}
			view, err := service.Get(test.gameID)
			if err != nil {
				t.Fatal(err)
			}
			closed := findPlayLimitItem(t, view.Items, test.playCode)
			if closed.Configured || closed.Odds != 0 || closed.ConfigurationSource != oddsSourceLegacyUnconfirmed {
				t.Fatalf("legacy row opened upgraded market: %+v", closed)
			}
			if _, err := trade.Resolve(member.UserID, test.gameID, test.playCode, 20, 999, 0); apperrors.GetErrorCode(err) != "ODDS_NOT_CONFIGURED" {
				t.Fatalf("legacy row resolved as a quote: %v", err)
			}

			for index := range view.Items {
				if view.Items[index].PlayCode == test.playCode {
					view.Items[index].Odds = test.price
				}
			}
			opened, err := service.Update(test.gameID, UpdateOddsLimitsInput{Items: view.Items})
			if err != nil {
				t.Fatal(err)
			}
			active := findPlayLimitItem(t, opened.Items, test.playCode)
			if !active.Configured || active.Odds != test.price || active.ConfigurationSource != oddsSourceAdminSave || active.ConfiguredAt == nil {
				t.Fatalf("administrator save did not activate quote: %+v", active)
			}
			resolved, err := trade.Resolve(member.UserID, test.gameID, test.playCode, 20, 999, 0)
			if err != nil || resolved.Odds != test.price || resolved.PricingCode != test.playCode {
				t.Fatalf("confirmed quote did not resolve: %+v / %v", resolved, err)
			}
			var persisted odds.PlayLimit
			if err := db.Where("game_id = ? AND play_code = ?", test.gameID, test.playCode).First(&persisted).Error; err != nil {
				t.Fatal(err)
			}
			if !persisted.ExplicitlyConfigured || persisted.ConfigurationSource != oddsSourceAdminSave || persisted.ConfiguredAt == nil {
				t.Fatalf("activation audit missing: %+v", persisted)
			}

			reset, err := service.Reset(test.gameID)
			if err != nil {
				t.Fatal(err)
			}
			closed = findPlayLimitItem(t, reset.Items, test.playCode)
			if closed.Configured || closed.Odds != 0 || closed.ConfigurationSource != oddsSourceUnconfigured {
				t.Fatalf("reset left upgraded quote enabled: %+v", closed)
			}
			if _, err := trade.Resolve(member.UserID, test.gameID, test.playCode, 20, 999, 0); apperrors.GetErrorCode(err) != "ODDS_NOT_CONFIGURED" {
				t.Fatalf("reset quote remained resolvable: %v", err)
			}
		})
	}
}

func TestBingoRacingASelectionOddsPostgresFreezeExactTicketPrices(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "bingo_a_selection_odds", "782072")
	member := timingPostgresMember(t, db, room, "bingo_a_selection_member")
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, bingoRacingAGameID, true); err != nil {
		t.Fatal(err)
	}
	timingPostgresSettings(t, db, room.ID, `{"seal_seconds":30}`)
	issue := "982201"
	if err := db.Model(&lottery.Game{}).Where("id = ?", bingoRacingAGameID).Updates(map[string]any{
		"enabled": true, "source_kind": "external", "timing_source": "upstream",
		"sync_status": "ok", "last_sync_error": "", "next_issue": issue,
		"next_draw_at": time.Now().UTC().Add(10 * time.Minute), "draw_interval": 3600,
	}).Error; err != nil {
		t.Fatal(err)
	}

	admin := NewOddsAdminService(db)
	view, err := admin.Get(bingoRacingAGameID)
	if err != nil {
		t.Fatal(err)
	}
	wantPrices := map[string]float64{"sum_big": 2.18, "sum_small": 1.775, "sum_3": 42.5}
	for index := range view.Items {
		if price, ok := wantPrices[view.Items[index].PlayCode]; ok {
			view.Items[index].Odds = price
		}
	}
	if _, err := admin.Update(bingoRacingAGameID, UpdateOddsLimitsInput{Items: view.Items}); err != nil {
		t.Fatal(err)
	}

	service := NewBetAdminService(db)
	service.suppressNotifications = true
	inputs := []PlaceBetInput{
		{GameID: bingoRacingAGameID, Issue: issue, UserID: member.UserID, PlayCode: "sum", PlayName: "冠亚和", Position: 6, Selection: "大", Amount: 10},
		{GameID: bingoRacingAGameID, Issue: issue, UserID: member.UserID, PlayCode: "sum", PlayName: "冠亚和", Position: 6, Selection: "小", Amount: 10},
		{GameID: bingoRacingAGameID, Issue: issue, UserID: member.UserID, PlayCode: "sum", PlayName: "冠亚和", Position: 6, Selection: "3", Amount: 10},
	}
	placed, err := service.PlaceBatch(inputs)
	if err != nil || len(placed) != len(inputs) {
		t.Fatalf("selection-priced batch rejected: %+v / %v", placed, err)
	}
	for _, ticket := range placed {
		pricingCode, pricingErr := oddsPricingCode(ticket.GameID, ticket.PlayCode, ticket.Selection)
		if pricingErr != nil || ticket.PlayCode != "sum" || ticket.RuleVersion != "racing-v2" || ticket.Odds != wantPrices[pricingCode] {
			t.Fatalf("ticket lost selection quote: pricing=%s ticket=%+v err=%v", pricingCode, ticket, pricingErr)
		}
	}
	var rows []bet.Bet
	if err := db.Where("game_id = ? AND issue = ? AND user_id = ?", bingoRacingAGameID, issue, member.UserID).Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("stored %d selection tickets, want 3", len(rows))
	}
	for _, row := range rows {
		pricingCode, pricingErr := oddsPricingCode(row.GameID, row.PlayCode, row.Selection)
		if pricingErr != nil || row.PlayCode != "sum" || row.Odds != wantPrices[pricingCode] {
			t.Fatalf("persisted snapshot lost selection price: pricing=%s row=%+v err=%v", pricingCode, row, pricingErr)
		}
	}
}
