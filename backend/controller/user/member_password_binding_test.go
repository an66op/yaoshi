package user

import (
	apperrors "backend/errors"
	"backend/utils"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func bindChangePasswordRequest(t *testing.T, body string) (changePasswordRequest, error) {
	t.Helper()
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("POST", "/api/member/change-password", bytes.NewBufferString(body))
	context.Request.Header.Set("Content-Type", "application/json")
	var request changePasswordRequest
	err := context.ShouldBindJSON(&request)
	return request, err
}

func TestWrongOldPasswordIsARecoverableFormError(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	sendChangePasswordError(context, apperrors.NewBusinessError("OLD_PASSWORD_INCORRECT", "原密码不正确"))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("wrong old password status = %d, want 400", recorder.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["message"] != "原密码不正确" {
		t.Fatalf("wrong old password message = %#v", body["message"])
	}
}

func TestChangePasswordBindingLeavesUTF8ByteLengthToServiceValidator(t *testing.T) {
	gin.SetMode(gin.TestMode)

	request, err := bindChangePasswordRequest(t, `{"old_password":"current-secret","new_password":"密码好"}`)
	if err != nil {
		t.Fatalf("three-rune, nine-byte password was rejected by HTTP binding: %v", err)
	}
	if err := utils.ValidatePassword(request.NewPassword); err != nil {
		t.Fatalf("service rejected valid UTF-8 byte length: %v", err)
	}

	request, err = bindChangePasswordRequest(t, fmt.Sprintf(`{"old_password":"current-secret","new_password":"%s"}`, strings.Repeat("密", 25)))
	if err != nil {
		t.Fatalf("HTTP binding should defer maximum length to the service: %v", err)
	}
	if err := utils.ValidatePassword(request.NewPassword); err == nil {
		t.Fatal("service accepted a password longer than 72 UTF-8 bytes")
	}

	if _, err := bindChangePasswordRequest(t, `{"old_password":"current-secret","new_password":""}`); err == nil {
		t.Fatal("HTTP binding accepted an empty new password")
	}
}
