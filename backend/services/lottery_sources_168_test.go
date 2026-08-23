package services

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseAPI168RowsObjectAndArray(t *testing.T) {
	object := json.RawMessage(`{"preDrawIssue":"2026092","preDrawTime":"2026-08-22 21:30:00","preDrawCode":"12,9,34,25,40,7,29"}`)
	rows := parseAPI168Rows(object)
	if len(rows) != 1 || rows[0].IssueText() != "2026092" {
		t.Fatalf("unexpected object rows: %+v", rows)
	}

	array := json.RawMessage(`[{"preDrawIssue":34128818,"preDrawTime":"1787469820","preDrawCode":"5,8,1,9,10,6,7,2,4,3"}]`)
	rows = parseAPI168Rows(array)
	if len(rows) != 1 || rows[0].IssueText() != "34128818" {
		t.Fatalf("unexpected array rows: %+v", rows)
	}
}

func TestParse168DrawTime(t *testing.T) {
	location, _ := time.LoadLocation("Asia/Shanghai")
	parsed := parse168DrawTime("2026-08-22 21:30:00").In(location)
	if parsed.Format("2006-01-02 15:04:05") != "2026-08-22 21:30:00" {
		t.Fatalf("datetime parse failed: %s", parsed)
	}
	unix := parse168DrawTime("1787469820")
	if unix.Unix() != 1787469820 {
		t.Fatalf("unix parse failed: %d", unix.Unix())
	}
}

func TestBingoTransforms(t *testing.T) {
	raw := []int{5, 7, 8, 9, 11, 14, 16, 21, 23, 27, 30, 32, 44, 46, 66, 67, 68, 70, 71, 80}
	ssc := bingoSSCNumbers(1)(raw)
	if len(ssc) != 5 || ssc[0] != 7 || ssc[4] != 4 {
		t.Fatalf("unexpected ssc transform: %v", ssc)
	}
	racing := bingoRacingNumbers(0)(raw)
	if len(racing) != 10 {
		t.Fatalf("expected 10 racing numbers, got %v", racing)
	}
	marksix := bingoMarkSixNumbers(raw)
	if len(marksix) != 7 {
		t.Fatalf("expected 7 mark-six numbers, got %v", marksix)
	}
}
