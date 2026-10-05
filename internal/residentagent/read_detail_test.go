package residentagent

import (
	"testing"
	"time"
)

type detailDisconnectConnection struct {
	disconnectTestConnection
	detail func(int, time.Time)
}

func (c *detailDisconnectConnection) SetReadDetailObserver(f func(int, time.Time)) bool {
	c.detail = f
	return true
}

func TestDisconnectReadDetailPreservesCallbackAndMissingCapability(t *testing.T) {
	raw := &detailDisconnectConnection{}
	connection := &disconnectConnection{Connection: raw}
	bytes := 0
	if !connection.SetReadDetailObserver(func(n int, _ time.Time) { bytes += n }) || raw.detail == nil {
		t.Fatal("disconnect wrapper hid byte detail")
	}
	raw.detail(12, time.Now())
	if bytes != 12 {
		t.Fatal("disconnect wrapper lost callback")
	}
	connection.Connection = &disconnectTestConnection{}
	if connection.SetReadDetailObserver(nil) {
		t.Fatal("missing underlying observer reported present")
	}
}
