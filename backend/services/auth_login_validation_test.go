package services

import (
	apperrors "backend/errors"
	"strings"
	"testing"
)

func TestLoginRejectsOversizedInputBeforeDatabaseLookup(t *testing.T) {
	service := NewAuthService(nil)
	tests := []struct {
		name      string
		username  string
		password  string
		workspace string
		wantCode  string
	}{
		{name: "username", username: strings.Repeat("王", maxLoginUsernameRunes+1), password: "abcdefgh", workspace: "平台", wantCode: "INVALID_USERNAME"},
		{name: "short password", username: "admin", password: "1234567", workspace: "平台", wantCode: "INVALID_PASSWORD"},
		{name: "password bytes", username: "admin", password: strings.Repeat("王", 25), workspace: "平台", wantCode: "INVALID_PASSWORD"},
		{name: "workspace", username: "admin", password: "abcdefgh", workspace: strings.Repeat("王", maxLoginWorkspaceRunes+1), wantCode: "INVALID_WORKSPACE"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, login := range []struct {
				name string
				call func() error
			}{
				{name: "management", call: func() error { _, _, err := service.Login(test.username, test.password, test.workspace, "admin"); return err }},
				{name: "member", call: func() error {
					_, _, err := service.LoginMember(test.username, test.password, test.workspace)
					return err
				}},
			} {
				t.Run(login.name, func(t *testing.T) {
					// The service intentionally has a nil DB. Reaching any lookup would
					// panic, so a typed validation error also proves ordering.
					err := login.call()
					if code := apperrors.GetErrorCode(err); code != test.wantCode {
						t.Fatalf("error code = %q (%v), want %q", code, err, test.wantCode)
					}
				})
			}
		})
	}
}

func TestValidateLoginInputUsesRunesAndPasswordBytes(t *testing.T) {
	if err := validateLoginInput(strings.Repeat("王", maxLoginUsernameRunes), strings.Repeat("王", 24), strings.Repeat("室", maxLoginWorkspaceRunes)); err != nil {
		t.Fatalf("valid boundary rejected: %v", err)
	}
}

func TestManagementLoginRequiresPersistedRoleNameBeforeDatabaseLookup(t *testing.T) {
	service := NewAuthService(nil)
	for _, role := range []string{"", "platform", "member", "ADMIN"} {
		if _, _, err := service.Login("admin", "Password#2026", "", role); apperrors.GetErrorCode(err) != "INVALID_ROLE" {
			t.Fatalf("role %q error = %v, want INVALID_ROLE", role, err)
		}
	}
}

func TestValidateHumanUsernameUsesUnicodeCharacters(t *testing.T) {
	if err := validateHumanUsername(strings.Repeat("王", 50)); err != nil {
		t.Fatalf("50 Unicode characters rejected: %v", err)
	}
	for _, username := range []string{
		strings.Repeat("王", 51),
		"ab",
		string([]byte{0xff, 'a', 'b'}),
	} {
		if code := apperrors.GetErrorCode(validateHumanUsername(username)); code != "INVALID_USERNAME" {
			t.Fatalf("username %q error code = %q, want INVALID_USERNAME", username, code)
		}
	}
}
