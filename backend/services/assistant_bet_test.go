package services

import (
	"math"
	"testing"
)

func TestParseAssistantBetCompactTicket(t *testing.T) {
	lines, err := ParseAssistantBet("买12345/1000#3大/2000#6/123456/100")
	if err != nil {
		t.Fatalf("ParseAssistantBet returned error: %v", err)
	}
	if len(lines) != 12 {
		t.Fatalf("expected 12 parsed lines, got %d", len(lines))
	}
	var total float64
	for _, line := range lines {
		total += line.Amount
	}
	if math.Abs(total-3100) > 0.00001 {
		t.Fatalf("expected exact ticket total 3100, got %.2f", total)
	}
	if lines[0].Position != 1 || lines[0].Selection != "1" || lines[0].Amount != 200 {
		t.Fatalf("unexpected first compact line: %#v", lines[0])
	}
	if lines[5].Position != 3 || lines[5].Selection != "大" || lines[5].Amount != 2000 {
		t.Fatalf("unexpected side-bet line: %#v", lines[5])
	}
}

func TestParseAssistantBetSplitsCentsExactly(t *testing.T) {
	lines, err := ParseAssistantBet("6/123456/100")
	if err != nil {
		t.Fatalf("ParseAssistantBet returned error: %v", err)
	}
	if len(lines) != 6 {
		t.Fatalf("expected six selections, got %d", len(lines))
	}
	var cents int64
	for _, line := range lines {
		cents += int64(math.Round(line.Amount * 100))
	}
	if cents != 10000 {
		t.Fatalf("expected 10000 cents after split, got %d", cents)
	}
}

func TestParseAssistantBetRejectsInvalidSyntax(t *testing.T) {
	if _, err := ParseAssistantBet("冠军大"); err == nil {
		t.Fatal("expected missing amount to be rejected")
	}
}
