package services

import (
	"backend/data/models/bet"
	"backend/data/models/chat"
	"backend/data/models/lottery"
	apperrors "backend/errors"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

// Opt-in only: timingPostgresDatabase refuses non-loopback, incorrectly named,
// or nonempty databases and rolls its complete fixture schema back on cleanup.
// These tests never read the business DB configuration or call a remote source.
func TestSettlementRulesPostgresAtomicity(t *testing.T) {
	db := timingPostgresDatabase(t)
	room := timingPostgresRoom(t, db, "rule_atomic_tenant", "76801")
	member := timingPostgresMember(t, db, room, "rule_atomic_member")
	service := NewBetAdminService(db)
	service.suppressNotifications = true
	seed := func(t *testing.T, gameID, issue string, numbers []int, versions []string) []bet.Bet {
		t.Helper()
		draw := lottery.Draw{GameID: gameID, Issue: issue, Numbers: joinNumbers(numbers), DrawAt: time.Now().UTC().Add(-time.Minute)}
		if err := db.Create(&draw).Error; err != nil {
			t.Fatal(err)
		}
		rows := make([]bet.Bet, 0, len(versions))
		for index, version := range versions {
			row := bet.Bet{
				WorkspaceID: room.ID, GameID: gameID, Issue: issue, RoomScope: room.Scope,
				UserID: member.UserID, Username: member.Username, RuleVersion: version,
				PlayCode: "sum", PlayName: "总和", Position: 6, Selection: "大",
				AmountCents: 200, Odds: 2, Status: "pending", Remark: "accepted fixture",
				RequestReference: fmt.Sprintf("rule-atomic:%s:%d", issue, index),
			}
			if err := db.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
			rows = append(rows, row)
		}
		return rows
	}
	readTickets := func(t *testing.T, gameID, issue string) []bet.Bet {
		t.Helper()
		var rows []bet.Bet
		if err := db.Where("game_id = ? AND issue = ?", gameID, issue).Order("id ASC").Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		return rows
	}
	twenty := make([]int, 20)
	for i := range twenty {
		twenty[i] = i + 1
	}
	for index, test := range []struct {
		name, gameID, code string
		numbers            []int
		versions           []string
	}{
		{"later unknown version rolls back prior winner", "speed-ssc", "RULES_NOT_READY", []int{9, 9, 9, 9, 9}, []string{"digits5-v2", "unknown-v99"}},
		{"later wrong family rolls back prior winner", "speed-ssc", "RULES_NOT_READY", []int{9, 9, 9, 9, 9}, []string{"digits5-v2", "racing-v2"}},
		{"invalid digit shape preserves both versions", "speed-ssc", "INVALID_DRAW", []int{9, 9, 9, 9}, []string{"digits5-v2", ""}},
		{"twenty balls are not racing", "speed-racing", "INVALID_DRAW", twenty, []string{"racing-v2", ""}},
		{"unmodelled legacy history stays pending", "official-tw-bingo", "RULES_NOT_READY", twenty, []string{""}},
	} {
		t.Run(test.name, func(t *testing.T) {
			issue := fmt.Sprintf("961%02d", index)
			seed(t, test.gameID, issue, test.numbers, test.versions)
			beforeRows := readTickets(t, test.gameID, issue)
			beforeMoney := timingPostgresMoney(t, db, member.UserID)
			beforeEffects := rulesPostgresTableCounts(t, db, []string{"member_notifications", "member_chat_messages", "user_balance_transactions"})
			result, err := service.SettleIssue(test.gameID, issue, "rule-atomic-test")
			if result != nil || apperrors.GetErrorCode(err) != test.code {
				t.Fatalf("result=%+v err=%v, want %s without settlement", result, err, test.code)
			}
			if after := readTickets(t, test.gameID, issue); !reflect.DeepEqual(after, beforeRows) {
				t.Fatalf("failed settlement rewrote tickets: before=%+v after=%+v", beforeRows, after)
			}
			if after := timingPostgresMoney(t, db, member.UserID); after != beforeMoney {
				t.Fatalf("failed settlement changed money: before=%+v after=%+v", beforeMoney, after)
			}
			if after := rulesPostgresTableCounts(t, db, []string{"member_notifications", "member_chat_messages", "user_balance_transactions"}); !reflect.DeepEqual(after, beforeEffects) {
				t.Fatalf("failed settlement committed side effects: before=%v after=%v", beforeEffects, after)
			}
			lifecycle := rolloverPostgresIssue(t, db, test.gameID, issue)
			if lifecycle.Status != lottery.IssueStatusError || lifecycle.LastError == "" {
				t.Fatalf("failed issue lost its reviewable error: %+v", lifecycle)
			}
		})
	}
	t.Run("mixed versions settle under separate snapshots and retry once", func(t *testing.T) {
		const issue = "96199"
		seed(t, "speed-ssc", issue, []int{9, 9, 9, 9, 9}, []string{"", "digits5-v2"})
		before := timingPostgresMoney(t, db, member.UserID)
		result, err := service.SettleIssue("speed-ssc", issue, "rule-version-test")
		if err != nil || result.Won != 1 || result.Lost != 1 || result.PayoutAmount != 4 {
			t.Fatalf("mixed versions did not retain separate meanings: result=%+v err=%v", result, err)
		}
		rows := readTickets(t, "speed-ssc", issue)
		if len(rows) != 2 || rows[0].RuleVersion != "" || rows[0].Status != "lost" || rows[0].PayoutCents != 0 ||
			rows[1].RuleVersion != "digits5-v2" || rows[1].Status != "won" || rows[1].PayoutCents != 400 {
			t.Fatalf("rule snapshots were changed or ignored: %+v", rows)
		}
		after := timingPostgresMoney(t, db, member.UserID)
		if after.BalanceCents != before.BalanceCents+400 || after.LedgerRows != before.LedgerRows+1 {
			t.Fatalf("versioned payout not credited exactly once: before=%+v after=%+v", before, after)
		}
		if _, err := service.SettleIssue("speed-ssc", issue, "rule-version-retry"); err != nil {
			t.Fatal(err)
		}
		if retryRows := readTickets(t, "speed-ssc", issue); !reflect.DeepEqual(retryRows, rows) {
			t.Fatalf("already-settled historical rows were recalculated: before=%+v after=%+v", rows, retryRows)
		}
		if retryMoney := timingPostgresMoney(t, db, member.UserID); retryMoney != after {
			t.Fatalf("retry paid twice: before=%+v after=%+v", after, retryMoney)
		}
	})
	t.Run("empty-version upgraded digits retain legacy labels without backfill", func(t *testing.T) {
		const issue = "96198"
		draw := lottery.Draw{GameID: "speed-ssc", Issue: issue, Numbers: "1,1,2,3,4", DrawAt: time.Now().UTC().Add(-time.Minute)}
		if err := db.Create(&draw).Error; err != nil {
			t.Fatal(err)
		}
		rows := []bet.Bet{
			{
				WorkspaceID: room.ID, GameID: "speed-ssc", Issue: issue, RoomScope: room.Scope,
				UserID: member.UserID, Username: member.Username, RuleVersion: "",
				PlayCode: "pair", PlayName: "对子", Position: 1, Selection: "pair",
				AmountCents: 200, Odds: 2, Status: "pending", RequestReference: "legacy-label:pair",
			},
			{
				WorkspaceID: room.ID, GameID: "speed-ssc", Issue: issue, RoomScope: room.Scope,
				UserID: member.UserID, Username: member.Username, RuleVersion: "",
				PlayCode: "dragon_tiger", PlayName: "龙虎", Position: 2, Selection: "虎",
				AmountCents: 200, Odds: 2, Status: "pending", RequestReference: "legacy-label:dragon-tiger",
			},
		}
		if err := db.Create(&rows).Error; err != nil {
			t.Fatal(err)
		}
		result, err := service.SettleIssue("speed-ssc", issue, "legacy-label-test")
		if err != nil || result.Won != 2 || result.PayoutAmount != 8 {
			t.Fatalf("empty-version tickets did not retain legacy payout: %+v %v", result, err)
		}
		var settled []bet.Bet
		if err := db.Where("game_id = ? AND issue = ?", "speed-ssc", issue).Order("id ASC").Find(&settled).Error; err != nil {
			t.Fatal(err)
		}
		if len(settled) != 2 || settled[0].RuleVersion != "" || settled[1].RuleVersion != "" || settled[0].Status != "won" || settled[1].Status != "won" {
			t.Fatalf("settlement backfilled or changed legacy rows: %+v", settled)
		}
		var messages []chat.Message
		if err := db.Where("game_id = ? AND reference_id = ? AND message_type = ?", "speed-ssc", draw.ID, "settlement").Find(&messages).Error; err != nil {
			t.Fatal(err)
		}
		if len(messages) != 1 || !strings.Contains(messages[0].Content, "前三对子 [pair/2.00") || !strings.Contains(messages[0].Content, "第2球 [虎/2.00") || strings.Contains(messages[0].Content, "冠军 [") || strings.Contains(messages[0].Content, "亚军 [") {
			t.Fatalf("cross-version settlement displayed racing/v3 labels: %+v", messages)
		}
	})
}

func rulesPostgresTableCounts(t *testing.T, db *gorm.DB, tables []string) map[string]int64 {
	t.Helper()
	counts := make(map[string]int64, len(tables))
	for _, table := range tables {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		counts[table] = count
	}
	return counts
}

func TestDrawGenerationPostgresFailureIsReadOnly(t *testing.T) {
	db := timingPostgresDatabase(t)
	service := NewBetAdminService(db)
	service.suppressNotifications = true
	tables := []string{"lottery_draws", "lottery_issues", "lottery_issue_windows", "lottery_bets", "user_balance_transactions", "member_notifications", "member_chat_messages"}
	for index, test := range []struct {
		name, gameID, sourceKind, code string
		numbers                        []int
	}{
		{"platform entropy failure", "sg-ssc", "platform", "DRAW_RANDOM_FAILED", nil},
		{"simulated entropy failure", "sg-ssc", "simulated", "DRAW_RANDOM_FAILED", nil},
		{"legacy platform entropy failure", "sg-ssc", "", "DRAW_RANDOM_FAILED", nil},
		{"external missing result", "speed-racing", "external", "DRAW_NOT_FOUND", nil},
		{"official missing result", "official-fc3d", "official", "DRAW_NOT_FOUND", nil},
		{"unmodelled generator", "happy8-mark-six", "platform", "RULES_NOT_READY", nil},
		{"invalid explicit result", "sg-ssc", "platform", "INVALID_DRAW", []int{1, 2, 3}},
	} {
		t.Run(test.name, func(t *testing.T) {
			issue := fmt.Sprintf("962%02d", index)
			if err := db.Model(&lottery.Game{}).Where("id = ?", test.gameID).Updates(map[string]any{
				"source_kind": test.sourceKind, "next_issue": issue, "next_draw_at": time.Now().UTC().Add(time.Minute),
			}).Error; err != nil {
				t.Fatal(err)
			}
			var beforeGame lottery.Game
			if err := db.First(&beforeGame, "id = ?", test.gameID).Error; err != nil {
				t.Fatal(err)
			}
			before := rulesPostgresTableCounts(t, db, tables)
			result, err := service.publishDrawWithEntropy(test.gameID, issue, test.numbers, "generator-failure-test", failingDrawEntropy{errors.New("fixture entropy unavailable")})
			if result != nil || apperrors.GetErrorCode(err) != test.code {
				t.Fatalf("result=%+v err=%v, want %s", result, err, test.code)
			}
			var afterGame lottery.Game
			if err := db.First(&afterGame, "id = ?", test.gameID).Error; err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(afterGame, beforeGame) {
				t.Fatalf("failed generator advanced or changed game: before=%+v after=%+v", beforeGame, afterGame)
			}
			if after := rulesPostgresTableCounts(t, db, tables); !reflect.DeepEqual(after, before) {
				t.Fatalf("failed generator wrote lifecycle, draw, or financial data: before=%v after=%v", before, after)
			}
		})
	}
	t.Run("explicit external result preserves manual publish behavior", func(t *testing.T) {
		const issue = "96299"
		if err := db.Model(&lottery.Game{}).Where("id = ?", "speed-racing").Updates(map[string]any{
			"source_kind": "external", "next_issue": issue, "next_draw_at": time.Now().UTC().Add(time.Minute),
		}).Error; err != nil {
			t.Fatal(err)
		}
		numbers := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
		result, err := service.publishDrawWithEntropy("speed-racing", issue, numbers, "manual-fixture", failingDrawEntropy{errors.New("must not read entropy")})
		if err != nil || result == nil || !reflect.DeepEqual(result.Numbers, numbers) {
			t.Fatalf("explicit external result was changed/rejected: result=%+v err=%v", result, err)
		}
	})
}
