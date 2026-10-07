package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestRuntimeExpiredAdmissionDoesNotAdvanceStreamOrLoseInputAccounting(t *testing.T) {
	port := &recordingInputPort{}
	observer := &recordingInputDispositionObserver{}
	runtime, err := NewSessionRuntimeObserved(noopEffectExecutor{}, port, observer)
	if err != nil {
		t.Fatal(err)
	}
	grantInbound(t, runtime, time.Unix(100, 0))
	envelope := control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1}
	action := control.InputAction{Kind: control.InputKeyDown, Code: 30}
	runtime.mu.Lock()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runtime.HandleInputAction(ctx, envelope, action) }()
	select {
	case err := <-done:
		runtime.mu.Unlock()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		runtime.mu.Unlock()
		t.Fatal("runtime admission waited for owner")
	}
	if observer.received != 1 || observer.rejected != 1 || observer.injectionStarted != 0 || runtime.Snapshot().PressedCount != 0 {
		t.Fatal("timed-out admission affected input or accounting")
	}
	if err := runtime.HandleInputAction(context.Background(), envelope, action); err != nil {
		t.Fatal("unadmitted sequence was consumed", err)
	}
	if observer.received != 2 || observer.applied != 1 || observer.rejected != 1 {
		t.Fatal("disposition conservation changed")
	}
	if err := runtime.Fail(context.Background(), errors.New("test cleanup")); err == nil {
		t.Fatal("failure cause lost")
	}
}

func TestRuntimeRecoveryAndSnapshotHonorLockDeadline(t *testing.T) {
	runtime := newTestRuntime(t, &recordingInputPort{})
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	cause := errors.New("session interrupted")
	done := make(chan error, 1)
	go func() { done <- runtime.Fail(ctx, cause) }()
	select {
	case err := <-done:
		if !errors.Is(err, cause) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cleanup lock wait exceeded budget")
	}
	if state, err := runtime.SnapshotContext(ctx); !errors.Is(err, context.DeadlineExceeded) || state.Control.State != "" {
		t.Fatal("unavailable runtime reported a guessed state")
	}
}
