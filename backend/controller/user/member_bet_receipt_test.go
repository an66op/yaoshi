package user

import (
	"backend/services"
	"strings"
	"testing"
)

func TestAssistantAcceptedReceiptIsCompact(t *testing.T) {
	result := &services.AssistantBetResult{
		GameName: "极速飞艇", Issue: "54776105", Total: 352, Balance: 10035483,
		Lines: []services.AssistantBetLine{{Position: 1, Selection: "4", PlayCode: "ball_1_5", Amount: 352, Label: "冠军[4/352.00]", Odds: 9.9}},
	}
	want := "【极速飞艇 - 54776105】下单成功\n冠军[4/352]\n\n使用：352\n剩余：10035483.00"
	if got := formatAssistantAccepted(result); got != want {
		t.Fatalf("receipt = %q, want %q", got, want)
	}
	if result.Lines[0].Odds != 9.9 {
		t.Fatal("presentation must not change the authoritative odds snapshot")
	}
}

func TestAssistantAcceptedReceiptGroupsExplicitRanks(t *testing.T) {
	lines, err := services.ParseAssistantBet("2/大小单双12/20#1/大小单双12/20")
	if err != nil {
		t.Fatal(err)
	}
	result := &services.AssistantBetResult{GameName: "极速赛车", Issue: "34137265", Lines: lines, Total: 240, Balance: 100717.84}
	want := "【极速赛车 - 34137265】下单成功\n冠军[1/20 2/20 单/20 双/20 大/20 小/20]\n亚军[1/20 2/20 单/20 双/20 大/20 小/20]\n\n使用：240\n剩余：100717.84"
	if got := formatAssistantAccepted(result); got != want {
		t.Fatalf("receipt = %q", got)
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

func TestCompactTextualBetsReachAssistantParser(t *testing.T) {
	for _, content := range []string{"1大5", "和大5", "豹子5", "前三豹子5"} {
		if !isRoomBetContent(content) || !isRoomCommandRequest("group", "speed-racing", content) {
			t.Fatalf("compact bet %q was treated as ordinary chat", content)
		}
	}
}

func TestRepeatedDigitsStillUseServerParsedAmount(t *testing.T) {
	lines, err := services.ParseAssistantBet("4444/88")
	if err != nil || len(lines) != 1 || lines[0].Amount != 352 || lines[0].Selection != "4" {
		t.Fatalf("expected one merged 352-point line, got %#v, err=%v", lines, err)
	}
}

func TestRoomCommandsUseServerBettingPeriodWithoutChangingDrawingPeriod(t *testing.T) {
	if got := roomCommandBettingIssue(nil); got != "" {
		t.Fatalf("missing status invented issue %q", got)
	}
	status := &services.AssistantDrawStatus{Issue: "34137173", IssueStatus: "awaiting_draw"}
	if got := roomCommandBettingIssue(status); got != "34137173" {
		t.Fatalf("ordinary query changed period: %q", got)
	}
	status.BettingWindow = &services.BettingWindow{Issue: "34137174", IssueStatus: "accepting"}
	if got := roomCommandBettingIssue(status); got != "34137174" || status.Issue != "34137173" {
		t.Fatalf("commands did not select separate next period: %q %+v", got, status)
	}
}
