package user

import (
	apperrors "backend/errors"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCheckInBusinessErrorIsNotReportedAsServerFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	sendCheckInError(context, apperrors.NewBusinessError("CHECKIN_ALREADY_COMPLETED", "今日已签到，请明日再来"))

	if recorder.Code != http.StatusConflict {
		t.Fatalf("duplicate check-in status = %d, want 409; body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Message != "今日已签到，请明日再来" {
		t.Fatalf("duplicate check-in message = %q", body.Message)
	}
}
