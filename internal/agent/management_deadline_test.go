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
)

type managementPendingInput struct {
	managementTestInput
	entered, resume chan struct{}
}

func (input *managementPendingInput) Press(context.Context, control.PressedToken) error {
	close(input.entered)
	<-input.resume
	return nil
}

func TestManagementDeadlineDoesNotWaitForNativeInputOrInterruptSession(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		input := &managementPendingInput{entered: make(chan struct{}), resume: make(chan struct{})}
		runtime, err := application.NewSessionRuntime(managementTestEffects{}, input)
		if err != nil {
			t.Fatal(err)
		}
		request := managementTestRequest("request-input-deadline", LeaseRevocationOnly)
		if _, err := runtime.GrantInbound(context.Background(), control.Event{Kind: control.EventInboundLeaseGranted, SessionID: request.Lease.SessionID, LeaseID: request.Lease.LeaseID}, request.Lease.Epoch, time.Now()); err != nil {
			t.Fatal(err)
		}
		connection := &managementTestConnection{}
		coordinator, err := peer.NewSessionWriteCoordinator(connection)
		if err != nil {
			t.Fatal(err)
		}
		source := &managementTestSource{active: true}
		managed, err := newManagedPeerSession(managementTestPeer(), runtime, coordinator, source, connection, nil, time.Now, newTestInput(false))
		if err != nil {
			t.Fatal(err)
		}
		inputDone := make(chan error, 1)
		go func() {
			inputDone <- runtime.HandleInputAction(context.Background(), control.InputEnvelope{SessionID: request.Lease.SessionID, Epoch: request.Lease.Epoch, Sequence: 1}, control.InputAction{Kind: control.InputKeyDown, Code: 30})
		}()
		<-input.entered
		go func() { time.Sleep(500 * time.Millisecond); close(input.resume) }()
		defer func() {
			if err := <-inputDone; err != nil {
				t.Error("native input fixture failed", err)
			}
			_ = runtime.Fail(context.Background(), context.Canceled)
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()
		started := time.Now()
		err = managed.Revoke(ctx, request)
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 25*time.Millisecond {
			t.Errorf("management request exceeded its admission deadline: err=%v elapsed=%v", err, time.Since(started))
		}
		frames, interrupts := connection.Snapshot()
		if !source.Active() || source.Deactivations() != 0 || len(frames) != 0 || len(interrupts) != 0 {
			t.Error("expired admission changed source, peer frames, or connection", source.Deactivations(), len(frames), len(interrupts))
		}
		pulse, err := peer.PulseLiveness(context.Background(), coordinator, runtime, time.Now, 2*time.Second)
		if err != nil || !pulse.Sent {
			t.Error("expired management request blocked heartbeat or retired lease", pulse, err)
		}
	})
}
