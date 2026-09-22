package peer

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

type preparationSource struct {
	recordingSourceActivation
	prepare func()
}

func (source *preparationSource) Activate() error {
	source.prepare()
	return source.recordingSourceActivation.Activate()
}

func TestCapturePreparationDoesNotSpendActiveHeartbeatBudget(t *testing.T) {
	for _, delay := range []time.Duration{250 * time.Millisecond, time.Second, 1900 * time.Millisecond, 2100 * time.Millisecond} {
		t.Run(delay.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				runtime := newLocalRuntime(t)
				writer := &recordingFrameWriter{}
				coordinator := testWriteCoordinator(t, writer)
				source := &preparationSource{prepare: func() {
					if watch := runtime.LeaseWatch(); watch.State != control.StatePending {
						t.Fatalf("preparation already active: %+v", watch)
					}
					time.Sleep(delay)
				}}
				controller, _ := NewOutboundController(coordinator, runtime, source)
				if err := controller.Begin(context.Background(), "session", "epoch", control.ScreenEdgeRight); err != nil {
					t.Fatal(err)
				}
				grant := transport.ControlCommand{Kind: transport.ControlLeaseGrant, SessionID: "session", LeaseID: "lease", Epoch: "epoch"}
				if err := controller.GrantPeer(context.Background(), grant, time.Now()); err != nil {
					t.Fatal(err)
				}
				if snapshot := runtime.Snapshot(); !snapshot.Liveness.LastObserved.Equal(time.Now()) {
					t.Fatalf("capture consumed heartbeat budget: %+v", snapshot.Liveness)
				}
				if _, err := PulseLiveness(context.Background(), coordinator, runtime, time.Now, 2*time.Second); err != nil {
					t.Fatalf("fresh lease expired: %v", err)
				}
				time.Sleep(2 * time.Second)
				if _, err := PulseLiveness(context.Background(), coordinator, runtime, time.Now, 2*time.Second); !errors.Is(err, ErrPeerLivenessExpired) {
					t.Fatalf("real expiry was extended: %v", err)
				}
			})
		})
	}
}

func TestLateCaptureSuccessCannotReviveExpiredOrCanceledPreparation(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "canceled", true: "expired"}[expired], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				runtime := newLocalRuntime(t)
				writer := &recordingFrameWriter{}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				source := &preparationSource{prepare: func() {
					if expired {
						time.Sleep(application.LeaseHandshakeTimeout)
					} else {
						cancel()
					}
				}}
				controller, _ := NewOutboundController(testWriteCoordinator(t, writer), runtime, source)
				if err := controller.Begin(ctx, "session", "epoch", control.ScreenEdgeRight); err != nil {
					t.Fatal(err)
				}
				err := controller.GrantPeer(ctx, transport.ControlCommand{Kind: transport.ControlLeaseGrant, SessionID: "session", LeaseID: "lease", Epoch: "epoch"}, time.Now())
				cause := error(context.Canceled)
				if expired {
					cause = application.ErrPendingLeaseExpired
				}
				if !errors.Is(err, cause) || source.Active() || runtime.Snapshot().Control.State != control.StateLocal {
					t.Fatalf("late success revived lease: %v, active=%t, state=%s", err, source.Active(), runtime.Snapshot().Control.State)
				}
				for _, frame := range writer.frames {
					command, err := transport.DecodeControlPayload(frame.Payload)
					if err == nil && command.Kind == transport.ControlLeaseCommit {
						t.Fatal("expired preparation committed")
					}
				}
			})
		})
	}
}
