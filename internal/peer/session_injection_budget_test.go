package peer

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

type deadlineStalledInput struct {
	inputPort
	entered         chan time.Time
	abort           chan struct{}
	releaseErr      error
	cleanupDeadline time.Time
}

func (p *deadlineStalledInput) Press(ctx context.Context, token control.PressedToken) error {
	p.presses = append(p.presses, token) // The key may have reached the OS.
	deadline, _ := ctx.Deadline()
	p.entered <- deadline
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.abort:
		return errors.New("test aborted stalled input")
	}
}

func (p *deadlineStalledInput) Release(ctx context.Context, token control.PressedToken) error {
	p.releaseErr = ctx.Err()
	p.cleanupDeadline, _ = ctx.Deadline()
	if p.releaseErr != nil {
		return p.releaseErr
	}
	return p.inputPort.Release(ctx, token)
}

func TestStalledInjectionKeepsHeartbeatIndependentAndGetsFreshCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		input := &deadlineStalledInput{entered: make(chan time.Time, 1), abort: make(chan struct{})}
		defer close(input.abort)
		runtime, err := application.NewSessionRuntime(effectExecutor{}, input)
		if err != nil {
			t.Fatal(err)
		}
		_, err = runtime.GrantInbound(context.Background(), control.Event{Kind: control.EventInboundLeaseGranted, SessionID: "s", LeaseID: "l"}, "e", time.Unix(100, 0))
		if err != nil {
			t.Fatal(err)
		}
		payload, _ := transport.EncodeInputPayload(control.InputEnvelope{SessionID: "s", Epoch: "e", Sequence: 1}, control.InputAction{Kind: control.InputKeyDown, Code: 30})
		heartbeat, _ := transport.EncodeHeartbeatPayload("s")
		connection := &scriptedConnection{frames: []transport.Frame{{Kind: transport.MessageInput, Payload: payload}, {Kind: transport.MessageHeartbeat, Payload: heartbeat}}}
		observer := &progressTestObserver{}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			done <- runSession(ctx, connection, nil, runtime, nil, nil, fixedClock(time.Unix(101, 0)), 3*time.Second, observer)
		}()
		select {
		case deadline := <-input.entered:
			if deadline.IsZero() || time.Until(deadline) > application.InputInjectionTimeout {
				t.Fatal("input has no bounded deadline")
			}
		case <-time.After(time.Second):
			t.Fatal("input did not enter")
		}
		synctest.Wait()
		if observer.read.Load() != 1 || observer.heartbeat.Load() != 1 {
			t.Fatal("heartbeat did not bypass the input wait")
		}
		select {
		case err := <-done:
			if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrPeerInputDispatch) {
				t.Fatal(err)
			}
		case <-time.After(application.InputInjectionTimeout + time.Second):
			t.Fatal("stalled injection did not terminate")
		}
		if ctx.Err() != nil {
			t.Fatal("operation timeout canceled the parent")
		}
		if input.releaseErr != nil || input.cleanupDeadline.IsZero() || len(input.releases) != 1 {
			t.Fatal("key-up used expired context or was skipped")
		}
		if observer.read.Load() != 1 || observer.heartbeat.Load() != 1 {
			t.Fatal("heartbeat was lost or counted twice")
		}
		if state := runtime.Snapshot(); state.PressedCount != 0 || state.Control.State != control.StateLocal {
			t.Fatal(state)
		}
		if !connection.closed || connection.closeCalls != 1 {
			t.Fatal("connection not retired once")
		}
	})
}
