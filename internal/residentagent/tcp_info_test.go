package residentagent

import (
	"context"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

type tcpProductConnection struct {
	disconnectTestConnection
	rtt   uint32
	calls int
}

func (c *tcpProductConnection) DiagnosticTCPInfo() ([4]uint32, bool) {
	c.calls++
	return [4]uint32{c.rtt, 50, 100, 2}, true
}

func TestProductConnectionPreservesTCPInfoWithoutWritingOrClosing(t *testing.T) {
	var summaries []handoff.Summary
	collector := handoff.New(func(s handoff.Summary) { summaries = append(summaries, s) })
	for _, rtt := range []uint32{100, 200} {
		raw := &tcpProductConnection{rtt: rtt}
		source := &disconnectSource{ConnectionSource: correlationSource{connection: raw}, intent: context.Background()}
		wrapped, err := source.Next(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		query, ok := wrapped.(interface{ DiagnosticTCPInfo() ([4]uint32, bool) })
		if !ok {
			t.Fatal("product wrapper hid live-socket statistics")
		}
		ctx := handoff.WithSession(context.Background(), collector)
		handoff.ConfigureTCPInfo(ctx, query.DiagnosticTCPInfo)
		p := handoff.Begin(ctx, "source", "s", "e", time.Now())
		handoff.Granted(handoff.WithControlRead(p, time.Now()))
		handoff.Finish(p, "ok")
		handoff.EndSession(ctx)
		got := summaries[len(summaries)-1].TCPInfo
		if raw.calls != 2 || got == nil || got.RTTMicros != ([2]uint32{rtt, rtt}) || len(raw.frames) != 0 || raw.closed != 0 {
			t.Fatal("statistics changed connection lifecycle or used old socket", raw.calls, got)
		}
	}
	for _, missing := range []*disconnectConnection{nil, {}, {Connection: &disconnectTestConnection{}}} {
		if got, ok := missing.DiagnosticTCPInfo(); ok || got != ([4]uint32{}) {
			t.Fatal("missing capability became zero observations")
		}
	}
}
