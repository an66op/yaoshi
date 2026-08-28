package services

import "testing"

func TestValidateAgentRoomCodeUsesFiveToTwelveASCIIDigits(t *testing.T) {
	for _, value := range []string{"10000", "88001", "123456789012"} {
		if err := validateAgentRoomCode(value); err != nil {
			t.Fatalf("validateAgentRoomCode(%q) error = %v", value, err)
		}
	}
	for _, value := range []string{"", "8801", "1234567890123", "１２３４５", "1234a"} {
		if err := validateAgentRoomCode(value); err == nil {
			t.Fatalf("validateAgentRoomCode(%q) unexpectedly succeeded", value)
		}
	}
}
