package peer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func TestPulseLivenessSkipsInactiveRuntime(t *testing.T) {
	runtime := newLocalRuntime(t)
	writer := &recordingFrameWriter{}
	active, err := PulseLiveness(
		context.Background(), testWriteCoordinator(t, writer), runtime, fixedClock(time.Unix(101, 0)), 3*time.Second,
	)
	if err != nil || active || len(writer.frames) != 0 {
		t.Fatalf("PulseLiveness(LOCAL) = active %t frames %d error %v", active, len(writer.frames), err)
	}
}

func TestPulseLivenessWritesCurrentSessionHeartbeat(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	writer := &recordingFrameWriter{}
	active, err := PulseLiveness(
		context.Background(), testWriteCoordinator(t, writer), runtime, fixedClock(time.Unix(101, 0)), 3*time.Second,
	)
	if err != nil || !active || len(writer.frames) != 1 || writer.frames[0].Kind != transport.MessageHeartbeat {
		t.Fatalf("PulseLiveness(active) = active %t frames %+v error %v", active, writer.frames, err)
	}
	sessionID, err := transport.DecodeHeartbeatPayload(writer.frames[0].Payload)
	if err != nil || sessionID != "session-1" {
		t.Fatalf("heartbeat session = %q error %v", sessionID, err)
	}
}

func TestPulseLivenessExpiresWithoutWritingHeartbeat(t *testing.T) {
	runtime, port := newInboundRuntime(t)
	token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 65}
	if err := runtime.HandlePressedInput(context.Background(), control.InputEnvelope{
		SessionID: "session-1", Epoch: "epoch-1", Sequence: 1,
	}, token, true); err != nil {
		t.Fatalf("HandlePressedInput() error = %v", err)
	}
	writer := &recordingFrameWriter{}
	active, err := PulseLiveness(
		context.Background(), testWriteCoordinator(t, writer), runtime, fixedClock(time.Unix(103, 0)), 3*time.Second,
	)
	if !active || !errors.Is(err, ErrPeerLivenessExpired) || len(writer.frames) != 0 {
		t.Fatalf("PulseLiveness(expired) = active %t frames %+v error %v", active, writer.frames, err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal || snapshot.PressedCount != 0 {
		t.Fatalf("expired runtime = %s", snapshot)
	}
	if len(port.releases) != 1 || port.releases[0] != token {
		t.Fatalf("expired runtime releases = %+v", port.releases)
	}
}

func TestPulseLivenessWriteFailureCleansRuntime(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	cause := errors.New("heartbeat write failed")
	writer := &recordingFrameWriter{err: cause}
	active, err := PulseLiveness(
		context.Background(), testWriteCoordinator(t, writer), runtime, fixedClock(time.Unix(101, 0)), 3*time.Second,
	)
	if !active || !errors.Is(err, cause) {
		t.Fatalf("PulseLiveness(write failure) = active %t error %v", active, err)
	}
	if state := runtime.Snapshot().Control.State; state != control.StateLocal {
		t.Fatalf("write failure runtime = %s", runtime.Snapshot())
	}
}

func TestPulseLivenessCanceledBlockedWriteCleansRuntimeAndReleasesCoordinator(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	writer := &blockingLivenessWriter{started: make(chan struct{})}
	coordinator := testWriteCoordinator(t, writer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := PulseLiveness(ctx, coordinator, runtime, fixedClock(time.Unix(101, 0)), 3*time.Second)
		done <- err
	}()
	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("heartbeat write did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("PulseLiveness(blocked) error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled heartbeat write remained blocked")
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("blocked heartbeat runtime = %s", snapshot)
	}
	active, err := PulseLiveness(context.Background(), coordinator, runtime, fixedClock(time.Unix(102, 0)), 3*time.Second)
	if err != nil || active {
		t.Fatalf("PulseLiveness(after cancel) = active %t error %v", active, err)
	}
}

func TestPulseLivenessReadsDeadlineClockAfterCoordinatorWait(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	writer := &recordingFrameWriter{}
	coordinator := testWriteCoordinator(t, writer)
	holderReady := make(chan struct{})
	releaseHolder := make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- coordinator.Execute(context.Background(), func(FrameWriter) error {
			close(holderReady)
			<-releaseHolder
			return nil
		})
	}()
	<-holderReady

	clockCalled := make(chan struct{})
	clockValue := make(chan time.Time, 1)
	pulseDone := make(chan error, 1)
	go func() {
		_, err := PulseLiveness(context.Background(), coordinator, runtime, func() time.Time {
			close(clockCalled)
			return <-clockValue
		}, 3*time.Second)
		pulseDone <- err
	}()
	select {
	case <-clockCalled:
		t.Fatal("deadline clock was read before the coordinator became available")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseHolder)
	if err := <-holderDone; err != nil {
		t.Fatalf("holder operation error = %v", err)
	}
	select {
	case <-clockCalled:
	case <-time.After(time.Second):
		t.Fatal("deadline clock was not read after coordinator acquisition")
	}
	clockValue <- time.Unix(103, 0)
	if err := <-pulseDone; !errors.Is(err, ErrPeerLivenessExpired) {
		t.Fatalf("PulseLiveness(delayed) error = %v", err)
	}
	if len(writer.frames) != 0 || runtime.Snapshot().Control.State != control.StateLocal {
		t.Fatalf("delayed pulse survived timeout: frames=%+v runtime=%s", writer.frames, runtime.Snapshot())
	}
}

type blockingLivenessWriter struct {
	started chan struct{}
}

func (writer *blockingLivenessWriter) WriteFrameContext(ctx context.Context, _ transport.Frame) error {
	close(writer.started)
	<-ctx.Done()
	return ctx.Err()
}
