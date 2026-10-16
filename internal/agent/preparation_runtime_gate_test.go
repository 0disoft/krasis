package agent

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
)

type preparationBlockedPress struct {
	*testInput
	entered, release chan struct{}
}

func (p *preparationBlockedPress) Press(context.Context, control.PressedToken) error {
	close(p.entered)
	<-p.release
	return nil
}

func TestPreparationCannotHoldNetworkGateBehindSlowInjection(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		port := &preparationBlockedPress{testInput: newTestInput(false), entered: make(chan struct{}), release: make(chan struct{})}
		runtime, _ := application.NewSessionRuntime(noopEffects{}, port)
		if _, err := runtime.GrantInbound(context.Background(), control.Event{Kind: control.EventInboundLeaseGranted, SessionID: "s", LeaseID: "l"}, "e", time.Now()); err != nil {
			t.Fatal(err)
		}
		injected := make(chan error, 1)
		go func() {
			injected <- runtime.HandleInputAction(context.Background(), control.InputEnvelope{SessionID: "s", Epoch: "e", Sequence: 1}, control.InputAction{Kind: control.InputButtonDown, Code: 1})
		}()
		<-port.entered
		writer := preparationFrameWriter{calls: make(chan transport.Frame, 1)}
		coordinator, _ := peer.NewSessionWriteCoordinator(writer)
		hints := make(chan time.Time, 1)
		hints <- time.Now()
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- runPreparationSender(ctx, hints, runtime, coordinator) }()
		synctest.Wait()
		gateCtx, stopGate := context.WithTimeout(ctx, 100*time.Millisecond)
		err := coordinator.Execute(gateCtx, func(peer.FrameWriter) error { return nil })
		stopGate()
		cancel()
		close(port.release)
		if injectionErr := <-injected; injectionErr != nil {
			t.Fatal(injectionErr)
		}
		<-done
		if err != nil || len(writer.calls) != 0 {
			t.Fatal("preparation starved the heartbeat gate or wrote during inbound input", err)
		}
	})
}
