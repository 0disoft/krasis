package peer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func TestPulseLivenessSkipsInactiveRuntime(t *testing.T) {
	runtime := newLocalRuntime(t)
	writer := &recordingFrameWriter{}
	pulse, err := PulseLiveness(
		context.Background(), testWriteCoordinator(t, writer), runtime, fixedClock(time.Unix(101, 0)), 3*time.Second,
	)
	if err != nil || pulse.Active || pulse.Sent || len(writer.frames) != 0 {
		t.Fatalf("PulseLiveness(LOCAL) = pulse %+v frames %d error %v", pulse, len(writer.frames), err)
	}
}

type uncooperativeHeartbeatInput struct {
	inputPort
	entered, resume chan struct{}
}

func (p *uncooperativeHeartbeatInput) Press(context.Context, control.PressedToken) error {
	close(p.entered)
	<-p.resume
	return errors.New("native outcome unknown")
}

func TestPulseLivenessDoesNotWaitForUncooperativeInjection(t *testing.T) {
	port := &uncooperativeHeartbeatInput{entered: make(chan struct{}), resume: make(chan struct{})}
	runtime, err := application.NewSessionRuntime(effectExecutor{}, port)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.GrantInbound(context.Background(), control.Event{Kind: control.EventInboundLeaseGranted, SessionID: "s", LeaseID: "l"}, "e", time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	inputDone := make(chan error, 1)
	go func() {
		inputDone <- runtime.HandleInputAction(context.Background(), control.InputEnvelope{SessionID: "s", Epoch: "e", Sequence: 1}, control.InputAction{Kind: control.InputKeyDown, Code: 30})
	}()
	defer func() {
		close(port.resume)
		select {
		case <-inputDone:
		case <-time.After(3 * time.Second):
			t.Error("native test owner did not retire")
		}
	}()
	select {
	case <-port.entered:
	case <-time.After(time.Second):
		t.Fatal("native operation did not enter")
	}
	writer := &recordingFrameWriter{}
	coordinator := testWriteCoordinator(t, writer)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := PulseLiveness(ctx, coordinator, runtime, fixedClock(time.Unix(101, 0)), 3*time.Second)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil || len(writer.frames) != 1 || writer.frames[0].Kind != transport.MessageHeartbeat {
			t.Fatal("native input blocked the independent heartbeat", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pulse retained coordinator behind native input")
	}
	// Heartbeats retain semantic write ordering while the native owner stays
	// joined; sending does not advance receive-side liveness or acknowledge input.
	if err := coordinator.Execute(context.Background(), func(FrameWriter) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestPulseLivenessWritesCurrentSessionHeartbeat(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	writer := &recordingFrameWriter{}
	pulse, err := PulseLiveness(
		context.Background(), testWriteCoordinator(t, writer), runtime, fixedClock(time.Unix(101, 0)), 3*time.Second,
	)
	if err != nil || !pulse.Active || !pulse.Sent || pulse.SessionID != "session-1" || !pulse.ObservedAt.Equal(time.Unix(101, 0)) || len(writer.frames) != 1 || writer.frames[0].Kind != transport.MessageHeartbeat {
		t.Fatalf("PulseLiveness(active) = pulse %+v frames %+v error %v", pulse, writer.frames, err)
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
	pulse, err := PulseLiveness(
		context.Background(), testWriteCoordinator(t, writer), runtime, fixedClock(time.Unix(103, 0)), 3*time.Second,
	)
	if !pulse.Active || pulse.Sent || !errors.Is(err, ErrPeerLivenessExpired) || len(writer.frames) != 0 {
		t.Fatalf("PulseLiveness(expired) = pulse %+v frames %+v error %v", pulse, writer.frames, err)
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
	pulse, err := PulseLiveness(
		context.Background(), testWriteCoordinator(t, writer), runtime, fixedClock(time.Unix(101, 0)), 3*time.Second,
	)
	if !pulse.Active || pulse.Sent || !errors.Is(err, cause) {
		t.Fatalf("PulseLiveness(write failure) = pulse %+v error %v", pulse, err)
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
	pulse, err := PulseLiveness(context.Background(), coordinator, runtime, fixedClock(time.Unix(102, 0)), 3*time.Second)
	if err != nil || pulse.Active || pulse.Sent {
		t.Fatalf("PulseLiveness(after cancel) = pulse %+v error %v", pulse, err)
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
	var clockOnce sync.Once
	var observedClock time.Time
	pulseDone := make(chan error, 1)
	go func() {
		_, err := PulseLiveness(context.Background(), coordinator, runtime, func() time.Time {
			clockOnce.Do(func() { close(clockCalled); observedClock = <-clockValue })
			return observedClock
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

func TestPulseLivenessTimeoutCleanupSurvivesConcurrentSessionCancellation(t *testing.T) {
	input := &cancelAwareInputPort{}
	runtime, err := application.NewSessionRuntime(effectExecutor{}, input)
	if err != nil {
		t.Fatalf("NewSessionRuntime() error = %v", err)
	}
	if _, err := runtime.GrantInbound(context.Background(), control.Event{
		Kind: control.EventInboundLeaseGranted, SessionID: "session-1", LeaseID: "lease-1",
	}, "epoch-1", time.Unix(100, 0)); err != nil {
		t.Fatalf("GrantInbound() error = %v", err)
	}
	token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 65}
	if err := runtime.HandlePressedInput(context.Background(), control.InputEnvelope{
		SessionID: "session-1", Epoch: "epoch-1", Sequence: 1,
	}, token, true); err != nil {
		t.Fatalf("HandlePressedInput() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	pulse, err := PulseLiveness(ctx, testWriteCoordinator(t, &recordingFrameWriter{}), runtime, func() time.Time {
		cancel()
		return time.Unix(103, 0)
	}, 3*time.Second)
	if !pulse.Active || pulse.Sent || !errors.Is(err, ErrPeerLivenessExpired) {
		t.Fatalf("PulseLiveness(canceled timeout) = pulse %+v error %v", pulse, err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal || snapshot.PressedCount != 0 {
		t.Fatalf("canceled timeout runtime = %s", snapshot)
	}
	if len(input.releases) != 1 || input.releases[0] != token {
		t.Fatalf("canceled timeout releases = %+v", input.releases)
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

type cancelAwareInputPort struct {
	releases []control.PressedToken
}

func (*cancelAwareInputPort) Press(context.Context, control.PressedToken) error { return nil }
func (port *cancelAwareInputPort) Release(ctx context.Context, token control.PressedToken) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	port.releases = append(port.releases, token)
	return nil
}
func (*cancelAwareInputPort) Move(context.Context, int32, int32) error  { return nil }
func (*cancelAwareInputPort) Wheel(context.Context, int32, int32) error { return nil }
