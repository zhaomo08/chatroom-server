package ws

import "testing"

type fakeConn struct {
	received [][]byte
}

func (f *fakeConn) WriteMessage(messageType int, data []byte) error {
	f.received = append(f.received, data)
	return nil
}

func TestHubSendToUsers(t *testing.T) {
	h := NewHub()
	c1 := &fakeConn{}
	c2 := &fakeConn{}
	h.Register(1, c1)
	h.Register(2, c2)

	h.SendToUsers([]int64{1}, []byte("hi"))

	if len(c1.received) != 1 {
		t.Fatalf("c1 received %d messages, want 1", len(c1.received))
	}
	if len(c2.received) != 0 {
		t.Fatalf("c2 received %d messages, want 0 (not a recipient)", len(c2.received))
	}
}

func TestHubBroadcastAll(t *testing.T) {
	h := NewHub()
	c1 := &fakeConn{}
	c2 := &fakeConn{}
	h.Register(1, c1)
	h.Register(2, c2)

	h.BroadcastAll([]byte("hi all"))

	if len(c1.received) != 1 || len(c2.received) != 1 {
		t.Fatalf("expected both connections to receive the broadcast, c1=%d c2=%d", len(c1.received), len(c2.received))
	}
}

func TestHubUnregisterStopsDelivery(t *testing.T) {
	h := NewHub()
	c1 := &fakeConn{}
	h.Register(1, c1)
	h.Unregister(1, c1)

	h.SendToUsers([]int64{1}, []byte("hi"))

	if len(c1.received) != 0 {
		t.Errorf("received %d messages after unregister, want 0", len(c1.received))
	}
}
