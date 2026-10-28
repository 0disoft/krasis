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

// Model the transport's separate write gate. A hint may enter the semantic
// coordinator while an earlier wire operation still owns that gate.
type queuedHintWriter struct {
	calls chan struct{}
	err   error
}

func (w *queuedHintWriter) WriteFrameContext(ctx context.Context, _ transport.Frame) error {
	w.calls <- struct{}{}
	<-ctx.Done()
	return w.err
}

func TestPreparationUnsentWireAdmissionExpiryKeepsSessionUsable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		writer := &queuedHintWriter{calls: make(chan struct{}, 2), err: &transport.FrameWriteFailure{
			Kind: transport.MessageControl, Phase: "admission", Cause: context.DeadlineExceeded,
		}}
		coordinator, _ := peer.NewSessionWriteCoordinator(writer)
		runtime, _ := application.NewSessionRuntime(noopEffects{}, newTestInput(false))
		hints := make(chan time.Time, 1)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- runPreparationSender(ctx, hints, runtime, coordinator) }()
		started := time.Now()
		hints <- started
		<-writer.calls
		synctest.Wait()
		time.Sleep(preparationHintLifetime)
		synctest.Wait()
		select {
		case err := <-done:
			t.Fatalf("zero-byte admission expiry disconnected session: %v", err)
		default:
		}
		// Essential semantic work can still acquire the gate after the hint.
		if err := coordinator.Execute(ctx, func(peer.FrameWriter) error { return nil }); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2*time.Second - time.Since(started))
		hints <- time.Now()
		<-writer.calls // A fresh hint still runs; no retry of the old frame.
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatal("session cancellation was not joined", err)
		}
		if runtime.Snapshot().Control.State != control.StateLocal {
			t.Fatal("unsent hint changed input authority")
		}
	})
}

func TestPreparationWriteReceiptCannotHideUncertainOrInvalidFailure(t *testing.T) {
	for _, failure := range []error{
		context.DeadlineExceeded, // No receipt proves that zero bytes were sent.
		&transport.FrameWriteFailure{Kind: transport.MessageControl, Phase: "io", Cause: context.DeadlineExceeded},
		&transport.FrameWriteFailure{Kind: transport.MessageControl, Phase: "deadline", Cause: context.Canceled},
		&transport.FrameWriteFailure{Kind: transport.MessageInput, Phase: "admission", Cause: context.DeadlineExceeded},
		&transport.FrameWriteFailure{Kind: transport.MessageControl, Phase: "admission", Cause: errors.New("broken transport")},
	} {
		t.Run(failure.Error(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				writer := &queuedHintWriter{calls: make(chan struct{}, 1), err: failure}
				coordinator, _ := peer.NewSessionWriteCoordinator(writer)
				runtime, _ := application.NewSessionRuntime(noopEffects{}, newTestInput(false))
				hints := make(chan time.Time, 1)
				hints <- time.Now()
				done := make(chan error, 1)
				go func() { done <- runPreparationSender(context.Background(), hints, runtime, coordinator) }()
				<-writer.calls
				time.Sleep(preparationHintLifetime)
				if err := <-done; !errors.Is(err, failure) {
					t.Fatal("uncertain write failure was hidden", err)
				}
			})
		})
	}
}
