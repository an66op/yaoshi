package user

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestChatSinceRejectsMalformedTimestampBeforeDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/member/chat/messages?game_id=speed-racing&since=not-a-date", nil)
	context.Set("user_id", uint64(42))
	(&memberHandler{}).ListChatMessages(context)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
