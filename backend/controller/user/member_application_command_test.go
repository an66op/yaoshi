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
		{content: "上分100 线下已确认", wantMatched: true, wantType: "credit", wantAmount: 100},
		{content: "1/12345/100", wantMatched: false},
		{content: "上分", wantMatched: false},
		{content: "下分0", wantMatched: false},
		{content: "上分100.123", wantMatched: false},
		{content: "上分100.", wantMatched: false},
		{content: "上分1e3", wantMatched: false},
		{content: "上分100备注", wantMatched: false},
		{content: "下分-10", wantMatched: false},
		{content: "下分100/其他", wantMatched: false},
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

func TestIsRoomCommandRequest(t *testing.T) {
	tests := []struct {
		name     string
		roomType string
		gameID   string
		content  string
		want     bool
	}{
		{name: "compact bet", roomType: "group", gameID: "speed-racing", content: "1/12345/100", want: true},
		{name: "application", roomType: "group", gameID: "speed-racing", content: "上分100", want: true},
		{name: "cancel", roomType: "group", gameID: "speed-racing", content: "取消", want: true},
		{name: "query", roomType: "group", gameID: "speed-racing", content: "查", want: true},
		{name: "repeat", roomType: "group", gameID: "speed-racing", content: "重复", want: true},
		{name: "all in", roomType: "group", gameID: "speed-racing", content: "大梭哈", want: true},
		{name: "ordinary chat", roomType: "group", gameID: "speed-racing", content: "大家好", want: false},
		{name: "invalid application amount", roomType: "group", gameID: "speed-racing", content: "上分100.123", want: false},
		{name: "lobby has no betting commands", roomType: "group", gameID: "lobby", content: "1/12345/100", want: false},
		{name: "service room has no betting commands", roomType: "service", gameID: "speed-racing", content: "1/12345/100", want: false},
		{name: "missing game", roomType: "group", gameID: "", content: "1/12345/100", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isRoomCommandRequest(test.roomType, test.gameID, test.content); got != test.want {
				t.Fatalf("isRoomCommandRequest(%q, %q, %q) = %v, want %v", test.roomType, test.gameID, test.content, got, test.want)
			}
		})
	}
}
