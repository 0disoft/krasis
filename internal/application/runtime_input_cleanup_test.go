package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

type canceledPressInput struct {
	recordingInputPort
	cancel         context.CancelFunc
	cleanupErr     error
	releaseFailure error
}

func (p *canceledPressInput) Press(ctx context.Context, _ control.PressedToken) error {
	p.cancel()
	// Simulate a late native success after session cancellation. The ledger
	// must still be released; success may not restore an expired lease.
	return nil
}

func (p *canceledPressInput) Release(ctx context.Context, token control.PressedToken) error {
	p.cleanupErr = ctx.Err()
	if p.releaseFailure != nil {
		return p.releaseFailure
	}
	if p.cleanupErr != nil {
		return p.cleanupErr
	}
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("unbounded cleanup")
	}
	return p.recordingInputPort.Release(ctx, token)
}

func TestCanceledLatePressUsesFreshCleanupAndRetainsFailedReleases(t *testing.T) {
	for _, failRelease := range []bool{false, true} {
		t.Run(map[bool]string{false: "cleanup succeeds", true: "cleanup fails"}[failRelease], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			port := &canceledPressInput{cancel: cancel}
			if failRelease {
				port.releaseFailure = errors.New("release not confirmed")
			}
			runtime := newTestRuntime(t, port)
			grantInbound(t, runtime, time.Unix(100, 0))
			err := runtime.HandleInputAction(ctx, control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1}, control.InputAction{Kind: control.InputKeyDown, Code: 30})
			if !errors.Is(err, context.Canceled) || port.cleanupErr != nil {
				t.Fatal("cancellation or fresh cleanup lost", err, port.cleanupErr)
			}
			state := runtime.Snapshot()
			if failRelease {
				if !errors.Is(err, port.releaseFailure) || state.PressedCount != 1 || state.Control.State != control.StateFault {
					t.Fatal("unconfirmed release was discarded", err, state)
				}
			} else if state.PressedCount != 0 || state.Control.State != control.StateLocal {
				t.Fatal("late press not released", state)
			}
		})
	}
}
