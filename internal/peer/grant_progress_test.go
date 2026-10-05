package peer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/transport"
)

type grantDetailConnection struct {
	scriptedConnection
	reader *transport.Reader
	delay  time.Duration
}

func (c *grantDetailConnection) ReadFrame() (transport.Frame, error) {
	if c.delay > 0 {
		time.Sleep(c.delay)
		c.delay = 0
	}
	return c.reader.ReadFrame()
}

func TestGrantProgressSeparatesReadDelayFromLocalGrantDispatch(t *testing.T) {
	for _, slowRead := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			var summaries []handoff.Summary
			ctx := handoff.WithSession(context.Background(), handoff.New(func(s handoff.Summary) { summaries = append(summaries, s) }))
			defer handoff.EndSession(ctx)
			handoff.StartGrantProgress(ctx, true, time.Now())
			runtime := newLocalRuntime(t)
			coordinator := testWriteCoordinator(t, &recordingFrameWriter{})
			controller, _ := NewOutboundController(coordinator, runtime, &recordingSourceActivation{})
			if err := controller.BeginBoundary(ctx, control.BoundaryIntent{Edge: control.ScreenEdgeRight}, OutboundIdentity{SessionID: "session", Epoch: "epoch"}); err != nil {
				t.Fatal(err)
			}
			grant := transport.ControlCommand{Kind: transport.ControlLeaseGrant, SessionID: "session", LeaseID: "lease", Epoch: "epoch"}
			revoke := grant
			revoke.Kind = transport.ControlLeaseRevoke
			var wire bytes.Buffer
			writer, _ := transport.NewWriter(&wire)
			for _, command := range []transport.ControlCommand{grant, revoke} {
				if err := writer.WriteFrame(controlFrame(t, command)); err != nil {
					t.Fatal(err)
				}
			}
			reader, _ := transport.NewReader(&wire)
			reader.SetReadDetailObserver(func(n int, at time.Time) { handoff.ReadProgressBytes(ctx, n, at) })
			connection := &grantDetailConnection{reader: reader}
			if slowRead {
				connection.delay = 20 * time.Millisecond
			}
			first := true
			clock := func() time.Time {
				if first && !slowRead {
					time.Sleep(20 * time.Millisecond)
				}
				first = false
				return time.Unix(100, 0)
			}
			err := RunSessionWithLifecycle(ctx, connection, coordinator, runtime, nil, controller, clock)
			if !errors.Is(err, io.ErrUnexpectedEOF) || len(summaries) != 1 || !summaries[0].Valid() {
				t.Fatal("grant observation changed ordered session outcome", err, summaries)
			}
			p := summaries[0].GrantProgress
			if p == nil || !p.Complete || !p.ByteProgressMeasured || p.Bytes == 0 || p.Frames != 1 || p.ElapsedMicros != 20000 {
				t.Fatal("grant wait progress was missing or contaminated by later revoke", p)
			}
			phase := handoff.ReadDispatch
			if slowRead {
				phase = handoff.ReadWaiting
			}
			if p.Micros[phase] != 20000 || p.MaxByteGapMicros != 20000 {
				t.Fatal("local dispatch stall and pre-byte read stall collapsed", p)
			}
		})
	}
}
