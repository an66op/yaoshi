package services

import (
	"backend/data/models/user"
	"testing"
)

func TestServiceWelcomeMessageIsDurableAndPrivate(t *testing.T) {
	account := user.User{UserID: 42, WorkspaceID: 8}
	row := newServiceWelcomeMessage(account, "user:42", "agent:9", "service")

	if row.WorkspaceID != 8 || row.UserID != 0 {
		t.Fatalf("welcome ownership = workspace %d user %d", row.WorkspaceID, row.UserID)
	}
	if row.RoomType != "service" || row.Scope != "user:42" || row.RoomScope != "agent:9" || row.GameID != "service" {
		t.Fatalf("welcome escaped its private service conversation: %#v", row)
	}
	if row.Username != "support" || row.Nickname != "客服小七" || row.MessageType != "welcome" {
		t.Fatalf("unexpected persisted support identity: %#v", row)
	}
	if row.Content != serviceWelcomeContent || row.Content == "" {
		t.Fatalf("unexpected greeting content: %q", row.Content)
	}
}
