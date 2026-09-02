package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	"backend/data/models/odds"
	"backend/data/models/user"
	apperrors "backend/errors"
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"
)

func TestDigits5V3AssistantPostgresMissingTieOddsRejectsWholeTicket(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "digits5_v3_missing_tie_room", "782004")
	member := timingPostgresMember(t, db, room, "digits5_v3_missing_tie_member")
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "speed-ssc", true); err != nil {
		t.Fatal(err)
	}
	timingPostgresSettings(t, db, room.ID, `{"seal_seconds":30}`)
	// v3 has no code-authored fallback prices. Configure only dragon/tiger so
	// this test proves that the missing independent tie quote closes the batch.
	dragonOdds := odds.PlayLimit{GameID: "speed-ssc", PlayCode: "dragon_tiger", PlayName: "龙虎", Odds: 1.993, MinBet: 1, MaxBet: 50000, MaxUserPeriod: 50000, MaxPeriodTotal: 100000, ExplicitlyConfigured: true, ConfigurationSource: oddsSourceAdminSave}
	if err := db.Where("game_id = ? AND play_code = ?", dragonOdds.GameID, dragonOdds.PlayCode).Assign(dragonOdds).FirstOrCreate(&dragonOdds).Error; err != nil {
		t.Fatal(err)
	}
	issue := "942102"
	if err := db.Model(&lottery.Game{}).Where("id = ?", "speed-ssc").Updates(map[string]any{
		"enabled": true, "source_kind": "external", "timing_source": "upstream",
		"sync_status": "ok", "last_sync_error": "", "next_issue": issue,
		"next_draw_at": time.Now().UTC().Add(10 * time.Minute), "draw_interval": 3600,
	}).Error; err != nil {
		t.Fatal(err)
	}

	before := timingPostgresMoney(t, db, member.UserID)
	result, err := NewBetAssistantService(db).Place(member.UserID, "speed-ssc", issue, "1/龙/20#1/和/20", "fixture", "digits5-v3-missing-tie")
	if result != nil || apperrors.GetErrorCode(err) != "ODDS_NOT_CONFIGURED" {
		t.Fatalf("missing independent tie odds did not fail closed: result=%+v err=%v", result, err)
	}
	if after := timingPostgresMoney(t, db, member.UserID); after != before {
		t.Fatalf("rejected mixed ticket changed money or rows: before=%+v after=%+v", before, after)
	}
}

func TestDigits5V3AssistantPostgresPersistsReceiptVersionAndTieOdds(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "digits5_v3_receipt_room", "782003")
	member := timingPostgresMember(t, db, room, "digits5_v3_receipt_member")
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "speed-ssc", true); err != nil {
		t.Fatal(err)
	}
	timingPostgresSettings(t, db, room.ID, `{"seal_seconds":30}`)
	// An administrator-provided quote, rather than ensureDefaults, activates
	// the independent tie market.
	tieOdds := odds.PlayLimit{GameID: "speed-ssc", PlayCode: "dragon_tiger_tie", PlayName: "龙虎和", Odds: 8.7, MinBet: 1, MaxBet: 50000, MaxUserPeriod: 50000, MaxPeriodTotal: 100000, ExplicitlyConfigured: true, ConfigurationSource: oddsSourceAdminSave}
	if err := db.Where("game_id = ? AND play_code = ?", tieOdds.GameID, tieOdds.PlayCode).Assign(tieOdds).FirstOrCreate(&tieOdds).Error; err != nil {
		t.Fatal(err)
	}
	issue := "942101"
	if err := db.Model(&lottery.Game{}).Where("id = ?", "speed-ssc").Updates(map[string]any{
		"enabled": true, "source_kind": "external", "timing_source": "upstream",
		"sync_status": "ok", "last_sync_error": "", "next_issue": issue,
		"next_draw_at": time.Now().UTC().Add(10 * time.Minute), "draw_interval": 3600,
	}).Error; err != nil {
		t.Fatal(err)
	}

	service := NewBetAssistantService(db)
	result, err := service.Place(member.UserID, "speed-ssc", issue, "1/和/20", "fixture", "digits5-v3-tie-receipt")
	if err != nil {
		t.Fatal(err)
	}
	if result.RuleVersion != "digits5-v3" || result.BetCount != 1 || len(result.Lines) != 1 ||
		result.Lines[0].PlayCode != "dragon_tiger_tie" || result.Lines[0].Odds != 8.7 {
		t.Fatalf("v3 receipt lost independent tie contract: %+v", result)
	}
	var request bet.AssistantRequest
	if err := db.Where("user_id = ? AND request_id = ?", member.UserID, "digits5-v3-tie-receipt").First(&request).Error; err != nil {
		t.Fatal(err)
	}
	var persisted AssistantBetResult
	if err := json.Unmarshal([]byte(request.ResultJSON), &persisted); err != nil || persisted.RuleVersion != "digits5-v3" {
		t.Fatalf("result_json did not freeze v3: %+v %v", persisted, err)
	}
	var ticket bet.Bet
	if err := db.Where("user_id = ? AND request_reference = ?", member.UserID, assistantBetRequestReference(request.ID)).First(&ticket).Error; err != nil {
		t.Fatal(err)
	}
	if ticket.RuleVersion != "digits5-v3" || ticket.PlayCode != "dragon_tiger_tie" || ticket.Odds != 8.7 {
		t.Fatalf("stored ticket lost v3 tie terms: %+v", ticket)
	}

	beforeReplay := timingPostgresMoney(t, db, member.UserID)
	replay, err := service.Place(member.UserID, "speed-ssc", issue, "1/和/20", "fixture", "digits5-v3-tie-receipt")
	if err != nil || replay.RuleVersion != "digits5-v3" || timingPostgresMoney(t, db, member.UserID) != beforeReplay {
		t.Fatalf("idempotent replay changed v3 receipt or money: %+v %v", replay, err)
	}
}

func TestAssistantRulesPostgresDocumentedAmountsAndAtomicity(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "receipt_rules_room", "782001")
	member := timingPostgresMember(t, db, room, "receipt_rules_member")
	if err := db.Model(&user.User{}).Where("user_id = ?", member.UserID).Update("balance_cents", 10000000).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, "speed-racing", true); err != nil {
		t.Fatal(err)
	}
	timingPostgresSettings(t, db, room.ID, `{"seal_seconds":30}`)
	if err := db.Model(&lottery.Game{}).Where("id = ?", "speed-racing").Updates(map[string]any{"enabled": true, "source_kind": "external", "timing_source": "upstream", "sync_status": "ok", "last_sync_error": "", "next_issue": "942001", "next_draw_at": time.Now().UTC().Add(10 * time.Minute), "draw_interval": 3600}).Error; err != nil {
		t.Fatal(err)
	}
	service := NewBetAssistantService(db)
	for index, test := range []struct {
		content string
		total   float64
		count   int
	}{
		{"489/0178/48", 576, 12}, {"5/045/343", 1029, 3}, {"68/单大/811", 3244, 4},
		{"62437/546", 2730, 5}, {"1/12345/100#6/大/200#7/67890/100", 1200, 11},
		{"冠军/12345/100", 500, 5}, {"4444/88", 352, 1},
		{"1/1/20#2/4/20#3/3/20#4/7/20#5/9/20#6/5/20#7/5/20#8/5/20#9/2/20#0/8/20", 200, 10},
		{"1/1/80#1/2/80", 160, 2},
		{"2/大小单双12/20#1/大小单双12/20", 240, 12},
		{"1/3/50#2/0/50#3/0/50#4/9/50#5/0/50#6/9/50#7/4/50#8/7/50#9/6/50#0/6/50", 500, 10},
		{"1/大小单双12/20.00#2/大小单双12/20.00", 240, 12},
		{"2/5/20.00#0/0/1.25", 21.25, 2},
	} {
		before := timingPostgresMoney(t, db, member.UserID)
		id := fmt.Sprintf("documented-receipt-%d", index)
		result, err := service.Place(member.UserID, "speed-racing", "942001", test.content, "fixture", id)
		if err != nil || result.Total != test.total || result.BetCount != test.count {
			t.Fatalf("%s: %+v %v", test.content, result, err)
		}
		parsed, parseErr := ParseAssistantBet(test.content)
		if parseErr != nil || len(parsed) != len(result.Lines) {
			t.Fatalf("receipt missing parsed ranks: %+v %v", result, parseErr)
		}
		for index, line := range result.Lines {
			want := parsed[index]
			// The game-aware placement normalizes shorthand number 0 to 10.
			if want.PlayCode == "ball_1_5" && want.Selection == "0" {
				want.Selection = "10"
			}
			if line.Label != assistantLineLabel(want) || line.Position != want.Position {
				t.Fatalf("persisted receipt changed rank: %+v -> %+v", want, line)
			}
		}
		after := timingPostgresMoney(t, db, member.UserID)
		if before.BalanceCents-after.BalanceCents != int64(math.Round(test.total*100)) || after.Bets-before.Bets != int64(test.count) || after.LedgerRows-before.LedgerRows != 1 {
			t.Fatalf("wrong debit for %s: %+v -> %+v", test.content, before, after)
		}
		if _, err = service.Place(member.UserID, "speed-racing", "942001", test.content, "fixture", id); err != nil {
			t.Fatal(err)
		}
		if replay := timingPostgresMoney(t, db, member.UserID); replay != after {
			t.Fatal("replay double charged")
		}
	}
	before := timingPostgresMoney(t, db, member.UserID)
	if _, err := service.Place(member.UserID, "speed-racing", "942001", "1/12345/100#无效内容/9", "fixture", "documented-invalid-ticket"); err == nil {
		t.Fatal("invalid part accepted")
	}
	if after := timingPostgresMoney(t, db, member.UserID); after != before {
		t.Fatal("invalid ticket partially charged")
	}
}
