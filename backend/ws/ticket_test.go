package ws

import "testing"

func TestTicketIsSingleUse(t *testing.T) {
	store := ticketStore{tickets: make(map[string]connectionTicket)}
	ticket, _, err := store.create(99)
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if userID, ok := store.consume(ticket); !ok || userID != 99 {
		t.Fatalf("first consume = (%d, %v), want (99, true)", userID, ok)
	}
	if _, ok := store.consume(ticket); ok {
		t.Fatal("ticket must not be reusable")
	}
}
