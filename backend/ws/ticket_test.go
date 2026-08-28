package ws

import "testing"

func TestTicketIsSingleUse(t *testing.T) {
	store := ticketStore{tickets: make(map[string]connectionTicket)}
	want := SessionIdentity{UserID: 99, AuthVersion: 7, WorkspaceID: 8801}
	ticket, _, err := store.create(want)
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if identity, ok, consumeErr := store.consume(ticket); consumeErr != nil || !ok || identity != want {
		t.Fatalf("first consume = (%+v, %v), want (%+v, true)", identity, ok, want)
	}
	if _, ok, consumeErr := store.consume(ticket); consumeErr != nil || ok {
		t.Fatal("ticket must not be reusable")
	}
}

func TestTicketRejectsIncompleteSessionIdentity(t *testing.T) {
	store := ticketStore{tickets: make(map[string]connectionTicket)}
	for _, identity := range []SessionIdentity{
		{},
		{UserID: 9},
		{UserID: 9, WorkspaceID: 81},
	} {
		if _, _, err := store.create(identity); err == nil {
			t.Fatalf("incomplete identity was accepted: %+v", identity)
		}
	}
}
