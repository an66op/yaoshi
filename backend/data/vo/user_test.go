package vo

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLoginResponseOmitsEmptyBearerToken(t *testing.T) {
	payload, err := json.Marshal(LoginResponse{User: UserResponse{ID: 7, Username: "member"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), `"token"`) {
		t.Fatalf("browser login response exposed bearer token field: %s", payload)
	}
}
