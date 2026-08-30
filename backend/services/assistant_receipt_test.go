package services

import (
	"reflect"
	"testing"
)

func TestBetAmountDisplayPreservesCents(t *testing.T) {
	for amount, want := range map[float64]string{0: "0", 20: "20", 50: "50", 550: "550", 0.5: "0.50", 1.25: "1.25", 12.01: "12.01"} {
		if got := FormatBetAmount(amount); got != want {
			t.Fatalf("amount %v: got %q, want %q", amount, got, want)
		}
	}
}

func TestAssistantGroupedReceiptIntegerStakes(t *testing.T) {
	lines, err := ParseAssistantBet("1/23/50#2/23/50#3/348/50#4/49/50#0/50/50")
	if err != nil {
		t.Fatal(err)
	}
	// A racing placement canonicalizes shorthand 0 to car 10 before receipt.
	for index := range lines {
		if lines[index].Selection == "0" {
			lines[index].Selection = "10"
		}
	}
	want := []string{"冠军[2/50 3/50]", "亚军[2/50 3/50]", "第三名[3/50 4/50 8/50]", "第四名[4/50 9/50]", "第十名[5/50 10/50]"}
	if got := AssistantReceiptLines(lines); !reflect.DeepEqual(got, want) {
		t.Fatalf("receipt: %v", got)
	}
}

func TestAssistantGroupedReceiptPreservesAuthoritativeItems(t *testing.T) {
	lines, err := ParseAssistantBet("2/大小单双12/20#1/大小单双12/20")
	if err != nil || len(lines) != 12 {
		t.Fatalf("mixed input: %+v %v", lines, err)
	}
	before := append([]AssistantBetLine(nil), lines...)
	want := []string{"冠军[1/20 2/20 单/20 双/20 大/20 小/20]", "亚军[1/20 2/20 单/20 双/20 大/20 小/20]"}
	if got := AssistantReceiptLines(lines); !reflect.DeepEqual(got, want) {
		t.Fatalf("grouped receipt: %v", got)
	}
	if !reflect.DeepEqual(lines, before) {
		t.Fatal("receipt presentation mutated order items")
	}
	var total float64
	for _, line := range lines {
		total += line.Amount
	}
	if total != 240 {
		t.Fatalf("incorrect total %v", total)
	}
}

func TestAssistantGroupedReceiptDistinguishesSumAndExplicitChampion(t *testing.T) {
	lines, err := ParseAssistantBet("1/1/80#1/2/80#6/大/5#冠亚/大/10")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"冠军[1/80 2/80]", "第六名[大/5]", "冠亚和[大/10]"}
	if got := AssistantReceiptLines(lines); !reflect.DeepEqual(got, want) {
		t.Fatalf("grouped receipt: %v", got)
	}
}
