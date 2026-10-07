package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

type cancelingRelease struct {
	cancel context.CancelFunc
	calls  int
	err    error
}

func (p *cancelingRelease) Release(context.Context, control.PressedToken) error {
	p.calls++
	p.cancel()
	return p.err
}

func TestReleaseAllStopsExpiredPlanAndKeepsUnsubmittedTokens(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "first release confirmed", true: "first release uncertain"}[failed], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			port := &cancelingRelease{cancel: cancel}
			if failed {
				port.err = errors.New("release not confirmed")
			}
			var state control.PressedState
			first := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
			second := control.PressedToken{Kind: control.PressedMouseButton, Code: 1}
			pressAll(t, &state, first, second)
			report, err := ReleaseAll(ctx, &state, port)
			if !errors.Is(err, context.Canceled) || port.calls != 1 || !state.Contains(first) || report.Attempted != 2 {
				t.Fatal("expired plan continued or lost ownership", report, err)
			}
			want := 1
			if failed {
				want = 2
				if !errors.Is(err, port.err) {
					t.Fatal("release failure lost")
				}
			}
			if report.Remaining != want || state.Count() != want {
				t.Fatal("unconfirmed release removed", report)
			}
			if recovered, err := ReleaseAll(context.Background(), &state, &recordingReleasePort{}); err != nil || recovered.Remaining != 0 || recovered.Released != want {
				t.Fatal("remaining plan could not be recovered", recovered, err)
			}
		})
	}
}

type slowReleaseInput struct {
	recordingInputPort
	releases       int
	cleanupFailure error
	cleanupErr     error
}

func (p *slowReleaseInput) Release(ctx context.Context, token control.PressedToken) error {
	p.releases++
	if p.releases == 1 {
		<-ctx.Done()
		return ctx.Err()
	}
	p.cleanupErr = ctx.Err()
	if p.cleanupFailure != nil {
		return p.cleanupFailure
	}
	return p.recordingInputPort.Release(ctx, token)
}

func TestSlowKeyUpGetsIndependentRecoveryAndRetainsFailedCleanup(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "recovery succeeds", true: "recovery fails"}[failed], func(t *testing.T) {
			port := &slowReleaseInput{}
			if failed {
				port.cleanupFailure = errors.New("cleanup release uncertain")
			}
			runtime := newTestRuntime(t, port)
			grantInbound(t, runtime, time.Unix(100, 0))
			envelope := control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1}
			if err := runtime.HandleInputAction(context.Background(), envelope, control.InputAction{Kind: control.InputKeyDown, Code: 30}); err != nil {
				t.Fatal(err)
			}
			envelope.Sequence++
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
			defer cancel()
			err := runtime.HandleInputAction(ctx, envelope, control.InputAction{Kind: control.InputKeyUp, Code: 30})
			if !errors.Is(err, context.DeadlineExceeded) || port.cleanupErr != nil || port.releases != 2 {
				t.Fatal("slow release lost fresh recovery context", err)
			}
			state := runtime.Snapshot()
			if failed {
				if !errors.Is(err, port.cleanupFailure) || state.PressedCount != 1 || state.Control.State != control.StateFault {
					t.Fatal("failed cleanup was cleared", state, err)
				}
				port.cleanupFailure = nil
				_ = runtime.Fail(context.Background(), errors.New("retry recovery"))
				state = runtime.Snapshot()
			}
			if state.PressedCount != 0 || state.Control.State != control.StateLocal {
				t.Fatal("pressed key survived confirmed recovery", state)
			}
		})
	}
}
