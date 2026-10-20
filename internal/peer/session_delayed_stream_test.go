package peer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

// The pipe preserves every byte in order; pausing it does not model TCP loss,
// retransmission, or a peer that has already written bytes into the kernel.
func TestOrderedStreamPauseKeepsHeartbeatBehindIncompleteInput(t *testing.T) {
	wire, inputEnd, inputPayload := orderedInputAndHeartbeat(t)
	for _, test := range []struct {
		name string
		cut  int
	}{
		{name: "delivery stopped before first byte"},
		{name: "delivery stopped inside input payload", cut: inputEnd - len(inputPayload) + 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime, input := newInboundRuntime(t)
			left, right := net.Pipe()
			defer left.Close()
			defer right.Close()
			reader, err := transport.NewReader(left)
			if err != nil {
				t.Fatal(err)
			}
			var readBytes atomic.Int64
			progress := make(chan struct{}, 1)
			reader.SetReadProgressObserver(func(count int, _ time.Time) {
				if readBytes.Add(int64(count)) >= int64(test.cut) {
					select {
					case progress <- struct{}{}:
					default:
					}
				}
			})
			connection := &framedPipeConnection{connection: left, reader: reader}
			observer := newOrderedStallObserver()
			coordinator := testWriteCoordinator(t, &recordingFrameWriter{})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			sessionDone := make(chan error, 1)
			go func() {
				sessionDone <- RunSessionWithLifecycleObserved(ctx, connection,
					coordinator, runtime, nil, nil,
					fixedClock(time.Unix(101, 0)), 3*time.Second, observer)
			}()
			awaitStallSignal(t, observer.readStarted, "session read")

			stageDone := make(chan error, 1)
			sendDone := make(chan error, 1)
			release := make(chan struct{})
			released := false
			t.Cleanup(func() {
				if !released {
					close(release)
				}
				_ = right.Close()
			})
			go func() {
				_, err := io.Copy(right, bytes.NewReader(wire[:test.cut]))
				stageDone <- err
				if err != nil {
					sendDone <- err
					return
				}
				<-release
				_, err = io.Copy(right, bytes.NewReader(wire[test.cut:]))
				_ = right.Close()
				sendDone <- err
			}()
			if err := awaitStallResult(t, stageDone, "first byte segment"); err != nil {
				t.Fatal(err)
			}
			if test.cut > 0 {
				awaitStallSignal(t, progress, "partial frame read progress")
			}
			if got := readBytes.Load(); got != int64(test.cut) {
				t.Fatalf("paused read bytes = %d, want %d", got, test.cut)
			}
			assertOrderedStreamPaused(t, runtime.Snapshot().PressedCount, observer, sessionDone)
			close(release)
			released = true
			if err := awaitStallResult(t, sendDone, "remaining byte segment"); err != nil {
				t.Fatal(err)
			}
			assertOrderedStreamFinished(t, awaitStallResult(t, sessionDone, "session shutdown"),
				runtime.Snapshot().Control.State, input, observer, readBytes.Load(), len(wire))
		})
	}
}

func TestPausedReceiverDoesNotValidateQueuedHeartbeat(t *testing.T) {
	wire, _, _ := orderedInputAndHeartbeat(t)
	runtime, input := newInboundRuntime(t)
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	gate := &firstReadGate{Conn: left, entered: make(chan struct{}), resume: make(chan struct{})}
	reader, err := transport.NewReader(gate)
	if err != nil {
		t.Fatal(err)
	}
	var readBytes atomic.Int64
	reader.SetReadProgressObserver(func(count int, _ time.Time) { readBytes.Add(int64(count)) })
	observer := newOrderedStallObserver()
	coordinator := testWriteCoordinator(t, &recordingFrameWriter{})
	defer gate.open()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sessionDone := make(chan error, 1)
	go func() {
		sessionDone <- RunSessionWithLifecycleObserved(ctx,
			&framedPipeConnection{connection: gate, reader: reader},
			coordinator, runtime, nil, nil,
			fixedClock(time.Unix(101, 0)), 3*time.Second, observer)
	}()
	awaitStallSignal(t, gate.entered, "receiver read gate")
	observedWriter := &observedWriteConn{Conn: right, entered: make(chan struct{})}
	sendDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(observedWriter, bytes.NewReader(wire))
		_ = right.Close()
		sendDone <- err
	}()
	awaitStallSignal(t, observedWriter.entered, "receiver-paused send")
	if readBytes.Load() != 0 {
		t.Fatal("paused receiver reported bytes before reading")
	}
	assertOrderedStreamPaused(t, runtime.Snapshot().PressedCount, observer, sessionDone)
	select {
	case err := <-sendDone:
		t.Fatalf("ordered pipe sent bytes while receiver was paused: %v", err)
	default:
	}
	gate.open()
	if err := awaitStallResult(t, sendDone, "receiver-resumed send"); err != nil {
		t.Fatal(err)
	}
	assertOrderedStreamFinished(t, awaitStallResult(t, sessionDone, "session shutdown"),
		runtime.Snapshot().Control.State, input, observer, readBytes.Load(), len(wire))
}

func TestOrderedStreamCancellationReleasesHeldInputBeforeHeartbeatCanArrive(t *testing.T) {
	wire, inputEnd, _ := orderedInputAndHeartbeat(t)
	runtime, input := newInboundRuntime(t)
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	reader, err := transport.NewReader(left)
	if err != nil {
		t.Fatal(err)
	}
	observer := newOrderedStallObserver()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- RunSessionWithLifecycleObserved(ctx,
			&framedPipeConnection{connection: left, reader: reader},
			testWriteCoordinator(t, &recordingFrameWriter{}), runtime, nil, nil,
			fixedClock(time.Unix(101, 0)), 3*time.Second, observer)
	}()
	awaitStallSignal(t, observer.readStarted, "initial read")
	sent := make(chan error, 1)
	go func() {
		_, err := io.Copy(right, bytes.NewReader(wire[:inputEnd]))
		sent <- err
	}()
	if err := awaitStallResult(t, sent, "input before delivery stall"); err != nil {
		t.Fatal(err)
	}
	// The second read begins only after the key-down has completed. No
	// heartbeat bytes are delivered, so recovery must interrupt that read.
	awaitStallSignal(t, observer.readStarted, "read stalled after held input")
	if snapshot := runtime.Snapshot(); snapshot.PressedCount != 1 {
		t.Fatal("input was not held before the stall", snapshot)
	}
	cancel()
	if err := awaitStallResult(t, done, "stalled session recovery"); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation cause lost during recovery", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal || snapshot.PressedCount != 0 {
		t.Fatal("stalled stream kept input authority", snapshot)
	}
	if len(input.presses) != 1 || len(input.releases) != 1 || input.presses[0] != input.releases[0] ||
		observer.frames.Load() != 1 || observer.heartbeats.Load() != 0 {
		t.Fatal("recovery lost held input or validated an undelivered heartbeat")
	}
}

func orderedInputAndHeartbeat(t *testing.T) ([]byte, int, []byte) {
	t.Helper()
	inputPayload, err := transport.EncodeInputPayload(
		control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1},
		control.InputAction{Kind: control.InputKeyDown, Code: 30},
	)
	if err != nil {
		t.Fatal(err)
	}
	heartbeatPayload, err := transport.EncodeHeartbeatPayload("session-1")
	if err != nil {
		t.Fatal(err)
	}
	var wire bytes.Buffer
	writer, err := transport.NewWriter(&wire)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteFrame(transport.Frame{Kind: transport.MessageInput, Payload: inputPayload}); err != nil {
		t.Fatal(err)
	}
	inputEnd := wire.Len()
	if err := writer.WriteFrame(transport.Frame{Kind: transport.MessageHeartbeat, Payload: heartbeatPayload}); err != nil {
		t.Fatal(err)
	}
	return wire.Bytes(), inputEnd, inputPayload
}

type orderedStallObserver struct {
	readStarted chan struct{}
	frames      atomic.Uint64
	heartbeats  atomic.Uint64
}

func newOrderedStallObserver() *orderedStallObserver {
	return &orderedStallObserver{readStarted: make(chan struct{}, 1)}
}

func (observer *orderedStallObserver) ObserveReadStarted(time.Time) {
	select {
	case observer.readStarted <- struct{}{}:
	default:
	}
}
func (observer *orderedStallObserver) ObserveReadCompleted(time.Time)                 { observer.frames.Add(1) }
func (*orderedStallObserver) ObserveDispatchStarted(transport.MessageKind, time.Time) {}
func (*orderedStallObserver) ObserveDispatchCompleted(time.Time)                      {}
func (*orderedStallObserver) ObserveHeartbeatSent(string, time.Time)                  {}
func (observer *orderedStallObserver) ObserveHeartbeatReceived(string, time.Time) {
	observer.heartbeats.Add(1)
}

type firstReadGate struct {
	net.Conn
	entered  chan struct{}
	resume   chan struct{}
	once     sync.Once
	openOnce sync.Once
}

func (gate *firstReadGate) Read(data []byte) (int, error) {
	gate.once.Do(func() { close(gate.entered); <-gate.resume })
	return gate.Conn.Read(data)
}

func (gate *firstReadGate) open() { gate.openOnce.Do(func() { close(gate.resume) }) }

type observedWriteConn struct {
	net.Conn
	entered chan struct{}
	once    sync.Once
}

func (connection *observedWriteConn) Write(data []byte) (int, error) {
	connection.once.Do(func() { close(connection.entered) })
	return connection.Conn.Write(data)
}

func awaitStallSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func awaitStallResult(t *testing.T, result <-chan error, name string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
		return nil
	}
}

func assertOrderedStreamPaused(t *testing.T, pressed int, observer *orderedStallObserver, sessionDone <-chan error) {
	t.Helper()
	if pressed != 0 || observer.frames.Load() != 0 || observer.heartbeats.Load() != 0 {
		t.Fatalf("paused stream advanced: pressed=%d frames=%d heartbeats=%d", pressed, observer.frames.Load(), observer.heartbeats.Load())
	}
	select {
	case err := <-sessionDone:
		t.Fatalf("session ended before stream resumed: %v", err)
	default:
	}
}

func assertOrderedStreamFinished(t *testing.T, err error, state control.State, input *inputPort, observer *orderedStallObserver, readBytes int64, wireSize int) {
	t.Helper()
	if !errors.Is(err, ErrPeerRemoteEOF) {
		t.Fatalf("session end = %v, want remote EOF", err)
	}
	if observer.frames.Load() != 2 || observer.heartbeats.Load() != 1 || readBytes != int64(wireSize) {
		t.Fatalf("ordered delivery: frames=%d heartbeats=%d bytes=%d want=%d", observer.frames.Load(), observer.heartbeats.Load(), readBytes, wireSize)
	}
	if state != control.StateLocal || len(input.presses) != 1 || len(input.releases) != 1 || input.presses[0] != input.releases[0] {
		t.Fatalf("cleanup: state=%s presses=%v releases=%v", state, input.presses, input.releases)
	}
}
