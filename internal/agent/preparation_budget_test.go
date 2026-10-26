package agent

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

type budgetPreparationPort struct {
	duration time.Duration
	done     chan error
}

func (p budgetPreparationPort) PrepareInput(ctx context.Context, _ transport.AuthenticatedPeerContext, _ time.Time) {
	select {
	case <-time.After(p.duration):
		p.done <- nil
	case <-ctx.Done():
		p.done <- ctx.Err()
	}
}

func TestIdlePreparationSurvivesSlowConstructionButRemainsBounded(t *testing.T) {
	for _, duration := range []time.Duration{500 * time.Millisecond, time.Second} {
		t.Run(duration.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				runtime, _ := application.NewSessionRuntime(noopEffects{}, newTestInput(false))
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				hints := make(chan time.Time, 1)
				hints <- time.Now()
				port := budgetPreparationPort{duration: duration, done: make(chan error, 1)}
				joined := make(chan error, 1)
				start := time.Now()
				go func() {
					joined <- runPreparationReceiver(ctx, hints, runtime, port, transport.AuthenticatedPeerContext{})
				}()
				err := <-port.done
				wantTimeout := duration > InputPreparationTimeout
				if errors.Is(err, context.DeadlineExceeded) != wantTimeout || time.Since(start) != min(duration, InputPreparationTimeout) {
					t.Fatal("idle construction was prematurely canceled or unbounded", err, time.Since(start))
				}
				cancel()
				if !errors.Is(<-joined, context.Canceled) || runtime.Snapshot().Control.State != control.StateLocal {
					t.Fatal("optional preparation granted input or failed to join shutdown")
				}
			})
		})
	}
}
