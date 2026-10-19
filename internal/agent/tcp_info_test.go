package agent

import (
	"context"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

type tcpTestConnection struct {
	stubConnection
	rtt   uint32
	calls int
}

func (c *tcpTestConnection) DiagnosticTCPInfo() ([4]uint32, bool) {
	c.calls++
	return [4]uint32{c.rtt, 50, 0, 0}, true
}

func TestTCPInfoForwardedThroughSupervisorAndReboundOnReconnect(t *testing.T) {
	var summaries []handoff.Summary
	collector := handoff.New(func(s handoff.Summary) { summaries = append(summaries, s) })
	for _, rtt := range []uint32{100, 200} {
		ctx := handoff.WithSession(context.Background(), collector)
		raw := &tcpTestConnection{rtt: rtt}
		configureTCPInfo(ctx, &supervisedConnection{Connection: raw})
		p := handoff.Begin(ctx, "source", "s", "e", time.Now())
		handoff.Granted(handoff.WithControlRead(p, time.Now()))
		handoff.Finish(p, "ok")
		handoff.EndSession(ctx)
		got := summaries[len(summaries)-1].TCPInfo
		if raw.calls != 2 || got == nil || got.RTTMicros != ([2]uint32{rtt, rtt}) {
			t.Fatal("supervisor hid statistics or reused previous socket", raw.calls, got)
		}
	}
	for _, missing := range []*supervisedConnection{nil, {}, {Connection: &stubConnection{}}} {
		if got, ok := missing.DiagnosticTCPInfo(); ok || got != ([4]uint32{}) {
			t.Fatal("missing capability became a zero observation")
		}
	}
}
