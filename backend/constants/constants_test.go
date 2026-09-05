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

func TestSendErrorMapsResourceAndDuplicateConflicts(t *testing.T) {
	for _, test := range []struct {
		code    string
		message string
	}{
		{code: "PAYMENT_ACCOUNT_LIMIT_REACHED", message: "收款方式已达到 10 个上限"},
		{code: "CHECKIN_ALREADY_COMPLETED", message: "今日已签到，请明日再来"},
		{code: "PASSWORD_CHANGED_CONCURRENTLY", message: "密码已被其他请求更新，请重新登录后再试"},
	} {
		t.Run(test.code, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			SendError(context, http.StatusBadRequest, "操作失败", apperrors.NewBusinessError(test.code, test.message))
			if recorder.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409; body=%s", recorder.Code, recorder.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["message"] != test.message {
				t.Fatalf("message = %#v, want %q", body["message"], test.message)
			}
		})
	}
}

func TestSendErrorKeepsWrongOldPasswordOutOfAuthenticationExpiry(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	SendError(context, http.StatusUnauthorized, "修改密码失败", apperrors.NewBusinessError("OLD_PASSWORD_INCORRECT", "原密码不正确"))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("OLD_PASSWORD_INCORRECT status = %d, want 400", recorder.Code)
	}
}
