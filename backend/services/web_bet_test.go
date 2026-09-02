package services

import (
	apperrors "backend/errors"
	"strings"
	"testing"
)

func TestWebBetEnvelopeIsBoundedBeforeDatabaseWork(t *testing.T) {
	service := NewBetAssistantService(nil)
	valid := WebBetItem{PlayCode: "marksix_special_a_number", Position: 7, Selection: "49", Amount: 10}
	for _, test := range []struct {
		name      string
		requestID string
		issue     string
		items     []WebBetItem
	}{
		{"missing request id", "", "123", []WebBetItem{valid}},
		{"short request id", "short", "123", []WebBetItem{valid}},
		{"no items", "web-test-0001", "123", nil},
		{"too many items", "web-test-0002", "123", make([]WebBetItem, 201)},
		{"long issue", "web-test-0003", strings.Repeat("1", 65), []WebBetItem{valid}},
		{"long code", "web-test-0004", "123", []WebBetItem{{PlayCode: strings.Repeat("x", 41), Position: 7, Selection: "49", Amount: 10}}},
		{"long name", "web-test-0005", "123", []WebBetItem{{PlayCode: valid.PlayCode, PlayName: strings.Repeat("名", 41), Position: 7, Selection: "49", Amount: 10}}},
		{"long selection", "web-test-0006", "123", []WebBetItem{{PlayCode: valid.PlayCode, Position: 7, Selection: strings.Repeat("1", 41), Amount: 10}}},
		{"duplicate exact line", "web-test-0007", "123", []WebBetItem{valid, valid}},
		{"duplicate canonical line", "web-test-0008", "123", []WebBetItem{
			{PlayCode: "marksix_combo_2_all", Position: 0, Selection: "1,49", Amount: 10},
			{PlayCode: " MARKSIX_COMBO_2_ALL ", Position: 0, Selection: "49，1", Amount: 20},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.PlaceWeb(1, "bingo-mark-six", test.issue, test.items, "member", test.requestID); apperrors.GetErrorCode(err) != "INVALID_REQUEST" {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestWebBetRejectsNonMarkSixPathBeforeDatabaseWork(t *testing.T) {
	service := NewBetAssistantService(nil)
	_, err := service.PlaceWeb(1, "speed-racing", "123", []WebBetItem{{
		PlayCode: "marksix_special_a_number", Position: 7, Selection: "49", Amount: 10,
	}}, "member", "web-wrong-game-001")
	if apperrors.GetErrorCode(err) != "BET_MODE_UNAVAILABLE" {
		t.Fatalf("non-Mark-Six path reached database work: %v", err)
	}
}
