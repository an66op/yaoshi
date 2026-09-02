package services

import (
	"backend/data/models/bet"
	apperrors "backend/errors"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPlacementRequestReferenceOwnsOneFinancialOperation(t *testing.T) {
	seen := make(map[string]bool)
	for index := 0; index < 100; index++ {
		reference, err := placementRequestReference([]PlaceBetInput{{}, {}})
		if err != nil || !strings.HasPrefix(reference, "internal_bet:") || seen[reference] {
			t.Fatalf("internal calls reused an operation: %q / %v", reference, err)
		}
		if nonce, err := hex.DecodeString(strings.TrimPrefix(reference, "internal_bet:")); err != nil || len(nonce) != 16 {
			t.Fatalf("invalid internal reference: %q / %v", reference, err)
		}
		seen[reference] = true
	}
	for _, reference := range []string{directBetRequestReference(41), assistantBetRequestReference(41)} {
		actual, err := placementRequestReference([]PlaceBetInput{{LedgerReference: " " + reference}, {LedgerReference: reference + " "}})
		if err != nil || actual != reference {
			t.Fatalf("member idempotency reference changed: %q / %v", actual, err)
		}
	}
	for _, inputs := range [][]PlaceBetInput{
		nil,
		{{}, {LedgerReference: "assistant_request:1"}},
		{{LedgerReference: "assistant_request:1"}, {}},
		{{LedgerReference: "assistant_request:1"}, {LedgerReference: "assistant_request:2"}},
	} {
		if _, err := placementRequestReference(inputs); apperrors.GetErrorCode(err) != "INVALID_REQUEST" {
			t.Fatalf("mixed debit evidence accepted: %+v / %v", inputs, err)
		}
	}
}

func placementTestDraft() bet.Bet {
	return bet.Bet{
		WorkspaceID: 23, UserID: 81, Username: "placement-fixture", RoomScope: "tenant:71",
		GameID: "speed-racing", Issue: "99101", PlayCode: "ball_1_5", PlayName: "冠军", Position: 1, Selection: "2",
		RuleVersion: "racing-v2", RequestReference: "internal_bet:fixture", Status: "pending",
		AmountCents: 2000, FlyCents: 200, Odds: 9.9, RebateRateSnapshot: 0.5, AgentShareRateSnapshot: 30,
	}
}

func TestAggregatePlacementRowsOnlyCombinesSameRequestDrafts(t *testing.T) {
	first := placementTestDraft()
	second := first
	second.Selection = "3"
	third := first
	third.AmountCents, third.FlyCents = 3000, 400
	inputs := []bet.Bet{first, second, third}
	before := append([]bet.Bet(nil), inputs...)
	rows, indexes, err := aggregatePlacementRows(inputs)
	if err != nil || len(rows) != 2 || !reflect.DeepEqual(indexes, []int{0, 1, 0}) {
		t.Fatalf("draft aggregation/index mapping: %+v %v / %v", rows, indexes, err)
	}
	expected := first
	expected.AmountCents, expected.FlyCents = 5000, 600
	if !reflect.DeepEqual(rows[0], expected) || !reflect.DeepEqual(rows[1], second) || !reflect.DeepEqual(inputs, before) {
		t.Fatalf("aggregation mutated terms or caller inputs: %+v inputs=%+v", rows, inputs)
	}
}

func TestAggregatePlacementRowsDoesNotCrossContractKeys(t *testing.T) {
	for name, mutate := range map[string]func(*bet.Bet){
		"request":   func(row *bet.Bet) { row.RequestReference = "internal_bet:other" },
		"version":   func(row *bet.Bet) { row.RuleVersion = "" },
		"workspace": func(row *bet.Bet) { row.WorkspaceID++ },
		"user":      func(row *bet.Bet) { row.UserID++ },
		"scope":     func(row *bet.Bet) { row.RoomScope = "tenant:72" },
		"game":      func(row *bet.Bet) { row.GameID = "speed-fly" },
		"issue":     func(row *bet.Bet) { row.Issue = "99102" },
		"position":  func(row *bet.Bet) { row.Position = 2 },
		"selection": func(row *bet.Bet) { row.Selection = "4" },
		"play":      func(row *bet.Bet) { row.PlayCode = "sum" },
	} {
		t.Run(name, func(t *testing.T) {
			first := placementTestDraft()
			second := first
			mutate(&second)
			rows, indexes, err := aggregatePlacementRows([]bet.Bet{first, second})
			if err != nil || len(rows) != 2 || !reflect.DeepEqual(indexes, []int{0, 1}) {
				t.Fatalf("different contract collapsed: %+v %v / %v", rows, indexes, err)
			}
		})
	}
}

func TestAggregatePlacementRowsRejectsChangedFinancialSnapshots(t *testing.T) {
	for name, mutate := range map[string]func(*bet.Bet){
		"odds":   func(row *bet.Bet) { row.Odds = 8 },
		"rebate": func(row *bet.Bet) { row.RebateRateSnapshot = 1.5 },
		"share":  func(row *bet.Bet) { row.AgentShareRateSnapshot = 40 },
	} {
		t.Run(name, func(t *testing.T) {
			first := placementTestDraft()
			second := first
			mutate(&second)
			if rows, _, err := aggregatePlacementRows([]bet.Bet{first, second}); rows != nil || apperrors.GetErrorCode(err) != "INVALID_REQUEST" {
				t.Fatalf("changed %s averaged/replaced old terms: %+v / %v", name, rows, err)
			}
		})
	}
}

func TestAggregatePlacementRowsRejectsHistoricOrInvalidRows(t *testing.T) {
	for name, mutate := range map[string]func(*bet.Bet){
		"saved pending": func(row *bet.Bet) { row.ID = 1 },
		"cancelled":     func(row *bet.Bet) { row.Status = "cancelled" },
		"won":           func(row *bet.Bet) { row.Status = "won" },
		"lost":          func(row *bet.Bet) { row.Status = "lost" },
		"settled time":  func(row *bet.Bet) { at := time.Now(); row.SettledAt = &at },
		"payout":        func(row *bet.Bet) { row.PayoutCents = 1 },
		"rebate":        func(row *bet.Bet) { row.RebateCents = 1 },
		"share":         func(row *bet.Bet) { row.AgentShareCents = 1 },
		"unlinked":      func(row *bet.Bet) { row.RequestReference = " " },
		"zero amount":   func(row *bet.Bet) { row.AmountCents = 0 },
		"negative fly":  func(row *bet.Bet) { row.FlyCents = -1 },
		"excess fly":    func(row *bet.Bet) { row.FlyCents = row.AmountCents + 1 },
	} {
		t.Run(name, func(t *testing.T) {
			row := placementTestDraft()
			mutate(&row)
			if rows, _, err := aggregatePlacementRows([]bet.Bet{row}); rows != nil || apperrors.GetErrorCode(err) != "INVALID_REQUEST" {
				t.Fatalf("unsafe row became a draft: %+v / %v", rows, err)
			}
		})
	}
	row := placementTestDraft()
	row.AmountCents, row.FlyCents = maxSignedInt64, 0
	if rows, _, err := aggregatePlacementRows([]bet.Bet{row, placementTestDraft()}); rows != nil || apperrors.GetErrorCode(err) != "INVALID_REQUEST" {
		t.Fatalf("overflow accepted: %+v / %v", rows, err)
	}
}
