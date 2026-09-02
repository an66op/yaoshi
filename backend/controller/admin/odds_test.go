package admin

import (
	apperrors "backend/errors"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestOddsMutationConflictsAreActionableHTTP409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, code := range []string{"ODDS_CONFIGURATION_CONFLICT", "RULE_VERSION_CONFLICT"} {
		t.Run(code, func(t *testing.T) {
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			sendOddsMutationError(context, "保存失败", apperrors.NewBusinessError(code, "赔率配置已更新，请刷新"))
			var payload struct {
				Code      int    `json:"code"`
				ErrorCode string `json:"error_code"`
				Message   string `json:"message"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusConflict || payload.Code != http.StatusConflict || payload.ErrorCode != code || payload.Message != "赔率配置已更新，请刷新" {
				t.Fatalf("conflict response = %d / %+v", response.Code, payload)
			}
		})
	}
}
