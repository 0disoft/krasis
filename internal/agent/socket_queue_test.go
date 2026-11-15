package agent

import "testing"

type queueTestConnection struct {
	stubConnection
	bytes uint32
	calls int
}

func (c *queueTestConnection) DiagnosticReceiveQueue() (uint32, bool) {
	c.calls++
	return c.bytes, true
}

func TestSupervisorPreservesConnectionLocalReceiveQueue(t *testing.T) {
	for _, bytes := range []uint32{0, 116, 232} {
		raw := &queueTestConnection{bytes: bytes}
		connection := &supervisedConnection{Connection: raw}
		if got, ok := connection.DiagnosticReceiveQueue(); !ok || got != bytes || raw.calls != 1 {
			t.Fatal("wrong socket", got, ok, raw.calls)
		}
	}
	for _, c := range []*supervisedConnection{nil, {}, {Connection: &stubConnection{}}} {
		if n, ok := c.DiagnosticReceiveQueue(); ok || n != 0 {
			t.Fatal("unavailable became zero queue evidence")
		}
	}
}
