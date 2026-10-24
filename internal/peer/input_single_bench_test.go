package peer

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

// The source offers the next move only after the preceding injection. This
// exercises the actual session dispatcher without a backlog forming a batch.
type isolatedMoveConnection struct {
	next, closed   chan struct{}
	closeOnce      sync.Once
	session, epoch string
	sequence, end  uint64
	moves          int
}

func (c *isolatedMoveConnection) ReadFrame() (transport.Frame, error) {
	if c.sequence == c.end {
		return transport.Frame{}, io.EOF
	}
	select {
	case <-c.next:
	case <-c.closed:
		return transport.Frame{}, io.EOF
	}
	c.sequence++
	payload, err := transport.EncodeInputPayload(control.InputEnvelope{
		SessionID: c.session, Epoch: c.epoch, Sequence: c.sequence,
	}, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1})
	return transport.Frame{Kind: transport.MessageInput, Payload: payload}, err
}

func (c *isolatedMoveConnection) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (*isolatedMoveConnection) Press(context.Context, control.PressedToken) error   { return nil }
func (*isolatedMoveConnection) Release(context.Context, control.PressedToken) error { return nil }
func (*isolatedMoveConnection) Wheel(context.Context, int32, int32) error           { return nil }
func (c *isolatedMoveConnection) Move(context.Context, int32, int32) error {
	c.moves++
	c.next <- struct{}{}
	return nil
}

func BenchmarkSessionIsolatedPointerMoves(b *testing.B) {
	const count = 256
	ctx := context.Background()
	session, epoch := strings.Repeat("a", 32), strings.Repeat("b", 32)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		connection := &isolatedMoveConnection{next: make(chan struct{}, 1), closed: make(chan struct{}), session: session, epoch: epoch, end: count}
		connection.next <- struct{}{}
		runtime, err := application.NewSessionRuntime(effectExecutor{}, connection)
		if err != nil {
			b.Fatal(err)
		}
		_, err = runtime.GrantInbound(ctx, control.Event{Kind: control.EventInboundLeaseGranted, SessionID: session, LeaseID: "lease-1"}, epoch, time.Now())
		if err != nil {
			b.Fatal(err)
		}
		err = runSession(ctx, connection, nil, runtime, nil, nil, time.Now, 2*time.Second, nil)
		if !errors.Is(err, io.ErrUnexpectedEOF) || connection.moves != count {
			b.Fatalf("session result: moves=%d, error=%v", connection.moves, err)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*count), "ns/move")
}
