package services

import (
	"backend/data/models/user"
	"testing"
)

func TestChatScope(t *testing.T) {
	agentID := uint64(42)
	cases := []struct {
		name     string
		account  user.User
		roomType string
		want     string
	}{
		{"service is always private", user.User{UserID: 11, Role: "member"}, "service", "user:11"},
		{"agent owns its room", user.User{UserID: 42, Role: "agent"}, "group", "agent:42"},
		{"member joins agent room", user.User{UserID: 12, Role: "member", ParentAgentID: &agentID}, "group", "agent:42"},
		{"unassigned member uses lobby", user.User{UserID: 13, Role: "member"}, "group", "lobby"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := chatScope(tc.account, tc.roomType); got != tc.want {
				t.Fatalf("chatScope() = %q, want %q", got, tc.want)
			}
		})
	}
}
