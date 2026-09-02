package user

import (
	"backend/services"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type recordingMarkSixWebAssistant struct {
	memberBetAssistant
	calls     int
	userID    uint64
	gameID    string
	issue     string
	items     []services.WebBetItem
	operator  string
	requestID string
}

func (s *recordingMarkSixWebAssistant) PlaceWeb(userID uint64, gameID, issue string, items []services.WebBetItem, operator, requestID string) (*services.AssistantBetResult, error) {
	s.calls++
	s.userID, s.gameID, s.issue = userID, gameID, issue
	s.items, s.operator, s.requestID = items, operator, requestID
	return &services.AssistantBetResult{GameID: gameID, Issue: issue, BetCount: len(items)}, nil
}

func TestBingoMarkSixChatBetBoundaryRejectsOnlyNewChatBets(t *testing.T) {
	for _, content := range []string{"7/49/10", "买49/20", "梭哈", "重复"} {
		if !bingoMarkSixChatBetBlocked("bingo-mark-six", content) {
			t.Fatalf("chat bet was not blocked: %q", content)
		}
	}
	for _, content := range []string{"查", "取消", "上分100", "下分50", "普通聊天"} {
		if bingoMarkSixChatBetBlocked("bingo-mark-six", content) {
			t.Fatalf("non-placement room action was blocked: %q", content)
		}
	}
	if bingoMarkSixChatBetBlocked("speed-racing", "7/49/10") {
		t.Fatal("another game's chat parser was changed")
	}
}

func TestBingoMarkSixChatBetIsRejectedBeforeChatPersistence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &memberHandler{} // A nil chat service proves the boundary returns first.
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set("user_id", uint64(42)) })
	engine.POST("/chat/commands", handler.PostChatCommand)

	for _, content := range []string{"7/49/10", "买49/20", "重复"} {
		request := httptest.NewRequest(http.MethodPost, "/chat/commands", strings.NewReader(`{"room_type":"group","game_id":"bingo-mark-six","content":"`+content+`","request_id":"request-123"}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "不支持聊天投注") {
			t.Fatalf("content=%q status=%d body=%s", content, response.Code, response.Body.String())
		}
	}
}

func TestBingoMarkSixWebHTTPBindsSessionPathAndTopLevelIssue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assistant := &recordingMarkSixWebAssistant{}
	handler := &memberHandler{assistant: assistant}
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("user_id", uint64(42))
		c.Set("username", "session-member")
	})
	engine.POST("/games/:id/web-bets", handler.WebBets)
	body := `{
		"game_id":"client-game-must-be-ignored",
		"issue":"986001",
		"request_id":"web-http-bind-001",
		"items":[{
			"game_id":"client-item-game-must-be-ignored",
			"issue":"client-item-issue-must-be-ignored",
			"play_code":"marksix_combo_2_all",
			"play_name":"client-name",
			"position":0,
			"selection":"1,2",
			"amount":20,
			"odds":999
		}]
	}`
	request := httptest.NewRequest(http.MethodPost, "/games/bingo-mark-six/web-bets", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || assistant.calls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, assistant.calls, response.Body.String())
	}
	if assistant.userID != 42 || assistant.gameID != "bingo-mark-six" || assistant.issue != "986001" || assistant.operator != "session-member" || assistant.requestID != "web-http-bind-001" {
		t.Fatalf("HTTP authority binding changed: %+v", assistant)
	}
	if len(assistant.items) != 1 || assistant.items[0].PlayCode != "marksix_combo_2_all" || assistant.items[0].Selection != "1,2" || assistant.items[0].Amount != 20 {
		t.Fatalf("typed item binding changed: %+v", assistant.items)
	}
}

func TestBingoMarkSixWebHTTPRejectsOversizedBodyBeforeService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assistant := &recordingMarkSixWebAssistant{}
	handler := &memberHandler{assistant: assistant}
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set("user_id", uint64(42)) })
	engine.POST("/games/:id/web-bets", handler.WebBets)
	body := `{"issue":"1","request_id":"web-http-large-001","items":[{"play_code":"marksix_special_a_number","position":7,"selection":"` + strings.Repeat("1", 300<<10) + `","amount":10}]}`
	request := httptest.NewRequest(http.MethodPost, "/games/bingo-mark-six/web-bets", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || assistant.calls != 0 {
		t.Fatalf("oversized body reached service: status=%d calls=%d body=%s", response.Code, assistant.calls, response.Body.String())
	}
}
