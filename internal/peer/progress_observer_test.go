package peer

import (
	"context"
	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
	"sync/atomic"
	"testing"
	"time"
)

type stalledProgressInput struct {
	inputPort
	entered, release chan struct{}
}

func (p *stalledProgressInput) Move(context.Context, int32, int32) error {
	close(p.entered)
	<-p.release
	return nil
}

type progressTestObserver struct{ read, dispatch, complete, heartbeat atomic.Uint32 }

func (o *progressTestObserver) ObserveReadStarted(time.Time)   {}
func (o *progressTestObserver) ObserveReadCompleted(time.Time) { o.read.Add(1) }
func (o *progressTestObserver) ObserveDispatchStarted(transport.MessageKind, time.Time) {
	o.dispatch.Add(1)
}
func (o *progressTestObserver) ObserveDispatchCompleted(time.Time)         { o.complete.Add(1) }
func (o *progressTestObserver) ObserveHeartbeatSent(string, time.Time)     {}
func (o *progressTestObserver) ObserveHeartbeatReceived(string, time.Time) { o.heartbeat.Add(1) }

func TestProgressSeparatesReceivedFrameFromStalledInjection(t *testing.T) {
	input := &stalledProgressInput{entered: make(chan struct{}), release: make(chan struct{})}
	runtime, _ := application.NewSessionRuntime(effectExecutor{}, input)
	_, err := runtime.GrantInbound(context.Background(), control.Event{Kind: control.EventInboundLeaseGranted, SessionID: "s", LeaseID: "l"}, "e", time.Unix(100, 0))
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := transport.EncodeInputPayload(control.InputEnvelope{SessionID: "s", Epoch: "e", Sequence: 1}, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1})
	heartbeat, _ := transport.EncodeHeartbeatPayload("s")
	connection := &scriptedConnection{frames: []transport.Frame{{Kind: transport.MessageInput, Payload: payload}, {Kind: transport.MessageHeartbeat, Payload: heartbeat}}}
	observer := &progressTestObserver{}
	done := make(chan error, 1)
	go func() {
		done <- runSession(context.Background(), connection, nil, runtime, nil, nil, fixedClock(time.Unix(101, 0)), 3*time.Second, observer)
	}()
	select {
	case <-input.entered:
	case <-time.After(time.Second):
		close(input.release)
		t.Fatal("input did not start")
	}
	read, dispatch, complete, accepted := observer.read.Load(), observer.dispatch.Load(), observer.complete.Load(), observer.heartbeat.Load()
	close(input.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("session did not finish")
	}
	if read != 1 || dispatch != 1 || complete != 0 || accepted != 0 {
		t.Fatalf("stalled injection mislabeled: %d %d %d %d", read, dispatch, complete, accepted)
	}
	if observer.heartbeat.Load() != 1 || observer.complete.Load() != 2 || runtime.Snapshot().PressedCount != 0 || runtime.Snapshot().Control.State != control.StateLocal {
		t.Fatal("ordered dispatch/cleanup changed")
	}
}
