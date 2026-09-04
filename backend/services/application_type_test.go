package services

import "testing"

func TestValidRequestTypeRetiresRoleMutationApplication(t *testing.T) {
	for _, value := range []string{"credit", "debit", "join"} {
		if got, err := validRequestType(value); err != nil || got != value {
			t.Fatalf("valid type %q rejected: got=%q err=%v", value, got, err)
		}
	}
	if _, err := validRequestType("agent"); err == nil {
		t.Fatal("retired agent role-mutation application was accepted")
	}
}
