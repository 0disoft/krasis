package agent

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
)

type budgetHintWriter struct {
	entered  chan struct{}
	deadline time.Time
}

func (w *budgetHintWriter) WriteFrameContext(ctx context.Context, _ transport.Frame) error {
	w.deadline, _ = ctx.Deadline()
	close(w.entered)
	// Match transport's existing independent maximum, even if the caller
	// accidentally supplies the long-lived session context again.
	run, cancel := context.WithTimeout(ctx, transport.FrameWriteTimeout)
	defer cancel()
	<-run.Done()
	return run.Err()
}

func TestPreparationWriteSharesHintExpiryAndReleasesSemanticGate(t *testing.T) {
	for _, age := range []time.Duration{0, 200 * time.Millisecond} {
		t.Run(age.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				writer := &budgetHintWriter{entered: make(chan struct{})}
				coordinator, _ := peer.NewSessionWriteCoordinator(writer)
				runtime, _ := application.NewSessionRuntime(noopEffects{}, newTestInput(false))
				hints := make(chan time.Time, 1)
				started := time.Now()
				at := started.Add(-age)
				hints <- at
				done := make(chan error, 1)
				go func() { done <- runPreparationSender(context.Background(), hints, runtime, coordinator) }()
				<-writer.entered
				// A real request uses the same gate. It cannot enter while this
				// hint owns a possibly partial TLS write, but must not wait 2s.
				if err := coordinator.Execute(context.Background(), func(peer.FrameWriter) error { return nil }); err != nil {
					t.Fatal(err)
				}
				if !errors.Is(<-done, context.DeadlineExceeded) || !writer.deadline.Equal(at.Add(preparationHintLifetime)) || time.Since(started) != preparationHintLifetime-age {
					t.Fatal("stale hint retained the gate or hid an unknown write outcome", time.Since(started), writer.deadline)
				}
				if runtime.Snapshot().Control.State != control.StateLocal {
					t.Fatal("hint granted input authority")
				}
			})
		})
	}
}
