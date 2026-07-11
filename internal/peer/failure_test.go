package peer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

func TestFailRuntimeRetriesRetainedPressedInputOnce(t *testing.T) {
	releaseErr := errors.New("release temporarily failed")
	input := &retryReleaseInput{firstReleaseErr: releaseErr}
	runtime, err := application.NewSessionRuntime(effectExecutor{}, input)
	if err != nil {
		t.Fatalf("NewSessionRuntime() error = %v", err)
	}
	if _, err := runtime.GrantInbound(context.Background(), control.Event{
		Kind: control.EventInboundLeaseGranted, SessionID: "session-1", LeaseID: "lease-1",
	}, "epoch-1", time.Unix(100, 0)); err != nil {
		t.Fatalf("GrantInbound() error = %v", err)
	}
	if err := runtime.HandlePressedInput(context.Background(), control.InputEnvelope{
		SessionID: "session-1", Epoch: "epoch-1", Sequence: 1,
	}, control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}, true); err != nil {
		t.Fatalf("HandlePressedInput() error = %v", err)
	}

	cause := errors.New("connection failed")
	err = failRuntime(context.Background(), runtime, cause)
	if !errors.Is(err, cause) || !errors.Is(err, releaseErr) {
		t.Fatalf("failRuntime() error = %v, want cause and first release failure", err)
	}
	if input.releaseCalls != 2 {
		t.Fatalf("release calls = %d, want initial attempt and one recovery retry", input.releaseCalls)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal || snapshot.PressedCount != 0 {
		t.Fatalf("recovery retry did not converge local: %s", snapshot)
	}
}

func TestFailRuntimeStopsAfterOneFailedRecoveryRetry(t *testing.T) {
	releaseErr := errors.New("release remains blocked")
	input := &retryReleaseInput{firstReleaseErr: releaseErr, alwaysFail: true}
	runtime, err := application.NewSessionRuntime(effectExecutor{}, input)
	if err != nil {
		t.Fatalf("NewSessionRuntime() error = %v", err)
	}
	if _, err := runtime.GrantInbound(context.Background(), control.Event{
		Kind: control.EventInboundLeaseGranted, SessionID: "session-1", LeaseID: "lease-1",
	}, "epoch-1", time.Unix(100, 0)); err != nil {
		t.Fatalf("GrantInbound() error = %v", err)
	}
	if err := runtime.HandlePressedInput(context.Background(), control.InputEnvelope{
		SessionID: "session-1", Epoch: "epoch-1", Sequence: 1,
	}, control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}, true); err != nil {
		t.Fatalf("HandlePressedInput() error = %v", err)
	}

	err = failRuntime(context.Background(), runtime, errors.New("connection failed"))
	if !errors.Is(err, releaseErr) {
		t.Fatalf("failRuntime() error = %v, want release failure", err)
	}
	if input.releaseCalls != 2 {
		t.Fatalf("release calls = %d, want exactly two attempts", input.releaseCalls)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateFault || snapshot.PressedCount != 1 {
		t.Fatalf("failed recovery retry lost fault state: %s", snapshot)
	}
}

func TestFailRuntimeCoordinatedPreservesCauseWhenCoordinatorTimesOut(t *testing.T) {
	runtime := newOutboundRuntime(t)
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

	cause := errors.New("connection failed")
	err := failRuntimeCoordinatedWithin(
		context.Background(), coordinator, runtime, cause, nil, 10*time.Millisecond,
	)
	if !errors.Is(err, cause) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("failRuntimeCoordinatedWithin() error = %v, want cause and deadline", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StatePending {
		t.Fatalf("timed-out cleanup changed runtime outside coordinator: %s", snapshot)
	}

	close(release)
	select {
	case err := <-holderDone:
		if err != nil {
			t.Fatalf("held coordinator operation error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("held coordinator operation did not finish")
	}
}

type retryReleaseInput struct {
	releaseCalls    int
	firstReleaseErr error
	alwaysFail      bool
}

func (*retryReleaseInput) Press(context.Context, control.PressedToken) error { return nil }
func (input *retryReleaseInput) Release(context.Context, control.PressedToken) error {
	input.releaseCalls++
	if input.releaseCalls == 1 || input.alwaysFail {
		return input.firstReleaseErr
	}
	return nil
}
func (*retryReleaseInput) Move(context.Context, int32, int32) error  { return nil }
func (*retryReleaseInput) Wheel(context.Context, int32, int32) error { return nil }
