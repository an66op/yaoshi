package services

import (
	"backend/data/models/bet"
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"reflect"
	"strconv"
	"testing"
	"time"
)

var directMarkSixRuleCases = []struct {
	gameID  string
	version string
}{
	{"hong-kong-mark-six", hongKongMarkSixRuleVersion},
	{"happy8-mark-six", happy8MarkSixRuleVersion},
	{"new-macau-mark-six", newMacauMarkSixRuleVersion},
	{"old-macau-mark-six", oldMacauMarkSixRuleVersion},
}

func TestDirectMarkSixProductsHaveExactWebOnlyContracts(t *testing.T) {
	allVersions := []string{
		markSixRuleVersion, hongKongMarkSixRuleVersion, happy8MarkSixRuleVersion,
		newMacauMarkSixRuleVersion, oldMacauMarkSixRuleVersion,
	}
	for _, test := range directMarkSixRuleCases {
		t.Run(test.gameID, func(t *testing.T) {
			game := &lottery.Game{ID: test.gameID}
			profile, ready := rulesForGame(game)
			if !ready || profile.Version != test.version || !profile.MarkSix || profile.BallCount != 7 || profile.MinNumber != 1 || profile.MaxNumber != 49 || !profile.Unique {
				t.Fatalf("profile=%+v ready=%v", profile, ready)
			}
			if !gameSupportsRuleVersion(test.gameID, test.version) {
				t.Fatal("game rejected its exact rule version")
			}
			for _, version := range allVersions {
				if version != test.version && gameSupportsRuleVersion(test.gameID, version) {
					t.Fatalf("game accepted foreign Mark Six version %q", version)
				}
			}
			if err := ensurePlacementBetMode(profile, "web"); err != nil {
				t.Fatalf("web mode rejected: %v", err)
			}
			for _, mode := range []string{"", "chat", "assistant", "WEB"} {
				if code := apperrors.GetErrorCode(ensurePlacementBetMode(profile, mode)); code != "BET_MODE_UNAVAILABLE" {
					t.Fatalf("mode %q returned %q", mode, code)
				}
			}
			if _, err := parseAssistantBetForGame(game, "7/49/10"); apperrors.GetErrorCode(err) != "BET_MODE_UNAVAILABLE" {
				t.Fatalf("chat parser did not fail closed: %v", err)
			}
			ready, version, message, modes := memberOddsRuleStatus(test.gameID)
			if !ready || version != test.version || message != "" || modes.Chat || !modes.Web {
				t.Fatalf("member contract=%v/%q/%q/%+v", ready, version, message, modes)
			}
		})
	}
}

func TestDirectMarkSixCatalogsMatchBingoButRemainGameScoped(t *testing.T) {
	want := PlayCatalogForGame("bingo-mark-six")
	if len(want) != 242 {
		t.Fatalf("Bingo catalogue size=%d, want 242", len(want))
	}
	for _, test := range directMarkSixRuleCases {
		got := PlayCatalogForGame(test.gameID)
		if len(got) != 242 || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s catalogue differs from Bingo: got=%d want=%d", test.gameID, len(got), len(want))
		}
		if got := got; len(got) > 0 {
			got[0].PlayName = "changed locally"
			if PlayCatalogForGame("bingo-mark-six")[0].PlayName == "changed locally" {
				t.Fatal("catalogues share mutable per-game state")
			}
		}
		if code, err := oddsPricingCode(test.gameID, "marksix_combo_3_2", "1,2,3"); err != nil || code != "marksix_combo_3_2_exact2" {
			t.Fatalf("composite pricing code=%q err=%v", code, err)
		}
	}
}

func TestDirectMarkSixWebChoicesAndSettlementAreVersioned(t *testing.T) {
	draw := []int{1, 2, 3, 4, 5, 6, 48}
	checks := []struct {
		code      string
		position  int
		selection string
	}{
		{"marksix_special_a_number", 7, "48"},
		{"marksix_regular_number", 0, "3"},
		{"marksix_regular_position_number", 4, "4"},
		{"marksix_combo_2_all", 0, "1,2"},
		{"marksix_special_big_small", 7, "大"},
	}
	for _, test := range directMarkSixRuleCases {
		t.Run(test.gameID, func(t *testing.T) {
			game := &lottery.Game{ID: test.gameID}
			for _, check := range checks {
				code, name, err := InferPlayForGame(game, check.code, "forged name", check.position, check.selection)
				if err != nil || code != check.code || name == "" || name == "forged name" {
					t.Fatalf("infer %s=%q/%q/%v", check.code, code, name, err)
				}
				won, _, err := evaluateBetForRuleVersion(game, test.version, draw, check.code, check.position, check.selection)
				if err != nil || !won {
					t.Fatalf("settle %s won=%v err=%v", check.code, won, err)
				}
			}
			decision, err := decideMarkSixSettlement(test.gameID, bet.Bet{
				GameID: test.gameID, RuleVersion: test.version,
				PlayCode: "marksix_special_a_number", Position: 7, Selection: "48",
				AmountCents: 100, Odds: 48,
			}, draw, time.Date(2026, 9, 4, 21, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60)))
			if err != nil || decision.Outcome != markSixOutcomeWon || decision.EffectiveOdds != 48 {
				t.Fatalf("decision=%+v err=%v", decision, err)
			}
			if _, _, err := evaluateBetForRuleVersion(game, markSixRuleVersion, draw, "marksix_special_a_number", 7, "48"); apperrors.GetErrorCode(err) != "RULES_NOT_READY" {
				t.Fatalf("foreign Bingo version settled: %v", err)
			}
			for _, invalid := range [][]int{
				{1, 2, 3, 4, 5, 6},
				{1, 2, 3, 4, 5, 6, 6},
				{0, 2, 3, 4, 5, 6, 7},
				{1, 2, 3, 4, 5, 6, 50},
			} {
				if _, _, err := evaluateBetForRuleVersion(game, test.version, invalid, "marksix_special_a_number", 7, "48"); apperrors.GetErrorCode(err) != "INVALID_DRAW" {
					t.Fatalf("invalid draw %v returned %v", invalid, err)
				}
			}
		})
	}

	bingo := &lottery.Game{ID: "bingo-mark-six"}
	if _, _, err := evaluateBetForRuleVersion(bingo, markSixRuleVersion, draw, "marksix_special_a_number", 7, "48"); err != nil {
		t.Fatalf("historical Bingo contract changed: %v", err)
	}
	if _, _, err := evaluateBetForRuleVersion(bingo, hongKongMarkSixRuleVersion, draw, "marksix_special_a_number", 7, "48"); apperrors.GetErrorCode(err) != "RULES_NOT_READY" {
		t.Fatalf("Bingo accepted product-specific version: %v", err)
	}
}

func TestDirectMarkSixFiveElementContractsUseTheFixedOriginalTable(t *testing.T) {
	want := map[string][]int{
		"metal": {6, 7, 20, 21, 28, 29, 36, 37},
		"wood":  {2, 3, 10, 11, 18, 19, 32, 33, 40, 41, 48, 49},
		"water": {8, 9, 16, 17, 24, 25, 38, 39, 46, 47},
		"fire":  {4, 5, 12, 13, 26, 27, 34, 35, 42, 43},
		"earth": {1, 14, 15, 22, 23, 30, 31, 44, 45},
	}
	if !reflect.DeepEqual(markSixFiveElements, want) {
		t.Fatalf("five-element table changed: got=%v want=%v", markSixFiveElements, want)
	}
	for _, test := range directMarkSixRuleCases {
		t.Run(test.gameID, func(t *testing.T) {
			game := &lottery.Game{ID: test.gameID}
			for element, members := range want {
				for special := 1; special <= 49; special++ {
					draw := make([]int, 0, 7)
					for number := 1; number <= 49 && len(draw) < 6; number++ {
						if number != special {
							draw = append(draw, number)
						}
					}
					draw = append(draw, special)
					won, _, err := evaluateBetForRuleVersion(game, test.version, draw, "marksix_five_element_"+element, 7, map[string]string{
						"metal": "金", "wood": "木", "water": "水", "fire": "火", "earth": "土",
					}[element])
					if err != nil || won != containsInt(members, special) {
						t.Fatalf("element=%s special=%d won=%v err=%v", element, special, won, err)
					}
				}
			}
		})
	}
}

func TestDirectMarkSixWebPlacementPersistsProductRuleVersionPostgres(t *testing.T) {
	for index, test := range directMarkSixRuleCases {
		t.Run(test.gameID, func(t *testing.T) {
			db := timingPostgresDatabase(t)
			room := timingPostgresRoom(t, db, "direct_marksix_room_"+test.gameID, "7867"+strconv.Itoa(index))
			member := timingPostgresMember(t, db, room, "direct_marksix_member_"+test.gameID)
			if _, err := NewChatAdminService(db).SetLotteryRoomEnabledForWorkspace(room, test.gameID, true); err != nil {
				t.Fatal(err)
			}
			timingPostgresSettings(t, db, room.ID, `{"seal_seconds":30}`)
			now := time.Now().UTC().Truncate(time.Second)
			issue := "direct-mark-six-" + strconv.Itoa(index+1)
			if err := db.Model(&lottery.Game{}).Where("id = ?", test.gameID).Updates(map[string]any{
				"enabled": true, "source_kind": "external", "timing_source": "upstream", "sync_status": "ok",
				"last_sync_error": "", "last_sync_at": now, "next_issue": issue,
				"next_draw_at": now.Add(3 * time.Minute), "draw_interval": 300,
			}).Error; err != nil {
				t.Fatal(err)
			}
			configureTestGameOdds(t, db, test.gameID, map[string]float64{"marksix_special_a_number": 48})
			service := NewBetAssistantService(db)
			service.bets.suppressNotifications = true
			accepted, err := service.PlaceWeb(member.UserID, test.gameID, issue, []WebBetItem{{
				PlayCode: "marksix_special_a_number", PlayName: "forged", Position: 7, Selection: "48", Amount: 10,
			}}, member.Username, "direct-mark-six-web-"+test.gameID)
			if err != nil {
				t.Fatal(err)
			}
			if accepted.RuleVersion != test.version || accepted.GameID != test.gameID || accepted.BetCount != 1 || accepted.Lines[0].PlayName != "特码A" || accepted.Lines[0].Odds != 48 {
				t.Fatalf("receipt=%+v", accepted)
			}
			var row bet.Bet
			if err := db.Where("game_id = ? AND issue = ? AND user_id = ?", test.gameID, issue, member.UserID).First(&row).Error; err != nil {
				t.Fatal(err)
			}
			if row.RuleVersion != test.version || row.GameID != test.gameID || row.PlayName != "特码A" || row.Odds != 48 {
				t.Fatalf("stored ticket=%+v", row)
			}
		})
	}
}
