package constants

import (
	apperrors "backend/errors"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSendErrorRecognizesWrappedBusinessError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	err := fmt.Errorf("review failed: %w", apperrors.NewBusinessError("INVALID_REQUEST", "参数不正确"))
	SendError(context, http.StatusInternalServerError, "审核失败", err)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	var body map[string]any
	if decodeErr := json.Unmarshal(recorder.Body.Bytes(), &body); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}
	if body["message"] != "参数不正确" {
		t.Fatalf("message = %#v", body["message"])
	}
}
