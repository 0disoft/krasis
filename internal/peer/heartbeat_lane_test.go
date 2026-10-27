package peer

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

type laneHeartbeatObserver struct{ received chan struct{} }

func (*laneHeartbeatObserver) ObserveHeartbeatSent(string, time.Time)       {}
func (o *laneHeartbeatObserver) ObserveHeartbeatReceived(string, time.Time) { o.received <- struct{}{} }

type laneConnection struct {
	frames chan transport.Frame
	closed chan struct{}
	once   sync.Once
}

func (c *laneConnection) ReadFrame() (transport.Frame, error) {
	select {
	case frame := <-c.frames:
		return frame, nil
	case <-c.closed:
		return transport.Frame{}, io.EOF
	}
}
func (c *laneConnection) Close() error { c.once.Do(func() { close(c.closed) }); return nil }

type laneInput struct {
	inputPort
	entered, resume chan struct{}
}

func (p *laneInput) Press(ctx context.Context, token control.PressedToken) error {
	p.presses = append(p.presses, token)
	close(p.entered)
	select {
	case <-p.resume:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestHeartbeatSendAndReceiveContinueDuringJoinedInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		input := &laneInput{entered: make(chan struct{}), resume: make(chan struct{})}
		runtime, _ := application.NewSessionRuntime(effectExecutor{}, input)
		_, err := runtime.GrantInbound(context.Background(), control.Event{Kind: control.EventInboundLeaseGranted, SessionID: "s", LeaseID: "l"}, "e", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		conn := &laneConnection{frames: make(chan transport.Frame, 1), closed: make(chan struct{})}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		writer := &recordingFrameWriter{}
		coordinator := testWriteCoordinator(t, writer)
		observer := &laneHeartbeatObserver{received: make(chan struct{}, 10)}
		done := make(chan error, 1)
		go func() {
			done <- runSession(ctx, conn, coordinator, runtime, nil, nil, time.Now, 2*time.Second, observer)
		}()
		payload, _ := transport.EncodeInputPayload(control.InputEnvelope{SessionID: "s", Epoch: "e", Sequence: 1}, control.InputAction{Kind: control.InputKeyDown, Code: 30})
		conn.frames <- transport.Frame{Kind: transport.MessageInput, Payload: payload}
		<-input.entered
		heartbeat, _ := transport.EncodeHeartbeatPayload("s")
		for range 6 {
			time.Sleep(250 * time.Millisecond)
			conn.frames <- transport.Frame{Kind: transport.MessageHeartbeat, Payload: heartbeat}
			<-observer.received
			pulse, err := PulseLiveness(ctx, coordinator, runtime, time.Now, 2*time.Second)
			if err != nil || !pulse.Sent || !runtime.LeaseWatch().Progress.Equal(time.Now()) {
				t.Fatalf("input blocked heartbeat: pulse=%+v err=%v", pulse, err)
			}
		}
		latest := runtime.LeaseWatch()
		close(input.resume)
		synctest.Wait()
		if state := runtime.Snapshot(); state.PressedCount != 1 || !state.Liveness.LastObserved.Equal(latest.Liveness.LastObserved) || !runtime.LeaseWatch().Progress.Equal(latest.Progress) {
			t.Fatal("old input receipt overwrote newer heartbeat or released a held key", state)
		}
		if len(writer.frames) != 6 {
			t.Fatal("heartbeat write count", len(writer.frames))
		}
		cancel()
		<-done
		if state := runtime.Snapshot(); state.PressedCount != 0 || state.Control.State != control.StateLocal || len(input.releases) != 1 {
			t.Fatal("input cleanup was not joined", state)
		}
	})
}

func TestFullInputQueueCancelsWithBacklogCauseInsteadOfHeartbeatTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		input := &laneInput{entered: make(chan struct{}), resume: make(chan struct{})}
		runtime, _ := application.NewSessionRuntime(effectExecutor{}, input)
		runtime.GrantInbound(context.Background(), control.Event{Kind: control.EventInboundLeaseGranted, SessionID: "s", LeaseID: "l"}, "e", time.Now())
		conn := &laneConnection{frames: make(chan transport.Frame, application.MaxPointerMoveBatch+2), closed: make(chan struct{})}
		done := make(chan error, 1)
		start := time.Now()
		go func() {
			done <- runSession(context.Background(), conn, testWriteCoordinator(t, &recordingFrameWriter{}), runtime, nil, nil, time.Now, 2*time.Second, nil)
		}()
		for seq := uint64(1); seq <= application.MaxPointerMoveBatch+2; seq++ {
			payload, _ := transport.EncodeInputPayload(control.InputEnvelope{SessionID: "s", Epoch: "e", Sequence: seq}, control.InputAction{Kind: control.InputKeyDown, Code: 30})
			conn.frames <- transport.Frame{Kind: transport.MessageInput, Payload: payload}
		}
		err := <-done
		if !errors.Is(err, ErrInputDispatchBacklog) || !errors.Is(err, ErrPeerInputDispatch) || errors.Is(err, ErrPeerLivenessExpired) {
			t.Fatal("backlog misclassified", err)
		}
		if time.Since(start) != inputDispatchQueueTimeout || len(input.presses) != 1 || len(input.releases) != 1 || runtime.Snapshot().PressedCount != 0 {
			t.Fatal("backlog lost its bound or joined recovery", time.Since(start))
		}
	})
}

func TestContinuingHeartbeatsCannotExtendNativeInputDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		input := &laneInput{entered: make(chan struct{}), resume: make(chan struct{})}
		runtime, _ := application.NewSessionRuntime(effectExecutor{}, input)
		runtime.GrantInbound(context.Background(), control.Event{Kind: control.EventInboundLeaseGranted, SessionID: "s", LeaseID: "l"}, "e", time.Now())
		conn := &laneConnection{frames: make(chan transport.Frame, 1), closed: make(chan struct{})}
		observer := &laneHeartbeatObserver{received: make(chan struct{}, 1)}
		done := make(chan error, 1)
		start := time.Now()
		go func() {
			done <- runSession(context.Background(), conn, testWriteCoordinator(t, &recordingFrameWriter{}), runtime, nil, nil, time.Now, 2*time.Second, observer)
		}()
		payload, _ := transport.EncodeInputPayload(control.InputEnvelope{SessionID: "s", Epoch: "e", Sequence: 1}, control.InputAction{Kind: control.InputKeyDown, Code: 30})
		conn.frames <- transport.Frame{Kind: transport.MessageInput, Payload: payload}
		<-input.entered
		heartbeat, _ := transport.EncodeHeartbeatPayload("s")
		for range 8 {
			time.Sleep(200 * time.Millisecond)
			conn.frames <- transport.Frame{Kind: transport.MessageHeartbeat, Payload: heartbeat}
			<-observer.received
		}
		err := <-done
		if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrPeerInputDispatch) || errors.Is(err, ErrPeerLivenessExpired) {
			t.Fatal("input failure disguised as liveness or acknowledged", err)
		}
		if time.Since(start) != application.InputInjectionTimeout || len(input.releases) != 1 || runtime.Snapshot().PressedCount != 0 {
			t.Fatal("heartbeats extended input deadline or detached cleanup")
		}
	})
}
