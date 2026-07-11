package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestSessionRuntimeInboundInputAndFaultCleanup(t *testing.T) {
	port := &recordingInputPort{}
	runtime := newTestRuntime(t, port)
	now := time.Unix(100, 0)
	grantInbound(t, runtime, now)

	token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 65}
	err := runtime.HandlePressedInput(context.Background(), control.InputEnvelope{
		SessionID: "session-1",
		Epoch:     "epoch-1",
		Sequence:  1,
	}, token, true)
	if err != nil {
		t.Fatalf("HandlePressedInput() error = %v", err)
	}
	before := runtime.Snapshot()
	if before.Control.State != control.StateInboundActive || before.PressedCount != 1 || before.InputStream.NextSequence != 2 {
		t.Fatalf("unexpected active snapshot: %s", before)
	}

	_, err = runtime.Dispatch(context.Background(), control.Event{Kind: control.EventFaultDetected}, time.Time{})
	if err != nil {
		t.Fatalf("Dispatch(fault) error = %v", err)
	}
	after := runtime.Snapshot()
	if after.Control.State != control.StateLocal || after.HasStream || after.HasLiveness || after.PressedCount != 0 {
		t.Fatalf("unexpected cleanup snapshot: %s", after)
	}
	if len(port.releaseCalls) != 1 || port.releaseCalls[0] != token {
		t.Fatalf("released = %v, want [%v]", port.releaseCalls, token)
	}
}

func TestSessionRuntimeSequenceGapFailsSafe(t *testing.T) {
	port := &recordingInputPort{}
	runtime := newTestRuntime(t, port)
	grantInbound(t, runtime, time.Unix(100, 0))

	err := runtime.HandlePressedInput(context.Background(), control.InputEnvelope{
		SessionID: "session-1",
		Epoch:     "epoch-1",
		Sequence:  2,
	}, control.PressedToken{Kind: control.PressedMouseButton, Code: 1}, true)
	if !errors.Is(err, control.ErrInputSequenceGap) {
		t.Fatalf("error = %v, want ErrInputSequenceGap", err)
	}
	snapshot := runtime.Snapshot()
	if snapshot.Control.State != control.StateLocal || snapshot.PressedCount != 0 || len(port.pressCalls) != 0 {
		t.Fatalf("unexpected snapshot after gap: %s, presses=%v", snapshot, port.pressCalls)
	}
}

func TestSessionRuntimeMovesPointerAndWheelInSequence(t *testing.T) {
	port := &recordingInputPort{}
	runtime := newTestRuntime(t, port)
	grantInbound(t, runtime, time.Unix(100, 0))

	events := []struct {
		sequence uint64
		action   control.InputAction
	}{
		{sequence: 1, action: control.InputAction{Kind: control.InputPointerMove, DeltaX: -3, DeltaY: 4}},
		{sequence: 2, action: control.InputAction{Kind: control.InputWheel, DeltaY: 120}},
	}
	for _, event := range events {
		err := runtime.HandleInputAction(context.Background(), control.InputEnvelope{
			SessionID: "session-1",
			Epoch:     "epoch-1",
			Sequence:  event.sequence,
		}, event.action)
		if err != nil {
			t.Fatalf("HandleInputAction(%d) error = %v", event.sequence, err)
		}
	}
	if len(port.moves) != 1 || port.moves[0] != [2]int32{-3, 4} {
		t.Fatalf("moves = %v", port.moves)
	}
	if len(port.wheels) != 1 || port.wheels[0] != [2]int32{0, 120} {
		t.Fatalf("wheels = %v", port.wheels)
	}
	if snapshot := runtime.Snapshot(); snapshot.InputStream.NextSequence != 3 || snapshot.PressedCount != 0 {
		t.Fatalf("unexpected snapshot: %s", snapshot)
	}
}

func TestSessionRuntimePointerFailureFailsSafe(t *testing.T) {
	moveErr := errors.New("pointer injection failed")
	port := &recordingInputPort{moveErr: moveErr}
	runtime := newTestRuntime(t, port)
	grantInbound(t, runtime, time.Unix(100, 0))

	err := runtime.HandleInputAction(context.Background(), control.InputEnvelope{
		SessionID: "session-1",
		Epoch:     "epoch-1",
		Sequence:  1,
	}, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1})
	if !errors.Is(err, moveErr) {
		t.Fatalf("error = %v, want pointer injection failure", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal || snapshot.HasStream {
		t.Fatalf("unexpected snapshot after pointer failure: %s", snapshot)
	}
}

func TestSessionRuntimeInvalidReleaseFailsSafe(t *testing.T) {
	port := &recordingInputPort{}
	runtime := newTestRuntime(t, port)
	grantInbound(t, runtime, time.Unix(100, 0))

	err := runtime.HandlePressedInput(context.Background(), control.InputEnvelope{
		SessionID: "session-1",
		Epoch:     "epoch-1",
		Sequence:  1,
	}, control.PressedToken{Kind: "UNKNOWN", Code: 1}, false)
	if !errors.Is(err, control.ErrInvalidPressedToken) {
		t.Fatalf("error = %v, want ErrInvalidPressedToken", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal || snapshot.HasStream {
		t.Fatalf("unexpected snapshot after invalid release: %s", snapshot)
	}
}

func TestSessionRuntimeInjectionFailureReleasesRecordedPress(t *testing.T) {
	injectErr := errors.New("injection failed")
	port := &recordingInputPort{pressErr: injectErr}
	runtime := newTestRuntime(t, port)
	grantInbound(t, runtime, time.Unix(100, 0))
	token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 17}

	err := runtime.HandlePressedInput(context.Background(), control.InputEnvelope{
		SessionID: "session-1",
		Epoch:     "epoch-1",
		Sequence:  1,
	}, token, true)
	if !errors.Is(err, injectErr) {
		t.Fatalf("error = %v, want injection failure", err)
	}
	snapshot := runtime.Snapshot()
	if snapshot.Control.State != control.StateLocal || snapshot.PressedCount != 0 {
		t.Fatalf("unexpected snapshot after injection failure: %s", snapshot)
	}
	if len(port.releaseCalls) != 1 || port.releaseCalls[0] != token {
		t.Fatalf("released = %v, want conservative release of %v", port.releaseCalls, token)
	}
}

func TestSessionRuntimeStaleHeartbeatFailsSafe(t *testing.T) {
	port := &recordingInputPort{}
	runtime := newTestRuntime(t, port)
	grantInbound(t, runtime, time.Unix(100, 0))

	err := runtime.ObserveHeartbeat(context.Background(), "old-session", time.Unix(101, 0))
	if !errors.Is(err, control.ErrStaleHeartbeat) {
		t.Fatalf("error = %v, want ErrStaleHeartbeat", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal || snapshot.HasLiveness {
		t.Fatalf("unexpected snapshot after stale heartbeat: %s", snapshot)
	}
}

func TestSessionRuntimeLiveThenExpired(t *testing.T) {
	port := &recordingInputPort{}
	runtime := newTestRuntime(t, port)
	now := time.Unix(100, 0)
	grantInbound(t, runtime, now)

	faulted, err := runtime.CheckLiveness(context.Background(), now.Add(time.Second), 2*time.Second)
	if err != nil || faulted {
		t.Fatalf("live CheckLiveness() = (%t, %v), want (false, nil)", faulted, err)
	}
	faulted, err = runtime.CheckLiveness(context.Background(), now.Add(2*time.Second), 2*time.Second)
	if err != nil || !faulted {
		t.Fatalf("expired CheckLiveness() = (%t, %v), want (true, nil)", faulted, err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("snapshot after expiry = %s", snapshot)
	}
}

func TestSessionRuntimeRejectsInboundGrantThroughGenericDispatch(t *testing.T) {
	runtime := newTestRuntime(t, &recordingInputPort{})
	_, err := runtime.Dispatch(context.Background(), control.Event{
		Kind:      control.EventInboundLeaseGranted,
		SessionID: "session-1",
		LeaseID:   "lease-1",
	}, time.Unix(100, 0))
	if err == nil {
		t.Fatal("Dispatch(inbound grant) error = nil")
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("snapshot = %s, want LOCAL", snapshot)
	}
}

func TestSessionRuntimeOwnsOutboundEpochAndSequence(t *testing.T) {
	runtime := newTestRuntime(t, &recordingInputPort{})
	if _, err := runtime.BeginOutbound(context.Background(), "session-1", "epoch-1"); err != nil {
		t.Fatalf("BeginOutbound() error = %v", err)
	}
	pending := runtime.Snapshot()
	if pending.Control.State != control.StatePending || !pending.HasOutboundStream || pending.OutboundStream.Epoch != "epoch-1" {
		t.Fatalf("pending snapshot = %s", pending)
	}
	if _, err := runtime.GrantOutbound(context.Background(), "session-1", "lease-1", "epoch-1", time.Unix(100, 0)); err != nil {
		t.Fatalf("GrantOutbound() error = %v", err)
	}
	for want := uint64(1); want <= 2; want++ {
		envelope, err := runtime.NextOutboundEnvelope(context.Background())
		if err != nil {
			t.Fatalf("NextOutboundEnvelope() error = %v", err)
		}
		if envelope.SessionID != "session-1" || envelope.Epoch != "epoch-1" || envelope.Sequence != want {
			t.Fatalf("envelope = %+v", envelope)
		}
	}
}

func TestSessionRuntimeEmptyOutboundLeaseFailsSafe(t *testing.T) {
	runtime := newTestRuntime(t, &recordingInputPort{})
	if _, err := runtime.BeginOutbound(context.Background(), "session-1", "epoch-1"); err != nil {
		t.Fatalf("BeginOutbound() error = %v", err)
	}
	_, err := runtime.GrantOutbound(context.Background(), "session-1", "", "epoch-1", time.Unix(100, 0))
	if err == nil {
		t.Fatal("GrantOutbound(empty lease) error = nil")
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal || snapshot.HasOutboundStream {
		t.Fatalf("runtime did not fail safe: %s", snapshot)
	}
}

func TestSessionRuntimeStaleOutboundGrantFailsSafe(t *testing.T) {
	runtime := newTestRuntime(t, &recordingInputPort{})
	if _, err := runtime.BeginOutbound(context.Background(), "session-1", "epoch-1"); err != nil {
		t.Fatalf("BeginOutbound() error = %v", err)
	}
	_, err := runtime.GrantOutbound(context.Background(), "session-1", "lease-1", "old-epoch", time.Unix(100, 0))
	if err == nil {
		t.Fatal("GrantOutbound(stale epoch) error = nil")
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal || snapshot.HasOutboundStream {
		t.Fatalf("runtime did not fail safe: %s", snapshot)
	}
}

func TestSessionRuntimeRejectsOutboundLifecycleThroughGenericDispatch(t *testing.T) {
	runtime := newTestRuntime(t, &recordingInputPort{})
	_, err := runtime.Dispatch(context.Background(), control.Event{
		Kind:      control.EventOutboundIntentDetected,
		SessionID: "session-1",
	}, time.Time{})
	if err == nil {
		t.Fatal("Dispatch(outbound intent) error = nil")
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("snapshot = %s, want LOCAL", snapshot)
	}
}

func TestSessionRuntimeRejectsReleaseCompletionThroughGenericDispatch(t *testing.T) {
	runtime := newTestRuntime(t, &recordingInputPort{})
	grantInbound(t, runtime, time.Unix(100, 0))

	_, err := runtime.Dispatch(context.Background(), control.Event{
		Kind: control.EventReleaseAllSucceeded,
	}, time.Time{})
	if err == nil {
		t.Fatal("Dispatch(release succeeded) error = nil")
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateInboundActive {
		t.Fatalf("snapshot = %s, want INBOUND_ACTIVE", snapshot)
	}
}

func newTestRuntime(t *testing.T, port InputPort) *SessionRuntime {
	t.Helper()
	runtime, err := NewSessionRuntime(noopEffectExecutor{}, port)
	if err != nil {
		t.Fatalf("NewSessionRuntime() error = %v", err)
	}
	return runtime
}

func grantInbound(t *testing.T, runtime *SessionRuntime, observedAt time.Time) {
	t.Helper()
	_, err := runtime.GrantInbound(context.Background(), control.Event{
		Kind:      control.EventInboundLeaseGranted,
		SessionID: "session-1",
		LeaseID:   "lease-1",
	}, "epoch-1", observedAt)
	if err != nil {
		t.Fatalf("GrantInbound() error = %v", err)
	}
}

type noopEffectExecutor struct{}

func (noopEffectExecutor) Execute(context.Context, control.Effect) error { return nil }
