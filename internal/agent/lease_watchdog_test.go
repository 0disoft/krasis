package agent

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"testing"
	"testing/synctest"
	"time"
)

type watchdogInput struct {
	*testInput
	pressed int
}

func (input *watchdogInput) Press(context.Context, control.PressedToken) error {
	input.pressed++
	return nil
}
func (input *watchdogInput) Release(context.Context, control.PressedToken) error {
	input.pressed--
	return nil
}

func TestWatchdogInterruptsBlockedGateWithoutExternalCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		input := &watchdogInput{testInput: newTestInput(false)}
		runtime, _ := application.NewSessionRuntime(noopEffects{}, input)
		_, err := runtime.GrantInbound(context.Background(), control.Event{Kind: control.EventInboundLeaseGranted, SessionID: "s", LeaseID: "l"}, "e", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err := runtime.HandleInputAction(context.Background(), control.InputEnvelope{SessionID: "s", Epoch: "e", Sequence: 1}, control.InputAction{Kind: control.InputButtonDown, Code: 1}); err != nil {
			t.Fatal(err)
		}
		coordinator, _ := peer.NewSessionWriteCoordinator(&livenessWriter{})
		closed := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			coordinator.Execute(context.Background(), func(peer.FrameWriter) error { <-closed; return nil })
			coordinator.Execute(context.Background(), func(peer.FrameWriter) error { return runtime.Fail(context.Background(), peer.ErrPeerLivenessExpired) })
		}()
		synctest.Wait()
		err = runLeaseWatchdog(context.Background(), runtime.LeaseWatch, LivenessPolicy{HeartbeatInterval: time.Second, PeerTimeout: 2 * time.Second}, func(error) { close(closed) })
		<-done
		if !errors.Is(err, peer.ErrPeerLivenessExpired) || input.pressed != 0 || runtime.Snapshot().PressedCount != 0 {
			t.Fatalf("watchdog=%v pressed=%d", err, input.pressed)
		}
	})
}

func TestWatchdogBoundsPendingHandshake(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		watch := application.LeaseWatch{State: control.StatePending, Progress: time.Now()}
		stopped := false
		err := runLeaseWatchdog(context.Background(), func() application.LeaseWatch { return watch }, LivenessPolicy{HeartbeatInterval: time.Second, PeerTimeout: 2 * time.Second}, func(error) { stopped = true })
		if !stopped || !errors.Is(err, ErrLeaseHandshakeExpired) || time.Since(watch.Progress) != 5*time.Second {
			t.Fatalf("pending watchdog=%v", err)
		}
	})
}

func TestWatchdogUsesExecutionTimeAfterDelayedTick(t *testing.T) {
	start := time.Unix(100, 0)
	ticks := make(chan time.Time, 1)
	ticks <- start.Add(250 * time.Millisecond)
	watch := application.LeaseWatch{State: control.StateInboundActive, Progress: start}
	stopped := false
	err := runLeaseWatchdogTicks(context.Background(), func() application.LeaseWatch { return watch },
		LivenessPolicy{HeartbeatInterval: 250 * time.Millisecond, PeerTimeout: 2 * time.Second},
		func(error) { stopped = true }, ticks, func() time.Time { return start.Add(3 * time.Second) })
	if !stopped || !errors.Is(err, peer.ErrPeerLivenessExpired) {
		t.Fatalf("delayed worker retained expired lease: %v", err)
	}
}
