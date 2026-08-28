package ws

import (
	"backend/cluster"
	"testing"
	"time"
)

func TestClusterEnvelopeHonorsRecipientAndWorkspace(t *testing.T) {
	previous := defaultHub
	defaultHub = NewHub()
	t.Cleanup(func() { defaultHub = previous })
	defaultHub.setSessionValidator(func(SessionIdentity) bool { return true })
	allowed := &client{identity: SessionIdentity{UserID: 7, AuthVersion: 1, WorkspaceID: 81}, send: make(chan []byte, 1)}
	wrongRoom := &client{identity: SessionIdentity{UserID: 7, AuthVersion: 1, WorkspaceID: 82}, send: make(chan []byte, 1)}
	defaultHub.register(allowed)
	defaultHub.register(wrongRoom)

	deliverClusterEnvelope(clusterEnvelope{Origin: "another-instance", Action: clusterActionUsers, WorkspaceID: 81, UserIDs: []uint64{7}, Payload: []byte(`{"type":"balance"}`)})
	select {
	case <-allowed.send:
	case <-time.After(time.Second):
		t.Fatal("remote event did not reach the intended room connection")
	}
	select {
	case <-wrongRoom.send:
		t.Fatal("remote event crossed a workspace boundary")
	default:
	}

	deliverClusterEnvelope(clusterEnvelope{Origin: cluster.InstanceID(), Action: clusterActionUsers, UserIDs: []uint64{7}, Payload: []byte(`{"type":"loop"}`)})
	select {
	case <-allowed.send:
		t.Fatal("instance consumed its own Redis publication")
	default:
	}
}

func TestClusterDisconnectClosesRemoteInstanceSocket(t *testing.T) {
	previous := defaultHub
	defaultHub = NewHub()
	t.Cleanup(func() { defaultHub = previous })
	defaultHub.setSessionValidator(func(SessionIdentity) bool { return true })
	revoked := &client{identity: SessionIdentity{UserID: 19, AuthVersion: 2, WorkspaceID: 81}, send: make(chan []byte, 1)}
	newLogin := &client{identity: SessionIdentity{UserID: 19, AuthVersion: 3, WorkspaceID: 81}, send: make(chan []byte, 1)}
	defaultHub.register(revoked)
	defaultHub.register(newLogin)
	envelope := clusterEnvelope{
		Origin: "another-instance", EventID: 71, Action: clusterActionDisconnect,
		UserIDs: []uint64{19}, RevokedAuthVersion: 2,
	}
	deliverClusterEnvelope(envelope)
	select {
	case <-revoked.done:
	case <-time.After(time.Second):
		t.Fatal("remote revoke did not close the local socket")
	}
	select {
	case <-newLogin.done:
		t.Fatal("delayed revoke closed a newer authenticated generation")
	default:
	}
	// Disconnect is deliberately idempotent because Redis publication retries
	// may deliver the same revoke more than once.
	deliverClusterEnvelope(envelope)
}

func TestClusterDisconnectRejectsIncompleteDurableEnvelope(t *testing.T) {
	previous := defaultHub
	defaultHub = NewHub()
	t.Cleanup(func() { defaultHub = previous })
	connection := &client{identity: SessionIdentity{UserID: 19, AuthVersion: 2}, send: make(chan []byte, 1)}
	defaultHub.register(connection)

	deliverClusterEnvelope(clusterEnvelope{Origin: "another-instance", Action: clusterActionDisconnect, UserIDs: []uint64{19}})
	select {
	case <-connection.done:
		t.Fatal("an unversioned best-effort message closed a session")
	default:
	}
}
