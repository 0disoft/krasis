package peer

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func TestRunSessionRoutesInputAndFailsSafeOnEOF(t *testing.T) {
	runtime, port := newInboundRuntime(t)
	payload, err := transport.EncodeInputPayload(
		control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1},
		control.InputAction{Kind: control.InputPointerMove, DeltaX: 3, DeltaY: -2},
	)
	if err != nil {
		t.Fatalf("EncodeInputPayload() error = %v", err)
	}
	connection := &scriptedConnection{frames: []transport.Frame{{Kind: transport.MessageInput, Payload: payload}}}
	err = RunSession(context.Background(), connection, runtime, nil, fixedClock(time.Unix(101, 0)))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("RunSession() error = %v, want unexpected EOF", err)
	}
	if len(port.moves) != 1 || port.moves[0] != [2]int32{3, -2} {
		t.Fatalf("moves = %v", port.moves)
	}
	if !connection.closed {
		t.Fatal("connection was not closed")
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("runtime did not fail safe: %s", snapshot)
	}
}

func TestRunSessionPreservesEOFCauseAndConnectionCloseFailure(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	closeErr := errors.New("connection close failed")
	connection := &scriptedConnection{closeErr: closeErr}
	err := RunSession(context.Background(), connection, runtime, nil, fixedClock(time.Unix(101, 0)))
	if !errors.Is(err, io.ErrUnexpectedEOF) || !errors.Is(err, closeErr) {
		t.Fatalf("RunSession() error = %v, want EOF and close failure", err)
	}
	if connection.closeCalls != 1 {
		t.Fatalf("connection close calls = %d", connection.closeCalls)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("runtime did not fail safe: %s", snapshot)
	}
}

func TestRunSessionPreservesCancellationAndConnectionCloseFailure(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	closeErr := errors.New("connection close failed")
	connection := newCancelBlockingConnection(closeErr)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- RunSession(ctx, connection, runtime, nil, fixedClock(time.Unix(101, 0)))
	}()
	select {
	case <-connection.readStarted:
	case <-time.After(time.Second):
		t.Fatal("session did not start connection read")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || !errors.Is(err, closeErr) {
			t.Fatalf("RunSession() error = %v, want cancellation and close failure", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled session did not finish")
	}
	if connection.closeCalls != 1 {
		t.Fatalf("connection close calls = %d", connection.closeCalls)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("runtime did not fail safe: %s", snapshot)
	}
}

func TestRunSessionRoutesControlGrantThenHeartbeat(t *testing.T) {
	runtime := newOutboundRuntime(t)
	grant := controlFrame(t, transport.ControlCommand{
		Kind:      transport.ControlLeaseGrant,
		SessionID: "session-1",
		LeaseID:   "lease-1",
		Epoch:     "epoch-1",
	})
	heartbeatPayload, err := transport.EncodeHeartbeatPayload("session-1")
	if err != nil {
		t.Fatalf("EncodeHeartbeatPayload() error = %v", err)
	}
	connection := &scriptedConnection{frames: []transport.Frame{
		grant,
		{Kind: transport.MessageHeartbeat, Payload: heartbeatPayload},
	}}
	observedAt := time.Unix(101, 0)
	err = RunSession(context.Background(), connection, runtime, nil, fixedClock(observedAt))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("RunSession() error = %v, want unexpected EOF", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("runtime did not clean up after EOF: %s", snapshot)
	}
}

func TestRunSessionObservedReportsOnlyAcceptedHeartbeat(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	payload, err := transport.EncodeHeartbeatPayload("session-1")
	if err != nil {
		t.Fatalf("EncodeHeartbeatPayload() error = %v", err)
	}
	connection := &scriptedConnection{frames: []transport.Frame{{Kind: transport.MessageHeartbeat, Payload: payload}}}
	observer := &sessionLivenessObserver{}
	observedAt := time.Unix(101, 0)
	err = RunSessionWithLifecycleObserved(
		context.Background(), connection, testWriteCoordinator(t, &recordingFrameWriter{}), runtime,
		nil, nil, fixedClock(observedAt), 3*time.Second, observer,
	)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("RunSessionWithLifecycleObserved() error = %v", err)
	}
	if len(observer.received) != 1 || observer.received[0].sessionID != "session-1" || !observer.received[0].observedAt.Equal(observedAt) {
		t.Fatalf("received observations = %+v", observer.received)
	}
}

func TestRunSessionObservedRejectsHeartbeatAtExpiredDeadline(t *testing.T) {
	runtime, port := newInboundRuntime(t)
	if err := runtime.HandlePressedInput(context.Background(), control.InputEnvelope{
		SessionID: "session-1", Epoch: "epoch-1", Sequence: 1,
	}, control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}, true); err != nil {
		t.Fatalf("HandlePressedInput() error = %v", err)
	}
	payload, err := transport.EncodeHeartbeatPayload("session-1")
	if err != nil {
		t.Fatalf("EncodeHeartbeatPayload() error = %v", err)
	}
	connection := &scriptedConnection{frames: []transport.Frame{{Kind: transport.MessageHeartbeat, Payload: payload}}}
	observer := &sessionLivenessObserver{}
	err = RunSessionWithLifecycleObserved(
		context.Background(), connection, testWriteCoordinator(t, &recordingFrameWriter{}), runtime,
		nil, nil, fixedClock(time.Unix(103, 0)), 3*time.Second, observer,
	)
	if !errors.Is(err, ErrPeerLivenessExpired) {
		t.Fatalf("RunSessionWithLifecycleObserved() error = %v, want peer liveness expired", err)
	}
	if len(observer.received) != 0 {
		t.Fatalf("expired heartbeat observations = %+v", observer.received)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal || snapshot.HasLiveness || snapshot.PressedCount != 0 {
		t.Fatalf("expired heartbeat did not clean runtime: %s", snapshot)
	}
	if len(port.releases) == 0 {
		t.Fatal("expired heartbeat did not release held input")
	}
}

func TestRunSessionObservedReadsHeartbeatClockAfterCoordinatorWait(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	payload, err := transport.EncodeHeartbeatPayload("session-1")
	if err != nil {
		t.Fatalf("EncodeHeartbeatPayload() error = %v", err)
	}
	connection := newObservedFrameConnection(transport.Frame{Kind: transport.MessageHeartbeat, Payload: payload})
	coordinator := testWriteCoordinator(t, &recordingFrameWriter{})
	coordinatorAcquired := make(chan struct{})
	releaseCoordinator := make(chan struct{})
	coordinatorDone := make(chan error, 1)
	go func() {
		coordinatorDone <- coordinator.Execute(context.Background(), func(FrameWriter) error {
			close(coordinatorAcquired)
			<-releaseCoordinator
			return nil
		})
	}()
	select {
	case <-coordinatorAcquired:
	case <-time.After(time.Second):
		t.Fatal("blocking coordinator operation did not acquire coordinator")
	}

	sessionDone := make(chan error, 1)
	go func() {
		sessionDone <- RunSessionWithLifecycleObserved(
			context.Background(), connection, coordinator, runtime, nil, nil,
			func() time.Time {
				select {
				case <-releaseCoordinator:
					return time.Unix(103, 0)
				default:
					return time.Unix(101, 0)
				}
			},
			3*time.Second, nil,
		)
	}()
	select {
	case <-connection.read:
	case <-time.After(time.Second):
		t.Fatal("session did not read heartbeat frame")
	}
	close(releaseCoordinator)
	select {
	case err := <-coordinatorDone:
		if err != nil {
			t.Fatalf("blocking coordinator operation error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocking coordinator operation did not finish")
	}
	select {
	case err := <-sessionDone:
		if !errors.Is(err, ErrPeerLivenessExpired) {
			t.Fatalf("RunSessionWithLifecycleObserved() error = %v, want peer liveness expired", err)
		}
	case <-time.After(time.Second):
		t.Fatal("session did not reject expired heartbeat")
	}
}

func TestRunSessionEOFDisablesOutboundSuppression(t *testing.T) {
	runtime := newOutboundRuntime(t)
	if _, err := runtime.GrantOutbound(context.Background(), "session-1", "lease-1", "epoch-1", time.Unix(100, 0)); err != nil {
		t.Fatalf("GrantOutbound() error = %v", err)
	}
	source := &recordingSourceActivation{active: true}
	controller, err := NewOutboundController(testWriteCoordinator(t, &recordingFrameWriter{}), runtime, source)
	if err != nil {
		t.Fatalf("NewOutboundController() error = %v", err)
	}
	connection := &scriptedConnection{}
	err = RunSessionWithLifecycle(context.Background(), connection, controller.Coordinator, runtime, nil, controller, fixedClock(time.Unix(101, 0)))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("RunSessionWithLifecycle() error = %v", err)
	}
	if source.active {
		t.Fatal("source suppression remained active after EOF")
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("runtime did not fail safe: %s", snapshot)
	}
}

func TestRunSessionClosesConnectionBeforeRuntimeCleanup(t *testing.T) {
	connection := &scriptedConnection{}
	effects := &closeCheckingEffects{connection: connection}
	runtime, err := application.NewSessionRuntime(effects, &inputPort{})
	if err != nil {
		t.Fatalf("NewSessionRuntime() error = %v", err)
	}
	if _, err := runtime.GrantInbound(context.Background(), control.Event{
		Kind: control.EventInboundLeaseGranted, SessionID: "session-1", LeaseID: "lease-1",
	}, "epoch-1", time.Unix(100, 0)); err != nil {
		t.Fatalf("GrantInbound() error = %v", err)
	}
	effects.calls = 0
	effects.closedForEveryCall = true
	err = RunSession(context.Background(), connection, runtime, nil, fixedClock(time.Unix(101, 0)))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("RunSession() error = %v, want unexpected EOF", err)
	}
	if effects.calls == 0 || !effects.closedForEveryCall {
		t.Fatalf("cleanup observed open connection: calls=%d all_closed=%t", effects.calls, effects.closedForEveryCall)
	}
}

func TestRunSessionFailsSafeOnMalformedFramePayload(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	connection := &scriptedConnection{frames: []transport.Frame{{Kind: transport.MessageInput, Payload: []byte{1}}}}
	err := RunSession(context.Background(), connection, runtime, nil, fixedClock(time.Unix(101, 0)))
	if !errors.Is(err, transport.ErrInvalidInputPayload) {
		t.Fatalf("RunSession() error = %v, want ErrInvalidInputPayload", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("runtime did not fail safe: %s", snapshot)
	}
}

func TestRunSessionWrongDirectionInputFailsOutboundRuntime(t *testing.T) {
	runtime := newOutboundRuntime(t)
	if _, err := runtime.GrantOutbound(context.Background(), "session-1", "lease-1", "epoch-1", time.Unix(100, 0)); err != nil {
		t.Fatalf("GrantOutbound() error = %v", err)
	}
	payload, err := transport.EncodeInputPayload(
		control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1},
		control.InputAction{Kind: control.InputPointerMove, DeltaX: 1},
	)
	if err != nil {
		t.Fatalf("EncodeInputPayload() error = %v", err)
	}
	connection := &scriptedConnection{frames: []transport.Frame{{Kind: transport.MessageInput, Payload: payload}}}
	err = RunSession(context.Background(), connection, runtime, nil, fixedClock(time.Unix(101, 0)))
	if !errors.Is(err, application.ErrRuntimeNotInbound) {
		t.Fatalf("RunSession() error = %v, want wrong-direction error", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("runtime did not fail safe: %s", snapshot)
	}
}

func TestLifecycleSessionCoordinatesInboundFaultTransitions(t *testing.T) {
	tests := []struct {
		name                string
		frame               transport.Frame
		wantConnectionClose bool
	}{
		{
			name:  "input dispatch waits for coordinator",
			frame: transport.Frame{Kind: transport.MessageInput, Payload: []byte{1}},
		},
		{
			name:                "control decode closes before coordinated cleanup",
			frame:               transport.Frame{Kind: transport.MessageControl, Payload: []byte{1}},
			wantConnectionClose: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime, _ := newInboundRuntime(t)
			connection := newObservedFrameConnection(test.frame)
			coordinator := testWriteCoordinator(t, &recordingFrameWriter{})
			held := make(chan struct{})
			release := make(chan struct{})
			holderDone := make(chan error, 1)
			go func() {
				holderDone <- coordinator.Execute(context.Background(), func(FrameWriter) error {
					close(held)
					<-release
					return nil
				})
			}()
			<-held

			done := make(chan error, 1)
			go func() {
				done <- RunSessionWithLifecycle(
					context.Background(), connection, coordinator, runtime, nil, nil, fixedClock(time.Unix(101, 0)),
				)
			}()
			<-connection.read
			if test.wantConnectionClose {
				select {
				case <-connection.closed:
				case <-time.After(time.Second):
					t.Fatal("control decode failure did not close connection before cleanup")
				}
			}
			if state := runtime.Snapshot().Control.State; state != control.StateInboundActive {
				t.Fatalf("runtime changed outside coordinator: %s", state)
			}
			select {
			case err := <-done:
				t.Fatalf("session finished while coordinator was held: %v", err)
			default:
			}

			close(release)
			if err := <-holderDone; err != nil {
				t.Fatalf("held Execute() error = %v", err)
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("malformed frame session error = nil")
				}
			case <-time.After(time.Second):
				t.Fatal("malformed frame session did not finish")
			}
			if state := runtime.Snapshot().Control.State; state != control.StateLocal {
				t.Fatalf("runtime did not finish local: %s", runtime.Snapshot())
			}
		})
	}
}

type scriptedConnection struct {
	frames     []transport.Frame
	index      int
	closed     bool
	closeCalls int
	closeErr   error
}

type observedFrameConnection struct {
	frame     transport.Frame
	delivered bool
	read      chan struct{}
	closed    chan struct{}
	closeOnce sync.Once
}

type cancelBlockingConnection struct {
	readStarted chan struct{}
	closed      chan struct{}
	closeOnce   sync.Once
	closeCalls  int
	closeErr    error
}

func newCancelBlockingConnection(closeErr error) *cancelBlockingConnection {
	return &cancelBlockingConnection{
		readStarted: make(chan struct{}),
		closed:      make(chan struct{}),
		closeErr:    closeErr,
	}
}

func (connection *cancelBlockingConnection) ReadFrame() (transport.Frame, error) {
	close(connection.readStarted)
	<-connection.closed
	return transport.Frame{}, errors.New("connection closed")
}

func (connection *cancelBlockingConnection) Close() error {
	connection.closeOnce.Do(func() {
		connection.closeCalls++
		close(connection.closed)
	})
	return connection.closeErr
}

func newObservedFrameConnection(frame transport.Frame) *observedFrameConnection {
	return &observedFrameConnection{frame: frame, read: make(chan struct{}), closed: make(chan struct{})}
}

func (connection *observedFrameConnection) ReadFrame() (transport.Frame, error) {
	if connection.delivered {
		return transport.Frame{}, io.EOF
	}
	connection.delivered = true
	close(connection.read)
	return connection.frame, nil
}

func (connection *observedFrameConnection) Close() error {
	connection.closeOnce.Do(func() { close(connection.closed) })
	return nil
}

type closeCheckingEffects struct {
	connection         *scriptedConnection
	calls              int
	closedForEveryCall bool
}

func (effects *closeCheckingEffects) Execute(context.Context, control.Effect) error {
	if effects.calls == 0 {
		effects.closedForEveryCall = true
	}
	effects.calls++
	effects.closedForEveryCall = effects.closedForEveryCall && effects.connection.closed
	return nil
}

func (connection *scriptedConnection) ReadFrame() (transport.Frame, error) {
	if connection.index >= len(connection.frames) {
		return transport.Frame{}, io.EOF
	}
	frame := connection.frames[connection.index]
	connection.index++
	return frame, nil
}

func (connection *scriptedConnection) Close() error {
	connection.closed = true
	connection.closeCalls++
	return connection.closeErr
}

func fixedClock(observedAt time.Time) ObservationClock {
	return func() time.Time { return observedAt }
}

type sessionLivenessObservation struct {
	sessionID  string
	observedAt time.Time
}

type sessionLivenessObserver struct {
	sent     []sessionLivenessObservation
	received []sessionLivenessObservation
}

func (observer *sessionLivenessObserver) ObserveHeartbeatSent(sessionID string, observedAt time.Time) {
	observer.sent = append(observer.sent, sessionLivenessObservation{sessionID: sessionID, observedAt: observedAt})
}

func (observer *sessionLivenessObserver) ObserveHeartbeatReceived(sessionID string, observedAt time.Time) {
	observer.received = append(observer.received, sessionLivenessObservation{sessionID: sessionID, observedAt: observedAt})
}
