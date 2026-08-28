package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPublishDrawRejectsMalformedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/admin/games/speed-racing/draw", strings.NewReader("{"))
	context.Request.Header.Set("Content-Type", "application/json")
	context.Params = gin.Params{{Key: "id", Value: "speed-racing"}}

	(&BetHandler{}).PublishDraw(context)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected malformed JSON to return 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestPromoteAgentRejectsMalformedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/admin/agents/23/promote", strings.NewReader("{"))
	context.Request.Header.Set("Content-Type", "application/json")
	context.Params = gin.Params{{Key: "id", Value: "23"}}

	(&AgentHandler{}).Promote(context)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected malformed JSON to return 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
