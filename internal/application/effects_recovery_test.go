package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

type safetyReleaseFunc func(context.Context, control.PressedToken) error

func (f safetyReleaseFunc) Release(ctx context.Context, token control.PressedToken) error {
	return f(ctx, token)
}

func TestSafetyReleaseOwnsBudgetAcrossParentCancellation(t *testing.T) {
	for _, before := range []bool{true, false} {
		t.Run(map[bool]string{true: "before release", false: "during release"}[before], func(t *testing.T) {
			type profileKey struct{}
			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), profileKey{}, "profile"))
			defer cancel()
			if before {
				cancel()
			}
			var pressed control.PressedState
			key := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
			button := control.PressedToken{Kind: control.PressedMouseButton, Code: 1}
			pressAll(t, &pressed, key, button)
			var calls []control.PressedToken
			release := safetyReleaseFunc(func(cleanup context.Context, token control.PressedToken) error {
				calls = append(calls, token)
				deadline, ok := cleanup.Deadline()
				if cleanup.Err() != nil || !ok || time.Until(deadline) <= 0 || time.Until(deadline) > InputFailureCleanupTimeout || cleanup.Value(profileKey{}) != "profile" {
					t.Error("release lost its independent deadline or diagnostic context")
				}
				cancel()
				return nil
			})
			executor, err := NewSafetyEffectExecutor(&recordingEffectExecutor{}, &pressed, release)
			if err != nil {
				t.Fatal(err)
			}
			if err := executor.Execute(ctx, control.EffectReleaseAll); err != nil || pressed.Count() != 0 || !reflect.DeepEqual(calls, []control.PressedToken{button, key}) {
				t.Fatal("parent cancellation skipped owned key-up recovery", err, calls, pressed.Count())
			}
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatal("recovery changed parent cancellation")
			}
		})
	}
}

func TestSafetyReleaseTimeoutKeepsUnconfirmedAndUnsubmittedTokens(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var pressed control.PressedState
		pressAll(t, &pressed, control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}, control.PressedToken{Kind: control.PressedMouseButton, Code: 1})
		calls := 0
		release := safetyReleaseFunc(func(cleanup context.Context, _ control.PressedToken) error {
			calls++
			<-cleanup.Done()
			return cleanup.Err()
		})
		executor, err := NewSafetyEffectExecutor(&recordingEffectExecutor{}, &pressed, release)
		if err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		err = executor.Execute(ctx, control.EffectReleaseAll)
		if !errors.Is(err, context.DeadlineExceeded) || calls != 1 || pressed.Count() != 2 || time.Since(started) != InputFailureCleanupTimeout {
			t.Fatal("recovery exceeded its budget or discarded unconfirmed input", err, calls, pressed.Count(), time.Since(started))
		}
	})
}

type cancelOnControlStop struct{ cancel context.CancelFunc }

func (d cancelOnControlStop) Execute(_ context.Context, effect control.Effect) error {
	if effect == control.EffectStopRemoteControl {
		d.cancel()
	}
	return nil
}

func TestRevokeReleasesHeldKeyWhenSessionCancelsDuringStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	port := &recordingInputPort{}
	runtime, err := NewSessionRuntime(cancelOnControlStop{cancel}, port)
	if err != nil {
		t.Fatal(err)
	}
	grantInbound(t, runtime, time.Unix(100, 0))
	if err := runtime.HandleInputAction(ctx, control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1}, control.InputAction{Kind: control.InputKeyDown, Code: 30}); err != nil {
		t.Fatal(err)
	}
	_, err = runtime.RevokePeer(ctx, "session-1", "lease-1", "epoch-1")
	state := runtime.Snapshot()
	if err != nil || state.PressedCount != 0 || state.Control.State != control.StateLocal || len(port.releaseCalls) != 1 {
		t.Fatal("session stop skipped a held key-up", err, state, port.releaseCalls)
	}
}
