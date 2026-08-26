package utils

import (
	"strings"
	"testing"
)

func TestValidatePasswordBoundaries(t *testing.T) {
	for _, test := range []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "too short", value: strings.Repeat("a", MinPasswordLength-1), wantErr: true},
		{name: "minimum", value: strings.Repeat("a", MinPasswordLength)},
		{name: "maximum", value: strings.Repeat("a", MaxPasswordLength)},
		{name: "too long", value: strings.Repeat("a", MaxPasswordLength+1), wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidatePassword(test.value)
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidatePassword() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestMissingUserPasswordUsesValidHash(t *testing.T) {
	// A malformed dummy hash would make the absent-user branch noticeably
	// cheaper than a real bcrypt comparison and re-introduce enumeration.
	if !CheckPasswordHash("invalid-login-padding-value", dummyPasswordHash) {
		t.Fatal("dummy password hash is not a valid hash for its source value")
	}
	CheckMissingUserPassword("wrong-password")
}
