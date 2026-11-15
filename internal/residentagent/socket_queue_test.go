package residentagent

import "testing"

type queueProductConnection struct {
	disconnectTestConnection
	bytes uint32
}

func (c *queueProductConnection) DiagnosticReceiveQueue() (uint32, bool) { return c.bytes, true }

func TestDisconnectWrapperPreservesLiveReceiveQueue(t *testing.T) {
	for _, bytes := range []uint32{0, 116, 232} {
		raw := &queueProductConnection{bytes: bytes}
		connection := &disconnectConnection{Connection: raw}
		if got, ok := connection.DiagnosticReceiveQueue(); !ok || got != bytes || len(raw.frames) != 0 || raw.closed != 0 {
			t.Fatal("queue probe changed connection", got, ok)
		}
	}
	for _, c := range []*disconnectConnection{nil, {}, {Connection: &disconnectTestConnection{}}} {
		if n, ok := c.DiagnosticReceiveQueue(); ok || n != 0 {
			t.Fatal("unavailable became zero queue evidence")
		}
	}
}
