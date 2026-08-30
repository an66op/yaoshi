package user

import (
	"backend/services"
	"strings"
	"testing"
)

func TestAssistantAcceptedReceiptIsCompact(t *testing.T) {
	result := &services.AssistantBetResult{
		GameName: "极速飞艇", Issue: "54776105", Total: 352, Balance: 10035483,
		Lines: []services.AssistantBetLine{{Label: "冠军[4/352.00]", Odds: 9.9}},
	}
	want := "【极速飞艇 - 54776105】下单成功\n冠军[4/352.00]\n\n使用：352.00\n剩余：10035483.00"
	if got := formatAssistantAccepted(result); got != want {
		t.Fatalf("receipt = %q, want %q", got, want)
	}
	if result.Lines[0].Odds != 9.9 {
		t.Fatal("presentation must not change the authoritative odds snapshot")
	}
}

func TestIncompleteRoomBetsReachParserWithoutBecomingBets(t *testing.T) {
	for _, content := range []string{"3", "6", "4444", "单", "冠军", "冠军4", "买123", "3,4,5"} {
		t.Run(content, func(t *testing.T) {
			if !isRoomCommandRequest("group", "speed-fly", content) || !isRoomBetContent(content) {
				t.Fatal("incomplete bet must reach the command parser for a durable failure reply")
			}
			lines, err := services.ParseAssistantBet(content)
			if err == nil || len(lines) != 0 {
				t.Fatalf("incomplete input created bet lines: %#v, err=%v", lines, err)
			}
			if !strings.Contains(roomCommandError(err), "缺少金额") {
				t.Fatalf("missing useful parsing feedback: %v", err)
			}
			if isRoomCommandRequest("service", "speed-fly", content) || isRoomCommandRequest("group", "lobby", content) {
				t.Fatal("bet fragments must not become commands outside a game room")
			}
		})
	}
}

func TestOrdinaryRoomTextDoesNotBypassChatBoundary(t *testing.T) {
	for _, content := range []string{"", " ", "...", ".", ",", "，", "#", "好", "大家好", "今天3个人", "大伙一起聊"} {
		if isRoomBetContent(content) || isRoomCommandRequest("group", "speed-fly", content) {
			t.Fatalf("ordinary text %q incorrectly classified as a command", content)
		}
	}
}

func TestRepeatedDigitsStillUseServerParsedAmount(t *testing.T) {
	lines, err := services.ParseAssistantBet("4444/88")
	if err != nil || len(lines) != 1 || lines[0].Amount != 352 || lines[0].Selection != "4" {
		t.Fatalf("expected one merged 352-point line, got %#v, err=%v", lines, err)
	}
}
