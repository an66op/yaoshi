package services

import (
	apperrors "backend/errors"
	"testing"
)

func TestOwnerAccountCreationRequiresLoginCredentials(t *testing.T) {
	for _, role := range []string{"tenant", "agent"} {
		for _, tc := range []struct {
			name, username, password, code string
		}{
			{"missing account", "", "OwnerFixture#2026_a9", "INVALID_USERNAME"},
			{"missing password", "owner_fixture", "", "INVALID_PASSWORD"},
			{"short password", "owner_fixture", "short", "INVALID_PASSWORD"},
		} {
			t.Run(role+"/"+tc.name, func(t *testing.T) {
				// A nil DB proves invalid credentials are rejected before any room
				// or owner can be created, not merely by a client-side validator.
				var err error
				if role == "tenant" {
					_, err = NewTenantAdminService(nil).Create(TenantPayload{Username: tc.username, Password: tc.password, RoomCode: "77621", Status: 1})
				} else {
					_, err = NewAgentAdminService(nil).Create(CreateAgentInput{Username: tc.username, Password: tc.password, RoomCode: "77622", Status: 1})
				}
				if apperrors.GetErrorCode(err) != tc.code {
					t.Fatalf("owner creation accepted invalid login credentials: got %v want %s", err, tc.code)
				}
			})
		}
	}
}
