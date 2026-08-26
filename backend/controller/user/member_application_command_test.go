package user

import "testing"

func TestParseRoomApplicationCommand(t *testing.T) {
	tests := []struct {
		content     string
		wantMatched bool
		wantType    string
		wantAmount  float64
	}{
		{content: "上分100", wantMatched: true, wantType: "credit", wantAmount: 100},
		{content: "申请上分 200.50", wantMatched: true, wantType: "credit", wantAmount: 200.5},
		{content: "下分/10", wantMatched: true, wantType: "debit", wantAmount: 10},
		{content: "下分：88 已经沟通", wantMatched: true, wantType: "debit", wantAmount: 88},
		{content: "1/12345/100", wantMatched: false},
		{content: "上分", wantMatched: false},
		{content: "下分0", wantMatched: false},
	}
	for _, test := range tests {
		t.Run(test.content, func(t *testing.T) {
			command, matched := parseRoomApplicationCommand(test.content)
			if matched != test.wantMatched {
				t.Fatalf("matched = %v, want %v", matched, test.wantMatched)
			}
			if !matched {
				return
			}
			if command.RequestType != test.wantType || command.Amount != test.wantAmount {
				t.Fatalf("command = %#v, want type %q amount %.2f", command, test.wantType, test.wantAmount)
			}
		})
	}
}
