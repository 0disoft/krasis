package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
)

func TestLivenessPolicyRejectsMissingOrInvertedBudget(t *testing.T) {
	for _, policy := range []LivenessPolicy{
		{},
		{HeartbeatInterval: time.Second, PeerTimeout: time.Second},
		{HeartbeatInterval: 2 * time.Second, PeerTimeout: time.Second},
	} {
		if err := policy.Validate(); err == nil {
			t.Fatalf("Validate(%+v) error = nil", policy)
		}
	}
	if err := (LivenessPolicy{HeartbeatInterval: time.Second, PeerTimeout: 3 * time.Second}).Validate(); err != nil {
		t.Fatalf("Validate(valid) error = %v", err)
	}
}

func TestRunLivenessTicksExpiresSilentActivePeer(t *testing.T) {
	runtime := newLivenessRuntime(t)
	writer := &livenessWriter{}
	coordinator, err := peer.NewSessionWriteCoordinator(writer)
	if err != nil {
		t.Fatalf("NewSessionWriteCoordinator() error = %v", err)
	}
	policy := LivenessPolicy{HeartbeatInterval: time.Second, PeerTimeout: 3 * time.Second}
	observer := &recordingLivenessObserver{}
	ticks := make(chan time.Time, 2)
	ticks <- time.Unix(1, 0)
	ticks <- time.Unix(2, 0)
	times := make(chan time.Time, 2)
	times <- time.Unix(101, 0)
	times <- time.Unix(103, 0)
	err = runLivenessTicks(context.Background(), coordinator, runtime, policy, func() time.Time { return <-times }, observer, ticks)
	if !errors.Is(err, peer.ErrPeerLivenessExpired) {
		t.Fatalf("runLivenessTicks() error = %v", err)
	}
	if len(writer.frames) != 1 || writer.frames[0].Kind != transport.MessageHeartbeat {
		t.Fatalf("heartbeat frames = %+v", writer.frames)
	}
	if len(observer.sent) != 1 || observer.sent[0].sessionID != "session-1" || !observer.sent[0].observedAt.Equal(time.Unix(101, 0)) {
		t.Fatalf("sent observations = %+v", observer.sent)
	}
	if state := runtime.Snapshot().Control.State; state != control.StateLocal {
		t.Fatalf("silent peer runtime = %s", runtime.Snapshot())
	}
}

func TestRunLivenessTicksStopsOnCancellation(t *testing.T) {
	runtime, err := application.NewSessionRuntime(noopEffects{}, newTestInput(false))
	if err != nil {
		t.Fatalf("NewSessionRuntime() error = %v", err)
	}
	coordinator, err := peer.NewSessionWriteCoordinator(&livenessWriter{})
	if err != nil {
		t.Fatalf("NewSessionWriteCoordinator() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = runLivenessTicks(
		ctx,
		coordinator,
		runtime,
		LivenessPolicy{HeartbeatInterval: time.Second, PeerTimeout: 3 * time.Second},
		time.Now,
		nil,
		make(chan time.Time),
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runLivenessTicks(canceled) error = %v", err)
	}
}

func newLivenessRuntime(t *testing.T) *application.SessionRuntime {
	t.Helper()
	runtime, err := application.NewSessionRuntime(noopEffects{}, newTestInput(false))
	if err != nil {
		t.Fatalf("NewSessionRuntime() error = %v", err)
	}
	if _, err := runtime.GrantInbound(context.Background(), control.Event{
		Kind: control.EventInboundLeaseGranted, SessionID: "session-1", LeaseID: "lease-1",
	}, "epoch-1", time.Unix(100, 0)); err != nil {
		t.Fatalf("GrantInbound() error = %v", err)
	}
	return runtime
}

type livenessWriter struct {
	frames []transport.Frame
}

type livenessObservation struct {
	sessionID  string
	observedAt time.Time
}

type recordingLivenessObserver struct {
	sent     []livenessObservation
	received []livenessObservation
}

func (observer *recordingLivenessObserver) ObserveHeartbeatSent(sessionID string, observedAt time.Time) {
	observer.sent = append(observer.sent, livenessObservation{sessionID: sessionID, observedAt: observedAt})
}

func (observer *recordingLivenessObserver) ObserveHeartbeatReceived(sessionID string, observedAt time.Time) {
	observer.received = append(observer.received, livenessObservation{sessionID: sessionID, observedAt: observedAt})
}

func (writer *livenessWriter) WriteFrameContext(_ context.Context, frame transport.Frame) error {
	writer.frames = append(writer.frames, frame)
	return nil
}
